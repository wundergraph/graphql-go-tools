package graphql_datasource

import (
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestGraphQLDataSourceFederation(t *testing.T) {
	batchFactory := NewBatchFactory()
	federationFactory := &Factory{BatchFactory: batchFactory}

	t.Run("composite keys", RunTest(
		`
		type User {
			id: ID!
			account: Account
		}

		type Account {
			id: ID!
			name: String!
			info: Info!
		}

		type Info {
			a: ID!
			b: ID!
		}

		type Query {
			user: User
		}`,
		`
		query ComposedKeys {
			user {
				account {
					name
				}
			}
		}`,
		"ComposedKeys",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.ParallelFetch{
						Fetches: []resolve.Fetch{
							&resolve.SingleFetch{
								BufferId:              0,
								Input:                 `{"method":"POST","url":"http://user.service","body":{"query":"query{user{account{id info{a b}}}}"}}`,
								DataSource:            &Source{},
								DataSourceIdentifier:  []byte("graphql_datasource.Source"),
								ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
							},
						},
					},
					Fields: []*resolve.Field{
						{
							HasBuffer: true,
							BufferID:  0,
							Name:      []byte("user"),
							Value: &resolve.Object{
								Path:     []string{"user"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										HasBuffer: true,
										BufferID:  1,
										Name:      []byte("account"),

										Value: &resolve.Object{
											Path:     []string{"account"},
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path: []string{"name"},
													},
												},
											},
											Fetch: &resolve.SingleFetch{
												BufferId:   1,
												Input:      `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Account {name}}}","variables":{"representations":[{"upc":$$0$$,"__typename":"Product"}]}}}`,
												DataSource: &Source{},
												Variables: resolve.NewVariables(
													&resolve.ContextVariable{
														Path:     []string{"b"},
														Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
													},
												),
												DataSourceIdentifier:  []byte("graphql_datasource.Source"),
												ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"user"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"id", "account"},
						},
						{
							TypeName:   "Account",
							FieldNames: []string{"id", "info"},
						},
						{
							TypeName:   "Info",
							FieldNames: []string{"a", "b"},
						},
					},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "http://user.service",
						},
						Federation: FederationConfiguration{
							Enabled:    true,
							ServiceSDL: "extend type Query { user: User } type User @key(fields: \"id\") { id: ID! account: Account } extend type Account @key(fields: \"id info {a b}\") { id: ID! info: Info } extend type Info { a: ID! b: ID! }",
						},
					}),
					Factory: federationFactory,
					TypesNew: plan.TypeConfigurations{
						{
							TypeName:          "Account",
							RequiresFieldsNew: "id info {a b}",
						},
					},
				},
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Account",
							FieldNames: []string{"id", "name", "info"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Info",
							FieldNames: []string{"a", "b"},
						},
					},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "http://account.service",
						},
						Federation: FederationConfiguration{
							Enabled:    true,
							ServiceSDL: "type Account @key(fields: \"id info {a b}\") { id: ID! name: String info: Info } type Info { a: ID! b: ID! }",
						},
					}),
					Factory: federationFactory,
					TypesNew: plan.TypeConfigurations{
						{
							TypeName:          "Account",
							RequiresFieldsNew: "id info {a b}",
						},
					},
				},
			},
			DisableResolveFieldPositions: true,
			Debug: plan.DebugConfiguration{
				PrintOperationWithRequiredFields: true,
				PrintPlanningPaths:               true,
				PrintQueryPlans:                  true,
				ConfigurationVisitor:             true,
				PlanningVisitor:                  true,
				DatasourceVisitor:                true,
			},
		}))
}
