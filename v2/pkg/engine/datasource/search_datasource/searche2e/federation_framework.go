package searche2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/search_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

// --- Entity subgraph data ---

type Review struct {
	Text  string `json:"text"`
	Stars int    `json:"stars"`
}

type ProductDetail struct {
	Reviews      []Review `json:"reviews"`
	Rating       float64  `json:"rating"`
	Manufacturer string   `json:"manufacturer"`
}

var productDetails = map[string]ProductDetail{
	"1": {Reviews: []Review{{Text: "Great shoes", Stars: 5}}, Rating: 4.5, Manufacturer: "Nike"},
	"2": {Reviews: []Review{{Text: "Good grip", Stars: 4}}, Rating: 4.2, Manufacturer: "Adidas"},
	"3": {Reviews: []Review{{Text: "Nice belt", Stars: 3}}, Rating: 3.8, Manufacturer: "Gucci"},
	"4": {Reviews: []Review{{Text: "Warm socks", Stars: 5}}, Rating: 4.7, Manufacturer: "Smartwool"},
}

// entitySubgraphSDL is the federation SDL for the entity subgraph.
const entitySubgraphSDL = `
type Product @key(fields: "id") {
    id: ID! @external
    reviews: [Review!]!
    rating: Float
    manufacturer: String
}

type Review {
    text: String!
    stars: Int!
}

type Query {
    _entities(representations: [_Any!]!): [_Entity]!
}

scalar _Any
union _Entity = Product
`

