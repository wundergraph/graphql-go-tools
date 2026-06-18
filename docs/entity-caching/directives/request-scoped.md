# Directive Specification: `@requestScoped`

> Part of the entity-caching re-implementation document set.
> Cross-references:
> [adr/0005-request-scoped.md](../adr/0005-request-scoped.md) (decision record),
> [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md) (foundation seam, L1/L2 model, StructuralCopy invariants),
> [02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md) (directive taxonomy + PR mapping).
>
> Re-implementation PR: **PR-REQUEST-SCOPED** (gqtools PR 17-20).
> Assume the reader has no prior knowledge of the feature.

---

## 1. Purpose & responsibility

`@requestScoped` is the **only new wire directive** this feature introduces.
It marks a field whose resolved value is identical for the entire request *within one subgraph*,
for example a `currentViewer` derived from the request's auth context.
When two or more fields in the same subgraph carry `@requestScoped(key: "X")`,
the engine maintains a tiny per-request **coordinate L1** cache so that whichever annotated field resolves first populates the value,
and every later field with the same key injects that value and **skips its own subgraph fetch**.
The model is purely symmetric: there is no provider/receiver distinction,
every annotated field is simultaneously a reader (it may inject from the coordinate L1) and a writer (it always exports to the coordinate L1 after resolving).
The coordinate L1 is a separate mechanism from entity L1/L2,
but it rides on the same `EnableL1Cache` per-request flag and the same StructuralCopy isolation primitives,
so it is treated as part of the L1 layer, not a third cache tier.

---

## 2. SDL / configuration definition

### Wire directive (subgraph SDL, composition-side)

```graphql
directive @requestScoped(key: String!) on FIELD_DEFINITION
```

- Applies only to `FIELD_DEFINITION`.
- `key` is a mandatory non-null `String`.
- Repeated usage across fields with the same `key` in the same subgraph is the intended pattern (that is how two fields coordinate).

### Planner-side metadata (one row per annotated field)

Composition output is surfaced to the planner as a flat list on the subgraph's federation metadata.
The struct carries no shape, only identity:

```go
type RequestScopedField struct {
    FieldName string // e.g. "currentViewer"
    TypeName  string // enclosing type, e.g. "Query" or "Personalized"
    L1Key     string // "{subgraphName}.{key}", e.g. "accounts.viewer"
}
```

The `L1Key` is constructed once at composition/config time as `"{subgraphName}.{key}"`,
so it is alias-independent and subgraph-scoped by construction.
Lookups:
- `RequestScopedFieldsForType(typeName)` returns all annotated fields on a type (used by entity fetches).
- `RequestScopedExportsForField(typeName, fieldName)` returns the `L1Key`(s) for a single coordinate (used by root fetches and by the widening visitor).

### Resolver-side carrier (per fetch)

The planner emits one of these per participating field onto the fetch's cache configuration.
This is the entire on-the-wire surface between planner and resolver:

```go
type RequestScopedField struct {
    FieldName    string   // response key at the fetch location (alias if present, else schema name)
    FieldPath    []string // path in the response data, response-keyed, e.g. ["currentViewer"]
    L1Key        string   // coordinate L1 key "{subgraphName}.{key}"
    ProvidesData *Object  // alias-aware value shape this fetch expects at FieldPath; nil ⇒ field not selected here
}
```

`RequestScopedFields []RequestScopedField` lives on `FetchCacheConfiguration` alongside the entity/root cache fields.
It is preserved through every caching-enabled planner path (plain fetch, entity-cache-enabled fetch, L2-enabled root fetch).

---

## 3. Composition rules & validation

Composition is the source of truth for the directive.
It enforces two rules:

1. **`key` is mandatory.**
   The directive definition declares `key: String!`,
   so a `@requestScoped` with no `key` (or a null `key`) is a composition error.

2. **A lone reader is a warning, not an error.**
   When a `key` value appears on exactly **one** field within a subgraph,
   composition emits a *warning*.
   A single annotated field can never coordinate with a second field,
   so the directive is meaningless in that subgraph (it does no harm, but it does nothing).
   This is a warning rather than a hard error because the schema is still valid and composable.

There is no schema-level constraint that the two fields share a return type;
type compatibility is checked at plan time (see §4, widening), not at composition time.

---

## 4. Runtime semantics

The directive acts at two stages: **plan** (annotate) and **resolve** (act).

### 4.1 Plan stage

