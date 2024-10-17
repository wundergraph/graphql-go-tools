package resolve

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/flags"
)

type _fakeDataSource struct {
	t                 TestingTB
	input             []byte
	data              []byte
	artificialLatency time.Duration
}

func (f *_fakeDataSource) Load(ctx context.Context, input []byte, out *bytes.Buffer) (err error) {
	if f.artificialLatency != 0 {
		time.Sleep(f.artificialLatency)
	}
	if f.input != nil {
		if !bytes.Equal(f.input, input) {
			require.Equal(f.t, string(f.input), string(input), "input mismatch")
		}
	}
	_, err = out.Write(f.data)
	return
}

func (f *_fakeDataSource) LoadWithFiles(ctx context.Context, input []byte, files []httpclient.File, out *bytes.Buffer) (err error) {
	if f.artificialLatency != 0 {
		time.Sleep(f.artificialLatency)
	}
	if f.input != nil {
		if !bytes.Equal(f.input, input) {
			require.Equal(f.t, string(f.input), string(input), "input mismatch")
		}
	}
	_, err = out.Write(f.data)
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

type TestErrorWriter struct {
}

func (t *TestErrorWriter) WriteError(ctx *Context, err error, res *GraphQLResponse, w io.Writer) {
	_, err = w.Write([]byte(fmt.Sprintf(`{"errors":[{"message":"%s"}],"data":null}`, err.Error())))
	if err != nil {
		panic(err)
	}
	err = w.(*SubscriptionRecorder).Flush()
	if err != nil {
		panic(err)
	}
}

func newResolver(ctx context.Context) *Resolver {
	return New(ctx, ResolverOptions{
		MaxConcurrency:               1024,
		Debug:                        false,
		PropagateSubgraphErrors:      true,
		PropagateSubgraphStatusCodes: true,
		AsyncErrorWriter:             &TestErrorWriter{},
	})
}

type customResolver struct{}

func (customResolver) Resolve(_ *Context, value []byte) ([]byte, error) {
	return value, nil
}

type customErrResolve struct{}

func (customErrResolve) Resolve(_ *Context, value []byte) ([]byte, error) {
	return nil, errors.New("custom error")
}

func TestResolver_ResolveNode(t *testing.T) {
	testFn := func(enableSingleFlight bool, fn func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string)) func(t *testing.T) {
		ctrl := gomock.NewController(t)
		rCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := newResolver(rCtx)
		response, ctx, expectedOutput := fn(t, ctrl)
		if t.Skipped() {
			return func(t *testing.T) {}
		}

		if response.Info == nil {
			response.Info = &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			}
		}

		return func(t *testing.T) {
			buf := &bytes.Buffer{}
			_, err := r.ResolveGraphQLResponse(&ctx, response, nil, buf)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutput, buf.String())
			ctrl.Finish()
		}
	}

	testGraphQLErrFn := func(fn func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedErr string)) func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		c, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := newResolver(c)
		response, ctx, expectedErr := fn(t, r, ctrl)

		if response.Info == nil {
			response.Info = &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			}
		}

		return func(t *testing.T) {
			t.Helper()
			buf := &bytes.Buffer{}
			_, err := r.ResolveGraphQLResponse(&ctx, response, nil, buf)
			assert.NoError(t, err)
			assert.Equal(t, expectedErr, buf.String())
			ctrl.Finish()
		}
	}

	t.Run("Nullable empty object", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: true,
			},
		}, Context{ctx: context.Background()}, `{"data":{}}`
	}))
	t.Run("empty object", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{},
		}, Context{ctx: context.Background()}, `{"data":{}}`
	}))
	t.Run("BigInt", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: FakeDataSource(`{"n": 12345, "ns_small": "12346", "ns_big": "1152921504606846976"}`),
				},
			}),
			Data: &Object{
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
			},
		}, Context{ctx: context.Background()}, `{"data":{"n":12345,"ns_small":"12346","ns_big":"1152921504606846976"}}`
	}))
	t.Run("Scalar", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"int": 12345, "float": 3.5, "int_str": "12346", "bigint_str": "1152921504606846976", "str":"value", "object":{"foo": "bar"}, "encoded_object": "{\"foo\": \"bar\"}"}`)},
			}),
			Data: &Object{
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
			},
		}, Context{ctx: context.Background()}, `{"data":{"int":12345,"float":3.5,"int_str":"12346","bigint_str":"1152921504606846976","str":"value","object":{"foo":"bar"},"encoded_object":"{\"foo\": \"bar\"}"}}`
	}))
	t.Run("object with null field", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Fields: []*Field{
					{
						Name:  []byte("foo"),
						Value: &Null{},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"data":{"foo":null}}`
	}))
	t.Run("default graphql object", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Fields: []*Field{
					{
						Name:  []byte("data"),
						Value: &Null{},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"data":{"data":null}}`
	}))
	t.Run("graphql object with simple data source", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`)},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
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
			},
		}, Context{ctx: context.Background()}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}}}`
	}))
	t.Run("skip single field should resolve to empty response", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`)},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
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
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"skip":true}`)}, `{"data":{"user":{}}}`
	}))
	t.Run("skip multiple fields should resolve to empty response", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`)},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
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
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"skip":true}`)}, `{"data":{"user":{}}}`
	}))
	t.Run("skip __typename field be possible", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`)},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
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
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"skip":true}`)}, `{"data":{"user":{"id":"1"}}}`
	}))
	t.Run("include __typename field be possible", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"},"__typename":"User"}`)},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
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
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"include":true}`)}, `{"data":{"user":{"id":"1","__typename":"User"}}}`
	}))
	t.Run("include __typename field with false value", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"},"__typename":"User"}`)},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
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
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"include":false}`)}, `{"data":{"user":{"id":"1"}}}`
	}))
	t.Run("skip field when skip variable is true", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`)},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
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
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"skip":true}`)}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky"}}}}`
	}))
	t.Run("don't skip field when skip variable is false", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`)},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
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
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"skip":false}`)}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}}}`
	}))
	t.Run("don't skip field when skip variable is missing", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`)},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
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
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{}`)}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}}}`
	}))
	t.Run("include field when include variable is true", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`)},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
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
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"include":true}`)}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}}}`
	}))
	t.Run("exclude field when include variable is false", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`)},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
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
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"include":false}`)}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky"}}}}`
	}))
	t.Run("exclude field when include variable is missing", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`)},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
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
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{}`)}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky"}}}}`
	}))
	t.Run("fetch with context variable resolver", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), []byte(`{"id":1}`), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			Do(func(ctx context.Context, input []byte, w *bytes.Buffer) (err error) {
				_, err = w.Write([]byte(`{"name":"Jens"}`))
				return
			}).
			Return(nil)
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					Input:      `{"id":$$0$$}`,
					Variables: NewVariables(&ContextVariable{
						Path: []string{"id"},
					}),
				},
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
							Renderer:           NewPlainVariableRenderer(),
						},
						{
							SegmentType: StaticSegmentType,
							Data:        []byte(`}`),
						},
					},
				},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("name"),
						Value: &String{
							Path: []string{"name"},
						},
					},
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"id":1}`)}, `{"data":{"name":"Jens"}}`
	}))
	t.Run("resolve array of strings", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"strings": ["Alex", "true", "123"]}`)},
			}),
			Data: &Object{
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
			},
		}, Context{ctx: context.Background()}, `{"data":{"strings":["Alex","true","123"]}}`
	}))
	t.Run("resolve array of mixed scalar types", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"strings": ["Alex", "true", 123]}`)},
			}),
			Data: &Object{
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
			},
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"String cannot represent non-string value: \"123\"","path":["strings",2]}],"data":null}`
	}))
	t.Run("resolve array items", func(t *testing.T) {
		t.Run("with unescape json enabled", func(t *testing.T) {
			t.Run("json encoded input", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
				return &GraphQLResponse{
					Fetches: Single(&SingleFetch{
						FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"jsonList":["{\"field\":\"value\"}"]}`)},
					}),
					Data: &Object{
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
					},
				}, Context{ctx: context.Background()}, `{"data":{"jsonList":[{"field":"value"}]}}`
			}))
		})
		t.Run("with unescape json disabled", func(t *testing.T) {
			t.Run("json encoded input", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
				return &GraphQLResponse{
					Fetches: Single(&SingleFetch{
						FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"jsonList":["{\"field\":\"value\"}"]}`)},
					}),
					Data: &Object{
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
					},
				}, Context{ctx: context.Background()}, `{"data":{"jsonList":["{\"field\":\"value\"}"]}}`
			}))
		})
	})
	t.Run("resolve arrays", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"friends":[{"id":1,"name":"Alex"},{"id":2,"name":"Patric"}],"strings":["foo","bar","baz"],"integers":[123,456,789],"floats":[1.2,3.4,5.6],"booleans":[true,false,true]}`)},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("synchronousFriends"),
						Value: &Array{
							Path:     []string{"friends"},
							Nullable: true,
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
							Path:     []string{"friends"},
							Nullable: true,
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
							Path:     []string{"strings"},
							Nullable: true,
							Item: &String{
								Nullable: false,
							},
						},
					},
					{
						Name: []byte("integers"),
						Value: &Array{
							Path:     []string{"integers"},
							Nullable: true,
							Item: &Integer{
								Nullable: false,
							},
						},
					},
					{
						Name: []byte("floats"),
						Value: &Array{
							Path:     []string{"floats"},
							Nullable: true,
							Item: &Float{
								Nullable: false,
							},
						},
					},
					{
						Name: []byte("booleans"),
						Value: &Array{
							Path:     []string{"booleans"},
							Nullable: true,
							Item: &Boolean{
								Nullable: false,
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"data":{"synchronousFriends":[{"id":1,"name":"Alex"},{"id":2,"name":"Patric"}],"asynchronousFriends":[{"id":1,"name":"Alex"},{"id":2,"name":"Patric"}],"nullableFriends":null,"strings":["foo","bar","baz"],"integers":[123,456,789],"floats":[1.2,3.4,5.6],"booleans":[true,false,true]}}`
	}))
	t.Run("array response from data source", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
				Fetches: Single(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: FakeDataSource(`{"data":{"pets":[{"__typename":"Dog","name":"Woofie"},{"__typename":"Cat","name":"Mietzie"}]}}`),
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
				}),
				Data: &Object{
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
				},
			}, Context{ctx: context.Background()},
			`{"data":{"pets":[{"name":"Woofie"},{}]}}`
	}))
	t.Run("non null object with field condition can be null", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
				Fetches: Single(&SingleFetch{
					FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"__typename":"Dog","name":"Woofie"}`)},
				}),
				Data: &Object{
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
				},
			}, Context{ctx: context.Background()},
			`{"data":{"cat":{}}}`
	}))
	t.Run("object with multiple type conditions", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
				Fetches: Single(&SingleFetch{
					FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"namespaceCreate":{"__typename":"Error","code":"UserAlreadyHasPersonalNamespace","message":""}}`)},
				}),
				Data: &Object{
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
				},
			}, Context{ctx: context.Background()},
			`{"data":{"namespaceCreate":{"code":"UserAlreadyHasPersonalNamespace","message":""}}}`
	}))
	t.Run("resolve fieldsets based on __typename", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
				Fetches: Single(&SingleFetch{
					FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"pets":[{"__typename":"Dog","name":"Woofie"},{"__typename":"Cat","name":"Mietzie"}]}`)},
				}),
				Data: &Object{
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
				},
			}, Context{ctx: context.Background()},
			`{"data":{"pets":[{"name":"Woofie"},{}]}}`
	}))

	t.Run("resolve fieldsets based on __typename when field is Nullable", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
				Fetches: Single(&SingleFetch{
					FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"pet":{"id": "1", "detail": null}}`)},
				}),
				Data: &Object{
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
				},
			}, Context{ctx: context.Background()},
			`{"data":{"pet":{"id":"1","detail":null}}}`
	}))

	t.Run("resolve fieldsets asynchronous based on __typename", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
				Fetches: Single(&SingleFetch{
					FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"pets":[{"__typename":"Dog","name":"Woofie"},{"__typename":"Cat","name":"Mietzie"}]}`)},
				}),
				Data: &Object{
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
				},
			}, Context{ctx: context.Background()},
			`{"data":{"pets":[{"name":"Woofie"},{}]}}`
	}))
	t.Run("with unescape json enabled", func(t *testing.T) {
		t.Run("json object within a string", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
			return &GraphQLResponse{
				Fetches: Single(&SingleFetch{
					// Datasource returns a JSON object within a string
					FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"data":"{\"hello\":\"world\",\"numberAsString\":\"1\",\"number\":1,\"bool\":true,\"null\":null,\"array\":[1,2,3],\"object\":{\"key\":\"value\"}}"}`)},
				}),
				Data: &Object{
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
				},
			}, Context{ctx: context.Background()}, `{"data":{"data":{"hello":"world","numberAsString":"1","number":1,"bool":true,"null":null,"array":[1,2,3],"object":{"key":"value"}}}}`
		}))
		t.Run("json array within a string", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
			return &GraphQLResponse{
				Fetches: Single(&SingleFetch{
					// Datasource returns a JSON array within a string
					FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"data":"[1,2,3]"}`)},
				}),
				Data: &Object{
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
				},
			}, Context{ctx: context.Background()}, `{"data":{"data":[1,2,3]}}`
		}))
		t.Run("plain scalar values within a string", func(t *testing.T) {
			t.Run("boolean", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
				return &GraphQLResponse{
					Fetches: Single(&SingleFetch{
						// Datasource returns a JSON boolean within a string
						FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"data":"true"}`)},
					}),
					Data: &Object{
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
					},
				}, Context{ctx: context.Background()}, `{"data":{"data":true}}`
			}))
			t.Run("int", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
				return &GraphQLResponse{
					Fetches: Single(&SingleFetch{
						// Datasource returns a JSON number within a string
						FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"data": "1"}`)},
					}),
					Data: &Object{
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
					},
				}, Context{ctx: context.Background()}, `{"data":{"data":1}}`
			}))
			t.Run("float", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
				return &GraphQLResponse{
					Fetches: Single(&SingleFetch{
						// Datasource returns a JSON number within a string
						FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"data": "2.0"}`)},
					}),
					Data: &Object{
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
					},
				}, Context{ctx: context.Background()}, `{"data":{"data":2.0}}`
			}))
			t.Run("null", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
				return &GraphQLResponse{
					Fetches: Single(&SingleFetch{
						// Datasource returns a JSON number within a string
						FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"data": "null"}`)},
					}),
					Data: &Object{
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
					},
				}, Context{ctx: context.Background()}, `{"data":{"data":null}}`
			}))
			t.Run("string", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
				return &GraphQLResponse{
					Fetches: Single(&SingleFetch{
						FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"data": "hello world"}`)},
					}),
					Data: &Object{
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
					},
				}, Context{ctx: context.Background()}, `{"data":{"data":"hello world"}}`
			}))
		})
	})

	t.Run("custom", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"id": "1"}`)},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("id"),
						Value: &CustomNode{
							CustomResolve: customResolver{},
							Path:          []string{"id"},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"data":{"id":"1"}}`
	}))
	t.Run("custom nullable", testGraphQLErrFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedErr string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"id": null}`)},
			}),
			Data: &Object{
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
			},
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.id'.","path":["id"]}],"data":null}`
	}))
	t.Run("custom error", testFn(false, func(t *testing.T, ctrl *gomock.Controller) (response *GraphQLResponse, ctx Context, expectedOut string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"id": "1"}`)},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("id"),
						Value: &CustomNode{
							CustomResolve: customErrResolve{},
							Path:          []string{"id"},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"custom error","path":["id"]}],"data":null}`
	}))
}

