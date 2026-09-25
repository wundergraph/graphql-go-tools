package postprocess

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// protectedProductsData is a data tree with one protected nested field: Product.secret, resolved by
// the "products" data source, under Query.products.
func protectedProductsData() *resolve.Object {
	return &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name: []byte("products"),
				Info: &resolve.FieldInfo{
					Name:                "products",
					ExactParentTypeName: "Query",
					Source:              resolve.TypeFieldSource{IDs: []string{"products"}, Names: []string{"products"}},
				},
				Value: &resolve.Array{
					Path:     []string{"products"},
					Nullable: true,
					Item: &resolve.Object{
						Nullable: true,
						TypeName: "Product",
						Fields: []*resolve.Field{
							{
								Name: []byte("secret"),
								Info: &resolve.FieldInfo{
									Name:                 "secret",
									ExactParentTypeName:  "Product",
									Source:               resolve.TypeFieldSource{IDs: []string{"products"}, Names: []string{"products"}},
									HasAuthorizationRule: true,
								},
								Value: &resolve.String{Path: []string{"secret"}, Nullable: true},
							},
						},
					},
				},
			},
		},
	}
}

func TestCollectAuthorizationCoordinates_FlatFetchTreeAndDataTree(t *testing.T) {
	response := &resolve.GraphQLResponse{
		Info: &resolve.GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: resolve.Sequence(
			resolve.SingleWithPath(&resolve.SingleFetch{
				Info: &resolve.FetchInfo{
					DataSourceID: "catalog",
					RootFields: []resolve.GraphCoordinate{
						{TypeName: "Query", FieldName: "products", HasAuthorizationRule: true},
						{TypeName: "Query", FieldName: "public"},
					},
				},
			}, "query"),
		),
		Data: protectedProductsData(),
	}

	(&collectAuthorizationCoordinates{}).Process(response)

	assert.Equal(t, []resolve.AuthorizationCoordinate{
		{DataSourceID: "catalog", Coordinate: resolve.GraphCoordinate{TypeName: "Query", FieldName: "products"}},
		{DataSourceID: "products", Coordinate: resolve.GraphCoordinate{TypeName: "Product", FieldName: "secret"}},
	}, response.Info.AuthorizationCoordinates)
}

// When fetch extraction has not run (or is disabled), the fetches still live in RawFetches; the
// collector covers that container too.
func TestCollectAuthorizationCoordinates_RawFetches(t *testing.T) {
	response := &resolve.GraphQLResponse{
		Info: &resolve.GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		RawFetches: []*resolve.FetchItem{
			{Fetch: &resolve.SingleFetch{
				Info: &resolve.FetchInfo{
					DataSourceID: "catalog",
					RootFields: []resolve.GraphCoordinate{
						{TypeName: "Query", FieldName: "products", HasAuthorizationRule: true},
						{TypeName: "Query", FieldName: "public"},
					},
				},
			}},
		},
	}

	(&collectAuthorizationCoordinates{}).Process(response)

	assert.Equal(t, []resolve.AuthorizationCoordinate{
		{DataSourceID: "catalog", Coordinate: resolve.GraphCoordinate{TypeName: "Query", FieldName: "products"}},
	}, response.Info.AuthorizationCoordinates)
}

func TestCollectAuthorizationCoordinates_DeduplicatesFetchAndDataTree(t *testing.T) {
	// The same coordinate reachable via a fetch root field and via the data tree yields one entry;
	// a @shareable field with several data sources yields one entry per source.
	response := &resolve.GraphQLResponse{
		Info: &resolve.GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		RawFetches: []*resolve.FetchItem{
			{Fetch: &resolve.SingleFetch{
				Info: &resolve.FetchInfo{
					DataSourceID: "users",
					RootFields: []resolve.GraphCoordinate{
						{TypeName: "Query", FieldName: "me", HasAuthorizationRule: true},
					},
				},
			}},
		},
		Data: &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("me"),
					Info: &resolve.FieldInfo{
						Name:                 "me",
						ExactParentTypeName:  "Query",
						Source:               resolve.TypeFieldSource{IDs: []string{"accounts", "users"}, Names: []string{"accounts", "users"}},
						HasAuthorizationRule: true,
					},
					Value: &resolve.String{Path: []string{"me"}},
				},
			},
		},
	}

	(&collectAuthorizationCoordinates{}).Process(response)

	assert.Equal(t, []resolve.AuthorizationCoordinate{
		{DataSourceID: "accounts", Coordinate: resolve.GraphCoordinate{TypeName: "Query", FieldName: "me"}},
		{DataSourceID: "users", Coordinate: resolve.GraphCoordinate{TypeName: "Query", FieldName: "me"}},
	}, response.Info.AuthorizationCoordinates)
}