1. **Datasource `ConfigureFetch` emits one `RequestScopedField` per annotated field** (symmetric — no reader/writer split):
   - For **root-field fetches**, it iterates the query's root fields,
     looks up `RequestScopedExportsForField(typeName, fieldName)`,
     and emits a carrier whose `FieldName`/`FieldPath` use the field's response key (alias if present).
   - For **entity fetches**, it iterates `RequestScopedFieldsForType(entityType)`,
     plus — via `InterfaceObjects` config — the interface types the concrete entity implements
     (so a directive declared on interface `Personalized` is found for concrete entity `Article`).
     Entries are de-duplicated by `(FieldName, L1Key)`.
     The response path is rewritten to the outer query's alias when one was recorded (`requestScopedResponseKeys`).
   - At this stage `ProvidesData` is still `nil`.

2. **The planner populates `ProvidesData`** in `configureFetchCaching` via `populateRequestScopedFieldsProvidesData`:
   - For each carrier, it locates the matching sub-`Object` in the planner's response tree (`plannerObjects[fetchID]`) by response key.
   - It sets `ProvidesData` to that alias-aware `*Object` and runs `ComputeHasAliases` on it.
   - **Carriers whose field is not present in this fetch's selection set are dropped** (their `ProvidesData` would be nil).
     This is the critical fail-closed filter: a fetch must never be skipped on the strength of a hint describing a field it did not actually select.

3. **Selection widening (`propagateRequestScopedWidening`)** is a separate node-selection visitor pass.
   When several fields share the same `(L1Key, subgraph)` group,
   it computes the *union* of their selection sets (including hidden `@requires` dependencies) and widens each participant's fetch to that union,
   so the first fetch produces a value rich enough to satisfy every later reader.
   Field/argument conflicts within the union are resolved by assigning deterministic **synthetic aliases** so the upstream subgraph query stays valid,
   and the response-side keys are remapped back for the client.
   Widening only proceeds when all participants in a group share the same return type; otherwise the group is skipped.

### 4.2 Resolve stage — where it acts in the 4-phase flow

The coordinate L1 is a plain `map[string]*astjson.Value` (`requestScopedL1`) on the Loader,
allocated per request, **main-thread only**, never touched by a goroutine.
Two operations run against it: `tryRequestScopedInjection` (read) and `exportRequestScopedFields` (write).
Both are gated on `ctx.ExecutionOptions.Caching.EnableL1Cache`.

Parallel path (`resolveParallel`):

- **Phase 1 — entity L1 check (main thread).** Unchanged.
- **Phase 1.5 — `@requestScoped` injection (main thread).**
  For each fetch not already skipped by Phase 1, call `tryRequestScopedInjection`.
  On success: set `res.fetchSkipped = true`, set `ensureFetchTrace(f).LoadSkipped = true`,
  and set `res.cacheTraceRequestScopedHits = res.cacheTraceEntityCount`.
  A skipped fetch is excluded from the bulk L2 lookup and the HTTP goroutines.
- **Phase 2-L2 — bulk L2 lookup (main thread).** Excludes fetches with `fetchSkipped`.
- **Phase 2-HTTP — parallel HTTP (goroutines).** Excludes fetches with `fetchSkipped`. HTTP only.
- **Phase 3 — merge analytics (main thread).**
- **Phase 3.5 — retry injection (main thread).**
  Re-run `tryRequestScopedInjection` for fetches whose hint became satisfiable only after a sibling fetch produced the value.
  Same success bookkeeping as Phase 1.5.
- **Phase 4 — merge results + populate caches (main thread).**
  After `mergeResult`, call `exportRequestScopedFields` for each fetch to populate the coordinate L1 from the freshly resolved data.

Single path (`resolveSingle`): the same `tryRequestScopedInjection` (before fetch) and `exportRequestScopedFields` (after merge) calls apply, with `LoadSkipped`/hit-counter bookkeeping at all single-fetch variants.

### 4.3 Injection contract (`tryRequestScopedInjection`)

Collect-then-inject; never partial-inject:

1. Return `false` immediately if the fetch has no `RequestScopedFields`, or if `EnableL1Cache` is off.
2. For every hint, build a pending injection. A hint fails the whole call (returns `false`, items untouched) if:
   - the `L1Key` is absent from `requestScopedL1`, or the stored value is nil;
   - `hint.ProvidesData == nil` (fail-closed — this fetch did not select the field);
   - the **field-widening check fails**: `validateItemHasRequiredData(cachedValue, hint.ProvidesData)` reports the cached value is missing any field the query needs.
