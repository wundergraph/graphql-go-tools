package engine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

type requestScopedE2EServer struct {
	server *httptest.Server

	mu                 sync.Mutex
	requests           []requestScopedE2ERequest
	unexpectedRequests []requestScopedE2ERequest
}

type requestScopedE2ERequest struct {
	Query     string
	Variables string
}

func newRequestScopedE2EServer(t *testing.T, responder func(request requestScopedE2ERequest) (response string, ok bool)) *requestScopedE2EServer {
	t.Helper()

	s := &requestScopedE2EServer{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Helper()

		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			http.Error(w, `{"errors":[{"message":"invalid request body"}]}`, http.StatusBadRequest)
			return
		}

		var payload struct {
			Query     string          `json:"query"`
			Variables json.RawMessage `json:"variables"`
		}
		if !assert.NoError(t, json.Unmarshal(body, &payload)) {
			http.Error(w, `{"errors":[{"message":"invalid graphql payload"}]}`, http.StatusBadRequest)
			return
		}

		request := requestScopedE2ERequest{
			Query:     payload.Query,
			Variables: normalizeRequestScopedVariables(t, payload.Variables),
		}

		s.mu.Lock()
		s.requests = append(s.requests, request)
		s.mu.Unlock()

		response, ok := responder(request)
		if !ok {
			s.mu.Lock()
			s.unexpectedRequests = append(s.unexpectedRequests, request)
			s.mu.Unlock()
			response = `{"errors":[{"message":"unexpected upstream query"}]}`
		}

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write([]byte(response))
		assert.NoError(t, err)
	}))

	t.Cleanup(s.server.Close)
	return s
}

// normalizeRequestScopedVariables runs on the httptest handler goroutine, so it
// must not use require/FailNow-family assertions. It inlines the compact-JSON
// logic with non-fatal assert.NoError; on marshal failure it falls through with
// the raw bytes so any test assertion can still diff against a recognizable
// value.
func normalizeRequestScopedVariables(t *testing.T, raw json.RawMessage) string {
	t.Helper()

	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	var value any
	if !assert.NoError(t, json.Unmarshal(raw, &value)) {
		return string(raw)
	}
	normalized, err := json.Marshal(value)
	if !assert.NoError(t, err) {
		return string(raw)
	}
	return string(normalized)
}

func (s *requestScopedE2EServer) URL() string {
	return s.server.URL
}

func (s *requestScopedE2EServer) Requests() []requestScopedE2ERequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]requestScopedE2ERequest, len(s.requests))
	copy(out, s.requests)
	return out
}

func (s *requestScopedE2EServer) AssertExactRequests(t *testing.T, expected ...requestScopedE2ERequest) {
	t.Helper()

	s.mu.Lock()
	defer s.mu.Unlock()

	assert.Equal(t, expected, s.requests)
	assert.Equal(t, []requestScopedE2ERequest(nil), s.unexpectedRequests)
}

type requestScopedE2EDataSourceSpec struct {
	name string
	url  string
	sdl  string

	rootNodes          []plan.TypeField
	childNodes         []plan.TypeField
	federationMetaData plan.FederationMetaData
}

