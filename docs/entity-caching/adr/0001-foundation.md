# ADR-0001: Foundation & Architecture for Entity Caching

## Status

Proposed

## Context

This project adds a two-level entity cache to the GraphQL router engine.
The engine resolves federated GraphQL queries by walking a tree of fetches against subgraphs.
Each fetch either fetches a root field (`Query.topProducts`) or resolves an entity by its `@key` (the federated `_entities` lookup).
The goal of entity caching is to avoid re-fetching the same entity or root field from a subgraph when the answer is already known, either within a single request (L1) or across requests (L2, an external store such as Redis).

If you are new to this feature, the rest of this section gives you the minimum mental model.
The deeper references are linked at the end.

### The two cache levels

- **L1** is a per-request, in-memory cache.
  It lives for the lifetime of one request, is read and written only on the resolver's main thread, and applies only to entity fetches (a root field has no prior entity data to reuse).
  Its purpose is to deduplicate identical entity lookups that occur within one query plan.
- **L2** is an external cache, owned and implemented by the router (the engine never opens a Redis connection).
  It survives across requests and applies to both root-field and entity fetches.
  The engine talks to it only through a narrow interface.

### Where the engine plugs in

Two engine components do the actual resolution:

- the **loader** (`v2/pkg/engine/resolve/loader.go`) drives fetches, batching, and the merge of fetched JSON back into the response tree,
- the **resolvable** renders the final JSON response.

Both are large, hot, and correctness-critical.
The central design tension of this ADR is that caching must hook into the loader's fetch and merge path **without** rewriting the loader.

### The JSON substrate

The engine represents response data as `astjson.Value` trees allocated on an arena (`go-arena`).
Arena memory is released per request, so there is no GC pressure during a request, but it also means any value that escapes the request lifetime is unsafe to keep.
The cache layer leans heavily on one astjson primitive, **`StructuralCopy`**, which clones container nodes (objects, arrays) onto an arena while aliasing scalar leaves and object keys from the source.
This is cheap (one tree walk, no re-parse) and is the basis for keeping the cache and the live response tree from corrupting each other.
Crucially, the astjson APIs this feature needs are **unreleased**: they exist only on an open astjson PR branch, so the cache work cannot compile against the released astjson tag.
This makes the astjson release a hard prerequisite, captured in [05-ASTJSON-PRIMITIVES.md](../05-ASTJSON-PRIMITIVES.md).

### What "the foundation" must establish

The full feature spans many directives, planner passes, analytics events, and operation-type-specific behaviors (queries vs mutations vs subscriptions).
Trying to land all of that in one change would produce an unreviewable diff against the engine's most sensitive files.
This ADR exists to fix the **architecture and the seams** first, so that every later directive can land as a small, additive PR against stable contracts.
The directive specs are inventoried in [02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md) and detailed under [directives/](../directives/); the clean-architecture and seam picture is in [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md).

## Decision

The foundation PR ships the **interfaces, the loader seam, and the documentation** for entity caching.
It does **not** ship any full directive implementation.
Concretely, the foundation establishes the seven decisions below.

### 1. A minimal integration seam into the loader and resolvable

Caching enters the loader at exactly **five call sites plus one boolean per fetch**, and the resolvable is left untouched.

The five seams are:

1. a **pre-fetch cache lookup** that, before a fetch is dispatched, asks the cache whether the entities or root field are already known,
2. a **post-fetch cache write** that, after a fetch returns, offers the fresh data to the cache,
3. a **merge point** where cached data is folded into the response tree,
4. a **per-request L1 read/write** for entity fetches on the main thread,
5. an **invalidation hook** that processes subgraph-supplied invalidation hints and mutation/subscription effects.

The single per-fetch boolean is `cacheSkipFetch` (a fetch fully served from cache is not dispatched to the subgraph), with a companion `cacheMustBeUpdated` driving the write side.
The merge funnel is the existing `mergeResult` path in the loader (around `loader.go:1510`): when `cacheSkipFetch` is set, that funnel merges the cached value (`CacheKey.FromCache`) into the response via `astjson.MergeValues`, with a load-bearing `StructuralCopy` first.

The collaborator object that owns all cache logic is a thin **`entityCache`** type held by the loader.
The loader calls into it at the five seams; the cache logic (key rendering, L1 map, L2 batching, merge orchestration, analytics emission) lives in the collaborator and its files (`loader_cache.go`, `loader_cache_transform.go`, `caching.go`, `cache_analytics.go`).
The behavior-bearing interfaces the loader depends on are extracted: **`LoaderCache`** (the L2 backend) and **`CacheKeyTemplate`** (key rendering).

