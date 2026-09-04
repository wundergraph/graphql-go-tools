package caching

import (
	"net/http"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache"
)

func TTL(headers http.Header, defaultTTL time.Duration) (time.Duration, bool) {
	cc, err := cache.ParseCacheControlResponse(headers)
	if err != nil {
		return 0, false
	}

	if cc.NoStore {
		return 0, false
	}

	// We currently treat no-cache and private as not reusable by this shared cache.
	if cc.NoCache != nil || cc.Private != nil {
		return 0, false
	}

	// Explicit freshness takes precedence over the configured fallback.
	switch {
	case cc.SMaxAge != nil:
		if *cc.SMaxAge <= 0 {
			return 0, false
		}
		return cc.SMaxAge.AsDuration(), true

	case cc.MaxAge != nil:
		if *cc.MaxAge <= 0 {
			return 0, false
		}
		return cc.MaxAge.AsDuration(), true
	}

	// Only recognized cache directives opt into the configured fallback.
	if !cc.HasCachingDirectives() {
		return 0, false
	}

	if defaultTTL <= 0 {
		return 0, false
	}
	return defaultTTL, true
}
