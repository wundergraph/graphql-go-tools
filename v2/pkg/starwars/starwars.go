package starwars

import (
	"encoding/json"
	"io/ioutil"
	"path"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/pkg/execution"
	"github.com/wundergraph/graphql-go-tools/pkg/execution/datasource"
)

type QueryVariables map[string]interface{}

const (
	FileSimpleHeroQuery            = "testdata/queries/simple_hero.query"
	FileHeroWithOperationNameQuery = "testdata/queries/hero_with_operation_name.query"
	FileHeroWithAliasesQuery       = "testdata/queries/hero_with_aliases.query"
	FileDroidWithArgQuery          = "testdata/queries/droid_with_arg.query"
	FileDroidWithArgAndVarQuery    = "testdata/queries/droid_with_arg_and_var.query"
	FileFragmentsQuery             = "testdata/queries/fragments.query"
	FileDirectivesIncludeQuery     = "testdata/queries/directives_include.query"
	FileDirectivesSkipQuery        = "testdata/queries/directives_skip.query"
	FileCreateReviewMutation       = "testdata/mutations/create_review.mutation"
	FileInlineFragmentsQuery       = "testdata/queries/inline_fragments.query"
	FileUnionQuery                 = "testdata/queries/inline_fragments.query"
	FileRemainingJedisSubscription = "testdata/subscriptions/remaining_jedis.subscription"
	FileIntrospectionQuery         = "testdata/queries/introspection.query"
	FileMultiQueries               = "testdata/queries/multi_queries.query"
	FileMultiQueriesWithArguments  = "testdata/queries/multi_queries_with_arguments.query"
	FileInvalidQuery               = "testdata/queries/invalid.query"
	FileInvalidFragmentsQuery      = "testdata/queries/invalid_fragments.query"
	FileInterfaceFragmentsOnUnion  = "testdata/queries/interface_fragments_on_union.graphql"
)

var (
	testdataPath = "./"
)

type TestCase struct {
	Name        string
	RequestBody []byte
}

func SetRelativePathToStarWarsPackage(path string) {
	testdataPath = path
}

func NewExecutionHandler(t *testing.T) *execution.Handler {
	base, err := datasource.NewBaseDataSourcePlanner(Schema(t), datasource.PlannerConfiguration{}, abstractlogger.NoopLogger)
	require.NoError(t, err)
	executionHandler := execution.NewHandler(base, nil)
	return executionHandler
}

func Schema(t *testing.T) []byte {
	schema, err := ioutil.ReadFile(path.Join(testdataPath, "testdata/star_wars.graphql"))
	require.NoError(t, err)
	return schema
}

func LoadQuery(t *testing.T, fileName string, variables QueryVariables) []byte {
	query, err := ioutil.ReadFile(path.Join(testdataPath, fileName))
	require.NoError(t, err)

	return RequestBody(t, string(query), variables)
}

func InvalidQueryRequestBody(t *testing.T) []byte {
	return RequestBody(t, "query { trap { meme } }", nil)
}

func RequestBody(t *testing.T, query string, variables QueryVariables) []byte {
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

func ReviewInput() map[string]interface{} {
	return map[string]interface{}{
		"stars":      5,
		"commentary": "This is a great movie!",
	}
}
