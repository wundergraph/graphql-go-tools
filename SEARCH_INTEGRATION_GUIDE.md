# Search Integration Guide for Cosmo Router

This guide explains how to integrate the search datasource from `graphql-go-tools` into the Cosmo router. It covers every integration point, the public APIs, and the complete data flow from configuration to query execution.

## Architecture Overview

The search datasource is a **virtual subgraph** — it has no running HTTP server. Instead, the router:

1. Parses a **config schema** (GraphQL SDL with custom directives) that declares what entities are searchable and how
2. **Generates** a federation-compliant subgraph SDL from those directives
3. **Composes** that generated SDL with other subgraphs using `composition-go`
4. At runtime, the search datasource resolves queries by calling a local search index rather than making HTTP fetches

The search subgraph is identified in composition output by the sentinel fetch URL `"http://search.local"`.

## Packages

| Package | Import Path | Purpose |
|---------|------------|---------|
| `searchindex` | `v2/pkg/searchindex` | Core interfaces: `Index`, `IndexFactory`, `Embedder`, `EmbedderRegistry`, `IndexFactoryRegistry` |
| `search_datasource` | `v2/pkg/engine/datasource/search_datasource` | GraphQL integration: `Factory`, `Planner`, `Source`, `Manager`, directive parsing, SDL generation |
| Backend implementations | `v2/pkg/searchindex/{pgvector,elasticsearch,weaviate,qdrant,bleve,algolia,typesense,meilisearch}` | Each exports a `NewFactory() IndexFactory` |
| Embedding providers | `v2/pkg/searchindex/embedder/{openai,ollama}` | Each exports a constructor returning `searchindex.Embedder` |

## Step-by-Step Integration

### Step 1: Parse the Config Schema

The config schema is a GraphQL SDL file written by the user. It uses custom directives to declare indices, searchable entities, indexed fields, and embeddings.

```go
import (
    "github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
    "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/search_datasource"
)

doc, report := astparser.ParseGraphqlDocumentString(configSDL)
if report.HasErrors() {
    return fmt.Errorf("parse config schema: %s", report.Error())
}

parsedConfig, err := search_datasource.ParseConfigSchema(&doc)
// parsedConfig.Indices       -- []IndexDirective  (one per @index)
// parsedConfig.Entities      -- []SearchableEntity (one per @searchable type)
// parsedConfig.Populations   -- []PopulateDirective
// parsedConfig.Subscriptions -- []SubscribeDirective
```

**Directive syntax:**

```graphql
# Declare an index with a backend
extend schema @index(name: "products", backend: "pgvector", config: "{}")

# Mark an entity as searchable
type Product @key(fields: "id") @searchable(index: "products", searchField: "searchProducts") {
  id: ID\!
  name: String @indexed(type: TEXT, filterable: true, sortable: true)
  description: String @indexed(type: TEXT)
  category: String @indexed(type: KEYWORD, filterable: true, sortable: true)
  price: Float @indexed(type: NUMERIC, filterable: true, sortable: true)
  inStock: Boolean @indexed(type: BOOL, filterable: true)
  _embedding: [Float\!] @embedding(fields: "name description", template: "{{name}}. {{description}}", model: "text-embedding-3-small")
}
```

**`@indexed` types:** `TEXT`, `KEYWORD`, `NUMERIC`, `BOOL`, `VECTOR`, `GEO`

**`@embedding` directive:**
- `fields` is a **space-separated string** (NOT an array), parsed internally by `strings.Fields()`
- `template` uses Go template syntax with field names as variables
- `model` is the key used to look up the embedder in `EmbedderRegistry`

**`@searchable` options:**
- `resultsMetaInformation: false` -- flat array results instead of wrapper types (no `hits`, `score`, `totalCount`)
- `@index(cursorBasedPagination: true)` -- enables Relay-style cursor pagination

### Step 2: Generate the Subgraph SDL

```go
searchSDL, err := search_datasource.GenerateSubgraphSDL(parsedConfig)
```

This produces a complete federation-compliant SDL. The shape depends on the entity configuration:

**Text-only entity (with wrapper):**
```graphql
type Query {
  searchProducts(query: String\!, fuzziness: Fuzziness, filter: ProductFilter, sort: [ProductSort\!], limit: Int, offset: Int, facets: [String\!]): SearchProductResult\!
}
```

The `Fuzziness` enum (`EXACT`, `LOW`, `HIGH`) controls typo tolerance at query time. It maps to edit distance 0, 1, 2 respectively. Omitting it uses the backend default.

