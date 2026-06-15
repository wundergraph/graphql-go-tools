# Architecture Specification: Entity Caching (Clean Re-implementation)

> Part of the entity-caching re-implementation document set.
> See [00-OVERVIEW.md](./00-OVERVIEW.md) for the executive summary and navigation,
> [02-DIRECTIVE-INVENTORY.md](./02-DIRECTIVE-INVENTORY.md) for the directive table,
> [05-ASTJSON-PRIMITIVES.md](./05-ASTJSON-PRIMITIVES.md) for the astjson dependency contract,
> and [adr/0001-foundation.md](./adr/0001-foundation.md) for the foundation decision record.

## 0. Who this document is for

This is the load-bearing architecture spec for a from-scratch re-implementation of entity caching in `graphql-go-tools`.
It assumes you have never seen this feature before.
It explains where caching attaches in the engine,
the two-level cache model,
the memory invariants that keep it correct,
and — most importantly — the **Integration Seam**:
the small set of interfaces and hook points that let us add caching while leaving the existing `loader` and `resolvable` code essentially untouched.

Leaving the existing resolution engine untouched is a **hard requirement**, not a preference.
The central section (§7) specifies exactly what that seam looks like.

---

## 1. The data flow, and where caching attaches

A GraphQL request moves through six stages:

```text
parse → normalize → validate → plan → resolve → response
```

Caching attaches at exactly **two** of these stages, and nowhere else.

- **plan** (compile-time, once per operation): the planner decides *what could be cached* and attaches a small, declarative configuration to each fetch.
  It does not touch any cache.
  It produces a cache-key *template* (how to compute a key), a *provides-shape* (what fields a fetch yields), and per-fetch flags (L2 on/off, TTL, cache name).
  This is covered by [03-PR-PLAN-graphql-go-tools.md](./03-PR-PLAN-graphql-go-tools.md) and the per-directive specs under [directives/](./directives/).

- **resolve** (run-time, once per request): the resolver executes the fetch tree.
  This is where caches are actually read and written.
  L1 (per-request) is consulted on the main thread before a fetch is dispatched.
  L2 (external) is consulted in a single bulk read per cache instance.
  After a fetch returns, both caches are populated.

`parse`, `normalize`, `validate`, and `response` rendering are caching-agnostic and stay untouched.
The cleanest mental model: **the planner annotates, the resolver acts, everything else is unaware.**

A small, important nuance: between `plan` and `resolve` there is a **post-processing** pass that walks the concrete fetch tree.
One post-process step (the L1 optimizer) is the *only* place that decides whether L1 is actually turned on for a given fetch.
This is described in §6 and in [03-PR-PLAN-graphql-go-tools.md](./03-PR-PLAN-graphql-go-tools.md).

---

## 2. The two-level model (L1 + L2)

Entity caching has two tiers with different lifetimes, scopes, and storage.

| Tier | Storage | Lifetime | Scope | Thread model |
|------|---------|----------|-------|--------------|
| **L1** | Plain `map[string]*astjson.Value` on the Loader | One request | Within a single resolution | Main thread only, no locking |
| **L2** | External backend behind the `LoaderCache` interface (Redis, in-memory, etc.) | Cross-request | Shared by all requests | Backend must be concurrency-safe; engine reads in one bulk call on the main thread |

**Why two tiers.**
A single federated query can ask for the same entity several times along different fetch paths.
L1 deduplicates *within* one request: the first fetch that produces `User:1234` populates L1,
and a later fetch for the same entity reads it back and skips its subgraph call.
L2 deduplicates *across* requests: an entity fetched on request A is served from the external cache on request B without any subgraph call at all.

**Key principle, shared by both tiers.**
Both L1 and L2 key entities on their `@key` fields only — never on `@requires` fields and never on arbitrary selected fields.
This keeps entity identity stable regardless of what a given query happens to select.
Field arguments are handled separately via a hash suffix (see §4), not by widening the key.

**What is cached at each tier.**
L1 applies only to **entity fetches** (a nested `_entities` fetch has prior entity data to key on; a root field does not).
L2 applies to both **entity fetches and root-field fetches**.

**Operation-type behavior** (the resolver relies on this; directive specs in [directives/](./directives/) pin it per directive):

- **Queries**: L1 → L2 → subgraph, then populate L1 + L2.
  A complete L1 hit skips L2 and the goroutine entirely.
