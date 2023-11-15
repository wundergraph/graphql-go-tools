package resolve

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
)

func TestResolvable_Resolve(t *testing.T) {
	topProducts := `{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1","name":"user-1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":8},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1","name":"user-1"}}],"stock":2},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":5}]}`
	res := NewResolvable()
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
	err = res.Resolve(object, out)
	assert.NoError(t, err)
	assert.Equal(t, `{"data":{"topProducts":[{"name":"Table","stock":8,"reviews":[{"body":"Love Table!","author":{"name":"user-1"}},{"body":"Prefer other Table.","author":{"name":"user-2"}}]},{"name":"Couch","stock":2,"reviews":[{"body":"Couch Too expensive.","author":{"name":"user-1"}}]},{"name":"Chair","stock":5,"reviews":[{"body":"Chair Could be better.","author":{"name":"user-2"}}]}]}}`, out.String())
}

func TestResolvable_ResolveWithTypeMismatch(t *testing.T) {
	topProducts := `{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1","name":true}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":8},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1","name":"user-1"}}],"stock":2},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":5}]}`
	res := NewResolvable()
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
	err = res.Resolve(object, out)
	assert.NoError(t, err)
	assert.Equal(t, `{"errors":[{"message":"String cannot represent non-string value: \"true\"","path":["topProducts",0,"reviews",0,"author","name"]}],"data":{"topProducts":[{"name":"Table","stock":8,"reviews":[{"body":"Love Table!","author":null},{"body":"Prefer other Table.","author":{"name":"user-2"}}]},{"name":"Couch","stock":2,"reviews":[{"body":"Couch Too expensive.","author":{"name":"user-1"}}]},{"name":"Chair","stock":5,"reviews":[{"body":"Chair Could be better.","author":{"name":"user-2"}}]}]}}`, out.String())
}

func TestResolvable_ResolveWithErrorBubbleUp(t *testing.T) {
	topProducts := `{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":8},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1","name":"user-1"}}],"stock":2},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":5}]}`
	res := NewResolvable()
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
	err = res.Resolve(object, out)
	assert.NoError(t, err)
	assert.Equal(t, `{"errors":[{"message":"Cannot return null for non-nullable field Query.topProducts.reviews.author.name.","path":["topProducts",0,"reviews",0,"author","name"]}],"data":{"topProducts":[{"name":"Table","stock":8,"reviews":[{"body":"Love Table!","author":null},{"body":"Prefer other Table.","author":{"name":"user-2"}}]},{"name":"Couch","stock":2,"reviews":[{"body":"Couch Too expensive.","author":{"name":"user-1"}}]},{"name":"Chair","stock":5,"reviews":[{"body":"Chair Could be better.","author":{"name":"user-2"}}]}]}}`, out.String())
}

func TestResolvable_ResolveWithErrorBubbleUpUntilData(t *testing.T) {
	topProducts := `{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1","name":"user-1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2"}}],"stock":8},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1","name":"user-1"}}],"stock":2},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":5}]}`
	res := NewResolvable()
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
	err = res.Resolve(object, out)
	assert.NoError(t, err)
	assert.Equal(t, `{"errors":[{"message":"Cannot return null for non-nullable field Query.topProducts.reviews.author.name.","path":["topProducts",0,"reviews",1,"author","name"]}],"data":null}`, out.String())
}

func BenchmarkResolvable_Resolve(b *testing.B) {
	topProducts := `{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1","name":"user-1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":8},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1","name":"user-1"}}],"stock":2},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":5}]}`
	res := NewResolvable()
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
		err = res.Resolve(object, out)
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
	res := NewResolvable()
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
	err = res.Resolve(object, out)
	assert.NoError(b, err)
	expected := []byte(`{"errors":[{"message":"Cannot return null for non-nullable field Query.topProducts.reviews.author.name.","path":["topProducts",0,"reviews",0,"author","name"]}],"data":{"topProducts":[{"name":"Table","stock":8,"reviews":[{"body":"Love Table!","author":null},{"body":"Prefer other Table.","author":{"name":"user-2"}}]},{"name":"Couch","stock":2,"reviews":[{"body":"Couch Too expensive.","author":{"name":"user-1"}}]},{"name":"Chair","stock":5,"reviews":[{"body":"Chair Could be better.","author":{"name":"user-2"}}]}]}}`)
	b.SetBytes(int64(len(expected)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out.Reset()
		err = res.Resolve(object, out)
		if err != nil {
			b.Fatal(err)
		}
		if !bytes.Equal(expected, out.Bytes()) {
			b.Fatal("not equal")
		}
	}
}

func TestResolvable_WithTracing(t *testing.T) {
	topProducts := `{"topProducts":[{"name":"Table","__typename":"Product","upc":"1","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1","name":"user-1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":8},{"name":"Couch","__typename":"Product","upc":"2","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1","name":"user-1"}}],"stock":2},{"name":"Chair","__typename":"Product","upc":"3","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2","name":"user-2"}}],"stock":5}]}`
	res := NewResolvable()
	res.enableTracing = true
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
	err = res.Resolve(object, out)

	assert.NoError(t, err)
	assert.Equal(t, `{"data":{"topProducts":[{"name":"Table","stock":8,"reviews":[{"body":"Love Table!","author":{"name":"user-1"}},{"body":"Prefer other Table.","author":{"name":"user-2"}}]},{"name":"Couch","stock":2,"reviews":[{"body":"Couch Too expensive.","author":{"name":"user-1"}}]},{"name":"Chair","stock":5,"reviews":[{"body":"Chair Could be better.","author":{"name":"user-2"}}]}]},"extensions":{"trace":{"node_type":"object","nullable":true,"fields":[{"name":"topProducts","value":{"node_type":"array","path":["topProducts"],"items":[{"node_type":"object","nullable":true,"fields":[{"name":"name","value":{"node_type":"string","path":["name"]}},{"name":"stock","value":{"node_type":"integer","path":["stock"]}},{"name":"reviews","value":{"node_type":"array","path":["reviews"],"items":[{"node_type":"object","nullable":true,"fields":[{"name":"body","value":{"node_type":"string","path":["body"]}},{"name":"author","value":{"node_type":"object","path":["author"],"fields":[{"name":"name","value":{"node_type":"string","path":["name"]}}]}}]}]}}]}]}}]}}}`, out.String())
}
