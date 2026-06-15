package resolve

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCacheAnalyticsCollectorRecordAndSnapshotBuild(t *testing.T) {
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
	collector := ctx.cacheAnalytics()

	collector.recordL1Read(CacheKeyEvent{
		Key:        "entity:User:1",
		EntityType: "User",
		Kind:       CacheAnalyticsEventKindL1Read,
		Hit:        true,
		Bytes:      48,
	})
	collector.recordL2Read(CacheKeyEvent{
		Key:        "entity:User:2",
		EntityType: "User",
		Kind:       CacheAnalyticsEventKindL2Read,
		Hit:        false,
	})
	collector.recordL1Write(CacheWriteEvent{
		Key:        "entity:User:1",
		EntityType: "User",
		Kind:       CacheAnalyticsEventKindL1Write,
		Bytes:      64,
		Reason:     CacheWriteReasonRefresh,
	})
	collector.recordL2Write(CacheWriteEvent{
		Key:        "entity:User:2",
		EntityType: "User",
		Kind:       CacheAnalyticsEventKindL2Write,
		Bytes:      72,
		TTL:        30 * time.Second,
		Reason:     CacheWriteReasonRefresh,
	})
	collector.recordFetchTiming(FetchTimingEvent{
		SubgraphName: "users",
		CacheName:    "entities",
		Operation:    "fetch",
		Duration:     5 * time.Millisecond,
		Bytes:        120,
	})
	collector.recordEntityType(EntityTypeEvent{
		EntityType: "User",
		Count:      2,
	})
	collector.recordFieldHash(FieldHashEvent{
		EntityType: "User",
		FieldPath:  "User.name",
		Hash:       77,
	})
	collector.recordHeaderImpact(HeaderImpactEvent{
		SubgraphName: "users",
		CacheName:    "entities",
		HeaderHash:   "99",
		KeyPrefix:    "99:",
	})
	collector.recordCacheOperationError(CacheOperationError{
		Operation: "get",
		CacheName: "entities",
		Key:       "entity:User:3",
		Error:     "backend unavailable",
	})

	actual := ctx.GetCacheStats()

	assert.Equal(t, CacheAnalyticsSnapshot{
		L1Reads: []CacheKeyEvent{
			{
				Key:        "entity:User:1",
				EntityType: "User",
				Kind:       CacheAnalyticsEventKindL1Read,
				Hit:        true,
				Bytes:      48,
			},
		},
		L2Reads: []CacheKeyEvent{
			{
				Key:        "entity:User:2",
				EntityType: "User",
				Kind:       CacheAnalyticsEventKindL2Read,
				Hit:        false,
			},
		},
		L1Writes: []CacheWriteEvent{
			{
				Key:        "entity:User:1",
				EntityType: "User",
				Kind:       CacheAnalyticsEventKindL1Write,
				Bytes:      64,
				Reason:     CacheWriteReasonRefresh,
			},
		},
		L2Writes: []CacheWriteEvent{
			{
				Key:        "entity:User:2",
				EntityType: "User",
				Kind:       CacheAnalyticsEventKindL2Write,
				Bytes:      72,
				TTL:        30 * time.Second,
				Reason:     CacheWriteReasonRefresh,
			},
		},
		FetchTimings: []FetchTimingEvent{
			{
				SubgraphName: "users",
				CacheName:    "entities",
				Operation:    "fetch",
				Duration:     5 * time.Millisecond,
				Bytes:        120,
			},
		},
		EntityTypes: []EntityTypeEvent{
			{
				EntityType: "User",
				Count:      2,
			},
		},
		FieldHashes: []FieldHashEvent{
			{
				EntityType: "User",
				FieldPath:  "User.name",
				Hash:       77,
			},
		},
		HeaderImpactEvents: []HeaderImpactEvent{
			{
				SubgraphName: "users",
				CacheName:    "entities",
				HeaderHash:   "99",
				KeyPrefix:    "99:",
			},
		},
		CacheOpErrors: []CacheOperationError{
			{
				Operation: "get",
				CacheName: "entities",
				Key:       "entity:User:3",
				Error:     "backend unavailable",
			},
		},
	}, actual)
	assert.Equal(t, CacheAnalyticsSnapshot{}, ctx.GetCacheStats())
}

