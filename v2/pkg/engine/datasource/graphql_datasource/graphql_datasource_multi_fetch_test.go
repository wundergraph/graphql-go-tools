package graphql_datasource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// planAndPostprocessMultiFetch plans with query-plan info enabled and runs the
// given postprocess options, returning the executable plan. Field info is left
// on (candidacy needs FetchInfo), unlike the datasourcetesting harness which
// force-disables it.
func planAndPostprocessMultiFetch(t *testing.T, definition, operation, operationName string, config plan.Configuration, procOpts ...postprocess.ProcessorOption) plan.Plan {
	t.Helper()

	def := unsafeparser.ParseGraphqlDocumentString(definition)
	op := unsafeparser.ParseGraphqlDocumentString(operation)
	require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&def))

	norm := astnormalization.NewWithOpts(
		astnormalization.WithExtractVariables(),
		astnormalization.WithInlineFragmentSpreads(),
		astnormalization.WithRemoveFragmentDefinitions(),
		astnormalization.WithRemoveUnusedVariables(),
	)
	var report operationreport.Report
	norm.NormalizeOperation(&op, &def, &report)
	require.False(t, report.HasErrors(), report.Error())

	astvalidation.DefaultOperationValidator().Validate(&op, &def, &report)
	require.False(t, report.HasErrors(), report.Error())

	p, err := plan.NewPlanner(config)
	require.NoError(t, err)

	actualPlan := p.Plan(&op, &def, operationName, &report, plan.IncludeQueryPlanInResponse())
	require.False(t, report.HasErrors(), report.Error())

	postprocess.NewProcessor(procOpts...).Process(actualPlan)
	return actualPlan
}

func walkFetches(node *resolve.FetchTreeNode, fn func(resolve.Fetch)) {
	if node == nil {
		return
	}
	if node.Item != nil && node.Item.Fetch != nil {
		fn(node.Item.Fetch)
	}
	for _, child := range node.ChildNodes {
		walkFetches(child, fn)
	}
}

func collectMultiEntityFetches(node *resolve.FetchTreeNode) []*resolve.MultiEntityFetch {
	var out []*resolve.MultiEntityFetch
	walkFetches(node, func(f resolve.Fetch) {
		if m, ok := f.(*resolve.MultiEntityFetch); ok {
			out = append(out, m)
		}
	})
	return out
}

// entityFetchCount counts entity fetches (single + batch) targeting the given
// subgraph in a fully post-processed tree, where such fetches are concrete
// EntityFetch/BatchEntityFetch nodes.
func entityFetchCount(node *resolve.FetchTreeNode, subgraphName string) int {
	count := 0
	walkFetches(node, func(f resolve.Fetch) {
		switch e := f.(type) {
		case *resolve.EntityFetch:
			if e.Info != nil && e.Info.DataSourceName == subgraphName {
				count++
			}
		case *resolve.BatchEntityFetch:
			if e.Info != nil && e.Info.DataSourceName == subgraphName {
				count++
			}
		}
	})
	return count
}

func multiFetchDefinition() string {
	return `
		type Query {
			employees: [Employee]
			employee: Employee
		}
		type Employee {
			id: ID!
			products: [String]
			notes: String
		}`
}

func multiFetchPlanConfig(t *testing.T, enableMultiFetch bool) plan.Configuration {
	accountsSDL := `
		type Query {
			employees: [Employee]
			employee: Employee
		}
		type Employee @key(fields: "id") {
			id: ID!
		}`
	productsSDL := `
		type Employee @key(fields: "id") {
			id: ID!
			products: [String]
			notes: String
		}`

	accounts := mustDataSourceConfiguration(t, "accounts",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"employees", "employee"}},
				{TypeName: "Employee", FieldNames: []string{"id"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: []plan.FederationFieldConfiguration{{TypeName: "Employee", SelectionSet: "id"}},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: "http://accounts"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: accountsSDL}, accountsSDL),
		}))

	products := mustDataSourceConfiguration(t, "products",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{{TypeName: "Employee", FieldNames: []string{"id", "products", "notes"}}},
			FederationMetaData: plan.FederationMetaData{
				Keys: []plan.FederationFieldConfiguration{{TypeName: "Employee", SelectionSet: "id"}},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: "http://products"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: productsSDL}, productsSDL),
		}))

	return plan.Configuration{
		DataSources:                  []plan.DataSource{accounts, products},
		DisableResolveFieldPositions: true,
		EnableMultiFetch:             enableMultiFetch,
	}
}

