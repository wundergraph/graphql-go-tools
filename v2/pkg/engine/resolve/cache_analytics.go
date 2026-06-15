package resolve

import (
	"sync"
	"time"
)

type CacheAnalyticsEventKind string

const (
	CacheAnalyticsEventKindL1Read  CacheAnalyticsEventKind = "l1_read"
	CacheAnalyticsEventKindL2Read  CacheAnalyticsEventKind = "l2_read"
	CacheAnalyticsEventKindL1Write CacheAnalyticsEventKind = "l1_write"
	CacheAnalyticsEventKindL2Write CacheAnalyticsEventKind = "l2_write"
)

type CacheAnalyticsSnapshot struct {
	L1Reads            []CacheKeyEvent
	L2Reads            []CacheKeyEvent
	L1Writes           []CacheWriteEvent
	L2Writes           []CacheWriteEvent
	FetchTimings       []FetchTimingEvent
	ShadowComparisons  []ShadowComparisonEvent
	MutationEvents     []MutationEvent
	CacheInvalidations []CacheInvalidationEvent
	EntityTypes        []EntityTypeEvent
	FieldHashes        []FieldHashEvent
	HeaderImpactEvents []HeaderImpactEvent
	CacheOpErrors      []CacheOperationError
}

type CacheKeyEvent struct {
	Key        string
	EntityType string
	Kind       CacheAnalyticsEventKind
	Hit        bool
	Negative   bool
	Shadow     bool
	Bytes      int
}

type CacheWriteEvent struct {
	Key        string
	EntityType string
	Kind       CacheAnalyticsEventKind
	Bytes      int
	TTL        time.Duration
	Reason     CacheWriteReason
	Negative   bool
}

type FetchTimingEvent struct {
	SubgraphName string
	CacheName    string
	Operation    string
	Duration     time.Duration
	Bytes        int
}

type ShadowComparisonEvent struct {
	Key        string
	EntityType string
	Matched    bool
	CachedHash uint64
	FreshHash  uint64
	CachedSize int
	FreshSize  int
}

type MutationEvent struct {
	EntityType string
	Operation  string
	Key        string
	Deleted    bool
	Written    bool
}

type CacheInvalidationEvent struct {
	EntityType   string
	SubgraphName string
	CacheName    string
	Key          string
	Source       string
	Deleted      bool
}

type EntityTypeEvent struct {
	EntityType string
	Count      int
}

type FieldHashEvent struct {
	EntityType string
	FieldPath  string
	Hash       uint64
}

type HeaderImpactEvent struct {
	SubgraphName string
	CacheName    string
	HeaderHash   string
	KeyPrefix    string
}

type CacheOperationError struct {
	Operation string
	CacheName string
	Key       string
	Error     string
}

type cacheAnalyticsCollector struct {
	mu sync.Mutex

	l1Reads            []CacheKeyEvent
	l2Reads            []CacheKeyEvent
	l1Writes           []CacheWriteEvent
	l2Writes           []CacheWriteEvent
	fetchTimings       []FetchTimingEvent
	shadowComparisons  []ShadowComparisonEvent
	mutationEvents     []MutationEvent
	cacheInvalidations []CacheInvalidationEvent
	entityTypes        []EntityTypeEvent
	fieldHashes        []FieldHashEvent
	headerImpactEvents []HeaderImpactEvent
	cacheOpErrors      []CacheOperationError
}

var cacheAnalyticsCollectorPool = sync.Pool{
	New: func() any {
		return &cacheAnalyticsCollector{}
	},
}

var cacheAnalyticsContextMu sync.Mutex

func (c *Context) cacheAnalytics() *cacheAnalyticsCollector {
	if c == nil || !c.ExecutionOptions.Caching.EnableCacheAnalytics {
		return nil
	}
	cacheAnalyticsContextMu.Lock()
	defer cacheAnalyticsContextMu.Unlock()
	if c.cacheAnalyticsCollector == nil {
		collector := cacheAnalyticsCollectorPool.Get().(*cacheAnalyticsCollector)
		collector.reset()
		c.cacheAnalyticsCollector = collector
	}
	return c.cacheAnalyticsCollector
}

