package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// variableContext builds a resolve.Context carrying the given variables JSON.
func variableContext(t *testing.T, variables string) *resolve.Context {
	t.Helper()
	ctx := resolve.NewContext(t.Context())
	if variables != "" {
		ctx.Variables = astjson.MustParseBytes([]byte(variables))
	}
	return ctx
}

func testTx() *resolve.CacheTransaction {
	return resolve.NewTransactionBeginner(nil, &resolve.DataBuffer{}).Begin()
}

// aliasedTree is a ProvidesData tree with an aliased scalar, an arg-suffixed
// list field, and a nested object with an aliased leaf.
func aliasedTree() *resolve.Object {
	tree := &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name:         []byte("productName"),
				OriginalName: []byte("name"),
				Value:        &resolve.Scalar{Nullable: false, Path: []string{"productName"}},
			},
			{
				Name:      []byte("friends"),
				CacheArgs: []resolve.CacheFieldArg{{Name: "first", VariableName: "first"}},
				Value: &resolve.Array{
					Nullable: true,
					Path:     []string{"friends"},
					Item: &resolve.Object{
						Nullable: true,
						Fields: []*resolve.Field{
							{
								Name:         []byte("handle"),
								OriginalName: []byte("username"),
								Value:        &resolve.Scalar{Nullable: true, Path: []string{"handle"}},
							},
						},
					},
				},
			},
		},
	}
	resolve.ComputeHasAliases(tree)
	return tree
}

func TestNormalizedFieldName(t *testing.T) {
	ctx := variableContext(t, `{"first":5}`)

	t.Run("plain field", func(t *testing.T) {
		assert.Equal(t, "name", normalizedFieldName(ctx, &resolve.Field{Name: []byte("name")}))
	})

	t.Run("aliased field uses the schema name", func(t *testing.T) {
		assert.Equal(t, "name", normalizedFieldName(ctx, &resolve.Field{Name: []byte("productName"), OriginalName: []byte("name")}))
	})

	t.Run("argument suffix is deterministic and order-independent", func(t *testing.T) {
		ctxAB := variableContext(t, `{"a":1,"b":2}`)
		args := []resolve.CacheFieldArg{{Name: "x", VariableName: "a"}, {Name: "y", VariableName: "b"}}
		reversed := []resolve.CacheFieldArg{{Name: "y", VariableName: "b"}, {Name: "x", VariableName: "a"}}
		nameA := normalizedFieldName(ctxAB, &resolve.Field{Name: []byte("f"), CacheArgs: args})
		nameB := normalizedFieldName(ctxAB, &resolve.Field{Name: []byte("f"), CacheArgs: reversed})
		assert.Equal(t, nameA, nameB)
		assert.Equal(t, "f"+computeArgSuffix(ctxAB, args), nameA)
	})

	t.Run("different argument values produce different suffixes", func(t *testing.T) {
		field := &resolve.Field{Name: []byte("friends"), CacheArgs: []resolve.CacheFieldArg{{Name: "first", VariableName: "first"}}}
		name5 := normalizedFieldName(variableContext(t, `{"first":5}`), field)
		name20 := normalizedFieldName(variableContext(t, `{"first":20}`), field)
		assert.NotEqual(t, name5, name20)
	})

	t.Run("remapped variables resolve through the remap", func(t *testing.T) {
		ctx := variableContext(t, `{"a":5}`)
		ctx.RemapVariables = map[string]string{"first": "a"}
		field := &resolve.Field{Name: []byte("friends"), CacheArgs: []resolve.CacheFieldArg{{Name: "first", VariableName: "first"}}}
		direct := normalizedFieldName(variableContext(t, `{"first":5}`), field)
		assert.Equal(t, direct, normalizedFieldName(ctx, field))
	})

	t.Run("absent variables hash as null", func(t *testing.T) {
		field := &resolve.Field{Name: []byte("friends"), CacheArgs: []resolve.CacheFieldArg{{Name: "first", VariableName: "first"}}}
		noVars := normalizedFieldName(variableContext(t, ""), field)
		nilCtx := normalizedFieldName(nil, field)
		assert.Equal(t, noVars, nilCtx)
	})
}

