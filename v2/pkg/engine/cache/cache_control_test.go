package cache

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestParseResponseCacheControl pins the response-header parser: which
// directives are recognized, how tolerant the syntax handling is, and that a
// malformed value degrades to "silent" instead of to an error.
func TestParseResponseCacheControl(t *testing.T) {
	rows := []struct {
		name string
		// header is the full response header the parser reads.
		header http.Header
		want   responseCacheControl
	}{
		{
			name:   "a nil header says nothing",
			header: nil,
			want:   responseCacheControl{},
		},
		{
			name:   "a header without Cache-Control says nothing",
			header: http.Header{"Content-Type": []string{"application/json"}},
			want:   responseCacheControl{},
		},
		{
			name:   "max-age carries the lifetime",
			header: http.Header{"Cache-Control": []string{"max-age=60"}},
			want:   responseCacheControl{MaxAge: time.Minute, HasMaxAge: true},
		},
		{
			name:   "directive names are case-insensitive",
			header: http.Header{"Cache-Control": []string{"MAX-AGE=60"}},
			want:   responseCacheControl{MaxAge: time.Minute, HasMaxAge: true},
		},
		{
			name:   "whitespace around the name and the value is ignored",
			header: http.Header{"Cache-Control": []string{"  max-age =  60  "}},
			want:   responseCacheControl{MaxAge: time.Minute, HasMaxAge: true},
		},
		{
			name:   "a quoted value is unwrapped",
			header: http.Header{"Cache-Control": []string{`max-age="60"`}},
			want:   responseCacheControl{MaxAge: time.Minute, HasMaxAge: true},
		},
		{
			name:   "max-age zero is well-formed and means do not keep this",
			header: http.Header{"Cache-Control": []string{"max-age=0"}},
			want:   responseCacheControl{MaxAge: 0, HasMaxAge: true},
		},
		{
			name:   "a non-numeric max-age is dropped",
			header: http.Header{"Cache-Control": []string{"max-age=soon"}},
			want:   responseCacheControl{},
		},
		{
			name:   "a fractional max-age is dropped",
			header: http.Header{"Cache-Control": []string{"max-age=1.5"}},
			want:   responseCacheControl{},
		},
		{
			name:   "a negative max-age is dropped",
			header: http.Header{"Cache-Control": []string{"max-age=-5"}},
			want:   responseCacheControl{},
		},
		{
			name:   "an empty max-age value is dropped",
			header: http.Header{"Cache-Control": []string{"max-age="}},
			want:   responseCacheControl{},
		},
		{
			name:   "a valueless max-age is dropped",
			header: http.Header{"Cache-Control": []string{"max-age"}},
			want:   responseCacheControl{},
		},
		{
			name:   "s-maxage is deliberately ignored and never read as max-age",
			header: http.Header{"Cache-Control": []string{"s-maxage=60"}},
			want:   responseCacheControl{},
		},
		{
			name:   "s-maxage alongside max-age leaves max-age in charge",
			header: http.Header{"Cache-Control": []string{"s-maxage=600, max-age=30"}},
			want:   responseCacheControl{MaxAge: 30 * time.Second, HasMaxAge: true},
		},
		{
			name:   "no-store forbids storing",
			header: http.Header{"Cache-Control": []string{"no-store"}},
			want:   responseCacheControl{NoStore: true},
		},
		{
			name:   "no-cache is read as no-store: this cache cannot revalidate",
			header: http.Header{"Cache-Control": []string{"no-cache"}},
			want:   responseCacheControl{NoStore: true},
		},
		{
			name:   "no-store wins over a max-age on the same line",
			header: http.Header{"Cache-Control": []string{"max-age=60, no-store"}},
			want:   responseCacheControl{MaxAge: time.Minute, HasMaxAge: true, NoStore: true},
		},
		{
			name:   "private marks a requester-specific result",
			header: http.Header{"Cache-Control": []string{"private, max-age=60"}},
			want:   responseCacheControl{MaxAge: time.Minute, HasMaxAge: true, Private: true},
		},
		{
			name:   "public is accepted and changes nothing",
			header: http.Header{"Cache-Control": []string{"public, max-age=60"}},
			want:   responseCacheControl{MaxAge: time.Minute, HasMaxAge: true},
		},
		{
			name:   "unknown directives are skipped",
			header: http.Header{"Cache-Control": []string{"stale-while-revalidate=30, immutable, max-age=60"}},
			want:   responseCacheControl{MaxAge: time.Minute, HasMaxAge: true},
		},
		{
			name:   "the first well-formed max-age wins over a later duplicate",
			header: http.Header{"Cache-Control": []string{"max-age=60, max-age=120"}},
			want:   responseCacheControl{MaxAge: time.Minute, HasMaxAge: true},
		},
		{
			name:   "a leading malformed max-age does not block a later well-formed one",
			header: http.Header{"Cache-Control": []string{"max-age=soon, max-age=120"}},
			want:   responseCacheControl{MaxAge: 2 * time.Minute, HasMaxAge: true},
		},
		{
			name:   "empty directives from stray commas are skipped",
			header: http.Header{"Cache-Control": []string{",, max-age=60 ,,"}},
			want:   responseCacheControl{MaxAge: time.Minute, HasMaxAge: true},
		},
		{
			name: "directives spread over several header lines all count",
			header: http.Header{"Cache-Control": []string{
				"max-age=60",
				"private",
			}},
			want: responseCacheControl{MaxAge: time.Minute, HasMaxAge: true, Private: true},
		},
		{
			name:   "every recognized directive at once",
			header: http.Header{"Cache-Control": []string{"no-store, no-cache, private, public, max-age=10"}},
			want:   responseCacheControl{MaxAge: 10 * time.Second, HasMaxAge: true, NoStore: true, Private: true},
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			assert.Equal(t, row.want, parseResponseCacheControl(row.header))
		})
	}
}

