# Search DataSource Implementation

GraphQL-native search integration for the WunderGraph router. Adds full-text search, vector search, filtering, sorting, pagination, and faceted search to any federated GraphQL graph via schema directives.

**Status:** Work in progress. ~19,800 lines across 80+ files.

## Remaining TODO

- Check vector search
- Create instructions to implement search in cosmo router

## Package Layout

```
v2/pkg/engine/datasource/search_datasource/   # GraphQL datasource (planner, source, SDL generation)
v2/pkg/engine/datasource/search_datasource/searche2e/  # Unit-level e2e tests (no composition)
v2/pkg/searchindex/                            # Backend-agnostic abstractions (Index, Filter, Embedder)
v2/pkg/searchindex/{bleve,elasticsearch,...}/   # Backend implementations
v2/pkg/searchindex/embedder/{openai,ollama}/   # Embedding providers
execution/searchtesting/                       # Full-stack e2e tests (with cosmo composition)
execution/searchtesting/productdetails/        # Federation entity subgraph (gqlgen)
execution/searchtesting/shareddata/            # Shared test data (4 products)
```

## Architecture Overview

```
Schema Directives (@index, @searchable, @indexed, @embedding)
    |
    v
directives.go: ParseConfigSchema() -> ParsedConfig
    |
    v
generator.go: GenerateSubgraphSDL() -> GraphQL subgraph schema
    |
    v
lifecycle.go: Manager.Start() -> creates indices, populates data, starts subscriptions
    |
    v
planner.go: Planner -> walks GraphQL operation, collects search arguments
    |
    v
source.go: Source.Load() -> builds SearchRequest, calls Index.Search(), formats response
    |
    v
searchindex.Index (backend) -> SearchResult (hits, scores, facets, cursors)
```

## Key Files

### `search_datasource/` (GraphQL integration)

| File | Lines | Purpose |
|------|-------|---------|
| `directives.go` | 434 | Parses `@index`, `@searchable`, `@indexed`, `@embedding`, `@populate`, `@subscribe` from schema AST into `ParsedConfig` |
| `generator.go` | 328 | Generates subgraph SDL from `ParsedConfig` (filter inputs, sort enums, result types, query fields, entity stubs) |
| `source.go` | 354 | `Source.Load()` - builds `SearchRequest` from planner input, calls backend, formats response (inline/wrapper/connection) |
| `lifecycle.go` | 280 | `Manager` - creates indices, sets up embedding pipelines, runs populate queries, starts subscriptions, handles shutdown |
| `filter_parser.go` | 239 | Converts GraphQL filter JSON to `searchindex.Filter` tree (term, range, prefix, AND/OR/NOT) |
| `planner.go` | 119 | `Planner` - visitor-based, detects search field, collects arguments, builds fetch input JSON with `{{.arguments.X}}` templates |
| `entity_extractor.go` | 118 | Extracts `EntityDocument` slice from GraphQL response JSON (for populate/subscribe) |
| `factory.go` | 87 | `Factory` - creates planners, holds index/embedder registries |
| `configuration.go` | 53 | `Configuration` struct (JSON-serialized per-entity config for planner/source) |
| `cursor.go` | 26 | Base64+JSON cursor encoding/decoding for Relay-style pagination |

### `searchindex/` (backend abstractions)

| File | Lines | Purpose |
|------|-------|---------|
| `index.go` | 25 | `Index` interface: `IndexDocument`, `IndexDocuments`, `DeleteDocument`, `DeleteDocuments`, `Search`, `Close` |
| `document.go` | 77 | `EntityDocument`, `SearchRequest`, `SearchResult`, `SearchHit`, `SortField`, `FacetRequest/Result` |
| `filter.go` | 47 | `Filter` tree: `And`, `Or`, `Not`, `Term`, `Terms`, `Range`, `Prefix`, `Exists` |
| `config.go` | 62 | `FieldType` enum (Text, Keyword, Numeric, Bool, Vector), `FieldConfig`, `IndexConfig` |
| `embedder.go` | 49 | `Embedder` interface, `TextTransformer` interface, `EmbeddingPipeline` (transformer + embedder) |
| `registry.go` | 68 | Thread-safe `IndexFactoryRegistry` and `EmbedderRegistry` |
| `template_transformer.go` | 69 | `TemplateTransformer` - converts `{{title}}` shorthand to Go templates for embedding text generation |

### Backend implementations (all implement `searchindex.Index`)

