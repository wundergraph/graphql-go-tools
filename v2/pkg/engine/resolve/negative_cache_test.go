package resolve

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func TestNegativeCacheStoreThenServeKnownAbsentEntity(t *testing.T) {
	users := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"users":[{"__typename":"User","id":"1"}]}}`),
		},
	}
	entities := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"_entities":[null]}}`),
		},
	}
	cache := newTTLNegativeCacheTestLoaderCache()
	response := negativeCacheTestBatchEntityResponse(users, entities, negativeCacheTestUserConfig(time.Minute))
	options := ResolverOptions{
		Caches: map[string]LoaderCache{
			"default": cache,
		},
	}

	out1 := resolveCacheTestGraphQLResponse(t, response, options, enableL2Cache)
	out2 := resolveCacheTestGraphQLResponse(t, response, options, enableL2Cache)

	assert.Equal(t, `{"data":{"users":[null]}}`, out1)
	assert.Equal(t, `{"data":{"users":[null]}}`, out2)
	assert.Equal(t, 2, users.CallCount())
	assert.Equal(t, 1, entities.CallCount())
	assert.Equal(t, map[string]negativeCacheTestEntrySnapshot{
		`{"__typename":"User","key":{"id":"1"}}`: {
			Value:       `null`,
			TTL:         time.Minute,
			WriteReason: CacheWriteReasonRefresh,
		},
	}, cache.Snapshot())
}

func TestNegativeCacheTTLExpiryRefetchesKnownAbsentEntity(t *testing.T) {
	users := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"users":[{"__typename":"User","id":"1"}]}}`),
		},
	}
	entities := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"_entities":[null]}}`),
			[]byte(`{"data":{"_entities":[null]}}`),
		},
	}
	cache := newTTLNegativeCacheTestLoaderCache()
	response := negativeCacheTestBatchEntityResponse(users, entities, negativeCacheTestUserConfig(10*time.Millisecond))
	options := ResolverOptions{
		Caches: map[string]LoaderCache{
			"default": cache,
		},
	}

	out1 := resolveCacheTestGraphQLResponse(t, response, options, enableL2Cache)
	time.Sleep(20 * time.Millisecond)
	out2 := resolveCacheTestGraphQLResponse(t, response, options, enableL2Cache)

	assert.Equal(t, `{"data":{"users":[null]}}`, out1)
	assert.Equal(t, `{"data":{"users":[null]}}`, out2)
	assert.Equal(t, 2, users.CallCount())
	assert.Equal(t, 2, entities.CallCount())
	assert.Equal(t, map[string]negativeCacheTestEntrySnapshot{
		`{"__typename":"User","key":{"id":"1"}}`: {
			Value:       `null`,
			TTL:         10 * time.Millisecond,
			WriteReason: CacheWriteReasonRefresh,
		},
	}, cache.Snapshot())
}

func TestNegativeCacheOverwriteAfterExpiryStoresRealEntity(t *testing.T) {
	users := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"users":[{"__typename":"User","id":"1"}]}}`),
		},
	}
	entities := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"_entities":[null]}}`),
			[]byte(`{"data":{"_entities":[{"__typename":"User","id":"1","name":"Ada"}]}}`),
		},
	}
	cache := newTTLNegativeCacheTestLoaderCache()
	response := negativeCacheTestBatchEntityResponse(users, entities, negativeCacheTestUserConfig(10*time.Millisecond))
	options := ResolverOptions{
		Caches: map[string]LoaderCache{
			"default": cache,
		},
	}

	out1 := resolveCacheTestGraphQLResponse(t, response, options, enableL2Cache)
	time.Sleep(20 * time.Millisecond)
	out2 := resolveCacheTestGraphQLResponse(t, response, options, enableL2Cache)
	out3 := resolveCacheTestGraphQLResponse(t, response, options, enableL2Cache)

	assert.Equal(t, `{"data":{"users":[null]}}`, out1)
	assert.Equal(t, `{"data":{"users":[{"id":"1","name":"Ada"}]}}`, out2)
	assert.Equal(t, `{"data":{"users":[{"id":"1","name":"Ada"}]}}`, out3)
	assert.Equal(t, 3, users.CallCount())
	assert.Equal(t, 2, entities.CallCount())
	assert.Equal(t, map[string]negativeCacheTestEntrySnapshot{
		`{"__typename":"User","key":{"id":"1"}}`: {
			Value:       `{"__typename":"User","id":"1","name":"Ada"}`,
			TTL:         time.Hour,
			WriteReason: CacheWriteReasonRefresh,
		},
	}, cache.Snapshot())
}

