package engine

import (
	"context"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// Two independent fetch chains:
//   - alpha root -> b entity fetch,
//   - and b root -> alpha entity fetch.
const scheduleFetchesSchema = `
	type Query {
		a: A
		aTwo: A
		b: B
	}
	type A {
		id: ID!
		bField: String
	}
	type B {
		id: ID!
		aField: String
	}
`
const scheduleFetchesAlphaSDL = `
	type Query {
		a: A
		aTwo: A
	}
	type A @key(fields: "id") {
		id: ID!
	}
	type B @key(fields: "id") {
		id: ID!
		aField: String
	}
`
const scheduleFetchesBetaSDL = `
	type Query {
		b: B
	}
	type B @key(fields: "id") {
		id: ID!
	}
	type A @key(fields: "id") {
		id: ID!
		bField: String
	}
`
const scheduleFetchesQuery = `
	query {
		a { bField }
		b { aField }
	}
`
const scheduleFetchesCombinedQuery = `
	query {
		a { bField }
		aTwo { bField }
		b { aField }
	}
`

const (
	scheduleFetchesAlphaRootBody   = `{"query":"{a {__typename id}}"}`
	scheduleFetchesAlphaRootData   = `{"data":{"a":{"__typename":"A","id":"1"}}}`
	scheduleFetchesAlphaEntityBody = `{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on B {__typename aField}}}","variables":{"representations":[{"__typename":"B","id":"2"}]}}`
	scheduleFetchesAlphaEntityData = `{"data":{"_entities":[{"__typename":"B","aField":"a"}]}}`

	scheduleFetchesBetaRootBody   = `{"query":"{b {__typename id}}"}`
	scheduleFetchesBetaRootData   = `{"data":{"b":{"__typename":"B","id":"2"}}}`
	scheduleFetchesBetaEntityBody = `{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on A {__typename bField}}}","variables":{"representations":[{"__typename":"A","id":"1"}]}}`
	scheduleFetchesBetaEntityData = `{"data":{"_entities":[{"__typename":"A","bField":"b"}]}}`

	scheduleFetchesClientResponse = `{"data":{"a":{"bField":"b"},"b":{"aField":"a"}}}`

	scheduleFetchesCombinedAlphaRootBody = `{"query":"{a {__typename id} aTwo {__typename id}}"}`
	scheduleFetchesCombinedAlphaRootData = `{"data":{"a":{"__typename":"A","id":"1"},"aTwo":{"__typename":"A","id":"3"}}}`
	scheduleFetchesMergedBetaBody        = `{"query":"query($representations_f1: [_Any!]!, $includeF1: Boolean!, $representations_f2: [_Any!]!, $includeF2: Boolean!){f1: _entities(representations: $representations_f1)@include(if: $includeF1) {... on A {__typename bField}} f2: _entities(representations: $representations_f2)@include(if: $includeF2) {... on A {__typename bField}}}","variables":{"representations_f1":[{"__typename":"A","id":"1"}],"includeF1":true,"representations_f2":[{"__typename":"A","id":"3"}],"includeF2":true}}`
	scheduleFetchesMergedBetaData        = `{"data":{"f1":[{"__typename":"A","bField":"b"}],"f2":[{"__typename":"A","bField":"b2"}]}}`

	scheduleFetchesCombinedClientResponse = `{"data":{"a":{"bField":"b"},"aTwo":{"bField":"b2"},"b":{"aField":"a"}}}`
)

func scheduleFetchesDataSource(t *testing.T, name, sdl string, rec *multiFetchRecorder, responses map[string]sendResponse, rootNodes []plan.TypeField) plan.DataSource {
	t.Helper()

	return mustGraphqlDataSourceConfiguration(t,
		name,
		mustFactory(t, recordingClient(t, rec, name, "/", responses)),
		&plan.DataSourceMetadata{
			RootNodes: rootNodes,
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "A", SelectionSet: "id"},
					{TypeName: "B", SelectionSet: "id"},
				},
			},
		},
		mustConfiguration(t, graphql_datasource.ConfigurationInput{
			Fetch: &graphql_datasource.FetchConfiguration{URL: "http://" + name + "/", Method: "POST"},
			SchemaConfiguration: mustSchemaConfig(t,
				&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: sdl},
				sdl,
			),
		}),
	)
}

func scheduleFetchesDataSources(t *testing.T, aRec, bRec *multiFetchRecorder) []plan.DataSource {
	t.Helper()

	a := scheduleFetchesDataSource(t, "a", scheduleFetchesAlphaSDL, aRec,
		map[string]sendResponse{
			scheduleFetchesAlphaRootBody:         {statusCode: 200, body: scheduleFetchesAlphaRootData},
			scheduleFetchesAlphaEntityBody:       {statusCode: 200, body: scheduleFetchesAlphaEntityData},
			scheduleFetchesCombinedAlphaRootBody: {statusCode: 200, body: scheduleFetchesCombinedAlphaRootData},
		},
		[]plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"a", "aTwo"}},
			{TypeName: "A", FieldNames: []string{"id"}},
			{TypeName: "B", FieldNames: []string{"id", "aField"}},
		})

	b := scheduleFetchesDataSource(t, "b", scheduleFetchesBetaSDL, bRec,
		map[string]sendResponse{
			scheduleFetchesBetaRootBody:   {statusCode: 200, body: scheduleFetchesBetaRootData},
			scheduleFetchesBetaEntityBody: {statusCode: 200, body: scheduleFetchesBetaEntityData},
			scheduleFetchesMergedBetaBody: {statusCode: 200, body: scheduleFetchesMergedBetaData},
		},
		[]plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"b"}},
			{TypeName: "B", FieldNames: []string{"id"}},
			{TypeName: "A", FieldNames: []string{"id", "bField"}},
		})

	return []plan.DataSource{a, b}
}