### 2. The L1/L2 cache interface

**L2** is defined by the `LoaderCache` interface that the router implements:

```go
Get(ctx, keys []string) ([]*CacheEntry, error)   // slice same length as keys, nil = miss
Set(ctx, entries []*CacheEntry) error            // TTL is per-entry, NOT a Set parameter
Delete(ctx, keys []string) error
```

A `CacheEntry` carries `Key`, `Value []byte` (opaque JSON payload), `TTL`, `RemainingTTL` (set by the backend on read), and `WriteReason`.
Two points are deliberate and contradict the older integration doc, which is stale here:

- `Set` takes **no** `ttl` argument; TTL rides on each `CacheEntry.TTL`.
  This is more flexible because one `Set` call can mix regular and negative-cache TTLs.
- Reads are issued in **bulk on the main thread** (`bulkL2Lookup`): one `Get` per batch of instances, and a single bulk-`Get` error fails the whole batch back to the subgraph rather than failing the request.
  The interface still documents `Get`/`Set`/`Delete` as concurrency-safe so backends remain future-proof, but the current call pattern is main-thread bulk.

**L1** is **not** a router-facing interface.
It is an engine-internal `map[string]*astjson.Value` on the loader, read and written only on the main thread, and gated per request by `ctx.ExecutionOptions.Caching.EnableL1Cache`.
The router never sees L1 except through that toggle.

The router registers named L2 backends in `ResolverOptions.Caches map[string]LoaderCache`; every declarative config selects one by `CacheName`.

### 3. The cache-key contract

A cache key is produced by a **`CacheKeyTemplate`**, an engine-internal interface (see decision 7 on why it is internal):

```go
RenderCacheKeys(a arena.Arena, ctx *Context, items []*astjson.Value, prefix string) ([]*CacheKey, error)
IsEntityFetch() bool
BatchEntityKeyArgumentPath() []string
EntityMergePath(pp PostProcessingConfiguration) []string
```

Two implementations exist: an **entity** template (key shape `{"__typename":"User","key":{"id":"123"}}`, built from `@key` fields only, never `@requires`) and a **root-field** template (key shape `{"__typename":"Query","field":"x","args":{...}}`, args sorted alphabetically).
Templates are alias-independent by construction; alias awareness lives in the separate `ProvidesData` field tree (see decision 5).

The key contract the router must mirror for manual invalidation is the **transform pipeline**, applied identically on read, write, and delete:

```
GlobalCacheKeyPrefix  ->  subgraph header-hash prefix  ->  L2CacheKeyInterceptor
```

Any caller that forgets `GlobalCacheKeyPrefix` will target the wrong key and leave stale entries; the spec calls this out as an easy router-side footgun.
The exported helper `ParseKeyFields(selectionSet string) []KeyField` lets the router compute entity keys from a `@key` selection-set string for invalidation and analytics.

### 4. The analytics sink

Analytics is **pull-based and single-shot**.
The router sets `ctx.ExecutionOptions.Caching.EnableCacheAnalytics` per request; when false the collector is a no-op with zero overhead.
After resolution the router calls `ctx.GetCacheStats()` exactly once, which **snapshots and releases** the pooled collector and returns a `CacheAnalyticsSnapshot`.

The snapshot is a flat struct of event slices (`L1Reads`/`L2Reads`, `L1Writes`/`L2Writes`, `FetchTimings`, `ShadowComparisons`, `MutationEvents`, plus error and header-impact events) with convenience derivations (`L1HitRate`, `CachedBytesServed`, `EventsByEntityType`, etc.).
A separate, finer-grained surface — per-fetch `CacheTrace` in response extensions — is gated on `ctx.TracingOptions` (`Enable && !ExcludeCacheStats`), not on the analytics flag.

The foundation exposes `GetCacheStats()` as the **single sanctioned read path**.
The collector also has many exported `Record*`/`Merge*` methods flagged in code as "for external consumers"; the foundation treats `GetCacheStats()` as the contract and defers a decision on whether those lower-level methods stay exported (see Open Questions).

### 5. The arena / StructuralCopy strategy

Every boundary where the cache and the response tree meet is crossed with **`StructuralCopy`**, never a raw pointer hand-off.
The invariant: clone container nodes onto the request arena, alias leaves from the source; safe only because cache values and response values share the same arena lifetime within one request.

