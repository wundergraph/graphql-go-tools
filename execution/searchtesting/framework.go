package searchtesting

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/wundergraph/cosmo/composition-go"
	nodev1 "github.com/wundergraph/cosmo/router/gen/proto/wg/cosmo/node/v1"

	"github.com/wundergraph/graphql-go-tools/execution/searchtesting/productdetails"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/search_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

// expectedSupergraphSDL is the expected composed supergraph schema for the standard Product test setup.
// All backends use the same config SDL structure, so the composed supergraph is identical.
var expectedSupergraphSDL = `directive @tag(name: String!) repeatable on ARGUMENT_DEFINITION | ENUM | ENUM_VALUE | FIELD_DEFINITION | INPUT_FIELD_DEFINITION | INPUT_OBJECT | INTERFACE | OBJECT | SCALAR | UNION

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

enum Fuzziness {
  EXACT
  LOW
  HIGH
}

type SearchFacet {
  field: String!
  values: [SearchFacetValue!]!
}

type SearchFacetValue {
  value: String!
  count: Int!
}

type SearchHighlight {
  field: String!
  fragments: [String!]!
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
  highlights: [SearchHighlight!]
  node: Product!
}

type Query {
  searchProducts(query: String!, fuzziness: Fuzziness, filter: ProductFilter, sort: [ProductSort!], limit: Int, offset: Int, facets: [String!]): SearchProductResult!
}

type Product {
  id: ID!
  name: String
  description: String
  category: String
  price: Float
  inStock: Boolean
  reviews: [Review!]!
  rating: Float
  manufacturer: String
}

type Review {
  text: String!
  stars: Int!
}`

// expectedInlineSupergraphSDL is the expected composed supergraph schema for inline style (no wrapper types).
var expectedInlineSupergraphSDL = `directive @tag(name: String!) repeatable on ARGUMENT_DEFINITION | ENUM | ENUM_VALUE | FIELD_DEFINITION | INPUT_FIELD_DEFINITION | INPUT_OBJECT | INTERFACE | OBJECT | SCALAR | UNION

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

enum Fuzziness {
  EXACT
  LOW
  HIGH
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

type Query {
  searchProducts(query: String!, fuzziness: Fuzziness, filter: ProductFilter, sort: [ProductSort!], limit: Int, offset: Int): [Product!]!
}

type Product {
  id: ID!
  name: String
  description: String
  category: String
  price: Float
  inStock: Boolean
  reviews: [Review!]!
  rating: Float
  manufacturer: String
}

type Review {
  text: String!
  stars: Int!
}`

// BackendCaps describes what capabilities a backend supports.
type BackendCaps struct {
	HasTextSearch bool
	HasFacets     bool
}

// BackendHooks provides hooks for backend-specific behavior.
type BackendHooks struct {
	WaitForIndex func(t *testing.T)
}

// BackendSetup holds the configuration for a single backend's e2e test.
type BackendSetup struct {
	// Name is the backend identifier (e.g. "bleve", "elasticsearch").
	Name string
	// ConfigSDL is the complete configuration schema SDL with @index, @searchable, @indexed directives.
	ConfigSDL string
	// CreateIndex creates a search index for the backend.
	CreateIndex func(t *testing.T, name string, schema searchindex.IndexConfig, configJSON []byte) searchindex.Index
	// Caps describes the backend's capabilities.
	Caps BackendCaps
	// Hooks provides backend-specific behavior.
	Hooks BackendHooks
	// ExpectedResponses maps test scenario names to their expected JSON response strings.
	ExpectedResponses map[string]string
}

// entitySubgraphSDL returns the SDL of the entity subgraph by reading the schema file.
func entitySubgraphSDL(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	schemaPath := filepath.Join(filepath.Dir(thisFile), "productdetails", "graph", "schema.graphqls")
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read entity subgraph schema: %v", err)
	}
	return string(data)
}

// buildIndexSchema converts parsed entity fields into a searchindex.IndexConfig.
func buildIndexSchema(indexName string, entity *search_datasource.SearchableEntity) searchindex.IndexConfig {
	schema := searchindex.IndexConfig{Name: indexName}
	for _, f := range entity.Fields {
		schema.Fields = append(schema.Fields, searchindex.FieldConfig{
			Name:         f.FieldName,
			Type:         f.IndexType,
			Filterable:   f.Filterable,
			Sortable:     f.Sortable,
			Dimensions:   f.Dimensions,
			Weight:       f.Weight,
			Autocomplete: f.Autocomplete,
		})
	}
	for _, ef := range entity.EmbeddingFields {
		schema.Fields = append(schema.Fields, searchindex.FieldConfig{
			Name: ef.FieldName,
			Type: searchindex.FieldTypeVector,
		})
	}
	return schema
}

// entityToConfiguration converts a parsed SearchableEntity to a search_datasource.Configuration.
func entityToConfiguration(entity *search_datasource.SearchableEntity) search_datasource.Configuration {
	cfg := search_datasource.Configuration{
		IndexName:              entity.IndexName,
		SearchField:            entity.SearchField,
		EntityTypeName:         entity.TypeName,
		KeyFields:              entity.KeyFields,
		HasTextSearch:          entity.HasTextSearch(),
		HasVectorSearch:        entity.HasVectorSearch(),
		ResultsMetaInformation: entity.ResultsMetaInformation,
		CursorBasedPagination:  entity.CursorBasedPagination,
		CursorBidirectional:    entity.CursorBidirectional,
	}
	for _, f := range entity.Fields {
		cfg.Fields = append(cfg.Fields, search_datasource.IndexedFieldConfig{
			FieldName:    f.FieldName,
			GraphQLType:  f.GraphQLType,
			IndexType:    f.IndexType,
			Filterable:   f.Filterable,
			Sortable:     f.Sortable,
			Dimensions:   f.Dimensions,
			Weight:       f.Weight,
			Autocomplete: f.Autocomplete,
		})
	}
	for _, ef := range entity.EmbeddingFields {
		cfg.EmbeddingFields = append(cfg.EmbeddingFields, search_datasource.EmbeddingFieldConfig{
			FieldName:    ef.FieldName,
			SourceFields: ef.SourceFields,
			Template:     ef.Template,
			Model:        ef.Model,
		})
	}
	return cfg
}

// composeSubgraphs composes the search and entity subgraph SDLs using cosmo composition-go.
func composeSubgraphs(t *testing.T, searchSDL, entitySDL, entityURL string) *nodev1.RouterConfig {
	t.Helper()

	subgraphs := []*composition.Subgraph{
		{
			Name:                 "search",
			URL:                  "http://search.local",
			Schema:               searchSDL,
			SubscriptionProtocol: "ws",
		},
		{
			Name:                 "productdetails",
			URL:                  entityURL,
			Schema:               entitySDL,
			SubscriptionProtocol: "ws",
		},
	}

	resultJSON, err := composition.BuildRouterConfiguration(subgraphs...)
	if err != nil {
		t.Fatalf("composition failed: %v", err)
	}

	var routerConfig nodev1.RouterConfig
	if err := protojson.Unmarshal([]byte(resultJSON), &routerConfig); err != nil {
		t.Fatalf("unmarshal router config: %v", err)
	}

	return &routerConfig
}

// loadInternedString resolves an interned string from the engine config.
func loadInternedString(engineConfig *nodev1.EngineConfiguration, str *nodev1.InternedString) (string, error) {
	key := str.GetKey()
	s, ok := engineConfig.StringStorage[key]
	if !ok {
		return "", fmt.Errorf("no string found for key %q", key)
	}
	return s, nil
}

// noopSubscriptionClient satisfies graphql_datasource.GraphQLSubscriptionClient for tests.
type noopSubscriptionClient struct{}

func (n *noopSubscriptionClient) Subscribe(_ *resolve.Context, _ graphql_datasource.GraphQLSubscriptionOptions, _ resolve.SubscriptionUpdater) error {
	return nil
}
func (n *noopSubscriptionClient) SubscribeAsync(_ *resolve.Context, _ uint64, _ graphql_datasource.GraphQLSubscriptionOptions, _ resolve.SubscriptionUpdater) error {
	return nil
}
func (n *noopSubscriptionClient) Unsubscribe(_ uint64) {}

// buildPlanConfiguration builds a plan.Configuration from the composition output,
// replacing the search subgraph's datasource with a search_datasource.
func buildPlanConfiguration(
	t *testing.T,
	routerConfig *nodev1.RouterConfig,
	idx searchindex.Index,
	searchConfig search_datasource.Configuration,
	entityServerURL string,
	embedderRegistry *searchindex.EmbedderRegistry,
) plan.Configuration {
	t.Helper()

	engineConfig := routerConfig.EngineConfig
	var planConfig plan.Configuration
	planConfig.DefaultFlushIntervalMillis = engineConfig.DefaultFlushInterval

	// Extract field configurations from composition output.
	for _, fc := range engineConfig.FieldConfigurations {
		var args []plan.ArgumentConfiguration
		for _, ac := range fc.ArgumentsConfiguration {
			arg := plan.ArgumentConfiguration{
				Name:         ac.Name,
				RenderConfig: plan.RenderArgumentAsJSONValue,
			}
			switch ac.SourceType {
			case nodev1.ArgumentSource_FIELD_ARGUMENT:
				arg.SourceType = plan.FieldArgumentSource
			case nodev1.ArgumentSource_OBJECT_FIELD:
				arg.SourceType = plan.ObjectFieldSource
			}
			args = append(args, arg)
		}
		planConfig.Fields = append(planConfig.Fields, plan.FieldConfiguration{
			TypeName:  fc.TypeName,
			FieldName: fc.FieldName,
			Arguments: args,
		})
	}

	// Extract type configurations.
	for _, tc := range engineConfig.TypeConfigurations {
		planConfig.Types = append(planConfig.Types, plan.TypeConfiguration{
			TypeName: tc.TypeName,
			RenameTo: tc.RenameTo,
		})
	}

	// Build datasources from composition output.
	for _, ds := range engineConfig.DatasourceConfigurations {
		metadata := extractDataSourceMetadata(ds)

		fetchURL := ""
		if ds.CustomGraphql != nil && ds.CustomGraphql.Fetch != nil {
			fetchURL = ds.CustomGraphql.Fetch.GetUrl().GetStaticVariableContent()
		}

		if fetchURL == "http://search.local" {
			// Search datasource — use search_datasource.Factory.
			searchFactory := search_datasource.NewFactory(context.Background(), nil, embedderRegistry)
			searchFactory.RegisterIndex(searchConfig.IndexName, idx)

			searchDS, err := plan.NewDataSourceConfiguration[search_datasource.Configuration](
				ds.Id,
				searchFactory,
				metadata,
				searchConfig,
			)
			if err != nil {
				t.Fatalf("NewDataSourceConfiguration (search): %v", err)
			}
			planConfig.DataSources = append(planConfig.DataSources, searchDS)
		} else {
			// Entity datasource — use graphql_datasource.Factory.
			graphqlSchema, err := loadInternedString(engineConfig, ds.CustomGraphql.GetUpstreamSchema())
			if err != nil {
				t.Fatalf("load upstream schema: %v", err)
			}

			schemaConfig, err := graphql_datasource.NewSchemaConfiguration(
				graphqlSchema,
				&graphql_datasource.FederationConfiguration{
					Enabled:    ds.CustomGraphql.Federation.Enabled,
					ServiceSDL: ds.CustomGraphql.Federation.ServiceSdl,
				},
			)
			if err != nil {
				t.Fatalf("NewSchemaConfiguration (entity): %v", err)
			}

			entityConfig, err := graphql_datasource.NewConfiguration(graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{
					URL: entityServerURL,
				},
				SchemaConfiguration: schemaConfig,
			})
			if err != nil {
				t.Fatalf("NewConfiguration (entity): %v", err)
			}

			entityFactory, err := graphql_datasource.NewFactory(context.Background(), http.DefaultClient, &noopSubscriptionClient{})
			if err != nil {
				t.Fatalf("NewFactory (entity): %v", err)
			}

			entityDS, err := plan.NewDataSourceConfiguration[graphql_datasource.Configuration](
				ds.Id,
				entityFactory,
				metadata,
				entityConfig,
			)
			if err != nil {
				t.Fatalf("NewDataSourceConfiguration (entity): %v", err)
			}
			planConfig.DataSources = append(planConfig.DataSources, entityDS)
		}
	}

	planConfig.DisableResolveFieldPositions = true

	return planConfig
}