// TestNormalizeDenormalizeRoundTrip proves the transform pair: an alias-shaped
// response normalizes to schema names + arg suffixes (full stored form
// asserted) and denormalizes back under a DIFFERENT alias set.
func TestNormalizeDenormalizeRoundTrip(t *testing.T) {
	ctx := variableContext(t, `{"first":5}`)
	tx := testTx()
	defer tx.Commit()

	suffix := computeArgSuffix(ctx, []resolve.CacheFieldArg{{Name: "first", VariableName: "first"}})
	response := astjson.MustParseBytes([]byte(`{"__typename":"User","productName":"Table","friends":[{"handle":"alice"},{"handle":"bob"}]}`))

	stored := normalizeToSchema(tx, ctx, response, aliasedTree())
	assert.Equal(t,
		`{"name":"Table","friends`+suffix+`":[{"username":"alice"},{"username":"bob"}],"__typename":"User"}`,
		string(stored.MarshalTo(nil)))

	// A DIFFERENT operation selects the same data under other aliases.
	otherTree := &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name:         []byte("title"),
				OriginalName: []byte("name"),
				Value:        &resolve.Scalar{Nullable: false, Path: []string{"title"}},
			},
			{
				Name:         []byte("buddies"),
				OriginalName: []byte("friends"),
				CacheArgs:    []resolve.CacheFieldArg{{Name: "first", VariableName: "first"}},
				Value: &resolve.Array{
					Nullable: true,
					Path:     []string{"buddies"},
					Item: &resolve.Object{
						Nullable: true,
						Fields: []*resolve.Field{
							{
								Name:         []byte("nick"),
								OriginalName: []byte("username"),
								Value:        &resolve.Scalar{Nullable: true, Path: []string{"nick"}},
							},
						},
					},
				},
			},
		},
	}
	resolve.ComputeHasAliases(otherTree)
	require.True(t, covers(ctx, stored, otherTree))

	served := denormalizeToSelection(tx, ctx, stored, otherTree)
	assert.Equal(t,
		`{"title":"Table","buddies":[{"nick":"alice"},{"nick":"bob"}],"__typename":"User"}`,
		string(served.MarshalTo(nil)))
}

// TestArgumentMismatchIsMiss is the D6 coverage rule: a value cached under one
// argument suffix does not satisfy the same field with different arguments.
func TestArgumentMismatchIsMiss(t *testing.T) {
	writeCtx := variableContext(t, `{"first":5}`)
	readCtx := variableContext(t, `{"first":20}`)
	tx := testTx()
	defer tx.Commit()

	tree := aliasedTree()
	response := astjson.MustParseBytes([]byte(`{"__typename":"User","productName":"Table","friends":[{"handle":"alice"}]}`))
	stored := normalizeToSchema(tx, writeCtx, response, tree)

	assert.True(t, covers(writeCtx, stored, tree))
	assert.False(t, covers(readCtx, stored, tree))
}

// TestControllerAliasRoundTrip drives the transforms through the controller:
// an alias-shaped response is STORED normalized, and a second request with a
// different alias set is served, spliced under ITS aliases.
func TestControllerAliasRoundTrip(t *testing.T) {
	store := newTestStore()

	writeCfg := entityConfig(t, time.Minute)
	writeCfg.ProvidesData = &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name:         []byte("productName"),
				OriginalName: []byte("name"),
				Value:        &resolve.Scalar{Nullable: false, Path: []string{"productName"}},
			},
		},
	}
	resolve.ComputeHasAliases(writeCfg.ProvidesData)

	rc := NewController(store, nil).BeginRequest(variableContext(t, ""))
	item := productItem(t, "1")
	_, handle := prepare(t, rc, writeCfg, item)
	require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
		Items:        []*astjson.Value{item},
		ResponseData: astjson.MustParseBytes([]byte(`{"__typename":"Product","productName":"Table"}`)),
		Arena:        beginner(),
	}))
	rc.EndRequest()
	key := handle.Items[0].RenderedKeys[0]
	// The STORED form carries the schema name.
	value, _, ok := store.Get(key)
	require.True(t, ok)
	assert.Equal(t, `{"name":"Table","__typename":"Product"}`, string(value))

	// A second operation selects the same field under ANOTHER alias.
	readCfg := entityConfig(t, time.Minute)
	readCfg.ProvidesData = &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name:         []byte("label"),
				OriginalName: []byte("name"),
				Value:        &resolve.Scalar{Nullable: false, Path: []string{"label"}},
			},
		},
	}
	resolve.ComputeHasAliases(readCfg.ProvidesData)

	rc2 := NewController(store, nil).BeginRequest(variableContext(t, ""))
	item2 := productItem(t, "1")
	decision, handle2 := prepare(t, rc2, readCfg, item2)
	require.Equal(t, resolve.DecisionSkipFullHit, decision)
	require.NoError(t, rc2.OnFetchSkipped(handle2, resolve.MergeInput{
		Items: []*astjson.Value{item2},
		Arena: beginner(),
	}))
	assert.Equal(t, `{"__typename":"Product","upc":"1","label":"Table"}`, string(item2.MarshalTo(nil)))
}
