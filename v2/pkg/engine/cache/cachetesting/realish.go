package cachetesting

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// NewRealishCache builds the REAL cache controller over the in-memory
// FakeStore, for end-to-end rows that exercise actual lookup/write behavior
// instead of scripted decisions. obs may be nil; options carry the runtime
// knobs (cache.WithGlobalConfig).
func NewRealishCache(store *FakeStore, obs resolve.CacheObserver, options ...cache.ControllerOption) resolve.CacheController {
	return cache.NewController(store, obs, options...)
}