// TestResolveCaching pins the two-tier storability ladder: header over static,
// the MaxTTL clamp over both, the no-store kill of BOTH layers, and the
// runtime-private store veto that leaves the request-lifetime layer alone.
func TestResolveCaching(t *testing.T) {
	rows := []struct {
		name string
		cc   responseCacheControl
		in   cachingInput
		want cachingDecision
	}{
		{
			name: "no header falls through to the static tier",
			cc:   responseCacheControl{},
			in:   cachingInput{L1: true, L2: true, StaticTTL: time.Minute},
			want: cachingDecision{L1: true, TTL: time.Minute, Scope: cacheScopePublic},
		},
		{
			name: "header max-age beats the static tier",
			cc:   responseCacheControl{MaxAge: 2 * time.Minute, HasMaxAge: true},
			in:   cachingInput{L1: true, L2: true, StaticTTL: time.Minute},
			want: cachingDecision{L1: true, TTL: 2 * time.Minute, Scope: cacheScopePublic},
		},
		{
			name: "header max-age of zero stops the store write but keeps L1",
			cc:   responseCacheControl{MaxAge: 0, HasMaxAge: true},
			in:   cachingInput{L1: true, L2: true, StaticTTL: time.Minute},
			want: cachingDecision{L1: true, TTL: 0, Scope: cacheScopePublic},
		},
		{
			name: "a static TTL of zero stops the store write but keeps L1",
			cc:   responseCacheControl{},
			in:   cachingInput{L1: true, L2: true, StaticTTL: 0},
			want: cachingDecision{L1: true, TTL: 0, Scope: cacheScopePublic},
		},
		{
			name: "header max-age enables a store write the static tier alone would not",
			cc:   responseCacheControl{MaxAge: time.Minute, HasMaxAge: true},
			in:   cachingInput{L1: true, L2: true, StaticTTL: 0, NegativeTTL: 5 * time.Second},
			want: cachingDecision{L1: true, TTL: time.Minute, NegativeTTL: 5 * time.Second, Scope: cacheScopePublic},
		},
		{
			name: "no-store clears both layers, TTLs and all",
			cc:   responseCacheControl{NoStore: true, MaxAge: time.Minute, HasMaxAge: true},
			in:   cachingInput{L1: true, L2: true, StaticTTL: time.Minute, NegativeTTL: 5 * time.Second},
			want: cachingDecision{},
		},
		{
			name: "MaxTTL clamps the header tier",
			cc:   responseCacheControl{MaxAge: 10 * time.Minute, HasMaxAge: true},
			in:   cachingInput{L1: true, L2: true, StaticTTL: time.Minute, MaxTTL: time.Minute},
			want: cachingDecision{L1: true, TTL: time.Minute, Scope: cacheScopePublic},
		},
		{
			name: "MaxTTL clamps the static tier",
			cc:   responseCacheControl{},
			in:   cachingInput{L1: true, L2: true, StaticTTL: 10 * time.Minute, MaxTTL: time.Minute},
			want: cachingDecision{L1: true, TTL: time.Minute, Scope: cacheScopePublic},
		},
		{
			name: "MaxTTL above the winning tier changes nothing",
			cc:   responseCacheControl{MaxAge: time.Minute, HasMaxAge: true},
			in:   cachingInput{L1: true, L2: true, StaticTTL: 10 * time.Minute, MaxTTL: time.Hour},
			want: cachingDecision{L1: true, TTL: time.Minute, Scope: cacheScopePublic},
		},
		{
			name: "MaxTTL clamps the negative sentinel too",
			cc:   responseCacheControl{},
			in:   cachingInput{L1: true, L2: true, StaticTTL: time.Minute, NegativeTTL: time.Hour, MaxTTL: 30 * time.Second},
			want: cachingDecision{L1: true, TTL: 30 * time.Second, NegativeTTL: 30 * time.Second, Scope: cacheScopePublic},
		},
		{
			name: "the sentinel lifetime never comes from max-age",
			cc:   responseCacheControl{MaxAge: time.Hour, HasMaxAge: true},
			in:   cachingInput{L1: true, L2: true, StaticTTL: time.Minute, NegativeTTL: 5 * time.Second},
			want: cachingDecision{L1: true, TTL: time.Hour, NegativeTTL: 5 * time.Second, Scope: cacheScopePublic},
		},
		{
			name: "an L1-only fetch derives no store TTLs and owes no private hint",
			cc:   responseCacheControl{MaxAge: time.Minute, HasMaxAge: true, Private: true},
			in:   cachingInput{L1: true, L2: false, StaticTTL: time.Minute, NegativeTTL: 5 * time.Second},
			want: cachingDecision{L1: true, Private: true, Scope: cacheScopePublic},
		},
		{
			name: "a runtime-private result keeps L1, writes nothing, and owes one hint",
			cc:   responseCacheControl{MaxAge: time.Minute, HasMaxAge: true, Private: true},
			in:   cachingInput{L1: true, L2: true, StaticTTL: time.Minute, NegativeTTL: 5 * time.Second},
			want: cachingDecision{L1: true, UncacheablePrivate: true, Private: true, Scope: cacheScopePublic},
		},
		{
			name: "no-store on a private result suppresses the hint along with the layers",
			cc:   responseCacheControl{NoStore: true, Private: true},
			in:   cachingInput{L1: true, L2: true, StaticTTL: time.Minute, NegativeTTL: 5 * time.Second},
			want: cachingDecision{},
		},
		{
			// The declared scope already partitions the keys, so the header only
			// confirms it: the entries are written, and no hint is owed.
			name: "a private header on a statically-private fetch changes nothing",
			cc:   responseCacheControl{MaxAge: time.Minute, HasMaxAge: true, Private: true},
			in:   cachingInput{L1: true, L2: true, Private: true, StaticTTL: 5 * time.Minute, NegativeTTL: 5 * time.Second},
			want: cachingDecision{L1: true, TTL: time.Minute, NegativeTTL: 5 * time.Second, Private: true, Scope: cacheScopePrivate},
		},
		{
			name: "a statically-private fetch records the private scope without any header",
			cc:   responseCacheControl{},
			in:   cachingInput{L1: true, L2: true, Private: true, StaticTTL: time.Minute},
			want: cachingDecision{L1: true, TTL: time.Minute, Private: true, Scope: cacheScopePrivate},
		},
		{
			// Storability outranks identity: no-store is about keeping the result
			// at all, not about whose result it is.
			name: "no-store kills a statically-private fetch's writes too",
			cc:   responseCacheControl{NoStore: true},
			in:   cachingInput{L1: true, L2: true, Private: true, StaticTTL: time.Minute, NegativeTTL: 5 * time.Second},
			want: cachingDecision{},
		},
		{
			name: "a root-field fetch carries no L1 and resolves its coordinate TTL",
			cc:   responseCacheControl{},
			in:   cachingInput{L1: false, L2: true, StaticTTL: 5 * time.Minute},
			want: cachingDecision{L1: false, TTL: 5 * time.Minute, Scope: cacheScopePublic},
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			assert.Equal(t, row.want, resolveCaching(row.cc, row.in))
		})
	}
}