func (c *Context) GetCacheStats() CacheAnalyticsSnapshot {
	if c == nil {
		return CacheAnalyticsSnapshot{}
	}
	cacheAnalyticsContextMu.Lock()
	collector := c.cacheAnalyticsCollector
	if collector == nil {
		cacheAnalyticsContextMu.Unlock()
		return CacheAnalyticsSnapshot{}
	}
	c.cacheAnalyticsCollector = nil
	cacheAnalyticsContextMu.Unlock()

	snapshot := collector.snapshot()
	collector.reset()
	cacheAnalyticsCollectorPool.Put(collector)
	return snapshot
}

func (c *cacheAnalyticsCollector) recordL1Read(event CacheKeyEvent) {
	if c == nil {
		return
	}
	event.Kind = CacheAnalyticsEventKindL1Read
	c.mu.Lock()
	c.l1Reads = append(c.l1Reads, event)
	c.mu.Unlock()
}

func (c *cacheAnalyticsCollector) recordL2Read(event CacheKeyEvent) {
	if c == nil {
		return
	}
	event.Kind = CacheAnalyticsEventKindL2Read
	c.mu.Lock()
	c.l2Reads = append(c.l2Reads, event)
	c.mu.Unlock()
}

func (c *cacheAnalyticsCollector) recordL1Write(event CacheWriteEvent) {
	if c == nil {
		return
	}
	event.Kind = CacheAnalyticsEventKindL1Write
	c.mu.Lock()
	c.l1Writes = append(c.l1Writes, event)
	c.mu.Unlock()
}

func (c *cacheAnalyticsCollector) recordL2Write(event CacheWriteEvent) {
	if c == nil {
		return
	}
	event.Kind = CacheAnalyticsEventKindL2Write
	c.mu.Lock()
	c.l2Writes = append(c.l2Writes, event)
	c.mu.Unlock()
}

func (c *cacheAnalyticsCollector) recordFetchTiming(event FetchTimingEvent) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.fetchTimings = append(c.fetchTimings, event)
	c.mu.Unlock()
}

func (c *cacheAnalyticsCollector) recordShadowComparison(event ShadowComparisonEvent) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.shadowComparisons = append(c.shadowComparisons, event)
	c.mu.Unlock()
}

func (c *cacheAnalyticsCollector) recordMutationEvent(event MutationEvent) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.mutationEvents = append(c.mutationEvents, event)
	c.mu.Unlock()
}

func (c *cacheAnalyticsCollector) recordCacheInvalidation(event CacheInvalidationEvent) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.cacheInvalidations = append(c.cacheInvalidations, event)
	c.mu.Unlock()
}

func (c *cacheAnalyticsCollector) recordEntityType(event EntityTypeEvent) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.entityTypes = append(c.entityTypes, event)
	c.mu.Unlock()
}

func (c *cacheAnalyticsCollector) recordFieldHash(event FieldHashEvent) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.fieldHashes = append(c.fieldHashes, event)
	c.mu.Unlock()
}

func (c *cacheAnalyticsCollector) recordHeaderImpact(event HeaderImpactEvent) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.headerImpactEvents = append(c.headerImpactEvents, event)
	c.mu.Unlock()
}

func (c *cacheAnalyticsCollector) recordCacheOperationError(event CacheOperationError) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.cacheOpErrors = append(c.cacheOpErrors, event)
	c.mu.Unlock()
}

func (c *cacheAnalyticsCollector) snapshot() CacheAnalyticsSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	return CacheAnalyticsSnapshot{
		L1Reads:            dedupCacheKeyEvents(c.l1Reads),
		L2Reads:            dedupCacheKeyEvents(c.l2Reads),
		L1Writes:           dedupCacheWriteEvents(c.l1Writes),
		L2Writes:           dedupCacheWriteEvents(c.l2Writes),
		FetchTimings:       cloneSlice(c.fetchTimings),
		ShadowComparisons:  cloneSlice(c.shadowComparisons),
		MutationEvents:     cloneSlice(c.mutationEvents),
		CacheInvalidations: cloneSlice(c.cacheInvalidations),
		EntityTypes:        cloneSlice(c.entityTypes),
		FieldHashes:        cloneSlice(c.fieldHashes),
		HeaderImpactEvents: cloneSlice(c.headerImpactEvents),
		CacheOpErrors:      cloneSlice(c.cacheOpErrors),
	}
}

