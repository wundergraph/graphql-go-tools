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
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/pkg/fastbuffer"
)

type _fakeDataSource struct {
	data              []byte
	artificialLatency time.Duration
}

func (f *_fakeDataSource) Load(ctx context.Context, input []byte, w io.Writer) (err error) {
	if f.artificialLatency != 0 {
		time.Sleep(f.artificialLatency)
	}
	_, err = w.Write(f.data)
	return
}

func FakeDataSource(data string) *_fakeDataSource {
	return &_fakeDataSource{
		data: []byte(data),
	}
}

type _fakeDataSourceBatch struct {
	resultedInput *fastbuffer.FastBuffer
	bufPairs      []*BufPair
}

func (f *_fakeDataSourceBatch) Input() *fastbuffer.FastBuffer {
	return f.resultedInput
}

func (f *_fakeDataSourceBatch) Demultiplex(responseBufPair *BufPair, bufPairs []*BufPair) (err error) {
	for i := range bufPairs {
		bufPairs[i].Data.WriteBytes(f.bufPairs[i].Data.Bytes())
		bufPairs[i].Errors.WriteBytes(f.bufPairs[i].Errors.Bytes())
	}

	return nil
}

type resultedBufPair struct {
	data string
	err  string
}

func NewFakeDataSourceBatch(resultedInput string, resultedBufPairs []resultedBufPair) *_fakeDataSourceBatch {
	bufInput := fastbuffer.New()
	bufInput.WriteString(resultedInput)

	bufPairs := make([]*BufPair, len(resultedBufPairs))
	for i, v := range resultedBufPairs {
		bufPair := NewBufPair()
		bufPair.Data.WriteString(v.data)
		if v.err != "" {
			bufPair.WriteErr([]byte(v.err), nil, nil, nil)
		}

		bufPairs[i] = bufPair
	}

	return &_fakeDataSourceBatch{
		resultedInput: bufInput,
		bufPairs:      bufPairs,
	}
}

type _byteMatchter struct {
	data []byte
}

func (b _byteMatchter) Matches(x interface{}) bool {
	return bytes.Equal(b.data, x.([]byte))
}

func (b _byteMatchter) String() string {
	return "bytes: " + string(b.data)
}

func matchBytes(bytes string) *_byteMatchter {
	return &_byteMatchter{data: []byte(bytes)}
}

type gotBytesFormatter struct {
}

func (g gotBytesFormatter) Got(got interface{}) string {
	return "bytes: " + string(got.([]byte))
}

