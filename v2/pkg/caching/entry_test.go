package caching

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEntryRoundTrip(t *testing.T) {
	tests := []struct {
		name          string
		value         []byte
		surrogateKeys []string
	}{
		{name: "value and surrogate keys", value: []byte(`{"id":42}`), surrogateKeys: []string{"subgraph-accounts", "type-accounts-User", "user-42"}},
		{name: "no surrogate keys decodes to nil", value: []byte(`{"id":42}`)},
		{name: "empty value", value: []byte{}, surrogateKeys: []string{"a"}},
		{name: "long, non ascii and empty surrogate keys", value: []byte("v"), surrogateKeys: []string{strings.Repeat("x", 300), "ünïcödé", ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, surrogateKeys, err := DecodeEntry(EncodeEntry(tt.value, tt.surrogateKeys))
			require.NoError(t, err)
			require.Equal(t, tt.value, value)
			require.Equal(t, tt.surrogateKeys, surrogateKeys)
		})
	}
}

func TestEntryDecodeRefuses(t *testing.T) {
	encoded := EncodeEntry([]byte("value"), []string{"a-long-surrogateKey", "another"})

	unknownVersion := append([]byte(nil), encoded...)
	unknownVersion[0] = 9

	tests := []struct {
		name  string
		input []byte
	}{
		{name: "nil", input: nil},
		{name: "raw value", input: []byte(`{"id":42}`)},
		{name: "unknown version", input: unknownVersion},
		{name: "surrogate key count larger than the input", input: []byte{entryFormatVersion, 0xff, 0xff, 0x7f}},
		{name: "cut inside the count", input: encoded[:1]},
		{name: "cut inside a surrogate key length", input: encoded[:2]},
		{name: "cut inside a surrogate key", input: encoded[:5]},
		{name: "cut before the last surrogate key", input: encoded[:len(encoded)-len("value")-len("another")-1]},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := DecodeEntry(tt.input)
			require.ErrorIs(t, err, ErrEntryFormat)
		})
	}

	// Every cut through the surrogateKey section, not just the ones named above.
	for cut := 1; cut < len(encoded)-len("value"); cut++ {
		_, _, err := DecodeEntry(encoded[:cut])
		require.ErrorIs(t, err, ErrEntryFormat, "cut at %d", cut)
	}
}