**Vector-enabled entity (with `@embedding`):**
```graphql
input SearchProductInput @oneOf {
  query: String
  vector: [Float\!]
}

type SearchProductHit {
  score: Float\!
  distance: Float
  node: Product\!
}

type Query {
  searchProducts(search: SearchProductInput\!, fuzziness: Fuzziness, filter: ProductFilter, sort: [ProductSort\!], limit: Int, offset: Int): SearchProductResult\!
}
```

Key differences when `HasVectorSearch()` is true:
- Query argument changes from `query: String\!` to `search: SearchProductInput\!` (a `@oneOf` input with `query`/`vector`)
- Hits include a `distance: Float` field
- No `facets` argument or facet types

**Inline style (`resultsMetaInformation: false`):**
```graphql
type Query {
  searchProducts(query: String\!, fuzziness: Fuzziness, ...): [Product\!]\!
}
```

**Cursor pagination (`cursorBasedPagination: true`):**
```graphql
type Query {
  searchProducts(query: String\!, fuzziness: Fuzziness, first: Int, after: String, last: Int, before: String): SearchProductConnection\!
}
```

### Step 3: Compose with Other Subgraphs

Use `composition-go` to compose the generated search SDL with entity subgraphs. The search subgraph uses the sentinel URL `"http://search.local"`:

```go
import "github.com/wundergraph/cosmo/composition-go"

subgraphs := []*composition.Subgraph{
    {
        Name:   "search",
        URL:    "http://search.local",  // sentinel -- no real HTTP server
        Schema: searchSDL,
    },
    {
        Name:   "productdetails",
        URL:    entitySubgraphURL,
        Schema: entitySDL,
    },
}

routerConfigJSON, err := composition.BuildRouterConfiguration(subgraphs...)
```

### Step 4: Register Backend Factories

Create registries and register the backends you want to support:

```go
import (
    "github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
    "github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex/pgvector"
    "github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex/elasticsearch"
    "github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex/bleve"
)

indexRegistry := searchindex.NewIndexFactoryRegistry()
indexRegistry.Register("pgvector", pgvector.NewFactory())
indexRegistry.Register("elasticsearch", elasticsearch.NewFactory())
indexRegistry.Register("bleve", bleve.NewFactory())
// Register all 8 backends as needed
```

### Step 5: Register Embedding Providers

If any entity uses `@embedding`, register the corresponding model in the embedder registry:

```go
embedderRegistry := searchindex.NewEmbedderRegistry()
embedderRegistry.Register("text-embedding-3-small", openaiEmbedder)
embedderRegistry.Register("nomic-embed-text", ollamaEmbedder)
```

The model name in the registry must match the `model` argument in `@embedding(model: "...")`.

### Step 6: Wire the Plan Configuration

When building the `plan.Configuration` from the composition output, identify the search datasource by checking if the fetch URL is `"http://search.local"` and use `search_datasource.Factory` instead of `graphql_datasource.Factory`:

```go
searchFactory := search_datasource.NewFactory(ctx, indexRegistry, embedderRegistry)

for _, ds := range engineConfig.DatasourceConfigurations {
    fetchURL := ds.CustomGraphql.Fetch.GetUrl().GetStaticVariableContent()

    if fetchURL == "http://search.local" {
        entity := &parsedConfig.Entities[0]
        searchConfig := entityToConfiguration(entity)

        searchDS, err := plan.NewDataSourceConfiguration[search_datasource.Configuration](
            ds.Id,
            searchFactory,
            metadata,
            searchConfig,
        )
        planConfig.DataSources = append(planConfig.DataSources, searchDS)
    } else {
        // Standard GraphQL datasource
    }
}
```

**Converting a `SearchableEntity` to `Configuration`:**

```go
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
            FieldName:   f.FieldName,
            GraphQLType: f.GraphQLType,
            IndexType:   f.IndexType,
            Filterable:  f.Filterable,
            Sortable:    f.Sortable,
            Dimensions:  f.Dimensions,
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
```

### Step 7: Lifecycle Management

The `Manager` handles index creation, initial data population, embedding pipelines, and live subscriptions:

```go
manager := search_datasource.NewManager(
    searchFactory,
    indexRegistry,
    embedderRegistry,
    executor,      // implements GraphQLExecutor interface
    parsedConfig,
)

if err := manager.Start(ctx); err \!= nil {
    return err
}
defer manager.Stop()
```

**`GraphQLExecutor` interface:**
```go
type GraphQLExecutor interface {
    Execute(ctx context.Context, operation string) ([]byte, error)
}
```

