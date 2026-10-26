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
	// SharedOnly is a shareable response with no max-age or Expires: s-maxage,
	// or public on the fallback. Its lifetime is for shared caches only.
	// Without public a fallback lifetime stays a max-age, as s-maxage would
	// grant sharing.
	SharedOnly bool
	// ClientShortfall is how much less than TTL a client may keep it: the
	// response's max-age, else Expires, net of Age, under the s-maxage TTL was
	// taken from.
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
	// Time already spent in upstream caches is gone (RFC 9111 §4.2.3).
	spent := age(headers)
	hasExpires := len(headers.Values("Expires")) > 0
	var ttl time.Duration
	switch {
	case cc.SMaxAge != nil:
		ttl = cc.SMaxAge.AsDuration() - spent
	case cc.MaxAge != nil:
		ttl = cc.MaxAge.AsDuration() - spent
	case hasExpires:
		ttl = expiresTTL(headers, spent)
	case !cc.HasCachingDirectives():
		// Only recognized cache directives opt into the configured fallback.
		return Freshness{Decision: StoreDecisionNoDirective}
	default:
		ttl = defaultTTL - spent
	}
	if ttl <= 0 {
		return Freshness{Decision: StoreDecisionNoLifetime}
	}

	f := Freshness{TTL: ttl, Private: private, Decision: StoreDecisionStored, Public: cc.Public}
	// A client's lifetime is max-age, else Expires; s-maxage is for shared
	// caches only (RFC 9111 §5.2.2.10).
	var client time.Duration
	switch {
	case cc.MaxAge != nil:
		client = cc.MaxAge.AsDuration() - spent
	case hasExpires:
		client = expiresTTL(headers, spent)
	default:
		f.SharedOnly = !private && (cc.SMaxAge != nil || cc.Public)
		return f
	}
	if client = max(client, 0); client < ttl {
		f.ClientShortfall = ttl - client
	}
	return f
}

// expiresTTL is what Expires leaves after spent: Expires less Date, or less
// now without a Date, and never past Expires itself, whatever Date said. An
// invalid Expires, "0" included, is already expired (RFC 9111 §5.3).
func expiresTTL(headers http.Header, spent time.Duration) time.Duration {
	expires, err := http.ParseTime(headers.Get("Expires"))
	if err != nil {
		return 0
	}
	date, err := http.ParseTime(headers.Get("Date"))
	if err != nil {
		date = time.Now()
	}
	return min(expires.Sub(date)-spent, time.Until(expires))
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
