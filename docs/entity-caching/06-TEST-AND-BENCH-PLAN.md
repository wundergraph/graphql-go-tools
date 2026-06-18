# 06 — Test & Benchmark Plan

> Part of the entity-caching re-implementation document set.
> Navigation: [00-OVERVIEW](./00-OVERVIEW.md) ·
> [01-ARCHITECTURE-SPEC](./01-ARCHITECTURE-SPEC.md) ·
> [02-DIRECTIVE-INVENTORY](./02-DIRECTIVE-INVENTORY.md) ·
> [03-PR-PLAN-graphql-go-tools](./03-PR-PLAN-graphql-go-tools.md) ·
> [04-PR-PLAN-router](./04-PR-PLAN-router.md) ·
> [05-ASTJSON-PRIMITIVES](./05-ASTJSON-PRIMITIVES.md) ·
> **06-TEST-AND-BENCH-PLAN (this file)** ·
> [07-UNRELATED-FINDINGS](./07-UNRELATED-FINDINGS.md) ·
> [08-EXECUTION-RUNBOOK](./08-EXECUTION-RUNBOOK.md)

---

## 0. Who this document is for

You are an engineer (or an AI agent) about to re-implement the entity-caching
feature as a stack of small PRs.
You have never seen this feature before.
This document tells you, for every PR, **what tests prove the PR is correct**,
**what benchmarks guard against performance regressions**,
and **the non-negotiable rules every test must follow**.

The single most important idea:
this feature already has a complete, working test suite on the
`entity-caching-v2` branch.
We are not inventing tests from nothing.
We are **re-deriving** a clean, stacked test suite from a proven reference suite,
keeping its taxonomy, its conventions, and its assertions, while splitting the
large monolithic test files into reviewable per-PR slices.

Two source trees matter throughout this document.

- The **reference tree** (already implemented, the thing we copy patterns from):
  `entity-caching-v2/v2/pkg/engine/resolve/` and
  `entity-caching-v2/execution/engine/`.
- The **target tree** (where we re-implement):
  the same relative paths under the re-implementation worktree.

All file paths below are relative to the repo root unless stated otherwise.

---

## 1. The two test tiers

The suite splits cleanly into two layers.
Understand this split before writing a single line.

### 1.1 Unit tier — white-box, package `resolve`

Location: `v2/pkg/engine/resolve/`.
These tests reach inside the package.
They build a `Loader` struct literal directly, inject a gomock `DataSource`,
drive `Loader.LoadGraphQLResponseData`, and assert on rendered JSON, on cache
state, and on internal helper return values.
They are fast, deterministic, and exercise the mechanics: cache-key rendering,
L1/L2 copy invariants, merge sites, analytics collection, negative/mutation/
request-scoped paths.

### 1.2 E2E tier — black-box, package `engine_test`

Location: `execution/engine/`.
These tests stand up a **real 3-subgraph federation gateway**
(accounts / products / reviews) over HTTP, send GraphQL through it, and assert on
the HTTP response body, on a recorded cache-operation log, on subgraph HTTP call
counts, and on a full `CacheAnalyticsSnapshot` returned via the
`X-Cache-Analytics` response header.
They prove the feature works end to end through real planning, real fetch trees,
and a real (fake-backed) cache.

> **Do not blur the tiers.**
> A copy-invariant bug belongs in a unit test.
> A "two requests, second one hits cache, zero subgraph calls" assertion belongs
> in an E2E test.
> If you find yourself standing up a gateway to test cache-key string rendering,
> stop — that is a unit test.

---

## 2. MANDATORY conventions — the checklist

These rules are enforced by `CLAUDE.md`,
`v2/pkg/engine/resolve/CLAUDE.md`, and `execution/engine/CLAUDE.md`.
They are machine-checkable and reviewers will reject violations.
Treat this as a pre-flight checklist for **every** test you write or edit.

### 2.1 Universal rules (both tiers)

- [ ] **Exact assertions only.**
  Use `assert.Equal` with the full expected value.
  Never `assert.Contains`, `assert.GreaterOrEqual`, `assert.Greater`,
  `assert.Less`, or any fuzzy comparison.
  A fuzzy comparison is a code smell: it means you did not compute the real value.
  Investigate until you know the exact value, then assert it.
- [ ] **Assert entire structs inline.**
  Assert a whole `CacheAnalyticsSnapshot` or a whole `[]CacheLogEntry` in one
  `assert.Equal`.
  Never loop over fields with individual assertions.
  For large structs, write the full expected value inline anyway — readability
  inside the test beats line count.
- [ ] **Inline literal data.**
  GraphQL queries, cache keys, byte sizes, expected JSON responses appear
  inline at the assertion or setup site that uses them.
  Never in file-level `const` blocks or shared vars that force a reviewer to
  scroll away.
- [ ] **Snapshot "why" comments.**
  Every event line in a `CacheAnalyticsSnapshot` (or any event-stream / cache-log
  assertion) carries a trailing comment explaining **why** that event occurred
  (the causation), not what its value is.
  Good: `// First request, L2 empty`.
  Bad: `// this is a miss`.
