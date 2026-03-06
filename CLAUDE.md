# graphql-go-tools

GraphQL Router / API Gateway framework for Go. Federation-first, with query planning, parallel resolution, and entity caching.

Module: `github.com/wundergraph/graphql-go-tools` (Go 1.25, go.work workspace)

## Data Flow

```
parse → normalize → validate → plan → resolve → response
```

## Package Map

### Core (v2/pkg/)

| Package | Purpose |
|---------|---------|
| `ast` | GraphQL AST representation |
| `astparser` | GraphQL parser (schema + operations) |
| `astnormalization` | AST normalization passes |
| `astvalidation` | Schema and query validation |
| `astvisitor` | AST visitor pattern for tree walking |
| `astprinter` | AST to string serialization |
| `asttransform` | AST transformations |
| `astimport` | AST import/merge utilities |
| `fastjsonext` | JSON manipulation extensions (astjson API) |
| `federation` | Federation composition utilities |
| `errorcodes` | Error code definitions |

### Engine (v2/pkg/engine/)

| Package | Purpose |
|---------|---------|
| `plan` | Query planning, federation metadata, cache configuration types |
| **`resolve`** | **Resolution engine: fetching, caching, rendering** → see [resolve/CLAUDE.md](v2/pkg/engine/resolve/CLAUDE.md) |
| `datasource/graphql_datasource` | GraphQL subgraph datasource adapter |
| `postprocess` | Response post-processing passes (L1 cache optimization, fetch tree building) |

### Execution (execution/)

| Package | Purpose |
|---------|---------|
| `engine` | Federation engine config factory (`SubgraphCachingConfig`, `WithSubgraphEntityCachingConfigs`), E2E tests |
| `federationtesting` | Test federation services: accounts, products, reviews |
| `graphql` | GraphQL execution utilities |

## Key Architectural Decisions

- **Federation-first**: designed for federated GraphQL with entity resolution and `@key`/`@provides`/`@requires`
- **Arena-based allocation**: JSON values live on arena memory (no GC pressure), released per-request
- **Parallel resolution**: fetch tree with Sequence/Parallel nodes, 4-phase parallel execution with L1/L2 caching
- **Two-pass rendering**: pre-walk (validate, collect errors) + print-walk (render JSON)

## Entity Caching

Two-level entity caching system (L1 per-request + L2 external). See:
- [v2/pkg/engine/resolve/CLAUDE.md](v2/pkg/engine/resolve/CLAUDE.md) — full resolve package reference (resolution pipeline + caching internals)
- [ENTITY_CACHING_INTEGRATION.md](ENTITY_CACHING_INTEGRATION.md) — router integration guide (public APIs, configuration, examples)

## Testing Conventions

- **Exact assertions only**: use `assert.Equal` with exact expected values, never `GreaterOrEqual`, `Contains`, or vague comparisons
- **Snapshot comments**: every event line in `CacheAnalyticsSnapshot` assertions must explain **why** that event occurred
- **Cache log rule**: every `ClearLog()` must have `GetLog()` + assertions before the next `ClearLog()`
- **Federation test services**: `accounts`, `products`, `reviews` in `execution/federationtesting/`
- Run: `go test ./v2/pkg/engine/resolve/... -v` and `go test ./execution/engine/... -v`