The executor runs GraphQL operations against the federated graph itself. It is used for:
- **Population queries** (`@populate`): Fetches all entities and indexes them
- **Subscription updates** (`@subscribe`): Receives entity changes for live re-indexing

**What `Manager.Start()` does:**
1. Creates indices via `IndexFactoryRegistry.Get(backend).CreateIndex(ctx, name, schema, configJSON)`
2. Registers each index with `Factory.RegisterIndex(name, idx)` so the planner can find them
3. Sets up `EmbeddingPipeline` for each entity with `@embedding` fields (template transformer + embedder from registry)
4. Runs population queries -- executes GraphQL operations, extracts entities with `ExtractEntities()`, computes embeddings with the pipeline, and calls `idx.IndexDocuments()`
5. Starts subscription goroutines for live updates

### Step 8: Query Execution Flow

At query time, the flow is:

1. **Planner** (`Planner.EnterField`) detects the search field, collects which arguments are present
2. **Planner** (`Planner.ConfigureFetch`) builds a JSON template using `{{.arguments.X}}` syntax, creates a `Source` via `Factory.CreateSourceForConfig(config)`, and returns a `FetchConfiguration` with `PostProcessing.SelectResponseDataPath: ["data"]`
3. **Resolver** resolves the template variables and calls `Source.Load(ctx, headers, input)`
4. **Source** parses the input JSON, builds a `SearchRequest`, calls `index.Search(ctx, req)`, formats the response

**Auto-embedding flow in `Source.Load()`:**
- If `search.query` is provided AND the source has an embedder: `embedder.EmbedSingle(query)` produces `req.Vector`
- If `search.vector` is provided: use as `req.Vector` directly
- Otherwise: `req.TextQuery` for full-text search

**Response wrapping:**
- Source wraps results in `{"data": {"<searchField>": {...}}}` which matches `PostProcessing.SelectResponseDataPath: ["data"]`
- After the resolver extracts `"data"`, the result is keyed by the search field name, aligning with the plan visitor's response tree

## Backend Support Matrix

| Backend | Vector | Text | Facets | Cursor | Fuzziness | Package |
|---------|--------|------|--------|--------|-----------|---------|
| pgvector | native + hybrid RRF | tsvector | yes | bidirectional | no | `searchindex/pgvector` |
| Elasticsearch | dense_vector/kNN | yes | yes | forward only | yes (`multi_match.fuzziness`) | `searchindex/elasticsearch` |
| Weaviate | nearVector | BM25 | no | no | no | `searchindex/weaviate` |
| Qdrant | native | payload filter only | no | no | no | `searchindex/qdrant` |
| Bleve | no (silently ignores) | yes | yes | bidirectional | yes (`SetFuzziness()`) | `searchindex/bleve` |
| Algolia | no | yes | yes | no | EXACT only (`typoTolerance: false`) | `searchindex/algolia` |
| TypeSense | no | yes | yes | no | yes (`num_typos`) | `searchindex/typesense` |
| MeiliSearch | no | yes | yes | no | no (built-in, not per-query) | `searchindex/meilisearch` |

## Core Types Reference

### `searchindex.Index`
```go
type Index interface {
    IndexDocument(ctx context.Context, doc EntityDocument) error
    IndexDocuments(ctx context.Context, docs []EntityDocument) error
    DeleteDocument(ctx context.Context, id DocumentIdentity) error
    DeleteDocuments(ctx context.Context, ids []DocumentIdentity) error
    Search(ctx context.Context, req SearchRequest) (*SearchResult, error)
    Close() error
}
```

### `searchindex.IndexFactory`
```go
type IndexFactory interface {
    CreateIndex(ctx context.Context, name string, schema IndexConfig, configJSON []byte) (Index, error)
}
```

### `searchindex.Embedder`
```go
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    EmbedSingle(ctx context.Context, text string) ([]float32, error)
    Dimensions() int
}
```

### `searchindex.IndexConfig`
```go
type IndexConfig struct {
    Name   string
    Fields []FieldConfig
}

type FieldConfig struct {
    Name       string
    Type       FieldType    // TEXT, KEYWORD, NUMERIC, BOOL, VECTOR, GEO
    Filterable bool
    Sortable   bool
    Dimensions int          // required for VECTOR fields
}
```

