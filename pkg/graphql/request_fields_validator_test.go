package graphql

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/pkg/starwars"
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

		validator := fieldsValidator{}
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

		validator := fieldsValidator{}
		result, err := validator.Validate(&request, schema, blockedFields)
		assert.NoError(t, err)
		assert.True(t, result.Valid)
		assert.Equal(t, 0, result.Errors.Count())
	})
}
