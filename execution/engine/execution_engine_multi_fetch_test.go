package engine

// Full federation integration tests for the MultiFetch optimization.

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// multiFetchRecorder records the request bodies a subgraph receives so a test
// can assert both the number of requests and their exact bytes.
type multiFetchRecorder struct {
	mu     sync.Mutex
	bodies []string
}

func (r *multiFetchRecorder) record(body string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bodies = append(r.bodies, body)
}

func (r *multiFetchRecorder) requests() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.bodies))
	copy(out, r.bodies)
	return out
}

// recordingClient returns an *http.Client whose round tripper records every
// request body into rec and replies from responses (keyed by exact body).
func recordingClient(t *testing.T, rec *multiFetchRecorder, host, path string, responses map[string]sendResponse) *http.Client {
	t.Helper()
	rt := testRoundTripper(func(req *http.Request) *http.Response {
		assert.Equal(t, host, req.URL.Host)
		assert.Equal(t, path, req.URL.Path)
		require.NotNil(t, req.Body)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		if rec != nil {
			rec.record(string(body))
		}
		resp, ok := responses[string(body)]
		if !ok {
			t.Logf("[%s] unexpected body: %s", host, string(body))
			return &http.Response{StatusCode: 400, Body: io.NopCloser(bytes.NewBufferString("unexpected body"))}
		}
		return &http.Response{StatusCode: resp.statusCode, Body: io.NopCloser(bytes.NewBufferString(resp.body))}
	})
	return &http.Client{Transport: rt}
}

// multiFetchSchema is the client-facing supergraph. The accounts subgraph owns
// the two Query roots and Employee.id; the products subgraph extends Employee
// with products and notes. A query selecting through both roots produces two
// same-subgraph (products) entity fetches in the same parallel wave, the
// MultiFetch merge target.
const multiFetchSchema = `
	type Query {
		employees: [Employee!]!
		topEmployee: Employee
	}
	type Employee {
		id: ID!
		products: [Product!]
		notes: String
	}
	type Product {
		upc: String!
	}
`

const multiFetchAccountsSDL = `
	type Query {
		employees: [Employee!]!
		topEmployee: Employee
	}
	type Employee @key(fields: "id") {
		id: ID!
	}
`

const multiFetchProductsSDL = `
	type Employee @key(fields: "id") {
		id: ID!
		products: [Product!]
		notes: String
	}
	type Product {
		upc: String!
	}
`

const multiFetchQuery = `
	query {
		employees { products { upc } }
		topEmployee { notes }
	}
`

// multiFetchDataSources builds the accounts + products federation data sources.
// productsRec (if non-nil) records the request bodies the products subgraph
// receives, so a test can assert the request count and bytes.
func multiFetchDataSources(t *testing.T, productsRec *multiFetchRecorder, accountsResponses, productsResponses map[string]sendResponse) []plan.DataSource {
	t.Helper()

	accounts := mustGraphqlDataSourceConfiguration(t,
		"accounts",
		mustFactory(t, recordingClient(t, nil, "accounts", "/", accountsResponses)),
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"employees", "topEmployee"}},
				{TypeName: "Employee", FieldNames: []string{"id"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Employee", SelectionSet: "id"},
				},
			},
		},
		mustConfiguration(t, graphql_datasource.ConfigurationInput{
			Fetch: &graphql_datasource.FetchConfiguration{URL: "http://accounts/", Method: "POST"},
			SchemaConfiguration: mustSchemaConfig(t,
				&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: multiFetchAccountsSDL},
				multiFetchAccountsSDL,
			),
		}),
	)

	products := mustGraphqlDataSourceConfiguration(t,
		"products",
		mustFactory(t, recordingClient(t, productsRec, "products", "/", productsResponses)),
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Employee", FieldNames: []string{"id", "products", "notes"}},
			},
			ChildNodes: []plan.TypeField{
				{TypeName: "Product", FieldNames: []string{"upc"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Employee", SelectionSet: "id"},
				},
			},
		},
		mustConfiguration(t, graphql_datasource.ConfigurationInput{
			Fetch: &graphql_datasource.FetchConfiguration{URL: "http://products/", Method: "POST"},
			SchemaConfiguration: mustSchemaConfig(t,
				&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: multiFetchProductsSDL},
				multiFetchProductsSDL,
			),
		}),
	)

	return []plan.DataSource{accounts, products}
}

