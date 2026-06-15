# Directive Specification: `@provides`

> Part of the entity-caching re-implementation document set.
> See [02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md) for the full directive table,
> [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) for the integration seam and the L1/L2 model,
> and [adr/0004-provides.md](../adr/0004-provides.md) for the decision record behind this contract.
>
> Re-implementation PR: **gqtools PR 5 / PR-CACHE-PROJECTION** (see [03-PR-PLAN-graphql-go-tools.md](../03-PR-PLAN-graphql-go-tools.md)).
>
> Reader assumption: you have never seen this feature.
> This document explains what `@provides` means, what the caching layer reads from it, and how to verify the behavior.

---

## 1. Purpose & responsibility

`@provides` is a standard GraphQL Federation directive on a field definition.
It declares that, when a subgraph returns a particular field, it can *also* return some extra fields of the referenced entity inline, without a separate entity fetch.
The classic example is `Review.author: User! @provides(fields: "username")`: the reviews subgraph normally only knows a `User`'s `id`, but on the `author` edge it promises to also return `username` inline.
The entity-caching layer never *defines* or *enforces* federation resolution from `@provides`; that is the planner's job.
What the caching layer does is **consume the shape that `@provides` (together with the query selection at a fetch) produces** — the per-fetch `ProvidesData *Object` — and use it for three things: to **project** the value stored in L2 down to exactly the fields a fetch owns, to run the **field-widening check** that stops a narrow cached value from satisfying a wider query, and to **re-apply aliases** (denormalize) when a cached value is read back into the active response tree.
In one sentence: `@provides` is the source of the *cache shape*, and `ProvidesData` is the data structure that carries that shape from planner to resolver.

---

## 2. SDL / configuration definition

### 2.1 The wire directive (federation SDL)

```graphql
directive @provides(fields: FieldSet!) on FIELD_DEFINITION
```

It appears on the field that returns the entity, naming a `_FieldSet` over the *returned* type:

```graphql
type Review {
    body: String!
    author: User! @provides(fields: "username")   # reviews can return User.username inline here
}
```

`@provides` is *not* a caching configuration struct.
The caching layer never reads the directive text directly — it reads the data shape the planner derives from the directive plus the operation's selection set.

### 2.2 The data shape the caching layer actually consumes: `ProvidesData *Object`

Every fetch carries a `FetchInfo` whose `ProvidesData` field is the alias-aware description of the exact shape the fetch yields at its location in the response.
It is a `*resolve.Object` — the same node type used by the response plan tree — so it is naturally recursive (objects within objects, arrays of objects).
Tiny signature, set by the planner visitor:

```go
// v2/pkg/engine/plan/visitor.go (configureFetchObject)
singleFetch.Info.ProvidesData = providesData   // = v.caching.plannerObjects[fetchID]
resolve.ComputeHasAliases(providesData)         // sets Object.HasAliases recursively
```

Two flags on the `*Object` are load-bearing for caching:

- `HasAliases` — true when any field in the (sub)tree is aliased (output name differs from schema name).
  Computed once at plan time via `ComputeHasAliases`.
  When false, the resolver takes a fast path (plain `StructuralCopy`, no Transform).
- `Fields []*Field` — each field carries `Name` (response key / alias), `OriginalName` (schema name, nil when not aliased), `Value Node` (nested shape), and `CacheArgs` (field arguments that contribute an xxhash suffix to the cache field name).

`ProvidesData` is `nil` when the planner runs with `DisableFetchProvidesData = true` (test/programmatic construction).
Production planners always populate it.
The resolver treats a `nil` `ProvidesData` as "no projection, no widening check" — see §4.5.

**Critical placement rule (carried over verbatim):** for a nested *entity* fetch, `ProvidesData` must contain the **entity** fields (`id`, `username`), NOT the parent edge field (`author`).
The planner rewrites `FieldName` / `FieldPath` so the object describes the entity at the fetch boundary, not the field that pointed at it.

---

## 3. Composition rules & validation

`@provides` is validated by federation composition, *upstream* of everything in this repository.
The caching layer adds **no** new composition rules for it.
The rules the re-implementation must assume are already enforced:

