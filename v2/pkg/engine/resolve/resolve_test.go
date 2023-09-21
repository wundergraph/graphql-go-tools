package resolve

// go:generate mockgen -package resolve -destination resolve_mock_test.go . DataSource,BeforeFetchHook,AfterFetchHook,DataSourceBatch,DataSourceBatchFactory

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type _fakeDataSource struct {
	t                 TestingTB
	input             []byte
	data              []byte
	artificialLatency time.Duration
}

func (f *_fakeDataSource) Load(ctx context.Context, input []byte, w io.Writer) (err error) {
	if f.artificialLatency != 0 {
		time.Sleep(f.artificialLatency)
	}
	if f.input != nil {
		if !bytes.Equal(f.input, input) {
			require.Equal(f.t, string(f.input), string(input), "input mismatch")
		}
	}
	_, err = w.Write(f.data)
	return
}

func FakeDataSource(data string) *_fakeDataSource {
	return &_fakeDataSource{
		data: []byte(data),
	}
}

func fakeDataSourceWithInputCheck(t TestingTB, input []byte, data []byte) *_fakeDataSource {
	return &_fakeDataSource{
		t:     t,
		input: input,
		data:  data,
	}
}

func newResolver(ctx context.Context, enableSingleFlight bool) *Resolver {
	return New(ctx, enableSingleFlight)
}

type customResolver struct{}

func (customResolver) Resolve(value []byte) ([]byte, error) {
	return value, nil
}

type customErrResolve struct{}

func (customErrResolve) Resolve(value []byte) ([]byte, error) {
	return nil, errors.New("custom error")
}

