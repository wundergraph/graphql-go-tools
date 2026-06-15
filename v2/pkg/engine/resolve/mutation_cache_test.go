package resolve

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

type mutationCacheLogEntry struct {
	Operation string
	Items     []mutationCacheLogItem
}

type mutationCacheLogItem struct {
	Key   string
	Value string
	TTL   time.Duration
}

type mutationCacheTestHeadersBuilder struct{}

func (mutationCacheTestHeadersBuilder) HeadersForSubgraph(subgraphName string) (http.Header, uint64) {
	if subgraphName == "users" {
		return nil, 99887766
	}
	return nil, 0
}

func (mutationCacheTestHeadersBuilder) HashAll() uint64 {
	return 99887766
}

type mutationLoggingLoaderCache struct {
	mu      sync.Mutex
	entries map[string][]byte
	log     []mutationCacheLogEntry
}

func newMutationLoggingLoaderCache() *mutationLoggingLoaderCache {
	return &mutationLoggingLoaderCache{
		entries: map[string][]byte{},
	}
}

func (c *mutationLoggingLoaderCache) Get(_ context.Context, keys []string) ([]*CacheEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	items := make([]mutationCacheLogItem, 0, len(keys))
	entries := make([]*CacheEntry, len(keys))
	for i, key := range keys {
		items = append(items, mutationCacheLogItem{Key: key})
		value, ok := c.entries[key]
		if !ok {
			continue
		}
		entries[i] = &CacheEntry{
			Key:   key,
			Value: append([]byte(nil), value...),
		}
	}
	c.log = append(c.log, mutationCacheLogEntry{
		Operation: "get",
		Items:     items,
	})
	return entries, nil
}

func (c *mutationLoggingLoaderCache) Set(_ context.Context, entries []*CacheEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	items := make([]mutationCacheLogItem, 0, len(entries))
	for _, entry := range entries {
		c.entries[entry.Key] = append([]byte(nil), entry.Value...)
		items = append(items, mutationCacheLogItem{
			Key:   entry.Key,
			Value: string(entry.Value),
			TTL:   entry.TTL,
		})
	}
	c.log = append(c.log, mutationCacheLogEntry{
		Operation: "set",
		Items:     items,
	})
	return nil
}

func (c *mutationLoggingLoaderCache) Delete(_ context.Context, keys []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	items := make([]mutationCacheLogItem, 0, len(keys))
	for _, key := range keys {
		delete(c.entries, key)
		items = append(items, mutationCacheLogItem{Key: key})
	}
	c.log = append(c.log, mutationCacheLogEntry{
		Operation: "delete",
		Items:     items,
	})
	return nil
}

func (c *mutationLoggingLoaderCache) Seed(key string, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = []byte(value)
}

func (c *mutationLoggingLoaderCache) Snapshot() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()

	snapshot := make(map[string]string, len(c.entries))
	for key, value := range c.entries {
		snapshot[key] = string(value)
	}
	return snapshot
}

func (c *mutationLoggingLoaderCache) GetLog() []mutationCacheLogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()

	log := make([]mutationCacheLogEntry, len(c.log))
	copy(log, c.log)
	return log
}

func (c *mutationLoggingLoaderCache) ClearLog() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.log = nil
}

func TestMutationNavigateProvidesDataToField(t *testing.T) {
	providesData := &Object{
		Fields: []*Field{
			{
				Name:         []byte("renameUser"),
				OriginalName: []byte("updateUsername"),
				Value: &Object{
					Fields: []*Field{
						{
							Name:  []byte("id"),
							Value: &String{},
						},
					},
				},
			},
		},
	}

	assert.Equal(t, providesData.Fields[0].Value, navigateProvidesDataToField(providesData, "updateUsername"))
	assert.Nil(t, navigateProvidesDataToField(providesData, "deleteUsername"))
}

func TestMutationBuildEntityKeyValue(t *testing.T) {
	a := acquireCacheKeyTestArena(t)
	entity := mustParseMutationJSON(t, `{"id":"1234","org":{"slug":"acme"},"name":"Ada"}`)

	key, ok := buildEntityKeyValue(a, entity, []KeyField{
		{
			Name: "id",
		},
		{
			Name: "org",
			Children: []KeyField{
				{
					Name: "slug",
				},
			},
		},
	})

	require.True(t, ok)
	assert.Equal(t, `{"id":"1234","org":{"slug":"acme"}}`, string(key.MarshalTo(nil)))
}

