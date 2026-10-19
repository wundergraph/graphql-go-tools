package caching

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTTL(t *testing.T) {
	t.Parallel()

	const defaultTTL = 5 * time.Minute

	for _, tc := range []struct {
		name         string
		cacheControl string
		defaultTTL   time.Duration
		wantTTL      time.Duration // Zero means the response should not be cached.
		wantPrivate  bool
	}{
		// A positive freshness lifetime is sufficient to store a response. Public
		// permits storage in cases that would otherwise forbid it; it is not a
		// general prerequisite for shared caching.
		{name: "max-age without public", cacheControl: "max-age=60", defaultTTL: defaultTTL, wantTTL: time.Minute},
		{name: "s-maxage without public", cacheControl: "s-maxage=60", defaultTTL: defaultTTL, wantTTL: time.Minute},
		{name: "public with max-age", cacheControl: "public, max-age=60", defaultTTL: defaultTTL, wantTTL: time.Minute},
		{name: "public with s-maxage", cacheControl: "public, s-maxage=60", defaultTTL: defaultTTL, wantTTL: time.Minute},
		{name: "max-age does not need a usable default", cacheControl: "max-age=60", wantTTL: time.Minute},

		// s-maxage is the directive addressed to shared caches, so it outranks
		// max-age here however the two are ordered in the header.
		{name: "s-maxage outranks max-age", cacheControl: "s-maxage=60, max-age=3600", defaultTTL: defaultTTL, wantTTL: time.Minute},
		{name: "s-maxage outranks max-age reversed", cacheControl: "max-age=3600, s-maxage=60", defaultTTL: defaultTTL, wantTTL: time.Minute},
		{name: "zero s-maxage overrides positive max-age", cacheControl: "max-age=60, s-maxage=0", defaultTTL: defaultTTL},
		{name: "positive s-maxage overrides zero max-age", cacheControl: "max-age=0, s-maxage=60", defaultTTL: defaultTTL, wantTTL: time.Minute},

		// A recognized caching directive without a freshness lifetime opts into
		// the configured fallback. Public is one such directive, not a requirement.
		{name: "public alone", cacheControl: "public", defaultTTL: defaultTTL, wantTTL: defaultTTL},
		{name: "must-revalidate without public", cacheControl: "must-revalidate", defaultTTL: defaultTTL, wantTTL: defaultTTL},
		{name: "proxy-revalidate without public", cacheControl: "proxy-revalidate", defaultTTL: defaultTTL, wantTTL: defaultTTL},
		{name: "bare stale-if-error", cacheControl: "stale-if-error", defaultTTL: defaultTTL},
		{name: "stale-if-error without public", cacheControl: "stale-if-error=60", defaultTTL: defaultTTL, wantTTL: defaultTTL},
		{name: "stale-while-revalidate without public", cacheControl: "stale-while-revalidate=60", defaultTTL: defaultTTL, wantTTL: defaultTTL},

		// No header or a header without recognized caching intent does not use the
		// configured fallback.
		{name: "no cache-control header", defaultTTL: defaultTTL},
		{name: "extension directive without public", cacheControl: "cdn-cache-control=60", defaultTTL: defaultTTL},
		{name: "immutable without public", cacheControl: "immutable", defaultTTL: defaultTTL},
		{name: "no-transform without public", cacheControl: "no-transform", defaultTTL: defaultTTL},
		{name: "must-understand without public", cacheControl: "must-understand", defaultTTL: defaultTTL},

		// Explicit refusals win whether public is present or not.
		{name: "no-store", cacheControl: "max-age=60, no-store", defaultTTL: defaultTTL},
		{name: "no-cache", cacheControl: "max-age=60, no-cache", defaultTTL: defaultTTL},

		// no-cache="field" only bars the named headers, which entries don't store.
		{name: "field-specific no-cache", cacheControl: `max-age=60, no-cache="Set-Cookie"`, defaultTTL: defaultTTL, wantTTL: time.Minute},
		{name: "field-specific no-cache falls back to the default", cacheControl: `no-cache="Set-Cookie"`, defaultTTL: defaultTTL, wantTTL: defaultTTL},
		{name: "unqualified no-cache outranks field-specific", cacheControl: `no-cache="Set-Cookie", max-age=60, no-cache`, defaultTTL: defaultTTL},
		{name: "empty no-cache field list", cacheControl: `max-age=60, no-cache=""`, defaultTTL: defaultTTL},

		// private is cacheable, but only under a user-scoped key. It opts in
		// like any other recognized directive, follows the same lifetime rules
		// and does not override an explicit refusal.
		{name: "private alone", cacheControl: "private", defaultTTL: defaultTTL, wantTTL: defaultTTL, wantPrivate: true},
		{name: "private with max-age", cacheControl: "private, max-age=60", defaultTTL: defaultTTL, wantTTL: time.Minute, wantPrivate: true},
		{name: "private with s-maxage", cacheControl: "private, s-maxage=30, max-age=60", defaultTTL: defaultTTL, wantTTL: 30 * time.Second, wantPrivate: true},
		{name: "private after public", cacheControl: "public, private", defaultTTL: defaultTTL, wantTTL: defaultTTL, wantPrivate: true},
		{name: "private before public", cacheControl: "private, public", defaultTTL: defaultTTL, wantTTL: defaultTTL, wantPrivate: true},
		{name: "field-specific private", cacheControl: `private="Set-Cookie", max-age=60`, defaultTTL: defaultTTL, wantTTL: time.Minute, wantPrivate: true},
		{name: "private with no-store", cacheControl: "private, max-age=60, no-store", defaultTTL: defaultTTL},
		{name: "private with no-cache", cacheControl: "private, max-age=60, no-cache", defaultTTL: defaultTTL},
		{name: "private with zero max-age", cacheControl: "private, max-age=0", defaultTTL: defaultTTL},
		{name: "private with non-positive default", cacheControl: "private"},
		{name: "public is not private", cacheControl: "public, max-age=60", defaultTTL: defaultTTL, wantTTL: time.Minute},

		// A lifetime of zero is a refusal, and malformed lifetimes cannot opt a
		// response into caching.
		{name: "zero max-age", cacheControl: "max-age=0", defaultTTL: defaultTTL},
		{name: "zero s-maxage", cacheControl: "s-maxage=0", defaultTTL: defaultTTL},
		{name: "public does not override zero max-age", cacheControl: "public, max-age=0", defaultTTL: defaultTTL},
		{name: "invalid max-age", cacheControl: "max-age=abc", defaultTTL: defaultTTL},
		{name: "invalid stale-if-error", cacheControl: "stale-if-error=abc", defaultTTL: defaultTTL},
		{name: "invalid stale-while-revalidate", cacheControl: "stale-while-revalidate=abc", defaultTTL: defaultTTL},

		// The fallback must itself be usable.
		{name: "caching intent with zero default", cacheControl: "must-revalidate"},
		{name: "caching intent with negative default", cacheControl: "must-revalidate", defaultTTL: -time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			headers := http.Header{}
			if tc.cacheControl != "" {
				headers.Set("Cache-Control", tc.cacheControl)
			}

			f := FreshnessOf(headers, nil, tc.defaultTTL)
			ttl, private, ok := f.TTL, f.Private, f.Decision == StoreDecisionStored
			assert.Equal(t, tc.wantTTL > 0, ok)
			assert.Equal(t, tc.wantTTL, ttl)
			assert.Equal(t, tc.wantPrivate, private)
		})
	}
}