func TestResolver_ResolveNode(t *testing.T) {
	testFn := func(enableSingleFlight bool, fn func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string)) func(t *testing.T) {
		ctrl := gomock.NewController(t)
		rCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := newResolver(rCtx, enableSingleFlight)
		node, ctx, expectedOutput := fn(t, ctrl)
		if t.Skipped() {
			return func(t *testing.T) {}
		}

		return func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := r.ResolveGraphQLResponse(&ctx, &GraphQLResponse{
				Data: node,
			}, nil, buf)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutput, buf.String())
			ctrl.Finish()
		}
	}

	testErrFn := func(fn func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedErr string)) func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		c, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := newResolver(c, false)
		node, ctx, expectedErr := fn(t, r, ctrl)
		return func(t *testing.T) {
			t.Helper()
			buf := &bytes.Buffer{}
			err := r.ResolveGraphQLResponse(&ctx, &GraphQLResponse{
				Data: node,
			}, nil, buf)
			assert.EqualError(t, err, expectedErr)
			ctrl.Finish()
		}
	}

	testGraphQLErrFn := func(fn func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedErr string)) func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		c, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := newResolver(c, false)
		node, ctx, expectedErr := fn(t, r, ctrl)
		return func(t *testing.T) {
			t.Helper()
			buf := &bytes.Buffer{}
			err := r.ResolveGraphQLResponse(&ctx, &GraphQLResponse{
				Data: node,
			}, nil, buf)
			assert.NoError(t, err)
			assert.Equal(t, expectedErr, buf.String())
			ctrl.Finish()
		}
	}

	t.Run("Nullable empty object", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Nullable: true,
		}, Context{ctx: context.Background()}, `{"data":null}`
	}))
	t.Run("empty object", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &EmptyObject{}, Context{ctx: context.Background()}, `{"data":{}}`
	}))
	t.Run("BigInt", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fetch: &SingleFetch{
				DataSource: FakeDataSource(`{"n": 12345, "ns_small": "12346", "ns_big": "1152921504606846976"`),
			},
			Fields: []*Field{
				{
					Name: []byte("n"),
					Value: &BigInt{
						Path:     []string{"n"},
						Nullable: false,
					},
				},
				{
					Name: []byte("ns_small"),
					Value: &BigInt{
						Path:     []string{"ns_small"},
						Nullable: false,
					},
				},
				{
					Name: []byte("ns_big"),
					Value: &BigInt{
						Path:     []string{"ns_big"},
						Nullable: false,
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"data":{"n":12345,"ns_small":"12346","ns_big":"1152921504606846976"}}`
	}))
	t.Run("Scalar", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fetch: &SingleFetch{
				DataSource: FakeDataSource(`{"int": 12345, "float": 3.5, "int_str": "12346", "bigint_str": "1152921504606846976", "str":"value", "object":{"foo": "bar"}, "encoded_object": "{\"foo\": \"bar\"}"}`),
			},
			Fields: []*Field{
				{
					Name: []byte("int"),
					Value: &Scalar{
						Path:     []string{"int"},
						Nullable: false,
					},
				},
				{
					Name: []byte("float"),
					Value: &Scalar{
						Path:     []string{"float"},
						Nullable: false,
					},
				},
				{
					Name: []byte("int_str"),
					Value: &Scalar{
						Path:     []string{"int_str"},
						Nullable: false,
					},
				},
				{
					Name: []byte("bigint_str"),
					Value: &Scalar{
						Path:     []string{"bigint_str"},
						Nullable: false,
					},
				},
				{
					Name: []byte("str"),
					Value: &Scalar{
						Path:     []string{"str"},
						Nullable: false,
					},
				},
				{
					Name: []byte("object"),
					Value: &Scalar{
						Path:     []string{"object"},
						Nullable: false,
					},
				},
				{
					Name: []byte("encoded_object"),
					Value: &Scalar{
						Path:     []string{"encoded_object"},
						Nullable: false,
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"data":{"int":12345,"float":3.5,"int_str":"12346","bigint_str":"1152921504606846976","str":"value","object":{"foo": "bar"},"encoded_object":"{\"foo\": \"bar\"}"}}`
	}))
	t.Run("object with null field", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name:  []byte("foo"),
					Value: &Null{},
				},
			},
		}, Context{ctx: context.Background()}, `{"data":{"foo":null}}`
	}))
	t.Run("default graphql object", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name:  []byte("data"),
					Value: &Null{},
				},
			},
		}, Context{ctx: context.Background()}, `{"data":{"data":null}}`
	}))
	t.Run("graphql object with simple data source", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("user"),
					Value: &Object{
						Fetch: &SingleFetch{
							DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
						},
						Fields: []*Field{
							{
								Name: []byte("id"),
								Value: &String{
									Path: []string{"id"},
								},
							},
							{
								Name: []byte("name"),
								Value: &String{
									Path: []string{"name"},
								},
							},
							{
								Name: []byte("registered"),
								Value: &Boolean{
									Path: []string{"registered"},
								},
							},
							{
								Name: []byte("pet"),
								Value: &Object{
									Path: []string{"pet"},
									Fields: []*Field{
										{
											Name: []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											Name: []byte("kind"),
											Value: &String{
												Path: []string{"kind"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}}}`
	}))
	t.Run("skip single field should resolve to empty response", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("user"),
					Value: &Object{
						Fetch: &SingleFetch{
							DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
						},
						Fields: []*Field{
							{
								Name: []byte("id"),
								Value: &String{
									Path: []string{"id"},
								},
								SkipDirectiveDefined: true,
								SkipVariableName:     "skip",
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"skip":true}`)}, `{"data":{"user":{}}}`
	}))
	t.Run("skip multiple fields should resolve to empty response", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("user"),
					Value: &Object{
						Fetch: &SingleFetch{
							DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
						},
						Fields: []*Field{
							{
								Name: []byte("id"),
								Value: &String{
									Path: []string{"id"},
								},
								SkipDirectiveDefined: true,
								SkipVariableName:     "skip",
							},
							{
								Name: []byte("name"),
								Value: &String{
									Path: []string{"name"},
								},
								SkipDirectiveDefined: true,
								SkipVariableName:     "skip",
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"skip":true}`)}, `{"data":{"user":{}}}`
	}))
	t.Run("skip __typename field be possible", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("user"),
					Value: &Object{
						Fetch: &SingleFetch{
							DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
						},
						Fields: []*Field{
							{
								Name: []byte("id"),
								Value: &String{
									Path: []string{"id"},
								},
							},
							{
								Name: []byte("__typename"),
								Value: &String{
									Path: []string{"__typename"},
								},
								SkipDirectiveDefined: true,
								SkipVariableName:     "skip",
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"skip":true}`)}, `{"data":{"user":{"id":"1"}}}`
	}))
	t.Run("include __typename field be possible", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("user"),
					Value: &Object{
						Fetch: &SingleFetch{
							DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"},"__typename":"User"}`),
						},
						Fields: []*Field{
							{
								Name: []byte("id"),
								Value: &String{
									Path: []string{"id"},
								},
							},
							{
								Name: []byte("__typename"),
								Value: &String{
									Path: []string{"__typename"},
								},
								IncludeDirectiveDefined: true,
								IncludeVariableName:     "include",
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"include":true}`)}, `{"data":{"user":{"id":"1","__typename":"User"}}}`
	}))
	t.Run("include __typename field with false value", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("user"),
					Value: &Object{
						Fetch: &SingleFetch{
							DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"},"__typename":"User"}`),
						},
						Fields: []*Field{
							{
								Name: []byte("id"),
								Value: &String{
									Path: []string{"id"},
								},
							},
							{
								Name: []byte("__typename"),
								Value: &String{
									Path: []string{"__typename"},
								},
								IncludeDirectiveDefined: true,
								IncludeVariableName:     "include",
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"include":false}`)}, `{"data":{"user":{"id":"1"}}}`
	}))
	t.Run("skip field when skip variable is true", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("user"),
					Value: &Object{
						Fetch: &SingleFetch{
							DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
						},
						Fields: []*Field{
							{
								Name: []byte("id"),
								Value: &String{
									Path: []string{"id"},
								},
							},
							{
								Name: []byte("name"),
								Value: &String{
									Path: []string{"name"},
								},
							},
							{
								Name: []byte("registered"),
								Value: &Boolean{
									Path: []string{"registered"},
								},
							},
							{
								Name: []byte("pet"),
								Value: &Object{
									Path: []string{"pet"},
									Fields: []*Field{
										{
											Name: []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											Name: []byte("kind"),
											Value: &String{
												Path: []string{"kind"},
											},
											SkipDirectiveDefined: true,
											SkipVariableName:     "skip",
										},
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"skip":true}`)}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky"}}}}`
	}))
	t.Run("don't skip field when skip variable is false", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("user"),
					Value: &Object{
						Fetch: &SingleFetch{
							DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
						},
						Fields: []*Field{
							{
								Name: []byte("id"),
								Value: &String{
									Path: []string{"id"},
								},
							},
							{
								Name: []byte("name"),
								Value: &String{
									Path: []string{"name"},
								},
							},
							{
								Name: []byte("registered"),
								Value: &Boolean{
									Path: []string{"registered"},
								},
							},
							{
								Name: []byte("pet"),
								Value: &Object{
									Path: []string{"pet"},
									Fields: []*Field{
										{
											Name: []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											Name: []byte("kind"),
											Value: &String{
												Path: []string{"kind"},
											},
											SkipDirectiveDefined: true,
											SkipVariableName:     "skip",
										},
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"skip":false}`)}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}}}`
	}))
	t.Run("don't skip field when skip variable is missing", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("user"),
					Value: &Object{
						Fetch: &SingleFetch{
							DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
						},
						Fields: []*Field{
							{
								Name: []byte("id"),
								Value: &String{
									Path: []string{"id"},
								},
							},
							{
								Name: []byte("name"),
								Value: &String{
									Path: []string{"name"},
								},
							},
							{
								Name: []byte("registered"),
								Value: &Boolean{
									Path: []string{"registered"},
								},
							},
							{
								Name: []byte("pet"),
								Value: &Object{
									Path: []string{"pet"},
									Fields: []*Field{
										{
											Name: []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											Name: []byte("kind"),
											Value: &String{
												Path: []string{"kind"},
											},
											SkipDirectiveDefined: true,
											SkipVariableName:     "skip",
										},
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{}`)}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}}}`
	}))
	t.Run("include field when include variable is true", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("user"),
					Value: &Object{
						Fetch: &SingleFetch{
							DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
						},
						Fields: []*Field{
							{
								Name: []byte("id"),
								Value: &String{
									Path: []string{"id"},
								},
							},
							{
								Name: []byte("name"),
								Value: &String{
									Path: []string{"name"},
								},
							},
							{
								Name: []byte("registered"),
								Value: &Boolean{
									Path: []string{"registered"},
								},
							},
							{
								Name: []byte("pet"),
								Value: &Object{
									Path: []string{"pet"},
									Fields: []*Field{
										{
											Name: []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											Name: []byte("kind"),
											Value: &String{
												Path: []string{"kind"},
											},
											IncludeDirectiveDefined: true,
											IncludeVariableName:     "include",
										},
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"include":true}`)}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}}}`
	}))
	t.Run("exclude field when include variable is false", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("user"),
					Value: &Object{
						Fetch: &SingleFetch{
							DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
						},
						Fields: []*Field{
							{
								Name: []byte("id"),
								Value: &String{
									Path: []string{"id"},
								},
							},
							{
								Name: []byte("name"),
								Value: &String{
									Path: []string{"name"},
								},
							},
							{
								Name: []byte("registered"),
								Value: &Boolean{
									Path: []string{"registered"},
								},
							},
							{
								Name: []byte("pet"),
								Value: &Object{
									Path: []string{"pet"},
									Fields: []*Field{
										{
											Name: []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											Name: []byte("kind"),
											Value: &String{
												Path: []string{"kind"},
											},
											IncludeDirectiveDefined: true,
											IncludeVariableName:     "include",
										},
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"include":false}`)}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky"}}}}`
	}))
	t.Run("exclude field when include variable is missing", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("user"),
					Value: &Object{
						Fetch: &SingleFetch{
							DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
						},
						Fields: []*Field{
							{
								Name: []byte("id"),
								Value: &String{
									Path: []string{"id"},
								},
							},
							{
								Name: []byte("name"),
								Value: &String{
									Path: []string{"name"},
								},
							},
							{
								Name: []byte("registered"),
								Value: &Boolean{
									Path: []string{"registered"},
								},
							},
							{
								Name: []byte("pet"),
								Value: &Object{
									Path: []string{"pet"},
									Fields: []*Field{
										{
											Name: []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											Name: []byte("kind"),
											Value: &String{
												Path: []string{"kind"},
											},
											IncludeDirectiveDefined: true,
											IncludeVariableName:     "include",
										},
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{}`)}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky"}}}}`
	}))
	t.Run("fetch with context variable resolver", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), []byte(`{"id":1}`), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			Do(func(ctx context.Context, input []byte, w *bytes.Buffer) (err error) {
				_, err = w.Write([]byte(`{"name":"Jens"}`))
				return
			}).
			Return(nil)
		return &Object{
			Fetch: &SingleFetch{
				DataSource: mockDataSource,
				Input:      `{"id":$$0$$}`,
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							SegmentType: StaticSegmentType,
							Data:        []byte(`{"id":`),
						},
						{
							SegmentType:        VariableSegmentType,
							VariableKind:       ContextVariableKind,
							VariableSourcePath: []string{"id"},
							Renderer:           NewPlainVariableRendererWithValidation(`{"type":"number"}`),
						},
						{
							SegmentType: StaticSegmentType,
							Data:        []byte(`}`),
						},
					},
				},
				Variables: NewVariables(&ContextVariable{
					Path: []string{"id"},
				}),
			},
			Fields: []*Field{
				{
					Name: []byte("name"),
					Value: &String{
						Path: []string{"name"},
					},
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"id":1}`)}, `{"data":{"name":"Jens"}}`
	}))
	t.Run("resolve array of strings", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fetch: &SingleFetch{
				DataSource: FakeDataSource(`{"strings": ["Alex", "true", "123"]}`),
			},
			Fields: []*Field{
				{
					Name: []byte("strings"),
					Value: &Array{
						Path: []string{"strings"},
						Item: &String{
							Nullable: false,
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"data":{"strings":["Alex","true","123"]}}`
	}))
	t.Run("resolve array of mixed scalar types", testErrFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedErr string) {
		return &Object{
			Fetch: &SingleFetch{
				DataSource: FakeDataSource(`{"strings": ["Alex", "true", 123]}`),
			},
			Fields: []*Field{
				{
					Name: []byte("strings"),
					Value: &Array{
						Path: []string{"strings"},
						Item: &String{
							Nullable: false,
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `invalid value type 'number' for path /data/strings/2, expecting string, got: 123. You can fix this by configuring this field as Int/Float/JSON Scalar`
	}))
	t.Run("resolve array items", func(t *testing.T) {
		t.Run("with unescape json enabled", func(t *testing.T) {
			t.Run("json encoded input", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						DataSource: FakeDataSource(`{"jsonList":["{\"field\":\"value\"}"]}`),
					},
					Fields: []*Field{
						{
							Name: []byte("jsonList"),
							Value: &Array{
								Path: []string{"jsonList"},
								Item: &String{
									Nullable:             false,
									UnescapeResponseJson: true,
								},
							},
						},
					},
				}, Context{ctx: context.Background()}, `{"data":{"jsonList":[{"field":"value"}]}}`
			}))
			t.Run("json input", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						DataSource: FakeDataSource(`{"jsonList":[{"field":"value"}]}`),
					},
					Fields: []*Field{
						{
							Name: []byte("jsonList"),
							Value: &Array{
								Path: []string{"jsonList"},
								Item: &String{
									Nullable:             false,
									UnescapeResponseJson: true,
								},
							},
						},
					},
				}, Context{ctx: context.Background()}, `{"data":{"jsonList":[{"field":"value"}]}}`
			}))
		})
		t.Run("with unescape json disabled", func(t *testing.T) {
			t.Run("json encoded input", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						DataSource: FakeDataSource(`{"jsonList":["{\"field\":\"value\"}"]}`),
					},
					Fields: []*Field{
						{
							Name: []byte("jsonList"),
							Value: &Array{
								Path: []string{"jsonList"},
								Item: &String{
									Nullable:             false,
									UnescapeResponseJson: false,
								},
							},
						},
					},
				}, Context{ctx: context.Background()}, `{"data":{"jsonList":["{\"field\":\"value\"}"]}}`
			}))
			t.Run("json input", testErrFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedErr string) {
				return &Object{
						Fetch: &SingleFetch{
							DataSource: FakeDataSource(`{"jsonList":[{"field":"value"}]}`),
						},
						Fields: []*Field{
							{
								Name: []byte("jsonList"),
								Value: &Array{
									Path: []string{"jsonList"},
									Item: &String{
										Nullable:             false,
										UnescapeResponseJson: false,
									},
								},
							},
						},
					}, Context{ctx: context.Background()},
					`invalid value type 'object' for path /data/jsonList/0, expecting string, got: {"field":"value"}. You can fix this by configuring this field as Int/Float/JSON Scalar`
			}))
		})
	})
	t.Run("resolve arrays", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fetch: &SingleFetch{
				DataSource: FakeDataSource(`{"friends":[{"id":1,"name":"Alex"},{"id":2,"name":"Patric"}],"strings":["foo","bar","baz"],"integers":[123,456,789],"floats":[1.2,3.4,5.6],"booleans":[true,false,true]}`),
			},
			Fields: []*Field{
				{
					Name: []byte("synchronousFriends"),
					Value: &Array{
						Path:                []string{"friends"},
						ResolveAsynchronous: false,
						Nullable:            true,
						Item: &Object{
							Fields: []*Field{
								{
									Name: []byte("id"),
									Value: &Integer{
										Path: []string{"id"},
									},
								},
								{
									Name: []byte("name"),
									Value: &String{
										Path: []string{"name"},
									},
								},
							},
						},
					},
				},
				{
					Name: []byte("asynchronousFriends"),
					Value: &Array{
						Path:                []string{"friends"},
						ResolveAsynchronous: true,
						Nullable:            true,
						Item: &Object{
							Fields: []*Field{
								{
									Name: []byte("id"),
									Value: &Integer{
										Path: []string{"id"},
									},
								},
								{
									Name: []byte("name"),
									Value: &String{
										Path: []string{"name"},
									},
								},
							},
						},
					},
				},
				{
					Name: []byte("nullableFriends"),
					Value: &Array{
						Path:     []string{"nonExistingField"},
						Nullable: true,
						Item:     &Object{},
					},
				},
				{
					Name: []byte("strings"),
					Value: &Array{
						Path:                []string{"strings"},
						ResolveAsynchronous: false,
						Nullable:            true,
						Item: &String{
							Nullable: false,
						},
					},
				},
				{
					Name: []byte("integers"),
					Value: &Array{
						Path:                []string{"integers"},
						ResolveAsynchronous: false,
						Nullable:            true,
						Item: &Integer{
							Nullable: false,
						},
					},
				},
				{
					Name: []byte("floats"),
					Value: &Array{
						Path:                []string{"floats"},
						ResolveAsynchronous: false,
						Nullable:            true,
						Item: &Float{
							Nullable: false,
						},
					},
				},
				{
					Name: []byte("booleans"),
					Value: &Array{
						Path:                []string{"booleans"},
						ResolveAsynchronous: false,
						Nullable:            true,
						Item: &Boolean{
							Nullable: false,
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"data":{"synchronousFriends":[{"id":1,"name":"Alex"},{"id":2,"name":"Patric"}],"asynchronousFriends":[{"id":1,"name":"Alex"},{"id":2,"name":"Patric"}],"nullableFriends":null,"strings":["foo","bar","baz"],"integers":[123,456,789],"floats":[1.2,3.4,5.6],"booleans":[true,false,true]}}`
	}))
	t.Run("array response from data source", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
				Fetch: &SingleFetch{
					DataSource: FakeDataSource(`[{"__typename":"Dog","name":"Woofie"},{"__typename":"Cat","name":"Mietzie"}]`),
				},
				Fields: []*Field{
					{
						Name: []byte("pets"),
						Value: &Array{
							Item: &Object{
								Fields: []*Field{
									{
										OnTypeNames: [][]byte{[]byte("Dog")},
										Name:        []byte("name"),
										Value: &String{
											Path: []string{"name"},
										},
									},
								},
							},
						},
					},
				},
			}, Context{ctx: context.Background()},
			`{"data":{"pets":[{"name":"Woofie"},{}]}}`
	}))
	t.Run("non null object with field condition can be null", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
				Fetch: &SingleFetch{
					DataSource: FakeDataSource(`{"__typename":"Dog","name":"Woofie"}`),
				},
				Fields: []*Field{
					{
						Name: []byte("cat"),
						Value: &Object{
							Nullable: false,
							Fields: []*Field{
								{
									OnTypeNames: [][]byte{[]byte("Cat")},
									Name:        []byte("name"),
									Value: &String{
										Path: []string{"name"},
									},
								},
							},
						},
					},
				},
			}, Context{ctx: context.Background()},
			`{"data":{"cat":{}}}`
	}))
	t.Run("object with multiple type conditions", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
				Fetch: &SingleFetch{
					DataSource: FakeDataSource(`{"namespaceCreate":{"__typename":"Error","code":"UserAlreadyHasPersonalNamespace","message":""}}`),
				},
				Fields: []*Field{
					{
						Name: []byte("namespaceCreate"),
						Value: &Object{
							Path: []string{"namespaceCreate"},
							Fields: []*Field{
								{
									Name:        []byte("namespace"),
									OnTypeNames: [][]byte{[]byte("NamespaceCreated")},
									Value: &Object{
										Path:     []string{"namespace"},
										Nullable: false,
										Fields: []*Field{
											{
												Name: []byte("id"),
												Value: &String{
													Nullable: false,
													Path:     []string{"id"},
												},
											},
											{
												Name: []byte("name"),
												Value: &String{
													Nullable: false,
													Path:     []string{"name"},
												},
											},
										},
									},
								},
								{
									Name:        []byte("code"),
									OnTypeNames: [][]byte{[]byte("Error")},
									Value: &String{
										Nullable: false,
										Path:     []string{"code"},
									},
								},
								{
									Name:        []byte("message"),
									OnTypeNames: [][]byte{[]byte("Error")},
									Value: &String{
										Nullable: false,
										Path:     []string{"message"},
									},
								},
							},
						},
					},
				},
			}, Context{ctx: context.Background()},
			`{"data":{"namespaceCreate":{"code":"UserAlreadyHasPersonalNamespace","message":""}}}`
	}))
	t.Run("resolve fieldsets based on __typename", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
				Fetch: &SingleFetch{
					DataSource: FakeDataSource(`{"pets":[{"__typename":"Dog","name":"Woofie"},{"__typename":"Cat","name":"Mietzie"}]}`),
				},
				Fields: []*Field{
					{
						Name: []byte("pets"),
						Value: &Array{
							Path: []string{"pets"},
							Item: &Object{
								Fields: []*Field{
									{
										OnTypeNames: [][]byte{[]byte("Dog")},
										Name:        []byte("name"),
										Value: &String{
											Path: []string{"name"},
										},
									},
								},
							},
						},
					},
				},
			}, Context{ctx: context.Background()},
			`{"data":{"pets":[{"name":"Woofie"},{}]}}`
	}))

	t.Run("resolve fieldsets based on __typename when field is Nullable", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
				Fetch: &SingleFetch{
					DataSource: FakeDataSource(`{"pet":{"id": "1", "detail": null}}`),
				},
				Fields: []*Field{
					{
						Name: []byte("pet"),
						Value: &Object{
							Path: []string{"pet"},
							Fields: []*Field{
								{
									Name: []byte("id"),
									Value: &String{
										Path: []string{"id"},
									},
								},
								{
									Name: []byte("detail"),
									Value: &Object{
										Path:     []string{"detail"},
										Nullable: true,
										Fields: []*Field{
											{
												OnTypeNames: [][]byte{[]byte("Dog")},
												Name:        []byte("name"),
												Value: &String{
													Path: []string{"name"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}, Context{ctx: context.Background()},
			`{"data":{"pet":{"id":"1","detail":null}}}`
	}))

	t.Run("resolve fieldsets asynchronous based on __typename", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
				Fetch: &SingleFetch{
					DataSource: FakeDataSource(`{"pets":[{"__typename":"Dog","name":"Woofie"},{"__typename":"Cat","name":"Mietzie"}]}`),
				},
				Fields: []*Field{
					{
						Name: []byte("pets"),
						Value: &Array{
							ResolveAsynchronous: true,
							Path:                []string{"pets"},
							Item: &Object{
								Fields: []*Field{
									{
										OnTypeNames: [][]byte{[]byte("Dog")},
										Name:        []byte("name"),
										Value: &String{
											Path: []string{"name"},
										},
									},
								},
							},
						},
					},
				},
			}, Context{ctx: context.Background()},
			`{"data":{"pets":[{"name":"Woofie"},{}]}}`
	}))
	t.Run("with unescape json enabled", func(t *testing.T) {
		t.Run("json object within a string", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
			return &Object{
				Fetch: &SingleFetch{
					// Datasource returns a JSON object within a string
					DataSource: FakeDataSource(`{"data":"{\"hello\":\"world\",\"numberAsString\":\"1\",\"number\":1,\"bool\":true,\"null\":null,\"array\":[1,2,3],\"object\":{\"key\":\"value\"}}"}`),
				},
				Nullable: false,
				Fields: []*Field{
					{
						Name: []byte("data"),
						// Value is a string and unescape json is enabled
						Value: &String{
							Path:                 []string{"data"},
							Nullable:             true,
							UnescapeResponseJson: true,
							IsTypeName:           false,
						},
						Position: Position{
							Line:   2,
							Column: 3,
						},
					},
				},
				// expected output is a JSON object
			}, Context{ctx: context.Background()}, `{"data":{"data":{"hello":"world","numberAsString":"1","number":1,"bool":true,"null":null,"array":[1,2,3],"object":{"key":"value"}}}}`
		}))
		t.Run("json array within a string", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
			return &Object{
				Fetch: &SingleFetch{
					// Datasource returns a JSON array within a string
					DataSource: FakeDataSource(`{"data":"[1,2,3]"}`),
				},
				Nullable: false,
				Fields: []*Field{
					{
						Name: []byte("data"),
						// Value is a string and unescape json is enabled
						Value: &String{
							Path:                 []string{"data"},
							Nullable:             true,
							UnescapeResponseJson: true,
							IsTypeName:           false,
						},
						Position: Position{
							Line:   2,
							Column: 3,
						},
					},
				},
				// expected output is a JSON array
			}, Context{ctx: context.Background()}, `{"data":{"data":[1,2,3]}}`
		}))
		t.Run("string with array and objects brackets", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
			return &Object{
				Fetch: &SingleFetch{
					// Datasource returns a string with array and object brackets
					DataSource: FakeDataSource(`{"data":"hi[1beep{2}]"}`),
				},
				Nullable: false,
				Fields: []*Field{
					{
						Name: []byte("data"),
						// Value is a string and unescape json is enabled
						Value: &String{
							Path:                 []string{"data"},
							Nullable:             true,
							UnescapeResponseJson: true,
							IsTypeName:           false,
						},
						Position: Position{
							Line:   2,
							Column: 3,
						},
					},
				},
				// expected output is a string
			}, Context{ctx: context.Background()}, `{"data":{"data":"hi[1beep{2}]"}}`
		}))
		t.Run("plain scalar values within a string", func(t *testing.T) {
			t.Run("boolean", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						// Datasource returns a JSON boolean within a string
						DataSource: FakeDataSource(`{"data": "true"}`),
					},
					Nullable: false,
					Fields: []*Field{
						{
							Name: []byte("data"),
							// Value is a string and unescape json is enabled
							Value: &String{
								Path:                 []string{"data"},
								Nullable:             true,
								UnescapeResponseJson: true,
								IsTypeName:           false,
							},
						},
					},
					// expected output is a string
				}, Context{ctx: context.Background()}, `{"data":{"data":"true"}}`
			}))
			t.Run("int", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						// Datasource returns a JSON number within a string
						DataSource: FakeDataSource(`{"data": "1"}`),
					},
					Nullable: false,
					Fields: []*Field{
						{
							Name: []byte("data"),
							// Value is a string and unescape json is enabled
							Value: &String{
								Path:                 []string{"data"},
								Nullable:             true,
								UnescapeResponseJson: true,
								IsTypeName:           false,
							},
							Position: Position{
								Line:   2,
								Column: 3,
							},
						},
					},
					// expected output is a string
				}, Context{ctx: context.Background()}, `{"data":{"data":"1"}}`
			}))
			t.Run("float", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						// Datasource returns a JSON number within a string
						DataSource: FakeDataSource(`{"data": "2.0"}`),
					},
					Nullable: false,
					Fields: []*Field{
						{
							Name: []byte("data"),
							// Value is a string and unescape json is enabled
							Value: &String{
								Path:                 []string{"data"},
								Nullable:             true,
								UnescapeResponseJson: true,
								IsTypeName:           false,
							},
							Position: Position{
								Line:   2,
								Column: 3,
							},
						},
					},
					// expected output is a string
				}, Context{ctx: context.Background()}, `{"data":{"data":"2.0"}}`
			}))
			t.Run("null", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						// Datasource returns a JSON number within a string
						DataSource: FakeDataSource(`{"data": "null"}`),
					},
					Nullable: false,
					Fields: []*Field{
						{
							Name: []byte("data"),
							// Value is a string and unescape json is enabled
							Value: &String{
								Path:                 []string{"data"},
								Nullable:             true,
								UnescapeResponseJson: true,
								IsTypeName:           false,
							},
							Position: Position{
								Line:   2,
								Column: 3,
							},
						},
					},
					// expected output is a string
				}, Context{ctx: context.Background()}, `{"data":{"data":"null"}}`
			}))
			t.Run("string", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						DataSource: FakeDataSource(`{"data": "hello world"}`),
					},
					Nullable: false,
					Fields: []*Field{
						{
							Name: []byte("data"),
							// Value is a string and unescape json is enabled
							Value: &String{
								Path:                 []string{"data"},
								Nullable:             true,
								UnescapeResponseJson: true,
								IsTypeName:           false,
							},
							Position: Position{
								Line:   2,
								Column: 3,
							},
						},
					},
					// expect data value to be valid JSON string
				}, Context{ctx: context.Background()}, `{"data":{"data":"hello world"}}`
			}))
		})
		t.Run("plain scalar values as is", func(t *testing.T) {
			t.Run("boolean", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						// Datasource returns a JSON boolean within a string
						DataSource: FakeDataSource(`{"data": true}`),
					},
					Nullable: false,
					Fields: []*Field{
						{
							Name: []byte("data"),
							// Value is a string and unescape json is enabled
							Value: &String{
								Path:                 []string{"data"},
								Nullable:             true,
								UnescapeResponseJson: true,
								IsTypeName:           false,
							},
						},
					},
					// expected output is a JSON boolean
				}, Context{ctx: context.Background()}, `{"data":{"data":true}}`
			}))
			t.Run("int", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						// Datasource returns a JSON number within a string
						DataSource: FakeDataSource(`{"data": 1}`),
					},
					Nullable: false,
					Fields: []*Field{
						{
							Name: []byte("data"),
							// Value is a string and unescape json is enabled
							Value: &String{
								Path:                 []string{"data"},
								Nullable:             true,
								UnescapeResponseJson: true,
								IsTypeName:           false,
							},
							Position: Position{
								Line:   2,
								Column: 3,
							},
						},
					},
					// expected output is a JSON boolean
				}, Context{ctx: context.Background()}, `{"data":{"data":1}}`
			}))
			t.Run("float", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						// Datasource returns a JSON number within a string
						DataSource: FakeDataSource(`{"data": 2.0}`),
					},
					Nullable: false,
					Fields: []*Field{
						{
							Name: []byte("data"),
							// Value is a string and unescape json is enabled
							Value: &String{
								Path:                 []string{"data"},
								Nullable:             true,
								UnescapeResponseJson: true,
								IsTypeName:           false,
							},
							Position: Position{
								Line:   2,
								Column: 3,
							},
						},
					},
					// expected output is a JSON boolean
				}, Context{ctx: context.Background()}, `{"data":{"data":2.0}}`
			}))
			t.Run("null", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						DataSource: FakeDataSource(`{"data": null}`),
					},
					Nullable: false,
					Fields: []*Field{
						{
							Name: []byte("data"),
							// Value is a string and unescape json is enabled
							Value: &String{
								Path:                 []string{"data"},
								Nullable:             true,
								UnescapeResponseJson: true,
								IsTypeName:           false,
							},
							Position: Position{
								Line:   2,
								Column: 3,
							},
						},
					},
					// expect data value to be valid JSON string
				}, Context{ctx: context.Background()}, `{"data":{"data":null}}`
			}))
		})
	})

	t.Run("custom", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fetch: &SingleFetch{
				DataSource: FakeDataSource(`{"id": "1"}`),
			},
			Fields: []*Field{
				{
					Name: []byte("id"),
					Value: &CustomNode{
						CustomResolve: customResolver{},
						Path:          []string{"id"},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"data":{"id":1}}`
	}))
	t.Run("custom nullable", testGraphQLErrFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedErr string) {
		return &Object{
			Fetch: &SingleFetch{
				DataSource: FakeDataSource(`{"id": null}`),
			},
			Fields: []*Field{
				{
					Name: []byte("id"),
					Value: &CustomNode{
						CustomResolve: customErrResolve{},
						Path:          []string{"id"},
						Nullable:      false,
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"unable to resolve","locations":[{"line":0,"column":0}]}],"data":null}`
	}))
	t.Run("custom error", testErrFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedErr string) {
		return &Object{
			Fetch: &SingleFetch{
				DataSource: FakeDataSource(`{"id": "1"}`),
			},
			Fields: []*Field{
				{
					Name: []byte("id"),
					Value: &CustomNode{
						CustomResolve: customErrResolve{},
						Path:          []string{"id"},
					},
				},
			},
		}, Context{ctx: context.Background()}, `failed to resolve value type string for path /data/id via custom resolver`
	}))
}

func testFn(enableSingleFlight bool, fn func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string)) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		rCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := newResolver(rCtx, enableSingleFlight)
		node, ctx, expectedOutput := fn(t, ctrl)

		if t.Skipped() {
			return
		}

		buf := &bytes.Buffer{}
		err := r.ResolveGraphQLResponse(&ctx, node, nil, buf)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutput, buf.String())
		ctrl.Finish()
	}
}
func testFnWithError(enableSingleFlight bool, fn func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedErrorMessage string)) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		rCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := newResolver(rCtx, enableSingleFlight)
		node, ctx, expectedOutput := fn(t, ctrl)

		if t.Skipped() {
			return
		}

		buf := &bytes.Buffer{}
		err := r.ResolveGraphQLResponse(&ctx, node, nil, buf)
		assert.Error(t, err, expectedOutput)
		ctrl.Finish()
	}
}

func TestResolver_ResolveGraphQLResponse(t *testing.T) {

	t.Run("empty graphql response", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: true,
			},
		}, Context{ctx: context.Background()}, `{"data":null}`
	}))
	t.Run("__typename without renaming", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
							Fetch: &SingleFetch{
								DataSource: FakeDataSource(`{"id":1,"name":"Jannik","__typename":"User","rewritten":"User"}`),
							},
							Fields: []*Field{
								{
									Name: []byte("id"),
									Value: &Integer{
										Path:     []string{"id"},
										Nullable: false,
									},
								},
								{
									Name: []byte("name"),
									Value: &String{
										Path:     []string{"name"},
										Nullable: false,
									},
								},
								{
									Name: []byte("__typename"),
									Value: &String{
										Path:       []string{"__typename"},
										Nullable:   false,
										IsTypeName: true,
									},
								},
								{
									Name: []byte("aliased"),
									Value: &String{
										Path:       []string{"__typename"},
										Nullable:   false,
										IsTypeName: true,
									},
								},
								{
									Name: []byte("rewritten"),
									Value: &String{
										Path:       []string{"rewritten"},
										Nullable:   false,
										IsTypeName: true,
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"data":{"user":{"id":1,"name":"Jannik","__typename":"User","aliased":"User","rewritten":"User"}}}`
	}))
	t.Run("__typename with renaming", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
				Data: &Object{
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Fetch: &SingleFetch{
									DataSource: FakeDataSource(`{"id":1,"name":"Jannik","__typename":"User","rewritten":"User"}`),
								},
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &Integer{
											Path:     []string{"id"},
											Nullable: false,
										},
									},
									{
										Name: []byte("name"),
										Value: &String{
											Path:     []string{"name"},
											Nullable: false,
										},
									},
									{
										Name: []byte("__typename"),
										Value: &String{
											Path:       []string{"__typename"},
											Nullable:   false,
											IsTypeName: true,
										},
									},
									{
										Name: []byte("aliased"),
										Value: &String{
											Path:       []string{"__typename"},
											Nullable:   false,
											IsTypeName: true,
										},
									},
									{
										Name: []byte("rewritten"),
										Value: &String{
											Path:       []string{"rewritten"},
											Nullable:   false,
											IsTypeName: true,
										},
									},
								},
							},
						},
					},
				},
			}, Context{
				ctx: context.Background(),
				RenameTypeNames: []RenameTypeName{
					{
						From: []byte("User"),
						To:   []byte("namespaced_User"),
					},
				},
			}, `{"data":{"user":{"id":1,"name":"Jannik","__typename":"namespaced_User","aliased":"namespaced_User","rewritten":"namespaced_User"}}}`
	}))
	t.Run("empty graphql response for not nullable query field", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: false,
				Fields: []*Field{
					{
						Name: []byte("country"),
						Position: Position{
							Line:   3,
							Column: 4,
						},
						Value: &Object{
							Nullable: false,
							Path:     []string{"country"},
							Fields: []*Field{
								{
									Name: []byte("name"),
									Value: &String{
										Nullable: true,
										Path:     []string{"name"},
									},
									Position: Position{
										Line:   4,
										Column: 5,
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"unable to resolve","locations":[{"line":3,"column":4}],"path":["country"]}],"data":null}`
	}))
	t.Run("fetch with simple error", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				pair := NewBufPair()
				pair.WriteErr([]byte("errorMessage"), nil, nil, nil)
				return writeGraphqlResponse(pair, w, false)
			})
		return &GraphQLResponse{
			Data: &Object{
				Nullable: false,
				Fetch: &SingleFetch{
					DataSource: mockDataSource,
				},
				Fields: []*Field{
					{
						Name: []byte("name"),
						Value: &String{
							Path:     []string{"name"},
							Nullable: true,
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"errorMessage"}]}`
	}))
	t.Run("fetch with two Errors", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				pair := NewBufPair()
				pair.WriteErr([]byte("errorMessage1"), nil, nil, nil)
				pair.WriteErr([]byte("errorMessage2"), nil, nil, nil)
				return writeGraphqlResponse(pair, w, false)
			}).
			Return(nil)
		return &GraphQLResponse{
			Data: &Object{
				Fetch: &SingleFetch{
					DataSource: mockDataSource,
				},
				Fields: []*Field{
					{
						Name: []byte("name"),
						Value: &String{
							Path:     []string{"name"},
							Nullable: true,
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"errorMessage1"},{"message":"errorMessage2"}]}`
	}))
	t.Run("not nullable object in nullable field", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: false,
				Fetch: &SingleFetch{
					DataSource: FakeDataSource(`{"nullable_field": null}`),
				},
				Fields: []*Field{
					{
						Name: []byte("nullableField"),
						Value: &Object{
							Nullable: true,
							Path:     []string{"nullable_field"},
							Fields: []*Field{
								{
									Name: []byte("notNullableField"),
									Value: &Object{
										Nullable: false,
										Path:     []string{"not_nullable_field"},
										Fields: []*Field{
											{
												Name: []byte("someField"),
												Value: &String{
													Nullable: false,
													Path:     []string{"some_field"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"data":{"nullableField":null}}`
	}))

	t.Run("interface response", func(t *testing.T) {
		t.Run("fields nullable", func(t *testing.T) {
			obj := func(fakeData string) *GraphQLResponse {
				return &GraphQLResponse{
					Data: &Object{
						Fetch: &SingleFetch{
							DataSource:           FakeDataSource(fakeData),
							Input:                `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"{thing {id abstractThing {__typename ... on ConcreteOne {name}}}}"}}`,
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*Field{
							{
								Name: []byte("thing"),
								Value: &Object{
									Path:     []string{"thing"},
									Nullable: true,
									Fields: []*Field{
										{
											Name: []byte("id"),
											Value: &String{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("abstractThing"),
											Value: &Object{
												Path:     []string{"abstractThing"},
												Nullable: true,
												Fields: []*Field{
													{
														Name: []byte("name"),
														Value: &String{
															Nullable: true,
															Path:     []string{"name"},
														},
														OnTypeNames: [][]byte{[]byte("ConcreteOne")},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				}
			}

			t.Run("interface response with matching type", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
				return obj(`{"thing":{"id":"1","abstractThing":{"__typename":"ConcreteOne","name":"foo"}}}`),
					Context{ctx: context.Background()},
					`{"data":{"thing":{"id":"1","abstractThing":{"name":"foo"}}}}`
			}))

			t.Run("interface response with not matching type", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
				return obj(`{"thing":{"id":"1","abstractThing":{"__typename":"ConcreteTwo"}}}`),
					Context{ctx: context.Background()},
					`{"data":{"thing":{"id":"1","abstractThing":{}}}}`
			}))
		})

		t.Run("array of not nullable fields", func(t *testing.T) {
			obj := func(fakeData string) *GraphQLResponse {
				return &GraphQLResponse{
					Data: &Object{
						Fetch: &SingleFetch{
							DataSource:           FakeDataSource(fakeData),
							Input:                `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"{things {id abstractThing {__typename ... on ConcreteOne {name}}}}"}}`,
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data"},
							},
						},
						Fields: []*Field{
							{
								Name: []byte("things"),
								Value: &Array{
									Path: []string{"things"},
									Item: &Object{
										Fields: []*Field{
											{
												Name: []byte("id"),
												Value: &String{
													Path: []string{"id"},
												},
											},
											{
												Name: []byte("abstractThing"),
												Value: &Object{
													Path: []string{"abstractThing"},
													Fields: []*Field{
														{
															Name: []byte("name"),
															Value: &String{
																Path: []string{"name"},
															},
															OnTypeNames: [][]byte{[]byte("ConcreteOne")},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				}
			}

			t.Run("interface response with matching type", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
				return obj(`{"data":{"things":[{"id":"1","abstractThing":{"__typename":"ConcreteOne","name":"foo"}}]}}`),
					Context{ctx: context.Background()},
					`{"data":{"things":[{"id":"1","abstractThing":{"name":"foo"}}]}}`
			}))

			t.Run("interface response with not matching type", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
				return obj(`{"data":{"things":[{"id":"1","abstractThing":{"__typename":"ConcreteTwo"}}]}}`),
					Context{ctx: context.Background()},
					`{"data":{"things":[{"id":"1","abstractThing":{}}]}}`
			}))
		})
	})

	t.Run("null field should bubble up to parent with error", testFnWithError(false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: true,
				Fetch: &SingleFetch{
					DataSource: FakeDataSource(`[{"id":1},{"id":2},{"id":3}]`),
				},
				Fields: []*Field{
					{
						Name: []byte("stringObject"),
						Value: &Object{
							Nullable: true,
							Fields: []*Field{
								{
									Name: []byte("stringField"),
									Value: &String{
										Nullable: false,
									},
								},
							},
						},
					},
					{
						Name: []byte("integerObject"),
						Value: &Object{
							Nullable: true,
							Fields: []*Field{
								{
									Name: []byte("integerField"),
									Value: &Integer{
										Nullable: false,
									},
								},
							},
						},
					},
					{
						Name: []byte("floatObject"),
						Value: &Object{
							Nullable: true,
							Fields: []*Field{
								{
									Name: []byte("floatField"),
									Value: &Float{
										Nullable: false,
									},
								},
							},
						},
					},
					{
						Name: []byte("booleanObject"),
						Value: &Object{
							Nullable: true,
							Fields: []*Field{
								{
									Name: []byte("booleanField"),
									Value: &Boolean{
										Nullable: false,
									},
								},
							},
						},
					},
					{
						Name: []byte("objectObject"),
						Value: &Object{
							Nullable: true,
							Fields: []*Field{
								{
									Name: []byte("objectField"),
									Value: &Object{
										Nullable: false,
									},
								},
							},
						},
					},
					{
						Name: []byte("arrayObject"),
						Value: &Object{
							Nullable: true,
							Fields: []*Field{
								{
									Name: []byte("arrayField"),
									Value: &Array{
										Nullable: false,
										Item: &String{
											Nullable: false,
											Path:     []string{"nonExisting"},
										},
									},
								},
							},
						},
					},
					{
						Name: []byte("asynchronousArrayObject"),
						Value: &Object{
							Nullable: true,
							Fields: []*Field{
								{
									Name: []byte("arrayField"),
									Value: &Array{
										Nullable:            false,
										ResolveAsynchronous: true,
										Item: &String{
											Nullable: false,
											Path:     []string{"nonExisting"},
										},
									},
								},
							},
						},
					},
					{
						Name: []byte("nullableArray"),
						Value: &Array{
							Nullable: true,
							Item: &String{
								Nullable: false,
								Path:     []string{"name"},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `invalid value type 'array' for path /data/stringObject/stringField, expecting string, got: [{"id":1},{"id":2},{"id":3}]. You can fix this by configuring this field as Int/Float Scalar`
	}))
	t.Run("empty nullable array should resolve correctly", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: true,
				Fetch: &SingleFetch{
					DataSource: FakeDataSource(`[]`),
				},
				Fields: []*Field{
					{
						Name: []byte("nullableArray"),
						Value: &Array{
							Nullable: true,
							Item: &Object{
								Nullable: false,
								Fields: []*Field{
									{
										Name: []byte("foo"),
										Value: &String{
											Nullable: false,
										},
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"data":{"nullableArray":[]}}`
	}))
	t.Run("empty not nullable array should resolve correctly", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: false,
				Fetch: &SingleFetch{
					DataSource: FakeDataSource(`{"some_path": []}`),
				},
				Fields: []*Field{
					{
						Name: []byte("notNullableArray"),
						Value: &Array{
							Path:     []string{"some_path"},
							Nullable: false,
							Item: &Object{
								Nullable: false,
								Fields: []*Field{
									{
										Name: []byte("foo"),
										Value: &String{
											Nullable: false,
										},
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"data":{"notNullableArray":[]}}`
	}))
	t.Run("when data null not nullable array should resolve to data null and errors", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: false,
				Fetch: &SingleFetch{
					DataSource: FakeDataSource(`{"data":null}`),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				Fields: []*Field{
					{
						Name: []byte("nonNullArray"),
						Value: &Array{
							Nullable: false,
							Item: &Object{
								Nullable: false,
								Fields: []*Field{
									{
										Name: []byte("foo"),
										Value: &String{
											Nullable: false,
										},
									},
								},
							},
						},
					},
					{
						Name: []byte("nullableArray"),
						Value: &Array{
							Nullable: true,
							Item: &Object{
								Nullable: false,
								Fields: []*Field{
									{
										Name: []byte("foo"),
										Value: &String{
											Nullable: false,
										},
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"unable to resolve","locations":[{"line":0,"column":0}]}],"data":null}`
	}))
	t.Run("when data null and errors present not nullable array should result to null data upsteam error and resolve error", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: false,
				Fetch: &SingleFetch{
					DataSource: FakeDataSource(
						`{"errors":[{"message":"Could not get a name","locations":[{"line":3,"column":5}],"path":["todos",0,"name"]}],"data":null}`),
				},
				Fields: []*Field{
					{
						Name: []byte("todos"),
						Value: &Array{
							Nullable: false,
							Item: &Object{
								Nullable: false,
								Fields: []*Field{
									{
										Name: []byte("name"),
										Value: &String{
											Nullable: false,
										},
										Position: Position{
											Line:   100,
											Column: 777,
										},
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"Could not get a name","locations":[{"line":3,"column":5}],"path":["todos",0,"name"]}]}`
	}))
	t.Run("complex GraphQL Server plan", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		serviceOne := NewMockDataSource(ctrl)
		serviceOne.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				expected := `{"url":"https://service.one","body":{"query":"query($firstArg: String, $thirdArg: Int){serviceOne(serviceOneArg: $firstArg){fieldOne} anotherServiceOne(anotherServiceOneArg: $thirdArg){fieldOne} reusingServiceOne(reusingServiceOneArg: $firstArg){fieldOne}}","variables":{"thirdArg":123,"firstArg":"firstArgValue"}}}`
				assert.Equal(t, expected, actual)
				pair := NewBufPair()
				pair.Data.WriteString(`{"serviceOne":{"fieldOne":"fieldOneValue"},"anotherServiceOne":{"fieldOne":"anotherFieldOneValue"},"reusingServiceOne":{"fieldOne":"reUsingFieldOneValue"}}`)
				return writeGraphqlResponse(pair, w, false)
			})

		serviceTwo := NewMockDataSource(ctrl)
		serviceTwo.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				expected := `{"url":"https://service.two","body":{"query":"query($secondArg: Boolean, $fourthArg: Float){serviceTwo(serviceTwoArg: $secondArg){fieldTwo} secondServiceTwo(secondServiceTwoArg: $fourthArg){fieldTwo}}","variables":{"fourthArg":12.34,"secondArg":true}}}`
				assert.Equal(t, expected, actual)

				pair := NewBufPair()
				pair.Data.WriteString(`{"serviceTwo":{"fieldTwo":"fieldTwoValue"},"secondServiceTwo":{"fieldTwo":"secondFieldTwoValue"}}`)
				return writeGraphqlResponse(pair, w, false)
			})

		nestedServiceOne := NewMockDataSource(ctrl)
		nestedServiceOne.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				expected := `{"url":"https://service.one","body":{"query":"{serviceOne {fieldOne}}"}}`
				assert.Equal(t, expected, actual)
				pair := NewBufPair()
				pair.Data.WriteString(`{"serviceOne":{"fieldOne":"fieldOneValue"}}`)
				return writeGraphqlResponse(pair, w, false)
			})

		return &GraphQLResponse{
			Data: &Object{
				Fetch: &ParallelFetch{
					Fetches: []Fetch{
						&SingleFetch{
							Input: `{"url":"https://service.one","body":{"query":"query($firstArg: String, $thirdArg: Int){serviceOne(serviceOneArg: $firstArg){fieldOne} anotherServiceOne(anotherServiceOneArg: $thirdArg){fieldOne} reusingServiceOne(reusingServiceOneArg: $firstArg){fieldOne}}","variables":{"thirdArg":$$1$$,"firstArg":$$0$$}}}`,
							InputTemplate: InputTemplate{
								Segments: []TemplateSegment{
									{
										SegmentType: StaticSegmentType,
										Data:        []byte(`{"url":"https://service.one","body":{"query":"query($firstArg: String, $thirdArg: Int){serviceOne(serviceOneArg: $firstArg){fieldOne} anotherServiceOne(anotherServiceOneArg: $thirdArg){fieldOne} reusingServiceOne(reusingServiceOneArg: $firstArg){fieldOne}}","variables":{"thirdArg":`),
									},
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"thirdArg"},
										Renderer:           NewPlainVariableRendererWithValidation(`{"type":"number"}`),
									},
									{
										SegmentType: StaticSegmentType,
										Data:        []byte(`,"firstArg":"`),
									},
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"firstArg"},
										Renderer:           NewPlainVariableRendererWithValidation(`{"type":"string"}`),
									},
									{
										SegmentType: StaticSegmentType,
										Data:        []byte(`"}}}`),
									},
								},
							},
							DataSource: serviceOne,
							Variables: NewVariables(
								&ContextVariable{
									Path: []string{"firstArg"},
								},
								&ContextVariable{
									Path: []string{"thirdArg"},
								},
							),
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data"},
							},
						},
						&SingleFetch{
							Input: `{"url":"https://service.two","body":{"query":"query($secondArg: Boolean, $fourthArg: Float){serviceTwo(serviceTwoArg: $secondArg){fieldTwo} secondServiceTwo(secondServiceTwoArg: $fourthArg){fieldTwo}}","variables":{"fourthArg":$$1$$,"secondArg":$$0$$}}}`,
							InputTemplate: InputTemplate{
								Segments: []TemplateSegment{
									{
										SegmentType: StaticSegmentType,
										Data:        []byte(`{"url":"https://service.two","body":{"query":"query($secondArg: Boolean, $fourthArg: Float){serviceTwo(serviceTwoArg: $secondArg){fieldTwo} secondServiceTwo(secondServiceTwoArg: $fourthArg){fieldTwo}}","variables":{"fourthArg":`),
									},
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"fourthArg"},
										Renderer:           NewPlainVariableRendererWithValidation(`{"type":"number"}`),
									},
									{
										SegmentType: StaticSegmentType,
										Data:        []byte(`,"secondArg":`),
									},
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"secondArg"},
										Renderer:           NewPlainVariableRendererWithValidation(`{"type":"boolean"}`),
									},
									{
										SegmentType: StaticSegmentType,
										Data:        []byte(`}}}`),
									},
								},
							},
							DataSource: serviceTwo,
							Variables: NewVariables(
								&ContextVariable{
									Path: []string{"secondArg"},
								},
								&ContextVariable{
									Path: []string{"fourthArg"},
								},
							),
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data"},
							},
						},
					},
				},
				Fields: []*Field{
					{
						Name: []byte("serviceOne"),
						Value: &Object{
							Path: []string{"serviceOne"},
							Fields: []*Field{
								{
									Name: []byte("fieldOne"),
									Value: &String{
										Path: []string{"fieldOne"},
									},
								},
							},
						},
					},
					{
						Name: []byte("serviceTwo"),
						Value: &Object{
							Path: []string{"serviceTwo"},
							Fetch: &SingleFetch{
								Input: `{"url":"https://service.one","body":{"query":"{serviceOne {fieldOne}}"}}`,
								InputTemplate: InputTemplate{
									Segments: []TemplateSegment{
										{
											SegmentType: StaticSegmentType,
											Data:        []byte(`{"url":"https://service.one","body":{"query":"{serviceOne {fieldOne}}"}}`),
										},
									},
								},
								DataSource: nestedServiceOne,
								Variables:  Variables{},
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data"},
								},
							},
							Fields: []*Field{
								{
									Name: []byte("fieldTwo"),
									Value: &String{
										Path: []string{"fieldTwo"},
									},
								},
								{
									Name: []byte("serviceOneResponse"),
									Value: &Object{
										Path: []string{"serviceOne"},
										Fields: []*Field{
											{
												Name: []byte("fieldOne"),
												Value: &String{
													Path: []string{"fieldOne"},
												},
											},
										},
									},
								},
							},
						},
					},
					{
						Name: []byte("anotherServiceOne"),
						Value: &Object{
							Path: []string{"anotherServiceOne"},
							Fields: []*Field{
								{
									Name: []byte("fieldOne"),
									Value: &String{
										Path: []string{"fieldOne"},
									},
								},
							},
						},
					},
					{
						Name: []byte("secondServiceTwo"),
						Value: &Object{
							Path: []string{"secondServiceTwo"},
							Fields: []*Field{
								{
									Name: []byte("fieldTwo"),
									Value: &String{
										Path: []string{"fieldTwo"},
									},
								},
							},
						},
					},
					{
						Name: []byte("reusingServiceOne"),
						Value: &Object{
							Path: []string{"reusingServiceOne"},
							Fields: []*Field{
								{
									Name: []byte("fieldOne"),
									Value: &String{
										Path: []string{"fieldOne"},
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"firstArg":"firstArgValue","thirdArg":123,"secondArg": true, "fourthArg": 12.34}`)}, `{"data":{"serviceOne":{"fieldOne":"fieldOneValue"},"serviceTwo":{"fieldTwo":"fieldTwoValue","serviceOneResponse":{"fieldOne":"fieldOneValue"}},"anotherServiceOne":{"fieldOne":"anotherFieldOneValue"},"secondServiceTwo":{"fieldTwo":"secondFieldTwoValue"},"reusingServiceOne":{"fieldOne":"reUsingFieldOneValue"}}}`
	}))
	t.Run("federation", func(t *testing.T) {
		t.Run("simple", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

			userService := NewMockDataSource(ctrl)
			userService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"me":{"id":"1234","username":"Me","__typename":"User"}}`)
					return writeGraphqlResponse(pair, w, false)
				})

			reviewsService := NewMockDataSource(ctrl)
			reviewsService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"1234","__typename":"User"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"_entities":[{"reviews":[{"body": "A highly effective form of birth control.","product": {"upc": "top-1","__typename": "Product"}},{"body": "Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product": {"upc": "top-2","__typename": "Product"}}]}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			var productServiceCallCount atomic.Int64

			productService := NewMockDataSource(ctrl)
			productService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					productServiceCallCount.Add(1)
					switch actual {
					case `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}}}`:
						pair := NewBufPair()
						pair.Data.WriteString(`{"_entities":[{"name": "Furby"}]}`)
						return writeGraphqlResponse(pair, w, false)
					case `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}}}`:
						pair := NewBufPair()
						pair.Data.WriteString(`{"_entities":[{"name": "Trilby"}]}`)
						return writeGraphqlResponse(pair, w, false)
					default:
						t.Fatalf("unexpected request: %s", actual)
					}
					return
				}).
				Return(nil).Times(2)

			return &GraphQLResponse{
				Data: &Object{
					Fetch: &SingleFetch{
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						DataSource: userService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Fields: []*Field{
						{
							Name: []byte("me"),
							Value: &Object{
								Fetch: &SingleFetch{
									InputTemplate: InputTemplate{
										Segments: []TemplateSegment{
											{
												Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[`),
												SegmentType: StaticSegmentType,
											},
											{
												SegmentType:  VariableSegmentType,
												VariableKind: ResolvableObjectVariableKind,
												Renderer: NewGraphQLVariableResolveRenderer(&Object{
													Fields: []*Field{
														{
															Name: []byte("id"),
															Value: &String{
																Path: []string{"id"},
															},
														},
														{
															Name: []byte("__typename"),
															Value: &String{
																Path: []string{"__typename"},
															},
														},
													},
												}),
											},
											{
												Data:        []byte(`]}}}`),
												SegmentType: StaticSegmentType,
											},
										},
									},
									DataSource: reviewsService,
									PostProcessing: PostProcessingConfiguration{
										SelectResponseDataPath: []string{"data", "_entities", "[0]"},
									},
								},
								Path:     []string{"me"},
								Nullable: true,
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &String{
											Path: []string{"id"},
										},
									},
									{
										Name: []byte("username"),
										Value: &String{
											Path: []string{"username"},
										},
									},
									{

										Name: []byte("reviews"),
										Value: &Array{
											Path:     []string{"reviews"},
											Nullable: true,
											Item: &Object{
												Nullable: true,
												Fields: []*Field{
													{
														Name: []byte("body"),
														Value: &String{
															Path: []string{"body"},
														},
													},
													{
														Name: []byte("product"),
														Value: &Object{
															Path: []string{"product"},
															Fetch: &ParallelListItemFetch{
																Fetch: &SingleFetch{
																	DataSource: productService,
																	InputTemplate: InputTemplate{
																		Segments: []TemplateSegment{
																			{
																				Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[`),
																				SegmentType: StaticSegmentType,
																			},
																			{
																				SegmentType:  VariableSegmentType,
																				VariableKind: ResolvableObjectVariableKind,
																				Renderer: NewGraphQLVariableResolveRenderer(&Object{
																					Fields: []*Field{
																						{
																							Name: []byte("upc"),
																							Value: &String{
																								Path: []string{"upc"},
																							},
																						},
																						{
																							Name: []byte("__typename"),
																							Value: &String{
																								Path: []string{"__typename"},
																							},
																						},
																					},
																				}),
																			},
																			{
																				Data:        []byte(`]}}}`),
																				SegmentType: StaticSegmentType,
																			},
																		},
																	},
																	PostProcessing: PostProcessingConfiguration{
																		SelectResponseDataPath: []string{"data", "_entities", "[0]"},
																	},
																},
															},
															Fields: []*Field{
																{
																	Name: []byte("upc"),
																	Value: &String{
																		Path: []string{"upc"},
																	},
																},
																{
																	Name: []byte("name"),
																	Value: &String{
																		Path: []string{"name"},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Furby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Trilby"}}]}}}`
		}))
		t.Run("federation with batch", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
			userService := NewMockDataSource(ctrl)
			userService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w *bytes.Buffer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"me":{"id":"1234","username":"Me","__typename": "User"}}`)
					return writeGraphqlResponse(pair, w, false)
				})

			reviewsService := NewMockDataSource(ctrl)
			reviewsService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w *bytes.Buffer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"__typename":"User","id":"1234"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"_entities": [{"__typename":"User","reviews": [{"body": "A highly effective form of birth control.","product": {"upc": "top-1","__typename": "Product"}},{"body": "Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product": {"upc": "top-2","__typename": "Product"}}]}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			productService := NewMockDataSource(ctrl)
			productService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w *bytes.Buffer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"__typename":"Product","upc":"top-1"},{"__typename":"Product","upc":"top-2"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"_entities": [{"name": "Trilby"},{"name": "Fedora"}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			return &GraphQLResponse{
				Data: &Object{
					Fetch: &SingleFetch{
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						DataSource: userService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Fields: []*Field{
						{
							Name: []byte("me"),
							Value: &Object{
								Fetch: &SingleFetch{
									InputTemplate: InputTemplate{
										Segments: []TemplateSegment{
											{
												Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[`),
												SegmentType: StaticSegmentType,
											},
											{
												SegmentType:  VariableSegmentType,
												VariableKind: ResolvableObjectVariableKind,
												Renderer: NewGraphQLVariableResolveRenderer(&Object{
													Fields: []*Field{
														{
															Name: []byte("__typename"),
															Value: &String{
																Path: []string{"__typename"},
															},
														},
														{
															Name: []byte("id"),
															Value: &String{
																Path: []string{"id"},
															},
														},
													},
												}),
											},
											{
												Data:        []byte(`]}}}`),
												SegmentType: StaticSegmentType,
											},
										},
									},
									DataSource: reviewsService,
									PostProcessing: PostProcessingConfiguration{
										SelectResponseDataPath: []string{"data", "_entities", "[0]"},
									},
								},
								Path:     []string{"me"},
								Nullable: true,
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &String{
											Path: []string{"id"},
										},
									},
									{
										Name: []byte("username"),
										Value: &String{
											Path: []string{"username"},
										},
									},
									{
										Name: []byte("reviews"),
										Value: &Array{
											Path:     []string{"reviews"},
											Nullable: true,
											Item: &Object{
												Nullable: true,
												Fields: []*Field{
													{
														Name: []byte("body"),
														Value: &String{
															Path: []string{"body"},
														},
													},
													{
														Name: []byte("product"),
														Value: &Object{
															Path: []string{"product"},
															Fetch: &SingleFetch{
																DataSource: productService,
																InputTemplate: InputTemplate{
																	Segments: []TemplateSegment{
																		{
																			Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":`),
																			SegmentType: StaticSegmentType,
																		},
																		{
																			SegmentType:  VariableSegmentType,
																			VariableKind: ResolvableObjectVariableKind,
																			Renderer: NewGraphQLVariableResolveRenderer(&Array{
																				Item: &Object{
																					Fields: []*Field{
																						{
																							Name: []byte("__typename"),
																							Value: &String{
																								Path: []string{"__typename"},
																							},
																						},
																						{
																							Name: []byte("upc"),
																							Value: &String{
																								Path: []string{"upc"},
																							},
																						},
																					},
																				},
																			}),
																		},
																		{
																			Data:        []byte(`}}}`),
																			SegmentType: StaticSegmentType,
																		},
																	},
																},
																PostProcessing: PostProcessingConfiguration{
																	SelectResponseDataPath: []string{"data", "_entities"},
																},
															},
															Fields: []*Field{
																{
																	Name: []byte("upc"),
																	Value: &String{
																		Path: []string{"upc"},
																	},
																},
																{
																	Name: []byte("name"),
																	Value: &String{
																		Path: []string{"name"},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Fedora"}}]}}}`
		}))
		t.Run("federation with merge paths", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
			userService := NewMockDataSource(ctrl)
			userService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w *bytes.Buffer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"me":{"id":"1234","username":"Me","__typename": "User"}}`)
					return writeGraphqlResponse(pair, w, false)
				})

			reviewsService := NewMockDataSource(ctrl)
			reviewsService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w *bytes.Buffer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"__typename":"User","id":"1234"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"_entities": [{"__typename":"User","reviews": [{"body": "A highly effective form of birth control.","product": {"upc": "top-1","__typename": "Product"}},{"body": "Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product": {"upc": "top-2","__typename": "Product"}}]}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			productService := NewMockDataSource(ctrl)
			productService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w *bytes.Buffer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"__typename":"Product","upc":"top-1"},{"__typename":"Product","upc":"top-2"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"_entities": [{"name": "Trilby"},{"name": "Fedora"}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			return &GraphQLResponse{
				Data: &Object{
					Fetch: &SingleFetch{
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						DataSource: userService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Fields: []*Field{
						{
							Name: []byte("me"),
							Value: &Object{
								Fetch: &SingleFetch{
									InputTemplate: InputTemplate{
										Segments: []TemplateSegment{
											{
												Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[`),
												SegmentType: StaticSegmentType,
											},
											{
												SegmentType:  VariableSegmentType,
												VariableKind: ResolvableObjectVariableKind,
												Renderer: NewGraphQLVariableResolveRenderer(&Object{
													Fields: []*Field{
														{
															Name: []byte("__typename"),
															Value: &String{
																Path: []string{"__typename"},
															},
														},
														{
															Name: []byte("id"),
															Value: &String{
																Path: []string{"id"},
															},
														},
													},
												}),
											},
											{
												Data:        []byte(`]}}}`),
												SegmentType: StaticSegmentType,
											},
										},
									},
									DataSource: reviewsService,
									PostProcessing: PostProcessingConfiguration{
										SelectResponseDataPath: []string{"data", "_entities", "[0]"},
									},
								},
								Path:     []string{"me"},
								Nullable: true,
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &String{
											Path: []string{"id"},
										},
									},
									{
										Name: []byte("username"),
										Value: &String{
											Path: []string{"username"},
										},
									},
									{
										Name: []byte("reviews"),
										Value: &Array{
											Path:     []string{"reviews"},
											Nullable: true,
											Item: &Object{
												Nullable: true,
												Fields: []*Field{
													{
														Name: []byte("body"),
														Value: &String{
															Path: []string{"body"},
														},
													},
													{
														Name: []byte("product"),
														Value: &Object{
															Path: []string{"product"},
															Fetch: &SingleFetch{
																DataSource: productService,
																InputTemplate: InputTemplate{
																	Segments: []TemplateSegment{
																		{
																			Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":`),
																			SegmentType: StaticSegmentType,
																		},
																		{
																			SegmentType:  VariableSegmentType,
																			VariableKind: ResolvableObjectVariableKind,
																			Renderer: NewGraphQLVariableResolveRenderer(&Array{
																				Item: &Object{
																					Fields: []*Field{
																						{
																							Name: []byte("__typename"),
																							Value: &String{
																								Path: []string{"__typename"},
																							},
																						},
																						{
																							Name: []byte("upc"),
																							Value: &String{
																								Path: []string{"upc"},
																							},
																						},
																					},
																				},
																			}),
																		},
																		{
																			Data:        []byte(`}}}`),
																			SegmentType: StaticSegmentType,
																		},
																	},
																},
																PostProcessing: PostProcessingConfiguration{
																	SelectResponseDataPath: []string{"data", "_entities"},
																	MergePath:              []string{"data"},
																},
															},
															Fields: []*Field{
																{
																	Name: []byte("upc"),
																	Value: &String{
																		Path: []string{"upc"},
																	},
																},
																{
																	Name: []byte("name"),
																	Value: &String{
																		Path: []string{"data", "name"},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Fedora"}}]}}}`
		}))
		t.Run("federation with null response", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
			userService := NewMockDataSource(ctrl)
			userService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"me":{"id":"1234","username":"Me","__typename": "User"}}`)
					return writeGraphqlResponse(pair, w, false)
				})

			reviewsService := NewMockDataSource(ctrl)
			reviewsService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"1234","__typename":"User"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"_entities":[{"reviews": [
						{"body": "foo","product": {"upc": "top-1","__typename": "Product"}},
						{"body": "bar","product": {"upc": "top-2","__typename": "Product"}},
						{"body": "baz","product": null},
						{"body": "bat","product": {"upc": "top-4","__typename": "Product"}},
						{"body": "bal","product": {"upc": "top-5","__typename": "Product"}},
						{"body": "ban","product": {"upc": "top-6","__typename": "Product"}}
]}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			productService := NewMockDataSource(ctrl)
			productService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"},{"upc":"top-2","__typename":"Product"},{"upc":"top-4","__typename":"Product"},{"upc":"top-5","__typename":"Product"},{"upc":"top-6","__typename":"Product"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"_entities":[{"name":"Trilby"},{"name":"Fedora"},{"name":"Boater"},{"name":"Top Hat"},{"name":"Bowler"}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			return &GraphQLResponse{
				Data: &Object{
					Fetch: &SingleFetch{
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						DataSource: userService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Fields: []*Field{
						{
							Name: []byte("me"),
							Value: &Object{
								Fetch: &SingleFetch{
									InputTemplate: InputTemplate{
										Segments: []TemplateSegment{
											{
												Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[`),
												SegmentType: StaticSegmentType,
											},
											{
												SegmentType:  VariableSegmentType,
												VariableKind: ResolvableObjectVariableKind,
												Renderer: NewGraphQLVariableResolveRenderer(&Object{
													Fields: []*Field{
														{
															Name: []byte("id"),
															Value: &String{
																Path: []string{"id"},
															},
														},
														{
															Name: []byte("__typename"),
															Value: &String{
																Path: []string{"__typename"},
															},
														},
													},
												}),
											},
											{
												Data:        []byte(`]}}}`),
												SegmentType: StaticSegmentType,
											},
										},
										SetTemplateOutputToNullOnVariableNull: true,
									},
									DataSource: reviewsService,
									PostProcessing: PostProcessingConfiguration{
										SelectResponseDataPath: []string{"data", "_entities", "[0]"},
									},
								},
								Path:     []string{"me"},
								Nullable: true,
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &String{
											Path: []string{"id"},
										},
									},
									{
										Name: []byte("username"),
										Value: &String{
											Path: []string{"username"},
										},
									},
									{

										Name: []byte("reviews"),
										Value: &Array{
											Path:     []string{"reviews"},
											Nullable: true,
											Item: &Object{
												Nullable: true,
												Fields: []*Field{
													{
														Name: []byte("body"),
														Value: &String{
															Path: []string{"body"},
														},
													},
													{
														Name: []byte("product"),
														Value: &Object{
															Nullable: true,
															Path:     []string{"product"},
															Fetch: &BatchFetch{
																DataSource: productService,
																Input: BatchInput{
																	Header: InputTemplate{
																		Segments: []TemplateSegment{
																			{
																				Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[`),
																				SegmentType: StaticSegmentType,
																			},
																		},
																	},
																	Items: []InputTemplate{
																		{
																			Segments: []TemplateSegment{
																				{
																					SegmentType:  VariableSegmentType,
																					VariableKind: ResolvableObjectVariableKind,
																					Renderer: NewGraphQLVariableResolveRenderer(&Object{
																						Fields: []*Field{
																							{
																								Name: []byte("upc"),
																								Value: &String{
																									Path: []string{"upc"},
																								},
																							},
																							{
																								Name: []byte("__typename"),
																								Value: &String{
																									Path: []string{"__typename"},
																								},
																							},
																						},
																					}),
																				},
																			},
																		},
																	},
																	SkipNullItems: true,
																	SkipErrItems:  true,
																	Separator: InputTemplate{
																		Segments: []TemplateSegment{
																			{
																				Data:        []byte(`,`),
																				SegmentType: StaticSegmentType,
																			},
																		},
																	},
																	Footer: InputTemplate{
																		Segments: []TemplateSegment{
																			{
																				Data:        []byte(`]}}}`),
																				SegmentType: StaticSegmentType,
																			},
																		},
																	},
																},
																PostProcessing: PostProcessingConfiguration{
																	SelectResponseDataPath:   []string{"data", "_entities"},
																	SelectResponseErrorsPath: []string{"errors"},
																},
															},
															Fields: []*Field{
																{
																	Name: []byte("upc"),
																	Value: &String{
																		Path: []string{"upc"},
																	},
																},
																{
																	Name: []byte("name"),
																	Value: &String{
																		Path: []string{"name"},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"foo","product":{"upc":"top-1","name":"Trilby"}},{"body":"bar","product":{"upc":"top-2","name":"Fedora"}},{"body":"baz","product":null},{"body":"bat","product":{"upc":"top-4","name":"Boater"}},{"body":"bal","product":{"upc":"top-5","name":"Top Hat"}},{"body":"ban","product":{"upc":"top-6","name":"Bowler"}}]}}}`
		}))
		t.Run("federation with fetch error ", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

			userService := NewMockDataSource(ctrl)
			userService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"me": {"id": "1234","username": "Me","__typename": "User"}}`)
					return writeGraphqlResponse(pair, w, false)
				})

			reviewsService := NewMockDataSource(ctrl)
			reviewsService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"1234","__typename":"User"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"_entities":[{"reviews":[{"body": "A highly effective form of birth control.","product":{"upc": "top-1","__typename":"Product"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","__typename":"Product"}}]}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			productService := NewMockDataSource(ctrl)
			productService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"},{"upc":"top-2","__typename":"Product"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.WriteErr([]byte("errorMessage"), nil, nil, nil)
					return writeGraphqlResponse(pair, w, false)
				})

			return &GraphQLResponse{
				Data: &Object{
					Fetch: &SingleFetch{
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						DataSource: userService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Fields: []*Field{
						{
							Name: []byte("me"),
							Value: &Object{
								Fetch: &SingleFetch{
									InputTemplate: InputTemplate{
										Segments: []TemplateSegment{
											{
												Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"`),
												SegmentType: StaticSegmentType,
											},
											{
												SegmentType:        VariableSegmentType,
												VariableKind:       ObjectVariableKind,
												VariableSourcePath: []string{"id"},
												Renderer:           NewPlainVariableRendererWithValidation(`{"type":"string"}`),
											},
											{
												Data:        []byte(`","__typename":"User"}]}}}`),
												SegmentType: StaticSegmentType,
											},
										},
									},
									DataSource: reviewsService,
									PostProcessing: PostProcessingConfiguration{
										SelectResponseDataPath: []string{"data", "_entities", "[0]"},
									},
								},
								Path:     []string{"me"},
								Nullable: true,
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &String{
											Path: []string{"id"},
										},
									},
									{
										Name: []byte("username"),
										Value: &String{
											Path: []string{"username"},
										},
									},
									{

										Name: []byte("reviews"),
										Value: &Array{
											Path:     []string{"reviews"},
											Nullable: true,
											Item: &Object{
												Nullable: true,
												Fields: []*Field{
													{
														Name: []byte("body"),
														Value: &String{
															Path: []string{"body"},
														},
													},
													{
														Name: []byte("product"),
														Value: &Object{
															Path: []string{"product"},
															Fetch: &SingleFetch{
																DataSource: productService,
																InputTemplate: InputTemplate{
																	Segments: []TemplateSegment{
																		{
																			Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":`),
																			SegmentType: StaticSegmentType,
																		},
																		{
																			SegmentType:        VariableSegmentType,
																			VariableKind:       ResolvableObjectVariableKind,
																			VariableSourcePath: []string{"upc"},
																			Renderer: NewGraphQLVariableResolveRenderer(&Array{
																				Item: &Object{
																					Fields: []*Field{
																						{
																							Name: []byte("upc"),
																							Value: &String{
																								Path: []string{"upc"},
																							},
																						},
																						{
																							Name: []byte("__typename"),
																							Value: &String{
																								Path: []string{"__typename"},
																							},
																						},
																					},
																				},
																			}),
																		},
																		{
																			Data:        []byte(`}}}`),
																			SegmentType: StaticSegmentType,
																		},
																	},
																},
																PostProcessing: PostProcessingConfiguration{
																	SelectResponseDataPath: []string{"data", "_entities"},
																},
															},
															Fields: []*Field{
																{
																	Name: []byte("upc"),
																	Value: &String{
																		Path: []string{"upc"},
																	},
																},
																{
																	Name: []byte("name"),
																	Value: &String{
																		Path: []string{"name"},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}, Context{ctx: context.Background(), Variables: nil}, `{"errors":[{"message":"errorMessage"}]}`
		}))
		t.Run("federation with optional variable", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
			userService := NewMockDataSource(ctrl)
			userService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:8080/query","body":{"query":"{me {id}}"}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"me":{"id":"1234","__typename":"User"}}`)
					return writeGraphqlResponse(pair, w, false)
				})

			employeeService := NewMockDataSource(ctrl)
			employeeService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:8081/query","body":{"query":"query($representations: [_Any!]!, $companyId: ID!){_entities(representations: $representations){... on User {employment(companyId: $companyId){id}}}}","variables":{"companyId":"abc123","representations":[{"id":"1234","__typename":"User"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"_entities":[{"employment":{"id":"xyz987"}}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			timeService := NewMockDataSource(ctrl)
			timeService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:8082/query","body":{"query":"query($representations: [_Any!]!, $date: LocalTime){_entities(representations: $representations){... on Employee {times(date: $date){id employee {id} start end}}}}","variables":{"date":null,"representations":[{"id":"xyz987","__typename":"Employee"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"_entities":[{"times":[{"id": "t1","employee":{"id":"xyz987"},"start":"2022-11-02T08:00:00","end":"2022-11-02T12:00:00"}]}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			return &GraphQLResponse{
				Data: &Object{
					Fetch: &SingleFetch{
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://localhost:8080/query","body":{"query":"{me {id}}"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						DataSource: userService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Fields: []*Field{
						{
							Name: []byte("me"),
							Value: &Object{
								Nullable: false,
								Path:     []string{"me"},
								Fields: []*Field{
									{
										Name: []byte("employment"),
										Value: &Object{
											Nullable: false,
											Path:     []string{"employment"},
											Fields: []*Field{
												{
													Name: []byte("id"),
													Value: &String{
														Path:     []string{"id"},
														Nullable: false,
													},
												},
												{
													Name: []byte("times"),
													Value: &Array{
														Path:     []string{"times"},
														Nullable: false,
														Item: &Object{
															Nullable: true,
															Fields: []*Field{
																{
																	Name:  []byte("id"),
																	Value: &String{Path: []string{"id"}},
																},
																{
																	Name: []byte("employee"),
																	Value: &Object{
																		Path: []string{"employee"},
																		Fields: []*Field{
																			{
																				Name: []byte("id"),
																				Value: &String{
																					Path: []string{"id"},
																				},
																			},
																		},
																	},
																},
																{
																	Name:  []byte("start"),
																	Value: &String{Path: []string{"start"}},
																},
																{
																	Name: []byte("end"),
																	Value: &String{
																		Path:     []string{"end"},
																		Nullable: true,
																	},
																},
															},
														},
													},
												},
											},
											Fetch: &SingleFetch{
												InputTemplate: InputTemplate{
													Segments: []TemplateSegment{
														{
															Data:        []byte(`{"method":"POST","url":"http://localhost:8082/query","body":{"query":"query($representations: [_Any!]!, $date: LocalTime){_entities(representations: $representations){... on Employee {times(date: $date){id employee {id} start end}}}}","variables":{"date":`),
															SegmentType: StaticSegmentType,
														},
														{
															SegmentType:        VariableSegmentType,
															VariableKind:       ContextVariableKind,
															VariableSourcePath: []string{"date"},
															Renderer:           NewJSONVariableRendererWithValidation(`{}`),
														},
														{
															Data:        []byte(`,"representations":[{"id":`),
															SegmentType: StaticSegmentType,
														},
														{
															SegmentType:        VariableSegmentType,
															VariableKind:       ObjectVariableKind,
															VariableSourcePath: []string{"id"},
															Renderer:           NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
														},
														{
															Data:        []byte(`,"__typename":"Employee"}]}}}`),
															SegmentType: StaticSegmentType,
														},
													},
													SetTemplateOutputToNullOnVariableNull: true,
												},
												DataSource: timeService,
												PostProcessing: PostProcessingConfiguration{
													SelectResponseDataPath: []string{"data", "_entities", "[0]"},
												},
											},
										},
									},
								},
								Fetch: &SingleFetch{
									InputTemplate: InputTemplate{
										Segments: []TemplateSegment{
											{
												Data:        []byte(`{"method":"POST","url":"http://localhost:8081/query","body":{"query":"query($representations: [_Any!]!, $companyId: ID!){_entities(representations: $representations){... on User {employment(companyId: $companyId){id}}}}","variables":{"companyId":`),
												SegmentType: StaticSegmentType,
											},
											{
												SegmentType:        VariableSegmentType,
												VariableKind:       ContextVariableKind,
												VariableSourcePath: []string{"companyId"},
												Renderer:           NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
											},
											{
												Data:        []byte(`,"representations":[{"id":`),
												SegmentType: StaticSegmentType,
											},
											{
												SegmentType:        VariableSegmentType,
												VariableKind:       ObjectVariableKind,
												VariableSourcePath: []string{"id"},
												Renderer:           NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
											},
											{
												Data:        []byte(`,"__typename":"User"}]}}}`),
												SegmentType: StaticSegmentType,
											},
										},
										SetTemplateOutputToNullOnVariableNull: true,
									},
									DataSource: employeeService,
									PostProcessing: PostProcessingConfiguration{
										SelectResponseDataPath: []string{"data", "_entities", "[0]"},
									},
								},
							},
						},
					},
				},
			}, Context{ctx: context.Background(), Variables: []byte(`{"companyId":"abc123","date":null}`)}, `{"data":{"me":{"employment":{"id":"xyz987","times":[{"id":"t1","employee":{"id":"xyz987"},"start":"2022-11-02T08:00:00","end":"2022-11-02T12:00:00"}]}}}}`
		}))
	})
}

