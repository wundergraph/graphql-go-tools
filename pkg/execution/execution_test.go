package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"testing"
	"time"

	"github.com/cespare/xxhash/v2"
	log "github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/execution/datasource"
	"github.com/wundergraph/graphql-go-tools/pkg/introspection"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
	"github.com/wundergraph/graphql-go-tools/pkg/testing/goldie"
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
		// dumpRequest(t, r, "graphQL1")

		_, err := w.Write(userData)
		if err != nil {
			t.Fatal(err)
		}
	}))

	graphQL2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// dumpRequest(t, r, "graphQL2")

		_, err := w.Write(petsData)
		if err != nil {
			t.Fatal(err)
		}
	}))

	REST1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// dumpRequest(t, r, "rest1")

		_, err := w.Write(friendsData)
		if err != nil {
			t.Fatal(err)
		}
	}))

	REST2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// dumpRequest(t, r, "rest1")

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
									Args: []datasource.Argument{
										&datasource.ContextVariableArgument{
											Name:         []byte("name"),
											VariableName: []byte("name"),
										},
									},
									DataSource: &datasource.TypeDataSource{},
								},
								BufferName: "__type",
							},
							&SingleFetch{
								Source: &DataSourceInvocation{
									Args: []datasource.Argument{
										&datasource.StaticVariableArgument{
											Name:  literal.URL,
											Value: []byte(graphQL1.URL + "/graphql"),
										},
										&datasource.StaticVariableArgument{
											Name:  literal.QUERY,
											Value: []byte("query q1($id: String!){user{id name birthday}}"),
										},
										&datasource.ContextVariableArgument{
											Name:         []byte("id"),
											VariableName: []byte("id"),
										},
									},
									DataSource: &datasource.GraphQLDataSource{
										Log:    log.NoopLogger,
										Client: datasource.DefaultHttpClient(),
									},
								},
								BufferName: "user",
							},
						},
					},
					Fields: []Field{
						{
							Name:            []byte("__type"),
							HasResolvedData: true,
							Value: &Object{
								DataResolvingConfig: DataResolvingConfig{
									PathSelector: datasource.PathSelector{
										Path: "__type",
									},
								},
								Fields: []Field{
									{
										Name: []byte("name"),
										Value: &Value{
											DataResolvingConfig: DataResolvingConfig{
												PathSelector: datasource.PathSelector{
													Path: "name",
												},
											},
											ValueType: StringValueType,
										},
									},
									{
										Name: []byte("fields"),
										Value: &List{
											DataResolvingConfig: DataResolvingConfig{
												PathSelector: datasource.PathSelector{
													Path: "fields",
												},
											},
											Value: &Object{
												Fields: []Field{
													{
														Name: []byte("name"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: datasource.PathSelector{
																	Path: "name",
																},
															},
															ValueType: StringValueType,
														},
													},
													{
														Name: []byte("type"),
														Value: &Object{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: datasource.PathSelector{
																	Path: "type",
																},
															},
															Fields: []Field{
																{
																	Name: []byte("name"),
																	Value: &Value{
																		DataResolvingConfig: DataResolvingConfig{
																			PathSelector: datasource.PathSelector{
																				Path: "name",
																			},
																		},
																		ValueType: StringValueType,
																	},
																},
															},
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
							Name:            []byte("user"),
							HasResolvedData: true,
							Value: &Object{
								DataResolvingConfig: DataResolvingConfig{
									PathSelector: datasource.PathSelector{
										Path: "user",
									},
								},
								Fetch: &ParallelFetch{
									Fetches: []Fetch{
										&SingleFetch{
											Source: &DataSourceInvocation{
												Args: []datasource.Argument{
													&datasource.StaticVariableArgument{
														Name:  literal.URL,
														Value: []byte(REST1.URL + "/user/{{ .id }}/friends"),
													},
													&datasource.StaticVariableArgument{
														Name:  literal.METHOD,
														Value: []byte("GET"),
													},
													&datasource.ObjectVariableArgument{
														Name: []byte("id"),
														PathSelector: datasource.PathSelector{
															Path: "id",
														},
													},
												},
												DataSource: &datasource.HttpJsonDataSource{
													Log:    log.NoopLogger,
													Client: datasource.DefaultHttpClient(),
												},
											},
											BufferName: "friends",
										},
										&SingleFetch{
											Source: &DataSourceInvocation{
												Args: []datasource.Argument{
													&datasource.StaticVariableArgument{
														Name:  literal.URL,
														Value: []byte(graphQL2.URL + "/graphql"),
													},
													&datasource.StaticVariableArgument{
														Name:  literal.QUERY,
														Value: []byte(`query q1($id: String!){userPets(id: $id){	__typename name nickname... on Dog {woof} ... on Cat {meow}}}`),
													},
													&datasource.ObjectVariableArgument{
														Name: []byte("id"),
														PathSelector: datasource.PathSelector{
															Path: "id",
														},
													},
												},
												DataSource: &datasource.GraphQLDataSource{
													Log:    log.NoopLogger,
													Client: datasource.DefaultHttpClient(),
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
											DataResolvingConfig: DataResolvingConfig{
												PathSelector: datasource.PathSelector{
													Path: "id",
												},
											},
											ValueType: IntegerValueType,
										},
									},
									{
										Name: []byte("name"),
										Value: &Value{
											DataResolvingConfig: DataResolvingConfig{
												PathSelector: datasource.PathSelector{
													Path: "name",
												},
											},
											ValueType: StringValueType,
										},
									},
									{
										Name: []byte("birthday"),
										Value: &Value{
											DataResolvingConfig: DataResolvingConfig{
												PathSelector: datasource.PathSelector{
													Path: "birthday",
												},
											},
											ValueType: StringValueType,
										},
									},
									{
										Name:            []byte("friends"),
										HasResolvedData: true,
										Value: &List{
											Value: &Object{
												Fetch: &SingleFetch{
													Source: &DataSourceInvocation{
														Args: []datasource.Argument{
															&datasource.StaticVariableArgument{
																Name:  literal.URL,
																Value: []byte(REST2.URL + "/friends/{{ .id }}/pets"),
															},
															&datasource.StaticVariableArgument{
																Name:  literal.METHOD,
																Value: []byte("GET"),
															},
															&datasource.ObjectVariableArgument{
																Name: []byte("id"),
																PathSelector: datasource.PathSelector{
																	Path: "id",
																},
															},
														},
														DataSource: &datasource.HttpJsonDataSource{
															Log:    log.NoopLogger,
															Client: datasource.DefaultHttpClient(),
														},
													},
													BufferName: "pets",
												},
												Fields: []Field{
													{
														Name: []byte("id"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: datasource.PathSelector{
																	Path: "id",
																},
															},
															ValueType: IntegerValueType,
														},
													},
													{
														Name: []byte("name"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: datasource.PathSelector{
																	Path: "name",
																},
															},
															ValueType: StringValueType,
														},
													},
													{
														Name: []byte("birthday"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: datasource.PathSelector{
																	Path: "birthday",
																},
															},
															ValueType: StringValueType,
														},
													},
													{
														Name:            []byte("pets"),
														HasResolvedData: true,
														Value: &List{
															Value: &Object{
																Fields: []Field{
																	{
																		Name: []byte("__typename"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: datasource.PathSelector{
																					Path: "__typename",
																				},
																			},
																			ValueType: StringValueType,
																		},
																	},
																	{
																		Name: []byte("name"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: datasource.PathSelector{
																					Path: "name",
																				},
																			},
																			ValueType: StringValueType,
																		},
																	},
																	{
																		Name: []byte("nickname"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: datasource.PathSelector{
																					Path: "nickname",
																				},
																			},
																			ValueType: StringValueType,
																		},
																	},
																	{
																		Name: []byte("woof"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: datasource.PathSelector{
																					Path: "woof",
																				},
																			},
																			ValueType: StringValueType,
																		},
																		Skip: &IfNotEqual{
																			Left: &datasource.ObjectVariableArgument{
																				PathSelector: datasource.PathSelector{
																					Path: "__typename",
																				},
																			},
																			Right: &datasource.StaticVariableArgument{
																				Value: []byte("Dog"),
																			},
																		},
																	},
																	{
																		Name: []byte("meow"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: datasource.PathSelector{
																					Path: "meow",
																				},
																			},
																			ValueType: StringValueType,
																		},
																		Skip: &IfNotEqual{
																			Left: &datasource.ObjectVariableArgument{
																				PathSelector: datasource.PathSelector{
																					Path: "__typename",
																				},
																			},
																			Right: &datasource.StaticVariableArgument{
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
										Name:            []byte("pets"),
										HasResolvedData: true,
										Value: &List{
											DataResolvingConfig: DataResolvingConfig{
												PathSelector: datasource.PathSelector{
													Path: "userPets",
												},
											},
											Value: &Object{
												Fields: []Field{
													{
														Name: []byte("__typename"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: datasource.PathSelector{
																	Path: "__typename",
																},
															},
															ValueType: StringValueType,
														},
													},
													{
														Name: []byte("name"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: datasource.PathSelector{
																	Path: "name",
																},
															},
															ValueType: StringValueType,
														},
													},
													{
														Name: []byte("nickname"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: datasource.PathSelector{
																	Path: "nickname",
																},
															},
															ValueType: StringValueType,
														},
													},
													{
														Name: []byte("woof"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: datasource.PathSelector{
																	Path: "woof",
																},
															},
															ValueType: StringValueType,
														},
														Skip: &IfNotEqual{
															Left: &datasource.ObjectVariableArgument{
																PathSelector: datasource.PathSelector{
																	Path: "__typename",
																},
															},
															Right: &datasource.StaticVariableArgument{
																Value: []byte("Dog"),
															},
														},
													},
													{
														Name: []byte("meow"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: datasource.PathSelector{
																	Path: "meow",
																},
															},
															ValueType: StringValueType,
														},
														Skip: &IfNotEqual{
															Left: &datasource.ObjectVariableArgument{
																PathSelector: datasource.PathSelector{
																	Path: "__typename",
																},
															},
															Right: &datasource.StaticVariableArgument{
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
	ex := NewExecutor(nil)
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

		fixture, err := os.ReadFile("./fixtures/execution.golden")
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, fixture, pretty)
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
	ex := NewExecutor(nil)

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

func (f FakeDataSource) Resolve(ctx context.Context, args datasource.ResolverArgs, out io.Writer) (n int, err error) {
	return out.Write(f.data)
}

func genField() Field {

	return Field{
		Name: []byte("data"),
		Value: &Object{
			Fetch: &ParallelFetch{
				Fetches: []Fetch{
					&SingleFetch{
						Source: &DataSourceInvocation{
							Args: []datasource.Argument{
								&datasource.ContextVariableArgument{
									Name:         []byte("name"),
									VariableName: []byte("name"),
								},
							},
							DataSource: &datasource.TypeDataSource{},
						},
						BufferName: "__type",
					},
					&SingleFetch{
						Source: &DataSourceInvocation{
							Args: []datasource.Argument{
								&datasource.StaticVariableArgument{
									Name:  literal.URL,
									Value: []byte("localhost:8001/graphql"),
								},
								&datasource.StaticVariableArgument{
									Name:  literal.QUERY,
									Value: []byte("query q1($id: String!){user{id name birthday}}"),
								},
								&datasource.ContextVariableArgument{
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
					Name:            []byte("__type"),
					HasResolvedData: true,
					Value: &Object{
						DataResolvingConfig: DataResolvingConfig{
							PathSelector: datasource.PathSelector{
								Path: "__type",
							},
						},
						Fields: []Field{
							{
								Name: []byte("name"),
								Value: &Value{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: datasource.PathSelector{
											Path: "name",
										},
									},
								},
							},
							{
								Name: []byte("fields"),
								Value: &List{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: datasource.PathSelector{
											Path: "fields",
										},
									},
									Value: &Object{
										Fields: []Field{
											{
												Name: []byte("name"),
												Value: &Value{
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: datasource.PathSelector{
															Path: "name",
														},
													},
												},
											},
											{
												Name: []byte("type"),
												Value: &Object{
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: datasource.PathSelector{
															Path: "type",
														},
													},
													Fields: []Field{
														{
															Name: []byte("name"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: datasource.PathSelector{
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
				},
				{
					Name:            []byte("user"),
					HasResolvedData: true,
					Value: &Object{
						Fetch: &ParallelFetch{
							Fetches: []Fetch{
								&SingleFetch{
									Source: &DataSourceInvocation{
										Args: []datasource.Argument{
											&datasource.StaticVariableArgument{
												Name:  literal.URL,
												Value: []byte("/user/:id/friends"),
											},
											&datasource.ObjectVariableArgument{
												Name: []byte("id"),
												PathSelector: datasource.PathSelector{
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
										Args: []datasource.Argument{
											&datasource.StaticVariableArgument{
												Name:  literal.URL,
												Value: []byte("localhost:8002/graphql"),
											},
											&datasource.StaticVariableArgument{
												Name:  literal.QUERY,
												Value: []byte(`query q1($id: String!){userPets(id: $id){	__typename name nickname... on Dog {woof} ... on Cat {meow}}}`),
											},
											&datasource.ObjectVariableArgument{
												Name: []byte("id"),
												PathSelector: datasource.PathSelector{
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
						DataResolvingConfig: DataResolvingConfig{
							PathSelector: datasource.PathSelector{
								Path: "data.user",
							},
						},
						Fields: []Field{
							{
								Name: []byte("id"),
								Value: &Value{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: datasource.PathSelector{
											Path: "id",
										},
									},
								},
							},
							{
								Name: []byte("name"),
								Value: &Value{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: datasource.PathSelector{
											Path: "name",
										},
									},
									ValueType: StringValueType,
								},
							},
							{
								Name: []byte("birthday"),
								Value: &Value{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: datasource.PathSelector{
											Path: "birthday",
										},
									},
								},
							},
							{
								Name:            []byte("friends"),
								HasResolvedData: true,
								Value: &List{
									Value: &Object{
										Fields: []Field{
											{
												Name: []byte("id"),
												Value: &Value{
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: datasource.PathSelector{
															Path: "id",
														},
													},
												},
											},
											{
												Name: []byte("name"),
												Value: &Value{
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: datasource.PathSelector{
															Path: "name",
														},
													},
													ValueType: StringValueType,
												},
											},
											{
												Name: []byte("birthday"),
												Value: &Value{
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: datasource.PathSelector{
															Path: "birthday",
														},
													},
												},
											},
										},
									},
								},
							},
							{
								Name:            []byte("pets"),
								HasResolvedData: true,
								Value: &List{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: datasource.PathSelector{
											Path: "data.userPets",
										},
									},
									Value: &Object{
										Fields: []Field{
											{
												Name: []byte("__typename"),
												Value: &Value{
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: datasource.PathSelector{
															Path: "__typename",
														},
													},
												},
											},
											{
												Name: []byte("name"),
												Value: &Value{
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: datasource.PathSelector{
															Path: "name",
														},
													},
												},
											},
											{
												Name: []byte("nickname"),
												Value: &Value{
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: datasource.PathSelector{
															Path: "nickname",
														},
													},
												},
											},
											{
												Name: []byte("woof"),
												Value: &Value{
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: datasource.PathSelector{
															Path: "woof",
														},
													},
													ValueType: StringValueType,
												},
												Skip: &IfNotEqual{
													Left: &datasource.ObjectVariableArgument{
														PathSelector: datasource.PathSelector{
															Path: "__typename",
														},
													},
													Right: &datasource.StaticVariableArgument{
														Value: []byte("Dog"),
													},
												},
											},
											{
												Name: []byte("meow"),
												Value: &Value{
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: datasource.PathSelector{
															Path: "meow",
														},
													},
													ValueType: StringValueType,
												},
												Skip: &IfNotEqual{
													Left: &datasource.ObjectVariableArgument{
														PathSelector: datasource.PathSelector{
															Path: "__typename",
														},
													},
													Right: &datasource.StaticVariableArgument{
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
	ex := NewExecutor(nil)
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
							Args: []datasource.Argument{
								&datasource.StaticVariableArgument{
									Name:  literal.HOST,
									Value: []byte(REST1.URL),
								},
								&datasource.StaticVariableArgument{
									Name:  literal.URL,
									Value: []byte("/bal"),
								},
							},
							DataSource: &datasource.HttpPollingStreamDataSource{
								Delay: time.Millisecond,
								Log:   log.NoopLogger,
							},
						},
						BufferName: "stream",
					},
					Fields: []Field{
						{
							Name:            []byte("stream"),
							HasResolvedData: true,
							Value: &Object{
								Fields: []Field{
									{
										Name: []byte("bar"),
										Value: &Value{
											DataResolvingConfig: DataResolvingConfig{
												PathSelector: datasource.PathSelector{
													Path: "bar",
												},
											},
											ValueType: StringValueType,
										},
									},
									{
										Name: []byte("baz"),
										Value: &Value{
											DataResolvingConfig: DataResolvingConfig{
												PathSelector: datasource.PathSelector{
													Path: "baz",
												},
											},
											ValueType: IntegerValueType,
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
	for i := 1; i < 4; i++ {
		out.Reset()
		err = ex.Execute(ctx, streamPlan, &out) // nolint
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
	err = ex.Execute(ctx, streamPlan, &out)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-ctx.Done():
		return
	default:
		t.Fatalf("expected context.Context to be cancelled")
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
							DataSource: &datasource.StaticDataSource{
								Data: []byte("[{\"bar\":\"1\"},{\"bar\":\"2\"},{\"bar\":\"3\"}]"),
							},
						},
						BufferName: "foos",
					},
					Fields: []Field{
						{
							Name:            []byte("foos"),
							HasResolvedData: true,
							Value: &List{
								Filter: &ListFilterFirstN{
									FirstN: 2,
								},
								Value: &Object{
									Fields: []Field{
										{
											Name: []byte("bar"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
														Path: "bar",
													},
												},
												ValueType: StringValueType,
											},
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
	ex := NewExecutor(nil)
	ctx := Context{
		Context: context.Background(),
	}

	err := ex.Execute(ctx, plan, out)
	if err != nil {
		t.Fatal(err)
	}

	want := `{"data":{"foos":[{"bar":"1"},{"bar":"2"}]}}`
	got := out.String()

	if got != want {
		t.Fatalf("want: %s\ngot: %s\n", want, got)
	}
}

func TestExecutor_ObjectVariables(t *testing.T) {

	REST1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if r.Method != http.MethodPost {
			t.Fatalf("want method POST, got: %s", r.Method)
			return
		}

		got, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}

		want := []byte(`{"id":1}`)

		if !bytes.Equal(want, got) {
			t.Fatalf("want: '%s', got: '%s'", string(want), string(got))
		}

		response := []byte(`{"name":"Woof","age":3}`)

		_, err = w.Write(response)
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
							DataSource: &datasource.StaticDataSource{
								Data: []byte(`{"name": "Jens","id":1}`),
							},
						},
						BufferName: "user",
					},
					Fields: []Field{
						{
							Name:            []byte("user"),
							HasResolvedData: true,
							Value: &Object{
								Fetch: &SingleFetch{
									BufferName: "pet",
									Source: &DataSourceInvocation{
										Args: []datasource.Argument{
											&datasource.StaticVariableArgument{
												Name:  literal.URL,
												Value: []byte(REST1.URL + "/"),
											},
											&datasource.StaticVariableArgument{
												Name:  literal.METHOD,
												Value: []byte("POST"),
											},
											&datasource.StaticVariableArgument{
												Name:  literal.BODY,
												Value: []byte(`{"id":{{ .object.id }}}`),
											},
										},
										DataSource: &datasource.HttpJsonDataSource{
											Log:    log.NoopLogger,
											Client: datasource.DefaultHttpClient(),
										},
									},
								},
								Fields: []Field{
									{
										Name: []byte("name"),
										Value: &Value{
											DataResolvingConfig: DataResolvingConfig{
												PathSelector: datasource.PathSelector{
													Path: "name",
												},
											},
											ValueType: StringValueType,
										},
									},
									{
										Name: []byte("id"),
										Value: &Value{
											DataResolvingConfig: DataResolvingConfig{
												PathSelector: datasource.PathSelector{
													Path: "id",
												},
											},
											ValueType: IntegerValueType,
										},
									},
									{
										Name:            []byte("pet"),
										HasResolvedData: true,
										Value: &Object{
											Fields: []Field{
												{
													Name: []byte("name"),
													Value: &Value{
														DataResolvingConfig: DataResolvingConfig{
															PathSelector: datasource.PathSelector{
																Path: "name",
															},
														},
														ValueType: StringValueType,
													},
												},
												{
													Name: []byte("age"),
													Value: &Value{
														DataResolvingConfig: DataResolvingConfig{
															PathSelector: datasource.PathSelector{
																Path: "age",
															},
														},
														ValueType: IntegerValueType,
													},
												},
											},
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
	ex := NewExecutor(nil)
	ctx := Context{
		Context: context.Background(),
	}

	err := ex.Execute(ctx, plan, out)
	if err != nil {
		t.Fatal(err)
	}

	want := `{"data":{"user":{"name":"Jens","id":1,"pet":{"name":"Woof","age":3}}}}`
	got := out.String()

	if got != want {
		t.Fatalf("want: %s\ngot: %s\n", want, got)
	}
}

func TestExecutor_NestedObjectVariables(t *testing.T) {

	previewService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":1}}`))
	}))

	defer previewService.Close()

	additionalDataService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/api/additional_data?data_ids=1"
		if r.RequestURI != want {
			t.Fatalf("want uri: %s, got: %s", want, r.RequestURI)
		}
		_, _ = w.Write([]byte(`{"name":"foo"}`))
	}))

	defer additionalDataService.Close()

	plan := &Object{
		operationType: ast.OperationTypeQuery,
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fetch: &SingleFetch{
						BufferName: "preview",
						Source: &DataSourceInvocation{
							Args: []datasource.Argument{
								&datasource.StaticVariableArgument{
									Name:  literal.URL,
									Value: []byte(previewService.URL + "/"),
								},
								&datasource.StaticVariableArgument{
									Name:  literal.METHOD,
									Value: []byte("GET"),
								},
							},
							DataSource: &datasource.HttpJsonDataSource{
								Log:    log.NoopLogger,
								Client: datasource.DefaultHttpClient(),
							},
						},
					},
					Fields: []Field{
						{
							Name:            []byte("preview"),
							HasResolvedData: true,
							Value: &Object{
								Fields: []Field{
									{
										Name: []byte("data"),
										Value: &Object{
											DataResolvingConfig: DataResolvingConfig{
												PathSelector: datasource.PathSelector{
													Path: "data",
												},
											},
											Fetch: &SingleFetch{
												BufferName: "additional_data_for_id",
												Source: &DataSourceInvocation{
													Args: []datasource.Argument{
														&datasource.StaticVariableArgument{
															Name:  literal.URL,
															Value: []byte(additionalDataService.URL + "/api/additional_data?data_ids={{ .object.id }}"),
														},
														&datasource.StaticVariableArgument{
															Name:  literal.METHOD,
															Value: []byte("GET"),
														},
													},
													DataSource: &datasource.HttpJsonDataSource{
														Log:    log.NoopLogger,
														Client: datasource.DefaultHttpClient(),
													},
												},
											},
											Fields: []Field{
												{
													Name: []byte("id"),
													Value: &Value{
														DataResolvingConfig: DataResolvingConfig{
															PathSelector: datasource.PathSelector{
																Path: "id",
															},
														},
														ValueType: IntegerValueType,
													},
												},
												{
													Name:            []byte("additional_data_for_id"),
													HasResolvedData: true,
													Value: &Object{
														Fields: []Field{
															{
																Name: []byte("name"),
																Value: &Value{
																	ValueType: StringValueType,
																	DataResolvingConfig: DataResolvingConfig{
																		PathSelector: datasource.PathSelector{
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
					},
				},
			},
		},
	}

	out := &bytes.Buffer{}
	ex := NewExecutor(nil)
	ctx := Context{
		Context: context.Background(),
	}

	err := ex.Execute(ctx, plan, out)
	if err != nil {
		t.Fatal(err)
	}

	want := `{"data":{"preview":{"data":{"id":1,"additional_data_for_id":{"name":"foo"}}}}}`
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
							DataSource: &datasource.StaticDataSource{
								Data: []byte(`{"apis": [{"id": 1},{"id":2}]}`),
							},
						},
						BufferName: "apis",
					},
					Fields: []Field{
						{
							Name:            []byte("apis"),
							HasResolvedData: true,
							Value: &List{
								DataResolvingConfig: DataResolvingConfig{
									PathSelector: datasource.PathSelector{
										Path: "apis",
									},
								},
								Value: &Object{
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
														Path: "id",
													},
												},
												ValueType: IntegerValueType,
											},
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
	ex := NewExecutor(nil)
	ctx := Context{
		Context: context.Background(),
	}

	err := ex.Execute(ctx, plan, out)
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

		_, err := w.Write([]byte(`{
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
							Args: []datasource.Argument{
								&datasource.StaticVariableArgument{
									Name:  literal.URL,
									Value: []byte(graphQL1.URL + "/graphql"),
								},
								&datasource.StaticVariableArgument{
									Name:  literal.QUERY,
									Value: []byte("query q1($code: String!){countries(code: $code){id}}"),
								},
								&datasource.ContextVariableArgument{
									Name:         []byte("code"),
									VariableName: []byte("code"),
								},
							},
							DataSource: &datasource.GraphQLDataSource{
								Log:    log.NoopLogger,
								Client: datasource.DefaultHttpClient(),
							},
						},
						BufferName: "countries",
					},
					Fields: []Field{
						{
							Name:            []byte("countries"),
							HasResolvedData: true,
							Value: &List{
								DataResolvingConfig: DataResolvingConfig{
									PathSelector: datasource.PathSelector{
										Path: "countries",
									},
								},
								Value: &Object{
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
														Path: "id",
													},
												},
												ValueType: IntegerValueType,
											},
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
	ex := NewExecutor(nil)
	ctx := Context{
		Context: context.Background(),
		Variables: map[uint64][]byte{
			xxhash.Sum64String("code"): []byte("DE"),
		},
	}

	err := ex.Execute(ctx, plan, out)
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
							DataSource: &datasource.StaticDataSource{
								Data: []byte(`{"api": {"id": 1}`),
							},
						},
						BufferName: "id",
					},
					Fields: []Field{
						{
							Name:            []byte("id"),
							HasResolvedData: true,
							Value: &Value{
								DataResolvingConfig: DataResolvingConfig{
									PathSelector: datasource.PathSelector{
										Path: "api.id",
									},
								},
								ValueType: IntegerValueType,
							},
						},
					},
				},
			},
		},
	}

	out := &bytes.Buffer{}
	ex := NewExecutor(nil)
	ctx := Context{
		Context: context.Background(),
	}

	err := ex.Execute(ctx, plan, out)
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
	e := NewExecutor(nil)
	e.context = Context{
		Context: context.Background(),
		Variables: map[uint64][]byte{
			xxhash.Sum64String("input"): []byte(`{"foo": "fooValue"}`),
		},
	}

	args := []datasource.Argument{
		&datasource.StaticVariableArgument{
			Name:  []byte("body"),
			Value: []byte("{\\\"key\\\":\\\"{{ .arguments.input.foo }}\\\"}"),
		},
		&datasource.ContextVariableArgument{
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
	e := NewExecutor(nil)
	e.context = Context{
		Context: context.Background(),
		Variables: map[uint64][]byte{
			xxhash.Sum64String("id"): []byte("foo123"),
		},
	}

	args := []datasource.Argument{
		&datasource.StaticVariableArgument{
			Name:  []byte("url"),
			Value: []byte("/apis/{{ .arguments.id }}"),
		},
		&datasource.ContextVariableArgument{
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
	e := NewExecutor(nil)
	e.context = Context{
		Context: context.Background(),
		Variables: map[uint64][]byte{
			xxhash.Sum64String("from"):  []byte(`{"year":2019,"month":11,"day":1}`),
			xxhash.Sum64String("until"): []byte(`{"year":2019,"month":12,"day":31}`),
			xxhash.Sum64String("page"):  []byte(`0`),
		},
	}

	args := []datasource.Argument{
		&datasource.StaticVariableArgument{
			Name:  []byte("url"),
			Value: []byte("/api/usage/apis/{{ .id }}/{{ .arguments.from.day }}/{{ .arguments.from.month }}/{{ .arguments.from.year }}/{{ .arguments.until.day }}/{{ .arguments.until.month }}/{{ .arguments.until.year }}?by=Hits&sort=1&p={{ .arguments.page }}"),
		},
		&datasource.StaticVariableArgument{
			Name:  []byte("id"),
			Value: []byte("1"),
		},
		&datasource.ContextVariableArgument{
			Name:         []byte(".arguments.from"),
			VariableName: []byte("from"),
		},
		&datasource.ContextVariableArgument{
			Name:         []byte(".arguments.until"),
			VariableName: []byte("until"),
		},
		&datasource.ContextVariableArgument{
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
	e := NewExecutor(nil)
	e.context = Context{
		Context: context.Background(),
		Variables: map[uint64][]byte{
			xxhash.Sum64String("input"): []byte(`{"foo": "fooValue", "bar": {"bal": "baz"}}`),
		},
	}

	args := []datasource.Argument{
		&datasource.StaticVariableArgument{
			Name:  []byte("body"),
			Value: []byte("{{ .arguments.input }}"),
		},
		&datasource.ContextVariableArgument{
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
	e := NewExecutor(nil)
	e.context = Context{
		Context: context.Background(),
		Variables: map[uint64][]byte{
			xxhash.Sum64String("input"): []byte(`{"foo": "fooValue", "bar": {"bal": "baz"}}`),
		},
	}

	args := []datasource.Argument{
		&datasource.StaticVariableArgument{
			Name:  []byte("body"),
			Value: []byte("{{ .arguments.input.bar }}"),
		},
		&datasource.ContextVariableArgument{
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
		t.Fatalf("want key 'body' with value: '%s'\ngot: '%s'", want, resolved.ByKey([]byte("body")))
	}
}

func TestExecutor_ResolveArgsFlatObjectContainingJSON(t *testing.T) {
	e := NewExecutor(nil)
	e.context = Context{
		Context: context.Background(),
		Variables: map[uint64][]byte{
			xxhash.Sum64String("request"): []byte(`{"header":{"Authorization":"foo"}}`),
		},
	}

	args := []datasource.Argument{
		&datasource.StaticVariableArgument{
			Name:  []byte("header"),
			Value: []byte("{{ .request.header.Authorization }}"),
		},
		&datasource.ContextVariableArgument{
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
	e := NewExecutor(nil)
	e.context = Context{
		Context: context.Background(),
	}

	args := []datasource.Argument{
		&datasource.ListArgument{
			Name: []byte("headers"),
			Arguments: []datasource.Argument{
				&datasource.StaticVariableArgument{
					Name:  []byte("foo"),
					Value: []byte("fooVal"),
				},
				&datasource.StaticVariableArgument{
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
	createRESTServer := func(wantString string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			if r.Method != http.MethodPost {
				t.Fatalf("wantUpstream: %s, got: %s\n", http.MethodPost, r.Method)
				return
			}

			data, err := io.ReadAll(r.Body)
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
	}

	createPlanForRestServer := func(restServer *httptest.Server, bodyValue string) *Object {
		return &Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							BufferName: "withBody",
							Source: &DataSourceInvocation{
								DataSource: &datasource.HttpJsonDataSource{
									Log:    log.NoopLogger,
									Client: datasource.DefaultHttpClient(),
								},
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte(restServer.URL + "/"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("POST"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("body"),
										Value: []byte(bodyValue),
									},
									&datasource.ContextVariableArgument{
										Name:         []byte(".arguments.input"),
										VariableName: []byte("input"),
									},
								},
							},
						},
						Fields: []Field{
							{
								Name:            []byte("withBody"),
								HasResolvedData: true,
								Value: &Value{
									ValueType: StringValueType,
								},
							},
						},
					},
				},
			},
		}
	}

	run := func(t *testing.T, testServer *httptest.Server, bodyValue string, argumentsInputString string, expectedResult map[string]interface{}) {
		plan := createPlanForRestServer(testServer, bodyValue)

		out := &bytes.Buffer{}
		ex := NewExecutor(nil)
		ctx := Context{
			Context: context.Background(),
			Variables: map[uint64][]byte{
				xxhash.Sum64String("input"): []byte(argumentsInputString),
			},
		}

		err := ex.Execute(ctx, plan, out)
		if err != nil {
			t.Fatal(err)
		}

		wantResult, err := json.MarshalIndent(expectedResult, "", "  ")
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

	t.Run("should successfully use data source with body", func(t *testing.T) {
		wantUpstream := map[string]interface{}{
			"key": "fooValue",
		}
		wantBytes, err := json.MarshalIndent(wantUpstream, "", "  ")
		if err != nil {
			t.Fatal(err)
			return
		}

		expectedResult := map[string]interface{}{
			"data": map[string]interface{}{
				"withBody": "bar",
			},
		}

		wantString := string(wantBytes)
		restServer := createRESTServer(wantString)
		defer restServer.Close()

		run(t, restServer, `{"key":"{{ .arguments.input.foo }}"}`, `{"foo": "fooValue"}`, expectedResult)
	})

	t.Run("should successfully use data source with body including json object as value in json object argument", func(t *testing.T) {
		wantUpstream := map[string]interface{}{
			"key": "{ \"obj_key\": \"obj_value\" }",
		}
		wantBytes, err := json.MarshalIndent(wantUpstream, "", "  ")
		if err != nil {
			t.Fatal(err)
			return
		}

		expectedResult := map[string]interface{}{
			"data": map[string]interface{}{
				"withBody": "bar",
			},
		}

		wantString := string(wantBytes)
		restServer := createRESTServer(wantString)
		defer restServer.Client()

		run(t, restServer, `{ "key": "{{ .arguments.input.foo }}" }`, `{"foo": "{ \"obj_key\": \"obj_value\" }"}`, expectedResult)
	})

	t.Run("should successfully use data source with body including complex json object with escaped strings", func(t *testing.T) {
		wantUpstream := map[string]interface{}{
			"meta_data": map[string]interface{}{
				"test": "{\"foo\": \"bar\", \"re\":\"\\w+\"}",
			},
		}
		wantBytes, err := json.MarshalIndent(wantUpstream, "", "  ")
		if err != nil {
			t.Fatal(err)
			return
		}

		expectedResult := map[string]interface{}{
			"data": map[string]interface{}{
				"withBody": "bar",
			},
		}

		wantString := string(wantBytes)
		restServer := createRESTServer(wantString)
		defer restServer.Client()

		run(t, restServer, `{{ .arguments.input.query }}`, `{"query": { "meta_data": { "test": "{\"foo\": \"bar\", \"re\":\"\\w+\"}" } }`, expectedResult)
	})

}

func TestExecutor_Execute_WithUnions(t *testing.T) {

	planApi := func(apiResponse string, succesFirst bool) *Object {

		successFields := []Field{
			{
				Name: []byte("id"),
				Skip: &IfNotEqual{
					Left: &datasource.ObjectVariableArgument{
						PathSelector: datasource.PathSelector{
							Path: "__typename",
						},
					},
					Right: &datasource.StaticVariableArgument{
						Value: []byte("Api"),
					},
				},
				Value: &Value{
					ValueType: StringValueType,
					DataResolvingConfig: DataResolvingConfig{
						PathSelector: datasource.PathSelector{
							Path: "id",
						},
					},
				},
			},
			{
				Name: []byte("name"),
				Skip: &IfNotEqual{
					Left: &datasource.ObjectVariableArgument{
						PathSelector: datasource.PathSelector{
							Path: "__typename",
						},
					},
					Right: &datasource.StaticVariableArgument{
						Value: []byte("Api"),
					},
				},
				Value: &Value{
					ValueType: StringValueType,
					DataResolvingConfig: DataResolvingConfig{
						PathSelector: datasource.PathSelector{
							Path: "name",
						},
					},
				},
			},
		}

		errorFields := []Field{
			{
				Name: []byte("status"),
				Skip: &IfNotEqual{
					Left: &datasource.ObjectVariableArgument{
						PathSelector: datasource.PathSelector{
							Path: "__typename",
						},
					},
					Right: &datasource.StaticVariableArgument{
						Value: []byte("RequestResult"),
					},
				},
				Value: &Value{
					ValueType: StringValueType,
					DataResolvingConfig: DataResolvingConfig{
						PathSelector: datasource.PathSelector{
							Path: "Status",
						},
					},
				},
			},
			{
				Name: []byte("message"),
				Skip: &IfNotEqual{
					Left: &datasource.ObjectVariableArgument{
						PathSelector: datasource.PathSelector{
							Path: "__typename",
						},
					},
					Right: &datasource.StaticVariableArgument{
						Value: []byte("RequestResult"),
					},
				},
				Value: &Value{
					ValueType: StringValueType,
					DataResolvingConfig: DataResolvingConfig{
						PathSelector: datasource.PathSelector{
							Path: "Message",
						},
					},
				},
			},
		}

		var fields []Field
		if succesFirst {
			fields = append(successFields, errorFields...)
		} else {
			fields = append(errorFields, successFields...)
		}

		return &Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								DataSource: FakeDataSource{
									data: []byte(apiResponse),
								},
							},
							BufferName: "api",
						},
						Fields: []Field{
							{
								Name:            []byte("api"),
								HasResolvedData: true,
								Value: &Object{
									Fields: fields,
								},
							},
						},
					},
				},
			},
		}
	}

	planApis := func(apiResponse string, succesFirst bool) *Object {
		successField := Field{
			Name: []byte("apis"),
			Skip: &IfNotEqual{
				Left: &datasource.ObjectVariableArgument{
					PathSelector: datasource.PathSelector{
						Path: "__typename",
					},
				},
				Right: &datasource.StaticVariableArgument{
					Value: []byte("ApisResultSuccess"),
				},
			},
			Value: &List{
				DataResolvingConfig: DataResolvingConfig{
					PathSelector: datasource.PathSelector{
						Path: "apis",
					},
				},
				Value: &Object{
					Fields: []Field{
						{
							Name: []byte("name"),
							Value: &Value{
								DataResolvingConfig: DataResolvingConfig{
									PathSelector: datasource.PathSelector{
										Path: "name",
									},
								},
								ValueType: StringValueType,
							},
						},
					},
				},
			},
		}

		errorFields := []Field{
			{
				Name: []byte("status"),
				Skip: &IfNotEqual{
					Left: &datasource.ObjectVariableArgument{
						PathSelector: datasource.PathSelector{
							Path: "__typename",
						},
					},
					Right: &datasource.StaticVariableArgument{
						Value: []byte("RequestResult"),
					},
				},
				Value: &Value{
					ValueType: StringValueType,
					DataResolvingConfig: DataResolvingConfig{
						PathSelector: datasource.PathSelector{
							Path: "Status",
						},
					},
				},
			},
			{
				Name: []byte("message"),
				Skip: &IfNotEqual{
					Left: &datasource.ObjectVariableArgument{
						PathSelector: datasource.PathSelector{
							Path: "__typename",
						},
					},
					Right: &datasource.StaticVariableArgument{
						Value: []byte("RequestResult"),
					},
				},
				Value: &Value{
					ValueType: StringValueType,
					DataResolvingConfig: DataResolvingConfig{
						PathSelector: datasource.PathSelector{
							Path: "Message",
						},
					},
				},
			},
		}

		var fields []Field
		if succesFirst {
			fields = append(fields, successField, errorFields[0], errorFields[1])
		} else {
			fields = append(fields, errorFields[0], errorFields[1], successField)
		}

		return &Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								DataSource: FakeDataSource{
									data: []byte(apiResponse),
								},
							},
							BufferName: "apis",
						},
						Fields: []Field{
							{
								Name:            []byte("apis"),
								HasResolvedData: true,
								Value: &Object{
									Fields: fields,
								},
							},
						},
					},
				},
			},
		}
	}

	makeTest := func(apiResponse, wantResult string, planner func(apiResponse string, successFirst bool) *Object) func(t *testing.T) {
		return func(t *testing.T) {
			out := &bytes.Buffer{}
			ex := NewExecutor(nil)
			ctx := Context{
				Context: context.Background(),
			}
			err := ex.Execute(ctx, planner(apiResponse, true), out)
			if err != nil {
				panic(err)
			}

			successFirst := out.String()
			out.Reset()
			err = ex.Execute(ctx, planner(apiResponse, false), out)
			if err != nil {
				panic(err)
			}
			errorFirst := out.String()
			if successFirst != errorFirst {
				panic(fmt.Errorf("successFirst: %s\nerrorFirst: %s\n", successFirst, errorFirst))
			}
			if wantResult != successFirst {
				panic(fmt.Errorf("want: %s\n got: %s\n", wantResult, successFirst))
			}
		}
	}

	t.Run("list response error case", makeTest(
		`{"__typename":"RequestResult","Status":"Error","Message":"Could not retrieve Apis details"}`,
		`{"data":{"apis":{"status":"Error","message":"Could not retrieve Apis details"}}}`, planApis),
	)
	t.Run("list response valid response", makeTest(
		`{"__typename":"ApisResultSuccess","apis":[{"id":"1","name":"a", "__typename":"Api"},{"id":"2","name":"b", "__typename":"Api"}]}`,
		`{"data":{"apis":{"apis":[{"name":"a"},{"name":"b"}]}}}`, planApis),
	)
	t.Run("object response valid response", makeTest(
		`{"id":"1","name":"a", "__typename":"Api"}}`,
		`{"data":{"api":{"id":"1","name":"a"}}}`, planApi),
	)
	t.Run("object response error response", makeTest(
		`{"__typename":"RequestResult","Status":"Error","Message":"Could not retrieve Api detail","Meta":null}`,
		`{"data":{"api":{"status":"Error","message":"Could not retrieve Api detail"}}}`, planApi))
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

		data, err := io.ReadAll(r.Body)
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
							DataSource: &datasource.HttpJsonDataSource{
								Log:    log.NoopLogger,
								Client: datasource.DefaultHttpClient(),
							},
							Args: []datasource.Argument{
								&datasource.StaticVariableArgument{
									Name:  []byte("url"),
									Value: []byte(REST1.URL + "/"),
								},
								&datasource.StaticVariableArgument{
									Name:  []byte("method"),
									Value: []byte("POST"),
								},
								&datasource.StaticVariableArgument{
									Name:  []byte("body"),
									Value: []byte("{{ .arguments.input }}"),
								},
								&datasource.ContextVariableArgument{
									Name:         []byte(".arguments.input"),
									VariableName: []byte("input"),
								},
							},
						},
					},
					Fields: []Field{
						{
							Name:            []byte("withBody"),
							HasResolvedData: true,
							Value: &Value{
								ValueType: StringValueType,
							},
						},
					},
				},
			},
		},
	}

	out := &bytes.Buffer{}
	ex := NewExecutor(nil)
	ctx := Context{
		Context: context.Background(),
		Variables: map[uint64][]byte{
			xxhash.Sum64String("input"): []byte(`{"foo": "fooValue", "bar": {"bal": "baz"}}`),
		},
	}

	err = ex.Execute(ctx, plan, out)
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

func TestExecutor_HTTPJSONDataSource_ArrayResponse(t *testing.T) {

	response := []interface{}{
		map[string]interface{}{
			"fieldValue": "foo",
		},
		map[string]interface{}{
			"fieldValue": "bar",
		},
		map[string]interface{}{
			"fieldValue": "baz",
		},
	}

	REST1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		responseData, err := json.Marshal(response)
		assert.NoError(t, err)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(responseData)
	}))

	plan := &Object{
		operationType: ast.OperationTypeQuery,
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fetch: &SingleFetch{
						BufferName: "objects",
						Source: &DataSourceInvocation{
							DataSource: &datasource.HttpJsonDataSource{
								Log:    log.NoopLogger,
								Client: datasource.DefaultHttpClient(),
							},
							Args: []datasource.Argument{
								&datasource.StaticVariableArgument{
									Name:  []byte("url"),
									Value: []byte(REST1.URL + "/"),
								},
								&datasource.StaticVariableArgument{
									Name:  []byte("method"),
									Value: []byte("GET"),
								},
								&datasource.StaticVariableArgument{
									Name:  []byte("__typename"),
									Value: []byte(`{"defaultTypeName":"SimpleType"}`),
								},
							},
						},
					},
					Fields: []Field{
						{
							Name:            []byte("objects"),
							HasResolvedData: true,
							Value: &List{
								Value: &Object{
									Fields: []Field{
										{
											Name: []byte("__typename"),
											Value: &Value{
												ValueType: StringValueType,
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
														Path: "__typename",
													},
												},
											},
										},
										{
											Name: []byte("fieldValue"),
											Value: &Value{
												ValueType: StringValueType,
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
														Path: "fieldValue",
													},
												},
											},
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
	ex := NewExecutor(nil)
	ctx := Context{
		Context: context.Background(),
	}

	err := ex.Execute(ctx, plan, out)
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]interface{}{
		"data": map[string]interface{}{
			"objects": []interface{}{
				map[string]interface{}{
					"__typename": "SimpleType",
					"fieldValue": "foo",
				},
				map[string]interface{}{
					"__typename": "SimpleType",
					"fieldValue": "bar",
				},
				map[string]interface{}{
					"__typename": "SimpleType",
					"fieldValue": "baz",
				},
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
							DataSource: &datasource.HttpJsonDataSource{
								Log:    log.NoopLogger,
								Client: datasource.DefaultHttpClient(),
							},
							Args: []datasource.Argument{
								&datasource.StaticVariableArgument{
									Name:  []byte("url"),
									Value: []byte(REST1.URL + "/"),
								},
								&datasource.StaticVariableArgument{
									Name:  []byte("method"),
									Value: []byte("GET"),
								},
								&datasource.ListArgument{
									Name: []byte("headers"),
									Arguments: []datasource.Argument{
										&datasource.StaticVariableArgument{
											Name:  []byte("foo"),
											Value: []byte("fooVal"),
										},
										&datasource.StaticVariableArgument{
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
							Name:            []byte("withHeaders"),
							HasResolvedData: true,
							Value: &Value{
								ValueType: StringValueType,
							},
						},
					},
				},
			},
		},
	}

	out := &bytes.Buffer{}
	ex := NewExecutor(nil)
	ctx := Context{
		Context: context.Background(),
	}

	err := ex.Execute(ctx, plan, out)
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
							DataSource: &datasource.HttpJsonDataSource{
								Log:    log.NoopLogger,
								Client: datasource.DefaultHttpClient(),
							},
							Args: []datasource.Argument{
								&datasource.StaticVariableArgument{
									Name:  []byte("url"),
									Value: []byte(REST1.URL + "/"),
								},
								&datasource.StaticVariableArgument{
									Name:  []byte("method"),
									Value: []byte("GET"),
								},
							},
						},
					},
					Fields: []Field{
						{
							Name:            []byte("friends"),
							HasResolvedData: true,
							Value: &Object{
								Fields: []Field{
									{
										Name: []byte("firstNames"),
										Value: &List{
											DataResolvingConfig: DataResolvingConfig{
												PathSelector: datasource.PathSelector{
													Path: "friends.#.first",
												},
											},
											Value: &Value{
												ValueType: StringValueType,
											},
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
	ex := NewExecutor(nil)
	ctx := Context{
		Context: context.Background(),
	}

	err := ex.Execute(ctx, plan, out)
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
	var data interface{}
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
	executor := NewExecutor(nil)
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

	config := datasource.PlannerConfiguration{
		TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
			{
				TypeName:  "query",
				FieldName: "__schema",
				DataSource: datasource.SourceConfig{
					Name:   "SchemaDataSource",
					Config: toJSON(datasource.SchemaDataSourcePlannerConfig{}),
				},
			},
		},
	}

	base, err := datasource.NewBaseDataSourcePlanner(schema, config, log.NoopLogger)
	if err != nil {
		t.Fatal(err)
	}

	err = base.RegisterDataSourcePlannerFactory("SchemaDataSource", datasource.SchemaDataSourcePlannerFactoryFactory{})
	if err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(base, nil)

	gen := introspection.NewGenerator()
	report := operationreport.Report{}
	data := introspection.Data{}
	gen.Generate(handler.base.Definition, &report, &data)

	introspectionData, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}

	out := bytes.Buffer{}
	err = executor.Execute(ctx, introspectionQuery(introspectionData), &out)
	if err != nil {
		t.Fatal(err)
	}

	response := out.Bytes()

	goldie.Assert(t, "introspection_execution", response)
	if t.Failed() {
		fixture, err := os.ReadFile("./fixtures/introspection_execution.golden")
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, fixture, response)
	}
}

func TestIsJSONObjectAsBytes(t *testing.T) {
	run := func(inputString string, expectedResult bool) (string, func(t *testing.T)) {
		return fmt.Sprintf("%s is %v", inputString, expectedResult), func(t *testing.T) {
			inputBytes := []byte(inputString)
			result := isJSONObjectAsBytes(inputBytes)
			assert.Equal(t, expectedResult, result)
		}
	}

	t.Run(run("hello", false))
	t.Run(run("1", false))
	t.Run(run("query { meow }", false))
	t.Run(run(": - }", false))
	t.Run(run("{ - )", false))
	t.Run(run("{}", true))
	t.Run(run("   {}", true))
	t.Run(run("{}   ", true))
	t.Run(run("{\"hello\": \"world\"}", true))
	t.Run(run(`{"hello": "world"}`, true))
}

func Test_byteSliceContainsEscapedQuotes(t *testing.T) {
	run := func(inputString string, expectedResult bool) (string, func(t *testing.T)) {
		return fmt.Sprintf("%s is %v", inputString, expectedResult), func(t *testing.T) {
			inputBytes := []byte(inputString)
			result := byteSliceContainsEscapedQuotes(inputBytes)
			assert.Equal(t, expectedResult, result)
		}
	}

	t.Run(run(`"my dog is a string"`, false))
	t.Run(run(`[1, 2, 3, 4]`, false))
	t.Run(run(`{"key": "value"}`, false))

	t.Run(run(`"the name of my dog is \"Hans\""`, true))
	t.Run(run(`{"test": "{foo: \"bar\"}"}`, true))
}

func Test_byteSliceContainsQuotes(t *testing.T) {
	run := func(inputString string, expectedResult bool) (string, func(t *testing.T)) {
		return fmt.Sprintf("%s is %v", inputString, expectedResult), func(t *testing.T) {
			inputBytes := []byte(inputString)
			result := byteSliceContainsQuotes(inputBytes)
			assert.Equal(t, expectedResult, result)
		}
	}

	t.Run(run(`my dog is a string`, false))
	t.Run(run(`[1, 2, 3, 4]`, false))

	t.Run(run(`the name of my dog is "Hans"`, true))
}
