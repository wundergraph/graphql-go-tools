package execution

import (
	"bytes"
	"fmt"
	"github.com/cespare/xxhash"
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
				Name: []byte("__type"),
				Resolve: &Resolve{
					Args: []Argument{
						{
							Name:         []byte("name"),
							VariableName: []byte("name"),
						},
					},
					Resolver: &TypeResolver{},
				},
				Value: &Object{
					Path: []byte("__type"),
					Fields: []Field{
						{
							Name: []byte("name"),
							Value: &Value{
								Path: []byte("name"),
							},
						},
						{
							Name: []byte("fields"),
							Value: &List{
								Path: []byte("fields"),
								Value: &Object{
									Fields: []Field{
										{
											Name: []byte("name"),
											Value: &Value{
												Path: []byte("name"),
											},
										},
										{
											Name: []byte("type"),
											Value: &Object{
												Path: []byte("type"),
												Fields: []Field{
													{
														Name: []byte("name"),
														Value: &Value{
															Path: []byte("name"),
														},
													},
												},
											},
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

	fmt.Printf("Result:\n%s\n", out.String())
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
		b.Run(fmt.Sprintf("size: %d", size), func(b *testing.B) {
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
		Name: []byte("__type"),
		Resolve: &Resolve{
			Args: []Argument{
				{
					Name:         []byte("name"),
					VariableName: []byte("name"),
				},
			},
			Resolver: &TypeResolver{},
		},
		Value: &Object{
			Path: []byte("__type"),
			Fields: []Field{
				{
					Name: []byte("name"),
					Value: &Value{
						Path: []byte("name"),
					},
				},
				{
					Name: []byte("fields"),
					Value: &List{
						Path: []byte("fields"),
						Value: &Object{
							Fields: []Field{
								{
									Name: []byte("name"),
									Value: &Value{
										Path: []byte("name"),
									},
								},
								{
									Name: []byte("type"),
									Value: &Object{
										Path: []byte("type"),
										Fields: []Field{
											{
												Name: []byte("name"),
												Value: &Value{
													Path: []byte("name"),
												},
											},
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
