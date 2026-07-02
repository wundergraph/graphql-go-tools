package postprocess

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// The postprocess-level caching suite: the FULL Processor pipeline over
// synthetic plans, pinning that EnableCaching configures fetches in the sync
// arm AND in the defer arm (where ConfigureCaching runs AFTER buildDeferTree
// and derives group ancestry from the built DeferTree), plus direct rows for
// collectDeferCachingTrees. The plan-driven end-to-end equivalents live in
// execution/cachingtesting.

func cachingTestDefinition(t *testing.T) *ast.Document {
	t.Helper()
	definition, report := astparser.ParseGraphqlDocumentString(`
		scalar String
		scalar Int

		type Query {
			products: [Product]
		}

		type Product {
			upc: String!
			name: String!
			price: Int
		}
	`)
	require.False(t, report.HasErrors(), "parse definition: %v", report)
	return &definition
}

func cachingTestFederation(t *testing.T) map[string]plan.FederationMetaData {
	t.Helper()
	// The metadata needs Init (entity index + parsed key selection sets) —
	// exactly what the engine wiring provides in production.
	metadata := &plan.DataSourceMetadata{
		FederationMetaData: plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{TypeName: "Product", SelectionSet: "upc"},
			},
		},
	}
	require.NoError(t, metadata.Init())
	return map[string]plan.FederationMetaData{"products": metadata.FederationMetaData}
}

func cachingTestProviders(ttl time.Duration) map[string]cacheconfig.CacheConfigProvider {
	return map[string]cacheconfig.CacheConfigProvider{
		"products": &cacheconfig.CachingConfiguration{
			Entities: []cacheconfig.EntityCachePolicy{
				{TypeName: "Product", CacheName: "products", TTL: ttl},
			},
		},
	}
}

// productTree builds an entity coverage tree with the given scalar fields.
func productTree(fields ...string) *resolve.Object {
	obj := &resolve.Object{}
	for _, name := range fields {
		obj.Fields = append(obj.Fields, &resolve.Field{
			Name:  []byte(name),
			Value: &resolve.Scalar{Nullable: true, Path: []string{name}},
		})
	}
	return obj
}

// entityRawFetch builds a raw SingleFetch that createConcreteSingleFetchTypes
// converts into an EntityFetch (RequiresEntityFetch + a representations
// variable segment), attributed to the products datasource.
func entityRawFetch(fetchID, deferID int) (*resolve.FetchItem, *resolve.FetchInfo) {
	info := &resolve.FetchInfo{
		DataSourceID: "products",
		RootFields:   []resolve.GraphCoordinate{{TypeName: "Product"}},
	}
	fetch := &resolve.SingleFetch{
		FetchDependencies: resolve.FetchDependencies{FetchID: fetchID, DeferID: deferID},
		FetchConfiguration: resolve.FetchConfiguration{
			RequiresEntityFetch: true,
			Input:               `{"method":"POST","url":"http://products","body":{"query":"...","variables":{"representations":[$$0$$]}}}`,
			Variables: resolve.Variables{
				resolve.NewResolvableObjectVariable(&resolve.Object{
					Fields: []*resolve.Field{
						{Name: []byte("__typename"), Value: &resolve.String{Path: []string{"__typename"}}},
						{Name: []byte("upc"), Value: &resolve.String{Path: []string{"upc"}}},
					},
				}),
			},
		},
		Info: info,
	}
	return &resolve.FetchItem{Fetch: fetch, ResponsePath: "products"}, info
}

func minimalData() *resolve.Object {
	return &resolve.Object{
		Fields: []*resolve.Field{
			{Name: []byte("products"), Value: &resolve.String{Path: []string{"products"}, Nullable: true}},
		},
	}
}

// findCacheConfigs walks a fetch tree collecting each fetch's cache config
// string (nil-safe), depth-first.
func findCacheConfigs(node *resolve.FetchTreeNode) []string {
	if node == nil {
		return nil
	}
	var out []string
	if node.Item != nil && node.Item.Fetch != nil {
		out = append(out, node.Item.Fetch.CacheConfig().String())
	}
	for _, child := range node.ChildNodes {
		out = append(out, findCacheConfigs(child)...)
	}
	return out
}

