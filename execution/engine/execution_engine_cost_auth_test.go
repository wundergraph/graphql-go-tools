package engine

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// denyCoordinatesAuthorizer is a resolve.BatchAuthorizer that denies the configured
// coordinates and allows everything else.
type denyCoordinatesAuthorizer struct {
	denied map[resolve.GraphCoordinate]string // coordinate -> deny reason
}

func (a *denyCoordinatesAuthorizer) AuthorizeFields(_ *resolve.Context, coordinates []resolve.GraphCoordinate) ([]resolve.AuthorizationDecision, error) {
	decisions := make([]resolve.AuthorizationDecision, len(coordinates))
	for i, coordinate := range coordinates {
		if reason, ok := a.denied[coordinate]; ok {
			decisions[i] = resolve.AuthorizationDecision{Allowed: false, Reason: reason}
		} else {
			decisions[i] = resolve.AuthorizationDecision{Allowed: true}
		}
	}
	return decisions, nil
}

// postFetchDenyAuthorizer is a resolve.Authorizer (legacy post-fetch mode) that denies the
// configured coordinates on AuthorizeObjectField and allows everything else.
type postFetchDenyAuthorizer struct {
	denied map[resolve.GraphCoordinate]string // coordinate -> deny reason
}

func (a *postFetchDenyAuthorizer) AuthorizePreFetch(_ *resolve.Context, _ string, _ json.RawMessage, _ resolve.GraphCoordinate) (*resolve.AuthorizationDeny, error) {
	return nil, nil
}

func (a *postFetchDenyAuthorizer) AuthorizeObjectField(_ *resolve.Context, _ string, _ json.RawMessage, coordinate resolve.GraphCoordinate) (*resolve.AuthorizationDeny, error) {
	if reason, ok := a.denied[coordinate]; ok {
		return &resolve.AuthorizationDeny{Reason: reason}, nil
	}
	return nil, nil
}

func (a *postFetchDenyAuthorizer) HasResponseExtensionData(_ *resolve.Context) bool { return false }

func (a *postFetchDenyAuthorizer) RenderResponseExtension(_ *resolve.Context, _ io.Writer) error {
	return nil
}