| Backend | Lines | Notes |
|---------|-------|-------|
| `bleve/` | 688 | In-memory/file-based. Full-featured: text, facets, prefix, cursor pagination. No external deps at runtime. |
| `elasticsearch/` | 898 | HTTP client. Uses `_search` API with query DSL. Supports `search_after` for cursor pagination. |
| `pgvector/` | 1342 | PostgreSQL with pgvector extension. SQL-based filters. Supports both text (tsvector) and vector search. |
| `weaviate/` | 1219 | GraphQL-based vector DB. Uses BM25 for text, nearVector for vectors. Class-based schema. |
| `typesense/` | 911 | HTTP client. Filter syntax string-based. Native text search with faceting. |
| `qdrant/` | 838 | gRPC client. Point-based storage. Vector search with payload filtering. |
| `algolia/` | 806 | HTTP client. Uses Algolia search API with faceting and filters. |
| `meilisearch/` | 757 | HTTP client. Filter string syntax. Native text search with faceting. |

### Embedding providers

| Provider | Lines | Notes |
|----------|-------|-------|
| `openai/` | 270 | OpenAI embeddings API. Batch support (up to 2048). Exponential backoff retries. |
| `ollama/` | 148 | Local Ollama server. Batch via `/api/embed`. Caller provides dimensions. |

## Schema Directives

```graphql
# On schema extension: declare a search index
extend schema @index(name: "products", backend: "bleve", config: "{}", cursorBasedPagination: true)

# On object type: make it searchable
type Product @key(fields: "id") @searchable(
  index: "products"
  searchField: "searchProducts"
  resultsMetaInformation: true
) {
  id: ID!
  name: String!  @indexed(type: TEXT, filterable: true, sortable: true)
  price: Float!  @indexed(type: NUMERIC, filterable: true, sortable: true)
  category: String! @indexed(type: KEYWORD, filterable: true, sortable: true)
  inStock: Boolean! @indexed(type: BOOLEAN, filterable: true)
  embedding: [Float!]! @embedding(sourceFields: ["name", "description"], template: "{{name}}. {{description}}", model: "text-embedding-3-small")
}
```

**Index types:** `TEXT` (full-text), `KEYWORD` (exact match), `NUMERIC` (range queries), `BOOLEAN`, `VECTOR` (pre-computed embeddings)

## Generated SDL Modes

The generator produces different SDL based on configuration:

1. **Inline** (`resultsMetaInformation: false`, no cursor): Returns `[Product!]!` directly
2. **Wrapper** (`resultsMetaInformation: true`, no cursor): Returns `SearchProductResult!` with `hits[]{score, node}`, `totalCount`, `facets`
3. **Connection** (cursor pagination): Returns `SearchProductConnection!` with `edges[]{cursor, node, score}`, `pageInfo`, `totalCount`

Query arguments vary by mode:
- Inline/Wrapper: `query`, `filter`, `sort`, `limit`, `offset`, `facets`
- Connection: `query`, `filter`, `sort`, `first`, `after`, `last` (if bidirectional), `before`, `facets`

Vector search adds: `search: SearchProductInput!` (a `@oneOf` input with `query: String` or `vector: [Float!]`)

## Response Formatting

Source formats responses differently per mode, all wrapped in `{"data": {"<searchField>": ...}}`:

- **Inline:** `[{id, name, ...}, ...]`
- **Wrapper:** `{hits: [{score, node: {...}}, ...], totalCount, facets}`
- **Connection:** `{edges: [{cursor, node: {...}, score}, ...], pageInfo: {hasNextPage, ...}, totalCount}`

## Lifecycle Manager

`Manager.Start()` orchestrates:
1. Creates indices from `@index` directives via `IndexFactoryRegistry`
2. Sets up `EmbeddingPipeline` for each `@embedding` field (TemplateTransformer + Embedder)
3. Runs initial population via `@populate` queries (executes GraphQL, extracts entities, batches embeddings, indexes)
4. Starts subscription listeners via `@subscribe` for real-time updates
5. Schedules periodic resync if `resyncInterval` is set

## Filter System

GraphQL filter JSON is parsed into a composable `searchindex.Filter` tree:

```json
{"category": {"eq": "Footwear"}, "price": {"gte": 10, "lte": 100}, "AND": [...], "OR": [...], "NOT": {...}}
```

Operators by field type:
- **String (Text/Keyword):** `eq`, `ne`, `in`, `contains`, `startsWith`
- **Numeric:** `eq`, `gt`, `gte`, `lt`, `lte`
- **Boolean:** direct value

Each backend translates the `Filter` tree to its native query language.

## Cursor Pagination

Uses base64-encoded JSON sort values as opaque cursors. Over-fetches by 1 to detect `hasNextPage`/`hasPreviousPage`. Backward pagination (`last`/`before`) reverses sort direction, then reverses results back.