3. Each pending value is materialized via `structuralCopyDenormalized(cachedValue, hint.ProvidesData)` — a StructuralCopy onto `l.jsonArena` that re-applies the query's aliases. This produces a value independent of the cached entry, so the response tree may mutate it freely.
4. Only if **all** hints succeed, inject. For a single item, `Set` the materialized value directly; for multiple items, give each item its own additional `StructuralCopy` so items never alias each other.
5. Set `res.fetchSkipped = true` and return `true`.

The widening check is the load-bearing safety property:
a narrow root query (`{id, name}`) must not poison the coordinate L1 such that a wider entity fetch (`{id, name, email}`) wrongly skips.
Widening (§4.1.3) is what *prevents* that by widening the first fetch up front;
the runtime check is the backstop that refuses to skip if the cached value is nonetheless too narrow.

### 4.4 Export contract (`exportRequestScopedFields`)

1. Return immediately if the fetch has no `RequestScopedFields`, or if `EnableL1Cache` is off.
2. Source list is the fetch's `items`; for root fetches with empty items, fall back to `l.resolvable.data`.
3. For each annotated field, read `item.Get(field.FieldPath...)`. Skip nil or JSON-null values (only the first non-null entity is sampled, since the value is request-identical).
4. Normalize the value for storage via `structuralCopyNormalized(value, field.ProvidesData)` — StructuralCopy onto `l.jsonArena` that renames aliases to schema names and appends arg-hash suffixes for arg-variant sub-fields. This is the **copy-on-export** that keeps the stored value independent of the response tree.
5. Store under `field.L1Key`:
   - if no entry exists, store the normalized value directly;
   - if an entry exists, use **working-copy-and-swap**: `StructuralCopy` the existing entry into a working copy, `MergeValues(working, normalized)`, store the working copy on success, and on merge failure keep the existing entry intact (drop the working copy). Never mutate a live entry in place.

---

## 5. Cache key & data shape

- **Coordinate key.** The key is `"{subgraphName}.{key}"`, fixed at composition time. It is *not* derived from `@key` fields, response data, or aliases — it is a pure coordinate. There is no per-request hashing, no header prefix, and no `EntityKeyMappings`; this is intentionally simpler than entity/root cache keys (see §4 of [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md)).

- **Stored shape (normalized, projected).** The export path uses `structuralCopyNormalized` (the non-passthrough projection): the stored value is in **schema field names** with arg-hash suffixes for arg-variant fields, projected to the `ProvidesData` shape. `__typename` is preserved. Nullable nested objects that are present-but-null survive normalization so later validation can satisfy a selection that includes them.

- **Read shape (denormalized, alias-restored).** The injection path uses `structuralCopyDenormalized` to re-apply the *current* query's aliases when writing the value onto response items. The same schema-named L1 entry can therefore serve two queries that alias the field differently.

- **Passthrough vs projection.** Unlike entity L1 (which uses the *passthrough* variant to accumulate all fields across fetches), the coordinate L1 uses **projection** on both sides — it stores exactly the `ProvidesData` shape. The accumulation that does happen is via the working-copy-and-swap `MergeValues` in export, which lets a later, wider participant enrich the entry written by an earlier, narrower one.

---

## 6. Interaction with the foundation seam and other directives

- **Foundation (astjson StructuralCopy + arena).** The coordinate L1 depends entirely on the StructuralCopy isolation discipline of §3 of [01-ARCHITECTURE-SPEC.md](../01-ARCHITECTURE-SPEC.md): copy-on-export, copy-on-inject, working-copy-and-swap on merge. All values live on `l.jsonArena` for the request lifetime; there are no heap↔arena crossings, so no `DeepCopy` is needed.

- **`@provides` / `ProvidesData`.** The widening check and the normalize/denormalize transforms reuse the exact `*Object` machinery built for `@provides` ([directives/provides.md](provides.md)). `RequestScopedField.ProvidesData` is the same alias-aware `*Object` type. This is the only directive dependency: `@requestScoped` cannot ship before `@provides` `ProvidesData` exists.

- **`@requires`.** Widening folds in hidden `@requires` selection sets so a widened first fetch still carries the inputs a later participant's `@requires` chain needs (see the requires-chain e2e case in §7).