// TestExecutionEngine_Cost_DeniedFields verifies that fields denied by (pre-fetch) field
// authorization are not charged in the actual cost: the client never received them, so
// they must not count against the budget.
func TestExecutionEngine_Cost_DeniedFields(t *testing.T) {
	t.Parallel()

	schemaSDL := `
		schema { query: Query }
		type Query {
			user: User
			account: Account!
			users: [User]
		}
		type User {
			id: ID!
			secret: String
			address: Address
			token: String!
			petName: String
		}
		type Account {
			token: String!
			petName: String
		}
		type Address {
			street: String
		}
	`
	schema, err := graphql.NewSchemaFromString(schemaSDL)
	require.NoError(t, err)

	rootNodes := []plan.TypeField{
		{TypeName: "Query", FieldNames: []string{"user", "account", "users"}},
		{TypeName: "User", FieldNames: []string{"id", "secret", "address", "token", "petName"}},
		{TypeName: "Account", FieldNames: []string{"token", "petName"}},
		{TypeName: "Address", FieldNames: []string{"street"}},
	}
	var childNodes []plan.TypeField
	costConfig := &plan.DataSourceCostConfig{
		Weights: map[plan.FieldCoordinate]*plan.FieldCost{
			{TypeName: "Query", FieldName: "user"}:      {HasWeight: true, Weight: 5},
			{TypeName: "Query", FieldName: "account"}:   {HasWeight: true, Weight: 5},
			{TypeName: "User", FieldName: "secret"}:     {HasWeight: true, Weight: 17},
			{TypeName: "User", FieldName: "address"}:    {HasWeight: true, Weight: 7},
			{TypeName: "User", FieldName: "token"}:      {HasWeight: true, Weight: 11},
			{TypeName: "User", FieldName: "petName"}:    {HasWeight: true, Weight: 2},
			{TypeName: "Account", FieldName: "token"}:   {HasWeight: true, Weight: 11},
			{TypeName: "Account", FieldName: "petName"}: {HasWeight: true, Weight: 2},
			{TypeName: "Address", FieldName: "street"}:  {HasWeight: true, Weight: 3},
		},
	}
	customConfig := mustConfiguration(t, graphql_datasource.ConfigurationInput{
		Fetch: &graphql_datasource.FetchConfiguration{
			URL:    "https://example.com/",
			Method: "GET",
		},
		SchemaConfiguration: mustSchemaConfig(t, nil, schemaSDL),
	})
	fields := []plan.FieldConfiguration{
		{TypeName: "Query", FieldName: "user", HasAuthorizationRule: true},
		{TypeName: "User", FieldName: "secret", HasAuthorizationRule: true},
		{TypeName: "User", FieldName: "address", HasAuthorizationRule: true},
		{TypeName: "User", FieldName: "token", HasAuthorizationRule: true},
		{TypeName: "Account", FieldName: "token", HasAuthorizationRule: true},
	}

	dataSourceWithResponse := func(t *testing.T, responseBody string) []plan.DataSource {
		return []plan.DataSource{
			mustGraphqlDataSourceConfiguration(t, "ds-id",
				mustFactory(t,
					testNetHttpClient(t, roundTripperTestCase{
						expectedHost: "example.com", expectedPath: "/", expectedBody: "",
						sendResponseBody: responseBody,
						sendStatusCode:   200,
					}),
				),
				&plan.DataSourceMetadata{
					RootNodes:  rootNodes,
					ChildNodes: childNodes,
					CostConfig: costConfig,
				},
				customConfig,
			),
		}
	}

	preFetchAuth := func(denied map[resolve.GraphCoordinate]string) []ExecutionOptions {
		return []ExecutionOptions{
			WithPreFetchFieldAuthorizer(&denyCoordinatesAuthorizer{denied: denied}),
		}
	}

	t.Run("control: all fields allowed, actual equals estimated", runWithoutError(
		ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ user { id secret address { street } } }`,
				}
			},
			dataSources: dataSourceWithResponse(t,
				`{"data":{"user":{"id":"1","secret":"s3cr3t","address":{"street":"Main"}}}}`),
			fields:           fields,
			engineOptions:    preFetchAuth(nil),
			expectedResponse: `{"data":{"user":{"id":"1","secret":"s3cr3t","address":{"street":"Main"}}}}`,
			// Query.user (5) + User.secret (17) + User.address (7) + Address.street (3)
			expectedEstimatedCost: intPtr(32),
			expectedActualCost:    intPtr(32),
		},
		computeCosts(),
	))

	t.Run("denied nullable leaf field is not charged", runWithoutError(
		ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ user { id secret } }`,
				}
			},
			dataSources: dataSourceWithResponse(t,
				`{"data":{"user":{"id":"1","secret":"s3cr3t"}}}`),
			fields: fields,
			engineOptions: preFetchAuth(map[resolve.GraphCoordinate]string{
				{TypeName: "User", FieldName: "secret"}: "missing scope 'secret:read'",
			}),
			expectedResponse: `{"errors":[{"message":"Unauthorized to load field 'Query.user.secret', Reason: missing scope 'secret:read'.","path":["user","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":{"user":{"id":"1","secret":null}}}`,

			expectedEstimatedCost: intPtr(22), // Query.user (5) + User.secret (17)
			expectedActualCost:    intPtr(5),  // Query.user (5)
		},
		computeCosts(),
	))

	t.Run("denied object field and its subtree are not charged", runWithoutError(
		ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ user { id address { street } } }`,
				}
			},
			dataSources: dataSourceWithResponse(t,
				`{"data":{"user":{"id":"1","address":{"street":"Main"}}}}`),
			fields: fields,
			engineOptions: preFetchAuth(map[resolve.GraphCoordinate]string{
				{TypeName: "User", FieldName: "address"}: "missing scope 'address:read'",
			}),
			expectedResponse: `{"errors":[{"message":"Unauthorized to load field 'Query.user.address', Reason: missing scope 'address:read'.","path":["user","address"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":{"user":{"id":"1","address":null}}}`,

			expectedEstimatedCost: intPtr(15), // Query.user (5) + User.address (7) + Address.street (3)
			expectedActualCost:    intPtr(5),  // Query.user (5)
		},
		computeCosts(),
	))

	t.Run("denied root field skips fetch and nothing under it is charged", runWithoutError(
		ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ user { id secret } }`,
				}
			},
			// The fetch is skipped entirely; if the round tripper is called, the test fails
			// on the unexpected response body assertion below.
			dataSources: dataSourceWithResponse(t,
				`{"data":{"user":{"id":"unexpected-fetch","secret":"unexpected-fetch"}}}`),
			fields: fields,
			engineOptions: preFetchAuth(map[resolve.GraphCoordinate]string{
				{TypeName: "Query", FieldName: "user"}: "missing scope 'user:read'",
			}),
			expectedResponse: `{"errors":[{"message":"Unauthorized to load field 'Query.user', Reason: missing scope 'user:read'.","path":["user"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":{"user":null}}`,

			expectedEstimatedCost: intPtr(22), //  Query.user (5) + User.secret (17)
			expectedActualCost:    intPtr(0),
		},
		computeCosts(),
	))

	t.Run("post-fetch authorizer: denied leaf field is not charged", runWithoutError(
		ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ user { id secret } }`,
				}
			},
			dataSources: dataSourceWithResponse(t,
				`{"data":{"user":{"id":"1","secret":"s3cr3t"}}}`),
			fields: fields,
			engineOptions: []ExecutionOptions{
				WithAuthorizer(&postFetchDenyAuthorizer{denied: map[resolve.GraphCoordinate]string{
					{TypeName: "User", FieldName: "secret"}: "missing scope 'secret:read'",
				}}),
			},
			expectedResponse: `{"errors":[{"message":"Unauthorized to load field 'Query.user.secret', Reason: missing scope 'secret:read'.","path":["user","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":{"user":{"id":"1","secret":null}}}`,

			expectedEstimatedCost: intPtr(22), // Query.user (5) + User.secret (17)
			expectedActualCost:    intPtr(5),  // Query.user (5)
		},
		computeCosts(),
	))

	// Non-nullable child nulls its parent object:

	t.Run("denied non-nullable field nulls the parent: earlier siblings are not charged", runWithoutError(
		ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ user { id petName token } }`,
				}
			},
			dataSources: dataSourceWithResponse(t,
				`{"data":{"user":{"id":"1","petName":"Rex","token":"t0k3n"}}}`),
			fields: fields,
			engineOptions: preFetchAuth(map[resolve.GraphCoordinate]string{
				{TypeName: "User", FieldName: "token"}: "missing scope 'token:read'",
			}),
			expectedResponse: `{"errors":[{"message":"Unauthorized to load field 'Query.user.token', Reason: missing scope 'token:read'.","path":["user","token"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":{"user":null}}`,
			// Query.user (5) + User.token (11) + User.petName (2)
			expectedEstimatedCost: intPtr(18),
			// Query.user (5) only
			expectedActualCost: intPtr(5),
		},
		computeCosts(),
	))

	t.Run("denied non-nullable field nulls the parent: later sibling is not charged", runWithoutError(
		ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ user { id token petName } }`,
				}
			},
			dataSources: dataSourceWithResponse(t,
				`{"data":{"user":{"id":"1","token":"t0k3n","petName":"Rex"}}}`),
			fields: fields,
			engineOptions: preFetchAuth(map[resolve.GraphCoordinate]string{
				{TypeName: "User", FieldName: "token"}: "missing scope 'token:read'",
			}),
			expectedResponse: `{"errors":[{"message":"Unauthorized to load field 'Query.user.token', Reason: missing scope 'token:read'.","path":["user","token"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":{"user":null}}`,
			// Same selection as above, only the field order differs: estimation is unaffected.
			expectedEstimatedCost: intPtr(18),
			// Query.user (5) only.
			expectedActualCost: intPtr(5),
		},
		computeCosts(),
	))

	t.Run("subgraph null for non-nullable field nulls the parent: earlier siblings are not charged", runWithoutError(
		ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ user { id petName token } }`,
				}
			},
			// No authorizer at all: plain null propagation, the subgraph violates token's
			// non-nullability.
			dataSources: dataSourceWithResponse(t,
				`{"data":{"user":{"id":"1","petName":"Rex","token":null}}}`),
			fields:                fields,
			expectedResponse:      `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.user.token'.","path":["user","token"]}],"data":{"user":null}}`,
			expectedEstimatedCost: intPtr(18),
			// Query.user (5) only.
			expectedActualCost: intPtr(5),
		},
		computeCosts(),
	))

	t.Run("subgraph null for non-nullable field nulls the parent: later sibling is not charged", runWithoutError(
		ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ user { id token petName } }`,
				}
			},
			dataSources: dataSourceWithResponse(t,
				`{"data":{"user":{"id":"1","token":null,"petName":"Rex"}}}`),
			fields:                fields,
			expectedResponse:      `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.user.token'.","path":["user","token"]}],"data":{"user":null}}`,
			expectedEstimatedCost: intPtr(18),
			// Query.user (5) only.
			expectedActualCost: intPtr(5),
		},
		computeCosts(),
	))

	t.Run("denied non-nullable field bubbles to the root: nothing is charged", runWithoutError(
		ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ account { petName token } }`,
				}
			},
			dataSources: dataSourceWithResponse(t,
				`{"data":{"account":{"petName":"Rex","token":"t0k3n"}}}`),
			fields: fields,
			engineOptions: preFetchAuth(map[resolve.GraphCoordinate]string{
				{TypeName: "Account", FieldName: "token"}: "missing scope 'token:read'",
			}),
			expectedResponse:      `{"errors":[{"message":"Unauthorized to load field 'Query.account.token', Reason: missing scope 'token:read'.","path":["account","token"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":null}`,
			expectedEstimatedCost: intPtr(18),
			// The denial took the entire response away, so nothing is charged.
			expectedActualCost: intPtr(0),
		},
		computeCosts(),
	))

	// Arrays:

	t.Run("list: denied nullable field is not charged, delivered elements are", runWithoutError(
		ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ users { petName secret } }`,
				}
			},
			dataSources: dataSourceWithResponse(t,
				`{"data":{"users":[{"petName":"Rex","secret":"a"},{"petName":"Bob","secret":"b"}]}}`),
			fields: fields,
			engineOptions: preFetchAuth(map[resolve.GraphCoordinate]string{
				{TypeName: "User", FieldName: "secret"}: "missing scope 'secret:read'",
			}),
			expectedResponse: `{"errors":[{"message":"Unauthorized to load field 'Query.users.secret', Reason: missing scope 'secret:read'.","path":["users",0,"secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}},{"message":"Unauthorized to load field 'Query.users.secret', Reason: missing scope 'secret:read'.","path":["users",1,"secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":{"users":[{"petName":"Rex","secret":null},{"petName":"Bob","secret":null}]}}`,
			// User type weight (1) x default list size (10) + (petName (2) + secret (17)) x 10
			expectedEstimatedCost: intPtr(200),
			// Two delivered elements: User type weight (1) x 2 + petName (2) x 2. secret is
			// denied and free.
			expectedActualCost: intPtr(6),
		},
		computeCosts(),
	))

	t.Run("list: denied non-nullable field nulls every element, nothing is charged", runWithoutError(
		ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ users { petName token } }`,
				}
			},
			// The denial is per coordinate, so it nulls every element identically.
			dataSources: dataSourceWithResponse(t,
				`{"data":{"users":[{"petName":"Rex","token":"a"},{"petName":"Bob","token":"b"}]}}`),
			fields: fields,
			engineOptions: preFetchAuth(map[resolve.GraphCoordinate]string{
				{TypeName: "User", FieldName: "token"}: "missing scope 'token:read'",
			}),
			expectedResponse: `{"errors":[{"message":"Unauthorized to load field 'Query.users.token', Reason: missing scope 'token:read'.","path":["users",0,"token"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}},{"message":"Unauthorized to load field 'Query.users.token', Reason: missing scope 'token:read'.","path":["users",1,"token"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":{"users":[null,null]}}`,
			// User type weight (1) x default list size (10) + (petName (2) + token (11)) x 10
			expectedEstimatedCost: intPtr(140),
			// The client received only null elements: no User objects, no fields.
			expectedActualCost: intPtr(0),
		},
		computeCosts(),
	))

	t.Run("list: one element violates non-nullability, the delivered element stays charged", runWithoutError(
		ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ users { petName token } }`,
				}
			},
			// No authorizer: the second element violates token's non-nullability and is
			// nulled; the first element is delivered in full.
			dataSources: dataSourceWithResponse(t,
				`{"data":{"users":[{"petName":"Rex","token":"a"},{"petName":"Bob","token":null}]}}`),
			fields:                fields,
			expectedResponse:      `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.users.token'.","path":["users",1,"token"]}],"data":{"users":[{"petName":"Rex","token":"a"},null]}}`,
			expectedEstimatedCost: intPtr(140),
			// One delivered element: User type weight (1) + petName (2) + token (11).
			expectedActualCost: intPtr(14),
		},
		computeCosts(),
	))
}
