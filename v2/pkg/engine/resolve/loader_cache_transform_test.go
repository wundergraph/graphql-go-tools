package resolve

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

func TestStructuralCopyNormalized_NilAndNoAliases(t *testing.T) {
	l := newTestLoader(t)

	// structuralCopyNormalized with nil obj is plain StructuralCopy.
	parsed := astjson.MustParseBytes([]byte(`{"id":"1"}`))
	result := l.structuralCopyNormalized(parsed, nil)
	assert.Equal(t, `{"id":"1"}`, string(result.MarshalTo(nil)))

	// No aliases: plain StructuralCopy.
	noAlias := &Object{
		Fields: []*Field{
			{Name: []byte("id"), Value: &Scalar{}},
		},
	}
	result = l.structuralCopyNormalized(parsed, noAlias)
	assert.Equal(t, `{"id":"1"}`, string(result.MarshalTo(nil)))
}

func TestStructuralCopyNormalized_SingleFieldAlias(t *testing.T) {
	l := newTestLoader(t)

	obj := &Object{
		HasAliases: true,
		Fields: []*Field{
			{Name: []byte("nickname"), OriginalName: []byte("name"), Value: &Scalar{}},
		},
	}

	parsed := astjson.MustParseBytes([]byte(`{"nickname":"Alice","__typename":"User"}`))

	// Normalize: alias "nickname" → schema "name".
	normalized := l.structuralCopyNormalized(parsed, obj)
	assert.Equal(t, `{"name":"Alice","__typename":"User"}`, string(normalized.MarshalTo(nil)))

	// Denormalize: schema "name" → alias "nickname".
	schemaShaped := astjson.MustParseBytes([]byte(`{"name":"Alice","__typename":"User"}`))
	denormalized := l.structuralCopyDenormalized(schemaShaped, obj)
	assert.Equal(t, `{"nickname":"Alice","__typename":"User"}`, string(denormalized.MarshalTo(nil)))
}

func TestStructuralCopyNormalized_NestedObjectWithAliases(t *testing.T) {
	l := newTestLoader(t)

	inner := &Object{
		HasAliases: true,
		Fields: []*Field{
			{Name: []byte("handle"), OriginalName: []byte("name"), Value: &Scalar{}},
		},
	}
	outer := &Object{
		HasAliases: true,
		Fields: []*Field{
			{Name: []byte("id"), Value: &Scalar{}},
			{Name: []byte("usr"), OriginalName: []byte("user"), Value: inner},
		},
	}

	parsed := astjson.MustParseBytes([]byte(`{"id":"1","usr":{"handle":"Alice","__typename":"User"},"__typename":"Parent"}`))
	normalized := l.structuralCopyNormalized(parsed, outer)
	assert.Equal(t, `{"id":"1","user":{"name":"Alice","__typename":"User"},"__typename":"Parent"}`, string(normalized.MarshalTo(nil)))

	schemaShaped := astjson.MustParseBytes([]byte(`{"id":"1","user":{"name":"Alice","__typename":"User"},"__typename":"Parent"}`))
	denormalized := l.structuralCopyDenormalized(schemaShaped, outer)
	assert.Equal(t, `{"id":"1","usr":{"handle":"Alice","__typename":"User"},"__typename":"Parent"}`, string(denormalized.MarshalTo(nil)))
}

func TestStructuralCopyNormalized_ArrayOfObjectsWithAliases(t *testing.T) {
	l := newTestLoader(t)

	itemObj := &Object{
		HasAliases: true,
		Fields: []*Field{
			{Name: []byte("handle"), OriginalName: []byte("name"), Value: &Scalar{}},
		},
	}
	outer := &Object{
		HasAliases: true,
		Fields: []*Field{
			{Name: []byte("users"), Value: &Array{Item: itemObj}},
		},
	}

	parsed := astjson.MustParseBytes([]byte(`{"users":[{"handle":"Alice","__typename":"User"},{"handle":"Bob","__typename":"User"}]}`))
	normalized := l.structuralCopyNormalized(parsed, outer)
	assert.Equal(t, `{"users":[{"name":"Alice","__typename":"User"},{"name":"Bob","__typename":"User"}]}`, string(normalized.MarshalTo(nil)))
}