func TestTTLAge(t *testing.T) {
	t.Parallel()

	const defaultTTL = 30 * time.Second

	for _, tc := range []struct {
		name         string
		cacheControl string
		age          string
		wantTTL      time.Duration // Zero means the response should not be cached.
	}{
		{name: "age shortens max-age", cacheControl: "max-age=45", age: "40", wantTTL: 5 * time.Second},
		{name: "age shortens s-maxage", cacheControl: "s-maxage=10, max-age=45", age: "4", wantTTL: 6 * time.Second},
		{name: "age shortens the default", cacheControl: "public", age: "10", wantTTL: 20 * time.Second},
		{name: "age past max-age", cacheControl: "max-age=10", age: "30"},
		{name: "age equal to max-age", cacheControl: "max-age=10", age: "10"},
		{name: "zero age", cacheControl: "max-age=10", age: "0", wantTTL: 10 * time.Second},
		{name: "age with whitespace", cacheControl: "max-age=10", age: " 3 ", wantTTL: 7 * time.Second},
		{name: "invalid age is ignored", cacheControl: "max-age=10", age: "soon", wantTTL: 10 * time.Second},
		{name: "negative age is ignored", cacheControl: "max-age=10", age: "-5", wantTTL: 10 * time.Second},
		{name: "overflowing age", cacheControl: "max-age=10", age: "99999999999999999999"},
		{name: "age without a header", age: "5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			headers := http.Header{}
			if tc.cacheControl != "" {
				headers.Set("Cache-Control", tc.cacheControl)
			}
			headers.Set("Age", tc.age)

			f := FreshnessOf(headers, nil, defaultTTL)
			ttl, ok := f.TTL, f.Decision == StoreDecisionStored
			assert.Equal(t, tc.wantTTL > 0, ok)
			assert.Equal(t, tc.wantTTL, ttl)
		})
	}
}

