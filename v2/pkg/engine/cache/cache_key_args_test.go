package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// argsTemplateFor builds the key template for an "upc"-keyed Product entity
// fetch whose selection is provides, exactly as PrepareFetch does — the
// argument digest included. The selection tree is folded first, like the
// configurator folds every ProvidesData tree it attaches.
func argsTemplateFor(t *testing.T, ctx *resolve.Context, provides *resolve.Object) cacheKeyTemplate {
	t.Helper()
	resolve.ComputeHasAliases(provides)
	template, ok := newCacheKeyTemplate(ctx, &resolve.FetchCacheConfig{
		SubgraphName: "products",
		KeySpec:      resolve.CacheKeySpec{Scope: resolve.CacheScopeEntity, TypeName: "Product", Representation: productRepresentation(t, "upc")},
		ProvidesData: provides,
	}, 0, l2Access{enabled: true})
	require.True(t, ok)
	return template
}

// TestCacheKeyArgsSegment pins the argument segment of the entity key preimage:
// when (and only when) the fetch selects parameterized fields, the argument
// VALUES of the request join the identity of both layers' keys, so argument
// variants become independent entries instead of replacing each other.
func TestCacheKeyArgsSegment(t *testing.T) {
	item := astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1"}`))

	t.Run("a selection without parameterized fields renders the unchanged preimage", func(t *testing.T) {
		keys, ok := argsTemplateFor(t, variableContext(t, `{"days":3}`), &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name:  []byte("stock"),
					Value: &resolve.Scalar{Path: []string{"stock"}},
				},
			},
		}).render(item)
		require.True(t, ok)
		// No empty segment: the preimage is byte-identical to the one a build
		// without the argument segment renders.
		assert.Equal(t, itemKeys{
			L1: `products:{"__typename":"Product","representation":{"upc":"1"}}`,
			L2: renderCacheKey("v1:products", []byte(`{"__typename":"Product","representation":{"upc":"1"}}`)),
		}, keys)
	})

	t.Run("a parameterized field adds the digest to the L1 and the L2 key", func(t *testing.T) {
		ctx := variableContext(t, `{"days":3}`)
		keys, ok := argsTemplateFor(t, ctx, &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name:      []byte("stockHistory"),
					CacheArgs: []resolve.CacheFieldArg{{Name: "days", VariableName: "days"}},
					Value:     &resolve.Array{Path: []string{"stockHistory"}, Item: &resolve.Scalar{}},
				},
			},
		}).render(item)
		require.True(t, ok)
		// The digest hashes ONE entry: the field's normalized path, whose last
		// segment carries the value-derived argument suffix.
		digest := hashHex([]byte("stockHistory" + computeArgSuffix(ctx, []resolve.CacheFieldArg{{Name: "days", VariableName: "days"}})))
		preimage := `{"__typename":"Product","representation":{"upc":"1"},"args":"` + digest + `"}`
		assert.Equal(t, itemKeys{
			L1: "products:" + preimage,
			L2: renderCacheKey("v1:products", []byte(preimage)),
		}, keys)
		// The parameterized selection never shares the arg-less selection's entry.
		assert.NotEqual(t, `products:{"__typename":"Product","representation":{"upc":"1"}}`, keys.L1)
	})

	t.Run("a changed argument value is a new entry in both layers", func(t *testing.T) {
		// One selection, two requests: only the variable value differs.
		provides := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name:      []byte("stockHistory"),
					CacheArgs: []resolve.CacheFieldArg{{Name: "days", VariableName: "days"}},
					Value:     &resolve.Array{Path: []string{"stockHistory"}, Item: &resolve.Scalar{}},
				},
			},
		}
		ctxThree := variableContext(t, `{"days":3}`)
		ctxOne := variableContext(t, `{"days":1}`)

		three, ok := argsTemplateFor(t, ctxThree, provides).render(item)
		require.True(t, ok)
		one, ok := argsTemplateFor(t, ctxOne, provides).render(item)
		require.True(t, ok)

		threePreimage := `{"__typename":"Product","representation":{"upc":"1"},"args":"` +
			hashHex([]byte("stockHistory"+computeArgSuffix(ctxThree, []resolve.CacheFieldArg{{Name: "days", VariableName: "days"}}))) + `"}`
		onePreimage := `{"__typename":"Product","representation":{"upc":"1"},"args":"` +
			hashHex([]byte("stockHistory"+computeArgSuffix(ctxOne, []resolve.CacheFieldArg{{Name: "days", VariableName: "days"}}))) + `"}`
		assert.Equal(t, itemKeys{
			L1: "products:" + threePreimage,
			L2: renderCacheKey("v1:products", []byte(threePreimage)),
		}, three)
		assert.Equal(t, itemKeys{
			L1: "products:" + onePreimage,
			L2: renderCacheKey("v1:products", []byte(onePreimage)),
		}, one)
		// Both layers split: the in-request L1 must partition by arguments for
		// the same reason the store does.
		assert.NotEqual(t, three.L1, one.L1)
		assert.NotEqual(t, three.L2, one.L2)
	})

	t.Run("the same values under other aliases and variable names key identically", func(t *testing.T) {
		// Operation A: the field is aliased and bound to $days.
		aliased, ok := argsTemplateFor(t, variableContext(t, `{"days":3}`), &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name:         []byte("history"),
					OriginalName: []byte("stockHistory"),
					CacheArgs:    []resolve.CacheFieldArg{{Name: "days", VariableName: "days"}},
					Value:        &resolve.Array{Path: []string{"history"}, Item: &resolve.Scalar{}},
				},
			},
		}).render(item)
		require.True(t, ok)

		// Operation B: no alias, and the SAME value arrives under $a.
		ctxPlain := variableContext(t, `{"a":3}`)
		plain, ok := argsTemplateFor(t, ctxPlain, &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name:      []byte("stockHistory"),
					CacheArgs: []resolve.CacheFieldArg{{Name: "days", VariableName: "a"}},
					Value:     &resolve.Array{Path: []string{"stockHistory"}, Item: &resolve.Scalar{}},
				},
			},
		}).render(item)
		require.True(t, ok)

		preimage := `{"__typename":"Product","representation":{"upc":"1"},"args":"` +
			hashHex([]byte("stockHistory"+computeArgSuffix(ctxPlain, []resolve.CacheFieldArg{{Name: "days", VariableName: "a"}}))) + `"}`
		assert.Equal(t, itemKeys{
			L1: "products:" + preimage,
			L2: renderCacheKey("v1:products", []byte(preimage)),
		}, aliased)
		assert.Equal(t, aliased, plain)
	})

	t.Run("a nested parameterized field carries its path into the digest", func(t *testing.T) {
		ctx := variableContext(t, `{"days":3}`)
		args := []resolve.CacheFieldArg{{Name: "days", VariableName: "days"}}

		// The same schema field, selected one level down under warehouse.
		nested, ok := argsTemplateFor(t, ctx, &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("warehouse"),
					Value: &resolve.Object{
						Path: []string{"warehouse"},
						Fields: []*resolve.Field{
							{
								Name:         []byte("history"),
								OriginalName: []byte("stockHistory"),
								CacheArgs:    args,
								Value:        &resolve.Array{Path: []string{"history"}, Item: &resolve.Scalar{}},
							},
						},
					},
				},
			},
		}).render(item)
		require.True(t, ok)

		top, ok := argsTemplateFor(t, ctx, &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name:      []byte("stockHistory"),
					CacheArgs: args,
					Value:     &resolve.Array{Path: []string{"stockHistory"}, Item: &resolve.Scalar{}},
				},
			},
		}).render(item)
		require.True(t, ok)

		nestedPreimage := `{"__typename":"Product","representation":{"upc":"1"},"args":"` +
			hashHex([]byte("warehouse.stockHistory"+computeArgSuffix(ctx, args))) + `"}`
		topPreimage := `{"__typename":"Product","representation":{"upc":"1"},"args":"` +
			hashHex([]byte("stockHistory"+computeArgSuffix(ctx, args))) + `"}`
		assert.Equal(t, itemKeys{
			L1: "products:" + nestedPreimage,
			L2: renderCacheKey("v1:products", []byte(nestedPreimage)),
		}, nested)
		assert.Equal(t, itemKeys{
			L1: "products:" + topPreimage,
			L2: renderCacheKey("v1:products", []byte(topPreimage)),
		}, top)
		assert.NotEqual(t, nested, top)
	})

	t.Run("two variants of one field in a single selection sort into one digest", func(t *testing.T) {
		ctx := variableContext(t, `{"a":3,"b":1}`)
		daysA := []resolve.CacheFieldArg{{Name: "days", VariableName: "a"}}
		daysB := []resolve.CacheFieldArg{{Name: "days", VariableName: "b"}}

		// euro/usd-style multi-variant: both aliases of the SAME field live in
		// ONE entry, under their argument-suffixed stored names.
		keys, ok := argsTemplateFor(t, ctx, &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name:         []byte("recent"),
					OriginalName: []byte("stockHistory"),
					CacheArgs:    daysA,
					Value:        &resolve.Array{Path: []string{"recent"}, Item: &resolve.Scalar{}},
				},
				{
					Name:         []byte("yesterday"),
					OriginalName: []byte("stockHistory"),
					CacheArgs:    daysB,
					Value:        &resolve.Array{Path: []string{"yesterday"}, Item: &resolve.Scalar{}},
				},
			},
		}).render(item)
		require.True(t, ok)

		// The digest input is SORTED, so selection order cannot move it: the
		// entries are the two suffixed names, comma-joined in sorted order.
		entries := []string{
			"stockHistory" + computeArgSuffix(ctx, daysA),
			"stockHistory" + computeArgSuffix(ctx, daysB),
		}
		if entries[0] > entries[1] {
			entries[0], entries[1] = entries[1], entries[0]
		}
		preimage := `{"__typename":"Product","representation":{"upc":"1"},"args":"` +
			hashHex([]byte(entries[0]+","+entries[1])) + `"}`
		assert.Equal(t, itemKeys{
			L1: "products:" + preimage,
			L2: renderCacheKey("v1:products", []byte(preimage)),
		}, keys)

		// A differently-aliased operation selecting the same two variants in the
		// opposite order lands on the SAME entry.
		swapped, ok := argsTemplateFor(t, ctx, &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name:         []byte("old"),
					OriginalName: []byte("stockHistory"),
					CacheArgs:    daysB,
					Value:        &resolve.Array{Path: []string{"old"}, Item: &resolve.Scalar{}},
				},
				{
					Name:         []byte("fresh"),
					OriginalName: []byte("stockHistory"),
					CacheArgs:    daysA,
					Value:        &resolve.Array{Path: []string{"fresh"}, Item: &resolve.Scalar{}},
				},
			},
		}).render(item)
		require.True(t, ok)
		assert.Equal(t, keys, swapped)
	})

	t.Run("one digest per fetch serves every item of a batch", func(t *testing.T) {
		ctx := variableContext(t, `{"days":3}`)
		args := []resolve.CacheFieldArg{{Name: "days", VariableName: "days"}}
		template := argsTemplateFor(t, ctx, &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name:      []byte("stockHistory"),
					CacheArgs: args,
					Value:     &resolve.Array{Path: []string{"stockHistory"}, Item: &resolve.Scalar{}},
				},
			},
		})
		digest := hashHex([]byte("stockHistory" + computeArgSuffix(ctx, args)))
		// The digest is derived ONCE, when the template is built — the per-item
		// render only appends it.
		assert.Equal(t, digest, template.args)

		first, ok := template.render(astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1"}`)))
		require.True(t, ok)
		second, ok := template.render(astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"2"}`)))
		require.True(t, ok)

		firstPreimage := `{"__typename":"Product","representation":{"upc":"1"},"args":"` + digest + `"}`
		secondPreimage := `{"__typename":"Product","representation":{"upc":"2"},"args":"` + digest + `"}`
		assert.Equal(t, itemKeys{
			L1: "products:" + firstPreimage,
			L2: renderCacheKey("v1:products", []byte(firstPreimage)),
		}, first)
		assert.Equal(t, itemKeys{
			L1: "products:" + secondPreimage,
			L2: renderCacheKey("v1:products", []byte(secondPreimage)),
		}, second)
	})
}