func TestCacheAnalyticsFieldHashingAndEntityCounts(t *testing.T) {
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
	collector := ctx.cacheAnalytics()

	collector.recordFieldHash(FieldHashEvent{
		EntityType: "User",
		FieldPath:  "User.name",
		Hash:       11,
	})
	collector.recordFieldHash(FieldHashEvent{
		EntityType: "Product",
		FieldPath:  "Product.price",
		Hash:       22,
	})
	collector.recordEntityType(EntityTypeEvent{
		EntityType: "User",
		Count:      3,
	})
	collector.recordEntityType(EntityTypeEvent{
		EntityType: "Product",
		Count:      1,
	})

	snapshot := ctx.GetCacheStats()

	assert.Equal(t, []FieldHashEvent{
		{
			EntityType: "User",
			FieldPath:  "User.name",
			Hash:       11,
		},
		{
			EntityType: "Product",
			FieldPath:  "Product.price",
			Hash:       22,
		},
	}, snapshot.FieldHashes)
	assert.Equal(t, map[string]int{
		"Product": 1,
		"User":    3,
	}, snapshot.EventsByEntityType())
}

func TestCacheAnalyticsDerivedMetrics(t *testing.T) {
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
	collector := ctx.cacheAnalytics()

	collector.recordL1Read(CacheKeyEvent{
		Key:        "entity:User:1",
		EntityType: "User",
		Kind:       CacheAnalyticsEventKindL1Read,
		Hit:        true,
		Bytes:      10,
	})
	collector.recordL1Read(CacheKeyEvent{
		Key:        "entity:User:2",
		EntityType: "User",
		Kind:       CacheAnalyticsEventKindL1Read,
		Hit:        false,
		Bytes:      20,
	})
	collector.recordL2Read(CacheKeyEvent{
		Key:        "entity:User:3",
		EntityType: "User",
		Kind:       CacheAnalyticsEventKindL2Read,
		Hit:        true,
		Bytes:      30,
	})
	collector.recordL2Read(CacheKeyEvent{
		Key:        "entity:User:4",
		EntityType: "User",
		Kind:       CacheAnalyticsEventKindL2Read,
		Hit:        true,
		Bytes:      40,
	})
	collector.recordL2Read(CacheKeyEvent{
		Key:        "entity:User:5",
		EntityType: "User",
		Kind:       CacheAnalyticsEventKindL2Read,
		Hit:        false,
		Bytes:      50,
	})

	snapshot := ctx.GetCacheStats()

	assert.Equal(t, 0.5, snapshot.L1HitRate())
	assert.Equal(t, 2.0/3.0, snapshot.L2HitRate())
	assert.Equal(t, 80, snapshot.CachedBytesServed())
}

func TestCacheAnalyticsSnapshotDedup(t *testing.T) {
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
	collector := ctx.cacheAnalytics()

	collector.recordL1Read(CacheKeyEvent{
		Key:        "entity:User:1",
		EntityType: "User",
		Kind:       CacheAnalyticsEventKindL1Read,
		Hit:        true,
		Bytes:      10,
	})
	collector.recordL1Read(CacheKeyEvent{
		Key:        "entity:User:1",
		EntityType: "User",
		Kind:       CacheAnalyticsEventKindL1Read,
		Hit:        false,
		Bytes:      99,
	})
	collector.recordL2Read(CacheKeyEvent{
		Key:        "entity:User:1",
		EntityType: "User",
		Kind:       CacheAnalyticsEventKindL2Read,
		Hit:        true,
		Bytes:      20,
	})
	collector.recordL2Write(CacheWriteEvent{
		Key:        "entity:User:1",
		EntityType: "User",
		Kind:       CacheAnalyticsEventKindL2Write,
		Bytes:      30,
		Reason:     CacheWriteReasonRefresh,
	})
	collector.recordL2Write(CacheWriteEvent{
		Key:        "entity:User:1",
		EntityType: "User",
		Kind:       CacheAnalyticsEventKindL2Write,
		Bytes:      300,
		Reason:     CacheWriteReasonDerived,
	})

	assert.Equal(t, CacheAnalyticsSnapshot{
		L1Reads: []CacheKeyEvent{
			{
				Key:        "entity:User:1",
				EntityType: "User",
				Kind:       CacheAnalyticsEventKindL1Read,
				Hit:        true,
				Bytes:      10,
			},
		},
		L2Reads: []CacheKeyEvent{
			{
				Key:        "entity:User:1",
				EntityType: "User",
				Kind:       CacheAnalyticsEventKindL2Read,
				Hit:        true,
				Bytes:      20,
			},
		},
		L2Writes: []CacheWriteEvent{
			{
				Key:        "entity:User:1",
				EntityType: "User",
				Kind:       CacheAnalyticsEventKindL2Write,
				Bytes:      30,
				Reason:     CacheWriteReasonRefresh,
			},
		},
	}, ctx.GetCacheStats())
}

func TestCacheAnalyticsDisabledPathReturnsEmptySnapshot(t *testing.T) {
	ctx := NewContext(context.Background())

	assert.Nil(t, ctx.cacheAnalytics())
	assert.Equal(t, CacheAnalyticsSnapshot{}, ctx.GetCacheStats())
	assert.Nil(t, ctx.cacheAnalyticsCollector)
}