// extractDataSourceMetadata extracts plan.DataSourceMetadata from a composition datasource config.
func extractDataSourceMetadata(ds *nodev1.DataSourceConfiguration) *plan.DataSourceMetadata {
	meta := &plan.DataSourceMetadata{
		RootNodes:  make([]plan.TypeField, 0, len(ds.RootNodes)),
		ChildNodes: make([]plan.TypeField, 0, len(ds.ChildNodes)),
		FederationMetaData: plan.FederationMetaData{
			Keys:     make([]plan.FederationFieldConfiguration, 0, len(ds.Keys)),
			Requires: make([]plan.FederationFieldConfiguration, 0, len(ds.Requires)),
			Provides: make([]plan.FederationFieldConfiguration, 0, len(ds.Provides)),
		},
	}

	for _, node := range ds.RootNodes {
		meta.RootNodes = append(meta.RootNodes, plan.TypeField{
			TypeName:   node.TypeName,
			FieldNames: node.FieldNames,
		})
	}
	for _, node := range ds.ChildNodes {
		meta.ChildNodes = append(meta.ChildNodes, plan.TypeField{
			TypeName:   node.TypeName,
			FieldNames: node.FieldNames,
		})
	}
	for _, key := range ds.Keys {
		meta.FederationMetaData.Keys = append(meta.FederationMetaData.Keys, plan.FederationFieldConfiguration{
			TypeName:     key.TypeName,
			FieldName:    key.FieldName,
			SelectionSet: key.SelectionSet,
		})
	}
	for _, req := range ds.Requires {
		meta.FederationMetaData.Requires = append(meta.FederationMetaData.Requires, plan.FederationFieldConfiguration{
			TypeName:     req.TypeName,
			FieldName:    req.FieldName,
			SelectionSet: req.SelectionSet,
		})
	}
	for _, prov := range ds.Provides {
		meta.FederationMetaData.Provides = append(meta.FederationMetaData.Provides, plan.FederationFieldConfiguration{
			TypeName:     prov.TypeName,
			FieldName:    prov.FieldName,
			SelectionSet: prov.SelectionSet,
		})
	}
	for _, ei := range ds.EntityInterfaces {
		meta.FederationMetaData.EntityInterfaces = append(meta.FederationMetaData.EntityInterfaces, plan.EntityInterfaceConfiguration{
			InterfaceTypeName: ei.InterfaceTypeName,
			ConcreteTypeNames: ei.ConcreteTypeNames,
		})
	}
	for _, io := range ds.InterfaceObjects {
		meta.FederationMetaData.InterfaceObjects = append(meta.FederationMetaData.InterfaceObjects, plan.EntityInterfaceConfiguration{
			InterfaceTypeName: io.InterfaceTypeName,
			ConcreteTypeNames: io.ConcreteTypeNames,
		})
	}

	if len(ds.Directives) > 0 {
		d := make(plan.DirectiveConfigurations, 0, len(ds.Directives))
		for _, dir := range ds.Directives {
			d = append(d, plan.DirectiveConfiguration{
				DirectiveName: dir.DirectiveName,
				RenameTo:      dir.DirectiveName,
			})
		}
		meta.Directives = &d
	}

	return meta
}

// testEnv holds the shared test environment built by setupTestEnv.
type testEnv struct {
	Pipeline      *testPipeline
	SupergraphDef string
	DefaultSort   string
}

// setupTestEnv orchestrates steps 1-7 of the e2e test pipeline and returns a reusable testEnv.
func setupTestEnv(t *testing.T, setup BackendSetup) testEnv {
	t.Helper()

	// 1. Parse the config schema SDL.
	doc, parseReport := astparser.ParseGraphqlDocumentString(setup.ConfigSDL)
	if parseReport.HasErrors() {
		t.Fatalf("parse config schema: %s", parseReport.Error())
	}
	parsedConfig, err := search_datasource.ParseConfigSchema(&doc)
	if err != nil {
		t.Fatalf("ParseConfigSchema: %v", err)
	}
	if len(parsedConfig.Entities) == 0 {
		t.Fatal("no entities found in config schema")
	}

	// 2. Generate the search subgraph SDL.
	searchSDL, err := search_datasource.GenerateSubgraphSDL(parsedConfig)
	if err != nil {
		t.Fatalf("GenerateSubgraphSDL: %v", err)
	}

	entity := &parsedConfig.Entities[0]
	indexDirective := parsedConfig.Indices[0]

	// 3. Build index schema and create the search index.
	indexSchema := buildIndexSchema(indexDirective.Name, entity)
	idx := setup.CreateIndex(t, fmt.Sprintf("test_%s", setup.Name), indexSchema, []byte(indexDirective.ConfigJSON))

	// 4. Populate with test data.
	if err := idx.IndexDocuments(context.Background(), testProducts()); err != nil {
		t.Fatalf("populate test data: %v", err)
	}
	if setup.Hooks.WaitForIndex != nil {
		setup.Hooks.WaitForIndex(t)
	}

	// 5. Start the entity subgraph server.
	entityServer := httptest.NewServer(productdetails.Handler())
	t.Cleanup(entityServer.Close)

	// 6. Compose the subgraphs.
	entitySDL := entitySubgraphSDL(t)
	routerConfig := composeSubgraphs(t, searchSDL, entitySDL, entityServer.URL)

	// 7. Build the plan configuration.
	searchConfig := entityToConfiguration(entity)
	supergraphDef := routerConfig.EngineConfig.GraphqlSchema
	planConfig := buildPlanConfiguration(t, routerConfig, idx, searchConfig, entityServer.URL, nil)

	return testEnv{
		Pipeline: &testPipeline{
			PlanConfig:    planConfig,
			SupergraphDef: supergraphDef,
		},
		SupergraphDef: supergraphDef,
		DefaultSort:   `[{"field": "PRICE", "direction": "ASC"}]`,
	}
}

// RunAllScenarios orchestrates the full e2e test pipeline for a given backend.
func RunAllScenarios(t *testing.T, setup BackendSetup) {
	t.Helper()

	env := setupTestEnv(t, setup)
	pipeline := env.Pipeline
	defaultSort := env.DefaultSort

	// Run test scenarios.
	t.Run("supergraph_sdl", func(t *testing.T) {
		t.Parallel()
		assertResponse(t, "supergraph_sdl", setup.ExpectedResponses, env.SupergraphDef)
	})

	t.Run("basic_search_with_entity_join", func(t *testing.T) {
		t.Parallel()
		query := `query($s: [ProductSort!]) { searchProducts(query: "*", sort: $s) { hits { node { id name price manufacturer } } totalCount } }`
		if setup.Caps.HasTextSearch {
			query = `query($s: [ProductSort!]) { searchProducts(query: "shoes", sort: $s) { hits { node { id name price manufacturer } } totalCount } }`
		}
		vars := `{"s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "basic_search_with_entity_join", setup.ExpectedResponses, raw)
	})

	t.Run("filter_keyword_with_entity_join", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) { searchProducts(query: "*", filter: $f, sort: $s) { hits { node { id name rating } } } }`
		vars := `{"f": {"category": {"eq": "Footwear"}}, "s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "filter_keyword_with_entity_join", setup.ExpectedResponses, raw)
	})

	t.Run("filter_boolean", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) { searchProducts(query: "*", filter: $f, sort: $s) { hits { node { id manufacturer } } totalCount } }`
		vars := `{"f": {"inStock": false}, "s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "filter_boolean", setup.ExpectedResponses, raw)
	})

	t.Run("filter_numeric_range", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) { searchProducts(query: "*", filter: $f, sort: $s) { hits { node { id manufacturer } } totalCount } }`
		vars := `{"f": {"price": {"gte": 30, "lte": 100}}, "s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "filter_numeric_range", setup.ExpectedResponses, raw)
	})

	t.Run("filter_AND", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) { searchProducts(query: "*", filter: $f, sort: $s) { hits { node { id manufacturer } } totalCount } }`
		vars := `{"f": {"AND": [{"category": {"eq": "Footwear"}}, {"inStock": true}]}, "s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "filter_AND", setup.ExpectedResponses, raw)
	})

	t.Run("filter_OR", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) { searchProducts(query: "*", filter: $f, sort: $s) { hits { node { id manufacturer } } totalCount } }`
		vars := `{"f": {"OR": [{"category": {"eq": "Accessories"}}, {"price": {"gte": 100}}]}, "s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "filter_OR", setup.ExpectedResponses, raw)
	})

	t.Run("filter_NOT", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) { searchProducts(query: "*", filter: $f, sort: $s) { hits { node { id manufacturer } } totalCount } }`
		vars := `{"f": {"NOT": {"category": {"eq": "Footwear"}}}, "s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "filter_NOT", setup.ExpectedResponses, raw)
	})

	t.Run("sort_with_entity_join", func(t *testing.T) {
		t.Parallel()
		query := `query($s: [ProductSort!]) { searchProducts(query: "*", sort: $s) { hits { node { id name price manufacturer } } } }`
		vars := `{"s": [{"field": "PRICE", "direction": "ASC"}]}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "sort_with_entity_join", setup.ExpectedResponses, raw)
	})

	t.Run("pagination_with_entity_join", func(t *testing.T) {
		t.Parallel()
		query := `query($s: [ProductSort!], $lim: Int, $off: Int) {
			searchProducts(query: "*", sort: $s, limit: $lim, offset: $off) {
				hits { node { id reviews { text stars } } }
				totalCount
			}
		}`
		vars := `{"s": [{"field": "PRICE", "direction": "ASC"}], "lim": 2, "off": 1}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "pagination_with_entity_join", setup.ExpectedResponses, raw)
	})

	t.Run("score_and_totalCount", func(t *testing.T) {
		t.Parallel()
		query := `query($s: [ProductSort!]) { searchProducts(query: "*", sort: $s) { hits { score node { id manufacturer } } totalCount } }`
		vars := `{"s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "score_and_totalCount", setup.ExpectedResponses, raw)
	})

	if setup.Caps.HasFacets {
		t.Run("facets_with_entity_join", func(t *testing.T) {
			t.Parallel()
			query := `query($fac: [String!], $s: [ProductSort!]) { searchProducts(query: "*", facets: $fac, sort: $s) { hits { node { id manufacturer } } facets { field values { value count } } } }`
			vars := `{"fac": ["category"], "s": ` + defaultSort + `}`
			raw := executeQuery(t, pipeline, query, vars)
			assertResponse(t, "facets_with_entity_join", setup.ExpectedResponses, raw)
		})
	}
}