func TestMutationBuildEntityKeyValueMissingField(t *testing.T) {
	a := acquireCacheKeyTestArena(t)
	entity := mustParseMutationJSON(t, `{"name":"Ada"}`)

	key, ok := buildEntityKeyValue(a, entity, []KeyField{
		{
			Name: "id",
		},
	})

	assert.False(t, ok)
	assert.Nil(t, key)
}

func TestMutationBuildEntityCacheKeyAppliesTransformPipeline(t *testing.T) {
	cache := &FetchCacheConfiguration{
		CacheName: "entities",
		MutationEntityImpactConfig: &MutationEntityImpactConfig{
			EntityTypeName:              "User",
			KeyFields:                   []KeyField{{Name: "id"}},
			CacheName:                   "entities",
			IncludeSubgraphHeaderPrefix: true,
		},
	}
	loader := &Loader{
		ctx:       NewContext(context.Background()),
		jsonArena: acquireCacheKeyTestArena(t),
	}
	loader.ctx.ExecutionOptions.Caching.GlobalCacheKeyPrefix = "global:"
	loader.ctx.ExecutionOptions.Caching.L2CacheKeyInterceptor = func(info L2CacheKeyInterceptorInfo, key string) string {
		assert.Equal(t, L2CacheKeyInterceptorInfo{
			SubgraphName: "users",
			CacheName:    "entities",
		}, info)
		return "tenant-42:" + key
	}
	loader.ctx.SubgraphHeadersBuilder = mutationCacheTestHeadersBuilder{}

	key, ok := buildMutationEntityCacheKey(loader, cache, mustParseMutationJSON(t, `{"id":"1234"}`), "users")

	require.True(t, ok)
	assert.Equal(t, `tenant-42:99887766:global:{"__typename":"User","key":{"id":"1234"}}`, key)
}

func TestMutationDetectEntityImpactDeletesImpactedKeysAndRecordsAnalytics(t *testing.T) {
	cache := newMutationLoggingLoaderCache()
	cache.Seed(`{"__typename":"User","key":{"id":"1234"}}`, `{"id":"1234","username":"OldMe"}`)
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
	loader := &Loader{
		ctx: ctx,
		info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeMutation,
		},
		caches: map[string]LoaderCache{
			"entities": cache,
		},
		jsonArena: acquireCacheKeyTestArena(t),
	}
	fetchCache := mutationImpactFetchCache(true, false, 0)
	responseData := mustParseMutationJSON(t, `{"updateUsername":{"id":"1234","username":"NewMe"}}`)

	deletedKeys := loader.detectMutationEntityImpact(fetchCache, responseData, "users", nil)

	assert.Equal(t, map[string]struct{}{
		`{"__typename":"User","key":{"id":"1234"}}`: {},
	}, deletedKeys)
	assert.Equal(t, []mutationCacheLogEntry{
		{
			Operation: "delete",
			Items: []mutationCacheLogItem{
				{
					Key: `{"__typename":"User","key":{"id":"1234"}}`,
				},
			},
		},
	}, cache.GetLog())
	assert.Equal(t, map[string]string{}, cache.Snapshot())
	assert.Equal(t, []MutationEvent{
		{
			EntityType: "User",
			Operation:  "updateUsername",
			Key:        `{"__typename":"User","key":{"id":"1234"}}`,
			Deleted:    true,
		},
	}, ctx.GetCacheStats().MutationEvents)
}

func TestMutationDetectEntityImpactPopulatesWithTTLOverride(t *testing.T) {
	cache := newMutationLoggingLoaderCache()
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
	loader := &Loader{
		ctx: ctx,
		info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeMutation,
		},
		caches: map[string]LoaderCache{
			"entities": cache,
		},
		jsonArena: acquireCacheKeyTestArena(t),
	}
	fetchCache := mutationImpactFetchCache(false, true, 60*time.Second)
	responseData := mustParseMutationJSON(t, `{"updateUsername":{"id":"u-pop","username":"PopMe","ignored":"raw-only"}}`)

	deletedKeys := loader.detectMutationEntityImpact(fetchCache, responseData, "users", nil)

	assert.Equal(t, map[string]struct{}{}, deletedKeys)
	assert.Equal(t, []mutationCacheLogEntry{
		{
			Operation: "set",
			Items: []mutationCacheLogItem{
				{
					Key:   `{"__typename":"User","key":{"id":"u-pop"}}`,
					Value: `{"id":"u-pop","username":"PopMe"}`,
					TTL:   60 * time.Second,
				},
			},
		},
	}, cache.GetLog())
	assert.Equal(t, []MutationEvent{
		{
			EntityType: "User",
			Operation:  "updateUsername",
			Key:        `{"__typename":"User","key":{"id":"u-pop"}}`,
			Written:    true,
		},
	}, ctx.GetCacheStats().MutationEvents)
}

