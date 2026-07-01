package graphql_datasource

import (
	"fmt"
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// Reproduces The Guild federation-gateway-audit "partial-union-complex" suite.
//
// A union appears below shareable fields on a shared entity. The union has
// different members per subgraph:
//
//	subgraph A: union Action = Common | OnlyA
//	subgraph B: union Action = Common | OnlyB
//
// The planner must restrict the inline fragments it sends to a subgraph to the
// union members that subgraph actually defines. Sending `... on OnlyB` to A (or
// `... on OnlyA` to B) makes the subgraph reject the query (HTTP 400/500).
func partialUnionComplexPlanConfiguration(t *testing.T) plan.Configuration {
	subgraphASDL := `
		type Query {
			rootA: Container
			shared: Container @shareable
		}

		type Container @key(fields: "id") {
			id: ID!
			wrapper: Wrapper @shareable
		}

		type Wrapper @shareable {
			actions: [Action!]! @shareable
		}

		union Action = Common | OnlyA

		type Common @shareable {
			label: String
		}

		type OnlyA {
			a: String
		}
	`

	subgraphA := mustDataSourceConfiguration(
		t,
		"subgraph-a",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"rootA", "shared"}},
				{TypeName: "Container", FieldNames: []string{"id", "wrapper"}},
			},
			ChildNodes: []plan.TypeField{
				{TypeName: "Wrapper", FieldNames: []string{"actions"}},
				{TypeName: "Common", FieldNames: []string{"label"}},
				{TypeName: "OnlyA", FieldNames: []string{"a"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Container", SelectionSet: "id"},
				},
			},
		},
		mustCustomConfiguration(t,
			ConfigurationInput{
				Fetch: &FetchConfiguration{URL: "http://subgraph-a"},
				SchemaConfiguration: mustSchema(t,
					&FederationConfiguration{Enabled: true, ServiceSDL: subgraphASDL},
					subgraphASDL,
				),
			},
		),
	)

	subgraphBSDL := `
		type Query {
			rootB: Container
			shared: Container @shareable
		}

		type Container @key(fields: "id") {
			id: ID!
			wrapper: Wrapper @shareable
			bWrapper: Wrapper
		}

		type Wrapper @shareable {
			actions: [Action!]! @shareable
		}

		union Action = Common | OnlyB

		type Common @shareable {
			label: String
		}

		type OnlyB {
			b: String
		}
	`

	subgraphB := mustDataSourceConfiguration(
		t,
		"subgraph-b",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"rootB", "shared"}},
				{TypeName: "Container", FieldNames: []string{"id", "wrapper", "bWrapper"}},
			},
			ChildNodes: []plan.TypeField{
				{TypeName: "Wrapper", FieldNames: []string{"actions"}},
				{TypeName: "Common", FieldNames: []string{"label"}},
				{TypeName: "OnlyB", FieldNames: []string{"b"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Container", SelectionSet: "id"},
				},
			},
		},
		mustCustomConfiguration(t,
			ConfigurationInput{
				Fetch: &FetchConfiguration{URL: "http://subgraph-b"},
				SchemaConfiguration: mustSchema(t,
					&FederationConfiguration{Enabled: true, ServiceSDL: subgraphBSDL},
					subgraphBSDL,
				),
			},
		),
	)

	return plan.Configuration{
		DataSources: []plan.DataSource{
			subgraphA,
			subgraphB,
		},
		DisableResolveFieldPositions: true,
	}
}

const partialUnionComplexDefinition = `
	type Container {
		id: ID!
		wrapper: Wrapper
		bWrapper: Wrapper
	}

	type Wrapper {
		actions: [Action!]!
	}

	union Action = Common | OnlyA | OnlyB

	type Common {
		label: String
	}

	type OnlyA {
		a: String
	}

	type OnlyB {
		b: String
	}

	type Query {
		rootA: Container
		rootB: Container
		shared: Container
	}
`

func partialUnionTypenameField(onTypeNames ...string) *resolve.Field {
	field := &resolve.Field{
		Name:  []byte("__typename"),
		Value: &resolve.String{Path: []string{"__typename"}, IsTypeName: true},
	}
	for _, name := range onTypeNames {
		field.OnTypeNames = append(field.OnTypeNames, []byte(name))
	}
	return field
}

func partialUnionStringField(name, onTypeName string) *resolve.Field {
	return &resolve.Field{
		Name:        []byte(name),
		Value:       &resolve.String{Path: []string{name}, Nullable: true},
		OnTypeNames: [][]byte{[]byte(onTypeName)},
	}
}

