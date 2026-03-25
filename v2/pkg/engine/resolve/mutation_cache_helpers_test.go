package resolve

import (
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// buildMutationTTLResponse creates a GraphQLResponse for testing mutation TTL override.
// The root fetch is a mutation that sets EnableMutationL2CachePopulation and MutationCacheTTLOverride
// on the Loader. The entity fetch that follows inherits these flags via resolveSingle propagation.
func buildMutationTTLResponse(
	rootDS, entityDS DataSource,
	cacheKeyTemplate CacheKeyTemplate,
	providesData *Object,
	enableL2Population bool,
	mutationTTLOverride time.Duration,
	entityTTL time.Duration,
) *GraphQLResponse {
	return &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeMutation},
		Fetches: Sequence(
			// Root mutation fetch — propagates EnableMutationL2CachePopulation and MutationCacheTTLOverride to Loader
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource:     rootDS,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
					Caching: FetchCacheConfiguration{
						EnableMutationL2CachePopulation: enableL2Population,
						MutationCacheTTLOverride:        mutationTTLOverride,
					},
				},
				InputTemplate: InputTemplate{Segments: []TemplateSegment{
					{Data: []byte(`{"method":"POST","url":"http://accounts.service","body":{"query":"mutation{updateUser(id:\"u1\",name:\"Alice\"){__typename id}}"}}`), SegmentType: StaticSegmentType},
				}},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
				Info: &FetchInfo{
					DataSourceID: "accounts", DataSourceName: "accounts",
					RootFields:    []GraphCoordinate{{TypeName: "Mutation", FieldName: "updateUser"}},
					OperationType: ast.OperationTypeMutation,
				},
			}, "mutation"),

			// Entity fetch — inherits mutation L2 flags, uses caching config with entity TTL
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource:     entityDS,
					PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities", "0"}},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						TTL:              entityTTL,
						CacheKeyTemplate: cacheKeyTemplate,
						UseL1Cache:       true,
					},
				},
				InputTemplate: InputTemplate{Segments: []TemplateSegment{
					{Data: []byte(`{"method":"POST","url":"http://accounts.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {name}}}","variables":{"representations":[`), SegmentType: StaticSegmentType},
					{SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind, Renderer: NewGraphQLVariableResolveRenderer(&Object{
						Fields: []*Field{
							{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
							{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
						},
					})},
					{Data: []byte(`]}}}`), SegmentType: StaticSegmentType},
				}},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
				Info: &FetchInfo{
					DataSourceID: "accounts", DataSourceName: "accounts",
					RootFields:    []GraphCoordinate{{TypeName: "User", FieldName: "name"}},
					OperationType: ast.OperationTypeQuery, // Entity fetches resolve from non-root types, so planner sets Query
					ProvidesData:  providesData,
				},
			}, "mutation.updateUser", ObjectPath("updateUser")),
		),
		Data: &Object{
			Fields: []*Field{{
				Name: []byte("updateUser"),
				Value: &Object{
					Path: []string{"updateUser"},
					Fields: []*Field{
						{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
						{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
					},
				},
			}},
		},
	}
}

// newMutationUserCacheKeyTemplate returns a cache key template for User entities in mutation tests.
func newMutationUserCacheKeyTemplate() CacheKeyTemplate {
	return &EntityQueryCacheKeyTemplate{
		Keys: NewResolvableObjectVariable(&Object{
			Fields: []*Field{
				{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
			},
		}),
	}
}

// newMutationUserProvidesData returns a ProvidesData for User entities in mutation tests.
func newMutationUserProvidesData() *Object {
	return &Object{
		Fields: []*Field{
			{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}, Nullable: false}},
		},
	}
}
