package caching

import (
	"net/http"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache"
)

// TTL reads the lifetime a subgraph response may be cached for. private
// reports that the response is user-specific and may only be stored under a
// key scoped to the user it was produced for; bare and qualified forms both
// count, and private opts into caching like any other recognized directive.
func TTL(headers http.Header, defaultTTL time.Duration) (ttl time.Duration, private bool, ok bool) {
	cc, err := cache.ParseCacheControlResponse(headers)
	if err != nil {
		return 0, false, false
	}

	if cc.NoStore {
		return 0, false, false
	}

	// no-cache is not reusable by this cache in any form.
	if cc.NoCache != nil {
		return 0, false, false
	}

	private = cc.Private != nil

	// Explicit freshness takes precedence over the configured fallback.
	switch {
	case cc.SMaxAge != nil:
		if *cc.SMaxAge <= 0 {
			return 0, false, false
		}
		return cc.SMaxAge.AsDuration(), private, true

	case cc.MaxAge != nil:
		if *cc.MaxAge <= 0 {
			return 0, false, false
		}
		return cc.MaxAge.AsDuration(), private, true
	}

	// Only recognized cache directives opt into the configured fallback.
	if !cc.HasCachingDirectives() {
		return 0, false, false
	}

	if defaultTTL <= 0 {
		return 0, false, false
	}
	return defaultTTL, private, true
}
