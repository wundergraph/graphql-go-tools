package plan

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestConfigureFetchCachingEntityConfigPresentEnablesL2(t *testing.T) {
	state := testCachingPlannerState()

	cache := state.configureFetchCaching(fetchCachingInput{
		fetchID:       1,
		operationType: ast.OperationTypeQuery,
		federation: FederationMetaData{
			Keys: FederationFieldConfigurations{
				{TypeName: "User", SelectionSet: "id"},
			},
			EntityCacheConfig: EntityCacheConfigurations{
				{
					TypeName:                    "User",
					CacheName:                   "users",
					TTL:                         time.Minute,
					IncludeSubgraphHeaderPrefix: true,
					EnablePartialCacheLoad:      true,
					NegativeCacheTTL:            time.Second,
				},
			},
		},
		requiredFields: FederationFieldConfigurations{
			{TypeName: "User", SelectionSet: "id name"},
		},
	})

	require.NotNil(t, cache)
	assert.True(t, cache.EnableL2Cache)
	assert.False(t, cache.UseL1Cache)
	assert.Equal(t, "users", cache.CacheName)
	assert.Equal(t, time.Minute, cache.TTL)
	assert.Equal(t, time.Second, cache.NegativeCacheTTL)
	assert.True(t, cache.IncludeSubgraphHeaderPrefix)
	assert.True(t, cache.EnablePartialCacheLoad)
	require.IsType(t, &resolve.EntityQueryCacheKeyTemplate{}, cache.KeyTemplate)
	assert.Equal(t, []resolve.KeyField{{Name: "id"}}, cache.KeyTemplate.(*resolve.EntityQueryCacheKeyTemplate).KeyFields())
	assert.Same(t, state.plannerObjects[1], cache.ProvidesData)
}

func TestConfigureFetchCachingEntityConfigAbsentKeepsKeyTemplateWithL2Off(t *testing.T) {
	state := testCachingPlannerState()

	cache := state.configureFetchCaching(fetchCachingInput{
		fetchID:       1,
		operationType: ast.OperationTypeQuery,
		federation: FederationMetaData{
			Keys: FederationFieldConfigurations{
				{TypeName: "User", SelectionSet: "id"},
			},
		},
		requiredFields: FederationFieldConfigurations{
			{TypeName: "User", SelectionSet: "id"},
		},
	})

	require.NotNil(t, cache)
	assert.False(t, cache.EnableL2Cache)
	assert.False(t, cache.UseL1Cache)
	require.IsType(t, &resolve.EntityQueryCacheKeyTemplate{}, cache.KeyTemplate)
	assert.Equal(t, []resolve.KeyField{{Name: "id"}}, cache.KeyTemplate.(*resolve.EntityQueryCacheKeyTemplate).KeyFields())
}

func TestConfigureFetchCachingRootFieldsSharedConfigEnablesL2(t *testing.T) {
	state := testCachingPlannerState()
	state.rootFields[2] = []resolve.RootField{
		{TypeName: "Query", FieldName: "user", ResponseKey: "viewer", Arguments: []resolve.RootFieldArgument{{Name: "id", VariablePath: []string{"id"}}}},
		{TypeName: "Query", FieldName: "me"},
	}

	cache := state.configureFetchCaching(fetchCachingInput{
		fetchID:       2,
		operationType: ast.OperationTypeQuery,
		federation: FederationMetaData{
			RootFieldCacheConfig: RootFieldCacheConfigurations{
				{TypeName: "Query", FieldName: "user", CacheName: "roots", TTL: time.Minute},
				{TypeName: "Query", FieldName: "me", CacheName: "roots", TTL: time.Minute},
			},
		},
		rootFields: []resolve.GraphCoordinate{
			{TypeName: "Query", FieldName: "user"},
			{TypeName: "Query", FieldName: "me"},
		},
	})

	require.NotNil(t, cache)
	assert.True(t, cache.EnableL2Cache)
	assert.Equal(t, "roots", cache.CacheName)
	assert.Equal(t, time.Minute, cache.TTL)
	require.IsType(t, &resolve.RootQueryCacheKeyTemplate{}, cache.KeyTemplate)
	template := cache.KeyTemplate.(*resolve.RootQueryCacheKeyTemplate)
	assert.Equal(t, state.rootFields[2], template.RootFields)
}

func TestConfigureFetchCachingRootFieldsDifferingConfigKeepsTemplateWithL2Off(t *testing.T) {
	state := testCachingPlannerState()
	state.rootFields[2] = []resolve.RootField{
		{TypeName: "Query", FieldName: "user"},
		{TypeName: "Query", FieldName: "product"},
	}

	cache := state.configureFetchCaching(fetchCachingInput{
		fetchID:       2,
		operationType: ast.OperationTypeQuery,
		federation: FederationMetaData{
			RootFieldCacheConfig: RootFieldCacheConfigurations{
				{TypeName: "Query", FieldName: "user", CacheName: "users"},
				{TypeName: "Query", FieldName: "product", CacheName: "products"},
			},
		},
		rootFields: []resolve.GraphCoordinate{
			{TypeName: "Query", FieldName: "user"},
			{TypeName: "Query", FieldName: "product"},
		},
	})

	require.NotNil(t, cache)
	assert.False(t, cache.EnableL2Cache)
	assert.Empty(t, cache.CacheName)
	require.IsType(t, &resolve.RootQueryCacheKeyTemplate{}, cache.KeyTemplate)
}

func TestConfigureFetchCachingDisableEntityCachingHardGate(t *testing.T) {
	state := testCachingPlannerState()
	state.config.DisableEntityCaching = true

	cache := state.configureFetchCaching(fetchCachingInput{
		fetchID:       1,
		operationType: ast.OperationTypeQuery,
		federation: FederationMetaData{
			EntityCacheConfig: EntityCacheConfigurations{{TypeName: "User", CacheName: "users"}},
		},
		requiredFields: FederationFieldConfigurations{{TypeName: "User", SelectionSet: "id"}},
	})

	assert.Nil(t, cache)
}

func testCachingPlannerState() *cachingPlannerState {
	return &cachingPlannerState{
		config: &Configuration{},
		plannerObjects: map[int]*resolve.Object{
			1: {
				Fields: []*resolve.Field{
					{Name: []byte("id"), Value: &resolve.String{}},
				},
			},
		},
		rootFields: map[int][]resolve.RootField{},
	}
}
