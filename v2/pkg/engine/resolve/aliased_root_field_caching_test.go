package resolve

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

// Regression coverage for aliased root field handling in caching paths.
//
// Background: a root field can be requested under an alias, e.g.
//
//	{ p: products(upcs: $upcs) { id name } }
//
// In the response JSON the data lives under the alias key ("p"), NOT under the
// schema field name ("products"). Several caching paths used to derive their
// JSON paths from FetchInfo.RootFields, which only carries schema-side
// coordinates, so aliased queries either stored cache entries under wrong paths
// or skipped subgraph fetches with empty results.

func TestAliasedRootField_EntityMergePath(t *testing.T) {
	t.Parallel()

	t.Run("aliased single root field uses alias as merge path", func(t *testing.T) {
		t.Parallel()
		tpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "u",
				},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings:  []EntityFieldMappingConfig{{EntityKeyField: "id", ArgumentPath: []string{"id"}}},
				},
			},
		}
		got := tpl.EntityMergePath(PostProcessingConfiguration{})
		assert.Equal(t, []string{"u"}, got)
	})

	t.Run("non-aliased single root field still uses schema name", func(t *testing.T) {
		t.Parallel()
		tpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "user",
				},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings:  []EntityFieldMappingConfig{{EntityKeyField: "id", ArgumentPath: []string{"id"}}},
				},
			},
		}
		got := tpl.EntityMergePath(PostProcessingConfiguration{})
		assert.Equal(t, []string{"user"}, got)
	})

	t.Run("explicit MergePath wins over derived response key", func(t *testing.T) {
		t.Parallel()
		tpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "u",
				},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings:  []EntityFieldMappingConfig{{EntityKeyField: "id", ArgumentPath: []string{"id"}}},
				},
			},
		}
		got := tpl.EntityMergePath(PostProcessingConfiguration{MergePath: []string{"data", "user"}})
		assert.Equal(t, []string{"data", "user"}, got)
	})
}

func TestAliasedRootField_BatchResponseFieldKey(t *testing.T) {
	// Verifies that prepareCacheKeys captures the response key (not the schema
	// field name) on the result so populateBatchCacheKeysFromResponse and
	// mergeBatchPartialResponse navigate to the right array path.
	t.Parallel()

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	ctx.Variables = astjson.MustParseBytes([]byte(`{"upcs":["1","2"]}`))

	loader := &Loader{
		ctx:       ctx,
		jsonArena: ar,
	}

	cfg := FetchCacheConfiguration{
		Enabled: true,
		CacheKeyTemplate: NewRootQueryCacheKeyTemplate(
			[]QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "products"},
					ResponseKey: "p",
					Args: []FieldArgument{
						{Name: "upcs", Variable: &ContextVariable{Path: []string{"upcs"}, Renderer: NewPlainVariableRenderer()}},
					},
				},
			},
			[]EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings:  []EntityFieldMappingConfig{{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true}},
				},
			},
		),
	}

	res := &result{}
	_, err := loader.prepareCacheKeys(&FetchInfo{}, cfg, []*astjson.Value{astjson.MustParseBytes([]byte(`{}`))}, res)
	require.NoError(t, err)

	assert.True(t, res.batchEntityKeyMode, "batch entity key mode must be set")
	assert.Equal(t, "p", res.batchResponseFieldKey, "must capture alias as the array's response key, not schema field name 'products'")
}

func TestResolveArgumentVariablePath_NestedRemap(t *testing.T) {
	// Regression: when RemapVariables maps the FIRST segment of a nested
	// argument path, the rest of the path must be carried through. The previous
	// implementation only remapped single-segment paths, so ["a", "ids"] with
	// RemapVariables["a"] == "input" silently became ["a", "ids"] (a value miss),
	// which triggered the empty-batch shortcut even when input.ids was non-empty.
	t.Parallel()

	t.Run("single segment remapped", func(t *testing.T) {
		t.Parallel()
		ctx := NewContext(context.Background())
		ctx.RemapVariables = map[string]string{"a": "input"}
		got := resolveArgumentVariablePath(ctx, []string{"a"})
		assert.Equal(t, []string{"input"}, got)
	})

	t.Run("multi segment remapped", func(t *testing.T) {
		t.Parallel()
		ctx := NewContext(context.Background())
		ctx.RemapVariables = map[string]string{"a": "input"}
		got := resolveArgumentVariablePath(ctx, []string{"a", "ids"})
		assert.Equal(t, []string{"input", "ids"}, got)
	})

	t.Run("multi segment with deeper nesting", func(t *testing.T) {
		t.Parallel()
		ctx := NewContext(context.Background())
		ctx.RemapVariables = map[string]string{"a": "input"}
		got := resolveArgumentVariablePath(ctx, []string{"a", "filter", "upcs"})
		assert.Equal(t, []string{"input", "filter", "upcs"}, got)
	})

	t.Run("first segment not remapped passes through unchanged", func(t *testing.T) {
		t.Parallel()
		ctx := NewContext(context.Background())
		ctx.RemapVariables = map[string]string{"x": "y"}
		got := resolveArgumentVariablePath(ctx, []string{"a", "ids"})
		assert.Equal(t, []string{"a", "ids"}, got)
	})

	t.Run("identity mapping returns same slice", func(t *testing.T) {
		t.Parallel()
		ctx := NewContext(context.Background())
		ctx.RemapVariables = map[string]string{"a": "a"}
		got := resolveArgumentVariablePath(ctx, []string{"a", "ids"})
		assert.Equal(t, []string{"a", "ids"}, got)
	})

	t.Run("nil ctx returns unchanged", func(t *testing.T) {
		t.Parallel()
		got := resolveArgumentVariablePath(nil, []string{"a", "ids"})
		assert.Equal(t, []string{"a", "ids"}, got)
	})

	t.Run("nil RemapVariables returns unchanged", func(t *testing.T) {
		t.Parallel()
		ctx := NewContext(context.Background())
		got := resolveArgumentVariablePath(ctx, []string{"a", "ids"})
		assert.Equal(t, []string{"a", "ids"}, got)
	})

	t.Run("empty path returns empty", func(t *testing.T) {
		t.Parallel()
		ctx := NewContext(context.Background())
		ctx.RemapVariables = map[string]string{"a": "input"}
		got := resolveArgumentVariablePath(ctx, nil)
		assert.Nil(t, got)
	})
}

func TestResolveArgumentValue_NestedRemap(t *testing.T) {
	// End-to-end check: with nested remap and a real ctx.Variables, the value
	// must be resolved from the remapped top-level name + original nested path.
	t.Parallel()

	ctx := NewContext(context.Background())
	ctx.RemapVariables = map[string]string{"a": "input"}
	ctx.Variables = astjson.MustParseBytes([]byte(`{"input":{"ids":["1","2","3"]}}`))

	got := resolveArgumentValue(ctx, []string{"a", "ids"})
	require.NotNil(t, got, "value must resolve via remapped first segment")
	assert.Equal(t, astjson.TypeArray, got.Type())
	assert.Equal(t, `["1","2","3"]`, string(got.MarshalTo(nil)))
}
