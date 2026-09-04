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
		{name: "field-specific no-cache", cacheControl: `max-age=60, no-cache="Set-Cookie"`, defaultTTL: defaultTTL},
		{name: "private", cacheControl: "max-age=60, private", defaultTTL: defaultTTL},
		{name: "private takes precedence after public", cacheControl: "public, private", defaultTTL: defaultTTL},
		{name: "private takes precedence before public", cacheControl: "private, public", defaultTTL: defaultTTL},

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

			ttl, ok := TTL(headers, tc.defaultTTL)
			assert.Equal(t, tc.wantTTL > 0, ok)
			assert.Equal(t, tc.wantTTL, ttl)
		})
	}
}
