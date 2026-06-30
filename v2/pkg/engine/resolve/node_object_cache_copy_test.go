package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestObjectAndFieldCopyCarryCacheMetadata(t *testing.T) {
	obj := &Object{
		HasAliases: true,
		Fields: []*Field{
			{
				Name:         []byte("displayName"),
				OriginalName: []byte("name"),
				Value:        &String{},
				CacheArgs: []CacheFieldArg{
					{Name: "id", VariableName: "productID"},
				},
			},
		},
	}

	copied := obj.Copy()

	assert.Equal(t, &Object{
		HasAliases: true,
		Fields: []*Field{
			{
				Name:         []byte("displayName"),
				OriginalName: []byte("name"),
				Value:        &String{},
				CacheArgs: []CacheFieldArg{
					{Name: "id", VariableName: "productID"},
				},
			},
		},
	}, copied)
}
