package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
)

func TestTypeLinker(t *testing.T) {
	t.Run("add single mapping", func(t *testing.T) {
		l := make(typeLinker)
		l.add("Animal", "Cat")
		iface, ok := l.getLinkedType("Cat")
		assert.True(t, ok)
		assert.Equal(t, "Animal", iface)
	})

	t.Run("add multiple linked types in one call", func(t *testing.T) {
		l := make(typeLinker)
		l.add("Resource", "Product", "Storage", "Warehouse")
		for _, m := range []string{"Product", "Storage", "Warehouse"} {
			iface, ok := l.getLinkedType(m)
			assert.True(t, ok, "expected %s to be linked", m)
			assert.Equal(t, "Resource", iface)
		}
	})

	t.Run("add does not override existing mapping", func(t *testing.T) {
		l := make(typeLinker)
		l.add("Animal", "Cat")
		l.add("Mammal", "Cat")
		iface, ok := l.getLinkedType("Cat")
		assert.True(t, ok)
		assert.Equal(t, "Animal", iface, "first interface should win")
	})

	t.Run("getLinkedType on nil typeLinker", func(t *testing.T) {
		var l typeLinker
		iface, ok := l.getLinkedType("Cat")
		assert.False(t, ok)
		assert.Empty(t, iface)
	})

	t.Run("getLinkedType for unknown type", func(t *testing.T) {
		l := make(typeLinker)
		l.add("Animal", "Cat")
		iface, ok := l.getLinkedType("Dog")
		assert.False(t, ok)
		assert.Empty(t, iface)
	})
}

func TestGetRepresentations(t *testing.T) {
	t.Run("returns nil when representations key missing", func(t *testing.T) {
		vars := gjson.Parse(`{"other":"value"}`)
		assert.Nil(t, getRepesentations(vars))
	})

	t.Run("returns empty slice when representations is empty array", func(t *testing.T) {
		vars := gjson.Parse(`{"representations":[]}`)
		reps := getRepesentations(vars)
		assert.NotNil(t, reps)
		assert.Empty(t, reps)
	})

	t.Run("returns representations when present", func(t *testing.T) {
		vars := gjson.Parse(`{"representations":[{"__typename":"Product","id":"1"},{"__typename":"Storage","id":"2"}]}`)
		reps := getRepesentations(vars)
		assert.Len(t, reps, 2)
		assert.Equal(t, "Product", reps[0].Get("__typename").String())
		assert.Equal(t, "Storage", reps[1].Get("__typename").String())
	})
}

func TestGetMemberTypesForInterface(t *testing.T) {
	schema := grpctest.MustGraphQLSchema(t)
	def := &schema

	t.Run("returns nil for unknown type", func(t *testing.T) {
		assert.Nil(t, getMemberTypesForInterface(def, "DoesNotExist"))
	})

	t.Run("returns nil for non-interface type", func(t *testing.T) {
		assert.Nil(t, getMemberTypesForInterface(def, "Product"))
	})

	t.Run("returns implementing types for interface", func(t *testing.T) {
		members := getMemberTypesForInterface(def, "Resource")
		assert.ElementsMatch(t, []string{"Product", "Storage", "Warehouse"}, members)
	})

	t.Run("returns implementing types for Animal interface", func(t *testing.T) {
		members := getMemberTypesForInterface(def, "Animal")
		assert.ElementsMatch(t, []string{"Cat", "Dog"}, members)
	})
}

