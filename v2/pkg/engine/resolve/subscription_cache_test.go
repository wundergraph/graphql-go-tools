package resolve

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const subscriptionCacheTestTimeout = 30 * time.Second

type subscriptionCacheLogEntry struct {
	Operation string
	Items     []subscriptionCacheLogItem
}

type subscriptionCacheLogItem struct {
	Key   string
	Value string
	TTL   time.Duration
}

type subscriptionCacheLoaderCache struct {
	mu      sync.Mutex
	entries map[string][]byte
	log     []subscriptionCacheLogEntry
}

func newSubscriptionCacheLoaderCache() *subscriptionCacheLoaderCache {
	return &subscriptionCacheLoaderCache{
		entries: map[string][]byte{},
	}
}

func (c *subscriptionCacheLoaderCache) Get(_ context.Context, keys []string) ([]*CacheEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	items := make([]subscriptionCacheLogItem, 0, len(keys))
	entries := make([]*CacheEntry, len(keys))
	for i, key := range keys {
		items = append(items, subscriptionCacheLogItem{Key: key})
		value, ok := c.entries[key]
		if !ok {
			continue
		}
		entries[i] = &CacheEntry{
			Key:   key,
			Value: append([]byte(nil), value...),
		}
	}
	c.log = append(c.log, subscriptionCacheLogEntry{
		Operation: "get",
		Items:     items,
	})
	return entries, nil
}

func (c *subscriptionCacheLoaderCache) Set(_ context.Context, entries []*CacheEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	items := make([]subscriptionCacheLogItem, 0, len(entries))
	for _, entry := range entries {
		c.entries[entry.Key] = append([]byte(nil), entry.Value...)
		items = append(items, subscriptionCacheLogItem{
			Key:   entry.Key,
			Value: string(entry.Value),
			TTL:   entry.TTL,
		})
	}
	c.log = append(c.log, subscriptionCacheLogEntry{
		Operation: "set",
		Items:     items,
	})
	return nil
}

func (c *subscriptionCacheLoaderCache) Delete(_ context.Context, keys []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	items := make([]subscriptionCacheLogItem, 0, len(keys))
	for _, key := range keys {
		delete(c.entries, key)
		items = append(items, subscriptionCacheLogItem{Key: key})
	}
	c.log = append(c.log, subscriptionCacheLogEntry{
		Operation: "delete",
		Items:     items,
	})
	return nil
}

func (c *subscriptionCacheLoaderCache) Snapshot() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()

	snapshot := make(map[string]string, len(c.entries))
	for key, value := range c.entries {
		snapshot[key] = string(value)
	}
	return snapshot
}

func (c *subscriptionCacheLoaderCache) GetLog() []subscriptionCacheLogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()

	log := make([]subscriptionCacheLogEntry, len(c.log))
	copy(log, c.log)
	return log
}

type subscriptionCacheStream struct {
	data []byte
}

func (s subscriptionCacheStream) Start(ctx *Context, _ http.Header, _ []byte, updater SubscriptionUpdater) error {
	go func() {
		updater.Update(s.data)
		updater.Complete()
		updater.Done()
	}()
	return nil
}

func (s subscriptionCacheStream) HashTriggerInput(input []byte, xxh *xxhash.Digest) error {
	_, err := xxh.Write(input)
	return err
}

type subscriptionCacheWriter struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	flushes  chan string
	complete chan struct{}
}

func newSubscriptionCacheWriter() *subscriptionCacheWriter {
	return &subscriptionCacheWriter{
		flushes:  make(chan string, 1),
		complete: make(chan struct{}, 1),
	}
}

func (w *subscriptionCacheWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *subscriptionCacheWriter) Flush() error {
	w.mu.Lock()
	message := w.buf.String()
	w.buf.Reset()
	w.mu.Unlock()
	w.flushes <- message
	return nil
}

func (w *subscriptionCacheWriter) Complete() {
	w.complete <- struct{}{}
}

func (w *subscriptionCacheWriter) Heartbeat() error {
	return nil
}

func (w *subscriptionCacheWriter) Error(_ []byte) {
}

