package resolve

import (
	"context"
	"testing"

	"github.com/cespare/xxhash/v2"
)

var cacheAnalyticsBenchSink uint64

func BenchmarkCacheAnalytics_Disabled(b *testing.B) {
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableCacheAnalytics = false
	event := CacheKeyEvent{
		Key:        `{"__typename":"User","key":{"id":"1"}}`,
		EntityType: "User",
		Hit:        true,
		Bytes:      64,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		ctx.cacheAnalytics().recordL1Read(event)
	}
}

func BenchmarkCacheAnalytics_Enabled(b *testing.B) {
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
	collector := ctx.cacheAnalytics()
	collector.l1Reads = make([]CacheKeyEvent, 0, 1024)
	event := CacheKeyEvent{
		Key:        `{"__typename":"User","key":{"id":"1"}}`,
		EntityType: "User",
		Hit:        true,
		Bytes:      64,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if len(collector.l1Reads) == cap(collector.l1Reads) {
			collector.l1Reads = collector.l1Reads[:0]
		}
		collector.recordL1Read(event)
	}
}

func BenchmarkCacheAnalytics_FieldHashing(b *testing.B) {
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
	collector := ctx.cacheAnalytics()
	collector.fieldHashes = make([]FieldHashEvent, 0, 1024)
	fieldPath := []byte("User.profile.displayName")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if len(collector.fieldHashes) == cap(collector.fieldHashes) {
			collector.fieldHashes = collector.fieldHashes[:0]
		}
		hash := xxhash.Sum64(fieldPath)
		collector.recordFieldHash(FieldHashEvent{
			EntityType: "User",
			FieldPath:  "profile.displayName",
			Hash:       hash,
		})
		cacheAnalyticsBenchSink = hash
	}
}