func TestGraphQLDataSourceFederation_MultiFetch(t *testing.T) {
	operation := `{ employees { id products } employee { id notes } }`

	t.Run("two same-wave entity fetches merge", func(t *testing.T) {
		syncPlan := planAndPostprocessMultiFetch(t,
			multiFetchDefinition(), operation, "",
			multiFetchPlanConfig(t, true),
			postprocess.EnableMultiFetch(),
		).(*plan.SynchronousResponsePlan)

		multis := collectMultiEntityFetches(syncPlan.Response.Fetches)
		require.Len(t, multis, 1, "expected exactly one MultiEntity fetch")
		multi := multis[0]

		assert.Equal(t, "products", multi.Info.DataSourceName)
		assert.Equal(t, []int{1, 2}, multi.MergedFetchIDs)
		assert.Zero(t, entityFetchCount(syncPlan.Response.Fetches, "products"),
			"no standalone entity fetches remain after merge")

		query := multi.Info.QueryPlan.Query
		assert.Contains(t, query, `f1: _entities(representations: $representations_f1)@include(if: $includeF1)`)
		assert.Contains(t, query, `f2: _entities(representations: $representations_f2)@include(if: $includeF2)`)

		pretty := syncPlan.Response.Fetches.QueryPlan().PrettyPrint()
		assert.Contains(t, pretty, `Fetch(service: "products")`)
		assert.Contains(t, pretty, `f1: _entities(representations: $representations_f1)@include(if: $includeF1)`)
		assert.Contains(t, pretty, `f2: _entities(representations: $representations_f2)@include(if: $includeF2)`)
	})

	t.Run("flag off keeps two separate entity fetches", func(t *testing.T) {
		syncPlan := planAndPostprocessMultiFetch(t,
			multiFetchDefinition(), operation, "",
			multiFetchPlanConfig(t, false),
		).(*plan.SynchronousResponsePlan)

		assert.Empty(t, collectMultiEntityFetches(syncPlan.Response.Fetches))
		assert.Equal(t, 2, entityFetchCount(syncPlan.Response.Fetches, "products"),
			"both entity fetches remain unmerged with the flag off")
	})
}

func TestGraphQLDataSourceFederation_MultiFetch_WaveSeparation(t *testing.T) {
	// products is fetched twice: Employee.upc via key id in wave 1, and
	// Manager.title via key mid in wave 2 (the manager key comes from the org
	// subgraph). The two products fetches are in different waves and must not merge.
	accountsSDL := `
		type Query { employee: Employee }
		type Employee @key(fields: "id") { id: ID! }`
	orgSDL := `
		type Employee @key(fields: "id") { id: ID! manager: Manager }
		type Manager @key(fields: "mid") { mid: ID! }`
	productsSDL := `
		type Employee @key(fields: "id") { id: ID! upc: String }
		type Manager @key(fields: "mid") { mid: ID! title: String }`
	def := `
		type Query { employee: Employee }
		type Employee { id: ID! upc: String manager: Manager }
		type Manager { mid: ID! title: String }`

	accounts := mustDataSourceConfiguration(t, "accounts",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"employee"}},
				{TypeName: "Employee", FieldNames: []string{"id"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: []plan.FederationFieldConfiguration{{TypeName: "Employee", SelectionSet: "id"}},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: "http://accounts"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: accountsSDL}, accountsSDL),
		}))
	org := mustDataSourceConfiguration(t, "org",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Employee", FieldNames: []string{"id", "manager"}},
				{TypeName: "Manager", FieldNames: []string{"mid"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: []plan.FederationFieldConfiguration{
					{TypeName: "Employee", SelectionSet: "id"},
					{TypeName: "Manager", SelectionSet: "mid"},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: "http://org"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: orgSDL}, orgSDL),
		}))
	products := mustDataSourceConfiguration(t, "products",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Employee", FieldNames: []string{"id", "upc"}},
				{TypeName: "Manager", FieldNames: []string{"mid", "title"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: []plan.FederationFieldConfiguration{
					{TypeName: "Employee", SelectionSet: "id"},
					{TypeName: "Manager", SelectionSet: "mid"},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: "http://products"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: productsSDL}, productsSDL),
		}))

	config := plan.Configuration{
		DataSources:                  []plan.DataSource{accounts, org, products},
		DisableResolveFieldPositions: true,
		EnableMultiFetch:             true,
	}

	syncPlan := planAndPostprocessMultiFetch(t, def,
		`{ employee { upc manager { title } } }`, "", config,
		postprocess.EnableMultiFetch(),
	).(*plan.SynchronousResponsePlan)

	assert.Empty(t, collectMultiEntityFetches(syncPlan.Response.Fetches),
		"different-wave products fetches must not merge")
	assert.Equal(t, 2, entityFetchCount(syncPlan.Response.Fetches, "products"),
		"both products fetches remain separate")
}