func newResolver(ctx context.Context, enableSingleFlight bool, enableDataLoader bool) *Resolver {
	return New(ctx, NewFetcher(enableSingleFlight), enableDataLoader)
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
	testFn := func(enableSingleFlight bool, enableDataLoader bool, fn func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string)) func(t *testing.T) {
		ctrl := gomock.NewController(t)
		rCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := newResolver(rCtx, enableSingleFlight, enableDataLoader)
		node, ctx, expectedOutput := fn(t, ctrl)
		return func(t *testing.T) {
			buf := &BufPair{
				Data:   fastbuffer.New(),
				Errors: fastbuffer.New(),
			}
			err := r.resolveNode(&ctx, node, nil, buf)
			assert.Equal(t, buf.Errors.String(), "", "want error buf to be empty")
			assert.NoError(t, err)
			assert.Equal(t, expectedOutput, buf.Data.String())
			ctrl.Finish()
		}
	}

	testErrFn := func(fn func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedErr string)) func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		c, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := newResolver(c, false, false)
		node, ctx, expectedErr := fn(t, r, ctrl)
		return func(t *testing.T) {
			t.Helper()

			buf := &BufPair{
				Data:   fastbuffer.New(),
				Errors: fastbuffer.New(),
			}
			err := r.resolveNode(&ctx, node, nil, buf)
			assert.EqualError(t, err, expectedErr)
			ctrl.Finish()
		}
	}

	t.Run("Nullable empty object", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Nullable: true,
		}, Context{ctx: context.Background()}, `null`
	}))
	t.Run("empty object", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &EmptyObject{}, Context{ctx: context.Background()}, `{}`
	}))
	t.Run("BigInt", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fetch: &SingleFetch{
				BufferId:   0,
				DataSource: FakeDataSource(`{"n": 12345, "ns_small": "12346", "ns_big": "1152921504606846976"`),
			},
			Fields: []*Field{
				{
					BufferID:  0,
					HasBuffer: true,
					Name:      []byte("n"),
					Value: &BigInt{
						Path:     []string{"n"},
						Nullable: false,
					},
				},
				{
					BufferID:  0,
					HasBuffer: true,
					Name:      []byte("ns_small"),
					Value: &BigInt{
						Path:     []string{"ns_small"},
						Nullable: false,
					},
				},
				{
					BufferID:  0,
					HasBuffer: true,
					Name:      []byte("ns_big"),
					Value: &BigInt{
						Path:     []string{"ns_big"},
						Nullable: false,
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"n":12345,"ns_small":"12346","ns_big":"1152921504606846976"}`
	}))
	t.Run("object with null field", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name:  []byte("foo"),
					Value: &Null{},
				},
			},
		}, Context{ctx: context.Background()}, `{"foo":null}`
	}))
	t.Run("default graphql object", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Nullable: true,
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"data":null}`
	}))
	t.Run("graphql object with simple data source", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fields: []*Field{
							{
								Name: []byte("user"),
								Value: &Object{
									Fetch: &SingleFetch{
										BufferId:   0,
										DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
									},
									Fields: []*Field{
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("id"),
											Value: &String{
												Path: []string{"id"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("registered"),
											Value: &Boolean{
												Path: []string{"registered"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("pet"),
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
				},
			},
		}, Context{ctx: context.Background()}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}}}`
	}))
	t.Run("skip single field should resolve to empty response", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fields: []*Field{
							{
								Name: []byte("user"),
								Value: &Object{
									Fetch: &SingleFetch{
										BufferId:   0,
										DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
									},
									Fields: []*Field{
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("id"),
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
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"skip":true}`)}, `{"data":{"user":{}}}`
	}))
	t.Run("skip multiple fields should resolve to empty response", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fields: []*Field{
							{
								Name: []byte("user"),
								Value: &Object{
									Fetch: &SingleFetch{
										BufferId:   0,
										DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
									},
									Fields: []*Field{
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("id"),
											Value: &String{
												Path: []string{"id"},
											},
											SkipDirectiveDefined: true,
											SkipVariableName:     "skip",
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("name"),
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
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"skip":true}`)}, `{"data":{"user":{}}}`
	}))
	t.Run("skip __typename field be possible", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fields: []*Field{
							{
								Name: []byte("user"),
								Value: &Object{
									Fetch: &SingleFetch{
										BufferId:   0,
										DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
									},
									Fields: []*Field{
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("id"),
											Value: &String{
												Path: []string{"id"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("__typename"),
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
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"skip":true}`)}, `{"data":{"user":{"id":"1"}}}`
	}))
	t.Run("include __typename field be possible", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fields: []*Field{
							{
								Name: []byte("user"),
								Value: &Object{
									Fetch: &SingleFetch{
										BufferId:   0,
										DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"},"__typename":"User"}`),
									},
									Fields: []*Field{
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("id"),
											Value: &String{
												Path: []string{"id"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("__typename"),
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
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"include":true}`)}, `{"data":{"user":{"id":"1","__typename":"User"}}}`
	}))
	t.Run("include __typename field with false value", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fields: []*Field{
							{
								Name: []byte("user"),
								Value: &Object{
									Fetch: &SingleFetch{
										BufferId:   0,
										DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"},"__typename":"User"}`),
									},
									Fields: []*Field{
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("id"),
											Value: &String{
												Path: []string{"id"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("__typename"),
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
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"include":false}`)}, `{"data":{"user":{"id":"1"}}}`
	}))
	t.Run("skip field when skip variable is true", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fields: []*Field{
							{
								Name: []byte("user"),
								Value: &Object{
									Fetch: &SingleFetch{
										BufferId:   0,
										DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
									},
									Fields: []*Field{
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("id"),
											Value: &String{
												Path: []string{"id"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("registered"),
											Value: &Boolean{
												Path: []string{"registered"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("pet"),
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
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"skip":true}`)}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky"}}}}`
	}))
	t.Run("don't skip field when skip variable is false", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fields: []*Field{
							{
								Name: []byte("user"),
								Value: &Object{
									Fetch: &SingleFetch{
										BufferId:   0,
										DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
									},
									Fields: []*Field{
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("id"),
											Value: &String{
												Path: []string{"id"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("registered"),
											Value: &Boolean{
												Path: []string{"registered"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("pet"),
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
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"skip":false}`)}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}}}`
	}))
	t.Run("don't skip field when skip variable is missing", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fields: []*Field{
							{
								Name: []byte("user"),
								Value: &Object{
									Fetch: &SingleFetch{
										BufferId:   0,
										DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
									},
									Fields: []*Field{
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("id"),
											Value: &String{
												Path: []string{"id"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("registered"),
											Value: &Boolean{
												Path: []string{"registered"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("pet"),
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
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{}`)}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}}}`
	}))
	t.Run("include field when include variable is true", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fields: []*Field{
							{
								Name: []byte("user"),
								Value: &Object{
									Fetch: &SingleFetch{
										BufferId:   0,
										DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
									},
									Fields: []*Field{
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("id"),
											Value: &String{
												Path: []string{"id"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("registered"),
											Value: &Boolean{
												Path: []string{"registered"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("pet"),
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
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"include":true}`)}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}}}`
	}))
	t.Run("exclude field when include variable is false", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fields: []*Field{
							{
								Name: []byte("user"),
								Value: &Object{
									Fetch: &SingleFetch{
										BufferId:   0,
										DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
									},
									Fields: []*Field{
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("id"),
											Value: &String{
												Path: []string{"id"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("registered"),
											Value: &Boolean{
												Path: []string{"registered"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("pet"),
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
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"include":false}`)}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky"}}}}`
	}))
	t.Run("exclude field when include variable is missing", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fields: []*Field{
							{
								Name: []byte("user"),
								Value: &Object{
									Fetch: &SingleFetch{
										BufferId:   0,
										DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
									},
									Fields: []*Field{
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("id"),
											Value: &String{
												Path: []string{"id"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("registered"),
											Value: &Boolean{
												Path: []string{"registered"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("pet"),
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
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{}`)}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky"}}}}`
	}))
	t.Run("fetch with context variable resolver", testFn(true, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), []byte(`{"id":1}`), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				_, err = w.Write([]byte(`{"name":"Jens"}`))
				return
			}).
			Return(nil)
		return &Object{
			Fetch: &SingleFetch{
				BufferId:   0,
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
					HasBuffer: true,
					BufferID:  0,
					Name:      []byte("name"),
					Value: &String{
						Path: []string{"name"},
					},
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"id":1}`)}, `{"name":"Jens"}`
	}))
	t.Run("resolve array of strings", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fetch: &SingleFetch{
				BufferId:   0,
				DataSource: FakeDataSource(`{"strings": ["Alex", "true", "123"]}`),
			},
			Fields: []*Field{
				{
					BufferID:  0,
					HasBuffer: true,
					Name:      []byte("strings"),
					Value: &Array{
						Path: []string{"strings"},
						Item: &String{
							Nullable: false,
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"strings":["Alex","true","123"]}`
	}))
	t.Run("resolve array of mixed scalar types", testErrFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedErr string) {
		return &Object{
			Fetch: &SingleFetch{
				BufferId:   0,
				DataSource: FakeDataSource(`{"strings": ["Alex", "true", 123]}`),
			},
			Fields: []*Field{
				{
					BufferID:  0,
					HasBuffer: true,
					Name:      []byte("strings"),
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
			t.Run("json encoded input", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						BufferId:   0,
						DataSource: FakeDataSource(`{"jsonList":["{\"field\":\"value\"}"]}`),
					},
					Fields: []*Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("jsonList"),
							Value: &Array{
								Path: []string{"jsonList"},
								Item: &String{
									Nullable:             false,
									UnescapeResponseJson: true,
								},
							},
						},
					},
				}, Context{ctx: context.Background()}, `{"jsonList":[{"field":"value"}]}`
			}))
			t.Run("json input", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						BufferId:   0,
						DataSource: FakeDataSource(`{"jsonList":[{"field":"value"}]}`),
					},
					Fields: []*Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("jsonList"),
							Value: &Array{
								Path: []string{"jsonList"},
								Item: &String{
									Nullable:             false,
									UnescapeResponseJson: true,
								},
							},
						},
					},
				}, Context{ctx: context.Background()}, `{"jsonList":[{"field":"value"}]}`
			}))
		})
		t.Run("with unescape json disabled", func(t *testing.T) {
			t.Run("json encoded input", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						BufferId:   0,
						DataSource: FakeDataSource(`{"jsonList":["{\"field\":\"value\"}"]}`),
					},
					Fields: []*Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("jsonList"),
							Value: &Array{
								Path: []string{"jsonList"},
								Item: &String{
									Nullable:             false,
									UnescapeResponseJson: false,
								},
							},
						},
					},
				}, Context{ctx: context.Background()}, `{"jsonList":["{\"field\":\"value\"}"]}`
			}))
			t.Run("json input", testErrFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedErr string) {
				return &Object{
						Fetch: &SingleFetch{
							BufferId:   0,
							DataSource: FakeDataSource(`{"jsonList":[{"field":"value"}]}`),
						},
						Fields: []*Field{
							{
								BufferID:  0,
								HasBuffer: true,
								Name:      []byte("jsonList"),
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
	t.Run("resolve arrays", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fetch: &SingleFetch{
				BufferId:   0,
				DataSource: FakeDataSource(`{"friends":[{"id":1,"name":"Alex"},{"id":2,"name":"Patric"}],"strings":["foo","bar","baz"],"integers":[123,456,789],"floats":[1.2,3.4,5.6],"booleans":[true,false,true]}`),
			},
			Fields: []*Field{
				{
					BufferID:  0,
					HasBuffer: true,
					Name:      []byte("synchronousFriends"),
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
					BufferID:  0,
					HasBuffer: true,
					Name:      []byte("asynchronousFriends"),
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
					BufferID:  0,
					HasBuffer: true,
					Name:      []byte("nullableFriends"),
					Value: &Array{
						Path:     []string{"nonExistingField"},
						Nullable: true,
						Item:     &Object{},
					},
				},
				{
					BufferID:  0,
					HasBuffer: true,
					Name:      []byte("strings"),
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
					BufferID:  0,
					HasBuffer: true,
					Name:      []byte("integers"),
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
					BufferID:  0,
					HasBuffer: true,
					Name:      []byte("floats"),
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
					BufferID:  0,
					HasBuffer: true,
					Name:      []byte("booleans"),
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
		}, Context{ctx: context.Background()}, `{"synchronousFriends":[{"id":1,"name":"Alex"},{"id":2,"name":"Patric"}],"asynchronousFriends":[{"id":1,"name":"Alex"},{"id":2,"name":"Patric"}],"nullableFriends":null,"strings":["foo","bar","baz"],"integers":[123,456,789],"floats":[1.2,3.4,5.6],"booleans":[true,false,true]}`
	}))
	t.Run("array response from data source", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
				Fetch: &SingleFetch{
					BufferId:   0,
					DataSource: FakeDataSource(`[{"__typename":"Dog","name":"Woofie"},{"__typename":"Cat","name":"Mietzie"}]`),
				},
				Fields: []*Field{
					{
						BufferID:  0,
						HasBuffer: true,
						Name:      []byte("pets"),
						Value: &Array{
							Item: &Object{
								Fields: []*Field{
									{
										BufferID:    0,
										HasBuffer:   true,
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
			`{"pets":[{"name":"Woofie"}]}`
	}))
	t.Run("non null object with field condition can be null", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
				Fetch: &SingleFetch{
					BufferId:   0,
					DataSource: FakeDataSource(`{"__typename":"Dog","name":"Woofie"}`),
				},
				Fields: []*Field{
					{
						BufferID:  0,
						HasBuffer: true,
						Name:      []byte("cat"),
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
			`{}`
	}))
	t.Run("object with multiple type conditions", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
				Fetch: &SingleFetch{
					BufferId:   0,
					DataSource: FakeDataSource(`{"data":{"namespaceCreate":{"__typename":"Error","code":"UserAlreadyHasPersonalNamespace","message":""}}}`),
				},
				Fields: []*Field{
					{
						BufferID:  0,
						HasBuffer: true,
						Name:      []byte("data"),
						Value: &Object{
							Nullable: false,
							Path:     []string{"data"},
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
					},
				},
			}, Context{ctx: context.Background()},
			`{"data":{"namespaceCreate":{"code":"UserAlreadyHasPersonalNamespace","message":""}}}`
	}))
	t.Run("resolve fieldsets based on __typename", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
				Fetch: &SingleFetch{
					BufferId:   0,
					DataSource: FakeDataSource(`{"pets":[{"__typename":"Dog","name":"Woofie"},{"__typename":"Cat","name":"Mietzie"}]}`),
				},
				Fields: []*Field{
					{
						BufferID:  0,
						HasBuffer: true,
						Name:      []byte("pets"),
						Value: &Array{
							Path: []string{"pets"},
							Item: &Object{
								Fields: []*Field{
									{
										BufferID:    0,
										HasBuffer:   true,
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
			`{"pets":[{"name":"Woofie"}]}`
	}))

	t.Run("resolve fieldsets based on __typename when field is Nullable", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
				Fetch: &SingleFetch{
					BufferId:   0,
					DataSource: FakeDataSource(`{"pet":{"id": "1", "detail": null}}`),
				},
				Fields: []*Field{
					{
						BufferID:  0,
						HasBuffer: true,
						Name:      []byte("pet"),
						Value: &Object{
							Path: []string{"pet"},
							Fields: []*Field{
								{
									BufferID:  0,
									HasBuffer: false,
									Name:      []byte("id"),
									Value: &String{
										Path: []string{"id"},
									},
								},
								{
									BufferID:  0,
									HasBuffer: false,
									Name:      []byte("detail"),
									Value: &Object{
										Path:     []string{"detail"},
										Nullable: true,
										Fields: []*Field{
											{
												BufferID:    0,
												HasBuffer:   false,
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
			`{"pet":{"id":"1","detail":null}}`
	}))

	t.Run("resolve fieldsets asynchronous based on __typename", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
				Fetch: &SingleFetch{
					BufferId:   0,
					DataSource: FakeDataSource(`{"pets":[{"__typename":"Dog","name":"Woofie"},{"__typename":"Cat","name":"Mietzie"}]}`),
				},
				Fields: []*Field{
					{
						BufferID:  0,
						HasBuffer: true,
						Name:      []byte("pets"),
						Value: &Array{
							ResolveAsynchronous: true,
							Path:                []string{"pets"},
							Item: &Object{
								Fields: []*Field{
									{
										BufferID:    0,
										HasBuffer:   true,
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
			`{"pets":[{"name":"Woofie"}]}`
	}))
	t.Run("parent object variables", testFn(true, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.GotFormatterAdapter(gotBytesFormatter{}, matchBytes(`{"id":1}`)), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				_, err = w.Write([]byte(`{"name":"Woofie"}`))
				return
			}).
			Return(nil)
		return &Object{
			Fetch: &SingleFetch{
				BufferId:   0,
				DataSource: FakeDataSource(`{"id":1,"name":"Jens"}`),
			},
			Fields: []*Field{
				{
					HasBuffer: true,
					BufferID:  0,
					Name:      []byte("id"),
					Value: &Integer{
						Path: []string{"id"},
					},
				},
				{
					HasBuffer: true,
					BufferID:  0,
					Name:      []byte("name"),
					Value: &String{
						Path: []string{"name"},
					},
				},
				{
					HasBuffer: true,
					BufferID:  0,
					Name:      []byte("pet"),
					Value: &Object{
						Fetch: &SingleFetch{
							BufferId:   0,
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
										VariableKind:       ObjectVariableKind,
										VariableSourcePath: []string{"id"},
										Renderer:           NewGraphQLVariableRenderer(`{"type":"number"}`),
									},
									{
										SegmentType: StaticSegmentType,
										Data:        []byte(`}`),
									},
								},
							},
							Variables: NewVariables(&ObjectVariable{
								Path: []string{"id"},
							}),
						},
						Fields: []*Field{
							{
								BufferID:  0,
								HasBuffer: true,
								Name:      []byte("name"),
								Value: &String{
									Path: []string{"name"},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"id":1,"name":"Jens","pet":{"name":"Woofie"}}`
	}))
	t.Run("with unescape json enabled", func(t *testing.T) {
		t.Run("json object within a string", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
			return &Object{
				Fetch: &SingleFetch{
					BufferId: 0,
					// Datasource returns a JSON object within a string
					DataSource: FakeDataSource(`{"data":"{ \"hello\": \"world\", \"numberAsString\": \"1\", \"number\": 1, \"bool\": true, \"null\": null, \"array\": [1,2,3], \"object\": {\"key\": \"value\"} }"}`),
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
						HasBuffer: true,
					},
				},
				// expected output is a JSON object
			}, Context{ctx: context.Background()}, `{"data":{ "hello": "world", "numberAsString": "1", "number": 1, "bool": true, "null": null, "array": [1,2,3], "object": {"key": "value"} }}`
		}))
		t.Run("json array within a string", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
			return &Object{
				Fetch: &SingleFetch{
					BufferId: 0,
					// Datasource returns a JSON array within a string
					DataSource: FakeDataSource(`{"data": "[1, 2, 3]"}`),
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
						HasBuffer: true,
					},
				},
				// expected output is a JSON array
			}, Context{ctx: context.Background()}, `{"data":[1, 2, 3]}`
		}))
		t.Run("string with array and objects brackets", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
			return &Object{
				Fetch: &SingleFetch{
					BufferId: 0,
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
						HasBuffer: true,
					},
				},
				// expected output is a string
			}, Context{ctx: context.Background()}, `{"data":"hi[1beep{2}]"}`
		}))
		t.Run("plain scalar values within a string", func(t *testing.T) {
			t.Run("boolean", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						BufferId: 0,
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
							HasBuffer: true,
						},
					},
					// expected output is a string
				}, Context{ctx: context.Background()}, `{"data":"true"}`
			}))
			t.Run("int", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						BufferId: 0,
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
							HasBuffer: true,
						},
					},
					// expected output is a string
				}, Context{ctx: context.Background()}, `{"data":"1"}`
			}))
			t.Run("float", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						BufferId: 0,
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
							HasBuffer: true,
						},
					},
					// expected output is a string
				}, Context{ctx: context.Background()}, `{"data":"2.0"}`
			}))
			t.Run("null", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						BufferId: 0,
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
							HasBuffer: true,
						},
					},
					// expected output is a string
				}, Context{ctx: context.Background()}, `{"data":"null"}`
			}))
			t.Run("string", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						BufferId:   0,
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
							HasBuffer: true,
						},
					},
					// expect data value to be valid JSON string
				}, Context{ctx: context.Background()}, `{"data":"hello world"}`
			}))
		})
		t.Run("plain scalar values as is", func(t *testing.T) {
			t.Run("boolean", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						BufferId: 0,
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
							HasBuffer: true,
						},
					},
					// expected output is a JSON boolean
				}, Context{ctx: context.Background()}, `{"data":true}`
			}))
			t.Run("int", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						BufferId: 0,
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
							HasBuffer: true,
						},
					},
					// expected output is a JSON boolean
				}, Context{ctx: context.Background()}, `{"data":1}`
			}))
			t.Run("float", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						BufferId: 0,
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
							HasBuffer: true,
						},
					},
					// expected output is a JSON boolean
				}, Context{ctx: context.Background()}, `{"data":2.0}`
			}))
			t.Run("null", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
				return &Object{
					Fetch: &SingleFetch{
						BufferId:   0,
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
							HasBuffer: true,
						},
					},
					// expect data value to be valid JSON string
				}, Context{ctx: context.Background()}, `{"data":null}`
			}))
		})
	})

	t.Run("custom", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fetch: &SingleFetch{
				BufferId:   0,
				DataSource: FakeDataSource(`{"id": "1"}`),
			},
			Fields: []*Field{
				{
					BufferID:  0,
					HasBuffer: true,
					Name:      []byte("id"),
					Value: &CustomNode{
						CustomResolve: customResolver{},
						Path:          []string{"id"},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"id":1}`
	}))
	t.Run("custom nullable", testErrFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedErr string) {
		return &Object{
			Fetch: &SingleFetch{
				BufferId:   0,
				DataSource: FakeDataSource(`{"id": null}`),
			},
			Fields: []*Field{
				{
					BufferID:  0,
					HasBuffer: true,
					Name:      []byte("id"),
					Value: &CustomNode{
						CustomResolve: customErrResolve{},
						Path:          []string{"id"},
						Nullable:      false,
					},
				},
			},
		}, Context{ctx: context.Background()}, errNonNullableFieldValueIsNull.Error()
	}))
	t.Run("custom error", testErrFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedErr string) {
		return &Object{
			Fetch: &SingleFetch{
				BufferId:   0,
				DataSource: FakeDataSource(`{"id": "1"}`),
			},
			Fields: []*Field{
				{
					BufferID:  0,
					HasBuffer: true,
					Name:      []byte("id"),
					Value: &CustomNode{
						CustomResolve: customErrResolve{},
						Path:          []string{"id"},
					},
				},
			},
		}, Context{ctx: context.Background()}, `failed to resolve value type string for path /data/id via custom resolver`
	}))
}

func TestResolver_WithHooks(t *testing.T) {
	testFn := func(enableSingleFlight bool, enableDataLoader bool, fn func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string)) func(t *testing.T) {
		ctrl := gomock.NewController(t)
		rCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := newResolver(rCtx, enableSingleFlight, enableDataLoader)
		node, ctx, expectedOutput := fn(t, ctrl)
		return func(t *testing.T) {
			buf := &BufPair{
				Data:   fastbuffer.New(),
				Errors: fastbuffer.New(),
			}
			err := r.resolveNode(&ctx, node, nil, buf)
			assert.Equal(t, buf.Errors.String(), "", "want error buf to be empty")
			assert.NoError(t, err)
			assert.Equal(t, expectedOutput, buf.Data.String())
			ctrl.Finish()
		}
	}
	t.Run("resolve with hooks", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {

		pathEq := func(expected string) gomock.Matcher {
			return hookContextPathMatcher{path: expected}
		}

		beforeFetch := NewMockBeforeFetchHook(ctrl)
		beforeFetch.EXPECT().OnBeforeFetch(pathEq("/data/user"), []byte("fakeInput")).Return()
		afterFetch := NewMockAfterFetchHook(ctrl)
		afterFetch.EXPECT().OnData(pathEq("/data/user"), []byte(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`), false).Return()
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fields: []*Field{
							{
								Name: []byte("user"),
								Value: &Object{
									Fetch: &SingleFetch{
										BufferId:   0,
										DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
										InputTemplate: InputTemplate{
											Segments: []TemplateSegment{
												{
													SegmentType: StaticSegmentType,
													Data:        []byte("fakeInput"),
												},
											},
										},
									},
									Fields: []*Field{
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("id"),
											Value: &String{
												Path: []string{"id"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("registered"),
											Value: &Boolean{
												Path: []string{"registered"},
											},
										},
										{
											BufferID:  0,
											HasBuffer: true,
											Name:      []byte("pet"),
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
				},
			},
		}, Context{ctx: context.Background(), beforeFetchHook: beforeFetch, afterFetchHook: afterFetch}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}}}`
	}))

}

func TestResolver_ResolveGraphQLResponse(t *testing.T) {
	testFn := func(enableSingleFlight bool, enableDataLoader bool, fn func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string)) func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		rCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := newResolver(rCtx, enableSingleFlight, enableDataLoader)
		node, ctx, expectedOutput := fn(t, ctrl)
		return func(t *testing.T) {
			t.Helper()

			buf := &bytes.Buffer{}
			err := r.ResolveGraphQLResponse(&ctx, node, nil, buf)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutput, buf.String())
			ctrl.Finish()
		}
	}
	testFnWithError := func(enableSingleFlight bool, enableDataLoader bool, fn func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedErrorMessage string)) func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		rCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := newResolver(rCtx, enableSingleFlight, enableDataLoader)
		node, ctx, expectedOutput := fn(t, ctrl)
		return func(t *testing.T) {
			t.Helper()

			buf := &bytes.Buffer{}
			err := r.ResolveGraphQLResponse(&ctx, node, nil, buf)
			assert.Error(t, err, expectedOutput)
			ctrl.Finish()
		}
	}
	t.Run("empty graphql response", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: true,
			},
		}, Context{ctx: context.Background()}, `{"data":null}`
	}))
	t.Run("__typename without renaming", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
							Fetch: &SingleFetch{
								BufferId:   0,
								DataSource: FakeDataSource(`{"id":1,"name":"Jannik","__typename":"User","rewritten":"User"}`),
							},
							Fields: []*Field{
								{
									Name:      []byte("id"),
									HasBuffer: true,
									BufferID:  0,
									Value: &Integer{
										Path:     []string{"id"},
										Nullable: false,
									},
								},
								{
									Name:      []byte("name"),
									HasBuffer: true,
									BufferID:  0,
									Value: &String{
										Path:     []string{"name"},
										Nullable: false,
									},
								},
								{
									Name:      []byte("__typename"),
									HasBuffer: true,
									BufferID:  0,
									Value: &String{
										Path:       []string{"__typename"},
										Nullable:   false,
										IsTypeName: true,
									},
								},
								{
									Name:      []byte("aliased"),
									HasBuffer: true,
									BufferID:  0,
									Value: &String{
										Path:       []string{"__typename"},
										Nullable:   false,
										IsTypeName: true,
									},
								},
								{
									Name:      []byte("rewritten"),
									HasBuffer: true,
									BufferID:  0,
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
	t.Run("__typename with renaming", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
				Data: &Object{
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Fetch: &SingleFetch{
									BufferId:   0,
									DataSource: FakeDataSource(`{"id":1,"name":"Jannik","__typename":"User","rewritten":"User"}`),
								},
								Fields: []*Field{
									{
										Name:      []byte("id"),
										HasBuffer: true,
										BufferID:  0,
										Value: &Integer{
											Path:     []string{"id"},
											Nullable: false,
										},
									},
									{
										Name:      []byte("name"),
										HasBuffer: true,
										BufferID:  0,
										Value: &String{
											Path:     []string{"name"},
											Nullable: false,
										},
									},
									{
										Name:      []byte("__typename"),
										HasBuffer: true,
										BufferID:  0,
										Value: &String{
											Path:       []string{"__typename"},
											Nullable:   false,
											IsTypeName: true,
										},
									},
									{
										Name:      []byte("aliased"),
										HasBuffer: true,
										BufferID:  0,
										Value: &String{
											Path:       []string{"__typename"},
											Nullable:   false,
											IsTypeName: true,
										},
									},
									{
										Name:      []byte("rewritten"),
										HasBuffer: true,
										BufferID:  0,
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
	t.Run("empty graphql response for not nullable query field", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: false,
				Fields: []*Field{
					{
						BufferID:  0,
						HasBuffer: true,
						Name:      []byte("country"),
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
	t.Run("fetch with simple error", testFn(true, false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
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
					BufferId:   0,
					DataSource: mockDataSource,
					ProcessResponseConfig: ProcessResponseConfig{
						ExtractGraphqlResponse: true,
					},
				},
				Fields: []*Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("name"),
						Value: &String{
							Path:     []string{"name"},
							Nullable: true,
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"errorMessage"}],"data":{"name":null}}`
	}))
	t.Run("nested fetch error for non-nullable field", testFn(true, false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
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
					BufferId:   0,
					DataSource: FakeDataSource(`{"id":1}`),
				},
				Fields: []*Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("nestedObject"),
						Value: &Object{
							Nullable: false,
							Fetch: &SingleFetch{
								BufferId:   1,
								DataSource: mockDataSource,
								ProcessResponseConfig: ProcessResponseConfig{
									ExtractGraphqlResponse: true,
								},
							},
							Fields: []*Field{
								{
									HasBuffer: true,
									BufferID:  1,
									Name:      []byte("foo"),
									Value: &String{
										Path:     []string{"foo"},
										Nullable: false,
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"errorMessage"},{"message":"unable to resolve","locations":[{"line":0,"column":0}],"path":["nestedObject"]}],"data":null}`
	}))
	t.Run("fetch with two Errors", testFn(true, false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
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
					BufferId:   0,
					DataSource: mockDataSource,
					ProcessResponseConfig: ProcessResponseConfig{
						ExtractGraphqlResponse: true,
					},
				},
				Fields: []*Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("name"),
						Value: &String{
							Path:     []string{"name"},
							Nullable: true,
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"errorMessage1"},{"message":"errorMessage2"}],"data":{"name":null}}`
	}))
	t.Run("not nullable object in nullable field", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: false,
				Fetch: &SingleFetch{
					BufferId:   0,
					DataSource: FakeDataSource(`{"nullable_field": null}`),
				},
				Fields: []*Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("nullableField"),
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
	t.Run("null field should bubble up to parent with error", testFnWithError(false, false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: true,
				Fetch: &SingleFetch{
					BufferId:   0,
					DataSource: FakeDataSource(`[{"id":1},{"id":2},{"id":3}]`),
				},
				Fields: []*Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("stringObject"),
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
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("integerObject"),
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
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("floatObject"),
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
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("booleanObject"),
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
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("objectObject"),
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
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("arrayObject"),
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
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("asynchronousArrayObject"),
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
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("nullableArray"),
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
	t.Run("empty nullable array should resolve correctly", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: true,
				Fetch: &SingleFetch{
					BufferId:   0,
					DataSource: FakeDataSource(`[]`),
				},
				Fields: []*Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("nullableArray"),
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
	t.Run("empty not nullable array should resolve correctly", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: false,
				Fetch: &SingleFetch{
					BufferId:   0,
					DataSource: FakeDataSource(`{"some_path": []}`),
				},
				Fields: []*Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("notNullableArray"),
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
	t.Run("when data null not nullable array should resolve to data null and errors", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: false,
				Fetch: &SingleFetch{
					BufferId:              0,
					DataSource:            FakeDataSource(`{"data":null}`),
					ProcessResponseConfig: ProcessResponseConfig{ExtractGraphqlResponse: true},
				},
				Fields: []*Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("nonNullArray"),
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
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("nullableArray"),
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
	t.Run("when data null and errors present not nullable array should result to null data upsteam error and resolve error", testFn(false, false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: false,
				Fetch: &SingleFetch{
					BufferId: 0,
					DataSource: FakeDataSource(
						`{"errors":[{"message":"Could not get a name","locations":[{"line":3,"column":5}],"path":["todos",0,"name"]}],"data":null}`),
					ProcessResponseConfig: ProcessResponseConfig{ExtractGraphqlResponse: true},
				},
				Fields: []*Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("todos"),
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
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"Could not get a name","locations":[{"line":3,"column":5}],"path":["todos",0,"name"]},{"message":"unable to resolve","locations":[{"line":0,"column":0}]}],"data":null}`
	}))
	t.Run("complex GraphQL Server plan", testFn(true, false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
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
							BufferId: 0,
							Input:    `{"url":"https://service.one","body":{"query":"query($firstArg: String, $thirdArg: Int){serviceOne(serviceOneArg: $firstArg){fieldOne} anotherServiceOne(anotherServiceOneArg: $thirdArg){fieldOne} reusingServiceOne(reusingServiceOneArg: $firstArg){fieldOne}}","variables":{"thirdArg":$$1$$,"firstArg":$$0$$}}}`,
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
							ProcessResponseConfig: ProcessResponseConfig{
								ExtractGraphqlResponse: true,
							},
						},
						&SingleFetch{
							BufferId: 1,
							Input:    `{"url":"https://service.two","body":{"query":"query($secondArg: Boolean, $fourthArg: Float){serviceTwo(serviceTwoArg: $secondArg){fieldTwo} secondServiceTwo(secondServiceTwoArg: $fourthArg){fieldTwo}}","variables":{"fourthArg":$$1$$,"secondArg":$$0$$}}}`,
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
							ProcessResponseConfig: ProcessResponseConfig{
								ExtractGraphqlResponse: true,
							},
						},
					},
				},
				Fields: []*Field{
					{
						BufferID:  0,
						HasBuffer: true,
						Name:      []byte("serviceOne"),
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
						BufferID:  1,
						HasBuffer: true,
						Name:      []byte("serviceTwo"),
						Value: &Object{
							Path: []string{"serviceTwo"},
							Fetch: &SingleFetch{
								BufferId: 2,
								Input:    `{"url":"https://service.one","body":{"query":"{serviceOne {fieldOne}}"}}`,
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
								ProcessResponseConfig: ProcessResponseConfig{
									ExtractGraphqlResponse: true,
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
									BufferID:  2,
									HasBuffer: true,
									Name:      []byte("serviceOneResponse"),
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
						BufferID:  0,
						HasBuffer: true,
						Name:      []byte("anotherServiceOne"),
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
						BufferID:  1,
						HasBuffer: true,
						Name:      []byte("secondServiceTwo"),
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
						BufferID:  0,
						HasBuffer: true,
						Name:      []byte("reusingServiceOne"),
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
	t.Run("federation", testFn(true, false, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

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
				pair.Data.WriteString(`{"reviews": [{"body": "A highly effective form of birth control.","product": {"upc": "top-1","__typename": "Product"}},{"body": "Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product": {"upc": "top-1","__typename": "Product"}}]}`)
				return writeGraphqlResponse(pair, w, false)
			})

		productServiceCallCount := 0

		productService := NewMockDataSource(ctrl)
		productService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				productServiceCallCount++
				switch productServiceCallCount {
				case 1:
					expected := `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"name": "Trilby"}`)
					return writeGraphqlResponse(pair, w, false)
				case 2:
					expected := `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"name": "Trilby"}`)
					return writeGraphqlResponse(pair, w, false)
				}
				return
			}).
			Return(nil).Times(2)

		return &GraphQLResponse{
			Data: &Object{
				Fetch: &SingleFetch{
					BufferId: 0,
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					DataSource: userService,
					ProcessResponseConfig: ProcessResponseConfig{
						ExtractGraphqlResponse: true,
					},
				},
				Fields: []*Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("me"),
						Value: &Object{
							Fetch: &SingleFetch{
								BufferId: 1,
								InputTemplate: InputTemplate{
									Segments: []TemplateSegment{
										{
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":`),
											SegmentType: StaticSegmentType,
										},
										{
											SegmentType:        VariableSegmentType,
											VariableKind:       ObjectVariableKind,
											VariableSourcePath: []string{"id"},
											Renderer:           NewJSONVariableRendererWithValidation(`{"type":"string"}`),
										},
										{
											Data:        []byte(`,"__typename":"User"}]}}}`),
											SegmentType: StaticSegmentType,
										},
									},
								},
								DataSource: reviewsService,
								ProcessResponseConfig: ProcessResponseConfig{
									ExtractGraphqlResponse: true,
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

									HasBuffer: true,
									BufferID:  1,
									Name:      []byte("reviews"),
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
															BufferId:   2,
															DataSource: productService,
															InputTemplate: InputTemplate{
																Segments: []TemplateSegment{
																	{
																		Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":`),
																		SegmentType: StaticSegmentType,
																	},
																	{
																		SegmentType:        VariableSegmentType,
																		VariableKind:       ObjectVariableKind,
																		VariableSourcePath: []string{"upc"},
																		Renderer:           NewJSONVariableRendererWithValidation(`{"type":"string"}`),
																	},
																	{
																		Data:        []byte(`,"__typename":"Product"}]}}}`),
																		SegmentType: StaticSegmentType,
																	},
																},
															},
															ProcessResponseConfig: ProcessResponseConfig{
																ExtractGraphqlResponse: true,
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
																HasBuffer: true,
																BufferID:  2,
																Name:      []byte("name"),
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
		}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-1","name":"Trilby"}}]}}}`
	}))
	t.Run("federation with enabled dataloader", testFn(true, true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
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

		reviewBatchFactory := NewMockDataSourceBatchFactory(ctrl)
		reviewBatchFactory.EXPECT().
			CreateBatch([][]byte{
				[]byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"1234","__typename":"User"}]}}}`),
			}).
			Return(NewFakeDataSourceBatch(
				`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"1234","__typename":"User"}]}}}`,
				[]resultedBufPair{
					{data: `{"reviews": [{"body": "A highly effective form of birth control.","product": {"upc": "top-1","__typename": "Product"}},{"body": "Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product": {"upc": "top-2","__typename": "Product"}}]}`},
				}), nil)
		reviewsService := NewMockDataSource(ctrl)
		reviewsService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"1234","__typename":"User"}]}}}`
				assert.Equal(t, expected, actual)
				pair := NewBufPair()
				pair.Data.WriteString(`{"reviews": [{"body": "A highly effective form of birth control.","product": {"upc": "top-1","__typename": "Product"}},{"body": "Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product": {"upc": "top-2","__typename": "Product"}}]}`)
				return writeGraphqlResponse(pair, w, false)
			})

		productBatchFactory := NewMockDataSourceBatchFactory(ctrl)
		productBatchFactory.EXPECT().
			CreateBatch(
				[][]byte{
					[]byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}}}`),
					[]byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}}}`),
				},
			).Return(NewFakeDataSourceBatch(
			`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"},{"upc":"top-2","__typename":"Product"}]}}}`,
			[]resultedBufPair{
				{data: `{"name": "Trilby"}`},
				{data: `{"name": "Fedora"}`},
			}), nil)
		productService := NewMockDataSource(ctrl)
		productService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				expected := `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"},{"upc":"top-2","__typename":"Product"}]}}}`
				assert.Equal(t, expected, actual)
				pair := NewBufPair()
				pair.Data.WriteString(`[{"name": "Trilby"},{"name": "Fedora"}]`)
				return writeGraphqlResponse(pair, w, false)
			})

		return &GraphQLResponse{
			Data: &Object{
				Fetch: &SingleFetch{
					BufferId: 0,
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					DataSource: userService,
					ProcessResponseConfig: ProcessResponseConfig{
						ExtractGraphqlResponse: true,
					},
				},
				Fields: []*Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("me"),
						Value: &Object{
							Fetch: &BatchFetch{
								Fetch: &SingleFetch{
									BufferId: 1,
									InputTemplate: InputTemplate{
										Segments: []TemplateSegment{
											{
												Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":`),
												SegmentType: StaticSegmentType,
											},
											{
												SegmentType:        VariableSegmentType,
												VariableKind:       ObjectVariableKind,
												VariableSourcePath: []string{"id"},
												Renderer:           NewJSONVariableRendererWithValidation(`{"type":"string"}`),
											},
											{
												Data:        []byte(`,"__typename":"User"}]}}}`),
												SegmentType: StaticSegmentType,
											},
										},
									},
									DataSource: reviewsService,
									ProcessResponseConfig: ProcessResponseConfig{
										ExtractGraphqlResponse: true,
									},
								},
								BatchFactory: reviewBatchFactory,
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

									HasBuffer: true,
									BufferID:  1,
									Name:      []byte("reviews"),
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
														Fetch: &BatchFetch{
															Fetch: &SingleFetch{
																BufferId:   2,
																DataSource: productService,
																InputTemplate: InputTemplate{
																	Segments: []TemplateSegment{
																		{
																			Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":`),
																			SegmentType: StaticSegmentType,
																		},
																		{
																			SegmentType:        VariableSegmentType,
																			VariableKind:       ObjectVariableKind,
																			VariableSourcePath: []string{"upc"},
																			Renderer:           NewJSONVariableRendererWithValidation(`{"type":"string"}`),
																		},
																		{
																			Data:        []byte(`,"__typename":"Product"}]}}}`),
																			SegmentType: StaticSegmentType,
																		},
																	},
																},
																ProcessResponseConfig: ProcessResponseConfig{
																	ExtractGraphqlResponse: true,
																},
															},
															BatchFactory: productBatchFactory,
														},
														Fields: []*Field{
															{
																Name: []byte("upc"),
																Value: &String{
																	Path: []string{"upc"},
																},
															},
															{
																HasBuffer: true,
																BufferID:  2,
																Name:      []byte("name"),
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
	t.Run("federation with null response", testFn(true, true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
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

		reviewBatchFactory := NewMockDataSourceBatchFactory(ctrl)
		reviewBatchFactory.EXPECT().
			CreateBatch([][]byte{
				[]byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"1234","__typename":"User"}]}}}`),
			}).
			Return(NewFakeDataSourceBatch(
				`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"1234","__typename":"User"}]}}}`,
				[]resultedBufPair{
					{data: `{"reviews":[{"body":"foo","product":{"upc":"top-1","__typename":"Product"}},{"body":"bar","product":{"upc":"top-2","__typename":"Product"}},{"body":"baz","product":null},{"body":"bat","product":{"upc":"top-4","__typename":"Product"}},{"body":"bal","product":{"upc":"top-5","__typename":"Product"}},{"body":"ban","product":{"upc":"top-6","__typename":"Product"}}]}`},
				}), nil)
		reviewsService := NewMockDataSource(ctrl)
		reviewsService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"1234","__typename":"User"}]}}}`
				assert.Equal(t, expected, actual)
				pair := NewBufPair()
				pair.Data.WriteString(`{"reviews": [
						{"body": "foo","product": {"upc": "top-1","__typename": "Product"}},
						{"body": "bar","product": {"upc": "top-2","__typename": "Product"}},
						{"body": "baz","product": null},
						{"body": "bat","product": {"upc": "top-4","__typename": "Product"}},
						{"body": "bal","product": {"upc": "top-5","__typename": "Product"}},
						{"body": "ban","product": {"upc": "top-6","__typename": "Product"}}
]}`)
				return writeGraphqlResponse(pair, w, false)
			})

		productBatchFactory := NewMockDataSourceBatchFactory(ctrl)
		productBatchFactory.EXPECT().
			CreateBatch(
				[][]byte{
					[]byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}}}`),
					[]byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}}}`),
					[]byte("null"),
					[]byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-4","__typename":"Product"}]}}}`),
					[]byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-5","__typename":"Product"}]}}}`),
					[]byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-6","__typename":"Product"}]}}}`),
				},
			).Return(NewFakeDataSourceBatch(
			`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"},{"upc":"top-2","__typename":"Product"},{"upc":"top-4","__typename":"Product"},{"upc":"top-5","__typename":"Product"},{"upc":"top-6","__typename":"Product"}]}}}`,
			[]resultedBufPair{
				{data: `{"name": "Trilby"}`},
				{data: `{"name": "Fedora"}`},
				{data: `null`},
				{data: `{"name": "Boater"}`},
				{data: `{"name": "Top Hat"}`},
				{data: `{"name": "Bowler"}`},
			}), nil)
		productService := NewMockDataSource(ctrl)
		productService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				expected := `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"},{"upc":"top-2","__typename":"Product"},{"upc":"top-4","__typename":"Product"},{"upc":"top-5","__typename":"Product"},{"upc":"top-6","__typename":"Product"}]}}}`
				assert.Equal(t, expected, actual)
				pair := NewBufPair()
				pair.Data.WriteString(`[{"name":"Trilby"},{"name":"Fedora"},{"name":"Boater"},{"name":"Top Hat"},{"name":"Bowler"}]`)
				return writeGraphqlResponse(pair, w, false)
			})

		return &GraphQLResponse{
			Data: &Object{
				Fetch: &SingleFetch{
					BufferId: 0,
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					DataSource: userService,
					ProcessResponseConfig: ProcessResponseConfig{
						ExtractGraphqlResponse: true,
					},
				},
				Fields: []*Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("me"),
						Value: &Object{
							Fetch: &BatchFetch{
								Fetch: &SingleFetch{
									BufferId: 1,
									InputTemplate: InputTemplate{
										Segments: []TemplateSegment{
											{
												Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":`),
												SegmentType: StaticSegmentType,
											},
											{
												SegmentType:        VariableSegmentType,
												VariableKind:       ObjectVariableKind,
												VariableSourcePath: []string{"id"},
												Renderer:           NewJSONVariableRendererWithValidation(`{"type":"string"}`),
											},
											{
												Data:        []byte(`,"__typename":"User"}]}}}`),
												SegmentType: StaticSegmentType,
											},
										},
										SetTemplateOutputToNullOnVariableNull: true,
									},
									DataSource: reviewsService,
									ProcessResponseConfig: ProcessResponseConfig{
										ExtractGraphqlResponse: true,
									},
								},
								BatchFactory: reviewBatchFactory,
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

									HasBuffer: true,
									BufferID:  1,
									Name:      []byte("reviews"),
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
															Fetch: &SingleFetch{
																BufferId:   2,
																DataSource: productService,
																InputTemplate: InputTemplate{
																	Segments: []TemplateSegment{
																		{
																			Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":`),
																			SegmentType: StaticSegmentType,
																		},
																		{
																			SegmentType:        VariableSegmentType,
																			VariableKind:       ObjectVariableKind,
																			VariableSourcePath: []string{"upc"},
																			Renderer:           NewJSONVariableRendererWithValidation(`{"type":"string"}`),
																		},
																		{
																			Data:        []byte(`,"__typename":"Product"}]}}}`),
																			SegmentType: StaticSegmentType,
																		},
																	},
																	SetTemplateOutputToNullOnVariableNull: true,
																},
																ProcessResponseConfig: ProcessResponseConfig{
																	ExtractGraphqlResponse: true,
																},
															},
															BatchFactory: productBatchFactory,
														},
														Fields: []*Field{
															{
																Name: []byte("upc"),
																Value: &String{
																	Path: []string{"upc"},
																},
															},
															{
																HasBuffer: true,
																BufferID:  2,
																Name:      []byte("name"),
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
			}, // ...															 `{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"foo","product":{"upc":"top-1","name":"Trilby"}},{"body":"bar","product":{"upc":"top-2","name":"Fedora"}},{"body":"baz","product":null},{"body":"bat","product":null},{"body":"bal","product":{"upc":"top-5","name":"Boater"}},{"body":"ban","product":{"upc":"top-6","name":"Top Hat"}}]}}}
		}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"foo","product":{"upc":"top-1","name":"Trilby"}},{"body":"bar","product":{"upc":"top-2","name":"Fedora"}},{"body":"baz","product":null},{"body":"bat","product":{"upc":"top-4","name":"Boater"}},{"body":"bal","product":{"upc":"top-5","name":"Top Hat"}},{"body":"ban","product":{"upc":"top-6","name":"Bowler"}}]}}}`
	}))
	t.Run("federation with enabled dataloader and fetch error ", testFn(true, true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

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

		reviewBatchFactory := NewMockDataSourceBatchFactory(ctrl)
		reviewBatchFactory.EXPECT().
			CreateBatch([][]byte{
				[]byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"1234","__typename":"User"}]}}}`),
			}).
			Return(NewFakeDataSourceBatch(
				`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"1234","__typename":"User"}]}}}`,
				[]resultedBufPair{
					{data: `{"reviews": [{"body": "A highly effective form of birth control.","product": {"upc": "top-1","__typename": "Product"}},{"body": "Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product": {"upc": "top-2","__typename": "Product"}}]}`},
				}), nil)
		reviewsService := NewMockDataSource(ctrl)
		reviewsService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"1234","__typename":"User"}]}}}`
				assert.Equal(t, expected, actual)
				pair := NewBufPair()
				pair.Data.WriteString(`{"reviews": [{"body": "A highly effective form of birth control.","product": {"upc": "top-1","__typename": "Product"}},{"body": "Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product": {"upc": "top-2","__typename": "Product"}}]}`)
				return writeGraphqlResponse(pair, w, false)
			})

		productBatchFactory := NewMockDataSourceBatchFactory(ctrl)
		productBatchFactory.EXPECT().
			CreateBatch(
				[][]byte{
					[]byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}}}`),
					[]byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}}}`),
				},
			).Return(NewFakeDataSourceBatch(
			`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"},{"upc":"top-2","__typename":"Product"}]}}}`,
			[]resultedBufPair{{data: `null`, err: "errorMessage"}, {data: `null`}}), nil)
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
					BufferId: 0,
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					DataSource: userService,
					ProcessResponseConfig: ProcessResponseConfig{
						ExtractGraphqlResponse: true,
					},
				},
				Fields: []*Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("me"),
						Value: &Object{
							Fetch: &BatchFetch{
								Fetch: &SingleFetch{
									BufferId: 1,
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
								},
								BatchFactory: reviewBatchFactory,
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

									HasBuffer: true,
									BufferID:  1,
									Name:      []byte("reviews"),
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
														Fetch: &BatchFetch{
															Fetch: &SingleFetch{
																BufferId:   2,
																DataSource: productService,
																InputTemplate: InputTemplate{
																	Segments: []TemplateSegment{
																		{
																			Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"`),
																			SegmentType: StaticSegmentType,
																		},
																		{
																			SegmentType:        VariableSegmentType,
																			VariableKind:       ObjectVariableKind,
																			VariableSourcePath: []string{"upc"},
																			Renderer:           NewPlainVariableRendererWithValidation(`{"type":"string"}`),
																		},
																		{
																			Data:        []byte(`","__typename":"Product"}]}}}`),
																			SegmentType: StaticSegmentType,
																		},
																	},
																},
																ProcessResponseConfig: ProcessResponseConfig{
																	ExtractGraphqlResponse: true,
																},
															},
															BatchFactory: productBatchFactory,
														},
														Fields: []*Field{
															{
																Name: []byte("upc"),
																Value: &String{
																	Path: []string{"upc"},
																},
															},
															{
																HasBuffer: true,
																BufferID:  2,
																Name:      []byte("name"),
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
		}, Context{ctx: context.Background(), Variables: nil}, `{"errors":[{"message":"errorMessage"},{"message":"unable to resolve","locations":[{"line":0,"column":0}],"path":["me","reviews","0","product"]},{"message":"unable to resolve","locations":[{"line":0,"column":0}],"path":["me","reviews","1","product"]}],"data":{"me":{"id":"1234","username":"Me","reviews":[null,null]}}}`
	}))
	t.Run("federation with optional variable", testFn(true, true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
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

		employeeBatchFactory := NewMockDataSourceBatchFactory(ctrl)
		employeeBatchFactory.EXPECT().
			CreateBatch([][]byte{
				[]byte(`{"method":"POST","url":"http://localhost:8081/query","body":{"query":"query($representations: [_Any!]!, $companyId: ID!){_entities(representations: $representations){... on User {employment(companyId: $companyId){id}}}}","variables":{"companyId":"abc123","representations":[{"id":"1234","__typename":"User"}]}}}`),
			}).
			Return(NewFakeDataSourceBatch(
				`{"method":"POST","url":"http://localhost:8081/query","body":{"query":"query($representations: [_Any!]!, $companyId: ID!){_entities(representations: $representations){... on User {employment(companyId: $companyId){id}}}}","variables":{"companyId":"abc123","representations":[{"id":"1234","__typename":"User"}]}}}`,
				[]resultedBufPair{
					{data: `{"employment": {"id": "xyz987"}}`},
				}), nil)
		employeeService := NewMockDataSource(ctrl)
		employeeService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				expected := `{"method":"POST","url":"http://localhost:8081/query","body":{"query":"query($representations: [_Any!]!, $companyId: ID!){_entities(representations: $representations){... on User {employment(companyId: $companyId){id}}}}","variables":{"companyId":"abc123","representations":[{"id":"1234","__typename":"User"}]}}}`
				assert.Equal(t, expected, actual)
				pair := NewBufPair()
				pair.Data.WriteString(`{"employment": {"id": "xyz987"}}`)
				return writeGraphqlResponse(pair, w, false)
			})

		timeBatchFactory := NewMockDataSourceBatchFactory(ctrl)
		timeBatchFactory.EXPECT().
			CreateBatch(
				[][]byte{
					[]byte(`{"method":"POST","url":"http://localhost:8082/query","body":{"query":"query($representations: [_Any!]!, $date: LocalTime){_entities(representations: $representations){... on Employee {times(date: $date){id employee {id} start end}}}}","variables":{"date":null,"representations":[{"id":"xyz987","__typename":"Employee"}]}}}`),
				},
			).Return(NewFakeDataSourceBatch(
			`{"method":"POST","url":"http://localhost:8082/query","body":{"query":"query($representations: [_Any!]!, $date: LocalTime){_entities(representations: $representations){... on Employee {times(date: $date){id employee {id} start end}}}}","variables":{"date":null,"representations":[{"id":"xyz987","__typename":"Employee"}]}}}`,
			[]resultedBufPair{
				{data: `{"times":[{"id": "t1","employee":{"id":"xyz987"},"start":"2022-11-02T08:00:00","end":"2022-11-02T12:00:00"}]}`},
			}), nil)
		timeService := NewMockDataSource(ctrl)
		timeService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				expected := `{"method":"POST","url":"http://localhost:8082/query","body":{"query":"query($representations: [_Any!]!, $date: LocalTime){_entities(representations: $representations){... on Employee {times(date: $date){id employee {id} start end}}}}","variables":{"date":null,"representations":[{"id":"xyz987","__typename":"Employee"}]}}}`
				assert.Equal(t, expected, actual)
				pair := NewBufPair()
				pair.Data.WriteString(`{"times":[{"id": "t1","employee":{"id":"xyz987"},"start":"2022-11-02T08:00:00","end":"2022-11-02T12:00:00"}]}`)
				return writeGraphqlResponse(pair, w, false)
			})

		return &GraphQLResponse{
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
													Stream: Stream{
														Enabled: false,
													},
												},
												HasBuffer: true,
												BufferID:  2,
											},
										},
										Fetch: &BatchFetch{
											Fetch: &SingleFetch{
												BufferId: 2,
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
												ProcessResponseConfig: ProcessResponseConfig{
													ExtractGraphqlResponse:    true,
													ExtractFederationEntities: true,
												},
											},
											BatchFactory: timeBatchFactory,
										},
									},
									HasBuffer: true,
									BufferID:  1,
								},
							},
							Fetch: &BatchFetch{
								Fetch: &SingleFetch{
									BufferId: 1,
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
									ProcessResponseConfig: ProcessResponseConfig{
										ExtractGraphqlResponse:    true,
										ExtractFederationEntities: true,
									},
								},
								BatchFactory: employeeBatchFactory,
							},
						},
						HasBuffer: true,
						BufferID:  0,
					},
				},
				Fetch: &SingleFetch{
					BufferId: 0,
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://localhost:8080/query","body":{"query":"{me {id}}"}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					DataSource: userService,
					ProcessResponseConfig: ProcessResponseConfig{
						ExtractGraphqlResponse: true,
					},
				},
			},
		}, Context{ctx: context.Background(), Variables: []byte(`{"companyId":"abc123","date":null}`)}, `{"data":{"me":{"employment":{"id":"xyz987","times":[{"id":"t1","employee":{"id":"xyz987"},"start":"2022-11-02T08:00:00","end":"2022-11-02T12:00:00"}]}}}}`
	}))
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
			resolver := newResolver(rCtx, false, false)

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
						BufferId:   0,
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
							HasBuffer: true,
							BufferID:  0,
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
				Source:                stream,
				ProcessResponseConfig: ProcessResponseConfig{ExtractGraphqlResponse: true},
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

		return newResolver(ctx, false, false), plan, out
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