func testFn(fn func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string)) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		rCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := newResolver(rCtx)
		node, ctx, expectedOutput := fn(t, ctrl)

		if node.Info == nil {
			node.Info = &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			}
		}

		if t.Skipped() {
			return
		}

		buf := &bytes.Buffer{}
		_, err := r.ResolveGraphQLResponse(&ctx, node, nil, buf)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutput, buf.String())
		ctrl.Finish()
	}
}

type apolloCompatibilityOptions struct {
	valueCompletion     bool
	suppressFetchErrors bool
}

func testFnApolloCompatibility(fn func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string), options *apolloCompatibilityOptions) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		rCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		resolvableOptions := ResolvableOptions{
			ApolloCompatibilityValueCompletionInExtensions: true,
			ApolloCompatibilitySuppressFetchErrors:         false,
		}
		if options != nil {
			resolvableOptions.ApolloCompatibilityValueCompletionInExtensions = options.valueCompletion
			resolvableOptions.ApolloCompatibilitySuppressFetchErrors = options.suppressFetchErrors
		}
		r := New(rCtx, ResolverOptions{
			MaxConcurrency:               1024,
			Debug:                        false,
			PropagateSubgraphErrors:      true,
			PropagateSubgraphStatusCodes: true,
			AsyncErrorWriter:             &TestErrorWriter{},
			ResolvableOptions:            resolvableOptions,
		})
		node, ctx, expectedOutput := fn(t, ctrl)

		if node.Info == nil {
			node.Info = &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			}
		}

		if t.Skipped() {
			return
		}

		buf := &bytes.Buffer{}
		_, err := r.ResolveGraphQLResponse(&ctx, node, nil, buf)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutput, buf.String())
		ctrl.Finish()
	}
}

func testFnSubgraphErrorsPassthrough(fn func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string)) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		rCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := New(rCtx, ResolverOptions{
			MaxConcurrency:               1024,
			Debug:                        false,
			PropagateSubgraphErrors:      true,
			PropagateSubgraphStatusCodes: true,
			SubgraphErrorPropagationMode: SubgraphErrorPropagationModePassThrough,
		})
		node, ctx, expectedOutput := fn(t, ctrl)

		if node.Info == nil {
			node.Info = &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			}
		}

		if t.Skipped() {
			return
		}

		buf := &bytes.Buffer{}
		_, err := r.ResolveGraphQLResponse(&ctx, node, nil, buf)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutput, buf.String())
		ctrl.Finish()
	}
}

func testFnSubgraphErrorsWithExtensionFieldCode(fn func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string)) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		rCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := New(rCtx, ResolverOptions{
			MaxConcurrency:               1024,
			Debug:                        false,
			PropagateSubgraphErrors:      true,
			PropagateSubgraphStatusCodes: true,
			AllowedErrorExtensionFields:  []string{"code"},
			SubgraphErrorPropagationMode: SubgraphErrorPropagationModePassThrough,
		})
		node, ctx, expectedOutput := fn(t, ctrl)

		if node.Info == nil {
			node.Info = &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			}
		}

		if t.Skipped() {
			return
		}

		buf := &bytes.Buffer{}
		_, err := r.ResolveGraphQLResponse(&ctx, node, nil, buf)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutput, buf.String())
		ctrl.Finish()
	}
}

func testFnSubgraphErrorsWithExtensionFieldServiceName(fn func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string)) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		rCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := New(rCtx, ResolverOptions{
			MaxConcurrency:                     1024,
			Debug:                              false,
			PropagateSubgraphErrors:            true,
			PropagateSubgraphStatusCodes:       true,
			AttachServiceNameToErrorExtensions: true,
			AllowedErrorExtensionFields:        []string{"code"},
			DefaultErrorExtensionCode:          "DOWNSTREAM_SERVICE_ERROR",
			SubgraphErrorPropagationMode:       SubgraphErrorPropagationModePassThrough,
		})
		node, ctx, expectedOutput := fn(t, ctrl)

		if node.Info == nil {
			node.Info = &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			}
		}

		if t.Skipped() {
			return
		}

		buf := &bytes.Buffer{}
		_, err := r.ResolveGraphQLResponse(&ctx, node, nil, buf)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutput, buf.String())
		ctrl.Finish()
	}
}

