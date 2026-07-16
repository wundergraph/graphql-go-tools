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
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
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

// router62Fixture builds the shared 3-subgraph federation setup (accounts /
// products / reviews) used by every ROUTER-62 test. products is the "shared"
// provider subgraph the merge feature must not call more than once.
func router62Fixture(t *testing.T) (definition string, config plan.Configuration) {
	t.Helper()

	// Supergraph schema (what the client sees).
	definition = `
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

	return definition, plan.Configuration{
		DataSources:                  []plan.DataSource{accountsDS, productsDS, reviewsDS},
		DisableResolveFieldPositions: true,
	}
}

func TestGraphQLDataSourceFederation_BatchEntityResolution_ROUTER62(t *testing.T) {
	definition, planConfiguration := router62Fixture(t)

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

// planFederationSyncPlan runs the same plan+post-process pipeline the router
// uses and returns the full synchronous response plan. Fetch info is enabled so
// each fetch can be attributed to its subgraph.
func planFederationSyncPlan(t *testing.T, definition, operation, operationName, variables string, config plan.Configuration) *plan.SynchronousResponsePlan {
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
	return syncPlan
}

// planFederationFetchTree returns just the post-processed fetch tree, for the
// node-counting tests.
func planFederationFetchTree(t *testing.T, definition, operation, operationName, variables string, config plan.Configuration) *resolve.FetchTreeNode {
	t.Helper()
	return planFederationSyncPlan(t, definition, operation, operationName, variables, config).Response.Fetches
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

// ROUTER-62 — end-to-end guard for the merged-request ASSEMBLY.
//
// The node-counting tests above (countRequestsPerSubgraph) prove the planner
// DECIDES to merge, but a post-processor that only collapses node structure
// (correct FetchInfo, rewired dependency IDs) would satisfy them without ever
// rendering a merged request. The resolve-layer tests
// (resolve_federation_batch_entity_resolution_test.go) prove the loader can
// demultiplex a merged node, but they feed it HAND-AUTHORED request bytes.
//
// Nothing connects the two: the risky part — assembling ONE aliased _entities
// document (f1:/f2:) with isolated per-alias representation variables out of N
// independently-planned _entities query strings (each baked as static text in
// FetchConfiguration.Input) — is unguarded. This test closes that seam. It
// plans + post-processes a real operation, then EXECUTES the planner-produced
// tree through the resolver with each subgraph's DataSource swapped for a
// recorder, and asserts the exact request bytes the shared subgraph receives.
//
// RED until both the merge post-processor and the loader's FetchKindMultiEntity
// handling exist: today products gets two separate _entities requests, so the
// "exactly one request" assertion fails. Green only when the assembled bytes
// are byte-correct — the guarantee node-counting cannot give.
func TestGraphQLDataSourceFederation_BatchEntityResolution_ROUTER62_MergedRequestBytes(t *testing.T) {
	definition, planConfiguration := router62Fixture(t)

	// Cross-type: firstEmployee.products (Employee) + store.reviewScore (Store),
	// both resolved by products -> must become ONE aliased _entities request.
	const operation = `query CrossType { firstEmployee { products } store { reviewScore } }`

	// Canned per-subgraph responses. accounts is the root fetch (plain data);
	// products is the merged, aliased shape the single request must return.
	cannedResponses := map[string]string{
		router62AccountsService: `{"data":{"firstEmployee":{"__typename":"Employee","id":"1"},"store":{"__typename":"Store","id":"s1"}}}`,
		router62ProductsService: `{"data":{"f1":[{"__typename":"Employee","products":["p1","p2"]}],"f2":[{"__typename":"Store","reviewScore":5}]}}`,
	}

	output, requestsBySubgraph := resolveFederationPlan(t, definition, operation, "CrossType", "", planConfiguration, cannedResponses)

	// TARGET (best-guess) merged request the products subgraph must receive: one
	// aliased _entities document with isolated representation variables. The
	// alias/variable naming (f1 / representations_f1) is a guess — adjust to the
	// scheme the merge step emits. The INVARIANTS are the request COUNT (one) and
	// the resolved OUTPUT below, plus that the single request is one aliased
	// _entities document covering BOTH selections.
	// (Per-fragment __typename mirrors the real planned selection sets the harness
	// captures for the un-merged case: `... on Employee {__typename products}`.)
	const wantMergedRequest = `{"method":"POST","url":"http://products.service","body":{` +
		`"query":"query($representations_f1: [_Any!]!, $representations_f2: [_Any!]!){` +
		`f1: _entities(representations: $representations_f1){... on Employee {__typename products}} ` +
		`f2: _entities(representations: $representations_f2){... on Store {__typename reviewScore}}}",` +
		`"variables":{` +
		`"representations_f1":[{"__typename":"Employee","id":"1"}],` +
		`"representations_f2":[{"__typename":"Store","id":"s1"}]}}}`

	const wantOutput = `{"data":{"firstEmployee":{"products":["p1","p2"]},"store":{"reviewScore":5}}}`

	productsRequests := requestsBySubgraph[router62ProductsService]

	// (1) Design-agnostic invariant: AC1 measured at the WIRE, not by counting
	// nodes. This is what the count-only tests cannot see and what fails RED today.
	require.Lenf(t, productsRequests, 1,
		"products must receive exactly ONE merged request; got %d:\n%v",
		len(productsRequests), productsRequests)

	// (2) The precise query-assembly guard: the single request's bytes.
	assert.Equal(t, wantMergedRequest, productsRequests[0],
		"the merged request must be one aliased _entities document with isolated per-alias variables")

	// (3) End-to-end demultiplex correctness.
	assert.Equal(t, wantOutput, output)
}

// resolveFederationPlan plans + post-processes the operation exactly like
// planFederationFetchTree, then EXECUTES the planner-produced response tree
// through the resolver. Every request-producing fetch has its DataSource
// swapped for a recorder, so the exact upstream request bytes the loader renders
// are captured per subgraph (no HTTP, deterministic). Returns the resolved
// client output plus, keyed by subgraph name, every request body that subgraph
// received.
//
// This is the only harness that connects the post-processor's output to the
// loader's input — the assembly of the request is done by the loader rendering
// the post-processor's InputTemplate, which is exactly what this captures.
func resolveFederationPlan(t *testing.T, definition, operation, operationName, variables string, config plan.Configuration, cannedResponses map[string]string) (output string, requestsBySubgraph map[string][]string) {
	t.Helper()

	syncPlan := planFederationSyncPlan(t, definition, operation, operationName, variables, config)

	rec := &recordingDataSources{responses: cannedResponses, requests: map[string][]string{}}
	swapFetchDataSources(t, syncPlan.Response.Fetches, rec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := resolve.New(ctx, resolve.ResolverOptions{
		MaxConcurrency:          1024,
		PropagateSubgraphErrors: true,
		AsyncErrorWriter:        noopAsyncErrorWriter{},
	})

	buf := &bytes.Buffer{}
	_, err := r.ResolveGraphQLResponse(resolve.NewContext(ctx), syncPlan.Response, nil, buf)
	// Non-fatal: in the RED state the entity fetches receive a merged-shaped
	// response they cannot parse, which surfaces as field errors, not a Go error.
	// Keep going so the request-count assertion (the intended RED signal) is reached.
	assert.NoError(t, err)

	return buf.String(), rec.snapshot()
}

// swapFetchDataSources replaces the DataSource of every request-producing fetch
// in the tree with a recorder for that fetch's subgraph.
func swapFetchDataSources(t *testing.T, root *resolve.FetchTreeNode, rec *recordingDataSources) {
	t.Helper()
	var walk func(n *resolve.FetchTreeNode)
	walk = func(n *resolve.FetchTreeNode) {
		if n == nil {
			return
		}
		if n.Item != nil && n.Item.Fetch != nil {
			setFetchDataSource(t, n.Item.Fetch, rec.dataSourceFor(fetchSubgraphName(n.Item)))
		}
		for _, c := range n.ChildNodes {
			walk(c)
		}
	}
	walk(root)
}

// setFetchDataSource points a fetch at the given DataSource. The field location
// differs per fetch kind. An unhandled kind is fatal so a future request-
// producing node can't silently escape recording (and skew the request count).
func setFetchDataSource(t *testing.T, fetch resolve.Fetch, ds resolve.DataSource) {
	t.Helper()
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		f.FetchConfiguration.DataSource = ds
	case *resolve.EntityFetch:
		f.DataSource = ds
	case *resolve.BatchEntityFetch:
		f.DataSource = ds
	case *resolve.MultiEntityFetch:
		f.DataSource = ds
	default:
		t.Fatalf("router62: unhandled fetch kind %T in resolveFederationPlan; add a DataSource-swap case", fetch)
	}
}

// recordingDataSources hands out per-subgraph recorders and collects every
// request body each subgraph receives. Concurrency-safe: parallel fetch nodes
// call Load concurrently.
type recordingDataSources struct {
	mu        sync.Mutex
	responses map[string]string
	requests  map[string][]string
}

func (r *recordingDataSources) dataSourceFor(name string) resolve.DataSource {
	return &recordingDataSource{parent: r, name: name}
}

func (r *recordingDataSources) record(name string, input []byte) []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests[name] = append(r.requests[name], string(input))
	return []byte(r.responses[name])
}

func (r *recordingDataSources) snapshot() map[string][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string][]string, len(r.requests))
	for k, v := range r.requests {
		out[k] = append([]string(nil), v...)
	}
	return out
}

type recordingDataSource struct {
	parent *recordingDataSources
	name   string
}

func (d *recordingDataSource) Load(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
	return d.parent.record(d.name, input), nil
}

func (d *recordingDataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) ([]byte, error) {
	return d.parent.record(d.name, input), nil
}

// noopAsyncErrorWriter satisfies resolve.ResolverOptions.AsyncErrorWriter for
// the synchronous execution path exercised here.
type noopAsyncErrorWriter struct{}

func (noopAsyncErrorWriter) WriteError(ctx *resolve.Context, err error, res *resolve.GraphQLResponse, w io.Writer) {
}