// startEntitySubgraph starts an HTTP test server that handles _entities queries.
func startEntitySubgraph(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var req struct {
			Query     string          `json:"query"`
			Variables json.RawMessage `json:"variables"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var vars struct {
			Representations []map[string]any `json:"representations"`
		}
		if err := json.Unmarshal(req.Variables, &vars); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		entities := make([]any, 0, len(vars.Representations))
		for _, rep := range vars.Representations {
			id, _ := rep["id"].(string)
			detail, ok := productDetails[id]
			if !ok {
				entities = append(entities, nil)
				continue
			}
			entity := map[string]any{
				"id":           id,
				"__typename":   "Product",
				"reviews":      detail.Reviews,
				"rating":       detail.Rating,
				"manufacturer": detail.Manufacturer,
			}
			entities = append(entities, entity)
		}

		resp := map[string]any{
			"data": map[string]any{
				"_entities": entities,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(server.Close)
	return server
}

// --- Supergraph definition ---

// The merged supergraph schema combining search subgraph + entity subgraph.
// This is a hand-written merge for the Product test case.
const supergraphDefinition = `
input StringFilter {
  eq: String
  ne: String
  in: [String!]
  contains: String
  startsWith: String
}

input FloatFilter {
  eq: Float
  gt: Float
  gte: Float
  lt: Float
  lte: Float
}

input IntFilter {
  eq: Int
  gt: Int
  gte: Int
  lt: Int
  lte: Int
}

enum SortDirection {
  ASC
  DESC
}

type SearchFacet {
  field: String!
  values: [SearchFacetValue!]!
}

type SearchFacetValue {
  value: String!
  count: Int!
}

input ProductFilter {
  name: StringFilter
  category: StringFilter
  price: FloatFilter
  inStock: Boolean
  AND: [ProductFilter!]
  OR: [ProductFilter!]
  NOT: ProductFilter
}

enum ProductSortField {
  RELEVANCE
  NAME
  CATEGORY
  PRICE
}

input ProductSort {
  field: ProductSortField!
  direction: SortDirection!
}

type SearchProductResult {
  hits: [SearchProductHit!]!
  totalCount: Int!
  facets: [SearchFacet!]
}

type SearchProductHit {
  score: Float!
  node: Product!
}

type Product {
  id: ID!
  reviews: [Review!]!
  rating: Float
  manufacturer: String
}

type Review {
  text: String!
  stars: Int!
}

type Query {
  searchProducts(
    query: String!
    filter: ProductFilter
    sort: [ProductSort!]
    limit: Int
    offset: Int
    facets: [String!]
  ): SearchProductResult!
}
`

// --- Config builder ---

// FederatedTestSetup holds the configuration and cleanup for a federated test.
type FederatedTestSetup struct {
	PlanConfig plan.Configuration
	Definition string
	Cleanup    func()
}

// BuildFederatedConfig creates a federated plan.Configuration with a search datasource
// and an entity subgraph datasource.
func BuildFederatedConfig(t *testing.T, idx searchindex.Index) *FederatedTestSetup {
	t.Helper()

	entityServer := startEntitySubgraph(t)

	// --- Search datasource (DS1) ---
	searchConfig := ProductDatasourceConfig()
	searchFactory := search_datasource.NewFactory(context.Background(), nil, nil)
	searchFactory.RegisterIndex(searchConfig.IndexName, idx)

	searchDS, err := plan.NewDataSourceConfiguration[search_datasource.Configuration](
		"search-ds",
		searchFactory,
		&plan.DataSourceMetadata{
			RootNodes: plan.TypeFields{
				{TypeName: "Query", FieldNames: []string{"searchProducts"}},
				{TypeName: "Product", FieldNames: []string{"id"}},
			},
			ChildNodes: plan.TypeFields{
				{TypeName: "SearchProductResult", FieldNames: []string{"hits", "totalCount", "facets"}},
				{TypeName: "SearchProductHit", FieldNames: []string{"score", "node"}},
				{TypeName: "SearchFacet", FieldNames: []string{"field", "values"}},
				{TypeName: "SearchFacetValue", FieldNames: []string{"value", "count"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Product", SelectionSet: "id"},
				},
			},
		},
		searchConfig,
	)
	if err != nil {
		t.Fatalf("NewDataSourceConfiguration (search): %v", err)
	}

	// --- Entity datasource (DS2) ---
	entitySchemaConfig, err := graphql_datasource.NewSchemaConfiguration(
		entitySubgraphSDL,
		&graphql_datasource.FederationConfiguration{
			Enabled:    true,
			ServiceSDL: entitySubgraphSDL,
		},
	)
	if err != nil {
		t.Fatalf("NewSchemaConfiguration (entity): %v", err)
	}

	entityConfig, err := graphql_datasource.NewConfiguration(graphql_datasource.ConfigurationInput{
		Fetch: &graphql_datasource.FetchConfiguration{
			URL: entityServer.URL,
		},
		SchemaConfiguration: entitySchemaConfig,
	})
	if err != nil {
		t.Fatalf("NewConfiguration (entity): %v", err)
	}

	entityFactory, err := graphql_datasource.NewFactory(context.Background(), http.DefaultClient, &noopSubscriptionClient{})
	if err != nil {
		t.Fatalf("NewFactory (entity): %v", err)
	}

	entityDS, err := plan.NewDataSourceConfiguration[graphql_datasource.Configuration](
		"entity-ds",
		entityFactory,
		&plan.DataSourceMetadata{
			RootNodes: plan.TypeFields{
				{TypeName: "Product", FieldNames: []string{"id", "reviews", "rating", "manufacturer"}},
			},
			ChildNodes: plan.TypeFields{
				{TypeName: "Review", FieldNames: []string{"text", "stars"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Product", SelectionSet: "id"},
				},
			},
		},
		entityConfig,
	)
	if err != nil {
		t.Fatalf("NewDataSourceConfiguration (entity): %v", err)
	}

	// --- Plan configuration ---
	planConfig := plan.Configuration{
		DataSources: []plan.DataSource{
			searchDS,
			entityDS,
		},
		Fields: plan.FieldConfigurations{
			{
				TypeName:  "Query",
				FieldName: "searchProducts",
				Arguments: plan.ArgumentsConfigurations{
					{Name: "query", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsJSONValue},
					{Name: "filter", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsJSONValue},
					{Name: "sort", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsJSONValue},
					{Name: "limit", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsJSONValue},
					{Name: "offset", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsJSONValue},
					{Name: "facets", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsJSONValue},
				},
			},
		},
		DisableResolveFieldPositions: true,
	}

	return &FederatedTestSetup{
		PlanConfig: planConfig,
		Definition: supergraphDefinition,
		Cleanup: func() {
			entityServer.Close()
		},
	}
}

// --- Execution helper ---

// ExecuteFederatedQuery plans and resolves a GraphQL query through the full pipeline.
func ExecuteFederatedQuery(t *testing.T, setup *FederatedTestSetup, query string, variables string) string {
	t.Helper()

	def := unsafeparser.ParseGraphqlDocumentString(setup.Definition)
	op := unsafeparser.ParseGraphqlDocumentString(query)

	if err := asttransform.MergeDefinitionWithBaseSchema(&def); err != nil {
		t.Fatalf("MergeDefinitionWithBaseSchema: %v", err)
	}

	report := &operationreport.Report{}
	norm := astnormalization.NewNormalizer(true, true)
	norm.NormalizeOperation(&op, &def, report)
	if report.HasErrors() {
		t.Fatalf("normalize: %s", report.Error())
	}

	valid := astvalidation.DefaultOperationValidator()
	valid.Validate(&op, &def, report)
	if report.HasErrors() {
		t.Fatalf("validate: %s", report.Error())
	}

	p, err := plan.NewPlanner(setup.PlanConfig)
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}

	executionPlan := p.Plan(&op, &def, "", report)
	if report.HasErrors() {
		t.Fatalf("plan: %s", report.Error())
	}

	// Post-process the plan to build the fetch tree from raw fetches.
	proc := postprocess.NewProcessor()
	proc.Process(executionPlan)

	syncPlan, ok := executionPlan.(*plan.SynchronousResponsePlan)
	if !ok {
		t.Fatalf("expected SynchronousResponsePlan, got %T", executionPlan)
	}

	if syncPlan.Response.Info == nil {
		syncPlan.Response.Info = &resolve.GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
		}
	}

	resolver := resolve.New(context.Background(), resolve.ResolverOptions{
		MaxConcurrency:          32,
		PropagateSubgraphErrors: true,
	})

	ctx := resolve.NewContext(context.Background())
	if variables != "" {
		ctx.Variables = astjson.MustParseBytes([]byte(variables))
	}

	buf := &bytes.Buffer{}
	_, err = resolver.ResolveGraphQLResponse(ctx, syncPlan.Response, nil, buf)
	if err != nil {
		t.Fatalf("ResolveGraphQLResponse: %v", err)
	}

	return buf.String()
}

// --- Test runner ---

// FederatedSearchResponse is the parsed response from a federated search query.
type FederatedSearchResponse struct {
	Data struct {
		SearchProducts struct {
			Hits []struct {
				Score float64 `json:"score"`
				Node  struct {
					ID           string   `json:"id"`
					Manufacturer string   `json:"manufacturer"`
					Rating       float64  `json:"rating"`
					Reviews      []Review `json:"reviews"`
				} `json:"node"`
			} `json:"hits"`
			TotalCount int `json:"totalCount"`
			Facets     []struct {
				Field  string `json:"field"`
				Values []struct {
					Value string `json:"value"`
					Count int    `json:"count"`
				} `json:"values"`
			} `json:"facets"`
		} `json:"searchProducts"`
	} `json:"data"`
}

func parseFederatedResponse(t *testing.T, raw string) FederatedSearchResponse {
	t.Helper()
	var resp FederatedSearchResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("parse federated response: %v\nraw: %s", err, raw)
	}
	return resp
}

// RunFederatedBackendTests runs federation e2e tests against the given index.
func RunFederatedBackendTests(t *testing.T, idx searchindex.Index, caps BackendCaps, hooks BackendHooks, indexFactory func(t *testing.T) searchindex.Index) {
	// Populate shared index with test data.
	if err := idx.IndexDocuments(context.Background(), TestProducts()); err != nil {
		t.Fatalf("populate test data: %v", err)
	}
	if hooks.WaitForIndex != nil {
		hooks.WaitForIndex(t)
	}

	setup := BuildFederatedConfig(t, idx)

	t.Run("basic_search_with_join", func(t *testing.T) {
		t.Parallel()
		query := `{ searchProducts(query: "shoes") { hits { node { id manufacturer } } totalCount } }`
		raw := ExecuteFederatedQuery(t, setup, query, "")
		resp := parseFederatedResponse(t, raw)

		if resp.Data.SearchProducts.TotalCount < 2 {
			t.Errorf("expected totalCount >= 2, got %d", resp.Data.SearchProducts.TotalCount)
		}
		if len(resp.Data.SearchProducts.Hits) < 2 {
			t.Errorf("expected >= 2 hits, got %d", len(resp.Data.SearchProducts.Hits))
		}
		for _, hit := range resp.Data.SearchProducts.Hits {
			if hit.Node.ID == "" {
				t.Error("expected non-empty id")
			}
			if hit.Node.Manufacturer == "" {
				t.Errorf("expected manufacturer for product %s, got empty", hit.Node.ID)
			}
			expected, ok := productDetails[hit.Node.ID]
			if ok && hit.Node.Manufacturer != expected.Manufacturer {
				t.Errorf("product %s: manufacturer = %q, want %q", hit.Node.ID, hit.Node.Manufacturer, expected.Manufacturer)
			}
		}
	})

	t.Run("filter_with_join", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter) { searchProducts(query: "*", filter: $f) { hits { node { id rating } } } }`
		vars := `{"f": {"category": {"eq": "Footwear"}}}`
		raw := ExecuteFederatedQuery(t, setup, query, vars)
		resp := parseFederatedResponse(t, raw)

		if len(resp.Data.SearchProducts.Hits) < 2 {
			t.Errorf("expected >= 2 hits for Footwear filter, got %d", len(resp.Data.SearchProducts.Hits))
		}
		for _, hit := range resp.Data.SearchProducts.Hits {
			if hit.Node.Rating == 0 {
				t.Errorf("expected rating for product %s, got 0", hit.Node.ID)
			}
		}
	})

	t.Run("sort_with_join", func(t *testing.T) {
		t.Parallel()
		query := `query($s: [ProductSort!]) { searchProducts(query: "*", sort: $s) { hits { node { id manufacturer } } } }`
		vars := `{"s": [{"field": "price", "direction": "ASC"}]}`
		raw := ExecuteFederatedQuery(t, setup, query, vars)
		resp := parseFederatedResponse(t, raw)

		if len(resp.Data.SearchProducts.Hits) < 2 {
			t.Errorf("expected >= 2 hits, got %d", len(resp.Data.SearchProducts.Hits))
		}
		// First hit should be cheapest (Wool Socks, id=4)
		if len(resp.Data.SearchProducts.Hits) > 0 {
			first := resp.Data.SearchProducts.Hits[0]
			if first.Node.ID != "4" {
				t.Errorf("expected first hit id=4 (cheapest), got %s", first.Node.ID)
			}
			if first.Node.Manufacturer == "" {
				t.Error("expected manufacturer on sorted hit")
			}
		}
	})

	t.Run("pagination_with_join", func(t *testing.T) {
		t.Parallel()
		query := `query($s: [ProductSort!], $lim: Int, $off: Int) {
			searchProducts(query: "*", sort: $s, limit: $lim, offset: $off) {
				hits { node { id reviews { text stars } } }
				totalCount
			}
		}`
		vars := `{"s": [{"field": "price", "direction": "ASC"}], "lim": 2, "off": 1}`
		raw := ExecuteFederatedQuery(t, setup, query, vars)
		resp := parseFederatedResponse(t, raw)

		if len(resp.Data.SearchProducts.Hits) != 2 {
			t.Errorf("expected 2 hits with limit=2 offset=1, got %d", len(resp.Data.SearchProducts.Hits))
		}
		for _, hit := range resp.Data.SearchProducts.Hits {
			if len(hit.Node.Reviews) == 0 {
				t.Errorf("expected reviews for product %s", hit.Node.ID)
			}
			for _, r := range hit.Node.Reviews {
				if r.Text == "" {
					t.Error("expected non-empty review text")
				}
				if r.Stars == 0 {
					t.Error("expected non-zero review stars")
				}
			}
		}
	})

	t.Run("full_hit_fields", func(t *testing.T) {
		t.Parallel()
		query := `{ searchProducts(query: "shoes") { hits { score node { id } } totalCount } }`
		raw := ExecuteFederatedQuery(t, setup, query, "")
		resp := parseFederatedResponse(t, raw)

		if resp.Data.SearchProducts.TotalCount < 2 {
			t.Errorf("expected totalCount >= 2, got %d", resp.Data.SearchProducts.TotalCount)
		}
		for _, hit := range resp.Data.SearchProducts.Hits {
			if hit.Node.ID == "" {
				t.Error("expected non-empty id in full_hit_fields")
			}
		}
	})

	if caps.HasFacets {
		t.Run("facets_with_join", func(t *testing.T) {
			t.Parallel()
			query := `query($fac: [String!]) { searchProducts(query: "*", facets: $fac) { hits { node { id manufacturer } } facets { field values { value count } } } }`
			vars := `{"fac": ["category"]}`
			raw := ExecuteFederatedQuery(t, setup, query, vars)
			resp := parseFederatedResponse(t, raw)

			if len(resp.Data.SearchProducts.Facets) == 0 {
				t.Fatal("expected at least 1 facet")
			}
			found := false
			for _, f := range resp.Data.SearchProducts.Facets {
				if f.Field == "category" {
					found = true
					if len(f.Values) < 2 {
						t.Errorf("expected >= 2 facet values for category, got %d", len(f.Values))
					}
				}
			}
			if !found {
				t.Error("expected category facet in response")
			}
			// Also verify entity join worked
			for _, hit := range resp.Data.SearchProducts.Hits {
				if hit.Node.Manufacturer == "" {
					t.Errorf("expected manufacturer for product %s in facets test", hit.Node.ID)
				}
			}
		})
	}
}

// --- Helpers ---

// noopSubscriptionClient satisfies graphql_datasource.GraphQLSubscriptionClient for tests.
type noopSubscriptionClient struct{}

func (n *noopSubscriptionClient) Subscribe(_ *resolve.Context, _ graphql_datasource.GraphQLSubscriptionOptions, _ resolve.SubscriptionUpdater) error {
	return nil
}
func (n *noopSubscriptionClient) SubscribeAsync(_ *resolve.Context, _ uint64, _ graphql_datasource.GraphQLSubscriptionOptions, _ resolve.SubscriptionUpdater) error {
	return nil
}
func (n *noopSubscriptionClient) Unsubscribe(_ uint64) {}
