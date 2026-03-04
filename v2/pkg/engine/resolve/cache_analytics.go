package resolve

import (
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"

	"github.com/wundergraph/astjson"
)

// CacheLevel indicates whether a cache operation targets L1 or L2.
type CacheLevel uint8

const (
	CacheLevelL1 CacheLevel = iota + 1
	CacheLevelL2
)

// CacheKeyEventKind classifies the result of a cache key lookup.
type CacheKeyEventKind uint8

const (
	CacheKeyHit        CacheKeyEventKind = iota + 1
	CacheKeyMiss                         // Key not found or value nil
	CacheKeyPartialHit                   // Key found but missing required fields
)

// FieldSource indicates where the data for an entity came from.
type FieldSource uint8

const (
	FieldSourceSubgraph     FieldSource = iota // Default: data came from subgraph fetch
	FieldSourceL1                              // Data came from L1 (per-request) cache
	FieldSourceL2                              // Data came from L2 (external) cache
	FieldSourceShadowCached                    // Cached value saved during shadow comparison
)

// CacheKeyEvent records a single cache key lookup result.
type CacheKeyEvent struct {
	CacheKey   string
	EntityType string
	Kind       CacheKeyEventKind
	DataSource string
	ByteSize   int
	CacheAgeMs int64 // age of cached entry in ms (L2 hits only, 0 = unknown)
	Shadow     bool  // true if this event occurred in shadow mode
}

// CacheWriteEvent records a single cache write operation.
type CacheWriteEvent struct {
	CacheKey   string
	EntityType string
	ByteSize   int
	DataSource string
	CacheLevel CacheLevel
	TTL        time.Duration
	Shadow     bool // true if this write occurred in shadow mode
}

// FetchTimingEvent records the duration of a subgraph fetch or cache lookup.
type FetchTimingEvent struct {
	DataSource     string      // subgraph name
	EntityType     string      // entity type (empty for root fetches)
	DurationMs     int64       // time spent on this operation in milliseconds
	Source         FieldSource // what handled this: Subgraph (fetch), L2 (cache GET)
	ItemCount      int         // number of entities in this fetch/lookup
	IsEntityFetch  bool        // true for _entities, false for root field
	HTTPStatusCode int         // HTTP status code from subgraph response (0 for cache hits)
	ResponseBytes  int         // response body size in bytes (0 for cache hits)
	TTFBMs         int64       // time to first byte in milliseconds (0 when unavailable)
}

// SubgraphErrorEvent records a subgraph error for analytics.
type SubgraphErrorEvent struct {
	DataSource string // subgraph name
	EntityType string // entity type (empty for root fetches)
	Message    string // error message (truncated for safety)
	Code       string // error code from errors[0].extensions.code (empty if not present)
}

// EntityFieldHash stores an xxhash of a scalar field value on an entity type,
// along with the entity's key data and the source of the data.
type EntityFieldHash struct {
	EntityType string
	FieldName  string
	FieldHash  uint64      // xxhash of the non-key field value
	KeyRaw     string      // raw key JSON e.g. {"id":"1234"} (when HashKeys=false)
	KeyHash    uint64      // xxhash of key JSON (when HashKeys=true)
	Source     FieldSource // where the entity data came from (L1/L2/Subgraph)
}

// EntityTypeInfo holds the entity type name and its instance count.
type EntityTypeInfo struct {
	TypeName   string
	Count      int
	UniqueKeys int // number of distinct entity keys
}

// entityCount is an internal type for accumulating entity counts.
type entityCount struct {
	typeName   string
	count      int
	uniqueKeys map[string]struct{} // set of seen entity key JSONs
}

// entitySourceRecord records where each entity's data came from.
type entitySourceRecord struct {
	entityType string
	keyJSON    string
	source     FieldSource
}

// ShadowComparisonEvent records a comparison between cached and fresh data in shadow mode.
type ShadowComparisonEvent struct {
	CacheKey      string        // cache key for correlation
	EntityType    string        // entity type name
	IsFresh       bool          // true if ProvidesData fields match between cached and fresh
	CachedHash    uint64        // xxhash of extracted ProvidesData fields from cached value
	FreshHash     uint64        // xxhash of extracted ProvidesData fields from fresh value
	CachedBytes   int           // byte size of cached ProvidesData fields
	FreshBytes    int           // byte size of fresh ProvidesData fields
	DataSource    string        // which subgraph provided the data (e.g. "accounts")
	CacheAgeMs    int64         // how old the cached entry was in milliseconds (0 = unknown)
	ConfiguredTTL time.Duration // TTL configured for this entity type
}