func BenchmarkResolver_ResolveNode(b *testing.B) {
	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resolver := newResolver(rCtx, true, false)

	serviceOneDS := FakeDataSource(`{"serviceOne":{"fieldOne":"fieldOneValue"},"anotherServiceOne":{"fieldOne":"anotherFieldOneValue"},"reusingServiceOne":{"fieldOne":"reUsingFieldOneValue"}}`)
	serviceTwoDS := FakeDataSource(`{"serviceTwo":{"fieldTwo":"fieldTwoValue"},"secondServiceTwo":{"fieldTwo":"secondFieldTwoValue"}}`)
	nestedServiceOneDS := FakeDataSource(`{"serviceOne":{"fieldOne":"fieldOneValue"}}`)

	plan := &GraphQLResponse{
		Data: &Object{
			Fetch: &ParallelFetch{
				Fetches: []Fetch{
					&SingleFetch{
						BufferId: 0,
						Input:    `{"url":"https://service.one","body":{"query":"query($firstArg: String, $thirdArg: Int){serviceOne(serviceOneArg: $firstArg){fieldOne} anotherServiceOne(anotherServiceOneArg: $thirdArg){fieldOne} reusingServiceOne(reusingServiceOneArg: $firstArg){fieldOne}}","variables":{"thirdArg":$$1$$,"firstArg":$$0$$}}}`,
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
								},
								{
									SegmentType: StaticSegmentType,
									Data:        []byte(`,"firstArg":`),
								},
								{
									SegmentType:        VariableSegmentType,
									VariableKind:       ContextVariableKind,
									VariableSourcePath: []string{"firstArg"},
								},
								{
									SegmentType: StaticSegmentType,
									Data:        []byte(`}}}`),
								},
							},
						},
						DataSource: serviceOneDS,
						Variables: NewVariables(
							&ContextVariable{
								Path: []string{"firstArg"},
							},
							&ContextVariable{
								Path: []string{"thirdArg"},
							},
						),
					},
					&SingleFetch{
						BufferId: 1,
						Input:    `{"url":"https://service.two","body":{"query":"query($secondArg: Boolean, $fourthArg: Float){serviceTwo(serviceTwoArg: $secondArg){fieldTwo} secondServiceTwo(secondServiceTwoArg: $fourthArg){fieldTwo}}","variables":{"fourthArg":$$1$$,"secondArg":$$0$$}}}`,
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
								},
								{
									SegmentType: StaticSegmentType,
									Data:        []byte(`,"secondArg":`),
								},
								{
									SegmentType:        VariableSegmentType,
									VariableKind:       ContextVariableKind,
									VariableSourcePath: []string{"secondArg"},
								},
								{
									SegmentType: StaticSegmentType,
									Data:        []byte(`}}}`),
								},
							},
						},
						DataSource: serviceTwoDS,
						Variables: NewVariables(
							&ContextVariable{
								Path: []string{"secondArg"},
							},
							&ContextVariable{
								Path: []string{"fourthArg"},
							},
						),
					},
				},
			},
			Fields: []*Field{
				{
					BufferID:  0,
					HasBuffer: true,
					Name:      []byte("serviceOne"),
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
					BufferID:  1,
					HasBuffer: true,
					Name:      []byte("serviceTwo"),
					Value: &Object{
						Path: []string{"serviceTwo"},
						Fetch: &SingleFetch{
							BufferId: 2,
							Input:    `{"url":"https://service.one","body":{"query":"{serviceOne {fieldOne}}"}}`,
							InputTemplate: InputTemplate{
								Segments: []TemplateSegment{
									{
										SegmentType: StaticSegmentType,
										Data:        []byte(`{"url":"https://service.one","body":{"query":"{serviceOne {fieldOne}}"}}`),
									},
								},
							},
							DataSource: nestedServiceOneDS,
							Variables:  Variables{},
						},
						Fields: []*Field{
							{
								Name: []byte("fieldTwo"),
								Value: &String{
									Path: []string{"fieldTwo"},
								},
							},
							{
								BufferID:  2,
								HasBuffer: true,
								Name:      []byte("serviceOneResponse"),
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
					BufferID:  0,
					HasBuffer: true,
					Name:      []byte("anotherServiceOne"),
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
					BufferID:  1,
					HasBuffer: true,
					Name:      []byte("secondServiceTwo"),
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
					BufferID:  0,
					HasBuffer: true,
					Name:      []byte("reusingServiceOne"),
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
	}

	var err error
	expected := []byte(`{"data":{"serviceOne":{"fieldOne":"fieldOneValue"},"serviceTwo":{"fieldTwo":"fieldTwoValue","serviceOneResponse":{"fieldOne":"fieldOneValue"}},"anotherServiceOne":{"fieldOne":"anotherFieldOneValue"},"secondServiceTwo":{"fieldTwo":"secondFieldTwoValue"},"reusingServiceOne":{"fieldOne":"reUsingFieldOneValue"}}}`)

	pool := sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 1024))
		},
	}

	variables := []byte(`{"firstArg":"firstArgValue","thirdArg":123,"secondArg": true, "fourthArg": 12.34}`)

	ctxPool := sync.Pool{
		New: func() interface{} {
			return NewContext(context.Background())
		},
	}

	runBench := func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(expected)))
		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				// _ = resolver.ResolveGraphQLResponse(ctx, plan, nil, ioutil.Discard)
				ctx := ctxPool.Get().(*Context)
				ctx.Variables = variables
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

	b.Run("singleflight enabled (latency 0)", func(b *testing.B) {
		serviceOneDS.artificialLatency = 0
		serviceTwoDS.artificialLatency = 0
		nestedServiceOneDS.artificialLatency = 0
		runBench(b)
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

type initTestVariableRenderer func(jsonSchema string) VariableRenderer

func useTestPlainVariableRenderer() initTestVariableRenderer {
	return func(jsonSchema string) VariableRenderer {
		return NewPlainVariableRendererWithValidation(jsonSchema)
	}
}

func useTestJSONVariableRenderer() initTestVariableRenderer {
	return func(jsonSchema string) VariableRenderer {
		return NewJSONVariableRendererWithValidation(jsonSchema)
	}
}
