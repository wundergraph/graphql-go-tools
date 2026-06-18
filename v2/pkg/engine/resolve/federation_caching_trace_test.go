package resolve

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFederationCachingTrace(t *testing.T) {
	t.Run("builds cache trace when tracing enabled", func(t *testing.T) {
		loader := &Loader{
			ctx: &Context{
				TracingOptions: TraceOptions{
					Enable: true,
				},
				ExecutionOptions: ExecutionOptions{
					Caching: CachingOptions{
						EnableL1Cache: true,
						EnableL2Cache: true,
					},
				},
			},
		}
		res := &result{
			cacheKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"User","key":{"id":"1"}}`,
					},
				},
			},
			cacheTraceDurationSinceStartNano: 7,
			cacheTraceDurationNano:           11,
			cacheTraceEntityCount:            1,
			cacheTraceL1Hits:                 0,
			cacheTraceL1Misses:               1,
			cacheTraceRequestScopedHits:      1,
			cacheTraceL2Hits:                 1,
			cacheTraceL2Misses:               0,
			cacheTraceL2GetDuration:          3 * time.Nanosecond,
			cacheTraceEntityDetails: []CacheTraceEntity{
				{
					Key:      `{"__typename":"User","key":{"id":"1"}}`,
					Source:   "l2",
					ByteSize: 37,
				},
			},
		}

		actual := loader.buildCacheTrace(res, &FetchCacheConfiguration{
			CacheName:        "default",
			EnableL2Cache:    true,
			TTL:              30 * time.Second,
			UseL1Cache:       true,
			KeyTemplate:      &EntityQueryCacheKeyTemplate{TypeName: "User"},
			ProvidesData:     &Object{},
			ShadowMode:       true,
			NegativeCacheTTL: 5 * time.Second,
		})

		assert.Equal(t, &CacheTrace{
			DurationSinceStartNano:   7,
			DurationSinceStartPretty: "7ns",
			DurationNano:             11,
			DurationPretty:           "11ns",
			L1Enabled:                true,
			L2Enabled:                true,
			CacheName:                "default",
			TTLSeconds:               30,
			EntityCount:              1,
			L1Hit:                    1,
			L1Miss:                   0,
			L2Hit:                    1,
			L2Miss:                   0,
			L2GetDurationNano:        3,
			L2GetDurationPretty:      "3ns",
			ShadowMode:               true,
			Entities: []CacheTraceEntity{
				{
					Key:      `{"__typename":"User","key":{"id":"1"}}`,
					Source:   "l2",
					ByteSize: 37,
				},
			},
			Keys: []string{
				`{"__typename":"User","key":{"id":"1"}}`,
			},
		}, actual)
	})

	t.Run("does not build cache trace when tracing disabled", func(t *testing.T) {
		loader := &Loader{
			ctx: &Context{
				TracingOptions: TraceOptions{},
			},
		}

		actual := loader.buildCacheTrace(&result{}, &FetchCacheConfiguration{
			EnableL2Cache: true,
			KeyTemplate:   &EntityQueryCacheKeyTemplate{TypeName: "User"},
		})

		assert.Nil(t, actual)
	})

	t.Run("does not build cache trace when cache stats excluded", func(t *testing.T) {
		loader := &Loader{
			ctx: &Context{
				TracingOptions: TraceOptions{
					Enable:            true,
					ExcludeCacheStats: true,
				},
			},
		}

		actual := loader.buildCacheTrace(&result{}, &FetchCacheConfiguration{
			EnableL2Cache: true,
			KeyTemplate:   &EntityQueryCacheKeyTemplate{TypeName: "User"},
		})

		assert.Nil(t, actual)
	})

	t.Run("omits keys when raw input data excluded", func(t *testing.T) {
		loader := &Loader{
			ctx: &Context{
				TracingOptions: TraceOptions{
					Enable:              true,
					ExcludeRawInputData: true,
				},
				ExecutionOptions: ExecutionOptions{
					Caching: CachingOptions{
						EnableL2Cache: true,
					},
				},
			},
		}
		res := &result{
			cacheKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"Query","field":"me"}`,
					},
				},
			},
			cacheTraceEntityCount: 1,
		}

		actual := loader.buildCacheTrace(res, &FetchCacheConfiguration{
			CacheName:     "default",
			EnableL2Cache: true,
			KeyTemplate: &RootQueryCacheKeyTemplate{
				RootFields: []RootField{
					{
						TypeName:  "Query",
						FieldName: "me",
					},
				},
			},
		})

		assert.Equal(t, &CacheTrace{
			L2Enabled:   true,
			CacheName:   "default",
			EntityCount: 1,
		}, actual)
	})
}

func TestCacheTraceEmittedOnFetchTrace(t *testing.T) {
	loader := &Loader{
		ctx: &Context{
			TracingOptions: TraceOptions{
				Enable: true,
			},
			ExecutionOptions: ExecutionOptions{
				Caching: CachingOptions{
					EnableL2Cache: true,
				},
			},
		},
	}
	fetch := &SingleFetch{}
	res := &result{
		cacheKeys: []*CacheKey{
			{
				Keys: []string{
					`{"__typename":"Query","field":"me"}`,
				},
			},
		},
		cacheTraceEntityCount: 1,
		cacheTraceL2Misses:    1,
	}

	loader.attachCacheTrace(fetch, res, &FetchCacheConfiguration{
		CacheName:                       "default",
		EnableL2Cache:                   true,
		EnableMutationL2CachePopulation: true,
		KeyTemplate: &RootQueryCacheKeyTemplate{
			RootFields: []RootField{
				{
					TypeName:  "Query",
					FieldName: "me",
				},
			},
		},
	})

	assert.Equal(t, &DataSourceLoadTrace{
		CacheTrace: &CacheTrace{
			L2Enabled:   true,
			CacheName:   "default",
			EntityCount: 1,
			L2Miss:      1,
			Keys: []string{
				`{"__typename":"Query","field":"me"}`,
			},
		},
	}, fetch.Trace)
}

func TestCacheTraceNotEmittedWhenRequestCachingDisabled(t *testing.T) {
	loader := &Loader{
		ctx: &Context{
			TracingOptions: TraceOptions{
				Enable: true,
			},
		},
	}
	fetch := &SingleFetch{}
	res := &result{
		cacheTraceEntityCount: 1,
		cacheTraceL2Misses:    1,
	}

	loader.attachCacheTrace(fetch, res, &FetchCacheConfiguration{
		CacheName:     "default",
		EnableL2Cache: true,
		KeyTemplate: &RootQueryCacheKeyTemplate{
			RootFields: []RootField{
				{
					TypeName:  "Query",
					FieldName: "me",
				},
			},
		},
	})

	assert.Nil(t, fetch.Trace)
}