func TestResolver_WithHeader(t *testing.T) {
	cases := []struct {
		name, header, variable string
	}{
		{"header and variable are of equal case", "Authorization", "Authorization"},
		{"header is downcased and variable is uppercased", "authorization", "AUTHORIZATION"},
		{"header is uppercasesed and variable is downcased", "AUTHORIZATION", "authorization"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			resolver := newResolver(rCtx, false)

			header := make(http.Header)
			header.Set(tc.header, "foo")
			ctx := &Context{
				ctx: context.Background(),
				Request: Request{
					Header: header,
				},
			}

			ctrl := gomock.NewController(t)
			fakeService := NewMockDataSource(ctrl)
			fakeService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					assert.Equal(t, "foo", actual)
					_, err = w.Write([]byte(`{"bar":"baz"}`))
					return
				}).
				Return(nil)

			out := &bytes.Buffer{}
			res := &GraphQLResponse{
				Data: &Object{
					Fetch: &SingleFetch{
						DataSource: fakeService,
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									SegmentType:        VariableSegmentType,
									VariableKind:       HeaderVariableKind,
									VariableSourcePath: []string{tc.variable},
								},
							},
						},
					},
					Fields: []*Field{
						{
							Name: []byte("bar"),
							Value: &String{
								Path: []string{"bar"},
							},
						},
					},
				},
			}
			err := resolver.ResolveGraphQLResponse(ctx, res, nil, out)
			assert.NoError(t, err)
			assert.Equal(t, `{"data":{"bar":"baz"}}`, out.String())
		})
	}
}