func TestCreateEntityIndexMap(t *testing.T) {
	schema := grpctest.MustGraphQLSchema(t)
	def := &schema

	t.Run("returns nil for empty representations", func(t *testing.T) {
		assert.Nil(t, createEntityIndexMap(def, nil))
		assert.Nil(t, createEntityIndexMap(def, []gjson.Result{}))
	})

	t.Run("indexes concrete entity types", func(t *testing.T) {
		reps := getRepesentations(gjson.Parse(`{"representations":[
			{"__typename":"Product","id":"1"},
			{"__typename":"Product","id":"2"},
			{"__typename":"Storage","id":"3"}
		]}`))

		im := createEntityIndexMap(def, reps)
		assert.NotNil(t, im)
		assert.Len(t, im.entities["Product"], 2)
		assert.Len(t, im.entities["Storage"], 1)

		assert.Equal(t, 0, im.entities["Product"][0].representationIndex)
		assert.Equal(t, 0, im.entities["Product"][0].resultIndex)
		assert.Equal(t, 1, im.entities["Product"][1].representationIndex)
		assert.Equal(t, 1, im.entities["Product"][1].resultIndex)
		assert.Equal(t, 0, im.entities["Storage"][0].representationIndex)
		assert.Equal(t, 2, im.entities["Storage"][0].resultIndex)

		assert.Empty(t, im.typeLinker)
	})

	t.Run("indexes concrete entity types with alternating representation indices", func(t *testing.T) {
		reps := getRepesentations(gjson.Parse(`{"representations":[
			{"__typename":"Product","id":"1"}, 
			{"__typename":"Storage","id":"2"},
			{"__typename":"Product","id":"3"},
			{"__typename":"Storage","id":"4"},
			{"__typename":"Storage","id":"5"},
			{"__typename":"Product","id":"6"},
		]}`))

		im := createEntityIndexMap(def, reps)
		assert.NotNil(t, im)
		assert.Len(t, im.entities["Product"], 3)
		assert.Len(t, im.entities["Storage"], 3)

		// Products
		// representation indexes: 0,1,2
		// result indexes:         0,2,5

		// Storages
		// representation indexes: 0,1,2
		// result indexes:         1,3,4

		// Check Products representation & result indexes
		assert.Equal(t, 0, im.entities["Product"][0].representationIndex)
		assert.Equal(t, 0, im.entities["Product"][0].resultIndex)

		assert.Equal(t, 1, im.entities["Product"][1].representationIndex)
		assert.Equal(t, 2, im.entities["Product"][1].resultIndex)

		assert.Equal(t, 2, im.entities["Product"][2].representationIndex)
		assert.Equal(t, 5, im.entities["Product"][2].resultIndex)

		// Check Storages representation & result indexes
		assert.Equal(t, 0, im.entities["Storage"][0].representationIndex)
		assert.Equal(t, 1, im.entities["Storage"][0].resultIndex)

		assert.Equal(t, 1, im.entities["Storage"][1].representationIndex)
		assert.Equal(t, 3, im.entities["Storage"][1].resultIndex)

		assert.Equal(t, 2, im.entities["Storage"][2].representationIndex)
		assert.Equal(t, 4, im.entities["Storage"][2].resultIndex)

		assert.Empty(t, im.typeLinker)
	})

	t.Run("populates typeLinker for interface entity representations", func(t *testing.T) {
		reps := getRepesentations(gjson.Parse(`{"representations":[
			{"__typename":"Resource","id":"1"},
			{"__typename":"Resource","id":"2"}
		]}`))

		im := createEntityIndexMap(def, reps)
		assert.NotNil(t, im)
		assert.Len(t, im.entities["Resource"], 2)

		// All implementing types should be linked back to Resource
		for _, member := range []string{"Product", "Storage", "Warehouse"} {
			iface, ok := im.typeLinker.getLinkedType(member)
			assert.True(t, ok, "expected %s to link to Resource", member)
			assert.Equal(t, "Resource", iface)
		}
	})
}

func TestEntityIndexMap_GetResultIndex(t *testing.T) {
	schema := grpctest.MustGraphQLSchema(t)
	def := &schema

	t.Run("nil receiver returns representation index", func(t *testing.T) {
		var im *entityIndexMap
		val := astjson.MustParse(`{"__typename":"Product"}`)
		assert.Equal(t, 5, im.getResultIndex(val, 5))
	})

	t.Run("nil entities returns representation index", func(t *testing.T) {
		im := &entityIndexMap{}
		val := astjson.MustParse(`{"__typename":"Product"}`)
		assert.Equal(t, 3, im.getResultIndex(val, 3))
	})

	t.Run("nil val returns representation index", func(t *testing.T) {
		reps := getRepesentations(gjson.Parse(`{"representations":[{"__typename":"Product","id":"1"}]}`))
		im := createEntityIndexMap(def, reps)
		assert.Equal(t, 7, im.getResultIndex(nil, 7))
	})

	t.Run("returns mapped result index for known type", func(t *testing.T) {
		reps := getRepesentations(gjson.Parse(`{"representations":[
			{"__typename":"Product","id":"1"},
			{"__typename":"Storage","id":"2"},
			{"__typename":"Product","id":"3"}
		]}`))
		im := createEntityIndexMap(def, reps)

		assert.Equal(t, 0, im.getResultIndex(astjson.MustParse(`{"__typename":"Product"}`), 0))
		assert.Equal(t, 2, im.getResultIndex(astjson.MustParse(`{"__typename":"Product"}`), 1))
		assert.Equal(t, 1, im.getResultIndex(astjson.MustParse(`{"__typename":"Storage"}`), 0))
	})

	t.Run("falls back to linked interface for member type", func(t *testing.T) {
		reps := getRepesentations(gjson.Parse(`{"representations":[
			{"__typename":"Resource","id":"1"},
			{"__typename":"Resource","id":"2"},
			{"__typename":"Resource","id":"3"}
		]}`))
		im := createEntityIndexMap(def, reps)

		// Response __typename is the concrete type but representations used the interface
		assert.Equal(t, 0, im.getResultIndex(astjson.MustParse(`{"__typename":"Product"}`), 0))
		assert.Equal(t, 1, im.getResultIndex(astjson.MustParse(`{"__typename":"Storage"}`), 1))
		assert.Equal(t, 2, im.getResultIndex(astjson.MustParse(`{"__typename":"Warehouse"}`), 2))
	})

	t.Run("unknown type without link returns representation index", func(t *testing.T) {
		reps := getRepesentations(gjson.Parse(`{"representations":[{"__typename":"Product","id":"1"}]}`))
		im := createEntityIndexMap(def, reps)

		assert.Equal(t, 4, im.getResultIndex(astjson.MustParse(`{"__typename":"Unknown"}`), 4))
	})
}