- [ ] **Cache-log clear+assert pairing.**
  Every `cache.ClearLog()` is followed by `cache.GetLog()` plus full assertions
  **before** the next `ClearLog()` or the end of the test.
  Never clear a log without first verifying its contents.
- [ ] **Vertical multi-key literals.**
  Any struct literal with two or more nested slices/maps/long-string fields
  (cache-log entries, snapshot events) wraps one item per line.
  Never a 200-character single line.
- [ ] **Modern Go.**
  Before writing Go, load the project guidelines via the `/use-modern-go` skill.
  Use range-over-int, range-over-func where it reads cleaner, `errors.Is`/`As`,
  structured idioms.
  No legacy `for i := 0; i < n; i++` where `for i := range n` reads better.
  Benchmarks use `for b.Loop()`, not `for i := 0; i < b.N; i++`.

### 2.2 Unit-tier-only rules (package `resolve`)

- [ ] **Singleflight off.**
  Set `ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true`.
  Forgetting this silently changes fetch counts.
  This is the most commonly-forgotten line — see [§9 risk](#9-known-risks-and-anti-patterns).
- [ ] **Caching flags explicit.**
  Set `ctx.ExecutionOptions.Caching = CachingOptions{EnableL1Cache: ..., EnableL2Cache: ...}`
  for every test, even when both are false.
- [ ] **Arena every time.**
  `arena.NewMonotonicArena(arena.WithMinBufferSize(1024))`, then
  `NewResolvable(ar, ResolvableOptions{})`, then
  `resolvable.Init(ctx, nil, ast.OperationTypeQuery)`.
- [ ] **Shared setup helpers are allowed here** (the no-shared-helper rule is
  E2E-only) — a small `newCachingTestLoader(...)` inside the package is fine and
  is the recommended home for the singleflight-off + arena boilerplate.

### 2.3 E2E-tier-only rules (package `engine_test`)

- [ ] **Self-contained subtests.**
  Each `t.Run` reads top-to-bottom on its own.
  **Duplication is preferred over sharing.**
  Do NOT extract `newXxxFederationTestEnv(...)` helpers.
  Do NOT lift `SubgraphCachingConfigs` / `CachingOptions` into a top-level var
  used by one subtest — inline them into the subtest body.
- [ ] **Inline GraphQL.**
  Use `QueryStringWithHeaders` with the query string inline.
  Do not load queries from `.query` files via `cachingTestQueryPath(...)`
  (that helper is legacy — see [§9](#9-known-risks-and-anti-patterns)).
- [ ] **Full snapshot assertions.**
  Assert the entire normalized `CacheAnalyticsSnapshot`, not a partial one.
- [ ] **Header `.Clone()`.**
  Any `http.Header` returned from a mock must be `.Clone()`'d — the HTTP client
  mutates the map in place, which corrupts header hashes used in cache keys.
- [ ] **Subscription cleanup immediate.**
  Register `t.Cleanup(closeFn)` immediately after creating a subscription/setup —
  a `t.Fatal`/`require` triggers `runtime.Goexit` and skips later explicit closes.
- [ ] **No new shared helpers** in `execution/engine/` without explicit approval.
  The one sanctioned shared-helper file is `federation_caching_helpers_test.go`
  (the fake cache, call tracker, gateway builder, snapshot normalizer).

### 2.4 The acceptance-criteria sync rule (hard requirement)

Whenever you add or modify a caching test you **must** update
`docs/entity-caching/ENTITY_CACHING_ACCEPTANCE_CRITERIA.md`:
every acceptance criterion links to its covering tests with a relative path,
a line number, and the test name;
every new test links back to its AC; every new AC needs at least one test link.
Treat the AC doc as **the index of which test proves which behavior**.
The `@requestScoped` block AC-RS-01..07 must stay covered.
A PR that touches tests but not the AC doc is incomplete.

---

## 3. Unit-tier taxonomy (what file proves what)

The reference suite groups unit tests by concern.
The re-implementation keeps these groupings but slices each into per-PR files.
For each group below: the **concern**, the **reference file** to copy patterns
from, and the **key behaviors** it must prove.

### 3.1 Cache-key rendering

Reference: `cache_key_test.go`, `cache_key_parity_test.go`,
`l2_cache_key_interceptor_test.go`.

Proves the `CacheKeyTemplate.RenderCacheKeys` contract end to end:

- `RootQueryCacheKeyTemplate` renders `{"__typename":"Query","field":F,"args":{...}}`
  for no-args, single arg, multiple args, string/bool/array/object/null/missing
  arg shapes.
- `EntityQueryCacheKeyTemplate` renders `{"__typename":T,"key":{...}}` for single
  key, composite key, array key fields, nested key fields, with prefix.
- `DerivedEntityCacheKey` via `EntityKeyMappings`: simple ID, integer→string
  coercion (an int arg and a string arg that name the same value produce
  identical keys), nested object path, deep path, array-index path (empty/null →
  skip caching, zero keys produced), multiple key fields, dot-notation merging
  into one object, multiple mappings → multiple keys, partial-missing skips only
  that one key, prefix, variable remapping.
- Read/write/invalidation **key parity** — the same logical entity yields the
  same key on the read path, the write path, and the invalidation path.
- `L2CacheKeyInterceptor` transform (e.g. tenant isolation) applied after the
  header-hash prefix.

Assertion shape: build the template struct directly, call
`tmpl.RenderCacheKeys(arenaOrNil, ctx, []*astjson.Value{data}, prefix)`,
and `assert.Equal` the full `[]*CacheKey` slice — including the `Item` pointer.
The number-coercion case is table-driven; assert the full `Keys` slice per row.

### 3.2 L1 cache

Reference: `l1_cache_test.go`, `l1_cache_normalize_test.go`,
`l1_l2_cache_e2e_test.go` (in-package end-to-end with a mock `DataSource`).

Proves: dedup (same entity fetched twice → one fetch), partial-loading
(`EnablePartialCacheLoad` true and false), L1-only partial, nested entities,
`UseL1Cache`-disabled gate; plus the normalize machinery — alias validation,
projected copy, `ComputeHasAliases`, arg-suffix, `mergeEntityFields`,
`validateItemHasRequiredData`.

### 3.3 Loader internals

Reference: `loader_cache_test.go`, `loader_cache_merge_test.go`,
`loader_cache_phase2_test.go`, `loader_cache_transform_test.go`,
`cache_load_test.go`, `cache_utility_coverage_test.go`.

Proves the phase machinery and the StructuralCopy transform helpers
(`structuralCopyNormalized` / `Denormalized` and their passthrough variants)
in isolation.

### 3.4 Batch entity caching

Reference: `batch_entity_cache_test.go`.

Proves: all-miss → all-hit, partial-hit (only missing entities refetched),
multi-candidate projection merge, negative hit, analytics accounting,
mutation-skip, tracing, L2-disabled, interceptor interaction.

### 3.5 Negative cache

Reference: `negative_cache_test.go`.

Proves: null-sentinel store / serve / TTL / mutation-interaction /
overwrite-after-expiry lifecycle, plus nullable-field regression guards.

### 3.6 Mutation cache

Reference: `mutation_cache_test.go`.

Proves the helpers `navigateProvidesDataToField`, `buildEntityKeyValue`,
`buildMutationEntityCacheKey`, `detectMutationEntityImpact`, and the TTL override.

### 3.7 @requestScoped coordinate L1

Reference: `request_scoped_test.go`.

Proves: injection (no-hints → false; missing L1 key → false; field-widening
check rejects a narrow cached value against a wider `ProvidesData`;
collect-then-inject all-or-nothing; L1-gating on `EnableL1Cache`), export
(copy-on-export via `structuralCopyNormalized`), round-trip, copy-independence
(mutate the response, assert L1 intact), alias handling
(`{subgraph}.{key}` is alias-independent, stored value normalized to schema
names, denormalized read re-applies the query alias), `ProvidesData` shapes,
synthetic alias, GC-survival and arena-residency
(a reflection helper proving injected values live on the request arena under
`debug.SetGCPercent(1)` plus heap churn).

### 3.8 Extensions invalidation

Reference: `extensions_cache_invalidation_test.go`.

Proves: a subgraph that returns `extensions.cacheInvalidation.keys` triggers a
`Delete`, with the skip-delete-if-being-written optimization.

### 3.9 Cache analytics

Reference: `cache_analytics_test.go` (very large — split per concern in the
re-implementation, see [§9](#9-known-risks-and-anti-patterns)).

Proves: collector record/merge, field hashing, entity counts, derived metrics
(`L1HitRate`, `L2HitRate`, `CachedBytesServed`), shadow freshness, snapshot
independence.

### 3.10 Copy invariants (the load-bearing four)

Reference: `loader_cache_copy_invariant_test.go`.

Exactly four adversarial tests, one per StructuralCopy merge site:

- `TestCopyInvariant_MergeBatchCacheHit`
- `TestCopyInvariant_MergeBatchPartialResponse`
- `TestCopyInvariant_MergeResultCacheSkipFetch`
- `TestCopyInvariant_MergeResultPartialCache`

Each merges, then mutates a nested container under the merged value
(e.g. `mergedValue.Get("profile")`), then asserts the cache entry's `FromCache`
stays intact.
Removing any one of the four StructuralCopies corrupts the nested container and
the test fails.
These four pair 1:1 with the four `BenchmarkMerge*` benches and with the
**Copy Budget** table in `v2/pkg/engine/resolve/CLAUDE.md`.
See [§7](#7-the-copy-budget-triangle).

---

## 4. E2E-tier taxonomy (what file proves what)

Reference files live in `execution/engine/`.
The shared plumbing lives in `federation_caching_helpers_test.go` (the one
sanctioned shared-helper file).

| Concern | Reference file | Proves |
|---|---|---|
| Basics | `federation_caching_test.go` | miss-then-hit, mutation-skips-L2-read, plan-time typename |
| L1 | `federation_caching_l1_test.go` | HTTP-call reduction, field accumulation w/ aliases, 3-fetch accumulation, interface/union, self-referential, child entity list, nested list dedup, root-field entity-list population, union-of-provider-fields, entity-union optimization |
| L2 | `federation_caching_l2_test.go` | L2-only, L1+L2 combined, partial entity fetch, root-field caching, error-skips-cache, mutation invalidation |
| Batch | `federation_caching_batch_test.go` | batch all-miss/all-hit/partial-hit through the gateway |
| Root args | `federation_caching_root_args_test.go`, `federation_caching_root_entity_test.go`, `federation_caching_root_split_test.go` | root-field caching variants + L1 promotion short-circuit |
| Entity field args | `federation_caching_entity_field_args_test.go` | arg-aware xxhash suffix keys |
| Extensions | `federation_caching_ext_invalidation_test.go` | extension-driven `Delete` + analytics |
| Analytics | `federation_caching_analytics_test.go` | full-snapshot assertions (split per concern in re-impl) |
| Trace | `federation_caching_trace_test.go` | cache trace UI fields (L1Hit/L1Miss, LoadSkipped) |
| Source | `federation_caching_source_test.go` | `FieldSource` tracking |
| Remap vars | `federation_caching_remap_variables_test.go` | per-request variable remapping in cache keys |
| Subscription | `federation_subscription_caching_test.go` | populate/invalidate via `NewManualFederationSetup` + product subscription `Emit()` |
| Partial | `partial_cache_test.go` | partial cache loading through the gateway |
| @requestScoped widening | `request_scoped_widening_e2e_test.go` | documented EXCEPTION — package `engine`, top-level recorder helpers |
| @requestScoped | `federation_caching_request_scoped_test.go` | currently `t.Skip` pending planner work |

### 4.1 The canonical E2E shape

Every E2E caching subtest follows the same skeleton.
Inline everything.
Request 1: drive a query, then `cache.GetLog()` and `assert.Equal` the full log
(get-miss + set per entity type) **and** assert subgraph call counts equal 1,
**and** assert the full response body.
`cache.ClearLog()`.
Request 2: drive the same query, assert the full all-hit get log, assert call
counts equal 0, assert the identical response body.
Always pair `ClearLog → GetLog + assert` around each request.

### 4.2 E2E infrastructure (in `federation_caching_helpers_test.go`)

Carry these forward verbatim — they are the seam every test plugs into.

- `FakeLoaderCache` — implements `resolve.LoaderCache`; records a
  `[]CacheLogEntry{Operation, Items: []CacheLogItem{Key, Hit, TTL}}` log;
  `ClearLog` / `GetLog` / `Peek` (no-log read); `WaitForOperation` (channel for
  async subscription assertions); `setCurrentTime` (deterministic TTL); copies
  bytes in and out to prevent aliasing.
  `CacheOperation` ∈ `{"get","set","delete"}`.
- `subgraphCallTracker` — an `http.RoundTripper` wrapper counting requests per
  host; `GetCount(host)` / `Reset` / `GetCounts`.
- `addCachingGateway(opts...)` — functional-options builder
  (`withCachingLoaderCache`, `withHTTPClient`, `withCachingOptionsFunc`,
  `withSubgraphEntityCachingConfigs`, `withDebugMode`, `withResolverOptions`,
  `withRemapVariables`, ...) wrapping `federationtesting.NewFederationSetup`.
- `parseCacheAnalytics` — reads the `X-Cache-Analytics` response header into a
  `CacheAnalyticsSnapshot`.
- `normalizeSnapshot` — sorts all event slices, zeros non-deterministic
  `CacheAgeMs` / `DurationMs`, nulls `FetchTimings`, collapses empty slices to
  nil. `normalizeFetchTimings` preserves fields but zeros `DurationMs`.
  `sortCacheLogEntries` for non-deterministic key order.
- `typenameStrippingTransport` — simulates a non-compliant subgraph.
- Two `SubgraphHeadersBuilder` mocks — a manual-hash mock and a real
  header-forwarding mock (the latter `.Clone()`s its headers).

### 4.3 Federation test services

`accounts`, `products`, `reviews` live under `execution/federationtesting/`.
The standard entity graph used across all E2E caching tests:
accounts owns `Query.me → User{id:1234, username:Me}`;
reviews extends `User` with `reviews[]` and `Product` with `reviews[]`;
products owns `Query.topProducts → Product{upc: top-1 Trilby, top-2 Fedora}`.
Canonical cache keys you will see in assertions:
`{"__typename":"Query","field":"topProducts"}`,
`{"__typename":"Product","key":{"upc":"top-1"}}`,
`{"__typename":"User","key":{"id":"1234"}}`.
Query tests use `NewFederationSetup`;
subscription tests use `NewManualFederationSetup` plus
`setup.NextProductSubscription(ctx).Emit()` to drive deterministic events.

---

## 5. Per-directive test matrix (what each PR must prove)

Each directive ships as a PR (see
[03-PR-PLAN-graphql-go-tools](./03-PR-PLAN-graphql-go-tools.md) and the
per-directive specs under `directives/`).
For each directive: the unit tests it must add (and the reference file to mirror),
the E2E tests it must add, and the acceptance-criteria range it satisfies.
A directive PR is not done until **both** tiers pass and the AC doc links them.

### 5.1 `@key` entity caching (L1 + L2)

Specs: [directives/key.md](./directives/key.md),
[adr/0002-key-entity-caching.md](./adr/0002-key-entity-caching.md).

**Unit** (mirror `cache_key_test.go`, `l1_cache_test.go`,
`batch_entity_cache_test.go`):

- `EntityQueryCacheKeyTemplate` table: single key, composite key, array/nested
  key fields, prefix — assert the full `{"__typename":T,"key":{...}}` string.
- L1 dedup: same entity twice → one fetch; field accumulation across fetches;
  `UseL1Cache`-disabled gate.
- Batch: all-miss → all-hit; partial-hit (only missing entities refetched);
  multi-candidate projection merge.

**E2E** (mirror `federation_caching_l2_test.go`):
request 1 asserts the full cache log (get-miss + set per entity type) **and**
subgraph call counts = 1; request 2 asserts the all-hit get log **and** call
counts = 0; both requests assert the identical full response body.
Opt-in via
`SubgraphCachingConfigs{EntityCaching: EntityCacheConfigurations{{TypeName, CacheName, TTL}}}`.

### 5.2 Root-field caching + `EntityKeyMappings`

Specs: [directives/root-field-caching.md](./directives/root-field-caching.md),
[adr/0003-root-field-caching.md](./adr/0003-root-field-caching.md).

**Unit** (mirror the `TestDerivedEntityCacheKey` table in `cache_key_test.go`):
`RootQueryCacheKeyTemplate` for no-args / single / multiple / string / bool /
array / object / null / missing args, plus `EntityKeyMappings` derivation —
simple ID, integer→string coercion, nested object path, deep path,
array-index path (empty/null → skip caching, zero keys), multiple key fields,
dot-notation merge, multiple mappings → multiple keys, partial-missing skips
that one key only, prefix, variable remapping.
Assert the full `Keys` slice per case.

**E2E** (mirror `federation_caching_root_args_test.go` /
`_root_entity_test.go` / `_root_split_test.go`):
assert the root-field L2 get/set log; assert that root-field L1 promotion lets a
later entity fetch short-circuit (call count drops);
smart-backfill — assert the requested-vs-rendered key write decisions via
`cacheKeysToExactRootFieldEntityEntries` on a value mismatch.

### 5.3 `@requestScoped` coordinate L1

Specs: [directives/request-scoped.md](./directives/request-scoped.md),
[adr/0004-request-scoped.md](./adr/0004-request-scoped.md).
AC range: **AC-RS-01..07**.

**Unit** (mirror `request_scoped_test.go`):
`TestTryRequestScopedInjection` (no-hints → false; missing L1 key → false;
field-widening check; collect-then-inject all-or-nothing; L1-gating);
`TestExportRequestScopedFields` (copy-on-export);
`TestRequestScopedRoundTrip`;
`TestExportedValuesAreIndependentCopies` (mutate response → L1 intact);
`TestRequestScopedAliasHandling` + `TestRequestScopedProvidesDataShapes` +
synthetic-alias;
GC-survival + arena-residency.
Build the `RequestScopedField{FieldName, FieldPath, L1Key:"{subgraph}.{key}", ProvidesData:*Object}`
inline.

**E2E** (mirror `federation_caching_request_scoped_test.go` —
currently `t.Skip`):
assert that symmetric dedup reduces subgraph call counts with **exact**
counts.
Do NOT copy the skipped tests' temporary fuzzy `if reviewsCalls == 0` smoke
checks — those violate the exact-assertion rule.
The sanctioned style exception `request_scoped_widening_e2e_test.go`
(package `engine`, top-level recorder helpers) stays as the one documented
exception; do not replicate its style elsewhere.

> **Open question for this PR** (see [§10](#10-open-questions)):
> are the skipped E2E tests un-skipped now (planner work landed) so AC-RS-01..07
> get real E2E coverage, or do they stay skipped with unit coverage only?

### 5.4 Mutation invalidation

Specs: [directives/mutation-invalidation.md](./directives/mutation-invalidation.md).

**Unit** (mirror `mutation_cache_test.go`): `navigateProvidesDataToField`,
`buildEntityKeyValue`, `buildMutationEntityCacheKey`,
`detectMutationEntityImpact`, TTL override.
**E2E** (mirror `federation_caching_test.go` `MutationSkipsL2Read` +
`federation_caching_l2_test.go` `MutationInvalidation`):
assert a `Delete` log entry for the impacted entity key, with the
skip-delete-if-being-written optimization.

### 5.5 Negative caching

Specs: [directives/negative-cache.md](./directives/negative-cache.md).

**Unit** (mirror `negative_cache_test.go`): null-sentinel store/serve/TTL/
mutation-interaction/overwrite-after-expiry + nullable-field regression guards.

### 5.6 Subscription entity caching

Specs: [directives/subscription-caching.md](./directives/subscription-caching.md),
`docs/entity-caching/SUBSCRIPTION_CACHE_SPEC.md`.

**E2E** (mirror `federation_subscription_caching_test.go`):
`NewManualFederationSetup` + product subscription `Emit()`;
populate-mode writes the entity to L2 across events;
invalidate-mode deletes;
`t.Cleanup(closeFn)` registered immediately.
Note the invariant: `SubscriptionEntityPopulationConfiguration` requires BOTH
`TypeName` and `FieldName` set, else the lookup silently no-ops.

### 5.7 Extensions invalidation

Specs: [directives/extensions-invalidation.md](./directives/extensions-invalidation.md).

**Unit** (mirror `extensions_cache_invalidation_test.go`) + **E2E**
(mirror `federation_caching_ext_invalidation_test.go`):
subgraph returns `extensions.cacheInvalidation.keys` → assert `Delete` +
analytics event.

### 5.8 Shadow mode

Specs: [directives/shadow-mode.md](./directives/shadow-mode.md).

**Unit** (mirror the shadow tests in `cache_analytics_test.go` —
`FieldSourceShadowCached`, `ShadowComparisonEvent`, `ShadowFreshnessRate`):
assert `Shadow:true` on read events, assert the freshness rate, assert cached
data is **never served** (fresh data always rendered).

### 5.9 Cache analytics (cross-cutting)

Specs: [directives/analytics.md](./directives/analytics.md).

Full `normalizeSnapshot(CacheAnalyticsSnapshot{...})` assertions with a per-event
"why" comment, covering `L1Reads`/`L2Reads` (`CacheKeyEvent`),
`L1Writes`/`L2Writes` (`CacheWriteEvent` with `ByteSize` + `TTL` + `CacheLevel`),
`EntityTypes`, `FieldHashes`, `MutationEvents`, `ShadowComparisons`.

---

## 6. Benchmark suite

Benchmarks are a **deliberate ladder**: each rung isolates one cost so a
regression points at one layer.
All benchmarks use `b.ReportAllocs()`, `b.ResetTimer()`, the `for b.Loop()`
form, and `arena.Reset()` per iteration.
Reference files all end in `_bench_test.go` under `v2/pkg/engine/resolve/`.

### 6.1 The overhead ladder — `caching_overhead_bench_test.go`

Drives the full `Loader.LoadGraphQLResponseData` over a realistic `topProducts`
root → batch-entity tree.
Shared `benchDataSource` (fixed bytes) and `benchCache` (zero-latency RWMutex
map).
Three top-level benchmarks, each with the named sub-benchmarks below.

`BenchmarkCachingOverhead_Sequential` sub-rungs (`b.Run` names):

| Sub-bench | Measures |
|---|---|
| `Disabled` | True zero baseline — no template at all |
| `ConfiguredButDisabled` | Template SET but flags off — **catches guard leaks** (if this is slower than `Disabled`, a guard is leaking) |
| `L1Only` | L1 enabled, L2 off |
| `L1L2_Miss` | Both enabled, cold cache (every entity misses) |
| `L1L2_Hit` | Both enabled, warm cache (every entity hits) |

`BenchmarkCachingOverhead_Parallel` — the same 5-rung ladder over the 4-phase
parallel path (root → 2 parallel batch fetches, 5 products + 5 reviews).
Measures the goroutine / main-thread split overhead under caching.

`BenchmarkCachingOverhead_Analytics` — `AnalyticsOff` vs `AnalyticsOn` over an
L1+L2 hit.
The delta is the cost of `EnableCacheAnalytics` (per-entity events, field
hashing, timings).

### 6.2 Isolated copy primitives — `structural_copy_bench_test.go`

Eight benchmarks isolating each cache-flow StructuralCopy primitive, each in a
`_NoTransform` and a `_WithTransform` variant over a fixed 10-field aliased
`Product` payload:

`BenchmarkStructuralCopy_L1Write_NoTransform` / `_WithTransform`,
`BenchmarkStructuralCopy_L1Read_NoTransform` / `_WithTransform`,
`BenchmarkStructuralCopy_L2Read_NoTransform` / `_WithTransform`,
`BenchmarkStructuralCopy_L2Write_NoTransform` / `_WithTransform`.

The `WithTransform` delta is the alias/arg-normalization cost over a plain
structural copy.

### 6.3 The Copy-Budget pair — merge benches + non-caching floor

`loader_cache_copy_bench_test.go` — the four merge-site benches, entity counts
`{1, 10, 100}`, one per StructuralCopy merge site:

- `BenchmarkMergeBatchCacheHit`
- `BenchmarkMergeBatchPartialResponse`
- `BenchmarkMergeResultCacheSkipFetch`
- `BenchmarkMergeResultPartialCache`

`loader_noncaching_bench_test.go` — the **zero-copy floor**, entity counts
`{1, 10, 100}`:

- `BenchmarkNonCachingParseMergeCore` (raw `ParseBytesWithArena` +
  `MergeValuesWithPath`)
- `BenchmarkNonCachingMergeResult` (full `mergeResult` with caching disabled)

The non-caching floor is the baseline against which each merge bench's extra
StructuralCopy is measured.
These pair 1:1 with the four copy-invariant tests and the Copy Budget table —
see [§7](#7-the-copy-budget-triangle).

### 6.4 Cache-hit fast path — `entity_cache_hit_bench_test.go`

`BenchmarkEntityCacheHitPath` — nested `{L1, L2} × {tracing off, on} ×
entity counts {1, 32}` over deeply-nested `Article` entities.
The cache is pre-populated; a miss is a `b.Fatal` (the bench must measure a
complete hit, never a partial).

### 6.5 Analytics micro-benches — in `cache_analytics_test.go`

`BenchmarkCacheAnalytics_Disabled`, `BenchmarkCacheAnalytics_Enabled`,
`BenchmarkFieldHashing` — micro-benches for the collector and the xxhash field
hashing.

### 6.6 Arena model justification — `arena_thread_safety_bench_test.go`

`BenchmarkConcurrentArena` vs `BenchmarkPerGoroutineArena` — justifies the
main-thread-only arena model versus per-goroutine arenas.
This is a **one-time justification bench**, not a per-PR regression gate
(see [§6.8](#68-which-benches-are-regression-gates)).

### 6.7 How to run and how to compare before/after

Run targeted unit tests:

```sh
go test -run TestL1Cache ./v2/pkg/engine/resolve/... -v
go test -run TestFederationCaching ./execution/engine/... -v
go test -race ./v2/pkg/engine/resolve/...
```

Run a single benchmark family:

```sh
go test -run=^$ -bench BenchmarkCachingOverhead -benchmem ./v2/pkg/engine/resolve/...
```

Compare before/after a change — capture two runs and diff with `benchstat`:

```sh
# on the base branch
go test -run=^$ -bench 'BenchmarkCachingOverhead|BenchmarkMerge|BenchmarkNonCaching' \
  -benchmem -count=10 ./v2/pkg/engine/resolve/... | tee /tmp/before.txt
# on the change branch
go test -run=^$ -bench 'BenchmarkCachingOverhead|BenchmarkMerge|BenchmarkNonCaching' \
  -benchmem -count=10 ./v2/pkg/engine/resolve/... | tee /tmp/after.txt
benchstat /tmp/before.txt /tmp/after.txt
```

Use `-count=10` so `benchstat` can compute variance and flag a real
regression versus noise.
The two rungs to watch hardest:
`ConfiguredButDisabled` must stay at parity with `Disabled` (any gap is a guard
leak), and each `BenchmarkMerge*` must stay within one StructuralCopy of the
matching `BenchmarkNonCaching*` floor.

### 6.8 Which benches are regression gates

Carry these forward as **load-bearing regression gates** (referenced by
`CLAUDE.md`):

- the `BenchmarkCachingOverhead_*` ladder (guard-leak + per-layer overhead);
- the four `BenchmarkMerge*` + the two `BenchmarkNonCaching*` (the Copy Budget).

Treat these as **one-time justification benches** (run once when the model is
chosen, not gated per PR):

- `arena_thread_safety_bench_test.go`;
- the `structural_copy_bench_test.go` micro-benches.

---

## 7. The Copy-Budget triangle

This is the single most important structural rule in the test suite.

The **Copy Budget table** in `v2/pkg/engine/resolve/CLAUDE.md` records the
minimum StructuralCopy count for each data flow.
It is the contract between three artifacts that must always move together:

1. the **table** (the documented budget);
2. the four `TestCopyInvariant_*` tests (prove each copy is necessary by removing
   it and corrupting a nested container);
3. the four `BenchmarkMerge*` benches plus the two `BenchmarkNonCaching*` floor
   benches (measure the cost of each copy against the zero-copy baseline).

The merge sites the table pins (loader.go line references in the reference tree,
for orientation only): batch L2 cache-hit splice (`mergeBatchCacheHit`, ~1220),
partial batch response interleave (`mergeBatchPartialResponse`, ~1372),
full L1 cache-hit merge (`mergeResult` cacheSkipFetch, ~1472),
partial-cache L1 merge (`mergeResult` partialCache, ~1491).

**Rule:** any PR that changes the copy budget updates the table, the invariant
tests, and the benches together — in one reviewable PR per merge site.
The recommendation for the re-implementation: keep this triangle as a single PR
unit so a reviewer sees the table change, the test that proves it, and the bench
that measures it side by side.

---

## 8. Per-PR test deliverables (the working contract)

A directive PR in this stack is **done** only when all of the following hold.
Use this as the PR author's and the reviewer's shared checklist.

- [ ] Unit tests for the directive added, mirroring the reference file named in
  [§5](#5-per-directive-test-matrix), passing under `go test` **and**
  `go test -race`.
- [ ] E2E tests for the directive added (where the matrix calls for them),
  self-contained, inline GraphQL, full normalized snapshot assertions.
- [ ] If the PR touches a StructuralCopy merge site: the Copy-Budget triangle
  (table + invariant test + bench) updated together — see [§7](#7-the-copy-budget-triangle).
- [ ] No new shared helpers in `execution/engine/` (the no-shared-helper rule).
- [ ] `docs/entity-caching/ENTITY_CACHING_ACCEPTANCE_CRITERIA.md` updated:
  every new/changed test linked from its AC with relative path + line + name.
- [ ] Every convention in the [§2 checklist](#2-mandatory-conventions--the-checklist)
  satisfied — especially exact assertions, full-struct asserts, "why" comments,
  and the cache-log clear+assert pairing.
- [ ] Modern Go throughout (`/use-modern-go` loaded before writing).

---

## 9. Known risks and anti-patterns

These are concrete traps surfaced by the reference suite.
Avoid them in the re-implementation.

- **Legacy `.query` file loading.**
  Some older E2E tests load queries via `cachingTestQueryPath("queries/*.query")`
  from `federationtesting/testdata` (e.g. parts of
  `federation_caching_l2_test.go`).
  This contradicts the inline-queries convention.
  Standardize on inline `QueryStringWithHeaders` and do not carry the
  file-loading helper forward.
  (See [§10](#10-open-questions) — confirm no query is too large to inline.)
- **The two skipped `@requestScoped` E2E tests** in
  `federation_caching_request_scoped_test.go` contain temporary fuzzy
  `if reviewsCalls == 0` smoke checks explicitly flagged as placeholder.
  They violate the exact-assertion rule.
  When re-enabled, replace with exact call-count assertions.
  Never copy the fuzzy pattern.
- **Giant analytics files.**
  `cache_analytics_test.go` (~2090 lines) and
  `federation_caching_analytics_test.go` (~120 KB) are correct per convention but
  hard to review as one PR.
  In the re-implementation, split analytics tests per concern — collector unit,
  L1 integration, L2 integration, shadow, mutation events — into separate small
  files.
- **Centralize snapshot normalization.**
  `normalizeSnapshot` zeros `CacheAgeMs` / `DurationMs` and nulls `FetchTimings` —
  this is the **one sanctioned exception** to "assert exact" (these fields are
  non-deterministic).
  Keep normalization in a single helper so reviewers know exactly which fields
  are intentionally not asserted.
  Use `normalizeFetchTimings` only when timing-structure assertions are actually
  needed.
- **`request_scoped_widening_e2e_test.go` deliberately breaks conventions**
  (top-level recorder helpers, package `engine` not `engine_test`).
  It is the ONE documented exception.
  Any new request-scoped E2E that copies its style without that documented
  exemption is a regression.
- **Forgetting `DisableSubgraphRequestDeduplication = true`** in unit tests
  silently changes fetch counts and produces confusing failures.
  Bake it into a shared in-package setup helper (allowed in `resolve`, the
  no-shared-helper rule is E2E-only) or document it prominently at the top of
  each unit test file.

---

## 10. Open questions

These must be resolved before the corresponding PRs land.

- **Legacy query files:** remove `cachingTestQueryPath` entirely, or keep it for
  the very largest existing queries?
  Convention says inline — confirm no query is too large to inline reasonably.
- **Setup unification:** keep both `NewFederationSetup` and
  `NewManualFederationSetup`, or unify?
  Subscription cache tests depend on the manual trigger (`Emit`), so the manual
  path cannot be dropped.
- **`@requestScoped` E2E coverage:** are the skipped E2E tests un-skipped as part
  of this re-implementation (planner work landed), giving AC-RS-01..07 real E2E
  coverage now — or do they stay skipped with unit-only coverage?
- **Regression-gate set:** confirm the load-bearing gate set is exactly the
  `CachingOverhead` ladder + the Copy-Budget benches, and that
  `arena_thread_safety` and `structural_copy` micro-benches are one-time
  justification benches rather than per-PR gates (see [§6.8](#68-which-benches-are-regression-gates)).

---

## 11. Quick reference — commands

```sh
# unit, one family
go test -run TestL1Cache       ./v2/pkg/engine/resolve/... -v
go test -run TestCopyInvariant ./v2/pkg/engine/resolve/... -v

# e2e, one family
go test -run TestFederationCaching ./execution/engine/... -v

# race (always before merge)
go test -race ./v2/pkg/engine/resolve/...
go test -race ./execution/engine/...

# benchmarks, one family with allocs
go test -run=^$ -bench BenchmarkCachingOverhead -benchmem ./v2/pkg/engine/resolve/...

# before/after comparison (see §6.7)
go test -run=^$ -bench 'BenchmarkCachingOverhead|BenchmarkMerge|BenchmarkNonCaching' \
  -benchmem -count=10 ./v2/pkg/engine/resolve/... | tee /tmp/after.txt
benchstat /tmp/before.txt /tmp/after.txt
```

See [08-EXECUTION-RUNBOOK](./08-EXECUTION-RUNBOOK.md) for how these commands slot
into the Codex-driven implementation loop, and
[07-UNRELATED-FINDINGS](./07-UNRELATED-FINDINGS.md) for out-of-scope issues found
while mapping the suite.
