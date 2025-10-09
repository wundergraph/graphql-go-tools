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

	"github.com/wundergraph/astjson"

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

func (f *_fakeDataSource) LoadWithFiles(ctx context.Context, input []byte, files []*httpclient.FileUpload, out *bytes.Buffer) (err error) {
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
	_, err = fmt.Fprintf(w, `{"errors":[{"message":"%s"}],"data":null}`, err.Error())
	if err != nil {
		panic(err)
	}
	err = w.(*SubscriptionRecorder).Flush()
	if err != nil {
		panic(err)
	}
}

var subscriptionHeartbeatInterval = 100 * time.Millisecond

func newResolver(ctx context.Context) *Resolver {
	return New(ctx, ResolverOptions{
		MaxConcurrency:                1024,
		Debug:                         false,
		PropagateSubgraphErrors:       true,
		PropagateSubgraphStatusCodes:  true,
		AsyncErrorWriter:              &TestErrorWriter{},
		SubscriptionHeartbeatInterval: subscriptionHeartbeatInterval,
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
		}, Context{ctx: context.Background(), Variables: astjson.MustParseBytes([]byte(`{"id":1}`))}, `{"data":{"name":"Jens"}}`
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
			SubgraphErrorPropagationMode: SubgraphErrorPropagationModePassThrough,
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

func testFnSubgraphErrorsWithAllowAllExtensionFields(fn func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string)) func(t *testing.T) {
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
			AllowAllErrorExtensionFields: true,
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
		}, Context{ctx: context.Background(), Variables: astjson.MustParseBytes([]byte(`{"firstArg":"firstArgValue","thirdArg":123,"secondArg": true, "fourthArg": 12.34}`))}, `{"data":{"serviceOne":{"fieldOne":"fieldOneValue"},"serviceTwo":{"fieldTwo":"fieldTwoValue","serviceOneResponse":{"fieldOne":"fieldOneValue"}},"anotherServiceOne":{"fieldOne":"anotherFieldOneValue"},"secondServiceTwo":{"fieldTwo":"secondFieldTwoValue"},"reusingServiceOne":{"fieldOne":"reUsingFieldOneValue"}}}`
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
					//           {"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":["id":"1234","__typename":"User"]}}}
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
															Nullable: true,
															Path:     []string{"product"},
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
			}, Context{ctx: context.Background(), Variables: astjson.MustParseBytes([]byte(`{"companyId":"abc123","date":null}`))}, `{"data":{"me":{"employment":{"id":"xyz987","times":[{"id":"t1","employee":{"id":"xyz987"},"start":"2022-11-02T08:00:00","end":"2022-11-02T12:00:00"}]}}}}`
		}))
	})
}

