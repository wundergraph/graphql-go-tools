package caching

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache"
)

//go:generate go tool stringer -type=StoreDecision -linecomment -output=storedecision_string.go

// StoreDecision is what became of a response on its way into the cache.
// Order matters: lower wins when merged. Insert new values in check order.
type StoreDecision uint8

const (
	// StoreDecisionNone is a response nothing was decided for: the fetch was
	// answered from the cache, or is not cached at all.
	StoreDecisionNone                StoreDecision = iota // none
	StoreDecisionStored                                   // stored
	StoreDecisionFetchFailed                              // fetch_failed
	StoreDecisionUncacheableStatus                        // uncacheable_status
	StoreDecisionResponseErrors                           // response_errors
	StoreDecisionInvalidResponse                          // invalid_response
	StoreDecisionInvalidCacheControl                      // invalid_cache_control
	StoreDecisionNoStore                                  // no_store
	StoreDecisionNoCache                                  // no_cache
	StoreDecisionNoDirective                              // no_directive
	StoreDecisionNoLifetime                               // no_lifetime
	StoreDecisionUnusableVary                             // unusable_vary
	StoreDecisionPrivateWithoutID                         // private_without_id
	StoreDecisionNoEntity                                 // no_entity
)

// CacheableStatus reports whether a response with this status may be stored:
// 200 or 203, or 0 from a data source without HTTP. A cache without Range or
// validation must not store 206 or 304 (RFC 9111 §3).
func CacheableStatus(code int) bool {
	return code == 0 || code == http.StatusOK || code == http.StatusNonAuthoritativeInfo
}

// Freshness is what a response allows a cache to do with it.
type Freshness struct {
	TTL      time.Duration
	Private  bool
	Decision StoreDecision
	// Public is the response's own public directive.
	Public bool
	// SharedOnly is a shareable response with no max-age: s-maxage, or public
	// on the fallback. Its lifetime is for shared caches only. Without public a
	// fallback lifetime stays a max-age, as s-maxage would grant sharing.
	SharedOnly bool
	// ClientShortfall is how much less than TTL a client may keep it: the
	// response's max-age, net of Age, under the s-maxage TTL was taken from.
	ClientShortfall time.Duration
}

// FreshnessOf reads how long a response may be stored for, and what a client
// may be told of it. The decision is StoreDecisionStored, or the reason the
// response is not to be stored. sent is the request
// the subgraph answered: one carrying Authorization is only shared when the
// response allows it (RFC 9111 §3.5), and is otherwise private.
func FreshnessOf(headers, sent http.Header, defaultTTL time.Duration) Freshness {
	cc, err := cache.ParseCacheControlResponse(headers)
	if err != nil {
		return Freshness{Decision: StoreDecisionInvalidCacheControl}
	}

	if cc.NoStore {
		return Freshness{Decision: StoreDecisionNoStore}
	}

	// Unqualified no-cache is not reusable in any form. no-cache="field" only
	// bars the named headers, and entries store no headers.
	if cc.NoCache != nil && !cc.NoCache.Qualified() {
		return Freshness{Decision: StoreDecisionNoCache}
	}

	private := cc.Private != nil
	if sent.Get("Authorization") != "" && !cc.Public && cc.SMaxAge == nil && !cc.MustRevalidate {
		private = true
	}

	// Explicit freshness takes precedence over the configured fallback.
	var lifetime time.Duration
	var fromExpires bool
	switch {
	case cc.SMaxAge != nil:
		lifetime = cc.SMaxAge.AsDuration()
	case cc.MaxAge != nil:
		lifetime = cc.MaxAge.AsDuration()
	case len(headers.Values("Expires")) > 0:
		lifetime, fromExpires = expiresLifetime(headers), true
	case !cc.HasCachingDirectives():
		// Only recognized cache directives opt into the configured fallback.
		return Freshness{Decision: StoreDecisionNoDirective}
	default:
		lifetime = defaultTTL
	}

	if lifetime <= 0 {
		return Freshness{Decision: StoreDecisionNoLifetime}
	}
	// Time already spent in upstream caches is gone (RFC 9111 §4.2.3).
	spent := age(headers)
	ttl := lifetime - spent
	if ttl <= 0 {
		return Freshness{Decision: StoreDecisionNoLifetime}
	}

	f := Freshness{TTL: ttl, Private: private, Decision: StoreDecisionStored, Public: cc.Public}
	// An Expires lifetime is the client's too.
	f.SharedOnly = !private && cc.MaxAge == nil && !fromExpires && (cc.SMaxAge != nil || cc.Public)
	if cc.MaxAge != nil {
		if client := max(cc.MaxAge.AsDuration()-spent, 0); client < ttl {
			f.ClientShortfall = ttl - client
		}
	}
	return f
}

// expiresLifetime is Expires less Date, or less now without a Date. An invalid
// Expires, "0" included, is already expired (RFC 9111 §5.3).
func expiresLifetime(headers http.Header) time.Duration {
	expires, err := http.ParseTime(headers.Get("Expires"))
	if err != nil {
		return 0
	}
	date, err := http.ParseTime(headers.Get("Date"))
	if err != nil {
		date = time.Now()
	}
	return expires.Sub(date)
}

// age is the Age header, zero when absent or invalid. A list uses its first
// member (RFC 9111 §5.1). ParseUint takes digits only, as delta-seconds is;
// overflow is older than anything storable.
func age(headers http.Header) time.Duration {
	first, _, _ := strings.Cut(headers.Get("Age"), ",")
	seconds, err := strconv.ParseUint(strings.TrimSpace(first), 10, 64)
	if errors.Is(err, strconv.ErrRange) || seconds > math.MaxInt64/uint64(time.Second) {
		return math.MaxInt64
	}
	if err != nil {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
