package resolve

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestFetchConfigurationEquals_CachingDifference verifies that FetchCacheConfiguration.Equals
// detects differences in every compared field. The field count guard ensures that adding a new
// field to FetchCacheConfiguration forces an update to both Equals() and this test.
func TestFetchConfigurationEquals_CachingDifference(t *testing.T) {
	base := FetchConfiguration{
		Input: `{"query":"{ user { id } }"}`,
		Caching: FetchCacheConfiguration{
			Enabled:                         true,
			CacheName:                       "default",
			TTL:                             30 * time.Second,
			IncludeSubgraphHeaderPrefix:     true,
			EnablePartialCacheLoad:          true,
			ShadowMode:                      false,
			EnableMutationL2CachePopulation: false,
			MutationCacheTTLOverride:        0,
			NegativeCacheTTL:                0,
		},
	}

	tests := []struct {
		name   string
		mutate func(fc *FetchConfiguration)
	}{
		{
			name: "Enabled differs",
			mutate: func(fc *FetchConfiguration) {
				fc.Caching.Enabled = false
			},
		},
		{
			name: "CacheName differs",
			mutate: func(fc *FetchConfiguration) {
				fc.Caching.CacheName = "other"
			},
		},
		{
			name: "TTL differs",
			mutate: func(fc *FetchConfiguration) {
				fc.Caching.TTL = 60 * time.Second
			},
		},
		{
			name: "IncludeSubgraphHeaderPrefix differs",
			mutate: func(fc *FetchConfiguration) {
				fc.Caching.IncludeSubgraphHeaderPrefix = false
			},
		},
		{
			name: "EnablePartialCacheLoad differs",
			mutate: func(fc *FetchConfiguration) {
				fc.Caching.EnablePartialCacheLoad = false
			},
		},
		{
			name: "ShadowMode differs",
			mutate: func(fc *FetchConfiguration) {
				fc.Caching.ShadowMode = true
			},
		},
		{
			name: "EnableMutationL2CachePopulation differs",
			mutate: func(fc *FetchConfiguration) {
				fc.Caching.EnableMutationL2CachePopulation = true
			},
		},
		{
			name: "MutationCacheTTLOverride differs",
			mutate: func(fc *FetchConfiguration) {
				fc.Caching.MutationCacheTTLOverride = 10 * time.Second
			},
		},
		{
			name: "NegativeCacheTTL differs",
			mutate: func(fc *FetchConfiguration) {
				fc.Caching.NegativeCacheTTL = 5 * time.Second
			},
		},
		{
			name: "PartialBatchLoad differs",
			mutate: func(fc *FetchConfiguration) {
				fc.Caching.PartialBatchLoad = true
			},
		},
		{
			name: "BatchEntityKeyArgumentPathHint differs",
			mutate: func(fc *FetchConfiguration) {
				fc.Caching.BatchEntityKeyArgumentPathHint = []string{"upcs"}
			},
		},
	}

	// Fields intentionally not compared by Equals (not relevant for fetch deduplication):
	// CacheKeyTemplate, RootFieldL1EntityCacheKeyTemplates, UseL1Cache,
	// HashAnalyticsKeys, KeyFields, MutationEntityImpactConfig,
	// RequestScopedFields
	skippedFields := 7

	totalFields := reflect.TypeFor[FetchCacheConfiguration]().NumField()
	assert.Equal(t, totalFields, len(tests)+skippedFields,
		"FetchCacheConfiguration has %d fields but test covers %d and skips %d — update this test and Equals() for new fields",
		totalFields, len(tests), skippedFields)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			other := base // copy
			tc.mutate(&other)
			assert.False(t, base.Equals(&other), "expected Equals to return false when %s", tc.name)
		})
	}

	t.Run("identical configs are equal", func(t *testing.T) {
		other := base // copy
		assert.True(t, base.Equals(&other))
	})
}
