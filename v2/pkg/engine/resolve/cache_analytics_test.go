package resolve

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"
)

// =============================================================================
// Unit Tests for CacheAnalyticsCollector
// =============================================================================

// TestCacheAnalyticsCollector_RecordEvents verifies that L1/L2 key events are
// recorded with correct fields. Without this, cache analytics could silently
// drop or misattribute events.
func TestCacheAnalyticsCollector_RecordEvents(t *testing.T) {
	t.Run("L1 and L2 key events are recorded with exact counts", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.RecordL1KeyEvent(CacheKeyHit, "User", "key1", "accounts", 128)
		c.RecordL1KeyEvent(CacheKeyMiss, "User", "key2", "accounts", 0)
		c.RecordL1KeyEvent(CacheKeyHit, "Product", "key3", "products", 256)

		c.RecordL2KeyEvent(CacheKeyHit, "User", "key4", "accounts", 512)
		c.RecordL2KeyEvent(CacheKeyMiss, "Product", "key5", "products", 0)

		snap := c.Snapshot()

		// L1: 3 events recorded (2 hits + 1 miss), L2: 2 events (1 hit + 1 miss)
		assert.Equal(t, 3, len(snap.L1Reads))
		assert.Equal(t, 2, len(snap.L2Reads))

		// Verify specific events
		assert.Equal(t, CacheKeyHit, snap.L1Reads[0].Kind)
		assert.Equal(t, "User", snap.L1Reads[0].EntityType)
		assert.Equal(t, "key1", snap.L1Reads[0].CacheKey)
		assert.Equal(t, "accounts", snap.L1Reads[0].DataSource)
		assert.Equal(t, 128, snap.L1Reads[0].ByteSize)

		assert.Equal(t, CacheKeyMiss, snap.L1Reads[1].Kind)
		assert.Equal(t, 0, snap.L1Reads[1].ByteSize)
	})

	t.Run("partial hits count as misses in summary", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.RecordL2KeyEvent(CacheKeyPartialHit, "User", "key1", "accounts", 0)
		c.RecordL2KeyEvent(CacheKeyHit, "User", "key2", "accounts", 100)

		snap := c.Snapshot()

		assert.Equal(t, 2, len(snap.L2Reads))
		assert.Equal(t, CacheKeyPartialHit, snap.L2Reads[0].Kind)
		assert.Equal(t, CacheKeyHit, snap.L2Reads[1].Kind)
	})
}

// TestCacheAnalyticsCollector_MergeL2Events verifies that L2 events accumulated
// in goroutines merge correctly into the collector on the main thread.
func TestCacheAnalyticsCollector_MergeL2Events(t *testing.T) {
	c := NewCacheAnalyticsCollector()

	// Simulate events from goroutine 1
	events1 := []CacheKeyEvent{
		{CacheKey: "key1", EntityType: "User", Kind: CacheKeyHit, DataSource: "accounts", ByteSize: 100},
		{CacheKey: "key2", EntityType: "User", Kind: CacheKeyMiss, DataSource: "accounts", ByteSize: 0},
	}
	// Simulate events from goroutine 2
	events2 := []CacheKeyEvent{
		{CacheKey: "key3", EntityType: "Product", Kind: CacheKeyHit, DataSource: "products", ByteSize: 200},
	}

	c.MergeL2Events(events1)
	c.MergeL2Events(events2)

	snap := c.Snapshot()
	// 2 events from goroutine 1 + 1 from goroutine 2
	assert.Equal(t, 3, len(snap.L2Reads))

	// Count hits and misses from events
	var l2Hits, l2Misses int
	for _, ev := range snap.L2Reads {
		switch ev.Kind {
		case CacheKeyHit:
			l2Hits++
		case CacheKeyMiss:
			l2Misses++
		}
	}
	assert.Equal(t, 2, l2Hits)
	assert.Equal(t, 1, l2Misses)
}

// TestCacheAnalyticsCollector_WriteEvents verifies that L1/L2 write events
// are partitioned correctly and carry TTL and size metadata.
func TestCacheAnalyticsCollector_WriteEvents(t *testing.T) {
	c := NewCacheAnalyticsCollector()

	c.RecordWrite(CacheWriteEvent{CacheKey: "key1", EntityType: "User", ByteSize: 128, DataSource: "accounts", CacheLevel: CacheLevelL1, Source: CacheSourceQuery})
	c.RecordWrite(CacheWriteEvent{CacheKey: "key2", EntityType: "User", ByteSize: 256, DataSource: "accounts", CacheLevel: CacheLevelL2, TTL: 30 * time.Second, Source: CacheSourceQuery})
	c.RecordWrite(CacheWriteEvent{CacheKey: "key3", EntityType: "Product", ByteSize: 512, DataSource: "products", CacheLevel: CacheLevelL2, TTL: 60 * time.Second, Source: CacheSourceQuery})

	snap := c.Snapshot()
	// 1 L1 write, 2 L2 writes
	assert.Equal(t, 1, len(snap.L1Writes))
	assert.Equal(t, 2, len(snap.L2Writes))

	assert.Equal(t, time.Duration(0), snap.L1Writes[0].TTL)
	assert.Equal(t, 128, snap.L1Writes[0].ByteSize)
	assert.Equal(t, "User", snap.L1Writes[0].EntityType)

	assert.Equal(t, 30*time.Second, snap.L2Writes[0].TTL)
	assert.Equal(t, 256, snap.L2Writes[0].ByteSize)

	assert.Equal(t, "Product", snap.L2Writes[1].EntityType)
	assert.Equal(t, 512, snap.L2Writes[1].ByteSize)
}

// TestCacheAnalyticsCollector_FieldHashing verifies xxhash-based field value
// hashing for staleness detection. Same input must produce identical hashes,
// different input must produce different hashes.
func TestCacheAnalyticsCollector_FieldHashing(t *testing.T) {
	t.Run("same input produces same hash", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.HashFieldValue("User", "name", []byte(`"Alice"`), `{"id":"1"}`, 0, FieldSourceSubgraph)
		c.HashFieldValue("User", "name", []byte(`"Alice"`), `{"id":"1"}`, 0, FieldSourceSubgraph)

		snap := c.Snapshot()
		assert.Equal(t, 2, len(snap.FieldHashes))
		assert.Equal(t, snap.FieldHashes[0].FieldHash, snap.FieldHashes[1].FieldHash, "same input = same hash")
		assert.Equal(t, "User", snap.FieldHashes[0].EntityType)
		assert.Equal(t, "name", snap.FieldHashes[0].FieldName)
		assert.Equal(t, `{"id":"1"}`, snap.FieldHashes[0].KeyRaw)
		assert.Equal(t, FieldSourceSubgraph, snap.FieldHashes[0].Source)
	})

	t.Run("different input produces different hash", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.HashFieldValue("User", "name", []byte(`"Alice"`), `{"id":"1"}`, 0, FieldSourceSubgraph)
		c.HashFieldValue("User", "name", []byte(`"Bob"`), `{"id":"2"}`, 0, FieldSourceSubgraph)

		snap := c.Snapshot()
		assert.Equal(t, 2, len(snap.FieldHashes))
		assert.NotEqual(t, snap.FieldHashes[0].FieldHash, snap.FieldHashes[1].FieldHash, "different input = different hash")
	})

	t.Run("hashed keys mode", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.HashFieldValue("User", "name", []byte(`"Alice"`), "", 12345, FieldSourceL1)

		snap := c.Snapshot()
		assert.Equal(t, 1, len(snap.FieldHashes))
		assert.Equal(t, "", snap.FieldHashes[0].KeyRaw)
		assert.Equal(t, uint64(12345), snap.FieldHashes[0].KeyHash)
		assert.Equal(t, FieldSourceL1, snap.FieldHashes[0].Source)
	})

	t.Run("field source tracking", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.HashFieldValue("User", "name", []byte(`"Alice"`), `{"id":"1"}`, 0, FieldSourceSubgraph)
		c.HashFieldValue("User", "name", []byte(`"Alice"`), `{"id":"1"}`, 0, FieldSourceL1)
		c.HashFieldValue("User", "name", []byte(`"Alice"`), `{"id":"1"}`, 0, FieldSourceL2)

		snap := c.Snapshot()
		assert.Equal(t, 3, len(snap.FieldHashes))
		assert.Equal(t, FieldSourceSubgraph, snap.FieldHashes[0].Source)
		assert.Equal(t, FieldSourceL1, snap.FieldHashes[1].Source)
		assert.Equal(t, FieldSourceL2, snap.FieldHashes[2].Source)
	})
}

// TestCacheAnalyticsCollector_EntityCounts verifies per-type entity instance
// counting and unique key tracking. Duplicate keys should increment count
// but not unique keys.
func TestCacheAnalyticsCollector_EntityCounts(t *testing.T) {
	c := NewCacheAnalyticsCollector()

	c.IncrementEntityCount("User", `{"id":"1"}`)
	c.IncrementEntityCount("User", `{"id":"2"}`)
	c.IncrementEntityCount("User", `{"id":"1"}`) // duplicate key
	c.IncrementEntityCount("Product", `{"upc":"top-1"}`)

	snap := c.Snapshot()
	assert.Equal(t, 2, len(snap.EntityTypes))

	// Find counts by type
	var userCount, productCount int
	for _, et := range snap.EntityTypes {
		switch et.TypeName {
		case "User":
			userCount = et.Count
		case "Product":
			productCount = et.Count
		}
	}
	// User: 3 instances (id:1 twice + id:2), Product: 1 instance
	assert.Equal(t, 3, userCount)
	assert.Equal(t, 1, productCount)

	// Verify unique keys
	var userUniqueKeys, productUniqueKeys int
	for _, et := range snap.EntityTypes {
		switch et.TypeName {
		case "User":
			userUniqueKeys = et.UniqueKeys
		case "Product":
			productUniqueKeys = et.UniqueKeys
		}
	}
	// User: 2 unique keys (id:1, id:2), Product: 1 unique key
	assert.Equal(t, 2, userUniqueKeys)
	assert.Equal(t, 1, productUniqueKeys)
}

