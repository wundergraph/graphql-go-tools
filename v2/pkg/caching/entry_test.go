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
			value, surrogateKeys, vary, err := DecodeEntry(EncodeEntry(tt.value, tt.surrogateKeys))
			require.NoError(t, err)
			require.Equal(t, tt.value, value)
			require.Equal(t, tt.surrogateKeys, surrogateKeys)
			require.Nil(t, vary)
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
		{name: "surrogate key count larger than the input", input: []byte{entryFormatBody, 0xff, 0xff, 0x7f}},
		{name: "cut inside the count", input: encoded[:1]},
		{name: "cut inside a surrogate key length", input: encoded[:2]},
		{name: "cut inside a surrogate key", input: encoded[:5]},
		{name: "cut before the last surrogate key", input: encoded[:len(encoded)-len("value")-len("another")-1]},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := DecodeEntry(tt.input)
			require.ErrorIs(t, err, ErrEntryFormat)
		})
	}

	// Every cut through the surrogateKey section, not just the ones named above.
	for cut := 1; cut < len(encoded)-len("value"); cut++ {
		_, _, _, err := DecodeEntry(encoded[:cut])
		require.ErrorIs(t, err, ErrEntryFormat, "cut at %d", cut)
	}
}

func TestVaryRecordRoundTrip(t *testing.T) {
	names := []string{"accept-language", "x-region"}
	sets := [][]string{{"accept-language"}, names}

	for _, tt := range []struct {
		name string
		sets [][]string
	}{
		{name: "one set", sets: [][]string{names}},
		{name: "several sets, in order", sets: sets},
		{name: "long, non ascii and empty names", sets: [][]string{{strings.Repeat("x", 300), "ünïcödé", ""}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			value, surrogateKeys, vary, err := DecodeEntry(EncodeVaryRecord(tt.sets))
			require.NoError(t, err)
			require.Nil(t, value)
			require.Nil(t, surrogateKeys)
			require.Equal(t, tt.sets, vary)
		})
	}

	require.Equal(t, EncodeVaryRecord(sets), EncodeItem(Item{Vary: sets, Value: []byte("ignored")}), "a record has no body")
	require.Equal(t, EncodeEntry([]byte("v"), []string{"a"}), EncodeItem(Item{Value: []byte("v"), SurrogateKeys: []string{"a"}}))

	encoded := EncodeVaryRecord(sets)
	tests := []struct {
		name  string
		input []byte
	}{
		{name: "no sets", input: EncodeVaryRecord(nil)},
		{name: "an empty set", input: EncodeVaryRecord([][]string{{"accept-language"}, {}})},
		{name: "bytes after the sets", input: append(EncodeVaryRecord(sets), 'x')},
		{name: "cut inside a name", input: encoded[:5]},
		{name: "cut between sets", input: encoded[:2+1+1+len("accept-language")]},
		{name: "set count larger than the input", input: []byte{entryFormatRecord, 0xff, 0xff, 0x7f}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := DecodeEntry(tt.input)
			require.ErrorIs(t, err, ErrEntryFormat)
		})
	}

	// Every cut through a record, not just the ones named above.
	for cut := 1; cut < len(encoded); cut++ {
		_, _, _, err := DecodeEntry(encoded[:cut])
		require.ErrorIs(t, err, ErrEntryFormat, "cut at %d", cut)
	}
}
