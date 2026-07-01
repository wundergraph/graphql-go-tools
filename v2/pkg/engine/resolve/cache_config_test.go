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
		L1:                          true,
		L2:                          true,
		CacheName:                   "products",
		TTL:                         30 * time.Second,
		NegativeCacheTTL:            5 * time.Second,
		IncludeSubgraphHeaderPrefix: true,
		EnablePartialCacheLoad:      true,
		PartialBatchLoad:            true,
		ShadowMode:                  true,
		HashAnalyticsKeys:           true,
		KeySpec: CacheKeySpec{
			Scope:     CacheScopeEntity,
			TypeName:  "Product",
			FieldName: "product",
			Candidates: []CacheKeyCandidate{
				{
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
			},
			EntityKeyMappings: []EntityKeyMapping{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMapping{
						{
							EntityKeyField:      "upc",
							ArgumentPath:        []string{"upc"},
							ArgumentIsEntityKey: true,
						},
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
		{"CacheName", func(c *FetchCacheConfig) { c.CacheName = "reviews" }},
		{"TTL", func(c *FetchCacheConfig) { c.TTL = time.Minute }},
		{"NegativeCacheTTL", func(c *FetchCacheConfig) { c.NegativeCacheTTL = time.Minute }},
		{"IncludeSubgraphHeaderPrefix", func(c *FetchCacheConfig) { c.IncludeSubgraphHeaderPrefix = false }},
		{"EnablePartialCacheLoad", func(c *FetchCacheConfig) { c.EnablePartialCacheLoad = false }},
		{"PartialBatchLoad", func(c *FetchCacheConfig) { c.PartialBatchLoad = false }},
		{"ShadowMode", func(c *FetchCacheConfig) { c.ShadowMode = false }},
		{"HashAnalyticsKeys", func(c *FetchCacheConfig) { c.HashAnalyticsKeys = false }},
		{"PopulateL2OnMutation", func(c *FetchCacheConfig) { c.PopulateL2OnMutation = false }},
		{"MutationTTLOverride", func(c *FetchCacheConfig) { c.MutationTTLOverride = time.Minute }},
		{"KeySpec.Scope", func(c *FetchCacheConfig) { c.KeySpec.Scope = CacheScopeRootField }},
		{"KeySpec.TypeName", func(c *FetchCacheConfig) { c.KeySpec.TypeName = "Review" }},
		{"KeySpec.FieldName", func(c *FetchCacheConfig) { c.KeySpec.FieldName = "review" }},
		{"KeySpec.Candidates length", func(c *FetchCacheConfig) { c.KeySpec.Candidates = nil }},
		{"KeySpec.Candidates representation", func(c *FetchCacheConfig) {
			c.KeySpec.Candidates[0].Representation.Fields[0].Name = []byte("sku")
		}},
		{"KeySpec.Candidates nil representation", func(c *FetchCacheConfig) {
			c.KeySpec.Candidates[0].Representation = nil
		}},
		{"KeySpec.EntityKeyMappings entity type", func(c *FetchCacheConfig) {
			c.KeySpec.EntityKeyMappings[0].EntityTypeName = "Review"
		}},
		{"KeySpec.EntityKeyMappings key field", func(c *FetchCacheConfig) {
			c.KeySpec.EntityKeyMappings[0].FieldMappings[0].EntityKeyField = "sku"
		}},
		{"KeySpec.EntityKeyMappings argument path", func(c *FetchCacheConfig) {
			c.KeySpec.EntityKeyMappings[0].FieldMappings[0].ArgumentPath = []string{"sku"}
		}},
		{"KeySpec.EntityKeyMappings argument is entity key", func(c *FetchCacheConfig) {
			c.KeySpec.EntityKeyMappings[0].FieldMappings[0].ArgumentIsEntityKey = false
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

// TestFetchConfigurationEqualsCacheClause covers the plan-dedup cache clause,
// appendix rows P1–P5.
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

	t.Run("[P5] differ in one candidate representation", func(t *testing.T) {
		a := &FetchConfiguration{Cache: fullFetchCacheConfig()}
		b := &FetchConfiguration{Cache: fullFetchCacheConfig()}
		b.Cache.KeySpec.Candidates[0].Representation.Fields[0].Name = []byte("sku")
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
			"{l1:true l2:true cacheName:products ttl:30s negativeTTL:5s includeHeaders:true partial:true partialBatch:true shadow:true hashAnalytics:true scope:Entity type:Product field:product candidates:1 entityKeyMappings:1 providesData:true populateL2OnMutation:true mutationTTL:10s}",
			fullFetchCacheConfig().String())
	})

	t.Run("zero value", func(t *testing.T) {
		assert.Equal(t,
			"{l1:false l2:false cacheName: ttl:0s negativeTTL:0s includeHeaders:false partial:false partialBatch:false shadow:false hashAnalytics:false scope:RootField type: field: candidates:0 entityKeyMappings:0 providesData:false populateL2OnMutation:false mutationTTL:0s}",
			(&FetchCacheConfig{}).String())
	})
}

func TestCacheScopeString(t *testing.T) {
	assert.Equal(t, "RootField", CacheScopeRootField.String())
	assert.Equal(t, "Entity", CacheScopeEntity.String())
	assert.Equal(t, "CacheScope(7)", CacheScope(7).String())
}