func TestMutationLoaderSkipsL2ReadAndAlwaysFetches(t *testing.T) {
	cache := newMutationLoggingLoaderCache()
	cache.Seed(`{"__typename":"User","key":{"id":"u1"}}`, `{"id":"u1","name":"Cached"}`)
	userEntities := &mutationCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"_entities":[{"__typename":"User","id":"u1","name":"Fresh"}]}}`),
		},
	}
	response := mutationHydrationResponse(
		&mutationCacheTestDataSource{
			responses: [][]byte{
				[]byte(`{"data":{"updateUser":{"__typename":"User","id":"u1"}}}`),
			},
		},
		userEntities,
		mutationEntityFetchCache(false, 0),
	)
	out := resolveMutationCacheTestGraphQLResponse(t, response, ResolverOptions{
		Caches: map[string]LoaderCache{
			"entities": cache,
		},
	}, func(ctx *Context) {
		ctx.ExecutionOptions.Caching.EnableL2Cache = true
	})

	assert.Equal(t, `{"data":{"updateUser":{"id":"u1","name":"Fresh"}}}`, out)
	assert.Equal(t, 1, userEntities.CallCount())
	assert.Equal(t, []mutationCacheLogEntry{}, cache.GetLog())
}

func TestMutationLoaderWritesL2OnlyWhenPopulationEnabledWithTTLOverride(t *testing.T) {
	cache := newMutationLoggingLoaderCache()
	userEntities := &mutationCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"_entities":[{"__typename":"User","id":"u1","name":"Fresh"}]}}`),
		},
	}
	response := mutationHydrationResponse(
		&mutationCacheTestDataSource{
			responses: [][]byte{
				[]byte(`{"data":{"updateUser":{"__typename":"User","id":"u1"}}}`),
			},
		},
		userEntities,
		mutationEntityFetchCache(true, 60*time.Second),
	)
	out := resolveMutationCacheTestGraphQLResponse(t, response, ResolverOptions{
		Caches: map[string]LoaderCache{
			"entities": cache,
		},
	}, func(ctx *Context) {
		ctx.ExecutionOptions.Caching.EnableL2Cache = true
	})

	assert.Equal(t, `{"data":{"updateUser":{"id":"u1","name":"Fresh"}}}`, out)
	assert.Equal(t, []mutationCacheLogEntry{
		{
			Operation: "set",
			Items: []mutationCacheLogItem{
				{
					Key:   `{"__typename":"User","key":{"id":"u1"}}`,
					Value: `{"id":"u1","name":"Fresh"}`,
					TTL:   60 * time.Second,
				},
			},
		},
	}, cache.GetLog())
}