func testFnSubgraphErrorsWithExtensionDefaultCode(fn func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string)) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		rCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := New(rCtx, ResolverOptions{
			MaxConcurrency:               1024,
			Debug:                        false,
			PropagateSubgraphErrors:      true,
			PropagateSubgraphStatusCodes: true,
			AllowedErrorExtensionFields:  []string{"code"},
			SubgraphErrorPropagationMode: SubgraphErrorPropagationModePassThrough,
			DefaultErrorExtensionCode:    "DOWNSTREAM_SERVICE_ERROR",
		})
		node, ctx, expectedOutput := fn(t, ctrl)

		if node.Info == nil {
			node.Info = &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			}
		}

		if t.Skipped() {
			return
		}

		buf := &bytes.Buffer{}
		_, err := r.ResolveGraphQLResponse(&ctx, node, nil, buf)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutput, buf.String())
		ctrl.Finish()
	}
}

func testFnNoSubgraphErrorForwarding(fn func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string)) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		rCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := New(rCtx, ResolverOptions{
			MaxConcurrency:               1024,
			Debug:                        false,
			PropagateSubgraphErrors:      false,
			PropagateSubgraphStatusCodes: false,
		})
		node, ctx, expectedOutput := fn(t, ctrl)

		if node.Info == nil {
			node.Info = &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			}
		}

		if t.Skipped() {
			return
		}

		buf := &bytes.Buffer{}
		_, err := r.ResolveGraphQLResponse(&ctx, node, nil, buf)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutput, buf.String())
		ctrl.Finish()
	}
}

func testFnWithPostEvaluation(fn func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx *Context, expectedOutput string, postEvaluation func(t *testing.T))) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		rCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := newResolver(rCtx)
		node, ctx, expectedOutput, postEvaluation := fn(t, ctrl)

		if t.Skipped() {
			return
		}

		buf := &bytes.Buffer{}
		_, err := r.ResolveGraphQLResponse(ctx, node, nil, buf)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutput, buf.String())
		ctrl.Finish()
		postEvaluation(t)
	}
}

func testFnWithError(fn func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedErrorMessage string)) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		rCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := newResolver(rCtx)
		node, ctx, expectedOutput := fn(t, ctrl)

		if t.Skipped() {
			return
		}

		buf := &bytes.Buffer{}
		_, err := r.ResolveGraphQLResponse(&ctx, node, nil, buf)
		assert.Error(t, err, expectedOutput)
		ctrl.Finish()
	}
}

func testFnSubgraphErrorsPassthroughAndOmitCustomFields(fn func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string)) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		rCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := New(rCtx, ResolverOptions{
			MaxConcurrency:               1024,
			Debug:                        false,
			PropagateSubgraphErrors:      true,
			PropagateSubgraphStatusCodes: true,
			SubgraphErrorPropagationMode: SubgraphErrorPropagationModePassThrough,
			AllowedErrorExtensionFields:  []string{"code"},
		})
		node, ctx, expectedOutput := fn(t, ctrl)

		if t.Skipped() {
			return
		}

		buf := &bytes.Buffer{}
		_, err := r.ResolveGraphQLResponse(&ctx, node, nil, buf)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutput, buf.String())
		ctrl.Finish()
	}
}

