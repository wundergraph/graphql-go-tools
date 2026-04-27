# graphql-go-tools

GraphQL Router / API Gateway framework for Go. Federation-first, with query planning, parallel resolution, and entity caching.

Module: `github.com/wundergraph/graphql-go-tools` (Go 1.25, go.work workspace)

## Data Flow

```text
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

Two-level entity caching system (L1 per-request + L2 external).
See:
- [v2/pkg/engine/resolve/CLAUDE.md](v2/pkg/engine/resolve/CLAUDE.md) — full resolve package reference (resolution pipeline + caching internals)
- [ENTITY_CACHING_INTEGRATION.md](docs/entity-caching/ENTITY_CACHING_INTEGRATION.md) — router integration guide (public APIs, configuration, examples)
- [ENTITY_CACHING_ACCEPTANCE_CRITERIA.md](docs/entity-caching/ENTITY_CACHING_ACCEPTANCE_CRITERIA.md) — acceptance criteria with test references (includes AC-RS-01..07 for @requestScoped)

Critical L1 invariant:
- **Always-StructuralCopy L1 writes and reads**: L1 writes (`l1Cache` and
  `requestScopedL1`) always StructuralCopy onto `l.jsonArena`.
  Entity L1 uses `structuralCopyNormalizedPassthrough` — renames aliases
  to schema names via `astjson.Transform` but keeps ALL source fields
  (including @key fields not in ProvidesData) via `Transform.Passthrough`.
  L1 reads use `structuralCopyDenormalizedPassthrough` — restores aliases
  while preserving all accumulated fields.
  StructuralCopy clones container nodes (objects, arrays) on the arena
  while aliasing leaf nodes from the source — safe because all values
  share the same arena lifetime within a request.
  Transforms are ephemeral: built inline via reusable `l.transformEntries`
  slab, consumed by `l.parser.StructuralCopyWithTransform`, then discarded.
  Merges into an existing L1 entry use the working-copy-and-swap pattern:
  StructuralCopy the existing entry into a working copy,
  run `astjson.MergeValues` against the working copy,
  and store either the working copy (on success) or the fresh incoming value (on merge failure).
  Never mutate the live cache entry in place — `MergeValues` is non-atomic on failure
  and a partial mutation would corrupt every sibling L1 key pointing at the same entry.
  L2 writes use non-passthrough `structuralCopyNormalized` which projects
  to ProvidesData fields only (rename + drop unlisted fields).

### @requestScoped Coordinate L1 (symmetric model)

Separate per-request `map[string]*astjson.Value` (`requestScopedL1`) on the Loader.
Main-thread only — read and written from `tryRequestScopedInjection` and `exportRequestScopedFields`,
which run on the resolver's main thread in parallel Phase 1.5, parallel Phase 3.5,
and `resolveSingle`.

**Directive (composition-side)**:
```graphql
directive @requestScoped(key: String!) on FIELD_DEFINITION
```

**Semantics**: purely symmetric — every field annotated with `@requestScoped(key: "X")`
in the same subgraph shares the same L1 entry `{subgraphName}.X`. There is no
receiver/provider distinction. Each participating field is BOTH a reader (hint) AND
a writer (export). Whichever field is resolved first populates L1; subsequent fields
with the same key inject from L1 and may skip their fetch.

Composition validates `key` is mandatory and warns when a key is declared on only
one field in the subgraph (the directive is meaningless without a second reader).

Key files:
- `v2/pkg/engine/resolve/fetch.go` — `RequestScopedField` carries `ProvidesData *Object` for alias-aware normalization
- `v2/pkg/engine/resolve/loader.go` — `requestScopedL1 map[string]*astjson.Value`, injection in `resolveParallel` Phase 1.5 + 3.5 and `resolveSingle`
- `v2/pkg/engine/resolve/loader_cache.go` — `tryRequestScopedInjection` and `exportRequestScopedFields` use `validateItemHasRequiredData` and ephemeral normalize / denormalize transforms via `structuralCopyNormalized` / `structuralCopyDenormalized` (the same StructuralCopy-driven pipeline as entity L1/L2)
- `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource.go` — `ConfigureFetch` emits a `RequestScopedField` for every @requestScoped field (symmetric)
- `v2/pkg/engine/plan/federation_metadata.go` — `RequestScopedField` (no more `ResolveFrom`), `RequestScopedExportsForField` returns the field's own L1 key
- `v2/pkg/engine/plan/visitor.go` — `configureFetchCaching` populates `ProvidesData` and rewrites `FieldName`/`FieldPath` to the outer query's alias via `populateRequestScopedFieldsProvidesData`

Critical invariants:
- **Field widening check**: `tryRequestScopedInjection` must verify the cached value has ALL fields
  listed in `hint.ProvidesData` (alias-aware `*Object`) before injecting, via `validateItemHasRequiredData`.
  Otherwise a narrow root query (`{id, name}`) poisons the L1 for a wider entity fetch (`{id, name, email}`).
  Use collect-then-inject: verify all hints first, only mutate items if ALL succeed.
  Never partial-inject — a later hint failure must leave items untouched.
- **Copy-on-inject**: cached values must be StructuralCopy'd via `structuralCopyDenormalized`
  before injection to prevent pointer aliasing with the response data tree.
- **Copy-on-export**: `exportRequestScopedFields` must ALSO copy values via
  `structuralCopyNormalized` before storing in `requestScopedL1`.
  StructuralCopy creates independent container nodes while aliasing leaf values
  on the same arena — safe for same-arena, same-request lifetime.
- **L1 gating**: `tryRequestScopedInjection` and `exportRequestScopedFields` must check
  `l.ctx.ExecutionOptions.Caching.EnableL1Cache`. The coordinate L1 is part of the L1 cache layer
  and must be disabled when L1 is disabled per-request.
- **Trace reporting (LoadSkipped)**: when injection succeeds and fetch is skipped,
  set `ensureFetchTrace(f).LoadSkipped = true` at ALL call sites (parallel Phase 1.5 + 3.5 and 3 single fetch variants).
- **Trace reporting (L1 hit counters)**: when injection succeeds, set
  `res.cacheTraceRequestScopedHits = res.cacheTraceEntityCount`. The `buildCacheTrace` function
  folds these into `L1Hit` / `L1Miss` so the trace UI correctly shows a red L1 hit instead of
  stale L1 misses recorded during Phase 1. Never mutate `cacheTraceL1Hits`/`cacheTraceL1Misses`
  directly at the injection site — use the dedicated counter and fold at trace-build time.
- **InterfaceObject mapping**: the planner resolves concrete entity types (Article) to interface types
  (Personalized) via `InterfaceObjects` config to find @requestScoped fields on the interface.

### Subscription Entity Caching

`SubscriptionEntityPopulationConfiguration` requires BOTH `TypeName` AND `FieldName` to be set.
The lookup method `FindByTypeAndFieldName` matches on both fields.
If `FieldName` is empty, the lookup always fails and subscription cache populate/invalidate silently does nothing.

The router's `factoryresolver.go` must set `FieldName: cp.FieldName` (populate) and `FieldName: ci.FieldName` (invalidate)
when creating these configs.

### @requestScoped Alias Handling

The coordinate L1 cache is fully alias-aware via the unified `*Object`/ProvidesData
pipeline shared with entity L1 and L2:
- **L1 key** is `{subgraphName}.{key}` — alias-independent by construction
- **L1 stored value** uses schema field names (aliases normalized away via `structuralCopyNormalized` with ephemeral Transform)
- **Widening check** uses `validateItemHasRequiredData` against the query's `ProvidesData`
- **Denormalized read** via `structuralCopyDenormalized` re-applies aliases for the current query

Planner populates `ProvidesData` on `RequestScopedFields` in `configureFetchCaching` by
locating the matching sub-Object in `plannerObjects[fetchID]` and rewriting
`FieldName`/`FieldPath` to the outer query's alias when needed.

### Per-Request Cache Control Headers

The router supports per-request cache control via headers (for debugging / playground):
- `X-WG-Disable-Entity-Cache: true` — disable both L1 and L2
- `X-WG-Disable-Entity-Cache-L1: true` — disable L1 only
- `X-WG-Disable-Entity-Cache-L2: true` — disable L2 only

These headers are gated on `reqCtx.operation.traceOptions.Enable` (i.e., dev mode or a valid studio
request token) to prevent production abuse. The gate is in `GraphQLHandler.cachingOptions` in
`router/core/graphql_handler.go`. Disabling L1 via these headers also disables @requestScoped
coordinate L1 (since it shares the `EnableL1Cache` flag).

## Code Comment Conventions

**Never reference pull requests, issue numbers, review threads, or reviewer names in code comments.**

Comments live in the codebase forever and outlive the workflow context they were written in.
A `PR #1259` reference is meaningful for two weeks and noise for the next ten years.
Reviewer attribution (`as requested in ysmolski's review`, `addresses SkArchon's comment`) belongs in commit messages and PR descriptions, never in source files.

