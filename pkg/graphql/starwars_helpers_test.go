package graphql

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/pkg/starwars"
)

func starwarsSchema(t *testing.T) *Schema {
	starwars.SetRelativePathToStarWarsPackage("../starwars")
	schemaBytes := starwars.Schema(t)

	schema, err := NewSchemaFromString(string(schemaBytes))
	require.NoError(t, err)

	return schema
}

func requestForQuery(t *testing.T, fileName string) Request {
	rawRequest := starwars.LoadQuery(t, fileName, nil)

	var request Request
	err := UnmarshalRequest(bytes.NewBuffer(rawRequest), &request)
	require.NoError(t, err)

	return request
}

func loadStarWarsQuery(starwarsFile string, variables starwars.QueryVariables) func(t *testing.T) Request {
	return func(t *testing.T) Request {
		query := starwars.LoadQuery(t, starwarsFile, variables)
		request := Request{}
		err := UnmarshalRequest(bytes.NewBuffer(query), &request)
		require.NoError(t, err)

		return request
	}
}
