package resolve

// Resolve/loader-layer tests for batch entity resolution to the same subgraph.
//
// The plan-layer tests (graphql_datasource_federation_batch_entity_resolution_test.go)
// prove the planner DECIDES to merge. These tests cover the runtime half that
// devsergiy's comment on cosmo#1300 calls out as the hard part — the behavior a
// fetch-count assertion cannot see:
//
//   #2 multiplex/demultiplex — gather representations from several response
//      paths into one aliased request, then split the aliased response back to
//      each path.
//   #3 per-list-item null representations must be filtered from the batch and
//      results still mapped back to the correct (non-null) items.
//   #4 merge queries of the same/different types with different field selections
//      and aliases into one correctly-formed request.
//
// Each MultiEntityFetch tree renders the exact target wire contract: a shared
// request envelope (Header/Footer) plus, per aliased sub-fetch, an isolated
// representation variable rendered from that sub-fetch's FetchPath (repRenderer).
// mockedDS byte-matches the rendered request (Times(1)) and the resolved OUTPUT is
// asserted via testFn.

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
)

// ROUTER-62 #2 + #4 — multiplex/demultiplex across DIFFERENT entity types.
//
// employee.products (Employee) and store.reviewScore (Store) are both resolved
// by the products subgraph. They must be merged into ONE request with two
// aliased _entities (distinct inline fragments + isolated representation
// variables), and the aliased response demultiplexed back onto employee.products
// and store.reviewScore respectively.
func TestResolveGraphQLResponse_ROUTER62_MultiplexDemux_DifferentTypes(t *testing.T) {
	testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		// Root fetch to the accounts subgraph yields the entity keys for both paths.
		const rootResponse = `{"data":{"employee":{"__typename":"Employee","id":"1"},"store":{"__typename":"Store","id":"s1"}}}`

		// TARGET: a single request to products carrying two aliased _entities.
		const mergedRequest = `{"method":"POST","url":"http://products.service","body":{` +
			`"query":"query($representations_f1: [_Any!]!, $representations_f2: [_Any!]!){` +
			`f1: _entities(representations: $representations_f1){... on Employee {products}} ` +
			`f2: _entities(representations: $representations_f2){... on Store {reviewScore}}}",` +
			`"variables":{` +
			`"representations_f1":[{"__typename":"Employee","id":"1"}],` +
			`"representations_f2":[{"__typename":"Store","id":"s1"}]}}}`

		// TARGET: the aliased response the loader must demultiplex.
		const mergedResponse = `{"data":{` +
			`"f1":[{"__typename":"Employee","products":["p1","p2"]}],` +
			`"f2":[{"__typename":"Store","reviewScore":5}]}}`

		// Each alias's data must land on its own response path.
		const want = `{"data":{"employee":{"products":["p1","p2"]},"store":{"reviewScore":5}}}`

		const header = `{"method":"POST","url":"http://products.service","body":{` +
			`"query":"query($representations_f1: [_Any!]!, $representations_f2: [_Any!]!){` +
			`f1: _entities(representations: $representations_f1){... on Employee {products}} ` +
			`f2: _entities(representations: $representations_f2){... on Store {reviewScore}}}",` +
			`"variables":{`

		return &GraphQLResponse{
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: FakeDataSource(rootResponse),
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Info: &FetchInfo{DataSourceID: "accounts", DataSourceName: "accounts"},
				}, "query"),
				SingleWithPath(&MultiEntityFetch{
					// One upstream request; Times(1) fails until the loader issues it.
					DataSource: mockedDS(t, ctrl, mergedRequest, mergedResponse),
					Header:     staticSegment(header),
					Footer:     staticSegment(`}}}`),
					Fetches: []*MultiEntitySubFetch{
						{
							Alias:        "f1",
							FetchPath:    []FetchItemPathElement{ObjectPath("employee")},
							ResponsePath: "query.employee",
							Batch:        false,
							Input: BatchInput{
								Header:    staticSegment(`"representations_f1":[`),
								Items:     []InputTemplate{repRenderer("Employee", "id")},
								Separator: staticSegment(`,`),
								Footer:    staticSegment(`]`),
							},
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "f1", "0"},
							},
						},
						{
							Alias:        "f2",
							FetchPath:    []FetchItemPathElement{ObjectPath("store")},
							ResponsePath: "query.store",
							Batch:        false,
							Input: BatchInput{
								Header:    staticSegment(`,"representations_f2":[`),
								Items:     []InputTemplate{repRenderer("Store", "id")},
								Separator: staticSegment(`,`),
								Footer:    staticSegment(`]`),
							},
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "f2", "0"},
							},
						},
					},
					Info: &FetchInfo{DataSourceID: "products", DataSourceName: "products"},
				}, "query"),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("employee"),
						Value: &Object{
							Path:     []string{"employee"},
							Nullable: true,
							Fields: []*Field{
								{
									Name: []byte("products"),
									Value: &Array{
										Path:     []string{"products"},
										Nullable: true,
										Item:     &String{},
									},
								},
							},
						},
					},
					{
						Name: []byte("store"),
						Value: &Object{
							Path:     []string{"store"},
							Nullable: true,
							Fields: []*Field{
								{
									Name:  []byte("reviewScore"),
									Value: &Integer{Path: []string{"reviewScore"}, Nullable: true},
								},
							},
						},
					},
				},
			},
		}, *NewContext(context.Background()), want
	})(t)
}

