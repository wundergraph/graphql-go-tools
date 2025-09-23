package engine_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestFederationCaching(t *testing.T) {
	t.Run("query spans multiple federated servers", func(t *testing.T) {
		defaultCache := NewFakeLoaderCache()
		caches := map[string]resolve.LoaderCache{
			"default": defaultCache,
		}
		setup := federationtesting.NewFederationSetup(addGateway(withEnableART(false), withLoaderCache(caches)))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/multiple_upstream.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","author":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","author":{"username":"Me"}}]}]}}`, string(resp))
		resp = gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/multiple_upstream.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","author":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","author":{"username":"Me"}}]}]}}`, string(resp))
		defaultCache.mu.Lock()
		defer defaultCache.mu.Unlock()
		_, ok := defaultCache.storage[`{"__typename":"Product","upc":"top-1"}`]
		assert.True(t, ok)
		_, ok = defaultCache.storage[`{"__typename":"Product","upc":"top-2"}`]
		assert.True(t, ok)
	})
}

type cacheEntry struct {
	data      []byte
	expiresAt *time.Time
}

type FakeLoaderCache struct {
	mu      sync.RWMutex
	storage map[string]cacheEntry
}

func NewFakeLoaderCache() *FakeLoaderCache {
	return &FakeLoaderCache{
		storage: make(map[string]cacheEntry),
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

func (f *FakeLoaderCache) Get(ctx context.Context, keys []string) ([][]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Clean up expired entries before executing command
	f.cleanupExpired()

	result := make([][]byte, len(keys))
	for i, key := range keys {
		if entry, exists := f.storage[key]; exists {
			// Make a copy of the data to prevent external modifications
			dataCopy := make([]byte, len(entry.data))
			copy(dataCopy, entry.data)
			result[i] = dataCopy
		} else {
			result[i] = nil
		}
	}
	return result, nil
}

func (f *FakeLoaderCache) Set(ctx context.Context, keys []string, items [][]byte, ttl time.Duration) error {
	if len(keys) != len(items) {
		return nil // Silently ignore mismatched lengths like Redis would
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Clean up expired entries before executing command
	f.cleanupExpired()

	for i, key := range keys {
		entry := cacheEntry{
			// Make a copy of the data to prevent external modifications
			data: make([]byte, len(items[i])),
		}
		copy(entry.data, items[i])

		// If ttl is 0, store without expiration
		if ttl > 0 {
			expiresAt := time.Now().Add(ttl)
			entry.expiresAt = &expiresAt
		}

		f.storage[key] = entry
	}
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
	return nil
}

// TestFakeLoaderCache tests the cache implementation itself
func TestFakeLoaderCache(t *testing.T) {
	ctx := context.Background()
	cache := NewFakeLoaderCache()

	t.Run("SetAndGet", func(t *testing.T) {
		// Test basic set and get
		keys := []string{"key1", "key2", "key3"}
		items := [][]byte{[]byte("value1"), []byte("value2"), []byte("value3")}

		err := cache.Set(ctx, keys, items, 0) // No TTL
		require.NoError(t, err)

		// Get all keys
		result, err := cache.Get(ctx, keys)
		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.Equal(t, "value1", string(result[0]))
		assert.Equal(t, "value2", string(result[1]))
		assert.Equal(t, "value3", string(result[2]))

		// Get partial keys
		result, err = cache.Get(ctx, []string{"key2", "key4", "key1"})
		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.Equal(t, "value2", string(result[0]))
		assert.Nil(t, result[1]) // key4 doesn't exist
		assert.Equal(t, "value1", string(result[2]))
	})

	t.Run("Delete", func(t *testing.T) {
		// Set some keys
		keys := []string{"del1", "del2", "del3"}
		items := [][]byte{[]byte("v1"), []byte("v2"), []byte("v3")}
		err := cache.Set(ctx, keys, items, 0)
		require.NoError(t, err)

		// Delete some keys
		err = cache.Delete(ctx, []string{"del1", "del3"})
		require.NoError(t, err)

		// Check remaining keys
		result, err := cache.Get(ctx, keys)
		require.NoError(t, err)
		assert.Nil(t, result[0])                 // del1 was deleted
		assert.Equal(t, "v2", string(result[1])) // del2 still exists
		assert.Nil(t, result[2])                 // del3 was deleted
	})

	t.Run("TTL", func(t *testing.T) {
		// Set with 50ms TTL
		keys := []string{"ttl1", "ttl2"}
		items := [][]byte{[]byte("expire1"), []byte("expire2")}
		err := cache.Set(ctx, keys, items, 50*time.Millisecond)
		require.NoError(t, err)

		// Immediately get - should exist
		result, err := cache.Get(ctx, keys)
		require.NoError(t, err)
		assert.Equal(t, "expire1", string(result[0]))
		assert.Equal(t, "expire2", string(result[1]))

		// Wait for expiration
		time.Sleep(60 * time.Millisecond)

		// Get again - should be nil
		result, err = cache.Get(ctx, keys)
		require.NoError(t, err)
		assert.Nil(t, result[0])
		assert.Nil(t, result[1])
	})

	t.Run("MixedTTL", func(t *testing.T) {
		// Set some with TTL, some without
		err := cache.Set(ctx, []string{"perm1"}, [][]byte{[]byte("permanent")}, 0)
		require.NoError(t, err)

		err = cache.Set(ctx, []string{"temp1"}, [][]byte{[]byte("temporary")}, 50*time.Millisecond)
		require.NoError(t, err)

		// Wait for temporary to expire
		time.Sleep(60 * time.Millisecond)

		// Check both
		result, err := cache.Get(ctx, []string{"perm1", "temp1"})
		require.NoError(t, err)
		assert.Equal(t, "permanent", string(result[0])) // Still exists
		assert.Nil(t, result[1])                        // Expired
	})

	t.Run("ThreadSafety", func(t *testing.T) {
		// Test concurrent access
		done := make(chan bool)

		// Writer goroutine
		go func() {
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("concurrent_%d", i)
				value := fmt.Sprintf("value_%d", i)
				err := cache.Set(ctx, []string{key}, [][]byte{[]byte(value)}, 0)
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
		err := cache.Set(ctx, []string{"exist1", "exist3"}, [][]byte{[]byte("data1"), []byte("data3")}, 0)
		require.NoError(t, err)

		// Request mix of existing and non-existing keys
		keys := []string{"exist1", "missing1", "exist3", "missing2", "missing3"}
		result, err := cache.Get(ctx, keys)
		require.NoError(t, err)

		// Verify length matches exactly
		assert.Len(t, result, len(keys), "Result length must match keys length")
		assert.Len(t, result, 5, "Should return exactly 5 results")

		// Verify correct values
		assert.Equal(t, "data1", string(result[0])) // exist1
		assert.Nil(t, result[1])                    // missing1
		assert.Equal(t, "data3", string(result[2])) // exist3
		assert.Nil(t, result[3])                    // missing2
		assert.Nil(t, result[4])                    // missing3

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
