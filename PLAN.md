# PLAN — Pre-fetch field authorization mode

**Ticket:** [ENG-9828](https://linear.app/wundergraph/issue/ENG-9828/router-add-pre-fetch-scope-authorization-mode-for-query-fields) (Urgent).
**Customer:** Monday.com, migrating Apollo Router → Cosmo Router.
**Scope of this document:** the `graphql-go-tools` (`v2/`) engine changes only.
The Cosmo Router wiring (config option, JWT/scope extraction, batch authorizer method) lands in a **second PR** that references the primitives added here.

The ticket is framed around query fields, but the mechanism below is operation-type-agnostic (§3.4): when enabled it authorizes all protected fields up front, for queries, mutations, and subscriptions.

---

## 0. Status

A first cut is already implemented in `v2/pkg/engine/resolve` using a per-fetch, query-only approach
(`isFetchAuthorized` calls `AuthorizePreFetch` per protected query root field inside the parallel fetch
goroutines; `mergeResult` seeds the decision cache).
It passes (full `resolve` package + `-race`).

**This plan supersedes that with a single up-front batch design** (below), decided after confirming that
the Cosmo Router's authorization decision is a pure function of `(coordinate, request token scopes)` —
`input` / `object` / `dataSourceID` are unused (§5).
The implementation will be refactored to match.
The opt-in switch and the reuse of the resolvable's existing decision cache are kept; the flag is
generalized from query-only to all operation types.

---

## 1. Problem

`@requiresScopes` / `@authenticated` on query fields are enforced *after* the subgraph fetch today,
while walking returned data.
Two consequences the customer hit:

1. **Unnecessary subgraph work.**
   An unauthorized query selection still triggers its subgraph fetch;
   the field is only stripped from the response afterwards.

2. **Missing error on empty results.**
   When a protected field resolves to an empty list / no objects, no `UNAUTHORIZED_FIELD_OR_TYPE` error
   is emitted, because the post-fetch check only runs while iterating actual returned objects.

Apollo Router evaluates these directives *before* execution (they are scope-only, i.e. data-independent),
so the customer wants an opt-in mode that matches that behavior.

### 1.1 Root cause in the code

- **Queries are excluded from pre-fetch authorization.**
  `Loader.isFetchAuthorized` (`loader.go`) short-circuits for `ast.OperationTypeQuery`;
  only mutations / subscriptions reach `authorizer.AuthorizePreFetch` today.
- **Post-fetch authorization is data-driven.**
  `Resolvable.authorizeField` → `authorize` calls `authorizer.AuthorizeObjectField` while walking data;
  in `walkArray` (`resolvable.go:942`) an **empty** array never enters the element loop, so nested
  protected fields are never authorized → no error (AC3 gap; affects nested fields of all operation types).
- **The decision cache already exists.**
  `Resolvable.authorize` keys allow/deny by `xxhash(dataSourceID, typeName, fieldName)` and consults
  `authorizationAllow` / `authorizationDeny` **before** calling the authorizer — so a pre-seeded decision
  is honored during the walk for free.

---

## 2. Responsibility split

| Concern | Where |
| --- | --- |
| Read `@requiresScopes` / `@authenticated` from schema, populate `HasAuthorizationRule` + required-scope config | Router (already done) |
| Extract token scopes, decide allow/deny for a coordinate | Router (`CosmoAuthorizer`) |
| New **batch** authorizer interface; config option to enable pre-auth; docs (AC5) | Router (PR #2) |
| Detecting which fields need auth (plan time), the up-front batch call, fetch pruning, consistent errors | **`graphql-go-tools` — this PR** |

`graphql-go-tools` owns *timing and mechanics*; the router owns *policy*.

---

## 3. Design — opt-in up-front batch field authorization

### 3.0 Two modes, one switch

A per-request switch selects how protected fields are authorized:

- **Disabled (default) — unchanged, per-fetch / post-fetch behavior.**
  Queries filter post-fetch via `AuthorizeObjectField`;
  mutations / subscriptions pre-fetch via `AuthorizePreFetch` per root field.
  Byte-for-byte identical to today (AC4).

- **Enabled — single up-front batch.**
  All protected fields selected by the operation are authorized **once, before any fetch executes**,
  and every consumer downstream only *reads* the results — no authorizer calls during load or resolve.

The switch is `Context.SetPreFetchFieldAuthorization(bool)` (generalized from the query-only flag in the
current code), default off, reset in `Context.Free`.

### 3.1 Plan-time: detect which fields need a decision (and no-op when none)

The set of coordinates that need an authorization decision is **request-independent** — it is fixed by the
operation and the schema's directives, both known at planning.
So it is computed **once, at plan build** (D1 resolved: eager at plan build; no shared-plan mutation at
request time), and stored on the cached plan in an explicit structure that is both human- and
machine-readable:

```go
// AuthorizationCoordinate is a protected field coordinate paired with the data source that resolves it.
// Collected once at plan build; drives the up-front batch when pre-fetch authorization is enabled.
type AuthorizationCoordinate struct {
    DataSourceID string
    Coordinate   GraphCoordinate // TypeName + FieldName
}
```

stored on `GraphQLResponseInfo` next to `OperationType`:

```go
type GraphQLResponseInfo struct {
    OperationType ast.OperationType
    // AuthorizationCoordinates lists every protected field the operation selects, deduplicated by
    // {DataSourceID, TypeName, FieldName}. Populated once at plan build. Empty when the operation
    // selects no protected field, in which case pre-fetch authorization is a no-op.
    AuthorizationCoordinates []AuthorizationCoordinate
}
```

**How it is collected** — `CollectAuthorizationCoordinates(response)` in the resolve package, invoked by
the planner once the `GraphQLResponse` is built:

- every `FetchInfo.RootFields[i]` with `HasAuthorizationRule`, paired with `FetchInfo.DataSourceID`, and
- every `Data`-tree `Field` with `Info.HasAuthorizationRule`, paired with `Field.Info.Source.IDs[0]`,
- deduped by `{DataSourceID, TypeName, FieldName}` (the resolvable's decision-cache key — §3.2 / D2).

Extracting from **both** sources guarantees the seeded cache entries match every lookup the loader
(`FetchInfo.DataSourceID`) and the resolvable (`Source.IDs[0]`) will perform.
This list is exactly the set of coordinates the authorizer would ever be asked about (§5), computed once
and reused across requests.

**No-op guarantee:** if `AuthorizationCoordinates` is empty, the enabled mode does nothing — no batch call,
no extra walk, zero overhead.
"Pre-auth enabled" is therefore free for the common case of operations that touch no protected field.

### 3.2 Interface: a separate, non-breaking batch authorizer

Add a **new interface**, distinct from `Authorizer`, so nothing about the existing contract changes.
It returns **explicit allow/deny decisions** — no nil-pointer sentinel:

```go
// AuthorizationDecision is an explicit allow/deny decision for a single field coordinate.
type AuthorizationDecision struct {
    // Allowed reports whether the coordinate is authorized for the request.
    Allowed bool
    // Reason optionally explains a denial. Only meaningful when Allowed is false.
    Reason string
}

// BatchAuthorizer authorizes a set of field coordinates in a single call, before execution.
// decisions is aligned by index with coordinates and must contain exactly one decision per
// coordinate. Decisions must be a pure function of the coordinate and the request context
// (token scopes); no subgraph input or response data is provided, by design.
// It is operation-type-agnostic.
type BatchAuthorizer interface {
    AuthorizeFields(ctx *Context, coordinates []GraphCoordinate) (decisions []AuthorizationDecision, err error)
}
```

Decisions are values, not pointers, precisely to avoid re-introducing a nil sentinel.
`AuthorizationDecision` coexists with the existing `AuthorizationDeny{Reason}` used by the per-field
hooks — no change to the legacy path.
When pre-auth is enabled, the resolver type-asserts the configured `Authorizer` to `BatchAuthorizer` and
seeds the cache:

```go
for i := range coordinates {
    if decisions[i].Allowed {
        r.seedAuthorizationAllow(dataSourceID, coordinates[i])
    } else {
        r.seedAuthorizationDeny(dataSourceID, coordinates[i], decisions[i].Reason)
    }
}
```

The router's `CosmoAuthorizer` implements `AuthorizeFields` by looping its existing `validateScopes` over
the coordinates (§5) — a near-trivial addition.
(Interface name is bikeshed-able.)

> **Decision D2 (dataSourceID) — grounded in both codebases:**
> Authorization is architecturally **per-coordinate**, not per-`(coordinate, dataSource)`:
> - go-tools: `HasAuthorizationRule` is looked up by `ForTypeField(typeName, fieldName)`
>   (`plan/visitor.go:501`) and stored as a single bool on the coordinate; only `Source.IDs[0]` is ever
>   used (`resolvable.go:831`), even though `TypeFieldSource.IDs` is a slice (a coordinate can be served by
>   several subgraphs, e.g. `@shareable`).
> - cosmo: proto `FieldConfiguration` is `{type_name, field_name, authorization_configuration}` with no
>   subgraph dimension; `field_configurations` is one global list; `requiredScopesForField` matches on
>   coordinate only; composition forces all subgraphs exposing a field to agree on its scopes.
>
> Therefore:
> - **Batch interface → coordinate-only** (no `dataSourceID`). The decision is per-coordinate; the ask is
>   deduped by coordinate → exactly one authorizer decision per coordinate.
> - **Cache key → keep `dataSourceID`** (unchanged). It has been in the key since PR #728 and no code has
>   ever used a per-datasource decision, so it is redundant *but harmless* given per-coordinate semantics.
>   Keeping it is the **lower-complexity** choice: seed the exact `{dataSourceID, coordinate}` keys the
>   existing readers already compute — loader `{FetchInfo.DataSourceID, coord}`, resolvable
>   `{Source.IDs[0], coord}` — by extracting protected coordinates from **both** sources and seeding both.
>   No change to `authorize()`'s key logic, no mode-aware hashing, and no silent narrowing of the legacy
>   `Authorizer` contract (which passes `dataSourceID` and an external implementer is permitted to branch on).
>   The fan-out is a couple of extra map entries with an identical decision; the authorizer is still called
>   once per coordinate.

> **Decision D6 (result shape):** decisions are index-aligned with the input coordinates.
> Alternative: embed the `Coordinate` in `AuthorizationDecision` to make the result self-describing and
> order-independent, at the cost of echoing coordinates back.
> Recommend index-aligned (simpler 1:1 loop on the router side); document the contract.

### 3.3 Enabled path: resolve once, seed, never re-auth

In `Resolver.ResolveGraphQLResponse`, before the loader runs, when pre-auth is enabled **and** the plan
has protected coordinates (§3.1) **and** the authorizer implements `BatchAuthorizer`:

1. Call `AuthorizeFields(ctx, coordinates)` **once**.
2. Seed the resolvable decision cache from the result (`seedAuthorizationDeny` / `seedAuthorizationAllow`,
   keyed via the existing `authorizationDecisionID`).

This runs on a single goroutine before the loader, so there is no mutex, no shared-hasher race, no
`result`-recording, no `mergeResult` seeding.
From here on, the loader and resolvable **only read** an immutable map — authorization is never evaluated
again for the request.

### 3.4 Enforcement stays operation-type-specific (loader), reads only

The batch changes *where decisions come from*, not *what a deny does*.
`isFetchAuthorized` in the enabled path reads the seeded cache for `info.RootFields` (no authorizer call)
and enforces per operation type:

- **Query:** skip the origin request only when every root field the fetch serves is denied
  (authorized siblings still fetch); otherwise run and let the resolvable null the denied fields.
- **Mutation / Subscription:** whole-fetch reject on any denied root field — same guarantee as today
  (don't send unauthorized, side-effecting or single-root operations to the origin), just sourced from the
  cache instead of a per-fetch call.

### 3.5 Resolvable: consistent errors incl. empty results (AC3)

1. **During the walk:** unchanged — `authorize`'s existing cache lookup nulls + errors denied fields and,
   because decisions are pre-seeded, never calls `AuthorizeObjectField` in the enabled path.

2. **Nested / empty-parent emission (D3 — draft design).**
   The data walk misses a protected field only when it cannot descend to it: the enclosing list resolved
   to `[]`, or a nullable ancestor resolved to `null`.
   Because every protected coordinate already has a decision, closing this is a pure, local plan-tree walk
   (no authorizer calls, no concurrency): after resolution, walk the plan's `Object` / `Array` / `Field`
   tree, and wherever the data walk stopped, emit one error per **denied** protected coordinate in the
   unreached subtree.

   **Emitted error (draft)** — reusing the message/format of the existing `addRejectFieldError`
   (`resolvable.go:878`):

   ```json
   {
     "message": "Unauthorized to load field '<dotted coordinate path>', Reason: <reason>.",
     "path": ["<structural response path — field names; array segments carry no index>"],
     "extensions": {"code": "UNAUTHORIZED_FIELD_OR_TYPE"}
   }
   ```

   **Example** — `query { products { secret } }`, `Product.secret` denied, `products` resolves to `[]`:

   ```json
   {"errors":[{"message":"Unauthorized to load field 'Query.products.secret', Reason: missing scope 'read:secret'.","path":["products","secret"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}],"data":{"products":[]}}
   ```

   `products` still renders (`[]`); only the unauthorized `secret` is reported — matching Apollo's
   field-removal model.
   This closes the gap for nested fields of every operation type (the per-fetch design could not).

   **One detail to lock with a golden test:** the array-segment representation in `path` —
   index-less (`["products","secret"]`, drafted above), wildcard (`["products","@","secret"]`, matching the
   engine's fetch-path style), or list-only (`["products"]`).
   Finalize against Apollo's actual output.

### 3.6 Disabled path (default): unchanged

No batch call, no use of the extracted list.
Queries filter post-fetch via `AuthorizeObjectField`;
mutations / subscriptions pre-fetch via `AuthorizePreFetch`.
Identical to today (AC4).

### 3.7 Enabled but authorizer is not a `BatchAuthorizer`

**D4 resolved: hard error at setup.**
When pre-fetch field authorization is enabled but the configured `Authorizer` does not implement
`BatchAuthorizer`, fail loudly at setup rather than silently falling back to the disabled per-fetch path.
Misconfiguration should be obvious, not a silent downgrade of authorization behavior.
The router controls both the config option and the authorizer, so this is safe.

---

## 4. Why batch over per-fetch

| | Per-fetch (current / disabled path) | Up-front batch (enabled path) |
| --- | --- | --- |
| Authorizer calls | one per protected fetch / per object field | one per request (or zero — §3.1 no-op) |
| Concurrency | decisions produced in parallel goroutines | resolved before any goroutine; readers never mutate |
| Dedup | same coordinate re-checked per fetch / per array element | deduped once |
| Nested empty-parent (AC3) | not handled | closed via a pure plan-tree emission |
| Router | mutex on missing-scopes accumulation (concurrent hooks) | single call → **mutex removable** (§5) |
| Op-type coverage | queries excluded today | queries + mutations + subscriptions |

---

## 5. Router alignment (evidence)

`CosmoAuthorizer` (`router/core/authorizer.go`) — both decision hooks reduce to:

```go
isAuthenticated, actual := a.getAuth(ctx.Context())   // token scopes, from context
required := a.requiredScopesForField(coordinate)      // coordinate → scopes, precomputed at startup
return a.handleRejectUnauthorized(a.validateScopes(ctx, coordinate, required, isAuthenticated, actual))
```

Verified in the source:

- **`input` / `object` / `dataSourceID` are never referenced** in `AuthorizePreFetch`,
  `AuthorizeObjectField`, or `validateScopes` — decision = `f(coordinate, token scopes)`.
  → a coordinate-only up-front batch loses nothing, for any operation type.
- **Required scopes are precomputed per-schema** (`fieldConfigurations` from
  `EngineConfig.FieldConfigurations`, loaded at startup).
- **Token scopes come from `authentication.FromContext`** — identical for every coordinate in a request.
- The only per-request mutable state is the response-extension "missing scopes" accumulator
  (`addMissingScopes`), **mutex-protected precisely because the hooks are called concurrently**.
  A single up-front batch is not concurrent → **that mutex can be removed**, and the accumulator then
  naturally covers every selected protected coordinate (the more complete, Apollo-like extension output).

Conclusion: the batch fits the router's model exactly and simplifies both codebases.

---

## 6. Decisions (resolved)

- **D1 — RESOLVED.** Coordinate extraction runs eagerly at plan build, stored on
  `GraphQLResponseInfo.AuthorizationCoordinates` as `[]AuthorizationCoordinate` (§3.1).
- **D2 — RESOLVED.** Batch interface is coordinate-only; the decision cache keeps `dataSourceID` in its key
  (§3.2). Grounded: authorization is per-coordinate in both codebases.
- **D3 — DRAFTED (§3.5).** Empty/absent-parent errors emitted via a pure plan-tree walk.
  Only remaining detail: the array-segment representation in the error `path`, to be locked with a golden
  test against Apollo output.
- **D4 — RESOLVED.** Enabled without a `BatchAuthorizer` → hard startup error (§3.7).
- **D6 — RESOLVED.** Batch result is index-aligned `[]AuthorizationDecision` (§3.2).

---

## 7. Backward compatibility & proof

**Claim.** With pre-fetch authorization disabled (the default), every response — data, errors, and
extensions — is byte-for-byte identical to `master`, for all operations and all operation types; and the
additive refactors do not perturb the shared code paths.

The proof has three layers: structural (where change is even possible), local equivalence (the few edited
functions), and empirical (byte-exact evidence).
The first two make the third trustworthy.

### 7.1 Structural — the feature is unreachable when off

All new behavior is funneled through one runtime gate:

```
enabled := ctx.preFetchFieldAuthorization
        && len(response.Info.AuthorizationCoordinates) > 0   // no protected fields → no-op (§3.1)
        && authorizer implements BatchAuthorizer             // else hard startup error (D4), never silent
```

The up-front batch phase, the loader / resolvable cache reads, and the empty-parent emission all sit under
this gate.
Checkable invariants:

- the flag's zero value is `false`; it is set only by an explicit setter and reset in `Context.Free`;
- **when off, the decision cache is never seeded**, so the shared `authorize()` lookup finds nothing new
  and behaves exactly as today;
- no new branch runs without the gate (grep-enforceable).

### 7.2 Local equivalence — the few in-place edits

The batch design edits very little existing code (it adds an up-front phase + read sites):

- **`authorize()` cache-key extraction** — the helper writes `dataSourceID, typeName, fieldName` in the
  same order → identical hash → identical cache behavior.
- **New `GraphQLResponseInfo.AuthorizationCoordinates` field** — grounded: `Info` is read only for
  `.OperationType` (`resolve.go:355,407`) and is never marshaled into the response or query plan, so a new
  field cannot leak into output.
- The batch design does **not** touch `isFetchAuthorized` or `mergeResult`, removing those equivalence
  obligations entirely.

### 7.3 Empirical — byte-exact evidence (the actual proof)

- **Existing suite unchanged and green.** Tests assert full response bodies (the `assert.Equal` house
  rule) and the `execution/` e2e uses golden snapshots.
  Passing with **zero edits to existing tests and zero golden-file changes** is direct byte-level evidence.
- **Differential corpus (strongest).** Run a broad corpus of operations against `master` and against the
  branch with the flag off; byte-diff every response, every error array, and the subgraph-call sequence.
  Zero diff over the corpus is the proof.
- **Dormancy invariants (flag off).** `BatchAuthorizer.AuthorizeFields` call-count `== 0`;
  legacy `AuthorizePreFetch` / `AuthorizeObjectField` call counts and order match baseline.

### 7.4 Risks to guard explicitly

- **`IncludeQueryPlanInResponse`** — add one test asserting query-plan output is unchanged (expected to
  pass, since `Info` is not rendered — but assert it).
- **Plan-build extraction cost when off** — extraction runs even when disabled (request-independent,
  computed once, cached on the plan).
  This is perf, not behavior; benchmark the off path for allocation / latency regression, and gate
  extraction behind "any protected field exists" if needed.
- **Determinism** — extraction must produce a stable, deduplicated order so cached plans and any
  plan / debug output stay identical run-to-run.

### 7.5 Required CI evidence (merge gate)

1. Full `resolve` package + `execution/` e2e green, no existing test or golden file modified.
2. Differential corpus: master vs branch (flag off) → zero byte diffs.
3. Dormancy assertions (§7.3) in the disabled-mode tests.
4. Off-path benchmark: no significant allocation / latency regression.

---

## 8. Test plan

`v2/pkg/engine/resolve`, full-body `assert.Equal` (house rule), extend `testAuthorizer` with a
`BatchAuthorizer` implementation and a batch-call counter:

1. Disabled → byte-identical to current post-fetch / per-fetch behavior (AC4 regression guard).
2. Enabled, protected query root field denied, dedicated fetch → fetch **not** executed (mock `Load`
   count 0), `UNAUTHORIZED_FIELD_OR_TYPE` emitted, field null (AC2).
3. Enabled, denied field sharing a fetch with an authorized field → fetch runs once, denied field null +
   error, authorized field present (AC2 intent).
4. Enabled, protected field returning an empty list → error emitted (AC3, root level).
5. Enabled, **nested protected field under an empty / null parent** → error emitted (AC3).
6. Enabled, **batch called exactly once** per request regardless of fetch / array fan-out; and **zero**
   `AuthorizePreFetch` / `AuthorizeObjectField` calls (decisions come only from the batch).
7. Enabled, **no protected fields selected → batch not called at all** (§3.1 no-op).
8. Enabled, mutation with a denied root field → whole-fetch reject, sourced from the cache.
9. Enabled but authorizer is not a `BatchAuthorizer` → per D4 (startup error or documented fallback).

Confirm the `execution/` e2e module still passes (federation auth scenarios).

---

## 9. Router follow-up PR (#2)

- Implement `BatchAuthorizer.AuthorizeFields` on `CosmoAuthorizer` (loop `validateScopes`; already
  operation-type-agnostic).
- Remove the missing-scopes accumulator mutex (decisions now resolved in one non-concurrent call).
- Add a **config option** to enable the pre-auth mode and call
  `Context.SetPreFetchFieldAuthorization(true)` per request when enabled.
- Docs for AC5: post-fetch / data-aware (default) vs pre-fetch scope-only, and when to use each.
- Consider changing the default only in a future **major** release with migration guidance (AC4).

---

## 10. Acceptance-criteria mapping

| AC | Covered by |
| --- | --- |
| AC1 opt-in flag | §3.0 switch + Router PR #2 config option |
| AC2 reject/remove before fetch, no unnecessary subgraph calls | §3.4 loader pruning |
| AC3 consistent error even on empty list / no objects | §3.5 (walk + plan-tree emission) |
| AC4 post-fetch remains default | §3.6 default off; §7 proof; §8 test 1 |
| AC5 docs | Router PR #2 (§9) |