// MutationEvent records that a mutation returned a cacheable entity.
// Recorded during mutation execution by proactively comparing the mutation response
// with the L2 cached value for the same entity.
type MutationEvent struct {
	MutationRootField string // e.g., "updateUsername"
	EntityType        string // e.g., "User"
	EntityCacheKey    string // display key e.g. {"__typename":"User","key":{"id":"1234"}}
	HadCachedValue    bool   // true if L2 had a cached value for this entity
	IsStale           bool   // true if cached value differs from mutation response (always false when HadCachedValue=false)
	CachedHash        uint64 // xxhash of cached ProvidesData fields (0 when HadCachedValue=false)
	FreshHash         uint64 // xxhash of mutation response ProvidesData fields
	CachedBytes       int    // 0 when HadCachedValue=false
	FreshBytes        int
}

// CacheAnalyticsCollector accumulates cache analytics events during request execution.
// All methods are designed to be called from a single goroutine (main thread) except
// where noted. L2 events from goroutines are accumulated on per-result slices and
// merged on the main thread via MergeL2Events.
type CacheAnalyticsCollector struct {
	l1KeyEvents       []CacheKeyEvent
	l2KeyEvents       []CacheKeyEvent
	writeEvents       []CacheWriteEvent
	fieldHashes       []EntityFieldHash       // flat slice (was: nested maps)
	entityCounts      []entityCount           // simple type→count (was: map)
	entitySources     []entitySourceRecord    // records where each entity's data came from
	fetchTimings      []FetchTimingEvent      // main thread timings
	errorEvents       []SubgraphErrorEvent    // main thread errors
	l2ErrorEvents     []SubgraphErrorEvent    // accumulated in goroutines, merged on main thread
	l2FetchTimings    []FetchTimingEvent      // accumulated in goroutines, merged on main thread
	shadowComparisons []ShadowComparisonEvent // shadow mode staleness comparison events
	mutationEvents    []MutationEvent         // mutation entity impact events
	xxh               *xxhash.Digest
}

// NewCacheAnalyticsCollector creates a new collector with pre-allocated slices.
func NewCacheAnalyticsCollector() *CacheAnalyticsCollector {
	return &CacheAnalyticsCollector{
		l1KeyEvents:   make([]CacheKeyEvent, 0, 16),
		l2KeyEvents:   make([]CacheKeyEvent, 0, 16),
		writeEvents:   make([]CacheWriteEvent, 0, 8),
		fieldHashes:   make([]EntityFieldHash, 0, 32),
		entityCounts:  make([]entityCount, 0, 4),
		entitySources: make([]entitySourceRecord, 0, 16),
		fetchTimings:  make([]FetchTimingEvent, 0, 8),
		errorEvents:   make([]SubgraphErrorEvent, 0, 4),
		xxh:           xxhash.New(),
	}
}

// RecordL1KeyEvent records an L1 cache key lookup event. Main thread only.
func (c *CacheAnalyticsCollector) RecordL1KeyEvent(kind CacheKeyEventKind, entityType, cacheKey, dataSource string, byteSize int) {
	c.l1KeyEvents = append(c.l1KeyEvents, CacheKeyEvent{
		CacheKey:   cacheKey,
		EntityType: entityType,
		Kind:       kind,
		DataSource: dataSource,
		ByteSize:   byteSize,
	})
}

// RecordL2KeyEvent records an L2 cache key lookup event. Main thread only.
// Use MergeL2Events to merge events collected on per-result slices from goroutines.
func (c *CacheAnalyticsCollector) RecordL2KeyEvent(kind CacheKeyEventKind, entityType, cacheKey, dataSource string, byteSize int) {
	c.l2KeyEvents = append(c.l2KeyEvents, CacheKeyEvent{
		CacheKey:   cacheKey,
		EntityType: entityType,
		Kind:       kind,
		DataSource: dataSource,
		ByteSize:   byteSize,
	})
}

// MergeL2Events merges L2 events collected on a per-result slice (from goroutines)
// into the collector. Must be called on the main thread.
func (c *CacheAnalyticsCollector) MergeL2Events(events []CacheKeyEvent) {
	c.l2KeyEvents = append(c.l2KeyEvents, events...)
}