### `searchindex.SearchRequest`
```go
type SearchRequest struct {
    TextQuery       string
    TextFields      []TextFieldWeight  // field name + optional boost weight
    Vector          []float32
    VectorField     string
    Filter          *Filter
    Sort            []SortField
    Limit           int
    Offset          int
    Facets          []FacetRequest
    TypeName        string
    GeoDistanceSort *GeoDistanceSort
    Fuzziness       *Fuzziness         // nil = backend default; EXACT(0), LOW(1), HIGH(2)
    SearchAfter     []string           // cursor pagination
    SearchBefore    []string           // cursor pagination (backward)
}
```

### `search_datasource.Configuration`
```go
type Configuration struct {
    IndexName              string
    SearchField            string
    EntityTypeName         string
    KeyFields              []string
    Fields                 []IndexedFieldConfig
    EmbeddingFields        []EmbeddingFieldConfig
    HasVectorSearch        bool
    HasTextSearch          bool
    ResultsMetaInformation bool
    CursorBasedPagination  bool
    CursorBidirectional    bool
}
```

### `search_datasource.ParsedConfig`
```go
type ParsedConfig struct {
    Indices       []IndexDirective
    Entities      []SearchableEntity
    Populations   []PopulateDirective
    Subscriptions []SubscribeDirective
}
```

## Known Gaps

1. **Vector dimensions from `@embedding`**: `Manager.buildIndexSchema()` does NOT set `Dimensions` on vector fields created from `@embedding`. The dimensions come from `embedder.Dimensions()` at runtime. You must patch the `IndexConfig` after building it:

```go
for i, f := range schema.Fields {
    if f.Type == searchindex.FieldTypeVector && f.Dimensions == 0 {
        embedder, _ := embedderRegistry.Get(modelName)
        schema.Fields[i].Dimensions = embedder.Dimensions()
    }
}
```

2. **Population queries**: `Manager.populate()` calls `executor.Execute(ctx, "")` with an empty operation string. The actual population query mechanism needs wiring based on how the router provides the `GraphQLExecutor`.

3. **Subscription handlers**: `Manager.startSubscriptions()` is a placeholder -- it creates cancellable contexts but does not yet process subscription events.

## Reference Implementation

The e2e test framework in `execution/searchtesting/` is the authoritative reference:

- **`framework.go`** -- `setupTestEnv()` performs Steps 1-7 (parse, generate, compose, create index, populate, build plan config)
- **`framework.go`** -- `buildPlanConfiguration()` shows how to identify the search datasource by sentinel URL and wire the factory
- **`framework.go`** -- `setupVectorTestEnv()` extends this for vector search (patches dimensions, wires embedder registry)
- **Backend test files** (`pgvector_test.go`, etc.) -- per-backend factory creation with Docker testcontainers
- **`mock_embedder.go`** -- deterministic mock embedder for testing without external services
- **`testdata.go`** -- `testProducts()` and `testVectorProducts()` show document structure

### Running Tests

```bash
# Bleve (offline):
cd execution && go test ./searchtesting/ -run TestBleve -count=1

# Integration backends (requires Docker):
cd execution && go test -tags integration ./searchtesting/ -run TestPgvector -count=1 -timeout 120s
cd execution && go test -tags integration ./searchtesting/ -run TestElasticsearch -count=1 -timeout 120s
cd execution && go test -tags integration ./searchtesting/ -run TestWeaviate -count=1 -timeout 120s
cd execution && go test -tags integration ./searchtesting/ -run TestQdrant -count=1 -timeout 120s

# Vector search tests (requires Docker):
cd execution && go test -tags integration ./searchtesting/ -run TestPgvectorVector -count=1 -timeout 120s
```

## Integration Checklist

- [ ] Parse config schema SDL with `search_datasource.ParseConfigSchema()`
- [ ] Generate search subgraph SDL with `search_datasource.GenerateSubgraphSDL()`
- [ ] Compose with other subgraphs (search subgraph URL = `"http://search.local"`)
- [ ] Create `IndexFactoryRegistry` and register all desired backends
- [ ] Create `EmbedderRegistry` and register embedding providers (if using `@embedding`)
- [ ] Create `search_datasource.Factory` with both registries
- [ ] Detect search datasource by fetch URL `"http://search.local"` and use `search_datasource.Factory`
- [ ] Convert `SearchableEntity` to `search_datasource.Configuration` for each entity
- [ ] Patch vector field dimensions from `embedder.Dimensions()` if using `@embedding`
- [ ] Create `Manager` with factory, registries, executor, and parsed config
- [ ] Call `Manager.Start(ctx)` during router startup
- [ ] Call `Manager.Stop()` during router shutdown
- [ ] Implement `GraphQLExecutor` interface for population/subscription queries
