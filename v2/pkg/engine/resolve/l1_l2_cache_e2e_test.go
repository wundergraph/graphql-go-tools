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

func TestLoaderCacheL1MissThenHitWithinRequest(t *testing.T) {
	userEntities := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"_entities":[{"__typename":"User","id":"1","name":"Ada"}]}}`),
		},
	}
	response := cacheTestL1Response(userEntities, userNameCacheConfig(true))
	out := resolveCacheTestGraphQLResponse(t, response, ResolverOptions{}, func(ctx *Context) {
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
	})

	assert.Equal(t, `{"data":{"first":{"id":"1","name":"Ada"},"second":{"id":"1","name":"Ada"}}}`, out)
	assert.Equal(t, 1, userEntities.CallCount())
}

func TestLoaderCacheL2RootMissThenHitAcrossRequests(t *testing.T) {
	root := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"user":{"id":"1","name":"Ada"}}}`),
		},
	}
	cache := newMemoryLoaderCache()
	options := ResolverOptions{
		Caches: map[string]LoaderCache{
			"default": cache,
		},
	}
	response := cacheTestRootResponse(root, &FetchCacheConfiguration{
		CacheName:     "default",
		EnableL2Cache: true,
		TTL:           time.Minute,
		KeyTemplate: NewRootQueryCacheKeyTemplate(
			[]RootField{
				{
					TypeName:    "Query",
					FieldName:   "user",
					ResponseKey: "user",
				},
			},
			nil,
		),
		ProvidesData: cacheTestRootUserProvides(),
	})

	out1 := resolveCacheTestGraphQLResponse(t, response, options, func(ctx *Context) {
		ctx.ExecutionOptions.Caching.EnableL2Cache = true
	})
	out2 := resolveCacheTestGraphQLResponse(t, response, options, func(ctx *Context) {
		ctx.ExecutionOptions.Caching.EnableL2Cache = true
	})

	assert.Equal(t, `{"data":{"user":{"id":"1","name":"Ada"}}}`, out1)
	assert.Equal(t, out1, out2)
	assert.Equal(t, 1, root.CallCount())
	assert.Equal(t, map[string]string{
		`{"__typename":"Query","field":"user"}`: `{"user":{"id":"1","name":"Ada"}}`,
	}, cache.Snapshot())
}

type countingCacheTestDataSource struct {
	mu        sync.Mutex
	responses [][]byte
	calls     int
	inputs    []string
}

func (d *countingCacheTestDataSource) Load(_ context.Context, _ http.Header, input []byte) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	index := d.calls
	if index >= len(d.responses) {
		index = len(d.responses) - 1
	}
	d.calls++
	d.inputs = append(d.inputs, string(input))
	return append([]byte(nil), d.responses[index]...), nil
}

func (d *countingCacheTestDataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, _ []*httpclient.FileUpload) ([]byte, error) {
	return d.Load(ctx, headers, input)
}

func (d *countingCacheTestDataSource) CallCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

func (d *countingCacheTestDataSource) Inputs() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	inputs := make([]string, len(d.inputs))
	copy(inputs, d.inputs)
	return inputs
}

type memoryLoaderCache struct {
	mu      sync.Mutex
	entries map[string][]byte
	setOps  [][]*CacheEntry
}

func newMemoryLoaderCache() *memoryLoaderCache {
	return &memoryLoaderCache{
		entries: map[string][]byte{},
	}
}

func (c *memoryLoaderCache) Get(_ context.Context, keys []string) ([]*CacheEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entries := make([]*CacheEntry, len(keys))
	for i, key := range keys {
		value, ok := c.entries[key]
		if !ok {
			continue
		}
		entries[i] = &CacheEntry{
			Key:   key,
			Value: append([]byte(nil), value...),
		}
	}
	return entries, nil
}

func (c *memoryLoaderCache) Set(_ context.Context, entries []*CacheEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	copiedEntries := make([]*CacheEntry, len(entries))
	for _, entry := range entries {
		c.entries[entry.Key] = append([]byte(nil), entry.Value...)
	}
	for i, entry := range entries {
		if entry == nil {
			continue
		}
		copiedEntries[i] = &CacheEntry{
			Key:          entry.Key,
			Value:        append([]byte(nil), entry.Value...),
			TTL:          entry.TTL,
			RemainingTTL: entry.RemainingTTL,
			WriteReason:  entry.WriteReason,
		}
	}
	c.setOps = append(c.setOps, copiedEntries)
	return nil
}

func (c *memoryLoaderCache) Delete(_ context.Context, keys []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, key := range keys {
		delete(c.entries, key)
	}
	return nil
}

func (c *memoryLoaderCache) Snapshot() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()

	snapshot := make(map[string]string, len(c.entries))
	for key, value := range c.entries {
		snapshot[key] = string(value)
	}
	return snapshot
}

