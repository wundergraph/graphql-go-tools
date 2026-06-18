# ADR-0005: @requestScoped caching support

## Status

Proposed

## Context

`@requestScoped` is the **one new directive** the entity-caching feature introduces.
Every other caching concept either reuses an existing federation directive (`@key`, `@requires`, `@provides`) or arrives as a Go configuration struct synthesised by the router.
`@requestScoped` is declared in subgraph SDL and validated at composition time:

```graphql
directive @requestScoped(key: String!) on FIELD_DEFINITION
```

### The problem it solves

Some fields return a value that is **identical for the whole request inside one subgraph**, regardless of which entity the value is attached to.
The canonical case is a "current viewer" or per-session value that the same subgraph would re-derive on every entity fetch.
Without coordination, a federated query that touches many entities pays for the same subgraph round-trip many times to recompute one request-constant value.

`@requestScoped(key: "X")` lets the schema author declare that coordination point.
Its model is **purely symmetric**: every field annotated with the same `key` in the same subgraph shares one per-request coordinate L1 entry, keyed `{subgraphName}.X`.
There is no provider/receiver distinction.
Whichever annotated field resolves first **writes** its value into the coordinate L1; every later field with the same key **reads** it back and may skip its own subgraph fetch entirely.
Each participating field is therefore both a reader (an injection hint) and a writer (an export).

### What the foundation (ADR-0001) already provides

`@requestScoped` is a side-branch of the dependency graph.
It depends only on the foundation and on the `@provides` `ProvidesData` machinery; it is **independent of L2, mutations, and subscriptions**.
The foundation already supplies everything the run-time half needs:

- The **L1 layer concept** and its single per-request gate, `ctx.ExecutionOptions.Caching.EnableL1Cache`.
  The coordinate L1 is part of the L1 layer and rides on this same flag — disabling L1 disables `@requestScoped` too.
- The **StructuralCopy copy primitives** and the four loader helpers (`structuralCopyNormalized` / `structuralCopyDenormalized` and their passthrough variants), including the arena-lifetime invariant that every cache boundary is crossed with a copy, never a raw pointer hand-off.
- The **`ProvidesData *Object`** alias-aware field shape carried on each fetch, and the widening validator `validateItemHasRequiredData` that checks a cached value has all fields the current query needs.
- The **`FetchCacheConfiguration`** struct on every fetch, which is the additive carrier for any new per-fetch caching annotation.
- The **main-thread-only resolution discipline**: all cache logic runs on the resolver's main thread; goroutines do subgraph HTTP only.

The detailed contract (data shapes, the symmetric semantics, the widening check, the copy rules, alias handling) lives in the spec: [../directives/request-scoped.md](../directives/request-scoped.md).
This ADR records only the integration decision.

## Decision

Implement `@requestScoped` as a **separate per-request coordinate L1** that plugs into the existing foundation seam **without modifying the loader/resolvable hot path**.
It adds new plan-time metadata, one fetch annotation, and two resolver hooks — all additive, all gated on the foundation's existing `EnableL1Cache` flag.
The work ships as its own stacked PR on top of the foundation: **gqtools PR 17-20 / PR-REQUEST-SCOPED** (see [0001-foundation.md](0001-foundation.md) for why directives stack on the frozen seam).

### 1. Plan-time metadata (composition + planner)

- **Composition** introduces the directive grammar and validation.
  `key: String!` is mandatory.
  Composition **warns** when a `key` appears on only one field in a subgraph, because a lone reader can never coordinate with a second field and the annotation is meaningless.
- The planner carries the directive on `FederationMetaData.RequestScopedFields` as a flat list of `RequestScopedField{FieldName, TypeName, L1Key}`, where `L1Key` is `{subgraphName}.{key}` — alias-independent by construction.
  Lookups are symmetric: `RequestScopedFieldsForType(typeName)` and `RequestScopedExportsForField(typeName, fieldName)` both return the field's own L1 key, with no separate "resolve-from" notion.
- The planner's caching pass (`configureFetchCaching`) populates a resolver-side `RequestScopedField` per annotated field selected at a fetch location, and `populateRequestScopedFieldsProvidesData` attaches the alias-aware `ProvidesData *Object` for that field by locating the matching sub-`Object` in the planner's per-fetch objects, rewriting `FieldName`/`FieldPath` to the outer query's alias where needed.
- For interface objects, the planner resolves concrete entity types to their interface types via `InterfaceObjects` config so it can find `@requestScoped` fields declared on the interface.

