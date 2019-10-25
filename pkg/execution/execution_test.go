package execution

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/cespare/xxhash"
	"github.com/jensneuse/diffview"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/sebdah/goldie"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"testing"
)

func dumpRequest(t *testing.T, r *http.Request, name string) {
	dump, err := httputil.DumpRequest(r, true)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%s dump: \n%s\n", name, string(dump))
}

func TestExecution(t *testing.T) {
	exampleContext := Context{
		Variables: map[uint64][]byte{
			xxhash.Sum64String("name"): []byte("User"),
			xxhash.Sum64String("id"):   []byte("1"),
		},
	}

	graphQL1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//dumpRequest(t, r, "graphQL1")

		_, err := w.Write(userData)
		if err != nil {
			t.Fatal(err)
		}
	}))

	graphQL2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//dumpRequest(t, r, "graphQL2")

		_, err := w.Write(petsData)
		if err != nil {
			t.Fatal(err)
		}
	}))

	REST1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//dumpRequest(t, r, "rest1")

		_, err := w.Write(friendsData)
		if err != nil {
			t.Fatal(err)
		}
	}))

	defer graphQL1.Close()
	defer graphQL2.Close()
	defer REST1.Close()

	object := &Object{
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fields: []Field{
						{
							Name: []byte("__type"),
							Resolve: &DataSourceInvocation{
								Args: []Argument{
									&ContextVariableArgument{
										Name:         []byte("name"),
										VariableName: []byte("name"),
									},
								},
								DataSource: &TypeDataSource{},
							},
							Value: &Object{
								Path: []string{"__type"},
								Fields: []Field{
									{
										Name: []byte("name"),
										Value: &Value{
											Path:       []string{"name"},
											QuoteValue: true,
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
															Path:       []string{"name"},
															QuoteValue: true,
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
																		Path:       []string{"name"},
																		QuoteValue: true,
																	},
																},
															},
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
							Resolve: &DataSourceInvocation{
								Args: []Argument{
									&StaticVariableArgument{
										Name:  literal.HOST,
										Value: []byte(graphQL1.URL),
									},
									&StaticVariableArgument{
										Name:  literal.URL,
										Value: []byte("/graphql"),
									},
									&StaticVariableArgument{
										Name:  literal.QUERY,
										Value: []byte("query q1($id: String!){user{id name birthday}}"),
									},
									&ContextVariableArgument{
										Name:         []byte("id"),
										VariableName: []byte("id"),
									},
								},
								DataSource: &GraphQLDataSource{},
							},
							Value: &Object{
								Path: []string{"user"},
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
											Path:       []string{"name"},
											QuoteValue: true,
										},
									},
									{
										Name: []byte("birthday"),
										Value: &Value{
											Path:       []string{"birthday"},
											QuoteValue: true,
										},
									},
									{
										Name: []byte("friends"),
										Resolve: &DataSourceInvocation{
											Args: []Argument{
												&StaticVariableArgument{
													Name:  literal.HOST,
													Value: []byte(REST1.URL),
												},
												&StaticVariableArgument{
													Name:  literal.URL,
													Value: []byte("/user/{{ .id }}/friends"),
												},
												&ObjectVariableArgument{
													Name: []byte("id"),
													Path: []string{"id"},
												},
											},
											DataSource: &HttpJsonDataSource{},
										},
										Value: &List{
											Value: &Object{
												Fields: []Field{
													{
														Name: []byte("id"),
														Value: &Value{
															Path:       []string{"id"},
															QuoteValue: false,
														},
													},
													{
														Name: []byte("name"),
														Value: &Value{
															Path:       []string{"name"},
															QuoteValue: true,
														},
													},
													{
														Name: []byte("birthday"),
														Value: &Value{
															Path:       []string{"birthday"},
															QuoteValue: true,
														},
													},
												},
											},
										},
									},
									{
										Name: []byte("pets"),
										Resolve: &DataSourceInvocation{
											Args: []Argument{
												&StaticVariableArgument{
													Name:  literal.HOST,
													Value: []byte(graphQL2.URL),
												},
												&StaticVariableArgument{
													Name:  literal.URL,
													Value: []byte("/graphql"),
												},
												&StaticVariableArgument{
													Name: literal.QUERY,
													Value: []byte(`query q1($id: String!){userPets(id: $id){	__typename name nickname... on Dog {woof} ... on Cat {meow}}}`),
												},
												&ObjectVariableArgument{
													Name: []byte("id"),
													Path: []string{"id"},
												},
											},
											DataSource: &GraphQLDataSource{},
										},
										Value: &List{
											Path: []string{"userPets"},
											Value: &Object{
												Fields: []Field{
													{
														Name: []byte("__typename"),
														Value: &Value{
															Path:       []string{"__typename"},
															QuoteValue: true,
														},
													},
													{
														Name: []byte("name"),
														Value: &Value{
															Path:       []string{"name"},
															QuoteValue: true,
														},
													},
													{
														Name: []byte("nickname"),
														Value: &Value{
															Path:       []string{"nickname"},
															QuoteValue: true,
														},
													},
													{
														Name: []byte("woof"),
														Value: &Value{
															Path:       []string{"woof"},
															QuoteValue: true,
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
															Path:       []string{"meow"},
															QuoteValue: true,
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

	goldie.Assert(t, "execution", pretty)
	if t.Failed() {

		fixture, err := ioutil.ReadFile("./fixtures/execution.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("execution", fixture, pretty)
	}
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

type FakeDataSource struct {
	data []byte
}

func (f FakeDataSource) Resolve(ctx Context, args ResolvedArgs) []byte {
	return f.data
}

func genField() Field {
	return Field{
		Name: []byte("data"),
		Value: &Object{
			Fields: []Field{
				{
					Name: []byte("__type"),
					Resolve: &DataSourceInvocation{
						Args: []Argument{
							&ContextVariableArgument{
								Name:         []byte("name"),
								VariableName: []byte("name"),
							},
						},
						DataSource: &TypeDataSource{},
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
					Resolve: &DataSourceInvocation{
						Args: []Argument{
							&StaticVariableArgument{
								Name:  literal.HOST,
								Value: []byte("localhost:8001"),
							},
							&StaticVariableArgument{
								Name:  literal.URL,
								Value: []byte("/graphql"),
							},
							&StaticVariableArgument{
								Name:  literal.QUERY,
								Value: []byte("query q1($id: String!){user{id name birthday}}"),
							},
							&ContextVariableArgument{
								Name:         []byte("id"),
								VariableName: []byte("id"),
							},
						},
						DataSource: &FakeDataSource{
							data: userData,
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
									Path:       []string{"name"},
									QuoteValue: true,
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
								Resolve: &DataSourceInvocation{
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
									DataSource: &FakeDataSource{
										friendsData,
									},
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
													Path:       []string{"name"},
													QuoteValue: true,
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
								Resolve: &DataSourceInvocation{
									Args: []Argument{
										&StaticVariableArgument{
											Name:  literal.HOST,
											Value: []byte("localhost:8002"),
										},
										&StaticVariableArgument{
											Name:  literal.URL,
											Value: []byte("/graphql"),
										},
										&StaticVariableArgument{
											Name: literal.QUERY,
											Value: []byte(`query q1($id: String!){userPets(id: $id){	__typename name nickname... on Dog {woof} ... on Cat {meow}}}`),
										},
										&ObjectVariableArgument{
											Name: []byte("id"),
											Path: []string{"id"},
										},
									},
									DataSource: &FakeDataSource{
										data: petsData,
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
													Path:       []string{"woof"},
													QuoteValue: true,
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
													Path:       []string{"meow"},
													QuoteValue: true,
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

var userType = []byte(`{
			  "__type": {
				"name": "User",
				"fields": [
				  {
					"name": "id",
					"type": { "name": "String" }
				  },
				  {
					"name": "name",
					"type": { "name": "String" }
				  },
				  {
					"name": "birthday",
					"type": { "name": "Date" }
				  }
				]
			  }
			}`)

var userData = []byte(`
		{
			"data":	{
				"user":	{
					"id":1,
					"name":"Jens",
					"birthday":"08.02.1988"
				}
			}
		}`)

var userRestData = []byte(`
{
	"id":1,
	"name":"Jens",
	"birthday":"08.02.1988"
}`)

var friendsData = []byte(`[
   {
      "id":2,
      "name":"Yaara",
      "birthday":"1990 I guess? ;-)"
   },
   {
      "id":3,
      "name":"Ahmet",
      "birthday":"1980"
   }]`)

var petsData = []byte(`{
   "data":{
      "userPets":[{
            "__typename":"Dog",
            "name":"Paw",
            "nickname":"Pawie",
            "woof":"Woof! Woof!"
         },
         {
            "__typename":"Cat",
            "name":"Mietz",
            "nickname":"Mietzie",
            "meow":"Meow meow!"
         }]}
}`)
