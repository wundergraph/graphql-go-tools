package graphql

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/starwars"
)

func TestFieldsValidator_Validate(t *testing.T) {
	schema := starwarsSchema(t)
	request := requestForQuery(t, starwars.FileSimpleHeroQuery)

	t.Run("should invalidate if blocked fields are used", func(t *testing.T) {

		blockedFields := []Type{
			{
				Name:   "Character",
				Fields: []string{"name"},
			},
		}

		validator := DefaultFieldsValidator{}
		result, err := validator.Validate(&request, schema, blockedFields)
		assert.NoError(t, err)
		assert.False(t, result.Valid)
		assert.Equal(t, 1, result.Errors.Count())
	})

	t.Run("should validate if non-blocked fields are used", func(t *testing.T) {

		blockedFields := []Type{
			{
				Name:   "Character",
				Fields: []string{"friends"},
			},
		}

		validator := DefaultFieldsValidator{}
		result, err := validator.Validate(&request, schema, blockedFields)
		assert.NoError(t, err)
		assert.True(t, result.Valid)
		assert.Equal(t, 0, result.Errors.Count())
	})
}

func TestFieldsValidator_ValidateByFieldList(t *testing.T) {
	schema := starwarsSchema(t)
	request := requestForQuery(t, starwars.FileSimpleHeroQuery)

	t.Run("block list", func(t *testing.T) {
		t.Run("should invalidate if blocked fields are used", func(t *testing.T) {
			blockList := FieldRestrictionList{
				Kind: BlockList,
				Types: []Type{
					{
						Name:   "Character",
						Fields: []string{"name"},
					},
				},
			}

			validator := DefaultFieldsValidator{}
			result, err := validator.ValidateByFieldList(&request, schema, blockList)
			assert.NoError(t, err)
			assert.False(t, result.Valid)
			assert.Equal(t, 1, result.Errors.Count())
		})

		t.Run("should validate if non-blocked fields are used", func(t *testing.T) {
			blockList := FieldRestrictionList{
				Kind: BlockList,
				Types: []Type{
					{
						Name:   "Character",
						Fields: []string{"friends"},
					},
				},
			}

			validator := DefaultFieldsValidator{}
			result, err := validator.ValidateByFieldList(&request, schema, blockList)
			assert.NoError(t, err)
			assert.True(t, result.Valid)
			assert.Equal(t, 0, result.Errors.Count())
		})
	})

	t.Run("allow list", func(t *testing.T) {
		t.Run("should invalidate if a field which is not allowed is used", func(t *testing.T) {
			allowList := FieldRestrictionList{
				Kind: AllowList,
				Types: []Type{
					{
						Name:   "Query",
						Fields: []string{"hero"},
					},
					{
						Name:   "Character",
						Fields: []string{"friends"},
					},
				},
			}

			validator := DefaultFieldsValidator{}
			result, err := validator.ValidateByFieldList(&request, schema, allowList)
			assert.NoError(t, err)
			assert.False(t, result.Valid)
			assert.Equal(t, 1, result.Errors.Count())
		})

		t.Run("should validate if all fields are allowed", func(t *testing.T) {
			allowList := FieldRestrictionList{
				Kind: AllowList,
				Types: []Type{
					{
						Name:   "Query",
						Fields: []string{"hero"},
					},
					{
						Name:   "Character",
						Fields: []string{"name"},
					},
				},
			}

			validator := DefaultFieldsValidator{}
			result, err := validator.ValidateByFieldList(&request, schema, allowList)
			assert.NoError(t, err)
			assert.True(t, result.Valid)
			assert.Equal(t, 0, result.Errors.Count())
		})
	})

}