type TestFlushWriter struct {
	flushed []string
	buf     bytes.Buffer
}

func (t *TestFlushWriter) Write(p []byte) (n int, err error) {
	return t.buf.Write(p)
}

func (t *TestFlushWriter) Flush() {
	t.flushed = append(t.flushed, t.buf.String())
	t.buf.Reset()
}

func FakeStream(cancelFunc func(), messageFunc func(count int) (message string, ok bool)) *_fakeStream {
	return &_fakeStream{
		cancel:      cancelFunc,
		messageFunc: messageFunc,
	}
}

type _fakeStream struct {
	cancel      context.CancelFunc
	messageFunc func(counter int) (message string, ok bool)
}

func (f *_fakeStream) Start(ctx context.Context, input []byte, next chan<- []byte) error {
	go func() {
		time.Sleep(time.Millisecond)
		count := 0
		for {
			if count == 3 {
				f.cancel()
				return
			}
			message, ok := f.messageFunc(count)
			next <- []byte(message)
			if !ok {
				f.cancel()
				return
			}
			count++
		}
	}()
	return nil
}

func TestResolver_ResolveGraphQLSubscription(t *testing.T) {

	setup := func(ctx context.Context, stream SubscriptionDataSource) (*Resolver, *GraphQLSubscription, *TestFlushWriter) {
		plan := &GraphQLSubscription{
			Trigger: GraphQLSubscriptionTrigger{
				Source: stream,
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath:   []string{"data"},
					SelectResponseErrorsPath: []string{"errors"},
				},
			},
			Response: &GraphQLResponse{
				Data: &Object{
					Fields: []*Field{
						{
							Name: []byte("counter"),
							Value: &Integer{
								Path: []string{"counter"},
							},
						},
					},
				},
			},
		}

		out := &TestFlushWriter{
			buf: bytes.Buffer{},
		}

		return newResolver(ctx, false), plan, out
	}

	t.Run("should return errors if the upstream data has errors", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := FakeStream(cancel, func(count int) (message string, ok bool) {
			return `{"errors":[{"message":"Validation error occurred","locations":[{"line":1,"column":1}],"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}],"data":null}`, false
		})

		resolver, plan, out := setup(c, fakeStream)

		ctx := Context{
			ctx: c,
		}

		err := resolver.ResolveGraphQLSubscription(&ctx, plan, out)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(out.flushed))
		assert.Equal(t, `{"errors":[{"message":"unable to resolve","locations":[{"line":0,"column":0}]},{"message":"Validation error occurred","locations":[{"line":1,"column":1}],"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}],"data":null}`, out.flushed[0])
	})

	t.Run("should return an error if the data source has not been defined", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		resolver, plan, out := setup(c, nil)

		ctx := Context{
			ctx: c,
		}

		err := resolver.ResolveGraphQLSubscription(&ctx, plan, out)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(out.flushed))
		assert.Equal(t, `{"errors":[{"message":"no data source found"}]}`, out.flushed[0])
	})

	t.Run("should successfully get result from upstream", func(t *testing.T) {
		t.Skip("TODO: This test hangs with the race detector enabled")
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := FakeStream(cancel, func(count int) (message string, ok bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, count), true
		})

		resolver, plan, out := setup(c, fakeStream)

		ctx := Context{
			ctx: c,
		}

		err := resolver.ResolveGraphQLSubscription(&ctx, plan, out)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(out.flushed))
		assert.Equal(t, `{"data":{"counter":0}}`, out.flushed[0])
		assert.Equal(t, `{"data":{"counter":1}}`, out.flushed[1])
		assert.Equal(t, `{"data":{"counter":2}}`, out.flushed[2])
	})
}

