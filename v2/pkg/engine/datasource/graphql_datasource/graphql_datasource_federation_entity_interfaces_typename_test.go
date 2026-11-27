package graphql_datasource

import (
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestGraphQLDataSourceFederationEntityInterfaces_TypenamePlanning(t *testing.T) {
	definition, planConfiguration := interfaceObjectTypenamePlanConfiguration(t)

	t.Run("concrete typename from interface object", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query TestQuery {
					c {
						a {
							id
							... on A1 {
								fieldFromA
							}
						}
					}
				}
			`,
			"TestQuery",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID: 0,
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://c.service","body":{"query":"{c {a {__typename id}}}"}}`,
								DataSource:     &Source{},
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}),
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           1,
								DependsOnFetchIDs: []int{0},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://a.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on A1 {__typename fieldFromA}}}","variables":{"representations":[$$0$$]}}}`,
								Variables: []resolve.Variable{
									&resolve.ResolvableObjectVariable{
										Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("__typename"),
													Value: &resolve.String{
														Path: []string{"__typename"},
													},
													OnTypeNames: [][]byte{[]byte("A1"), []byte("A")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("A1"), []byte("A")},
												},
												{
													Name: []byte("__typename"),
													Value: &resolve.String{
														Path: []string{"__typename"},
													},
													OnTypeNames: [][]byte{[]byte("A")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("A")},
												},
											},
										}),
									},
								},
								DataSource:                            &Source{},
								PostProcessing:                        SingleEntityPostProcessingConfiguration,
								RequiresEntityFetch:                   true,
								SetTemplateOutputToNullOnVariableNull: true,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}, "c.a", resolve.ObjectPath("c"), resolve.ObjectPath("a")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("c"),
								Value: &resolve.Object{
									Path:          []string{"c"},
									Nullable:      true,
									PossibleTypes: map[string]struct{}{"C": {}},
									TypeName:      "C",
									Fields: []*resolve.Field{
										{
											Name: []byte("a"),
											Value: &resolve.Object{
												Path:          []string{"a"},
												Nullable:      true,
												PossibleTypes: map[string]struct{}{"A": {}, "A1": {}, "A2": {}},
												TypeName:      "A",
												Fields: []*resolve.Field{
													{
														Name: []byte("id"),
														Value: &resolve.Scalar{
															Path: []string{"id"},
														},
														OnTypeNames: [][]byte{[]byte("A1"), []byte("A")},
													},
													{
														Name: []byte("fieldFromA"),
														Value: &resolve.String{
															Path:     []string{"fieldFromA"},
															Nullable: true,
														},
														OnTypeNames: [][]byte{[]byte("A1")},
													},
													{
														Name: []byte("id"),
														Value: &resolve.Scalar{
															Path: []string{"id"},
														},
														OnTypeNames: [][]byte{[]byte("A2"), []byte("A")},
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
			planConfiguration,
			WithDefaultPostProcessor(),
		))
	})

	t.Run("mixed root and entity selections", func(t *testing.T) {
		t.Run("run", RunTest(
			definition,
			`
				query TestQuery {
					a {
						id
						fieldFromA
						... on A1 {
							fieldFromA1
							fieldFromB
						}
						... on A2 {
							fieldFromA2
						}
					}
					b {
						id
						fieldFromB
					}
					c {
						id
						fieldFromC
						a {
							id
							... on A1 {
								fieldFromA1
							}
						}
					}
				}
			`,
			"TestQuery",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Fetches: resolve.Sequence(
						resolve.Single(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID: 0,
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://a.service","body":{"query":"{a {id fieldFromA __typename ... on A1 {fieldFromA1 __typename id} ... on A2 {fieldFromA2}}}"}}`,
								DataSource:     &Source{},
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}),
						resolve.Single(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID: 1,
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://b.service","body":{"query":"{b {id fieldFromB}}"}}`,
								DataSource:     &Source{},
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}),
						resolve.Single(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID: 2,
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://c.service","body":{"query":"{c {id fieldFromC a {__typename id}}}"}}`,
								DataSource:     &Source{},
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}),
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           3,
								DependsOnFetchIDs: []int{2},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://a.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on A1 {__typename fieldFromA1}}}","variables":{"representations":[$$0$$]}}}`,
								Variables: []resolve.Variable{
									&resolve.ResolvableObjectVariable{
										Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("__typename"),
													Value: &resolve.String{
														Path: []string{"__typename"},
													},
													OnTypeNames: [][]byte{[]byte("A1"), []byte("A")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("A1"), []byte("A")},
												},
												{
													Name: []byte("__typename"),
													Value: &resolve.String{
														Path: []string{"__typename"},
													},
													OnTypeNames: [][]byte{[]byte("A")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("A")},
												},
											},
										}),
									},
								},
								DataSource:                            &Source{},
								PostProcessing:                        SingleEntityPostProcessingConfiguration,
								RequiresEntityFetch:                   true,
								SetTemplateOutputToNullOnVariableNull: true,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}, "c.a", resolve.ObjectPath("c"), resolve.ObjectPath("a")),
						resolve.SingleWithPath(&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           4,
								DependsOnFetchIDs: []int{0},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input: `{"method":"POST","url":"http://b.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on A1 {__typename fieldFromB}}}","variables":{"representations":[$$0$$]}}}`,
								Variables: []resolve.Variable{
									&resolve.ResolvableObjectVariable{
										Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("__typename"),
													Value: &resolve.String{
														Path: []string{"__typename"},
													},
													OnTypeNames: [][]byte{[]byte("A1")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("A1")},
												},
											},
										}),
									},
								},
								DataSource:                            &Source{},
								PostProcessing:                        SingleEntityPostProcessingConfiguration,
								RequiresEntityFetch:                   true,
								SetTemplateOutputToNullOnVariableNull: true,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}, "a", resolve.ObjectPath("a")),
					),
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("a"),
								Value: &resolve.Object{
									Path:          []string{"a"},
									Nullable:      true,
									PossibleTypes: map[string]struct{}{"A": {}, "A1": {}, "A2": {}},
									TypeName:      "A",
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("fieldFromA"),
											Value: &resolve.String{
												Path:     []string{"fieldFromA"},
												Nullable: true,
											},
										},
										{
											Name: []byte("fieldFromA1"),
											Value: &resolve.String{
												Path:     []string{"fieldFromA1"},
												Nullable: true,
											},
											OnTypeNames: [][]byte{[]byte("A1")},
										},
										{
											Name: []byte("fieldFromB"),
											Value: &resolve.String{
												Path:     []string{"fieldFromB"},
												Nullable: true,
											},
											OnTypeNames: [][]byte{[]byte("A1")},
										},
										{
											Name: []byte("fieldFromA2"),
											Value: &resolve.String{
												Path:     []string{"fieldFromA2"},
												Nullable: true,
											},
											OnTypeNames: [][]byte{[]byte("A2")},
										},
									},
								},
							},
							{
								Name: []byte("b"),
								Value: &resolve.Object{
									Path:          []string{"b"},
									Nullable:      true,
									PossibleTypes: map[string]struct{}{"B": {}},
									TypeName:      "B",
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("fieldFromB"),
											Value: &resolve.String{
												Path:     []string{"fieldFromB"},
												Nullable: true,
											},
										},
									},
								},
							},
							{
								Name: []byte("c"),
								Value: &resolve.Object{
									Path:          []string{"c"},
									Nullable:      true,
									PossibleTypes: map[string]struct{}{"C": {}},
									TypeName:      "C",
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("fieldFromC"),
											Value: &resolve.String{
												Path:     []string{"fieldFromC"},
												Nullable: true,
											},
										},
										{
											Name: []byte("a"),
											Value: &resolve.Object{
												Path:          []string{"a"},
												Nullable:      true,
												PossibleTypes: map[string]struct{}{"A": {}, "A1": {}, "A2": {}},
												TypeName:      "A",
												Fields: []*resolve.Field{
													{
														Name: []byte("id"),
														Value: &resolve.Scalar{
															Path: []string{"id"},
														},
														OnTypeNames: [][]byte{[]byte("A1"), []byte("A")},
													},
													{
														Name: []byte("fieldFromA1"),
														Value: &resolve.String{
															Path:     []string{"fieldFromA1"},
															Nullable: true,
														},
														OnTypeNames: [][]byte{[]byte("A1")},
													},
													{
														Name: []byte("id"),
														Value: &resolve.Scalar{
															Path: []string{"id"},
														},
														OnTypeNames: [][]byte{[]byte("A2"), []byte("A")},
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
			planConfiguration,
			WithDefaultPostProcessor(),
		))
	})
}

