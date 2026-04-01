package resolve

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

func TestLoader_cacheKeysToNegativeEntries_PreservesPositiveEntityDataWithNullableFields(t *testing.T) {
	t.Parallel()

	a := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))

	loader := &Loader{}
	// Start from an existing cached entity that already has non-key fields. This is the
	// branch where negative caching keeps an object-shaped payload instead of plain `null`.
	fromCache, err := astjson.ParseBytesWithArena(a, []byte(`{"__typename":"Item","id":"1","name":"Widget"}`))
	require.NoError(t, err)

	res := &result{
		providesData: &Object{
			Fields: []*Field{
				{
					Name: []byte("summary"),
					Value: &String{
						Path:     []string{"summary"},
						Nullable: true,
					},
				},
			},
		},
	}

	// Simulate a negative-cache write for the same entity key. The helper should preserve
	// the existing object shape and materialize the requested nullable field as explicit null.
	entries := loader.cacheKeysToNegativeEntries(a, res, []*CacheKey{{
		FromCache:        fromCache,
		Keys:             []string{`{"__typename":"Item","key":{"id":"1"}}`},
		NegativeCacheHit: true,
	}})

	require.Len(t, entries, 1)
	// `summary` was not present in the old payload, but because it is nullable in ProvidesData
	// the negative-cache value must include `"summary": null` so the same selection can validate from cache.
	require.JSONEq(t, `{"__typename":"Item","id":"1","name":"Widget","summary":null}`, string(entries[0].Value))
}

func TestLoader_cacheKeysToNegativeEntries_UsesNullSentinelWithoutPositiveEntityData(t *testing.T) {
	t.Parallel()

	a := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))

	loader := &Loader{}
	// With no existing non-key entity data, negative caching must collapse to the literal
	// `null` sentinel rather than storing key-only scaffolding as if it were a real entity.
	entries := loader.cacheKeysToNegativeEntries(a, &result{}, []*CacheKey{{
		Keys:             []string{`{"__typename":"Item","key":{"id":"1"}}`},
		NegativeCacheHit: true,
	}})

	require.Len(t, entries, 1)
	require.Equal(t, "null", string(entries[0].Value))
}
