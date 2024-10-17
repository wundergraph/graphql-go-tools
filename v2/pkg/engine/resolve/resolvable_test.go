package resolve

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func TestResolvable_Resolve(t *testing.T) {
	topProducts := `{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1","name":"user-1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":8},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1","name":"user-1"}}],"stock":2},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":5}]}`
	res := NewResolvable(ResolvableOptions{})
	ctx := &Context{
		Variables: nil,
	}
	err := res.Init(ctx, []byte(topProducts), ast.OperationTypeQuery)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	object := &Object{
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
	}

	out := &bytes.Buffer{}
	err = res.Resolve(context.Background(), object, nil, out)
	assert.NoError(t, err)
	assert.Equal(t, `{"data":{"topProducts":[{"name":"Table","stock":8,"reviews":[{"body":"Love Table!","author":{"name":"user-1"}},{"body":"Prefer other Table.","author":{"name":"user-2"}}]},{"name":"Couch","stock":2,"reviews":[{"body":"Couch Too expensive.","author":{"name":"user-1"}}]},{"name":"Chair","stock":5,"reviews":[{"body":"Chair Could be better.","author":{"name":"user-2"}}]}]}}`, out.String())
}

func TestResolvable_ResolveWithTypeMismatch(t *testing.T) {
	topProducts := `{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1","name":true}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":8},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1","name":"user-1"}}],"stock":2},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":5}]}`
	res := NewResolvable(ResolvableOptions{})
	ctx := &Context{
		Variables: nil,
	}
	err := res.Init(ctx, []byte(topProducts), ast.OperationTypeQuery)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	object := &Object{
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
													Path:     []string{"author"},
													Nullable: true,
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
	}

	out := &bytes.Buffer{}
	err = res.Resolve(context.Background(), object, nil, out)
	assert.NoError(t, err)
	assert.Equal(t, `{"errors":[{"message":"String cannot represent non-string value: \"true\"","path":["topProducts",0,"reviews",0,"author","name"]}],"data":{"topProducts":[{"name":"Table","stock":8,"reviews":[{"body":"Love Table!","author":null},{"body":"Prefer other Table.","author":{"name":"user-2"}}]},{"name":"Couch","stock":2,"reviews":[{"body":"Couch Too expensive.","author":{"name":"user-1"}}]},{"name":"Chair","stock":5,"reviews":[{"body":"Chair Could be better.","author":{"name":"user-2"}}]}]}}`, out.String())
}

func TestResolvable_ResolveWithErrorBubbleUp(t *testing.T) {
	topProducts := `{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":8},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1","name":"user-1"}}],"stock":2},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":5}]}`
	res := NewResolvable(ResolvableOptions{})
	ctx := &Context{
		Variables: nil,
	}
	err := res.Init(ctx, []byte(topProducts), ast.OperationTypeQuery)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	object := &Object{
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
													Nullable: true,
													Path:     []string{"author"},
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
	}

	out := &bytes.Buffer{}
	err = res.Resolve(context.Background(), object, nil, out)
	assert.NoError(t, err)
	assert.Equal(t, `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.topProducts.reviews.author.name'.","path":["topProducts",0,"reviews",0,"author","name"]}],"data":{"topProducts":[{"name":"Table","stock":8,"reviews":[{"body":"Love Table!","author":null},{"body":"Prefer other Table.","author":{"name":"user-2"}}]},{"name":"Couch","stock":2,"reviews":[{"body":"Couch Too expensive.","author":{"name":"user-1"}}]},{"name":"Chair","stock":5,"reviews":[{"body":"Chair Could be better.","author":{"name":"user-2"}}]}]}}`, out.String())
}

