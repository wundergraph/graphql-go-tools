package graphql_datasource

import (
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// Reproduces The Guild's federation gateway audit suite "child-type-mismatch": a union whose
// members select the same field (`id`) which is non-null in one subgraph (User.id: ID!) and
// nullable in another (Admin.id: ID). The generated subgraph operation must alias the field per
// member, otherwise both this engine's validator and the subgraph itself reject it.
func TestChildTypeMismatchUnion(t *testing.T) {
	definition := `
		union Account = User | Admin

		type User {
			id: ID
			name: String
		}

		type Admin {
			id: ID
			name: String
		}

		type Query {
			accounts: [Account!]!
		}
	`

	subgraphSDL := `
		union Account = User | Admin

		type User @key(fields: "id") {
			id: ID!
			name: String
		}

		type Admin {
			id: ID
			name: String @shareable
		}

		type Query {
			accounts: [Account!]!
		}
	`

	datasourceConfiguration := mustDataSourceConfiguration(
		t,
		"accounts-service",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"accounts"}},
				{TypeName: "User", FieldNames: []string{"id", "name"}},
			},
			ChildNodes: []plan.TypeField{
				{TypeName: "Admin", FieldNames: []string{"id", "name"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "User", SelectionSet: "id"},
				},
			},
		},
		mustCustomConfiguration(t,
			ConfigurationInput{
				Fetch: &FetchConfiguration{URL: "http://accounts.service"},
				SchemaConfiguration: mustSchema(t,
					&FederationConfiguration{Enabled: true, ServiceSDL: subgraphSDL},
					subgraphSDL,
				),
			},
		),
	)

	planConfiguration := plan.Configuration{
		DataSources:                  []plan.DataSource{datasourceConfiguration},
		DisableResolveFieldPositions: true,
	}

	t.Run("conflicting id across union members is aliased per member", RunTest(
		definition,
		`
		query Accounts {
			accounts {
				... on User { id name }
				... on Admin { id name }
			}
		}`,
		"Accounts",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Fetches: resolve.Sequence(
					resolve.Single(&resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:          `{"method":"POST","url":"http://accounts.service","body":{"query":"{accounts {__typename ... on User {__internal_merge_User_id: id name} ... on Admin {__internal_merge_Admin_id: id name}}}"}}`,
							PostProcessing: DefaultPostProcessingConfiguration,
							DataSource:     &Source{},
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}),
				),
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("accounts"),
							Value: &resolve.Array{
								Path:     []string{"accounts"},
								Nullable: false,
								Item: &resolve.Object{
									Nullable: false,
									PossibleTypes: map[string]struct{}{
										"Admin": {},
										"User":  {},
									},
									TypeName: "Account",
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path:     []string{"__internal_merge_User_id"},
												Nullable: true,
											},
											OnTypeNames: [][]byte{[]byte("User")},
										},
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path:     []string{"name"},
												Nullable: true,
											},
											OnTypeNames: [][]byte{[]byte("User")},
										},
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path:     []string{"__internal_merge_Admin_id"},
												Nullable: true,
											},
											OnTypeNames: [][]byte{[]byte("Admin")},
										},
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path:     []string{"name"},
												Nullable: true,
											},
											OnTypeNames: [][]byte{[]byte("Admin")},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		planConfiguration,
		WithDefaultPostProcessor(),
	))

	t.Run("user alias on the conflicting field is preserved and aliased per member", RunTest(
		definition,
		`
		query Accounts {
			accounts {
				... on User { account_id: id name }
				... on Admin { account_id: id name }
			}
		}`,
		"Accounts",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Fetches: resolve.Sequence(
					resolve.Single(&resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:          `{"method":"POST","url":"http://accounts.service","body":{"query":"{accounts {__typename ... on User {__internal_merge_User_account_id: id name} ... on Admin {__internal_merge_Admin_account_id: id name}}}"}}`,
							PostProcessing: DefaultPostProcessingConfiguration,
							DataSource:     &Source{},
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}),
				),
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("accounts"),
							Value: &resolve.Array{
								Path:     []string{"accounts"},
								Nullable: false,
								Item: &resolve.Object{
									Nullable: false,
									PossibleTypes: map[string]struct{}{
										"Admin": {},
										"User":  {},
									},
									TypeName: "Account",
									Fields: []*resolve.Field{
										{
											Name: []byte("account_id"),
											Value: &resolve.Scalar{
												Path:     []string{"__internal_merge_User_account_id"},
												Nullable: true,
											},
											OnTypeNames: [][]byte{[]byte("User")},
										},
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path:     []string{"name"},
												Nullable: true,
											},
											OnTypeNames: [][]byte{[]byte("User")},
										},
										{
											Name: []byte("account_id"),
											Value: &resolve.Scalar{
												Path:     []string{"__internal_merge_Admin_account_id"},
												Nullable: true,
											},
											OnTypeNames: [][]byte{[]byte("Admin")},
										},
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path:     []string{"name"},
												Nullable: true,
											},
											OnTypeNames: [][]byte{[]byte("Admin")},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		planConfiguration,
		WithDefaultPostProcessor(),
	))

	t.Run("distinct user aliases across members are not rewritten", RunTest(
		definition,
		`
		query Accounts {
			accounts {
				... on User { uid: id name }
				... on Admin { aid: id name }
			}
		}`,
		"Accounts",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Fetches: resolve.Sequence(
					resolve.Single(&resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:          `{"method":"POST","url":"http://accounts.service","body":{"query":"{accounts {__typename ... on User {uid: id name} ... on Admin {aid: id name}}}"}}`,
							PostProcessing: DefaultPostProcessingConfiguration,
							DataSource:     &Source{},
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					}),
				),
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("accounts"),
							Value: &resolve.Array{
								Path:     []string{"accounts"},
								Nullable: false,
								Item: &resolve.Object{
									Nullable: false,
									PossibleTypes: map[string]struct{}{
										"Admin": {},
										"User":  {},
									},
									TypeName: "Account",
									Fields: []*resolve.Field{
										{
											Name: []byte("uid"),
											Value: &resolve.Scalar{
												Path:     []string{"uid"},
												Nullable: true,
											},
											OnTypeNames: [][]byte{[]byte("User")},
										},
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path:     []string{"name"},
												Nullable: true,
											},
											OnTypeNames: [][]byte{[]byte("User")},
										},
										{
											Name: []byte("aid"),
											Value: &resolve.Scalar{
												Path:     []string{"aid"},
												Nullable: true,
											},
											OnTypeNames: [][]byte{[]byte("Admin")},
										},
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path:     []string{"name"},
												Nullable: true,
											},
											OnTypeNames: [][]byte{[]byte("Admin")},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		planConfiguration,
		WithDefaultPostProcessor(),
	))
}
