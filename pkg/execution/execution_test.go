package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/cespare/xxhash"
	"github.com/jensneuse/diffview"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/introspection"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"github.com/sebdah/goldie"
	"go.uber.org/zap"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"testing"
	"time"
)

// nolint
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

	REST2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//dumpRequest(t, r, "rest1")

		var data []byte

		switch r.RequestURI {
		case "/friends/3/pets":
			data = ahmetsPets
		case "/friends/2/pets":
			data = yaarasPets
		default:
			panic(fmt.Errorf("unexpected URI: %s", r.RequestURI))
		}

		_, err := w.Write(data)
		if err != nil {
			t.Fatal(err)
		}
	}))

	defer graphQL1.Close()
	defer graphQL2.Close()
	defer REST1.Close()
	defer REST2.Close()

	object := &Object{
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fetch: &ParallelFetch{
						Fetches: []Fetch{
							&SingleFetch{
								Source: &DataSourceInvocation{
									Args: []Argument{
										&ContextVariableArgument{
											Name:         []byte("name"),
											VariableName: []byte("name"),
										},
									},
									DataSource: &TypeDataSource{},
								},
								BufferName: "__type",
							},
							&SingleFetch{
								Source: &DataSourceInvocation{
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
									DataSource: &GraphQLDataSource{
										log: zap.NewNop(),
									},
								},
								BufferName: "user",
							},
						},
					},
					Fields: []Field{
						{
							Name:        []byte("__type"),
							HasResolver: true,
							Value: &Object{
								PathSelector: PathSelector{
									Path: "__type",
								},
								Fields: []Field{
									{
										Name: []byte("name"),
										Value: &Value{
											PathSelector: PathSelector{
												Path: "name",
											},
											QuoteValue: true,
										},
									},
									{
										Name: []byte("fields"),
										Value: &List{
											PathSelector: PathSelector{
												Path: "fields",
											},
											Value: &Object{
												Fields: []Field{
													{
														Name: []byte("name"),
														Value: &Value{
															PathSelector: PathSelector{
																Path: "name",
															},
															QuoteValue: true,
														},
													},
													{
														Name: []byte("type"),
														Value: &Object{
															PathSelector: PathSelector{
																Path: "type",
															},
															Fields: []Field{
																{
																	Name: []byte("name"),
																	Value: &Value{
																		PathSelector: PathSelector{
																			Path: "name",
																		},
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
							Name:        []byte("user"),
							HasResolver: true,
							Value: &Object{
								PathSelector: PathSelector{
									Path: "user",
								},
								Fetch: &ParallelFetch{
									Fetches: []Fetch{
										&SingleFetch{
											Source: &DataSourceInvocation{
												Args: []Argument{
													&StaticVariableArgument{
														Name:  literal.HOST,
														Value: []byte(REST1.URL),
													},
													&StaticVariableArgument{
														Name:  literal.URL,
														Value: []byte("/user/{{ .id }}/friends"),
													},
													&StaticVariableArgument{
														Name:  literal.METHOD,
														Value: []byte("GET"),
													},
													&ObjectVariableArgument{
														Name: []byte("id"),
														PathSelector: PathSelector{
															Path: "id",
														},
													},
												},
												DataSource: &HttpJsonDataSource{
													log: zap.NewNop(),
												},
											},
											BufferName: "friends",
										},
										&SingleFetch{
											Source: &DataSourceInvocation{
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
														Name:  literal.QUERY,
														Value: []byte(`query q1($id: String!){userPets(id: $id){	__typename name nickname... on Dog {woof} ... on Cat {meow}}}`),
													},
													&ObjectVariableArgument{
														Name: []byte("id"),
														PathSelector: PathSelector{
															Path: "id",
														},
													},
												},
												DataSource: &GraphQLDataSource{
													log: zap.NewNop(),
												},
											},
											BufferName: "pets",
										},
									},
								},
								Fields: []Field{
									{
										Name: []byte("id"),
										Value: &Value{
											PathSelector: PathSelector{
												Path: "id",
											},
										},
									},
									{
										Name: []byte("name"),
										Value: &Value{
											PathSelector: PathSelector{
												Path: "name",
											},
											QuoteValue: true,
										},
									},
									{
										Name: []byte("birthday"),
										Value: &Value{
											PathSelector: PathSelector{
												Path: "birthday",
											},
											QuoteValue: true,
										},
									},
									{
										Name:        []byte("friends"),
										HasResolver: true,
										Value: &List{
											Value: &Object{
												Fetch: &SingleFetch{
													Source: &DataSourceInvocation{
														Args: []Argument{
															&StaticVariableArgument{
																Name:  literal.HOST,
																Value: []byte(REST2.URL),
															},
															&StaticVariableArgument{
																Name:  literal.URL,
																Value: []byte("/friends/{{ .id }}/pets"),
															},
															&StaticVariableArgument{
																Name:  literal.METHOD,
																Value: []byte("GET"),
															},
															&ObjectVariableArgument{
																Name: []byte("id"),
																PathSelector: PathSelector{
																	Path: "id",
																},
															},
														},
														DataSource: &HttpJsonDataSource{
															log: zap.NewNop(),
														},
													},
													BufferName: "pets",
												},
												Fields: []Field{
													{
														Name: []byte("id"),
														Value: &Value{
															PathSelector: PathSelector{
																Path: "id",
															},
															QuoteValue: false,
														},
													},
													{
														Name: []byte("name"),
														Value: &Value{
															PathSelector: PathSelector{
																Path: "name",
															},
															QuoteValue: true,
														},
													},
													{
														Name: []byte("birthday"),
														Value: &Value{
															PathSelector: PathSelector{
																Path: "birthday",
															},
															QuoteValue: true,
														},
													},
													{
														Name:        []byte("pets"),
														HasResolver: true,
														Value: &List{
															Value: &Object{
																Fields: []Field{
																	{
																		Name: []byte("__typename"),
																		Value: &Value{
																			PathSelector: PathSelector{
																				Path: "__typename",
																			},
																			QuoteValue: true,
																		},
																	},
																	{
																		Name: []byte("name"),
																		Value: &Value{
																			PathSelector: PathSelector{
																				Path: "name",
																			},
																			QuoteValue: true,
																		},
																	},
																	{
																		Name: []byte("nickname"),
																		Value: &Value{
																			PathSelector: PathSelector{
																				Path: "nickname",
																			},
																			QuoteValue: true,
																		},
																	},
																	{
																		Name: []byte("woof"),
																		Value: &Value{
																			PathSelector: PathSelector{
																				Path: "woof",
																			},
																			QuoteValue: true,
																		},
																		Skip: &IfNotEqual{
																			Left: &ObjectVariableArgument{
																				PathSelector: PathSelector{
																					Path: "__typename",
																				},
																			},
																			Right: &StaticVariableArgument{
																				Value: []byte("Dog"),
																			},
																		},
																	},
																	{
																		Name: []byte("meow"),
																		Value: &Value{
																			PathSelector: PathSelector{
																				Path: "meow",
																			},
																			QuoteValue: true,
																		},
																		Skip: &IfNotEqual{
																			Left: &ObjectVariableArgument{
																				PathSelector: PathSelector{
																					Path: "__typename",
																				},
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
									{
										Name:        []byte("pets"),
										HasResolver: true,
										Value: &List{
											PathSelector: PathSelector{
												Path: "userPets",
											},
											Value: &Object{
												Fields: []Field{
													{
														Name: []byte("__typename"),
														Value: &Value{
															PathSelector: PathSelector{
																Path: "__typename",
															},
															QuoteValue: true,
														},
													},
													{
														Name: []byte("name"),
														Value: &Value{
															PathSelector: PathSelector{
																Path: "name",
															},
															QuoteValue: true,
														},
													},
													{
														Name: []byte("nickname"),
														Value: &Value{
															PathSelector: PathSelector{
																Path: "nickname",
															},
															QuoteValue: true,
														},
													},
													{
														Name: []byte("woof"),
														Value: &Value{
															PathSelector: PathSelector{
																Path: "woof",
															},
															QuoteValue: true,
														},
														Skip: &IfNotEqual{
															Left: &ObjectVariableArgument{
																PathSelector: PathSelector{
																	Path: "__typename",
																},
															},
															Right: &StaticVariableArgument{
																Value: []byte("Dog"),
															},
														},
													},
													{
														Name: []byte("meow"),
														Value: &Value{
															PathSelector: PathSelector{
																Path: "meow",
															},
															QuoteValue: true,
														},
														Skip: &IfNotEqual{
															Left: &ObjectVariableArgument{
																PathSelector: PathSelector{
																	Path: "__typename",
																},
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
	ex := NewExecutor()
	_, err := ex.Execute(exampleContext, object, &out)
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
	ex := NewExecutor()

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
				_, err := ex.Execute(exampleContext, object, &out)
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

func (f FakeDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) Instruction {
	_, _ = out.Write(f.data)
	return 0
}

func genField() Field {

	return Field{
		Name: []byte("data"),
		Value: &Object{
			Fetch: &ParallelFetch{
				Fetches: []Fetch{
					&SingleFetch{
						Source: &DataSourceInvocation{
							Args: []Argument{
								&ContextVariableArgument{
									Name:         []byte("name"),
									VariableName: []byte("name"),
								},
							},
							DataSource: &TypeDataSource{},
						},
						BufferName: "__type",
					},
					&SingleFetch{
						Source: &DataSourceInvocation{
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
						BufferName: "user",
					},
				},
			},
			Fields: []Field{
				{
					Name:        []byte("__type"),
					HasResolver: true,
					Value: &Object{
						PathSelector: PathSelector{
							Path: "__type",
						},
						Fields: []Field{
							{
								Name: []byte("name"),
								Value: &Value{
									PathSelector: PathSelector{
										Path: "name",
									},
								},
							},
							{
								Name: []byte("fields"),
								Value: &List{
									PathSelector: PathSelector{
										Path: "fields",
									},
									Value: &Object{
										Fields: []Field{
											{
												Name: []byte("name"),
												Value: &Value{
													PathSelector: PathSelector{
														Path: "name",
													},
												},
											},
											{
												Name: []byte("type"),
												Value: &Object{
													PathSelector: PathSelector{
														Path: "type",
													},
													Fields: []Field{
														{
															Name: []byte("name"),
															Value: &Value{
																PathSelector: PathSelector{
																	Path: "name",
																},
															},
														},
													},
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
					Name:        []byte("user"),
					HasResolver: true,
					Value: &Object{
						Fetch: &ParallelFetch{
							Fetches: []Fetch{
								&SingleFetch{
									Source: &DataSourceInvocation{
										Args: []Argument{
											&StaticVariableArgument{
												Name:  literal.URL,
												Value: []byte("/user/:id/friends"),
											},
											&ObjectVariableArgument{
												Name: []byte("id"),
												PathSelector: PathSelector{
													Path: "id",
												},
											},
										},
										DataSource: &FakeDataSource{
											friendsData,
										},
									},
									BufferName: "friends",
								},
								&SingleFetch{
									Source: &DataSourceInvocation{
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
												Name:  literal.QUERY,
												Value: []byte(`query q1($id: String!){userPets(id: $id){	__typename name nickname... on Dog {woof} ... on Cat {meow}}}`),
											},
											&ObjectVariableArgument{
												Name: []byte("id"),
												PathSelector: PathSelector{
													Path: "id",
												},
											},
										},
										DataSource: &FakeDataSource{
											data: petsData,
										},
									},
									BufferName: "pets",
								},
							},
						},
						PathSelector: PathSelector{
							Path: "data.user",
						},
						Fields: []Field{
							{
								Name: []byte("id"),
								Value: &Value{
									PathSelector: PathSelector{
										Path: "id",
									},
								},
							},
							{
								Name: []byte("name"),
								Value: &Value{
									PathSelector: PathSelector{
										Path: "name",
									},
									QuoteValue: true,
								},
							},
							{
								Name: []byte("birthday"),
								Value: &Value{
									PathSelector: PathSelector{
										Path: "birthday",
									},
								},
							},
							{
								Name:        []byte("friends"),
								HasResolver: true,
								Value: &List{
									Value: &Object{
										Fields: []Field{
											{
												Name: []byte("id"),
												Value: &Value{
													PathSelector: PathSelector{
														Path: "id",
													},
												},
											},
											{
												Name: []byte("name"),
												Value: &Value{
													PathSelector: PathSelector{
														Path: "name",
													},
													QuoteValue: true,
												},
											},
											{
												Name: []byte("birthday"),
												Value: &Value{
													PathSelector: PathSelector{
														Path: "birthday",
													},
												},
											},
										},
									},
								},
							},
							{
								Name:        []byte("pets"),
								HasResolver: true,
								Value: &List{
									PathSelector: PathSelector{
										Path: "data.userPets",
									},
									Value: &Object{
										Fields: []Field{
											{
												Name: []byte("__typename"),
												Value: &Value{
													PathSelector: PathSelector{
														Path: "__typename",
													},
												},
											},
											{
												Name: []byte("name"),
												Value: &Value{
													PathSelector: PathSelector{
														Path: "name",
													},
												},
											},
											{
												Name: []byte("nickname"),
												Value: &Value{
													PathSelector: PathSelector{
														Path: "nickname",
													},
												},
											},
											{
												Name: []byte("woof"),
												Value: &Value{
													PathSelector: PathSelector{
														Path: "woof",
													},
													QuoteValue: true,
												},
												Skip: &IfNotEqual{
													Left: &ObjectVariableArgument{
														PathSelector: PathSelector{
															Path: "__typename",
														},
													},
													Right: &StaticVariableArgument{
														Value: []byte("Dog"),
													},
												},
											},
											{
												Name: []byte("meow"),
												Value: &Value{
													PathSelector: PathSelector{
														Path: "meow",
													},
													QuoteValue: true,
												},
												Skip: &IfNotEqual{
													Left: &ObjectVariableArgument{
														PathSelector: PathSelector{
															Path: "__typename",
														},
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

var yaarasPets = []byte(`[
{
	"__typename":"Dog",
	"name":"Woof",
	"nickname":"Woofie",
	"woof":"Woof! Woof!"
 }
]`)

var ahmetsPets = []byte(`[
{
	"__typename":"Cat",
	"name":"KitCat",
	"nickname":"Kitty",
	"meow":"Meow meow!"
 }
]`)

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

func TestStreamExecution(t *testing.T) {
	out := bytes.Buffer{}
	ex := NewExecutor()
	c, cancel := context.WithCancel(context.Background())
	ctx := Context{
		Context: c,
	}

	want1 := `{"data":{"stream":{"bar":"bal","baz":1}}}`
	want2 := `{"data":{"stream":{"bar":"bal","baz":2}}}`
	want3 := `{"data":{"stream":{"bar":"bal","baz":3}}}`

	response1 := []byte(`{"bar":"bal","baz":1}`)
	response2 := []byte(`{"bar":"bal","baz":2}`)
	response3 := []byte(`{"bar":"bal","baz":3}`)

	resCount := 0

	REST1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		resCount++

		if r.RequestURI != "/bal" {
			t.Fatalf("want: /bal, got: %s\n", r.RequestURI)
		}

		var data []byte
		switch resCount {
		case 1:
			data = response1
		case 2:
			data = response2
		case 3:
			data = response2
		case 4:
			data = response3
		}

		_, err := w.Write(data)
		if err != nil {
			t.Fatal(err)
		}
	}))
	defer REST1.Close()

	streamPlan := &Object{
		operationType: ast.OperationTypeSubscription,
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fetch: &SingleFetch{
						Source: &DataSourceInvocation{
							Args: []Argument{
								&StaticVariableArgument{
									Name:  literal.HOST,
									Value: []byte(REST1.URL),
								},
								&StaticVariableArgument{
									Name:  literal.URL,
									Value: []byte("/bal"),
								},
							},
							DataSource: &HttpPollingStreamDataSource{
								delay: time.Millisecond,
								log:   zap.NewNop(),
							},
						},
						BufferName: "stream",
					},
					Fields: []Field{
						{
							Name:        []byte("stream"),
							HasResolver: true,
							Value: &Object{
								Fields: []Field{
									{
										Name: []byte("bar"),
										Value: &Value{
											PathSelector: PathSelector{
												Path: "bar",
											},
											QuoteValue: true,
										},
									},
									{
										Name: []byte("baz"),
										Value: &Value{
											PathSelector: PathSelector{
												Path: "baz",
											},
											QuoteValue: false,
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

	var instructions []Instruction
	var err error
	for i := 1; i < 4; i++ {
		out.Reset()
		instructions, err = ex.Execute(ctx, streamPlan, &out) // nolint
		if err != nil {
			t.Fatal(err)
		}
		var want string
		switch i {
		case 1:
			want = want1
		case 2:
			want = want2
		case 3:
			want = want3
		}

		got := out.String()
		if want != got {
			t.Fatalf("want(%d): %s\ngot: %s\n", i, want, got)
		}
	}

	cancel()
	instructions, err = ex.Execute(ctx, streamPlan, &out)
	if err != nil {
		t.Fatal(err)
	}

	if instructions[0] != CloseConnection {
		t.Fatalf("want CloseConnection, got: %d\n", instructions[0])
	}
}

func TestExecutor_ListFilterFirstN(t *testing.T) {

	plan := &Object{
		operationType: ast.OperationTypeQuery,
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fetch: &SingleFetch{
						Source: &DataSourceInvocation{
							Args: []Argument{
								&StaticVariableArgument{
									Value: []byte("[{\"bar\":\"1\"},{\"bar\":\"2\"},{\"bar\":\"3\"}]"),
								},
							},
							DataSource: &StaticDataSource{},
						},
						BufferName: "foos",
					},
					Fields: []Field{
						{
							Name:        []byte("foos"),
							HasResolver: true,
							Value: &List{
								Filter: &ListFilterFirstN{
									FirstN: 2,
								},
								Value: &Object{
									Fields: []Field{
										{
											Name: []byte("bar"),
											Value: &Value{
												PathSelector: PathSelector{
													Path: "bar",
												},
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
	}

	out := &bytes.Buffer{}
	ex := NewExecutor()
	ctx := Context{
		Context: context.Background(),
	}

	_, err := ex.Execute(ctx, plan, out)
	if err != nil {
		t.Fatal(err)
	}

	want := `{"data":{"foos":[{"bar":"1"},{"bar":"2"}]}}`
	got := out.String()

	if got != want {
		t.Fatalf("want: %s\ngot: %s\n", want, got)
	}
}

func TestExecutor_ListWithPath(t *testing.T) {

	plan := &Object{
		operationType: ast.OperationTypeQuery,
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fetch: &SingleFetch{
						Source: &DataSourceInvocation{
							Args: []Argument{
								&StaticVariableArgument{
									Value: []byte(`{"apis": [{"id": 1},{"id":2}]}`),
								},
							},
							DataSource: &StaticDataSource{},
						},
						BufferName: "apis",
					},
					Fields: []Field{
						{
							Name:        []byte("apis"),
							HasResolver: true,
							Value: &List{
								PathSelector: PathSelector{
									Path: "apis",
								},
								Value: &Object{
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												PathSelector: PathSelector{
													Path: "id",
												},
												QuoteValue: false,
											},
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
	ex := NewExecutor()
	ctx := Context{
		Context: context.Background(),
	}

	_, err := ex.Execute(ctx, plan, out)
	if err != nil {
		t.Fatal(err)
	}

	want := `{"data":{"apis":[{"id":1},{"id":2}]}}`
	got := out.String()

	if got != want {
		t.Fatalf("want: %s\ngot: %s\n", want, got)
	}
}

func TestExecutor_GraphqlDataSourceWithParams(t *testing.T) {

	graphQL1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		body, err := httputil.DumpRequest(r, true)
		if err != nil {
			t.Fatal(err)
		}

		fmt.Println(string(body))

		_, err = w.Write([]byte(`{
			"data": {
				"countries": [
					{"id":1},{"id":2}
				]
			}
		}`))
		if err != nil {
			t.Fatal(err)
		}
	}))

	plan := &Object{
		operationType: ast.OperationTypeQuery,
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fetch: &SingleFetch{
						Source: &DataSourceInvocation{
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
									Value: []byte("query q1($code: String!){countries(code: $code){id}}"),
								},
								&ContextVariableArgument{
									Name:         []byte("code"),
									VariableName: []byte("code"),
								},
							},
							DataSource: &GraphQLDataSource{
								log: zap.NewNop(),
							},
						},
						BufferName: "countries",
					},
					Fields: []Field{
						{
							Name:        []byte("countries"),
							HasResolver: true,
							Value: &List{
								PathSelector: PathSelector{
									Path: "countries",
								},
								Value: &Object{
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												PathSelector: PathSelector{
													Path: "id",
												},
												QuoteValue: false,
											},
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
	ex := NewExecutor()
	ctx := Context{
		Context: context.Background(),
		Variables: map[uint64][]byte{
			xxhash.Sum64String("code"): []byte("DE"),
		},
	}

	_, err := ex.Execute(ctx, plan, out)
	if err != nil {
		t.Fatal(err)
	}

	want := `{"data":{"countries":[{"id":1},{"id":2}]}}`
	got := out.String()

	if got != want {
		t.Fatalf("want: %s\ngot: %s\n", want, got)
	}
}

func TestExecutor_ObjectWithPath(t *testing.T) {

	plan := &Object{
		operationType: ast.OperationTypeQuery,
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fetch: &SingleFetch{
						Source: &DataSourceInvocation{
							Args: []Argument{
								&StaticVariableArgument{
									Value: []byte(`{"api": {"id": 1}`),
								},
							},
							DataSource: &StaticDataSource{},
						},
						BufferName: "id",
					},
					Fields: []Field{
						{
							Name:        []byte("id"),
							HasResolver: true,
							Value: &Value{
								PathSelector: PathSelector{
									Path: "api.id",
								},
								QuoteValue: false,
							},
						},
					},
				},
			},
		},
	}

	out := &bytes.Buffer{}
	ex := NewExecutor()
	ctx := Context{
		Context: context.Background(),
	}

	_, err := ex.Execute(ctx, plan, out)
	if err != nil {
		t.Fatal(err)
	}

	want := `{"data":{"id":1}}`
	got := out.String()

	if got != want {
		t.Fatalf("want: %s\ngot: %s\n", want, got)
	}
}

func TestExecutor_ResolveArgs(t *testing.T) {
	e := NewExecutor()
	e.context = Context{
		Context: context.Background(),
		Variables: map[uint64][]byte{
			xxhash.Sum64String("input"): []byte(`{"foo": "fooValue"}`),
		},
	}

	args := []Argument{
		&StaticVariableArgument{
			Name:  []byte("body"),
			Value: []byte("{\\\"key\\\":\\\"{{ .arguments.input.foo }}\\\"}"),
		},
		&ContextVariableArgument{
			Name:         []byte(".arguments.input"),
			VariableName: []byte("input"),
		},
	}

	resolved := e.ResolveArgs(args, nil)
	if len(resolved) != 1 {
		t.Fatalf("want 1, got: %d\n", len(resolved))
		return
	}
	want := []byte("{\\\"key\\\":\\\"fooValue\\\"}")
	if !bytes.Equal(resolved.ByKey([]byte("body")), want) {
		t.Fatalf("want key 'body' with value: '%s'\ndump: %s", string(want), resolved.Dump())
	}
}

func TestExecutor_ResolveArgsString(t *testing.T) {
	e := NewExecutor()
	e.context = Context{
		Context: context.Background(),
		Variables: map[uint64][]byte{
			xxhash.Sum64String("id"): []byte("foo123"),
		},
	}

	args := []Argument{
		&StaticVariableArgument{
			Name:  []byte("url"),
			Value: []byte("/apis/{{ .arguments.id }}"),
		},
		&ContextVariableArgument{
			Name:         []byte(".arguments.id"),
			VariableName: []byte("id"),
		},
	}

	resolved := e.ResolveArgs(args, nil)
	if len(resolved) != 1 {
		t.Fatalf("want 1, got: %d\n", len(resolved))
		return
	}
	want := []byte("/apis/foo123")
	if !bytes.Equal(resolved.ByKey([]byte("url")), want) {
		t.Fatalf("want key 'body' with value: '%s'\ndump: %s", string(want), resolved.Dump())
	}
}

func TestExecutor_ResolveArgs_MultipleNested(t *testing.T) {
	e := NewExecutor()
	e.context = Context{
		Context: context.Background(),
		Variables: map[uint64][]byte{
			xxhash.Sum64String("from"):  []byte(`{"year":2019,"month":11,"day":1}`),
			xxhash.Sum64String("until"): []byte(`{"year":2019,"month":12,"day":31}`),
			xxhash.Sum64String("page"):  []byte(`0`),
		},
	}

	args := []Argument{
		&StaticVariableArgument{
			Name:  []byte("url"),
			Value: []byte("/api/usage/apis/{{ .id }}/{{ .arguments.from.day }}/{{ .arguments.from.month }}/{{ .arguments.from.year }}/{{ .arguments.until.day }}/{{ .arguments.until.month }}/{{ .arguments.until.year }}?by=Hits&sort=1&p={{ .arguments.page }}"),
		},
		&StaticVariableArgument{
			Name:  []byte("id"),
			Value: []byte("1"),
		},
		&ContextVariableArgument{
			Name:         []byte(".arguments.from"),
			VariableName: []byte("from"),
		},
		&ContextVariableArgument{
			Name:         []byte(".arguments.until"),
			VariableName: []byte("until"),
		},
		&ContextVariableArgument{
			Name:         []byte(".arguments.page"),
			VariableName: []byte("page"),
		},
	}

	resolved := e.ResolveArgs(args, nil)
	if len(resolved) != 2 {
		t.Fatalf("want 1, got: %d\n", len(resolved))
		return
	}
	want := []byte("/api/usage/apis/1/1/11/2019/31/12/2019?by=Hits&sort=1&p=0")
	got := resolved.ByKey([]byte("url"))
	if !bytes.Equal(got, want) {
		t.Fatalf("want key 'body' with value: '%s'\ngot: '%s'\ndump: %s", string(want), string(got), resolved.Dump())
	}
}

func TestExecutor_ResolveArgsComplexPayload(t *testing.T) {
	e := NewExecutor()
	e.context = Context{
		Context: context.Background(),
		Variables: map[uint64][]byte{
			xxhash.Sum64String("input"): []byte(`{"foo": "fooValue", "bar": {"bal": "baz"}}`),
		},
	}

	args := []Argument{
		&StaticVariableArgument{
			Name:  []byte("body"),
			Value: []byte("{{ .arguments.input }}"),
		},
		&ContextVariableArgument{
			Name:         []byte(".arguments.input"),
			VariableName: []byte("input"),
		},
	}

	resolved := e.ResolveArgs(args, nil)
	if len(resolved) != 1 {
		t.Fatalf("want 1, got: %d\n", len(resolved))
		return
	}
	want := `{"foo": "fooValue", "bar": {"bal": "baz"}}`
	got := resolved.ByKey([]byte("body"))
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("want key 'body' with value:\n%s\ngot:\n%s\n", want, string(got))
	}
}

func TestExecutor_ResolveArgsComplexPayloadWithSelector(t *testing.T) {
	e := NewExecutor()
	e.context = Context{
		Context: context.Background(),
		Variables: map[uint64][]byte{
			xxhash.Sum64String("input"): []byte(`{"foo": "fooValue", "bar": {"bal": "baz"}}`),
		},
	}

	args := []Argument{
		&StaticVariableArgument{
			Name:  []byte("body"),
			Value: []byte("{{ .arguments.input.bar }}"),
		},
		&ContextVariableArgument{
			Name:         []byte(".arguments.input"),
			VariableName: []byte("input"),
		},
	}

	resolved := e.ResolveArgs(args, nil)
	if len(resolved) != 1 {
		t.Fatalf("want 1, got: %d\n", len(resolved))
		return
	}
	want := `{"bal": "baz"}`
	if !bytes.Equal(resolved.ByKey([]byte("body")), []byte(want)) {
		t.Fatalf("want key 'body' with value: '%s'", want)
	}
}

func TestExecutor_ResolveArgsFlatObjectContainingJSON(t *testing.T) {
	e := NewExecutor()
	e.context = Context{
		Context: context.Background(),
		Variables: map[uint64][]byte{
			xxhash.Sum64String("request"): []byte(`{"headers":{"Authorization":"foo"}}`),
		},
	}

	args := []Argument{
		&StaticVariableArgument{
			Name:  []byte("header"),
			Value: []byte("{{ .request.headers.Authorization }}"),
		},
		&ContextVariableArgument{
			Name:         []byte("request"),
			VariableName: []byte("request"),
		},
	}

	resolved := e.ResolveArgs(args, nil)
	if len(resolved) != 2 {
		t.Fatalf("want 2, got: %d\n", len(resolved))
		return
	}
	want := "foo"
	got := resolved.ByKey([]byte("header"))
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("want key 'request' with value: '%s', got: '%s'", want, string(got))
	}
}

func TestExecutor_ResolveArgsWithListArguments(t *testing.T) {
	e := NewExecutor()
	e.context = Context{
		Context: context.Background(),
	}

	args := []Argument{
		&ListArgument{
			Name: []byte("headers"),
			Arguments: []Argument{
				&StaticVariableArgument{
					Name:  []byte("foo"),
					Value: []byte("fooVal"),
				},
				&StaticVariableArgument{
					Name:  []byte("bar"),
					Value: []byte("barVal"),
				},
			},
		},
	}

	resolved := e.ResolveArgs(args, nil)
	if len(resolved) != 1 {
		t.Fatalf("want 1, got: %d\n", len(resolved))
		return
	}
	want := "{\"bar\":\"barVal\",\"foo\":\"fooVal\"}"
	got := string(resolved.ByKey([]byte("headers")))
	if want != got {
		t.Fatalf("want key 'headers' with value:\n%s, got:\n%s\ndump:\n%s\n", want, got, resolved.Dump())
	}
}

func TestExecutor_HTTPJSONDataSourceWithBody(t *testing.T) {

	wantUpstream := map[string]interface{}{
		"key": "fooValue",
	}
	wantBytes, err := json.MarshalIndent(wantUpstream, "", "  ")
	if err != nil {
		t.Fatal(err)
		return
	}

	wantString := string(wantBytes)

	REST1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if r.Method != http.MethodPost {
			t.Fatalf("wantUpstream: %s, got: %s\n", http.MethodPost, r.Method)
			return
		}

		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
			return
		}
		defer r.Body.Close()

		strData := string(data)
		_ = strData

		gotString := prettyJSON(bytes.NewReader(data))

		if wantString != gotString {
			t.Fatalf("wantUpstream: %s\ngot: %s\n", wantString, gotString)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("bar"))
	}))

	plan := &Object{
		operationType: ast.OperationTypeQuery,
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fetch: &SingleFetch{
						BufferName: "withBody",
						Source: &DataSourceInvocation{
							DataSource: &HttpJsonDataSource{
								log: zap.NewNop(),
							},
							Args: []Argument{
								&StaticVariableArgument{
									Name:  []byte("host"),
									Value: []byte(REST1.URL),
								},
								&StaticVariableArgument{
									Name:  []byte("url"),
									Value: []byte("/"),
								},
								&StaticVariableArgument{
									Name:  []byte("method"),
									Value: []byte("POST"),
								},
								&StaticVariableArgument{
									Name:  []byte("body"),
									Value: []byte("{\\\"key\\\":\\\"{{ .arguments.input.foo }}\\\"}"),
								},
								&ContextVariableArgument{
									Name:         []byte(".arguments.input"),
									VariableName: []byte("input"),
								},
							},
						},
					},
					Fields: []Field{
						{
							Name:        []byte("withBody"),
							HasResolver: true,
							Value: &Value{
								QuoteValue: true,
							},
						},
					},
				},
			},
		},
	}

	out := &bytes.Buffer{}
	ex := NewExecutor()
	ctx := Context{
		Context: context.Background(),
		Variables: map[uint64][]byte{
			xxhash.Sum64String("input"): []byte(`{"foo": "fooValue"}`),
		},
	}

	_, err = ex.Execute(ctx, plan, out)
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]interface{}{
		"data": map[string]interface{}{
			"withBody": "bar",
		},
	}

	wantResult, err := json.MarshalIndent(expected, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	want := string(wantResult)
	got := prettyJSON(out)

	if want != got {
		t.Fatalf("want: %s\ngot: %s\n", want, got)
		return
	}
}

func TestExecutor_HTTPJSONDataSourceWithBodyComplexPlayload(t *testing.T) {

	wantUpstream := map[string]interface{}{
		"foo": "fooValue",
		"bar": map[string]interface{}{
			"bal": "baz",
		},
	}

	wantBytes, err := json.MarshalIndent(wantUpstream, "", "  ")
	if err != nil {
		t.Fatal(err)
		return
	}

	wantString := string(wantBytes)

	REST1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if r.Method != http.MethodPost {
			t.Fatalf("wantUpstream: %s, got: %s\n", http.MethodPost, r.Method)
			return
		}

		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
			return
		}
		defer r.Body.Close()

		gotString := prettyJSON(bytes.NewReader(data))

		if wantString != gotString {
			t.Fatalf("wantUpstream: %s\ngot: %s\n", wantString, gotString)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("bar"))
	}))

	plan := &Object{
		operationType: ast.OperationTypeQuery,
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fetch: &SingleFetch{
						BufferName: "withBody",
						Source: &DataSourceInvocation{
							DataSource: &HttpJsonDataSource{
								log: zap.NewNop(),
							},
							Args: []Argument{
								&StaticVariableArgument{
									Name:  []byte("host"),
									Value: []byte(REST1.URL),
								},
								&StaticVariableArgument{
									Name:  []byte("url"),
									Value: []byte("/"),
								},
								&StaticVariableArgument{
									Name:  []byte("method"),
									Value: []byte("POST"),
								},
								&StaticVariableArgument{
									Name:  []byte("body"),
									Value: []byte("{{ .arguments.input }}"),
								},
								&ContextVariableArgument{
									Name:         []byte(".arguments.input"),
									VariableName: []byte("input"),
								},
							},
						},
					},
					Fields: []Field{
						{
							Name:        []byte("withBody"),
							HasResolver: true,
							Value: &Value{
								QuoteValue: true,
							},
						},
					},
				},
			},
		},
	}

	out := &bytes.Buffer{}
	ex := NewExecutor()
	ctx := Context{
		Context: context.Background(),
		Variables: map[uint64][]byte{
			xxhash.Sum64String("input"): []byte(`{"foo": "fooValue", "bar": {"bal": "baz"}}`),
		},
	}

	_, err = ex.Execute(ctx, plan, out)
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]interface{}{
		"data": map[string]interface{}{
			"withBody": "bar",
		},
	}

	wantResult, err := json.MarshalIndent(expected, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	want := string(wantResult)
	got := prettyJSON(out)

	if want != got {
		t.Fatalf("want: %s\ngot: %s\n", want, got)
		return
	}
}

func TestExecutor_HTTPJSONDataSourceWithHeaders(t *testing.T) {

	REST1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		for k, v := range map[string]string{
			"foo": "fooVal",
			"bar": "barVal",
		} {
			got := r.Header.Get(k)
			if got != v {
				t.Fatalf("want header with key '%s' and value '%s', got: '%s'", k, v, got)
			}
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("bar"))
	}))

	plan := &Object{
		operationType: ast.OperationTypeQuery,
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fetch: &SingleFetch{
						BufferName: "withHeaders",
						Source: &DataSourceInvocation{
							DataSource: &HttpJsonDataSource{
								log: zap.NewNop(),
							},
							Args: []Argument{
								&StaticVariableArgument{
									Name:  []byte("host"),
									Value: []byte(REST1.URL),
								},
								&StaticVariableArgument{
									Name:  []byte("url"),
									Value: []byte("/"),
								},
								&StaticVariableArgument{
									Name:  []byte("method"),
									Value: []byte("GET"),
								},
								&ListArgument{
									Name: []byte("headers"),
									Arguments: []Argument{
										&StaticVariableArgument{
											Name:  []byte("foo"),
											Value: []byte("fooVal"),
										},
										&StaticVariableArgument{
											Name:  []byte("bar"),
											Value: []byte("barVal"),
										},
									},
								},
							},
						},
					},
					Fields: []Field{
						{
							Name:        []byte("withHeaders"),
							HasResolver: true,
							Value: &Value{
								QuoteValue: true,
							},
						},
					},
				},
			},
		},
	}

	out := &bytes.Buffer{}
	ex := NewExecutor()
	ctx := Context{
		Context: context.Background(),
	}

	_, err := ex.Execute(ctx, plan, out)
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]interface{}{
		"data": map[string]interface{}{
			"withHeaders": "bar",
		},
	}

	wantResult, err := json.MarshalIndent(expected, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	want := string(wantResult)
	got := prettyJSON(out)

	if want != got {
		t.Fatalf("want: %s\ngot: %s\n", want, got)
		return
	}
}

func TestExecutor_HTTPJSONDataSourceWithPathSelector(t *testing.T) {

	response := []byte(`
{
	"name": {"first": "Tom", "last": "Anderson"},
	"age":37,
	"children": ["Sara","Alex","Jack"],
	"fav.movie": "Deer Hunter",
	"friends": [
		{"first": "Dale", "last": "Murphy", "age": 44, "nets": ["ig", "fb", "tw"]},
		{"first": "Roger", "last": "Craig", "age": 68, "nets": ["fb", "tw"]},
		{"first": "Jane", "last": "Murphy", "age": 47, "nets": ["ig", "tw"]}
	]
}`)

	REST1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(response)
	}))

	plan := &Object{
		operationType: ast.OperationTypeQuery,
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fetch: &SingleFetch{
						BufferName: "friends",
						Source: &DataSourceInvocation{
							DataSource: &HttpJsonDataSource{
								log: zap.NewNop(),
							},
							Args: []Argument{
								&StaticVariableArgument{
									Name:  []byte("host"),
									Value: []byte(REST1.URL),
								},
								&StaticVariableArgument{
									Name:  []byte("url"),
									Value: []byte("/"),
								},
								&StaticVariableArgument{
									Name:  []byte("method"),
									Value: []byte("GET"),
								},
							},
						},
					},
					Fields: []Field{
						{
							Name:        []byte("friends"),
							HasResolver: true,
							Value: &Object{
								Fields: []Field{
									{
										Name: []byte("firstNames"),
										Value: &List{
											PathSelector: PathSelector{
												Path: "friends.#.first",
											},
											Value: &Value{
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
	}

	out := &bytes.Buffer{}
	ex := NewExecutor()
	ctx := Context{
		Context: context.Background(),
	}

	_, err := ex.Execute(ctx, plan, out)
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]interface{}{
		"data": map[string]interface{}{
			"friends": map[string]interface{}{
				"firstNames": []string{"Dale", "Roger", "Jane"},
			},
		},
	}

	wantResult, err := json.MarshalIndent(expected, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	want := string(wantResult)
	got := prettyJSON(out)

	if want != got {
		t.Fatalf("want: %s\ngot: %s\n", want, got)
		return
	}
}

func prettyJSON(r io.Reader) string {
	data := map[string]interface{}{}
	err := json.NewDecoder(r).Decode(&data)
	if err != nil {
		panic(err)
	}
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(out)
}

func TestExecutor_Introspection(t *testing.T) {
	executor := NewExecutor()
	ctx := Context{
		Context: context.Background(),
	}

	schema := []byte(`
		schema {
			query: Query
		}
		type Query {
			"""
			multiline
			description
			"""
			foo: String
			__schema: __Schema!
		}
	`)

	handler, err := NewHandler(schema, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}

	gen := introspection.NewGenerator()
	report := operationreport.Report{}
	data := introspection.Data{}
	gen.Generate(&handler.definition, &report, &data)

	introspectionData, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}

	out := bytes.Buffer{}
	_, err = executor.Execute(ctx, introspectionQuery(introspectionData), &out)
	if err != nil {
		t.Fatal(err)
	}

	response := out.Bytes()

	goldie.Assert(t,"introspection_execution",response)
	if t.Failed() {

		fixture, err := ioutil.ReadFile("./fixtures/introspection_execution.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("execution", fixture, response)
	}
}