// TestCacheAnalyticsCollector_EntitySourceTracking verifies that the source
// (subgraph, L1, L2) of each entity is recorded and retrievable by type+key.
func TestCacheAnalyticsCollector_EntitySourceTracking(t *testing.T) {
	c := NewCacheAnalyticsCollector()

	c.RecordEntitySource("User", `{"id":"1"}`, FieldSourceSubgraph)
	c.RecordEntitySource("User", `{"id":"2"}`, FieldSourceL1)
	c.RecordEntitySource("Product", `{"upc":"top-1"}`, FieldSourceL2)

	assert.Equal(t, FieldSourceSubgraph, c.EntitySource("User", `{"id":"1"}`))
	assert.Equal(t, FieldSourceL1, c.EntitySource("User", `{"id":"2"}`))
	assert.Equal(t, FieldSourceL2, c.EntitySource("Product", `{"upc":"top-1"}`))
	// Unknown entity defaults to Subgraph source
	assert.Equal(t, FieldSourceSubgraph, c.EntitySource("Unknown", `{"id":"99"}`))
}

// TestCacheAnalyticsCollector_MergeEntitySources verifies that entity source
// records from goroutines merge into the main thread collector.
func TestCacheAnalyticsCollector_MergeEntitySources(t *testing.T) {
	c := NewCacheAnalyticsCollector()

	sources := []entitySourceRecord{
		{entityType: "User", keyJSON: `{"id":"1"}`, source: FieldSourceL2},
		{entityType: "User", keyJSON: `{"id":"2"}`, source: FieldSourceL2},
	}

	c.MergeEntitySources(sources)

	assert.Equal(t, FieldSourceL2, c.EntitySource("User", `{"id":"1"}`))
	assert.Equal(t, FieldSourceL2, c.EntitySource("User", `{"id":"2"}`))
}

// TestCacheAnalyticsCollector_SnapshotDerivedMetrics verifies computed metrics
// (hit rates, bytes served, entity/datasource breakdowns) derived from raw events.
func TestCacheAnalyticsCollector_SnapshotDerivedMetrics(t *testing.T) {
	t.Run("hit rates", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		// 3 L1 hits, 1 L1 miss = 75% hit rate
		c.RecordL1KeyEvent(CacheKeyHit, "User", "k1", "ds", 100)
		c.RecordL1KeyEvent(CacheKeyHit, "User", "k2", "ds", 100)
		c.RecordL1KeyEvent(CacheKeyHit, "User", "k3", "ds", 100)
		c.RecordL1KeyEvent(CacheKeyMiss, "User", "k4", "ds", 0)

		// 1 L2 hit, 1 L2 miss = 50% hit rate
		c.RecordL2KeyEvent(CacheKeyHit, "User", "k5", "ds", 200)
		c.RecordL2KeyEvent(CacheKeyMiss, "User", "k6", "ds", 0)

		snap := c.Snapshot()

		// 3 L1 hits / 4 total = 0.75, 1 L2 hit / 2 total = 0.5
		assert.Equal(t, 0.75, snap.L1HitRate())
		assert.Equal(t, 0.5, snap.L2HitRate())
	})

	t.Run("zero events returns zero hit rate", func(t *testing.T) {
		snap := CacheAnalyticsSnapshot{}
		assert.Equal(t, float64(0), snap.L1HitRate())
		assert.Equal(t, float64(0), snap.L2HitRate())
	})

	t.Run("cached bytes served", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.RecordL1KeyEvent(CacheKeyHit, "User", "k1", "ds", 100)
		c.RecordL1KeyEvent(CacheKeyHit, "User", "k2", "ds", 200)
		c.RecordL1KeyEvent(CacheKeyMiss, "User", "k3", "ds", 0)
		c.RecordL2KeyEvent(CacheKeyHit, "User", "k4", "ds", 300)
		c.RecordL2KeyEvent(CacheKeyMiss, "User", "k5", "ds", 0)

		snap := c.Snapshot()
		// 100 + 200 (L1 hits) + 300 (L2 hit) = 600
		assert.Equal(t, int64(600), snap.CachedBytesServed())
	})

	t.Run("events by entity type", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.RecordL1KeyEvent(CacheKeyHit, "User", "k1", "ds", 100)
		c.RecordL1KeyEvent(CacheKeyMiss, "User", "k2", "ds", 0)
		c.RecordL1KeyEvent(CacheKeyHit, "Product", "k3", "ds", 200)
		c.RecordL2KeyEvent(CacheKeyHit, "User", "k4", "ds", 300)
		c.RecordWrite(CacheWriteEvent{CacheKey: "k5", EntityType: "User", ByteSize: 150, DataSource: "ds", CacheLevel: CacheLevelL2, TTL: 30 * time.Second, Source: CacheSourceQuery})

		snap := c.Snapshot()
		byEntity := snap.EventsByEntityType()

		assert.Equal(t, int64(1), byEntity["User"].L1Hits)
		assert.Equal(t, int64(1), byEntity["User"].L1Misses)
		assert.Equal(t, int64(1), byEntity["User"].L2Hits)
		assert.Equal(t, int64(400), byEntity["User"].BytesServed) // 100 L1 + 300 L2
		assert.Equal(t, int64(150), byEntity["User"].BytesWritten)

		assert.Equal(t, int64(1), byEntity["Product"].L1Hits)
		assert.Equal(t, int64(200), byEntity["Product"].BytesServed)
	})

	t.Run("events by data source", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.RecordL1KeyEvent(CacheKeyHit, "User", "k1", "accounts", 100)
		c.RecordL2KeyEvent(CacheKeyMiss, "User", "k2", "accounts", 0)
		c.RecordL1KeyEvent(CacheKeyHit, "Product", "k3", "products", 200)
		c.RecordWrite(CacheWriteEvent{CacheKey: "k4", EntityType: "Product", ByteSize: 250, DataSource: "products", CacheLevel: CacheLevelL2, TTL: 30 * time.Second, Source: CacheSourceQuery})

		snap := c.Snapshot()
		byDS := snap.EventsByDataSource()

		assert.Equal(t, int64(1), byDS["accounts"].L1Hits)
		assert.Equal(t, int64(1), byDS["accounts"].L2Misses)
		assert.Equal(t, int64(100), byDS["accounts"].BytesServed)

		assert.Equal(t, int64(1), byDS["products"].L1Hits)
		assert.Equal(t, int64(200), byDS["products"].BytesServed)
		assert.Equal(t, int64(250), byDS["products"].BytesWritten)
	})

	t.Run("partial hit rate", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.RecordL1KeyEvent(CacheKeyHit, "User", "k1", "ds", 100)
		c.RecordL2KeyEvent(CacheKeyPartialHit, "User", "k2", "ds", 0)
		c.RecordL2KeyEvent(CacheKeyMiss, "User", "k3", "ds", 0)
		c.RecordL2KeyEvent(CacheKeyHit, "User", "k4", "ds", 200)

		snap := c.Snapshot()
		// 1 partial hit out of 4 total events = 0.25
		assert.Equal(t, 0.25, snap.PartialHitRate())
	})
}

// TestCacheAnalyticsCollector_DisabledReturnsEmpty verifies that GetCacheStats()
// returns an empty snapshot when EnableCacheAnalytics is not set. This ensures
// zero overhead when analytics is off.
func TestCacheAnalyticsCollector_DisabledReturnsEmpty(t *testing.T) {
	// When analytics is disabled, GetCacheStats() returns an empty snapshot
	ctx := NewContext(context.Background())
	// Do NOT enable analytics
	ctx.ExecutionOptions.Caching.EnableL1Cache = true

	// All nil because EnableCacheAnalytics was not set, so no collector exists
	snap := ctx.GetCacheStats()
	// All nil because EnableCacheAnalytics was not set
	assert.Nil(t, snap.L1Reads)
	assert.Nil(t, snap.L2Reads)
	assert.Nil(t, snap.L1Writes)
	assert.Nil(t, snap.L2Writes)
	assert.Nil(t, snap.FieldHashes)
	assert.Nil(t, snap.EntityTypes)
}

// TestBuildEntityKeyJSON verifies that entity key JSON is built from @key fields
// only, ignoring other fields. Composite keys must include nested sub-selections.
func TestBuildEntityKeyJSON(t *testing.T) {
	t.Run("simple key", func(t *testing.T) {
		var parser astjson.Parser

		val, err := parser.Parse(`{"id":"1234","name":"Alice","age":30}`)
		require.NoError(t, err)

		keyFields := []KeyField{{Name: "id"}}
		result := buildEntityKeyJSON(val, keyFields)
		assert.Equal(t, `{"id":"1234"}`, string(result))
	})

	t.Run("composite key", func(t *testing.T) {
		var parser astjson.Parser

		val, err := parser.Parse(`{"id":"1234","address":{"city":"NYC","street":"Main"},"name":"Alice"}`)
		require.NoError(t, err)

		keyFields := []KeyField{
			{Name: "id"},
			{Name: "address", Children: []KeyField{{Name: "city"}}},
		}
		result := buildEntityKeyJSON(val, keyFields)
		assert.Equal(t, `{"id":"1234","address":{"city":"NYC"}}`, string(result))
	})

	t.Run("nil key fields returns nil", func(t *testing.T) {
		result := buildEntityKeyJSON(nil, nil)
		assert.Nil(t, result)
	})
}