// runScheduleFetchesQuery plans and executes the query and returns
// the response body, the organized fetch tree, and the request bodies each subgraph received.
func runScheduleFetchesQuery(t *testing.T, query string, enableMultiFetch, enableScheduleFetches bool) (string, *resolve.FetchTreeNode, []string, []string) {
	t.Helper()

	schema, err := graphql.NewSchemaFromString(scheduleFetchesSchema)
	require.NoError(t, err)

	aRec, bRec := &multiFetchRecorder{}, &multiFetchRecorder{}
	engineConf := NewConfiguration(schema)
	engineConf.SetDataSources(scheduleFetchesDataSources(t, aRec, bRec))
	if enableMultiFetch {
		engineConf.EnableMultiFetch()
	}
	if enableScheduleFetches {
		engineConf.EnableScheduleFetches()
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	engine, err := NewExecutionEngine(ctx, abstractlogger.Noop{}, engineConf, resolve.ResolverOptions{MaxConcurrency: 1024})
	require.NoError(t, err)

	operation := graphql.Request{Query: query}
	resultWriter := graphql.NewEngineResultWriter()
	require.NoError(t, engine.Execute(ctx, &operation, &resultWriter))

	require.Equal(t, 1, engine.executionPlanCache.Len())
	_, cachedPlan, ok := engine.executionPlanCache.GetOldest()
	require.True(t, ok)
	syncPlan, ok := cachedPlan.(*plan.SynchronousResponsePlan)
	require.True(t, ok)
	return resultWriter.String(), syncPlan.Response.Fetches, aRec.requests(), bRec.requests()
}

func nodeKinds(nodes []*resolve.FetchTreeNode) []resolve.FetchTreeNodeKind {
	kinds := make([]resolve.FetchTreeNodeKind, len(nodes))
	for i, n := range nodes {
		kinds[i] = n.Kind
	}
	return kinds
}

func TestExecutionEngine_ScheduleFetches(t *testing.T) {
	// Request bodies are asserted as sets: under the scheduler the chains progress independently,
	// so arrival order at a host is not deterministic.
	cases := []struct {
		name                 string
		query                string
		multiFetch, schedule bool
		response             string
		aReqs, bReqs         []string
		rootKind             resolve.FetchTreeNodeKind
		childKinds           []resolve.FetchTreeNodeKind
	}{
		{
			name:       "default organizes legacy waves",
			query:      scheduleFetchesQuery,
			response:   scheduleFetchesClientResponse,
			aReqs:      []string{scheduleFetchesAlphaRootBody, scheduleFetchesAlphaEntityBody},
			bReqs:      []string{scheduleFetchesBetaRootBody, scheduleFetchesBetaEntityBody},
			rootKind:   resolve.FetchTreeNodeKindSequence,
			childKinds: []resolve.FetchTreeNodeKind{resolve.FetchTreeNodeKindParallel, resolve.FetchTreeNodeKindParallel},
		},
		{
			name:       "scheduling organizes independent inlined chains",
			query:      scheduleFetchesQuery,
			schedule:   true,
			response:   scheduleFetchesClientResponse,
			aReqs:      []string{scheduleFetchesAlphaRootBody, scheduleFetchesAlphaEntityBody},
			bReqs:      []string{scheduleFetchesBetaRootBody, scheduleFetchesBetaEntityBody},
			rootKind:   resolve.FetchTreeNodeKindParallel,
			childKinds: []resolve.FetchTreeNodeKind{resolve.FetchTreeNodeKindSequence, resolve.FetchTreeNodeKindSequence},
		},
		{
			name:       "multi fetch without scheduling merges within legacy waves",
			query:      scheduleFetchesCombinedQuery,
			multiFetch: true,
			response:   scheduleFetchesCombinedClientResponse,
			aReqs:      []string{scheduleFetchesCombinedAlphaRootBody, scheduleFetchesAlphaEntityBody},
			// The two same-wave b entity fetches merged into one aliased request,
			// while the tree keeps the legacy wave shape.
			bReqs:      []string{scheduleFetchesBetaRootBody, scheduleFetchesMergedBetaBody},
			rootKind:   resolve.FetchTreeNodeKindSequence,
			childKinds: []resolve.FetchTreeNodeKind{resolve.FetchTreeNodeKindParallel, resolve.FetchTreeNodeKindParallel},
		},
		{
			name:       "multi fetch and scheduling together produce two inlined chains",
			query:      scheduleFetchesCombinedQuery,
			multiFetch: true,
			schedule:   true,
			response:   scheduleFetchesCombinedClientResponse,
			aReqs:      []string{scheduleFetchesCombinedAlphaRootBody, scheduleFetchesAlphaEntityBody},
			// The two same-wave b entity fetches merged into one aliased request.
			bReqs:      []string{scheduleFetchesBetaRootBody, scheduleFetchesMergedBetaBody},
			rootKind:   resolve.FetchTreeNodeKindParallel,
			childKinds: []resolve.FetchTreeNodeKind{resolve.FetchTreeNodeKindSequence, resolve.FetchTreeNodeKindSequence},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response, fetches, aReqs, bReqs := runScheduleFetchesQuery(t, tc.query, tc.multiFetch, tc.schedule)
			require.Equal(t, tc.response, response)
			require.ElementsMatch(t, tc.aReqs, aReqs)
			require.ElementsMatch(t, tc.bReqs, bReqs)
			require.Equal(t, tc.rootKind, fetches.Kind)
			require.Equal(t, tc.childKinds, nodeKinds(fetches.ChildNodes))
		})
	}
}