func TestNegativeCacheNullableFieldDoesNotStoreSentinel(t *testing.T) {
	users := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"users":[{"__typename":"User","id":"1"}]}}`),
		},
	}
	entities := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"_entities":[{"__typename":"User","id":"1","name":null}]}}`),
		},
	}
	cache := newTTLNegativeCacheTestLoaderCache()
	response := negativeCacheTestBatchEntityResponse(users, entities, negativeCacheTestUserConfig(time.Minute))
	options := ResolverOptions{
		Caches: map[string]LoaderCache{
			"default": cache,
		},
	}

	out1 := resolveCacheTestGraphQLResponse(t, response, options, enableL2Cache)
	out2 := resolveCacheTestGraphQLResponse(t, response, options, enableL2Cache)

	assert.Equal(t, `{"data":{"users":[{"id":"1","name":null}]}}`, out1)
	assert.Equal(t, `{"data":{"users":[{"id":"1","name":null}]}}`, out2)
	assert.Equal(t, 2, users.CallCount())
	assert.Equal(t, 1, entities.CallCount())
	assert.Equal(t, map[string]negativeCacheTestEntrySnapshot{
		`{"__typename":"User","key":{"id":"1"}}`: {
			Value:       `{"__typename":"User","id":"1","name":null}`,
			TTL:         time.Hour,
			WriteReason: CacheWriteReasonRefresh,
		},
	}, cache.Snapshot())
}

type negativeCacheTestEntrySnapshot struct {
	Value       string
	TTL         time.Duration
	WriteReason CacheWriteReason
}

type ttlNegativeCacheTestLoaderCache struct {
	mu      sync.Mutex
	entries map[string]ttlNegativeCacheTestEntry
	now     func() time.Time
}

type ttlNegativeCacheTestEntry struct {
	value       []byte
	ttl         time.Duration
	writeReason CacheWriteReason
	expiresAt   time.Time
}

func newTTLNegativeCacheTestLoaderCache() *ttlNegativeCacheTestLoaderCache {
	return &ttlNegativeCacheTestLoaderCache{
		entries: map[string]ttlNegativeCacheTestEntry{},
		now:     time.Now,
	}
}

func (c *ttlNegativeCacheTestLoaderCache) Get(_ context.Context, keys []string) ([]*CacheEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entries := make([]*CacheEntry, len(keys))
	now := c.now()
	for i, key := range keys {
		entry, ok := c.entries[key]
		if !ok {
			continue
		}
		if !entry.expiresAt.IsZero() && !entry.expiresAt.After(now) {
			delete(c.entries, key)
			continue
		}
		entries[i] = &CacheEntry{
			Key:         key,
			Value:       append([]byte(nil), entry.value...),
			TTL:         entry.ttl,
			WriteReason: entry.writeReason,
		}
	}
	return entries, nil
}

func (c *ttlNegativeCacheTestLoaderCache) Set(_ context.Context, entries []*CacheEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	for _, entry := range entries {
		expiresAt := time.Time{}
		if entry.TTL > 0 {
			expiresAt = now.Add(entry.TTL)
		}
		c.entries[entry.Key] = ttlNegativeCacheTestEntry{
			value:       append([]byte(nil), entry.Value...),
			ttl:         entry.TTL,
			writeReason: entry.WriteReason,
			expiresAt:   expiresAt,
		}
	}
	return nil
}

func (c *ttlNegativeCacheTestLoaderCache) Delete(_ context.Context, keys []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, key := range keys {
		delete(c.entries, key)
	}
	return nil
}

func (c *ttlNegativeCacheTestLoaderCache) Snapshot() map[string]negativeCacheTestEntrySnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	snapshot := make(map[string]negativeCacheTestEntrySnapshot, len(c.entries))
	for key, entry := range c.entries {
		if !entry.expiresAt.IsZero() && !entry.expiresAt.After(now) {
			continue
		}
		snapshot[key] = negativeCacheTestEntrySnapshot{
			Value:       string(entry.value),
			TTL:         entry.ttl,
			WriteReason: entry.writeReason,
		}
	}
	return snapshot
}

func negativeCacheTestUserConfig(negativeTTL time.Duration) *FetchCacheConfiguration {
	config := userNameCacheConfig(false)
	config.CacheName = "default"
	config.EnableL2Cache = true
	config.TTL = time.Hour
	config.NegativeCacheTTL = negativeTTL
	return config
}

func negativeCacheTestBatchEntityResponse(rootSource DataSource, entitySource DataSource, cache *FetchCacheConfiguration) *GraphQLResponse {
	return &GraphQLResponse{
		Fetches: Sequence(
			Single(&SingleFetch{
				InputTemplate: cacheTestStaticInput(`{"method":"POST","url":"http://users","body":{"query":"query{users{__typename id}}"}}`),
				FetchConfiguration: FetchConfiguration{
					DataSource: rootSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
			}),
			SingleWithPath(cacheTestBatchEntityFetch(entitySource, cache), "query.users", ArrayPath("users")),
		),
		Info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
		},
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("users"),
					Value: &Array{
						Path:     []string{"users"},
						Nullable: true,
						Item: &Object{
							Nullable: true,
							Fields: []*Field{
								{
									Name:  []byte("id"),
									Value: &String{Path: []string{"id"}},
								},
								{
									Name:  []byte("name"),
									Value: &String{Path: []string{"name"}, Nullable: true},
								},
							},
						},
					},
				},
			},
		},
	}
}
