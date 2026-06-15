package engine_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	nodev1 "github.com/wundergraph/cosmo/router/gen/proto/wg/cosmo/node/v1"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type federationCachingE2ECacheLogEntry struct {
	Operation string
	Keys      []string
	Hits      []bool
}

type federationCachingE2ELoaderCache struct {
	mu      sync.Mutex
	entries map[string][]byte
	log     []federationCachingE2ECacheLogEntry
}

func (c *federationCachingE2ELoaderCache) Get(_ context.Context, keys []string) ([]*resolve.CacheEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entries := make([]*resolve.CacheEntry, len(keys))
	hits := make([]bool, len(keys))
	for i, key := range keys {
		value, ok := c.entries[key]
		hits[i] = ok
		if !ok {
			continue
		}
		entries[i] = &resolve.CacheEntry{
			Key:   key,
			Value: bytes.Clone(value),
		}
	}
	c.log = append(c.log, federationCachingE2ECacheLogEntry{
		Operation: "get",
		Keys:      append([]string(nil), keys...),
		Hits:      hits,
	})
	return entries, nil
}

func (c *federationCachingE2ELoaderCache) Set(_ context.Context, entries []*resolve.CacheEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		c.entries[entry.Key] = bytes.Clone(entry.Value)
		keys = append(keys, entry.Key)
	}
	c.log = append(c.log, federationCachingE2ECacheLogEntry{
		Operation: "set",
		Keys:      keys,
	})
	return nil
}

func (c *federationCachingE2ELoaderCache) Delete(_ context.Context, keys []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, key := range keys {
		delete(c.entries, key)
	}
	c.log = append(c.log, federationCachingE2ECacheLogEntry{
		Operation: "delete",
		Keys:      append([]string(nil), keys...),
	})
	return nil
}

func (c *federationCachingE2ELoaderCache) GetLog() []federationCachingE2ECacheLogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()

	log := make([]federationCachingE2ECacheLogEntry, len(c.log))
	copy(log, c.log)
	return log
}

func (c *federationCachingE2ELoaderCache) ClearLog() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.log = nil
}

type federationCachingE2ECountingTransport struct {
	base   http.RoundTripper
	mu     sync.Mutex
	counts map[string]int
}

func (t *federationCachingE2ECountingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.counts[req.URL.Host]++
	t.mu.Unlock()

	return t.base.RoundTrip(req)
}

func (t *federationCachingE2ECountingTransport) Count(url string) int {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.counts[req.URL.Host]
}

func (t *federationCachingE2ECountingTransport) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	clear(t.counts)
}

