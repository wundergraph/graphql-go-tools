package cachetesting

import (
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// StoreAdapter adapts FakeStore to the cache.Store interface (remaining-TTL
// view over the store's absolute expiries).
type StoreAdapter struct {
	Store *FakeStore
}

func (s StoreAdapter) Get(key string) ([]byte, time.Duration, bool) {
	entry, ok := s.Store.Get(key)
	if !ok {
		return nil, 0, false
	}
	return entry.Value, time.Until(entry.ExpiresAt), true
}

func (s StoreAdapter) Set(key string, value []byte, ttl time.Duration) {
	s.Store.Set(key, value, ttl)
}

// NewRealishCache builds the REAL cache controller over the in-memory
// FakeStore, for end-to-end rows that exercise actual lookup/write behavior
// instead of scripted decisions. obs may be nil.
func NewRealishCache(store *FakeStore, obs resolve.CacheObserver) resolve.CacheController {
	return cache.NewController(StoreAdapter{Store: store}, obs)
}
