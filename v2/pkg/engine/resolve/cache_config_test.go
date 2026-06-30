package resolve

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFetchConfigurationEqualsCacheClause(t *testing.T) {
	t.Run("[P1] both nil -> dedup", func(t *testing.T) {
		left := cacheTestFetchConfiguration(nil)
		right := cacheTestFetchConfiguration(nil)

		assert.Equal(t, true, left.Equals(&right))
	})

	t.Run("[P2] one nil one set -> not equal", func(t *testing.T) {
		left := cacheTestFetchConfiguration(nil)
		right := cacheTestFetchConfiguration(cacheTestFetchCacheConfig())

		assert.Equal(t, false, left.Equals(&right))
	})

	t.Run("[P3] both set and equal -> dedup", func(t *testing.T) {
		left := cacheTestFetchConfiguration(cacheTestFetchCacheConfig())
		right := cacheTestFetchConfiguration(cacheTestFetchCacheConfig())

		assert.Equal(t, true, left.Equals(&right))
	})

	t.Run("[P4] scalar field differs -> not equal", func(t *testing.T) {
		leftCache := cacheTestFetchCacheConfig()
		rightCache := cacheTestFetchCacheConfig()
		rightCache.TTL = 2 * time.Minute
		left := cacheTestFetchConfiguration(leftCache)
		right := cacheTestFetchConfiguration(rightCache)

		assert.Equal(t, false, left.Equals(&right))
	})

	t.Run("[P5] candidate representation differs -> not equal", func(t *testing.T) {
		leftCache := cacheTestFetchCacheConfig()
		rightCache := cacheTestFetchCacheConfig()
		rightCache.KeySpec.Candidates[0].Representation = &Object{
			Fields: []*Field{
				{Name: []byte("__typename"), Value: &String{}},
				{Name: []byte("sku"), Value: &String{}},
			},
		}
		left := cacheTestFetchConfiguration(leftCache)
		right := cacheTestFetchConfiguration(rightCache)

		assert.Equal(t, false, left.Equals(&right))
	})
}

func TestFetchCacheConfigString(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var cfg *FetchCacheConfig

		assert.Equal(t, "<nil>", cfg.String())
	})

	t.Run("populated value", func(t *testing.T) {
		cfg := cacheTestFetchCacheConfig()

		assert.Equal(t, "{l1:true l2:true cacheName:products ttl:1m0s negativeTTL:5s includeHeaders:true partial:true partialBatch:false shadow:false hashAnalytics:true scope:Entity type:Product field: candidates:1 entityKeyMappings:1 providesData:true populateL2OnMutation:true mutationTTL:30s}", cfg.String())
	})
}

func TestFetchCacheHandleString(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var handle *FetchCacheHandle

		assert.Equal(t, "<nil>", handle.String())
	})

	t.Run("populated value", func(t *testing.T) {
		handle := &FetchCacheHandle{
			Decision:      DecisionSkipFullHit,
			WasHit:        true,
			MustWriteBack: true,
			Items: []ItemCacheState{
				{FromCache: nil, NeedsWriteback: true},
				{FromCache: nil, NeedsWriteback: false},
				{FromCache: nil, NeedsWriteback: false},
			},
		}

		assert.Equal(t, "{decision:SkipFullHit items:3 hits:3 writeback:1 shadow:false}", handle.String())
	})
}

func cacheTestFetchConfiguration(cache *FetchCacheConfig) FetchConfiguration {
	return FetchConfiguration{
		Input:      `{"query":"query Product { product { id name } }"}`,
		Variables:  nil,
		DataSource: nil,
		PostProcessing: PostProcessingConfiguration{
			SelectResponseDataPath: []string{"data", "product"},
			MergePath:              []string{"product"},
		},
		Cache: cache,
	}
}

func cacheTestFetchCacheConfig() *FetchCacheConfig {
	return &FetchCacheConfig{
		L1:                          true,
		L2:                          true,
		CacheName:                   "products",
		TTL:                         time.Minute,
		NegativeCacheTTL:            5 * time.Second,
		IncludeSubgraphHeaderPrefix: true,
		EnablePartialCacheLoad:      true,
		PartialBatchLoad:            false,
		ShadowMode:                  false,
		HashAnalyticsKeys:           true,
		KeySpec: CacheKeySpec{
			Scope:    CacheScopeEntity,
			TypeName: "Product",
			Candidates: []CacheKeyCandidate{
				{
					Representation: &Object{
						Fields: []*Field{
							{Name: []byte("__typename"), Value: &String{}},
							{Name: []byte("id"), Value: &String{}},
						},
					},
				},
			},
			EntityKeyMappings: []EntityKeyMapping{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMapping{
						{
							EntityKeyField:      "id",
							ArgumentPath:        []string{"id"},
							ArgumentIsEntityKey: true,
						},
					},
				},
			},
		},
		ProvidesData: &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &String{}},
				{Name: []byte("name"), Value: &String{}},
			},
		},
		PopulateL2OnMutation: true,
		MutationTTLOverride:  30 * time.Second,
	}
}