- **`@key`.** Used only indirectly: entity fetches that participate still build their normal entity representations from `@key`; the coordinate L1 itself is keyed on the directive `key`, not on `@key`.

- **L2 / mutation / subscription specs.** Independent. The coordinate L1 never reads or writes L2, never participates in mutation invalidation, and never fires on subscription events. It is a side-branch in the dependency ordering ([02-DIRECTIVE-INVENTORY.md](../02-DIRECTIVE-INVENTORY.md) §3).

- **Per-request gating.** Disabling L1 via `X-WG-Disable-Entity-Cache` or `X-WG-Disable-Entity-Cache-L1` headers also disables the coordinate L1, because both `tryRequestScopedInjection` and `exportRequestScopedFields` check the shared `EnableL1Cache` flag.

---

## 7. End-to-end test plan

All assertions follow the universal + E2E rules:
`assert.Equal` on full values only, no `Contains`/`GreaterOrEqual`/`Greater`,
inline literal queries and responses,
vertical one-item-per-line struct literals for any multi-key/multi-event list,
and every `ClearLog()` immediately followed by `GetLog()` + full assertion.
The `execution/engine/request_scoped_widening_e2e_test.go` recorder pattern
(asserting the exact upstream request stream per subgraph) is the sanctioned exception
for these tests; new tests outside that file follow the inline `federationtesting` + `QueryStringWithHeaders` flow.

Federation services `accounts` / `products` / `reviews` are used where an entity is needed;
the widening cases use purpose-built `viewer` / `articles` subgraphs because the stock services do not declare `@requestScoped`.

### Case 1 — Root fetch widens, entity fetch is skipped (happy path)

- **Subgraphs.** `viewer` declares `Query.currentViewer @requestScoped(key: "viewer")` and `Article.currentViewer @requestScoped(key: "viewer")`. `articles` owns `Query.article`.
- **Query.**
  ```graphql
  query {
    currentViewer { id name }
    article { id title currentViewer { id name email } }
  }
  ```
- **What is coordinated.** `key: "viewer"` ⇒ coordinate L1 entry `viewer.viewer`. Widening lifts the root `currentViewer` fetch to `{id name email}` (the union). The article's `currentViewer` entity hop is satisfied by injection and skipped.
- **Assertions.**
  - Full client response with `assert.Equal` (compacted), keeping the narrow root shape and the wider article shape:
    `{"data":{"currentViewer":{"id":"v1","name":"Alice"},"article":{"id":"a1","title":"T1","currentViewer":{"id":"v1","name":"Alice","email":"alice@example.com"}}}}`.
  - Exact upstream request stream: `viewer` receives exactly `{currentViewer {id name email}}`; `articles` receives exactly `{article {id title __typename}}`. No viewer entity (`_entities`) request is sent.

### Case 2 — `@requires` chain widens the base fetch, entity hop skipped, third subgraph still fed

- **Subgraphs.** `viewer` base (owns `currentViewer.name`), `articles`, and `handles` which owns `Viewer.handle @requires(fields: "name")`.
- **What is coordinated.** Widening folds the hidden `@requires(name)` dependency into the widened root viewer fetch (aliased `name`, `__typename`, `id`). The requestScoped entity hop is skipped; the `handles` entity fetch still runs and receives representations carrying the widened `name`.
- **Assertions.**
  - `viewer` receives exactly `{currentViewer {viewerName: name __typename id}}` (one request).
  - `handles` receives exactly the `_entities` query with `Variables` equal (inline, compacted) to `{"representations":[{"__typename":"Viewer","id":"v1","name":"Alice"}]}`.
  - Full client response asserted with `assert.Equal`.

### Case 3 — Field / argument conflicts widen through synthetic aliases

- **Query.** Two participants select the same `key: "viewer"` field with conflicting arguments or field shapes.
- **What is coordinated.** Widening assigns deterministic synthetic aliases so the single widened upstream query is valid, then remaps response keys back per branch.
- **Assertions.**
  - Exact upstream query string (with synthetic aliases) asserted with `assert.Equal`.
  - Full client response asserted with `assert.Equal`, each branch keeping its own requested shape.

### Case 4 — Field-widening backstop blocks an unsafe skip (unit, `resolve` package)