- `@provides` is allowed only on `FIELD_DEFINITION`.
- `fields` is a **mandatory** `_FieldSet` parsed against the *returned* type of the field (here, `User`), not the enclosing type (`Review`).
- The named fields must exist on the returned entity type and the subgraph must actually be able to resolve them inline; otherwise composition fails.
- `@provides` fields are typically `@external` on the returned type from the providing subgraph's perspective (the field is *owned* elsewhere, *provided* here) — for example `User.username` is `@external` in the reviews subgraph but `@provides`d on `Review.author`.

The caching layer's only contract with composition is read-only: it consumes the *planned* selection shape, and trusts that the planner already honored `@provides` when it decided whether a fetch is needed at all.

---

## 4. Runtime semantics

This section walks where `ProvidesData` is read in the planner and across the four-phase resolve flow described in [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) §5.
The governing thread rule still holds: **all of this runs on the main thread**; goroutines do subgraph HTTP only and never touch `ProvidesData`, the arena, or any cache.

### 4.1 Plan time — building `ProvidesData`

The planner builds a `*Object` for each fetch (`plannerObjects[fetchID]`) describing the selection shape at that fetch's location, honoring `@provides` (so a `@provides`d field becomes part of the parent fetch's shape rather than forcing a child entity fetch).
At `configureFetchObject` it assigns this object to `FetchInfo.ProvidesData` and calls `ComputeHasAliases`.
For a nested entity fetch the object is rewritten so it describes the *entity* shape (`{id, username}`), not the edge (`author`).
No cache is touched at plan time.

### 4.2 Phase 1 / 2 — L1 read and bulk L2 read (main thread)

When a cached value is a *candidate* for satisfying a fetch, the resolver validates it against `ProvidesData` before serving it.

- **L1 read** (`tryL1CacheLoad`): a stored entity value is read back with `structuralCopyDenormalizedPassthrough(stored, ProvidesData)` — restores aliases for the current query *and keeps all accumulated fields*.
  The hit is only honored if the value satisfies the widening check (§4.4).
- **L2 read** (`bulkL2Lookup` → `applyEntityFetchL2Results` / `applyRootFetchL2Results`): a parsed L2 entry is validated against `ProvidesData` via `resolveMultiCandidateCacheValue` → `validateItemHasRequiredData`.
  If it covers all required fields, the value is denormalized with `structuralCopyDenormalized(FromCache, ProvidesData)` (projected read) and the fetch is marked `cacheSkipFetch`.

### 4.3 Phase 4 — cache writes (main thread)

After a fetch returns and is merged into the response tree, both caches are populated, and `ProvidesData` drives the shape of what is stored:

- **L1 write** (`populateL1Cache`): `structuralCopyNormalizedPassthrough(value, ProvidesData)`.
  Renames aliases → schema names but **keeps all source fields** (`Transform.Passthrough = true`), including `@key` fields not listed in `ProvidesData` and fields contributed by sibling fetches.
  L1 entries grow over a request — passthrough is what lets them accumulate.
- **L2 write** (`updateL2Cache`): non-passthrough projection.
  Renames *and* drops everything not in `ProvidesData`, then serializes with `MarshalToWithTransform`.
  L2 entries are minimal and self-contained so they round-trip cleanly across requests.

### 4.4 The field-widening check

`validateItemHasRequiredData(item, ProvidesData)` walks every field in the `*Object` and requires the cached value to contain each one (by `cacheFieldName`, i.e. schema name + arg suffix), recursing into nested objects and arrays.
A field that is **missing** fails the check even if it is nullable — present-but-null is acceptable, absent is not.
This is the guard that stops a narrow root query (`{ user { id name } }`) from poisoning the cache for a wider entity fetch (`{ user { id name email } }`): the narrow cached value lacks `email`, fails the check, and the wider fetch proceeds to the subgraph instead of serving stale-shape data.

### 4.5 Alias / normalization handling

`ProvidesData` is alias-aware end to end:

- **Normalize** (write side, `buildNormalizeTransform`): `InputKey = alias`, `OutputKey = cacheFieldName` (schema name + optional arg-hash suffix).
  Cache storage always uses schema field names, so the same entity produces the same stored shape regardless of how a query aliased it.
- **Denormalize** (read side, `buildDenormalizeTransform`): the inverse — `InputKey = cacheFieldName`, `OutputKey = alias` — re-applies the *current* query's aliases when serving a cached value into the response tree.
- **`__typename`** is force-added as an identity entry when the selection set omits it, so polymorphic type identity survives projection.
- **Fast path**: when `ProvidesData.HasAliases == false`, all four `structuralCopy*` helpers skip the Transform and fall back to plain `StructuralCopy` (containers cloned, leaves aliased) — no rename, full passthrough.