func TestCollectAuthorizationCoordinates_NoProtectedFields(t *testing.T) {
	response := &resolve.GraphQLResponse{
		Info: &resolve.GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		RawFetches: []*resolve.FetchItem{
			{Fetch: &resolve.SingleFetch{
				Info: &resolve.FetchInfo{
					DataSourceID: "catalog",
					RootFields:   []resolve.GraphCoordinate{{TypeName: "Query", FieldName: "public"}},
				},
			}},
		},
		Data: &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("public"),
					Info: &resolve.FieldInfo{
						Name:                "public",
						ExactParentTypeName: "Query",
						Source:              resolve.TypeFieldSource{IDs: []string{"catalog"}, Names: []string{"catalog"}},
					},
					Value: &resolve.String{Path: []string{"public"}},
				},
			},
		},
	}

	(&collectAuthorizationCoordinates{}).Process(response)

	assert.Nil(t, response.Info.AuthorizationCoordinates)
}

// TestProcess_CollectsAuthorizationCoordinates verifies the wiring: Process populates the
// coordinates for every plan kind, from both the raw fetches and the data tree.
func TestProcess_CollectsAuthorizationCoordinates(t *testing.T) {
	processor := NewProcessor(
		DisableDeduplicateSingleFetches(),
		DisableCreateConcreteSingleFetchTypes(),
		DisableMergeFields(),
		DisableCreateParallelNodes(),
		DisableAddMissingNestedDependencies(),
		DisableResolveInputTemplates(),
		DisableExtractDeferFetches(),
		DisableBuildDeferTree(),
	)

	rawFetches := func() []*resolve.FetchItem {
		return []*resolve.FetchItem{
			{Fetch: &resolve.SingleFetch{
				FetchDependencies: resolve.FetchDependencies{FetchID: 1},
				Info: &resolve.FetchInfo{
					DataSourceID: "catalog",
					RootFields: []resolve.GraphCoordinate{
						{TypeName: "Query", FieldName: "products", HasAuthorizationRule: true},
					},
				},
			}},
		}
	}
	expected := []resolve.AuthorizationCoordinate{
		{DataSourceID: "catalog", Coordinate: resolve.GraphCoordinate{TypeName: "Query", FieldName: "products"}},
		{DataSourceID: "products", Coordinate: resolve.GraphCoordinate{TypeName: "Product", FieldName: "secret"}},
	}

	t.Run("synchronous response plan", func(t *testing.T) {
		p := &plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Info:       &resolve.GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				RawFetches: rawFetches(),
				Data:       protectedProductsData(),
			},
		}

		processor.Process(p)

		assert.Equal(t, expected, p.Response.Info.AuthorizationCoordinates)
	})

	t.Run("defer response plan", func(t *testing.T) {
		p := &plan.DeferResponsePlan{
			Response: &resolve.GraphQLDeferResponse{
				Response: &resolve.GraphQLResponse{
					Info:       &resolve.GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
					RawFetches: rawFetches(),
					Data:       protectedProductsData(),
				},
			},
		}

		processor.Process(p)

		assert.Equal(t, expected, p.Response.Response.Info.AuthorizationCoordinates)
	})

	t.Run("subscription response plan", func(t *testing.T) {
		p := &plan.SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Response: &resolve.GraphQLResponse{
					Info: &resolve.GraphQLResponseInfo{OperationType: ast.OperationTypeSubscription},
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("events"),
								Info: &resolve.FieldInfo{
									Name:                 "events",
									ExactParentTypeName:  "Subscription",
									Source:               resolve.TypeFieldSource{IDs: []string{"events"}, Names: []string{"events"}},
									HasAuthorizationRule: true,
								},
								Value: &resolve.String{Path: []string{"events"}},
							},
						},
					},
				},
			},
		}

		processor.Process(p)

		assert.Equal(t, []resolve.AuthorizationCoordinate{
			{DataSourceID: "events", Coordinate: resolve.GraphCoordinate{TypeName: "Subscription", FieldName: "events"}},
		}, p.Response.Response.Info.AuthorizationCoordinates)
	})
}
