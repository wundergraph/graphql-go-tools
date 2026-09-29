package caching

import (
	"net/http"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache"
)

// StoreDecision is what became of a response on its way into the cache.
type StoreDecision uint8

const (
	// StoreDecisionNone is a response nothing was decided for: the fetch was
	// answered from the cache, or is not cached at all.
	StoreDecisionNone StoreDecision = iota
	StoreDecisionStored
	// StoreDecisionFetchFailed is a fetch that failed or was answered with an error status.
	StoreDecisionFetchFailed
	// StoreDecisionResponseErrors is a response carrying GraphQL errors.
	StoreDecisionResponseErrors
	// StoreDecisionInvalidResponse is a response that does not line up with what was asked.
	StoreDecisionInvalidResponse
	// StoreDecisionInvalidCacheControl is a Cache-Control that could not be parsed.
	StoreDecisionInvalidCacheControl
	StoreDecisionNoStore
	StoreDecisionNoCache
	// StoreDecisionNoDirective is a response without a caching directive, which opts out of the default TTL.
	StoreDecisionNoDirective
	// StoreDecisionNoLifetime is a lifetime of zero or less.
	StoreDecisionNoLifetime
	// StoreDecisionVary is a Vary that matches no request, or names too many headers.
	StoreDecisionVary
	// StoreDecisionPrivateWithoutID is a private response to a request without a user id.
	StoreDecisionPrivateWithoutID
	// StoreDecisionNoEntity is a response with nothing in it to store.
	StoreDecisionNoEntity
)

func (d StoreDecision) String() string {
	switch d {
	case StoreDecisionStored:
		return "stored"
	case StoreDecisionFetchFailed:
		return "fetch_failed"
	case StoreDecisionResponseErrors:
		return "response_errors"
	case StoreDecisionInvalidResponse:
		return "invalid_response"
	case StoreDecisionInvalidCacheControl:
		return "invalid_cache_control"
	case StoreDecisionNoStore:
		return "no_store"
	case StoreDecisionNoCache:
		return "no_cache"
	case StoreDecisionNoDirective:
		return "no_directive"
	case StoreDecisionNoLifetime:
		return "no_lifetime"
	case StoreDecisionVary:
		return "vary"
	case StoreDecisionPrivateWithoutID:
		return "private_without_id"
	case StoreDecisionNoEntity:
		return "no_entity"
	default:
		return ""
	}
}

// TTL is Lifetime for a caller that only needs to know whether to store.
func TTL(headers http.Header, defaultTTL time.Duration) (ttl time.Duration, private bool, ok bool) {
	ttl, private, decision := Lifetime(headers, defaultTTL)
	return ttl, private, decision == StoreDecisionStored
}

// Lifetime reads how long a response may be stored for. The decision is
// StoreDecisionStored, or the reason the response is not to be stored.
func Lifetime(headers http.Header, defaultTTL time.Duration) (ttl time.Duration, private bool, decision StoreDecision) {
	cc, err := cache.ParseCacheControlResponse(headers)
	if err != nil {
		return 0, false, StoreDecisionInvalidCacheControl
	}

	if cc.NoStore {
		return 0, false, StoreDecisionNoStore
	}

	// no-cache is not reusable by this cache in any form.
	if cc.NoCache != nil {
		return 0, false, StoreDecisionNoCache
	}

	private = cc.Private != nil

	// Explicit freshness takes precedence over the configured fallback.
	switch {
	case cc.SMaxAge != nil:
		if *cc.SMaxAge <= 0 {
			return 0, false, StoreDecisionNoLifetime
		}
		return cc.SMaxAge.AsDuration(), private, StoreDecisionStored

	case cc.MaxAge != nil:
		if *cc.MaxAge <= 0 {
			return 0, false, StoreDecisionNoLifetime
		}
		return cc.MaxAge.AsDuration(), private, StoreDecisionStored
	}

	// Only recognized cache directives opt into the configured fallback.
	if !cc.HasCachingDirectives() {
		return 0, false, StoreDecisionNoDirective
	}

	if defaultTTL <= 0 {
		return 0, false, StoreDecisionNoLifetime
	}
	return defaultTTL, private, StoreDecisionStored
}