func TestResolver_ResolveGraphQLResponse(t *testing.T) {

	t.Run("empty graphql response", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: true,
			},
		}, Context{ctx: context.Background()}, `{"data":{}}`
	}))
	t.Run("__typename without renaming", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"id":1,"name":"Jannik","__typename":"User","rewritten":"User"}`)},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
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
	t.Run("__typename checks", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"id":1,"name":"Jannik","__typename":"NotUser","rewritten":"User"}`)},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
							TypeName:      "User",
							PossibleTypes: map[string]struct{}{"User": {}},
							SourceName:    "Users",
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
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"Subgraph 'Users' returned invalid value 'NotUser' for __typename field.","extensions":{"code":"INVALID_GRAPHQL"}}],"data":null}`
	}))
	t.Run("__typename checks apollo compatibility object", testFnApolloCompatibility(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"data":{"user":{"id":1,"name":"Jannik","__typename":"NotUser","rewritten":"User"}}}`), PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data"},
				}},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
							Path:          []string{"user"},
							TypeName:      "User",
							PossibleTypes: map[string]struct{}{"User": {}},
							SourceName:    "Users",
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
		}, Context{ctx: context.Background()}, `{"data":null,"extensions":{"valueCompletion":[{"message":"Invalid __typename found for object at field Query.user.","path":["user"],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`
	}, nil))
	t.Run("__typename checks apollo compatibility array", testFnApolloCompatibility(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"data":{"users":[{"id":1,"name":"Jannik","__typename":"NotUser","rewritten":"User"}]}}`), PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data"},
				}},
			}),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("users"),
						Value: &Array{
							Path: []string{"users"},
							Item: &Object{
								TypeName:      "User",
								PossibleTypes: map[string]struct{}{"User": {}},
								SourceName:    "Users",
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
			},
		}, Context{ctx: context.Background()}, `{"data":null,"extensions":{"valueCompletion":[{"message":"Invalid __typename found for object at array element of type User at index 0.","path":["users",0],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`
	}, nil))
	t.Run("__typename with renaming", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
				Fetches: Single(&SingleFetch{
					FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"id":1,"name":"Jannik","__typename":"User","rewritten":"User"}`)},
				}),
				Data: &Object{
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
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
	t.Run("empty graphql response for non-nullable object query field", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
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
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.country'.","path":["country"]}],"data":null}`
	}))
	t.Run("empty graphql response for non-nullable array query field", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: false,
				Fields: []*Field{
					{
						Name: []byte("countries"),
						Value: &Array{
							Path: []string{"countries"},
							Item: &Object{
								Fields: []*Field{
									{
										Name: []byte("name"),
										Value: &String{
											Nullable: true,
											Path:     []string{"name"},
										},
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.countries'.","path":["countries"]}],"data":null}`
	}))
	t.Run("fetch with simple error without datasource ID", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				pair := NewBufPair()
				pair.WriteErr([]byte("errorMessage"), nil, nil, nil)
				return writeGraphqlResponse(pair, w, false)
			})
		return &GraphQLResponse{
			Fetches: SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
			}, ""),
			Data: &Object{
				Nullable: false,
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
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"Failed to fetch from Subgraph.","extensions":{"errors":[{"message":"errorMessage"}]}}],"data":{"name":null}}`
	}))
	t.Run("fetch with simple error without datasource ID no subgraph error forwarding", testFnNoSubgraphErrorForwarding(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				pair := NewBufPair()
				pair.WriteErr([]byte("errorMessage"), nil, nil, nil)
				return writeGraphqlResponse(pair, w, false)
			})
		return &GraphQLResponse{
			Fetches: SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
			}, "query"),
			Data: &Object{
				Nullable: false,
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
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"Failed to fetch from Subgraph at Path 'query'."}],"data":{"name":null}}`
	}))
	t.Run("fetch with simple error", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				pair := NewBufPair()
				pair.WriteErr([]byte("errorMessage"), nil, nil, nil)
				return writeGraphqlResponse(pair, w, false)
			})
		return &GraphQLResponse{
			Fetches: SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "Users",
					DataSourceName: "Users",
				},
			}, "query"),
			Data: &Object{
				Nullable: false,
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
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"Failed to fetch from Subgraph 'Users' at Path 'query'.","extensions":{"errors":[{"message":"errorMessage"}]}}],"data":{"name":null}}`
	}))
	t.Run("fetch with simple error in pass through Subgraph Error Mode", testFnSubgraphErrorsPassthrough(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				pair := NewBufPair()
				pair.WriteErr([]byte("errorMessage"), nil, nil, nil)
				return writeGraphqlResponse(pair, w, false)
			})
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "Users",
					DataSourceName: "Users",
				},
			}),
			Data: &Object{
				Nullable: false,
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
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"errorMessage"}],"data":{"name":null}}`
	}))
	t.Run("fetch with pass through mode and omit custom fields", testFnSubgraphErrorsPassthroughAndOmitCustomFields(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) error {
				_, err := w.Write([]byte(`{"errors":[{"message":"errorMessage","longMessage":"This is a long message","extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}],"data":{"name":null}}`))
				return err
			})
		return &GraphQLResponse{
			Info: &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			},
			Fetches: SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "Users",
					DataSourceName: "Users",
				},
			}, "query"),
			Data: &Object{
				Nullable: false,
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
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"errorMessage","extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}],"data":{"name":null}}`
	}))
	t.Run("fetch with returned err (with DataSourceID)", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				return &net.AddrError{}
			})
		return &GraphQLResponse{
			Fetches: SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "Users",
					DataSourceName: "Users",
				},
			}, "query"),
			Data: &Object{
				Nullable: false,
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
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"Failed to fetch from Subgraph 'Users' at Path 'query'."}],"data":{"name":null}}`
	}))
	t.Run("fetch with returned err (no DataSourceID)", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				return &net.AddrError{}
			})
		return &GraphQLResponse{
			Fetches: SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
			}, "query"),
			Data: &Object{
				Nullable: false,
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
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"Failed to fetch from Subgraph at Path 'query'."}],"data":{"name":null}}`
	}))
	t.Run("fetch with returned err and non-nullable root field", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				return &net.AddrError{}
			})
		return &GraphQLResponse{
			Fetches: SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "Users",
					DataSourceName: "Users",
				},
			}, "query"),
			Data: &Object{
				Nullable: false,
				Fields: []*Field{
					{
						Name: []byte("name"),
						Value: &String{
							Path:     []string{"name"},
							Nullable: false,
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"Failed to fetch from Subgraph 'Users' at Path 'query'."}],"data":null}`
	}))
	t.Run("root field with nested non-nullable fields returns null", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"user":{"name":null,"age":1}}`)},
			}),
			Data: &Object{
				Nullable: false,
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*Field{
								{
									Name: []byte("name"),
									Value: &String{
										Path:     []string{"name"},
										Nullable: false,
									},
								},
								{
									Name: []byte("age"),
									Value: &Integer{
										Path:     []string{"age"},
										Nullable: false,
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.user.name'.","path":["user","name"]}],"data":{"user":null}}`
	}))
	t.Run("multiple root fields with nested non-nullable fields each return null", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"one":{"name":null,"age":1},"two":{"name":"user:","age":null}}`)},
			}),
			Data: &Object{
				Nullable: false,
				Fields: []*Field{
					{
						Name: []byte("one"),
						Value: &Object{
							Path:     []string{"one"},
							Nullable: true,
							Fields: []*Field{
								{
									Name: []byte("name"),
									Value: &String{
										Path:     []string{"name"},
										Nullable: false,
									},
								},
								{
									Name: []byte("age"),
									Value: &Integer{
										Path:     []string{"age"},
										Nullable: false,
									},
								},
							},
						},
					},
					{
						Name: []byte("two"),
						Value: &Object{
							Path:     []string{"two"},
							Nullable: true,
							Fields: []*Field{
								{
									Name: []byte("name"),
									Value: &String{
										Path:     []string{"name"},
										Nullable: false,
									},
								},
								{
									Name: []byte("age"),
									Value: &Integer{
										Path:     []string{"age"},
										Nullable: false,
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.one.name'.","path":["one","name"]},{"message":"Cannot return null for non-nullable field 'Query.two.age'.","path":["two","age"]}],"data":{"one":null,"two":null}}`
	}))
	t.Run("root field with double nested non-nullable field returns partial data", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"user":{"nested":{"name":null,"age":1},"age":1}}`)},
			}),
			Data: &Object{
				Nullable: false,
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*Field{
								{
									Name: []byte("nested"),
									Value: &Object{
										Path:     []string{"nested"},
										Nullable: true,
										Fields: []*Field{
											{
												Name: []byte("name"),
												Value: &String{
													Path:     []string{"name"},
													Nullable: false,
												},
											},
											{
												Name: []byte("age"),
												Value: &Integer{
													Path:     []string{"age"},
													Nullable: false,
												},
											},
										},
									},
								},
								{
									Name: []byte("age"),
									Value: &Integer{
										Path:     []string{"age"},
										Nullable: false,
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.user.nested.name'.","path":["user","nested","name"]}],"data":{"user":{"nested":null,"age":1}}}`
	}))
	t.Run("fetch with two Errors", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
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
			Fetches: SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
			}, "query"),
			Data: &Object{
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
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"Failed to fetch from Subgraph at Path 'query'.","extensions":{"errors":[{"message":"errorMessage1"},{"message":"errorMessage2"}]}}],"data":{"name":null}}`
	}))
	t.Run("non-nullable object in nullable field", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"nullable_field": null}`)},
			}, "query"),
			Data: &Object{
				Nullable: false,
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
					Fetches: Single(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource: FakeDataSource(fakeData),
							Input:      `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"{thing {id abstractThing {__typename ... on ConcreteOne {name}}}}"}}`,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}),
					Data: &Object{
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
													{
														Name: []byte("__typename"),
														Value: &String{
															Nullable: true,
															Path:     []string{"__typename"},
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

			t.Run("interface response with matching type", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
				return obj(`{"thing":{"id":"1","abstractThing":{"__typename":"ConcreteOne","name":"foo"}}}`),
					Context{ctx: context.Background()},
					`{"data":{"thing":{"id":"1","abstractThing":{"name":"foo","__typename":"ConcreteOne"}}}}`
			}))

			t.Run("interface response with not matching type", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
				return obj(`{"thing":{"id":"1","abstractThing":{"__typename":"ConcreteTwo"}}}`),
					Context{ctx: context.Background()},
					`{"data":{"thing":{"id":"1","abstractThing":{}}}}`
			}))
		})

		t.Run("array of not nullable fields", func(t *testing.T) {
			obj := func(fakeData string) *GraphQLResponse {
				return &GraphQLResponse{
					Fetches: Single(&SingleFetch{
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
						FetchConfiguration: FetchConfiguration{
							DataSource: FakeDataSource(fakeData),
							Input:      `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"{things {id abstractThing {__typename ... on ConcreteOne {name}}}}"}}`,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data"},
							},
						},
					}),
					Data: &Object{
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

			t.Run("interface response with matching type", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
				return obj(`{"data":{"things":[{"id":"1","abstractThing":{"__typename":"ConcreteOne","name":"foo"}}]}}`),
					Context{ctx: context.Background()},
					`{"data":{"things":[{"id":"1","abstractThing":{"name":"foo"}}]}}`
			}))

			t.Run("interface response with not matching type", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
				return obj(`{"data":{"things":[{"id":"1","abstractThing":{"__typename":"ConcreteTwo"}}]}}`),
					Context{ctx: context.Background()},
					`{"data":{"things":[{"id":"1","abstractThing":{}}]}}`
			}))
		})
	})

	t.Run("empty nullable array should resolve correctly", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"nullableArray": []}`)},
			}),
			Data: &Object{
				Nullable: true,
				Fields: []*Field{
					{
						Name: []byte("nullableArray"),
						Value: &Array{
							Path:     []string{"nullableArray"},
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
	t.Run("empty not nullable array should resolve correctly", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"some_path": []}`)},
			}),
			Data: &Object{
				Nullable: false,
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
	t.Run("when data null not nullable array should resolve to data null and errors", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: FakeDataSource(`{"data":null}`),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
			}, "query"),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("nonNullArray"),
						Value: &Array{
							Nullable: false,
							Path:     []string{"nonNullArray"},
							Item: &Object{
								Nullable: false,
								Fields: []*Field{
									{
										Name: []byte("foo"),
										Value: &String{
											Nullable: false,
											Path:     []string{"foo"},
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
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"Failed to fetch from Subgraph at Path 'query', Reason: no data or errors in response."}],"data":null}`
	}))
	t.Run("when data null and errors present not nullable array should result to null data upstream error and resolve error", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Fetches: SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(
					`{"errors":[{"message":"Could not get name","locations":[{"line":3,"column":5}],"path":["todos","0","name"]}],"data":null}`),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
			}, "query"),
			Data: &Object{
				Nullable: false,
				Fields: []*Field{
					{
						Name: []byte("todos"),
						Value: &Array{
							Nullable: true,
							Path:     []string{"todos"},
							Item: &Object{
								Nullable: false,
								Fields: []*Field{
									{
										Name: []byte("name"),
										Value: &String{
											Nullable: false,
											Path:     []string{"name"},
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
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"Failed to fetch from Subgraph at Path 'query'.","extensions":{"errors":[{"message":"Could not get name","locations":[{"line":3,"column":5}],"path":["todos","0","name"]}]}}],"data":{"todos":null}}`
	}))
	t.Run("complex GraphQL Server plan", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
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
			Fetches: Sequence(
				Parallel(
					SingleWithPath(&SingleFetch{
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
									Renderer:           NewPlainVariableRenderer(),
								},
								{
									SegmentType: StaticSegmentType,
									Data:        []byte(`,"firstArg":"`),
								},
								{
									SegmentType:        VariableSegmentType,
									VariableKind:       ContextVariableKind,
									VariableSourcePath: []string{"firstArg"},
									Renderer:           NewPlainVariableRenderer(),
								},
								{
									SegmentType: StaticSegmentType,
									Data:        []byte(`"}}}`),
								},
							},
						},
						FetchConfiguration: FetchConfiguration{
							Input:      `{"url":"https://service.one","body":{"query":"query($firstArg: String, $thirdArg: Int){serviceOne(serviceOneArg: $firstArg){fieldOne} anotherServiceOne(anotherServiceOneArg: $thirdArg){fieldOne} reusingServiceOne(reusingServiceOneArg: $firstArg){fieldOne}}","variables":{"thirdArg":$$1$$,"firstArg":$$0$$}}}`,
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
					}, "query"),
					SingleWithPath(&SingleFetch{
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
									Renderer:           NewPlainVariableRenderer(),
								},
								{
									SegmentType: StaticSegmentType,
									Data:        []byte(`,"secondArg":`),
								},
								{
									SegmentType:        VariableSegmentType,
									VariableKind:       ContextVariableKind,
									VariableSourcePath: []string{"secondArg"},
									Renderer:           NewPlainVariableRenderer(),
								},
								{
									SegmentType: StaticSegmentType,
									Data:        []byte(`}}}`),
								},
							},
						},
						FetchConfiguration: FetchConfiguration{
							Input:      `{"url":"https://service.two","body":{"query":"query($secondArg: Boolean, $fourthArg: Float){serviceTwo(serviceTwoArg: $secondArg){fieldTwo} secondServiceTwo(secondServiceTwoArg: $fourthArg){fieldTwo}}","variables":{"fourthArg":$$1$$,"secondArg":$$0$$}}}`,
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
					}, "query"),
				),
				SingleWithPath(&SingleFetch{
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								SegmentType: StaticSegmentType,
								Data:        []byte(`{"url":"https://service.one","body":{"query":"{serviceOne {fieldOne}}"}}`),
							},
						},
					},
					FetchConfiguration: FetchConfiguration{
						Input:      `{"url":"https://service.one","body":{"query":"{serviceOne {fieldOne}}"}}`,
						DataSource: nestedServiceOne,
						Variables:  Variables{},
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
				}, "query", ObjectPath("serviceTwo")),
			),
			Data: &Object{
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
		t.Run("simple", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

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
				Fetches: Sequence(
					SingleWithPath(&SingleFetch{
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						FetchConfiguration: FetchConfiguration{
							DataSource: userService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data"},
							},
						},
					}, "query"),
					SingleWithPath(&SingleFetch{
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
						FetchConfiguration: FetchConfiguration{
							DataSource: reviewsService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities", "0"},
							},
						},
					}, "query.me", ObjectPath("me")),
					SingleWithPath(&ParallelListItemFetch{
						Fetch: &SingleFetch{
							FetchConfiguration: FetchConfiguration{
								DataSource: productService,
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data", "_entities", "0"},
								},
							},
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
						},
					}, "query.me.reviews.@.product", ObjectPath("me"), ArrayPath("reviews"), ObjectPath("product")),
				),
				Data: &Object{
					Fields: []*Field{
						{
							Name: []byte("me"),
							Value: &Object{
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
		t.Run("federation with batch", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
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
				Fetches: Sequence(
					SingleWithPath(&SingleFetch{
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						FetchConfiguration: FetchConfiguration{
							DataSource: userService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data"},
							},
						},
					}, "query"),
					SingleWithPath(&SingleFetch{
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
						FetchConfiguration: FetchConfiguration{
							DataSource: reviewsService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities", "0"},
							},
						},
					}, "query.me", ObjectPath("me")),
					SingleWithPath(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource: productService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities"},
							},
						},
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
					}, "query.me.reviews.@.product", ObjectPath("me"), ArrayPath("reviews"), ObjectPath("product")),
				),
				Data: &Object{
					Fields: []*Field{
						{
							Name: []byte("me"),
							Value: &Object{
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
		t.Run("federation with merge paths", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
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
				Fetches: Sequence(
					SingleWithPath(&SingleFetch{
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						FetchConfiguration: FetchConfiguration{
							DataSource: userService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data"},
							},
						},
					}, "query"),
					SingleWithPath(&SingleFetch{
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
						FetchConfiguration: FetchConfiguration{
							DataSource: reviewsService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities", "0"},
							},
						},
					}, "query.me", ObjectPath("me")),
					SingleWithPath(&SingleFetch{
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
						FetchConfiguration: FetchConfiguration{
							DataSource: productService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities"},
								MergePath:              []string{"data"},
							},
						},
					}, "query.me.reviews.@.product", ObjectPath("me"), ArrayPath("reviews"), ObjectPath("product")),
				),
				Data: &Object{
					Fields: []*Field{
						{
							Name: []byte("me"),
							Value: &Object{
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
		t.Run("federation with null response", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
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
				Fetches: Sequence(
					SingleWithPath(&SingleFetch{
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						FetchConfiguration: FetchConfiguration{
							DataSource: userService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data"},
							},
						},
					}, "query"),
					SingleWithPath(&SingleFetch{
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
						FetchConfiguration: FetchConfiguration{
							DataSource: reviewsService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities", "0"},
							},
						},
					}, "query.me", ObjectPath("me")),
					SingleWithPath(&BatchEntityFetch{
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
					}, "query.me.reviews.@.product", ObjectPath("me"), ArrayPath("reviews"), ObjectPath("product")),
				),
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
						FetchConfiguration: FetchConfiguration{
							DataSource: userService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data"},
							},
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
									FetchConfiguration: FetchConfiguration{
										DataSource: reviewsService,
										PostProcessing: PostProcessingConfiguration{
											SelectResponseDataPath: []string{"data", "_entities", "0"},
										},
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
															Fetch: &BatchEntityFetch{
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
		t.Run("federation with fetch error", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

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
				Fetches: Sequence(
					SingleWithPath(&SingleFetch{
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						FetchConfiguration: FetchConfiguration{
							DataSource: userService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath:   []string{"data"},
								SelectResponseErrorsPath: []string{"errors"},
							},
						},
					}, "query"),
					SingleWithPath(&SingleFetch{
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
									Renderer:           NewPlainVariableRenderer(),
								},
								{
									Data:        []byte(`","__typename":"User"}]}}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						FetchConfiguration: FetchConfiguration{
							DataSource: reviewsService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities", "0"},
							},
						},
					}, "query.me", ObjectPath("me")),
					SingleWithPath(&SingleFetch{
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
						FetchConfiguration: FetchConfiguration{
							DataSource: productService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities"},
							},
						},
					}, "query.me.reviews.@.product", ObjectPath("me"), ArrayPath("reviews"), ObjectPath("product")),
				),
				Data: &Object{
					Fields: []*Field{
						{
							Name: []byte("me"),
							Value: &Object{
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
			}, Context{ctx: context.Background(), Variables: nil}, `{"errors":[{"message":"Failed to fetch from Subgraph at Path 'query.me.reviews.@.product', Reason: no data or errors in response."},{"message":"Cannot return null for non-nullable field 'Query.me.reviews.product.name'.","path":["me","reviews",0,"product","name"]},{"message":"Cannot return null for non-nullable field 'Query.me.reviews.product.name'.","path":["me","reviews",1,"product","name"]}],"data":{"me":{"id":"1234","username":"Me","reviews":[null,null]}}}`
		}))
		t.Run("federation with fetch error and non null fields inside an array", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

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
				Fetches: Sequence(
					SingleWithPath(&SingleFetch{
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						FetchConfiguration: FetchConfiguration{
							DataSource: userService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data"},
							},
						},
					}, "query"),
					SingleWithPath(&SingleFetch{
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
									Renderer:           NewPlainVariableRenderer(),
								},
								{
									Data:        []byte(`","__typename":"User"}]}}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						FetchConfiguration: FetchConfiguration{
							DataSource: reviewsService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities", "0"},
							},
						},
					}, "query.me", ObjectPath("me")),
					SingleWithPath(&SingleFetch{
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
						FetchConfiguration: FetchConfiguration{
							DataSource: productService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities"},
							},
						},
					}, "query.me.reviews.@.product", ObjectPath("me"), ArrayPath("reviews"), ObjectPath("product")),
				),
				Data: &Object{
					Fields: []*Field{
						{
							Name: []byte("me"),
							Value: &Object{
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
			}, Context{ctx: context.Background(), Variables: nil}, `{"errors":[{"message":"Failed to fetch from Subgraph at Path 'query.me.reviews.@.product', Reason: no data or errors in response."},{"message":"Cannot return null for non-nullable field 'Query.me.reviews.product.name'.","path":["me","reviews",0,"product","name"]}],"data":{"me":{"id":"1234","username":"Me","reviews":null}}}`
		}))
		t.Run("federation with optional variable", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
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
				Fetches: Sequence(
					SingleWithPath(&SingleFetch{
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://localhost:8080/query","body":{"query":"{me {id}}"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						FetchConfiguration: FetchConfiguration{
							DataSource: userService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data"},
							},
						},
					}, "query"),
					SingleWithPath(&SingleFetch{
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
									Renderer:           NewJSONVariableRenderer(),
								},
								{
									Data:        []byte(`,"representations":[{"id":`),
									SegmentType: StaticSegmentType,
								},
								{
									SegmentType:        VariableSegmentType,
									VariableKind:       ObjectVariableKind,
									VariableSourcePath: []string{"id"},
									Renderer:           NewJSONVariableRenderer(),
								},
								{
									Data:        []byte(`,"__typename":"User"}]}}}`),
									SegmentType: StaticSegmentType,
								},
							},
							SetTemplateOutputToNullOnVariableNull: true,
						},
						FetchConfiguration: FetchConfiguration{
							DataSource: employeeService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities", "0"},
							},
						},
					}, "query.me", ObjectPath("me")),
					SingleWithPath(&SingleFetch{
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
									Renderer:           NewPlainVariableRenderer(),
								},
								{
									Data:        []byte(`,"representations":[{"id":`),
									SegmentType: StaticSegmentType,
								},
								{
									SegmentType:        VariableSegmentType,
									VariableKind:       ObjectVariableKind,
									VariableSourcePath: []string{"id"},
									Renderer:           NewJSONVariableRenderer(),
								},
								{
									Data:        []byte(`,"__typename":"Employee"}]}}}`),
									SegmentType: StaticSegmentType,
								},
							},
							SetTemplateOutputToNullOnVariableNull: true,
						},
						FetchConfiguration: FetchConfiguration{
							DataSource: timeService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities", "0"},
							},
						},
					}, "query.me.employment", ObjectPath("me"), ObjectPath("employment")),
				),
				Data: &Object{
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
										},
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

