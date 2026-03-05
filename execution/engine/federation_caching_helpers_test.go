package engine_test

import (
	"context"
	"encoding/json"
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

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting/gateway"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

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
	enableART                    bool
	withLoaderCache              map[string]resolve.LoaderCache
	httpClient                   *http.Client
	subgraphHeadersBuilder       resolve.SubgraphHeadersBuilder
	cachingOptions               resolve.CachingOptions
	subgraphEntityCachingConfigs engine.SubgraphCachingConfigs
	debugMode                    bool
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

func withSubgraphHeadersBuilder(builder resolve.SubgraphHeadersBuilder) func(*cachingGatewayOptions) {
	return func(opts *cachingGatewayOptions) {
		opts.subgraphHeadersBuilder = builder
	}
}

func withCachingOptionsFunc(cachingOpts resolve.CachingOptions) func(*cachingGatewayOptions) {
	return func(opts *cachingGatewayOptions) {
		opts.cachingOptions = cachingOpts
	}
}

func withSubgraphEntityCachingConfigs(configs engine.SubgraphCachingConfigs) func(*cachingGatewayOptions) {
	return func(opts *cachingGatewayOptions) {
		opts.subgraphEntityCachingConfigs = configs
	}
}

func withDebugMode(enabled bool) func(*cachingGatewayOptions) {
	return func(opts *cachingGatewayOptions) {
		opts.debugMode = enabled
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

		gtw := gateway.HandlerWithCaching(abstractlogger.NoopLogger, poller, httpClient, opts.enableART, opts.withLoaderCache, opts.subgraphHeadersBuilder, opts.cachingOptions, opts.subgraphEntityCachingConfigs, opts.debugMode)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		poller.Run(ctx)
		return httptest.NewServer(gtw)
	}
}

// mockSubgraphHeadersBuilder is a mock implementation of SubgraphHeadersBuilder
type mockSubgraphHeadersBuilder struct {
	hashes map[string]uint64
}

func (m *mockSubgraphHeadersBuilder) HeadersForSubgraph(subgraphName string) (http.Header, uint64) {
	hash := m.hashes[subgraphName]
	if hash == 0 {
		// Return default hash if not found
		return nil, 99999
	}
	return nil, hash
}

func (m *mockSubgraphHeadersBuilder) HashAll() uint64 {
	// Return a simple hash of all subgraph hashes combined
	var result uint64
	for _, hash := range m.hashes {
		result ^= hash
	}
	return result
}

func cachingTestQueryPath(name string) string {
	return path.Join("..", "federationtesting", "testdata", name)
}

type CacheLogEntry struct {
	Operation string        // "get", "set", "delete"
	Keys      []string      // Keys involved in the operation
	Hits      []bool        // For Get: whether each key was a hit (true) or miss (false)
	TTL       time.Duration // For Set: the TTL used
	Caller    string        // Fetch identity when debug enabled: "accounts: entity(User)" or "products: rootField(Query.topProducts)"
}

// sortCacheLogKeys sorts the keys (and corresponding hits) in each cache log entry.
// This makes comparisons order-independent when multiple keys are present.
// Caller is intentionally stripped — it's for debug logging, not assertions.
func sortCacheLogKeys(log []CacheLogEntry) []CacheLogEntry {
	sorted := make([]CacheLogEntry, len(log))
	for i, entry := range log {
		// Only sort if there are multiple keys
		if len(entry.Keys) <= 1 {
			sorted[i] = CacheLogEntry{
				Operation: entry.Operation,
				Keys:      entry.Keys,
				Hits:      entry.Hits,
			}
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
		if len(entry.Hits) > 0 {
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

// sortCacheLogKeysWithCaller is like sortCacheLogKeys but preserves the Caller field.
// Use this when you want assertions to verify which Loader method chain triggered each cache event.
func sortCacheLogKeysWithCaller(log []CacheLogEntry) []CacheLogEntry {
	sorted := make([]CacheLogEntry, len(log))
	for i, entry := range log {
		if len(entry.Keys) <= 1 {
			sorted[i] = CacheLogEntry{
				Operation: entry.Operation,
				Keys:      entry.Keys,
				Hits:      entry.Hits,
				Caller:    entry.Caller,
			}
			continue
		}

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
		sort.Slice(pairs, func(a, b int) bool {
			return pairs[a].key < pairs[b].key
		})
		sorted[i] = CacheLogEntry{
			Operation: entry.Operation,
			Keys:      make([]string, len(pairs)),
			Hits:      nil,
			Caller:    entry.Caller,
		}
		if len(entry.Hits) > 0 {
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

// sortCacheLogEntries sorts both the entries (by operation+first key) and the keys within entries.
// Use this when log entry order is non-deterministic (e.g., split datasources executing in parallel).
func sortCacheLogEntries(log []CacheLogEntry) []CacheLogEntry {
	sorted := sortCacheLogKeys(log)
	sort.Slice(sorted, func(a, b int) bool {
		if sorted[a].Operation != sorted[b].Operation {
			return sorted[a].Operation < sorted[b].Operation
		}
		keyA, keyB := "", ""
		if len(sorted[a].Keys) > 0 {
			keyA = sorted[a].Keys[0]
		}
		if len(sorted[b].Keys) > 0 {
			keyB = sorted[b].Keys[0]
		}
		return keyA < keyB
	})
	return sorted
}

// sortCacheLogKeysWithTTL is like sortCacheLogKeys but preserves the TTL field.
// Use this when assertions need to verify TTL values on set operations.
func sortCacheLogKeysWithTTL(log []CacheLogEntry) []CacheLogEntry {
	sorted := make([]CacheLogEntry, len(log))
	for i, entry := range log {
		if len(entry.Keys) <= 1 {
			sorted[i] = CacheLogEntry{
				Operation: entry.Operation,
				Keys:      entry.Keys,
				Hits:      entry.Hits,
				TTL:       entry.TTL,
			}
			continue
		}

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
		sort.Slice(pairs, func(a, b int) bool {
			return pairs[a].key < pairs[b].key
		})
		sorted[i] = CacheLogEntry{
			Operation: entry.Operation,
			Keys:      make([]string, len(pairs)),
			Hits:      nil,
			TTL:       entry.TTL,
		}
		if len(entry.Hits) > 0 {
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

// sortCacheLogEntriesWithTTL sorts both entries and keys while preserving TTL.
// Use this when entry order is non-deterministic and TTL values need to be verified.
func sortCacheLogEntriesWithTTL(log []CacheLogEntry) []CacheLogEntry {
	sorted := sortCacheLogKeysWithTTL(log)
	sort.Slice(sorted, func(a, b int) bool {
		if sorted[a].Operation != sorted[b].Operation {
			return sorted[a].Operation < sorted[b].Operation
		}
		keyA, keyB := "", ""
		if len(sorted[a].Keys) > 0 {
			keyA = sorted[a].Keys[0]
		}
		if len(sorted[b].Keys) > 0 {
			keyB = sorted[b].Keys[0]
		}
		return keyA < keyB
	})
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
			ce := &resolve.CacheEntry{
				Key:   key,
				Value: dataCopy,
			}
			// Populate RemainingTTL from expiresAt for cache age analytics
			if entry.expiresAt != nil {
				remaining := time.Until(*entry.expiresAt)
				if remaining > 0 {
					ce.RemainingTTL = remaining
				}
			}
			result[i] = ce
			hits[i] = true
		} else {
			result[i] = nil
			hits[i] = false
		}
	}

	// Log the operation
	caller := ""
	if cfi := resolve.GetCacheFetchInfo(ctx); cfi != nil {
		caller = cfi.String()
	}
	f.log = append(f.log, CacheLogEntry{
		Operation: "get",
		Keys:      keys,
		Hits:      hits,
		Caller:    caller,
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
	caller := ""
	if cfi := resolve.GetCacheFetchInfo(ctx); cfi != nil {
		caller = cfi.String()
	}
	f.log = append(f.log, CacheLogEntry{
		Operation: "set",
		Keys:      keys,
		Hits:      nil, // Set operations don't have hits/misses
		TTL:       ttl,
		Caller:    caller,
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
	caller := ""
	if cfi := resolve.GetCacheFetchInfo(ctx); cfi != nil {
		caller = cfi.String()
	}
	f.log = append(f.log, CacheLogEntry{
		Operation: "delete",
		Keys:      keys,
		Hits:      nil, // Delete operations don't have hits/misses
		Caller:    caller,
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

// GetLogWithCaller returns a copy of the cache operation log with Caller populated.
// Use this with sortCacheLogKeysWithCaller to assert on both operation details and
// the Loader method chain that triggered each cache event.
func (f *FakeLoaderCache) GetLogWithCaller() []CacheLogEntry {
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

// =============================================================================
// L1/L2 CACHE END-TO-END TESTS
// =============================================================================
//
// These tests verify the L1 (per-request in-memory) and L2 (external cross-request)
// caching behavior in a federated GraphQL setup.
//
// L1 Cache: Prevents redundant fetches for the same entity within a single request
// L2 Cache: Shares entity data across requests via external cache (e.g., Redis)
//
// Lookup Order (entity fetches): L1 -> L2 -> Subgraph Fetch
// Lookup Order (root fetches): L2 -> Subgraph Fetch (no L1)

func parseCacheAnalytics(t *testing.T, headers http.Header) resolve.CacheAnalyticsSnapshot {
	t.Helper()
	raw := headers.Get("X-Cache-Analytics")
	require.NotEmpty(t, raw, "X-Cache-Analytics header should be present")
	var snap resolve.CacheAnalyticsSnapshot
	err := json.Unmarshal([]byte(raw), &snap)
	require.NoError(t, err, "X-Cache-Analytics header should be valid JSON")
	return snap
}

// normalizeSnapshot makes a CacheAnalyticsSnapshot deterministically comparable by
// sorting EntityTypes, L1Reads, L2Reads, L1Writes, L2Writes, and FieldHashes.
func normalizeSnapshot(snap resolve.CacheAnalyticsSnapshot) resolve.CacheAnalyticsSnapshot {
	// Sort EntityTypes by TypeName
	if snap.EntityTypes != nil {
		sorted := make([]resolve.EntityTypeInfo, len(snap.EntityTypes))
		copy(sorted, snap.EntityTypes)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].TypeName < sorted[j].TypeName
		})
		snap.EntityTypes = sorted
	}

	// Sort L1Reads and zero out non-deterministic CacheAgeMs
	if snap.L1Reads != nil {
		sorted := make([]resolve.CacheKeyEvent, len(snap.L1Reads))
		copy(sorted, snap.L1Reads)
		for i := range sorted {
			sorted[i].CacheAgeMs = 0
		}
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].CacheKey != sorted[j].CacheKey {
				return sorted[i].CacheKey < sorted[j].CacheKey
			}
			return sorted[i].Kind < sorted[j].Kind
		})
		snap.L1Reads = sorted
	}

	// Sort L2Reads and zero out non-deterministic CacheAgeMs
	if snap.L2Reads != nil {
		sorted := make([]resolve.CacheKeyEvent, len(snap.L2Reads))
		copy(sorted, snap.L2Reads)
		for i := range sorted {
			sorted[i].CacheAgeMs = 0
		}
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].CacheKey != sorted[j].CacheKey {
				return sorted[i].CacheKey < sorted[j].CacheKey
			}
			return sorted[i].Kind < sorted[j].Kind
		})
		snap.L2Reads = sorted
	}

	// Sort L1Writes
	if snap.L1Writes != nil {
		sorted := make([]resolve.CacheWriteEvent, len(snap.L1Writes))
		copy(sorted, snap.L1Writes)
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].CacheKey != sorted[j].CacheKey {
				return sorted[i].CacheKey < sorted[j].CacheKey
			}
			return sorted[i].CacheLevel < sorted[j].CacheLevel
		})
		snap.L1Writes = sorted
	}

	// Sort L2Writes
	if snap.L2Writes != nil {
		sorted := make([]resolve.CacheWriteEvent, len(snap.L2Writes))
		copy(sorted, snap.L2Writes)
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].CacheKey != sorted[j].CacheKey {
				return sorted[i].CacheKey < sorted[j].CacheKey
			}
			return sorted[i].CacheLevel < sorted[j].CacheLevel
		})
		snap.L2Writes = sorted
	}

	// Sort FieldHashes for deterministic comparison
	if snap.FieldHashes != nil {
		sorted := make([]resolve.EntityFieldHash, len(snap.FieldHashes))
		copy(sorted, snap.FieldHashes)
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].EntityType != sorted[j].EntityType {
				return sorted[i].EntityType < sorted[j].EntityType
			}
			if sorted[i].FieldName != sorted[j].FieldName {
				return sorted[i].FieldName < sorted[j].FieldName
			}
			if sorted[i].KeyRaw != sorted[j].KeyRaw {
				return sorted[i].KeyRaw < sorted[j].KeyRaw
			}
			if sorted[i].KeyHash != sorted[j].KeyHash {
				return sorted[i].KeyHash < sorted[j].KeyHash
			}
			return sorted[i].FieldHash < sorted[j].FieldHash
		})
		snap.FieldHashes = sorted
	}

	// Sort ShadowComparisons by CacheKey and zero out non-deterministic CacheAgeMs
	if snap.ShadowComparisons != nil {
		sorted := make([]resolve.ShadowComparisonEvent, len(snap.ShadowComparisons))
		copy(sorted, snap.ShadowComparisons)
		for i := range sorted {
			sorted[i].CacheAgeMs = 0
		}
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].CacheKey != sorted[j].CacheKey {
				return sorted[i].CacheKey < sorted[j].CacheKey
			}
			return sorted[i].EntityType < sorted[j].EntityType
		})
		snap.ShadowComparisons = sorted
	}

	// Sort MutationEvents for deterministic comparison
	if snap.MutationEvents != nil {
		sorted := make([]resolve.MutationEvent, len(snap.MutationEvents))
		copy(sorted, snap.MutationEvents)
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].MutationRootField != sorted[j].MutationRootField {
				return sorted[i].MutationRootField < sorted[j].MutationRootField
			}
			return sorted[i].EntityCacheKey < sorted[j].EntityCacheKey
		})
		snap.MutationEvents = sorted
	}

	// Zero out non-deterministic FetchTimings (DurationMs varies between runs)
	// Use normalizeFetchTimings() when you need to assert FetchTimings fields.
	snap.FetchTimings = nil

	// Normalize empty slices to nil for consistent comparison
	// (JSON unmarshalling produces empty slices, expected literals produce nil)
	if len(snap.L1Reads) == 0 {
		snap.L1Reads = nil
	}
	if len(snap.L2Reads) == 0 {
		snap.L2Reads = nil
	}
	if len(snap.L1Writes) == 0 {
		snap.L1Writes = nil
	}
	if len(snap.L2Writes) == 0 {
		snap.L2Writes = nil
	}
	if len(snap.EntityTypes) == 0 {
		snap.EntityTypes = nil
	}
	if len(snap.FieldHashes) == 0 {
		snap.FieldHashes = nil
	}
	if len(snap.ErrorEvents) == 0 {
		snap.ErrorEvents = nil
	}
	if len(snap.ShadowComparisons) == 0 {
		snap.ShadowComparisons = nil
	}
	if len(snap.MutationEvents) == 0 {
		snap.MutationEvents = nil
	}

	return snap
}

// normalizeFetchTimings sorts FetchTimings deterministically and zeros DurationMs
// (the only non-deterministic field). Unlike normalizeSnapshot, this preserves
// all other fields (HTTPStatusCode, ResponseBytes, etc.) for assertion.
func normalizeFetchTimings(timings []resolve.FetchTimingEvent) []resolve.FetchTimingEvent {
	sorted := make([]resolve.FetchTimingEvent, len(timings))
	copy(sorted, timings)
	for i := range sorted {
		sorted[i].DurationMs = 0
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].DataSource != sorted[j].DataSource {
			return sorted[i].DataSource < sorted[j].DataSource
		}
		return sorted[i].Source < sorted[j].Source
	})
	return sorted
}

func mustParseHost(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		panic(fmt.Sprintf("failed to parse URL %q: %v", rawURL, err))
	}
	return parsed.Host
}
