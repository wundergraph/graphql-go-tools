package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestEntityTags pins the tags an entity entry is written under: the fixed
// three-tag vocabulary in its fixed order, and the property that makes the
// entity tag an ENTITY address rather than an entry address — its digest
// covers the item's @key fields alone, so every entry that describes one
// entity carries the same tag whatever else varies in its key.
func TestEntityTags(t *testing.T) {
	t.Run("a @key representation yields subgraph, type, and entity tags in that order", func(t *testing.T) {
		tags := templateFor(t, productRepresentation(t, "upc")).
			entityTags(astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1","name":"Table"}`)))
		// The entity digest is xxhash64 of the canonical @key rendering
		// `{"upc":"1"}`; fields outside the representation never reach it.
		assert.Equal(t, []string{
			"subgraph:products",
			"type:products:Product",
			"entity:products:Product:d3cc039c7a9789e7",
		}, tags)
	})

	t.Run("two entities of one type differ only in the entity tag", func(t *testing.T) {
		template := templateFor(t, productRepresentation(t, "upc"))
		assert.Equal(t, []string{
			"subgraph:products",
			"type:products:Product",
			"entity:products:Product:4f93140518d68e67", // `{"upc":"2"}`
		}, template.entityTags(astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"2"}`))))
		assert.Equal(t, []string{
			"subgraph:products",
			"type:products:Product",
			"entity:products:Product:dae7809df4f7fdee", // `{"upc":"3"}`
		}, template.entityTags(astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"3"}`))))
	})

	t.Run("@requires values change the KEY but not the entity tag", func(t *testing.T) {
		// The fetch sends @key(upc) plus the @requires field price, so its two
		// items are two independent entries — and one purge of the product must
		// clear both.
		template := templateFor(t, productRepresentation(t, "upc", "price"))
		cheap := astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1","price":100}`))
		pricey := astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1","price":120}`))

		cheapKeys, ok := template.render(cheap)
		require.True(t, ok)
		priceyKeys, ok := template.render(pricey)
		require.True(t, ok)
		assert.NotEqual(t, cheapKeys, priceyKeys)

		wantTags := []string{
			"subgraph:products",
			"type:products:Product",
			"entity:products:Product:d3cc039c7a9789e7", // `{"upc":"1"}` — price is not in it
		}
		assert.Equal(t, wantTags, template.entityTags(cheap))
		assert.Equal(t, wantTags, template.entityTags(pricey))
	})

	t.Run("argument variants of one entity share the entity tag", func(t *testing.T) {
		// Two requests of the same entity differing only in an argument value:
		// separate entries by design, one purge target.
		representation := productRepresentation(t, "upc")
		euro := cacheKeyTemplate{
			subgraph:       "products",
			prefix:         "v1:products",
			representation: representation,
			args:           "f123752fc2272dfc",
		}
		dollar := cacheKeyTemplate{
			subgraph:       "products",
			prefix:         "v1:products",
			representation: representation,
			args:           "b487e21691ea4c86",
		}
		item := astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1"}`))

		euroKeys, ok := euro.render(item)
		require.True(t, ok)
		dollarKeys, ok := dollar.render(item)
		require.True(t, ok)
		assert.NotEqual(t, euroKeys, dollarKeys)

		wantTags := []string{
			"subgraph:products",
			"type:products:Product",
			"entity:products:Product:d3cc039c7a9789e7", // `{"upc":"1"}` — the args digest is not in it
		}
		assert.Equal(t, wantTags, euro.entityTags(item))
		assert.Equal(t, wantTags, dollar.entityTags(item))
	})

	t.Run("private partitions of one entity share the entity tag", func(t *testing.T) {
		// A tag must never carry identity material: purging an entity has to
		// reach every requester's partition of it.
		representation := productRepresentation(t, "upc")
		public := cacheKeyTemplate{
			subgraph:       "products",
			prefix:         "v1:products",
			representation: representation,
		}
		userA := cacheKeyTemplate{
			subgraph:       "products",
			prefix:         "v1:products",
			representation: representation,
			partition:      sha256Hex("i:user-a"),
		}
		userB := cacheKeyTemplate{
			subgraph:       "products",
			prefix:         "v1:products",
			representation: representation,
			partition:      sha256Hex("i:user-b"),
		}
		item := astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1"}`))

		publicKeys, ok := public.render(item)
		require.True(t, ok)
		userAKeys, ok := userA.render(item)
		require.True(t, ok)
		userBKeys, ok := userB.render(item)
		require.True(t, ok)
		assert.NotEqual(t, publicKeys.L2, userAKeys.L2)
		assert.NotEqual(t, userAKeys.L2, userBKeys.L2)

		wantTags := []string{
			"subgraph:products",
			"type:products:Product",
			"entity:products:Product:d3cc039c7a9789e7", // `{"upc":"1"}` — no partition anywhere
		}
		assert.Equal(t, wantTags, public.entityTags(item))
		assert.Equal(t, wantTags, userA.entityTags(item))
		assert.Equal(t, wantTags, userB.entityTags(item))
	})

	t.Run("a static node __typename tags under the name the fetch sends", func(t *testing.T) {
		// An @interfaceObject target keys under the interface name, so its tags
		// address the same name — one entity, one tag, whatever implementer the
		// item claims to be.
		node := mergedRepresentation(t, plan.FederationMetaData{
			InterfaceObjects: []plan.EntityInterfaceConfiguration{
				{InterfaceTypeName: "Sellable", ConcreteTypeNames: []string{"Product"}},
			},
		}, plan.FederationFieldConfiguration{TypeName: "Product", SelectionSet: "upc"})
		template := templateFor(t, node)

		wantTags := []string{
			"subgraph:products",
			"type:products:Sellable",
			"entity:products:Sellable:d3cc039c7a9789e7", // `{"upc":"1"}`
		}
		assert.Equal(t, wantTags, template.entityTags(astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1"}`))))
		assert.Equal(t, wantTags, template.entityTags(astjson.MustParseBytes([]byte(`{"__typename":"Sellable","upc":"1"}`))))
	})

	t.Run("a representation of @requires fields alone still carries the coarse tags", func(t *testing.T) {
		// Nothing to address the entity by, but "purge this subgraph" and
		// "purge this type" must keep working.
		node := mergedRepresentation(t, plan.FederationMetaData{},
			plan.FederationFieldConfiguration{TypeName: "Product", FieldName: "shippingEstimate", SelectionSet: "price"},
		)
		assert.Equal(t, []string{
			"subgraph:products",
			"type:products:Product",
		}, templateFor(t, node).entityTags(astjson.MustParseBytes([]byte(`{"__typename":"Product","price":100}`))))
	})

	t.Run("an item with no type to tag under yields no tags", func(t *testing.T) {
		template := templateFor(t, productRepresentation(t, "upc"))
		assert.Nil(t, template.entityTags(astjson.MustParseBytes([]byte(`{"upc":"1"}`))))
		assert.Nil(t, template.entityTags(nil))
		assert.Nil(t, cacheKeyTemplate{subgraph: "products"}.entityTags(astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1"}`))))
	})

	t.Run("determinism: repeated derivation produces the identical ordered slice", func(t *testing.T) {
		template := templateFor(t, productRepresentation(t, "upc", "price"))
		item := astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1","price":100}`))
		assert.Equal(t, template.entityTags(item), template.entityTags(item))
	})
}

// TestRootFieldTags pins the root-field vocabulary: a whole-response entry is
// not one entity, so it carries the two coarse tags only, and its type is the
// coordinate's.
func TestRootFieldTags(t *testing.T) {
	assert.Equal(t, []string{
		"subgraph:products",
		"type:products:Query",
	}, rootFieldTags(&resolve.FetchCacheConfig{
		SubgraphName: "products",
		KeySpec:      resolve.CacheKeySpec{Scope: resolve.CacheScopeRootField, TypeName: "Query", FieldName: "topProducts"},
	}))
}
