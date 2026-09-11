package caching

import (
	"net/http"
	"testing"
	"time"
)

func TestTTL(t *testing.T) {
	const defaultTTL = 5 * time.Minute

	for _, tc := range []struct {
		name         string
		cacheControl string
		defaultTTL   time.Duration
		wantTTL      time.Duration
		wantOK       bool
		wantPrivate  bool
	}{
		// public is the opt-in, and on its own it takes the configured default.
		{name: "public alone", cacheControl: "public", defaultTTL: defaultTTL, wantTTL: defaultTTL, wantOK: true},
		{name: "public with max-age", cacheControl: "public, max-age=60", defaultTTL: defaultTTL, wantTTL: time.Minute, wantOK: true},
		{name: "public with s-maxage", cacheControl: "public, s-maxage=60", defaultTTL: defaultTTL, wantTTL: time.Minute, wantOK: true},

		// s-maxage is the directive addressed to shared caches, so it outranks
		// max-age here however the two are ordered in the header.
		{name: "s-maxage outranks max-age", cacheControl: "public, s-maxage=60, max-age=3600", defaultTTL: defaultTTL, wantTTL: time.Minute, wantOK: true},
		{name: "s-maxage outranks max-age reversed", cacheControl: "public, max-age=3600, s-maxage=60", defaultTTL: defaultTTL, wantTTL: time.Minute, wantOK: true},

		// Without public or private the response is not offered to this cache,
		// whatever lifetime it names.
		{name: "max-age without public", cacheControl: "max-age=60", defaultTTL: defaultTTL, wantOK: false},
		{name: "s-maxage without public", cacheControl: "s-maxage=60", defaultTTL: defaultTTL, wantOK: false},
		{name: "no cache-control header", cacheControl: "", defaultTTL: defaultTTL, wantOK: false},

		// Explicit refusals win over public.
		{name: "no-store", cacheControl: "public, no-store", defaultTTL: defaultTTL, wantOK: false},
		{name: "no-cache", cacheControl: "public, no-cache", defaultTTL: defaultTTL, wantOK: false},
		{name: "field-specific no-cache", cacheControl: `public, no-cache="Set-Cookie"`, defaultTTL: defaultTTL, wantOK: false},

		// private is cacheable, but only under a user-scoped key. It opts in
		// like public, follows the same lifetime rules and does not override an
		// explicit refusal.
		{name: "private alone", cacheControl: "private", defaultTTL: defaultTTL, wantTTL: defaultTTL, wantOK: true, wantPrivate: true},
		{name: "private with max-age", cacheControl: "private, max-age=60", defaultTTL: defaultTTL, wantTTL: time.Minute, wantOK: true, wantPrivate: true},
		{name: "private with s-maxage", cacheControl: "private, s-maxage=30, max-age=60", defaultTTL: defaultTTL, wantTTL: 30 * time.Second, wantOK: true, wantPrivate: true},
		{name: "private after public", cacheControl: "public, private", defaultTTL: defaultTTL, wantTTL: defaultTTL, wantOK: true, wantPrivate: true},
		{name: "private before public", cacheControl: "private, public", defaultTTL: defaultTTL, wantTTL: defaultTTL, wantOK: true, wantPrivate: true},
		{name: "field-specific private", cacheControl: `private="Set-Cookie", max-age=60`, defaultTTL: defaultTTL, wantTTL: time.Minute, wantOK: true, wantPrivate: true},
		{name: "private with no-store", cacheControl: "private, max-age=60, no-store", defaultTTL: defaultTTL, wantOK: false},
		{name: "private with no-cache", cacheControl: "private, max-age=60, no-cache", defaultTTL: defaultTTL, wantOK: false},
		{name: "private with zero max-age", cacheControl: "private, max-age=0", defaultTTL: defaultTTL, wantOK: false},
		{name: "private with non-positive default", cacheControl: "private", defaultTTL: 0, wantOK: false},
		{name: "public is not private", cacheControl: "public, max-age=60", defaultTTL: defaultTTL, wantTTL: time.Minute, wantOK: true},

		// A lifetime of zero is a refusal, and so is a default that was never
		// usable in the first place.
		{name: "zero max-age", cacheControl: "public, max-age=0", defaultTTL: defaultTTL, wantOK: false},
		{name: "zero s-maxage", cacheControl: "public, s-maxage=0", defaultTTL: defaultTTL, wantOK: false},
		{name: "public with non-positive default", cacheControl: "public", defaultTTL: 0, wantOK: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headers := http.Header{}
			if tc.cacheControl != "" {
				headers.Set("Cache-Control", tc.cacheControl)
			}

			ttl, private, ok := TTL(headers, tc.defaultTTL)
			if ok != tc.wantOK {
				t.Fatalf("TTL(%q) ok = %v, want %v", tc.cacheControl, ok, tc.wantOK)
			}
			if ok && ttl != tc.wantTTL {
				t.Fatalf("TTL(%q) = %s, want %s", tc.cacheControl, ttl, tc.wantTTL)
			}
			if !ok && ttl != 0 {
				t.Fatalf("TTL(%q) returned %s alongside ok=false, want 0", tc.cacheControl, ttl)
			}
			if private != tc.wantPrivate {
				t.Fatalf("TTL(%q) private = %v, want %v", tc.cacheControl, private, tc.wantPrivate)
			}
		})
	}
}
