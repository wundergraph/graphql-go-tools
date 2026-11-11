package graphql_datasource

import (
	"testing"

	"github.com/stretchr/testify/require"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// The tests below cover a user-selected field which the planner reuses as a key member.
// listings resolves user and provides the key legacyId,
// social is reachable via the key id.

const keyDependencyDefinition = `
	type User {
		id: ID!
		legacyId: String
		name: String
		avatar: String
	}

	type Query {
		user: User
	}
`

func keyDependencyListings(t *testing.T) plan.DataSource {
	return mustMultiHopDataSourceConfiguration(t, "listings",
		[]plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"user"}},
			{TypeName: "User", FieldNames: []string{"legacyId"}},
		},
		[]plan.FederationFieldConfiguration{
			{TypeName: "User", SelectionSet: "legacyId", DisableEntityResolver: true},
		},
		`
			type Query {
				user: User
			}

			type User @key(fields: "legacyId", resolvable: false) {
				legacyId: String
			}
		`,
	)
}

func keyDependencySocial(t *testing.T) plan.DataSource {
	return mustMultiHopDataSourceConfiguration(t, "social",
		[]plan.TypeField{
			{TypeName: "User", FieldNames: []string{"id", "avatar"}},
		},
		[]plan.FederationFieldConfiguration{
			{TypeName: "User", SelectionSet: "id"},
		},
		`
			type User @key(fields: "id") {
				id: ID!
				avatar: String
			}
		`,
	)
}

// TestKeyFieldMovedToAnotherDataSource: the first selection run places user.id on accounts next to name,
// with a dependency on the key legacyId.
// The key id for the jump to social is reachable only via analytics, so user.id moves to analytics.
// The dependency of user.id on accounts has to be dropped with that move.
func TestKeyFieldMovedToAnotherDataSource(t *testing.T) {
	analytics := mustMultiHopDataSourceConfiguration(t, "analytics",
		[]plan.TypeField{
			{TypeName: "User", FieldNames: []string{"id", "legacyId"}},
		},
		[]plan.FederationFieldConfiguration{
			{TypeName: "User", SelectionSet: "legacyId"},
			{TypeName: "User", SelectionSet: "id", DisableEntityResolver: true},
		},
		`
			type User @key(fields: "legacyId") @key(fields: "id", resolvable: false) {
				id: ID!
				legacyId: String
			}
		`,
	)

	accounts := mustMultiHopDataSourceConfiguration(t, "accounts",
		[]plan.TypeField{
			{TypeName: "User", FieldNames: []string{"id", "legacyId", "name"}},
		},
		[]plan.FederationFieldConfiguration{
			{TypeName: "User", SelectionSet: "legacyId"},
		},
		`
			type User @key(fields: "legacyId") {
				id: ID!
				legacyId: String
				name: String
			}
		`,
	)

	representation := func(keyField string, keyValue resolve.Node) []resolve.Variable {
		return []resolve.Variable{
			&resolve.ResolvableObjectVariable{
				Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
					Nullable: true,
					Fields: []*resolve.Field{
						{
							Name:        []byte("__typename"),
							Value:       &resolve.String{Path: []string{"__typename"}},
							OnTypeNames: [][]byte{[]byte("User")},
						},
						{
							Name:        []byte(keyField),
							Value:       keyValue,
							OnTypeNames: [][]byte{[]byte("User")},
						},
					},
				}),
			},
		}
	}

	entityFetch := func(fetchID, dependsOn int, url, selection string, variables []resolve.Variable) *resolve.FetchTreeNode {
		return resolve.SingleWithPath(&resolve.SingleFetch{
			FetchDependencies: resolve.FetchDependencies{
				FetchID:           fetchID,
				DependsOnFetchIDs: []int{dependsOn},
			},
			FetchConfiguration: resolve.FetchConfiguration{
				Input:                                 `{"method":"POST","url":"` + url + `","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename ` + selection + `}}}","variables":{"representations":[$$0$$]}}}`,
				DataSource:                            &Source{},
				SetTemplateOutputToNullOnVariableNull: true,
				RequiresEntityFetch:                   true,
				PostProcessing:                        SingleEntityPostProcessingConfiguration,
				Variables:                             variables,
			},
			DataSourceIdentifier: []byte("graphql_datasource.Source"),
		}, "user", resolve.ObjectPath("user"))
	}

	byLegacyId := representation("legacyId", &resolve.String{Path: []string{"legacyId"}, Nullable: true})
	byId := representation("id", &resolve.Scalar{Path: []string{"id"}})

	RunTest(
		keyDependencyDefinition,
		`query { user { id name avatar } }`,
		"",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Fetches: resolve.Sequence(
					resolve.Single(&resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:          `{"method":"POST","url":"https://example.com/listings","body":{"query":"{user {__typename legacyId}}"}}`,
							DataSource:     &Source{},
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}),
					entityFetch(1, 0, "https://example.com/analytics", "id", byLegacyId),
					entityFetch(2, 0, "https://example.com/accounts", "name", byLegacyId),
					entityFetch(3, 1, "https://example.com/social", "avatar", byId),
				),
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("user"),
							Value: &resolve.Object{
								Path:          []string{"user"},
								Nullable:      true,
								PossibleTypes: map[string]struct{}{"User": {}},
								TypeName:      "User",
								Fields: []*resolve.Field{
									{Name: []byte("id"), Value: &resolve.Scalar{Path: []string{"id"}}},
									{Name: []byte("name"), Value: &resolve.String{Path: []string{"name"}, Nullable: true}},
									{Name: []byte("avatar"), Value: &resolve.String{Path: []string{"avatar"}, Nullable: true}},
								},
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSource{
				keyDependencyListings(t),
				analytics,
				accounts,
				keyDependencySocial(t),
			},
			DisableResolveFieldPositions: true,
		},
		WithDefaultPostProcessor(),
	)(t)
}