func TestGraphQLDataSourceFederation_MultiFetch_ThreeFetchGroup(t *testing.T) {
	accountsSDL := `
		type Query {
			employees: [Employee]
			employee: Employee
			contractors: [Employee]
		}
		type Employee @key(fields: "id") { id: ID! }`
	productsSDL := `
		type Employee @key(fields: "id") {
			id: ID!
			products: [String]
			notes: String
		}`
	def := `
		type Query {
			employees: [Employee]
			employee: Employee
			contractors: [Employee]
		}
		type Employee { id: ID! products: [String] notes: String }`

	accounts := mustDataSourceConfiguration(t, "accounts",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"employees", "employee", "contractors"}},
				{TypeName: "Employee", FieldNames: []string{"id"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: []plan.FederationFieldConfiguration{{TypeName: "Employee", SelectionSet: "id"}},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: "http://accounts"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: accountsSDL}, accountsSDL),
		}))
	products := mustDataSourceConfiguration(t, "products",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{{TypeName: "Employee", FieldNames: []string{"id", "products", "notes"}}},
			FederationMetaData: plan.FederationMetaData{
				Keys: []plan.FederationFieldConfiguration{{TypeName: "Employee", SelectionSet: "id"}},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: "http://products"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: productsSDL}, productsSDL),
		}))

	config := plan.Configuration{
		DataSources:                  []plan.DataSource{accounts, products},
		DisableResolveFieldPositions: true,
		EnableMultiFetch:             true,
	}

	syncPlan := planAndPostprocessMultiFetch(t, def,
		`{ employees { id products } employee { id notes } contractors { id products } }`,
		"", config, postprocess.EnableMultiFetch(),
	).(*plan.SynchronousResponsePlan)

	multis := collectMultiEntityFetches(syncPlan.Response.Fetches)
	require.Len(t, multis, 1)
	multi := multis[0]
	assert.Len(t, multi.MergedFetchIDs, 3)
	assert.Zero(t, entityFetchCount(syncPlan.Response.Fetches, "products"))

	query := multi.Info.QueryPlan.Query
	assert.Contains(t, query, `f1: _entities(representations: $representations_f1)@include(if: $includeF1)`)
	assert.Contains(t, query, `f2: _entities(representations: $representations_f2)@include(if: $includeF2)`)
	assert.Contains(t, query, `f3: _entities(representations: $representations_f3)@include(if: $includeF3)`)
	assert.Contains(t, query, `$includeF3: Boolean!`)
}

func TestGraphQLDataSourceFederation_MultiFetch_Subscription(t *testing.T) {
	// The subscription trigger is not part of the response fetch tree, so the two
	// products entity fetches merge only because they share an in-tree provider:
	// the hub fetch that resolves the Update payload's employees/employee.
	accountsSDL := `
		type Query { _dummy: String }
		type Subscription { update: Update }
		type Update @key(fields: "id") { id: ID! }`
	hubSDL := `
		type Update @key(fields: "id") { id: ID! employees: [Employee] employee: Employee }
		type Employee @key(fields: "id") { id: ID! }`
	productsSDL := `
		type Employee @key(fields: "id") {
			id: ID!
			products: [String]
			notes: String
		}`
	def := `
		type Query { _dummy: String }
		type Subscription { update: Update }
		type Update { id: ID! employees: [Employee] employee: Employee }
		type Employee { id: ID! products: [String] notes: String }`

	accounts := mustDataSourceConfiguration(t, "accounts",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"_dummy"}},
				{TypeName: "Subscription", FieldNames: []string{"update"}},
				{TypeName: "Update", FieldNames: []string{"id"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: []plan.FederationFieldConfiguration{{TypeName: "Update", SelectionSet: "id"}},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: "http://accounts"},
			Subscription:        &SubscriptionConfiguration{URL: "ws://accounts"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: accountsSDL}, accountsSDL),
		}))
	hub := mustDataSourceConfiguration(t, "hub",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Update", FieldNames: []string{"id", "employees", "employee"}},
				{TypeName: "Employee", FieldNames: []string{"id"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: []plan.FederationFieldConfiguration{
					{TypeName: "Update", SelectionSet: "id"},
					{TypeName: "Employee", SelectionSet: "id"},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: "http://hub"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: hubSDL}, hubSDL),
		}))
	products := mustDataSourceConfiguration(t, "products",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{{TypeName: "Employee", FieldNames: []string{"id", "products", "notes"}}},
			FederationMetaData: plan.FederationMetaData{
				Keys: []plan.FederationFieldConfiguration{{TypeName: "Employee", SelectionSet: "id"}},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: "http://products"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: productsSDL}, productsSDL),
		}))

	config := plan.Configuration{
		DataSources:                  []plan.DataSource{accounts, hub, products},
		DisableResolveFieldPositions: true,
		EnableMultiFetch:             true,
	}

	subPlan := planAndPostprocessMultiFetch(t, def,
		`subscription { update { employees { id products } employee { id notes } } }`,
		"", config, postprocess.EnableMultiFetch(),
	).(*plan.SubscriptionResponsePlan)

	fetches := subPlan.Response.Response.Fetches
	multis := collectMultiEntityFetches(fetches)
	require.Len(t, multis, 1, "the subscription response tree merges its same-wave products fetches")
	multi := multis[0]
	assert.Equal(t, "products", multi.Info.DataSourceName)
	assert.Zero(t, entityFetchCount(fetches, "products"))

	query := multi.Info.QueryPlan.Query
	assert.Contains(t, query, `f1: _entities(representations: $representations_f1)@include(if: $includeF1)`)
	assert.Contains(t, query, `f2: _entities(representations: $representations_f2)@include(if: $includeF2)`)
}