// runMultiFetchQuery plans and executes multiFetchQuery against the given data
// sources, toggling MultiFetch via the engine.Configuration setter under test.
func runMultiFetchQuery(t *testing.T, dataSources []plan.DataSource, enableMultiFetch bool) (string, error) {
	t.Helper()

	schema, err := graphql.NewSchemaFromString(multiFetchSchema)
	require.NoError(t, err)

	engineConf := NewConfiguration(schema)
	engineConf.SetDataSources(dataSources)
	if enableMultiFetch {
		engineConf.EnableMultiFetch()
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	engine, err := NewExecutionEngine(ctx, abstractlogger.Noop{}, engineConf, resolve.ResolverOptions{MaxConcurrency: 1024})
	require.NoError(t, err)

	operation := graphql.Request{Query: multiFetchQuery}
	resultWriter := graphql.NewEngineResultWriter()
	execErr := engine.Execute(ctx, &operation, &resultWriter)
	return resultWriter.String(), execErr
}

// Golden bodies/responses, all derived from observed engine output (never hand
// guessed) and asserted with full equality.
const (
	// accounts root fetch: both Query roots resolve here, one request.
	multiFetchAccountsBody = `{"query":"{employees {__typename id} topEmployee {__typename id}}"}`
	multiFetchAccountsData = `{"data":{"employees":[{"__typename":"Employee","id":"1"},{"__typename":"Employee","id":"2"}],"topEmployee":{"__typename":"Employee","id":"3"}}}`

	// products merged request (MultiFetch on): the two entity fetches (products
	// for the employees list, notes for topEmployee) merged into ONE aliased
	// _entities request guarded by per-alias @include variables.
	multiFetchMergedBody = `{"query":"query($representations_f1: [_Any!]!, $includeF1: Boolean!, $representations_f2: [_Any!]!, $includeF2: Boolean!){f1: _entities(representations: $representations_f1)@include(if: $includeF1) {... on Employee {__typename products {upc}}} f2: _entities(representations: $representations_f2)@include(if: $includeF2) {... on Employee {__typename notes}}}","variables":{"representations_f1":[{"__typename":"Employee","id":"1"},{"__typename":"Employee","id":"2"}],"includeF1":true,"representations_f2":[{"__typename":"Employee","id":"3"}],"includeF2":true}}`
	multiFetchMergedData = `{"data":{"f1":[{"__typename":"Employee","products":[{"upc":"p1"},{"upc":"p2"}]},{"__typename":"Employee","products":[{"upc":"p3"}]}],"f2":[{"__typename":"Employee","notes":"note-3"}]}}`

	// client-visible response, identical whether MultiFetch is on or off.
	multiFetchClientResponse = `{"data":{"employees":[{"products":[{"upc":"p1"},{"upc":"p2"}]},{"products":[{"upc":"p3"}]}],"topEmployee":{"notes":"note-3"}}}`

	// products unmerged requests (MultiFetch off): two separate _entities requests.
	multiFetchUnmergedProductsBody = `{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Employee {__typename products {upc}}}}","variables":{"representations":[{"__typename":"Employee","id":"1"},{"__typename":"Employee","id":"2"}]}}`
	multiFetchUnmergedNotesBody    = `{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Employee {__typename notes}}}","variables":{"representations":[{"__typename":"Employee","id":"3"}]}}`
)

func TestExecutionEngine_MultiFetch(t *testing.T) {
	accounts := map[string]sendResponse{
		multiFetchAccountsBody: {statusCode: 200, body: multiFetchAccountsData},
	}

	t.Run("merges same-subgraph entity fetches into one request", func(t *testing.T) {
		rec := &multiFetchRecorder{}
		products := map[string]sendResponse{
			multiFetchMergedBody: {statusCode: 200, body: multiFetchMergedData},
		}
		ds := multiFetchDataSources(t, rec, accounts, products)

		resp, err := runMultiFetchQuery(t, ds, true)
		require.NoError(t, err)

		// Exactly ONE request reached the products subgraph: the merge happened.
		requests := rec.requests()
		require.Len(t, requests, 1)
		assert.Equal(t, multiFetchMergedBody, requests[0])
		assert.Equal(t, multiFetchClientResponse, resp)
	})

	t.Run("subgraph errors under one alias only", func(t *testing.T) {
		rec := &multiFetchRecorder{}
		// f1 (employees->products) fails; f2 (topEmployee->notes) succeeds.
		products := map[string]sendResponse{
			multiFetchMergedBody: {
				statusCode: 200,
				body:       `{"data":{"f1":[{"__typename":"Employee","products":null},{"__typename":"Employee","products":null}],"f2":[{"__typename":"Employee","notes":"note-3"}]},"errors":[{"message":"products subgraph unavailable","path":["f1"]}]}`,
			},
		}
		ds := multiFetchDataSources(t, rec, accounts, products)

		resp, err := runMultiFetchQuery(t, ds, true)
		require.NoError(t, err)

		require.Len(t, rec.requests(), 1)
		// The f1 error attributes to that entry's response path ("employees"); the
		// other entry's data (topEmployee.notes) is intact.
		assert.Equal(t, `{"errors":[{"message":"Failed to fetch from Subgraph 'products' at Path 'employees'."}],"data":{"employees":[{"products":null},{"products":null}],"topEmployee":{"notes":"note-3"}}}`, resp)
	})

	t.Run("entry with no live representations is switched off with includeF2 false", func(t *testing.T) {
		rec := &multiFetchRecorder{}
		accountsNull := map[string]sendResponse{
			multiFetchAccountsBody: {
				statusCode: 200,
				body:       `{"data":{"employees":[{"__typename":"Employee","id":"1"},{"__typename":"Employee","id":"2"}],"topEmployee":null}}`,
			},
		}
		// f2 has no live representation (topEmployee was null): empty array + includeF2:false.
		mergedBodyNoF2 := `{"query":"query($representations_f1: [_Any!]!, $includeF1: Boolean!, $representations_f2: [_Any!]!, $includeF2: Boolean!){f1: _entities(representations: $representations_f1)@include(if: $includeF1) {... on Employee {__typename products {upc}}} f2: _entities(representations: $representations_f2)@include(if: $includeF2) {... on Employee {__typename notes}}}","variables":{"representations_f1":[{"__typename":"Employee","id":"1"},{"__typename":"Employee","id":"2"}],"includeF1":true,"representations_f2":[],"includeF2":false}}`
		products := map[string]sendResponse{
			mergedBodyNoF2: {
				statusCode: 200,
				body:       `{"data":{"f1":[{"__typename":"Employee","products":[{"upc":"p1"},{"upc":"p2"}]},{"__typename":"Employee","products":[{"upc":"p3"}]}]}}`,
			},
		}
		ds := multiFetchDataSources(t, rec, accountsNull, products)

		resp, err := runMultiFetchQuery(t, ds, true)
		require.NoError(t, err)

		requests := rec.requests()
		require.Len(t, requests, 1)
		assert.Equal(t, mergedBodyNoF2, requests[0])
		assert.Equal(t, `{"data":{"employees":[{"products":[{"upc":"p1"},{"upc":"p2"}]},{"products":[{"upc":"p3"}]}],"topEmployee":null}}`, resp)
	})

	t.Run("disabled produces two requests with a byte-identical client response", func(t *testing.T) {
		rec := &multiFetchRecorder{}
		products := map[string]sendResponse{
			multiFetchUnmergedProductsBody: {
				statusCode: 200,
				body:       `{"data":{"_entities":[{"__typename":"Employee","products":[{"upc":"p1"},{"upc":"p2"}]},{"__typename":"Employee","products":[{"upc":"p3"}]}]}}`,
			},
			multiFetchUnmergedNotesBody: {
				statusCode: 200,
				body:       `{"data":{"_entities":[{"__typename":"Employee","notes":"note-3"}]}}`,
			},
		}
		ds := multiFetchDataSources(t, rec, accounts, products)

		resp, err := runMultiFetchQuery(t, ds, false)
		require.NoError(t, err)

		// Two separate requests reach products (no merge). Their order is
		// nondeterministic (same parallel wave), so compare as an unordered set.
		assert.ElementsMatch(t, []string{multiFetchUnmergedProductsBody, multiFetchUnmergedNotesBody}, rec.requests())
		// The client response is byte-identical to the merged (MultiFetch-on) run.
		assert.Equal(t, multiFetchClientResponse, resp)
	})
}