- **Mutations**: always skip L2 *reads* (fetch fresh); skip L2 *writes* unless explicitly enabled; optionally delete impacted entity keys.
- **Subscriptions**: per event, either populate L2 with entity data or invalidate (delete) when only `@key` fields are present.

There is a third, narrow coordinate-L1 mechanism for `@requestScoped` fields.
It rides on the same L1 enable flag and the same copy primitives,
but is logically its own concern; see [directives/requestScoped.md](./directives/requestScoped.md).
This spec treats it as part of the L1 layer, not a separate tier.

---

## 3. Arena allocation and the StructuralCopy invariant

This is the single most important correctness concept in the whole feature.
Get this wrong and you corrupt cached data silently.

### 3.1 Arena, in one paragraph

The engine allocates all `*astjson.Value` nodes on a per-request **arena** — a bump-pointer region freed wholesale at the end of the request, with no GC tracing into it.
Within one request, every value (response data, parsed subgraph bytes, L1 cache entries) shares the same arena lifetime.
The arena is **not** thread-safe, so only the main thread allocates on it.

### 3.2 The three copy primitives

The foundation depends on three astjson primitives (full contract in [05-ASTJSON-PRIMITIVES.md](./05-ASTJSON-PRIMITIVES.md)):

- **`StructuralCopy(arena, v)`** — clones *container* nodes (objects, arrays) onto the arena while *aliasing* leaf nodes (strings, numbers, bools, nulls) and object keys from the source.
  Cheap: one tree walk, no re-parse, no byte round-trip.
  Safe **only** when source and destination share the same arena lifetime (same request).
  This is the workhorse for every L1 read and write and every cache-into-response merge.

- **`StructuralCopyWithTransform(arena, v, t)`** — same container-clone/leaf-alias semantics, plus per-field rename / project / passthrough driven by a `Transform`.
  This is how alias normalization and argument-aware keys are applied during a copy.

- **`DeepCopy(arena, v)`** — clones *everything*, including scalar payloads, so the result shares no memory with the source.
  Needed only when crossing a heap↔arena boundary.
  The foundation uses it in exactly **one** place: isolating per-request `Variables` on the heap.

### 3.3 The invariant, stated plainly

`MergeValues(dst, src)` **aliases** nested containers from `src` into `dst`.
So if you merge a cache entry directly into the response tree and a later fetch mutates that part of the response tree, you have just mutated the cache entry.

The rule that prevents this:

> Every cache **write** StructuralCopies *into* the cache.
> Every cache **read** StructuralCopies *out* of the cache before merging into the response tree.
> Every **merge-into-existing-L1-entry** uses *working-copy-and-swap*: copy the live entry, merge into the copy, then store the copy (or the fresh value on merge failure) — never mutate the live entry in place, because `MergeValues` is non-atomic on failure and a partial mutation corrupts every sibling key pointing at that entry.

These copy counts are not negotiable padding; they are the minimum for isolation.
They are pinned by a **Copy Budget** table and four adversarial mutation tests (see [06-TEST-AND-BENCH-PLAN.md](./06-TEST-AND-BENCH-PLAN.md)).
Any change to copy counts must update the table, the invariant tests, and the matching benchmarks together.

### 3.4 L1 vs L2 projection: the Passthrough switch

The `Transform` has exactly one switch that distinguishes L1 from L2 copies: `Passthrough`.

- **L1 (`Passthrough = true`)** — rename aliases to schema names, but **keep all source fields**, including `@key` fields not in the provides-shape and fields accumulated by sibling fetches.
  L1 entries grow as more fetches contribute to the same entity within a request.

- **L2 (`Passthrough = false`)** — rename **and project**: keep only the fields the fetch provides, drop everything else.
  L2 entries are minimal and self-contained so they round-trip cleanly across requests.

L2 writes serialize the normalized value to heap bytes (`MarshalTo`) before handing them to the backend — this is the heap boundary that makes external storage safe.

---

## 4. The cache-key model

A cache key is a deterministic, byte-identical JSON string.
Byte-identity matters: two requests that should hit the same entry must produce exactly the same bytes.

Two key shapes exist, produced by two template implementations:

- **Entity key** — `{"__typename":"User","key":{"id":"123"}}`.
  Built from `@key` fields only.
  Numbers in keys are coerced to strings so an integer `id` and a string `id` collide on the same entry.