func (c *memoryLoaderCache) SetOps() [][]*CacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()

	ops := make([][]*CacheEntry, len(c.setOps))
	for i, entries := range c.setOps {
		ops[i] = make([]*CacheEntry, len(entries))
		for j, entry := range entries {
			if entry == nil {
				continue
			}
			ops[i][j] = &CacheEntry{
				Key:          entry.Key,
				Value:        append([]byte(nil), entry.Value...),
				TTL:          entry.TTL,
				RemainingTTL: entry.RemainingTTL,
				WriteReason:  entry.WriteReason,
			}
		}
	}
	return ops
}

func (c *memoryLoaderCache) ClearSetOps() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.setOps = nil
}

func resolveCacheTestGraphQLResponse(t *testing.T, response *GraphQLResponse, options ResolverOptions, configure func(ctx *Context)) string {
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

func cacheTestRootResponse(source DataSource, cache *FetchCacheConfiguration) *GraphQLResponse {
	return &GraphQLResponse{
		Fetches: Single(&SingleFetch{
			InputTemplate: cacheTestStaticInput(`{"method":"POST","url":"http://users","body":{"query":"query{user{id name}}"}}`),
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
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("user"),
					Value: &Object{
						Path: []string{"user"},
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
		},
	}
}

func cacheTestL1Response(source DataSource, cache *FetchCacheConfiguration) *GraphQLResponse {
	return &GraphQLResponse{
		Fetches: Sequence(
			Single(&SingleFetch{
				InputTemplate: cacheTestStaticInput(`{"method":"POST","url":"http://users","body":{"query":"query{first{id} second{id}}"}}`),
				FetchConfiguration: FetchConfiguration{
					DataSource: &countingCacheTestDataSource{
						responses: [][]byte{
							[]byte(`{"data":{"first":{"__typename":"User","id":"1"},"second":{"__typename":"User","id":"1"}}}`),
						},
					},
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
			}),
			SingleWithPath(cacheTestEntityFetch(source, cache), "query.first", ObjectPath("first")),
			SingleWithPath(cacheTestEntityFetch(source, cache), "query.second", ObjectPath("second")),
		),
		Info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
		},
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("first"),
					Value: &Object{
						Path: []string{"first"},
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
				{
					Name: []byte("second"),
					Value: &Object{
						Path: []string{"second"},
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
		},
	}
}

func cacheTestEntityFetch(source DataSource, cache *FetchCacheConfiguration) *EntityFetch {
	return &EntityFetch{
		Input: EntityInput{
			Header: cacheTestStaticInput(`{"method":"POST","url":"http://users","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {id name}}}","variables":{"representations":[`),
			Item: InputTemplate{
				Segments: []TemplateSegment{
					{
						SegmentType:  VariableSegmentType,
						VariableKind: ResolvableObjectVariableKind,
						Renderer: NewGraphQLVariableResolveRenderer(&Object{
							Fields: []*Field{
								{
									Name:  []byte("__typename"),
									Value: &String{Path: []string{"__typename"}},
								},
								{
									Name:  []byte("id"),
									Value: &String{Path: []string{"id"}},
								},
							},
						}),
					},
				},
			},
			Footer:      cacheTestStaticInput(`]}}}`),
			SkipErrItem: true,
		},
		DataSource: source,
		PostProcessing: PostProcessingConfiguration{
			SelectResponseDataPath: []string{"data", "_entities", "0"},
		},
		Cache: cache,
		Info: &FetchInfo{
			DataSourceID:   "users",
			DataSourceName: "users",
			OperationType:  ast.OperationTypeQuery,
		},
	}
}

func userNameCacheConfig(useL1 bool) *FetchCacheConfiguration {
	return &FetchCacheConfiguration{
		UseL1Cache:  useL1,
		KeyTemplate: cacheTestUserKeyTemplate(),
		ProvidesData: &Object{
			Fields: []*Field{
				{
					Name:  []byte("__typename"),
					Value: &String{},
				},
				{
					Name:  []byte("id"),
					Value: &String{},
				},
				{
					Name:  []byte("name"),
					Value: &String{},
				},
			},
		},
	}
}

func cacheTestUserKeyTemplate() *EntityQueryCacheKeyTemplate {
	return &EntityQueryCacheKeyTemplate{
		TypeName: "User",
		Keys: NewResolvableObjectVariable(&Object{
			Fields: []*Field{
				{
					Name:  []byte("id"),
					Value: &String{},
				},
			},
		}),
	}
}

func cacheTestRootUserProvides() *Object {
	return &Object{
		Fields: []*Field{
			{
				Name:  []byte("user"),
				Value: userNameCacheConfig(false).ProvidesData,
			},
		},
	}
}

func cacheTestStaticInput(data string) InputTemplate {
	return InputTemplate{
		Segments: []TemplateSegment{
			{
				Data:        []byte(data),
				SegmentType: StaticSegmentType,
			},
		},
	}
}