// ROUTER-62 #3 — per-list-item null representation filtering inside a merged batch.
//
// employees is a list; some elements resolve to a null entity representation
// (e.g. an @include branch that is inactive, or a null parent). Those items must
// be dropped from the outgoing batch, and the returned entities must map back to
// the correct NON-null positions — index alignment preserved. This is the exact
// behavior the abandoned PoC broke by discarding batchStats.
func TestResolveGraphQLResponse_ROUTER62_BatchNullRepresentationFiltering(t *testing.T) {
	testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		// employees[1] has no resolvable key -> its representation renders null.
		const rootResponse = `{"data":{"employees":[` +
			`{"__typename":"Employee","id":"1"},` +
			`null,` +
			`{"__typename":"Employee","id":"3"}]}}`

		// TARGET: the outgoing batch contains ONLY the two non-null representations.
		const mergedRequest = `{"method":"POST","url":"http://products.service","body":{` +
			`"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Employee {products}}}",` +
			`"variables":{"representations":[` +
			`{"__typename":"Employee","id":"1"},` +
			`{"__typename":"Employee","id":"3"}]}}}`

		// Two entities returned, aligned to the two representations that were sent.
		const mergedResponse = `{"data":{"_entities":[` +
			`{"__typename":"Employee","products":["p1"]},` +
			`{"__typename":"Employee","products":["p3"]}]}}`

		// Results scatter back to indices 0 and 2; the null item (index 1) stays null.
		const want = `{"data":{"employees":[` +
			`{"products":["p1"]},` +
			`null,` +
			`{"products":["p3"]}]}}`

		const header = `{"method":"POST","url":"http://products.service","body":{` +
			`"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Employee {products}}}",` +
			`"variables":{`

		return &GraphQLResponse{
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: FakeDataSource(rootResponse),
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Info: &FetchInfo{DataSourceID: "accounts", DataSourceName: "accounts"},
				}, "query"),
				SingleWithPath(&MultiEntityFetch{
					DataSource: mockedDS(t, ctrl, mergedRequest, mergedResponse),
					Header:     staticSegment(header),
					Footer:     staticSegment(`}}}`),
					Fetches: []*MultiEntitySubFetch{
						{
							FetchPath:    []FetchItemPathElement{ArrayPath("employees")},
							ResponsePath: "query.employees",
							Batch:        true,
							Input: BatchInput{
								Header:        staticSegment(`"representations":[`),
								Items:         []InputTemplate{repRenderer("Employee", "id")},
								Separator:     staticSegment(`,`),
								Footer:        staticSegment(`]`),
								SkipNullItems: true,
							},
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities"},
							},
						},
					},
					Info: &FetchInfo{DataSourceID: "products", DataSourceName: "products"},
				}, "query"),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("employees"),
						Value: &Array{
							Path:     []string{"employees"},
							Nullable: true,
							Item: &Object{
								Nullable: true,
								Fields: []*Field{
									{
										Name: []byte("products"),
										Value: &Array{
											Path:     []string{"products"},
											Nullable: true,
											Item:     &String{},
										},
									},
								},
							},
						},
					},
				},
			},
		}, *NewContext(context.Background()), want
	})(t)
}

