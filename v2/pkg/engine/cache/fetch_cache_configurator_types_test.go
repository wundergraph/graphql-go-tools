package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestFetchCacheConfiguratorTypeDeclarations pins the narrowest static tier as
// it lands on a fetch: a type's own declaration replaces the subgraph lifetime,
// a type declared uncacheable is stamped with no configuration at all, and a
// fetch batching several concrete types folds all their declarations into one.
func TestFetchCacheConfiguratorTypeDeclarations(t *testing.T) {
	t.Run("the declared lifetime replaces the subgraph DefaultTTL", func(t *testing.T) {
		configurator := newEntityConfigurator(productCaching(cacheconfig.SubgraphCacheConfig{
			DefaultTTL:       ptr(time.Minute),
			NegativeCacheTTL: ptr(5 * time.Second),
			Types: map[string]cacheconfig.TypeCacheConfig{
				"Product": {MaxAge: 30 * time.Second},
			},
		}))
		fetch := entityFetchWith(productRepresentation(t, "upc"))
		tree := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: fetch}}
		configurator.configureTree(&resolve.GraphQLResponse{}, tree)

		require.NotNil(t, fetch.Cache)
		assert.Equal(t, &resolve.FetchCacheConfig{
			L1:               true,
			L2:               true,
			SubgraphName:     "products",
			TTL:              30 * time.Second,
			NegativeCacheTTL: 5 * time.Second,
			KeySpec: resolve.CacheKeySpec{
				Scope:          resolve.CacheScopeEntity,
				TypeName:       "Product",
				Representation: upcRepresentation(),
			},
		}, fetch.Cache)
	})

	t.Run("a PRIVATE declaration partitions one type of a public subgraph", func(t *testing.T) {
		configurator := newEntityConfigurator(productCaching(cacheconfig.SubgraphCacheConfig{
			DefaultTTL: ptr(time.Minute),
			Types: map[string]cacheconfig.TypeCacheConfig{
				"Product": {MaxAge: 30 * time.Second, Scope: cacheconfig.CacheScopePrivate},
			},
		}))
		fetch := entityFetchWith(productRepresentation(t, "upc"))
		tree := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: fetch}}
		configurator.configureTree(&resolve.GraphQLResponse{}, tree)

		require.NotNil(t, fetch.Cache)
		assert.Equal(t, &resolve.FetchCacheConfig{
			L1:           true,
			L2:           true,
			SubgraphName: "products",
			TTL:          30 * time.Second,
			Private:      true,
			KeySpec: resolve.CacheKeySpec{
				Scope:          resolve.CacheScopeEntity,
				TypeName:       "Product",
				Representation: upcRepresentation(),
			},
		}, fetch.Cache)
	})

	t.Run("a type declared uncacheable is stamped with nothing, sentinel included", func(t *testing.T) {
		configurator := newEntityConfigurator(productCaching(cacheconfig.SubgraphCacheConfig{
			DefaultTTL:       ptr(time.Minute),
			NegativeCacheTTL: ptr(5 * time.Second),
			Types: map[string]cacheconfig.TypeCacheConfig{
				"Product": {MaxAge: 0},
			},
		}))
		fetch := entityFetchWith(productRepresentation(t, "upc"))
		tree := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: fetch}}
		configurator.configureTree(&resolve.GraphQLResponse{}, tree)

		assert.Nil(t, fetch.CacheConfig())
	})

	// The abstract-path node: one conditioned key-field group per concrete type,
	// built by the planner's own representation builder. Its OnTypeNames are the
	// only static source of the types ONE such fetch can resolve.
	abstractRepresentation := func(t *testing.T) *resolve.Object {
		t.Helper()
		return mergedRepresentation(t, plan.FederationMetaData{},
			plan.FederationFieldConfiguration{TypeName: "Product", SelectionSet: "upc"},
			plan.FederationFieldConfiguration{TypeName: "Brand", SelectionSet: "id"},
		)
	}
	wantAbstractRepresentation := &resolve.Object{
		Nullable: true,
		Fields: []*resolve.Field{
			{
				Name:        []byte("__typename"),
				Value:       &resolve.String{Path: []string{"__typename"}},
				OnTypeNames: [][]byte{[]byte("Product")},
			},
			{
				Name:                []byte("upc"),
				Value:               &resolve.String{Path: []string{"upc"}},
				OnTypeNames:         [][]byte{[]byte("Product")},
				CacheEntityKeyField: true,
			},
			{
				Name:        []byte("__typename"),
				Value:       &resolve.String{Path: []string{"__typename"}},
				OnTypeNames: [][]byte{[]byte("Brand")},
			},
			{
				Name:                []byte("id"),
				Value:               &resolve.String{Path: []string{"id"}},
				OnTypeNames:         [][]byte{[]byte("Brand")},
				CacheEntityKeyField: true,
			},
		},
	}

	t.Run("a batch over two concrete types takes the shortest declared lifetime", func(t *testing.T) {
		configurator := newEntityConfigurator(productCaching(cacheconfig.SubgraphCacheConfig{
			DefaultTTL: ptr(time.Hour),
			Types: map[string]cacheconfig.TypeCacheConfig{
				"Product": {MaxAge: 30 * time.Second},
				"Brand":   {MaxAge: 10 * time.Second},
			},
		}))
		fetch := batchEntityFetchWith(abstractRepresentation(t))
		tree := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: fetch}}
		configurator.configureTree(&resolve.GraphQLResponse{}, tree)

		require.NotNil(t, fetch.Cache)
		assert.Equal(t, &resolve.FetchCacheConfig{
			L1:           true,
			L2:           true,
			SubgraphName: "products",
			TTL:          10 * time.Second,
			KeySpec: resolve.CacheKeySpec{
				Scope:          resolve.CacheScopeEntity,
				TypeName:       "Product",
				Representation: wantAbstractRepresentation,
			},
		}, fetch.Cache)
	})

	t.Run("one private declaration turns the whole batch private", func(t *testing.T) {
		configurator := newEntityConfigurator(productCaching(cacheconfig.SubgraphCacheConfig{
			DefaultTTL: ptr(time.Minute),
			Types: map[string]cacheconfig.TypeCacheConfig{
				"Brand": {MaxAge: 45 * time.Second, Scope: cacheconfig.CacheScopePrivate},
			},
		}))
		fetch := batchEntityFetchWith(abstractRepresentation(t))
		tree := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: fetch}}
		configurator.configureTree(&resolve.GraphQLResponse{}, tree)

		require.NotNil(t, fetch.Cache)
		assert.Equal(t, &resolve.FetchCacheConfig{
			L1:           true,
			L2:           true,
			SubgraphName: "products",
			// Product is undeclared and contributes the subgraph minute; Brand's
			// declared 45s is shorter and wins the fold.
			TTL:     45 * time.Second,
			Private: true,
			KeySpec: resolve.CacheKeySpec{
				Scope:          resolve.CacheScopeEntity,
				TypeName:       "Product",
				Representation: wantAbstractRepresentation,
			},
		}, fetch.Cache)
	})

	t.Run("one uncacheable concrete type vetoes the whole batch", func(t *testing.T) {
		configurator := newEntityConfigurator(productCaching(cacheconfig.SubgraphCacheConfig{
			DefaultTTL: ptr(time.Minute),
			Types: map[string]cacheconfig.TypeCacheConfig{
				"Brand": {MaxAge: 0},
			},
		}))
		fetch := batchEntityFetchWith(abstractRepresentation(t))
		tree := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: fetch}}
		configurator.configureTree(&resolve.GraphQLResponse{}, tree)

		assert.Nil(t, fetch.CacheConfig())
	})
}