### 2. Fetch annotation (additive, no new fetch type)

The directive rides on the existing `FetchCacheConfiguration` already present on every fetch — no new fetch type, no signature change:

```go
RequestScopedFields []RequestScopedField   // on resolve.FetchCacheConfiguration
```

The resolver-side `RequestScopedField` is the tiny on-the-wire surface that carries the directive from planner to resolver:

- `FieldName` — the response key at the fetch location (alias if present, else schema name), used when writing an injected value onto entity items.
- `FieldPath` — the path in response data, using response keys (aliases), used to read the value out for export.
- `L1Key` — the coordinate key `{subgraphName}.{key}`.
- `ProvidesData *Object` — the alias-aware value shape at this fetch location, used for the widening check on inject and the normalize/denormalize transforms on both sides.

The datasource's `ConfigureFetch` emits exactly **one** `RequestScopedField` per annotated field — symmetric, no reader/writer split.
When `RequestScopedFields` is empty (the common case), every hook below is a guard-clause early-return, so non-`@requestScoped` fetches pay nothing.

### 3. Resolver hooks (new state, no hot-path rewrite)

The run-time half is a **separate coordinate L1 map** on the Loader, distinct from the entity `l1Cache`:

```go
requestScopedL1 map[string]*astjson.Value   // main-thread only
```

It is allocated per request, read and written **only on the main thread**, and gated on `EnableL1Cache`.
Two new hook methods carry all the logic; they slot into existing phase boundaries and the loader's resolution shape is unchanged:

- **`tryRequestScopedInjection(res, cfg, items)`** — the reader.
  Collect-then-inject: it first verifies that **all** hints are satisfiable (each L1 entry exists, has `ProvidesData`, and passes `validateItemHasRequiredData`), materialising each value via a denormalizing StructuralCopy onto the arena.
  Only if **every** hint succeeds does it mutate items (one independent copy per item when there are several), set `res.fetchSkipped = true`, and return `true` to skip the fetch.
  On any failure it leaves items untouched (no partial injection).
- **`exportRequestScopedFields(res, cfg, items)`** — the writer.
  After merge, it samples the value from the first entity (or the root data for root-field fetches), normalizes it via `structuralCopyNormalized` (alias → schema name, arg → arg-hash), and stores it under `L1Key`.
  A merge into an existing entry uses the foundation's **working-copy-and-swap**: StructuralCopy the live entry, `MergeValues` into the copy, store the copy on success, keep the live entry intact on failure.

These hooks are invoked at the existing seams, all on the main thread:

- **Parallel Phase 1.5** — injection before launching HTTP goroutines.
- **Parallel Phase 3.5** — retry injection for hints that became satisfiable after sibling fetches produced the hinted data.
- **Parallel Phase 4** — export after merge.
- **`resolveSingle`** — the same inject/export bracket on the sequential path.

### 4. Correctness invariants honored

- **Field-widening (anti-poisoning)**: inject only when the cached value has all fields in the hint's `ProvidesData`; fail closed when `ProvidesData` is nil.
  A narrow root query must never poison the L1 for a wider entity fetch.
- **Copy-on-inject / copy-on-export**: cached values are StructuralCopy'd on both read and write to prevent pointer aliasing with the response tree, same-arena/same-request only.
- **L1 gating**: both hooks early-return when `EnableL1Cache` is false; the coordinate L1 is part of the L1 layer.
- **Trace reporting**: on a successful skip, set `LoadSkipped = true` on the fetch trace and `res.cacheTraceRequestScopedHits = res.cacheTraceEntityCount` at all call sites; `buildCacheTrace` folds the dedicated counter into `L1Hit`/`L1Miss` so the UI shows a red L1 hit rather than stale Phase-1 misses — never mutate the L1 hit/miss counters directly at the injection site.

### Why this does not touch the loader/resolvable hot path

The loader's `single` / `sequence` / `parallel` dispatch, the four-phase machinery, `mergeResult`, and the two-pass rendering are all unchanged.
The only additions are: one map field and a few state fields on the Loader, two collaborator methods, and call sites at phase boundaries that already exist.
When `@requestScoped` is unused or L1 is disabled, every hook is a no-op, so the disabled path is identical to today.
The resolvable needs no changes at all — injected data is indistinguishable from fetched data once written onto the response items.

