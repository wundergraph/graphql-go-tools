package http

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/require"

	"github.com/jensneuse/graphql-go-tools/pkg/execution"
)

type queryVariables map[string]interface{}

const (
	fileSimpleHeroQuery            = "./testdata/queries/simple_hero.query"
	fileHeroWithOperationNameQuery = "./testdata/queries/hero_with_operation_name.query"
	fileHeroWithAliasesQuery       = "./testdata/queries/hero_with_aliases.query"
	fileDroidWithArgAndVarQuery    = "./testdata/queries/droid_with_arg_and_var.query"
	fileFragmentsQuery             = "./testdata/queries/fragments.query"
	fileDirectivesIncludeQuery     = "./testdata/queries/directives_include.query"
	fileDirectivesSkipQuery        = "./testdata/queries/directives_skip.query"
	fileCreateReviewMutation       = "./testdata/mutations/create_review.mutation"
	fileInlineFragmentsQuery       = "./testdata/queries/inline_fragments.query"
	fileUnionQuery                 = "./testdata/queries/inline_fragments.query"
	fileRemainingJedisSubscription = "./testdata/subscriptions/remaining_jedis.subscription"
)

type starWarsTestCase struct {
	name        string
	requestBody []byte
}

func newStarWarsExecutionHandler(t *testing.T) *execution.Handler {
	base, err := execution.NewBaseDataSourcePlanner(starWarsSchema(t), execution.PlannerConfiguration{}, abstractlogger.NoopLogger)
	require.NoError(t, err)
	executionHandler := execution.NewHandler(base, nil)
	return executionHandler
}

func starWarsSchema(t *testing.T) []byte {
	schema, err := ioutil.ReadFile("./testdata/star_wars.graphql")
	require.NoError(t, err)
	return schema
}

func starWarsLoadQuery(t *testing.T, fileName string, variables queryVariables) []byte {
	query, err := ioutil.ReadFile(fileName)
	require.NoError(t, err)

	return starWarsRequestBody(t, string(query), variables)
}

func invalidQueryRequestBody(t *testing.T) []byte {
	return starWarsRequestBody(t, "query { trap { meme } }", nil)
}

func starWarsRequestBody(t *testing.T, query string, variables queryVariables) []byte {
	var variableJsonBytes []byte
	if len(variables) > 0 {
		var err error
		variableJsonBytes, err = json.Marshal(variables)
		require.NoError(t, err)
	}

	body := execution.GraphqlRequest{
		OperationName: "",
		Variables:     variableJsonBytes,
		Query:         query,
	}

	jsonBytes, err := json.Marshal(body)
	require.NoError(t, err)

	return jsonBytes
}

func starWarsReviewInput() map[string]interface{} {
	return map[string]interface{}{
		"stars":      5,
		"commentary": "This is a great movie!",
	}
}