// TestParseKeyFields verifies parsing of @key field selection strings into
// structured KeyField slices, including nested composite keys.
func TestParseKeyFields(t *testing.T) {
	t.Run("simple key", func(t *testing.T) {
		fields := ParseKeyFields("id")
		assert.Equal(t, []KeyField{{Name: "id"}}, fields)
	})

	t.Run("composite key", func(t *testing.T) {
		fields := ParseKeyFields("id address { city }")
		assert.Equal(t, []KeyField{
			{Name: "id"},
			{Name: "address", Children: []KeyField{{Name: "city"}}},
		}, fields)
	})

	t.Run("nested composite key", func(t *testing.T) {
		fields := ParseKeyFields("id address { city country }")
		assert.Equal(t, []KeyField{
			{Name: "id"},
			{Name: "address", Children: []KeyField{{Name: "city"}, {Name: "country"}}},
		}, fields)
	})
}

// =============================================================================
// Integration Tests
// =============================================================================

// TestCacheAnalytics_L1Integration verifies end-to-end L1 cache analytics:
// first entity fetch misses (cold cache), second fetch for same entity hits L1.
func TestCacheAnalytics_L1Integration(t *testing.T) {
	t.Run("L1 analytics records hit and miss events", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).Times(1)

		entityDS1 := NewMockDataSource(ctrl)
		entityDS1.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One"}]}}`), nil
			}).Times(1)

		// Second entity fetch - should NOT be called (L1 hit)
		entityDS2 := NewMockDataSource(ctrl)
		entityDS2.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)

		productCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		providesData := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}, Nullable: false}},
			},
		}

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			},
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: rootDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://root.service","body":{"query":"{product {__typename id}}"}}`), SegmentType: StaticSegmentType},
						},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),
				// First entity fetch - populates L1 cache
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: entityDS1,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities", "0"},
						},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: productCacheKeyTemplate,
							UseL1Cache:       true,
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://products.service","body":{"query":"...","variables":{"representations":[`), SegmentType: StaticSegmentType},
							{
								SegmentType:  VariableSegmentType,
								VariableKind: ResolvableObjectVariableKind,
								Renderer: NewGraphQLVariableResolveRenderer(&Object{
									Fields: []*Field{
										{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
										{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
									},
								}),
							},
							{Data: []byte(`]}}}`), SegmentType: StaticSegmentType},
						},
					},
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						RootFields:     []GraphCoordinate{{TypeName: "Product", FieldName: "name"}},
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.product", ObjectPath("product")),
				// Second entity fetch for SAME entity - should hit L1 cache
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: entityDS2,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities", "0"},
						},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: productCacheKeyTemplate,
							UseL1Cache:       true,
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://products.service","body":{"query":"...","variables":{"representations":[`), SegmentType: StaticSegmentType},
							{
								SegmentType:  VariableSegmentType,
								VariableKind: ResolvableObjectVariableKind,
								Renderer: NewGraphQLVariableResolveRenderer(&Object{
									Fields: []*Field{
										{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
										{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
									},
								}),
							},
							{Data: []byte(`]}}}`), SegmentType: StaticSegmentType},
						},
					},
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						RootFields:     []GraphCoordinate{{TypeName: "Product", FieldName: "name"}},
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.product", ObjectPath("product")),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("product"),
						Value: &Object{
							Path: []string{"product"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
							},
						},
					},
				},
			},
		}

		loader := &Loader{}
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1","name":"Product One"}}}`, out)

		// Verify analytics
		snap := ctx.GetCacheStats()

		// 2 events: 1st entity fetch misses (cache empty), 2nd hits (populated by 1st)
		assert.Equal(t, 2, len(snap.L1Reads))

		// 1st fetch: L1 miss (empty cache), 2nd fetch: L1 hit (same entity cached by 1st)
		var l1Hits, l1Misses int
		for _, ev := range snap.L1Reads {
			assert.Equal(t, "Product", ev.EntityType)
			assert.Equal(t, "products", ev.DataSource)
			if ev.Kind == CacheKeyHit {
				l1Hits++
				assert.Equal(t, 59, ev.ByteSize)
			} else {
				l1Misses++
			}
		}
		assert.Equal(t, 1, l1Hits)
		assert.Equal(t, 1, l1Misses)

		// L1 writes occur after 1st entity fetch resolved from subgraph
		assert.Equal(t, 1, len(snap.L1Writes))
		for _, we := range snap.L1Writes {
			assert.Equal(t, "Product", we.EntityType)
			assert.Equal(t, 59, we.ByteSize)
		}
	})
}

// TestCacheAnalytics_L2Integration verifies end-to-end L2 cache analytics:
// first request misses L2, fetches from subgraph, and writes to L2.
func TestCacheAnalytics_L2Integration(t *testing.T) {
	t.Run("L2 analytics records hit and write events", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cache := NewFakeLoaderCache()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).Times(1)

		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One"}]}}`), nil
			}).Times(1)

		productCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		providesData := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}, Nullable: false}},
			},
		}

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			},
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: rootDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://root.service","body":{"query":"{product {__typename id}}"}}`), SegmentType: StaticSegmentType},
						},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: entityDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities", "0"},
						},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: productCacheKeyTemplate,
							UseL1Cache:       true,
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://products.service","body":{"query":"...","variables":{"representations":[`), SegmentType: StaticSegmentType},
							{
								SegmentType:  VariableSegmentType,
								VariableKind: ResolvableObjectVariableKind,
								Renderer: NewGraphQLVariableResolveRenderer(&Object{
									Fields: []*Field{
										{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
										{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
									},
								}),
							},
							{Data: []byte(`]}}}`), SegmentType: StaticSegmentType},
						},
					},
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						RootFields:     []GraphCoordinate{{TypeName: "Product", FieldName: "name"}},
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.product", ObjectPath("product")),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("product"),
						Value: &Object{
							Path: []string{"product"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
							},
						},
					},
				},
			},
		}

		loader := &Loader{
			caches: map[string]LoaderCache{"default": cache},
		}

		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableL2Cache = true
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		out := fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
		assert.Equal(t, `{"data":{"product":{"__typename":"Product","id":"prod-1","name":"Product One"}}}`, out)

		snap := ctx.GetCacheStats()

		// L1 miss: single entity fetch, L1 cache empty (no prior population)
		assert.Equal(t, 1, len(snap.L1Reads))
		assert.Equal(t, CacheKeyMiss, snap.L1Reads[0].Kind)

		// L2 miss: first request, L2 cache starts empty
		assert.Equal(t, 1, len(snap.L2Reads))
		assert.Equal(t, CacheKeyMiss, snap.L2Reads[0].Kind)

		// Entity written to L2 after subgraph fetch; TTL from FetchCacheConfiguration
		assert.Equal(t, 1, len(snap.L2Writes))
		assert.Equal(t, 30*time.Second, snap.L2Writes[0].TTL)
		assert.Equal(t, 59, snap.L2Writes[0].ByteSize)
	})
}

// TestCacheAnalytics_UseL1CacheDisabled verifies that no L1 events are recorded
// when UseL1Cache is false on the fetch configuration. This prevents spurious
// analytics for fetches that deliberately bypass L1.
func TestCacheAnalytics_UseL1CacheDisabled(t *testing.T) {
	t.Run("no L1 events when UseL1Cache is false", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
			}).Times(1)

		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"Product","id":"prod-1","name":"Product One"}]}}`), nil
			}).Times(1)

		productCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		providesData := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
				{Name: []byte("name"), Value: &Scalar{Path: []string{"name"}, Nullable: false}},
			},
		}

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			},
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: rootDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
						},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: entityDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities", "0"},
						},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: productCacheKeyTemplate,
							UseL1Cache:       false, // L1 disabled for this fetch
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://products.service","body":{"query":"...","variables":{"representations":[`), SegmentType: StaticSegmentType},
							{
								SegmentType:  VariableSegmentType,
								VariableKind: ResolvableObjectVariableKind,
								Renderer: NewGraphQLVariableResolveRenderer(&Object{
									Fields: []*Field{
										{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
										{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
									},
								}),
							},
							{Data: []byte(`]}}}`), SegmentType: StaticSegmentType},
						},
					},
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						RootFields:     []GraphCoordinate{{TypeName: "Product", FieldName: "name"}},
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.product", ObjectPath("product")),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("product"),
						Value: &Object{
							Path: []string{"product"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
							},
						},
					},
				},
			},
		}

		loader := &Loader{}
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		snap := ctx.GetCacheStats()

		// UseL1Cache=false on FetchCacheConfiguration skips L1 lookup entirely
		// UseL1Cache=false on FetchCacheConfiguration skips L1 lookup entirely
		assert.Equal(t, 0, len(snap.L1Reads))
	})
}

