package cachingtesting

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// renderFetchCacheConfigs walks a fetch tree and renders every fetch's cache
// config (nil-safe) with its response path, for full-value plan-level asserts.
func renderFetchCacheConfigs(node *resolve.FetchTreeNode) []string {
	if node == nil {
		return nil
	}
	var out []string
	if node.Item != nil && node.Item.Fetch != nil {
		out = append(out, fmt.Sprintf("path:%q cache:%s", node.Item.ResponsePath, node.Item.Fetch.CacheConfig().String()))
	}
	for _, child := range node.ChildNodes {
		out = append(out, renderFetchCacheConfigs(child)...)
	}
	return out
}

func entityCachingFor(subgraphs ...string) map[string]cacheconfig.CachingConfiguration {
	caching := make(map[string]cacheconfig.CachingConfiguration, len(subgraphs))
	for _, name := range subgraphs {
		caching[name] = cacheconfig.CachingConfiguration{
			Entities: []cacheconfig.EntityCachePolicy{
				{TypeName: "Product", CacheName: "products", TTL: time.Minute},
			},
		}
	}
	return caching
}

// TestEntityCacheConfigSyncPlan pins the full per-fetch cache config over a
// REAL sync plan: the reviews batch entity fetch is configured, the products
// root fetch stays nil (root-field caching is task 13). The lone entity fetch
// has no L1 provider/consumer pair, so optimizeL1Cache (task 16) narrows L1
// off.
func TestEntityCacheConfigSyncPlan(t *testing.T) {
	result := Plan(t, `{ products(first: 2) { upc reviews { body } } }`, entityCachingFor("reviews"), nil)
	rendered := renderFetchCacheConfigs(result.Response.Fetches)
	assert.Equal(t, []string{
		`path:"" cache:<nil>`,
		`path:"products" cache:{l1:false l2:true cacheName:products ttl:1m0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:Entity type:Product field: candidates:1 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}`,
	}, rendered)
}

// TestEntityCacheConfigDeferPlan pins that DEFER-GROUP entity fetches carry
// config too: the initial inventory fetch and the deferred inventory fetch are
// both configured from the same policy.
func TestEntityCacheConfigDeferPlan(t *testing.T) {
	query := `
		query {
			me { favoriteProduct { upc stock warehouse { id location } } }
			products(first: 1) {
				upc
				... @defer { stock }
			}
		}`
	result := Plan(t, query, entityCachingFor("inventory"), nil)
	require.NotNil(t, result.DeferResponse)

	inventoryCfg := `{l1:true l2:true cacheName:products ttl:1m0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:Entity type:Product field: candidates:1 entityKeyMappings:0 providesData:true populateL2OnMutation:false mutationTTL:0s}`

	initial := renderFetchCacheConfigs(result.Response.Fetches)
	assert.Equal(t, []string{
		`path:"" cache:<nil>`,
		`path:"" cache:<nil>`,
		`path:"me" cache:<nil>`,
		`path:"me.favoriteProduct" cache:` + inventoryCfg,
	}, initial)

	groups := DeferGroups(result.DeferResponse)
	require.Len(t, groups, 1)
	deferred := renderFetchCacheConfigs(groups[0].Fetches)
	assert.Equal(t, []string{
		`path:"products" cache:` + inventoryCfg,
	}, deferred)
}

// TestEntityCacheConfigDeterminism plans the same operation twice and asserts
// identical rendered configs.
func TestEntityCacheConfigDeterminism(t *testing.T) {
	query := `{ products(first: 2) { upc reviews { body } } }`
	first := Plan(t, query, entityCachingFor("reviews", "inventory"), nil)
	second := Plan(t, query, entityCachingFor("reviews", "inventory"), nil)
	assert.Equal(t,
		strings.Join(renderFetchCacheConfigs(first.Response.Fetches), "\n"),
		strings.Join(renderFetchCacheConfigs(second.Response.Fetches), "\n"),
	)
}