// TestPostprocessCachingSyncArm: the full pipeline configures an entity fetch
// in a synchronous plan; a lone entity fetch has no L1 partner, so the
// narrowing pass turns L1 off and L2 remains.
func TestPostprocessCachingSyncArm(t *testing.T) {
	entityItem, info := entityRawFetch(2, 0)
	response := &resolve.GraphQLResponse{
		RawFetches: []*resolve.FetchItem{
			{Fetch: &resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}}},
			entityItem,
		},
		Data: minimalData(),
	}
	response.SetCacheProvidesData(map[*resolve.FetchInfo]*resolve.Object{
		info: productTree("name", "price"),
	})

	processor := NewProcessor(EnableCaching(cachingTestProviders(time.Minute), cachingTestFederation(t), cachingTestDefinition(t)))
	processor.Process(&plan.SynchronousResponsePlan{Response: response})

	assert.Equal(t, []string{
		`<nil>`,
		`{l1:false l2:true cacheName:products ttl:1m0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:Entity type:Product field: candidates:1 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}`,
	}, findCacheConfigs(response.Fetches))
}

// TestPostprocessCachingNoOp: a processor with EnableCaching over EMPTY
// providers produces a plan equal to a plain processor's — the no-op gate at
// the postprocess level.
func TestPostprocessCachingNoOp(t *testing.T) {
	build := func() *plan.SynchronousResponsePlan {
		entityItem, _ := entityRawFetch(2, 0)
		return &plan.SynchronousResponsePlan{Response: &resolve.GraphQLResponse{
			RawFetches: []*resolve.FetchItem{
				{Fetch: &resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}}},
				entityItem,
			},
			Data: minimalData(),
		}}
	}

	plain := build()
	NewProcessor().Process(plain)

	withEmptyCaching := build()
	NewProcessor(EnableCaching(map[string]cacheconfig.CacheConfigProvider{}, nil, nil)).Process(withEmptyCaching)

	assert.Equal(t, plain, withEmptyCaching)
	assert.Equal(t, []string{`<nil>`, `<nil>`}, findCacheConfigs(withEmptyCaching.Response.Fetches))
}

// deferCachingPlan builds a DeferResponsePlan with one uncached initial fetch
// and two ENTITY fetches in defer groups 1 and 2, whose coverage trees form a
// provider/consumer pair ({name,price} ⊇ {name}). The descriptors decide
// whether group 2 is NESTED under group 1 or its SIBLING.
func deferCachingPlan(t *testing.T, descriptors map[int]resolve.DeferDescriptor) (*plan.DeferResponsePlan, *resolve.GraphQLDeferResponse) {
	t.Helper()
	providerItem, providerInfo := entityRawFetch(2, 1)
	consumerItem, consumerInfo := entityRawFetch(3, 2)
	response := &resolve.GraphQLResponse{
		RawFetches: []*resolve.FetchItem{
			{Fetch: &resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}}},
			providerItem,
			consumerItem,
		},
		Data: minimalData(),
	}
	response.SetCacheProvidesData(map[*resolve.FetchInfo]*resolve.Object{
		providerInfo: productTree("name", "price"),
		consumerInfo: productTree("name"),
	})
	deferResponse := &resolve.GraphQLDeferResponse{
		Response:         response,
		DeferDescriptors: descriptors,
	}
	return &plan.DeferResponsePlan{Response: deferResponse}, deferResponse
}

// deferGroupCacheConfigs walks the BUILT DeferTree in execution order and
// returns each group's fetch cache configs.
func deferGroupCacheConfigs(node *resolve.DeferTreeNode) []string {
	if node == nil {
		return nil
	}
	var out []string
	if node.Item != nil {
		out = append(out, findCacheConfigs(node.Item.Fetches)...)
	}
	for _, child := range node.ChildNodes {
		out = append(out, deferGroupCacheConfigs(child)...)
	}
	return out
}

// TestPostprocessCachingDeferAncestry pins the reason ConfigureCaching runs
// AFTER buildDeferTree: the L1 narrowing pass orders defer groups by the
// AUTHORITATIVE tree's ancestry. The same provider/consumer pair keeps L1
// when the consumer's group is NESTED under the provider's (a parent group
// resolves fully before its children) and is narrowed off — configs re-nil'd,
// the policies being L1-only — when the groups are unordered SIBLINGS.
func TestPostprocessCachingDeferAncestry(t *testing.T) {
	l1OnlyConfig := `{l1:true l2:false cacheName:products ttl:0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:Entity type:Product field: candidates:1 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}`

	t.Run("nested groups keep the L1 pair", func(t *testing.T) {
		deferPlan, deferResponse := deferCachingPlan(t, map[int]resolve.DeferDescriptor{
			1: {ID: 1, ParentID: 0},
			2: {ID: 2, ParentID: 1},
		})
		NewProcessor(EnableCaching(cachingTestProviders(0), cachingTestFederation(t), cachingTestDefinition(t))).Process(deferPlan)
		require.NotNil(t, deferResponse.DeferTree)
		assert.Equal(t, []string{l1OnlyConfig, l1OnlyConfig}, deferGroupCacheConfigs(deferResponse.DeferTree))
	})

	t.Run("sibling groups are unordered: both narrowed off and re-nil'd", func(t *testing.T) {
		deferPlan, deferResponse := deferCachingPlan(t, map[int]resolve.DeferDescriptor{
			1: {ID: 1, ParentID: 0},
			2: {ID: 2, ParentID: 0},
		})
		NewProcessor(EnableCaching(cachingTestProviders(0), cachingTestFederation(t), cachingTestDefinition(t))).Process(deferPlan)
		require.NotNil(t, deferResponse.DeferTree)
		assert.Equal(t, []string{`<nil>`, `<nil>`}, deferGroupCacheConfigs(deferResponse.DeferTree))
	})
}