func TestResolver_mergeJSON(t *testing.T) {
	setup := func() *Loader {
		loader := &Loader{
			layers: []*layer{},
		}
		return loader
	}
	t.Run("a", func(t *testing.T) {
		loader := setup()
		left := `{"name":"Bill","info":{"id":11,"__typename":"Info"},"address":{"id": 55,"__typename":"Address"}}`
		right := `{"info":{"age":21},"address":{"line1":"Munich"}}`
		expected := `{"address":{"__typename":"Address","id":55,"line1":"Munich"},"info":{"__typename":"Info","age":21,"id":11},"name":"Bill"}`
		out, err := loader.mergeJSON([]byte(left), []byte(right))
		assert.NoError(t, err)
		assert.JSONEq(t, expected, string(out))
	})

	t.Run("b", func(t *testing.T) {
		loader := setup()
		left := `{"id":"1234","username":"Me","__typename":"User"}`
		right := `{"reviews":[{"body": "A highly effective form of birth control.","product": {"upc": "top-1","__typename": "Product"}},{"body": "Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product": {"upc": "top-2","__typename": "Product"}}]}`
		expected := `{"__typename":"User","id":"1234","reviews":[{"body":"A highly effective form of birth control.","product":{"__typename":"Product","upc":"top-1"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"__typename":"Product","upc":"top-2"}}],"username":"Me"}`
		out, err := loader.mergeJSON([]byte(left), []byte(right))
		assert.NoError(t, err)
		assert.JSONEq(t, expected, string(out))
	})

	t.Run("c", func(t *testing.T) {
		loader := setup()
		left := `{"__typename":"Product","upc":"top-1"}`
		right := `{"name": "Trilby"}`
		expected := `{"__typename":"Product","name":"Trilby","upc":"top-1"}`
		out, err := loader.mergeJSON([]byte(left), []byte(right))
		assert.NoError(t, err)
		assert.JSONEq(t, expected, string(out))
	})

	t.Run("d", func(t *testing.T) {
		loader := setup()
		left := `{"__typename":"Product","upc":"top-1"}`
		right := `{"__typename":"Product","name":"Trilby","upc":"top-1"}`
		expected := `{"__typename":"Product","name":"Trilby","upc":"top-1"}`
		out, err := loader.mergeJSON([]byte(left), []byte(right))
		assert.NoError(t, err)
		assert.JSONEq(t, expected, string(out))
	})

	t.Run("e", func(t *testing.T) {
		loader := setup()
		left := `{"__typename":"Address","id":"address-1","line1":"line1","line2":"line2"}`
		right := `{"__typename":"Address","country":"country-1","city":"city-1"}`
		expected := `{"__typename":"Address","city":"city-1","country":"country-1","id":"address-1","line1":"line1","line2":"line2"}`
		out, err := loader.mergeJSON([]byte(left), []byte(right))
		assert.NoError(t, err)
		assert.JSONEq(t, expected, string(out))
	})

	t.Run("f", func(t *testing.T) {
		loader := setup()
		left := `{"__typename":"Address","city":"city-1","country":"country-1","id":"address-1","line1":"line1","line2":"line2"}`
		right := `{"__typename": "Address", "line3": "line3-1", "zip": "zip-1"}`
		expected := `{"__typename":"Address","city":"city-1","country":"country-1","id":"address-1","line1":"line1","line2":"line2","line3":"line3-1","zip":"zip-1"}`
		out, err := loader.mergeJSON([]byte(left), []byte(right))
		assert.NoError(t, err)
		assert.JSONEq(t, expected, string(out))
	})

	t.Run("g", func(t *testing.T) {
		loader := setup()
		left := `{"__typename":"Address","city":"city-1","country":"country-1","id":"address-1","line1":"line1","line2":"line2","line3":"line3-1","zip":"zip-1"}`
		right := `{"__typename":"Address","fullAddress":"line1 line2 line3-1 city-1 country-1 zip-1"}`
		expected := `{"__typename":"Address","city":"city-1","country":"country-1","fullAddress":"line1 line2 line3-1 city-1 country-1 zip-1","id":"address-1","line1":"line1","line2":"line2","line3":"line3-1","zip":"zip-1"}`
		out, err := loader.mergeJSON([]byte(left), []byte(right))
		assert.NoError(t, err)
		assert.JSONEq(t, expected, string(out))
	})

	t.Run("h", func(t *testing.T) {
		loader := setup()
		left := `{"address":{"__typename":"Address","id":"address-1","line1":"line1","line2":"line2"}}`
		right := `{"__typename":"Address","city":"city-1","country":"country-1","fullAddress":"line1 line2 line3-1 city-1 country-1 zip-1","id":"address-1","line1":"line1","line2":"line2","line3":"line3-1","zip":"zip-1"}`
		expected := `{"__typename":"Address","address":{"__typename":"Address","id":"address-1","line1":"line1","line2":"line2"},"city":"city-1","country":"country-1","fullAddress":"line1 line2 line3-1 city-1 country-1 zip-1","id":"address-1","line1":"line1","line2":"line2","line3":"line3-1","zip":"zip-1"}`
		out, err := loader.mergeJSON([]byte(left), []byte(right))
		assert.NoError(t, err)
		assert.JSONEq(t, expected, string(out))
	})

	t.Run("i", func(t *testing.T) {
		loader := setup()
		left := `{"account":{"address":{"__typename":"Address","id":"address-1","line1":"line1","line2":"line2"}}}`
		right := `{"address":{"__typename":"Address","address":{"__typename":"Address","id":"address-1","line1":"line1","line2":"line2"},"city":"city-1","country":"country-1","fullAddress":"line1 line2 line3-1 city-1 country-1 zip-1","id":"address-1","line1":"line1","line2":"line2","line3":"line3-1","zip":"zip-1"}}`
		expected := `{"account":{"address":{"__typename":"Address","id":"address-1","line1":"line1","line2":"line2"}},"address":{"__typename":"Address","address":{"__typename":"Address","id":"address-1","line1":"line1","line2":"line2"},"city":"city-1","country":"country-1","fullAddress":"line1 line2 line3-1 city-1 country-1 zip-1","id":"address-1","line1":"line1","line2":"line2","line3":"line3-1","zip":"zip-1"}}`
		out, err := loader.mergeJSON([]byte(left), []byte(right))
		assert.NoError(t, err)
		assert.JSONEq(t, expected, string(out))
	})

	t.Run("j", func(t *testing.T) {
		loader := setup()
		left := `{"user":{"account":{"address":{"__typename":"Address","id":"address-1","line1":"line1","line2":"line2"}}}}`
		right := `{"account":{"account":{"address":{"__typename":"Address","id":"address-1","line1":"line1","line2":"line2"}},"address":{"__typename":"Address","address":{"__typename":"Address","id":"address-1","line1":"line1","line2":"line2"},"city":"city-1","country":"country-1","fullAddress":"line1 line2 line3-1 city-1 country-1 zip-1","id":"address-1","line1":"line1","line2":"line2","line3":"line3-1","zip":"zip-1"}}}`
		expected := `{"account":{"account":{"address":{"__typename":"Address","id":"address-1","line1":"line1","line2":"line2"}},"address":{"__typename":"Address","address":{"__typename":"Address","id":"address-1","line1":"line1","line2":"line2"},"city":"city-1","country":"country-1","fullAddress":"line1 line2 line3-1 city-1 country-1 zip-1","id":"address-1","line1":"line1","line2":"line2","line3":"line3-1","zip":"zip-1"}},"user":{"account":{"address":{"__typename":"Address","id":"address-1","line1":"line1","line2":"line2"}}}}`
		out, err := loader.mergeJSON([]byte(left), []byte(right))
		assert.NoError(t, err)
		assert.JSONEq(t, expected, string(out))
	})
}

