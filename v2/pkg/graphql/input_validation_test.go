package graphql

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/starwars"
)

func TestRequest_ValidateInput(t *testing.T) {
	t.Run("Should pass input validation", func(t *testing.T) {
		schema := starwarsSchema(t)
		request := requestForQuery(t, starwars.FileDroidWithArgAndVarQuery)
		request.Variables = []byte(`{"droidID":"testID"}`)

		result, err := request.ValidateInput(schema)
		assert.NoError(t, err)
		assert.True(t, result.Valid)
		assert.Nil(t, result.Errors)
	})

	t.Run("Should fail input validation", func(t *testing.T) {
		schema := starwarsSchema(t)
		request := requestForQuery(t, starwars.FileDroidWithArgAndVarQuery)

		result, err := request.ValidateInput(schema)
		assert.NoError(t, err)
		assert.False(t, result.Valid)
		assert.Equal(t, `Variable "$droidID" of required type "ID!" was not provided., locations: [], path: []`, result.Errors.Error())
	})
}