func (c *cacheAnalyticsCollector) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.l1Reads = c.l1Reads[:0]
	c.l2Reads = c.l2Reads[:0]
	c.l1Writes = c.l1Writes[:0]
	c.l2Writes = c.l2Writes[:0]
	c.fetchTimings = c.fetchTimings[:0]
	c.shadowComparisons = c.shadowComparisons[:0]
	c.mutationEvents = c.mutationEvents[:0]
	c.cacheInvalidations = c.cacheInvalidations[:0]
	c.entityTypes = c.entityTypes[:0]
	c.fieldHashes = c.fieldHashes[:0]
	c.headerImpactEvents = c.headerImpactEvents[:0]
	c.cacheOpErrors = c.cacheOpErrors[:0]
}

func (s CacheAnalyticsSnapshot) L1HitRate() float64 {
	return hitRate(s.L1Reads)
}

func (s CacheAnalyticsSnapshot) L2HitRate() float64 {
	return hitRate(s.L2Reads)
}

func (s CacheAnalyticsSnapshot) CachedBytesServed() int {
	bytes := 0
	for _, event := range s.L1Reads {
		if event.Hit {
			bytes += event.Bytes
		}
	}
	for _, event := range s.L2Reads {
		if event.Hit {
			bytes += event.Bytes
		}
	}
	return bytes
}

func (s CacheAnalyticsSnapshot) ShadowFreshnessRate() float64 {
	if len(s.ShadowComparisons) == 0 {
		return 0
	}
	matches := 0
	for _, event := range s.ShadowComparisons {
		if event.Matched {
			matches++
		}
	}
	return float64(matches) / float64(len(s.ShadowComparisons))
}

func (s CacheAnalyticsSnapshot) EventsByEntityType() map[string]int {
	counts := map[string]int{}
	for _, event := range s.EntityTypes {
		if event.EntityType == "" {
			continue
		}
		counts[event.EntityType] += event.Count
	}
	if len(counts) > 0 {
		return counts
	}
	addCacheKeyEventsByEntityType(counts, s.L1Reads)
	addCacheKeyEventsByEntityType(counts, s.L2Reads)
	addCacheWriteEventsByEntityType(counts, s.L1Writes)
	addCacheWriteEventsByEntityType(counts, s.L2Writes)
	for _, event := range s.FieldHashes {
		if event.EntityType != "" {
			counts[event.EntityType]++
		}
	}
	for _, event := range s.ShadowComparisons {
		if event.EntityType != "" {
			counts[event.EntityType]++
		}
	}
	for _, event := range s.MutationEvents {
		if event.EntityType != "" {
			counts[event.EntityType]++
		}
	}
	return counts
}

func hitRate(events []CacheKeyEvent) float64 {
	if len(events) == 0 {
		return 0
	}
	hits := 0
	for _, event := range events {
		if event.Hit {
			hits++
		}
	}
	return float64(hits) / float64(len(events))
}

func addCacheKeyEventsByEntityType(counts map[string]int, events []CacheKeyEvent) {
	for _, event := range events {
		if event.EntityType != "" {
			counts[event.EntityType]++
		}
	}
}

func addCacheWriteEventsByEntityType(counts map[string]int, events []CacheWriteEvent) {
	for _, event := range events {
		if event.EntityType != "" {
			counts[event.EntityType]++
		}
	}
}

type cacheAnalyticsDedupKey struct {
	key  string
	kind CacheAnalyticsEventKind
}

func dedupCacheKeyEvents(events []CacheKeyEvent) []CacheKeyEvent {
	if len(events) == 0 {
		return nil
	}
	seen := make(map[cacheAnalyticsDedupKey]struct{}, len(events))
	deduped := make([]CacheKeyEvent, 0, len(events))
	for _, event := range events {
		key := cacheAnalyticsDedupKey{
			key:  event.Key,
			kind: event.Kind,
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, event)
	}
	return deduped
}

func dedupCacheWriteEvents(events []CacheWriteEvent) []CacheWriteEvent {
	if len(events) == 0 {
		return nil
	}
	seen := make(map[cacheAnalyticsDedupKey]struct{}, len(events))
	deduped := make([]CacheWriteEvent, 0, len(events))
	for _, event := range events {
		key := cacheAnalyticsDedupKey{
			key:  event.Key,
			kind: event.Kind,
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, event)
	}
	return deduped
}

func cloneSlice[T any](in []T) []T {
	if len(in) == 0 {
		return nil
	}
	return append([]T(nil), in...)
}