func TestResolvable_ApolloCompatibilityMode_NonNullability(t *testing.T) {
	t.Run("Non-nullable root field", func(t *testing.T) {
		topProducts := `{"topProducts":null}`
		res := NewResolvable(ResolvableOptions{
			ApolloCompatibilityValueCompletionInExtensions: true,
		})
		ctx := &Context{
			Variables: nil,
		}
		err := res.Init(ctx, []byte(topProducts), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("topProducts"),
					Value: &Array{
						Path: []string{"topProducts"},
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":null,"extensions":{"valueCompletion":[{"message":"Cannot return null for non-nullable field Query.topProducts.","path":["topProducts"],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`, out.String())
	})
	t.Run("Non-Nullable root field and nested field", func(t *testing.T) {
		topProducts := `{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":8},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1","name":"user-1"}}],"stock":2},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":5}]}`
		res := NewResolvable(ResolvableOptions{
			ApolloCompatibilityValueCompletionInExtensions: true,
		})
		ctx := &Context{
			Variables: nil,
		}
		err := res.Init(ctx, []byte(topProducts), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
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
														Nullable: true,
														Path:     []string{"author"},
														Fields: []*Field{
															{
																Name: []byte("name"),
																Value: &String{
																	Path: []string{"name"},
																},
															},
														},
														TypeName: "User",
													},
												},
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

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Table","stock":8,"reviews":[{"body":"Love Table!","author":null},{"body":"Prefer other Table.","author":{"name":"user-2"}}]},{"name":"Couch","stock":2,"reviews":[{"body":"Couch Too expensive.","author":{"name":"user-1"}}]},{"name":"Chair","stock":5,"reviews":[{"body":"Chair Could be better.","author":{"name":"user-2"}}]}]},"extensions":{"valueCompletion":[{"message":"Cannot return null for non-nullable field User.name.","path":["topProducts",0,"reviews",0,"author","name"],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`, out.String())
	})
	t.Run("Nullable root field and non-Nullable nested field", func(t *testing.T) {
		topProducts := `{"topProduct":{"name":null}}`
		res := NewResolvable(ResolvableOptions{
			ApolloCompatibilityValueCompletionInExtensions: true,
		})
		ctx := &Context{
			Variables: nil,
		}
		err := res.Init(ctx, []byte(topProducts), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("topProduct"),
					Value: &Object{
						Path:     []string{"topProduct"},
						Nullable: true,
						Fields: []*Field{
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
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"topProduct":null},"extensions":{"valueCompletion":[{"message":"Cannot return null for non-nullable field Product.name.","path":["topProduct","name"],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`, out.String())

	})
	t.Run("Non-nullable array and array item", func(t *testing.T) {
		topProducts := `{"topProducts":[null]}`
		res := NewResolvable(ResolvableOptions{
			ApolloCompatibilityValueCompletionInExtensions: true,
		})
		ctx := &Context{
			Variables: nil,
		}
		err := res.Init(ctx, []byte(topProducts), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("topProducts"),
					Value: &Array{
						Path: []string{"topProducts"},
						Item: &Object{
							TypeName: "Product",
						},
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":null,"extensions":{"valueCompletion":[{"message":"Cannot return null for non-nullable array element of type Product at index 0.","path":["topProducts",0],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`, out.String())
	})
	t.Run("Nullable array and non-nullable array item", func(t *testing.T) {
		topProducts := `{"topProducts":[null]}`
		res := NewResolvable(ResolvableOptions{
			ApolloCompatibilityValueCompletionInExtensions: true,
		})
		ctx := &Context{
			Variables: nil,
		}
		err := res.Init(ctx, []byte(topProducts), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("topProducts"),
					Value: &Array{
						Nullable: true,
						Path:     []string{"topProducts"},
						Item: &Object{
							TypeName: "Product",
						},
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"topProducts":null},"extensions":{"valueCompletion":[{"message":"Cannot return null for non-nullable array element of type Product at index 0.","path":["topProducts",0],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`, out.String())
	})
	t.Run("Non-Nullable array, array item, and array item field", func(t *testing.T) {
		topProducts := `{"topProducts":[{"author":{"name":"Name"}},{"author":null}]}`
		res := NewResolvable(ResolvableOptions{
			ApolloCompatibilityValueCompletionInExtensions: true,
		})
		ctx := &Context{
			Variables: nil,
		}
		err := res.Init(ctx, []byte(topProducts), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("topProducts"),
					Value: &Array{
						Path: []string{"topProducts"},
						Item: &Object{
							Fields: []*Field{
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
										TypeName: "User",
									},
								},
							},
							TypeName: "Product",
						},
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":null,"extensions":{"valueCompletion":[{"message":"Cannot return null for non-nullable field User.author.","path":["topProducts",1,"author"],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`, out.String())
	})
}

func TestResolvable_ResolveWithErrorBubbleUpUntilData(t *testing.T) {
	topProducts := `{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1","name":"user-1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2"}}],"stock":8},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1","name":"user-1"}}],"stock":2},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":5}]}`
	res := NewResolvable(ResolvableOptions{})
	ctx := &Context{
		Variables: nil,
	}
	err := res.Init(ctx, []byte(topProducts), ast.OperationTypeQuery)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	object := &Object{
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
	}

	out := &bytes.Buffer{}
	err = res.Resolve(context.Background(), object, nil, out)
	assert.NoError(t, err)
	assert.Equal(t, `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.topProducts.reviews.author.name'.","path":["topProducts",0,"reviews",1,"author","name"]}],"data":null}`, out.String())
}

func BenchmarkResolvable_Resolve(b *testing.B) {
	topProducts := `{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1","name":"user-1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":8},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1","name":"user-1"}}],"stock":2},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":5}]}`
	res := NewResolvable(ResolvableOptions{})
	ctx := &Context{
		Variables: nil,
	}
	err := res.Init(ctx, []byte(topProducts), ast.OperationTypeQuery)
	assert.NoError(b, err)
	assert.NotNil(b, res)
	object := &Object{
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
	}

	out := &bytes.Buffer{}
	expected := []byte(`{"data":{"topProducts":[{"name":"Table","stock":8,"reviews":[{"body":"Love Table!","author":{"name":"user-1"}},{"body":"Prefer other Table.","author":{"name":"user-2"}}]},{"name":"Couch","stock":2,"reviews":[{"body":"Couch Too expensive.","author":{"name":"user-1"}}]},{"name":"Chair","stock":5,"reviews":[{"body":"Chair Could be better.","author":{"name":"user-2"}}]}]}}`)
	b.SetBytes(int64(len(expected)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		if err != nil {
			b.Fatal(err)
		}
		if !bytes.Equal(expected, out.Bytes()) {
			b.Fatal("not equal")
		}
	}
}

func BenchmarkResolvable_ResolveWithErrorBubbleUp(b *testing.B) {
	topProducts := `{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":8},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1","name":"user-1"}}],"stock":2},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":5}]}`
	res := NewResolvable(ResolvableOptions{})
	ctx := &Context{
		Variables: nil,
	}
	err := res.Init(ctx, []byte(topProducts), ast.OperationTypeQuery)
	assert.NoError(b, err)
	assert.NotNil(b, res)
	object := &Object{
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
													Nullable: true,
													Path:     []string{"author"},
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
	}

	out := &bytes.Buffer{}
	err = res.Resolve(context.Background(), object, nil, out)
	assert.NoError(b, err)
	expected := []byte(`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.topProducts.reviews.author.name'.","path":["topProducts",0,"reviews",0,"author","name"]}],"data":{"topProducts":[{"name":"Table","stock":8,"reviews":[{"body":"Love Table!","author":null},{"body":"Prefer other Table.","author":{"name":"user-2"}}]},{"name":"Couch","stock":2,"reviews":[{"body":"Couch Too expensive.","author":{"name":"user-1"}}]},{"name":"Chair","stock":5,"reviews":[{"body":"Chair Could be better.","author":{"name":"user-2"}}]}]}}`)
	b.SetBytes(int64(len(expected)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		if err != nil {
			b.Fatal(err)
		}
		if !bytes.Equal(expected, out.Bytes()) {
			b.Fatal("not equal")
		}
	}
}

func TestResolvable_WithTracingNotStarted(t *testing.T) {
	res := NewResolvable(ResolvableOptions{})
	// Do not start a trace with SetTraceStart(), but request it to be output
	ctx := NewContext(context.Background())
	ctx.TracingOptions.Enable = true
	ctx.TracingOptions.IncludeTraceOutputInResponseExtensions = true
	err := res.Init(ctx, []byte(`{"hello": "world"}`), ast.OperationTypeQuery)
	assert.NoError(t, err)
	object := &Object{
		Fields: []*Field{
			{
				Name: []byte("hello"),
				Value: &String{
					Path: []string{"hello"},
				},
			},
		},
	}
	out := &bytes.Buffer{}
	fetchTree := Sequence()
	err = res.Resolve(ctx.ctx, object, fetchTree, out)

	assert.NoError(t, err)
	assert.Equal(t, `{"data":{"hello":"world"},"extensions":{"trace":{"version":"1","info":null,"fetches":{"kind":"Sequence"}}}}`, out.String())
}

func TestResolveFloat(t *testing.T) {
	t.Run("default behaviour", func(t *testing.T) {
		res := NewResolvable(ResolvableOptions{})
		ctx := NewContext(context.Background())
		err := res.Init(ctx, []byte(`{"f":1.0}`), ast.OperationTypeQuery)
		assert.NoError(t, err)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("f"),
					Value: &Float{
						Path: []string{"f"},
					},
				},
			},
		}
		out := &bytes.Buffer{}
		fetchTree := Sequence()
		err = res.Resolve(ctx.ctx, object, fetchTree, out)

		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"f":1.0}}`, out.String())
	})
	t.Run("invalid float", func(t *testing.T) {
		res := NewResolvable(ResolvableOptions{})
		ctx := NewContext(context.Background())
		err := res.Init(ctx, []byte(`{"f":false}`), ast.OperationTypeQuery)
		assert.NoError(t, err)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("f"),
					Value: &Float{
						Path: []string{"f"},
					},
				},
			},
		}
		out := &bytes.Buffer{}
		fetchTree := Sequence()
		err = res.Resolve(ctx.ctx, object, fetchTree, out)

		assert.NoError(t, err)
		assert.Equal(t, `{"errors":[{"message":"Float cannot represent non-float value: \"false\"","path":["f"]}],"data":null}`, out.String())
	})
	t.Run("truncate float", func(t *testing.T) {
		res := NewResolvable(ResolvableOptions{
			ApolloCompatibilityTruncateFloatValues: true,
		})
		ctx := NewContext(context.Background())
		err := res.Init(ctx, []byte(`{"f":1.0}`), ast.OperationTypeQuery)
		assert.NoError(t, err)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("f"),
					Value: &Float{
						Path: []string{"f"},
					},
				},
			},
		}
		out := &bytes.Buffer{}
		fetchTree := Sequence()
		err = res.Resolve(ctx.ctx, object, fetchTree, out)

		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"f":1}}`, out.String())
	})
	t.Run("truncate float with decimal place", func(t *testing.T) {
		res := NewResolvable(ResolvableOptions{
			ApolloCompatibilityTruncateFloatValues: true,
		})
		ctx := NewContext(context.Background())
		err := res.Init(ctx, []byte(`{"f":1.1}`), ast.OperationTypeQuery)
		assert.NoError(t, err)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("f"),
					Value: &Float{
						Path: []string{"f"},
					},
				},
			},
		}
		out := &bytes.Buffer{}
		fetchTree := Sequence()
		err = res.Resolve(ctx.ctx, object, fetchTree, out)

		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"f":1.1}}`, out.String())
	})
}

func TestResolvable_ValueCompletion(t *testing.T) {
	t.Run("nested object", func(t *testing.T) {
		res := NewResolvable(ResolvableOptions{
			ApolloCompatibilityValueCompletionInExtensions: true,
		})
		ctx := NewContext(context.Background())
		err := res.Init(ctx, []byte(`{"object":{"hello":"world","__typename":"NotHello"}}`), ast.OperationTypeQuery)
		assert.NoError(t, err)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("object"),
					Value: &Object{
						Nullable:      true,
						Path:          []string{"object"},
						TypeName:      "Hello",
						PossibleTypes: map[string]struct{}{"Hello": {}},
						SourceName:    "World",
						Fields: []*Field{
							{
								Name: []byte("hello"),
								Value: &String{
									Path: []string{"hello"},
								},
							},
						},
					},
				},
			},
		}
		out := &bytes.Buffer{}
		fetchTree := Sequence()
		err = res.Resolve(ctx.ctx, object, fetchTree, out)

		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"object":null},"extensions":{"valueCompletion":[{"message":"Invalid __typename found for object at field Query.object.","path":["object"],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`, out.String())

		res.Reset(1024)
		err = res.Init(ctx, []byte(`{"object":{"hello":"world","__typename":"Hello"}}`), ast.OperationTypeQuery)
		assert.NoError(t, err)
		out.Reset()
		err = res.Resolve(ctx.ctx, object, fetchTree, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"object":{"hello":"world"}}}`, out.String())

		res.Reset(1024)
		err = res.Init(ctx, []byte(`{"object":{"hello":"world","__typename":"NotEvenATinyBitHello"}}`), ast.OperationTypeQuery)
		assert.NoError(t, err)
		out.Reset()
		err = res.Resolve(ctx.ctx, object, fetchTree, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"object":null},"extensions":{"valueCompletion":[{"message":"Invalid __typename found for object at field Query.object.","path":["object"],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`, out.String())
	})

	t.Run("complex", func(t *testing.T) {
		respJson := []byte(`{
            "entity": {
                "__typename": "Invalid",
                "id": "book1",
                "name": "Entity1"
            },
            "entities": [
                {
                    "__typename": "Invalid",
                    "id": "book2",
                    "name": "Entity1"
                },
                {
                    "__typename": "Invalid",
                    "id": "book3",
                    "name": "Entity1"
                }
            ],
            "entitiesUnion": [
                {
                    "__typename": "Invalid",
                    "id": "book4",
                    "name": "Entity1"
                },
                {
                    "__typename": "Invalid",
                    "id": "book5",
                    "name": "Entity1"
                }
            ],
            "entitiesInterface": [
                {
                    "__typename": "Invalid",
                    "id": "book6",
                    "name": "Entity1"
                },
                {
                    "__typename": "Invalid",
                    "id": "book7",
                    "name": "Entity1"
                }
            ]
        }`)

		t.Run("nullable", func(t *testing.T) {
			res := NewResolvable(ResolvableOptions{
				ApolloCompatibilityValueCompletionInExtensions: true,
			})
			ctx := NewContext(context.Background())
			err := res.Init(ctx, respJson, ast.OperationTypeQuery)
			assert.NoError(t, err)

			object := &Object{
				Fields: []*Field{
					{
						Name: []byte("entity"),
						Value: &Object{
							Nullable:      true,
							Path:          []string{"entity"},
							TypeName:      "Entity",
							PossibleTypes: map[string]struct{}{"Entity": {}},
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
					{
						Name: []byte("entities"),
						Value: &Array{
							Nullable: true,
							Path:     []string{"entities"},
							Item: &Object{
								Nullable:      true,
								TypeName:      "Entity",
								PossibleTypes: map[string]struct{}{"Entity": {}},
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
					{
						Name: []byte("entitiesUnion"),
						Value: &Array{
							Nullable: true,
							Path:     []string{"entities"},
							Item: &Object{
								Nullable:      true,
								TypeName:      "Union",
								PossibleTypes: map[string]struct{}{"Entity": {}},
								Fields: []*Field{
									{
										Name: []byte("name"),
										Value: &String{
											Path: []string{"name"},
										},
										OnTypeNames: [][]byte{[]byte("Entity")},
									},
								},
							},
						},
					},
					{
						Name: []byte("entitiesInterface"),
						Value: &Array{
							Nullable: true,
							Path:     []string{"entitiesInterface"},
							Item: &Object{
								Nullable:      true,
								TypeName:      "Interface",
								PossibleTypes: map[string]struct{}{"Entity": {}},
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
			}
			out := &bytes.Buffer{}
			fetchTree := Sequence()
			err = res.Resolve(ctx.ctx, object, fetchTree, out)

			assert.NoError(t, err)
			assert.Equal(t, `{"data":{"entity":null,"entities":[null,null],"entitiesUnion":[null,null],"entitiesInterface":[null,null]},"extensions":{"valueCompletion":[{"message":"Invalid __typename found for object at field Query.entity.","path":["entity"],"extensions":{"code":"INVALID_GRAPHQL"}},{"message":"Invalid __typename found for object at array element of type Entity at index 0.","path":["entities",0],"extensions":{"code":"INVALID_GRAPHQL"}},{"message":"Invalid __typename found for object at array element of type Entity at index 1.","path":["entities",1],"extensions":{"code":"INVALID_GRAPHQL"}},{"message":"Invalid __typename found for object at array element of type Union at index 0.","path":["entities",0],"extensions":{"code":"INVALID_GRAPHQL"}},{"message":"Invalid __typename found for object at array element of type Union at index 1.","path":["entities",1],"extensions":{"code":"INVALID_GRAPHQL"}},{"message":"Invalid __typename found for object at array element of type Interface at index 0.","path":["entitiesInterface",0],"extensions":{"code":"INVALID_GRAPHQL"}},{"message":"Invalid __typename found for object at array element of type Interface at index 1.","path":["entitiesInterface",1],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`, out.String())
		})

		t.Run("mixed nullability", func(t *testing.T) {
			res := NewResolvable(ResolvableOptions{
				ApolloCompatibilityValueCompletionInExtensions: true,
			})
			ctx := NewContext(context.Background())
			err := res.Init(ctx, respJson, ast.OperationTypeQuery)
			assert.NoError(t, err)

			object := &Object{
				Fields: []*Field{
					{
						Name: []byte("entity"),
						Value: &Object{
							Nullable:      true,
							Path:          []string{"entity"},
							TypeName:      "Entity",
							PossibleTypes: map[string]struct{}{"Entity": {}},
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
					{
						Name: []byte("entities"),
						Value: &Array{
							Nullable: false,
							Path:     []string{"entities"},
							Item: &Object{
								Nullable:      false,
								TypeName:      "Entity",
								PossibleTypes: map[string]struct{}{"Entity": {}},
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
					{
						Name: []byte("entitiesUnion"),
						Value: &Array{
							Nullable: true,
							Path:     []string{"entities"},
							Item: &Object{
								Nullable:      true,
								TypeName:      "Union",
								PossibleTypes: map[string]struct{}{"Entity": {}},
								Fields: []*Field{
									{
										Name: []byte("name"),
										Value: &String{
											Path: []string{"name"},
										},
										OnTypeNames: [][]byte{[]byte("Entity")},
									},
								},
							},
						},
					},
					{
						Name: []byte("entitiesInterface"),
						Value: &Array{
							Nullable: true,
							Path:     []string{"entitiesInterface"},
							Item: &Object{
								Nullable:      true,
								TypeName:      "Interface",
								PossibleTypes: map[string]struct{}{"Entity": {}},
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
			}
			out := &bytes.Buffer{}
			fetchTree := Sequence()
			err = res.Resolve(ctx.ctx, object, fetchTree, out)

			assert.NoError(t, err)
			assert.Equal(t, `{"data":null,"extensions":{"valueCompletion":[{"message":"Invalid __typename found for object at field Query.entity.","path":["entity"],"extensions":{"code":"INVALID_GRAPHQL"}},{"message":"Invalid __typename found for object at array element of type Entity at index 0.","path":["entities",0],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`, out.String())
		})
	})
}

func TestResolvable_WithTracing(t *testing.T) {
	topProducts := `{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1","name":"user-1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":8},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1","name":"user-1"}}],"stock":2},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":5}]}`
	res := NewResolvable(ResolvableOptions{})
	background := SetTraceStart(context.Background(), true)
	ctx := NewContext(background)
	ctx.TracingOptions.Enable = true
	ctx.TracingOptions.EnablePredictableDebugTimings = true
	ctx.TracingOptions.IncludeTraceOutputInResponseExtensions = true
	err := res.Init(ctx, []byte(topProducts), ast.OperationTypeQuery)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	object := &Object{
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
	}

	SetParseStats(ctx.ctx, PhaseStats{})
	SetNormalizeStats(ctx.ctx, PhaseStats{})
	SetValidateStats(ctx.ctx, PhaseStats{})
	SetPlannerStats(ctx.ctx, PhaseStats{})

	out := &bytes.Buffer{}
	fetchTree := Sequence()
	err = res.Resolve(ctx.ctx, object, fetchTree, out)

	assert.NoError(t, err)
	assert.Equal(t, `{"data":{"topProducts":[{"name":"Table","stock":8,"reviews":[{"body":"Love Table!","author":{"name":"user-1"}},{"body":"Prefer other Table.","author":{"name":"user-2"}}]},{"name":"Couch","stock":2,"reviews":[{"body":"Couch Too expensive.","author":{"name":"user-1"}}]},{"name":"Chair","stock":5,"reviews":[{"body":"Chair Could be better.","author":{"name":"user-2"}}]}]},"extensions":{"trace":{"version":"1","info":{"trace_start_time":"","trace_start_unix":0,"parse_stats":{"duration_nanoseconds":5,"duration_pretty":"5ns","duration_since_start_nanoseconds":5,"duration_since_start_pretty":"5ns"},"normalize_stats":{"duration_nanoseconds":5,"duration_pretty":"5ns","duration_since_start_nanoseconds":10,"duration_since_start_pretty":"10ns"},"validate_stats":{"duration_nanoseconds":5,"duration_pretty":"5ns","duration_since_start_nanoseconds":15,"duration_since_start_pretty":"15ns"},"planner_stats":{"duration_nanoseconds":5,"duration_pretty":"5ns","duration_since_start_nanoseconds":20,"duration_since_start_pretty":"20ns"}},"fetches":{"kind":"Sequence"}}}}`, out.String())
}