func newRequestScopedExecutionEngine(
	t *testing.T,
	specs ...requestScopedE2EDataSourceSpec,
) *ExecutionEngine {
	t.Helper()

	ctx := context.Background()

	subgraphs := make([]SubgraphConfiguration, 0, len(specs))
	for _, spec := range specs {
		subgraphs = append(subgraphs, SubgraphConfiguration{
			Name: spec.name,
			URL:  spec.url,
			SDL:  spec.sdl,
		})
	}

	factory := NewFederationEngineConfigFactory(ctx, subgraphs)
	engineConfig, err := factory.BuildEngineConfiguration()
	require.NoError(t, err)

	httpClient := http.DefaultClient
	subscriptionClient := graphql_datasource.NewGraphQLSubscriptionClient(ctx, graphql_datasource.WithUpgradeClient(httpClient), graphql_datasource.WithStreamingClient(httpClient))
	graphQLFactory, err := graphql_datasource.NewFactory(ctx, httpClient, subscriptionClient)
	require.NoError(t, err)

	dataSources := make([]plan.DataSource, 0, len(specs))
	for _, spec := range specs {
		schemaConfig, err := graphql_datasource.NewSchemaConfiguration(spec.sdl, &graphql_datasource.FederationConfiguration{
			Enabled:    true,
			ServiceSDL: spec.sdl,
		})
		require.NoError(t, err)

		customConfig, err := graphql_datasource.NewConfiguration(graphql_datasource.ConfigurationInput{
			Fetch: &graphql_datasource.FetchConfiguration{
				URL:    spec.url,
				Method: http.MethodPost,
			},
			SchemaConfiguration: schemaConfig,
		})
		require.NoError(t, err)

		dataSource, err := plan.NewDataSourceConfiguration[graphql_datasource.Configuration](
			spec.name,
			graphQLFactory,
			&plan.DataSourceMetadata{
				RootNodes:          spec.rootNodes,
				ChildNodes:         spec.childNodes,
				FederationMetaData: spec.federationMetaData,
			},
			customConfig,
		)
		require.NoError(t, err)

		dataSources = append(dataSources, dataSource)
	}

	engineConfig.SetDataSources(dataSources)

	executionEngine, err := NewExecutionEngine(ctx, abstractlogger.NoopLogger, engineConfig, resolve.ResolverOptions{
		MaxConcurrency: 1024,
	})
	require.NoError(t, err)

	return executionEngine
}

func executeRequestScopedQuery(t *testing.T, executionEngine *ExecutionEngine, query string) string {
	t.Helper()

	request := &graphql.Request{Query: query}
	writer := graphql.NewEngineResultWriter()

	err := executionEngine.Execute(
		context.Background(),
		request,
		&writer,
		WithCachingOptions(resolve.CachingOptions{EnableL1Cache: true}),
	)
	require.NoError(t, err)

	return writer.String()
}

func viewerRequestScopedSpec(
	viewerURL, viewerSDL string,
	viewerFields []string,
	childNodes []plan.TypeField,
	requires plan.FederationFieldConfigurations,
) requestScopedE2EDataSourceSpec {
	return requestScopedE2EDataSourceSpec{
		name: "viewer",
		url:  viewerURL,
		sdl:  viewerSDL,
		rootNodes: []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"currentViewer"}},
			{TypeName: "Article", FieldNames: []string{"id", "currentViewer"}},
			{TypeName: "Viewer", FieldNames: viewerFields},
		},
		childNodes: childNodes,
		federationMetaData: plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{TypeName: "Article", SelectionSet: "id"},
			},
			Requires: requires,
			RequestScopedFields: []plan.RequestScopedField{
				{TypeName: "Query", FieldName: "currentViewer", L1Key: "viewer.currentViewer"},
				{TypeName: "Article", FieldName: "currentViewer", L1Key: "viewer.currentViewer"},
			},
		},
	}
}

func viewerRequestScopedRequiresBaseSpec(viewerURL string) requestScopedE2EDataSourceSpec {
	return requestScopedE2EDataSourceSpec{
		name: "viewer",
		url:  viewerURL,
		sdl: `directive @requestScoped(key: String!) on FIELD_DEFINITION
type Query { currentViewer: Viewer @requestScoped(key: "viewer") }
type Viewer @key(fields: "id") { id: ID! name: String! }
type Article @key(fields: "id") { id: ID! currentViewer: Viewer @requestScoped(key: "viewer") }`,
		rootNodes: []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"currentViewer"}},
			{TypeName: "Article", FieldNames: []string{"id", "currentViewer"}},
			{TypeName: "Viewer", FieldNames: []string{"id", "name"}},
		},
		childNodes: []plan.TypeField{
			{TypeName: "Viewer", FieldNames: []string{"id", "name"}},
		},
		federationMetaData: plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{TypeName: "Viewer", SelectionSet: "id"},
				{TypeName: "Article", SelectionSet: "id"},
			},
			RequestScopedFields: []plan.RequestScopedField{
				{TypeName: "Query", FieldName: "currentViewer", L1Key: "viewer.currentViewer"},
				{TypeName: "Article", FieldName: "currentViewer", L1Key: "viewer.currentViewer"},
			},
		},
	}
}