- **Setup.** Pre-seed `requestScopedL1["viewer.Personalized.currentViewer"]` with a narrow value `{"id":"1","name":"Alice"}`; a hint whose `ProvidesData` requires `{id, name, email}`.
- **Assertions.** `tryRequestScopedInjection` returns `false` and the item is byte-for-byte unchanged: `assert.Equal(t, `{"id":"99"}`, string(items[0].MarshalTo(nil)))`. A symmetric subtest with a wide-enough cached value returns `true` and asserts the full injected item.

### Case 5 — Fail-closed on nil `ProvidesData` (unit)

- **Setup.** L1 has the value, but the hint's `ProvidesData` is nil (the field is not selected by this fetch).
- **Assertion.** `tryRequestScopedInjection` returns `false`; items untouched (`assert.Equal` on the full item).

### Case 6 — Export normalization + copy independence (unit)

- **Setup.** Resolve a fetch whose `@requestScoped` field is aliased and has aliased sub-fields; export to L1.
- **Assertions.**
  - The stored L1 value is in schema names (alias stripped): `assert.Equal` on the full marshaled L1 entry.
  - Mutating the source response value afterward does not change the stored L1 value (copy-on-export): `assert.Equal` on the full L1 entry before and after a source mutation.

### Case 7 — Analytics / trace folding (e2e or resolve)

- **Setup.** Run Case 1 with cache analytics enabled; read `ctx.GetCacheStats()`.
- **Assertions.** Full `CacheAnalyticsSnapshot` with `assert.Equal` (normalized). The skipped entity fetch must appear as an **L1 hit**, not a stale L1 miss — `cacheTraceRequestScopedHits` is folded into `L1Hit`/`L1Miss` at trace-build time. Each snapshot event line carries a trailing comment explaining why it occurred. If the test uses a cache log, every `ClearLog()` is followed by `GetLog()` + a full vertical assertion before the next clear.

---

## 8. Acceptance criteria

A reviewer can verify the implementation against this checklist (mirrors AC-RS-01..07 in the acceptance-criteria doc):

- [ ] **Directive definition.** `directive @requestScoped(key: String!) on FIELD_DEFINITION`; `key` is mandatory; composition rejects a missing `key`.
- [ ] **Composition warning.** A `key` declared on exactly one field in a subgraph produces a warning, not an error, and does not break composition.
- [ ] **Symmetric emission.** Datasource `ConfigureFetch` emits one `RequestScopedField` per annotated field for both root and entity fetches; entity fetches also resolve interface-object types via `InterfaceObjects`; entries are de-duplicated by `(FieldName, L1Key)`.
- [ ] **`ProvidesData` population + drop.** The planner sets `ProvidesData` from the response tree by response key, and **drops** carriers whose field is not selected by the fetch (nil `ProvidesData`).
- [ ] **L1 key shape.** Coordinate L1 key is exactly `"{subgraphName}.{key}"`, alias-independent, no header prefix, no per-request hash.
- [ ] **Thread + arena discipline.** `requestScopedL1` is read/written only on the main thread (Phase 1.5, Phase 3.5, `resolveSingle`); no goroutine touches it; all values live on `l.jsonArena`.
- [ ] **Gating.** Both inject and export return early when `EnableL1Cache` is false (including via the disable-L1 header).
- [ ] **Field-widening backstop.** Injection runs `validateItemHasRequiredData(cachedValue, hint.ProvidesData)` and refuses to skip when the cached value is too narrow.
- [ ] **Fail-closed.** Injection returns false when any hint's `ProvidesData` is nil.
- [ ] **Collect-then-inject.** A failure on any hint leaves all items untouched; never partial-inject.
- [ ] **Copy-on-inject / copy-on-export.** Inject uses `structuralCopyDenormalized`; export uses `structuralCopyNormalized`; multi-item injection copies per item.
- [ ] **Working-copy-and-swap on merge.** Export into an existing entry copies, merges into the copy, and stores the copy on success or keeps the live entry on merge failure — never mutates in place.
- [ ] **Selection widening.** `propagateRequestScopedWidening` widens grouped participants to the union of their selections (including hidden `@requires`), resolves conflicts with deterministic synthetic aliases, and only proceeds when all participants share a return type.
- [ ] **Trace/analytics.** On a successful skip, `LoadSkipped` is set at every call site and `cacheTraceRequestScopedHits` is folded into L1 hit counters; the snapshot shows the skip as an L1 hit.
- [ ] **End-to-end.** The widening e2e cases (root widening, requires chain, argument/field conflicts) pass with exact upstream-request-stream assertions and full-response `assert.Equal`.
