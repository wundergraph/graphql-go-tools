package engine

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// This reproduces The Guild's federation gateway audit suite "child-type-mismatch".
//
// Two subgraphs:
//   a: type User { id: ID @shareable }   Query.users: [User!]!
//   b: union Account = User | Admin
//      type User @key(fields:"id") { id: ID! name: String similarAccounts: [Account!]! }
//      type Admin { id: ID name: String @shareable similarAccounts: [Account!]! }
//      Query.accounts: [Account!]!
//
// User.id is nullable (ID) in subgraph a and non-null (ID!) in subgraph b. The union
// Account is traversed up to three levels deep through similarAccounts.
//
// Audit data: one user u1 (name "u1-name"); accounts is [User u1, Admin a1]; similarAccounts
// always returns that same [User u1, Admin a1] list.

const childTypeMismatchSchema = `
type Query {
	users: [User!]!
	accounts: [Account!]!
}

union Account = User | Admin

type User {
	id: ID
	name: String
	similarAccounts: [Account!]!
}

type Admin {
	id: ID
	name: String
	similarAccounts: [Account!]!
}
`

const childTypeMismatchSubgraphASDL = `
type User @shareable {
	id: ID
}

type Query {
	users: [User!]!
}
`

const childTypeMismatchSubgraphBSDL = `
union Account = User | Admin

type User @key(fields: "id") {
	id: ID!
	name: String
	similarAccounts: [Account!]!
}

type Admin {
	id: ID
	name: String @shareable
	similarAccounts: [Account!]!
}

type Query {
	accounts: [Account!]!
}
`

type childTypeMismatchUpstreamBody struct {
	Query     string `json:"query"`
	Variables struct {
		Representations []struct {
			Typename string `json:"__typename"`
			ID       string `json:"id"`
		} `json:"representations"`
	} `json:"variables"`
}

// childTypeMismatchRecord is one union member in the audit data set.
type childTypeMismatchRecord struct {
	typename string
	id       string
	name     string
}

// childTypeMismatchAccountList mirrors the audit's resolvers: accounts (and every level of
// similarAccounts) resolve to the same [User u1, Admin a1] list.
func childTypeMismatchAccountList() []childTypeMismatchRecord {
	return []childTypeMismatchRecord{
		{typename: "User", id: "u1", name: "u1-name"},
		{typename: "Admin", id: "a1", name: "a1-name"},
	}
}

func childTypeMismatchFind(typename, id string) childTypeMismatchRecord {
	for _, rec := range childTypeMismatchAccountList() {
		if rec.typename == typename && rec.id == id {
			return rec
		}
	}
	return childTypeMismatchRecord{typename: typename, id: id, name: id + "-name"}
}

// childTypeMismatchResolveObject is a minimal, alias-aware GraphQL executor over the audit data.
// It deliberately keys the response by each field's *alias* (not its name), so the planner's
// per-member disambiguation aliases (e.g. __internal_merge_Admin_id) are exercised end to end: if the
// planner failed to alias, or the resolver failed to read the alias back, the JSON would not match.
func childTypeMismatchResolveObject(doc *ast.Document, selectionSetRef int, rec childTypeMismatchRecord) map[string]any {
	obj := map[string]any{}
	for _, selectionRef := range doc.SelectionSets[selectionSetRef].SelectionRefs {
		selection := doc.Selections[selectionRef]
		switch selection.Kind {
		case ast.SelectionKindField:
			fieldRef := selection.Ref
			responseKey := doc.FieldAliasOrNameString(fieldRef)
			switch doc.FieldNameString(fieldRef) {
			case "__typename":
				obj[responseKey] = rec.typename
			case "id":
				obj[responseKey] = rec.id
			case "name":
				obj[responseKey] = rec.name
			case "similarAccounts":
				if nestedSet, ok := doc.FieldSelectionSet(fieldRef); ok {
					obj[responseKey] = childTypeMismatchResolveList(doc, nestedSet)
				}
			}
		case ast.SelectionKindInlineFragment:
			typeCondition := doc.InlineFragmentTypeConditionNameString(selection.Ref)
			if typeCondition != "" && typeCondition != rec.typename {
				continue
			}
			if nestedSet, ok := doc.InlineFragmentSelectionSet(selection.Ref); ok {
				maps.Copy(obj, childTypeMismatchResolveObject(doc, nestedSet, rec))
			}
		}
	}
	return obj
}

func childTypeMismatchResolveList(doc *ast.Document, selectionSetRef int) []any {
	records := childTypeMismatchAccountList()
	out := make([]any, 0, len(records))
	for _, rec := range records {
		out = append(out, childTypeMismatchResolveObject(doc, selectionSetRef, rec))
	}
	return out
}