If a comment exists to explain a non-obvious behavior, explain the **behavior**, not the historical reason it was added.

```go
// CORRECT — explains the invariant
// isEntityRootField previously compared a non-normalized current path against a
// normalized boundary path. Without normalizing here first, queries that wrap the
// boundary in `... on User { ... }` cause the prefix check to silently fail.

// WRONG — references the PR / review / ticket where the fix was discussed
// Regression guard for the A42 bug in PR #1259 raised by ysmolski:
// isEntityRootField previously compared a non-normalized current path...
```

This applies to all code comments — production, tests, doc comments, file headers.
Commit messages may reference PRs and reviewers; code may not.

## Testing Conventions

**Before writing or modifying any test, read the package's `CLAUDE.md` if one exists.**
Package-level conventions are mandatory and stricter than the universal rules below.
Known package conventions:
- [v2/pkg/engine/resolve/CLAUDE.md](v2/pkg/engine/resolve/CLAUDE.md) — unit and integration tests for the resolve engine.
- [execution/engine/CLAUDE.md](execution/engine/CLAUDE.md) — E2E tests against the federation gateway. **Stricter rules apply — see "E2E rules" below.**

### Universal rules (every package)

- **Exact assertions only**: use `assert.Equal` with exact expected values.
  Never use `GreaterOrEqual`, `Contains`, `Greater`, or any vague comparison.
  If you do not know the expected value, investigate until you do.