func TestResolver_ApolloCompatibilityMode_FetchError(t *testing.T) {
	options := apolloCompatibilityOptions{
		valueCompletion:     true,
		suppressFetchErrors: true,
	}
	t.Run("simple fetch with fetch error suppression", testFnApolloCompatibility(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				_, _ = w.Write([]byte("{}"))
				return
			})
		return &GraphQLResponse{
			Fetches: SingleWithPath(&SingleFetch{
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{query{name}}"}}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				FetchConfiguration: FetchConfiguration{
					DataSource: mockDataSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
			}, "query"),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("name"),
						Value: &String{
							Path: []string{"name"},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"data":null,"extensions":{"valueCompletion":[{"message":"Cannot return null for non-nullable field Query.name.","path":["name"],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`
	}, &options))
	t.Run("complex fetch with fetch error suppression", testFnApolloCompatibility(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
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
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					FetchConfiguration: FetchConfiguration{
						DataSource: userService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
				}, "query"),
				SingleWithPath(&SingleFetch{
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
								Renderer:           NewPlainVariableRenderer(),
							},
							{
								Data:        []byte(`","__typename":"User"}]}}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					FetchConfiguration: FetchConfiguration{
						DataSource: reviewsService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities", "0"},
						},
					},
				}, "query.me", ObjectPath("me")),
				SingleWithPath(&SingleFetch{
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
					FetchConfiguration: FetchConfiguration{
						DataSource: productService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities"},
						},
					},
				}, "query.me.reviews.@.product", ObjectPath("me"), ArrayPath("reviews"), ObjectPath("product")),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("me"),
						Value: &Object{
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
														TypeName: "Product",
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
		}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"me":{"id":"1234","username":"Me","reviews":null}},"extensions":{"valueCompletion":[{"message":"Cannot return null for non-nullable field Product.name.","path":["me","reviews",0,"product","name"],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`
	}, &options))
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
			resolver := newResolver(rCtx)

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
				Info: &GraphQLResponseInfo{
					OperationType: ast.OperationTypeQuery,
				},
				Fetches: SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: fakeService,
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       HeaderVariableKind,
								VariableSourcePath: []string{tc.variable},
							},
						},
					},
				}, "query"),
				Data: &Object{
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
			_, err := resolver.ResolveGraphQLResponse(ctx, res, nil, out)
			assert.NoError(t, err)
			assert.Equal(t, `{"data":{"bar":"baz"}}`, out.String())
		})
	}
}

type SubscriptionRecorder struct {
	buf      *bytes.Buffer
	messages []string
	complete atomic.Bool
	mux      sync.Mutex
}

func (s *SubscriptionRecorder) AwaitMessages(t *testing.T, count int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		s.mux.Lock()
		current := len(s.messages)
		s.mux.Unlock()
		if current == count {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for messages: %v", s.messages)
		}
		time.Sleep(time.Millisecond * 10)
	}
}

func (s *SubscriptionRecorder) AwaitAnyMessageCount(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		s.mux.Lock()
		current := len(s.messages)
		s.mux.Unlock()
		if current > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for messages: %v", s.messages)
		}
		time.Sleep(time.Millisecond * 10)
	}
}

func (s *SubscriptionRecorder) AwaitComplete(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if s.complete.Load() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for complete")
		}
		time.Sleep(time.Millisecond * 10)
	}
}

func (s *SubscriptionRecorder) Write(p []byte) (n int, err error) {
	s.mux.Lock()
	defer s.mux.Unlock()
	return s.buf.Write(p)
}

func (s *SubscriptionRecorder) Flush() error {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.messages = append(s.messages, s.buf.String())
	s.buf.Reset()
	return nil
}

func (s *SubscriptionRecorder) Complete() {
	s.complete.Store(true)
}

func (s *SubscriptionRecorder) Messages() []string {
	s.mux.Lock()
	defer s.mux.Unlock()
	return s.messages
}

func createFakeStream(messageFunc messageFunc, delay time.Duration, onStart func(input []byte)) *_fakeStream {
	return &_fakeStream{
		messageFunc: messageFunc,
		delay:       delay,
		onStart:     onStart,
	}
}

type messageFunc func(counter int) (message string, done bool)

type _fakeStream struct {
	messageFunc messageFunc
	onStart     func(input []byte)
	delay       time.Duration
	isDone      atomic.Bool
}

func (f *_fakeStream) AwaitIsDone(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if f.isDone.Load() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for complete")
		}
		time.Sleep(time.Millisecond * 10)
	}
}

func (f *_fakeStream) UniqueRequestID(ctx *Context, input []byte, xxh *xxhash.Digest) (err error) {
	_, err = xxh.WriteString("fakeStream")
	if err != nil {
		return
	}
	_, err = xxh.Write(input)
	return
}

func (f *_fakeStream) Start(ctx *Context, input []byte, updater SubscriptionUpdater) error {
	if f.onStart != nil {
		f.onStart(input)
	}
	go func() {
		counter := 0
		for {
			select {
			case <-ctx.ctx.Done():
				updater.Done()
				f.isDone.Store(true)
				return
			default:
				message, done := f.messageFunc(counter)
				updater.Update([]byte(message))
				if done {
					time.Sleep(f.delay)
					updater.Done()
					f.isDone.Store(true)
					return
				}
				counter++
				time.Sleep(f.delay)
			}
		}
	}()
	return nil
}

func TestResolver_ResolveGraphQLSubscription(t *testing.T) {
	defaultTimeout := time.Second * 30
	if flags.IsWindows {
		defaultTimeout = time.Second * 60
	}

	setup := func(ctx context.Context, stream SubscriptionDataSource) (*Resolver, *GraphQLSubscription, *SubscriptionRecorder, SubscriptionIdentifier) {
		plan := &GraphQLSubscription{
			Trigger: GraphQLSubscriptionTrigger{
				Source: stream,
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							SegmentType: StaticSegmentType,
							Data:        []byte(`{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`),
						},
					},
				},
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

		out := &SubscriptionRecorder{
			buf:      &bytes.Buffer{},
			messages: []string{},
			complete: atomic.Bool{},
		}
		out.complete.Store(false)

		id := SubscriptionIdentifier{
			ConnectionID:   1,
			SubscriptionID: 1,
		}

		return newResolver(ctx), plan, out, id
	}

	t.Run("should return errors if the upstream data has errors", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return `{"errors":[{"message":"Validation error occurred","locations":[{"line":1,"column":1}],"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}],"data":null}`, true
		}, 0, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`, string(input))
		})

		resolver, plan, recorder, id := setup(c, fakeStream)

		ctx := &Context{
			ctx: context.Background(),
		}

		err := resolver.AsyncResolveGraphQLSubscription(ctx, plan, recorder, id)
		assert.NoError(t, err)
		recorder.AwaitMessages(t, 1, defaultTimeout)
		recorder.AwaitComplete(t, defaultTimeout)
		assert.Equal(t, 1, len(recorder.Messages()))
		assert.Equal(t, `{"errors":[{"message":"Validation error occurred","locations":[{"line":1,"column":1}],"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}],"data":null}`, recorder.Messages()[0])
	})

	t.Run("should return an error if the data source has not been defined", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		resolver, plan, recorder, id := setup(c, nil)

		ctx := &Context{
			ctx: context.Background(),
		}

		err := resolver.AsyncResolveGraphQLSubscription(ctx, plan, recorder, id)
		assert.Error(t, err)
	})

	t.Run("should successfully get result from upstream", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), counter == 2
		}, 0, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`, string(input))
		})

		resolver, plan, recorder, id := setup(c, fakeStream)

		ctx := &Context{
			ctx: context.Background(),
		}

		err := resolver.AsyncResolveGraphQLSubscription(ctx, plan, recorder, id)
		assert.NoError(t, err)
		recorder.AwaitComplete(t, defaultTimeout)
		assert.Equal(t, 3, len(recorder.Messages()))
		assert.ElementsMatch(t, []string{
			`{"data":{"counter":0}}`,
			`{"data":{"counter":1}}`,
			`{"data":{"counter":2}}`,
		}, recorder.Messages())
	})

	t.Run("should propagate extensions to stream", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), counter == 2
		}, 0, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }","extensions":{"foo":"bar"}}}`, string(input))
		})

		resolver, plan, recorder, id := setup(c, fakeStream)

		ctx := Context{
			ctx:        context.Background(),
			Extensions: []byte(`{"foo":"bar"}`),
		}

		err := resolver.AsyncResolveGraphQLSubscription(&ctx, plan, recorder, id)
		assert.NoError(t, err)
		recorder.AwaitComplete(t, defaultTimeout)
		assert.Equal(t, 3, len(recorder.Messages()))
		assert.ElementsMatch(t, []string{
			`{"data":{"counter":0}}`,
			`{"data":{"counter":1}}`,
			`{"data":{"counter":2}}`,
		}, recorder.Messages())
	})

	t.Run("should propagate initial payload to stream", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), counter == 2
		}, 0, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"},"initial_payload":{"hello":"world"}}`, string(input))
		})

		resolver, plan, recorder, id := setup(c, fakeStream)

		ctx := Context{
			ctx:            context.Background(),
			InitialPayload: []byte(`{"hello":"world"}`),
		}

		err := resolver.AsyncResolveGraphQLSubscription(&ctx, plan, recorder, id)
		assert.NoError(t, err)
		recorder.AwaitComplete(t, defaultTimeout)
		assert.Equal(t, 3, len(recorder.Messages()))
		assert.ElementsMatch(t, []string{
			`{"data":{"counter":0}}`,
			`{"data":{"counter":1}}`,
			`{"data":{"counter":2}}`,
		}, recorder.Messages())
	})

	t.Run("should stop stream on unsubscribe subscription", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), false
		}, time.Millisecond*10, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`, string(input))
		})

		resolver, plan, recorder, id := setup(c, fakeStream)

		ctx := Context{
			ctx: context.Background(),
		}

		err := resolver.AsyncResolveGraphQLSubscription(&ctx, plan, recorder, id)
		assert.NoError(t, err)
		recorder.AwaitAnyMessageCount(t, defaultTimeout)
		err = resolver.AsyncUnsubscribeSubscription(id)
		assert.NoError(t, err)
		recorder.AwaitComplete(t, defaultTimeout)
		fakeStream.AwaitIsDone(t, defaultTimeout)
	})

	t.Run("should stop stream on unsubscribe client", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), false
		}, time.Millisecond*10, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`, string(input))
		})

		resolver, plan, recorder, id := setup(c, fakeStream)

		ctx := Context{
			ctx: context.Background(),
		}

		err := resolver.AsyncResolveGraphQLSubscription(&ctx, plan, recorder, id)
		assert.NoError(t, err)
		recorder.AwaitAnyMessageCount(t, defaultTimeout)
		err = resolver.AsyncUnsubscribeClient(id.ConnectionID)
		assert.NoError(t, err)
		recorder.AwaitComplete(t, defaultTimeout)
		fakeStream.AwaitIsDone(t, defaultTimeout)
	})
}