- **Root-field key** — `{"__typename":"Query","field":"topProducts","args":{...}}`.
  Args are sorted alphabetically for determinism.
  Optionally, `EntityKeyMappings` rewrite a root-field key into *entity* key shape so a root field and an `_entities` fetch share the same cache entry.

**Templates are alias-independent.**
The key is computed from `@key` fields, never from response aliases — so the same entity produces the same key regardless of how a query aliases its fields.

**Field arguments produce a hash suffix, not a wider key.**
A field with arguments gets an xxhash suffix derived from the per-request argument values.
The argument metadata is captured at plan time; the suffix is computed at resolve time because it depends on per-request variables.

**Key transform pipeline (applied identically on read, write, and delete):**

```text
GlobalCacheKeyPrefix  →  subgraph header-hash prefix  →  L2CacheKeyInterceptor
```

Anyone performing manual invalidation must reproduce this exact pipeline, or they target the wrong key.

The key template is an interface (`CacheKeyTemplate`) but it couples to the arena and astjson — it is an **engine-internal** seam, not a router-facing one.
The router configures keys *declaratively* via `EntityKeyMappings`; it never implements the template.
This is an explicit boundary decision recorded in [adr/0001-foundation.md](./adr/0001-foundation.md).

---

## 5. The four-phase parallel resolution touchpoints

The resolver executes a fetch tree of `Sequence` / `Parallel` / `Single` nodes.
The parallel path is where caching has the most touchpoints, and where the design discipline matters most.

**The governing rule: the main thread parses, merges, and runs all cache logic; goroutines do subgraph HTTP only.**
No goroutine ever touches the arena, the L1 map, or a cache backend.

Phases of `resolveParallel` (existing structure; caching hooks called between/within phases):

- **Phase 1 — prepare + L1 check (main thread).**
  Generate L1 and L2 keys for each fetch.
  Check L1; on a complete entity hit, mark the fetch skipped and copy the stored value out via a denormalizing StructuralCopy.

- **Phase 1.5 — `@requestScoped` injection (main thread).**
  Inject coordinate-L1 data when present; skip the fetch if satisfied.
  See [directives/requestScoped.md](./directives/requestScoped.md).

- **Phase 2-L2 — bulk L2 lookup (main thread).**
  Group L2-eligible fetches by cache instance, issue **one** `Get` per instance, parse the returned bytes verbatim onto the Loader arena, distribute values back to each fetch, then decide per-fetch whether the L2 hits cover all items.
  Failure semantics: a single bulk `Get` error fails *all* fetches in that batch back to the subgraph (acceptable because production backends rarely fail partially; the win is removing a goroutine and an arena per fetch).

- **Phase 2-HTTP — parallel HTTP (goroutines).**
  Only fetches not already satisfied by L1, `@requestScoped`, or L2 run here.
  Goroutines return a `[]byte` body; they do not parse or allocate on the arena.

- **Phase 3 / 3.5 — merge analytics + retry `@requestScoped` (main thread).**

- **Phase 4 — merge results + populate caches (main thread).**
  Parse bodies, merge into the response tree, then `populateL1Cache` / `updateL2Cache` / `exportRequestScopedFields`.

The single sequential path (`resolveSingle`) collapses the same steps without the bulk batching.

The crucial design property: **all of this is invoked through a handful of hook points** (§7), so the phase machinery itself does not need rewriting.

---

## 6. The L1 enable decision (post-process)

The planner attaches a key template to every cacheable fetch but **leaves L1 off** (`UseL1Cache = false`).
A dedicated post-process pass (the L1 optimizer) is the *single source of truth* that flips it on.

It runs **last** in the fetch-tree processor chain, after concrete `EntityFetch` / `BatchEntityFetch` types are created, because it dispatches on those concrete types.
For each entity fetch it asks two questions:

- **Can it read?** Does a prior (dependency-ordered) fetch — or the *union* of prior providers of the same entity type — provide all the fields this fetch needs?
- **Can it write?** Is there a later fetch this fetch could populate L1 for?

`UseL1Cache` becomes `read || write`.
A fetch that can neither read nor write L1 skips key generation, lookup, and populate entirely — pure CPU/memory savings.

