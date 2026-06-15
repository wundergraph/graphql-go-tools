package resolve

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

type extensionCacheLogEntry struct {
	Operation string
	Items     []extensionCacheLogItem
}

type extensionCacheLogItem struct {
	Key   string
	Value string
	TTL   time.Duration
}

type extensionLoggingLoaderCache struct {
	mu      sync.Mutex
	entries map[string][]byte
	log     []extensionCacheLogEntry
}

func newExtensionLoggingLoaderCache() *extensionLoggingLoaderCache {
	return &extensionLoggingLoaderCache{
		entries: map[string][]byte{},
	}
}

func (c *extensionLoggingLoaderCache) Get(_ context.Context, keys []string) ([]*CacheEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	items := make([]extensionCacheLogItem, 0, len(keys))
	entries := make([]*CacheEntry, len(keys))
	for i, key := range keys {
		items = append(items, extensionCacheLogItem{Key: key})
		value, ok := c.entries[key]
		if !ok {
			continue
		}
		entries[i] = &CacheEntry{
			Key:   key,
			Value: append([]byte(nil), value...),
		}
	}
	c.log = append(c.log, extensionCacheLogEntry{
		Operation: "get",
		Items:     items,
	})
	return entries, nil
}

func (c *extensionLoggingLoaderCache) Set(_ context.Context, entries []*CacheEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	items := make([]extensionCacheLogItem, 0, len(entries))
	for _, entry := range entries {
		c.entries[entry.Key] = append([]byte(nil), entry.Value...)
		items = append(items, extensionCacheLogItem{
			Key:   entry.Key,
			Value: string(entry.Value),
			TTL:   entry.TTL,
		})
	}
	c.log = append(c.log, extensionCacheLogEntry{
		Operation: "set",
		Items:     items,
	})
	return nil
}

func (c *extensionLoggingLoaderCache) Delete(_ context.Context, keys []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	items := make([]extensionCacheLogItem, 0, len(keys))
	for _, key := range keys {
		delete(c.entries, key)
		items = append(items, extensionCacheLogItem{Key: key})
	}
	c.log = append(c.log, extensionCacheLogEntry{
		Operation: "delete",
		Items:     items,
	})
	return nil
}

func (c *extensionLoggingLoaderCache) Log() []extensionCacheLogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()

	log := make([]extensionCacheLogEntry, len(c.log))
	copy(log, c.log)
	return log
}

type extensionCacheTestHeadersBuilder struct{}

func (extensionCacheTestHeadersBuilder) HeadersForSubgraph(subgraphName string) (http.Header, uint64) {
	if subgraphName == "users" {
		return nil, 99887766
	}
	return nil, 0
}

func (extensionCacheTestHeadersBuilder) HashAll() uint64 {
	return 99887766
}

type extensionCacheTestDataSource struct {
	mu       sync.Mutex
	response []byte
	calls    int
}

func (d *extensionCacheTestDataSource) Load(_ context.Context, _ http.Header, _ []byte) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.calls++
	return append([]byte(nil), d.response...), nil
}

func (d *extensionCacheTestDataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, _ []*httpclient.FileUpload) ([]byte, error) {
	return d.Load(ctx, headers, input)
}

func TestExtensionCacheInvalidationDeletesTransformedKeysAndRecordsAnalytics(t *testing.T) {
	cache := newExtensionLoggingLoaderCache()
	response := extensionCacheInvalidationRootResponse(&extensionCacheTestDataSource{
		response: []byte(`{"data":{"viewer":{"id":"1","name":"Ada"}},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":1}},{"typename":"Product","key":{"sku":"A1"}}]}}}`),
	}, nil)
	var testCtx *Context

	out := resolveExtensionCacheInvalidationGraphQLResponse(t, response, ResolverOptions{
		Caches: map[string]LoaderCache{
			"entities": cache,
		},
		EntityCacheConfigs: map[string]map[string]*EntityCacheInvalidationConfig{
			"users": {
				"User": {
					CacheName:                   "entities",
					IncludeSubgraphHeaderPrefix: true,
				},
				"Product": {
					CacheName:                   "entities",
					IncludeSubgraphHeaderPrefix: true,
				},
			},
		},
	}, func(ctx *Context) {
		ctx.ExecutionOptions.Caching.EnableL2Cache = true
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
		ctx.ExecutionOptions.Caching.GlobalCacheKeyPrefix = "global:"
		ctx.ExecutionOptions.Caching.L2CacheKeyInterceptor = func(info L2CacheKeyInterceptorInfo, key string) string {
			assert.Equal(t, L2CacheKeyInterceptorInfo{
				SubgraphName: "users",
				CacheName:    "entities",
			}, info)
			return "tenant-42:" + key
		}
		ctx.SubgraphHeadersBuilder = extensionCacheTestHeadersBuilder{}
		testCtx = ctx
	})
	stats := testCtx.GetCacheStats()

	assert.Equal(t, `{"data":{"viewer":{"id":"1","name":"Ada"}}}`, out)
	assert.Equal(t, []extensionCacheLogEntry{
		{
			Operation: "delete",
			Items: []extensionCacheLogItem{
				{
					Key: `tenant-42:99887766:global:{"__typename":"User","key":{"id":"1"}}`,
				},
				{
					Key: `tenant-42:99887766:global:{"__typename":"Product","key":{"sku":"A1"}}`,
				},
			},
		},
	}, cache.Log())
	assert.Equal(t, []CacheInvalidationEvent{
		{
			EntityType:   "User",
			SubgraphName: "users",
			CacheName:    "entities",
			Key:          `tenant-42:99887766:global:{"__typename":"User","key":{"id":"1"}}`,
			Source:       "extension",
			Deleted:      true,
		},
		{
			EntityType:   "Product",
			SubgraphName: "users",
			CacheName:    "entities",
			Key:          `tenant-42:99887766:global:{"__typename":"Product","key":{"sku":"A1"}}`,
			Source:       "extension",
			Deleted:      true,
		},
	}, stats.CacheInvalidations)
}

