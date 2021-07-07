package resolve

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/pkg/fastbuffer"
)

type _fakeDataSource struct {
	data              []byte
	artificialLatency time.Duration
}

var (
	_fakeDataSourceUniqueID = []byte("fake")
)

func (_ *_fakeDataSource) UniqueIdentifier() []byte {
	return _fakeDataSourceUniqueID
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

func TestResolver_ResolveNode(t *testing.T) {
	testFn := func(fn func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string)) func(t *testing.T) {
		ctrl := gomock.NewController(t)
		c, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := New(c)
		node, ctx, expectedOutput := fn(t, r, ctrl)
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
	t.Run("Nullable empty object", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Nullable: true,
		}, Context{Context: context.Background()}, `null`
	}))
	t.Run("empty object", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &EmptyObject{}, Context{Context: context.Background()}, `{}`
	}))
	t.Run("object with null field", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name:  []byte("foo"),
					Value: &Null{},
				},
			},
		}, Context{Context: context.Background()}, `{"foo":null}`
	}))
	t.Run("default graphql object", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Nullable: true,
					},
				},
			},
		}, Context{Context: context.Background()}, `{"data":null}`
	}))
	t.Run("graphql object with simple data source", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
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
		}, Context{Context: context.Background()}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}}}`
	}))
	t.Run("fetch with context variable resolver", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		r.EnableSingleFlightLoader = true
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
							VariableSource:     VariableSourceContext,
							VariableSourcePath: []string{"id"},
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
		}, Context{Context: context.Background(), Variables: []byte(`{"id":1}`)}, `{"name":"Jens"}`
	}))
	t.Run("resolve arrays", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
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
		}, Context{Context: context.Background()}, `{"synchronousFriends":[{"id":1,"name":"Alex"},{"id":2,"name":"Patric"}],"asynchronousFriends":[{"id":1,"name":"Alex"},{"id":2,"name":"Patric"}],"nullableFriends":null,"strings":["foo","bar","baz"],"integers":[123,456,789],"floats":[1.2,3.4,5.6],"booleans":[true,false,true]}`
	}))
	t.Run("array response from data source", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
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
										BufferID:   0,
										HasBuffer:  true,
										OnTypeName: []byte("Dog"),
										Name:       []byte("name"),
										Value: &String{
											Path: []string{"name"},
										},
									},
								},
							},
						},
					},
				},
			}, Context{Context: context.Background()},
			`{"pets":[{"name":"Woofie"}]}`
	}))
	t.Run("non null object with field condition can be null", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
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
									OnTypeName: []byte("Cat"),
									Name:       []byte("name"),
									Value: &String{
										Path: []string{"name"},
									},
								},
							},
						},
					},
				},
			}, Context{Context: context.Background()},
			`{}`
	}))
	t.Run("object with multiple type conditions", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
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
												Name:       []byte("namespace"),
												OnTypeName: []byte("NamespaceCreated"),
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
												Name:       []byte("code"),
												OnTypeName: []byte("Error"),
												Value: &String{
													Nullable: false,
													Path:     []string{"code"},
												},
											},
											{
												Name:       []byte("message"),
												OnTypeName: []byte("Error"),
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
			}, Context{Context: context.Background()},
			`{"data":{"namespaceCreate":{"code":"UserAlreadyHasPersonalNamespace","message":""}}}`
	}))
	t.Run("resolve fieldsets based on __typename", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
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
										BufferID:   0,
										HasBuffer:  true,
										OnTypeName: []byte("Dog"),
										Name:       []byte("name"),
										Value: &String{
											Path: []string{"name"},
										},
									},
								},
							},
						},
					},
				},
			}, Context{Context: context.Background()},
			`{"pets":[{"name":"Woofie"}]}`
	}))
	t.Run("resolve fieldsets asynchronous based on __typename", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
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
										BufferID:   0,
										HasBuffer:  true,
										OnTypeName: []byte("Dog"),
										Name:       []byte("name"),
										Value: &String{
											Path: []string{"name"},
										},
									},
								},
							},
						},
					},
				},
			}, Context{Context: context.Background()},
			`{"pets":[{"name":"Woofie"}]}`
	}))
	t.Run("parent object variables", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		r.EnableSingleFlightLoader = true
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
										VariableSource:     VariableSourceObject,
										VariableSourcePath: []string{"id"},
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
		}, Context{Context: context.Background()}, `{"id":1,"name":"Jens","pet":{"name":"Woofie"}}`
	}))
}

func TestResolver_WithHooks(t *testing.T) {
	testFn := func(fn func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string)) func(t *testing.T) {
		ctrl := gomock.NewController(t)
		c, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := New(c)
		node, ctx, expectedOutput := fn(t, r, ctrl)
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
	t.Run("resolve with hooks", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {

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
		}, Context{Context: context.Background(), beforeFetchHook: beforeFetch, afterFetchHook: afterFetch}, `{"data":{"user":{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}}}`
	}))
}

func TestResolver_ResolveGraphQLResponse(t *testing.T) {
	testFn := func(fn func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string)) func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		c, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := New(c)
		node, ctx, expectedOutput := fn(t, r, ctrl)
		return func(t *testing.T) {
			t.Helper()

			buf := &bytes.Buffer{}
			err := r.ResolveGraphQLResponse(&ctx, node, nil, buf)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutput, buf.String())
			ctrl.Finish()
		}
	}
	t.Run("empty graphql response for nullable object", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: true,
			},
		}, Context{Context: context.Background()}, `{"data":null}`
	}))
	t.Run("empty graphql response for not nullable query field", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
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
		}, Context{Context: context.Background()}, `{"errors":[{"message":"failed to resolve","locations":[{"line": 3, "path": 4}]}],"data":null}`
	}))
	t.Run("fetch with simple error", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		r.EnableSingleFlightLoader = true
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
		}, Context{Context: context.Background()}, `{"errors":[{"message":"errorMessage"}],"data":{"name":null}}`
	}))
	t.Run("nested fetch error for non-nullable field", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		r.EnableSingleFlightLoader = true
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
		}, Context{Context: context.Background()}, `{"errors":[{"message":"errorMessage"}],"data":null}`
	}))
	t.Run("fetch with two Errors", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		r.EnableSingleFlightLoader = true
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
		}, Context{Context: context.Background()}, `{"errors":[{"message":"errorMessage1"},{"message":"errorMessage2"}],"data":{"name":null}}`
	}))
	t.Run("null field should bubble up to parent with error", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
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
		}, Context{Context: context.Background()}, `{"errors":[{"message":"failed to resolve","locations":[{"line": 0, "path": 0}],"path":["objectObject","objectField"]}],"data":{"stringObject":null,"integerObject":null,"floatObject":null,"booleanObject":null,"objectObject":null,"arrayObject":null,"asynchronousArrayObject":null,"nullableArray":null}}`
	}))
	t.Run("empty array should resolve correctly", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
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
		}, Context{Context: context.Background()}, `{"data":{"nonNullArray":[],"nullableArray":null}}`
	}))
	t.Run("complex GraphQL Server plan", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		r.EnableSingleFlightLoader = true
		serviceOne := NewMockDataSource(ctrl)
		serviceOne.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				expected := `{"url":"https://service.one","body":{"query":"query($firstArg: String, $thirdArg: Int){serviceOne(serviceOneArg: $firstArg){fieldOne} anotherServiceOne(anotherServiceOneArg: $thirdArg){fieldOne} reusingServiceOne(reusingServiceOneArg: $firstArg){fieldOne}}","variables":{"thirdArg":123,"firstArg":"firstArgValue"}}}`
				assert.Equal(t, expected, actual)
				_, err = w.Write([]byte(`{"serviceOne":{"fieldOne":"fieldOneValue"},"anotherServiceOne":{"fieldOne":"anotherFieldOneValue"},"reusingServiceOne":{"fieldOne":"reUsingFieldOneValue"}}`))
				return
			}).
			Return(nil)

		serviceTwo := NewMockDataSource(ctrl)
		serviceTwo.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				expected := `{"url":"https://service.two","body":{"query":"query($secondArg: Boolean, $fourthArg: Float){serviceTwo(serviceTwoArg: $secondArg){fieldTwo} secondServiceTwo(secondServiceTwoArg: $fourthArg){fieldTwo}}","variables":{"fourthArg":12.34,"secondArg":true}}}`
				assert.Equal(t, expected, actual)
				_, err = w.Write([]byte(`{"serviceTwo":{"fieldTwo":"fieldTwoValue"},"secondServiceTwo":{"fieldTwo":"secondFieldTwoValue"}}`))
				return
			}).
			Return(nil)

		nestedServiceOne := NewMockDataSource(ctrl)
		nestedServiceOne.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				expected := `{"url":"https://service.one","body":{"query":"{serviceOne {fieldOne}}"}}`
				assert.Equal(t, expected, actual)
				_, err = w.Write([]byte(`{"serviceOne":{"fieldOne":"fieldOneValue"}}`))
				return
			}).
			Return(nil)

		return &GraphQLResponse{
			Data: &Object{
				Fetch: &ParallelFetch{
					Fetches: []*SingleFetch{
						{
							BufferId: 0,
							Input:    `{"url":"https://service.one","body":{"query":"query($firstArg: String, $thirdArg: Int){serviceOne(serviceOneArg: $firstArg){fieldOne} anotherServiceOne(anotherServiceOneArg: $thirdArg){fieldOne} reusingServiceOne(reusingServiceOneArg: $firstArg){fieldOne}}","variables":{"thirdArg":$$1$$,"firstArg":"$$0$$"}}}`,
							InputTemplate: InputTemplate{
								Segments: []TemplateSegment{
									{
										SegmentType: StaticSegmentType,
										Data:        []byte(`{"url":"https://service.one","body":{"query":"query($firstArg: String, $thirdArg: Int){serviceOne(serviceOneArg: $firstArg){fieldOne} anotherServiceOne(anotherServiceOneArg: $thirdArg){fieldOne} reusingServiceOne(reusingServiceOneArg: $firstArg){fieldOne}}","variables":{"thirdArg":`),
									},
									{
										SegmentType:        VariableSegmentType,
										VariableSource:     VariableSourceContext,
										VariableSourcePath: []string{"thirdArg"},
									},
									{
										SegmentType: StaticSegmentType,
										Data:        []byte(`,"firstArg":"`),
									},
									{
										SegmentType:        VariableSegmentType,
										VariableSource:     VariableSourceContext,
										VariableSourcePath: []string{"firstArg"},
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
						},
						{
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
										VariableSource:     VariableSourceContext,
										VariableSourcePath: []string{"fourthArg"},
									},
									{
										SegmentType: StaticSegmentType,
										Data:        []byte(`,"secondArg":`),
									},
									{
										SegmentType:        VariableSegmentType,
										VariableSource:     VariableSourceContext,
										VariableSourcePath: []string{"secondArg"},
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
		}, Context{Context: context.Background(), Variables: []byte(`{"firstArg":"firstArgValue","thirdArg":123,"secondArg": true, "fourthArg": 12.34}`)}, `{"data":{"serviceOne":{"fieldOne":"fieldOneValue"},"serviceTwo":{"fieldTwo":"fieldTwoValue","serviceOneResponse":{"fieldOne":"fieldOneValue"}},"anotherServiceOne":{"fieldOne":"anotherFieldOneValue"},"secondServiceTwo":{"fieldTwo":"secondFieldTwoValue"},"reusingServiceOne":{"fieldOne":"reUsingFieldOneValue"}}}`
	}))
	t.Run("federation", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		r.EnableSingleFlightLoader = true

		userService := NewMockDataSource(ctrl)
		userService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`
				assert.Equal(t, expected, actual)
				_, err = w.Write([]byte(`{"me": {"id": "1234","username": "Me","__typename": "User"}}`))
				return
			}).
			Return(nil)

		reviewsService := NewMockDataSource(ctrl)
		reviewsService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"1234","__typename":"User"}]}}}`
				assert.Equal(t, expected, actual)
				_, err = w.Write([]byte(`{"reviews": [{"body": "A highly effective form of birth control.","product": {"upc": "top-1","__typename": "Product"}},{"body": "Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product": {"upc": "top-1","__typename": "Product"}}]}`))
				return
			}).
			Return(nil)

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
					_, err = w.Write([]byte(`{"name": "Trilby"}`))
				case 2:
					expected := `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}}}`
					assert.Equal(t, expected, actual)
					_, err = w.Write([]byte(`{"name": "Trilby"}`))
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
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"`),
											SegmentType: StaticSegmentType,
										},
										{
											SegmentType:        VariableSegmentType,
											VariableSource:     VariableSourceObject,
											VariableSourcePath: []string{"id"},
										},
										{
											Data:        []byte(`","__typename":"User"}]}}}`),
											SegmentType: StaticSegmentType,
										},
									},
								},
								DataSource: reviewsService,
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
																		Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"`),
																		SegmentType: StaticSegmentType,
																	},
																	{
																		SegmentType:        VariableSegmentType,
																		VariableSource:     VariableSourceObject,
																		VariableSourcePath: []string{"upc"},
																	},
																	{
																		Data:        []byte(`","__typename":"Product"}]}}}`),
																		SegmentType: StaticSegmentType,
																	},
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
		}, Context{Context: context.Background(), Variables: nil}, `{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-1","name":"Trilby"}}]}}}`
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
			c, cancel := context.WithCancel(context.Background())
			defer cancel()
			resolver := New(c)

			header := make(http.Header)
			header.Set(tc.header, "foo")
			ctx := &Context{
				Context: context.Background(),
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
									VariableSource:     VariableSourceRequestHeader,
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

func (f *_fakeStream) UniqueIdentifier() []byte {
	return []byte("fake")
}

func TestResolver_ResolveGraphQLSubscription(t *testing.T) {
	setup := func(ctx context.Context, stream *_fakeStream) (*Resolver, *GraphQLSubscription, *TestFlushWriter) {
		plan := &GraphQLSubscription{
			Trigger: GraphQLSubscriptionTrigger{
				Source: stream,
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

		return New(ctx), plan, out
	}

	t.Run("should return errors if the upstream data has errors", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := FakeStream(cancel, func(count int) (message string, ok bool) {
			return `{"errors":[{"message":"Validation error occurred","locations":[{"line":1,"column":1}],"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}],"data":null}`, false
		})

		resolver, plan, out := setup(c, fakeStream)
		ctx := Context{
			Context: c,
		}

		err := resolver.ResolveGraphQLSubscription(&ctx, plan, out)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(out.flushed))
		assert.Equal(t, `{"errors":[{"message":"failed to resolve","locations":[{"line": 0, "path": 0}]},{"message":"Validation error occurred","locations":[{"line":1,"column":1}],"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}],"data":null}`, out.flushed[0])
	})

	t.Run("should successfully get result from upstream", func(t *testing.T) {
		c, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeStream := FakeStream(cancel, func(count int) (message string, ok bool) {
			return fmt.Sprintf(`{"data":{"counter":%d}}`, count), true
		})

		resolver, plan, out := setup(c, fakeStream)
		ctx := Context{
			Context: c,
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

	c, cancel := context.WithCancel(context.Background())
	defer cancel()

	resolver := New(c)

	serviceOneDS := FakeDataSource(`{"serviceOne":{"fieldOne":"fieldOneValue"},"anotherServiceOne":{"fieldOne":"anotherFieldOneValue"},"reusingServiceOne":{"fieldOne":"reUsingFieldOneValue"}}`)
	serviceTwoDS := FakeDataSource(`{"serviceTwo":{"fieldTwo":"fieldTwoValue"},"secondServiceTwo":{"fieldTwo":"secondFieldTwoValue"}}`)
	nestedServiceOneDS := FakeDataSource(`{"serviceOne":{"fieldOne":"fieldOneValue"}}`)

	plan := &GraphQLResponse{
		Data: &Object{
			Fetch: &ParallelFetch{
				Fetches: []*SingleFetch{
					{
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
									VariableSource:     VariableSourceContext,
									VariableSourcePath: []string{"thirdArg"},
								},
								{
									SegmentType: StaticSegmentType,
									Data:        []byte(`,"firstArg":`),
								},
								{
									SegmentType:        VariableSegmentType,
									VariableSource:     VariableSourceContext,
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
					{
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
									VariableSource:     VariableSourceContext,
									VariableSourcePath: []string{"fourthArg"},
								},
								{
									SegmentType: StaticSegmentType,
									Data:        []byte(`,"secondArg":`),
								},
								{
									SegmentType:        VariableSegmentType,
									VariableSource:     VariableSourceContext,
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
		resolver.EnableSingleFlightLoader = true
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

func TestInputTemplate_Render(t *testing.T) {

	runTest := func(variables string, sourcePath []string, renderAsGraphQLVariable bool, expected string) {
		template := InputTemplate{
			Segments: []TemplateSegment{
				{
					SegmentType:          VariableSegmentType,
					VariableSource:       VariableSourceContext,
					VariableSourcePath:   sourcePath,
					RenderAsGraphQLValue: renderAsGraphQLVariable,
				},
			},
		}
		ctx := &Context{
			Variables: []byte(variables),
		}
		buf := fastbuffer.New()
		err := template.Render(ctx, nil, buf)
		assert.NoError(t, err)
		out := buf.String()
		assert.Equal(t, expected, out)
	}

	t.Run("string scalar", func(t *testing.T) {
		runTest(`{"foo":"bar"}`, []string{"foo"}, false, "bar")
	})
	t.Run("boolean scalar", func(t *testing.T) {
		runTest(`{"foo":true}`, []string{"foo"}, false, "true")
	})
	t.Run("json object pass through", func(t *testing.T) {
		runTest(`{"foo":{"bar":"baz"}}`, []string{"foo"}, false, `{"bar":"baz"}`)
	})
	t.Run("json object as graphql object", func(t *testing.T) {
		runTest(`{"foo":{"bar":"baz"}}`, []string{"foo"}, true, `{bar:\"baz\"}`)
	})
	t.Run("json object as graphql object with null", func(t *testing.T) {
		runTest(`{"foo":null}`, []string{"foo"}, true, `null`)
	})
	t.Run("json object as graphql object with number", func(t *testing.T) {
		runTest(`{"foo":123}`, []string{"foo"}, true, `123`)
	})
	t.Run("json object as graphql object with boolean", func(t *testing.T) {
		runTest(`{"foo":{"bar":true}}`, []string{"foo"}, true, `{bar:true}`)
	})
	t.Run("json object as graphql object with number", func(t *testing.T) {
		runTest(`{"foo":{"bar":123}}`, []string{"foo"}, true, `{bar:123}`)
	})
	t.Run("json object as graphql object with float", func(t *testing.T) {
		runTest(`{"foo":{"bar":1.23}}`, []string{"foo"}, true, `{bar:1.23}`)
	})
	t.Run("json object as graphql object with nesting", func(t *testing.T) {
		runTest(`{"foo":{"bar":{"baz":"bat"}}}`, []string{"foo"}, true, `{bar:{baz:\"bat\"}}`)
	})
	t.Run("json object as graphql object with single array", func(t *testing.T) {
		runTest(`{"foo":["bar"]}`, []string{"foo"}, true, `[\"bar\"]`)
	})
	t.Run("json object as graphql object with array", func(t *testing.T) {
		runTest(`{"foo":["bar","baz"]}`, []string{"foo"}, true, `[\"bar\",\"baz\"]`)
	})
	t.Run("json object as graphql object with object array", func(t *testing.T) {
		runTest(`{"foo":[{"bar":"baz"},{"bar":"bat"}]}`, []string{"foo"}, true, `[{bar:\"baz\"},{bar:\"bat\"}]`)
	})
}