// ROUTER-62 #4 — merge SAME type with different field selections + aliases.
//
// The eBay shape: `a: firstEmployee { skillA }` and `b: firstEmployee { skillB }`
// both resolve Employee fields from the products subgraph at different response
// paths. The merged request must alias the two _entities blocks and keep their
// representation variables isolated; the response demultiplexes back to a/b.
func TestResolveGraphQLResponse_ROUTER62_MergeSameTypeDifferentSelections(t *testing.T) {
	testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		const rootResponse = `{"data":{` +
			`"a":{"__typename":"Employee","id":"1"},` +
			`"b":{"__typename":"Employee","id":"1"}}}`

		// TARGET: one request, two aliased _entities selecting disjoint Employee fields.
		const mergedRequest = `{"method":"POST","url":"http://products.service","body":{` +
			`"query":"query($representations_f1: [_Any!]!, $representations_f2: [_Any!]!){` +
			`f1: _entities(representations: $representations_f1){... on Employee {skillA}} ` +
			`f2: _entities(representations: $representations_f2){... on Employee {skillB}}}",` +
			`"variables":{` +
			`"representations_f1":[{"__typename":"Employee","id":"1"}],` +
			`"representations_f2":[{"__typename":"Employee","id":"1"}]}}}`

		const mergedResponse = `{"data":{` +
			`"f1":[{"__typename":"Employee","skillA":"go"}],` +
			`"f2":[{"__typename":"Employee","skillB":"rust"}]}}`

		const want = `{"data":{"a":{"skillA":"go"},"b":{"skillB":"rust"}}}`

		const header = `{"method":"POST","url":"http://products.service","body":{` +
			`"query":"query($representations_f1: [_Any!]!, $representations_f2: [_Any!]!){` +
			`f1: _entities(representations: $representations_f1){... on Employee {skillA}} ` +
			`f2: _entities(representations: $representations_f2){... on Employee {skillB}}}",` +
			`"variables":{`

		return &GraphQLResponse{
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: FakeDataSource(rootResponse),
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Info: &FetchInfo{DataSourceID: "accounts", DataSourceName: "accounts"},
				}, "query"),
				SingleWithPath(&MultiEntityFetch{
					DataSource: mockedDS(t, ctrl, mergedRequest, mergedResponse),
					Header:     staticSegment(header),
					Footer:     staticSegment(`}}}`),
					Fetches: []*MultiEntitySubFetch{
						{
							Alias:        "f1",
							FetchPath:    []FetchItemPathElement{ObjectPath("a")},
							ResponsePath: "query.a",
							Batch:        false,
							Input: BatchInput{
								Header:    staticSegment(`"representations_f1":[`),
								Items:     []InputTemplate{repRenderer("Employee", "id")},
								Separator: staticSegment(`,`),
								Footer:    staticSegment(`]`),
							},
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "f1", "0"},
							},
						},
						{
							Alias:        "f2",
							FetchPath:    []FetchItemPathElement{ObjectPath("b")},
							ResponsePath: "query.b",
							Batch:        false,
							Input: BatchInput{
								Header:    staticSegment(`,"representations_f2":[`),
								Items:     []InputTemplate{repRenderer("Employee", "id")},
								Separator: staticSegment(`,`),
								Footer:    staticSegment(`]`),
							},
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "f2", "0"},
							},
						},
					},
					Info: &FetchInfo{DataSourceID: "products", DataSourceName: "products"},
				}, "query"),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("a"),
						Value: &Object{
							Path:     []string{"a"},
							Nullable: true,
							Fields: []*Field{
								{
									Name:  []byte("skillA"),
									Value: &String{Path: []string{"skillA"}, Nullable: true},
								},
							},
						},
					},
					{
						Name: []byte("b"),
						Value: &Object{
							Path:     []string{"b"},
							Nullable: true,
							Fields: []*Field{
								{
									Name:  []byte("skillB"),
									Value: &String{Path: []string{"skillB"}, Nullable: true},
								},
							},
						},
					},
				},
			},
		}, *NewContext(context.Background()), want
	})(t)
}