// RecordWrite records a cache write event. Main thread only.
func (c *CacheAnalyticsCollector) RecordWrite(cacheLevel CacheLevel, entityType, cacheKey, dataSource string, byteSize int, ttl time.Duration) {
	c.writeEvents = append(c.writeEvents, CacheWriteEvent{
		CacheKey:   cacheKey,
		EntityType: entityType,
		ByteSize:   byteSize,
		DataSource: dataSource,
		CacheLevel: cacheLevel,
		TTL:        ttl,
	})
}

// HashFieldValue computes an xxhash of the given field value bytes and records it
// as an EntityFieldHash with entity key and source information.
func (c *CacheAnalyticsCollector) HashFieldValue(entityType, fieldName string, valueBytes []byte, keyRaw string, keyHash uint64, source FieldSource) {
	c.xxh.Reset()
	_, _ = c.xxh.Write(valueBytes)
	hash := c.xxh.Sum64()

	c.fieldHashes = append(c.fieldHashes, EntityFieldHash{
		EntityType: entityType,
		FieldName:  fieldName,
		FieldHash:  hash,
		KeyRaw:     keyRaw,
		KeyHash:    keyHash,
		Source:     source,
	})
}

// IncrementEntityCount increments the instance count for the given entity type.
// If keyJSON is non-empty, it is tracked for unique key counting.
func (c *CacheAnalyticsCollector) IncrementEntityCount(typeName string, keyJSON string) {
	for i := range c.entityCounts {
		if c.entityCounts[i].typeName == typeName {
			c.entityCounts[i].count++
			if keyJSON != "" {
				if c.entityCounts[i].uniqueKeys == nil {
					c.entityCounts[i].uniqueKeys = make(map[string]struct{}, 4)
				}
				c.entityCounts[i].uniqueKeys[keyJSON] = struct{}{}
			}
			return
		}
	}
	var keys map[string]struct{}
	if keyJSON != "" {
		keys = map[string]struct{}{keyJSON: {}}
	}
	c.entityCounts = append(c.entityCounts, entityCount{typeName: typeName, count: 1, uniqueKeys: keys})
}

// RecordEntitySource records the source of data for a specific entity instance.
// Main thread only.
func (c *CacheAnalyticsCollector) RecordEntitySource(entityType, keyJSON string, source FieldSource) {
	c.entitySources = append(c.entitySources, entitySourceRecord{
		entityType: entityType,
		keyJSON:    keyJSON,
		source:     source,
	})
}

// MergeEntitySources merges entity source records collected in goroutines
// into the collector. Must be called on the main thread.
func (c *CacheAnalyticsCollector) MergeEntitySources(sources []entitySourceRecord) {
	c.entitySources = append(c.entitySources, sources...)
}

// RecordFetchTiming records a fetch timing event. Main thread only.
func (c *CacheAnalyticsCollector) RecordFetchTiming(event FetchTimingEvent) {
	c.fetchTimings = append(c.fetchTimings, event)
}

// MergeL2FetchTimings merges fetch timing events collected in goroutines into the collector.
// Must be called on the main thread.
func (c *CacheAnalyticsCollector) MergeL2FetchTimings(timings []FetchTimingEvent) {
	c.fetchTimings = append(c.fetchTimings, timings...)
}

// RecordError records a subgraph error event. Main thread only.
func (c *CacheAnalyticsCollector) RecordError(event SubgraphErrorEvent) {
	c.errorEvents = append(c.errorEvents, event)
}

// MergeL2Errors merges error events collected in goroutines into the collector.
// Must be called on the main thread.
func (c *CacheAnalyticsCollector) MergeL2Errors(events []SubgraphErrorEvent) {
	c.errorEvents = append(c.errorEvents, events...)
}

// RecordShadowComparison records a shadow mode comparison between cached and fresh data.
// Main thread only.
func (c *CacheAnalyticsCollector) RecordShadowComparison(event ShadowComparisonEvent) {
	c.shadowComparisons = append(c.shadowComparisons, event)
}

// RecordMutationEvent records a mutation entity impact event. Main thread only.
func (c *CacheAnalyticsCollector) RecordMutationEvent(event MutationEvent) {
	c.mutationEvents = append(c.mutationEvents, event)
}

