package resolve

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// fullFetchCacheConfig returns a config with every field populated, so the
// Equals table can flip one field per row and prove each one participates.
func fullFetchCacheConfig() *FetchCacheConfig {
	return &FetchCacheConfig{
		L1:                     true,
		L2:                     true,
		SubgraphName:           "products",
		TTL:                    30 * time.Second,
		MaxTTL:                 2 * time.Minute,
		NegativeCacheTTL:       5 * time.Second,
		IncludeSubgraphHeaders: true,
		Private:                true,
		EnablePartialCacheLoad: true,
		PartialBatchLoad:       true,
		ShadowMode:             true,
		HashAnalyticsKeys:      true,
		KeySpec: CacheKeySpec{
			Scope:     CacheScopeEntity,
			TypeName:  "Product",
			FieldName: "product",
			Representation: &Object{
				Nullable: true,
				Fields: []*Field{
					{
						Name:        []byte("upc"),
						Value:       &String{Path: []string{"upc"}},
						OnTypeNames: [][]byte{[]byte("Product")},
					},
				},
			},
		},
		ProvidesData: &Object{
			Fields: []*Field{
				{
					Name:  []byte("name"),
					Value: &String{Path: []string{"name"}},
				},
			},
		},
		PopulateL2OnMutation: true,
		MutationTTLOverride:  10 * time.Second,
	}
}

func TestFetchCacheConfigEquals(t *testing.T) {
	t.Run("nil safety", func(t *testing.T) {
		var nilCfg *FetchCacheConfig
		assert.True(t, nilCfg.Equals(nil))
		assert.False(t, nilCfg.Equals(fullFetchCacheConfig()))
		assert.False(t, fullFetchCacheConfig().Equals(nil))
	})

	t.Run("equal when fully populated", func(t *testing.T) {
		assert.True(t, fullFetchCacheConfig().Equals(fullFetchCacheConfig()))
	})

	mutations := []struct {
		name   string
		mutate func(c *FetchCacheConfig)
	}{
		{"L1", func(c *FetchCacheConfig) { c.L1 = false }},
		{"L2", func(c *FetchCacheConfig) { c.L2 = false }},
		{"SubgraphName", func(c *FetchCacheConfig) { c.SubgraphName = "reviews" }},
		{"TTL", func(c *FetchCacheConfig) { c.TTL = time.Minute }},
		{"NegativeCacheTTL", func(c *FetchCacheConfig) { c.NegativeCacheTTL = time.Minute }},
		{"IncludeSubgraphHeaders", func(c *FetchCacheConfig) { c.IncludeSubgraphHeaders = false }},
		{"Private", func(c *FetchCacheConfig) { c.Private = false }},
		{"EnablePartialCacheLoad", func(c *FetchCacheConfig) { c.EnablePartialCacheLoad = false }},
		{"PartialBatchLoad", func(c *FetchCacheConfig) { c.PartialBatchLoad = false }},
		{"ShadowMode", func(c *FetchCacheConfig) { c.ShadowMode = false }},
		{"HashAnalyticsKeys", func(c *FetchCacheConfig) { c.HashAnalyticsKeys = false }},
		{"PopulateL2OnMutation", func(c *FetchCacheConfig) { c.PopulateL2OnMutation = false }},
		{"MutationTTLOverride", func(c *FetchCacheConfig) { c.MutationTTLOverride = time.Minute }},
		{"KeySpec.Scope", func(c *FetchCacheConfig) { c.KeySpec.Scope = CacheScopeRootField }},
		{"KeySpec.TypeName", func(c *FetchCacheConfig) { c.KeySpec.TypeName = "Review" }},
		{"KeySpec.FieldName", func(c *FetchCacheConfig) { c.KeySpec.FieldName = "review" }},
		{"KeySpec.Representation nil", func(c *FetchCacheConfig) { c.KeySpec.Representation = nil }},
		{"KeySpec.Representation shape", func(c *FetchCacheConfig) {
			c.KeySpec.Representation.Fields[0].Name = []byte("sku")
		}},
		{"ProvidesData nil", func(c *FetchCacheConfig) { c.ProvidesData = nil }},
		{"ProvidesData shape", func(c *FetchCacheConfig) {
			c.ProvidesData.Fields[0].Name = []byte("title")
		}},
	}
	for _, row := range mutations {
		t.Run("differ in "+row.name, func(t *testing.T) {
			mutated := fullFetchCacheConfig()
			row.mutate(mutated)
			assert.False(t, fullFetchCacheConfig().Equals(mutated))
			assert.False(t, mutated.Equals(fullFetchCacheConfig()))
		})
	}
}