// ROUTER-62 #1 — mixed batch + single object shapes in ONE merged request.
//
// employees is a list (BatchEntityFetch semantics) and store is a single object
// (EntityFetch semantics); both resolve on the products subgraph. The merged
// request's representation list is the concatenation of a batch (two Employee
// reps) and a singleton (one Store rep) — the clean "batch vs single" split must
// be abandoned for the merged node (gotcha #1). The aliased response is
// demultiplexed back onto employees[] (per-item) and store (object).
func TestResolveGraphQLResponse_ROUTER62_MixedBatchAndSingleShapes(t *testing.T) {
	testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		const rootResponse = `{"data":{` +
			`"employees":[{"__typename":"Employee","id":"1"},{"__typename":"Employee","id":"2"}],` +
			`"store":{"__typename":"Store","id":"s1"}}}`

		// TARGET: one request; f1 carries a BATCH of Employee reps, f2 a SINGLE Store rep.
		const mergedRequest = `{"method":"POST","url":"http://products.service","body":{` +
			`"query":"query($representations_f1: [_Any!]!, $representations_f2: [_Any!]!){` +
			`f1: _entities(representations: $representations_f1){... on Employee {products}} ` +
			`f2: _entities(representations: $representations_f2){... on Store {reviewScore}}}",` +
			`"variables":{` +
			`"representations_f1":[{"__typename":"Employee","id":"1"},{"__typename":"Employee","id":"2"}],` +
			`"representations_f2":[{"__typename":"Store","id":"s1"}]}}}`

		const mergedResponse = `{"data":{` +
			`"f1":[{"__typename":"Employee","products":["p1"]},{"__typename":"Employee","products":["p2"]}],` +
			`"f2":[{"__typename":"Store","reviewScore":5}]}}`

		const want = `{"data":{"employees":[{"products":["p1"]},{"products":["p2"]}],"store":{"reviewScore":5}}}`

		const header = `{"method":"POST","url":"http://products.service","body":{` +
			`"query":"query($representations_f1: [_Any!]!, $representations_f2: [_Any!]!){` +
			`f1: _entities(representations: $representations_f1){... on Employee {products}} ` +
			`f2: _entities(representations: $representations_f2){... on Store {reviewScore}}}",` +
			`"variables":{`

		return &GraphQLResponse{
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: FakeDataSource(rootResponse),
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Info: &FetchInfo{DataSourceID: "accounts", DataSourceName: "accounts"},
				}, "query"),
				SingleWithPath(&MultiEntityFetch{
					DataSource: mockedDS(t, ctrl, mergedRequest, mergedResponse),
					Header:     staticSegment(header),
					Footer:     staticSegment(`}}}`),
					Fetches: []*MultiEntitySubFetch{
						{
							Alias:        "f1",
							FetchPath:    []FetchItemPathElement{ArrayPath("employees")},
							ResponsePath: "query.employees",
							Batch:        true,
							Input: BatchInput{
								Header:    staticSegment(`"representations_f1":[`),
								Items:     []InputTemplate{repRenderer("Employee", "id")},
								Separator: staticSegment(`,`),
								Footer:    staticSegment(`]`),
							},
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "f1"},
							},
						},
						{
							Alias:        "f2",
							FetchPath:    []FetchItemPathElement{ObjectPath("store")},
							ResponsePath: "query.store",
							Batch:        false,
							Input: BatchInput{
								Header:    staticSegment(`,"representations_f2":[`),
								Items:     []InputTemplate{repRenderer("Store", "id")},
								Separator: staticSegment(`,`),
								Footer:    staticSegment(`]`),
							},
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "f2", "0"},
							},
						},
					},
					Info: &FetchInfo{DataSourceID: "products", DataSourceName: "products"},
				}, "query"),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("employees"),
						Value: &Array{
							Path:     []string{"employees"},
							Nullable: true,
							Item: &Object{
								Nullable: true,
								Fields: []*Field{
									{
										Name:  []byte("products"),
										Value: &Array{Path: []string{"products"}, Nullable: true, Item: &String{}},
									},
								},
							},
						},
					},
					{
						Name: []byte("store"),
						Value: &Object{
							Path:     []string{"store"},
							Nullable: true,
							Fields: []*Field{
								{
									Name:  []byte("reviewScore"),
									Value: &Integer{Path: []string{"reviewScore"}, Nullable: true},
								},
							},
						},
					},
				},
			},
		}, *NewContext(context.Background()), want
	})(t)
}