### 4.6 Ordering / threading constraints

- Transforms derived from `ProvidesData` are **ephemeral**: built inline on the Loader's reusable `transformEntries` / `transforms` / `transformMetas` slabs and consumed by `StructuralCopyWithTransform` / `MarshalToWithTransform` in the same call.
  They depend on per-request state (`Variables`, `RemapVariables` feeding `CacheArgs` arg-hash suffixes) and must **never** be cached on the `*Object`, the plan tree, the `Resolver`, or anything that outlives a request.
- `ProvidesData` itself is a shared planner `*Object` and is read-only at resolve time.
  All reads happen on the main thread; goroutines never see it.

---

## 5. Cache key & data shape

`@provides` / `ProvidesData` affects the **stored shape**, not the **cache key**.

- **Cache key**: keys are derived from `@key` fields only (see [key.md](key.md)), never from `@provides` fields or arbitrary selected fields.
  A `User` keyed by `id` produces `{"__typename":"User","key":{"id":"123"}}` whether or not `username` was provided.
  This keeps entity identity stable across queries that select different field subsets.
- **Stored shape — the passthrough vs projection switch** (the single `Transform.Passthrough` flag from [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) §3.4):

  | Tier | Helper | `Passthrough` | Result |
  |------|--------|---------------|--------|
  | **L1 write** | `structuralCopyNormalizedPassthrough` | `true` | rename aliases → schema names, **keep** all fields incl. `@key` and sibling-contributed fields; entry accumulates over the request |
  | **L1 read** | `structuralCopyDenormalizedPassthrough` | `true` | restore aliases, **keep** all accumulated fields |
  | **L2 write** | `structuralCopyNormalized` (then `MarshalToWithTransform`) | `false` | rename **and project** to exactly `ProvidesData` fields, drop the rest; minimal, self-contained |
  | **L2 read** | `structuralCopyDenormalized` | `false` | restore aliases, projected to `ProvidesData` |

- **Interaction with `@requires`** (exclusion): `@requires` fields are request-derived inputs, not entity-owned, and must **never** appear in `ProvidesData` and therefore never in the projected/stored shape.
  See [requires.md](requires.md) — `@requires` is precisely "the fields excluded from the shape that `@provides` defines".

---

## 6. Interaction with the foundation seam and other directives

- **Foundation** ([01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) §3, [adr/0001-foundation.md](../adr/0001-foundation.md)): every projection and denormalize uses the `StructuralCopy` / `StructuralCopyWithTransform` primitives and the `Transform { Entries, Forced, Passthrough }` shape.
  `ProvidesData` is the input that builds those Transforms.
  The copy budget (Architecture §3.3, resolve `CLAUDE.md` "Copy Budget") counts exactly one copy per cache write and one per cache read driven by `ProvidesData`; the re-implementation must not add or drop copies here.
- **`@key`** ([key.md](key.md)): supplies the cache key; L1 passthrough deliberately *retains* `@key` fields even when they are not in `ProvidesData`, because a later wider entity fetch needs the key present to merge.
  `@provides` and `@key` are complementary: `@key` is identity, `@provides` is shape.
  `@provides` therefore **depends on** `@key` being correct first (see the dependency chain in [02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md) §3).
- **`@requires`** ([requires.md](requires.md)): the exclusion rule above; specified *after* `@provides` because it is defined as "removed from the `ProvidesData` shape".
- **`@requestScoped`** ([request-scoped.md](request-scoped.md)): reuses `ProvidesData` for its widening check (`validateItemHasRequiredData`) and the same normalize/denormalize copy pipeline.
  `populateRequestScopedFieldsProvidesData` locates each request-scoped field's sub-`Object` in the planner tree and attaches it as that field's `ProvidesData`.
  So `@requestScoped` builds directly on this directive's machinery.
- **Mutation / subscription / config concepts**: all rely on `ProvidesData` to project what they write to L2 (`structuralCopyProjected` for shadow comparison and mutation analytics navigates `ProvidesData` to the entity sub-object via `navigateProvidesDataToField`).

---

## 7. End-to-end test plan

