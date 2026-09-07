package caching

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEntryRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		value  []byte
		labels []string
	}{
		{name: "value and labels", value: []byte(`{"id":42}`), labels: []string{"subgraph-accounts", "type-accounts-User", "user-42"}},
		{name: "no labels decodes to nil", value: []byte(`{"id":42}`)},
		{name: "empty value", value: []byte{}, labels: []string{"a"}},
		{name: "long, non ascii and empty labels", value: []byte("v"), labels: []string{strings.Repeat("x", 300), "ünïcödé", ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, labels, err := DecodeEntry(EncodeEntry(tt.value, tt.labels))
			require.NoError(t, err)
			require.Equal(t, tt.value, value)
			require.Equal(t, tt.labels, labels)
		})
	}
}

func TestEntryDecodeRefuses(t *testing.T) {
	encoded := EncodeEntry([]byte("value"), []string{"a-long-label", "another"})

	unknownVersion := append([]byte(nil), encoded...)
	unknownVersion[0] = 9

	tests := []struct {
		name  string
		input []byte
	}{
		{name: "nil", input: nil},
		{name: "raw value", input: []byte(`{"id":42}`)},
		{name: "unknown version", input: unknownVersion},
		{name: "label count larger than the input", input: []byte{entryFormatVersion, 0xff, 0xff, 0x7f}},
		{name: "cut inside the count", input: encoded[:1]},
		{name: "cut inside a label length", input: encoded[:2]},
		{name: "cut inside a label", input: encoded[:5]},
		{name: "cut before the last label", input: encoded[:len(encoded)-len("value")-len("another")-1]},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := DecodeEntry(tt.input)
			require.ErrorIs(t, err, ErrEntryFormat)
		})
	}

	// Every cut through the label section, not just the ones named above.
	for cut := 1; cut < len(encoded)-len("value"); cut++ {
		_, _, err := DecodeEntry(encoded[:cut])
		require.ErrorIs(t, err, ErrEntryFormat, "cut at %d", cut)
	}
}