// ROUTER-62 #3 (dedup variant) — two list items sharing a key send ONE
// representation, and the single returned entity is fanned back out to both.
//
// employees[0] and employees[1] have the same id; the merged batch must dedup
// them into a single representation in the outgoing request, then map the one
// returned entity back onto BOTH original list positions. This is the dedup
// counterpart to the null-filtering test — both exercise batchStats index
// mapping.
func TestResolveGraphQLResponse_ROUTER62_BatchDuplicateKeyDedup(t *testing.T) {
	testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		const rootResponse = `{"data":{"employees":[` +
			`{"__typename":"Employee","id":"1"},` +
			`{"__typename":"Employee","id":"1"}]}}`

		// TARGET: the duplicate key collapses to a SINGLE representation.
		const mergedRequest = `{"method":"POST","url":"http://products.service","body":{` +
			`"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Employee {products}}}",` +
			`"variables":{"representations":[{"__typename":"Employee","id":"1"}]}}}`

		// One entity returned for the one representation sent.
		const mergedResponse = `{"data":{"_entities":[{"__typename":"Employee","products":["p1"]}]}}`

		// The single entity is fanned out to both list positions.
		const want = `{"data":{"employees":[{"products":["p1"]},{"products":["p1"]}]}}`

		const header = `{"method":"POST","url":"http://products.service","body":{` +
			`"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Employee {products}}}",` +
			`"variables":{`

		return &GraphQLResponse{
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: FakeDataSource(rootResponse),
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Info: &FetchInfo{DataSourceID: "accounts", DataSourceName: "accounts"},
				}, "query"),
				SingleWithPath(&MultiEntityFetch{
					DataSource: mockedDS(t, ctrl, mergedRequest, mergedResponse),
					Header:     staticSegment(header),
					Footer:     staticSegment(`}}}`),
					Fetches: []*MultiEntitySubFetch{
						{
							FetchPath:    []FetchItemPathElement{ArrayPath("employees")},
							ResponsePath: "query.employees",
							Batch:        true,
							Input: BatchInput{
								Header:    staticSegment(`"representations":[`),
								Items:     []InputTemplate{repRenderer("Employee", "id")},
								Separator: staticSegment(`,`),
								Footer:    staticSegment(`]`),
							},
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities"},
							},
						},
					},
					Info: &FetchInfo{DataSourceID: "products", DataSourceName: "products"},
				}, "query"),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("employees"),
						Value: &Array{
							Path:     []string{"employees"},
							Nullable: true,
							Item: &Object{
								Nullable: true,
								Fields: []*Field{
									{
										Name:  []byte("products"),
										Value: &Array{Path: []string{"products"}, Nullable: true, Item: &String{}},
									},
								},
							},
						},
					},
				},
			},
		}, *NewContext(context.Background()), want
	})(t)
}

