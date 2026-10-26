package caching

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// declared is surrogate keys of the declared tier.
func declared(values ...string) []SurrogateKey {
	out := make([]SurrogateKey, len(values))
	for i, v := range values {
		out[i] = DeclaredSurrogateKey(v)
	}
	return out
}

func TestEntryRoundTrip(t *testing.T) {
	tests := []struct {
		name          string
		value         []byte
		surrogateKeys []SurrogateKey
	}{
		{name: "value and surrogate keys", value: []byte(`{"id":42}`), surrogateKeys: []SurrogateKey{SubgraphSurrogateKey("accounts"), TypeSurrogateKey("accounts", "User"), DeclaredSurrogateKey("user-42")}},
		{name: "a declared key spelled like a derived one keeps its tier", value: []byte("v"), surrogateKeys: []SurrogateKey{SubgraphSurrogateKey("accounts"), DeclaredSurrogateKey("subgraph-accounts")}},
		{name: "no surrogate keys decodes to nil", value: []byte(`{"id":42}`)},
		{name: "empty value", value: []byte{}, surrogateKeys: declared("a")},
		{name: "long, non ascii and empty surrogate keys", value: []byte("v"), surrogateKeys: declared(strings.Repeat("x", 300), "ünïcödé", "")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, surrogateKeys, vary, err := DecodeEntry(encodeEntry(tt.value, tt.surrogateKeys))
			require.NoError(t, err)
			require.Equal(t, tt.value, value)
			require.Equal(t, tt.surrogateKeys, surrogateKeys)
			require.Nil(t, vary)
		})
	}
}

// Written before tiers, so read back with the tier its prefix implies.
func TestEntryDecodeUntieredBody(t *testing.T) {
	encoded := []byte{entryFormatBody, 3}
	for _, key := range []string{"subgraph-accounts", "type-accounts-User", "user-42"} {
		encoded = append(encoded, byte(len(key)))
		encoded = append(encoded, key...)
	}
	encoded = append(encoded, `{"id":42}`...)

	value, surrogateKeys, vary, err := DecodeEntry(encoded)
	require.NoError(t, err)
	require.Equal(t, []byte(`{"id":42}`), value)
	require.Equal(t, []SurrogateKey{SubgraphSurrogateKey("accounts"), TypeSurrogateKey("accounts", "User"), DeclaredSurrogateKey("user-42")}, surrogateKeys)
	require.Nil(t, vary)

	_, _, _, err = DecodeEntry([]byte{entryFormatBody, 0xff, 0xff, 0x7f})
	require.ErrorIs(t, err, ErrEntryFormat)
}

func TestEntryDecodeRefuses(t *testing.T) {
	encoded := encodeEntry([]byte("value"), declared("a-long-surrogateKey", "another"))

	unknownTier := append([]byte(nil), encoded...)
	unknownTier[2] = byte(SurrogateKeyTierDeclared + 1)

	unknownVersion := append([]byte(nil), encoded...)
	unknownVersion[0] = 9

	tests := []struct {
		name  string
		input []byte
	}{
		{name: "nil", input: nil},
		{name: "raw value", input: []byte(`{"id":42}`)},
		{name: "unknown version", input: unknownVersion},
		{name: "unknown tier", input: unknownTier},
		{name: "surrogate key count larger than the input", input: []byte{entryFormatTieredBody, 0xff, 0xff, 0x7f}},
		{name: "cut inside the count", input: encoded[:1]},
		{name: "cut before a tier", input: encoded[:2]},
		{name: "cut inside a surrogate key length", input: encoded[:3]},
		{name: "cut inside a surrogate key", input: encoded[:6]},
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
			value, surrogateKeys, vary, err := DecodeEntry(encodeVaryRecord(tt.sets))
			require.NoError(t, err)
			require.Nil(t, value)
			require.Nil(t, surrogateKeys)
			require.Equal(t, tt.sets, vary)
		})
	}

	require.Equal(t, encodeVaryRecord(sets), EncodeItem(Item{Vary: sets, Value: []byte("ignored")}), "a record has no body")
	require.Equal(t, encodeEntry([]byte("v"), declared("a")), EncodeItem(Item{Value: []byte("v"), SurrogateKeys: declared("a")}))

	t.Run("empty sets are dropped so every record decodes", func(t *testing.T) {
		withEmpty := EncodeItem(Item{Vary: [][]string{{"accept-language"}, {}, {"accept"}}})
		require.Equal(t, encodeVaryRecord([][]string{{"accept-language"}, {"accept"}}), withEmpty)
		_, _, vary, err := DecodeEntry(withEmpty)
		require.NoError(t, err)
		require.Equal(t, [][]string{{"accept-language"}, {"accept"}}, vary)

		onlyEmpty := EncodeItem(Item{Vary: [][]string{{}}, Value: []byte("v"), SurrogateKeys: declared("a")})
		require.Equal(t, encodeEntry([]byte("v"), declared("a")), onlyEmpty, "no set left means a body")
		value, surrogateKeys, vary, err := DecodeEntry(onlyEmpty)
		require.NoError(t, err)
		require.Equal(t, []byte("v"), value)
		require.Equal(t, declared("a"), surrogateKeys)
		require.Nil(t, vary)
	})

	encoded := encodeVaryRecord(sets)
	tests := []struct {
		name  string
		input []byte
	}{
		{name: "no sets", input: encodeVaryRecord(nil)},
		{name: "an empty set", input: encodeVaryRecord([][]string{{"accept-language"}, {}})},
		{name: "bytes after the sets", input: append(encodeVaryRecord(sets), 'x')},
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