// TestCacheAnalytics_EntityCounting_Integration verifies that entity instances
// are counted during the two-pass resolution walk (not just during loading).
func TestCacheAnalytics_EntityCounting_Integration(t *testing.T) {
	t.Run("entity instances counted during resolution", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"users":[{"__typename":"User","id":"u1","name":"Alice"},{"__typename":"User","id":"u2","name":"Bob"}]}}`), nil
			}).Times(1)

		entityDS := NewMockDataSource(ctrl)
		entityDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"data":{"_entities":[{"__typename":"User","email":"alice@example.com"},{"__typename":"User","email":"bob@example.com"}]}}`), nil
			}).Times(1)

		userCacheKeyTemplate := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		}

		providesData := &Object{
			Fields: []*Field{
				{Name: []byte("email"), Value: &Scalar{Path: []string{"email"}, Nullable: false}},
			},
		}

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			},
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: rootDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
						},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource:               entityDS,
						RequiresEntityBatchFetch: true,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data", "_entities"},
						},
						Caching: FetchCacheConfiguration{
							Enabled:          true,
							CacheName:        "default",
							TTL:              30 * time.Second,
							CacheKeyTemplate: userCacheKeyTemplate,
							UseL1Cache:       true,
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST"}`), SegmentType: StaticSegmentType},
						},
					},
					Info: &FetchInfo{
						DataSourceID:   "accounts",
						DataSourceName: "accounts",
						RootFields:     []GraphCoordinate{{TypeName: "User", FieldName: "email"}},
						OperationType:  ast.OperationTypeQuery,
						ProvidesData:   providesData,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query.users", ObjectPath("users"), FetchItemPathElement{Kind: FetchItemPathElementKindArray}),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("users"),
						Value: &Array{
							Path: []string{"users"},
							Item: &Object{
								TypeName: "User",
								CacheAnalytics: &ObjectCacheAnalytics{
									KeyFields: []KeyField{{Name: "id"}},
								},
								Fields: []*Field{
									{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
									{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
									{Name: []byte("email"), Value: &String{Path: []string{"email"}}},
								},
							},
						},
					},
				},
			},
		}

		loader := &Loader{}
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		// Resolve to trigger entity counting and field hashing
		buf := &bytes.Buffer{}
		err = resolvable.Resolve(context.Background(), response.Data, response.Fetches, buf)
		require.NoError(t, err)

		snap := ctx.GetCacheStats()

		// 1 entity type (User); 2 instances from batch fetch (Alice, Bob)
		require.Equal(t, 1, len(snap.EntityTypes))
		assert.Equal(t, "User", snap.EntityTypes[0].TypeName)
		assert.Equal(t, 2, snap.EntityTypes[0].Count)
	})
}

// TestCacheAnalytics_ErrorCodeExtraction verifies that extensions.code is
// extracted from subgraph error responses into analytics error events.
func TestCacheAnalytics_ErrorCodeExtraction(t *testing.T) {
	t.Run("extracts extensions.code from subgraph error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"errors":[{"message":"not authorized","extensions":{"code":"UNAUTHORIZED"}}],"data":{"product":null}}`), nil
			}).Times(1)

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			},
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: rootDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath:   []string{"data"},
							SelectResponseErrorsPath: []string{"errors"},
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://products.service","body":{"query":"{product {id}}"}}`), SegmentType: StaticSegmentType},
						},
					},
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						RootFields:     []GraphCoordinate{{TypeName: "Query", FieldName: "product"}},
						OperationType:  ast.OperationTypeQuery,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("product"),
						Value: &Object{
							Path:     []string{"product"},
							Nullable: true,
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
							},
						},
					},
				},
			},
		}

		loader := &Loader{}
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		snap := ctx.GetCacheStats()

		// Code extracted from errors[0].extensions.code in the subgraph response
		require.Equal(t, 1, len(snap.ErrorEvents))
		assert.Equal(t, "products", snap.ErrorEvents[0].DataSource)
		assert.Equal(t, "not authorized", snap.ErrorEvents[0].Message)
		assert.Equal(t, "UNAUTHORIZED", snap.ErrorEvents[0].Code)
	})

	t.Run("empty code when no extensions.code", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rootDS := NewMockDataSource(ctrl)
		rootDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
				return []byte(`{"errors":[{"message":"internal server error"}],"data":{"product":null}}`), nil
			}).Times(1)

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			},
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: rootDS,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath:   []string{"data"},
							SelectResponseErrorsPath: []string{"errors"},
						},
					},
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{Data: []byte(`{"method":"POST","url":"http://products.service","body":{"query":"{product {id}}"}}`), SegmentType: StaticSegmentType},
						},
					},
					Info: &FetchInfo{
						DataSourceID:   "products",
						DataSourceName: "products",
						RootFields:     []GraphCoordinate{{TypeName: "Query", FieldName: "product"}},
						OperationType:  ast.OperationTypeQuery,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "query"),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("product"),
						Value: &Object{
							Path:     []string{"product"},
							Nullable: true,
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
							},
						},
					},
				},
			},
		}

		loader := &Loader{}
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		resolvable := NewResolvable(ar, ResolvableOptions{})
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		require.NoError(t, err)

		err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		require.NoError(t, err)

		snap := ctx.GetCacheStats()

		// Code is empty because the response error has no extensions object
		require.Equal(t, 1, len(snap.ErrorEvents))
		assert.Equal(t, "products", snap.ErrorEvents[0].DataSource)
		assert.Equal(t, "internal server error", snap.ErrorEvents[0].Message)
		assert.Equal(t, "", snap.ErrorEvents[0].Code)
	})
}

// =============================================================================
// Benchmarks
// =============================================================================

// TestCacheAnalyticsCollector_HitCount verifies the L1HitCount/L2HitCount
// convenience methods that count only hit events from raw event slices.
func TestCacheAnalyticsCollector_HitCount(t *testing.T) {
	c := NewCacheAnalyticsCollector()

	// 2 L1 hits, 1 L1 miss
	c.RecordL1KeyEvent(CacheKeyHit, "User", "k1", "accounts", 100)
	c.RecordL1KeyEvent(CacheKeyHit, "User", "k2", "accounts", 100)
	c.RecordL1KeyEvent(CacheKeyMiss, "User", "k3", "accounts", 0)

	// 1 L2 hit, 1 L2 miss
	c.RecordL2KeyEvent(CacheKeyHit, "Product", "k4", "products", 200)
	c.RecordL2KeyEvent(CacheKeyMiss, "Product", "k5", "products", 0)

	snap := c.Snapshot()
	// 2 L1 hits out of 3, 1 L2 hit out of 2
	assert.Equal(t, int64(2), snap.L1HitCount())
	assert.Equal(t, int64(1), snap.L2HitCount())
}

// TestCacheAnalyticsCollector_HitCount_Zero verifies hit counts return 0
// on an empty snapshot (no events recorded).
func TestCacheAnalyticsCollector_HitCount_Zero(t *testing.T) {
	snap := CacheAnalyticsSnapshot{}
	assert.Equal(t, int64(0), snap.L1HitCount())
	assert.Equal(t, int64(0), snap.L2HitCount())
}

// TestCacheAnalyticsCollector_FetchTiming verifies fetch timing recording,
// merging from goroutines, average duration computation, and time-saved
// estimation based on cache hits.
func TestCacheAnalyticsCollector_FetchTiming(t *testing.T) {
	t.Run("fetch timings recorded and merged", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		// Record main thread timing
		c.RecordFetchTiming(FetchTimingEvent{
			DataSource:    "accounts",
			EntityType:    "User",
			DurationMs:    5, // 5ms
			Source:        FieldSourceSubgraph,
			ItemCount:     2,
			IsEntityFetch: true,
		})

		// Simulate goroutine timings
		l2Timings := []FetchTimingEvent{
			{DataSource: "products", EntityType: "Product", DurationMs: 2, Source: FieldSourceL2, ItemCount: 3, IsEntityFetch: true},
			{DataSource: "accounts", EntityType: "User", DurationMs: 1, Source: FieldSourceL2, ItemCount: 1, IsEntityFetch: true},
		}
		c.MergeL2FetchTimings(l2Timings)

		snap := c.Snapshot()
		// 1 main-thread + 2 merged from goroutines
		assert.Equal(t, 3, len(snap.FetchTimings))

		assert.Equal(t, "accounts", snap.FetchTimings[0].DataSource)
		assert.Equal(t, FieldSourceSubgraph, snap.FetchTimings[0].Source)
		assert.Equal(t, int64(5), snap.FetchTimings[0].DurationMs)
		assert.Equal(t, 2, snap.FetchTimings[0].ItemCount)
		assert.Equal(t, true, snap.FetchTimings[0].IsEntityFetch)

		assert.Equal(t, "products", snap.FetchTimings[1].DataSource)
		assert.Equal(t, FieldSourceL2, snap.FetchTimings[1].Source)
	})

	t.Run("avg fetch duration by datasource", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.RecordFetchTiming(FetchTimingEvent{DataSource: "accounts", DurationMs: 4, Source: FieldSourceSubgraph})
		c.RecordFetchTiming(FetchTimingEvent{DataSource: "accounts", DurationMs: 6, Source: FieldSourceSubgraph})
		c.RecordFetchTiming(FetchTimingEvent{DataSource: "accounts", DurationMs: 1, Source: FieldSourceL2}) // L2 should be excluded
		c.RecordFetchTiming(FetchTimingEvent{DataSource: "products", DurationMs: 10, Source: FieldSourceSubgraph})

		snap := c.Snapshot()
		// accounts: (4+6)/2 = 5ms (L2 excluded), products: 10/1 = 10ms
		assert.Equal(t, int64(5), snap.AvgFetchDurationMs("accounts"))
		assert.Equal(t, int64(10), snap.AvgFetchDurationMs("products"))
		assert.Equal(t, int64(0), snap.AvgFetchDurationMs("unknown"))
	})

	t.Run("total time saved", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		// 2 subgraph fetches for accounts, avg 5ms
		c.RecordFetchTiming(FetchTimingEvent{DataSource: "accounts", DurationMs: 4, Source: FieldSourceSubgraph})
		c.RecordFetchTiming(FetchTimingEvent{DataSource: "accounts", DurationMs: 6, Source: FieldSourceSubgraph})

		// 3 cache hits for accounts
		c.RecordL1KeyEvent(CacheKeyHit, "User", "k1", "accounts", 100)
		c.RecordL1KeyEvent(CacheKeyHit, "User", "k2", "accounts", 100)
		c.RecordL2KeyEvent(CacheKeyHit, "User", "k3", "accounts", 100)

		snap := c.Snapshot()
		// avg fetch duration = 5ms, 3 hits = 15ms saved
		assert.Equal(t, int64(15), snap.TotalTimeSavedMs())
	})
}