func TestResolver_ApolloCompatibilityMode_FetchError(t *testing.T) {
	options := apolloCompatibilityOptions{
		valueCompletion:     true,
		suppressFetchErrors: true,
	}
	t.Run("simple fetch with fetch error suppression - empty response", testFnApolloCompatibility(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
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
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
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

	t.Run("simple fetch with fetch error suppression - response with error", testFnApolloCompatibility(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				_, _ = w.Write([]byte(`{"errors":[{"message":"Cannot query field 'name' on type 'Query'"}]}`))
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
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
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
		}, Context{ctx: context.Background()}, `{"errors":[{"message":"Cannot query field 'name' on type 'Query'"}],"data":null}`
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
							SelectResponseDataPath:   []string{"data", "_entities"},
							SelectResponseErrorsPath: []string{"errors"},
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
		}, Context{ctx: context.Background(), Variables: nil}, `{"errors":[{"message":"errorMessage"}],"data":{"me":{"id":"1234","username":"Me","reviews":null}}}`
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

func TestResolver_WithVariableRemapping(t *testing.T) {
	cases := []struct {
		name, variable string
		remap          map[string]string
		variables      *astjson.Value
		expectedOutput string
	}{
		{"a to foo", "a", map[string]string{"a": "foo"}, astjson.MustParseBytes([]byte(`{"foo":"Wunderbar"}`)), `Wunderbar`},
		{"a to a", "a", map[string]string{"a": "a"}, astjson.MustParseBytes([]byte(`{"a":"WunderWunderbar"}`)), `WunderWunderbar`},
		{"no mapping", "foo", map[string]string{}, astjson.MustParseBytes([]byte(`{"foo":"BarDeWunder"}`)), `BarDeWunder`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			resolver := newResolver(rCtx)

			ctx := &Context{
				ctx:            context.Background(),
				Variables:      tc.variables,
				RemapVariables: tc.remap,
			}

			ctrl := gomock.NewController(t)
			fakeService := NewMockDataSource(ctrl)
			fakeService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					assert.Equal(t, tc.expectedOutput, actual)
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
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{tc.variable},
								Renderer:           NewPlainVariableRenderer(),
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
	closed   atomic.Bool
	mux      sync.Mutex
	onFlush  func(p []byte)
}

var _ SubscriptionResponseWriter = (*SubscriptionRecorder)(nil)

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

func (s *SubscriptionRecorder) AwaitClosed(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if s.closed.Load() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for close")
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

func (s *SubscriptionRecorder) Heartbeat() error {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.messages = append(s.messages, "heartbeat")
	return nil
}

func (s *SubscriptionRecorder) Close(_ SubscriptionCloseKind) {
	s.closed.Store(true)
}

func (s *SubscriptionRecorder) Messages() []string {
	s.mux.Lock()
	defer s.mux.Unlock()
	return s.messages
}

func createFakeStream(messageFunc messageFunc, delay time.Duration, onStart func(input []byte), subscriptionOnStartFn func(ctx StartupHookContext, input []byte) (err error)) *_fakeStream {
	return &_fakeStream{
		messageFunc:           messageFunc,
		delay:                 delay,
		onStart:               onStart,
		subscriptionOnStartFn: subscriptionOnStartFn,
	}
}

type messageFunc func(counter int) (message string, done bool)

var fakeStreamRequestId atomic.Int32

type _fakeStream struct {
	uniqueRequestFn       func(ctx *Context, input []byte, xxh *xxhash.Digest) (err error)
	messageFunc           messageFunc
	onStart               func(input []byte)
	delay                 time.Duration
	isDone                atomic.Bool
	subscriptionOnStartFn func(ctx StartupHookContext, input []byte) (err error)
}

func (f *_fakeStream) SubscriptionOnStart(ctx StartupHookContext, input []byte) (err error) {
	if f.subscriptionOnStartFn == nil {
		return nil
	}
	return f.subscriptionOnStartFn(ctx, input)
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
	if f.uniqueRequestFn != nil {
		return f.uniqueRequestFn(ctx, input, xxh)
	}

	_, err = fmt.Fprint(xxh, fakeStreamRequestId.Add(1))
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
				updater.Complete()
				f.isDone.Store(true)
				return
			default:
				message, done := f.messageFunc(counter)
				updater.Update([]byte(message))
				if done {
					time.Sleep(f.delay)
					updater.Complete()
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

		fetches := Sequence()
		fetches.Trigger = &FetchTreeNode{
			Kind: FetchTreeNodeKindTrigger,
			Item: &FetchItem{
				Fetch: &SingleFetch{
					FetchDependencies: FetchDependencies{
						FetchID: 0,
					},
					Info: &FetchInfo{
						DataSourceID:   "0",
						DataSourceName: "counter",
						QueryPlan: &QueryPlan{
							Query: "subscription {\n    counter\n}",
						},
					},
				},
				ResponsePath: "counter",
			},
		}

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
							Info: &FieldInfo{
								Name:                "counter",
								ExactParentTypeName: "Subscription",
								Source: TypeFieldSource{
									IDs:   []string{"0"},
									Names: []string{"counter"},
								},
								FetchID: 0,
							},
						},
					},
				},
				Fetches: fetches,
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

	setupWithAdditionalDataLoad := func(ctx context.Context, stream SubscriptionDataSource) (*Resolver, *GraphQLSubscription, *SubscriptionRecorder, SubscriptionIdentifier) {
		fetches := Sequence()
		fetches.Trigger = &FetchTreeNode{
			Kind: FetchTreeNodeKindTrigger,
			Item: &FetchItem{
				Fetch: &SingleFetch{
					FetchDependencies: FetchDependencies{
						FetchID: 0,
					},
					Info: &FetchInfo{
						DataSourceID:   "0",
						DataSourceName: "country",
						QueryPlan: &QueryPlan{
							Query: "subscription {\n    countryUpdated {\n        name\n        time {\n            local\n        }\n        }\n}",
						},
					},
				},
				ResponsePath: "countryUpdated",
			},
		}
		fetches.ChildNodes = []*FetchTreeNode{{
			Kind: FetchTreeNodeKindSingle,
			Item: &FetchItem{
				Fetch: &SingleFetch{
					FetchDependencies: FetchDependencies{
						FetchID:           1,
						DependsOnFetchIDs: []int{0},
					},
					Info: &FetchInfo{
						DataSourceID:   "1",
						DataSourceName: "time",
						OperationType:  ast.OperationTypeQuery,
						QueryPlan: &QueryPlan{
							Query: "query($representations: [_Any!]!){\n    _entities(representations: $representations){\n        ... on Time {\n            __typename\n            local\n        }\n    }\n}",
						},
					},
				},
				ResponsePath: "countryUpdated.time",
			},
		}}

		plan := &GraphQLSubscription{
			Trigger: GraphQLSubscriptionTrigger{
				Source: stream,
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							SegmentType: StaticSegmentType,
							Data:        []byte(`{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { countryUpdated { name time { local } } }"}}`),
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
							Name: []byte("countryUpdated"),
							Value: &Object{
								Path: []string{"countryUpdated"},
								Fields: []*Field{{
									Name: []byte("name"),
									Value: &String{
										Path: []string{"name"},
									},
								}, {
									Name: []byte("time"),
									Value: &Object{
										Path: []string{"time"},
										Fields: []*Field{{
											Name: []byte("local"),
											Value: &String{
												Path: []string{"local"},
											},
										}},
									},
								}},
								SourceName: "country",
								TypeName:   "Country",
							},
							Info: &FieldInfo{
								Name:                "countryUpdated",
								ExactParentTypeName: "Subscription",
								Source: TypeFieldSource{
									IDs:   []string{"0"},
									Names: []string{"country"},
								},
								FetchID: 0,
							},
						},
					},
				},
				Fetches: fetches,
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
		}, nil)

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
		}, 1*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`, string(input))
		}, nil)

		resolver, plan, recorder, id := setup(c, fakeStream)

		ctx := &Context{
			ctx: context.Background(),
			ExecutionOptions: ExecutionOptions{
				SendHeartbeat: true,
			},
		}

		err := resolver.AsyncResolveGraphQLSubscription(ctx, plan, recorder, id)
		assert.NoError(t, err)

		recorder.AwaitComplete(t, defaultTimeout)
		messages := recorder.Messages()

		assert.Greater(t, len(messages), 2)
		time.Sleep(resolver.heartbeatInterval)
		// Validate that despite the time, we don't see any heartbeats sent
		assert.Contains(t, messages, `{"data":{"counter":0}}`)
		assert.Contains(t, messages, `{"data":{"counter":1}}`)
		assert.Contains(t, messages, `{"data":{"counter":2}}`)
	})

	t.Run("should successfully delete multiple finished subscriptions", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), counter == 1
		}, 1*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`, string(input))
		}, nil)

		resolver, plan, recorder, id := setup(c, fakeStream)

		ctx := &Context{
			ctx: context.Background(),
			ExecutionOptions: ExecutionOptions{
				SendHeartbeat: true,
			},
		}

		for i := 1; i <= 10; i++ {
			id.ConnectionID = int64(i)
			id.SubscriptionID = int64(i)
			recorder.complete.Store(false)
			err := resolver.AsyncResolveGraphQLSubscription(ctx, plan, recorder, id)
			assert.NoError(t, err)
			recorder.AwaitComplete(t, defaultTimeout)
		}

		recorder.AwaitComplete(t, defaultTimeout)

		time.Sleep(resolver.heartbeatInterval)

		assert.Len(t, recorder.Messages(), 20)

		messages := recorder.Messages()

		require.Equal(t, `{"data":{"counter":0}}`, messages[0])
		require.Equal(t, `{"data":{"counter":1}}`, messages[1])
		require.Equal(t, `{"data":{"counter":0}}`, messages[2])
		require.Equal(t, `{"data":{"counter":1}}`, messages[3])
		require.Equal(t, `{"data":{"counter":0}}`, messages[4])
		require.Equal(t, `{"data":{"counter":1}}`, messages[5])
		require.Equal(t, `{"data":{"counter":0}}`, messages[6])
		require.Equal(t, `{"data":{"counter":1}}`, messages[7])
		require.Equal(t, `{"data":{"counter":0}}`, messages[8])
		require.Equal(t, `{"data":{"counter":1}}`, messages[9])
		require.Equal(t, `{"data":{"counter":0}}`, messages[10])
		require.Equal(t, `{"data":{"counter":1}}`, messages[11])
		require.Equal(t, `{"data":{"counter":0}}`, messages[12])
		require.Equal(t, `{"data":{"counter":1}}`, messages[13])
		require.Equal(t, `{"data":{"counter":0}}`, messages[14])
		require.Equal(t, `{"data":{"counter":1}}`, messages[15])
		require.Equal(t, `{"data":{"counter":0}}`, messages[16])
		require.Equal(t, `{"data":{"counter":1}}`, messages[17])
		require.Equal(t, `{"data":{"counter":0}}`, messages[18])
		require.Equal(t, `{"data":{"counter":1}}`, messages[19])
	})

	t.Run("should propagate extensions to stream", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), counter == 2
		}, 100*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }","extensions":{"foo":"bar"}}}`, string(input))
		}, nil)

		resolver, plan, recorder, id := setup(c, fakeStream)

		ctx := Context{
			ctx:        context.Background(),
			Extensions: []byte(`{"foo":"bar"}`),
		}

		err := resolver.AsyncResolveGraphQLSubscription(&ctx, plan, recorder, id)
		assert.NoError(t, err)
		recorder.AwaitComplete(t, defaultTimeout)

		messages := recorder.Messages()
		assert.Len(t, messages, 3)
		assert.Contains(t, messages, `{"data":{"counter":0}}`)
		assert.Contains(t, messages, `{"data":{"counter":1}}`)
		assert.Contains(t, messages, `{"data":{"counter":2}}`)
	})

	t.Run("should propagate initial payload to stream", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), counter == 2
		}, 100*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"},"initial_payload":{"hello":"world"}}`, string(input))
		}, nil)

		resolver, plan, recorder, id := setup(c, fakeStream)

		ctx := Context{
			ctx:            context.Background(),
			InitialPayload: []byte(`{"hello":"world"}`),
		}

		err := resolver.AsyncResolveGraphQLSubscription(&ctx, plan, recorder, id)
		assert.NoError(t, err)
		recorder.AwaitComplete(t, defaultTimeout)

		messages := recorder.Messages()
		assert.Len(t, messages, 3)
		assert.Contains(t, messages, `{"data":{"counter":0}}`)
		assert.Contains(t, messages, `{"data":{"counter":1}}`)
		assert.Contains(t, messages, `{"data":{"counter":2}}`)
	})

	t.Run("should stop stream on unsubscribe subscription", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), false
		}, time.Millisecond*10, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`, string(input))
		}, nil)

		resolver, plan, recorder, id := setup(c, fakeStream)

		ctx := Context{
			ctx: context.Background(),
		}

		err := resolver.AsyncResolveGraphQLSubscription(&ctx, plan, recorder, id)
		assert.NoError(t, err)
		recorder.AwaitAnyMessageCount(t, defaultTimeout)
		err = resolver.AsyncUnsubscribeSubscription(id)
		assert.NoError(t, err)
		recorder.AwaitClosed(t, defaultTimeout)
		fakeStream.AwaitIsDone(t, defaultTimeout)
	})

	t.Run("should stop stream on unsubscribe client", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), false
		}, time.Millisecond*10, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`, string(input))
		}, nil)

		resolver, plan, recorder, id := setup(c, fakeStream)

		ctx := Context{
			ctx: context.Background(),
		}

		err := resolver.AsyncResolveGraphQLSubscription(&ctx, plan, recorder, id)
		assert.NoError(t, err)
		recorder.AwaitAnyMessageCount(t, defaultTimeout)
		err = resolver.AsyncUnsubscribeClient(id.ConnectionID)
		assert.NoError(t, err)
		recorder.AwaitClosed(t, defaultTimeout)
		fakeStream.AwaitIsDone(t, defaultTimeout)
	})

	t.Run("renders query plan with trigger", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), counter == 0
		}, 100*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`, string(input))
		}, nil)

		resolver, plan, recorder, id := setup(c, fakeStream)

		ctx := &Context{
			ctx: context.Background(),
			ExecutionOptions: ExecutionOptions{
				SkipLoader:                 true,
				IncludeQueryPlanInResponse: true,
			},
		}

		err := resolver.AsyncResolveGraphQLSubscription(ctx, plan, recorder, id)
		assert.NoError(t, err)
		recorder.AwaitComplete(t, defaultTimeout)
		assert.Equal(t, 1, len(recorder.Messages()))
		assert.ElementsMatch(t, []string{
			`{"data":null,"extensions":{"queryPlan":{"version":"1","kind":"Sequence","trigger":{"kind":"Trigger","path":"counter","subgraphName":"counter","subgraphId":"0","fetchId":0,"query":"subscription {\n    counter\n}"}}}}`,
		}, recorder.Messages())
	})

	t.Run("renders query plan with trigger and additional data", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), counter == 0
		}, 100*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { countryUpdated { name time { local } } }"}}`, string(input))
		}, nil)

		resolver, plan, recorder, id := setupWithAdditionalDataLoad(c, fakeStream)

		ctx := &Context{
			ctx: context.Background(),
			ExecutionOptions: ExecutionOptions{
				SkipLoader:                 true,
				IncludeQueryPlanInResponse: true,
			},
		}

		err := resolver.AsyncResolveGraphQLSubscription(ctx, plan, recorder, id)
		assert.NoError(t, err)
		recorder.AwaitComplete(t, defaultTimeout)
		assert.Equal(t, 1, len(recorder.Messages()))
		assert.ElementsMatch(t, []string{
			`{"data":null,"extensions":{"queryPlan":{"version":"1","kind":"Sequence","trigger":{"kind":"Trigger","path":"countryUpdated","subgraphName":"country","subgraphId":"0","fetchId":0,"query":"subscription {\n    countryUpdated {\n        name\n        time {\n            local\n        }\n        }\n}"},"children":[{"kind":"Single","fetch":{"kind":"Single","path":"countryUpdated.time","subgraphName":"time","subgraphId":"1","fetchId":1,"dependsOnFetchIds":[0],"query":"query($representations: [_Any!]!){\n    _entities(representations: $representations){\n        ... on Time {\n            __typename\n            local\n        }\n    }\n}"}}]}}}`,
		}, recorder.Messages())
	})

	t.Run("should successfully allow more than one subscription using http multipart", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), false
		}, 100*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`, string(input))
		}, nil)

		resolver, plan, _, _ := setup(c, fakeStream)

		ctx := &Context{
			ctx: context.Background(),
			ExecutionOptions: ExecutionOptions{
				SendHeartbeat: true,
			},
		}

		const numSubscriptions = 2
		var resolverCompleted atomic.Uint32
		var recorderCompleted atomic.Uint32
		for i := 0; i < numSubscriptions; i++ {
			recorder := &SubscriptionRecorder{
				buf:      &bytes.Buffer{},
				messages: []string{},
				complete: atomic.Bool{},
			}
			recorder.complete.Store(false)

			go func() {
				defer recorderCompleted.Add(1)

				recorder.AwaitAnyMessageCount(t, defaultTimeout)
			}()

			go func() {
				defer resolverCompleted.Add(1)

				err := resolver.ResolveGraphQLSubscription(ctx, plan, recorder)
				assert.ErrorIs(t, err, context.Canceled)
			}()
		}
		assert.Eventually(t, func() bool {
			return recorderCompleted.Load() == numSubscriptions
		}, defaultTimeout, time.Millisecond*100)

		cancel()

		assert.Eventually(t, func() bool {
			return resolverCompleted.Load() == numSubscriptions
		}, defaultTimeout, time.Millisecond*100)
	})

	t.Run("should wait for all in flight operations to be completed", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), true
		}, 100*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`, string(input))
		}, nil)

		resolver, plan, _, id := setup(c, fakeStream)
		recorder := &SubscriptionRecorder{
			buf:      &bytes.Buffer{},
			messages: []string{},
			complete: atomic.Bool{},
		}
		recorder.complete.Store(false)

		ctx := Context{
			ctx: context.Background(),
		}

		err := resolver.AsyncResolveGraphQLSubscription(&ctx, plan, recorder, id)
		assert.NoError(t, err)
		recorder.AwaitAnyMessageCount(t, defaultTimeout)

		err = resolver.AsyncUnsubscribeSubscription(id)
		assert.NoError(t, err)
		recorder.AwaitClosed(t, defaultTimeout)
		fakeStream.AwaitIsDone(t, defaultTimeout)
	})

	t.Run("should call SubscriptionOnStart hook", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		called := make(chan bool, 1)

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), counter == 0
		}, 1*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`, string(input))
		}, func(ctx StartupHookContext, input []byte) (err error) {
			called <- true
			return nil
		})

		resolver, plan, recorder, id := setup(c, fakeStream)

		ctx := &Context{
			ctx: context.Background(),
		}

		err := resolver.AsyncResolveGraphQLSubscription(ctx, plan, recorder, id)
		assert.NoError(t, err)

		select {
		case <-called:
			t.Log("SubscriptionOnStart hook was called")
		case <-time.After(defaultTimeout):
			t.Fatal("SubscriptionOnStart hook was not called")
		}

		recorder.AwaitComplete(t, defaultTimeout)
	})

	t.Run("SubscriptionOnStart ctx has a working subscription updater", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), counter == 0
		}, 1*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`, string(input))
		}, func(ctx StartupHookContext, input []byte) (err error) {
			ctx.Updater([]byte(`{"data":{"counter":1000}}`))
			return nil
		})

		resolver, plan, recorder, id := setup(c, fakeStream)

		ctx := &Context{
			ctx: context.Background(),
			ExecutionOptions: ExecutionOptions{
				SendHeartbeat: true,
			},
		}

		err := resolver.AsyncResolveGraphQLSubscription(ctx, plan, recorder, id)
		assert.NoError(t, err)

		recorder.AwaitComplete(t, defaultTimeout)
		assert.Equal(t, 2, len(recorder.Messages()))
		assert.Equal(t, `{"data":{"counter":1000}}`, recorder.Messages()[0])
		assert.Equal(t, `{"data":{"counter":0}}`, recorder.Messages()[1])
	})

	t.Run("SubscriptionOnStart ctx updater only updates the right subscription", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		executed := atomic.Bool{}
		subsStarted := sync.WaitGroup{}
		subsStarted.Add(2)

		id2 := SubscriptionIdentifier{
			ConnectionID:   1,
			SubscriptionID: 2,
		}

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), counter == 0
		}, 1*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`, string(input))
		}, func(ctx StartupHookContext, input []byte) (err error) {
			if executed.Load() {
				return
			}
			executed.Store(true)
			ctx.Updater([]byte(`{"data":{"counter":1000}}`))
			return nil
		})
		fakeStream.uniqueRequestFn = func(ctx *Context, input []byte, xxh *xxhash.Digest) (err error) {
			return nil
		}

		resolver, plan, recorder, id := setup(c, fakeStream)

		recorder2 := &SubscriptionRecorder{
			buf:      &bytes.Buffer{},
			messages: []string{},
			complete: atomic.Bool{},
		}
		recorder2.complete.Store(false)

		ctx := &Context{
			ctx: context.Background(),
			ExecutionOptions: ExecutionOptions{
				SendHeartbeat: false,
			},
		}

		ctx2 := &Context{
			ctx: context.Background(),
			ExecutionOptions: ExecutionOptions{
				SendHeartbeat: false,
			},
		}

		err := resolver.AsyncResolveGraphQLSubscription(ctx, plan, recorder, id)
		assert.NoError(t, err)
		subsStarted.Done()

		err2 := resolver.AsyncResolveGraphQLSubscription(ctx2, plan, recorder2, id2)
		assert.NoError(t, err2)
		subsStarted.Done()

		recorder.AwaitComplete(t, defaultTimeout)
		recorder2.AwaitComplete(t, defaultTimeout)

		recorders := []*SubscriptionRecorder{recorder, recorder2}

		recorderWith1Message := false
		recorderWith2Messages := false

		for _, r := range recorders {
			if len(r.Messages()) == 2 {
				recorderWith2Messages = true
				assert.Equal(t, `{"data":{"counter":1000}}`, r.Messages()[0])
				assert.Equal(t, `{"data":{"counter":0}}`, r.Messages()[1])
			}
			if len(r.Messages()) == 1 {
				recorderWith1Message = true
				assert.Equal(t, `{"data":{"counter":0}}`, r.Messages()[0])
			}
		}

		assert.True(t, recorderWith1Message)
		assert.True(t, recorderWith2Messages)
	})

	t.Run("SubscriptionOnStart ctx updater on multiple subscriptions with same trigger works", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		subsStarted := sync.WaitGroup{}
		subsStarted.Add(2)

		id2 := SubscriptionIdentifier{
			ConnectionID:   1,
			SubscriptionID: 2,
		}

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), counter == 0
		}, 1*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`, string(input))
		}, func(ctx StartupHookContext, input []byte) (err error) {
			ctx.Updater([]byte(`{"data":{"counter":1000}}`))
			return nil
		})
		fakeStream.uniqueRequestFn = func(ctx *Context, input []byte, xxh *xxhash.Digest) (err error) {
			_, err = xxh.WriteString("unique")
			return
		}

		resolver, plan, recorder, id := setup(c, fakeStream)

		recorder2 := &SubscriptionRecorder{
			buf:      &bytes.Buffer{},
			messages: []string{},
			complete: atomic.Bool{},
		}
		recorder2.complete.Store(false)

		ctx := &Context{
			ctx: context.Background(),
			ExecutionOptions: ExecutionOptions{
				SendHeartbeat: false,
			},
		}

		ctx2 := &Context{
			ctx: context.Background(),
			ExecutionOptions: ExecutionOptions{
				SendHeartbeat: false,
			},
		}

		err := resolver.AsyncResolveGraphQLSubscription(ctx, plan, recorder, id)
		assert.NoError(t, err)
		subsStarted.Done()

		err2 := resolver.AsyncResolveGraphQLSubscription(ctx2, plan, recorder2, id2)
		assert.NoError(t, err2)
		subsStarted.Done()

		recorder.AwaitComplete(t, defaultTimeout)
		recorder2.AwaitComplete(t, defaultTimeout)

		recorders := []*SubscriptionRecorder{recorder, recorder2}

		for _, r := range recorders {
			if len(r.Messages()) == 2 {
				assert.Equal(t, `{"data":{"counter":1000}}`, r.Messages()[0])
				assert.Equal(t, `{"data":{"counter":0}}`, r.Messages()[1])
			} else {
				assert.Fail(t, "should not be here")
			}
		}
	})

	t.Run("SubscriptionOnStart can send a lot of updates without blocking", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()
		workChanBufferSize := 10000

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), counter == 0
		}, 1*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`, string(input))
		}, func(ctx StartupHookContext, input []byte) (err error) {
			for i := 0; i < workChanBufferSize+1; i++ {
				ctx.Updater([]byte(fmt.Sprintf(`{"data":{"counter":%d}}`, i+100)))
			}
			return nil
		})

		resolver, plan, recorder, id := setup(c, fakeStream)

		ctx := &Context{
			ctx: context.Background(),
			ExecutionOptions: ExecutionOptions{
				SendHeartbeat: true,
			},
		}

		err := resolver.AsyncResolveGraphQLSubscription(ctx, plan, recorder, id)
		assert.NoError(t, err)

		recorder.AwaitComplete(t, defaultTimeout)
		assert.Equal(t, workChanBufferSize+2, len(recorder.Messages()))
		for i := 0; i < workChanBufferSize; i++ {
			assert.Equal(t, fmt.Sprintf(`{"data":{"counter":%d}}`, i+100), recorder.Messages()[i])
		}
		assert.Equal(t, `{"data":{"counter":0}}`, recorder.Messages()[workChanBufferSize+1])
	})

	t.Run("SubscriptionOnStart can send a lot of updates in a go routine while updates are coming from other sources", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		messagesToSendFromHook := int32(100)
		messagesDroppedFromHook := &atomic.Int32{}
		messagesToSendFromOtherSources := int32(100)

		firstMessageArrived := make(chan bool, 1)
		hookCompleted := make(chan bool, 1)
		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			if counter == 0 {
				select {
				case firstMessageArrived <- true:
				default:
				}
			}
			if counter == int(messagesToSendFromOtherSources)-1 {
				select {
				case hookCompleted <- true:
				case <-time.After(defaultTimeout):
				}
			}
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), counter == int(messagesToSendFromOtherSources)-1
		}, 1*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`, string(input))
		}, func(ctx StartupHookContext, input []byte) (err error) {
			// send the first update immediately
			ctx.Updater([]byte(fmt.Sprintf(`{"data":{"counter":%d}}`, 0+20000)))

			// start a go routine to send the updates after the source started emitting messages
			go func() {
				// Wait for the first message to arrive before sending updates
				select {
				case <-firstMessageArrived:
					for i := 1; i < int(messagesToSendFromHook); i++ {
						ctx.Updater([]byte(fmt.Sprintf(`{"data":{"counter":%d}}`, i+20000)))
					}
					hookCompleted <- true
				case <-time.After(defaultTimeout):
					// if the first message did not arrive, do not send any updates
					return
				}
			}()

			return nil
		})

		resolver, plan, recorder, id := setup(c, fakeStream)

		ctx := &Context{
			ctx: context.Background(),
			ExecutionOptions: ExecutionOptions{
				SendHeartbeat: false,
			},
		}

		err := resolver.AsyncResolveGraphQLSubscription(ctx, plan, recorder, id)
		assert.NoError(t, err)

		recorder.AwaitComplete(t, defaultTimeout*2)

		var messagesHeartbeat int32
		for _, m := range recorder.Messages() {
			if m == "{}" {
				messagesHeartbeat++
			}
		}
		assert.Equal(t, int32(messagesToSendFromHook+messagesToSendFromOtherSources-messagesDroppedFromHook.Load()+messagesHeartbeat), int32(len(recorder.Messages())))
		assert.Equal(t, `{"data":{"counter":20000}}`, recorder.Messages()[0])
	})

	t.Run("it is possible to have two subscriptions to the same trigger", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := createFakeStream(func(counter int) (message string, done bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), counter == 100
		}, 1*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`, string(input))
		}, func(ctx StartupHookContext, input []byte) (err error) {
			return nil
		})
		fakeStream.uniqueRequestFn = func(ctx *Context, input []byte, xxh *xxhash.Digest) (err error) {
			_, err = xxh.WriteString("unique")
			if err != nil {
				return
			}
			_, err = xxh.Write(input)
			return err
		}

		resolver1, plan1, recorder1, id1 := setup(c, fakeStream)
		_, _, recorder2, id2 := setup(c, fakeStream)
		id2.ConnectionID = id1.ConnectionID + 1
		id2.SubscriptionID = id1.SubscriptionID + 1

		ctx1 := &Context{
			ctx: context.Background(),
		}
		ctx2 := &Context{
			ctx: context.Background(),
		}

		err1 := resolver1.AsyncResolveGraphQLSubscription(ctx1, plan1, recorder1, id1)
		assert.NoError(t, err1)

		err2 := resolver1.AsyncResolveGraphQLSubscription(ctx2, plan1, recorder2, id2)
		assert.NoError(t, err2)

		// complete is called only on the last recorder
		recorder1.AwaitComplete(t, defaultTimeout)
		require.Equal(t, 101, len(recorder1.Messages()))
		assert.Equal(t, `{"data":{"counter":0}}`, recorder1.Messages()[0])
		assert.Equal(t, `{"data":{"counter":100}}`, recorder1.Messages()[100])

		recorder2.AwaitComplete(t, defaultTimeout)
		require.Equal(t, 101, len(recorder2.Messages()))
		assert.Equal(t, `{"data":{"counter":0}}`, recorder2.Messages()[0])
		assert.Equal(t, `{"data":{"counter":100}}`, recorder2.Messages()[100])
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
		}, 100*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000"}`, string(input))
		}, nil)

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
			ctx:       context.Background(),
			Variables: astjson.MustParseBytes([]byte(`{"id":1}`)),
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
		}, 100*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000"}`, string(input))
		}, nil)

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
			ctx:       context.Background(),
			Variables: astjson.MustParseBytes([]byte(`{"id":2}`)),
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
		}, 100*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000"}`, string(input))
		}, nil)

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
			ctx:       context.Background(),
			Variables: astjson.MustParseBytes([]byte(`{"ids":[1,2]}`)),
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
		}, 100*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000"}`, string(input))
		}, nil)

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
			ctx:       context.Background(),
			Variables: astjson.MustParseBytes([]byte(`{"ids":["2","3"]}`)),
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
		}, 100*time.Millisecond, func(input []byte) {
			assert.Equal(t, `{"method":"POST","url":"http://localhost:4000"}`, string(input))
		}, nil)

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
			ctx:       context.Background(),
			Variables: astjson.MustParseBytes([]byte(`{"a":[1,2],"b":[3,4]}`)),
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
		Info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
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
		Info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
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