func (w *subscriptionCacheWriter) AwaitFlush(t *testing.T) string {
	t.Helper()
	select {
	case message := <-w.flushes:
		return message
	case <-time.After(subscriptionCacheTestTimeout):
		t.Fatalf("timed out waiting for subscription flush")
		return ""
	}
}

func TestSubscriptionCachePopulateWritesL2AndCallback(t *testing.T) {
	cache := newSubscriptionCacheLoaderCache()
	var writes []CacheWriteEvent
	resolver := New(context.Background(), ResolverOptions{
		Caches: map[string]LoaderCache{
			"entities": cache,
		},
		OnSubscriptionCacheWrite: func(event CacheWriteEvent) {
			writes = append(writes, event)
		},
	})
	writer := newSubscriptionCacheWriter()
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	ctx.ExecutionOptions.Caching.GlobalCacheKeyPrefix = "global:"
	ctx.ExecutionOptions.Caching.L2CacheKeyInterceptor = func(info L2CacheKeyInterceptorInfo, key string) string {
		assert.Equal(t, L2CacheKeyInterceptorInfo{
			SubgraphName: "products",
			CacheName:    "entities",
		}, info)
		return "tenant:" + key
	}

	err := resolver.AsyncResolveGraphQLSubscription(ctx, subscriptionCachePlan([]byte(`{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":1}}}`), true), writer, SubscriptionIdentifier{
		ConnectionID:   1,
		SubscriptionID: 1,
	})
	require.NoError(t, err)

	message := writer.AwaitFlush(t)

	assert.Equal(t, `{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":1}}}`, message)
	assert.Equal(t, map[string]string{
		`tenant:global:{"__typename":"Product","key":{"upc":"top-4"}}`: `{"upc":"top-4","name":"Bowler","price":1,"__typename":"Product"}`,
	}, cache.Snapshot())
	assert.Equal(t, []subscriptionCacheLogEntry{
		{
			Operation: "set",
			Items: []subscriptionCacheLogItem{
				{
					Key:   `tenant:global:{"__typename":"Product","key":{"upc":"top-4"}}`,
					Value: `{"upc":"top-4","name":"Bowler","price":1,"__typename":"Product"}`,
					TTL:   30 * time.Second,
				},
			},
		},
	}, cache.GetLog())
	assert.Equal(t, []CacheWriteEvent{
		{
			Key:        `tenant:global:{"__typename":"Product","key":{"upc":"top-4"}}`,
			CacheKey:   `tenant:global:{"__typename":"Product","key":{"upc":"top-4"}}`,
			EntityType: "Product",
			Kind:       CacheAnalyticsEventKindL2Write,
			Bytes:      64,
			ByteSize:   64,
			TTL:        30 * time.Second,
			Reason:     CacheWriteReasonRefresh,
			DataSource: "products",
			CacheLevel: CacheLevelL2,
			Source:     CacheSourceSubscription,
		},
	}, writes)
}

func TestSubscriptionCacheInvalidateDeletesL2AndCallback(t *testing.T) {
	cache := newSubscriptionCacheLoaderCache()
	cache.entries[`{"__typename":"Product","key":{"upc":"top-4"}}`] = []byte(`{"upc":"top-4","name":"Old"}`)
	var invalidations []subscriptionCacheInvalidationEvent
	resolver := New(context.Background(), ResolverOptions{
		Caches: map[string]LoaderCache{
			"entities": cache,
		},
		OnSubscriptionCacheInvalidate: func(entityType string, keys []string) {
			invalidations = append(invalidations, subscriptionCacheInvalidationEvent{
				EntityType: entityType,
				Keys:       keys,
			})
		},
	})
	writer := newSubscriptionCacheWriter()
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableL2Cache = true

	err := resolver.AsyncResolveGraphQLSubscription(ctx, subscriptionCacheKeyOnlyPlan([]byte(`{"data":{"updateProductPrice":{"upc":"top-4"}}}`), true), writer, SubscriptionIdentifier{
		ConnectionID:   1,
		SubscriptionID: 1,
	})
	require.NoError(t, err)

	message := writer.AwaitFlush(t)

	assert.Equal(t, `{"data":{"updateProductPrice":{"upc":"top-4"}}}`, message)
	assert.Equal(t, map[string]string{}, cache.Snapshot())
	assert.Equal(t, []subscriptionCacheLogEntry{
		{
			Operation: "delete",
			Items: []subscriptionCacheLogItem{
				{
					Key: `{"__typename":"Product","key":{"upc":"top-4"}}`,
				},
			},
		},
	}, cache.GetLog())
	assert.Equal(t, []subscriptionCacheInvalidationEvent{
		{
			EntityType: "Product",
			Keys: []string{
				`{"__typename":"Product","key":{"upc":"top-4"}}`,
			},
		},
	}, invalidations)
}