// TestCacheAnalyticsCollector_ErrorEvents verifies error event recording,
// goroutine merging, per-datasource breakdown, and error rate computation.
func TestCacheAnalyticsCollector_ErrorEvents(t *testing.T) {
	t.Run("error events recorded and merged", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.RecordError(SubgraphErrorEvent{
			DataSource: "accounts",
			EntityType: "User",
			Message:    "connection refused",
		})

		// Simulate goroutine errors
		l2Errors := []SubgraphErrorEvent{
			{DataSource: "products", EntityType: "Product", Message: "timeout"},
		}
		c.MergeL2Errors(l2Errors)

		snap := c.Snapshot()
		assert.Equal(t, 2, len(snap.ErrorEvents))
		assert.Equal(t, "accounts", snap.ErrorEvents[0].DataSource)
		assert.Equal(t, "connection refused", snap.ErrorEvents[0].Message)
		assert.Equal(t, "products", snap.ErrorEvents[1].DataSource)
		assert.Equal(t, "timeout", snap.ErrorEvents[1].Message)
	})

	t.Run("errors by datasource", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.RecordError(SubgraphErrorEvent{DataSource: "accounts", Message: "err1"})
		c.RecordError(SubgraphErrorEvent{DataSource: "accounts", Message: "err2"})
		c.RecordError(SubgraphErrorEvent{DataSource: "products", Message: "err3"})

		snap := c.Snapshot()
		byDS := snap.ErrorsByDataSource()
		assert.Equal(t, 2, byDS["accounts"])
		assert.Equal(t, 1, byDS["products"])
	})

	t.Run("errors by datasource returns nil when no errors", func(t *testing.T) {
		snap := CacheAnalyticsSnapshot{}
		assert.Nil(t, snap.ErrorsByDataSource())
	})

	t.Run("error rate", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		// 3 successful fetches + 1 error = 25% error rate
		c.RecordFetchTiming(FetchTimingEvent{DataSource: "accounts", Source: FieldSourceSubgraph})
		c.RecordFetchTiming(FetchTimingEvent{DataSource: "accounts", Source: FieldSourceSubgraph})
		c.RecordFetchTiming(FetchTimingEvent{DataSource: "products", Source: FieldSourceSubgraph})
		c.RecordError(SubgraphErrorEvent{DataSource: "accounts", Message: "err"})

		snap := c.Snapshot()
		// 1 error / (3 fetches + 1 error) = 0.25
		assert.Equal(t, 0.25, snap.ErrorRate())
	})

	t.Run("error rate zero when no errors", func(t *testing.T) {
		snap := CacheAnalyticsSnapshot{}
		assert.Equal(t, float64(0), snap.ErrorRate())
	})

	t.Run("error code from extensions", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.RecordError(SubgraphErrorEvent{
			DataSource: "accounts",
			EntityType: "User",
			Message:    "not authorized",
			Code:       "UNAUTHORIZED",
		})
		c.RecordError(SubgraphErrorEvent{
			DataSource: "products",
			EntityType: "Product",
			Message:    "not found",
			// Code intentionally empty — no extensions.code
		})

		snap := c.Snapshot()
		assert.Equal(t, 2, len(snap.ErrorEvents))
		assert.Equal(t, "UNAUTHORIZED", snap.ErrorEvents[0].Code)
		assert.Equal(t, "", snap.ErrorEvents[1].Code)
	})
}

// TestCacheAnalyticsCollector_UniqueKeys verifies that entity unique key tracking
// correctly deduplicates keys while counting all instances.
func TestCacheAnalyticsCollector_UniqueKeys(t *testing.T) {
	t.Run("unique keys tracked correctly", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.IncrementEntityCount("User", `{"id":"1"}`)
		c.IncrementEntityCount("User", `{"id":"2"}`)
		c.IncrementEntityCount("User", `{"id":"1"}`) // duplicate
		c.IncrementEntityCount("User", `{"id":"3"}`)
		c.IncrementEntityCount("Product", `{"upc":"a"}`)

		snap := c.Snapshot()
		assert.Equal(t, 2, len(snap.EntityTypes))

		for _, et := range snap.EntityTypes {
			switch et.TypeName {
			case "User":
				assert.Equal(t, 4, et.Count, "User should have 4 instances")
				assert.Equal(t, 3, et.UniqueKeys, "User should have 3 unique keys")
			case "Product":
				assert.Equal(t, 1, et.Count, "Product should have 1 instance")
				assert.Equal(t, 1, et.UniqueKeys, "Product should have 1 unique key")
			}
		}
	})

	t.Run("empty keyJSON not tracked for unique keys", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.IncrementEntityCount("User", "")
		c.IncrementEntityCount("User", "")
		c.IncrementEntityCount("User", `{"id":"1"}`)

		snap := c.Snapshot()
		assert.Equal(t, 1, len(snap.EntityTypes))
		assert.Equal(t, 3, snap.EntityTypes[0].Count, "should count all 3 instances")
		assert.Equal(t, 1, snap.EntityTypes[0].UniqueKeys, "should have 1 unique key (empty strings not tracked)")
	})
}

// TestCacheAnalyticsCollector_CacheAge verifies cache age computation from
// remaining TTL, and average/max age aggregation across L2 hit events.
func TestCacheAnalyticsCollector_CacheAge(t *testing.T) {
	t.Run("cache age computed correctly", func(t *testing.T) {
		// Test computeCacheAgeMs directly
		assert.Equal(t, int64(5000), computeCacheAgeMs(25*time.Second, 30*time.Second))
		assert.Equal(t, int64(0), computeCacheAgeMs(0, 30*time.Second))
		assert.Equal(t, int64(0), computeCacheAgeMs(30*time.Second, 0))
		assert.Equal(t, int64(0), computeCacheAgeMs(35*time.Second, 30*time.Second))
	})

	t.Run("avg cache age", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		// Record L2 hits with different ages using MergeL2Events
		c.MergeL2Events([]CacheKeyEvent{
			{EntityType: "User", Kind: CacheKeyHit, CacheKey: "k1", DataSource: "ds", ByteSize: 100, CacheAgeMs: 5000},
			{EntityType: "User", Kind: CacheKeyHit, CacheKey: "k2", DataSource: "ds", ByteSize: 100, CacheAgeMs: 15000},
			{EntityType: "Product", Kind: CacheKeyHit, CacheKey: "k3", DataSource: "ds", ByteSize: 100, CacheAgeMs: 3000},
			{EntityType: "User", Kind: CacheKeyMiss, CacheKey: "k4", DataSource: "ds", ByteSize: 0, CacheAgeMs: 0}, // miss, should be ignored
		})

		snap := c.Snapshot()
		// User: (5000+15000)/2 = 10000, Product: 3000/1
		assert.Equal(t, int64(10000), snap.AvgCacheAgeMs("User"))
		assert.Equal(t, int64(3000), snap.AvgCacheAgeMs("Product"))
		assert.Equal(t, int64(0), snap.AvgCacheAgeMs("Unknown"))

		// Empty entity type = all types
		// (5000 + 15000 + 3000) / 3 = 7666
		assert.Equal(t, int64(7666), snap.AvgCacheAgeMs(""))
	})

	t.Run("max cache age", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.MergeL2Events([]CacheKeyEvent{
			{EntityType: "User", Kind: CacheKeyHit, CacheKey: "k1", DataSource: "ds", ByteSize: 100, CacheAgeMs: 5000},
			{EntityType: "User", Kind: CacheKeyHit, CacheKey: "k2", DataSource: "ds", ByteSize: 100, CacheAgeMs: 20000},
			{EntityType: "Product", Kind: CacheKeyHit, CacheKey: "k3", DataSource: "ds", ByteSize: 100, CacheAgeMs: 3000},
		})

		snap := c.Snapshot()
		assert.Equal(t, int64(20000), snap.MaxCacheAgeMs())
	})

	t.Run("max cache age zero when no hits", func(t *testing.T) {
		snap := CacheAnalyticsSnapshot{}
		assert.Equal(t, int64(0), snap.MaxCacheAgeMs())
	})
}