func TestStructuralCopyNormalized_ArgSuffixField(t *testing.T) {
	l := newTestLoader(t)
	l.ctx = NewContext(context.Background())
	l.ctx.Variables = astjson.MustParseBytes([]byte(`{"first":5}`))

	obj := &Object{
		HasAliases: true,
		Fields: []*Field{
			{
				Name:         []byte("friends"),
				OriginalName: []byte("friends"),
				CacheArgs:    []CacheFieldArg{{ArgName: "first", VariableName: "first"}},
				Value:        &Scalar{},
			},
		},
	}

	// Build normalize transform and inspect entries. The builder appends an
	// identity __typename entry when the selection set doesn't include it,
	// so the entity type survives projection to the cache shape.
	l.resetTransformSlabs(obj)
	normalizeXform := l.buildNormalizeTransform(obj)
	require.NotNil(t, normalizeXform)
	assert.Equal(t, []astjson.TransformEntry{
		{InputKey: "friends", OutputKey: "friends_08d4d396a3164ad4"},
		{InputKey: "__typename", OutputKey: "__typename"},
	}, normalizeXform.Entries)
}

func TestStructuralCopyNormalized_RequestScopedInvariant(t *testing.T) {
	obj := &Object{
		HasAliases: true,
		Fields: []*Field{
			{
				Name:         []byte("friends"),
				OriginalName: []byte("friends"),
				CacheArgs:    []CacheFieldArg{{ArgName: "first", VariableName: "remapped"}},
				Value:        &Scalar{},
			},
		},
	}

	ctx1 := NewContext(context.Background())
	ctx1.Variables = astjson.MustParseBytes([]byte(`{"original":"42"}`))
	ctx1.RemapVariables = map[string]string{"remapped": "original"}
	loader1 := newTestLoader(t)
	loader1.ctx = ctx1

	ctx2 := NewContext(context.Background())
	ctx2.Variables = astjson.MustParseBytes([]byte(`{"other":"99"}`))
	ctx2.RemapVariables = map[string]string{"remapped": "other"}
	loader2 := newTestLoader(t)
	loader2.ctx = ctx2

	loader1.resetTransformSlabs(obj)
	t1 := loader1.buildNormalizeTransform(obj)

	loader2.resetTransformSlabs(obj)
	t2 := loader2.buildNormalizeTransform(obj)

	require.NotNil(t, t1)
	require.NotNil(t, t2)
	assert.NotEqual(t, t1.Entries[0].OutputKey, t2.Entries[0].OutputKey,
		"Transforms built under different RemapVariables MUST have different arg-suffix OutputKeys")
}

func TestStructuralCopyNormalized_MixedAliases(t *testing.T) {
	l := newTestLoader(t)

	inner := &Object{
		HasAliases: false,
		Fields: []*Field{
			{Name: []byte("id"), Value: &Scalar{}},
		},
	}
	outer := &Object{
		HasAliases: true,
		Fields: []*Field{
			{Name: []byte("usr"), OriginalName: []byte("user"), Value: inner},
		},
	}

	parsed := astjson.MustParseBytes([]byte(`{"usr":{"id":"1"}}`))
	normalized := l.structuralCopyNormalized(parsed, outer)
	assert.Equal(t, `{"user":{"id":"1"}}`, string(normalized.MarshalTo(nil)))
}

func newTestLoader(t *testing.T) *Loader {
	t.Helper()
	return &Loader{
		jsonArena: arena.NewMonotonicArena(arena.WithMinBufferSize(1024)),
	}
}
