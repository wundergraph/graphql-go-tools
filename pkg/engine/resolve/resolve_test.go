package resolve

import (
	"bytes"
	"context"
	"io/ioutil"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

type _fakeDataSource struct {
	data []byte
}

func (f *_fakeDataSource) Load(ctx context.Context, input []byte, pair *BufPair) (err error) {
	_, err = pair.Data.Write(f.data)
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
	testFn := func(fn func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string)) func(t *testing.T) {
		ctrl := gomock.NewController(t)
		node, ctx, expectedOutput := fn(t, ctrl)
		return func(t *testing.T) {
			r := New()
			buf := &BufPair{
				Data:   bytes.NewBuffer(nil),
				Errors: bytes.NewBuffer(nil),
			}
			err := r.resolveNode(ctx, node, nil, buf)
			assert.Equal(t, buf.Errors.String(), "", "want error buf to be empty")
			assert.NoError(t, err)
			assert.Equal(t, expectedOutput, buf.Data.String())
			ctrl.Finish()
		}
	}
	t.Run("nullable empty object", testFn(func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			nullable: true,
		}, Context{Context: context.Background()}, `null`
	}))
	t.Run("empty object", testFn(func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &EmptyObject{}, Context{Context: context.Background()}, `{}`
	}))
	t.Run("object with null field", testFn(func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
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
	t.Run("default graphql object", testFn(func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			FieldSets: []FieldSet{
				{
					Fields: []Field{
						{
							Name: []byte("data"),
							Value: &Object{
								nullable: true,
							},
						},
					},
				},
			},
		}, Context{Context: context.Background()}, `{"data":null}`
	}))
	t.Run("graphql object with simple data source", testFn(func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
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
	t.Run("fetch with context variable resolver", testFn(func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), []byte(`{"id":1}`), gomock.AssignableToTypeOf(&BufPair{})).
			Do(func(ctx context.Context, input []byte, pair *BufPair) (err error) {
				_, err = pair.Data.Write([]byte(`{"name":"Jens"}`))
				return
			}).
			Return(nil)
		return &Object{
			Fetch: &SingleFetch{
				BufferId:   0,
				DataSource: mockDataSource,
				Input:      []byte(`{"id":$$0$$}`),
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
	t.Run("resolve arrays", testFn(func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		return &Object{
			Fetch: &SingleFetch{
				BufferId:   0,
				DataSource: FakeDataSource(`{"friends":[{"id":1,"name":"Alex"},{"id":2,"name":"Patric"}]}`),
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
								nullable:            true,
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
								nullable:            true,
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
								nullable: true,
								Item:     &Object{},
							},
						},
					},
				},
			},
		}, Context{Context: context.Background()}, `{"synchronousFriends":[{"id":1,"name":"Alex"},{"id":2,"name":"Patric"}],"asynchronousFriends":[{"id":1,"name":"Alex"},{"id":2,"name":"Patric"}],"nullableFriends":null}`
	}))
	t.Run("array response from data source", testFn(func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
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
	t.Run("resolve fieldsets based on __typename", testFn(func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
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
	t.Run("resolve fieldsets asynchronous based on __typename", testFn(func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
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
	t.Run("parent object variables", testFn(func(t *testing.T, ctrl *gomock.Controller) (node Node, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.GotFormatterAdapter(gotBytesFormatter{}, matchBytes(`{"id":1}`)), gomock.AssignableToTypeOf(&BufPair{})).
			Do(func(ctx context.Context, input []byte, pair *BufPair) (err error) {
				_, err = pair.Data.Write([]byte(`{"name":"Woofie"}`))
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
									Input:      []byte(`{"id":$$0$$}`),
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
	testFn := func(fn func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string)) func(t *testing.T) {
		ctrl := gomock.NewController(t)
		node, ctx, expectedOutput := fn(t, ctrl)
		return func(t *testing.T) {
			r := New()
			buf := &bytes.Buffer{}
			err := r.ResolveGraphQLResponse(ctx, node, nil, buf)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutput, buf.String())
			ctrl.Finish()
		}
	}
	t.Run("empty graphql response", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				nullable: true,
			},
		}, Context{Context: context.Background()}, `{"data":null}`
	}))
	t.Run("fetch with simple error", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&BufPair{})).
			Do(func(ctx context.Context, input []byte, pair *BufPair) (err error) {
				err = pair.WriteErr([]byte("errorMessage"), nil, nil)
				return
			}).
			Return(nil)
		return &GraphQLResponse{
			Data: &Object{
				nullable: true,
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
									nullable: true,
								},
							},
						},
					},
				},
			},
		}, Context{Context: context.Background()}, `{"Errors":[{"message":"errorMessage"}],"data":{"name":null}}`
	}))
	t.Run("fetch with two Errors", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		mockDataSource := NewMockDataSource(ctrl)
		mockDataSource.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&BufPair{})).
			Do(func(ctx context.Context, input []byte, pair *BufPair) (err error) {
				err = pair.WriteErr([]byte("errorMessage1"), nil, nil)
				if err != nil {
					return
				}
				err = pair.WriteErr([]byte("errorMessage2"), nil, nil)
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
									nullable: true,
								},
							},
						},
					},
				},
			},
		}, Context{Context: context.Background()}, `{"Errors":[{"message":"errorMessage1"},{"message":"errorMessage2"}],"data":{"name":null}}`
	}))
	t.Run("null field should bubble up to parent with error", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				nullable: true,
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
									nullable: true,
									FieldSets: []FieldSet{
										{
											Fields: []Field{
												{
													Name: []byte("stringField"),
													Value: &String{
														nullable: false,
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
									nullable: true,
									FieldSets: []FieldSet{
										{
											Fields: []Field{
												{
													Name: []byte("integerField"),
													Value: &Integer{
														nullable: false,
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
									nullable: true,
									FieldSets: []FieldSet{
										{
											Fields: []Field{
												{
													Name: []byte("floatField"),
													Value: &Float{
														nullable: false,
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
									nullable: true,
									FieldSets: []FieldSet{
										{
											Fields: []Field{
												{
													Name: []byte("booleanField"),
													Value: &Boolean{
														nullable: false,
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
									nullable: true,
									FieldSets: []FieldSet{
										{
											Fields: []Field{
												{
													Name: []byte("objectField"),
													Value: &Object{
														nullable: false,
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
									nullable: true,
									FieldSets: []FieldSet{
										{
											Fields: []Field{
												{
													Name: []byte("arrayField"),
													Value: &Array{
														nullable: false,
														Item: &String{
															nullable: false,
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
									nullable: true,
									FieldSets: []FieldSet{
										{
											Fields: []Field{
												{
													Name: []byte("arrayField"),
													Value: &Array{
														nullable:            false,
														ResolveAsynchronous: true,
														Item: &String{
															nullable: false,
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
									nullable: true,
									Item: &String{
										nullable: false,
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
	t.Run("complex GraphQL Server plan", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		return &GraphQLResponse{
			Data: &Object{
				Fetch: &ParallelFetch{
					Fetches: []*SingleFetch{
						{
							BufferId:   0,
							Input:      []byte(`{"url":"https://service.one","body":{"query":"query($firstArg: String, $thirdArg: Int){serviceOne(serviceOneArg: $firstArg){fieldOne} anotherServiceOne(anotherServiceOneArg: $thirdArg){fieldOne} reusingServiceOne(reusingServiceOneArg: $firstArg){fieldOne}}","variables":{"thirdArg":$$1$$,"firstArg":$$0$$}}}`),
							DataSource: FakeDataSource(`{"serviceOne":{"fieldOne":"fieldOneValue"},"anotherServiceOne":{"fieldOne":"anotherFieldOneValue"},"reusingServiceOne":{"fieldOne":"reUsingFieldOneValue"}}`),
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
							BufferId:   1,
							Input:      []byte(`{"url":"https://service.two","body":{"query":"query($secondArg: Boolean, $fourthArg: Float){serviceTwo(serviceTwoArg: $secondArg){fieldTwo} secondServiceTwo(secondServiceTwoArg: $fourthArg){fieldTwo}}","variables":{"fourthArg":$$1$$,"secondArg":$$0$$}}}`),
							DataSource: FakeDataSource(`{"serviceTwo":{"fieldTwo":"fieldTwoValue"},"secondServiceTwo":{"fieldTwo":"secondFieldTwoValue"}}`),
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
										BufferId:   2,
										Input:      []byte(`{"url":"https://service.one","body":{"query":"{serviceOne {fieldOne}}"}}`),
										DataSource: FakeDataSource(`{"serviceOne":{"fieldOne":"fieldOneValue"}}`),
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

func BenchmarkResolver_ResolveNode(b *testing.B) {
	resolver := New()
	ctx := Context{
		Context: context.Background(),
	}
	plan := &GraphQLResponse{
		Data: &Object{
			Fetch: &SingleFetch{
				BufferId:   0,
				DataSource: FakeDataSource(`{"friends":[{"id":1,"name":"Alex"},{"id":2,"name":"Patric"}]}`),
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
								nullable:            true,
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
								nullable:            true,
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
								nullable: true,
								Item:     &Object{},
							},
						},
						{
							Name: []byte("nonNullableFriends"),
							Value: &Array{
								Path:     []string{"nonExistingField"},
								nullable: true,
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
					},
				},
			},
		},
	}

	var err error

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			err = resolver.ResolveGraphQLResponse(ctx, plan, nil, ioutil.Discard)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
