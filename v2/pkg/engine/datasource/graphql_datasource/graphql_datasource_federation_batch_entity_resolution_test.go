package graphql_datasource

// End-to-end plan-level tests for batch entity resolution requests to the same subgraph
// within a query.
//
// Goal: when a single operation needs to resolve entities from
// the *same* subgraph at multiple points in the query plan, the router must
// merge those `_entities` fetches into a *single* upstream request (using
// aliased `_entities` when the selections/types differ) instead of issuing one
// HTTP request per fetch.
//
// These tests are intentionally design-agnostic: they do NOT assert the exact
// shape of the merged fetch node. Instead they assert the
// only property the acceptance criteria actually care about — the number of
// upstream requests made to each subgraph. `countRequestsPerSubgraph` walks the
// post-processed fetch tree and counts request-producing fetch nodes per
// subgraph.
//
// TDD status:
//   - The "merge" cases (expectMerge: true) are RED today: the planner emits N
//     separate fetches to the shared subgraph. They turn GREEN once the merge
//     step is implemented AND wired into `router62TestProcessor` below.
//   - The "guardrail" cases (expectMerge: false) are GREEN today and must STAY
//     green — they guard against over-merging (merging across subgraphs or
//     wrapping a lone fetch).
//
// When you implement the merge step: if it runs by default in
// postprocess.NewProcessor() you need change nothing here; if it is gated
// behind a ProcessorOption, add that option in `router62TestProcessor`.

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

const (
	router62AccountsService = "accounts.service" // root: employees / firstEmployee / store; owns Employee & Store keys
	router62ProductsService = "products.service" // extends Employee (products, skillA, skillB) and Store (reviewScore)
	router62ReviewsService  = "reviews.service"  // extends Employee (rating)
)

