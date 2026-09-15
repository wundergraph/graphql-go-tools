package caching

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		vary  []string
		names []string
		star  bool
	}{
		{name: "absent"},
		{name: "one", vary: []string{"Accept-Language"}, names: []string{"accept-language"}},
		{name: "sorted, lowercased, trimmed, deduplicated", vary: []string{" X-Region ,accept-language", "Accept-Language"}, names: []string{"accept-language", "x-region"}},
		{name: "empty members are skipped", vary: []string{", ,"}},
		{name: "star", vary: []string{"Accept-Language, *"}, star: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			headers := http.Header{}
			for _, v := range tt.vary {
				headers.Add("Vary", v)
			}
			names, star := Vary(headers)
			assert.Equal(t, tt.names, names)
			assert.Equal(t, tt.star, star)
		})
	}
}

func TestVaryDigest(t *testing.T) {
	t.Parallel()

	names := []string{"accept-language", "x-region"}
	de := http.Header{"Accept-Language": []string{"de"}, "X-Region": []string{"eu"}}

	assert.Equal(t, VaryDigest(names, de), VaryDigest(names, de))
	assert.NotEqual(t, VaryDigest(names, de), VaryDigest(names, http.Header{"Accept-Language": []string{"fr"}, "X-Region": []string{"eu"}}))
	assert.NotEqual(t, VaryDigest(names, de), VaryDigest(names[:1], de), "the names are part of the digest")

	absent := http.Header{"Accept-Language": []string{"de"}}
	assert.Equal(t, VaryDigest(names, absent), VaryDigest(names, http.Header{"Accept-Language": []string{"de"}, "X-Region": []string{""}}),
		"an absent header digests as empty")
	assert.Equal(t, VaryDigest(names, nil), VaryDigest(names, http.Header{}), "nothing sent at all")

	multi := http.Header{"Accept-Language": []string{"de", "en"}}
	assert.Equal(t, VaryDigest(names[:1], multi), VaryDigest(names[:1], http.Header{"Accept-Language": []string{"de,en"}}),
		"repeated fields join like one field")

	noncanonical := http.Header{}
	noncanonical.Set("accept-language", "de")
	assert.Equal(t, VaryDigest(names[:1], noncanonical), VaryDigest(names[:1], http.Header{"Accept-Language": []string{"de"}}))
}

func TestVariantKey(t *testing.T) {
	t.Parallel()

	base := Key(DigestString("entity"), DigestString("selection"))
	vary := DigestString("accept-language=de")
	variant := VariantKey(base, vary)

	require.True(t, len(variant) > len(base))
	assert.Equal(t, base+"+", variant[:len(base)+1])
	assert.Len(t, variant, len(base)+1+digestHex)
	assert.NotEqual(t, variant, VariantKey(base, DigestString("accept-language=fr")))
	assert.NotEqual(t, variant, VariantKey(PrivateKey(DigestString("entity"), DigestString("selection"), DigestString("u1")), vary))
}