// TestKeyChainThroughSelectedFieldDependsOnKeyOnce: user.id is selected on accounts,
// and it is the key of the chain listings -> accounts -> social.
// Both make user.id depend on legacyId, which has to be listed once.
func TestKeyChainThroughSelectedFieldDependsOnKeyOnce(t *testing.T) {
	accounts := mustMultiHopDataSourceConfiguration(t, "accounts",
		[]plan.TypeField{
			{TypeName: "User", FieldNames: []string{"id", "legacyId", "name"}},
		},
		[]plan.FederationFieldConfiguration{
			{TypeName: "User", SelectionSet: "legacyId"},
			{TypeName: "User", SelectionSet: "id"},
		},
		`
			type User @key(fields: "legacyId") @key(fields: "id") {
				id: ID!
				legacyId: String
				name: String
			}
		`,
	)

	checkDependencies := func(t *testing.T, actualPlan plan.Plan) {
		fetches := actualPlan.(*plan.SynchronousResponsePlan).Response.Fetches.ChildNodes
		require.Len(t, fetches, 3)

		dependencies := make(map[string][]resolve.FetchDependency, len(fetches))
		for _, fetch := range fetches {
			info := fetch.Item.Fetch.(*resolve.SingleFetch).Info
			dependencies[info.DataSourceName] = info.CoordinateDependencies
		}

		byLegacyId := []resolve.FetchDependencyOrigin{
			{
				FetchID:    0,
				Subgraph:   "listings",
				Coordinate: resolve.GraphCoordinate{TypeName: "User", FieldName: "legacyId"},
				IsKey:      true,
			},
		}
		require.Equal(t, map[string][]resolve.FetchDependency{
			"listings": {},
			"accounts": {
				{
					Coordinate:      resolve.GraphCoordinate{TypeName: "User", FieldName: "id"},
					IsUserRequested: true,
					DependsOn:       byLegacyId,
				},
				{
					Coordinate:      resolve.GraphCoordinate{TypeName: "User", FieldName: "name"},
					IsUserRequested: true,
					DependsOn:       byLegacyId,
				},
			},
			// precondition: social gets the key id from accounts
			"social": {
				{
					Coordinate:      resolve.GraphCoordinate{TypeName: "User", FieldName: "avatar"},
					IsUserRequested: true,
					DependsOn: []resolve.FetchDependencyOrigin{
						{
							FetchID:    1,
							Subgraph:   "accounts",
							Coordinate: resolve.GraphCoordinate{TypeName: "User", FieldName: "id"},
							IsKey:      true,
						},
					},
				},
			},
		}, dependencies)
	}

	RunTest(
		keyDependencyDefinition,
		`query { user { id name avatar } }`,
		"",
		nil,
		plan.Configuration{
			DataSources: []plan.DataSource{
				keyDependencyListings(t),
				accounts,
				keyDependencySocial(t),
			},
			DisableResolveFieldPositions: true,
		},
		WithDefaultPostProcessor(),
		WithIncludeFieldDependencies(),
		WithPlanCheck(checkDependencies),
	)(t)
}