func interfaceObjectTypenamePlanConfiguration(t *testing.T) (string, plan.Configuration) {
	t.Helper()

	definition := `
		type Query {
			c: C
			b: B
			a: A
		}

		type C {
			id: ID!
			fieldFromC: String
			a: A
		}

		interface A {
			id: ID!
			fieldFromA: String
		}

		type B {
			id: ID!
			fieldFromB: String
		}

		type A1 implements A {
			id: ID!
			fieldFromB: String
			fieldFromA: String
			fieldFromA1: String
		}

		type A2 implements A {
			id: ID!
			fieldFromA: String
			fieldFromA2: String
		}
	`

	subgraphCSDL := `
		type Query {
			c: C
		}

		type C @key(fields: "id") {
			id: ID!
			fieldFromC: String
			a: A
		}

		type A @key(fields: "id") @interfaceObject {
			id: ID!
		}

		extend schema @link(
			url: "https://specs.apollo.dev/federation/v2.3",
			import: ["@key", "@tag", "@shareable", "@inaccessible", "@override", "@external", "@provides", "@requires", "@composeDirective", "@interfaceObject"]
		)
	`

	subgraphBSDL := `
		extend schema
			@link(url: "https://specs.apollo.dev/federation/v2.3", import: ["@key", "@shareable", "@requires", "@external", "@interfaceObject"])

		type Query {
			b: B
		}

		type B @key(fields: "id") {
			id: ID!
			fieldFromB: String
		}

		extend type A1 @key(fields: "id") {
			id: ID!
			fieldFromB: String
		}
	`

	subgraphBSchema := `
		type Query {
			b: B
		}

		type B @key(fields: "id") {
			id: ID!
			fieldFromB: String
		}

		type A1 @key(fields: "id") {
			id: ID!
			fieldFromB: String
		}

		extend schema
			@link(url: "https://specs.apollo.dev/federation/v2.3", import: ["@key", "@shareable", "@requires", "@external", "@interfaceObject"])
	`

	subgraphASDL := `
		extend schema
			@link(url: "https://specs.apollo.dev/federation/v2.3", import: [
				"@key"
				"@tag"
				"@shareable"
				"@inaccessible"
				"@override"
				"@external"
				"@provides"
				"@requires"
				"@interfaceObject"
			])

		type Query {
			a: A
		}

		interface A @key(fields: "id") {
			id: ID!
			fieldFromA: String
		}

		type A1 implements A @key(fields: "id") {
			id: ID!
			fieldFromA: String
			fieldFromA1: String
		}

		type A2 implements A @key(fields: "id") {
			id: ID!
			fieldFromA: String
			fieldFromA2: String
		}
	`

	cDataSource := mustDataSourceConfiguration(
		t,
		"c-service",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"c"}},
				{TypeName: "C", FieldNames: []string{"id", "fieldFromC", "a"}},
				{TypeName: "A", FieldNames: []string{"id"}},
				{TypeName: "A1", FieldNames: []string{"id"}},
				{TypeName: "A2", FieldNames: []string{"id"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "C", SelectionSet: "id"},
					{TypeName: "A", SelectionSet: "id"},
					{TypeName: "A1", SelectionSet: "id"},
					{TypeName: "A2", SelectionSet: "id"},
				},
				InterfaceObjects: []plan.EntityInterfaceConfiguration{
					{InterfaceTypeName: "A", ConcreteTypeNames: []string{"A1", "A2"}},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch: &FetchConfiguration{URL: "http://c.service"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{
				Enabled:    true,
				ServiceSDL: subgraphCSDL,
			}, subgraphCSDL),
		}),
	)

	bDataSource := mustDataSourceConfiguration(
		t,
		"b-service",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"b"}},
				{TypeName: "B", FieldNames: []string{"id", "fieldFromB"}},
				{TypeName: "A1", FieldNames: []string{"id", "fieldFromB"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "B", SelectionSet: "id"},
					{TypeName: "A1", SelectionSet: "id"},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch: &FetchConfiguration{URL: "http://b.service"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{
				Enabled:    true,
				ServiceSDL: subgraphBSDL,
			}, subgraphBSchema),
		}),
	)

	aDataSource := mustDataSourceConfiguration(
		t,
		"a-service",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"a"}},
				{TypeName: "A", FieldNames: []string{"id", "fieldFromA"}},
				{TypeName: "A1", FieldNames: []string{"id", "fieldFromA", "fieldFromA1"}},
				{TypeName: "A2", FieldNames: []string{"id", "fieldFromA", "fieldFromA2"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "A", SelectionSet: "id"},
					{TypeName: "A1", SelectionSet: "id"},
					{TypeName: "A2", SelectionSet: "id"},
				},
				EntityInterfaces: []plan.EntityInterfaceConfiguration{
					{InterfaceTypeName: "A", ConcreteTypeNames: []string{"A1", "A2"}},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch: &FetchConfiguration{URL: "http://a.service"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{
				Enabled:    true,
				ServiceSDL: subgraphASDL,
			}, subgraphASDL),
		}),
	)

	return definition, plan.Configuration{
		DataSources:                  []plan.DataSource{cDataSource, bDataSource, aDataSource},
		DisableResolveFieldPositions: true,
	}
}