These cases run against the federation test services in `execution/federationtesting/` (`accounts`, `products`, `reviews`).
The schemas already contain a real `@provides`: in **reviews**, `Review.author: User! @provides(fields: "username")`, with a sibling `Review.authorWithoutProvides: User!` that forces an entity fetch to **accounts** for `username`.
`User @key(fields: "id")` lives in both subgraphs; `username` is `@external` in reviews and owned by accounts.

> Test-style requirements (from the repo and `execution/engine/CLAUDE.md`), mandatory for every case below:
> - **`assert.Equal` on full values only** — never `Contains`, `GreaterOrEqual`, `Greater`, or any fuzzy/substring comparison.
> - **Inline literals** — GraphQL queries, expected JSON responses, and cache keys appear inline at the assertion/setup site, never in shared `const`/`var` blocks or shared helpers.
> - **Self-contained subtests** — each `t.Run` sets up its own cache instances, gateway options, context, and URLs inline; duplication across subtests is preferred over shared helpers.
> - **Cache-log discipline** — every `cache.ClearLog()` is followed by `cache.GetLog()` + full assertions before the next clear or end of test.
> - **Vertical multi-key literals** — cache-log `Keys`/`Hits` and snapshot event lists wrap one item per line, each with a trailing comment explaining *why* that event occurred.

### Case 1 — `@provides` satisfies the query inline (no entity fetch, nothing entity-cached for User)

- **Query** (inline): `query { topReviews { body author { username } } }`
- **Expectation**: because reviews `@provides`d `username` on `author`, the planner does **not** emit a `User` entity fetch to accounts.
- **What is cached**: the reviews root-field fetch may be L2-cached (its `ProvidesData` includes `author { username }` inline), but there is **no** `User` entity L2 entry — accounts is never called.
- **Assertions**:
  - `assert.Equal` on the full response JSON, e.g. ``assert.Equal(t, `{"data":{"topReviews":[{"body":"A highly effective form of birth control.","author":{"username":"Me"}}]}}`, string(out))`` (exact value confirmed against fixtures before committing).
  - `assert.Equal(t, 0, accountsCalls)` — accounts never queried because `@provides` covered `username`.
  - Cache log: assert the reviews-fetch `get`/`set` keys exactly, one key per line, each with a why-comment (`// reviews root field, first request → miss then set`).

### Case 2 — `authorWithoutProvides` forces an entity fetch and a `User` cache entry

- **Query** (inline): `query { topReviews { body authorWithoutProvides { username } } }`
- **Expectation**: no `@provides`, so the gateway resolves `User.username` via an entity fetch to accounts.
- **What is cached**: a `User` entity entry projected to `ProvidesData = {id, username}`.
- **Assertions**:
  - Full-response `assert.Equal` on the JSON.
  - `assert.Equal(t, 1, accountsCalls)` on the first request — entity fetch happened.
  - Cache log (vertical, one key per line):
    ```go
    wantSet := []resolve.CacheLogEntry{
        {
            Operation: "set",
            Keys: []string{
                `{"__typename":"User","key":{"id":"1234"}}`, // entity User fetched from accounts, projected to {id,username}
            },
        },
    }
    ```
    (substitute the real key bytes once verified).
  - Second identical request: `assert.Equal(t, 0, accountsCalls)` (served from L2) and a `get` log entry with `Hits: []bool{true}`.

### Case 3 — projection: L2 stores only `ProvidesData` fields, not the whole entity

- **Setup**: query selects `{ user(id:"1") { id username } }` against accounts, where `User` also has `nickname`, `realName`, etc.
- **What is cached**: the L2 entry must contain **only** `{id, username}` (plus forced `__typename`), proving non-passthrough projection.
- **Assertions**:
  - Read the stored bytes from the cache log `set` and `assert.Equal` on the **exact** stored JSON, e.g. ``assert.Equal(t, `{"__typename":"User","id":"1234","username":"Me"}`, storedValue)``.
  - Negative assertion is implicit in the full-equal: if `nickname`/`realName` leaked in, the exact-equal fails.

### Case 4 — widening guard: a narrow cached entry does NOT satisfy a wider query

- **Setup**: Request A caches `User{id:"1"}` with shape `{id, username}`.
  Request B asks `{ user(id:"1") { id username realName } }`.