func TestValidateEntityResponse(t *testing.T) {
	reps := getRepesentations(gjson.Parse(`{"representations":[
		{"__typename":"Product","id":"1"},
		{"__typename":"Product","id":"2"}
	]}`))

	t.Run("returns error when data is nil", func(t *testing.T) {
		err := validateEntityResponse(nil, "Product", reps)
		assert.ErrorContains(t, err, "data is required")
	})

	t.Run("returns error when requested entity type is empty", func(t *testing.T) {
		data := astjson.MustParse(`{"_entities":[]}`)
		err := validateEntityResponse(data, "", reps)
		assert.ErrorContains(t, err, "requested entity type is required")
	})

	t.Run("returns error when representations are empty", func(t *testing.T) {
		data := astjson.MustParse(`{"_entities":[]}`)
		err := validateEntityResponse(data, "Product", nil)
		assert.ErrorContains(t, err, "representations are required")
	})

	t.Run("returns error when entity count mismatches representation count", func(t *testing.T) {
		data := astjson.MustParse(`{"_entities":[{"__typename":"Product","id":"1"}]}`)
		err := validateEntityResponse(data, "Product", reps)
		assert.ErrorContains(t, err, "entity type Product received 1 entities in the subgraph response, but 2 are expected")
	})

	t.Run("returns nil when entity count matches representation count", func(t *testing.T) {
		data := astjson.MustParse(`{"_entities":[{"__typename":"Product","id":"1"},{"__typename":"Product","id":"2"}]}`)
		assert.NoError(t, validateEntityResponse(data, "Product", reps))
	})

	t.Run("filters representations by requested type before counting", func(t *testing.T) {
		mixedReps := getRepesentations(gjson.Parse(`{"representations":[
			{"__typename":"Product","id":"1"},
			{"__typename":"Storage","id":"2"},
			{"__typename":"Product","id":"3"}
		]}`))
		data := astjson.MustParse(`{"_entities":[{"__typename":"Product","id":"1"},{"__typename":"Product","id":"3"}]}`)
		assert.NoError(t, validateEntityResponse(data, "Product", mixedReps))
	})

	t.Run("returns error when _entities path is not an array", func(t *testing.T) {
		data := astjson.MustParse(`{"_entities":"not an array"}`)
		err := validateEntityResponse(data, "Product", reps)
		assert.Error(t, err)
	})
}

func TestFilterRepresentations(t *testing.T) {
	reps := getRepesentations(gjson.Parse(`{"representations":[
		{"__typename":"Product","id":"1"},
		{"__typename":"Storage","id":"2"},
		{"__typename":"Product","id":"3"},
		{"__typename":"Warehouse","id":"4"}
	]}`))

	t.Run("returns empty slice when nothing matches", func(t *testing.T) {
		filtered := filterRepresentations(reps, "Unknown")
		assert.NotNil(t, filtered)
		assert.Empty(t, filtered)
	})

	t.Run("returns matching representations only", func(t *testing.T) {
		filtered := filterRepresentations(reps, "Product")
		assert.Equal(t, 2, filtered)
	})

	t.Run("returns single match", func(t *testing.T) {
		filtered := filterRepresentations(reps, "Warehouse")
		assert.Equal(t, 1, filtered)
	})

	t.Run("returns empty slice for empty input", func(t *testing.T) {
		filtered := filterRepresentations(nil, "Product")
		assert.NotNil(t, filtered)
		assert.Empty(t, filtered)
	})
}