func Benchmark_ResolveGraphQLResponse(b *testing.B) {
	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resolver := newResolver(rCtx, true)

	userService := FakeDataSource(`{"data":{"users":[{"name":"Bill","info":{"id":11,"__typename":"Info"},"address":{"id":55,"__typename":"Address"}},{"name":"John","info":{"id":12,"__typename":"Info"},"address":{"id":55,"__typename":"Address"}},{"name":"Jane","info":{"id":13,"__typename":"Info"},"address":{"id":55,"__typename":"Address"}}]}}`)
	infoService := FakeDataSource(`{"data":{"_entities":[{"age":21,"__typename":"Info"},{"line1":"Munich","__typename":"Address"},{"age":22,"__typename":"Info"},{"age":23,"__typename":"Info"}]}}`)

	plan := &GraphQLResponse{
		Data: &Object{
			Fetch: &SingleFetch{
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{ users { name info {id __typename} address {id __typename} } }"}}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				DataSource: userService,
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data"},
				},
			},
			Fields: []*Field{
				{
					Name: []byte("users"),
					Value: &Array{
						Path: []string{"users"},
						Item: &Object{
							Fetch: &BatchFetch{
								Input: BatchInput{
									Header: InputTemplate{
										Segments: []TemplateSegment{
											{
												Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age } ... on Address { line1 }}}}}","variables":{"representations":[`),
												SegmentType: StaticSegmentType,
											},
										},
									},
									Items: []InputTemplate{
										{
											Segments: []TemplateSegment{
												{
													SegmentType:  VariableSegmentType,
													VariableKind: ResolvableObjectVariableKind,
													Renderer: NewGraphQLVariableResolveRenderer(&Object{
														Path: []string{"info"},
														Fields: []*Field{
															{
																Name: []byte("id"),
																Value: &Integer{
																	Path: []string{"id"},
																},
															},
															{
																Name: []byte("__typename"),
																Value: &String{
																	Path: []string{"__typename"},
																},
															},
														},
													}),
												},
											},
										},
										{
											Segments: []TemplateSegment{
												{
													SegmentType:  VariableSegmentType,
													VariableKind: ResolvableObjectVariableKind,
													Renderer: NewGraphQLVariableResolveRenderer(&Object{
														Path: []string{"address"},
														Fields: []*Field{
															{
																Name: []byte("id"),
																Value: &Integer{
																	Path: []string{"id"},
																},
															},
															{
																Name: []byte("__typename"),
																Value: &String{
																	Path: []string{"__typename"},
																},
															},
														},
													}),
												},
											},
										},
									},
									Separator: InputTemplate{
										Segments: []TemplateSegment{
											{
												Data:        []byte(`,`),
												SegmentType: StaticSegmentType,
											},
										},
									},
									Footer: InputTemplate{
										Segments: []TemplateSegment{
											{
												Data:        []byte(`]}}}`),
												SegmentType: StaticSegmentType,
											},
										},
									},
								},
								DataSource: infoService,
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data", "_entities"},
									ResponseTemplate: &InputTemplate{
										Segments: []TemplateSegment{
											{
												SegmentType:  VariableSegmentType,
												VariableKind: ResolvableObjectVariableKind,
												Renderer: NewGraphQLVariableResolveRenderer(&Object{
													Fields: []*Field{
														{
															Name: []byte("info"),
															Value: &Object{
																Fields: []*Field{
																	{
																		Name: []byte("age"),
																		Value: &Integer{
																			Path: []string{"[0]", "age"},
																		},
																	},
																},
															},
														},
														{
															Name: []byte("address"),
															Value: &Object{
																Fields: []*Field{
																	{
																		Name: []byte("line1"),
																		Value: &String{
																			Path: []string{"[1]", "line1"},
																		},
																	},
																},
															},
														},
													},
												}),
											},
										},
									},
								},
							},
							Fields: []*Field{
								{
									Name: []byte("name"),
									Value: &String{
										Path: []string{"name"},
									},
								},
								{
									Name: []byte("info"),
									Value: &Object{
										Path: []string{"info"},
										Fields: []*Field{
											{
												Name: []byte("age"),
												Value: &Integer{
													Path: []string{"age"},
												},
											},
										},
									},
								},
								{
									Name: []byte("address"),
									Value: &Object{
										Path: []string{"address"},
										Fields: []*Field{
											{
												Name: []byte("line1"),
												Value: &String{
													Path: []string{"line1"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	var err error
	expected := []byte(`{"data":{"users":[{"name":"Bill","info":{"age":21},"address":{"line1":"Munich"}},{"name":"John","info":{"age":22},"address":{"line1":"Munich"}},{"name":"Jane","info":{"age":23},"address":{"line1":"Munich"}}]}}`)

	pool := sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 1024))
		},
	}

	ctxPool := sync.Pool{
		New: func() interface{} {
			return NewContext(context.Background())
		},
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(expected)))
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// _ = resolver.ResolveGraphQLResponse(ctx, plan, nil, ioutil.Discard)
			ctx := ctxPool.Get().(*Context)
			buf := pool.Get().(*bytes.Buffer)
			err = resolver.ResolveGraphQLResponse(ctx, plan, nil, buf)
			if err != nil {
				b.Fatal(err)
			}
			if !bytes.Equal(expected, buf.Bytes()) {
				b.Fatalf("want:\n%s\ngot:\n%s\n", string(expected), buf.String())
			}

			buf.Reset()
			pool.Put(buf)

			ctx.Free()
			ctxPool.Put(ctx)
		}
	})
}

