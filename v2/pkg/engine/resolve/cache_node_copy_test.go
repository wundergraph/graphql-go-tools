package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestObjectCopyCarriesCacheMetadata pins that Object.Copy / Field.Copy carry
// the caching planner metadata (HasAliases, OriginalName, CacheArgs), so
// defer's response-tree copies never lose it.
func TestObjectCopyCarriesCacheMetadata(t *testing.T) {
	original := &Object{
		Nullable:   true,
		Path:       []string{"product"},
		HasAliases: true,
		Fields: []*Field{
			{
				Name:         []byte("productName"),
				OriginalName: []byte("name"),
				Value: &String{
					Path: []string{"productName"},
				},
			},
			{
				Name: []byte("price"),
				CacheArgs: []CacheFieldArg{
					{Name: "currency", VariableName: "a"},
					{Name: "region", VariableName: "b"},
				},
				Value: &Float{
					Path: []string{"price"},
				},
			},
		},
	}

	copied := original.Copy()

	assert.Equal(t, &Object{
		Nullable:   true,
		Path:       []string{"product"},
		HasAliases: true,
		Fields: []*Field{
			{
				Name:         []byte("productName"),
				OriginalName: []byte("name"),
				Value: &String{
					Path: []string{"productName"},
				},
			},
			{
				Name: []byte("price"),
				CacheArgs: []CacheFieldArg{
					{Name: "currency", VariableName: "a"},
					{Name: "region", VariableName: "b"},
				},
				Value: &Float{
					Path: []string{"price"},
				},
			},
		},
	}, copied)

	// CacheArgs must be cloned, not aliased: mutating the copy must not reach
	// the original.
	copiedObject := copied.(*Object)
	copiedObject.Fields[1].CacheArgs[0].VariableName = "mutated"
	assert.Equal(t, "a", original.Fields[1].CacheArgs[0].VariableName)
}

// TestGraphQLResponseCacheProvidesData pins the ProvidesData side-table
// accessors on the response.
func TestGraphQLResponseCacheProvidesData(t *testing.T) {
	response := &GraphQLResponse{}
	assert.Nil(t, response.CacheProvidesData())

	info := &FetchInfo{DataSourceID: "products"}
	providesData := map[*FetchInfo]*Object{
		info: {
			Fields: []*Field{
				{
					Name:  []byte("name"),
					Value: &String{Path: []string{"name"}},
				},
			},
		},
	}
	response.SetCacheProvidesData(providesData)
	assert.Equal(t, providesData, response.CacheProvidesData())
}