// TestTruncateErrorMessage verifies UTF-8-safe truncation of error messages
// to prevent oversized analytics payloads.
func TestTruncateErrorMessage(t *testing.T) {
	assert.Equal(t, "short", truncateErrorMessage("short", 10))
	assert.Equal(t, "12345", truncateErrorMessage("1234567890", 5))
	assert.Equal(t, "", truncateErrorMessage("", 10))
	assert.Equal(t, "exact", truncateErrorMessage("exact", 5))
	assert.Equal(t, "hello ", truncateErrorMessage("hello 世界 test", 8), "cuts before 世 (3-byte char at positions 6-8)")
}

func BenchmarkCacheAnalytics_Disabled(b *testing.B) {
	// Verify zero overhead when analytics is disabled
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableL1Cache = true
	// EnableCacheAnalytics = false (default)

	b.ResetTimer()
	for b.Loop() {
		// This is the guard check that should be essentially free
		if ctx.cacheAnalyticsEnabled() {
			ctx.cacheAnalytics.RecordL1KeyEvent(CacheKeyHit, "User", "key", "ds", 100)
		}
	}
}

func BenchmarkCacheAnalytics_Enabled(b *testing.B) {
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableL1Cache = true
	ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
	ctx.initCacheAnalytics()

	b.ResetTimer()
	for b.Loop() {
		if ctx.cacheAnalyticsEnabled() {
			ctx.cacheAnalytics.RecordL1KeyEvent(CacheKeyHit, "User", "key", "ds", 100)
		}
	}
}

// =============================================================================
// Shadow Mode Unit Tests
// =============================================================================

// TestFieldSourceShadowCached verifies that FieldSourceShadowCached is a
// distinct source value that can be used in field hashing alongside
// Subgraph/L1/L2 sources for shadow mode comparisons.
func TestFieldSourceShadowCached(t *testing.T) {
	t.Run("constant value", func(t *testing.T) {
		assert.Equal(t, FieldSource(3), FieldSourceShadowCached, "FieldSourceShadowCached should be 3")
	})

	t.Run("HashFieldValue with FieldSourceShadowCached", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.HashFieldValue("User", "username", []byte(`"Alice"`), `{"id":"1"}`, 0, FieldSourceShadowCached)

		snap := c.Snapshot()
		require.Equal(t, 1, len(snap.FieldHashes))
		assert.Equal(t, "User", snap.FieldHashes[0].EntityType)
		assert.Equal(t, "username", snap.FieldHashes[0].FieldName)
		assert.Equal(t, `{"id":"1"}`, snap.FieldHashes[0].KeyRaw)
		assert.Equal(t, FieldSourceShadowCached, snap.FieldHashes[0].Source)
	})

	t.Run("can distinguish from other sources", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.HashFieldValue("User", "name", []byte(`"Alice"`), `{"id":"1"}`, 0, FieldSourceSubgraph)
		c.HashFieldValue("User", "name", []byte(`"Alice"`), `{"id":"1"}`, 0, FieldSourceShadowCached)

		snap := c.Snapshot()
		require.Equal(t, 2, len(snap.FieldHashes))
		assert.Equal(t, FieldSourceSubgraph, snap.FieldHashes[0].Source)
		assert.Equal(t, FieldSourceShadowCached, snap.FieldHashes[1].Source)
		// Same input, same hash regardless of source
		assert.Equal(t, snap.FieldHashes[0].FieldHash, snap.FieldHashes[1].FieldHash, "same input = same hash")
	})
}

// TestShadowComparisonEvent_Recording verifies that shadow comparison events
// capture all fields (hash, size, age, TTL) needed to detect staleness.
func TestShadowComparisonEvent_Recording(t *testing.T) {
	c := NewCacheAnalyticsCollector()

	c.RecordShadowComparison(ShadowComparisonEvent{
		CacheKey:      "key1",
		EntityType:    "User",
		IsFresh:       true,
		CachedHash:    12345,
		FreshHash:     12345,
		CachedBytes:   100,
		FreshBytes:    100,
		DataSource:    "accounts",
		CacheAgeMs:    5000,
		ConfiguredTTL: 30 * time.Second,
	})
	c.RecordShadowComparison(ShadowComparisonEvent{
		CacheKey:      "key2",
		EntityType:    "Product",
		IsFresh:       false,
		CachedHash:    11111,
		FreshHash:     22222,
		CachedBytes:   80,
		FreshBytes:    90,
		DataSource:    "products",
		CacheAgeMs:    10000,
		ConfiguredTTL: 60 * time.Second,
	})

	snap := c.Snapshot()
	assert.Equal(t, 2, len(snap.ShadowComparisons))

	assert.Equal(t, "key1", snap.ShadowComparisons[0].CacheKey)
	assert.Equal(t, "User", snap.ShadowComparisons[0].EntityType)
	assert.Equal(t, true, snap.ShadowComparisons[0].IsFresh)
	assert.Equal(t, uint64(12345), snap.ShadowComparisons[0].CachedHash)
	assert.Equal(t, uint64(12345), snap.ShadowComparisons[0].FreshHash)
	assert.Equal(t, 100, snap.ShadowComparisons[0].CachedBytes)
	assert.Equal(t, 100, snap.ShadowComparisons[0].FreshBytes)
	assert.Equal(t, "accounts", snap.ShadowComparisons[0].DataSource)
	assert.Equal(t, int64(5000), snap.ShadowComparisons[0].CacheAgeMs)
	assert.Equal(t, 30*time.Second, snap.ShadowComparisons[0].ConfiguredTTL)

	assert.Equal(t, "key2", snap.ShadowComparisons[1].CacheKey)
	assert.Equal(t, "Product", snap.ShadowComparisons[1].EntityType)
	assert.Equal(t, false, snap.ShadowComparisons[1].IsFresh)
	assert.Equal(t, uint64(11111), snap.ShadowComparisons[1].CachedHash)
	assert.Equal(t, uint64(22222), snap.ShadowComparisons[1].FreshHash)
	assert.Equal(t, "products", snap.ShadowComparisons[1].DataSource)
	assert.Equal(t, int64(10000), snap.ShadowComparisons[1].CacheAgeMs)
	assert.Equal(t, 60*time.Second, snap.ShadowComparisons[1].ConfiguredTTL)
}

// TestShadowFreshnessRate verifies the freshness rate calculation across
// all shadow comparisons (fresh / total).
func TestShadowFreshnessRate(t *testing.T) {
	t.Run("mix of fresh and stale", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.RecordShadowComparison(ShadowComparisonEvent{CacheKey: "k1", EntityType: "User", IsFresh: true})
		c.RecordShadowComparison(ShadowComparisonEvent{CacheKey: "k2", EntityType: "User", IsFresh: true})
		c.RecordShadowComparison(ShadowComparisonEvent{CacheKey: "k3", EntityType: "User", IsFresh: false})
		c.RecordShadowComparison(ShadowComparisonEvent{CacheKey: "k4", EntityType: "User", IsFresh: true})

		snap := c.Snapshot()
		// 3 fresh / 4 total = 0.75
		assert.Equal(t, 0.75, snap.ShadowFreshnessRate())
	})

	t.Run("all fresh", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.RecordShadowComparison(ShadowComparisonEvent{CacheKey: "k1", IsFresh: true})
		c.RecordShadowComparison(ShadowComparisonEvent{CacheKey: "k2", IsFresh: true})

		snap := c.Snapshot()
		assert.Equal(t, 1.0, snap.ShadowFreshnessRate())
	})

	t.Run("all stale", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.RecordShadowComparison(ShadowComparisonEvent{CacheKey: "k1", IsFresh: false})
		c.RecordShadowComparison(ShadowComparisonEvent{CacheKey: "k2", IsFresh: false})

		snap := c.Snapshot()
		assert.Equal(t, 0.0, snap.ShadowFreshnessRate())
	})

	t.Run("empty returns zero", func(t *testing.T) {
		snap := CacheAnalyticsSnapshot{}
		assert.Equal(t, 0.0, snap.ShadowFreshnessRate())
	})
}

// TestShadowFreshnessRateByEntityType verifies per-entity-type freshness rate
// breakdown for shadow mode comparisons.
func TestShadowFreshnessRateByEntityType(t *testing.T) {
	c := NewCacheAnalyticsCollector()

	c.RecordShadowComparison(ShadowComparisonEvent{CacheKey: "k1", EntityType: "User", IsFresh: true})
	c.RecordShadowComparison(ShadowComparisonEvent{CacheKey: "k2", EntityType: "User", IsFresh: false})
	c.RecordShadowComparison(ShadowComparisonEvent{CacheKey: "k3", EntityType: "Product", IsFresh: true})
	c.RecordShadowComparison(ShadowComparisonEvent{CacheKey: "k4", EntityType: "Product", IsFresh: true})

	snap := c.Snapshot()
	byType := snap.ShadowFreshnessRateByEntityType()

	// User: 1 fresh / 2 = 0.5, Product: 2 fresh / 2 = 1.0
	assert.Equal(t, 0.5, byType["User"])
	assert.Equal(t, 1.0, byType["Product"])
}

func TestShadowFreshnessRateByEntityType_Empty(t *testing.T) {
	snap := CacheAnalyticsSnapshot{}
	assert.Nil(t, snap.ShadowFreshnessRateByEntityType(), "should return nil with no events")
}