This pass is **self-contained** (it touches only public resolve fetch types) and is **safe to disable** (a no-op leaves L1 off everywhere).
That property makes it an ideal independent PR.
Note the coupling for re-implementers: if you skip this pass, L1 is effectively off even though templates are present.
Decide the default deliberately and document it (see open question in [adr/0001-foundation.md](./adr/0001-foundation.md)).

---

## 7. THE INTEGRATION SEAM (central section)

This section is the heart of the spec.
It defines the minimal, clean surface that lets us add caching while leaving the existing `loader` and `resolvable` mostly untouched.

### 7.1 Design goal, restated as a constraint

- The existing fetch-tree walker, the four-phase parallel machinery, two-pass rendering, and error/null-bubbling logic **must not be rewritten**.
- Caching enters through **named hook functions** invoked at existing seams, plus **a few state fields** on the Loader and per-fetch result.
- The set of cross-boundary **interfaces** is tiny: one cache backend interface, one key-template interface, one analytics sink. Everything else is plain data.

### 7.2 The control-flow seam: hooks, not rewrites

The Loader's resolution flow gains caching by calling these hook functions at points that already exist.
Each is a method on `*Loader`; none changes the *shape* of the existing flow:

- **`prepareCacheKeys(...)`** — once per fetch, before dispatch: render L1/L2 keys from the fetch's template + input items.
- **`tryL1CacheLoad(...)`** — main-thread L1 read; returns "skip this fetch" on a complete hit.
- **`bulkL2Lookup(...)`** (parallel) / **`tryL2CacheLoad(...)`** (single) — L2 read; returns "skip this fetch" per fetch when hits cover all items.
- **`mergeResult(...)`** — *unchanged in shape*: it already merges fetched data into the response tree; the only addition is that it honors `cacheSkipFetch` (merge the cached value instead of a subgraph body) and records negative-cache hits.
- **`populateL1Cache(...)` / `updateL2Cache(...)`** — after merge: write both caches using the copy primitives of §3.
- **`tryRequestScopedInjection(...)` / `exportRequestScopedFields(...)`** — coordinate-L1 read/write for `@requestScoped`.

The funnel point is `mergeResult`.
A single boolean pair carries cache state into it:

- **`cacheSkipFetch`** — this fetch was fully satisfied by L1 or L2; merge the cached value instead of dispatching.
- **`cacheMustBeUpdated`** — this fetch must (re)write L2 after merge (a miss, a partial hit, a backfill, or a forced refresh).

These two booleans live on the per-fetch `result` and are the *entire* contract between the cache hooks and the existing merge path.
That is what keeps the seam small.

### 7.3 The state seam: additive fields

Caching adds state without restructuring existing types:

- On the **Loader**: the named L2 cache registry (`map[string]LoaderCache`), the L1 map (`map[string]*astjson.Value` on the request arena), a reusable `astjson.Parser`, reusable `Transform` slabs, and the per-subgraph invalidation config map.
- On the per-fetch **result**: `cacheSkipFetch`, `cacheMustBeUpdated`, the rendered L1/L2 keys, and accumulated analytics/trace attachments.
- On the **Context** (per request): `ExecutionOptions.Caching` (the toggles of §9).

No existing field changes meaning.
The `resolvable` two-pass walk gains only *read-only* analytics hooks during the print pass (entity source, field hashing) — it is otherwise untouched.

### 7.4 The cache backend interface (router-facing)

This is the one interface the router actually implements (Redis, in-memory, circuit-breaker decorator).
Contract level — tiny signatures only:

```go
type LoaderCache interface {
    Get(ctx context.Context, keys []string) ([]*CacheEntry, error)   // result aligns 1:1 with keys; nil = miss
    Set(ctx context.Context, entries []*CacheEntry) error            // TTL is per-entry, not a Set argument
    Delete(ctx context.Context, keys []string) error
}

type CacheEntry struct {
    Key          string
    Value        []byte           // opaque JSON payload
    TTL          time.Duration    // write expiration: 0 = backend default, negative = indefinite
    RemainingTTL time.Duration    // set by backend on read (0 = unknown)
    WriteReason  CacheWriteReason // engine-set: refresh | backfill | derived | ""
}
```

Notes that the re-implementation must honor (these correct stale documentation):