func TestGraphQLDataSourceFederation_BatchEntityResolution_ROUTER62(t *testing.T) {
	// Supergraph schema (what the client sees).
	definition := `
		type Query {
			employees: [Employee!]!
			firstEmployee: Employee
			store: Store
		}
		type Employee {
			id: ID!
			name: String!
			products: [String!]!
			skillA: String!
			skillB: String!
			rating: Int!
		}
		type Store {
			id: ID!
			location: String!
			reviewScore: Int!
		}
	`

	// Subgraph A: accounts — root entry points + owns the Employee/Store keys.
	accountsSDL := `
		type Query {
			employees: [Employee!]!
			firstEmployee: Employee
			store: Store
		}
		type Employee @key(fields: "id") {
			id: ID!
			name: String!
		}
		type Store @key(fields: "id") {
			id: ID!
			location: String!
		}
	`
	accountsDS := mustDataSourceConfiguration(t,
		router62AccountsService,
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"employees", "firstEmployee", "store"}},
				{TypeName: "Employee", FieldNames: []string{"id", "name"}},
				{TypeName: "Store", FieldNames: []string{"id", "location"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Employee", SelectionSet: "id"},
					{TypeName: "Store", SelectionSet: "id"},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: "http://" + router62AccountsService},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: accountsSDL}, accountsSDL),
		}),
	)

	// Subgraph B: products — extends Employee and Store. This is the "shared"
	// provider subgraph the feature must not call more than once.
	productsSDL := `
		type Employee @key(fields: "id") {
			id: ID!
			products: [String!]!
			skillA: String!
			skillB: String!
		}
		type Store @key(fields: "id") {
			id: ID!
			reviewScore: Int!
		}
	`
	productsDS := mustDataSourceConfiguration(t,
		router62ProductsService,
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Employee", FieldNames: []string{"id", "products", "skillA", "skillB"}},
				{TypeName: "Store", FieldNames: []string{"id", "reviewScore"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Employee", SelectionSet: "id"},
					{TypeName: "Store", SelectionSet: "id"},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: "http://" + router62ProductsService},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: productsSDL}, productsSDL),
		}),
	)

	// Subgraph C: reviews — extends Employee with `rating`. Used to prove the
	// merge does NOT reach across subgraphs.
	reviewsSDL := `
		type Employee @key(fields: "id") {
			id: ID!
			rating: Int!
		}
	`
	reviewsDS := mustDataSourceConfiguration(t,
		router62ReviewsService,
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Employee", FieldNames: []string{"id", "rating"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Employee", SelectionSet: "id"},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: "http://" + router62ReviewsService},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: reviewsSDL}, reviewsSDL),
		}),
	)

	planConfiguration := plan.Configuration{
		DataSources:                  []plan.DataSource{accountsDS, productsDS, reviewsDS},
		DisableResolveFieldPositions: true,
	}

	cases := []struct {
		name          string
		operation     string
		operationName string
		variables     string
		// wantRequests is the DESIRED number of upstream requests per subgraph
		// once ROUTER-62 is implemented.
		wantRequests map[string]int
		// expectMerge documents whether this case is currently RED (a merge that
		// is not yet implemented) or a GREEN guardrail.
		expectMerge bool
		note        string
	}{
		{
			name: "list and single object to same subgraph are merged",
			// employees.products -> BatchEntityFetch(products); employee.products
			// -> EntityFetch(products). Two fetches to products today, must be one.
			operation: `
				query ListAndSingle {
					employees { id products }
					firstEmployee { id products }
				}`,
			operationName: "ListAndSingle",
			wantRequests: map[string]int{
				router62AccountsService: 1,
				router62ProductsService: 1,
			},
			expectMerge: true,
			note:        "AC1 + gotcha #1: mix of BatchEntityFetch (list) and EntityFetch (object) on the same subgraph.",
		},
		{
			name: "different entity types on same subgraph are merged via aliased _entities",
			// employee.products (Employee) + store.reviewScore (Store), both on
			// products subgraph -> one request with two aliased _entities.
			operation: `
				query CrossType {
					firstEmployee { products }
					store { reviewScore }
				}`,
			operationName: "CrossType",
			wantRequests: map[string]int{
				router62AccountsService: 1,
				router62ProductsService: 1,
			},
			expectMerge: true,
			note:        "AC2: two different entity types (Employee, Store) from one subgraph.",
		},
		{
			name: "aliased same field with different selections is merged (no directive)",
			// Proves @include is incidental: plain aliases with differing
			// selections already trigger duplicate provider calls.
			operation: `
				query AliasedFields {
					a: firstEmployee { skillA }
					b: firstEmployee { skillB }
				}`,
			operationName: "AliasedFields",
			wantRequests: map[string]int{
				router62AccountsService: 1,
				router62ProductsService: 1,
			},
			expectMerge: true,
			note:        "eBay-shaped duplication without @include; same type, disjoint field subsets, different response paths.",
		},
		{
			name: "aliased same field guarded by @include is merged when both branches active",
			operation: `
				query IncludeAliases($withA: Boolean!, $withB: Boolean!) {
					a: firstEmployee @include(if: $withA) { skillA }
					b: firstEmployee @include(if: $withB) { skillB }
				}`,
			operationName: "IncludeAliases",
			variables:     `{"withA":true,"withB":true}`,
			wantRequests: map[string]int{
				router62AccountsService: 1,
				router62ProductsService: 1,
			},
			expectMerge: true,
			note:        "The exact eBay report (cosmo#2900): aliased @include branches on the same provider subgraph.",
		},
		{
			name: "three fetches to same subgraph are merged into one",
			// employees.products (batch), employee.skillA (single Employee),
			// store.reviewScore (single Store) -> one products request.
			operation: `
				query ThreeWay {
					employees { products }
					firstEmployee { skillA }
					store { reviewScore }
				}`,
			operationName: "ThreeWay",
			wantRequests: map[string]int{
				router62AccountsService: 1,
				router62ProductsService: 1,
			},
			expectMerge: true,
			note:        "N-way (3) merge, not just pairwise; mixes list/object and two entity types.",
		},
		{
			name: "guardrail: fetches to different subgraphs are NOT merged",
			operation: `
				query DifferentSubgraphs {
					firstEmployee { products rating }
				}`,
			operationName: "DifferentSubgraphs",
			wantRequests: map[string]int{
				router62AccountsService: 1,
				router62ProductsService: 1,
				router62ReviewsService:  1,
			},
			expectMerge: false,
			note:        "products (B) and rating (C) must remain separate requests.",
		},
		{
			name: "guardrail: a single provider fetch is left untouched",
			operation: `
				query SingleProviderFetch {
					firstEmployee { products }
				}`,
			operationName: "SingleProviderFetch",
			wantRequests: map[string]int{
				router62AccountsService: 1,
				router62ProductsService: 1,
			},
			expectMerge: false,
			note:        "Nothing to merge: the merge step must be a no-op (no pointless wrapper node).",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fetches := planFederationFetchTree(t, definition, tc.operation, tc.operationName, tc.variables, planConfiguration)
			got := countRequestsPerSubgraph(fetches)

			if !assert.Equal(t, tc.wantRequests, got, tc.note) {
				t.Logf("query plan:\n%s", fetches.QueryPlan().PrettyPrint())
				if tc.expectMerge {
					t.Log("^ EXPECTED until ROUTER-62 merging is implemented: the shared subgraph is still called more than once.")
				}
			}
		})
	}
}