Backend support varies: Bleve and pgvector support bidirectional; Elasticsearch supports forward-only.

## Test Architecture

### Two test layers

**`searche2e/` (unit-level e2e):** Tests `Source.Load()` directly with a real index backend. No HTTP, no composition. Backend-agnostic framework with `RunBackendTests()`, `RunCursorTests()`, `RunGeoTests()`, and `RunFederatedBackendTests()`.

**`execution/searchtesting/` (full-stack e2e):** Full pipeline: parse config SDL -> generate SDL -> compose subgraphs (cosmo composition-go) -> plan -> resolve. Includes a real gqlgen federation entity subgraph for entity joins. Tests wrapper, inline, cursor, vector, hybrid, geo, highlight, and additional filter modes.

### Test data

4 products (shared via `shareddata.Products()`):
- Running Shoes ($89.99, Footwear, Nike, in stock, New York)
- Basketball Shoes ($129.99, Footwear, Adidas, in stock, Midtown Manhattan)
- Leather Belt ($35.00, Accessories, Gucci, out of stock, Los Angeles)
- Wool Socks ($12.99, Footwear, Smartwool, in stock, London)

### Test scenarios covered

- Text search, keyword/boolean/numeric-range filtering
- AND/OR/NOT filter combinations, IN (terms), NE, prefix filters
- Sorting (price ASC/DESC), offset pagination, cursor pagination (forward + backward)
- Faceted search (category aggregation)
- Federation entity joins (manufacturer, rating, reviews from separate subgraph)
- Document CRUD (index, upsert, delete single/batch)
- SDL generation (text-only, vector, inline, cursor variants)
- Identity roundtrip (`__typename` + key fields)
- Geo-spatial search (distance filter, bounding box, distance sort, combined filters)
- Search highlights (field + fragments validation)
- Vector search (auto-embed, raw vector, with filter, distance populated, entity join)
- Hybrid search (text + vector combined, relevance, filter, entity join)

### Running tests

```bash
# Unit-level e2e (bleve only, no external services)
cd v2 && go test ./pkg/engine/datasource/search_datasource/searche2e/ -run TestBleve -v

# Full-stack e2e (requires composition-go in execution/)
cd execution && go test ./searchtesting/ -run TestBleve -v

# External backends require running services (ES, pgvector, etc.)
# Use -tags=integration for external backend tests
cd execution && go test -tags=integration ./searchtesting/ -run TestElasticsearch -v
```

### IMPORTANT: Adding integration tests for new features

**Every new feature MUST have extensive integration tests in `execution/searchtesting/`.** This is the primary test layer that validates the full pipeline end-to-end with federation composition.

When adding a new search capability:

1. **Add a `Run<Feature>Scenarios()` function** to `execution/searchtesting/framework.go`. Follow the pattern of existing functions:
   - `RunAllScenarios()` — basic text search + filters + sort + pagination + facets
   - `RunInlineScenarios()` — inline result style (no wrapper types)
   - `RunCursorScenarios()` — cursor-based pagination
   - `RunVectorScenarios()` — vector/semantic search
   - `RunHybridScenarios()` — text + vector hybrid search
   - `RunGeoScenarios()` — geo-spatial search (distance, bounding box, geo sort)
   - `RunHighlightScenarios()` — search result highlights
   - `RunAdditionalFilterScenarios()` — NE, IN, startsWith filter operators

2. **Create a config SDL builder** if the feature needs additional fields (e.g., `geoConfigSDL()`, `vectorConfigSDL()`).

3. **Add a `setup<Feature>TestEnv()` function** if the feature requires custom data or configuration (e.g., `setupGeoTestEnv()`, `setupVectorTestEnv()`).

4. **Add test data functions** in `testdata.go` if the feature needs specialized data (e.g., `testGeoProducts()`, `testVectorProducts()`).

5. **Add backend test functions** in the per-backend `*_test.go` files:
   - `bleve_test.go` — always add here first (no external deps, fast CI)
   - `elasticsearch_test.go` — add for features ES supports (geo, highlights, etc.)
   - Other backends as applicable

6. **Use structural assertions** for backend-dependent values (scores, distances, highlights). Use exact JSON matching only for deterministic results (filter counts, sort order, entity joins).

7. **Also add unit-level tests** to `searche2e/framework.go` (`RunBackendTests()`, `RunGeoTests()`, etc.) — these test `Source.Load()` directly without composition overhead.

The goal: every `Run*Scenarios()` function covers the feature end-to-end through the full pipeline (parse SDL -> generate -> compose -> plan -> resolve -> entity join). If Bleve can run it, test it with Bleve first for fast feedback.
