package graphql_datasource

import (
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// Reproduces the failing federation-audit suite "parent-entity-call-complex".
//
// Subgraph a: Product @key(id) { id @external, category @shareable }, Category { details }
// Subgraph b: Product @key(id) { id @external, category @shareable }, Category { id @shareable } (implicit key id, entity resolver disabled)
// Subgraph c: Category @key(id) { id, name }
// Subgraph d: Query { productFromD }, Product @key(id) { id, name }
func TestGraphQLDataSourceFederation_ParentEntityCallComplex(t *testing.T) {
	definition := `
		type Product {
			id: ID
			name: String
			category: Category
		}

		type Category {
			id: ID
			name: String
			details: String
		}

		type Query {
			productFromD(id: ID!): Product
		}
	`

	subgraphASDL := `
		type Product @key(fields: "id") {
			id: ID @external
			category: Category @shareable
		}

		type Category {
			details: String
		}
	`

	subgraphBSDL := `
		type Product @key(fields: "id") {
			id: ID @external
			category: Category @shareable
		}

		type Category {
			id: ID @shareable
		}
	`

	subgraphCSDL := `
		type Category @key(fields: "id") {
			id: ID
			name: String
		}
	`

	subgraphDSDL := `
		type Query {
			productFromD(id: ID!): Product
		}

		type Product @key(fields: "id") {
			id: ID
			name: String
		}
	`

	dsA := mustDataSourceConfiguration(
		t,
		"a",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:           "Product",
					FieldNames:         []string{"category"},
					ExternalFieldNames: []string{"id"},
				},
			},
			ChildNodes: []plan.TypeField{
				{
					TypeName:   "Category",
					FieldNames: []string{"details"},
				},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:     "Product",
						SelectionSet: "id",
					},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch: &FetchConfiguration{
				URL: "http://a.service",
			},
			SchemaConfiguration: mustSchema(t,
				&FederationConfiguration{
					Enabled:    true,
					ServiceSDL: subgraphASDL,
				},
				subgraphASDL,
			),
		}),
	)

	dsB := mustDataSourceConfiguration(
		t,
		"b",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:           "Product",
					FieldNames:         []string{"category"},
					ExternalFieldNames: []string{"id"},
				},
				{
					TypeName:   "Category",
					FieldNames: []string{"id"},
				},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:     "Product",
						SelectionSet: "id",
					},
					{
						TypeName:              "Category",
						SelectionSet:          "id",
						DisableEntityResolver: true,
					},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch: &FetchConfiguration{
				URL: "http://b.service",
			},
			SchemaConfiguration: mustSchema(t,
				&FederationConfiguration{
					Enabled:    true,
					ServiceSDL: subgraphBSDL,
				},
				subgraphBSDL,
			),
		}),
	)

	dsC := mustDataSourceConfiguration(
		t,
		"c",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Category",
					FieldNames: []string{"id", "name"},
				},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:     "Category",
						SelectionSet: "id",
					},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch: &FetchConfiguration{
				URL: "http://c.service",
			},
			SchemaConfiguration: mustSchema(t,
				&FederationConfiguration{
					Enabled:    true,
					ServiceSDL: subgraphCSDL,
				},
				subgraphCSDL,
			),
		}),
	)

	dsD := mustDataSourceConfiguration(
		t,
		"d",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Query",
					FieldNames: []string{"productFromD"},
				},
				{
					TypeName:   "Product",
					FieldNames: []string{"id", "name"},
				},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:     "Product",
						SelectionSet: "id",
					},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch: &FetchConfiguration{
				URL: "http://d.service",
			},
			SchemaConfiguration: mustSchema(t,
				&FederationConfiguration{
					Enabled:    true,
					ServiceSDL: subgraphDSDL,
				},
				subgraphDSDL,
			),
		}),
	)

	planConfiguration := plan.Configuration{
		DataSources: []plan.DataSource{
			dsA,
			dsB,
			dsC,
			dsD,
		},
		DisableResolveFieldPositions: true,
		Fields: plan.FieldConfigurations{
			{
				TypeName:  "Query",
				FieldName: "productFromD",
				Arguments: plan.ArgumentsConfigurations{
					{
						Name:       "id",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
	}

	productKeyRepresentationVariable := &resolve.ResolvableObjectVariable{
		Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
			Nullable: true,
			Fields: []*resolve.Field{
				{
					Name: []byte("__typename"),
					Value: &resolve.String{
						Path: []string{"__typename"},
					},
					OnTypeNames: [][]byte{[]byte("Product")},
				},
				{
					Name: []byte("id"),
					Value: &resolve.Scalar{
						Path:     []string{"id"},
						Nullable: true,
					},
					OnTypeNames: [][]byte{[]byte("Product")},
				},
			},
		}),
	}

	categoryKeyRepresentationVariable := &resolve.ResolvableObjectVariable{
		Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
			Nullable: true,
			Fields: []*resolve.Field{
				{
					Name: []byte("__typename"),
					Value: &resolve.String{
						Path: []string{"__typename"},
					},
					OnTypeNames: [][]byte{[]byte("Category")},
				},
				{
					Name: []byte("id"),
					Value: &resolve.Scalar{
						Path:     []string{"id"},
						Nullable: true,
					},
					OnTypeNames: [][]byte{[]byte("Category")},
				},
			},
		}),
	}

	expectedPlan := &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Fetches: resolve.Sequence(
				resolve.Single(&resolve.SingleFetch{
					FetchDependencies: resolve.FetchDependencies{
						FetchID: 0,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					FetchConfiguration: resolve.FetchConfiguration{
						Input:      `{"method":"POST","url":"http://d.service","body":{"query":"query($a: ID!){productFromD(id: $a){id name __typename}}","variables":{"a":$$0$$}}}`,
						DataSource: &Source{},
						Variables: []resolve.Variable{
							&resolve.ContextVariable{
								Path:     []string{"a"},
								Renderer: resolve.NewJSONVariableRenderer(),
							},
						},
						PostProcessing: DefaultPostProcessingConfiguration,
					},
				}),
				resolve.SingleWithPath(&resolve.SingleFetch{
					FetchDependencies: resolve.FetchDependencies{
						FetchID:           1,
						DependsOnFetchIDs: []int{0},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					FetchConfiguration: resolve.FetchConfiguration{
						Input:                                 `{"method":"POST","url":"http://a.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename category {details __typename}}}}","variables":{"representations":[$$0$$]}}}`,
						DataSource:                            &Source{},
						SetTemplateOutputToNullOnVariableNull: true,
						Variables: []resolve.Variable{
							productKeyRepresentationVariable,
						},
						PostProcessing:      SingleEntityPostProcessingConfiguration,
						RequiresEntityFetch: true,
					},
				}, "productFromD", resolve.ObjectPath("productFromD")),
				resolve.SingleWithPath(&resolve.SingleFetch{
					FetchDependencies: resolve.FetchDependencies{
						FetchID:           2,
						DependsOnFetchIDs: []int{0},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					FetchConfiguration: resolve.FetchConfiguration{
						Input:                                 `{"method":"POST","url":"http://b.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename category {id}}}}","variables":{"representations":[$$0$$]}}}`,
						DataSource:                            &Source{},
						SetTemplateOutputToNullOnVariableNull: true,
						Variables: []resolve.Variable{
							productKeyRepresentationVariable,
						},
						PostProcessing:      SingleEntityPostProcessingConfiguration,
						RequiresEntityFetch: true,
					},
				}, "productFromD", resolve.ObjectPath("productFromD")),
				resolve.SingleWithPath(&resolve.SingleFetch{
					FetchDependencies: resolve.FetchDependencies{
						FetchID:           3,
						DependsOnFetchIDs: []int{2},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					FetchConfiguration: resolve.FetchConfiguration{
						Input:                                 `{"method":"POST","url":"http://c.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Category {__typename name}}}","variables":{"representations":[$$0$$]}}}`,
						DataSource:                            &Source{},
						SetTemplateOutputToNullOnVariableNull: true,
						Variables: []resolve.Variable{
							categoryKeyRepresentationVariable,
						},
						PostProcessing:      SingleEntityPostProcessingConfiguration,
						RequiresEntityFetch: true,
					},
				}, "productFromD.category", resolve.ObjectPath("productFromD"), resolve.ObjectPath("category")),
			),
			Data: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("productFromD"),
						Value: &resolve.Object{
							Path:     []string{"productFromD"},
							Nullable: true,
							PossibleTypes: map[string]struct{}{
								"Product": {},
							},
							TypeName: "Product",
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.Scalar{
										Path:     []string{"id"},
										Nullable: true,
									},
								},
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path:     []string{"name"},
										Nullable: true,
									},
								},
								{
									Name: []byte("category"),
									Value: &resolve.Object{
										Path:     []string{"category"},
										Nullable: true,
										PossibleTypes: map[string]struct{}{
											"Category": {},
										},
										TypeName: "Category",
										Fields: []*resolve.Field{
											{
												Name: []byte("id"),
												Value: &resolve.Scalar{
													Path:     []string{"id"},
													Nullable: true,
												},
											},
											{
												Name: []byte("name"),
												Value: &resolve.String{
													Path:     []string{"name"},
													Nullable: true,
												},
											},
											{
												Name: []byte("details"),
												Value: &resolve.String{
													Path:     []string{"details"},
													Nullable: true,
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
		},
	}

	t.Run("query products with categories", RunTest(
		definition,
		`
		query Requires {
			productFromD(id: "1") {
				id
				name
				category {
					id
					name
					details
				}
			}
		}
		`,
		"Requires",
		expectedPlan,
		planConfiguration,
		WithDefaultPostProcessor(),
	))
}