func Benchmark_NestedBatching(b *testing.B) {
	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resolver := newResolver(rCtx, true)

	productsService := fakeDataSourceWithInputCheck(b,
		[]byte(`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}"}}`),
		[]byte(`{"data":{"topProducts":[{"name":"Table","__typename":"Product","upc":"1"},{"name":"Couch","__typename":"Product","upc":"2"},{"name":"Chair","__typename":"Product","upc":"3"}]}}`))
	stockService := fakeDataSourceWithInputCheck(b,
		[]byte(`{"method":"POST","url":"http://stock","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {stock}}}","variables":{"representations":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"},{"__typename":"Product","upc":"3"}]}}}`),
		[]byte(`{"data":{"_entities":[{"stock":8},{"stock":2},{"stock":5}]}}`))
	reviewsService := fakeDataSourceWithInputCheck(b,
		[]byte(`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {reviews {body author {__typename id}}}}}","variables":{"representations":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"},{"__typename":"Product","upc":"3"}]}}}`),
		[]byte(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2"}}]},{"__typename":"Product","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1"}}]},{"__typename":"Product","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2"}}]}]}}`))
	usersService := fakeDataSourceWithInputCheck(b,
		[]byte(`{"method":"POST","url":"http://users","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name}}}","variables":{"representations":[{"__typename":"User","id":"1"},{"__typename":"User","id":"2"}]}}}`),
		[]byte(`{"data":{"_entities":[{"name":"user-1"},{"name":"user-2"}]}}`))

	plan := &GraphQLResponse{
		Data: &Object{
			Fetch: &SingleFetch{
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}"}}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				DataSource: productsService,
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data"},
				},
			},
			Fields: []*Field{
				{
					Name: []byte("topProducts"),
					Value: &Array{
						Path: []string{"topProducts"},
						Item: &Object{
							Fetch: &ParallelFetch{
								Fetches: []Fetch{
									&BatchFetch{
										Input: BatchInput{
											Header: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {reviews {body author {__typename id}}}}}","variables":{"representations":[`),
														SegmentType: StaticSegmentType,
													},
												},
											},
											Items: []InputTemplate{
												{
													Segments: []TemplateSegment{
														{
															SegmentType:  VariableSegmentType,
															VariableKind: ResolvableObjectVariableKind,
															Renderer: NewGraphQLVariableResolveRenderer(&Object{
																Fields: []*Field{
																	{
																		Name: []byte("__typename"),
																		Value: &String{
																			Path: []string{"__typename"},
																		},
																	},
																	{
																		Name: []byte("upc"),
																		Value: &String{
																			Path: []string{"upc"},
																		},
																	},
																},
															}),
														},
													},
												},
											},
											Separator: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`,`),
														SegmentType: StaticSegmentType,
													},
												},
											},
											Footer: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`]}}}`),
														SegmentType: StaticSegmentType,
													},
												},
											},
										},
										DataSource: reviewsService,
										PostProcessing: PostProcessingConfiguration{
											SelectResponseDataPath: []string{"data", "_entities"},
										},
									},
									&BatchFetch{
										Input: BatchInput{
											Header: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`{"method":"POST","url":"http://stock","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {stock}}}","variables":{"representations":[`),
														SegmentType: StaticSegmentType,
													},
												},
											},
											Items: []InputTemplate{
												{
													Segments: []TemplateSegment{
														{
															SegmentType:  VariableSegmentType,
															VariableKind: ResolvableObjectVariableKind,
															Renderer: NewGraphQLVariableResolveRenderer(&Object{
																Fields: []*Field{
																	{
																		Name: []byte("__typename"),
																		Value: &String{
																			Path: []string{"__typename"},
																		},
																	},
																	{
																		Name: []byte("upc"),
																		Value: &String{
																			Path: []string{"upc"},
																		},
																	},
																},
															}),
														},
													},
												},
											},
											Separator: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`,`),
														SegmentType: StaticSegmentType,
													},
												},
											},
											Footer: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`]}}}`),
														SegmentType: StaticSegmentType,
													},
												},
											},
										},
										DataSource: stockService,
										PostProcessing: PostProcessingConfiguration{
											SelectResponseDataPath: []string{"data", "_entities"},
										},
									},
								},
							},
							Fields: []*Field{
								{
									Name: []byte("name"),
									Value: &String{
										Path: []string{"name"},
									},
								},
								{
									Name: []byte("stock"),
									Value: &Integer{
										Path: []string{"stock"},
									},
								},
								{
									Name: []byte("reviews"),
									Value: &Array{
										Path: []string{"reviews"},
										Item: &Object{
											Fields: []*Field{
												{
													Name: []byte("body"),
													Value: &String{
														Path: []string{"body"},
													},
												},
												{
													Name: []byte("author"),
													Value: &Object{
														Path: []string{"author"},
														Fetch: &BatchFetch{
															Input: BatchInput{
																Header: InputTemplate{
																	Segments: []TemplateSegment{
																		{
																			Data:        []byte(`{"method":"POST","url":"http://users","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name}}}","variables":{"representations":[`),
																			SegmentType: StaticSegmentType,
																		},
																	},
																},
																Items: []InputTemplate{
																	{
																		Segments: []TemplateSegment{
																			{
																				SegmentType:  VariableSegmentType,
																				VariableKind: ResolvableObjectVariableKind,
																				Renderer: NewGraphQLVariableResolveRenderer(&Object{
																					Fields: []*Field{
																						{
																							Name: []byte("__typename"),
																							Value: &String{
																								Path: []string{"__typename"},
																							},
																						},
																						{
																							Name: []byte("id"),
																							Value: &String{
																								Path: []string{"id"},
																							},
																						},
																					},
																				}),
																			},
																		},
																	},
																},
																Separator: InputTemplate{
																	Segments: []TemplateSegment{
																		{
																			Data:        []byte(`,`),
																			SegmentType: StaticSegmentType,
																		},
																	},
																},
																Footer: InputTemplate{
																	Segments: []TemplateSegment{
																		{
																			Data:        []byte(`]}}}`),
																			SegmentType: StaticSegmentType,
																		},
																	},
																},
															},
															DataSource: usersService,
															PostProcessing: PostProcessingConfiguration{
																SelectResponseDataPath: []string{"data", "_entities"},
															},
														},
														Fields: []*Field{
															{
																Name: []byte("name"),
																Value: &String{
																	Path: []string{"name"},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	expected := []byte(`{"data":{"topProducts":[{"name":"Table","stock":8,"reviews":[{"body":"Love Table!","author":{"name":"user-1"}},{"body":"Prefer other Table.","author":{"name":"user-2"}}]},{"name":"Couch","stock":2,"reviews":[{"body":"Couch Too expensive.","author":{"name":"user-1"}}]},{"name":"Chair","stock":5,"reviews":[{"body":"Chair Could be better.","author":{"name":"user-2"}}]}]}}`)

	pool := sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 1024))
		},
	}

	ctxPool := sync.Pool{
		New: func() interface{} {
			return NewContext(context.Background())
		},
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(expected)))
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := ctxPool.Get().(*Context)
			buf := pool.Get().(*bytes.Buffer)
			ctx.ctx = context.Background()
			err := resolver.ResolveGraphQLResponse(ctx, plan, nil, buf)
			if err != nil {
				b.Fatal(err)
			}
			if !bytes.Equal(expected, buf.Bytes()) {
				require.Equal(b, string(expected), buf.String())
			}

			buf.Reset()
			pool.Put(buf)

			ctx.Free()
			ctxPool.Put(ctx)
		}
	})
}

func Benchmark_NestedBatchingWithoutChecks(b *testing.B) {
	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resolver := newResolver(rCtx, true)

	productsService := FakeDataSource(`{"data":{"topProducts":[{"name":"Table","__typename":"Product","upc":"1"},{"name":"Couch","__typename":"Product","upc":"2"},{"name":"Chair","__typename":"Product","upc":"3"}]}}`)
	stockService := FakeDataSource(`{"data":{"_entities":[{"stock":8},{"stock":2},{"stock":5}]}}`)
	reviewsService := FakeDataSource(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2"}}]},{"__typename":"Product","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1"}}]},{"__typename":"Product","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2"}}]}]}}`)
	usersService := FakeDataSource(`{"data":{"_entities":[{"name":"user-1"},{"name":"user-2"}]}}`)

	plan := &GraphQLResponse{
		Data: &Object{
			Fetch: &SingleFetch{
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}"}}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				DataSource: productsService,
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data"},
				},
			},
			Fields: []*Field{
				{
					Name: []byte("topProducts"),
					Value: &Array{
						Path: []string{"topProducts"},
						Item: &Object{
							Fetch: &ParallelFetch{
								Fetches: []Fetch{
									&BatchFetch{
										Input: BatchInput{
											Header: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {reviews {body author {__typename id}}}}}","variables":{"representations":[`),
														SegmentType: StaticSegmentType,
													},
												},
											},
											Items: []InputTemplate{
												{
													Segments: []TemplateSegment{
														{
															SegmentType:  VariableSegmentType,
															VariableKind: ResolvableObjectVariableKind,
															Renderer: NewGraphQLVariableResolveRenderer(&Object{
																Fields: []*Field{
																	{
																		Name: []byte("__typename"),
																		Value: &String{
																			Path: []string{"__typename"},
																		},
																	},
																	{
																		Name: []byte("upc"),
																		Value: &String{
																			Path: []string{"upc"},
																		},
																	},
																},
															}),
														},
													},
												},
											},
											Separator: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`,`),
														SegmentType: StaticSegmentType,
													},
												},
											},
											Footer: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`]}}}`),
														SegmentType: StaticSegmentType,
													},
												},
											},
										},
										DataSource: reviewsService,
										PostProcessing: PostProcessingConfiguration{
											SelectResponseDataPath: []string{"data", "_entities"},
										},
									},
									&BatchFetch{
										Input: BatchInput{
											Header: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`{"method":"POST","url":"http://stock","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {stock}}}","variables":{"representations":[`),
														SegmentType: StaticSegmentType,
													},
												},
											},
											Items: []InputTemplate{
												{
													Segments: []TemplateSegment{
														{
															SegmentType:  VariableSegmentType,
															VariableKind: ResolvableObjectVariableKind,
															Renderer: NewGraphQLVariableResolveRenderer(&Object{
																Fields: []*Field{
																	{
																		Name: []byte("__typename"),
																		Value: &String{
																			Path: []string{"__typename"},
																		},
																	},
																	{
																		Name: []byte("upc"),
																		Value: &String{
																			Path: []string{"upc"},
																		},
																	},
																},
															}),
														},
													},
												},
											},
											Separator: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`,`),
														SegmentType: StaticSegmentType,
													},
												},
											},
											Footer: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`]}}}`),
														SegmentType: StaticSegmentType,
													},
												},
											},
										},
										DataSource: stockService,
										PostProcessing: PostProcessingConfiguration{
											SelectResponseDataPath: []string{"data", "_entities"},
										},
									},
								},
							},
							Fields: []*Field{
								{
									Name: []byte("name"),
									Value: &String{
										Path: []string{"name"},
									},
								},
								{
									Name: []byte("stock"),
									Value: &Integer{
										Path: []string{"stock"},
									},
								},
								{
									Name: []byte("reviews"),
									Value: &Array{
										Path: []string{"reviews"},
										Item: &Object{
											Fields: []*Field{
												{
													Name: []byte("body"),
													Value: &String{
														Path: []string{"body"},
													},
												},
												{
													Name: []byte("author"),
													Value: &Object{
														Path: []string{"author"},
														Fetch: &BatchFetch{
															Input: BatchInput{
																Header: InputTemplate{
																	Segments: []TemplateSegment{
																		{
																			Data:        []byte(`{"method":"POST","url":"http://users","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name}}}","variables":{"representations":[`),
																			SegmentType: StaticSegmentType,
																		},
																	},
																},
																Items: []InputTemplate{
																	{
																		Segments: []TemplateSegment{
																			{
																				SegmentType:  VariableSegmentType,
																				VariableKind: ResolvableObjectVariableKind,
																				Renderer: NewGraphQLVariableResolveRenderer(&Object{
																					Fields: []*Field{
																						{
																							Name: []byte("__typename"),
																							Value: &String{
																								Path: []string{"__typename"},
																							},
																						},
																						{
																							Name: []byte("id"),
																							Value: &String{
																								Path: []string{"id"},
																							},
																						},
																					},
																				}),
																			},
																		},
																	},
																},
																Separator: InputTemplate{
																	Segments: []TemplateSegment{
																		{
																			Data:        []byte(`,`),
																			SegmentType: StaticSegmentType,
																		},
																	},
																},
																Footer: InputTemplate{
																	Segments: []TemplateSegment{
																		{
																			Data:        []byte(`]}}}`),
																			SegmentType: StaticSegmentType,
																		},
																	},
																},
															},
															DataSource: usersService,
															PostProcessing: PostProcessingConfiguration{
																SelectResponseDataPath: []string{"data", "_entities"},
															},
														},
														Fields: []*Field{
															{
																Name: []byte("name"),
																Value: &String{
																	Path: []string{"name"},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	expected := []byte(`{"data":{"topProducts":[{"name":"Table","stock":8,"reviews":[{"body":"Love Table!","author":{"name":"user-1"}},{"body":"Prefer other Table.","author":{"name":"user-2"}}]},{"name":"Couch","stock":2,"reviews":[{"body":"Couch Too expensive.","author":{"name":"user-1"}}]},{"name":"Chair","stock":5,"reviews":[{"body":"Chair Could be better.","author":{"name":"user-2"}}]}]}}`)

	pool := sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 1024))
		},
	}

	ctxPool := sync.Pool{
		New: func() interface{} {
			return NewContext(context.Background())
		},
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(expected)))
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := ctxPool.Get().(*Context)
			buf := pool.Get().(*bytes.Buffer)
			ctx.ctx = context.Background()
			err := resolver.ResolveGraphQLResponse(ctx, plan, nil, buf)
			if err != nil {
				b.Fatal(err)
			}
			if !bytes.Equal(expected, buf.Bytes()) {
				require.Equal(b, string(expected), buf.String())
			}

			buf.Reset()
			pool.Put(buf)

			ctx.Free()
			ctxPool.Put(ctx)
		}
	})
}

type hookContextPathMatcher struct {
	path string
}

func (h hookContextPathMatcher) Matches(x interface{}) bool {
	path := string(x.(HookContext).CurrentPath)
	return path == h.path
}

func (h hookContextPathMatcher) String() string {
	return fmt.Sprintf("is equal to %s", h.path)
}