// ROUTER-62 #3 (per-alias) — null filtering must be maintained PER ALIAS.
//
// Two aliased batches in one merged request, each with its own null pattern:
// employeesA drops its middle item (index 1), employeesB drops its first item
// (index 0). batchStats must be tracked independently per alias so each alias's
// results scatter back to its own correct non-null positions. This is the
// multi-alias generalization of BatchNullRepresentationFiltering and the exact
// place the PoC's shared/positional mapping broke.
func TestResolveGraphQLResponse_ROUTER62_PerAliasBatchNullFiltering(t *testing.T) {
	testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		const rootResponse = `{"data":{` +
			`"employeesA":[{"__typename":"Employee","id":"1"},null,{"__typename":"Employee","id":"3"}],` +
			`"employeesB":[null,{"__typename":"Employee","id":"5"}]}}`

		// TARGET: each alias's variable contains only ITS non-null representations.
		const mergedRequest = `{"method":"POST","url":"http://products.service","body":{` +
			`"query":"query($representations_f1: [_Any!]!, $representations_f2: [_Any!]!){` +
			`f1: _entities(representations: $representations_f1){... on Employee {skillA}} ` +
			`f2: _entities(representations: $representations_f2){... on Employee {skillB}}}",` +
			`"variables":{` +
			`"representations_f1":[{"__typename":"Employee","id":"1"},{"__typename":"Employee","id":"3"}],` +
			`"representations_f2":[{"__typename":"Employee","id":"5"}]}}}`

		const mergedResponse = `{"data":{` +
			`"f1":[{"__typename":"Employee","skillA":"a1"},{"__typename":"Employee","skillA":"a3"}],` +
			`"f2":[{"__typename":"Employee","skillB":"b5"}]}}`

		// Per-alias index alignment: A maps to indices 0,2 (1 stays null);
		// B maps to index 1 (0 stays null).
		const want = `{"data":{` +
			`"employeesA":[{"skillA":"a1"},null,{"skillA":"a3"}],` +
			`"employeesB":[null,{"skillB":"b5"}]}}`

		const header = `{"method":"POST","url":"http://products.service","body":{` +
			`"query":"query($representations_f1: [_Any!]!, $representations_f2: [_Any!]!){` +
			`f1: _entities(representations: $representations_f1){... on Employee {skillA}} ` +
			`f2: _entities(representations: $representations_f2){... on Employee {skillB}}}",` +
			`"variables":{`

		return &GraphQLResponse{
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: FakeDataSource(rootResponse),
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Info: &FetchInfo{DataSourceID: "accounts", DataSourceName: "accounts"},
				}, "query"),
				SingleWithPath(&MultiEntityFetch{
					DataSource: mockedDS(t, ctrl, mergedRequest, mergedResponse),
					Header:     staticSegment(header),
					Footer:     staticSegment(`}}}`),
					Fetches: []*MultiEntitySubFetch{
						{
							Alias:        "f1",
							FetchPath:    []FetchItemPathElement{ArrayPath("employeesA")},
							ResponsePath: "query.employeesA",
							Batch:        true,
							Input: BatchInput{
								Header:        staticSegment(`"representations_f1":[`),
								Items:         []InputTemplate{repRenderer("Employee", "id")},
								Separator:     staticSegment(`,`),
								Footer:        staticSegment(`]`),
								SkipNullItems: true,
							},
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "f1"},
							},
						},
						{
							Alias:        "f2",
							FetchPath:    []FetchItemPathElement{ArrayPath("employeesB")},
							ResponsePath: "query.employeesB",
							Batch:        true,
							Input: BatchInput{
								Header:        staticSegment(`,"representations_f2":[`),
								Items:         []InputTemplate{repRenderer("Employee", "id")},
								Separator:     staticSegment(`,`),
								Footer:        staticSegment(`]`),
								SkipNullItems: true,
							},
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "f2"},
							},
						},
					},
					Info: &FetchInfo{DataSourceID: "products", DataSourceName: "products"},
				}, "query"),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("employeesA"),
						Value: &Array{
							Path:     []string{"employeesA"},
							Nullable: true,
							Item: &Object{
								Nullable: true,
								Fields: []*Field{
									{Name: []byte("skillA"), Value: &String{Path: []string{"skillA"}, Nullable: true}},
								},
							},
						},
					},
					{
						Name: []byte("employeesB"),
						Value: &Array{
							Path:     []string{"employeesB"},
							Nullable: true,
							Item: &Object{
								Nullable: true,
								Fields: []*Field{
									{Name: []byte("skillB"), Value: &String{Path: []string{"skillB"}, Nullable: true}},
								},
							},
						},
					},
				},
			},
		}, *NewContext(context.Background()), want
	})(t)
}