func Test_ResolveGraphQLSubscriptionWithFilter(t *testing.T) {
	defaultTimeout := time.Second * 30
	if flags.IsWindows {
		defaultTimeout = time.Second * 60
	}

	/*

		GraphQL Schema:

		directive @key(fields: String!) repeatable on OBJECT | INTERFACE

		directive @openfed__subscriptionFilter(
			condition: SubscriptionFilter!
		) on FIELD_DEFINITION

		input openfed__SubscriptionFilterCondition {
			AND: [openfed__SubscriptionFilterCondition!]
			OR: [openfed__SubscriptionFilterCondition!]
			NOT: openfed__SubscriptionFilterCondition
			IN: openfed__SubscriptionFieldCondition
		}

		input openfed__SubscriptionFieldCondition {
			field: String!
			values: [String!]
		}

		type Subscription {
			oneUserByID(id: ID!): User @openfed__subscriptionFilter(condition: { IN: { field: "id", values: ["{{ args.id }}"] } })
			oneUserNotByID(id: ID!): User  @openfed__subscriptionFilter(condition: { NOT: { IN: { field: "id", values: ["{{ args.id }}"] } } })
			oneUserOrByID(first: ID! second: ID!) : User @openfed__subscriptionFilter(condition: { OR: [{ IN: { field: "id", values: ["{{ args.first }}"] } }, { IN: { field: "id", values: ["{{ args.second }}"] } }] })
			oneUserAndByID(id: ID! email: String!): User @openfed__subscriptionFilter(condition: { AND: [{ IN: { field: "id", values: ["{{ args.id }}"] } }, { IN: { field: "email", values: ["{{ args.email }}"] } }] })
			oneUserByInput(input: UserEmailInput!): User @openfed__subscriptionFilter(condition: { IN: { field: "email", values: ["{{ args.input.email }}"] } })
			oneUserByLegacyID(id: ID!): User @openfed__subscriptionFilter(condition: { IN: { field: "legacy.id", values: ["{{ args.id }}"] } })
			manyUsersByIDs(ids: [ID!]!): [User] @openfed__subscriptionFilter(condition: { IN: { field: "id", values: ["{{ args.ids }}"] } })
			manyUsersNotInIDs(ids: [ID!]!): [User] @openfed__subscriptionFilter(condition: { NOT_IN: { field: "id", values: ["{{ args.ids }}"] } })
		}

		type User @key(fields: "id") @key(fields: "email") @key(fields: "legacy.id") {
			id: ID!
			email: String!
			name: String!
			legacy: LegacyUser
		}

		type LegacyUser {
			id: ID!
		}

		input UserEmailInput {
			email: String!
		}

	*/

	t.Run("matching entity should be included", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		count := 0

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			count++
			if count <= 3 {
				return `{"id":1}`, false
			}
			return `{"id":2}`, true
		}, 0, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000"}`, string(input))
		})

		plan := &GraphQLSubscription{
			Trigger: GraphQLSubscriptionTrigger{
				Source: fakeStream,
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							SegmentType: StaticSegmentType,
							Data:        []byte(`{"method":"POST","url":"http://localhost:4000"}`),
						},
					},
				},
			},
			Filter: &SubscriptionFilter{
				In: &SubscriptionFieldFilter{
					FieldPath: []string{"id"},
					Values: []InputTemplate{
						{
							Segments: []TemplateSegment{
								{
									SegmentType:        VariableSegmentType,
									VariableKind:       ContextVariableKind,
									VariableSourcePath: []string{"id"},
									Renderer:           NewPlainVariableRenderer(),
								},
							},
						},
					},
				},
			},
			Response: &GraphQLResponse{
				Data: &Object{
					Fields: []*Field{
						{
							Name: []byte("oneUserByID"),
							Value: &Object{
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &Integer{
											Path: []string{"id"},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		out := &SubscriptionRecorder{
			buf:      &bytes.Buffer{},
			messages: []string{},
			complete: atomic.Bool{},
		}
		out.complete.Store(false)

		id := SubscriptionIdentifier{
			ConnectionID:   1,
			SubscriptionID: 1,
		}

		resolver := newResolver(c)

		ctx := &Context{
			Variables: []byte(`{"id":1}`),
		}

		err := resolver.AsyncResolveGraphQLSubscription(ctx, plan, out, id)
		assert.NoError(t, err)
		out.AwaitComplete(t, defaultTimeout)
		assert.Equal(t, 3, len(out.Messages()))
		assert.ElementsMatch(t, []string{
			`{"data":{"oneUserByID":{"id":1}}}`,
			`{"data":{"oneUserByID":{"id":1}}}`,
			`{"data":{"oneUserByID":{"id":1}}}`,
		}, out.Messages())
	})

	t.Run("non-matching entity should remain", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		count := 0

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			count++
			if count <= 3 {
				return `{"id":1}`, false
			}
			return `{"id":2}`, true
		}, 0, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000"}`, string(input))
		})

		plan := &GraphQLSubscription{
			Trigger: GraphQLSubscriptionTrigger{
				Source: fakeStream,
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							SegmentType: StaticSegmentType,
							Data:        []byte(`{"method":"POST","url":"http://localhost:4000"}`),
						},
					},
				},
			},
			Filter: &SubscriptionFilter{
				In: &SubscriptionFieldFilter{
					FieldPath: []string{"id"},
					Values: []InputTemplate{
						{
							Segments: []TemplateSegment{
								{
									SegmentType:        VariableSegmentType,
									VariableKind:       ContextVariableKind,
									VariableSourcePath: []string{"id"},
									Renderer:           NewPlainVariableRenderer(),
								},
							},
						},
					},
				},
			},
			Response: &GraphQLResponse{
				Data: &Object{
					Fields: []*Field{
						{
							Name: []byte("oneUserByID"),
							Value: &Object{
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &Integer{
											Path: []string{"id"},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		out := &SubscriptionRecorder{
			buf:      &bytes.Buffer{},
			messages: []string{},
			complete: atomic.Bool{},
		}
		out.complete.Store(false)

		id := SubscriptionIdentifier{
			ConnectionID:   1,
			SubscriptionID: 1,
		}

		resolver := newResolver(c)

		ctx := &Context{
			Variables: []byte(`{"id":2}`),
		}

		err := resolver.AsyncResolveGraphQLSubscription(ctx, plan, out, id)
		assert.NoError(t, err)
		out.AwaitComplete(t, defaultTimeout)
		assert.Equal(t, 1, len(out.Messages()))
		assert.ElementsMatch(t, []string{
			`{"data":{"oneUserByID":{"id":2}}}`,
		}, out.Messages())
	})

	t.Run("matching array values should be included", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		count := 0

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			count++
			if count <= 3 {
				return fmt.Sprintf(`{"id":%d}`, count), false
			}
			return `{"id":4}`, true
		}, 0, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000"}`, string(input))
		})

		plan := &GraphQLSubscription{
			Trigger: GraphQLSubscriptionTrigger{
				Source: fakeStream,
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							SegmentType: StaticSegmentType,
							Data:        []byte(`{"method":"POST","url":"http://localhost:4000"}`),
						},
					},
				},
			},
			Filter: &SubscriptionFilter{
				In: &SubscriptionFieldFilter{
					FieldPath: []string{"id"},
					Values: []InputTemplate{
						{
							Segments: []TemplateSegment{
								{
									SegmentType:        VariableSegmentType,
									VariableKind:       ContextVariableKind,
									VariableSourcePath: []string{"ids"},
									Renderer:           NewPlainVariableRenderer(),
								},
							},
						},
					},
				},
			},
			Response: &GraphQLResponse{
				Data: &Object{
					Fields: []*Field{
						{
							Name: []byte("oneUserByID"),
							Value: &Object{
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &Integer{
											Path: []string{"id"},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		out := &SubscriptionRecorder{
			buf:      &bytes.Buffer{},
			messages: []string{},
			complete: atomic.Bool{},
		}
		out.complete.Store(false)

		id := SubscriptionIdentifier{
			ConnectionID:   1,
			SubscriptionID: 1,
		}

		resolver := newResolver(c)

		ctx := &Context{
			Variables: []byte(`{"ids":[1,2]}`),
		}

		err := resolver.AsyncResolveGraphQLSubscription(ctx, plan, out, id)
		assert.NoError(t, err)
		out.AwaitComplete(t, defaultTimeout)
		assert.Equal(t, 2, len(out.Messages()))
		assert.ElementsMatch(t, []string{
			`{"data":{"oneUserByID":{"id":1}}}`,
			`{"data":{"oneUserByID":{"id":2}}}`,
		}, out.Messages())
	})

	t.Run("matching array values with prefix should be included", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		count := 0

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			count++
			if count <= 3 {
				return fmt.Sprintf(`{"id":"x.%d"}`, count), false
			}
			return `{"id":"x.4"}`, true
		}, 0, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000"}`, string(input))
		})

		plan := &GraphQLSubscription{
			Trigger: GraphQLSubscriptionTrigger{
				Source: fakeStream,
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							SegmentType: StaticSegmentType,
							Data:        []byte(`{"method":"POST","url":"http://localhost:4000"}`),
						},
					},
				},
			},
			Filter: &SubscriptionFilter{
				In: &SubscriptionFieldFilter{
					FieldPath: []string{"id"},
					Values: []InputTemplate{
						{
							Segments: []TemplateSegment{
								{
									SegmentType: StaticSegmentType,
									Data:        []byte(`x.`),
								},
								{
									SegmentType:        VariableSegmentType,
									VariableKind:       ContextVariableKind,
									VariableSourcePath: []string{"ids"},
									Renderer:           NewPlainVariableRenderer(),
								},
							},
						},
					},
				},
			},
			Response: &GraphQLResponse{
				Data: &Object{
					Fields: []*Field{
						{
							Name: []byte("oneUserByID"),
							Value: &Object{
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
					},
				},
			},
		}

		out := &SubscriptionRecorder{
			buf:      &bytes.Buffer{},
			messages: []string{},
			complete: atomic.Bool{},
		}
		out.complete.Store(false)

		id := SubscriptionIdentifier{
			ConnectionID:   1,
			SubscriptionID: 1,
		}

		resolver := newResolver(c)

		ctx := &Context{
			Variables: []byte(`{"ids":["2","3"]}`),
		}

		err := resolver.AsyncResolveGraphQLSubscription(ctx, plan, out, id)
		assert.NoError(t, err)
		out.AwaitComplete(t, defaultTimeout)
		assert.Equal(t, 2, len(out.Messages()))
		assert.ElementsMatch(t, []string{
			`{"data":{"oneUserByID":{"id":"x.2"}}}`,
			`{"data":{"oneUserByID":{"id":"x.3"}}}`,
		}, out.Messages())
	})

	t.Run("should err when subscription filter has multiple templates", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		count := 0

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			count++
			if count <= 3 {
				return `{"id":"x.1"}`, false
			}
			return `{"id":"x.2"}`, true
		}, 0, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000"}`, string(input))
		})

		plan := &GraphQLSubscription{
			Trigger: GraphQLSubscriptionTrigger{
				Source: fakeStream,
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							SegmentType: StaticSegmentType,
							Data:        []byte(`{"method":"POST","url":"http://localhost:4000"}`),
						},
					},
				},
			},
			Filter: &SubscriptionFilter{
				In: &SubscriptionFieldFilter{
					FieldPath: []string{"id"},
					Values: []InputTemplate{
						{
							Segments: []TemplateSegment{
								{
									SegmentType: StaticSegmentType,
									Data:        []byte(`x.`),
								},
								{
									SegmentType:        VariableSegmentType,
									VariableKind:       ContextVariableKind,
									VariableSourcePath: []string{"a"},
									Renderer:           NewPlainVariableRenderer(),
								},
								{
									SegmentType: StaticSegmentType,
									Data:        []byte(`.`),
								},
								{
									SegmentType:        VariableSegmentType,
									VariableKind:       ContextVariableKind,
									VariableSourcePath: []string{"b"},
									Renderer:           NewPlainVariableRenderer(),
								},
							},
						},
					},
				},
			},
			Response: &GraphQLResponse{
				Data: &Object{
					Fields: []*Field{
						{
							Name: []byte("oneUserByID"),
							Value: &Object{
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
					},
				},
			},
		}

		out := &SubscriptionRecorder{
			buf:      &bytes.Buffer{},
			messages: []string{},
			complete: atomic.Bool{},
		}
		out.complete.Store(false)

		id := SubscriptionIdentifier{
			ConnectionID:   1,
			SubscriptionID: 1,
		}

		resolver := newResolver(c)

		ctx := &Context{
			Variables: []byte(`{"a":[1,2],"b":[3,4]}`),
		}

		err := resolver.AsyncResolveGraphQLSubscription(ctx, plan, out, id)
		assert.NoError(t, err)
		out.AwaitComplete(t, defaultTimeout)
		assert.Equal(t, 4, len(out.Messages()))
		assert.ElementsMatch(t, []string{
			`{"errors":[{"message":"invalid subscription filter template"}],"data":null}`,
			`{"errors":[{"message":"invalid subscription filter template"}],"data":null}`,
			`{"errors":[{"message":"invalid subscription filter template"}],"data":null}`,
			`{"errors":[{"message":"invalid subscription filter template"}],"data":null}`,
		}, out.Messages())
	})
}

