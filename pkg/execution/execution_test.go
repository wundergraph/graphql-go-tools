package execution

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/cespare/xxhash"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"testing"
)

func TestExecution(t *testing.T) {

	exampleContext := Context{
		Variables: map[uint64][]byte{
			xxhash.Sum64String("name"): []byte("User"),
			xxhash.Sum64String("id"):   []byte("1"),
		},
	}

	object := &Object{
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fields: []Field{
						{
							Name: []byte("__type"),
							Resolve: &Resolve{
								Args: []Argument{
									&ContextVariableArgument{
										Name:         []byte("name"),
										VariableName: []byte("name"),
									},
								},
								Resolver: &TypeResolver{},
							},
							Value: &Object{
								Path: []string{"__type"},
								Fields: []Field{
									{
										Name: []byte("name"),
										Value: &Value{
											Path: []string{"name"},
										},
									},
									{
										Name: []byte("fields"),
										Value: &List{
											Path: []string{"fields"},
											Value: &Object{
												Fields: []Field{
													{
														Name: []byte("name"),
														Value: &Value{
															Path: []string{"name"},
														},
													},
													{
														Name: []byte("type"),
														Value: &Object{
															Path: []string{"type"},
															Fields: []Field{
																{
																	Name: []byte("name"),
																	Value: &Value{
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
						{
							Name: []byte("user"),
							Resolve: &Resolve{
								Args: []Argument{
									&StaticVariableArgument{
										Name:  literal.QUERY,
										Value: []byte("query q1($id: String!){user{id name birthday}}"),
									},
									&ContextVariableArgument{
										Name:         []byte("id"),
										VariableName: []byte("id"),
									},
								},
								Resolver: &GraphQLResolver{
									Upstream: "localhost:8001",
									URL:      "/graphql",
								},
							},
							Value: &Object{
								Path: []string{"data", "user"},
								Fields: []Field{
									{
										Name: []byte("id"),
										Value: &Value{
											Path: []string{"id"},
										},
									},
									{
										Name: []byte("name"),
										Value: &Value{
											Path: []string{"name"},
										},
									},
									{
										Name: []byte("birthday"),
										Value: &Value{
											Path: []string{"birthday"},
										},
									},
									{
										Name: []byte("friends"),
										Resolve: &Resolve{
											Args: []Argument{
												&StaticVariableArgument{
													Name:  literal.URL,
													Value: []byte("/user/:id/friends"),
												},
												&ObjectVariableArgument{
													Name: []byte("id"),
													Path: []string{"id"},
												},
											},
											Resolver: &RESTResolver{},
										},
										Value: &List{
											Value: &Object{
												Fields: []Field{
													{
														Name: []byte("id"),
														Value: &Value{
															Path: []string{"id"},
														},
													},
													{
														Name: []byte("name"),
														Value: &Value{
															Path: []string{"name"},
														},
													},
													{
														Name: []byte("birthday"),
														Value: &Value{
															Path: []string{"birthday"},
														},
													},
												},
											},
										},
									},
									{
										Name: []byte("pets"),
										Resolve: &Resolve{
											Args: []Argument{
												&StaticVariableArgument{
													Name: literal.QUERY,
													Value: []byte(`query q1($id: String!){userPets(id: $id){	__typename name nickname... on Dog {woof} ... on Cat {meow}}}`),
												},
												&ObjectVariableArgument{
													Name: []byte("id"),
													Path: []string{"id"},
												},
											},
											Resolver: &GraphQLResolver{
												Upstream: "localhost:8002",
												URL:      "/graphql",
											},
										},
										Value: &List{
											Path: []string{"data", "userPets"},
											Value: &Object{
												Fields: []Field{
													{
														Name: []byte("__typename"),
														Value: &Value{
															Path: []string{"__typename"},
														},
													},
													{
														Name: []byte("name"),
														Value: &Value{
															Path: []string{"name"},
														},
													},
													{
														Name: []byte("nickname"),
														Value: &Value{
															Path: []string{"nickname"},
														},
													},
													{
														Name: []byte("woof"),
														Value: &Value{
															Path: []string{"woof"},
														},
														Skip: &IfNotEqual{
															Left: &ObjectVariableArgument{
																Path: []string{"__typename"},
															},
															Right: &StaticVariableArgument{
																Value: []byte("Dog"),
															},
														},
													},
													{
														Name: []byte("meow"),
														Value: &Value{
															Path: []string{"meow"},
														},
														Skip: &IfNotEqual{
															Left: &ObjectVariableArgument{
																Path: []string{"__typename"},
															},
															Right: &StaticVariableArgument{
																Value: []byte("Cat"),
															},
														},
													},
												},
											},
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

	out := bytes.Buffer{}
	ex := Executor{}
	err := ex.Execute(exampleContext, object, &out)
	if err != nil {
		t.Fatal(err)
	}

	data := map[string]interface{}{}
	err = json.Unmarshal(out.Bytes(), &data)
	if err != nil {
		fmt.Println(out.String())
		t.Fatal(err)
	}

	pretty, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("Result:\nUgly:\n%s\nPretty:\n%s\n", out.String(), string(pretty))
}

func BenchmarkExecution(b *testing.B) {

	exampleContext := Context{
		Variables: map[uint64][]byte{
			xxhash.Sum64String("name"): []byte("User"),
			xxhash.Sum64String("id"):   []byte("1"),
		},
	}

	out := bytes.Buffer{}
	ex := Executor{}

	sizes := []int{1, 5, 10, 20, 50, 100}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size:%d", size), func(b *testing.B) {
			fields := make([]Field, 0, size)
			for i := 0; i < size; i++ {
				fields = append(fields, genField())
			}
			object := &Object{
				Fields: fields,
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				out.Reset()
				err := ex.Execute(exampleContext, object, &out)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func genField() Field {
	return Field{
		Name: []byte("data"),
		Value: &Object{
			Fields: []Field{
				{
					Name: []byte("__type"),
					Resolve: &Resolve{
						Args: []Argument{
							&ContextVariableArgument{
								Name:         []byte("name"),
								VariableName: []byte("name"),
							},
						},
						Resolver: &TypeResolver{},
					},
					Value: &Object{
						Path: []string{"__type"},
						Fields: []Field{
							{
								Name: []byte("name"),
								Value: &Value{
									Path: []string{"name"},
								},
							},
							{
								Name: []byte("fields"),
								Value: &List{
									Path: []string{"fields"},
									Value: &Object{
										Fields: []Field{
											{
												Name: []byte("name"),
												Value: &Value{
													Path: []string{"name"},
												},
											},
											{
												Name: []byte("type"),
												Value: &Object{
													Path: []string{"type"},
													Fields: []Field{
														{
															Name: []byte("name"),
															Value: &Value{
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
				{
					Name: []byte("user"),
					Resolve: &Resolve{
						Args: []Argument{
							&StaticVariableArgument{
								Name:  literal.QUERY,
								Value: []byte("query q1($id: String!){user{id name birthday}}"),
							},
							&ContextVariableArgument{
								Name:         []byte("id"),
								VariableName: []byte("id"),
							},
						},
						Resolver: &GraphQLResolver{
							Upstream: "localhost:8001",
							URL:      "/graphql",
						},
					},
					Value: &Object{
						Path: []string{"data", "user"},
						Fields: []Field{
							{
								Name: []byte("id"),
								Value: &Value{
									Path: []string{"id"},
								},
							},
							{
								Name: []byte("name"),
								Value: &Value{
									Path: []string{"name"},
								},
							},
							{
								Name: []byte("birthday"),
								Value: &Value{
									Path: []string{"birthday"},
								},
							},
							{
								Name: []byte("friends"),
								Resolve: &Resolve{
									Args: []Argument{
										&StaticVariableArgument{
											Name:  literal.URL,
											Value: []byte("/user/:id/friends"),
										},
										&ObjectVariableArgument{
											Name: []byte("id"),
											Path: []string{"id"},
										},
									},
									Resolver: &RESTResolver{},
								},
								Value: &List{
									Value: &Object{
										Fields: []Field{
											{
												Name: []byte("id"),
												Value: &Value{
													Path: []string{"id"},
												},
											},
											{
												Name: []byte("name"),
												Value: &Value{
													Path: []string{"name"},
												},
											},
											{
												Name: []byte("birthday"),
												Value: &Value{
													Path: []string{"birthday"},
												},
											},
										},
									},
								},
							},
							{
								Name: []byte("pets"),
								Resolve: &Resolve{
									Args: []Argument{
										&StaticVariableArgument{
											Name: literal.QUERY,
											Value: []byte(`query q1($id: String!){userPets(id: $id){	__typename name nickname... on Dog {woof} ... on Cat {meow}}}`),
										},
										&ObjectVariableArgument{
											Name: []byte("id"),
											Path: []string{"id"},
										},
									},
									Resolver: &GraphQLResolver{
										Upstream: "localhost:8002",
										URL:      "/graphql",
									},
								},
								Value: &List{
									Path: []string{"data", "userPets"},
									Value: &Object{
										Fields: []Field{
											{
												Name: []byte("__typename"),
												Value: &Value{
													Path: []string{"__typename"},
												},
											},
											{
												Name: []byte("name"),
												Value: &Value{
													Path: []string{"name"},
												},
											},
											{
												Name: []byte("nickname"),
												Value: &Value{
													Path: []string{"nickname"},
												},
											},
											{
												Name: []byte("woof"),
												Value: &Value{
													Path: []string{"woof"},
												},
												Skip: &IfNotEqual{
													Left: &ObjectVariableArgument{
														Path: []string{"__typename"},
													},
													Right: &StaticVariableArgument{
														Value: []byte("Dog"),
													},
												},
											},
											{
												Name: []byte("meow"),
												Value: &Value{
													Path: []string{"meow"},
												},
												Skip: &IfNotEqual{
													Left: &ObjectVariableArgument{
														Path: []string{"__typename"},
													},
													Right: &StaticVariableArgument{
														Value: []byte("Cat"),
													},
												},
											},
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