func handlesRequestScopedSpec(handlesURL string) requestScopedE2EDataSourceSpec {
	return requestScopedE2EDataSourceSpec{
		name: "handles",
		url:  handlesURL,
		sdl: `directive @external on FIELD_DEFINITION
directive @requires(fields: String!) on FIELD_DEFINITION
type Viewer @key(fields: "id") { id: ID! @external name: String! @external handle: String! @requires(fields: "name") }`,
		rootNodes: []plan.TypeField{
			{TypeName: "Viewer", FieldNames: []string{"id", "handle"}, ExternalFieldNames: []string{"name"}},
		},
		childNodes: []plan.TypeField{
			{TypeName: "Viewer", FieldNames: []string{"id", "handle"}, ExternalFieldNames: []string{"name"}},
		},
		federationMetaData: plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{TypeName: "Viewer", SelectionSet: "id"},
			},
			Requires: plan.FederationFieldConfigurations{
				{TypeName: "Viewer", FieldName: "handle", SelectionSet: "name"},
			},
		},
	}
}

func articlesRequestScopedSpec(articlesURL, articlesSDL string, queryFields []string) requestScopedE2EDataSourceSpec {
	return requestScopedE2EDataSourceSpec{
		name: "articles",
		url:  articlesURL,
		sdl:  articlesSDL,
		rootNodes: []plan.TypeField{
			{TypeName: "Query", FieldNames: queryFields},
			{TypeName: "Article", FieldNames: []string{"id", "title"}},
		},
		federationMetaData: plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{TypeName: "Article", SelectionSet: "id"},
			},
		},
	}
}

