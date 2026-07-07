package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFetchCachePolymorphism proves the Fetch interface cache accessors and
// shape predicates across ALL concrete fetch types, so caching code never
// needs a switch over concrete types.
func TestFetchCachePolymorphism(t *testing.T) {
	cases := []struct {
		name               string
		fetch              Fetch
		isEntityFetch      bool
		isBatchEntityFetch bool
	}{
		{
			name:               "SingleFetch",
			fetch:              &SingleFetch{},
			isEntityFetch:      false,
			isBatchEntityFetch: false,
		},
		{
			name:               "EntityFetch",
			fetch:              &EntityFetch{},
			isEntityFetch:      true,
			isBatchEntityFetch: false,
		},
		{
			name:               "BatchEntityFetch",
			fetch:              &BatchEntityFetch{},
			isEntityFetch:      false,
			isBatchEntityFetch: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Nil(t, tc.fetch.CacheConfig())

			cfg := &FetchCacheConfig{L2: true, CacheName: "products"}
			tc.fetch.SetCacheConfig(cfg)
			assert.Same(t, cfg, tc.fetch.CacheConfig())

			tc.fetch.SetCacheConfig(nil)
			assert.Nil(t, tc.fetch.CacheConfig())

			assert.Equal(t, tc.isEntityFetch, tc.fetch.IsEntityFetch())
			assert.Equal(t, tc.isBatchEntityFetch, tc.fetch.IsBatchEntityFetch())
		})
	}
}

// TestSingleFetchCacheConfigSharesFetchConfiguration pins that the SingleFetch
// accessor reads/writes the embedded FetchConfiguration.Cache field, which is
// what plan dedup compares.
func TestSingleFetchCacheConfigSharesFetchConfiguration(t *testing.T) {
	f := &SingleFetch{}
	cfg := &FetchCacheConfig{L1: true}
	f.SetCacheConfig(cfg)
	assert.Same(t, cfg, f.FetchConfiguration.Cache)
}
