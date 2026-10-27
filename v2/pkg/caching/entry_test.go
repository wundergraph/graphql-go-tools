package caching

import (
	"strings"
	"testing"
	"time"

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

func encodeItem(t *testing.T, item Item) []byte {
	t.Helper()
	encoded, err := EncodeItem(item)
	require.NoError(t, err)
	return encoded
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

// Bodies without flags: kind 1 untiered, kind 3 tiered.
func TestEntryDecodeRetiredBodies(t *testing.T) {
	for _, kind := range []byte{1, 3} {
		_, _, _, err := DecodeEntry([]byte{kind, 1, 2, 0x07, 'u', 's', 'e', 'r', '-', '4', '2', '{', '}'})
		require.ErrorIs(t, err, ErrEntryFormat, "kind %d", kind)
	}
}

func TestEntryDecodeRefuses(t *testing.T) {
	encoded := encodeEntry([]byte("value"), declared("a-long-surrogateKey", "another"))

	unknownTier := append([]byte(nil), encoded...)
	unknownTier[4] = byte(SurrogateKeyTierDeclared + 1)

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
		{name: "surrogate key count larger than the input", input: []byte{entryFormatBody, 0, 0, 0xff, 0xff, 0x7f}},
		{name: "cut before the flags", input: encoded[:1]},
		{name: "cut before the reduction", input: encoded[:2]},
		{name: "cut before the count", input: encoded[:3]},
		{name: "cut before a tier", input: encoded[:4]},
		{name: "cut inside a surrogate key length", input: encoded[:5]},
		{name: "cut inside a surrogate key", input: encoded[:8]},
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

	require.Equal(t, encodeVaryRecord(sets), encodeItem(t, Item{Vary: sets, Value: []byte("ignored")}), "a record has no body")
	require.Equal(t, encodeEntry([]byte("v"), declared("a")), encodeItem(t, Item{Value: []byte("v"), SurrogateKeys: declared("a")}))

	t.Run("unknown tier is refused", func(t *testing.T) {
		_, err := EncodeItem(Item{Key: "k", Value: []byte("v"), SurrogateKeys: []SurrogateKey{{Value: "a", Tier: 255}}})
		require.ErrorIs(t, err, ErrInvalidSurrogateKeyTier)
	})

	t.Run("empty sets are dropped so every record decodes", func(t *testing.T) {
		withEmpty := encodeItem(t, Item{Vary: [][]string{{"accept-language"}, {}, {"accept"}}})
		require.Equal(t, encodeVaryRecord([][]string{{"accept-language"}, {"accept"}}), withEmpty)
		_, _, vary, err := DecodeEntry(withEmpty)
		require.NoError(t, err)
		require.Equal(t, [][]string{{"accept-language"}, {"accept"}}, vary)

		onlyEmpty := encodeItem(t, Item{Vary: [][]string{{}}, Value: []byte("v"), SurrogateKeys: declared("a")})
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

func TestClientEntryRoundTrip(t *testing.T) {
	item := Item{Value: []byte(`{"id":1}`), SurrogateKeys: declared("a", "b"), Public: true, ClientTTLReduction: 540 * time.Second}
	got, err := DecodeItem(encodeItem(t, item))
	require.NoError(t, err)
	require.Equal(t, item.Value, got.Value)
	require.Equal(t, item.SurrogateKeys, got.SurrogateKeys)
	require.True(t, got.Public)
	require.Equal(t, 540*time.Second, got.ClientTTLReduction)

	private, err := DecodeItem(encodeItem(t, Item{Value: []byte("v")}))
	require.NoError(t, err)
	require.False(t, private.Public)
	require.False(t, private.SharedOnly)
	require.Zero(t, private.ClientTTLReduction)

	shared, err := DecodeItem(encodeItem(t, Item{Value: []byte("v"), SharedOnly: true}))
	require.NoError(t, err)
	require.True(t, shared.SharedOnly)
	require.False(t, shared.Public)

	_, err = DecodeItem([]byte{entryFormatBody})
	require.ErrorIs(t, err, ErrEntryFormat)
}

func encodeEntry(value []byte, surrogateKeys []SurrogateKey) []byte {
	return encodeBody(Item{Value: value, SurrogateKeys: surrogateKeys})
}