func TestSubscriptionCacheEmptyFieldNameConfigNoop(t *testing.T) {
	cache := newSubscriptionCacheLoaderCache()
	resolver := New(context.Background(), ResolverOptions{
		Caches: map[string]LoaderCache{
			"entities": cache,
		},
	})
	writer := newSubscriptionCacheWriter()
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	plan := subscriptionCachePlan([]byte(`{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":1}}}`), true)
	plan.EntityCachePopulation = nil

	err := resolver.AsyncResolveGraphQLSubscription(ctx, plan, writer, SubscriptionIdentifier{
		ConnectionID:   1,
		SubscriptionID: 1,
	})
	require.NoError(t, err)

	message := writer.AwaitFlush(t)

	assert.Equal(t, `{"data":{"updateProductPrice":{"upc":"top-4","name":"Bowler","price":1}}}`, message)
	assert.Equal(t, map[string]string{}, cache.Snapshot())
	assert.Equal(t, []subscriptionCacheLogEntry{}, cache.GetLog())
}

type subscriptionCacheInvalidationEvent struct {
	EntityType string
	Keys       []string
}

func subscriptionCachePlan(event []byte, enableInvalidationOnKeyOnly bool) *GraphQLSubscription {
	return &GraphQLSubscription{
		Trigger: GraphQLSubscriptionTrigger{
			Source: subscriptionCacheStream{
				data: event,
			},
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{
					{
						SegmentType: StaticSegmentType,
						Data:        []byte(`{}`),
					},
				},
			},
			PostProcessing: PostProcessingConfiguration{
				SelectResponseDataPath:   []string{"data"},
				SelectResponseErrorsPath: []string{"errors"},
			},
			SourceName: "products",
		},
		Response: &GraphQLResponse{
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("updateProductPrice"),
						Value: &Object{
							Path: []string{"updateProductPrice"},
							Fields: []*Field{
								{
									Name: []byte("upc"),
									Value: &String{
										Path: []string{"upc"},
									},
								},
								{
									Name: []byte("name"),
									Value: &String{
										Path: []string{"name"},
									},
								},
								{
									Name: []byte("price"),
									Value: &Integer{
										Path: []string{"price"},
									},
								},
							},
						},
					},
				},
			},
		},
		EntityCachePopulation: &SubscriptionEntityCachePopulation{
			Mode:                        SubscriptionCacheModePopulate,
			CacheKeyTemplate:            subscriptionCacheProductKeyTemplate(),
			CacheName:                   "entities",
			TTL:                         30 * time.Second,
			DataSourceName:              "products",
			SubscriptionFieldName:       "updateProductPrice",
			EntityTypeName:              "Product",
			EnableInvalidationOnKeyOnly: enableInvalidationOnKeyOnly,
		},
	}
}

func subscriptionCacheKeyOnlyPlan(event []byte, enableInvalidationOnKeyOnly bool) *GraphQLSubscription {
	plan := subscriptionCachePlan(event, enableInvalidationOnKeyOnly)
	rootField := plan.Response.Data.Fields[0]
	entity := rootField.Value.(*Object)
	entity.Fields = entity.Fields[:1]
	return plan
}

func subscriptionCacheProductKeyTemplate() *EntityQueryCacheKeyTemplate {
	return &EntityQueryCacheKeyTemplate{
		TypeName: "Product",
		Keys: NewResolvableObjectVariable(&Object{
			Fields: []*Field{
				{
					Name: []byte("upc"),
					Value: &String{
						Path: []string{"upc"},
					},
				},
			},
		}),
	}
}