func TestExtensionCacheInvalidationSkipsKeyWrittenSameFetch(t *testing.T) {
	cache := newExtensionLoggingLoaderCache()
	response := extensionCacheInvalidationEntityResponse(&extensionCacheTestDataSource{
		response: []byte(`{"data":{"_entities":[{"__typename":"User","id":"1","name":"Ada"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"1"}}]}}}`),
	}, extensionUserCacheConfig())

	out := resolveExtensionCacheInvalidationGraphQLResponse(t, response, ResolverOptions{
		Caches: map[string]LoaderCache{
			"entities": cache,
		},
		EntityCacheConfigs: map[string]map[string]*EntityCacheInvalidationConfig{
			"users": {
				"User": {
					CacheName: "entities",
				},
			},
		},
	}, func(ctx *Context) {
		ctx.ExecutionOptions.Caching.EnableL2Cache = true
	})

	assert.Equal(t, `{"data":{"viewer":{"id":"1","name":"Ada"}}}`, out)
	assert.Equal(t, []extensionCacheLogEntry{
		{
			Operation: "get",
			Items: []extensionCacheLogItem{
				{
					Key: `{"__typename":"User","key":{"id":"1"}}`,
				},
			},
		},
		{
			Operation: "set",
			Items: []extensionCacheLogItem{
				{
					Key:   `{"__typename":"User","key":{"id":"1"}}`,
					Value: `{"id":"1","name":"Ada"}`,
					TTL:   90 * time.Second,
				},
			},
		},
	}, cache.Log())
}

func TestExtensionCacheInvalidationNoExtensionDoesNothing(t *testing.T) {
	cache := newExtensionLoggingLoaderCache()
	response := extensionCacheInvalidationRootResponse(&extensionCacheTestDataSource{
		response: []byte(`{"data":{"viewer":{"id":"1","name":"Ada"}}}`),
	}, nil)

	out := resolveExtensionCacheInvalidationGraphQLResponse(t, response, ResolverOptions{
		Caches: map[string]LoaderCache{
			"entities": cache,
		},
		EntityCacheConfigs: map[string]map[string]*EntityCacheInvalidationConfig{
			"users": {
				"User": {
					CacheName: "entities",
				},
			},
		},
	}, func(ctx *Context) {
		ctx.ExecutionOptions.Caching.EnableL2Cache = true
	})

	assert.Equal(t, `{"data":{"viewer":{"id":"1","name":"Ada"}}}`, out)
	assert.Equal(t, []extensionCacheLogEntry{}, cache.Log())
}

func extensionCacheInvalidationRootResponse(source DataSource, cache *FetchCacheConfiguration) *GraphQLResponse {
	return &GraphQLResponse{
		Fetches: Single(&SingleFetch{
			InputTemplate: cacheTestStaticInput(`{"method":"POST","url":"http://users","body":{"query":"query{viewer{id name}}"}}`),
			FetchConfiguration: FetchConfiguration{
				DataSource: source,
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data"},
				},
			},
			Cache: cache,
			Info: &FetchInfo{
				DataSourceID:   "users",
				DataSourceName: "users",
				OperationType:  ast.OperationTypeQuery,
			},
		}),
		Info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
		},
		Data: extensionViewerResponseObject(),
	}
}

func extensionCacheInvalidationEntityResponse(source DataSource, cache *FetchCacheConfiguration) *GraphQLResponse {
	return &GraphQLResponse{
		Fetches: Sequence(
			Single(&SingleFetch{
				InputTemplate: cacheTestStaticInput(`{"method":"POST","url":"http://users","body":{"query":"query{viewer{id}}"}}`),
				FetchConfiguration: FetchConfiguration{
					DataSource: &extensionCacheTestDataSource{
						response: []byte(`{"data":{"viewer":{"__typename":"User","id":"1"}}}`),
					},
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "users",
					DataSourceName: "users",
					OperationType:  ast.OperationTypeQuery,
				},
			}),
			SingleWithPath(cacheTestEntityFetch(source, cache), "query.viewer", ObjectPath("viewer")),
		),
		Info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
		},
		Data: extensionViewerResponseObject(),
	}
}

func extensionViewerResponseObject() *Object {
	return &Object{
		Fields: []*Field{
			{
				Name: []byte("viewer"),
				Value: &Object{
					Path: []string{"viewer"},
					Fields: []*Field{
						{
							Name:  []byte("id"),
							Value: &String{Path: []string{"id"}},
						},
						{
							Name:  []byte("name"),
							Value: &String{Path: []string{"name"}},
						},
					},
				},
			},
		},
	}
}

func extensionUserCacheConfig() *FetchCacheConfiguration {
	return &FetchCacheConfiguration{
		CacheName:     "entities",
		EnableL2Cache: true,
		TTL:           90 * time.Second,
		KeyTemplate:   cacheTestUserKeyTemplate(),
		ProvidesData:  cacheTestUserProvides(),
	}
}

func resolveExtensionCacheInvalidationGraphQLResponse(t *testing.T, response *GraphQLResponse, options ResolverOptions, configure func(ctx *Context)) string {
	t.Helper()

	resolverCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resolver := New(resolverCtx, options)
	ctx := NewContext(context.Background())
	if configure != nil {
		configure(ctx)
	}

	var out bytes.Buffer
	_, err := resolver.ResolveGraphQLResponse(ctx, response, nil, &out)
	assert.NoError(t, err)
	return out.String()
}