Four loader helpers wrap the two astjson primitives (`StructuralCopy`, `StructuralCopyWithTransform`) and are the single seam for all four directions:

- **L1 write**: `structuralCopyNormalizedPassthrough` — rename aliases to schema names but keep **all** source fields (including `@key` fields not in `ProvidesData`), via `Transform.Passthrough = true`.
- **L1 read**: `structuralCopyDenormalizedPassthrough` — restore aliases, keep all accumulated fields.
- **L2 write**: `structuralCopyNormalized` — rename **and project** to `ProvidesData` fields only (`Passthrough = false`), then `MarshalTo` to heap bytes for the external store.
- **L2 read**: `structuralCopyDenormalized` — restore aliases, project.

`Transform.Passthrough` is the L1-vs-L2 switch: keep-everything for L1 (so sibling fetches accumulate), project-to-listed for L2 (so entries are minimal and self-contained).
Transforms are ephemeral, built inline on reusable slabs and discarded.
Merges into an existing L1 entry use **working-copy-and-swap**: `StructuralCopy` the live entry into a working copy, `MergeValues` against the copy, store the copy on success or the fresh incoming value on failure.
The live entry is never mutated in place, because `MergeValues` is non-atomic on failure and a partial mutation would corrupt every sibling L1 key pointing at the same entry.
`DeepCopy` (full clone including scalars) is used in exactly one place — heap isolation of per-request `Variables` — because that is the only heap/arena boundary the foundation crosses.
The astjson primitive set, including the breaking `MergeValues` signature change (`changed bool` return dropped), is specified in [05-ASTJSON-PRIMITIVES.md](../05-ASTJSON-PRIMITIVES.md).

### 6. ProvidesData: the alias-aware field shape

Alongside the cache-key template, each fetch carries a `ProvidesData *Object` field tree describing what the fetch yields, including aliases and per-field argument metadata.
Cache keys come from the template; **normalization, field-widening checks, and L1 optimization all key off `ProvidesData`**.
This split is part of the foundation contract because the directive PRs populate `ProvidesData` (planner side) and consume it (resolver side).

### 7. The foundation ships seam + interfaces + docs, NOT directives

The foundation PR is explicitly scoped to:

- the loader seam (five call sites plus the `cacheSkipFetch`/`cacheMustBeUpdated` booleans) and the `entityCache` collaborator,
- the interfaces (`LoaderCache`, `CacheKeyTemplate`) and the data shapes (`CacheEntry`, `CachingOptions`, `FetchCacheConfiguration`, `ProvidesData`, the analytics snapshot/event types, the cache-trace types),
- the StructuralCopy helper layer,
- the documentation set this ADR belongs to.

It does **not** ship the composition-side directive grammar, the per-directive planner wiring (`cachingPlannerState`, `configureFetchCaching`), the `@requestScoped` selection-set widening pass, the `optimizeL1Cache` postprocess, or any production cache backend.
Each of those lands as its own stacked PR against the now-stable seam.
The PR sequencing is in [03-PR-PLAN-graphql-go-tools.md](../03-PR-PLAN-graphql-go-tools.md) (engine) and [04-PR-PLAN-router.md](../04-PR-PLAN-router.md) (router); per-directive ADRs are `adr/00NN-<name>.md`; the implementation loop is [08-EXECUTION-RUNBOOK.md](../08-EXECUTION-RUNBOOK.md).

### Why this seam keeps the loader mostly untouched

The loader is the engine's hottest, most correctness-sensitive file.
The seam is designed so that, when caching is disabled, the loader behaves exactly as before:
the five hooks are guard-clause early-returns, and the one boolean per fetch defaults to "fetch normally."
All cache mechanism — key rendering, the L1 map, L2 batching, transform-driven copies, merge orchestration, analytics — lives in the `entityCache` collaborator and its sibling files, not inline in the loader's resolution logic.
The loader's only new responsibility is to **call the collaborator at the right moments and honor `cacheSkipFetch`**, and to route the cached value through its existing `mergeResult` funnel.
Because the merge funnel already exists for non-cache reasons, caching reuses it rather than adding a parallel merge path.
This means the loader diff is small and mechanical, the cache logic is independently testable against the interfaces, and the resolvable (rendering) needs no changes at all — cached data is indistinguishable from fetched data once merged into the response tree.
The result is that future directive PRs touch the collaborator and the planner, almost never the loader, which is exactly what makes the stacked-PR plan safe to execute.

## Consequences

### Positive