// RunInlineScenarios runs e2e scenarios for inline style (no wrapper types).
func RunInlineScenarios(t *testing.T, setup BackendSetup) {
	t.Helper()

	env := setupTestEnv(t, setup)
	pipeline := env.Pipeline
	defaultSort := env.DefaultSort

	t.Run("supergraph_sdl", func(t *testing.T) {
		t.Parallel()
		assertResponse(t, "supergraph_sdl", setup.ExpectedResponses, env.SupergraphDef)
	})

	t.Run("basic_search_inline", func(t *testing.T) {
		t.Parallel()
		query := `query($s: [ProductSort!]) { searchProducts(query: "*", sort: $s) { id name price manufacturer } }`
		vars := `{"s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "basic_search_inline", setup.ExpectedResponses, raw)
	})

	t.Run("filter_keyword_inline", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) { searchProducts(query: "*", filter: $f, sort: $s) { id name } }`
		vars := `{"f": {"category": {"eq": "Footwear"}}, "s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "filter_keyword_inline", setup.ExpectedResponses, raw)
	})

	t.Run("filter_boolean_inline", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) { searchProducts(query: "*", filter: $f, sort: $s) { id manufacturer } }`
		vars := `{"f": {"inStock": false}, "s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "filter_boolean_inline", setup.ExpectedResponses, raw)
	})

	t.Run("filter_numeric_range_inline", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) { searchProducts(query: "*", filter: $f, sort: $s) { id manufacturer } }`
		vars := `{"f": {"price": {"gte": 30, "lte": 100}}, "s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "filter_numeric_range_inline", setup.ExpectedResponses, raw)
	})

	t.Run("filter_AND_inline", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) { searchProducts(query: "*", filter: $f, sort: $s) { id manufacturer } }`
		vars := `{"f": {"AND": [{"category": {"eq": "Footwear"}}, {"inStock": true}]}, "s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "filter_AND_inline", setup.ExpectedResponses, raw)
	})

	t.Run("filter_OR_inline", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) { searchProducts(query: "*", filter: $f, sort: $s) { id manufacturer } }`
		vars := `{"f": {"OR": [{"category": {"eq": "Accessories"}}, {"price": {"gte": 100}}]}, "s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "filter_OR_inline", setup.ExpectedResponses, raw)
	})

	t.Run("filter_NOT_inline", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) { searchProducts(query: "*", filter: $f, sort: $s) { id manufacturer } }`
		vars := `{"f": {"NOT": {"category": {"eq": "Footwear"}}}, "s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "filter_NOT_inline", setup.ExpectedResponses, raw)
	})

	t.Run("sort_inline", func(t *testing.T) {
		t.Parallel()
		query := `query($s: [ProductSort!]) { searchProducts(query: "*", sort: $s) { id name price manufacturer } }`
		vars := `{"s": [{"field": "PRICE", "direction": "ASC"}]}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "sort_inline", setup.ExpectedResponses, raw)
	})

	t.Run("pagination_inline", func(t *testing.T) {
		t.Parallel()
		query := `query($s: [ProductSort!], $lim: Int, $off: Int) { searchProducts(query: "*", sort: $s, limit: $lim, offset: $off) { id name } }`
		vars := `{"s": [{"field": "PRICE", "direction": "ASC"}], "lim": 2, "off": 1}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "pagination_inline", setup.ExpectedResponses, raw)
	})
}

// RunCursorScenarios runs cursor-based pagination e2e scenarios for a given backend.
// Validates structure and content dynamically because cursor values depend on backend internals.
func RunCursorScenarios(t *testing.T, setup BackendSetup) {
	t.Helper()

	env := setupTestEnv(t, setup)
	pipeline := env.Pipeline

	t.Run("cursor_forward_page1", func(t *testing.T) {
		t.Parallel()
		gqlQuery := `query($s: [ProductSort!], $first: Int) {
			searchProducts(query: "*", sort: $s, first: $first) {
				edges { cursor node { id name price manufacturer } }
				pageInfo { hasNextPage hasPreviousPage startCursor endCursor }
				totalCount
			}
		}`
		vars := `{"s": [{"field": "PRICE", "direction": "ASC"}], "first": 2}`
		raw := executeQuery(t, pipeline, gqlQuery, vars)
		assertCursorResponse(t, raw, cursorExpectation{
			edgeCount:       2,
			firstNodeID:     "4", // Wool Socks
			lastNodeID:      "3", // Leather Belt
			hasNextPage:     true,
			hasPreviousPage: false,
			totalCount:      4,
			checkCursors:    true,
		})
	})

	t.Run("cursor_forward_page2", func(t *testing.T) {
		t.Parallel()
		// Get page 1 to obtain cursor.
		gqlQuery1 := `query($s: [ProductSort!], $first: Int) {
			searchProducts(query: "*", sort: $s, first: $first) {
				edges { cursor node { id } }
				pageInfo { endCursor }
			}
		}`
		vars1 := `{"s": [{"field": "PRICE", "direction": "ASC"}], "first": 2}`
		raw1 := executeQuery(t, pipeline, gqlQuery1, vars1)
		endCursor := extractEndCursor(t, raw1)

		gqlQuery2 := `query($s: [ProductSort!], $first: Int, $after: String) {
			searchProducts(query: "*", sort: $s, first: $first, after: $after) {
				edges { cursor node { id name manufacturer } }
				pageInfo { hasNextPage hasPreviousPage }
				totalCount
			}
		}`
		vars2 := fmt.Sprintf(`{"s": [{"field": "PRICE", "direction": "ASC"}], "first": 2, "after": %q}`, endCursor)
		raw2 := executeQuery(t, pipeline, gqlQuery2, vars2)
		assertCursorResponse(t, raw2, cursorExpectation{
			edgeCount:       2,
			firstNodeID:     "1", // Running Shoes
			lastNodeID:      "2", // Basketball Shoes
			hasNextPage:     false,
			hasPreviousPage: true,
			totalCount:      4,
		})
	})

	t.Run("cursor_entity_join", func(t *testing.T) {
		t.Parallel()
		gqlQuery := `query($s: [ProductSort!], $first: Int) {
			searchProducts(query: "*", sort: $s, first: $first) {
				edges { node { id manufacturer } }
			}
		}`
		vars := `{"s": [{"field": "PRICE", "direction": "ASC"}], "first": 1}`
		raw := executeQuery(t, pipeline, gqlQuery, vars)
		assertCursorResponse(t, raw, cursorExpectation{
			edgeCount:   1,
			firstNodeID: "4",
		})
		assertContainsJSON(t, raw, `"manufacturer":"Smartwool"`)
	})
}

type cursorExpectation struct {
	edgeCount       int
	firstNodeID     string
	lastNodeID      string
	hasNextPage     bool
	hasPreviousPage bool
	totalCount      int
	checkCursors    bool // when true, asserts that each edge has a non-empty cursor
}

func assertCursorResponse(t *testing.T, raw string, expect cursorExpectation) {
	t.Helper()
	var resp struct {
		Data struct {
			SearchProducts struct {
				Edges []struct {
					Cursor string `json:"cursor"`
					Node   struct {
						ID string `json:"id"`
					} `json:"node"`
				} `json:"edges"`
				PageInfo struct {
					HasNextPage     bool    `json:"hasNextPage"`
					HasPreviousPage bool    `json:"hasPreviousPage"`
					StartCursor     *string `json:"startCursor"`
					EndCursor       *string `json:"endCursor"`
				} `json:"pageInfo"`
				TotalCount int `json:"totalCount"`
			} `json:"searchProducts"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nraw: %s", err, raw)
	}
	sp := resp.Data.SearchProducts
	if len(sp.Edges) != expect.edgeCount {
		t.Errorf("expected %d edges, got %d\nraw: %s", expect.edgeCount, len(sp.Edges), raw)
		return
	}
	if expect.firstNodeID != "" && len(sp.Edges) > 0 && sp.Edges[0].Node.ID != expect.firstNodeID {
		t.Errorf("first edge id=%q, want %q\nraw: %s", sp.Edges[0].Node.ID, expect.firstNodeID, raw)
	}
	if expect.lastNodeID != "" && len(sp.Edges) > 1 && sp.Edges[len(sp.Edges)-1].Node.ID != expect.lastNodeID {
		t.Errorf("last edge id=%q, want %q\nraw: %s", sp.Edges[len(sp.Edges)-1].Node.ID, expect.lastNodeID, raw)
	}
	if sp.PageInfo.HasNextPage != expect.hasNextPage {
		t.Errorf("hasNextPage=%v, want %v\nraw: %s", sp.PageInfo.HasNextPage, expect.hasNextPage, raw)
	}
	if sp.PageInfo.HasPreviousPage != expect.hasPreviousPage {
		t.Errorf("hasPreviousPage=%v, want %v\nraw: %s", sp.PageInfo.HasPreviousPage, expect.hasPreviousPage, raw)
	}
	if expect.totalCount > 0 && sp.TotalCount != expect.totalCount {
		t.Errorf("totalCount=%d, want %d\nraw: %s", sp.TotalCount, expect.totalCount, raw)
	}
	if expect.checkCursors {
		for i, edge := range sp.Edges {
			if edge.Cursor == "" {
				t.Errorf("edge[%d] has empty cursor\nraw: %s", i, raw)
			}
		}
	}
}