func TestMutationLoaderSkipsL2WriteWhenPopulationDisabled(t *testing.T) {
	cache := newMutationLoggingLoaderCache()
	userEntities := &mutationCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"_entities":[{"__typename":"User","id":"u1","name":"Fresh"}]}}`),
		},
	}
	response := mutationHydrationResponse(
		&mutationCacheTestDataSource{
			responses: [][]byte{
				[]byte(`{"data":{"updateUser":{"__typename":"User","id":"u1"}}}`),
			},
		},
		userEntities,
		mutationEntityFetchCache(false, 60*time.Second),
	)
	out := resolveMutationCacheTestGraphQLResponse(t, response, ResolverOptions{
		Caches: map[string]LoaderCache{
			"entities": cache,
		},
	}, func(ctx *Context) {
		ctx.ExecutionOptions.Caching.EnableL2Cache = true
	})

	assert.Equal(t, `{"data":{"updateUser":{"id":"u1","name":"Fresh"}}}`, out)
	assert.Equal(t, 1, userEntities.CallCount())
	assert.Equal(t, []mutationCacheLogEntry{}, cache.GetLog())
}

func TestMutationLoaderDeletesImpactedKeysAfterSuccessfulMutation(t *testing.T) {
	cache := newMutationLoggingLoaderCache()
	cache.Seed(`{"__typename":"User","key":{"id":"1234"}}`, `{"id":"1234","username":"OldMe"}`)
	response := directMutationResponse(
		&mutationCacheTestDataSource{
			responses: [][]byte{
				[]byte(`{"data":{"updateUsername":{"id":"1234","username":"NewMe"}}}`),
			},
		},
		mutationImpactFetchCache(true, false, 0),
	)
	var testCtx *Context
	out := resolveMutationCacheTestGraphQLResponse(t, response, ResolverOptions{
		Caches: map[string]LoaderCache{
			"entities": cache,
		},
	}, func(ctx *Context) {
		ctx.ExecutionOptions.Caching.EnableL2Cache = true
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
		testCtx = ctx
	})
	stats := testCtx.GetCacheStats()

	assert.Equal(t, `{"data":{"updateUsername":{"id":"1234","username":"NewMe"}}}`, out)
	assert.Equal(t, []mutationCacheLogEntry{
		{
			Operation: "delete",
			Items: []mutationCacheLogItem{
				{
					Key: `{"__typename":"User","key":{"id":"1234"}}`,
				},
			},
		},
	}, cache.GetLog())
	assert.Equal(t, map[string]string{}, cache.Snapshot())
	assert.Equal(t, []MutationEvent{
		{
			EntityType: "User",
			Operation:  "updateUsername",
			Key:        `{"__typename":"User","key":{"id":"1234"}}`,
			Deleted:    true,
		},
	}, stats.MutationEvents)
}

func TestMutationLoaderSkipsDeleteWhenWrittenInSameFetch(t *testing.T) {
	cache := newMutationLoggingLoaderCache()
	response := directMutationResponse(
		&mutationCacheTestDataSource{
			responses: [][]byte{
				[]byte(`{"data":{"updateUsername":{"id":"1234","username":"NewMe"}}}`),
			},
		},
		mutationImpactFetchCache(true, true, 90*time.Second),
	)
	var testCtx *Context
	out := resolveMutationCacheTestGraphQLResponse(t, response, ResolverOptions{
		Caches: map[string]LoaderCache{
			"entities": cache,
		},
	}, func(ctx *Context) {
		ctx.ExecutionOptions.Caching.EnableL2Cache = true
		ctx.ExecutionOptions.Caching.EnableCacheAnalytics = true
		testCtx = ctx
	})
	stats := testCtx.GetCacheStats()

	assert.Equal(t, `{"data":{"updateUsername":{"id":"1234","username":"NewMe"}}}`, out)
	assert.Equal(t, []mutationCacheLogEntry{
		{
			Operation: "set",
			Items: []mutationCacheLogItem{
				{
					Key:   `{"__typename":"User","key":{"id":"1234"}}`,
					Value: `{"id":"1234","username":"NewMe"}`,
					TTL:   90 * time.Second,
				},
			},
		},
	}, cache.GetLog())
	assert.Equal(t, map[string]string{
		`{"__typename":"User","key":{"id":"1234"}}`: `{"id":"1234","username":"NewMe"}`,
	}, cache.Snapshot())
	assert.Equal(t, []MutationEvent{
		{
			EntityType: "User",
			Operation:  "updateUsername",
			Key:        `{"__typename":"User","key":{"id":"1234"}}`,
			Written:    true,
		},
	}, stats.MutationEvents)
}

func mutationImpactFetchCache(invalidate bool, populate bool, ttl time.Duration) *FetchCacheConfiguration {
	return &FetchCacheConfiguration{
		CacheName: "entities",
		ProvidesData: &Object{
			Fields: []*Field{
				{
					Name: []byte("updateUsername"),
					Value: &Object{
						Fields: []*Field{
							{
								Name:  []byte("id"),
								Value: &String{},
							},
							{
								Name:  []byte("username"),
								Value: &String{},
							},
						},
					},
				},
			},
		},
		MutationEntityImpactConfig: &MutationEntityImpactConfig{
			EntityTypeName:  "User",
			KeyFields:       []KeyField{{Name: "id"}},
			CacheName:       "entities",
			InvalidateCache: invalidate,
			PopulateCache:   populate,
			PopulateTTL:     ttl,
		},
	}
}

func mutationEntityFetchCache(populate bool, ttl time.Duration) *FetchCacheConfiguration {
	return &FetchCacheConfiguration{
		CacheName:                       "entities",
		EnableL2Cache:                   true,
		TTL:                             300 * time.Second,
		EnableMutationL2CachePopulation: populate,
		MutationCacheTTLOverride:        ttl,
		KeyTemplate:                     cacheTestUserKeyTemplate(),
		ProvidesData:                    cacheTestUserProvides(),
		MutationEntityImpactConfig:      nil,
		IncludeSubgraphHeaderPrefix:     false,
		EnablePartialCacheLoad:          false,
		UseL1Cache:                      false,
		NegativeCacheTTL:                0,
	}
}

func mutationHydrationResponse(rootSource DataSource, entitySource DataSource, entityCache *FetchCacheConfiguration) *GraphQLResponse {
	return &GraphQLResponse{
		Fetches: Sequence(
			Single(&SingleFetch{
				InputTemplate: cacheTestStaticInput(`{"method":"POST","url":"http://users","body":{"query":"mutation{updateUser{id}}"}}`),
				FetchConfiguration: FetchConfiguration{
					DataSource: rootSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				Info: &FetchInfo{
					DataSourceID:   "users",
					DataSourceName: "users",
					OperationType:  ast.OperationTypeMutation,
				},
			}),
			SingleWithPath(mutationEntityFetch(entitySource, entityCache), "mutation.updateUser", ObjectPath("updateUser")),
		),
		Info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeMutation,
		},
		Data: mutationUserResponseObject("updateUser", "name"),
	}
}

func mutationEntityFetch(source DataSource, cache *FetchCacheConfiguration) *EntityFetch {
	fetch := cacheTestEntityFetch(source, cache)
	fetch.Info.OperationType = ast.OperationTypeQuery
	return fetch
}

func directMutationResponse(source DataSource, cache *FetchCacheConfiguration) *GraphQLResponse {
	return &GraphQLResponse{
		Fetches: Single(&SingleFetch{
			InputTemplate: cacheTestStaticInput(`{"method":"POST","url":"http://users","body":{"query":"mutation{updateUsername{id username}}"}}`),
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
				OperationType:  ast.OperationTypeMutation,
			},
		}),
		Info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeMutation,
		},
		Data: mutationUserResponseObject("updateUsername", "username"),
	}
}

func mutationUserResponseObject(rootField string, secondField string) *Object {
	return &Object{
		Fields: []*Field{
			{
				Name: []byte(rootField),
				Value: &Object{
					Path: []string{rootField},
					Fields: []*Field{
						{
							Name:  []byte("id"),
							Value: &String{Path: []string{"id"}},
						},
						{
							Name:  []byte(secondField),
							Value: &String{Path: []string{secondField}},
						},
					},
				},
			},
		},
	}
}

func cacheTestUserProvides() *Object {
	return &Object{
		Fields: []*Field{
			{
				Name:  []byte("id"),
				Value: &String{},
			},
			{
				Name:  []byte("name"),
				Value: &String{},
			},
		},
	}
}

func resolveMutationCacheTestGraphQLResponse(t *testing.T, response *GraphQLResponse, options ResolverOptions, configure func(ctx *Context)) string {
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

func mustParseMutationJSON(t *testing.T, data string) *astjson.Value {
	t.Helper()

	value, err := astjson.ParseBytes([]byte(data))
	require.NoError(t, err)
	return value
}

type mutationCacheTestDataSource struct {
	mu        sync.Mutex
	responses [][]byte
	calls     int
}

func (d *mutationCacheTestDataSource) Load(_ context.Context, _ http.Header, _ []byte) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	index := d.calls
	if index >= len(d.responses) {
		index = len(d.responses) - 1
	}
	d.calls++
	return append([]byte(nil), d.responses[index]...), nil
}

func (d *mutationCacheTestDataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, _ []*httpclient.FileUpload) ([]byte, error) {
	return d.Load(ctx, headers, input)
}

func (d *mutationCacheTestDataSource) CallCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.calls
}
