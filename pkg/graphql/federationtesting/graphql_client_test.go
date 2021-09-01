package federationtesting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jensneuse/graphql-go-tools/pkg/graphql"
)

type queryVariables map[string]interface{}

func requestBody(t *testing.T, query string, variables queryVariables) []byte {
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

func loadQuery(t *testing.T, filePath string, variables queryVariables) []byte {
	query, err := ioutil.ReadFile(filePath)
	require.NoError(t, err)

	return requestBody(t, string(query), variables)
}

func NewGraphqlClient(httpClient *http.Client) *GraphqlClient {
	return &GraphqlClient{
		httpClient: httpClient,
	}
}

type GraphqlClient struct {
	httpClient *http.Client
}

func (g *GraphqlClient) Query(ctx context.Context, addr, queryFilePath string, variables queryVariables, t *testing.T) []byte {
	reqBody := loadQuery(t, queryFilePath, variables)
	req, err := http.NewRequest(http.MethodPost, addr, bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	req = req.WithContext(ctx)
	resp, err := g.httpClient.Do(req)
	require.NoError(t, err)
	responseBodyBytes, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	if http.StatusOK != resp.StatusCode {
		fmt.Println(">>>", string(responseBodyBytes))
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	return responseBodyBytes
}