// childTypeMismatchHandler is a faithful subgraph: it parses and validates each incoming operation
// and resolves only the root fields allowed for that subgraph. Parsing real GraphQL means a query
// that this engine wrongly leaves un-aliased (which a real subgraph would reject) cannot silently
// pass here either.
func childTypeMismatchHandler(t *testing.T, allowedRoots map[string]bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body childTypeMismatchUpstreamBody
		require.NoError(t, json.Unmarshal(raw, &body))

		doc, report := astparser.ParseGraphqlDocumentString(body.Query)
		require.Falsef(t, report.HasErrors(), "subgraph received an unparseable query %q: %s", body.Query, report.Error())

		data := map[string]any{}
		for _, rootNode := range doc.RootNodes {
			if rootNode.Kind != ast.NodeKindOperationDefinition {
				continue
			}
			operation := doc.OperationDefinitions[rootNode.Ref]
			if !operation.HasSelections {
				continue
			}
			for _, selectionRef := range doc.SelectionSets[operation.SelectionSet].SelectionRefs {
				selection := doc.Selections[selectionRef]
				if selection.Kind != ast.SelectionKindField {
					continue
				}
				fieldRef := selection.Ref
				fieldName := doc.FieldNameString(fieldRef)
				require.Truef(t, allowedRoots[fieldName], "subgraph received unexpected root field %q", fieldName)

				responseKey := doc.FieldAliasOrNameString(fieldRef)
				selectionSet, _ := doc.FieldSelectionSet(fieldRef)
				switch fieldName {
				case "users":
					data[responseKey] = []any{
						childTypeMismatchResolveObject(&doc, selectionSet, childTypeMismatchRecord{typename: "User", id: "u1", name: "u1-name"}),
					}
				case "accounts":
					data[responseKey] = childTypeMismatchResolveList(&doc, selectionSet)
				case "_entities":
					entities := make([]any, 0, len(body.Variables.Representations))
					for _, rep := range body.Variables.Representations {
						entities = append(entities, childTypeMismatchResolveObject(&doc, selectionSet, childTypeMismatchFind(rep.Typename, rep.ID)))
					}
					data[responseKey] = entities
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		out, err := json.Marshal(map[string]any{"data": data})
		require.NoError(t, err)
		_, _ = w.Write(out)
	}
}

func newChildTypeMismatchEngine(t *testing.T, ctx context.Context, aURL, bURL string) (*ExecutionEngine, *graphql.Schema) {
	t.Helper()

	subscriptionClient := graphql_datasource.NewGraphQLSubscriptionClient(ctx,
		graphql_datasource.WithUpgradeClient(httpclient.DefaultNetHttpClient),
		graphql_datasource.WithStreamingClient(httpclient.DefaultNetHttpClient),
	)
	factory, err := graphql_datasource.NewFactory(ctx, httpclient.DefaultNetHttpClient, subscriptionClient)
	require.NoError(t, err)

	// subgraph a
	aSchemaConfig, err := graphql_datasource.NewSchemaConfiguration(
		childTypeMismatchSubgraphASDL,
		&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: childTypeMismatchSubgraphASDL},
	)
	require.NoError(t, err)
	aConfig, err := graphql_datasource.NewConfiguration(graphql_datasource.ConfigurationInput{
		Fetch:               &graphql_datasource.FetchConfiguration{URL: aURL, Method: http.MethodPost},
		SchemaConfiguration: aSchemaConfig,
	})
	require.NoError(t, err)
	aDataSource, err := plan.NewDataSourceConfiguration[graphql_datasource.Configuration](
		"subgraph-a",
		factory,
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"users"}},
				{TypeName: "User", FieldNames: []string{"id"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "User", SelectionSet: "id"},
				},
			},
		},
		aConfig,
	)
	require.NoError(t, err)

	// subgraph b
	bSchemaConfig, err := graphql_datasource.NewSchemaConfiguration(
		childTypeMismatchSubgraphBSDL,
		&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: childTypeMismatchSubgraphBSDL},
	)
	require.NoError(t, err)
	bConfig, err := graphql_datasource.NewConfiguration(graphql_datasource.ConfigurationInput{
		Fetch:               &graphql_datasource.FetchConfiguration{URL: bURL, Method: http.MethodPost},
		SchemaConfiguration: bSchemaConfig,
	})
	require.NoError(t, err)
	bDataSource, err := plan.NewDataSourceConfiguration[graphql_datasource.Configuration](
		"subgraph-b",
		factory,
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"accounts"}},
				{TypeName: "User", FieldNames: []string{"id", "name", "similarAccounts"}},
			},
			ChildNodes: []plan.TypeField{
				{TypeName: "Admin", FieldNames: []string{"id", "name", "similarAccounts"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "User", SelectionSet: "id"},
				},
			},
		},
		bConfig,
	)
	require.NoError(t, err)

	schema, err := graphql.NewSchemaFromString(childTypeMismatchSchema)
	require.NoError(t, err)

	engineConfig := NewConfiguration(schema)
	engineConfig.AddDataSource(aDataSource)
	engineConfig.AddDataSource(bDataSource)

	engine, err := NewExecutionEngine(ctx, abstractlogger.Noop{}, engineConfig, resolve.ResolverOptions{
		MaxConcurrency: 1024,
	})
	require.NoError(t, err)

	return engine, schema
}

