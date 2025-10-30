package engine_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting/gateway"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestFederationCaching(t *testing.T) {
	t.Run("two subgraphs - miss then hit", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with tracking
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames for tracking (URL.Host includes host:port)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		// First query - should miss cache and then set
		defaultCache.ClearLog()
		tracker.Reset()
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","author":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","author":{"username":"Me"}}]}]}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		assert.Equal(t, 4, len(logAfterFirst))

		wantLog := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
			},
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Product","keys":{"upc":"top-1"}}`,
					`{"__typename":"Product","keys":{"upc":"top-2"}}`,
				},
				Hits: []bool{false, false},
			},
			{
				Operation: "set",
				Keys: []string{
					`{"__typename":"Product","keys":{"upc":"top-1"}}`,
					`{"__typename":"Product","keys":{"upc":"top-2"}}`,
				},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(logAfterFirst))

		// Verify subgraph calls for first query
		// First query should call products (topProducts) and reviews (reviews)
		// Accounts is not called directly because username is provided via reviews @provides
		productsCallsFirst := tracker.GetCount(productsHost)
		reviewsCallsFirst := tracker.GetCount(reviewsHost)
		accountsCallsFirst := tracker.GetCount(accountsHost)

		assert.Equal(t, 1, productsCallsFirst, "First query should call products subgraph exactly once")
		assert.Equal(t, 1, reviewsCallsFirst, "First query should call reviews subgraph exactly once")
		assert.Equal(t, 0, accountsCallsFirst, "First query should not call accounts subgraph (username provided via reviews @provides)")

		// Second query - should hit cache and then set
		defaultCache.ClearLog()
		tracker.Reset()
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, cachingTestQueryPath("queries/multiple_upstream.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","author":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","author":{"username":"Me"}}]}]}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		assert.Equal(t, 4, len(logAfterSecond))

		wantLogSecond := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{true}, // Should be a hit now
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
			},
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Product","keys":{"upc":"top-1"}}`,
					`{"__typename":"Product","keys":{"upc":"top-2"}}`,
				},
				Hits: []bool{true, true}, // Should be hits now, no misses
			},
			{
				Operation: "set",
				Keys: []string{
					`{"__typename":"Product","keys":{"upc":"top-1"}}`,
					`{"__typename":"Product","keys":{"upc":"top-2"}}`,
				},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond))

		// Verify subgraph calls for second query
		productsCallsSecond := tracker.GetCount(productsHost)
		reviewsCallsSecond := tracker.GetCount(reviewsHost)
		accountsCallsSecond := tracker.GetCount(accountsHost)

		assert.Equal(t, 1, productsCallsSecond, "Second query should hit cache and not call products subgraph again")
		assert.Equal(t, 1, reviewsCallsSecond, "Second query should hit cache and not call reviews subgraph again")
		assert.Equal(t, 0, accountsCallsSecond, "accounts not involved")
	})

	t.Run("two subgraphs - partial fields then full fields", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}

		// Create HTTP client with tracking
		tracker := newSubgraphCallTracker(http.DefaultTransport)
		trackingClient := &http.Client{
			Transport: tracker,
		}

		setup := federationtesting.NewFederationSetup(addCachingGateway(withCachingEnableART(false), withCachingLoaderCache(caches), withHTTPClient(trackingClient)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Extract hostnames for tracking (URL.Host includes host:port)
		accountsURLParsed, _ := url.Parse(setup.AccountsUpstreamServer.URL)
		productsURLParsed, _ := url.Parse(setup.ProductsUpstreamServer.URL)
		reviewsURLParsed, _ := url.Parse(setup.ReviewsUpstreamServer.URL)
		accountsHost := accountsURLParsed.Host
		productsHost := productsURLParsed.Host
		reviewsHost := reviewsURLParsed.Host

		// First query - only ask for name field (products subgraph only)
		defaultCache.ClearLog()
		tracker.Reset()
		firstQuery := `query {
			topProducts {
				name
			}
		}`
		resp := gqlClient.QueryString(ctx, setup.GatewayServer.URL, firstQuery, nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby"},{"name":"Fedora"}]}}`, string(resp))

		logAfterFirst := defaultCache.GetLog()
		assert.Equal(t, 2, len(logAfterFirst))

		wantLogFirst := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{false},
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogFirst), sortCacheLogKeys(logAfterFirst))

		// Verify first query calls products subgraph only
		productsCallsFirst := tracker.GetCount(productsHost)
		reviewsCallsFirst := tracker.GetCount(reviewsHost)
		accountsCallsFirst := tracker.GetCount(accountsHost)
		assert.Equal(t, 1, productsCallsFirst, "First query calls products subgraph once")
		assert.Equal(t, 0, reviewsCallsFirst, "First query does not call reviews subgraph")
		assert.Equal(t, 0, accountsCallsFirst, "First query does not call accounts subgraph")

		// Second query - ask for full fields including reviews (products + reviews + accounts)
		defaultCache.ClearLog()
		tracker.Reset()
		secondQuery := `query {
			topProducts {
				name
				reviews {
					body
					author {
						username
					}
				}
			}
		}`
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, secondQuery, nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","author":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","author":{"username":"Me"}}]}]}}`, string(resp))

		logAfterSecond := defaultCache.GetLog()
		assert.Equal(t, 4, len(logAfterSecond))

		wantLogSecond := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{true}, // Should be a hit from first query
			},
			{
				Operation: "set",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
			},
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Product","keys":{"upc":"top-1"}}`,
					`{"__typename":"Product","keys":{"upc":"top-2"}}`,
				},
				Hits: []bool{false, false}, // Miss because second query requests different fields (reviews)
			},
			{
				Operation: "set",
				Keys: []string{
					`{"__typename":"Product","keys":{"upc":"top-1"}}`,
					`{"__typename":"Product","keys":{"upc":"top-2"}}`,
				},
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogSecond), sortCacheLogKeys(logAfterSecond))

		// Verify second query: products name is cached, but reviews still need to be fetched
		productsCallsSecond := tracker.GetCount(productsHost)
		reviewsCallsSecond := tracker.GetCount(reviewsHost)
		accountsCallsSecond := tracker.GetCount(accountsHost)

		assert.Equal(t, 1, productsCallsSecond, "Second query calls products subgraph once (for reviews data)")
		assert.Equal(t, 1, reviewsCallsSecond, "Second query calls reviews subgraph once (reviews not cached)")
		assert.Equal(t, 0, accountsCallsSecond, "Second query does not call accounts subgraph")

		// Third query - repeat the second query (full fields)
		defaultCache.ClearLog()
		tracker.Reset()
		thirdQuery := `query {
			topProducts {
				name
				reviews {
					body
					author {
						username
					}
				}
			}
		}`
		resp = gqlClient.QueryString(ctx, setup.GatewayServer.URL, thirdQuery, nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","author":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","author":{"username":"Me"}}]}]}}`, string(resp))

		logAfterThird := defaultCache.GetLog()
		assert.Equal(t, 2, len(logAfterThird))

		wantLogThird := []CacheLogEntry{
			{
				Operation: "get",
				Keys:      []string{`{"__typename":"Query","field":"topProducts"}`},
				Hits:      []bool{true}, // Should be a hit from second query
			},
			{
				Operation: "get",
				Keys: []string{
					`{"__typename":"Product","keys":{"upc":"top-1"}}`,
					`{"__typename":"Product","keys":{"upc":"top-2"}}`,
				},
				Hits: []bool{true, true}, // Should be hits from second query
			},
		}
		assert.Equal(t, sortCacheLogKeys(wantLogThird), sortCacheLogKeys(logAfterThird))

		// Verify third query: all data should be cached, no subgraph calls
		productsCallsThird := tracker.GetCount(productsHost)
		reviewsCallsThird := tracker.GetCount(reviewsHost)
		accountsCallsThird := tracker.GetCount(accountsHost)

		// All cache entries show hits, so no subgraph calls should be made
		assert.Equal(t, 0, productsCallsThird, "Third query does not call products subgraph (all cache hits)")
		assert.Equal(t, 0, reviewsCallsThird, "Third query does not call reviews subgraph (all cache hits)")
		assert.Equal(t, 0, accountsCallsThird, "Third query does not call accounts subgraph")
	})
}

// subgraphCallTracker tracks HTTP requests made to subgraph servers
type subgraphCallTracker struct {
	mu       sync.RWMutex
	counts   map[string]int // Maps subgraph URL to call count
	original http.RoundTripper
}

func newSubgraphCallTracker(original http.RoundTripper) *subgraphCallTracker {
	return &subgraphCallTracker{
		counts:   make(map[string]int),
		original: original,
	}
}

func (t *subgraphCallTracker) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	host := req.URL.Host
	t.counts[host]++
	t.mu.Unlock()
	return t.original.RoundTrip(req)
}

func (t *subgraphCallTracker) GetCount(url string) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.counts[url]
}

func (t *subgraphCallTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.counts = make(map[string]int)
}

func (t *subgraphCallTracker) GetCounts() map[string]int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make(map[string]int)
	for k, v := range t.counts {
		result[k] = v
	}
	return result
}

func (t *subgraphCallTracker) DebugPrint() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return fmt.Sprintf("%v", t.counts)
}

// Helper functions for gateway setup with HTTP client support
type cachingGatewayOptions struct {
	enableART       bool
	withLoaderCache map[string]resolve.LoaderCache
	httpClient      *http.Client
}

func withCachingEnableART(enableART bool) func(*cachingGatewayOptions) {
	return func(opts *cachingGatewayOptions) {
		opts.enableART = enableART
	}
}

func withCachingLoaderCache(loaderCache map[string]resolve.LoaderCache) func(*cachingGatewayOptions) {
	return func(opts *cachingGatewayOptions) {
		opts.withLoaderCache = loaderCache
	}
}

func withHTTPClient(client *http.Client) func(*cachingGatewayOptions) {
	return func(opts *cachingGatewayOptions) {
		opts.httpClient = client
	}
}

type cachingGatewayOptionsToFunc func(opts *cachingGatewayOptions)

func addCachingGateway(options ...cachingGatewayOptionsToFunc) func(setup *federationtesting.FederationSetup) *httptest.Server {
	opts := &cachingGatewayOptions{}
	for _, option := range options {
		option(opts)
	}
	return func(setup *federationtesting.FederationSetup) *httptest.Server {
		httpClient := opts.httpClient
		if httpClient == nil {
			httpClient = http.DefaultClient
		}

		poller := gateway.NewDatasource([]gateway.ServiceConfig{
			{Name: "accounts", URL: setup.AccountsUpstreamServer.URL},
			{Name: "products", URL: setup.ProductsUpstreamServer.URL, WS: strings.ReplaceAll(setup.ProductsUpstreamServer.URL, "http:", "ws:")},
			{Name: "reviews", URL: setup.ReviewsUpstreamServer.URL},
		}, httpClient)

		gtw := gateway.Handler(abstractlogger.NoopLogger, poller, httpClient, opts.enableART, opts.withLoaderCache)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		poller.Run(ctx)
		return httptest.NewServer(gtw)
	}
}

func cachingTestQueryPath(name string) string {
	return path.Join("..", "federationtesting", "testdata", name)
}

type CacheLogEntry struct {
	Operation string   // "get", "set", "delete"
	Keys      []string // Keys involved in the operation
	Hits      []bool   // For Get: whether each key was a hit (true) or miss (false)
}

// normalizeCacheLog creates a copy of log entries without timestamps for comparison
func normalizeCacheLog(log []CacheLogEntry) []CacheLogEntry {
	normalized := make([]CacheLogEntry, len(log))
	for i, entry := range log {
		normalized[i] = CacheLogEntry{
			Operation: entry.Operation,
			Keys:      entry.Keys,
			Hits:      entry.Hits,
			// Timestamp is zero value for comparison
		}
	}
	return normalized
}

// sortCacheLogKeys sorts the keys (and corresponding hits) in each cache log entry
// This makes comparisons order-independent when multiple keys are present
func sortCacheLogKeys(log []CacheLogEntry) []CacheLogEntry {
	sorted := make([]CacheLogEntry, len(log))
	for i, entry := range log {
		// Only sort if there are multiple keys
		if len(entry.Keys) <= 1 {
			sorted[i] = entry
			continue
		}

		// Create pairs of (key, hit) to sort together
		pairs := make([]struct {
			key string
			hit bool
		}, len(entry.Keys))
		for j := range entry.Keys {
			pairs[j].key = entry.Keys[j]
			if entry.Hits != nil && j < len(entry.Hits) {
				pairs[j].hit = entry.Hits[j]
			}
		}

		// Sort pairs by key
		sort.Slice(pairs, func(a, b int) bool {
			return pairs[a].key < pairs[b].key
		})

		// Extract sorted keys and hits
		sorted[i] = CacheLogEntry{
			Operation: entry.Operation,
			Keys:      make([]string, len(pairs)),
			Hits:      nil,
		}
		if entry.Hits != nil && len(entry.Hits) > 0 {
			sorted[i].Hits = make([]bool, len(pairs))
		}
		for j := range pairs {
			sorted[i].Keys[j] = pairs[j].key
			if sorted[i].Hits != nil {
				sorted[i].Hits[j] = pairs[j].hit
			}
		}
	}
	return sorted
}

type cacheEntry struct {
	data      []byte
	expiresAt *time.Time
}

type FakeLoaderCache struct {
	mu      sync.RWMutex
	storage map[string]cacheEntry
	log     []CacheLogEntry
}

func NewFakeLoaderCache() *FakeLoaderCache {
	return &FakeLoaderCache{
		storage: make(map[string]cacheEntry),
		log:     make([]CacheLogEntry, 0),
	}
}

func (f *FakeLoaderCache) cleanupExpired() {
	now := time.Now()
	for key, entry := range f.storage {
		if entry.expiresAt != nil && now.After(*entry.expiresAt) {
			delete(f.storage, key)
		}
	}
}

func (f *FakeLoaderCache) Get(ctx context.Context, keys []string) ([]*resolve.CacheEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Clean up expired entries before executing command
	f.cleanupExpired()

	hits := make([]bool, len(keys))
	result := make([]*resolve.CacheEntry, len(keys))
	for i, key := range keys {
		if entry, exists := f.storage[key]; exists {
			// Make a copy of the data to prevent external modifications
			dataCopy := make([]byte, len(entry.data))
			copy(dataCopy, entry.data)
			result[i] = &resolve.CacheEntry{
				Key:   key,
				Value: dataCopy,
			}
			hits[i] = true
		} else {
			result[i] = nil
			hits[i] = false
		}
	}

	// Log the operation
	f.log = append(f.log, CacheLogEntry{
		Operation: "get",
		Keys:      keys,
		Hits:      hits,
	})

	return result, nil
}

func (f *FakeLoaderCache) Set(ctx context.Context, entries []*resolve.CacheEntry, ttl time.Duration) error {
	if len(entries) == 0 {
		return nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Clean up expired entries before executing command
	f.cleanupExpired()

	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		cacheEntry := cacheEntry{
			// Make a copy of the data to prevent external modifications
			data: make([]byte, len(entry.Value)),
		}
		copy(cacheEntry.data, entry.Value)

		// If ttl is 0, store without expiration
		if ttl > 0 {
			expiresAt := time.Now().Add(ttl)
			cacheEntry.expiresAt = &expiresAt
		}

		f.storage[entry.Key] = cacheEntry
		keys = append(keys, entry.Key)
	}

	// Log the operation
	f.log = append(f.log, CacheLogEntry{
		Operation: "set",
		Keys:      keys,
		Hits:      nil, // Set operations don't have hits/misses
	})

	return nil
}

func (f *FakeLoaderCache) Delete(ctx context.Context, keys []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Clean up expired entries before executing command
	f.cleanupExpired()

	for _, key := range keys {
		delete(f.storage, key)
	}

	// Log the operation
	f.log = append(f.log, CacheLogEntry{
		Operation: "delete",
		Keys:      keys,
		Hits:      nil, // Delete operations don't have hits/misses
	})

	return nil
}

// GetLog returns a copy of the cache operation log
func (f *FakeLoaderCache) GetLog() []CacheLogEntry {
	f.mu.RLock()
	defer f.mu.RUnlock()
	logCopy := make([]CacheLogEntry, len(f.log))
	copy(logCopy, f.log)
	return logCopy
}

// ClearLog clears the cache operation log
func (f *FakeLoaderCache) ClearLog() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = make([]CacheLogEntry, 0)
}

// TestFakeLoaderCache tests the cache implementation itself
func TestFakeLoaderCache(t *testing.T) {
	ctx := context.Background()
	cache := NewFakeLoaderCache()

	t.Run("SetAndGet", func(t *testing.T) {
		// Test basic set and get
		keys := []string{"key1", "key2", "key3"}
		entries := []*resolve.CacheEntry{
			{Key: "key1", Value: []byte("value1")},
			{Key: "key2", Value: []byte("value2")},
			{Key: "key3", Value: []byte("value3")},
		}

		err := cache.Set(ctx, entries, 0) // No TTL
		require.NoError(t, err)

		// Get all keys
		result, err := cache.Get(ctx, keys)
		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.NotNil(t, result[0])
		assert.Equal(t, "value1", string(result[0].Value))
		assert.NotNil(t, result[1])
		assert.Equal(t, "value2", string(result[1].Value))
		assert.NotNil(t, result[2])
		assert.Equal(t, "value3", string(result[2].Value))

		// Get partial keys
		result, err = cache.Get(ctx, []string{"key2", "key4", "key1"})
		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.NotNil(t, result[0])
		assert.Equal(t, "value2", string(result[0].Value))
		assert.Nil(t, result[1]) // key4 doesn't exist
		assert.NotNil(t, result[2])
		assert.Equal(t, "value1", string(result[2].Value))
	})

	t.Run("Delete", func(t *testing.T) {
		// Set some keys
		entries := []*resolve.CacheEntry{
			{Key: "del1", Value: []byte("v1")},
			{Key: "del2", Value: []byte("v2")},
			{Key: "del3", Value: []byte("v3")},
		}
		err := cache.Set(ctx, entries, 0)
		require.NoError(t, err)

		// Delete some keys
		err = cache.Delete(ctx, []string{"del1", "del3"})
		require.NoError(t, err)

		// Check remaining keys
		result, err := cache.Get(ctx, []string{"del1", "del2", "del3"})
		require.NoError(t, err)
		assert.Nil(t, result[0])    // del1 was deleted
		assert.NotNil(t, result[1]) // del2 still exists
		assert.Equal(t, "v2", string(result[1].Value))
		assert.Nil(t, result[2]) // del3 was deleted
	})

	t.Run("TTL", func(t *testing.T) {
		// Set with 50ms TTL
		entries := []*resolve.CacheEntry{
			{Key: "ttl1", Value: []byte("expire1")},
			{Key: "ttl2", Value: []byte("expire2")},
		}
		err := cache.Set(ctx, entries, 50*time.Millisecond)
		require.NoError(t, err)

		// Immediately get - should exist
		result, err := cache.Get(ctx, []string{"ttl1", "ttl2"})
		require.NoError(t, err)
		assert.NotNil(t, result[0])
		assert.Equal(t, "expire1", string(result[0].Value))
		assert.NotNil(t, result[1])
		assert.Equal(t, "expire2", string(result[1].Value))

		// Wait for expiration
		time.Sleep(60 * time.Millisecond)

		// Get again - should be nil
		result, err = cache.Get(ctx, []string{"ttl1", "ttl2"})
		require.NoError(t, err)
		assert.Nil(t, result[0])
		assert.Nil(t, result[1])
	})

	t.Run("MixedTTL", func(t *testing.T) {
		// Set some with TTL, some without
		err := cache.Set(ctx, []*resolve.CacheEntry{{Key: "perm1", Value: []byte("permanent")}}, 0)
		require.NoError(t, err)

		err = cache.Set(ctx, []*resolve.CacheEntry{{Key: "temp1", Value: []byte("temporary")}}, 50*time.Millisecond)
		require.NoError(t, err)

		// Wait for temporary to expire
		time.Sleep(60 * time.Millisecond)

		// Check both
		result, err := cache.Get(ctx, []string{"perm1", "temp1"})
		require.NoError(t, err)
		assert.NotNil(t, result[0])
		assert.Equal(t, "permanent", string(result[0].Value)) // Still exists
		assert.Nil(t, result[1])                              // Expired
	})

	t.Run("ThreadSafety", func(t *testing.T) {
		// Test concurrent access
		done := make(chan bool)

		// Writer goroutine
		go func() {
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("concurrent_%d", i)
				value := fmt.Sprintf("value_%d", i)
				err := cache.Set(ctx, []*resolve.CacheEntry{{Key: key, Value: []byte(value)}}, 0)
				assert.NoError(t, err)
			}
			done <- true
		}()

		// Reader goroutine
		go func() {
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("concurrent_%d", i%50)
				_, err := cache.Get(ctx, []string{key})
				assert.NoError(t, err)
			}
			done <- true
		}()

		// Deleter goroutine
		go func() {
			for i := 0; i < 50; i++ {
				key := fmt.Sprintf("concurrent_%d", i*2)
				err := cache.Delete(ctx, []string{key})
				assert.NoError(t, err)
			}
			done <- true
		}()

		// Wait for all goroutines
		<-done
		<-done
		<-done
	})

	t.Run("ResultLengthMatchesKeysLength", func(t *testing.T) {
		// Test that result length always matches input keys length

		// Set some data
		err := cache.Set(ctx, []*resolve.CacheEntry{
			{Key: "exist1", Value: []byte("data1")},
			{Key: "exist3", Value: []byte("data3")},
		}, 0)
		require.NoError(t, err)

		// Request mix of existing and non-existing keys
		keys := []string{"exist1", "missing1", "exist3", "missing2", "missing3"}
		result, err := cache.Get(ctx, keys)
		require.NoError(t, err)

		// Verify length matches exactly
		assert.Len(t, result, len(keys), "Result length must match keys length")
		assert.Len(t, result, 5, "Should return exactly 5 results")

		// Verify correct values
		assert.NotNil(t, result[0])
		assert.Equal(t, "data1", string(result[0].Value)) // exist1
		assert.Nil(t, result[1])                          // missing1
		assert.NotNil(t, result[2])
		assert.Equal(t, "data3", string(result[2].Value)) // exist3
		assert.Nil(t, result[3])                          // missing2
		assert.Nil(t, result[4])                          // missing3

		// Test with all missing keys
		allMissingKeys := []string{"missing4", "missing5", "missing6"}
		result, err = cache.Get(ctx, allMissingKeys)
		require.NoError(t, err)
		assert.Len(t, result, 3, "Should return 3 results for 3 keys")
		assert.Nil(t, result[0])
		assert.Nil(t, result[1])
		assert.Nil(t, result[2])

		// Test with empty keys
		result, err = cache.Get(ctx, []string{})
		require.NoError(t, err)
		assert.Len(t, result, 0, "Should return empty slice for empty keys")
	})
}