func TestShadowStaleCount(t *testing.T) {
	c := NewCacheAnalyticsCollector()

	c.RecordShadowComparison(ShadowComparisonEvent{CacheKey: "k1", IsFresh: true})
	c.RecordShadowComparison(ShadowComparisonEvent{CacheKey: "k2", IsFresh: false})
	c.RecordShadowComparison(ShadowComparisonEvent{CacheKey: "k3", IsFresh: false})
	c.RecordShadowComparison(ShadowComparisonEvent{CacheKey: "k4", IsFresh: true})

	snap := c.Snapshot()
	assert.Equal(t, int64(2), snap.ShadowStaleCount(), "should have exactly 2 stale entries")
}

func TestShadowStaleCount_Empty(t *testing.T) {
	snap := CacheAnalyticsSnapshot{}
	assert.Equal(t, int64(0), snap.ShadowStaleCount(), "should have 0 stale entries with no events")
}

func TestCacheKeyEvent_ShadowFlag(t *testing.T) {
	c := NewCacheAnalyticsCollector()

	// Record shadow events using MergeL2Events
	c.MergeL2Events([]CacheKeyEvent{
		{CacheKey: "key1", EntityType: "User", Kind: CacheKeyHit, DataSource: "accounts", ByteSize: 128, Shadow: true},
		{CacheKey: "key2", EntityType: "User", Kind: CacheKeyMiss, DataSource: "accounts", ByteSize: 0, Shadow: false},
	})

	snap := c.Snapshot()
	assert.Equal(t, 2, len(snap.L2Reads), "should have exactly 2 L2 events")
	assert.Equal(t, true, snap.L2Reads[0].Shadow, "first event should be shadow")
	assert.Equal(t, false, snap.L2Reads[1].Shadow, "second event should not be shadow")

	// Filter shadow events
	var shadowHits int
	for _, ev := range snap.L2Reads {
		if ev.Shadow && ev.Kind == CacheKeyHit {
			shadowHits++
		}
	}
	assert.Equal(t, 1, shadowHits, "should have exactly 1 shadow hit")
}

func BenchmarkFieldHashing(b *testing.B) {
	c := NewCacheAnalyticsCollector()
	value := []byte(`"some-user-id-value-12345"`)

	b.ResetTimer()
	for b.Loop() {
		c.HashFieldValue("User", "id", value, `{"id":"1"}`, 0, FieldSourceSubgraph)
	}
}

func TestSnapshotDeduplication(t *testing.T) {
	t.Run("duplicate L2 reads consolidated by CacheKey+Kind", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		// Simulate batch entity fetch where two reviews reference the same User 1234
		c.MergeL2Events([]CacheKeyEvent{
			{CacheKey: "user-1234", EntityType: "User", Kind: CacheKeyMiss, DataSource: "accounts"},
			{CacheKey: "user-1234", EntityType: "User", Kind: CacheKeyMiss, DataSource: "accounts"},
			{CacheKey: "product-1", EntityType: "Product", Kind: CacheKeyMiss, DataSource: "products"},
		})

		snap := c.Snapshot()
		assert.Equal(t, 2, len(snap.L2Reads), "duplicate User miss should be consolidated into one event")
		assert.Equal(t, "user-1234", snap.L2Reads[0].CacheKey)
		assert.Equal(t, "product-1", snap.L2Reads[1].CacheKey)
	})

	t.Run("same key with different Kind preserved", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		// Same key can have different kinds across requests (miss then hit) — both kept
		c.MergeL2Events([]CacheKeyEvent{
			{CacheKey: "user-1234", EntityType: "User", Kind: CacheKeyMiss, DataSource: "accounts"},
			{CacheKey: "user-1234", EntityType: "User", Kind: CacheKeyHit, DataSource: "accounts", ByteSize: 49},
		})

		snap := c.Snapshot()
		assert.Equal(t, 2, len(snap.L2Reads), "same key with different Kind should be kept as separate events")
		assert.Equal(t, CacheKeyMiss, snap.L2Reads[0].Kind)
		assert.Equal(t, CacheKeyHit, snap.L2Reads[1].Kind)
	})

	t.Run("duplicate L2 writes consolidated by CacheKey", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		// Same entity written twice from batch positions
		c.RecordWrite(CacheWriteEvent{CacheKey: "user-1234", EntityType: "User", ByteSize: 49, DataSource: "accounts", CacheLevel: CacheLevelL2, TTL: 30 * time.Second, Source: CacheSourceQuery})
		c.RecordWrite(CacheWriteEvent{CacheKey: "user-1234", EntityType: "User", ByteSize: 49, DataSource: "accounts", CacheLevel: CacheLevelL2, TTL: 30 * time.Second, Source: CacheSourceQuery})
		c.RecordWrite(CacheWriteEvent{CacheKey: "product-1", EntityType: "Product", ByteSize: 128, DataSource: "products", CacheLevel: CacheLevelL2, TTL: 30 * time.Second, Source: CacheSourceQuery})

		snap := c.Snapshot()
		assert.Equal(t, 2, len(snap.L2Writes), "duplicate User write should be consolidated into one event")
		assert.Equal(t, "user-1234", snap.L2Writes[0].CacheKey)
		assert.Equal(t, "product-1", snap.L2Writes[1].CacheKey)
	})

	t.Run("duplicate shadow comparisons consolidated by CacheKey", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.RecordShadowComparison(ShadowComparisonEvent{
			CacheKey: "user-1234", EntityType: "User", IsFresh: true, CachedHash: 123, FreshHash: 123,
		})
		c.RecordShadowComparison(ShadowComparisonEvent{
			CacheKey: "user-1234", EntityType: "User", IsFresh: true, CachedHash: 123, FreshHash: 123,
		})

		snap := c.Snapshot()
		assert.Equal(t, 1, len(snap.ShadowComparisons), "duplicate shadow comparison should be consolidated into one event")
	})

	t.Run("no events returns empty slices unchanged", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()
		snap := c.Snapshot()
		assert.Equal(t, 0, len(snap.L1Reads))
		assert.Equal(t, 0, len(snap.L2Reads))
		assert.Equal(t, 0, len(snap.L1Writes))
		assert.Equal(t, 0, len(snap.L2Writes))
		assert.Equal(t, 0, len(snap.ShadowComparisons))
	})

	t.Run("derived metrics correct after dedup", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		// Two L2 hits for same key (batch positions) — should count as 1 hit, not 2
		c.MergeL2Events([]CacheKeyEvent{
			{CacheKey: "user-1234", EntityType: "User", Kind: CacheKeyHit, DataSource: "accounts", ByteSize: 49},
			{CacheKey: "user-1234", EntityType: "User", Kind: CacheKeyHit, DataSource: "accounts", ByteSize: 49},
			{CacheKey: "product-1", EntityType: "Product", Kind: CacheKeyMiss, DataSource: "products"},
		})

		snap := c.Snapshot()
		assert.Equal(t, 2, len(snap.L2Reads), "should have 2 unique events after dedup")
		assert.Equal(t, int64(0), snap.L1HitCount(), "no L1 hits in this test")
		assert.Equal(t, int64(1), snap.L2HitCount(), "1 unique L2 hit after dedup")
		assert.Equal(t, int64(49), snap.CachedBytesServed(), "bytes served from 1 unique hit")
	})
}

func TestCacheAnalyticsCollector_HeaderImpactEvents(t *testing.T) {
	base := HeaderImpactEvent{
		BaseKey: "key1", HeaderHash: 111, ResponseHash: 999,
		EntityType: "User", DataSource: "accounts",
	}

	t.Run("exact duplicates are collapsed", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()
		c.RecordHeaderImpactEvent(base)
		c.RecordHeaderImpactEvent(base)
		c.RecordHeaderImpactEvent(base)
		snap := c.Snapshot()
		assert.Equal(t, []HeaderImpactEvent{base}, snap.HeaderImpactEvents)
	})

	t.Run("different BaseKey is preserved", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()
		other := base
		other.BaseKey = "key2"
		c.RecordHeaderImpactEvent(base)
		c.RecordHeaderImpactEvent(other)
		snap := c.Snapshot()
		assert.Equal(t, []HeaderImpactEvent{base, other}, snap.HeaderImpactEvents)
	})

	t.Run("different HeaderHash is preserved", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()
		other := base
		other.HeaderHash = 222
		c.RecordHeaderImpactEvent(base)
		c.RecordHeaderImpactEvent(other)
		snap := c.Snapshot()
		assert.Equal(t, []HeaderImpactEvent{base, other}, snap.HeaderImpactEvents)
	})

	t.Run("different ResponseHash is preserved", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()
		other := base
		other.ResponseHash = 888
		c.RecordHeaderImpactEvent(base)
		c.RecordHeaderImpactEvent(other)
		snap := c.Snapshot()
		assert.Equal(t, []HeaderImpactEvent{base, other}, snap.HeaderImpactEvents)
	})

	t.Run("different EntityType is preserved", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()
		other := base
		other.EntityType = "Product"
		c.RecordHeaderImpactEvent(base)
		c.RecordHeaderImpactEvent(other)
		snap := c.Snapshot()
		assert.Equal(t, []HeaderImpactEvent{base, other}, snap.HeaderImpactEvents)
	})

	t.Run("different DataSource is preserved", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()
		other := base
		other.DataSource = "reviews"
		c.RecordHeaderImpactEvent(base)
		c.RecordHeaderImpactEvent(other)
		snap := c.Snapshot()
		assert.Equal(t, []HeaderImpactEvent{base, other}, snap.HeaderImpactEvents)
	})

	t.Run("single event is preserved", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()
		c.RecordHeaderImpactEvent(base)
		snap := c.Snapshot()
		assert.Equal(t, []HeaderImpactEvent{base}, snap.HeaderImpactEvents)
	})

	t.Run("empty when no events recorded", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()
		snap := c.Snapshot()
		assert.Equal(t, 0, len(snap.HeaderImpactEvents))
	})
}