- **Expectation**: the cached value lacks `realName`, `validateItemHasRequiredData` fails, Request B re-fetches from the subgraph rather than serving a value missing `realName`.
- **What is cached**: after Request B, the entry is rewritten with the wider shape `{id, username, realName}`.
- **Assertions**:
  - Request B: `assert.Equal(t, 1, accountsCalls)` — widening miss forced a fetch.
  - Cache log for Request B: `get` with `Hits: []bool{false}` (widening miss, NOT a key miss), then `set` rewriting the wider shape; assert both, one key per line with why-comments.
  - Full-response `assert.Equal` includes `realName`.

### Case 5 — alias-aware projection round-trip

- **Query** (inline): `query { user(id:"1") { uid: id name: username } }`
- **Expectation**: stored shape is normalized to schema names (`{id, username}`); the served response re-applies the query aliases (`uid`, `name`).
- **Assertions**:
  - `assert.Equal` on the stored bytes uses **schema names**: ``assert.Equal(t, `{"__typename":"User","id":"1234","username":"Me"}`, storedValue)``.
  - `assert.Equal` on the response uses **aliases**: ``assert.Equal(t, `{"data":{"user":{"uid":"1234","name":"Me"}}}`, string(out))``.
  - A second request with *different* aliases (`{ user(id:"1") { x: id y: username } }`) is an L2 **hit** on the same key (key is alias-independent): cache-log `get` `Hits: []bool{true}`, `assert.Equal(t, 0, accountsCalls)`.

### Case 6 — `nil ProvidesData` short-circuits projection (defense-in-depth)

- **Setup**: a resolve-package unit test constructs a fetch with `DisableFetchProvidesData` / `ProvidesData == nil`.
- **Expectation**: cache write/read fall back to plain `StructuralCopy`; root-field L1 promotion is silently skipped (never stores aliased response-shape values).
- **Assertions**:
  - `assert.Equal` that the promoted/stored value is absent (no L1 entry created) — assert the L1 map state explicitly, not via a fuzzy check.

---

## 8. Acceptance criteria

A reviewer can check each item against the implementation and the tests:

- [ ] `@provides` is consumed read-only; the caching layer adds no composition rules for it.
- [ ] Each fetch's `FetchInfo.ProvidesData` is a `*resolve.Object` describing the **entity** shape at the fetch boundary (entity fields, not the parent edge field), with `HasAliases` computed at plan time.
- [ ] `ProvidesData == nil` is handled everywhere as "no projection, no widening, skip root-field L1 promotion" without panics.
- [ ] **Cache key is `@key`-only** — `@provides` fields never widen or alter the key; verified by Case 5's alias-independence and Case 3's projection.
- [ ] **L2 write projects** (non-passthrough): stored bytes contain exactly `ProvidesData` fields + forced `__typename`, nothing else (Case 3, exact-equal on stored bytes).
- [ ] **L1 write passes through** (`Passthrough = true`): `@key` fields and sibling-contributed fields are retained; entries accumulate over a request.
- [ ] **Widening check** (`validateItemHasRequiredData`) rejects a value missing any `ProvidesData` field (absent fails even if nullable; present-null passes) — Case 4.
- [ ] **Alias round-trip**: stored shape uses schema names; served value re-applies the current query's aliases; same entity under different aliases hits the same key — Case 5.
- [ ] **`@requires` exclusion**: `@requires` fields never appear in `ProvidesData` or the stored shape (cross-checked in [requires.md](requires.md)).
- [ ] Transforms built from `ProvidesData` are ephemeral (built on the reusable slabs, consumed in-call) and never cached beyond a request.
- [ ] All cache reads/writes driven by `ProvidesData` run on the main thread; goroutines never touch it.
- [ ] Copy budget unchanged: one `StructuralCopy`(+Transform) per cache write and one per cache read attributable to `ProvidesData`; adversarial mutation tests still pass.
- [ ] Every test uses `assert.Equal` on full values, inline literals, vertical multi-key cache-log literals with why-comments, and clear-then-assert log discipline.

---

## Cross-links

- Decision record: [adr/0004-provides.md](../adr/0004-provides.md)
- Architecture and integration seam: [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md)
- Directive inventory and dependency ordering: [02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md)
- Sibling directive specs: [key.md](key.md), [requires.md](requires.md), [request-scoped.md](request-scoped.md)
