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
	providers := map[string]cacheconfig.CacheConfigProvider{
		"products": &cacheconfig.CachingConfiguration{
			RootFields: []cacheconfig.RootFieldCachePolicy{
				{TypeName: "Query", FieldName: "products", CacheName: "products-cache", TTL: time.Minute},
			},
		},
	}
	productsField := &currentFieldInfo{
		typeName:  "Query",
		fieldName: "products",
		ds:        &dataSourceConfiguration[any]{id: "products"},
	}

	t.Run("cached query root field isolates", func(t *testing.T) {
		assert.True(t, shouldIsolateRootField(providers, productsField, "query"))
	})

	t.Run("no caching configured never isolates (the provable no-op)", func(t *testing.T) {
		assert.False(t, shouldIsolateRootField(nil, productsField, "query"))
		assert.False(t, shouldIsolateRootField(map[string]cacheconfig.CacheConfigProvider{}, productsField, "query"))
	})

	t.Run("only DIRECT children of the QUERY root isolate", func(t *testing.T) {
		assert.False(t, shouldIsolateRootField(providers, productsField, "mutation"))
		assert.False(t, shouldIsolateRootField(providers, productsField, "subscription"))
		assert.False(t, shouldIsolateRootField(providers, productsField, "query.products"))
	})

	t.Run("a datasource without a provider never isolates", func(t *testing.T) {
		otherDS := &currentFieldInfo{
			typeName:  "Query",
			fieldName: "products",
			ds:        &dataSourceConfiguration[any]{id: "reviews"},
		}
		assert.False(t, shouldIsolateRootField(providers, otherDS, "query"))
	})

	t.Run("a coordinate without a RootFieldPolicy never isolates", func(t *testing.T) {
		uncachedField := &currentFieldInfo{
			typeName:  "Query",
			fieldName: "promotions",
			ds:        &dataSourceConfiguration[any]{id: "products"},
		}
		assert.False(t, shouldIsolateRootField(providers, uncachedField, "query"))
	})
}
