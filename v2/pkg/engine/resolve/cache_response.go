package resolve

import "time"

// CacheResponseInfo is the ONE client-facing cache answer of a request: what
// the parts of the response allow a downstream cache — the client, a CDN — to
// do with it. The integrator reads it once, after resolution, and turns it into
// response headers; the engine itself never emits a header.
//
// Reading it:
//
//	!HasPolicy          the operation cached nothing at all — emit NO header
//	NoStore             some part of the response was not cacheable
//	otherwise           max-age is MaxAge, floored to seconds
//	Private             a part belongs to one requester: add the private token
//	Tags                the entries the response was built from, for a
//	                    tag-purging CDN; empty unless tag emission is enabled
type CacheResponseInfo struct {
	// HasPolicy reports that at least one cache-configured fetch contributed.
	// It is false for an operation whose fetches are none of the cache's
	// business — which is NOT the same as an operation that may not be stored.
	HasPolicy bool
	// MaxAge is the minimum remaining freshness over the contributing parts: a
	// served entry contributes what is left of its lifetime, a freshly stored
	// one the lifetime it was written under. Meaningless when NoStore.
	MaxAge time.Duration
	// Private marks a response with a part that belongs to one requester.
	Private bool
	// NoStore marks a response with a part that was not cacheable at all, which
	// is the most restrictive answer and outranks MaxAge.
	NoStore bool
	// Tags is the union of the invalidation tags of every entry the response
	// was built from, sorted, so one purge reaches the CDN copy and the store
	// entries alike.
	Tags []string
}

// CacheResponseInfoSource is the per-request producer of the client cache
// answer, implemented by the caching controller's request surface and installed
// on the Context when it begins. It is deliberately separate from the
// RequestCache field: the answer is read AFTER EndRequest, which releases that
// one.
type CacheResponseInfoSource interface {
	CacheResponseInfo() CacheResponseInfo
}
