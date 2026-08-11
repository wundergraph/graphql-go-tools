package cache

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// The runtime tier of cache control: a subgraph's `Cache-Control` RESPONSE
// header decides an entry's lifetime, its storability, and whether the result
// may be shared at all. Static configuration is the fallback that applies only
// when the header is silent.

// responseCacheControl is what one subgraph response's `Cache-Control` header
// says, reduced to the directives the cache acts on.
type responseCacheControl struct {
	// MaxAge is the lifetime the origin declared; meaningful only with HasMaxAge.
	MaxAge time.Duration
	// HasMaxAge marks a well-formed max-age directive. A max-age of ZERO is
	// well-formed and means "do not keep this"; a MALFORMED one leaves this
	// false, so the static tier decides instead.
	HasMaxAge bool
	// NoStore forbids keeping the result in any layer. It is also set by
	// `no-cache`: honoring `no-cache` means revalidating before every serve, and
	// this cache cannot revalidate, so the conservative reading is not to store.
	NoStore bool
	// Private marks a result that belongs to one requester only.
	Private bool
}

// parseResponseCacheControl reads the recognized directives — `max-age`,
// `no-store`, `no-cache`, `private` — out of every `Cache-Control` line of a
// subgraph response; everything else, `s-maxage` included, is skipped. The
// parse never fails: a malformed directive is dropped so the static tier
// decides. Names are case-insensitive with whitespace ignored (RFC 9111 §5.2),
// values may be quoted, the FIRST well-formed max-age wins, and the
// storability directives are sticky.
func parseResponseCacheControl(header http.Header) responseCacheControl {
	var cc responseCacheControl
	for _, line := range header.Values("Cache-Control") {
		for rest := line; rest != ""; {
			var directive string
			directive, rest, _ = strings.Cut(rest, ",")
			name, value, hasValue := strings.Cut(directive, "=")
			switch strings.ToLower(strings.TrimSpace(name)) {
			case "no-store", "no-cache":
				cc.NoStore = true
			case "private":
				cc.Private = true
			case "max-age":
				if !hasValue || cc.HasMaxAge {
					continue
				}
				seconds, err := strconv.Atoi(strings.Trim(strings.TrimSpace(value), `"`))
				if err != nil || seconds < 0 {
					continue
				}
				cc.MaxAge = time.Duration(seconds) * time.Second
				cc.HasMaxAge = true
			}
		}
	}
	return cc
}

// The reasons private data never reached the store, as passed to
// resolve.CacheObserver.OnUncacheablePrivate. Each names its own remedy.
const (
	// UncacheablePrivateResponseHeader: a statically-public fetch was answered
	// with `Cache-Control: private` — declare the scope statically to cache it
	// partitioned.
	UncacheablePrivateResponseHeader = "response-private"
	// UncacheablePrivateNoIdentity: a statically-private fetch ran without a
	// requester identity — supply a PrivatePartitionProvider, or key the
	// subgraph by its forwarded headers.
	UncacheablePrivateNoIdentity = "no-identity"
)

// cachingInput is the STATIC side of the storability ladder: the layers the
// fetch is configured for and the TTLs configuration contributes. StaticTTL is
// the seam for narrower static tiers (a per-type declaration): today it always
// carries the effective subgraph — or, for root fields, coordinate — value.
type cachingInput struct {
	L1 bool
	L2 bool
	// Private marks a STATICALLY private fetch: its store keys already carry the
	// requester's partition segment, so a `private` response header only
	// CONFIRMS what the configuration declared and changes nothing.
	Private     bool
	StaticTTL   time.Duration
	NegativeTTL time.Duration
	MaxTTL      time.Duration
}

// cachingDecision is the ONE storability answer for one fetch RESULT: which
// layers may be written and under which lifetimes. A zero TTL means "no store
// write" for that kind of entry, so call sites gate on the TTLs alone.
type cachingDecision struct {
	// L1 permits request-lifetime writes. Only `no-store` clears it — L1 is
	// request-scoped, so a merely-zero TTL does not affect it.
	L1 bool
	// TTL is the store lifetime of a fetched VALUE; 0 = do not store.
	TTL time.Duration
	// NegativeTTL is the store lifetime of the empty-entity SENTINEL; 0 = do not
	// store. It never derives from max-age: an origin declaring how long a value
	// stays fresh says nothing about how long an entity stays nonexistent.
	NegativeTTL time.Duration
	// UncacheablePrivate marks a statically-public fetch the origin answered with
	// `private`: the store writes are dropped and one observability hint is owed.
	UncacheablePrivate bool
	// Private marks a result that belongs to ONE requester, whichever side said
	// so — the static declaration or the response header. It is what the client
	// cache answer reads; the key derivation reads Scope, which only the STATIC
	// declaration moves. A result that may not be stored at all reports false:
	// no-store already forbids every form of sharing.
	Private bool
	// Scope is the privacy scope the permitted writes record, so a later read
	// through the other scope's key derivation can discard them. Empty when the
	// result may not be stored at all.
	Scope string
}

// resolveCaching runs the two-tier ladder for one fetch RESULT:
//
//	no-store / no-cache  -> nothing is written, in EITHER layer
//	header max-age       -> the entry TTL (the runtime truth)
//	else                 -> the static TTL (configuration cascade)
//	resolved TTL <= 0    -> no store write (L1 is unaffected)
//	MaxTTL, when set     -> clamps whichever tier won
//
// The empty-entity sentinel takes the same storability gates but always the
// configured NegativeTTL as its lifetime. A runtime-only `private` — one the
// static configuration did not declare — keeps L1 and drops both store writes;
// statically-private fetches write into the requester's partition as usual.
// ONE Cache-Control header governs a whole batch response, so the resolved TTL
// applies to every entity entry written from it.
func resolveCaching(cc responseCacheControl, in cachingInput) cachingDecision {
	if cc.NoStore {
		return cachingDecision{}
	}
	decision := cachingDecision{
		L1: in.L1,
		// A fetch that does not participate in the store never writes one, and a
		// private answer to a statically-public fetch must not either.
		UncacheablePrivate: cc.Private && in.L2 && !in.Private,
		Private:            cc.Private || in.Private,
		Scope:              entryScope(in.Private),
	}
	if !in.L2 || decision.UncacheablePrivate {
		return decision
	}
	ttl := in.StaticTTL
	if cc.HasMaxAge {
		ttl = cc.MaxAge
	}
	decision.TTL = clampTTL(ttl, in.MaxTTL)
	decision.NegativeTTL = clampTTL(in.NegativeTTL, in.MaxTTL)
	return decision
}

// entryScope names the scope a fetch's entries are written under.
func entryScope(private bool) string {
	if private {
		return cacheScopePrivate
	}
	return cacheScopePublic
}

// clampTTL caps a resolved TTL at maxTTL, which is unset at 0, and normalizes
// every non-positive result to 0 — the single "do not store" value.
func clampTTL(ttl, maxTTL time.Duration) time.Duration {
	if ttl <= 0 {
		return 0
	}
	if maxTTL > 0 && ttl > maxTTL {
		return maxTTL
	}
	return ttl
}
