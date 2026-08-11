package plan

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

// TestShouldIsolateRootField pins the gate predicate directly: every condition
// that must hold, and every one that must decline. The plan-level behavior
// (two parallel fetches, the entity-root-node trap, defer composition, the
// byte-identical no-op) is pinned over REAL plans in
// execution/cachingtesting/isolation_e2e_test.go.
func TestShouldIsolateRootField(t *testing.T) {
	caching := &cacheconfig.CachingConfiguration{
		Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
			"products": {
				RootFields: []cacheconfig.RootFieldCacheConfig{
					{TypeName: "Query", FieldName: "products", TTL: time.Minute},
				},
			},
		},
	}
	productsField := &currentFieldInfo{
		typeName:  "Query",
		fieldName: "products",
		ds:        &dataSourceConfiguration[any]{id: "products"},
	}

	t.Run("cached query root field isolates", func(t *testing.T) {
		assert.True(t, shouldIsolateRootField(caching, productsField, "query"))
	})

	t.Run("no caching configured never isolates (the provable no-op)", func(t *testing.T) {
		assert.False(t, shouldIsolateRootField(nil, productsField, "query"))
	})

	t.Run("only DIRECT children of the QUERY root isolate", func(t *testing.T) {
		assert.False(t, shouldIsolateRootField(caching, productsField, "mutation"))
		assert.False(t, shouldIsolateRootField(caching, productsField, "subscription"))
		assert.False(t, shouldIsolateRootField(caching, productsField, "query.products"))
	})

	t.Run("a subgraph without root field entries never isolates", func(t *testing.T) {
		otherDS := &currentFieldInfo{
			typeName:  "Query",
			fieldName: "products",
			ds:        &dataSourceConfiguration[any]{id: "reviews"},
		}
		assert.False(t, shouldIsolateRootField(caching, otherDS, "query"))
	})

	t.Run("a coordinate without an entry never isolates", func(t *testing.T) {
		uncachedField := &currentFieldInfo{
			typeName:  "Query",
			fieldName: "promotions",
			ds:        &dataSourceConfiguration[any]{id: "products"},
		}
		assert.False(t, shouldIsolateRootField(caching, uncachedField, "query"))
	})

	t.Run("a vetoed subgraph never isolates", func(t *testing.T) {
		vetoed := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"products": {
					Enabled: ptr(false),
					RootFields: []cacheconfig.RootFieldCacheConfig{
						{TypeName: "Query", FieldName: "products", TTL: time.Minute},
					},
				},
			},
		}
		assert.False(t, shouldIsolateRootField(vetoed, productsField, "query"))
	})

	t.Run("an INERT entry (no TTL, no shadow) never isolates", func(t *testing.T) {
		// The configurator's all-flags-false safety net drops such an entry's
		// config entirely; isolating for it would change the plan without
		// enabling any caching.
		inert := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"products": {
					RootFields: []cacheconfig.RootFieldCacheConfig{
						{TypeName: "Query", FieldName: "products"},
					},
				},
			},
		}
		assert.False(t, shouldIsolateRootField(inert, productsField, "query"))
	})

	t.Run("a shadow-only entry isolates", func(t *testing.T) {
		shadow := &cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"products": {
					RootFields: []cacheconfig.RootFieldCacheConfig{
						{TypeName: "Query", FieldName: "products", ShadowMode: true},
					},
				},
			},
		}
		assert.True(t, shouldIsolateRootField(shadow, productsField, "query"))
	})

	t.Run("a global DefaultTTL alone never isolates a root field", func(t *testing.T) {
		// Entity caching is subgraph-wide, root-field caching is per coordinate:
		// without an entry there is nothing to isolate for.
		globalOnly := &cacheconfig.CachingConfiguration{
			Global: cacheconfig.GlobalCacheConfig{DefaultTTL: time.Minute},
		}
		assert.False(t, shouldIsolateRootField(globalOnly, productsField, "query"))
	})
}

func ptr[T any](value T) *T {
	return &value
}