- `Set` takes **per-entry TTL**, not a `ttl` argument. This lets one `Set` mix regular and negative-cache TTLs.
- `Get` must return a slice the **same length as `keys`**, with `nil` for misses.
- The backend should be **concurrency-safe** as a forward-compatible contract, even though the engine currently issues bulk reads on the main thread.

A second, minimal config struct lives next to it so invalidation does not pull a `plan` dependency into `resolve`:

```go
type EntityCacheInvalidationConfig struct {
    CacheName                   string
    IncludeSubgraphHeaderPrefix bool
}
```

### 7.5 The cache-key template interface (engine-internal)

The router never implements this; it couples to the arena and astjson.
Contract level:

```go
type CacheKeyTemplate interface {
    RenderCacheKeys(a arena.Arena, ctx *Context, items []*astjson.Value, prefix string) ([]*CacheKey, error)
    IsEntityFetch() bool
    BatchEntityKeyArgumentPath() []string                          // nil if no batch support
    EntityMergePath(pp PostProcessingConfiguration) []string       // nil if it stores full payloads
}
```

Two implementations: `EntityQueryCacheKeyTemplate` (entity shape) and `RootQueryCacheKeyTemplate` (root-field shape, optionally entity-mapped).
`RenderCacheKeys` returns `[]*CacheKey`, where each `CacheKey` carries its input `*astjson.Value`, the rendered key strings, an optional `FromCache` value populated on hit, and batch/merge-path bookkeeping.

[05-ASTJSON-PRIMITIVES.md](./05-ASTJSON-PRIMITIVES.md) specifies the astjson side; the open question of whether this interface should be exported at all is recorded in [adr/0001-foundation.md](./adr/0001-foundation.md).

### 7.6 The analytics sink (observability seam)

Analytics is an opt-in, zero-overhead-when-off collector.
The router reads results through exactly one sanctioned method:

```go
func (c *Context) GetCacheStats() CacheAnalyticsSnapshot   // call once after resolve; releases the pooled collector
```

The snapshot is a plain struct of event slices (`L1Reads`, `L2Reads`, `L1Writes`, `L2Writes`, `FetchTimings`, `ShadowComparisons`, `MutationEvents`, `EntityTypes`, `FieldHashes`, `HeaderImpactEvents`, `CacheOpErrors`) plus convenience derivations (hit rates, bytes served).
When analytics is disabled, the snapshot is empty and the collector imposes no cost.
The re-implementation should treat `GetCacheStats()` as the only contract and keep the per-event `Record*` methods internal unless a concrete external consumer is confirmed (see open questions in [adr/0001-foundation.md](./adr/0001-foundation.md)).

### 7.7 The trace seam (per-fetch diagnostics)

Cache tracing is gated on `Context.TracingOptions.Enable && !ExcludeCacheStats`.
When on, each fetch attaches a `CacheTrace` (L1/L2 enabled, hit/miss counts, durations, per-entity source and byte size, raw keys when not excluded) into the response trace extensions.
This is purely a read-out; it adds no control-flow coupling beyond setting fields on the per-fetch result.

### 7.8 Why this seam keeps loader/resolvable untouched

- The walker dispatch (`single` / `sequence` / `parallel`) is unchanged; cache hooks slot into existing phase boundaries.
- `mergeResult` keeps its signature and purpose; it only learns to honor two booleans.
- `resolvable` gains read-only analytics hooks during rendering; its validation and null-bubbling are untouched.
- All cross-boundary contracts are either tiny interfaces (cache, key template, analytics read-out) or plain additive structs.

---

## 8. Trace and analytics: relationship to the seam

Trace and analytics are siblings, not the same thing:

- **Analytics** is a per-request aggregate read once via `GetCacheStats()`, for metrics export.
- **Trace** is a per-fetch diagnostic embedded in the response, for the playground / studio.

Both are **off by default** and gated independently.
Both write only to fields on per-fetch results and a pooled collector — neither participates in control flow.
This separation is why they can ship as their own stacked PRs after the core, with no risk to the resolution path.

---

## 9. Per-request toggles

All run-time behavior is controlled per request on `Context.ExecutionOptions.Caching`:

```go
type CachingOptions struct {
    EnableL1Cache         bool                  // per-request L1; also gates @requestScoped coordinate L1
    EnableL2Cache         bool                  // external L2
    EnableCacheAnalytics  bool                  // detailed events; off = empty snapshot, zero cost
    L2CacheKeyInterceptor L2CacheKeyInterceptor // custom key transform (L2 only)
    GlobalCacheKeyPrefix  string                // prepended to all L2 keys (schema versioning)
}
```

