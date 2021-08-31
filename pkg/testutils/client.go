package testutils

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jensneuse/graphql-go-tools/pkg/graphql"
)

type QueryVariables map[string]interface{}

func RequestBody(t *testing.T, query string, variables QueryVariables) []byte {
	var variableJsonBytes []byte
	if len(variables) > 0 {
		var err error
		variableJsonBytes, err = json.Marshal(variables)
		require.NoError(t, err)
	}

	body := graphql.Request{
		OperationName: "",
		Variables:     variableJsonBytes,
		Query:         query,
	}

	jsonBytes, err := json.Marshal(body)
	require.NoError(t, err)

	return jsonBytes
}

func LoadQuery(t *testing.T, filePath string, variables QueryVariables) []byte {
	query, err := ioutil.ReadFile(filePath)
	require.NoError(t, err)

	return RequestBody(t, string(query), variables)
}

func ExecuteQuery(ctx context.Context, body []byte, variables QueryVariables) {

}