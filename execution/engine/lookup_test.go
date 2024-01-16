package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/graphql"
)

func TestCreateTypeFieldLookupKey(t *testing.T) {
	lookupKey := CreateTypeFieldLookupKey("Query", "hello")
	assert.Equal(t, TypeFieldLookupKey("Query.hello"), lookupKey)
}

func TestCreateTypeFieldArgumentsLookupMap(t *testing.T) {
	t.Run("should return nil if slice is empty", func(t *testing.T) {
		lookupMap := CreateTypeFieldArgumentsLookupMap([]graphql.TypeFieldArguments{})
		assert.Nil(t, lookupMap)
	})

	t.Run("should return a lookup map", func(t *testing.T) {
		typeFieldArgs := []graphql.TypeFieldArguments{
			{
				TypeName:      "Query",
				FieldName:     "hello",
				ArgumentNames: []string{"name"},
			},
		}

		lookupMap := CreateTypeFieldArgumentsLookupMap(typeFieldArgs)
		assert.Equal(t, lookupMap[TypeFieldLookupKey("Query.hello")], typeFieldArgs[0])
	})
}