func extractEndCursor(t *testing.T, raw string) string {
	t.Helper()
	var resp struct {
		Data struct {
			SearchProducts struct {
				PageInfo struct {
					EndCursor *string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"searchProducts"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal response for cursor extraction: %v\nraw: %s", err, raw)
	}
	if resp.Data.SearchProducts.PageInfo.EndCursor == nil {
		t.Fatalf("endCursor is null\nraw: %s", raw)
	}
	return *resp.Data.SearchProducts.PageInfo.EndCursor
}

func assertContainsJSON(t *testing.T, raw, substr string) {
	t.Helper()
	if !strings.Contains(raw, substr) {
		t.Errorf("expected response to contain %q\nraw: %s", substr, raw)
	}
}

func assertResponse(t *testing.T, testName string, expected map[string]string, got string) {
	t.Helper()
	want, ok := expected[testName]
	if !ok {
		t.Fatalf("no expected response for %q (got: %s)", testName, got)
		return
	}
	if got != want {
		t.Fatalf("response mismatch\ngot:  %s\nwant: %s", got, want)
	}
}

// --- Vector search test infrastructure ---

// vectorConfigSDL returns a config SDL with @embedding for the given backend.
func vectorConfigSDL(backend, configJSON string) string {
	return fmt.Sprintf(`
extend schema @index(name: "products", backend: "%s", config: "%s")

type Product @key(fields: "id") @searchable(index: "products", searchField: "searchProducts") {
  id: ID!
  name: String @indexed(type: TEXT, filterable: true, sortable: true)
  description: String @indexed(type: TEXT)
  category: String @indexed(type: KEYWORD, filterable: true, sortable: true)
  price: Float @indexed(type: NUMERIC, filterable: true, sortable: true)
  inStock: Boolean @indexed(type: BOOL, filterable: true)
  _embedding: [Float!] @embedding(fields: "name description", template: "{{name}}. {{description}}", model: "test-model")
}
`, backend, configJSON)
}

// VectorBackendSetup extends BackendSetup with an embedder for vector search tests.
type VectorBackendSetup struct {
	BackendSetup
	Embedder searchindex.Embedder
}

// setupVectorTestEnv is like setupTestEnv but populates vector data and wires up the embedder.
func setupVectorTestEnv(t *testing.T, setup VectorBackendSetup) testEnv {
	t.Helper()

	// 1. Parse the config schema SDL.
	doc, parseReport := astparser.ParseGraphqlDocumentString(setup.ConfigSDL)
	if parseReport.HasErrors() {
		t.Fatalf("parse config schema: %s", parseReport.Error())
	}
	parsedConfig, err := search_datasource.ParseConfigSchema(&doc)
	if err != nil {
		t.Fatalf("ParseConfigSchema: %v", err)
	}
	if len(parsedConfig.Entities) == 0 {
		t.Fatal("no entities found in config schema")
	}

	// 2. Generate the search subgraph SDL.
	searchSDL, err := search_datasource.GenerateSubgraphSDL(parsedConfig)
	if err != nil {
		t.Fatalf("GenerateSubgraphSDL: %v", err)
	}

	entity := &parsedConfig.Entities[0]
	indexDirective := parsedConfig.Indices[0]

	// 3. Build index schema and set vector dimensions from embedder.
	indexSchema := buildIndexSchema(indexDirective.Name, entity)
	for i, f := range indexSchema.Fields {
		if f.Type == searchindex.FieldTypeVector && f.Dimensions == 0 {
			indexSchema.Fields[i].Dimensions = setup.Embedder.Dimensions()
		}
	}

	idx := setup.CreateIndex(t, fmt.Sprintf("test_%s_vector", setup.Name), indexSchema, []byte(indexDirective.ConfigJSON))

	// 4. Populate with vector test data.
	if err := idx.IndexDocuments(context.Background(), testVectorProducts(setup.Embedder)); err != nil {
		t.Fatalf("populate vector test data: %v", err)
	}
	if setup.Hooks.WaitForIndex != nil {
		setup.Hooks.WaitForIndex(t)
	}

	// 5. Start the entity subgraph server.
	entityServer := httptest.NewServer(productdetails.Handler())
	t.Cleanup(entityServer.Close)

	// 6. Compose the subgraphs.
	entitySDL := entitySubgraphSDL(t)
	routerConfig := composeSubgraphs(t, searchSDL, entitySDL, entityServer.URL)

	// 7. Build the plan configuration with embedder registry.
	searchConfig := entityToConfiguration(entity)
	supergraphDef := routerConfig.EngineConfig.GraphqlSchema

	embedderRegistry := searchindex.NewEmbedderRegistry()
	if len(searchConfig.EmbeddingFields) > 0 {
		embedderRegistry.Register(searchConfig.EmbeddingFields[0].Model, setup.Embedder)
	}

	planConfig := buildPlanConfiguration(t, routerConfig, idx, searchConfig, entityServer.URL, embedderRegistry)

	return testEnv{
		Pipeline: &testPipeline{
			PlanConfig:    planConfig,
			SupergraphDef: supergraphDef,
		},
		SupergraphDef: supergraphDef,
		DefaultSort:   `[{"field": "PRICE", "direction": "ASC"}]`,
	}
}

// RunVectorScenarios runs e2e vector search scenarios for a given backend.
// Uses structural assertions since distance values are backend-specific.
func RunVectorScenarios(t *testing.T, setup VectorBackendSetup) {
	t.Helper()

	env := setupVectorTestEnv(t, setup)
	pipeline := env.Pipeline
	defaultSort := env.DefaultSort

	t.Run("vector_text_query_auto_embed", func(t *testing.T) {
		t.Parallel()
		// Uses search: {query: "..."} which Source auto-embeds via the mock embedder.
		query := `query($s: [ProductSort!]) {
			searchProducts(search: {query: "shoes for running"}, sort: $s) {
				hits { score distance node { id name price manufacturer } }
				totalCount
			}
		}`
		vars := `{"s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertVectorResponse(t, raw, vectorExpectation{
			minHits:    1,
			totalCount: 4,
			hasEntityJoin: map[string]string{
				"manufacturer": "",
			},
		})
	})

	t.Run("vector_raw_vector_query", func(t *testing.T) {
		t.Parallel()
		// Use product 1's exact vector — it should be the closest match (distance ≈ 0).
		vec, err := setup.Embedder.EmbedSingle(context.Background(), "Running Shoes. Great for jogging and marathons")
		if err != nil {
			t.Fatalf("embed: %v", err)
		}
		vecJSON := formatVectorJSON(vec)
		query := `query($search: SearchProductInput!, $s: [ProductSort!]) {
			searchProducts(search: $search, sort: $s) {
				hits { distance node { id name } }
				totalCount
			}
		}`
		vars := fmt.Sprintf(`{"search": {"vector": %s}, "s": %s}`, vecJSON, defaultSort)
		raw := executeQuery(t, pipeline, query, vars)
		assertVectorResponse(t, raw, vectorExpectation{
			minHits:    1,
			totalCount: 4,
		})
	})

	t.Run("vector_with_filter", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) {
			searchProducts(search: {query: "shoes"}, filter: $f, sort: $s) {
				hits { node { id name manufacturer } }
				totalCount
			}
		}`
		vars := `{"f": {"category": {"eq": "Footwear"}}, "s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertVectorResponse(t, raw, vectorExpectation{
			minHits:       1,
			maxTotalCount: 3, // at most 3 footwear products
			allMatchFilter: func(node map[string]any) bool {
				// All returned nodes should be footwear (resolved via entity join)
				return node["id"] != "3" // id=3 is the Leather Belt (Accessories)
			},
		})
	})

	t.Run("vector_distance_populated", func(t *testing.T) {
		t.Parallel()
		query := `query {
			searchProducts(search: {query: "running shoes"}) {
				hits { distance node { id } }
			}
		}`
		raw := executeQuery(t, pipeline, query, "")
		assertVectorDistances(t, raw)
	})

	t.Run("vector_entity_join", func(t *testing.T) {
		t.Parallel()
		// Verify federation entity join works with vector search results.
		query := `query($s: [ProductSort!]) {
			searchProducts(search: {query: "socks"}, sort: $s) {
				hits { node { id manufacturer rating } }
			}
		}`
		vars := `{"s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertVectorResponse(t, raw, vectorExpectation{
			minHits: 1,
			hasEntityJoin: map[string]string{
				"manufacturer": "",
				"rating":       "",
			},
		})
	})
}

// vectorExpectation defines structural assertions for vector search responses.
type vectorExpectation struct {
	minHits        int
	totalCount     int                    // exact expected totalCount (0 = skip check)
	maxTotalCount  int                    // max expected totalCount (0 = skip check)
	hasEntityJoin  map[string]string      // fields that should be present in nodes (value ignored)
	allMatchFilter func(map[string]any) bool // if set, all nodes must pass this
}

func assertVectorResponse(t *testing.T, raw string, expect vectorExpectation) {
	t.Helper()
	var resp struct {
		Data struct {
			SearchProducts struct {
				Hits []struct {
					Score    float64        `json:"score"`
					Distance float64        `json:"distance"`
					Node     map[string]any `json:"node"`
				} `json:"hits"`
				TotalCount int `json:"totalCount"`
			} `json:"searchProducts"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, raw)
	}
	sp := resp.Data.SearchProducts
	if len(sp.Hits) < expect.minHits {
		t.Errorf("expected at least %d hits, got %d\nraw: %s", expect.minHits, len(sp.Hits), raw)
	}
	if expect.totalCount > 0 && sp.TotalCount != expect.totalCount {
		t.Errorf("totalCount=%d, want %d\nraw: %s", sp.TotalCount, expect.totalCount, raw)
	}
	if expect.maxTotalCount > 0 && sp.TotalCount > expect.maxTotalCount {
		t.Errorf("totalCount=%d, want at most %d\nraw: %s", sp.TotalCount, expect.maxTotalCount, raw)
	}
	for field := range expect.hasEntityJoin {
		for i, hit := range sp.Hits {
			if _, ok := hit.Node[field]; !ok {
				t.Errorf("hit[%d] missing entity join field %q\nraw: %s", i, field, raw)
			}
		}
	}
	if expect.allMatchFilter != nil {
		for i, hit := range sp.Hits {
			if !expect.allMatchFilter(hit.Node) {
				t.Errorf("hit[%d] failed filter assertion\nnode: %v\nraw: %s", i, hit.Node, raw)
			}
		}
	}
}

func assertVectorDistances(t *testing.T, raw string) {
	t.Helper()
	var resp struct {
		Data struct {
			SearchProducts struct {
				Hits []struct {
					Distance float64 `json:"distance"`
					Node     struct {
						ID string `json:"id"`
					} `json:"node"`
				} `json:"hits"`
			} `json:"searchProducts"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, raw)
	}
	if len(resp.Data.SearchProducts.Hits) == 0 {
		t.Fatalf("expected hits, got 0\nraw: %s", raw)
	}
}

func formatVectorJSON(vec []float32) string {
	b, _ := json.Marshal(vec)
	return string(b)
}

// --- Geo search test infrastructure ---

// boostConfigSDL returns a config SDL with name field boosted to weight 2.0 for the given backend.
func boostConfigSDL(backend, configJSON string) string {
	return fmt.Sprintf(`
extend schema @index(name: "products", backend: "%s", config: "%s")

type Product @key(fields: "id") @searchable(index: "products", searchField: "searchProducts") {
  id: ID!
  name: String @indexed(type: TEXT, filterable: true, sortable: true, weight: 2.0)
  description: String @indexed(type: TEXT)
  category: String @indexed(type: KEYWORD, filterable: true, sortable: true)
  price: Float @indexed(type: NUMERIC, filterable: true, sortable: true)
  inStock: Boolean @indexed(type: BOOL, filterable: true)
}
`, backend, configJSON)
}

// geoConfigSDL returns a config SDL with a location GEO field for the given backend.
func geoConfigSDL(backend, configJSON string) string {
	return fmt.Sprintf(`
extend schema @index(name: "products", backend: "%s", config: "%s")

type Product @key(fields: "id") @searchable(index: "products", searchField: "searchProducts") {
  id: ID!
  name: String @indexed(type: TEXT, filterable: true, sortable: true)
  description: String @indexed(type: TEXT)
  category: String @indexed(type: KEYWORD, filterable: true, sortable: true)
  price: Float @indexed(type: NUMERIC, filterable: true, sortable: true)
  inStock: Boolean @indexed(type: BOOL, filterable: true)
  location: GeoPoint @indexed(type: GEO, filterable: true, sortable: true)
}
`, backend, configJSON)
}

// GeoBackendSetup extends BackendSetup for geo tests.
type GeoBackendSetup struct {
	BackendSetup
}

// setupGeoTestEnv is like setupTestEnv but populates with geo data.
func setupGeoTestEnv(t *testing.T, setup GeoBackendSetup) testEnv {
	t.Helper()

	doc, parseReport := astparser.ParseGraphqlDocumentString(setup.ConfigSDL)
	if parseReport.HasErrors() {
		t.Fatalf("parse config schema: %s", parseReport.Error())
	}
	parsedConfig, err := search_datasource.ParseConfigSchema(&doc)
	if err != nil {
		t.Fatalf("ParseConfigSchema: %v", err)
	}
	if len(parsedConfig.Entities) == 0 {
		t.Fatal("no entities found in config schema")
	}

	searchSDL, err := search_datasource.GenerateSubgraphSDL(parsedConfig)
	if err != nil {
		t.Fatalf("GenerateSubgraphSDL: %v", err)
	}

	entity := &parsedConfig.Entities[0]
	indexDirective := parsedConfig.Indices[0]

	indexSchema := buildIndexSchema(indexDirective.Name, entity)
	idx := setup.CreateIndex(t, fmt.Sprintf("test_%s_geo", setup.Name), indexSchema, []byte(indexDirective.ConfigJSON))

	if err := idx.IndexDocuments(context.Background(), testGeoProducts()); err != nil {
		t.Fatalf("populate geo test data: %v", err)
	}
	if setup.Hooks.WaitForIndex != nil {
		setup.Hooks.WaitForIndex(t)
	}

	entityServer := httptest.NewServer(productdetails.Handler())
	t.Cleanup(entityServer.Close)

	entitySDL := entitySubgraphSDL(t)
	routerConfig := composeSubgraphs(t, searchSDL, entitySDL, entityServer.URL)

	searchConfig := entityToConfiguration(entity)
	supergraphDef := routerConfig.EngineConfig.GraphqlSchema
	planConfig := buildPlanConfiguration(t, routerConfig, idx, searchConfig, entityServer.URL, nil)

	return testEnv{
		Pipeline: &testPipeline{
			PlanConfig:    planConfig,
			SupergraphDef: supergraphDef,
		},
		SupergraphDef: supergraphDef,
		DefaultSort:   `[{"field": "PRICE", "direction": "ASC"}]`,
	}
}

// RunGeoScenarios runs full-stack e2e geo-spatial search scenarios with federation entity joins.
// Products are at:
//
//	#1 Running Shoes:    New York (40.7128, -74.0060)
//	#2 Basketball Shoes: Midtown Manhattan (40.7580, -73.9855) — ~5km from #1
//	#3 Leather Belt:     Los Angeles (34.0522, -118.2437) — ~3,940km from #1
//	#4 Wool Socks:       London (51.5074, -0.1278) — ~5,570km from #1
func RunGeoScenarios(t *testing.T, setup GeoBackendSetup) {
	t.Helper()

	env := setupGeoTestEnv(t, setup)
	pipeline := env.Pipeline

	t.Run("geo_distance_filter_with_entity_join", func(t *testing.T) {
		t.Parallel()
		// Search within 10km of New York — should find #1 and #2.
		query := `query($f: ProductFilter, $s: [ProductSort!]) {
			searchProducts(query: "*", filter: $f, sort: $s) {
				hits { node { id name manufacturer } }
				totalCount
			}
		}`
		vars := `{"f": {"location_distance": {"center": {"lat": 40.7128, "lon": -74.0060}, "distance": "10km"}}, "s": [{"field": "PRICE", "direction": "ASC"}]}`
		raw := executeQuery(t, pipeline, query, vars)
		assertGeoResponse(t, raw, geoExpectation{hitCount: 2})
	})

	t.Run("geo_distance_filter_wide", func(t *testing.T) {
		t.Parallel()
		// Search within 5000km of New York — should find #1, #2, #3 (not London).
		query := `query($f: ProductFilter, $s: [ProductSort!]) {
			searchProducts(query: "*", filter: $f, sort: $s) {
				hits { node { id manufacturer } }
				totalCount
			}
		}`
		vars := `{"f": {"location_distance": {"center": {"lat": 40.7128, "lon": -74.0060}, "distance": "5000km"}}, "s": [{"field": "PRICE", "direction": "ASC"}]}`
		raw := executeQuery(t, pipeline, query, vars)
		assertGeoResponse(t, raw, geoExpectation{hitCount: 3})
	})

	t.Run("geo_bounding_box_filter", func(t *testing.T) {
		t.Parallel()
		// Bounding box around NYC area — should find #1 and #2.
		query := `query($f: ProductFilter, $s: [ProductSort!]) {
			searchProducts(query: "*", filter: $f, sort: $s) {
				hits { node { id name manufacturer } }
				totalCount
			}
		}`
		vars := `{"f": {"location_boundingBox": {"topLeft": {"lat": 41.0, "lon": -74.5}, "bottomRight": {"lat": 40.5, "lon": -73.5}}}, "s": [{"field": "PRICE", "direction": "ASC"}]}`
		raw := executeQuery(t, pipeline, query, vars)
		assertGeoResponse(t, raw, geoExpectation{hitCount: 2})
	})

	t.Run("geo_distance_sort_with_entity_join", func(t *testing.T) {
		t.Parallel()
		// Sort by distance from NYC ASC.
		query := `query($gs: GeoDistanceSortInput) {
			searchProducts(query: "*", geoSort: $gs) {
				hits { geoDistance node { id name manufacturer } }
				totalCount
			}
		}`
		vars := `{"gs": {"field": "location", "center": {"lat": 40.7128, "lon": -74.0060}, "direction": "ASC", "unit": "km"}}`
		raw := executeQuery(t, pipeline, query, vars)
		assertGeoSortResponse(t, raw)
	})

	t.Run("geo_filter_combined_with_keyword", func(t *testing.T) {
		t.Parallel()
		// Footwear within 100km of NYC.
		query := `query($f: ProductFilter, $s: [ProductSort!]) {
			searchProducts(query: "*", filter: $f, sort: $s) {
				hits { node { id name manufacturer } }
				totalCount
			}
		}`
		vars := `{"f": {"location_distance": {"center": {"lat": 40.7128, "lon": -74.0060}, "distance": "100km"}, "category": {"eq": "Footwear"}}, "s": [{"field": "PRICE", "direction": "ASC"}]}`
		raw := executeQuery(t, pipeline, query, vars)
		assertGeoResponse(t, raw, geoExpectation{hitCount: 2})
	})
}

type geoExpectation struct {
	hitCount int
}

func assertGeoResponse(t *testing.T, raw string, expect geoExpectation) {
	t.Helper()
	var resp struct {
		Data struct {
			SearchProducts struct {
				Hits       []any `json:"hits"`
				TotalCount int   `json:"totalCount"`
			} `json:"searchProducts"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, raw)
	}
	if resp.Data.SearchProducts.TotalCount != expect.hitCount {
		t.Errorf("expected totalCount=%d, got %d\nraw: %s", expect.hitCount, resp.Data.SearchProducts.TotalCount, raw)
	}
}

func assertGeoSortResponse(t *testing.T, raw string) {
	t.Helper()
	var resp struct {
		Data struct {
			SearchProducts struct {
				Hits []struct {
					GeoDistance *float64       `json:"geoDistance"`
					Node       map[string]any `json:"node"`
				} `json:"hits"`
				TotalCount int `json:"totalCount"`
			} `json:"searchProducts"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, raw)
	}
	sp := resp.Data.SearchProducts
	if len(sp.Hits) < 4 {
		t.Fatalf("expected >= 4 hits, got %d\nraw: %s", len(sp.Hits), raw)
	}
	// Nearest to NYC should be Running Shoes.
	name, _ := sp.Hits[0].Node["name"].(string)
	if name != "Running Shoes" {
		t.Errorf("expected first hit to be Running Shoes (nearest to NYC), got %q\nraw: %s", name, raw)
	}
	// Farthest should be Wool Socks (London).
	lastName, _ := sp.Hits[3].Node["name"].(string)
	if lastName != "Wool Socks" {
		t.Errorf("expected last hit to be Wool Socks (London, farthest), got %q\nraw: %s", lastName, raw)
	}
	// All hits should have geoDistance populated.
	for i, hit := range sp.Hits {
		if hit.GeoDistance == nil {
			t.Errorf("hit[%d]: expected geoDistance to be populated\nraw: %s", i, raw)
		}
	}
	// Entity join should have resolved manufacturer.
	for i, hit := range sp.Hits {
		if _, ok := hit.Node["manufacturer"]; !ok {
			t.Errorf("hit[%d]: missing entity join field 'manufacturer'\nraw: %s", i, raw)
		}
	}
}

// --- Date/DateTime test infrastructure ---

// dateConfigSDL returns a config SDL with Date and DateTime fields for the given backend.
func dateConfigSDL(backend, configJSON string) string {
	return fmt.Sprintf(`
extend schema @index(name: "products", backend: "%s", config: "%s")

type Product @key(fields: "id") @searchable(index: "products", searchField: "searchProducts") {
  id: ID!
  name: String @indexed(type: TEXT, filterable: true, sortable: true)
  description: String @indexed(type: TEXT)
  category: String @indexed(type: KEYWORD, filterable: true, sortable: true)
  price: Float @indexed(type: NUMERIC, filterable: true, sortable: true)
  inStock: Boolean @indexed(type: BOOL, filterable: true)
  createdAt: Date @indexed(type: DATE, filterable: true, sortable: true)
  updatedAt: DateTime @indexed(type: DATETIME, filterable: true, sortable: true)
}
`, backend, configJSON)
}

// setupDateTestEnv is like setupTestEnv but populates with date data.
func setupDateTestEnv(t *testing.T, setup BackendSetup) testEnv {
	t.Helper()

	doc, parseReport := astparser.ParseGraphqlDocumentString(setup.ConfigSDL)
	if parseReport.HasErrors() {
		t.Fatalf("parse config schema: %s", parseReport.Error())
	}
	parsedConfig, err := search_datasource.ParseConfigSchema(&doc)
	if err != nil {
		t.Fatalf("ParseConfigSchema: %v", err)
	}
	if len(parsedConfig.Entities) == 0 {
		t.Fatal("no entities found in config schema")
	}

	searchSDL, err := search_datasource.GenerateSubgraphSDL(parsedConfig)
	if err != nil {
		t.Fatalf("GenerateSubgraphSDL: %v", err)
	}

	entity := &parsedConfig.Entities[0]
	indexDirective := parsedConfig.Indices[0]

	indexSchema := buildIndexSchema(indexDirective.Name, entity)
	idx := setup.CreateIndex(t, fmt.Sprintf("test_%s_date", setup.Name), indexSchema, []byte(indexDirective.ConfigJSON))

	if err := idx.IndexDocuments(context.Background(), testDateProducts()); err != nil {
		t.Fatalf("populate date test data: %v", err)
	}
	if setup.Hooks.WaitForIndex != nil {
		setup.Hooks.WaitForIndex(t)
	}

	entityServer := httptest.NewServer(productdetails.Handler())
	t.Cleanup(entityServer.Close)

	entitySDL := entitySubgraphSDL(t)
	routerConfig := composeSubgraphs(t, searchSDL, entitySDL, entityServer.URL)

	searchConfig := entityToConfiguration(entity)
	supergraphDef := routerConfig.EngineConfig.GraphqlSchema
	planConfig := buildPlanConfiguration(t, routerConfig, idx, searchConfig, entityServer.URL, nil)

	return testEnv{
		Pipeline: &testPipeline{
			PlanConfig:    planConfig,
			SupergraphDef: supergraphDef,
		},
		SupergraphDef: supergraphDef,
		DefaultSort:   `[{"field": "PRICE", "direction": "ASC"}]`,
	}
}

// RunDateScenarios runs full-stack e2e date/datetime filter and sort scenarios.
// Products have:
//
//	#1 Running Shoes:    CreatedAt 2024-01-15, UpdatedAt 2024-01-15T10:30:00Z
//	#2 Basketball Shoes: CreatedAt 2024-03-20, UpdatedAt 2024-03-20T14:00:00Z
//	#3 Leather Belt:     CreatedAt 2024-06-01, UpdatedAt 2024-06-01T09:00:00Z
//	#4 Wool Socks:       CreatedAt 2024-09-10, UpdatedAt 2024-09-10T16:45:00Z
func RunDateScenarios(t *testing.T, setup BackendSetup) {
	t.Helper()

	env := setupDateTestEnv(t, setup)
	pipeline := env.Pipeline

	t.Run("date_eq_filter", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) {
			searchProducts(query: "*", filter: $f, sort: $s) {
				hits { node { id name manufacturer } }
				totalCount
			}
		}`
		vars := `{"f": {"createdAt": {"eq": "2024-01-15"}}, "s": [{"field": "CREATEDAT", "direction": "ASC"}]}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "date_eq_filter", setup.ExpectedResponses, raw)
	})

	t.Run("date_range_gte_lte", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) {
			searchProducts(query: "*", filter: $f, sort: $s) {
				hits { node { id name manufacturer } }
				totalCount
			}
		}`
		vars := `{"f": {"createdAt": {"gte": "2024-01-15", "lte": "2024-06-01"}}, "s": [{"field": "CREATEDAT", "direction": "ASC"}]}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "date_range_gte_lte", setup.ExpectedResponses, raw)
	})

	t.Run("date_gt_lt", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) {
			searchProducts(query: "*", filter: $f, sort: $s) {
				hits { node { id name manufacturer } }
				totalCount
			}
		}`
		vars := `{"f": {"createdAt": {"gt": "2024-01-15", "lt": "2024-09-10"}}, "s": [{"field": "CREATEDAT", "direction": "ASC"}]}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "date_gt_lt", setup.ExpectedResponses, raw)
	})

	t.Run("date_after_before", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) {
			searchProducts(query: "*", filter: $f, sort: $s) {
				hits { node { id name manufacturer } }
				totalCount
			}
		}`
		vars := `{"f": {"createdAt": {"after": "2024-03-20", "before": "2024-09-10"}}, "s": [{"field": "CREATEDAT", "direction": "ASC"}]}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "date_after_before", setup.ExpectedResponses, raw)
	})

	t.Run("datetime_eq_filter", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) {
			searchProducts(query: "*", filter: $f, sort: $s) {
				hits { node { id name manufacturer } }
				totalCount
			}
		}`
		vars := `{"f": {"updatedAt": {"eq": "2024-03-20T14:00:00Z"}}, "s": [{"field": "UPDATEDAT", "direction": "ASC"}]}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "datetime_eq_filter", setup.ExpectedResponses, raw)
	})

	t.Run("datetime_range_gte_lte", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) {
			searchProducts(query: "*", filter: $f, sort: $s) {
				hits { node { id name manufacturer } }
				totalCount
			}
		}`
		vars := `{"f": {"updatedAt": {"gte": "2024-01-15T10:30:00Z", "lte": "2024-06-01T09:00:00Z"}}, "s": [{"field": "UPDATEDAT", "direction": "ASC"}]}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "datetime_range_gte_lte", setup.ExpectedResponses, raw)
	})

	t.Run("datetime_after_before", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) {
			searchProducts(query: "*", filter: $f, sort: $s) {
				hits { node { id name manufacturer } }
				totalCount
			}
		}`
		vars := `{"f": {"updatedAt": {"after": "2024-06-01T09:00:00Z"}}, "s": [{"field": "UPDATEDAT", "direction": "ASC"}]}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "datetime_after_before", setup.ExpectedResponses, raw)
	})

	t.Run("date_sort_asc", func(t *testing.T) {
		t.Parallel()
		query := `query($s: [ProductSort!]) {
			searchProducts(query: "*", sort: $s) {
				hits { node { id name manufacturer } }
			}
		}`
		vars := `{"s": [{"field": "CREATEDAT", "direction": "ASC"}]}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "date_sort_asc", setup.ExpectedResponses, raw)
	})

	t.Run("date_sort_desc", func(t *testing.T) {
		t.Parallel()
		query := `query($s: [ProductSort!]) {
			searchProducts(query: "*", sort: $s) {
				hits { node { id name manufacturer } }
			}
		}`
		vars := `{"s": [{"field": "CREATEDAT", "direction": "DESC"}]}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "date_sort_desc", setup.ExpectedResponses, raw)
	})

	t.Run("datetime_sort_asc", func(t *testing.T) {
		t.Parallel()
		query := `query($s: [ProductSort!]) {
			searchProducts(query: "*", sort: $s) {
				hits { node { id name manufacturer } }
			}
		}`
		vars := `{"s": [{"field": "UPDATEDAT", "direction": "ASC"}]}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "datetime_sort_asc", setup.ExpectedResponses, raw)
	})

	t.Run("date_combined_filter", func(t *testing.T) {
		t.Parallel()
		query := `query($f: ProductFilter, $s: [ProductSort!]) {
			searchProducts(query: "*", filter: $f, sort: $s) {
				hits { node { id name manufacturer } }
				totalCount
			}
		}`
		vars := `{"f": {"AND": [{"category": {"eq": "Footwear"}}, {"createdAt": {"gte": "2024-03-01"}}]}, "s": [{"field": "CREATEDAT", "direction": "ASC"}]}`
		raw := executeQuery(t, pipeline, query, vars)
		assertResponse(t, "date_combined_filter", setup.ExpectedResponses, raw)
	})
}

// --- Highlight test infrastructure ---

// RunHighlightScenarios runs e2e scenarios that exercise search highlights.
// Highlights are backend-dependent — uses structural assertions.
func RunHighlightScenarios(t *testing.T, setup BackendSetup) {
	t.Helper()

	env := setupTestEnv(t, setup)
	pipeline := env.Pipeline
	defaultSort := env.DefaultSort

	t.Run("highlights_returned_for_text_match", func(t *testing.T) {
		t.Parallel()
		query := `query($s: [ProductSort!]) {
			searchProducts(query: "shoes", sort: $s) {
				hits { highlights { field fragments } node { id name } }
				totalCount
			}
		}`
		vars := `{"s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertHighlightResponse(t, raw)
	})
}