The router maps dev/debug headers (`X-WG-Disable-Entity-Cache[-L1|-L2]`) onto these flags, gated on trace/dev mode to prevent production abuse.
Disabling L1 via these flags also disables `@requestScoped` coordinate L1 because it shares `EnableL1Cache`.

---

## 10. What stays untouched

The re-implementation **must not** rewrite any of the following.
If a change is needed here, it is a red flag that the seam is wrong.

- The fetch-tree walker and its `single` / `sequence` / `parallel` dispatch.
- The four-phase parallel execution structure and its main-thread/goroutine split.
- `mergeResult`'s signature and core responsibility (parse + merge into the response tree) — it gains only "honor `cacheSkipFetch` / `cacheMustBeUpdated`".
- The `resolvable` two-pass validate-then-render walk, including null bubbling and field authorization (it gains only read-only analytics hooks during the print pass).
- The arena pooling and early-release pattern in `ResolveGraphQLResponse`.
- The `DataSource` and `LoaderHooks` interfaces.
- `parse`, `normalize`, `validate`, and JSON response rendering.

## 11. What the foundation introduces

The foundation is the minimal, additive surface that everything else stacks on.

**Interfaces (the seam):**

- `LoaderCache` + `CacheEntry` + `EntityCacheInvalidationConfig` — the router-facing L2 backend contract (§7.4).
- `CacheKeyTemplate` + `CacheKey` — the engine-internal key seam (§7.5).
- The analytics read-out: `Context.GetCacheStats()` returning `CacheAnalyticsSnapshot` (§7.6).

**Additive data shapes:**

- `FetchCacheConfiguration` on each fetch (L2 flags, TTL, cache name, key template, provides-shape references, request-scoped fields, `UseL1Cache`).
- `FetchInfo.ProvidesData` — the per-fetch field-shape `*Object` that drives normalization, widening, and L1 optimization.
- `KeyField`, `CacheFieldArg`, `ObjectCacheAnalytics`, `MutationEntityImpactConfig` — analytics/key support shapes.
- `CachingOptions` on `Context.ExecutionOptions` (§9) and the caching fields on `ResolverOptions` (`Caches`, `EntityCacheConfigs`, subscription callbacks).

**Run-time mechanism:**

- The L1 map + L2 bulk-lookup path on the Loader, the two-boolean merge contract, and the StructuralCopy-based isolation discipline of §3.
- The L1-enable post-process pass of §6.

**Hard external prerequisite:**

- The astjson copy/transform/two-pass primitives the foundation leans on (`StructuralCopy`, `StructuralCopyWithTransform`, `Transform` with `Passthrough`, package-level `DeepCopy`, the two-return `MergeValues`) are **unreleased** at the time of writing.
  Cutting an astjson release that contains them is **PR #0** of the whole stack.
  See [05-ASTJSON-PRIMITIVES.md](./05-ASTJSON-PRIMITIVES.md) and [adr/0001-foundation.md](./adr/0001-foundation.md).

---

## 12. Cross-references

- Directive table: [02-DIRECTIVE-INVENTORY.md](./02-DIRECTIVE-INVENTORY.md); per-directive specs: [directives/](./directives/).
- Stacked PR plans: gqtools [03-PR-PLAN-graphql-go-tools.md](./03-PR-PLAN-graphql-go-tools.md), router [04-PR-PLAN-router.md](./04-PR-PLAN-router.md).
- astjson dependency: [05-ASTJSON-PRIMITIVES.md](./05-ASTJSON-PRIMITIVES.md).
- Test and benchmark plan (incl. Copy Budget): [06-TEST-AND-BENCH-PLAN.md](./06-TEST-AND-BENCH-PLAN.md).
- Out-of-scope findings (onError, service_datasource, planner correctness, harness rewrite, stale-base artifacts): [07-UNRELATED-FINDINGS.md](./07-UNRELATED-FINDINGS.md).
- Implementation loop: [08-EXECUTION-RUNBOOK.md](./08-EXECUTION-RUNBOOK.md).
- Foundation decision record: [adr/0001-foundation.md](./adr/0001-foundation.md).