// planFederationFetchTree runs the same plan+post-process pipeline the router
// uses and returns the resulting fetch tree. Fetch info is enabled so each
// fetch can be attributed to its subgraph.
func planFederationFetchTree(t *testing.T, definition, operation, operationName, variables string, config plan.Configuration) *resolve.FetchTreeNode {
	t.Helper()

	def := unsafeparser.ParseGraphqlDocumentString(definition)
	op := unsafeparser.ParseGraphqlDocumentString(operation)
	if variables != "" {
		op.Input.Variables = []byte(variables)
	}

	require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&def))

	norm := astnormalization.NewWithOpts(
		astnormalization.WithExtractVariables(),
		astnormalization.WithInlineFragmentSpreads(),
		astnormalization.WithRemoveFragmentDefinitions(),
		astnormalization.WithRemoveUnusedVariables(),
	)
	var report operationreport.Report
	norm.NormalizeOperation(&op, &def, &report)
	require.Falsef(t, report.HasErrors(), "normalization: %s", report.Error())

	astvalidation.DefaultOperationValidator().Validate(&op, &def, &report)
	require.Falsef(t, report.HasErrors(), "validation: %s", report.Error())

	// Fetch info carries the subgraph name we count on.
	config.DisableIncludeInfo = false

	p, err := plan.NewPlanner(config)
	require.NoError(t, err)

	rawPlan := p.Plan(&op, &def, operationName, &report)
	require.Falsef(t, report.HasErrors(), "planning: %s", report.Error())

	router62TestProcessor().Process(rawPlan)

	syncPlan, ok := rawPlan.(*plan.SynchronousResponsePlan)
	require.Truef(t, ok, "expected *plan.SynchronousResponsePlan, got %T", rawPlan)
	return syncPlan.Response.Fetches
}

// router62TestProcessor is the single edit point for wiring the merge step.
// When ROUTER-62 merging is implemented, ensure it runs here — either it is on
// by default (no change needed) or pass its ProcessorOption.
func router62TestProcessor() *postprocess.Processor {
	return postprocess.NewProcessor()
}

// countRequestsPerSubgraph returns the number of upstream requests the plan
// makes to each subgraph, keyed by subgraph name. This is the observable AC1
// property: after merging, the shared subgraph count must be 1.
func countRequestsPerSubgraph(root *resolve.FetchTreeNode) map[string]int {
	counts := map[string]int{}

	var walk func(n *resolve.FetchTreeNode)
	walk = func(n *resolve.FetchTreeNode) {
		if n == nil {
			return
		}
		switch n.Kind {
		case resolve.FetchTreeNodeKindSequence, resolve.FetchTreeNodeKindParallel:
			for _, c := range n.ChildNodes {
				walk(c)
			}
		case resolve.FetchTreeNodeKindSingle:
			if name := fetchSubgraphName(n.Item); name != "" {
				counts[name]++
			}
		default:
			// Defensive: a future merged/multi container kind counts as ONE
			// request per distinct subgraph among its members (a single merged
			// request), rather than one per child.
			if name := fetchSubgraphName(n.Item); name != "" {
				counts[name]++
				return
			}
			seen := map[string]bool{}
			for _, c := range n.ChildNodes {
				if name := fetchSubgraphName(c.Item); name != "" && !seen[name] {
					counts[name]++
					seen[name] = true
				}
			}
		}
	}
	walk(root)
	return counts
}

func fetchSubgraphName(item *resolve.FetchItem) string {
	if item == nil || item.Fetch == nil {
		return ""
	}
	if info := item.Fetch.FetchInfo(); info != nil {
		return info.DataSourceName
	}
	return ""
}