func Benchmark_ResolveGraphQLResponse(b *testing.B) {
	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resolver := newResolver(rCtx)

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
				FetchConfiguration: FetchConfiguration{
					DataSource: userService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
			},
			Fields: []*Field{
				{
					Name: []byte("users"),
					Value: &Array{
						Path: []string{"users"},
						Item: &Object{
							Fetch: &BatchEntityFetch{
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
																			Path: []string{"0", "age"},
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
																			Path: []string{"1", "line1"},
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
			_, err = resolver.ResolveGraphQLResponse(ctx, plan, nil, buf)
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

func Test_NestedBatching_WithStats(t *testing.T) {
	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resolver := newResolver(rCtx)

	productsService := fakeDataSourceWithInputCheck(t,
		[]byte(`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}"}}`),
		[]byte(`{"data":{"topProducts":[{"name":"Table","__typename":"Product","upc":"1"},{"name":"Couch","__typename":"Product","upc":"2"},{"name":"Chair","__typename":"Product","upc":"3"}]}}`))
	stockService := fakeDataSourceWithInputCheck(t,
		[]byte(`{"method":"POST","url":"http://stock","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {stock}}}","variables":{"representations":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"},{"__typename":"Product","upc":"3"}]}}}`),
		[]byte(`{"data":{"_entities":[{"stock":8},{"stock":2},{"stock":5}]}}`))
	reviewsService := fakeDataSourceWithInputCheck(t,
		[]byte(`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {reviews {body author {__typename id}}}}}","variables":{"representations":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"},{"__typename":"Product","upc":"3"}]}}}`),
		[]byte(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2"}}]},{"__typename":"Product","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1"}}]},{"__typename":"Product","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2"}}]}]}}`))
	usersService := fakeDataSourceWithInputCheck(t,
		[]byte(`{"method":"POST","url":"http://users","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name}}}","variables":{"representations":[{"__typename":"User","id":"1"},{"__typename":"User","id":"2"}]}}}`),
		[]byte(`{"data":{"_entities":[{"name":"user-1"},{"name":"user-2"}]}}`))

	plan := &GraphQLResponse{
		Info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
		},
		Fetches: Sequence(
			SingleWithPath(&SingleFetch{
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}"}}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				FetchConfiguration: FetchConfiguration{
					DataSource: productsService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
			}, "query"),
			Parallel(
				SingleWithPath(&BatchEntityFetch{
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
				}, "query.topProducts", ArrayPath("topProducts")),
				SingleWithPath(&BatchEntityFetch{
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
				}, "query.topProducts", ArrayPath("topProducts")),
			),
			SingleWithPath(&BatchEntityFetch{
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
			}, "query.topProducts.@.reviews.author", ArrayPath("topProducts"), ArrayPath("reviews"), ObjectPath("author")),
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("topProducts"),
					Value: &Array{
						Path: []string{"topProducts"},
						Item: &Object{
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

	ctx := NewContext(context.Background())
	buf := &bytes.Buffer{}

	_, err := resolver.ResolveGraphQLResponse(ctx, plan, nil, buf)
	assert.NoError(t, err)
	assert.Equal(t, string(expected), buf.String())
	assert.Equal(t, 29, ctx.Stats.ResolvedNodes, "resolved nodes")
	assert.Equal(t, 11, ctx.Stats.ResolvedObjects, "resolved objects")
	assert.Equal(t, 14, ctx.Stats.ResolvedLeafs, "resolved leafs")
	assert.Equal(t, int64(711), ctx.Stats.CombinedResponseSize.Load(), "combined response size")
	assert.Equal(t, int32(4), ctx.Stats.NumberOfFetches.Load(), "number of fetches")

	ctx.Free()
	ctx = ctx.WithContext(context.Background())
	buf.Reset()
	_, err = resolver.ResolveGraphQLResponse(ctx, plan, nil, buf)
	assert.NoError(t, err)
	assert.Equal(t, string(expected), buf.String())
	assert.Equal(t, 29, ctx.Stats.ResolvedNodes, "resolved nodes")
	assert.Equal(t, 11, ctx.Stats.ResolvedObjects, "resolved objects")
	assert.Equal(t, 14, ctx.Stats.ResolvedLeafs, "resolved leafs")
	assert.Equal(t, int64(711), ctx.Stats.CombinedResponseSize.Load(), "combined response size")
	assert.Equal(t, int32(4), ctx.Stats.NumberOfFetches.Load(), "number of fetches")
}

func Benchmark_NestedBatching(b *testing.B) {
	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resolver := newResolver(rCtx)

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
		Fetches: Sequence(
			SingleWithPath(&SingleFetch{
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}"}}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				FetchConfiguration: FetchConfiguration{
					DataSource: productsService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
			}, ""),
			Parallel(
				SingleWithPath(&BatchEntityFetch{
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
				}, "topProducts", ArrayPath("topProducts")),
				SingleWithPath(&BatchEntityFetch{
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
				}, "topProducts", ArrayPath("topProducts")),
			),
			SingleWithPath(&BatchEntityFetch{
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
			}, "topProducts.@.reviews.@.author", ArrayPath("topProducts"), ArrayPath("reviews"), ObjectPath("author")),
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("topProducts"),
					Value: &Array{
						Path: []string{"topProducts"},
						Item: &Object{
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
			_, err := resolver.ResolveGraphQLResponse(ctx, plan, nil, buf)
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

	resolver := newResolver(rCtx)

	productsService := FakeDataSource(`{"data":{"topProducts":[{"name":"Table","__typename":"Product","upc":"1"},{"name":"Couch","__typename":"Product","upc":"2"},{"name":"Chair","__typename":"Product","upc":"3"}]}}`)
	stockService := FakeDataSource(`{"data":{"_entities":[{"stock":8},{"stock":2},{"stock":5}]}}`)
	reviewsService := FakeDataSource(`{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2"}}]},{"__typename":"Product","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1"}}]},{"__typename":"Product","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2"}}]}]}}`)
	usersService := FakeDataSource(`{"data":{"_entities":[{"name":"user-1"},{"name":"user-2"}]}}`)

	plan := &GraphQLResponse{
		Fetches: Sequence(
			SingleWithPath(&SingleFetch{
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}"}}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				FetchConfiguration: FetchConfiguration{
					DataSource: productsService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				DataSourceIdentifier: []byte("graphql"),
			}, "query"),
			Parallel(
				SingleWithPath(&BatchEntityFetch{
					DataSourceIdentifier: []byte("graphql"),
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
				}, "query.topProducts", ObjectPath("topProducts")),
				SingleWithPath(&BatchEntityFetch{
					DataSourceIdentifier: []byte("graphql"),
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
				}, "query.topProducts", ObjectPath("topProducts")),
			),
			SingleWithPath(&BatchEntityFetch{
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
			}, "query.topProducts.reviews.author", ObjectPath("topProducts"), ArrayPath("reviews"), ObjectPath("author")),
		),
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
				FetchConfiguration: FetchConfiguration{
					DataSource: productsService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				DataSourceIdentifier: []byte("graphql"),
			},
			Fields: []*Field{
				{
					Name: []byte("topProducts"),
					Value: &Array{
						Path: []string{"topProducts"},
						Item: &Object{
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
			_, err := resolver.ResolveGraphQLResponse(ctx, plan, nil, buf)
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