func TestLifetime(t *testing.T) {
	t.Parallel()

	const defaultTTL = 5 * time.Minute

	for _, tc := range []struct {
		name         string
		cacheControl string
		defaultTTL   time.Duration
		want         StoreDecision
	}{
		{name: "max-age", cacheControl: "max-age=60", defaultTTL: defaultTTL, want: StoreDecisionStored},
		{name: "a directive falls back to the default", cacheControl: "public", defaultTTL: defaultTTL, want: StoreDecisionStored},
		{name: "no-store", cacheControl: "no-store, max-age=60", defaultTTL: defaultTTL, want: StoreDecisionNoStore},
		{name: "no-cache", cacheControl: "no-cache, max-age=60", defaultTTL: defaultTTL, want: StoreDecisionNoCache},
		{name: "field-specific no-cache", cacheControl: `no-cache="Set-Cookie", max-age=60`, defaultTTL: defaultTTL, want: StoreDecisionStored},
		{name: "max-age of zero", cacheControl: "max-age=0", defaultTTL: defaultTTL, want: StoreDecisionNoLifetime},
		{name: "s-maxage of zero", cacheControl: "s-maxage=0, max-age=60", defaultTTL: defaultTTL, want: StoreDecisionNoLifetime},
		{name: "a directive without a default", cacheControl: "public", want: StoreDecisionNoLifetime},
		{name: "no header", defaultTTL: defaultTTL, want: StoreDecisionNoDirective},
		{name: "an unparsable header", cacheControl: "max-age=soon", defaultTTL: defaultTTL, want: StoreDecisionInvalidCacheControl},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			headers := http.Header{}
			if tc.cacheControl != "" {
				headers.Set("Cache-Control", tc.cacheControl)
			}

			f := FreshnessOf(headers, nil, tc.defaultTTL)
			ttl, decision := f.TTL, f.Decision
			assert.Equal(t, tc.want, decision)
			assert.NotEmpty(t, decision.String())

			ok := decision == StoreDecisionStored
			assert.Equal(t, tc.want == StoreDecisionStored, ok)
			assert.Equal(t, ok, ttl > 0)
		})
	}
}

func TestFreshnessOf(t *testing.T) {
	t.Parallel()

	const defaultTTL = 30 * time.Second
	authorized := http.Header{"Authorization": {"Bearer x"}}

	for _, tc := range []struct {
		name          string
		cacheControl  string
		age           string
		sent          http.Header
		wantTTL       time.Duration
		wantPrivate   bool
		wantPublic    bool
		wantShortfall time.Duration
		wantShared    bool
	}{
		{name: "authorized without public is private", cacheControl: "max-age=60", sent: authorized, wantTTL: time.Minute, wantPrivate: true},
		{name: "authorized with public is shared", cacheControl: "public, max-age=60", sent: authorized, wantTTL: time.Minute, wantPublic: true},
		{name: "authorized with s-maxage is shared", cacheControl: "s-maxage=60", sent: authorized, wantTTL: time.Minute, wantShared: true},
		{name: "authorized with must-revalidate is shared", cacheControl: "must-revalidate, max-age=60", sent: authorized, wantTTL: time.Minute},
		{name: "unauthorized without public is shared", cacheControl: "max-age=60", wantTTL: time.Minute},
		{name: "max-age under s-maxage is the client's", cacheControl: "public, max-age=60, s-maxage=600", wantTTL: 600 * time.Second, wantPublic: true, wantShortfall: 540 * time.Second},
		{name: "max-age over s-maxage leaves no shortfall", cacheControl: "max-age=600, s-maxage=60", wantTTL: time.Minute},
		{name: "age comes off both", cacheControl: "max-age=60, s-maxage=600", age: "30", wantTTL: 570 * time.Second, wantShortfall: 540 * time.Second},
		{name: "a stale max-age leaves the client nothing", cacheControl: "max-age=10, s-maxage=600", age: "30", wantTTL: 570 * time.Second, wantShortfall: 570 * time.Second},
		{name: "no max-age, no shortfall", cacheControl: "public", wantTTL: defaultTTL, wantPublic: true, wantShared: true},
		{name: "s-maxage alone is shared only", cacheControl: "s-maxage=60", age: "20", wantTTL: 40 * time.Second, wantShared: true},
		{name: "a fallback without public is not shared only", cacheControl: "must-revalidate", wantTTL: defaultTTL},
		{name: "a private fallback is not shared only", cacheControl: "private", wantTTL: defaultTTL, wantPrivate: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			headers := http.Header{"Cache-Control": {tc.cacheControl}}
			if tc.age != "" {
				headers.Set("Age", tc.age)
			}
			f := FreshnessOf(headers, tc.sent, defaultTTL)
			assert.Equal(t, StoreDecisionStored, f.Decision)
			assert.Equal(t, tc.wantTTL, f.TTL)
			assert.Equal(t, tc.wantPrivate, f.Private)
			assert.Equal(t, tc.wantPublic, f.Public)
			assert.Equal(t, tc.wantShortfall, f.ClientShortfall)
			assert.Equal(t, tc.wantShared, f.SharedOnly)
		})
	}
}
