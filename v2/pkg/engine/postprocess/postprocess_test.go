package postprocess

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/kylelemons/godebug/pretty"
	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestProcess_ExtractFetches(t *testing.T) {
	type TestCase struct {
		name     string
		pre      plan.Plan
		expected plan.Plan
	}

	cases := []TestCase{
		{
			name: "1",
			pre: &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("field1"),
								Value: &resolve.String{
									Path: []string{"field1"},
								},
							},
						},
						Fetch: &resolve.MultiFetch{
							Fetches: []*resolve.SingleFetch{
								{FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
								{FetchDependencies: resolve.FetchDependencies{FetchID: 2}},
								{FetchDependencies: resolve.FetchDependencies{FetchID: 3}},
							},
						},
					},
				},
			},
			expected: &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("field1"),
								Value: &resolve.String{
									Path: []string{"field1"},
								},
							},
						},
					},
					FetchTree: &resolve.Object{
						Nullable: true,
						Fetch: &resolve.MultiFetch{
							Fetches: []*resolve.SingleFetch{
								{FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
								{FetchDependencies: resolve.FetchDependencies{FetchID: 2}},
								{FetchDependencies: resolve.FetchDependencies{FetchID: 3}},
							},
						},
					},
				},
			},
		},
		{
			name: "2",
			pre: &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("obj"),
								Value: &resolve.Object{
									Path: []string{"obj"},
									Fields: []*resolve.Field{
										{
											Name: []byte("field1"),
											Value: &resolve.String{
												Path: []string{"field1"},
											},
										},
									},
									Fetch: &resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2}},
								},
							},
							{
								Name: []byte("obj"),
								Value: &resolve.Object{
									Path: []string{"obj"},
									Fields: []*resolve.Field{
										{
											Name: []byte("field1"),
											Value: &resolve.String{
												Path: []string{"field1"},
											},
										},
									},
									Fetch: &resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3}},
								},
							},
						},
						Fetch: &resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
					},
				},
			},
			expected: &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("obj"),
								Value: &resolve.Object{
									Path: []string{"obj"},
									Fields: []*resolve.Field{
										{
											Name: []byte("field1"),
											Value: &resolve.String{
												Path: []string{"field1"},
											},
										},
									},
								},
							},
							{
								Name: []byte("obj"),
								Value: &resolve.Object{
									Path: []string{"obj"},
									Fields: []*resolve.Field{
										{
											Name: []byte("field1"),
											Value: &resolve.String{
												Path: []string{"field1"},
											},
										},
									},
								},
							},
						},
					},
					FetchTree: &resolve.Object{
						Nullable: true,
						Fields: []*resolve.Field{
							{
								Name: []byte("obj"),
								Value: &resolve.Object{
									Nullable: true,
									Path:     []string{"obj"},
									Fetch: &resolve.MultiFetch{
										Fetches: []*resolve.SingleFetch{
											{FetchDependencies: resolve.FetchDependencies{FetchID: 2}},
											{FetchDependencies: resolve.FetchDependencies{FetchID: 3}},
										},
									},
								},
							},
						},
						Fetch: &resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
					},
				},
			},
		},
		{
			name: "3",
			pre: &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("objects"),
								Value: &resolve.Array{
									Nullable: true,
									Path:     []string{"objects"},
									Item: &resolve.Object{
										Path: []string{"obj"},
										Fields: []*resolve.Field{
											{
												Name: []byte("field1"),
												Value: &resolve.String{
													Path: []string{"field1"},
												},
											},
										},
										Fetch: &resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2}},
									},
								},
							},
							{
								Name: []byte("objects"),
								Value: &resolve.Array{
									Nullable: true,
									Path:     []string{"objects"},
									Item: &resolve.Object{
										Path: []string{"obj"},
										Fields: []*resolve.Field{
											{
												Name: []byte("field1"),
												Value: &resolve.String{
													Path: []string{"field1"},
												},
											},
										},
										Fetch: &resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3}},
									},
								},
							},
						},
						Fetch: &resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
					},
				},
			},
			expected: &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("objects"),
								Value: &resolve.Array{
									Nullable: true,
									Path:     []string{"objects"},
									Item: &resolve.Object{
										Path: []string{"obj"},
										Fields: []*resolve.Field{
											{
												Name: []byte("field1"),
												Value: &resolve.String{
													Path: []string{"field1"},
												},
											},
										},
									},
								},
							},
							{
								Name: []byte("objects"),
								Value: &resolve.Array{
									Nullable: true,
									Path:     []string{"objects"},
									Item: &resolve.Object{
										Path: []string{"obj"},
										Fields: []*resolve.Field{
											{
												Name: []byte("field1"),
												Value: &resolve.String{
													Path: []string{"field1"},
												},
											},
										},
									},
								},
							},
						},
					},
					FetchTree: &resolve.Object{
						Nullable: true,
						Fields: []*resolve.Field{
							{
								Name: []byte("objects"),
								Value: &resolve.Array{
									Nullable: true,
									Path:     []string{"objects"},
									Item: &resolve.Object{
										Nullable: true,
										Path:     []string{"obj"},
										Fetch: &resolve.MultiFetch{
											Fetches: []*resolve.SingleFetch{
												{FetchDependencies: resolve.FetchDependencies{FetchID: 2}},
												{FetchDependencies: resolve.FetchDependencies{FetchID: 3}},
											},
										},
									},
								},
							},
						},
						Fetch: &resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
					},
				},
			},
		},
	}

	processor := &Processor{enableExtractFetches: true}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := processor.Process(c.pre)

			if !assert.Equal(t, c.expected, actual) {
				formatterConfig := map[reflect.Type]interface{}{
					reflect.TypeOf([]byte{}): func(b []byte) string { return fmt.Sprintf(`"%s"`, string(b)) },
				}

				prettyCfg := &pretty.Config{
					Diffable:          true,
					IncludeUnexported: false,
					Formatter:         formatterConfig,
				}

				if diff := prettyCfg.Compare(c.expected, actual); diff != "" {
					t.Errorf("Plan does not match(-want +got)\n%s", diff)
				}
			}
		})
	}
}