func assertHighlightResponse(t *testing.T, raw string) {
	t.Helper()
	var resp struct {
		Data struct {
			SearchProducts struct {
				Hits []struct {
					Highlights []struct {
						Field     string   `json:"field"`
						Fragments []string `json:"fragments"`
					} `json:"highlights"`
					Node map[string]any `json:"node"`
				} `json:"hits"`
				TotalCount int `json:"totalCount"`
			} `json:"searchProducts"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, raw)
	}
	sp := resp.Data.SearchProducts
	if len(sp.Hits) == 0 {
		t.Fatalf("expected at least 1 hit\nraw: %s", raw)
	}
	// At least one hit should have highlights.
	hasHighlights := false
	for _, hit := range sp.Hits {
		if len(hit.Highlights) > 0 {
			hasHighlights = true
			for _, hl := range hit.Highlights {
				if hl.Field == "" {
					t.Errorf("highlight has empty field\nraw: %s", raw)
				}
				if len(hl.Fragments) == 0 {
					t.Errorf("highlight for field %q has no fragments\nraw: %s", hl.Field, raw)
				}
			}
		}
	}
	if !hasHighlights {
		t.Errorf("expected at least one hit to have highlights\nraw: %s", raw)
	}
}

// --- Additional filter test infrastructure ---

// RunAdditionalFilterScenarios runs e2e scenarios for filter operators not covered
// by RunAllScenarios (ne, IN for strings, startsWith).
func RunAdditionalFilterScenarios(t *testing.T, setup BackendSetup) {
	t.Helper()

	env := setupTestEnv(t, setup)
	pipeline := env.Pipeline
	defaultSort := env.DefaultSort

	t.Run("filter_string_ne", func(t *testing.T) {
		t.Parallel()
		// "ne" on keyword — should exclude category "Footwear".
		query := `query($f: ProductFilter, $s: [ProductSort!]) {
			searchProducts(query: "*", filter: $f, sort: $s) {
				hits { node { id name manufacturer } }
				totalCount
			}
		}`
		vars := `{"f": {"category": {"ne": "Footwear"}}, "s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertFilterResponse(t, raw, "filter_string_ne", setup.ExpectedResponses)
	})

	t.Run("filter_string_in", func(t *testing.T) {
		t.Parallel()
		// "in" on keyword — should match Footwear OR Accessories = all 4.
		query := `query($f: ProductFilter, $s: [ProductSort!]) {
			searchProducts(query: "*", filter: $f, sort: $s) {
				hits { node { id manufacturer } }
				totalCount
			}
		}`
		vars := `{"f": {"category": {"in": ["Footwear", "Accessories"]}}, "s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertFilterResponse(t, raw, "filter_string_in", setup.ExpectedResponses)
	})

	if setup.Caps.HasTextSearch {
		t.Run("filter_string_startsWith", func(t *testing.T) {
			t.Parallel()
			// "startsWith" on keyword — should match "Foot*".
			query := `query($f: ProductFilter, $s: [ProductSort!]) {
				searchProducts(query: "*", filter: $f, sort: $s) {
					hits { node { id manufacturer } }
					totalCount
				}
			}`
			vars := `{"f": {"category": {"startsWith": "Foot"}}, "s": ` + defaultSort + `}`
			raw := executeQuery(t, pipeline, query, vars)
			assertFilterResponse(t, raw, "filter_string_startsWith", setup.ExpectedResponses)
		})
	}
}

func assertFilterResponse(t *testing.T, raw, testName string, expected map[string]string) {
	t.Helper()
	want, ok := expected[testName]
	if !ok {
		// If no exact expected response, at least verify it parses.
		var resp struct {
			Data struct {
				SearchProducts struct {
					Hits       []any `json:"hits"`
					TotalCount int   `json:"totalCount"`
				} `json:"searchProducts"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(raw), &resp); err != nil {
			t.Fatalf("response doesn't parse as JSON: %v\nraw: %s", err, raw)
		}
		return
	}
	if raw != want {
		t.Fatalf("response mismatch\ngot:  %s\nwant: %s", raw, want)
	}
}

// RunHybridScenarios runs e2e hybrid search scenarios (text + vector combined).
// It uses the vector test environment where Source auto-embeds text queries
// and sets both TextQuery and Vector on the SearchRequest.
func RunHybridScenarios(t *testing.T, setup VectorBackendSetup) {
	t.Helper()

	env := setupVectorTestEnv(t, setup)
	pipeline := env.Pipeline
	defaultSort := env.DefaultSort

	t.Run("hybrid_text_query_returns_results", func(t *testing.T) {
		t.Parallel()
		// With an embedder configured, search: {query: "..."} sets both TextQuery and Vector.
		query := `query($s: [ProductSort!]) {
			searchProducts(search: {query: "shoes"}, sort: $s) {
				hits { score node { id name } }
				totalCount
			}
		}`
		vars := `{"s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertVectorResponse(t, raw, vectorExpectation{
			minHits: 1,
		})
	})

	t.Run("hybrid_text_relevance", func(t *testing.T) {
		t.Parallel()
		// Search for "running" — Running Shoes should appear in results.
		query := `query {
			searchProducts(search: {query: "running"}) {
				hits { score node { id name } }
				totalCount
			}
		}`
		raw := executeQuery(t, pipeline, query, "")
		assertVectorResponse(t, raw, vectorExpectation{
			minHits: 1,
		})
		// Verify "Running Shoes" (id=1) is in the results.
		assertContainsJSON(t, raw, `"id":"1"`)
	})

	t.Run("hybrid_with_filter", func(t *testing.T) {
		t.Parallel()
		// Hybrid search + category filter.
		query := `query($f: ProductFilter, $s: [ProductSort!]) {
			searchProducts(search: {query: "shoes"}, filter: $f, sort: $s) {
				hits { node { id name } }
				totalCount
			}
		}`
		vars := `{"f": {"category": {"eq": "Footwear"}}, "s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertVectorResponse(t, raw, vectorExpectation{
			minHits:       1,
			maxTotalCount: 3,
			allMatchFilter: func(node map[string]any) bool {
				return node["id"] != "3" // id=3 is Leather Belt (Accessories)
			},
		})
	})

	t.Run("hybrid_entity_join", func(t *testing.T) {
		t.Parallel()
		// Hybrid search with federation entity join.
		query := `query($s: [ProductSort!]) {
			searchProducts(search: {query: "leather"}, sort: $s) {
				hits { node { id name manufacturer rating } }
			}
		}`
		vars := `{"s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertVectorResponse(t, raw, vectorExpectation{
			minHits: 1,
			hasEntityJoin: map[string]string{
				"manufacturer": "",
				"rating":       "",
			},
		})
	})
}

// RunBoostingScenarios runs e2e scenarios that verify field boosting/weights
// flow through the full pipeline (config parsing → SDL generation → composition → execution).
func RunBoostingScenarios(t *testing.T, setup BackendSetup) {
	t.Helper()

	env := setupTestEnv(t, setup)
	pipeline := env.Pipeline
	defaultSort := env.DefaultSort

	t.Run("boosted_search_returns_results", func(t *testing.T) {
		t.Parallel()
		// Verify that a search with weighted fields produces results.
		query := `query($s: [ProductSort!]) {
			searchProducts(query: "shoes", sort: $s) {
				hits { score node { id name } }
				totalCount
			}
		}`
		vars := `{"s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertBoostingResponse(t, raw, 2) // "shoes" in name of products 1 and 2
	})

	t.Run("boosted_search_without_sort", func(t *testing.T) {
		t.Parallel()
		// Verify that a boosted search without explicit sort returns results.
		query := `query {
			searchProducts(query: "leather") {
				hits { score node { id name } }
				totalCount
			}
		}`
		raw := executeQuery(t, pipeline, query, "")
		assertBoostingResponse(t, raw, 1) // "leather" in product 3's name and description
	})
}

func assertBoostingResponse(t *testing.T, raw string, minHits int) {
	t.Helper()
	var resp struct {
		Data struct {
			SearchProducts struct {
				Hits []struct {
					Score float64        `json:"score"`
					Node  map[string]any `json:"node"`
				} `json:"hits"`
				TotalCount int `json:"totalCount"`
			} `json:"searchProducts"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nraw: %s", err, raw)
	}
	hits := resp.Data.SearchProducts.Hits
	if len(hits) < minHits {
		t.Fatalf("expected at least %d hits, got %d\nraw: %s", minHits, len(hits), raw)
	}
}

// RunFuzzyScenarios runs fuzzy matching / typo tolerance tests through the full
// composition + plan + resolve pipeline.
func RunFuzzyScenarios(t *testing.T, setup BackendSetup) {
	t.Helper()

	env := setupTestEnv(t, setup)
	pipeline := env.Pipeline
	defaultSort := env.DefaultSort

	t.Run("fuzzy_low_finds_typo", func(t *testing.T) {
		t.Parallel()
		// "runing" is 1 edit away from "running" — fuzziness LOW should find it.
		query := `query($s: [ProductSort!]) {
			searchProducts(query: "runing", fuzziness: LOW, sort: $s) {
				hits { score node { id name } }
				totalCount
			}
		}`
		vars := `{"s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertFuzzyResponse(t, raw, 1)
	})

	t.Run("fuzzy_exact_misses_typo", func(t *testing.T) {
		t.Parallel()
		// "runing" with fuzziness EXACT should find nothing.
		query := `query($s: [ProductSort!]) {
			searchProducts(query: "runing", fuzziness: EXACT, sort: $s) {
				hits { score node { id name } }
				totalCount
			}
		}`
		vars := `{"s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertFuzzyResponse(t, raw, 0)
	})

	t.Run("fuzzy_high_finds_typo", func(t *testing.T) {
		t.Parallel()
		// "runnin" with fuzziness HIGH should still find results.
		query := `query($s: [ProductSort!]) {
			searchProducts(query: "runnin", fuzziness: HIGH, sort: $s) {
				hits { score node { id name } }
				totalCount
			}
		}`
		vars := `{"s": ` + defaultSort + `}`
		raw := executeQuery(t, pipeline, query, vars)
		assertFuzzyResponse(t, raw, 1)
	})
}

func assertFuzzyResponse(t *testing.T, raw string, expectedMinHits int) {
	t.Helper()
	var resp struct {
		Data struct {
			SearchProducts struct {
				Hits []struct {
					Score float64        `json:"score"`
					Node  map[string]any `json:"node"`
				} `json:"hits"`
				TotalCount int `json:"totalCount"`
			} `json:"searchProducts"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nraw: %s", err, raw)
	}
	hits := resp.Data.SearchProducts.Hits
	if expectedMinHits == 0 {
		if len(hits) != 0 {
			t.Fatalf("expected 0 hits, got %d\nraw: %s", len(hits), raw)
		}
		return
	}
	if len(hits) < expectedMinHits {
		t.Fatalf("expected at least %d hits, got %d\nraw: %s", expectedMinHits, len(hits), raw)
	}
}

// --- Suggest / autocomplete test infrastructure ---

// suggestConfigSDL returns a config SDL with suggestField and autocomplete: true on TEXT fields.
func suggestConfigSDL(backend, configJSON string) string {
	return fmt.Sprintf(`
extend schema @index(name: "products", backend: "%s", config: "%s")

type Product @key(fields: "id") @searchable(index: "products", searchField: "searchProducts", suggestField: "suggestProducts") {
  id: ID!
  name: String @indexed(type: TEXT, filterable: true, sortable: true, autocomplete: true)
  description: String @indexed(type: TEXT, autocomplete: true)
  category: String @indexed(type: KEYWORD, filterable: true, sortable: true)
  price: Float @indexed(type: NUMERIC, filterable: true, sortable: true)
  inStock: Boolean @indexed(type: BOOL, filterable: true)
}
`, backend, configJSON)
}

// filterMetadataRootFields returns a copy of metadata with only the specified field
// kept in the Query root node. All other root nodes and child nodes are preserved.
func filterMetadataRootFields(meta *plan.DataSourceMetadata, keepField string) *plan.DataSourceMetadata {
	clone := &plan.DataSourceMetadata{
		FederationMetaData: meta.FederationMetaData,
		Directives:         meta.Directives,
	}
	for _, rn := range meta.RootNodes {
		if rn.TypeName == "Query" {
			var filtered []string
			for _, fn := range rn.FieldNames {
				if fn == keepField {
					filtered = append(filtered, fn)
				}
			}
			if len(filtered) > 0 {
				clone.RootNodes = append(clone.RootNodes, plan.TypeField{
					TypeName:   rn.TypeName,
					FieldNames: filtered,
				})
			}
		} else {
			clone.RootNodes = append(clone.RootNodes, rn)
		}
	}
	clone.ChildNodes = make([]plan.TypeField, len(meta.ChildNodes))
	copy(clone.ChildNodes, meta.ChildNodes)
	return clone
}

// setupSuggestTestEnv builds a test environment with both search and suggest datasource entries.
func setupSuggestTestEnv(t *testing.T, setup BackendSetup) testEnv {
	t.Helper()

	// 1. Parse the config schema SDL.
	doc, parseReport := astparser.ParseGraphqlDocumentString(setup.ConfigSDL)
	if parseReport.HasErrors() {
		t.Fatalf("parse config schema: %s", parseReport.Error())
	}
	parsedConfig, err := search_datasource.ParseConfigSchema(&doc)
	if err != nil {
		t.Fatalf("ParseConfigSchema: %v", err)
	}
	if len(parsedConfig.Entities) == 0 {
		t.Fatal("no entities found in config schema")
	}

	// 2. Generate the search subgraph SDL.
	searchSDL, err := search_datasource.GenerateSubgraphSDL(parsedConfig)
	if err != nil {
		t.Fatalf("GenerateSubgraphSDL: %v", err)
	}

	entity := &parsedConfig.Entities[0]
	indexDirective := parsedConfig.Indices[0]

	// 3. Build index schema and create the search index.
	indexSchema := buildIndexSchema(indexDirective.Name, entity)
	idx := setup.CreateIndex(t, fmt.Sprintf("test_%s", setup.Name), indexSchema, []byte(indexDirective.ConfigJSON))

	// 4. Populate with test data.
	if err := idx.IndexDocuments(context.Background(), testProducts()); err != nil {
		t.Fatalf("populate test data: %v", err)
	}
	if setup.Hooks.WaitForIndex != nil {
		setup.Hooks.WaitForIndex(t)
	}

	// 5. Start the entity subgraph server.
	entityServer := httptest.NewServer(productdetails.Handler())
	t.Cleanup(entityServer.Close)

	// 6. Compose the subgraphs.
	entitySDL := entitySubgraphSDL(t)
	routerConfig := composeSubgraphs(t, searchSDL, entitySDL, entityServer.URL)

	// 7. Build plan configuration with split datasources.
	searchConfig := entityToConfiguration(entity)

	suggestConfig := entityToConfiguration(entity)
	suggestConfig.SearchField = entity.SuggestField
	suggestConfig.IsSuggest = true
	suggestConfig.ResultsMetaInformation = false
	suggestConfig.CursorBasedPagination = false

	supergraphDef := routerConfig.EngineConfig.GraphqlSchema
	planConfig := buildSuggestPlanConfiguration(t, routerConfig, idx, searchConfig, suggestConfig, entityServer.URL)

	return testEnv{
		Pipeline: &testPipeline{
			PlanConfig:    planConfig,
			SupergraphDef: supergraphDef,
		},
		SupergraphDef: supergraphDef,
		DefaultSort:   `[{"field": "PRICE", "direction": "ASC"}]`,
	}
}

// buildSuggestPlanConfiguration builds a plan.Configuration that splits the search datasource
// into two entries: one for searchProducts and one for suggestProducts.
func buildSuggestPlanConfiguration(
	t *testing.T,
	routerConfig *nodev1.RouterConfig,
	idx searchindex.Index,
	searchConfig search_datasource.Configuration,
	suggestConfig search_datasource.Configuration,
	entityServerURL string,
) plan.Configuration {
	t.Helper()

	engineConfig := routerConfig.EngineConfig
	var planConfig plan.Configuration
	planConfig.DefaultFlushIntervalMillis = engineConfig.DefaultFlushInterval

	for _, fc := range engineConfig.FieldConfigurations {
		var args []plan.ArgumentConfiguration
		for _, ac := range fc.ArgumentsConfiguration {
			arg := plan.ArgumentConfiguration{
				Name:         ac.Name,
				RenderConfig: plan.RenderArgumentAsJSONValue,
			}
			switch ac.SourceType {
			case nodev1.ArgumentSource_FIELD_ARGUMENT:
				arg.SourceType = plan.FieldArgumentSource
			case nodev1.ArgumentSource_OBJECT_FIELD:
				arg.SourceType = plan.ObjectFieldSource
			}
			args = append(args, arg)
		}
		planConfig.Fields = append(planConfig.Fields, plan.FieldConfiguration{
			TypeName:  fc.TypeName,
			FieldName: fc.FieldName,
			Arguments: args,
		})
	}

	for _, tc := range engineConfig.TypeConfigurations {
		planConfig.Types = append(planConfig.Types, plan.TypeConfiguration{
			TypeName: tc.TypeName,
			RenameTo: tc.RenameTo,
		})
	}

	for _, ds := range engineConfig.DatasourceConfigurations {
		metadata := extractDataSourceMetadata(ds)

		fetchURL := ""
		if ds.CustomGraphql != nil && ds.CustomGraphql.Fetch != nil {
			fetchURL = ds.CustomGraphql.Fetch.GetUrl().GetStaticVariableContent()
		}

		if fetchURL == "http://search.local" {
			// Split into search and suggest datasources sharing the same factory and index.
			searchFactory := search_datasource.NewFactory(context.Background(), nil, nil)
			searchFactory.RegisterIndex(searchConfig.IndexName, idx)

			// Search datasource — only searchProducts in root nodes.
			searchMeta := filterMetadataRootFields(metadata, searchConfig.SearchField)
			searchDS, err := plan.NewDataSourceConfiguration[search_datasource.Configuration](
				ds.Id+"_search",
				searchFactory,
				searchMeta,
				searchConfig,
			)
			if err != nil {
				t.Fatalf("NewDataSourceConfiguration (search): %v", err)
			}
			planConfig.DataSources = append(planConfig.DataSources, searchDS)

			// Suggest datasource — only suggestProducts in root nodes.
			suggestMeta := filterMetadataRootFields(metadata, suggestConfig.SearchField)
			// Add SuggestTerm as a child node for the suggest datasource.
			suggestMeta.ChildNodes = append(suggestMeta.ChildNodes, plan.TypeField{
				TypeName:   "SuggestTerm",
				FieldNames: []string{"term", "count"},
			})

			suggestDS, err := plan.NewDataSourceConfiguration[search_datasource.Configuration](
				ds.Id+"_suggest",
				searchFactory,
				suggestMeta,
				suggestConfig,
			)
			if err != nil {
				t.Fatalf("NewDataSourceConfiguration (suggest): %v", err)
			}
			planConfig.DataSources = append(planConfig.DataSources, suggestDS)
		} else {
			// Entity datasource — same as buildPlanConfiguration.
			graphqlSchema, err := loadInternedString(engineConfig, ds.CustomGraphql.GetUpstreamSchema())
			if err != nil {
				t.Fatalf("load upstream schema: %v", err)
			}

			schemaConfig, err := graphql_datasource.NewSchemaConfiguration(
				graphqlSchema,
				&graphql_datasource.FederationConfiguration{
					Enabled:    ds.CustomGraphql.Federation.Enabled,
					ServiceSDL: ds.CustomGraphql.Federation.ServiceSdl,
				},
			)
			if err != nil {
				t.Fatalf("NewSchemaConfiguration (entity): %v", err)
			}

			entityConfig, err := graphql_datasource.NewConfiguration(graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{
					URL: entityServerURL,
				},
				SchemaConfiguration: schemaConfig,
			})
			if err != nil {
				t.Fatalf("NewConfiguration (entity): %v", err)
			}

			entityFactory, err := graphql_datasource.NewFactory(context.Background(), http.DefaultClient, &noopSubscriptionClient{})
			if err != nil {
				t.Fatalf("NewFactory (entity): %v", err)
			}

			entityDS, err := plan.NewDataSourceConfiguration[graphql_datasource.Configuration](
				ds.Id,
				entityFactory,
				metadata,
				entityConfig,
			)
			if err != nil {
				t.Fatalf("NewDataSourceConfiguration (entity): %v", err)
			}
			planConfig.DataSources = append(planConfig.DataSources, entityDS)
		}
	}

	planConfig.DisableResolveFieldPositions = true

	return planConfig
}

// RunSuggestScenarios runs suggest/autocomplete e2e scenarios for a given backend.
func RunSuggestScenarios(t *testing.T, setup BackendSetup) {
	t.Helper()

	env := setupSuggestTestEnv(t, setup)
	pipeline := env.Pipeline

	t.Run("suggest_basic", func(t *testing.T) {
		t.Parallel()
		// "shoe" should match terms from name/description tokens.
		query := `{ suggestProducts(prefix: "shoe") { term count } }`
		raw := executeQuery(t, pipeline, query, "")
		assertSuggestResponse(t, raw, suggestExpectation{
			minResults: 1,
		})
	})

	t.Run("suggest_short_prefix_returns_empty", func(t *testing.T) {
		t.Parallel()
		// Single character is below minPrefixLength (2).
		query := `{ suggestProducts(prefix: "s") { term count } }`
		raw := executeQuery(t, pipeline, query, "")
		assertSuggestResponse(t, raw, suggestExpectation{exactCount: intPtr(0)})
	})

	t.Run("suggest_with_valid_prefix_and_limit", func(t *testing.T) {
		t.Parallel()
		query := `{ suggestProducts(prefix: "sh", limit: 1) { term count } }`
		raw := executeQuery(t, pipeline, query, "")
		assertSuggestResponse(t, raw, suggestExpectation{
			minResults: 1,
			maxResults: 1,
		})
	})

	t.Run("suggest_no_match", func(t *testing.T) {
		t.Parallel()
		query := `{ suggestProducts(prefix: "zzzzz") { term count } }`
		raw := executeQuery(t, pipeline, query, "")
		assertSuggestResponse(t, raw, suggestExpectation{exactCount: intPtr(0)})
	})

	t.Run("suggest_has_term_and_count", func(t *testing.T) {
		t.Parallel()
		query := `{ suggestProducts(prefix: "shoe") { term count } }`
		raw := executeQuery(t, pipeline, query, "")
		assertSuggestResponse(t, raw, suggestExpectation{
			minResults:    1,
			requireFields: []string{"term", "count"},
		})
	})

	t.Run("suggest_case_insensitive", func(t *testing.T) {
		t.Parallel()
		query := `{ suggestProducts(prefix: "SHOE") { term count } }`
		raw := executeQuery(t, pipeline, query, "")
		assertSuggestResponse(t, raw, suggestExpectation{minResults: 1})
	})

	t.Run("suggest_only_autocomplete_fields", func(t *testing.T) {
		t.Parallel()
		// "footwear" is a category value (KEYWORD field, no autocomplete).
		// It should not appear in suggest results.
		query := `{ suggestProducts(prefix: "footwear") { term count } }`
		raw := executeQuery(t, pipeline, query, "")
		assertSuggestResponse(t, raw, suggestExpectation{exactCount: intPtr(0)})
	})

	t.Run("suggest_search_still_works", func(t *testing.T) {
		t.Parallel()
		// Verify the search datasource still works alongside suggest.
		query := `query($s: [ProductSort!]) { searchProducts(query: "*", sort: $s) { hits { node { id name } } totalCount } }`
		vars := `{"s": [{"field": "PRICE", "direction": "ASC"}]}`
		raw := executeQuery(t, pipeline, query, vars)
		var resp struct {
			Data struct {
				SearchProducts struct {
					Hits       []any `json:"hits"`
					TotalCount int   `json:"totalCount"`
				} `json:"searchProducts"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(raw), &resp); err != nil {
			t.Fatalf("unmarshal: %v\nraw: %s", err, raw)
		}
		if resp.Data.SearchProducts.TotalCount != 4 {
			t.Fatalf("expected 4 products, got %d\nraw: %s", resp.Data.SearchProducts.TotalCount, raw)
		}
	})
}

type suggestExpectation struct {
	minResults    int
	maxResults    int      // 0 = no max
	exactCount    *int     // if non-nil, assert exact count
	requireFields []string // fields that must exist in each result
	containsTerm  string   // at least one result must have this term
}

func assertSuggestResponse(t *testing.T, raw string, expect suggestExpectation) {
	t.Helper()
	var resp struct {
		Data struct {
			SuggestProducts []struct {
				Term  string `json:"term"`
				Count int    `json:"count"`
			} `json:"suggestProducts"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal suggest response: %v\nraw: %s", err, raw)
	}

	results := resp.Data.SuggestProducts
	if expect.exactCount != nil {
		if len(results) != *expect.exactCount {
			t.Fatalf("expected exactly %d results, got %d: %s", *expect.exactCount, len(results), raw)
		}
		return
	}
	if len(results) < expect.minResults {
		t.Fatalf("expected at least %d results, got %d: %s", expect.minResults, len(results), raw)
	}
	if expect.maxResults > 0 && len(results) > expect.maxResults {
		t.Fatalf("expected at most %d results, got %d: %s", expect.maxResults, len(results), raw)
	}
	if expect.containsTerm != "" {
		found := false
		for _, r := range results {
			if r.Term == expect.containsTerm {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected term %q not found in results: %s", expect.containsTerm, raw)
		}
	}
	if len(expect.requireFields) > 0 {
		// For structural checks, verify via raw JSON that each result has the required fields.
		var rawResp struct {
			Data struct {
				SuggestProducts []map[string]any `json:"suggestProducts"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(raw), &rawResp); err != nil {
			t.Fatalf("unmarshal raw suggest response: %v", err)
		}
		for i, r := range rawResp.Data.SuggestProducts {
			for _, field := range expect.requireFields {
				if _, ok := r[field]; !ok {
					t.Errorf("result[%d] missing required field %q: %s", i, field, raw)
				}
			}
		}
	}
}

func intPtr(i int) *int { return &i }