func runChildTypeMismatch(t *testing.T, engine *ExecutionEngine, schema *graphql.Schema, operationName, query string) string {
	t.Helper()
	req := &graphql.Request{OperationName: operationName, Query: query}
	validationResult, err := req.ValidateForSchema(schema)
	require.NoError(t, err)
	require.True(t, validationResult.Valid, "operation invalid: %+v", validationResult.Errors)

	writer := graphql.NewEngineResultWriter()
	require.NoError(t, engine.Execute(t.Context(), req, &writer))
	return writer.String()
}

func TestChildTypeMismatchAudit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	subgraphA := httptest.NewServer(childTypeMismatchHandler(t, map[string]bool{"users": true}))
	t.Cleanup(subgraphA.Close)
	subgraphB := httptest.NewServer(childTypeMismatchHandler(t, map[string]bool{"accounts": true, "_entities": true}))
	t.Cleanup(subgraphB.Close)

	engine, schema := newChildTypeMismatchEngine(t, ctx, subgraphA.URL, subgraphB.URL)

	// The four operations below are The Guild's audit suite verbatim. The first three select `id`
	// on both union members and therefore hit the nullability conflict (these are the 3 Cosmo
	// failures); the last selects only `name` (String in both) and was never affected.

	t.Run("flat", func(t *testing.T) {
		got := runChildTypeMismatch(t, engine, schema, "", `
			{
				users { id name }
				accounts {
					... on User { id name }
					... on Admin { id name }
				}
			}
		`)
		assert.Equal(t, `{"data":{"users":[{"id":"u1","name":"u1-name"}],"accounts":[{"id":"u1","name":"u1-name"},{"id":"a1","name":"a1-name"}]}}`, got)
	})

	t.Run("nested one level", func(t *testing.T) {
		got := runChildTypeMismatch(t, engine, schema, "NestedInternalAlias", `
			query NestedInternalAlias {
				users { id name }
				accounts {
					... on User { id name similarAccounts { ... on User { id name } ... on Admin { id name } } }
					... on Admin { id name similarAccounts { ... on User { id name } ... on Admin { id name } } }
				}
			}
		`)
		assert.Equal(t, `{"data":{"users":[{"id":"u1","name":"u1-name"}],"accounts":[{"id":"u1","name":"u1-name","similarAccounts":[{"id":"u1","name":"u1-name"},{"id":"a1","name":"a1-name"}]},{"id":"a1","name":"a1-name","similarAccounts":[{"id":"u1","name":"u1-name"},{"id":"a1","name":"a1-name"}]}]}}`, got)
	})

	t.Run("deeply nested", func(t *testing.T) {
		got := runChildTypeMismatch(t, engine, schema, "DeeplyNestedInternalAlias", `
			query DeeplyNestedInternalAlias {
				accounts {
					... on User { id name similarAccounts { ... on User { id name similarAccounts { ... on User { id name } ... on Admin { id name } } } ... on Admin { id name similarAccounts { ... on User { id name } ... on Admin { id name } } } } }
					... on Admin { id name similarAccounts { ... on User { id name similarAccounts { ... on User { id name } ... on Admin { id name } } } ... on Admin { id name similarAccounts { ... on User { id name } ... on Admin { id name } } } } }
				}
			}
		`)
		assert.Equal(t, `{"data":{"accounts":[{"id":"u1","name":"u1-name","similarAccounts":[{"id":"u1","name":"u1-name","similarAccounts":[{"id":"u1","name":"u1-name"},{"id":"a1","name":"a1-name"}]},{"id":"a1","name":"a1-name","similarAccounts":[{"id":"u1","name":"u1-name"},{"id":"a1","name":"a1-name"}]}]},{"id":"a1","name":"a1-name","similarAccounts":[{"id":"u1","name":"u1-name","similarAccounts":[{"id":"u1","name":"u1-name"},{"id":"a1","name":"a1-name"}]},{"id":"a1","name":"a1-name","similarAccounts":[{"id":"u1","name":"u1-name"},{"id":"a1","name":"a1-name"}]}]}]}}`, got)
	})

	t.Run("deeply nested name only", func(t *testing.T) {
		got := runChildTypeMismatch(t, engine, schema, "DeeplyNested", `
			query DeeplyNested {
				accounts {
					... on User { name similarAccounts { ... on User { name similarAccounts { ... on User { name } ... on Admin { name } } } ... on Admin { name similarAccounts { ... on User { name } ... on Admin { name } } } } }
					... on Admin { name similarAccounts { ... on User { name similarAccounts { ... on User { name } ... on Admin { name } } } ... on Admin { name similarAccounts { ... on User { name } ... on Admin { name } } } } }
				}
			}
		`)
		assert.Equal(t, `{"data":{"accounts":[{"name":"u1-name","similarAccounts":[{"name":"u1-name","similarAccounts":[{"name":"u1-name"},{"name":"a1-name"}]},{"name":"a1-name","similarAccounts":[{"name":"u1-name"},{"name":"a1-name"}]}]},{"name":"a1-name","similarAccounts":[{"name":"u1-name","similarAccounts":[{"name":"u1-name"},{"name":"a1-name"}]},{"name":"a1-name","similarAccounts":[{"name":"u1-name"},{"name":"a1-name"}]}]}]}}`, got)
	})
}