// ROUTER-62 AC3 — a whole merged request that fails maps the failure onto every
// participating response path.
//
// When the single upstream request errors (transport/HTTP failure), both merged
// paths (employee.products, store.reviewScore) must surface the standard
// "Failed to fetch from Subgraph" error, exactly as they would if issued
// separately. The failure is reported once at the shared fetch path ("query")
// and every participating field stays null.
func TestResolveGraphQLResponse_ROUTER62_WholeRequestFailure(t *testing.T) {
	testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		const rootResponse = `{"data":{"employee":{"__typename":"Employee","id":"1"},"store":{"__typename":"Store","id":"s1"}}}`

		// The merged upstream request fails at the transport layer.
		failingDS := NewMockDataSource(ctrl)
		failingDS.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
				return nil, &net.AddrError{}
			}).Times(1)

		const want = `{"errors":[{"message":"Failed to fetch from Subgraph 'products' at Path 'query'."}],` +
			`"data":{"employee":{"products":null},"store":{"reviewScore":null}}}`

		const header = `{"method":"POST","url":"http://products.service","body":{` +
			`"query":"query($representations_f1: [_Any!]!, $representations_f2: [_Any!]!){` +
			`f1: _entities(representations: $representations_f1){... on Employee {products}} ` +
			`f2: _entities(representations: $representations_f2){... on Store {reviewScore}}}",` +
			`"variables":{`

		return &GraphQLResponse{
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: FakeDataSource(rootResponse),
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Info: &FetchInfo{DataSourceID: "accounts", DataSourceName: "accounts"},
				}, "query"),
				SingleWithPath(&MultiEntityFetch{
					DataSource: failingDS,
					Header:     staticSegment(header),
					Footer:     staticSegment(`}}}`),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
					Fetches: []*MultiEntitySubFetch{
						{
							Alias:        "f1",
							FetchPath:    []FetchItemPathElement{ObjectPath("employee")},
							ResponsePath: "query.employee",
							Input: BatchInput{
								Header:    staticSegment(`"representations_f1":[`),
								Items:     []InputTemplate{repRenderer("Employee", "id")},
								Separator: staticSegment(`,`),
								Footer:    staticSegment(`]`),
							},
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "f1", "0"},
							},
						},
						{
							Alias:        "f2",
							FetchPath:    []FetchItemPathElement{ObjectPath("store")},
							ResponsePath: "query.store",
							Input: BatchInput{
								Header:    staticSegment(`,"representations_f2":[`),
								Items:     []InputTemplate{repRenderer("Store", "id")},
								Separator: staticSegment(`,`),
								Footer:    staticSegment(`]`),
							},
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "f2", "0"},
							},
						},
					},
					Info: &FetchInfo{DataSourceID: "products", DataSourceName: "products"},
				}, "query"),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("employee"),
						Value: &Object{
							Path:     []string{"employee"},
							Nullable: true,
							Fields: []*Field{
								{Name: []byte("products"), Value: &Array{Path: []string{"products"}, Nullable: true, Item: &String{}}},
							},
						},
					},
					{
						Name: []byte("store"),
						Value: &Object{
							Path:     []string{"store"},
							Nullable: true,
							Fields: []*Field{
								{Name: []byte("reviewScore"), Value: &Integer{Path: []string{"reviewScore"}, Nullable: true}},
							},
						},
					},
				},
			},
		}, *NewContext(context.Background()), want
	})(t)
}

