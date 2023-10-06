package starwars

import (
	"encoding/json"
	"os"
	"path"

	"github.com/stretchr/testify/require"
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

type TestingTB interface {
	Errorf(format string, args ...interface{})
	Helper()
	FailNow()
}

type TestCase struct {
	Name        string
	RequestBody []byte
}

type GraphqlRequest struct {
	OperationName string          `json:"operationName"`
	Variables     json.RawMessage `json:"variables"`
	Query         string          `json:"query"`
}

func SetRelativePathToStarWarsPackage(path string) {
	testdataPath = path
}

func Schema(t TestingTB) []byte {
	schema, err := os.ReadFile(path.Join(testdataPath, "testdata/star_wars.graphql"))
	require.NoError(t, err)
	return schema
}

func LoadQuery(t TestingTB, fileName string, variables QueryVariables) []byte {
	query, err := os.ReadFile(path.Join(testdataPath, fileName))
	require.NoError(t, err)

	return RequestBody(t, string(query), variables)
}

func RequestBody(t TestingTB, query string, variables QueryVariables) []byte {
	var variableJsonBytes []byte
	if len(variables) > 0 {
		var err error
		variableJsonBytes, err = json.Marshal(variables)
		require.NoError(t, err)
	}

	body := GraphqlRequest{
		OperationName: "",
		Variables:     variableJsonBytes,
		Query:         query,
	}

	jsonBytes, err := json.Marshal(body)
	require.NoError(t, err)

	return jsonBytes
}