// EntitySource returns the source for a given entity instance.
// Returns FieldSourceSubgraph if no record is found (the default).
func (c *CacheAnalyticsCollector) EntitySource(entityType, keyJSON string) FieldSource {
	for i := len(c.entitySources) - 1; i >= 0; i-- {
		if c.entitySources[i].entityType == entityType && c.entitySources[i].keyJSON == keyJSON {
			return c.entitySources[i].source
		}
	}
	return FieldSourceSubgraph
}

// Snapshot produces a read-only CacheAnalyticsSnapshot from the collected data.
// Duplicate events (same cache key appearing multiple times due to entity batch positions)
// are consolidated: consumers see one event per unique (CacheKey, Kind) for reads,
// one per CacheKey for writes, and one per CacheKey for shadow comparisons.
func (c *CacheAnalyticsCollector) Snapshot() CacheAnalyticsSnapshot {
	snap := CacheAnalyticsSnapshot{
		L1Reads:           deduplicateKeyEvents(c.l1KeyEvents),
		L2Reads:           deduplicateKeyEvents(c.l2KeyEvents),
		FieldHashes:       c.fieldHashes,
		FetchTimings:      c.fetchTimings,
		ErrorEvents:       c.errorEvents,
		ShadowComparisons: deduplicateShadowComparisons(c.shadowComparisons),
		MutationEvents:    c.mutationEvents,
	}

	// Split write events into L1 and L2, then deduplicate each
	for _, we := range c.writeEvents {
		switch we.CacheLevel {
		case CacheLevelL1:
			snap.L1Writes = append(snap.L1Writes, we)
		case CacheLevelL2:
			snap.L2Writes = append(snap.L2Writes, we)
		}
	}
	snap.L1Writes = deduplicateWriteEvents(snap.L1Writes)
	snap.L2Writes = deduplicateWriteEvents(snap.L2Writes)

	// Build EntityTypes slice from entityCounts
	if len(c.entityCounts) > 0 {
		snap.EntityTypes = make([]EntityTypeInfo, len(c.entityCounts))
		for i, ec := range c.entityCounts {
			snap.EntityTypes[i] = EntityTypeInfo{
				TypeName:   ec.typeName,
				Count:      ec.count,
				UniqueKeys: len(ec.uniqueKeys),
			}
		}
	}

	return snap
}

// deduplicateKeyEvents removes duplicate cache key events, keeping the first
// occurrence for each (CacheKey, Kind) pair. This consolidates events where the
// same entity key appears multiple times in a batch (e.g., User 1234 referenced
// by two different reviews).
func deduplicateKeyEvents(events []CacheKeyEvent) []CacheKeyEvent {
	if len(events) == 0 {
		return events
	}
	type dedupKey struct {
		cacheKey string
		kind     CacheKeyEventKind
	}
	seen := make(map[dedupKey]struct{}, len(events))
	out := make([]CacheKeyEvent, 0, len(events))
	for _, ev := range events {
		k := dedupKey{cacheKey: ev.CacheKey, kind: ev.Kind}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, ev)
	}
	return out
}

// deduplicateWriteEvents removes duplicate cache write events, keeping the first
// occurrence for each CacheKey. Within a single cache level, the same key written
// multiple times (from batch positions referencing the same entity) is one operation.
func deduplicateWriteEvents(events []CacheWriteEvent) []CacheWriteEvent {
	if len(events) == 0 {
		return events
	}
	seen := make(map[string]struct{}, len(events))
	out := make([]CacheWriteEvent, 0, len(events))
	for _, ev := range events {
		if _, ok := seen[ev.CacheKey]; ok {
			continue
		}
		seen[ev.CacheKey] = struct{}{}
		out = append(out, ev)
	}
	return out
}

// deduplicateShadowComparisons removes duplicate shadow comparison events,
// keeping the first occurrence for each CacheKey.
func deduplicateShadowComparisons(events []ShadowComparisonEvent) []ShadowComparisonEvent {
	if len(events) == 0 {
		return events
	}
	seen := make(map[string]struct{}, len(events))
	out := make([]ShadowComparisonEvent, 0, len(events))
	for _, ev := range events {
		if _, ok := seen[ev.CacheKey]; ok {
			continue
		}
		seen[ev.CacheKey] = struct{}{}
		out = append(out, ev)
	}
	return out
}