- **Assert entire structs**: always `assert.Equal` on the complete struct.
  Never iterate over fields with individual assertions.
  For large structs, construct the full expected value inline anyway.
- **Inline literal data**: GraphQL queries, cache keys, byte sizes, expected JSON responses must appear inline at the assertion or setup site that uses them.
  Never hidden in file-level `const` blocks or shared vars that force reviewers to jump around.
- **Snapshot comments**: every event line in a `CacheAnalyticsSnapshot` (or any other event-stream assertion) must have a brief trailing comment explaining **why** that event occurred.
- **Cache log rule**: every `defaultCache.ClearLog()` must be followed by `GetLog()` + full assertions before the next `ClearLog()` or end of test.
  Never clear a log without verifying its contents.
- **Multi-key / multi-event struct literals must wrap one item per line**:
  cache log entries, snapshot events, and any struct literal with two or more nested slices, maps, or long string fields are unreadable on a single line.
  Format vertically.

  ```go
  // CORRECT — vertical, scannable
  wantLog := []CacheLogEntry{
      {
          Operation: "get",
          Keys: []string{
              `{"__typename":"Query","field":"cat"}`,
              `{"__typename":"Query","field":"me"}`,
          },
          Hits: []bool{false, false},
      },
      {
          Operation: "set",
          Keys:      []string{`{"__typename":"Query","field":"me"}`},
      },
  }

  // WRONG — single 200-character line, eye has to parse comma-by-comma
  wantLog := []CacheLogEntry{
      {Operation: "get", Keys: []string{`{"__typename":"Query","field":"cat"}`, `{"__typename":"Query","field":"me"}`}, Hits: []bool{false, false}},
  }
  ```

### E2E rules (under `execution/engine/`)

In addition to the universal rules above, [execution/engine/CLAUDE.md](execution/engine/CLAUDE.md) requires:

- **Self-contained subtests**: each `t.Run` must be independently readable top to bottom.
  **Duplication across subtests is preferred over sharing.**
  Do NOT extract setup into shared helpers like `newXxxFederationTestEnv(...)`.
  Do NOT define config structs as named vars when they are used in only one subtest.
- **Inline setup**: cache instances, tracker setup, gateway options, context, and URL parsing belong inside each subtest body.
- **Inline GraphQL queries**: use `QueryStringWithHeaders` with the query string inline.
  Do not load queries from external files.
- **No new shared test helpers** in `execution/engine/` without explicit approval — they violate the self-contained-subtest rule.

### LLM agent self-check (mandatory)

Before writing or editing any test, ask yourself:

| If you are about to... | STOP and instead... |
|---|---|
| Create a `newXxxEnv(...)` style helper used by multiple subtests in `execution/engine/` | Inline the setup into each subtest. |
| Pull a config struct out of a `t.Run` body into a top-level var or helper used once | Inline it back into the subtest. |
| Put two or more `Keys`/`Hits`/event-list entries on one line of a struct literal | Wrap to one item per line. |
| Add a test under `execution/engine/` | Re-read [execution/engine/CLAUDE.md](execution/engine/CLAUDE.md) first. |
| Add a test under `v2/pkg/engine/resolve/` | Re-read [v2/pkg/engine/resolve/CLAUDE.md](v2/pkg/engine/resolve/CLAUDE.md) first. |
| Use `assert.Contains`, `assert.GreaterOrEqual`, or any partial assertion | Investigate the actual expected value and use `assert.Equal`. |

If you find yourself extracting shared test scaffolding "to reduce duplication" in `execution/engine/`, that is the smell.
Duplication is the convention.

### Federation test services

`accounts`, `products`, `reviews` live in `execution/federationtesting/`.

### Run tests

```sh
go test ./v2/pkg/engine/resolve/... -v
go test ./execution/engine/... -v
```