func TestFederationCaching(t *testing.T) {
	t.Run("entity L2 hit is read on the second gateway request", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		defaultCache := &federationCachingE2ELoaderCache{
			entries: map[string][]byte{},
		}
		tracker := &federationCachingE2ECountingTransport{
			base:   http.DefaultTransport,
			counts: map[string]int{},
		}
		setup, err := federationtesting.NewFederationSetup()
		require.NoError(t, err)
		t.Cleanup(setup.Close)

		cfg := bytes.Clone(federationtesting.RouterConfigJson)
		cfg = bytes.ReplaceAll(cfg, []byte("http://accounts-url-placeholder"), []byte(setup.AccountsUpstreamServer.URL))
		cfg = bytes.ReplaceAll(cfg, []byte("http://products-url-placeholder"), []byte(setup.ProductsUpstreamServer.URL))
		cfg = bytes.ReplaceAll(cfg, []byte("http://reviews-url-placeholder"), []byte(setup.ReviewsUpstreamServer.URL))

		var routerConfig nodev1.RouterConfig
		require.NoError(t, protojson.Unmarshal(cfg, &routerConfig))
		engineConfig, err := engine.NewFederationEngineConfigFactory(
			ctx,
			engine.WithFederationHttpClient(&http.Client{Transport: tracker}),
			engine.WithSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "products",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{
							TypeName:  "Query",
							FieldName: "topProducts",
							CacheName: "default",
							TTL:       30 * time.Second,
						},
					},
				},
				{
					SubgraphName: "reviews",
					EntityCaching: plan.EntityCacheConfigurations{
						{
							TypeName:  "Product",
							CacheName: "default",
							TTL:       30 * time.Second,
						},
					},
				},
			}),
		).BuildEngineConfiguration(&routerConfig)
		require.NoError(t, err)
		executionEngine, err := engine.NewExecutionEngine(ctx, abstractlogger.NoopLogger, engineConfig, resolve.ResolverOptions{
			MaxConcurrency: 1024,
			Caches: map[string]resolve.LoaderCache{
				"default": defaultCache,
			},
		})
		require.NoError(t, err)
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var gqlRequest graphql.Request
			if err := graphql.UnmarshalHttpRequest(r, &gqlRequest); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			buf := bytes.NewBuffer(make([]byte, 0, 4096))
			resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)
			if err := executionEngine.Execute(r.Context(), &gqlRequest, &resultWriter, engine.WithRequestCachingOptions(resolve.CachingOptions{
				EnableL2Cache: true,
			})); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Add("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(buf.Bytes())
		})
		setup.GatewayServer = httptest.NewServer(handler)
		gqlClient := NewGraphqlClient(http.DefaultClient)

		resp1, headers1 := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
			`query { topProducts(first: 1) { __typename upc name reviews { body } } }`, nil, nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"__typename":"Product","upc":"top-1","name":"Trilby","reviews":[{"body":"A highly effective form of birth control."}]}]}}`, string(resp1))
		assert.Equal(t, "application/json", headers1.Get("Content-Type"))
		assert.Equal(t, 0, tracker.Count(setup.AccountsUpstreamServer.URL))
		assert.Equal(t, 1, tracker.Count(setup.ProductsUpstreamServer.URL))
		assert.Equal(t, 1, tracker.Count(setup.ReviewsUpstreamServer.URL))
		assert.Equal(t, []federationCachingE2ECacheLogEntry{
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Query","field":"topProducts","args":{"first":1}}`,
				},
				Hits: []bool{false},
			},
			{
				Operation: "set",
				Keys: []string{
					`{"__typename":"Query","field":"topProducts","args":{"first":1}}`,
				},
			},
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
				},
				Hits: []bool{false},
			},
			{
				Operation: "set",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
				},
			},
		}, defaultCache.GetLog())
		defaultCache.ClearLog()
		tracker.Clear()

		resp2, headers2 := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
			`query { topProducts(first: 1) { __typename upc name reviews { body } } }`, nil, nil, t)
		assert.Equal(t, resp1, resp2)
		assert.Equal(t, headers1.Get("Content-Type"), headers2.Get("Content-Type"))
		assert.Equal(t, 0, tracker.Count(setup.AccountsUpstreamServer.URL))
		assert.Equal(t, 0, tracker.Count(setup.ProductsUpstreamServer.URL))
		assert.Equal(t, 1, tracker.Count(setup.ReviewsUpstreamServer.URL))
		assert.Equal(t, []federationCachingE2ECacheLogEntry{
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Query","field":"topProducts","args":{"first":1}}`,
				},
				Hits: []bool{true},
			},
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
				},
				Hits: []bool{true},
			},
			{
				Operation: "set",
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
				},
			},
		}, defaultCache.GetLog())
	})

	t.Run("root-field L2 hit serves second gateway request without subgraph calls", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		defaultCache := &federationCachingE2ELoaderCache{
			entries: map[string][]byte{},
		}
		tracker := &federationCachingE2ECountingTransport{
			base:   http.DefaultTransport,
			counts: map[string]int{},
		}
		setup, err := federationtesting.NewFederationSetup()
		require.NoError(t, err)
		t.Cleanup(setup.Close)

		cfg := bytes.Clone(federationtesting.RouterConfigJson)
		cfg = bytes.ReplaceAll(cfg, []byte("http://accounts-url-placeholder"), []byte(setup.AccountsUpstreamServer.URL))
		cfg = bytes.ReplaceAll(cfg, []byte("http://products-url-placeholder"), []byte(setup.ProductsUpstreamServer.URL))
		cfg = bytes.ReplaceAll(cfg, []byte("http://reviews-url-placeholder"), []byte(setup.ReviewsUpstreamServer.URL))

		var routerConfig nodev1.RouterConfig
		require.NoError(t, protojson.Unmarshal(cfg, &routerConfig))
		engineConfig, err := engine.NewFederationEngineConfigFactory(
			ctx,
			engine.WithFederationHttpClient(&http.Client{Transport: tracker}),
			engine.WithSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
				{
					SubgraphName: "products",
					RootFieldCaching: plan.RootFieldCacheConfigurations{
						{
							TypeName:  "Query",
							FieldName: "topProducts",
							CacheName: "default",
							TTL:       30 * time.Second,
						},
					},
				},
			}),
		).BuildEngineConfiguration(&routerConfig)
		require.NoError(t, err)
		executionEngine, err := engine.NewExecutionEngine(ctx, abstractlogger.NoopLogger, engineConfig, resolve.ResolverOptions{
			MaxConcurrency: 1024,
			Caches: map[string]resolve.LoaderCache{
				"default": defaultCache,
			},
		})
		require.NoError(t, err)
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var gqlRequest graphql.Request
			if err := graphql.UnmarshalHttpRequest(r, &gqlRequest); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			buf := bytes.NewBuffer(make([]byte, 0, 4096))
			resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)
			if err := executionEngine.Execute(r.Context(), &gqlRequest, &resultWriter, engine.WithRequestCachingOptions(resolve.CachingOptions{
				EnableL2Cache: true,
			})); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Add("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(buf.Bytes())
		})
		setup.GatewayServer = httptest.NewServer(handler)
		gqlClient := NewGraphqlClient(http.DefaultClient)

		resp1, headers1 := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
			`query { topProducts { name price } }`, nil, nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","price":11},{"name":"Fedora","price":22}]}}`, string(resp1))
		assert.Equal(t, "application/json", headers1.Get("Content-Type"))
		assert.Equal(t, 0, tracker.Count(setup.AccountsUpstreamServer.URL))
		assert.Equal(t, 1, tracker.Count(setup.ProductsUpstreamServer.URL))
		assert.Equal(t, 0, tracker.Count(setup.ReviewsUpstreamServer.URL))
		assert.Equal(t, []federationCachingE2ECacheLogEntry{
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Query","field":"topProducts"}`,
				},
				Hits: []bool{false},
			},
			{
				Operation: "set",
				Keys: []string{
					`{"__typename":"Query","field":"topProducts"}`,
				},
			},
		}, defaultCache.GetLog())
		defaultCache.ClearLog()
		tracker.Clear()

		resp2, headers2 := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
			`query { topProducts { name price } }`, nil, nil, t)
		assert.Equal(t, resp1, resp2)
		assert.Equal(t, headers1.Get("Content-Type"), headers2.Get("Content-Type"))
		assert.Equal(t, 0, tracker.Count(setup.AccountsUpstreamServer.URL))
		assert.Equal(t, 0, tracker.Count(setup.ProductsUpstreamServer.URL))
		assert.Equal(t, 0, tracker.Count(setup.ReviewsUpstreamServer.URL))
		assert.Equal(t, []federationCachingE2ECacheLogEntry{
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Query","field":"topProducts"}`,
				},
				Hits: []bool{true},
			},
		}, defaultCache.GetLog())
	})
}