// partialUnionSingleFetchPlan builds the expected plan for a query that resolves
// `<rootField> { wrapper { actions { ... } } }` in a single subgraph fetch.
func partialUnionSingleFetchPlan(url, rootField, query string, itemFields []*resolve.Field) *plan.SynchronousResponsePlan {
	return &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Fetches: resolve.Sequence(
				resolve.Single(&resolve.SingleFetch{
					FetchConfiguration: resolve.FetchConfiguration{
						Input:          fmt.Sprintf(`{"method":"POST","url":"%s","body":{"query":%q}}`, url, query),
						PostProcessing: DefaultPostProcessingConfiguration,
						DataSource:     &Source{},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}),
			),
			Data: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte(rootField),
						Value: &resolve.Object{
							Path:          []string{rootField},
							Nullable:      true,
							PossibleTypes: map[string]struct{}{"Container": {}},
							TypeName:      "Container",
							Fields: []*resolve.Field{
								{
									Name: []byte("wrapper"),
									Value: &resolve.Object{
										Path:          []string{"wrapper"},
										Nullable:      true,
										PossibleTypes: map[string]struct{}{"Wrapper": {}},
										TypeName:      "Wrapper",
										Fields: []*resolve.Field{
											{
												Name: []byte("actions"),
												Value: &resolve.Array{
													Path: []string{"actions"},
													Item: &resolve.Object{
														PossibleTypes: map[string]struct{}{"Common": {}, "OnlyA": {}, "OnlyB": {}},
														TypeName:      "Action",
														Fields:        itemFields,
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
		},
	}
}

// TestPartialUnionComplex reproduces The Guild federation-gateway-audit
// "partial-union-complex" suite. The union Action has different members per
// subgraph (A: Common|OnlyA, B: Common|OnlyB) and the members are non-entity value
// types reached via a @shareable path on the shared Container entity.
//
// When the union field is resolvable by multiple candidate subgraphs, members not
// common to all candidates must not change the response shape based on which
// candidate the planner picks. A member unique to the resolving subgraph is kept in
// the response as null but excluded from the upstream fetch; a foreign member is
// dropped entirely. (case 5 - an entity hop forced into a single subgraph - keeps
// that subgraph's members and is exercised end-to-end by the audit.)
func TestPartialUnionComplex(t *testing.T) {
	planConfiguration := partialUnionComplexPlanConfiguration(t)

	t.Run("case 1 - rootA: own member OnlyA kept as null, foreign OnlyB dropped", RunTest(
		partialUnionComplexDefinition,
		`query { rootA { wrapper { actions { __typename ... on Common { label } ... on OnlyA { a } ... on OnlyB { b } } } } }`,
		"",
		partialUnionSingleFetchPlan(
			"http://subgraph-a", "rootA",
			"{rootA {wrapper {actions {__typename ... on Common {label} ... on OnlyA {__typename}}}}}",
			[]*resolve.Field{
				partialUnionTypenameField(),
				partialUnionStringField("label", "Common"),
				partialUnionStringField("a", "OnlyA"),
				partialUnionTypenameField("OnlyA"),
			},
		),
		planConfiguration,
		WithDefaultPostProcessor(),
	))

	t.Run("case 2 - rootB: own member OnlyB kept as null, foreign OnlyA dropped", RunTest(
		partialUnionComplexDefinition,
		`query { rootB { wrapper { actions { __typename ... on Common { label } ... on OnlyA { a } ... on OnlyB { b } } } } }`,
		"",
		partialUnionSingleFetchPlan(
			"http://subgraph-b", "rootB",
			"{rootB {wrapper {actions {__typename ... on Common {label} ... on OnlyB {__typename}}}}}",
			[]*resolve.Field{
				partialUnionTypenameField(),
				partialUnionStringField("label", "Common"),
				partialUnionStringField("b", "OnlyB"),
				partialUnionTypenameField("OnlyB"),
			},
		),
		planConfiguration,
		WithDefaultPostProcessor(),
	))

	t.Run("case 3 - rootA: only foreign OnlyB requested, pruned to __typename", RunTest(
		partialUnionComplexDefinition,
		`query { rootA { wrapper { actions { __typename ... on OnlyB { b } } } } }`,
		"",
		partialUnionSingleFetchPlan(
			"http://subgraph-a", "rootA",
			"{rootA {wrapper {actions {__typename}}}}",
			[]*resolve.Field{
				partialUnionTypenameField(),
			},
		),
		planConfiguration,
		WithDefaultPostProcessor(),
	))

	t.Run("case 4 - shared: resolvable in both, only common member kept", RunTest(
		partialUnionComplexDefinition,
		`query { shared { wrapper { actions { __typename ... on Common { label } ... on OnlyA { a } ... on OnlyB { b } } } } }`,
		"",
		partialUnionSingleFetchPlan(
			"http://subgraph-a", "shared",
			"{shared {wrapper {actions {__typename ... on Common {label}}}}}",
			[]*resolve.Field{
				partialUnionTypenameField(),
				partialUnionStringField("label", "Common"),
			},
		),
		planConfiguration,
		WithDefaultPostProcessor(),
	))
}