// TestCollectDeferCachingTrees pins the tree/ancestry collection directly
// over BUILT defer trees (and the not-built fallback).
func TestCollectDeferCachingTrees(t *testing.T) {
	groupFetches := func(response *resolve.GraphQLDeferResponse) map[int]*resolve.FetchTreeNode {
		out := make(map[int]*resolve.FetchTreeNode)
		var walk func(node *resolve.DeferTreeNode)
		walk = func(node *resolve.DeferTreeNode) {
			if node == nil {
				return
			}
			if node.Item != nil {
				out[node.Item.DeferID] = node.Item.Fetches
			}
			for _, child := range node.ChildNodes {
				walk(child)
			}
		}
		walk(response.DeferTree)
		return out
	}

	t.Run("single root group", func(t *testing.T) {
		p := makeDeferPlan(map[int]resolve.DeferDescriptor{1: {ID: 1, ParentID: 0}}, 1)
		runBuildDeferTree(p)
		trees, parents := collectDeferCachingTrees(p.Response)
		byID := groupFetches(p.Response)
		assert.Equal(t, []*resolve.FetchTreeNode{p.Response.Response.Fetches, byID[1]}, trees)
		assert.Equal(t, []int{-1, 0}, parents)
	})

	t.Run("two siblings share the initial parent", func(t *testing.T) {
		p := makeDeferPlan(map[int]resolve.DeferDescriptor{
			1: {ID: 1, ParentID: 0},
			2: {ID: 2, ParentID: 0},
		}, 1, 2)
		runBuildDeferTree(p)
		_, parents := collectDeferCachingTrees(p.Response)
		assert.Equal(t, []int{-1, 0, 0}, parents)
	})

	t.Run("a nested chain parents each group to its enclosing group", func(t *testing.T) {
		p := makeDeferPlan(map[int]resolve.DeferDescriptor{
			1: {ID: 1, ParentID: 0},
			3: {ID: 3, ParentID: 1},
			5: {ID: 5, ParentID: 3},
		}, 1, 3, 5)
		runBuildDeferTree(p)
		trees, parents := collectDeferCachingTrees(p.Response)
		byID := groupFetches(p.Response)
		assert.Equal(t, []*resolve.FetchTreeNode{p.Response.Response.Fetches, byID[1], byID[3], byID[5]}, trees)
		assert.Equal(t, []int{-1, 0, 1, 2}, parents)
	})

	t.Run("without a built tree every group falls back to the initial parent", func(t *testing.T) {
		p := makeDeferPlan(map[int]resolve.DeferDescriptor{
			1: {ID: 1, ParentID: 0},
			3: {ID: 3, ParentID: 1},
		}, 1, 3)
		// extract only — DeferTree stays nil (the disableBuildDeferTree shape).
		ext := &extractDeferFetches{}
		ext.Process(p)
		require.Nil(t, p.Response.DeferTree)
		_, parents := collectDeferCachingTrees(p.Response)
		assert.Equal(t, []int{-1, 0, 0}, parents)
	})

	t.Run("a fetchless parent group attaches its children to the nearest ancestor", func(t *testing.T) {
		child := &resolve.DeferFetchGroup{DeferID: 2, Fetches: resolve.Sequence()}
		response := &resolve.GraphQLDeferResponse{
			Response: &resolve.GraphQLResponse{Fetches: resolve.Sequence()},
			DeferTree: resolve.DeferSequence(
				resolve.DeferSingle(&resolve.DeferFetchGroup{DeferID: 1, Fetches: nil}),
				resolve.DeferSingle(child),
			),
		}
		trees, parents := collectDeferCachingTrees(response)
		assert.Equal(t, []*resolve.FetchTreeNode{response.Response.Fetches, child.Fetches}, trees)
		assert.Equal(t, []int{-1, 0}, parents)
	})
}
