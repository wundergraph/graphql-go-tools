package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"

	"github.com/wundergraph/astjson"
)

func TestNewEntityIndexMap(t *testing.T) {
	t.Run("returns empty map when no representations match", func(t *testing.T) {
		reps := getRepesentations(gjson.Parse(`{"representations":[
			{"__typename":"Storage","id":"1"}
		]}`))
		idx := newEntityIndexMap("Product", reps)
		assert.Equal(t, entityIndexMap{}, idx)
	})

	t.Run("returns empty map when representations are nil", func(t *testing.T) {
		idx := newEntityIndexMap("Product", nil)
		assert.Equal(t, entityIndexMap{}, idx)
	})

	t.Run("ordered representations [Product, Product, Storage, Storage]", func(t *testing.T) {
		reps := getRepesentations(gjson.Parse(`{"representations":[
			{"__typename":"Product","id":"1"},
			{"__typename":"Product","id":"2"},
			{"__typename":"Storage","id":"3"},
			{"__typename":"Storage","id":"4"}
		]}`))

		productIdx := newEntityIndexMap("Product", reps)
		assert.Equal(t, entityIndexMap{0, 1}, productIdx)

		storageIdx := newEntityIndexMap("Storage", reps)
		assert.Equal(t, entityIndexMap{2, 3}, storageIdx)
	})

	t.Run("unordered representations [Product, Storage, Product, Storage]", func(t *testing.T) {
		reps := getRepesentations(gjson.Parse(`{"representations":[
			{"__typename":"Product","id":"1"},
			{"__typename":"Storage","id":"2"},
			{"__typename":"Product","id":"3"},
			{"__typename":"Storage","id":"4"}
		]}`))

		productIdx := newEntityIndexMap("Product", reps)
		assert.Equal(t, entityIndexMap{0, 2}, productIdx)

		storageIdx := newEntityIndexMap("Storage", reps)
		assert.Equal(t, entityIndexMap{1, 3}, storageIdx)
	})

	t.Run("interleaved representations across three types", func(t *testing.T) {
		reps := getRepesentations(gjson.Parse(`{"representations":[
			{"__typename":"Product","id":"1"},
			{"__typename":"Storage","id":"2"},
			{"__typename":"Warehouse","id":"3"},
			{"__typename":"Product","id":"4"},
			{"__typename":"Warehouse","id":"5"},
			{"__typename":"Storage","id":"6"}
		]}`))

		assert.Equal(t, entityIndexMap{0, 3}, newEntityIndexMap("Product", reps))
		assert.Equal(t, entityIndexMap{1, 5}, newEntityIndexMap("Storage", reps))
		assert.Equal(t, entityIndexMap{2, 4}, newEntityIndexMap("Warehouse", reps))
	})

	t.Run("single matching representation", func(t *testing.T) {
		reps := getRepesentations(gjson.Parse(`{"representations":[
			{"__typename":"Storage","id":"1"},
			{"__typename":"Product","id":"2"},
			{"__typename":"Storage","id":"3"}
		]}`))

		assert.Equal(t, entityIndexMap{1}, newEntityIndexMap("Product", reps))
	})

	t.Run("preserves original positions for fully matching list", func(t *testing.T) {
		reps := getRepesentations(gjson.Parse(`{"representations":[
			{"__typename":"Product","id":"1"},
			{"__typename":"Product","id":"2"},
			{"__typename":"Product","id":"3"}
		]}`))

		assert.Equal(t, entityIndexMap{0, 1, 2}, newEntityIndexMap("Product", reps))
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
		filtered := filteredRepresentationCount(reps, "Unknown")
		assert.NotNil(t, filtered)
		assert.Empty(t, filtered)
	})

	t.Run("returns matching representations only", func(t *testing.T) {
		filtered := filteredRepresentationCount(reps, "Product")
		assert.Equal(t, 2, filtered)
	})

	t.Run("returns single match", func(t *testing.T) {
		filtered := filteredRepresentationCount(reps, "Warehouse")
		assert.Equal(t, 1, filtered)
	})

	t.Run("returns empty slice for empty input", func(t *testing.T) {
		filtered := filteredRepresentationCount(nil, "Product")
		assert.NotNil(t, filtered)
		assert.Empty(t, filtered)
	})
}
