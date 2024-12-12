package postprocess

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"

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
						Fetches: []resolve.Fetch{
							&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
							&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2}},
							&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3}},
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}}),
						resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2}}),
						resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3}}),
					),
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
									Fetches: []resolve.Fetch{
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2}},
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
									Fetches: []resolve.Fetch{
										&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3}},
									},
								},
							},
						},
						Fetches: []resolve.Fetch{
							&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
						},
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}}),
						resolve.SingleWithPath(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2}}, "obj", resolve.ObjectPath("obj")),
						resolve.SingleWithPath(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3}}, "obj", resolve.ObjectPath("obj")),
					),
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
										Fetches: []resolve.Fetch{
											&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{1}}},
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
										Fetches: []resolve.Fetch{
											&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3, DependsOnFetchIDs: []int{1}}},
										},
									},
								},
							},
						},
						Fetches: []resolve.Fetch{
							&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
						},
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}}),
						resolve.SingleWithPath(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{1}}}, "objects", resolve.ArrayPath("objects"), resolve.ObjectPath("obj")),
						resolve.SingleWithPath(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 3, DependsOnFetchIDs: []int{1}}}, "objects", resolve.ArrayPath("objects"), resolve.ObjectPath("obj")),
					),
				},
			},
		},
		{
			name: "4",
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
												Value: &resolve.Object{
													Path: []string{"field1"},
													Fetches: []resolve.Fetch{
														&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{1}}},
													},
													Fields: []*resolve.Field{
														{
															Name: []byte("nestedField1"),
															Value: &resolve.String{
																Path: []string{"nestedField1"},
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
												Value: &resolve.Object{
													Path: []string{"field1"},
													Fields: []*resolve.Field{
														{
															Name: []byte("nestedField1"),
															Value: &resolve.String{
																Path: []string{"nestedField1"},
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
					Fetches: resolve.Sequence(
						resolve.SingleWithPath(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{1}}}, "objects.@.field1", resolve.ArrayPath("objects"), resolve.ObjectPath("obj"), resolve.ObjectPath("field1")),
					),
				},
			},
		},
	}

	processor := NewProcessor(
		DisableDeduplicateSingleFetches(),
		DisableCreateConcreteSingleFetchTypes(),
		DisableMergeFields(),
		DisableCreateParallelNodes(),
		DisableAddMissingNestedDependencies(),
	)

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

func TestProcess_ExtractServiceNames(t *testing.T) {
	type TestCase struct {
		name     string
		pre      plan.Plan
		expected plan.Plan
	}

	cases := []TestCase{
		{
			name: "Collect all service names",
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
						Fetches: []resolve.Fetch{
							&resolve.SingleFetch{
								Info: &resolve.FetchInfo{
									DataSourceID:   "user-service",
									DataSourceName: "user-service",
									OperationType:  ast.OperationTypeQuery,
									RootFields: []resolve.GraphCoordinate{
										{
											TypeName:  "Query",
											FieldName: "user",
										},
									},
								},
								FetchDependencies: resolve.FetchDependencies{FetchID: 1},
							},
							&resolve.SingleFetch{
								Info: &resolve.FetchInfo{
									DataSourceID:   "product-service",
									DataSourceName: "product-service",
									OperationType:  ast.OperationTypeQuery,
									RootFields: []resolve.GraphCoordinate{
										{
											TypeName:  "Query",
											FieldName: "product",
										},
									},
								},
								FetchDependencies: resolve.FetchDependencies{FetchID: 2},
							},
							&resolve.SingleFetch{
								Info: &resolve.FetchInfo{
									DataSourceID:   "review-service",
									DataSourceName: "review-service",
									OperationType:  ast.OperationTypeQuery,
									RootFields: []resolve.GraphCoordinate{
										{
											TypeName:  "Query",
											FieldName: "review",
										},
									},
								},
								FetchDependencies: resolve.FetchDependencies{FetchID: 3}},
						},
					},
				},
			},
			expected: &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					DataSources: []resolve.DataSourceInfo{
						{ID: "user-service", Name: "user-service"},
						{ID: "product-service", Name: "product-service"},
						{ID: "review-service", Name: "review-service"},
					},
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
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							Info: &resolve.FetchInfo{
								DataSourceID:   "user-service",
								DataSourceName: "user-service",
								OperationType:  ast.OperationTypeQuery,
								RootFields: []resolve.GraphCoordinate{
									{
										TypeName:  "Query",
										FieldName: "user",
									},
								},
							},
							FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
						),
						resolve.Single(
							&resolve.SingleFetch{
								Info: &resolve.FetchInfo{
									DataSourceID:   "product-service",
									DataSourceName: "product-service",
									OperationType:  ast.OperationTypeQuery,
									RootFields: []resolve.GraphCoordinate{
										{
											TypeName:  "Query",
											FieldName: "product",
										},
									},
								},
								FetchDependencies: resolve.FetchDependencies{FetchID: 2},
							},
						),
						resolve.Single(
							&resolve.SingleFetch{
								Info: &resolve.FetchInfo{
									DataSourceID:   "review-service",
									DataSourceName: "review-service",
									OperationType:  ast.OperationTypeQuery,
									RootFields: []resolve.GraphCoordinate{
										{
											TypeName:  "Query",
											FieldName: "review",
										},
									},
								},
								FetchDependencies: resolve.FetchDependencies{FetchID: 3},
							},
						),
					),
				},
			},
		},
		{
			name: "Deduplicate the same service names",
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
						Fetches: []resolve.Fetch{
							&resolve.SingleFetch{
								Info: &resolve.FetchInfo{
									DataSourceID:   "product-service-1",
									DataSourceName: "product-service-1",
									OperationType:  ast.OperationTypeQuery,
									RootFields: []resolve.GraphCoordinate{
										{
											TypeName:  "Query",
											FieldName: "product",
										},
									},
								},
								FetchDependencies: resolve.FetchDependencies{FetchID: 1},
							},
							&resolve.SingleFetch{
								Info: &resolve.FetchInfo{
									DataSourceID:   "product-service",
									DataSourceName: "product-service",
									OperationType:  ast.OperationTypeQuery,
									RootFields: []resolve.GraphCoordinate{
										{
											TypeName:  "Query",
											FieldName: "product",
										},
									},
								},
								FetchDependencies: resolve.FetchDependencies{FetchID: 2},
							},
							&resolve.SingleFetch{
								Info: &resolve.FetchInfo{
									DataSourceID:   "product-service-1",
									DataSourceName: "product-service-1",
									OperationType:  ast.OperationTypeQuery,
									RootFields: []resolve.GraphCoordinate{
										{
											TypeName:  "Query",
											FieldName: "products",
										},
									},
								},
								FetchDependencies: resolve.FetchDependencies{FetchID: 3},
							},
						},
					},
				},
			},
			expected: &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					DataSources: []resolve.DataSourceInfo{
						{ID: "product-service-1", Name: "product-service-1"},
						{ID: "product-service", Name: "product-service"},
					},
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
					Fetches: resolve.Sequence(
						resolve.Single(
							&resolve.SingleFetch{
								Info: &resolve.FetchInfo{
									DataSourceID:   "product-service-1",
									DataSourceName: "product-service-1",
									OperationType:  ast.OperationTypeQuery,
									RootFields: []resolve.GraphCoordinate{
										{
											TypeName:  "Query",
											FieldName: "product",
										},
									},
								},
								FetchDependencies: resolve.FetchDependencies{FetchID: 1},
							},
						),
						resolve.Single(
							&resolve.SingleFetch{
								Info: &resolve.FetchInfo{
									DataSourceID:   "product-service",
									DataSourceName: "product-service",
									OperationType:  ast.OperationTypeQuery,
									RootFields: []resolve.GraphCoordinate{
										{
											TypeName:  "Query",
											FieldName: "product",
										},
									},
								},
								FetchDependencies: resolve.FetchDependencies{FetchID: 2},
							},
						),
						resolve.Single(
							&resolve.SingleFetch{
								Info: &resolve.FetchInfo{
									DataSourceID:   "product-service-1",
									DataSourceName: "product-service-1",
									OperationType:  ast.OperationTypeQuery,
									RootFields: []resolve.GraphCoordinate{
										{
											TypeName:  "Query",
											FieldName: "products",
										},
									},
								},
								FetchDependencies: resolve.FetchDependencies{FetchID: 3},
							},
						),
					),
				},
			},
		},
	}

	processor := NewProcessor(
		DisableDeduplicateSingleFetches(),
		DisableCreateConcreteSingleFetchTypes(),
		DisableMergeFields(),
		DisableCreateParallelNodes(),
		DisableAddMissingNestedDependencies(),
		CollectDataSourceInfo(),
	)

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