// TestCacheAnalyticsCollector_WriteEventSource verifies that the Source field
// (query vs mutation vs subscription) survives the record→snapshot pipeline.
// Without this, analytics consumers can't distinguish why a cache write happened,
// which breaks per-origin cache hit-rate reporting and mutation-aware invalidation dashboards.
func TestCacheAnalyticsCollector_WriteEventSource(t *testing.T) {
	// Each CacheSource variant must appear in the snapshot exactly as recorded.
	t.Run("write events preserve source field", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.RecordWrite(CacheWriteEvent{CacheKey: "key1", EntityType: "User", ByteSize: 128, DataSource: "accounts", CacheLevel: CacheLevelL2, TTL: 30 * time.Second, Source: CacheSourceQuery})
		c.RecordWrite(CacheWriteEvent{CacheKey: "key2", EntityType: "Product", ByteSize: 256, DataSource: "products", CacheLevel: CacheLevelL2, TTL: 60 * time.Second, Source: CacheSourceMutation})
		c.RecordWrite(CacheWriteEvent{CacheKey: "key3", EntityType: "Review", ByteSize: 512, DataSource: "reviews", CacheLevel: CacheLevelL2, TTL: 90 * time.Second, Source: CacheSourceSubscription})

		snap := c.Snapshot()
		// Assert entire L2Writes slice — each event preserves its Source from the recording call
		assert.Equal(t, []CacheWriteEvent{
			{CacheKey: "key1", EntityType: "User", ByteSize: 128, DataSource: "accounts", CacheLevel: CacheLevelL2, TTL: 30 * time.Second, Source: CacheSourceQuery},         // Recorded with CacheSourceQuery
			{CacheKey: "key2", EntityType: "Product", ByteSize: 256, DataSource: "products", CacheLevel: CacheLevelL2, TTL: 60 * time.Second, Source: CacheSourceMutation},   // Recorded with CacheSourceMutation
			{CacheKey: "key3", EntityType: "Review", ByteSize: 512, DataSource: "reviews", CacheLevel: CacheLevelL2, TTL: 90 * time.Second, Source: CacheSourceSubscription}, // Recorded with CacheSourceSubscription
		}, snap.L2Writes)
	})

	// MutationEvent is a struct passed by value — ensure Source isn't zeroed during copy.
	t.Run("mutation event preserves source field", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		event := MutationEvent{
			MutationRootField: "updateUsername",
			EntityType:        "User",
			EntityCacheKey:    `{"__typename":"User","key":{"id":"1"}}`,
			HadCachedValue:    true,
			IsStale:           true,
			CachedHash:        111,
			FreshHash:         222,
			CachedBytes:       64,
			FreshBytes:        72,
			Source:            CacheSourceMutation,
		}
		c.RecordMutationEvent(event)

		snap := c.Snapshot()
		// Assert entire MutationEvents slice — Source field preserved through record→snapshot
		assert.Equal(t, []MutationEvent{event}, snap.MutationEvents)
	})

	// Same entity type, different sources — verifies events aren't collapsed or overwritten.
	t.Run("mixed sources in single snapshot", func(t *testing.T) {
		c := NewCacheAnalyticsCollector()

		c.RecordWrite(CacheWriteEvent{CacheKey: "query-key-1", EntityType: "User", ByteSize: 128, DataSource: "accounts", CacheLevel: CacheLevelL2, TTL: 30 * time.Second, Source: CacheSourceQuery})       // Write from query resolution
		c.RecordWrite(CacheWriteEvent{CacheKey: "mutation-key-2", EntityType: "User", ByteSize: 256, DataSource: "accounts", CacheLevel: CacheLevelL2, TTL: 30 * time.Second, Source: CacheSourceMutation}) // Write from mutation resolution

		snap := c.Snapshot()
		// Assert entire L2Writes — different keys prevent deduplication, each retains its Source
		assert.Equal(t, []CacheWriteEvent{
			{CacheKey: "query-key-1", EntityType: "User", ByteSize: 128, DataSource: "accounts", CacheLevel: CacheLevelL2, TTL: 30 * time.Second, Source: CacheSourceQuery},       // Query-triggered write
			{CacheKey: "mutation-key-2", EntityType: "User", ByteSize: 256, DataSource: "accounts", CacheLevel: CacheLevelL2, TTL: 30 * time.Second, Source: CacheSourceMutation}, // Mutation-triggered write
		}, snap.L2Writes)
	})
}

// TestSnapshotIndependentOfPooledCollector verifies that a snapshot returned
// from Snapshot() does not share backing arrays with the collector's internal
// slices. GetCacheStats returns the collector to the pool immediately after
// snapshotting; a subsequent request may acquire the same collector and mutate
// its slices while the caller is still iterating the snapshot. Under -race
// this exposes a data race on the shared backing array. Uses single-event
// writes so that pool-recycled collectors hit position 0 of the pre-allocated
// backing array (cap 8) repeatedly, which is exactly the position the reader
// is iterating.
func TestSnapshotIndependentOfPooledCollector(t *testing.T) {
	// Populate a collector, snapshot it, release it to the pool.
	c := AcquireCacheAnalyticsCollector()
	c.RecordFetchTiming(FetchTimingEvent{DataSource: "ds", DurationMs: 42})
	snap := c.Snapshot()
	ReleaseCacheAnalyticsCollector(c)

	require.Len(t, snap.FetchTimings, 1)

	// Reader: iterate snap.FetchTimings repeatedly (simulates
	// recordEntityCacheMetrics iterating the snapshot).
	// Writer: re-acquire a collector (pool returns the same one whose
	// backing array is aliased by snap.FetchTimings) and record a fetch
	// timing, which overwrites position 0 of the shared backing array.
	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Go(func() {
		for {
			select {
			case <-done:
				return
			default:
				sum := int64(0)
				for _, ev := range snap.FetchTimings {
					sum += ev.DurationMs
				}
				_ = sum
			}
		}
	})

	wg.Go(func() {
		for range 10_000 {
			c2 := AcquireCacheAnalyticsCollector()
			c2.RecordFetchTiming(FetchTimingEvent{DataSource: "ds", DurationMs: 99})
			ReleaseCacheAnalyticsCollector(c2)
		}
		close(done)
	})
	wg.Wait()
}

// TestSnapshotSlicesAreIndependent verifies that mutating the collector's
// internal slices after Snapshot() — as happens when the pool recycles the
// collector via ResetForReuse + new Record* calls — does not alter the values
// observed through the snapshot. Without Snapshot() cloning each shared slice,
// the snapshot aliases the collector's backing arrays and the next request
// overwrites positions the caller is still reading.
func TestSnapshotSlicesAreIndependent(t *testing.T) {
	// Use a fresh collector instead of Acquire: RecordMutationEvent and
	// RecordCacheOperationError initialize slices that NewCacheAnalyticsCollector
	// leaves nil; Releasing the collector would leave the pool with a non-nil
	// empty slice and break downstream tests that assert.Equal a snapshot with
	// MutationEvents/CacheOpErrors set to nil.
	c := NewCacheAnalyticsCollector()

	c.RecordFetchTiming(FetchTimingEvent{DataSource: "ds-orig", DurationMs: 111})
	c.RecordError(SubgraphErrorEvent{DataSource: "ds-orig"})
	c.RecordMutationEvent(MutationEvent{EntityType: "User-orig"})
	c.RecordCacheOperationError(CacheOperationError{DataSource: "ds-orig"})
	c.HashFieldValue("User-orig", "name", []byte(`"a"`), "k-orig", 1, FieldSourceL1)

	snap := c.Snapshot()

	// Deep-copy the snapshot's slices BEFORE the collector is recycled.
	// These canonical values must still match snap.* after the collector
	// is reset and refilled with different events.
	origFetch := slices.Clone(snap.FetchTimings)
	origErrors := slices.Clone(snap.ErrorEvents)
	origMutations := slices.Clone(snap.MutationEvents)
	origCacheOpErrors := slices.Clone(snap.CacheOpErrors)
	origFieldHashes := slices.Clone(snap.FieldHashes)

	// Simulate the next request: pool returns c, ResetForReuse truncates
	// the slices to len=0 while retaining backing arrays, and subsequent
	// Record* calls overwrite position 0 of every shared backing array.
	c.ResetForReuse()
	for range 100 {
		c.RecordFetchTiming(FetchTimingEvent{DataSource: "ds-new", DurationMs: 999})
		c.RecordError(SubgraphErrorEvent{DataSource: "ds-new"})
		c.RecordMutationEvent(MutationEvent{EntityType: "User-new"})
		c.RecordCacheOperationError(CacheOperationError{DataSource: "ds-new"})
		c.HashFieldValue("User-new", "name", []byte(`"z"`), "k-new", 2, FieldSourceL2)
	}

	// Full-slice assertions — snapshot must still show the original events.
	assert.Equal(t, origFetch, snap.FetchTimings)
	assert.Equal(t, origErrors, snap.ErrorEvents)
	assert.Equal(t, origMutations, snap.MutationEvents)
	assert.Equal(t, origCacheOpErrors, snap.CacheOpErrors)
	assert.Equal(t, origFieldHashes, snap.FieldHashes)
}
