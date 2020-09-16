package resolve

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/subscription"
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

func (f *_fakeDataSource) Load(ctx context.Context, input []byte, pair *BufPair) (err error) {
	if f.artificialLatency != 0 {
		time.Sleep(f.artificialLatency)
	}
	pair.Data.WriteBytes(f.data)
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
		r := New()
		node, ctx, expectedOutput := fn(t, r, ctrl)
		return func(t *testing.T) {
			buf := &BufPair{
				Data:   fastbuffer.New(),
				Errors: fastbuffer.New(),
			}
			err := r.resolveNode(ctx, node, nil, buf)
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
			FieldSets: []FieldSet{
				{
					Fields: []Field{
						{
							Name:  []byte("foo"),
							Value: &Null{},
						},
					},
				},
			},
		}, Context{Context: context.Background()}, `{"foo":null}`
	}))
	t.Run("default graphql object", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			FieldSets: []FieldSet{
				{
					Fields: []Field{
						{
							Name: []byte("data"),
							Value: &Object{
								Nullable: true,
							},
						},
					},
				},
			},
		}, Context{Context: context.Background()}, `{"data":null}`
	}))
	t.Run("graphql object with simple data source", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			FieldSets: []FieldSet{
				{
					Fields: []Field{
						{
							Name: []byte("data"),
							Value: &Object{
								FieldSets: []FieldSet{
									{
										Fields: []Field{
											{
												Name: []byte("user"),
												Value: &Object{
													Fetch: &SingleFetch{
														BufferId:   0,
														DataSource: FakeDataSource(`{"id":"1","name":"Jens","registered":true,"pet":{"name":"Barky","kind":"Dog"}}`),
													},
													FieldSets: []FieldSet{
														{
															BufferID:  0,
															HasBuffer: true,
															Fields: []Field{
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
																		FieldSets: []FieldSet{
																			{
																				Fields: []Field{
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
		mockDataSource.EXPECT().UniqueIdentifier().Return([]byte("mock"))
		mockDataSource.EXPECT().
			Load(gomock.Any(), []byte(`{"id":1}`), gomock.AssignableToTypeOf(&BufPair{})).
			Do(func(ctx context.Context, input []byte, pair *BufPair) (err error) {
				pair.Data.WriteBytes([]byte(`{"name":"Jens"}`))
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
			FieldSets: []FieldSet{
				{
					HasBuffer: true,
					BufferID:  0,
					Fields: []Field{
						{
							Name: []byte("name"),
							Value: &String{
								Path: []string{"name"},
							},
						},
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
			FieldSets: []FieldSet{
				{
					BufferID:  0,
					HasBuffer: true,
					Fields: []Field{
						{
							Name: []byte("synchronousFriends"),
							Value: &Array{
								Path:                []string{"friends"},
								ResolveAsynchronous: false,
								Nullable:            true,
								Item: &Object{
									FieldSets: []FieldSet{
										{
											Fields: []Field{
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
							},
						},
						{
							Name: []byte("asynchronousFriends"),
							Value: &Array{
								Path:                []string{"friends"},
								ResolveAsynchronous: true,
								Nullable:            true,
								Item: &Object{
									FieldSets: []FieldSet{
										{
											Fields: []Field{
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
				FieldSets: []FieldSet{
					{
						BufferID:  0,
						HasBuffer: true,
						Fields: []Field{
							{
								Name: []byte("pets"),
								Value: &Array{
									Item: &Object{
										FieldSets: []FieldSet{
											{
												BufferID:   0,
												HasBuffer:  true,
												OnTypeName: []byte("Dog"),
												Fields: []Field{
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
			}, Context{Context: context.Background()},
			`{"pets":[{"name":"Woofie"}]}`
	}))
	t.Run("resolve fieldsets based on __typename", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
				Fetch: &SingleFetch{
					BufferId:   0,
					DataSource: FakeDataSource(`{"pets":[{"__typename":"Dog","name":"Woofie"},{"__typename":"Cat","name":"Mietzie"}]}`),
				},
				FieldSets: []FieldSet{
					{
						BufferID:  0,
						HasBuffer: true,
						Fields: []Field{
							{
								Name: []byte("pets"),
								Value: &Array{
									Path: []string{"pets"},
									Item: &Object{
										FieldSets: []FieldSet{
											{
												BufferID:   0,
												HasBuffer:  true,
												OnTypeName: []byte("Dog"),
												Fields: []Field{
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
			}, Context{Context: context.Background()},
			`{"pets":[{"name":"Woofie"}]}`
	}))
	t.Run("resolve fieldsets asynchronous based on __typename", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
				Fetch: &SingleFetch{
					BufferId:   0,
					DataSource: FakeDataSource(`{"pets":[{"__typename":"Dog","name":"Woofie"},{"__typename":"Cat","name":"Mietzie"}]}`),
				},
				FieldSets: []FieldSet{
					{
						BufferID:  0,
						HasBuffer: true,
						Fields: []Field{
							{
								Name: []byte("pets"),
								Value: &Array{
									ResolveAsynchronous: true,
									Path:                []string{"pets"},
									Item: &Object{
										FieldSets: []FieldSet{
											{
												BufferID:   0,
												HasBuffer:  true,
												OnTypeName: []byte("Dog"),
												Fields: []Field{
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
			}, Context{Context: context.Background()},
			`{"pets":[{"name":"Woofie"}]}`
	}))
	t.Run("parent object variables", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		r.EnableSingleFlightLoader = true
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().UniqueIdentifier().Return([]byte("mock"))
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.GotFormatterAdapter(gotBytesFormatter{}, matchBytes(`{"id":1}`)), gomock.AssignableToTypeOf(&BufPair{})).
			Do(func(ctx context.Context, input []byte, pair *BufPair) (err error) {
				pair.Data.WriteBytes([]byte(`{"name":"Woofie"}`))
				return
			}).
			Return(nil)
		return &Object{
			Fetch: &SingleFetch{
				BufferId:   0,
				DataSource: FakeDataSource(`{"id":1,"name":"Jens"}`),
			},
			FieldSets: []FieldSet{
				{
					HasBuffer: true,
					BufferID:  0,
					Fields: []Field{
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
						{
							Name: []byte("pet"),
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
								FieldSets: []FieldSet{
									{
										BufferID:  0,
										HasBuffer: true,
										Fields: []Field{
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
		}, Context{Context: context.Background()}, `{"id":1,"name":"Jens","pet":{"name":"Woofie"}}`
	}))
}

func TestResolver_ResolveGraphQLResponse(t *testing.T) {
	testFn := func(fn func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string)) func(t *testing.T) {
		ctrl := gomock.NewController(t)
		r := New()
		node, ctx, expectedOutput := fn(t, r, ctrl)
		return func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := r.ResolveGraphQLResponse(ctx, node, nil, buf)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutput, buf.String())
			ctrl.Finish()
		}
	}
	t.Run("empty graphql response", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: true,
			},
		}, Context{Context: context.Background()}, `{"data":null}`
	}))
	t.Run("fetch with simple error", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		r.EnableSingleFlightLoader = true
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().UniqueIdentifier().Return([]byte("mock"))
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&BufPair{})).
			Do(func(ctx context.Context, input []byte, pair *BufPair) (err error) {
				pair.WriteErr([]byte("errorMessage"), nil, nil)
				return
			}).
			Return(nil)
		return &GraphQLResponse{
			Data: &Object{
				Nullable: true,
				Fetch: &SingleFetch{
					BufferId:   0,
					DataSource: mockDataSource,
				},
				FieldSets: []FieldSet{
					{
						HasBuffer: true,
						BufferID:  0,
						Fields: []Field{
							{
								Name: []byte("name"),
								Value: &String{
									Path:     []string{"name"},
									Nullable: true,
								},
							},
						},
					},
				},
			},
		}, Context{Context: context.Background()}, `{"Errors":[{"message":"errorMessage"}],"data":{"name":null}}`
	}))
	t.Run("fetch with two Errors", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		r.EnableSingleFlightLoader = true
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().UniqueIdentifier().Return([]byte("mock"))
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&BufPair{})).
			Do(func(ctx context.Context, input []byte, pair *BufPair) (err error) {
				pair.WriteErr([]byte("errorMessage1"), nil, nil)
				pair.WriteErr([]byte("errorMessage2"), nil, nil)
				return
			}).
			Return(nil)
		return &GraphQLResponse{
			Data: &Object{
				Fetch: &SingleFetch{
					BufferId:   0,
					DataSource: mockDataSource,
				},
				FieldSets: []FieldSet{
					{
						HasBuffer: true,
						BufferID:  0,
						Fields: []Field{
							{
								Name: []byte("name"),
								Value: &String{
									Path:     []string{"name"},
									Nullable: true,
								},
							},
						},
					},
				},
			},
		}, Context{Context: context.Background()}, `{"Errors":[{"message":"errorMessage1"},{"message":"errorMessage2"}],"data":{"name":null}}`
	}))
	t.Run("null field should bubble up to parent with error", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: true,
				Fetch: &SingleFetch{
					BufferId:   0,
					DataSource: FakeDataSource(`[{"id":1},{"id":2},{"id":3}]`),
				},
				FieldSets: []FieldSet{
					{
						HasBuffer: true,
						BufferID:  0,
						Fields: []Field{
							{
								Name: []byte("stringObject"),
								Value: &Object{
									Nullable: true,
									FieldSets: []FieldSet{
										{
											Fields: []Field{
												{
													Name: []byte("stringField"),
													Value: &String{
														Nullable: false,
													},
												},
											},
										},
									},
								},
							},
							{
								Name: []byte("integerObject"),
								Value: &Object{
									Nullable: true,
									FieldSets: []FieldSet{
										{
											Fields: []Field{
												{
													Name: []byte("integerField"),
													Value: &Integer{
														Nullable: false,
													},
												},
											},
										},
									},
								},
							},
							{
								Name: []byte("floatObject"),
								Value: &Object{
									Nullable: true,
									FieldSets: []FieldSet{
										{
											Fields: []Field{
												{
													Name: []byte("floatField"),
													Value: &Float{
														Nullable: false,
													},
												},
											},
										},
									},
								},
							},
							{
								Name: []byte("booleanObject"),
								Value: &Object{
									Nullable: true,
									FieldSets: []FieldSet{
										{
											Fields: []Field{
												{
													Name: []byte("booleanField"),
													Value: &Boolean{
														Nullable: false,
													},
												},
											},
										},
									},
								},
							},
							{
								Name: []byte("objectObject"),
								Value: &Object{
									Nullable: true,
									FieldSets: []FieldSet{
										{
											Fields: []Field{
												{
													Name: []byte("objectField"),
													Value: &Object{
														Nullable: false,
													},
												},
											},
										},
									},
								},
							},
							{
								Name: []byte("arrayObject"),
								Value: &Object{
									Nullable: true,
									FieldSets: []FieldSet{
										{
											Fields: []Field{
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
								},
							},
							{
								Name: []byte("asynchronousArrayObject"),
								Value: &Object{
									Nullable: true,
									FieldSets: []FieldSet{
										{
											Fields: []Field{
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
				},
			},
		}, Context{Context: context.Background()}, `{"data":{"stringObject":null,"integerObject":null,"floatObject":null,"booleanObject":null,"objectObject":null,"arrayObject":null,"asynchronousArrayObject":null,"nullableArray":null}}`
	}))
	t.Run("empty array should resolve correctly", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Nullable: true,
				Fetch: &SingleFetch{
					BufferId:   0,
					DataSource: FakeDataSource(`[]`),
				},
				FieldSets: []FieldSet{
					{
						HasBuffer: true,
						BufferID:  0,
						Fields: []Field{
							{
								Name: []byte("nonNullArray"),
								Value: &Array{
									Nullable: false,
									Item: &Object{
										Nullable: false,
										FieldSets: []FieldSet{
											{
												Fields: []Field{
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
							{
								Name: []byte("nullableArray"),
								Value: &Array{
									Nullable: true,
									Item: &Object{
										Nullable: false,
										FieldSets: []FieldSet{
											{
												Fields: []Field{
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
						},
					},
				},
			},
		}, Context{Context: context.Background()}, `{"data":{"nonNullArray":[],"nullableArray":null}}`
	}))
	t.Run("complex GraphQL Server plan", testFn(func(t *testing.T, r *Resolver, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		r.EnableSingleFlightLoader = true
		serviceOne := NewMockDataSource(ctrl)
		serviceOne.EXPECT().UniqueIdentifier().Return([]byte("serviceOne"))
		serviceOne.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&BufPair{})).
			Do(func(ctx context.Context, input []byte, pair *BufPair) (err error) {
				actual := string(input)
				expected := `{"url":"https://service.one","body":{"query":"query($firstArg: String, $thirdArg: Int){serviceOne(serviceOneArg: $firstArg){fieldOne} anotherServiceOne(anotherServiceOneArg: $thirdArg){fieldOne} reusingServiceOne(reusingServiceOneArg: $firstArg){fieldOne}}","variables":{"thirdArg":123,"firstArg":"firstArgValue"}}}`
				assert.Equal(t, expected, actual)
				pair.Data.WriteString(`{"serviceOne":{"fieldOne":"fieldOneValue"},"anotherServiceOne":{"fieldOne":"anotherFieldOneValue"},"reusingServiceOne":{"fieldOne":"reUsingFieldOneValue"}}`)
				return
			}).
			Return(nil)

		serviceTwo := NewMockDataSource(ctrl)
		serviceTwo.EXPECT().UniqueIdentifier().Return([]byte("serviceTwo"))
		serviceTwo.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&BufPair{})).
			Do(func(ctx context.Context, input []byte, pair *BufPair) (err error) {
				actual := string(input)
				expected := `{"url":"https://service.two","body":{"query":"query($secondArg: Boolean, $fourthArg: Float){serviceTwo(serviceTwoArg: $secondArg){fieldTwo} secondServiceTwo(secondServiceTwoArg: $fourthArg){fieldTwo}}","variables":{"fourthArg":12.34,"secondArg":true}}}`
				assert.Equal(t, expected, actual)
				pair.Data.WriteString(`{"serviceTwo":{"fieldTwo":"fieldTwoValue"},"secondServiceTwo":{"fieldTwo":"secondFieldTwoValue"}}`)
				return
			}).
			Return(nil)

		nestedServiceOne := NewMockDataSource(ctrl)
		nestedServiceOne.EXPECT().UniqueIdentifier().Return([]byte("nestedServiceOne"))
		nestedServiceOne.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&BufPair{})).
			Do(func(ctx context.Context, input []byte, pair *BufPair) (err error) {
				actual := string(input)
				expected := `{"url":"https://service.one","body":{"query":"{serviceOne {fieldOne}}"}}`
				assert.Equal(t, expected, actual)
				pair.Data.WriteString(`{"serviceOne":{"fieldOne":"fieldOneValue"}}`)
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
				FieldSets: []FieldSet{
					{
						BufferID:  0,
						HasBuffer: true,
						Fields: []Field{
							{
								Name: []byte("serviceOne"),
								Value: &Object{
									Path: []string{"serviceOne"},
									FieldSets: []FieldSet{
										{
											Fields: []Field{
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
					},
					{
						BufferID:  1,
						HasBuffer: true,
						Fields: []Field{
							{
								Name: []byte("serviceTwo"),
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
									FieldSets: []FieldSet{
										{
											Fields: []Field{
												{
													Name: []byte("fieldTwo"),
													Value: &String{
														Path: []string{"fieldTwo"},
													},
												},
											},
										},
										{
											BufferID:  2,
											HasBuffer: true,
											Fields: []Field{
												{
													Name: []byte("serviceOneResponse"),
													Value: &Object{
														Path: []string{"serviceOne"},
														FieldSets: []FieldSet{
															{
																Fields: []Field{
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
										},
									},
								},
							},
						},
					},
					{
						BufferID:  0,
						HasBuffer: true,
						Fields: []Field{
							{
								Name: []byte("anotherServiceOne"),
								Value: &Object{
									Path: []string{"anotherServiceOne"},
									FieldSets: []FieldSet{
										{
											Fields: []Field{
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
					},
					{
						BufferID:  1,
						HasBuffer: true,
						Fields: []Field{
							{
								Name: []byte("secondServiceTwo"),
								Value: &Object{
									Path: []string{"secondServiceTwo"},
									FieldSets: []FieldSet{
										{
											Fields: []Field{
												{
													Name: []byte("fieldTwo"),
													Value: &String{
														Path: []string{"fieldTwo"},
													},
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
						Fields: []Field{
							{
								Name: []byte("reusingServiceOne"),
								Value: &Object{
									Path: []string{"reusingServiceOne"},
									FieldSets: []FieldSet{
										{
											Fields: []Field{
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
					},
				},
			},
		}, Context{Context: context.Background(), Variables: []byte(`{"firstArg":"firstArgValue","thirdArg":123,"secondArg": true, "fourthArg": 12.34}`)}, `{"data":{"serviceOne":{"fieldOne":"fieldOneValue"},"serviceTwo":{"fieldTwo":"fieldTwoValue","serviceOneResponse":{"fieldOne":"fieldOneValue"}},"anotherServiceOne":{"fieldOne":"anotherFieldOneValue"},"secondServiceTwo":{"fieldTwo":"secondFieldTwoValue"},"reusingServiceOne":{"fieldOne":"reUsingFieldOneValue"}}}`
	}))
}

type TestFlushWriter struct {
	flushed     []string
	buf         bytes.Buffer
}

func (t *TestFlushWriter) Write(p []byte) (n int, err error) {
	return t.buf.Write(p)
}

func (t *TestFlushWriter) Flush() {
	t.flushed = append(t.flushed, t.buf.String())
	t.buf.Reset()
}

type FakeStream struct {
	cancel      context.CancelFunc
}

func (f *FakeStream) Start(input []byte, next chan<- []byte, stop <-chan struct{}) {
	time.Sleep(time.Millisecond)
	count := 0
	for {
		if count == 3 {
			f.cancel()
			return
		}
		next <- []byte(fmt.Sprintf(`{"counter":%d}`, count))
		count++
	}
}

func (f *FakeStream) UniqueIdentifier() []byte {
	return []byte("fake")
}

func TestResolver_ResolveGraphQLSubscription(t *testing.T) {
	resolver := New()
	c, cancel := context.WithCancel(context.Background())
	defer cancel()
	man := subscription.NewManager(&FakeStream{
		cancel:      cancel,
	})
	manCtx, cancelMan := context.WithCancel(context.Background())
	defer cancelMan()
	man.Run(manCtx.Done())
	resolver.RegisterTriggerManager(man)
	plan := &GraphQLSubscription{
		Trigger: GraphQLSubscriptionTrigger{
			ManagerID: []byte("fake"),
			Input:     "",
		},
		Response: &GraphQLResponse{
			Data: &Object{
				FieldSets: []FieldSet{
					{
						Fields: []Field{
							{
								Name: []byte("counter"),
								Value: &Integer{
									Path: []string{"counter"},
								},
							},
						},
					},
				},
			},
		},
	}
	ctx := Context{
		Context: c,
	}
	out := &TestFlushWriter{
		buf:         bytes.Buffer{},
	}
	err := resolver.ResolveGraphQLSubscription(ctx, plan, out)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(out.flushed))
	assert.Equal(t, `{"data":{"counter":0}}`, out.flushed[0])
	assert.Equal(t, `{"data":{"counter":1}}`, out.flushed[1])
	assert.Equal(t, `{"data":{"counter":2}}`, out.flushed[2])
}

func BenchmarkResolver_ResolveNode(b *testing.B) {

	resolver := New()

	serviceOneDS := FakeDataSource(`{"serviceOne":{"fieldOne":"fieldOneValue"},"anotherServiceOne":{"fieldOne":"anotherFieldOneValue"},"reusingServiceOne":{"fieldOne":"reUsingFieldOneValue"}}`)
	serviceTwoDS := FakeDataSource(`{"serviceTwo":{"fieldTwo":"fieldTwoValue"},"secondServiceTwo":{"fieldTwo":"secondFieldTwoValue"}}`)
	nestedServiceOneDS := FakeDataSource(`{"serviceOne":{"fieldOne":"fieldOneValue"}}`)

	ctx := Context{Context: context.Background(), Variables: []byte(`{"firstArg":"firstArgValue","thirdArg":123,"secondArg": true, "fourthArg": 12.34}`)}

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
			FieldSets: []FieldSet{
				{
					BufferID:  0,
					HasBuffer: true,
					Fields: []Field{
						{
							Name: []byte("serviceOne"),
							Value: &Object{
								Path: []string{"serviceOne"},
								FieldSets: []FieldSet{
									{
										Fields: []Field{
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
				},
				{
					BufferID:  1,
					HasBuffer: true,
					Fields: []Field{
						{
							Name: []byte("serviceTwo"),
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
								FieldSets: []FieldSet{
									{
										Fields: []Field{
											{
												Name: []byte("fieldTwo"),
												Value: &String{
													Path: []string{"fieldTwo"},
												},
											},
										},
									},
									{
										BufferID:  2,
										HasBuffer: true,
										Fields: []Field{
											{
												Name: []byte("serviceOneResponse"),
												Value: &Object{
													Path: []string{"serviceOne"},
													FieldSets: []FieldSet{
														{
															Fields: []Field{
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
									},
								},
							},
						},
					},
				},
				{
					BufferID:  0,
					HasBuffer: true,
					Fields: []Field{
						{
							Name: []byte("anotherServiceOne"),
							Value: &Object{
								Path: []string{"anotherServiceOne"},
								FieldSets: []FieldSet{
									{
										Fields: []Field{
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
				},
				{
					BufferID:  1,
					HasBuffer: true,
					Fields: []Field{
						{
							Name: []byte("secondServiceTwo"),
							Value: &Object{
								Path: []string{"secondServiceTwo"},
								FieldSets: []FieldSet{
									{
										Fields: []Field{
											{
												Name: []byte("fieldTwo"),
												Value: &String{
													Path: []string{"fieldTwo"},
												},
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
					Fields: []Field{
						{
							Name: []byte("reusingServiceOne"),
							Value: &Object{
								Path: []string{"reusingServiceOne"},
								FieldSets: []FieldSet{
									{
										Fields: []Field{
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

	runBench := func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(expected)))
		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				//_ = resolver.ResolveGraphQLResponse(ctx, plan, nil, ioutil.Discard)
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
	b.Run("singleflight disabled (latency 0)", func(b *testing.B) {
		serviceOneDS.artificialLatency = 0
		serviceTwoDS.artificialLatency = 0
		nestedServiceOneDS.artificialLatency = 0
		resolver.EnableSingleFlightLoader = false
		runBench(b)
	})
	b.Run("singleflight enabled (latency 5ms)", func(b *testing.B) {
		serviceOneDS.artificialLatency = time.Millisecond * 5
		serviceTwoDS.artificialLatency = 0
		nestedServiceOneDS.artificialLatency = 0
		resolver.EnableSingleFlightLoader = true
		runBench(b)
	})
	b.Run("singleflight disabled (latency 5ms)", func(b *testing.B) {
		serviceOneDS.artificialLatency = time.Millisecond * 5
		serviceTwoDS.artificialLatency = 0
		nestedServiceOneDS.artificialLatency = 0
		resolver.EnableSingleFlightLoader = false
		runBench(b)
	})
}