// ROUTER-62 AC3 — a partial per-alias failure remaps the alias-prefixed error
// path back to the real response path, and unaffected aliases still resolve.
//
// The merged response carries data for f1 (employee.products) but an error for
// f2 (store.reviewScore). The loader demultiplexes f1's data normally AND rewrites
// the subgraph error's alias-prefixed path (["f2", 0, "reviewScore"]) to the real
// data path (["store", "reviewScore"]). The resolver runs in the default wrapped
// error-propagation mode, so the remapped subgraph error nests under
// extensions.errors of a wrapped "Failed to fetch" error attributed to store.
func TestResolveGraphQLResponse_ROUTER62_PartialPerAliasError(t *testing.T) {
	testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
		const rootResponse = `{"data":{"employee":{"__typename":"Employee","id":"1"},"store":{"__typename":"Store","id":"s1"}}}`

		const mergedRequest = `{"method":"POST","url":"http://products.service","body":{` +
			`"query":"query($representations_f1: [_Any!]!, $representations_f2: [_Any!]!){` +
			`f1: _entities(representations: $representations_f1){... on Employee {products}} ` +
			`f2: _entities(representations: $representations_f2){... on Store {reviewScore}}}",` +
			`"variables":{` +
			`"representations_f1":[{"__typename":"Employee","id":"1"}],` +
			`"representations_f2":[{"__typename":"Store","id":"s1"}]}}}`

		// f1 resolves; f2 is null and carries an alias-prefixed error path.
		const mergedResponse = `{"data":{` +
			`"f1":[{"__typename":"Employee","products":["p1","p2"]}],` +
			`"f2":[null]},` +
			`"errors":[{"message":"store unavailable","path":["f2",0,"reviewScore"]}]}`

		// f1 demultiplexed to employee.products; f2's error remapped to store.reviewScore
		// and wrapped (default propagation mode).
		const want = `{"errors":[{"message":"Failed to fetch from Subgraph 'products' at Path 'store'.",` +
			`"extensions":{"errors":[{"message":"store unavailable","path":["store","reviewScore"]}]}}],` +
			`"data":{"employee":{"products":["p1","p2"]},"store":{"reviewScore":null}}}`

		const header = `{"method":"POST","url":"http://products.service","body":{` +
			`"query":"query($representations_f1: [_Any!]!, $representations_f2: [_Any!]!){` +
			`f1: _entities(representations: $representations_f1){... on Employee {products}} ` +
			`f2: _entities(representations: $representations_f2){... on Store {reviewScore}}}",` +
			`"variables":{`

		return &GraphQLResponse{
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						DataSource: FakeDataSource(rootResponse),
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Info: &FetchInfo{DataSourceID: "accounts", DataSourceName: "accounts"},
				}, "query"),
				SingleWithPath(&MultiEntityFetch{
					DataSource: mockedDS(t, ctrl, mergedRequest, mergedResponse),
					Header:     staticSegment(header),
					Footer:     staticSegment(`}}}`),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseErrorsPath: []string{"errors"},
					},
					Fetches: []*MultiEntitySubFetch{
						{
							Alias:        "f1",
							FetchPath:    []FetchItemPathElement{ObjectPath("employee")},
							ResponsePath: "query.employee",
							Input: BatchInput{
								Header:    staticSegment(`"representations_f1":[`),
								Items:     []InputTemplate{repRenderer("Employee", "id")},
								Separator: staticSegment(`,`),
								Footer:    staticSegment(`]`),
							},
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "f1", "0"},
							},
						},
						{
							Alias:        "f2",
							FetchPath:    []FetchItemPathElement{ObjectPath("store")},
							ResponsePath: "query.store",
							Input: BatchInput{
								Header:    staticSegment(`,"representations_f2":[`),
								Items:     []InputTemplate{repRenderer("Store", "id")},
								Separator: staticSegment(`,`),
								Footer:    staticSegment(`]`),
							},
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "f2", "0"},
							},
						},
					},
					Info: &FetchInfo{DataSourceID: "products", DataSourceName: "products"},
				}, "query"),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("employee"),
						Value: &Object{
							Path:     []string{"employee"},
							Nullable: true,
							Fields: []*Field{
								{Name: []byte("products"), Value: &Array{Path: []string{"products"}, Nullable: true, Item: &String{}}},
							},
						},
					},
					{
						Name: []byte("store"),
						Value: &Object{
							Path:     []string{"store"},
							Nullable: true,
							Fields: []*Field{
								{Name: []byte("reviewScore"), Value: &Integer{Path: []string{"reviewScore"}, Nullable: true}},
							},
						},
					},
				},
			},
		}, *NewContext(context.Background()), want
	})(t)
}

// staticSegment builds an InputTemplate that renders a fixed static string. It is
// used for the shared request envelope (Header/Footer) and the per-alias variable
// header/separator/footer around the rendered representations.
func staticSegment(data string) InputTemplate {
	return InputTemplate{
		Segments: []TemplateSegment{
			{
				Data:        []byte(data),
				SegmentType: StaticSegmentType,
			},
		},
	}
}

// repRenderer builds a representation-variable InputTemplate that renders
// {"__typename":"<typeName>","<key>":"<value>",...} from each item selected at the
// sub-fetch's FetchPath, mirroring the planner-generated batch representation
// template. A null source item renders as JSON null (dropped when SkipNullItems is
// set), which drives the per-alias null-filtering behavior.
func repRenderer(typeName string, keys ...string) InputTemplate {
	fields := make([]*Field, 0, len(keys)+1)
	fields = append(fields, &Field{
		Name:        []byte("__typename"),
		Value:       &String{Path: []string{"__typename"}},
		OnTypeNames: [][]byte{[]byte(typeName)},
	})
	for _, k := range keys {
		fields = append(fields, &Field{
			Name:        []byte(k),
			Value:       &String{Path: []string{k}},
			OnTypeNames: [][]byte{[]byte(typeName)},
		})
	}
	return InputTemplate{
		SetTemplateOutputToNullOnVariableNull: true,
		Segments: []TemplateSegment{
			{
				SegmentType:  VariableSegmentType,
				VariableKind: ResolvableObjectVariableKind,
				Renderer:     NewGraphQLVariableResolveRenderer(&Object{Nullable: true, Fields: fields}),
			},
		},
	}
}