// TestFetchConfigurationEqualsCacheClause covers the plan-dedup cache clause.
func TestFetchConfigurationEqualsCacheClause(t *testing.T) {
	t.Run("[P1] both nil", func(t *testing.T) {
		a := &FetchConfiguration{}
		b := &FetchConfiguration{}
		assert.True(t, a.Equals(b))
	})

	t.Run("[P2] one nil", func(t *testing.T) {
		a := &FetchConfiguration{Cache: fullFetchCacheConfig()}
		b := &FetchConfiguration{}
		assert.False(t, a.Equals(b))
		assert.False(t, b.Equals(a))
	})

	t.Run("[P3] both non-nil, equal", func(t *testing.T) {
		a := &FetchConfiguration{Cache: fullFetchCacheConfig()}
		b := &FetchConfiguration{Cache: fullFetchCacheConfig()}
		assert.True(t, a.Equals(b))
	})

	t.Run("[P4] differ in one field", func(t *testing.T) {
		a := &FetchConfiguration{Cache: fullFetchCacheConfig()}
		b := &FetchConfiguration{Cache: fullFetchCacheConfig()}
		b.Cache.TTL = time.Minute
		assert.False(t, a.Equals(b))
	})

	t.Run("[P4] differ in the MaxTTL clamp", func(t *testing.T) {
		a := &FetchConfiguration{Cache: fullFetchCacheConfig()}
		b := &FetchConfiguration{Cache: fullFetchCacheConfig()}
		b.Cache.MaxTTL = time.Hour
		assert.False(t, a.Equals(b))
	})

	t.Run("[P5] differ in the representation shape", func(t *testing.T) {
		a := &FetchConfiguration{Cache: fullFetchCacheConfig()}
		b := &FetchConfiguration{Cache: fullFetchCacheConfig()}
		b.Cache.KeySpec.Representation.Fields[0].Name = []byte("sku")
		assert.False(t, a.Equals(b))
	})
}

func TestFetchCacheConfigString(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var cfg *FetchCacheConfig
		assert.Equal(t, "<nil>", cfg.String())
	})

	t.Run("populated", func(t *testing.T) {
		assert.Equal(t,
			"{l1:true l2:true subgraph:products ttl:30s maxTTL:2m0s negativeTTL:5s includeHeaders:true private:true partial:true partialBatch:true shadow:true hashAnalytics:true scope:Entity type:Product field:product representation:true providesData:true populateL2OnMutation:true mutationTTL:10s}",
			fullFetchCacheConfig().String())
	})

	t.Run("zero value", func(t *testing.T) {
		assert.Equal(t,
			"{l1:false l2:false subgraph: ttl:0s maxTTL:0s negativeTTL:0s includeHeaders:false private:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:RootField type: field: representation:false providesData:false populateL2OnMutation:false mutationTTL:0s}",
			(&FetchCacheConfig{}).String())
	})
}

func TestCacheScopeString(t *testing.T) {
	assert.Equal(t, "RootField", CacheScopeRootField.String())
	assert.Equal(t, "Entity", CacheScopeEntity.String())
	assert.Equal(t, "CacheScope(7)", CacheScope(7).String())
}