// TestRequestScopedWideningExecution verifies the end-to-end fetch behavior for
// requestScoped widening.
//
// Each subtest asserts two things:
//  1. The client-visible response still matches the original query shape.
//  2. The upstream traffic shows only the widened fetches we expect.
//
// The request recorder is intentionally strict: if the planner or resolver
// regresses and sends an extra entity hop, the test records it as an unexpected
// request and fails.
func TestRequestScopedWideningExecution(t *testing.T) {
	t.Parallel()

	t.Run("root fetch widens and skips the entity fetch", func(t *testing.T) {
		t.Parallel()

		// Scenario:
		//   - The root currentViewer selection is narrower than the article.currentViewer selection.
		//   - requestScoped widening should widen the root fetch to the wider shape.
		//
		// Expected flow:
		//  1. Root fetch to viewer requests {id name email}.
		//  2. Root fetch to articles requests the article shell.
		//  3. No viewer entity fetch happens for article.currentViewer because the widened
		//     root value is injected from requestScoped L1.
		viewer := newRequestScopedE2EServer(t, func(request requestScopedE2ERequest) (string, bool) {
			if request == (requestScopedE2ERequest{Query: `{currentViewer {id name email}}`}) {
				return `{"data":{"currentViewer":{"id":"v1","name":"Alice","email":"alice@example.com"}}}`, true
			}
			return "", false
		})

		articles := newRequestScopedE2EServer(t, func(request requestScopedE2ERequest) (string, bool) {
			if request == (requestScopedE2ERequest{Query: `{article {id title __typename}}`}) {
				return `{"data":{"article":{"id":"a1","title":"T1","__typename":"Article"}}}`, true
			}
			return "", false
		})

		executionEngine := newRequestScopedExecutionEngine(
			t,
			viewerRequestScopedSpec(
				viewer.URL(),
				`directive @requestScoped(key: String!) on FIELD_DEFINITION
type Query { currentViewer: Viewer @requestScoped(key: "viewer") }
type Viewer { id: ID! name: String! email: String! }
type Article @key(fields: "id") { id: ID! currentViewer: Viewer @requestScoped(key: "viewer") }`,
				[]string{"id", "name", "email"},
				nil,
				nil,
			),
			articlesRequestScopedSpec(
				articles.URL(),
				`type Query { article: Article }
type Article @key(fields: "id") { id: ID! title: String! }`,
				[]string{"article"},
			),
		)

		response := executeRequestScopedQuery(t, executionEngine, `query {
			currentViewer {
				id
				name
			}
			article {
				id
				title
				currentViewer {
					id
					name
					email
				}
			}
		}`)

		// The client response must keep the original narrow root shape and the wider
		// article.currentViewer shape even though the upstream root fetch was widened.
		assert.Equal(t,
			compactJSONForAssert(t, `{"data":{"currentViewer":{"id":"v1","name":"Alice"},"article":{"id":"a1","title":"T1","currentViewer":{"id":"v1","name":"Alice","email":"alice@example.com"}}}}`),
			compactJSONForAssert(t, response),
		)

		// Only the widened root fetch and the article shell fetch are allowed.
		viewer.AssertExactRequests(t, requestScopedE2ERequest{Query: `{currentViewer {id name email}}`})
		articles.AssertExactRequests(t, requestScopedE2ERequest{Query: `{article {id title __typename}}`})
	})

	t.Run("requires chain widens the base viewer fetch, skips the requestScoped entity hop, and still feeds the handle subgraph", func(t *testing.T) {
		t.Parallel()

		// Scenario:
		//   - The base viewer subgraph exposes name through currentViewer.
		//   - The article-side currentViewer participant is requestScoped with the same key.
		//   - A third handles subgraph owns handle and declares @requires(fields: "name").
		//
		// Expected flow:
		//  1. The root viewer fetch is widened to include the hidden dependency fields
		//     needed later: aliased name, __typename, and id.
		//  2. The requestScoped entity hop back into the viewer subgraph is skipped.
		//  3. The handles entity fetch still runs, receiving representations that include
		//     the hidden name dependency from the widened root fetch.
		viewer := newRequestScopedE2EServer(t, func(request requestScopedE2ERequest) (string, bool) {
			if request == (requestScopedE2ERequest{Query: `{currentViewer {viewerName: name __typename id}}`}) {
				return `{"data":{"currentViewer":{"viewerName":"Alice","__typename":"Viewer","id":"v1"}}}`, true
			}
			return "", false
		})

		articles := newRequestScopedE2EServer(t, func(request requestScopedE2ERequest) (string, bool) {
			if request == (requestScopedE2ERequest{Query: `{article {id title __typename}}`}) {
				return `{"data":{"article":{"id":"a1","title":"T1","__typename":"Article"}}}`, true
			}
			return "", false
		})

		handlesExpectedVariables := compactJSONForAssert(t, `{"representations":[{"__typename":"Viewer","id":"v1","name":"Alice"}]}`)
		handles := newRequestScopedE2EServer(t, func(request requestScopedE2ERequest) (string, bool) {
			if request == (requestScopedE2ERequest{
				Query:     `query($representations: [_Any!]!){_entities(representations: $representations){... on Viewer {__typename handle}}}`,
				Variables: handlesExpectedVariables,
			}) {
				return `{"data":{"_entities":[{"__typename":"Viewer","handle":"alice-handle"}]}}`, true
			}
			return "", false
		})

		executionEngine := newRequestScopedExecutionEngine(
			t,
			viewerRequestScopedRequiresBaseSpec(viewer.URL()),
			articlesRequestScopedSpec(
				articles.URL(),
				`type Query { article: Article }
type Article @key(fields: "id") { id: ID! title: String! }`,
				[]string{"article"},
			),
			handlesRequestScopedSpec(handles.URL()),
		)

		response := executeRequestScopedQuery(t, executionEngine, `query {
			currentViewer {
				viewerName: name
			}
			article {
				id
				title
				currentViewer {
					handle
				}
			}
		}`)

		// The response keeps the user-visible alias at the root and only exposes handle
		// on the nested branch even though name/id/__typename were fetched behind the scenes.
		assert.Equal(t,
			compactJSONForAssert(t, `{"data":{"currentViewer":{"viewerName":"Alice"},"article":{"id":"a1","title":"T1","currentViewer":{"handle":"alice-handle"}}}}`),
			compactJSONForAssert(t, response),
		)

		// The viewer subgraph must only receive the widened root fetch. The skipped
		// requestScoped entity hop would show up here as an unexpected extra request.
		viewer.AssertExactRequests(t, requestScopedE2ERequest{Query: `{currentViewer {viewerName: name __typename id}}`})
		articles.AssertExactRequests(t, requestScopedE2ERequest{Query: `{article {id title __typename}}`})

		// The downstream handles fetch still happens, and its representations must carry
		// the hidden name dependency supplied by the widened root fetch.
		handles.AssertExactRequests(t, requestScopedE2ERequest{
			Query:     `query($representations: [_Any!]!){_entities(representations: $representations){... on Viewer {__typename handle}}}`,
			Variables: compactJSONForAssert(t, `{"representations":[{"__typename":"Viewer","id":"v1","name":"Alice"}]}`),
		})
	})

	t.Run("argument conflicts widen through synthetic aliases and still render user-shaped data", func(t *testing.T) {
		t.Parallel()

		// Scenario:
		//   - Two requestScoped participants select the same field with different arguments.
		//   - The widened upstream fetch must keep both variants distinct with synthetic aliases.
		//
		// Expected flow:
		//  1. Root fetch to viewer requests both posts(first: 1) and posts(first: 2).
		//  2. The synthetic aliases keep the two cache entries separate inside requestScoped L1.
		//  3. The nested article.currentViewer branch is injected from the widened root value.
		viewerExpectedVariables := compactJSONForAssert(t, `{"a":1,"b":2}`)
		viewer := newRequestScopedE2EServer(t, func(request requestScopedE2ERequest) (string, bool) {
			if request == (requestScopedE2ERequest{
				Query:     `query($a: Int!, $b: Int!){currentViewer {id __request_scoped__posts_0: posts(first: $a){id} __request_scoped__posts_1: posts(first: $b){id title}}}`,
				Variables: viewerExpectedVariables,
			}) {
				return `{"data":{"currentViewer":{"id":"v1","__request_scoped__posts_0":[{"id":"p1"}],"__request_scoped__posts_1":[{"id":"p2","title":"Second"}]}}}`, true
			}
			return "", false
		})

		articles := newRequestScopedE2EServer(t, func(request requestScopedE2ERequest) (string, bool) {
			if request == (requestScopedE2ERequest{Query: `{article {id title __typename}}`}) {
				return `{"data":{"article":{"id":"a1","title":"T1","__typename":"Article"}}}`, true
			}
			return "", false
		})

		executionEngine := newRequestScopedExecutionEngine(
			t,
			viewerRequestScopedSpec(
				viewer.URL(),
				`directive @requestScoped(key: String!) on FIELD_DEFINITION
type Query { currentViewer: Viewer @requestScoped(key: "viewer") }
type Viewer { id: ID! posts(first: Int!): [Post!]! }
type Post { id: ID! title: String! }
type Article @key(fields: "id") { id: ID! currentViewer: Viewer @requestScoped(key: "viewer") }`,
				[]string{"id", "posts"},
				[]plan.TypeField{{TypeName: "Post", FieldNames: []string{"id", "title"}}},
				nil,
			),
			articlesRequestScopedSpec(
				articles.URL(),
				`type Query { article: Article }
type Article @key(fields: "id") { id: ID! title: String! }`,
				[]string{"article"},
			),
		)

		response := executeRequestScopedQuery(t, executionEngine, `query {
			currentViewer {
				id
				posts(first: 1) {
					id
				}
			}
			article {
				id
				title
				currentViewer {
					id
					posts(first: 2) {
						id
						title
					}
				}
			}
		}`)

		// The client still sees the original argument-specific branches rather than the
		// synthetic aliases used internally for widening and cache storage.
		assert.Equal(t,
			compactJSONForAssert(t, `{"data":{"currentViewer":{"id":"v1","posts":[{"id":"p1"}]},"article":{"id":"a1","title":"T1","currentViewer":{"id":"v1","posts":[{"id":"p2","title":"Second"}]}}}}`),
			compactJSONForAssert(t, response),
		)

		// The only viewer request allowed is the widened root fetch that carries both
		// argument variants. Any later entity hop would fail the exact request assertion.
		viewer.AssertExactRequests(t, requestScopedE2ERequest{
			Query:     `query($a: Int!, $b: Int!){currentViewer {id __request_scoped__posts_0: posts(first: $a){id} __request_scoped__posts_1: posts(first: $b){id title}}}`,
			Variables: compactJSONForAssert(t, `{"a":1,"b":2}`),
		})
		articles.AssertExactRequests(t, requestScopedE2ERequest{Query: `{article {id title __typename}}`})
	})

	t.Run("three conflicting participants widen to one root fetch while each response branch keeps its own shape", func(t *testing.T) {
		t.Parallel()

		// Scenario:
		//   - Three requestScoped participants all want to bind different schema fields into
		//     the same response position `name`.
		//   - The widened root fetch must carry all three variants without collapsing them.
		//
		// Expected flow:
		//  1. Root fetch to viewer requests name, email, and handle under distinct synthetic aliases.
		//  2. Both article branches fetch only their article shells.
		//  3. The nested currentViewer branches are injected from the common widened root value.
		viewer := newRequestScopedE2EServer(t, func(request requestScopedE2ERequest) (string, bool) {
			if request == (requestScopedE2ERequest{Query: `{currentViewer {id __request_scoped__name_2: name __request_scoped__name_0: email __request_scoped__name_1: handle}}`}) {
				return `{"data":{"currentViewer":{"id":"v1","__request_scoped__name_2":"Alice","__request_scoped__name_0":"alice@example.com","__request_scoped__name_1":"alice-handle"}}}`, true
			}
			return "", false
		})

		articles := newRequestScopedE2EServer(t, func(request requestScopedE2ERequest) (string, bool) {
			if request == (requestScopedE2ERequest{Query: `{article {id title __typename} featuredArticle {id title __typename}}`}) {
				return `{"data":{"article":{"id":"a1","title":"T1","__typename":"Article"},"featuredArticle":{"id":"a2","title":"T2","__typename":"Article"}}}`, true
			}
			return "", false
		})

		executionEngine := newRequestScopedExecutionEngine(
			t,
			viewerRequestScopedSpec(
				viewer.URL(),
				`directive @requestScoped(key: String!) on FIELD_DEFINITION
type Query { currentViewer: Viewer @requestScoped(key: "viewer") }
type Viewer { id: ID! name: String! email: String! handle: String! }
type Article @key(fields: "id") { id: ID! currentViewer: Viewer @requestScoped(key: "viewer") }`,
				[]string{"id", "name", "email", "handle"},
				nil,
				nil,
			),
			articlesRequestScopedSpec(
				articles.URL(),
				`type Query { article: Article featuredArticle: Article }
type Article @key(fields: "id") { id: ID! title: String! }`,
				[]string{"article", "featuredArticle"},
			),
		)

		response := executeRequestScopedQuery(t, executionEngine, `query {
			currentViewer {
				id
				name
			}
			article {
				id
				title
				currentViewer {
					id
					name: email
				}
			}
			featuredArticle {
				id
				title
				currentViewer {
					id
					name: handle
				}
			}
		}`)

		// Even though the upstream fetch uses three distinct aliases, each response branch
		// must still render the exact user-visible shape from the original query.
		assert.Equal(t,
			compactJSONForAssert(t, `{"data":{"currentViewer":{"id":"v1","name":"Alice"},"article":{"id":"a1","title":"T1","currentViewer":{"id":"v1","name":"alice@example.com"}},"featuredArticle":{"id":"a2","title":"T2","currentViewer":{"id":"v1","name":"alice-handle"}}}}`),
			compactJSONForAssert(t, response),
		)

		// The root viewer fetch is the only legal viewer request for this scenario.
		viewer.AssertExactRequests(t, requestScopedE2ERequest{Query: `{currentViewer {id __request_scoped__name_2: name __request_scoped__name_0: email __request_scoped__name_1: handle}}`})
		articles.AssertExactRequests(t, requestScopedE2ERequest{Query: `{article {id title __typename} featuredArticle {id title __typename}}`})
	})
}