// CacheAnalyticsSnapshot is a read-only snapshot of cache analytics data.
// Requires EnableCacheAnalytics to be set; returns empty when disabled.
type CacheAnalyticsSnapshot struct {
	// Cache read events (nil when analytics disabled)
	L1Reads []CacheKeyEvent
	L2Reads []CacheKeyEvent

	// Cache write events, split by level
	L1Writes []CacheWriteEvent
	L2Writes []CacheWriteEvent

	// Fetch timing events
	FetchTimings []FetchTimingEvent

	// Subgraph error events
	ErrorEvents []SubgraphErrorEvent

	// Field value hashes: flat slice of EntityFieldHash
	FieldHashes []EntityFieldHash

	// Entity tracking: type + count inline
	EntityTypes []EntityTypeInfo

	// Shadow mode comparison events
	ShadowComparisons []ShadowComparisonEvent

	// Mutation entity impact events
	MutationEvents []MutationEvent
}

// L1HitRate returns the L1 cache hit rate as a float64 in [0, 1].
// Returns 0 if there are no L1 events.
func (s *CacheAnalyticsSnapshot) L1HitRate() float64 {
	var hits, total int64
	for _, ev := range s.L1Reads {
		total++
		if ev.Kind == CacheKeyHit {
			hits++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

// L2HitRate returns the L2 cache hit rate as a float64 in [0, 1].
// Returns 0 if there are no L2 events.
func (s *CacheAnalyticsSnapshot) L2HitRate() float64 {
	var hits, total int64
	for _, ev := range s.L2Reads {
		total++
		if ev.Kind == CacheKeyHit {
			hits++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

// CachedBytesServed returns the total bytes served from cache (L1 + L2 hits).
func (s *CacheAnalyticsSnapshot) CachedBytesServed() int64 {
	var total int64
	for _, ev := range s.L1Reads {
		if ev.Kind == CacheKeyHit {
			total += int64(ev.ByteSize)
		}
	}
	for _, ev := range s.L2Reads {
		if ev.Kind == CacheKeyHit {
			total += int64(ev.ByteSize)
		}
	}
	return total
}

// EntityTypeCacheStats holds per-entity-type cache statistics.
type EntityTypeCacheStats struct {
	L1Hits       int64
	L1Misses     int64
	L2Hits       int64
	L2Misses     int64
	PartialHits  int64
	BytesServed  int64
	BytesWritten int64
}

// EventsByEntityType returns cache statistics grouped by entity type.
func (s *CacheAnalyticsSnapshot) EventsByEntityType() map[string]EntityTypeCacheStats {
	result := make(map[string]EntityTypeCacheStats)
	for _, ev := range s.L1Reads {
		stats := result[ev.EntityType]
		switch ev.Kind {
		case CacheKeyHit:
			stats.L1Hits++
			stats.BytesServed += int64(ev.ByteSize)
		case CacheKeyMiss:
			stats.L1Misses++
		case CacheKeyPartialHit:
			stats.L1Misses++
			stats.PartialHits++
		}
		result[ev.EntityType] = stats
	}
	for _, ev := range s.L2Reads {
		stats := result[ev.EntityType]
		switch ev.Kind {
		case CacheKeyHit:
			stats.L2Hits++
			stats.BytesServed += int64(ev.ByteSize)
		case CacheKeyMiss:
			stats.L2Misses++
		case CacheKeyPartialHit:
			stats.L2Misses++
			stats.PartialHits++
		}
		result[ev.EntityType] = stats
	}
	for _, ev := range s.L1Writes {
		stats := result[ev.EntityType]
		stats.BytesWritten += int64(ev.ByteSize)
		result[ev.EntityType] = stats
	}
	for _, ev := range s.L2Writes {
		stats := result[ev.EntityType]
		stats.BytesWritten += int64(ev.ByteSize)
		result[ev.EntityType] = stats
	}
	return result
}

// DataSourceCacheStats holds per-data-source cache statistics.
type DataSourceCacheStats struct {
	L1Hits       int64
	L1Misses     int64
	L2Hits       int64
	L2Misses     int64
	BytesServed  int64
	BytesWritten int64
}

// EventsByDataSource returns cache statistics grouped by data source name.
func (s *CacheAnalyticsSnapshot) EventsByDataSource() map[string]DataSourceCacheStats {
	result := make(map[string]DataSourceCacheStats)
	for _, ev := range s.L1Reads {
		stats := result[ev.DataSource]
		switch ev.Kind {
		case CacheKeyHit:
			stats.L1Hits++
			stats.BytesServed += int64(ev.ByteSize)
		case CacheKeyMiss, CacheKeyPartialHit:
			stats.L1Misses++
		}
		result[ev.DataSource] = stats
	}
	for _, ev := range s.L2Reads {
		stats := result[ev.DataSource]
		switch ev.Kind {
		case CacheKeyHit:
			stats.L2Hits++
			stats.BytesServed += int64(ev.ByteSize)
		case CacheKeyMiss, CacheKeyPartialHit:
			stats.L2Misses++
		}
		result[ev.DataSource] = stats
	}
	for _, ev := range s.L1Writes {
		stats := result[ev.DataSource]
		stats.BytesWritten += int64(ev.ByteSize)
		result[ev.DataSource] = stats
	}
	for _, ev := range s.L2Writes {
		stats := result[ev.DataSource]
		stats.BytesWritten += int64(ev.ByteSize)
		result[ev.DataSource] = stats
	}
	return result
}

// SubgraphCallsAvoided returns the number of subgraph fetch operations
// that were avoided due to cache hits (L1 + L2).
func (s *CacheAnalyticsSnapshot) SubgraphCallsAvoided() int64 {
	var hits int64
	for _, ev := range s.L1Reads {
		if ev.Kind == CacheKeyHit {
			hits++
		}
	}
	for _, ev := range s.L2Reads {
		if ev.Kind == CacheKeyHit {
			hits++
		}
	}
	return hits
}

// PartialHitRate returns the fraction of cache lookups that were partial hits.
// Returns 0 if there are no cache events.
func (s *CacheAnalyticsSnapshot) PartialHitRate() float64 {
	var partialHits, total int64
	for _, ev := range s.L1Reads {
		total++
		if ev.Kind == CacheKeyPartialHit {
			partialHits++
		}
	}
	for _, ev := range s.L2Reads {
		total++
		if ev.Kind == CacheKeyPartialHit {
			partialHits++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(partialHits) / float64(total)
}

// ErrorsByDataSource returns error counts grouped by data source name.
func (s *CacheAnalyticsSnapshot) ErrorsByDataSource() map[string]int {
	if len(s.ErrorEvents) == 0 {
		return nil
	}
	result := make(map[string]int, len(s.ErrorEvents))
	for _, ev := range s.ErrorEvents {
		result[ev.DataSource]++
	}
	return result
}

// ErrorRate returns the fraction of subgraph fetches that resulted in errors.
// Denominator is total subgraph fetches (FieldSourceSubgraph timings) + errors.
// Returns 0 if there are no fetches or errors.
func (s *CacheAnalyticsSnapshot) ErrorRate() float64 {
	errorCount := int64(len(s.ErrorEvents))
	if errorCount == 0 {
		return 0
	}
	var subgraphFetches int64
	for _, ft := range s.FetchTimings {
		if ft.Source == FieldSourceSubgraph {
			subgraphFetches++
		}
	}
	total := subgraphFetches + errorCount
	if total == 0 {
		return 0
	}
	return float64(errorCount) / float64(total)
}

// AvgFetchDurationMs returns the average fetch duration in milliseconds for the given data source.
// Only considers subgraph fetches (not cache lookups). Returns 0 if no fetches recorded.
func (s *CacheAnalyticsSnapshot) AvgFetchDurationMs(dataSource string) int64 {
	var total, count int64
	for _, ft := range s.FetchTimings {
		if ft.DataSource == dataSource && ft.Source == FieldSourceSubgraph {
			total += ft.DurationMs
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / count
}

// TotalTimeSavedMs estimates total time saved by cache hits in milliseconds.
// For each data source, multiplies the average fetch duration by the number of cache hits.
func (s *CacheAnalyticsSnapshot) TotalTimeSavedMs() int64 {
	// Compute average fetch duration per datasource
	type dsStats struct {
		totalDuration int64
		fetchCount    int64
		hitCount      int64
	}
	dss := make(map[string]*dsStats)
	for _, ft := range s.FetchTimings {
		ds, ok := dss[ft.DataSource]
		if !ok {
			ds = &dsStats{}
			dss[ft.DataSource] = ds
		}
		if ft.Source == FieldSourceSubgraph {
			ds.totalDuration += ft.DurationMs
			ds.fetchCount++
		}
	}
	// Count cache hits per datasource from key events
	for _, ev := range s.L1Reads {
		if ev.Kind == CacheKeyHit {
			ds, ok := dss[ev.DataSource]
			if !ok {
				ds = &dsStats{}
				dss[ev.DataSource] = ds
			}
			ds.hitCount++
		}
	}
	for _, ev := range s.L2Reads {
		if ev.Kind == CacheKeyHit {
			ds, ok := dss[ev.DataSource]
			if !ok {
				ds = &dsStats{}
				dss[ev.DataSource] = ds
			}
			ds.hitCount++
		}
	}
	var totalSaved int64
	for _, ds := range dss {
		if ds.fetchCount > 0 && ds.hitCount > 0 {
			avgDuration := ds.totalDuration / ds.fetchCount
			totalSaved += avgDuration * ds.hitCount
		}
	}
	return totalSaved
}

// AvgCacheAgeMs returns the average cache age in milliseconds for L2 hits of the given entity type.
// Only considers L2 hits with known age (CacheAgeMs > 0). Returns 0 if no data available.
// If entityType is empty, returns the average across all entity types.
func (s *CacheAnalyticsSnapshot) AvgCacheAgeMs(entityType string) int64 {
	var total, count int64
	for _, ev := range s.L2Reads {
		if ev.Kind == CacheKeyHit && ev.CacheAgeMs > 0 {
			if entityType == "" || ev.EntityType == entityType {
				total += ev.CacheAgeMs
				count++
			}
		}
	}
	if count == 0 {
		return 0
	}
	return total / count
}

// MaxCacheAgeMs returns the maximum cache age in milliseconds across all L2 hits.
// Returns 0 if no L2 hits with known age exist.
func (s *CacheAnalyticsSnapshot) MaxCacheAgeMs() int64 {
	var maxAge int64
	for _, ev := range s.L2Reads {
		if ev.Kind == CacheKeyHit && ev.CacheAgeMs > maxAge {
			maxAge = ev.CacheAgeMs
		}
	}
	return maxAge
}

// ShadowFreshnessRate returns the fraction of shadow cache hits where the cached data
// matched the fresh data (ProvidesData fields were identical).
// Returns 0.0 if there are no shadow comparisons.
func (s *CacheAnalyticsSnapshot) ShadowFreshnessRate() float64 {
	if len(s.ShadowComparisons) == 0 {
		return 0
	}
	var fresh int64
	for _, sc := range s.ShadowComparisons {
		if sc.IsFresh {
			fresh++
		}
	}
	return float64(fresh) / float64(len(s.ShadowComparisons))
}

// ShadowStaleCount returns the number of shadow comparisons where cached data was stale.
func (s *CacheAnalyticsSnapshot) ShadowStaleCount() int64 {
	var count int64
	for _, sc := range s.ShadowComparisons {
		if !sc.IsFresh {
			count++
		}
	}
	return count
}

// ShadowFreshnessRateByEntityType returns per-entity-type freshness rates.
// Returns nil if there are no shadow comparisons.
func (s *CacheAnalyticsSnapshot) ShadowFreshnessRateByEntityType() map[string]float64 {
	if len(s.ShadowComparisons) == 0 {
		return nil
	}
	type counts struct {
		fresh int64
		total int64
	}
	byType := make(map[string]*counts)
	for _, sc := range s.ShadowComparisons {
		c, ok := byType[sc.EntityType]
		if !ok {
			c = &counts{}
			byType[sc.EntityType] = c
		}
		c.total++
		if sc.IsFresh {
			c.fresh++
		}
	}
	result := make(map[string]float64, len(byType))
	for typeName, c := range byType {
		result[typeName] = float64(c.fresh) / float64(c.total)
	}
	return result
}

// SubgraphFetchMetrics holds metrics for a single subgraph fetch.
// Designed for export to external SLO systems (e.g., schema registry)
// where per-fetch granularity is needed for percentile computation.
type SubgraphFetchMetrics struct {
	SubgraphName   string
	EntityType     string
	DurationMs     int64
	HTTPStatusCode int
	ResponseBytes  int
	IsEntityFetch  bool
}

// SubgraphFetches returns one entry per actual subgraph fetch for this request.
// Cache hits (L1/L2) are excluded. Returns nil if there are no subgraph fetches.
func (s *CacheAnalyticsSnapshot) SubgraphFetches() []SubgraphFetchMetrics {
	var result []SubgraphFetchMetrics
	for _, ft := range s.FetchTimings {
		if ft.Source != FieldSourceSubgraph {
			continue
		}
		result = append(result, SubgraphFetchMetrics{
			SubgraphName:   ft.DataSource,
			EntityType:     ft.EntityType,
			DurationMs:     ft.DurationMs,
			HTTPStatusCode: ft.HTTPStatusCode,
			ResponseBytes:  ft.ResponseBytes,
			IsEntityFetch:  ft.IsEntityFetch,
		})
	}
	return result
}

// computeCacheAgeMs computes cache age in milliseconds from remaining TTL and original TTL.
// Returns 0 if either value is zero or if the computed age would be negative.
func computeCacheAgeMs(remainingTTL, originalTTL time.Duration) int64 {
	if remainingTTL <= 0 || originalTTL <= 0 {
		return 0
	}
	age := originalTTL - remainingTTL
	if age <= 0 {
		return 0
	}
	return age.Milliseconds()
}

// truncateErrorMessage truncates an error message to maxLen bytes for analytics safety.
func truncateErrorMessage(msg string, maxLen int) string {
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen]
}

// buildEntityKeyJSON builds a compact JSON key from an entity's key field values.
// For @key(fields: "id") and value={"id":"1234","name":"Alice"}:
//
//	returns {"id":"1234"}
//
// For @key(fields: "id address { city }") and value={"id":"1234","address":{"city":"NYC","street":"Main"}}:
//
//	returns {"id":"1234","address":{"city":"NYC"}}  (only key fields, not street)
func buildEntityKeyJSON(value *astjson.Value, keyFields []KeyField) []byte {
	if len(keyFields) == 0 {
		return nil
	}
	buf := make([]byte, 0, 64)
	buf = appendKeyFieldsJSON(buf, value, keyFields)
	return buf
}

func appendKeyFieldsJSON(buf []byte, value *astjson.Value, keyFields []KeyField) []byte {
	buf = append(buf, '{')
	first := true
	for _, kf := range keyFields {
		fieldValue := value.Get(kf.Name)
		if fieldValue == nil {
			continue
		}
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = append(buf, '"')
		buf = append(buf, kf.Name...)
		buf = append(buf, '"', ':')
		if len(kf.Children) > 0 {
			// Nested key: recursively extract only key fields
			buf = appendKeyFieldsJSON(buf, fieldValue, kf.Children)
		} else {
			// Scalar key: marshal the value directly
			buf = fieldValue.MarshalTo(buf)
		}
	}
	buf = append(buf, '}')
	return buf
}

// walkCachedResponseForSources walks a cached JSON value to find entity instances
// and accumulates their source records on a per-result slice (goroutine-safe).
func walkCachedResponseForSources(value *astjson.Value, keyFields []KeyField, entityType string, source FieldSource, out *[]entitySourceRecord) {
	if value == nil {
		return
	}
	switch value.Type() {
	case astjson.TypeArray:
		for _, item := range value.GetArray() {
			walkCachedResponseForSources(item, keyFields, entityType, source, out)
		}
	case astjson.TypeObject:
		keyJSON := buildEntityKeyJSON(value, keyFields)
		if len(keyJSON) > 0 {
			*out = append(*out, entitySourceRecord{
				entityType: entityType,
				keyJSON:    string(keyJSON),
				source:     source,
			})
		}
	}
}

// ParseKeyFields parses a selection set string into a structured KeyField tree.
// "id" → [{Name:"id"}]
// "id address { city country }" → [{Name:"id"}, {Name:"address", Children:[{Name:"city"}, {Name:"country"}]}]
func ParseKeyFields(selectionSet string) []KeyField {
	words := strings.Fields(selectionSet)
	fields, _ := parseKeyFieldsFromTokens(words, 0)
	return fields
}

func parseKeyFieldsFromTokens(tokens []string, pos int) ([]KeyField, int) {
	var fields []KeyField
	for pos < len(tokens) {
		token := tokens[pos]
		if token == "}" {
			return fields, pos + 1
		}
		if token == "{" {
			pos++
			continue
		}
		kf := KeyField{Name: token}
		pos++
		// Check if next token is "{" — nested fields
		if pos < len(tokens) && tokens[pos] == "{" {
			pos++ // skip "{"
			kf.Children, pos = parseKeyFieldsFromTokens(tokens, pos)
		}
		fields = append(fields, kf)
	}
	return fields, pos
}