- The loader and resolvable stay almost entirely as-is; the cache-disabled path is unchanged behavior.
- Cache mechanism is isolated behind two interfaces and one collaborator, so it is unit-testable without a live subgraph or Redis.
- Every later directive is an additive PR against frozen contracts, keeping each diff small and reviewable.
- L2 backends are pluggable and named; the router owns all I/O, the engine owns none.
- The arena/StructuralCopy discipline gives same-request cache isolation without GC cost and without byte round-trips on the hot path (L2 serialization to bytes happens only at the external boundary).

### Negative / costs

- The cache layer hard-depends on **unreleased** astjson primitives; an astjson release must land first, blocking everything.
  The breaking `MergeValues` signature change means the engine will not compile against the current released astjson tag.
- `CacheKeyTemplate` is coupled to `arena.Arena` and `astjson.Value`, so it is not cleanly router-facing; the router configures keys declaratively but cannot implement the template itself.
- StructuralCopy's leaf-aliasing is only safe under same-arena/same-request lifetime; any future code that lets a copied value outlive its source arena (without `MarshalTo` to heap bytes) is a use-after-free.
- The L1 map and `@requestScoped` coordinate L1 are main-thread-only; correctness depends on that and would break under naive parallelization.
- Several footguns are inherent to the contract and must be documented, not designed away: byte-identical cache keys across read/write/delete, the full key-transform pipeline for manual invalidation, and `SubscriptionEntityPopulationConfiguration.FieldName` being mandatory.

### Risks to manage downstream

- A single bulk-`Get` error now fails an entire cache batch back to the subgraph; backends must keep `Get` reliable.
- `MergeValues` is non-atomic on failure; the working-copy-and-swap pattern must be preserved everywhere L1 entries are merged.

## Alternatives Considered

### A. Rewrite the loader to be cache-aware natively

Bake cache lookup, L1, and L2 directly into the loader's fetch and merge logic instead of a collaborator.
**Rejected.**
It would couple the engine's hottest file to the entire cache feature, make the cache-disabled path harder to keep identical to today's behavior, and force every directive PR to touch the loader.
The thin-collaborator seam achieves the same runtime behavior with a fraction of the loader diff.

### B. Make L1 a router-facing interface like L2

Expose L1 through the same `LoaderCache` shape so the router could supply an in-memory implementation.
**Rejected.**
L1 is request-scoped, main-thread-only, and operates on arena `astjson.Value` trees — exposing it would force arena/astjson types into the router API and invite cross-arena lifetime bugs.
L1 stays engine-internal behind a single per-request boolean.

### C. Keep the older integration doc's signatures (`Set(ctx, entries, ttl)`, `RenderCacheKeys(ctx, fetch, *keys)`)

Standardize on the previously-documented API surface.
**Rejected** as stale.
Per-entry TTL on `CacheEntry` is strictly more capable (mixed regular and negative-cache TTLs in one `Set`), and the real `RenderCacheKeys` signature reflects the arena/astjson coupling that makes the template engine-internal.
The foundation standardizes on the actual code, not the doc.

### D. Ship the whole feature in one PR

Land directives, planner wiring, postprocess, analytics, and backend together.
**Rejected.**
The diff against the loader and planner would be unreviewable and high-risk.
The interfaces-plus-seam-first approach is what makes the stacked plan tractable.

### E. Push-based analytics (engine calls a router sink during resolution)

Have the engine invoke a router callback per cache event instead of snapshotting at the end.
**Rejected for the foundation.**
Pull-based `GetCacheStats()` keeps the disabled path zero-overhead, avoids interleaving router code into hot resolution, and gives one clear release point for the pooled collector.
Subscription writes/invalidations are the deliberate exception, since they are inherently event-driven and use the dedicated `OnSubscriptionCacheWrite`/`OnSubscriptionCacheInvalidate` callbacks.

## References

- [00-OVERVIEW.md](../00-OVERVIEW.md) — executive summary and navigation
- [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) — clean architecture and integration seam
- [05-ASTJSON-PRIMITIVES.md](../05-ASTJSON-PRIMITIVES.md) — the astjson dependency and unreleased-primitive prerequisite
- [06-TEST-AND-BENCH-PLAN.md](../06-TEST-AND-BENCH-PLAN.md) — test and benchmark plan
- [07-UNRELATED-FINDINGS.md](../07-UNRELATED-FINDINGS.md) — out-of-scope findings
- `v2/pkg/engine/resolve/loader.go`, `loader_cache.go`, `caching.go`, `cache_analytics.go`, `context.go` — the engine seam and cache types