## Consequences

### Positive

- A request that touches N entities pays **one** subgraph fetch for a request-constant field instead of N; the first resolver to produce the value short-circuits the rest.
- Purely additive: the diff lands in the planner caching pass, the datasource `ConfigureFetch`, and the cache collaborator — not the loader's resolution logic.
- The symmetric model removes an entire class of "who is the provider" configuration questions; any annotated field can satisfy any other with the same key.
- Reuses the foundation's `ProvidesData` widening check and StructuralCopy helpers, so it inherits the same isolation guarantees and the same copy-budget discipline as entity L1/L2.
- Establishes the pattern for **coordinate caches** keyed by something other than `@key`, which makes future request-constant caching directives cheap to add on the same seam.

### Negative / costs

- A second per-request map (`requestScopedL1`) and its main-thread-only constraint: correctness depends on never reading or writing it off the main thread, and would break under naive parallelization.
- Composition can only **warn**, not error, on a lone-key field; a misconfigured schema silently gets no benefit (the lone reader can never coordinate).
- The widening check must be exact and fail-closed; a relaxed check would let a narrow query poison the coordinate L1 for a wider fetch.
- The injection retry in Phase 3.5 adds a second pass over not-yet-satisfied hints; cost is bounded by the number of `@requestScoped` fetches, which is small in practice.

### Performance implications

- Net subgraph round-trips drop for request-constant fields; the savings scale with entity count.
- Per-fetch overhead when the feature is unused is zero (empty-slice guard clauses).
- When used, the cost is StructuralCopy on inject/export plus one widening walk per hint — all on the main thread, no goroutine or arena added.

### What becomes possible for later directives

- The coordinate-L1 mechanism is a reusable seam: any future "value is constant for the request within a subgraph" concept can reuse `requestScopedL1`, the gate, the copy helpers, and the inject/export hook shape without touching the loader again.

## Alternatives considered

### A. Provider/receiver (asymmetric) model

Designate one field as the provider that writes L1 and others as receivers that only read.
**Rejected.**
It requires composition to pick a provider and validate that exactly one exists, adds a `ResolveFrom`-style indirection to the metadata, and breaks when the "provider" field is not selected by a given query.
The symmetric model needs no provider election: whichever field resolves first writes, and the lookup returns the field's own key on both sides.
This is strictly simpler and more robust to selection-set variation.

### B. Reuse the entity `l1Cache` keyed on `@key` instead of a separate coordinate map

Store request-constant values in the existing entity L1 under entity keys.
**Rejected.**
The value is not entity-owned — it is request-constant across all entities — so an entity-keyed store would duplicate it per entity and could not be found by a field on a different entity type with the same `key`.
A separate map keyed `{subgraphName}.{key}` is the natural identity for a coordinate value and keeps the entity L1's `@key`-only invariant intact.

### C. Compute it as a planner-time constant or a shared variable

Resolve the request-constant value once up front and thread it through as a variable.
**Rejected.**
The value is produced by a subgraph at resolve time and depends on per-request context; it is not known at plan time.
Threading it as a variable would require the planner to model cross-fetch data dependencies it does not otherwise track, and would not generalise to multiple subgraphs or interface objects.
The run-time coordinate L1, populated by whichever fetch resolves first, captures the dependency naturally without new planner data-flow machinery.

## References

- [../directives/request-scoped.md](../directives/request-scoped.md) — the detailed `@requestScoped` contract (data shapes, semantics, widening, alias handling).
- [0001-foundation.md](0001-foundation.md) — the foundation seam, L1 layer, StructuralCopy invariants, and `ProvidesData` that this directive stacks on.
- [../01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) — §5 four-phase touchpoints (Phase 1.5 / 3.5), §9 per-request toggles.
- [../02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md) — directive taxonomy and PR mapping (PR-REQUEST-SCOPED).
- `v2/pkg/engine/resolve/loader.go`, `loader_cache.go`, `fetch.go` — `requestScopedL1`, `tryRequestScopedInjection`, `exportRequestScopedFields`, `validateItemHasRequiredData`, `RequestScopedField`.
- `v2/pkg/engine/plan/federation_metadata.go`, `node_selection_visitor_request_scoped.go`, `visitor.go` — planner metadata and `ProvidesData` population.
- `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource.go` — `ConfigureFetch` emits one `RequestScopedField` per annotated field.
