# 04 — Stacked PR Plan: Router (Cosmo)

This document defines the ordered stack of pull requests that integrate entity caching into the **Cosmo router**.

The router is a separate repository from `graphql-go-tools` (gqtools).
The router consumes gqtools as a Go module dependency, so every router PR that needs a new gqtools API must wait for a gqtools **release** (a published version tag) and then bump its `go.mod`.

Read this alongside:

- [00-OVERVIEW.md](00-OVERVIEW.md) — what entity caching is and why we are re-implementing it.
- [01-ARCHITECTURE-SPEC.md](01-ARCHITECTURE-SPEC.md) — the clean architecture and the integration seam between gqtools and the router.
- [03-PR-PLAN-graphql-go-tools.md](03-PR-PLAN-graphql-go-tools.md) — the **upstream** stack.
  Every router PR below depends on one or more gqtools PRs from that document.
- [05-ASTJSON-PRIMITIVES.md](05-ASTJSON-PRIMITIVES.md) — the astjson primitives the engine relies on (router never calls these directly).
- [06-TEST-AND-BENCH-PLAN.md](06-TEST-AND-BENCH-PLAN.md) — the cross-cutting test and benchmark plan.
- [07-UNRELATED-FINDINGS.md](07-UNRELATED-FINDINGS.md) — out-of-scope findings discovered while mapping the existing router PR (#2777).

---

## Background for a first-time reader

The existing router integration shipped as a single squashed PR (#2777) of roughly 96K lines across 310 files.
That is too large to review and too large to revert safely.
This plan breaks the same surface into a stack of small, independently reviewable PRs.

**What "entity caching" means for the router.**
The router builds the federation execution plan, runs requests through the gqtools resolve engine, and exposes configuration and observability.
The caching logic itself (cache keys, L1, L2 read/write/merge, shadow comparison, analytics collection) lives entirely in the gqtools `resolve` engine.
The router's job is narrow and falls into four buckets:

1. **Declare what to cache.**
   Translate composition output (per-subgraph cache configuration) into the gqtools `plan` config structs, and pass them through the federation config factory.
2. **Provide the L2 backend.**
   Implement the `resolve.LoaderCache` interface (Redis, in-memory) and register named instances in the resolver.
3. **Toggle per request.**
   Set `ctx.ExecutionOptions.Caching` (an `resolve.CachingOptions` value) for each incoming request, honoring config and dev/debug headers.
4. **Observe.**
   Read `ctx.GetCacheStats()` after resolution and export the analytics snapshot to OTLP/Prometheus; optionally surface per-fetch cache traces in response extensions.

**The seam is deliberately thin.**
The router never implements cache-key rendering, never touches the arena or astjson, and never implements L1.
It only implements `LoaderCache` (a plain `Get`/`Set`/`Delete` over `[]string` keys and `[]*CacheEntry`) and fills declarative config structs.
Keeping the seam thin is what makes the router stack independent of engine internals — see [01-ARCHITECTURE-SPEC.md](01-ARCHITECTURE-SPEC.md).

---

## Release and dependency model

The router depends on **two** gqtools modules, each released and tagged independently:

- `github.com/wundergraph/graphql-go-tools/v2` — tagged `v2.x.y` (the core engine: `resolve`, `plan`).
- `github.com/wundergraph/graphql-go-tools/execution` — tagged `execution/v1.x.y` (the federation config factory: `engine.SubgraphCachingConfig`, `WithSubgraphEntityCachingConfigs`).

A router PR that needs a new gqtools API cannot merge until:

1. the corresponding gqtools PR has merged on `master`, and
2. a release has been cut (a new `v2.x.y` and/or `execution/v1.x.y` tag), and
3. the router PR bumps both modules in `go.mod` to that tag.

Throughout this document a dependency is written as:

> **Waits on gqtools release:** RG-x (gqtools PR x → release `v2.A.B` + `execution/v1.C.D`).

The exact version numbers are filled in at release time.
What matters is the **ordering**: a router PR must not be opened against an unreleased gqtools API.
Work may be *prototyped* against a local `replace` directive, but the `replace` must be removed and a real tag pinned before the router PR is marked ready.

See [03-PR-PLAN-graphql-go-tools.md](03-PR-PLAN-graphql-go-tools.md) for the upstream PR numbering (`G1`, `G2`, …) referenced below.

---

## Branch strategy

Create one fresh feature branch off `main` in the router repository:

```
feat/entity-caching
```

Each PR below targets the previous PR's branch (a true stack), not `main` directly, until the foundation lands.
Once the foundation (R1) merges to `main`, the remaining PRs may target `main` individually if they are independent, or remain stacked where one strictly needs another.
The dependency graph at the end of this document shows which are independent.

Naming convention for the stack branches:

```
feat/entity-caching            (R1, foundation)
feat/entity-caching-l1         (R2)
feat/entity-caching-l2-redis   (R3)
feat/entity-caching-l2-memory  (R4)
feat/entity-caching-breaker    (R5)
feat/entity-caching-headers    (R6)
feat/entity-caching-shadow     (R7)
feat/entity-caching-analytics  (R8)
feat/entity-caching-invalidate (R9)
feat/entity-caching-subs       (R10)
feat/entity-caching-docs       (R11)
```

---

## PR stack overview

| PR | Title | Waits on gqtools release | Independent after R1? |
|----|-------|--------------------------|------------------------|
| **R1** | Foundation: config schema + cache wiring (no backend) | RG-1 (foundation: `plan.*` configs, `resolve.LoaderCache`, `CachingOptions`) | — |
| **R2** | L1 per-request cache wiring | none beyond R1 | yes |
| **R3** | L2 Redis backend | none beyond R1 | yes |
| **R4** | L2 in-memory backend | none beyond R1 | yes |
| **R5** | Circuit breaker decorator | none (decorates R3/R4) | needs R3 or R4 |
| **R6** | Per-request cache-control headers | none beyond R1 | yes |
| **R7** | Shadow mode wiring + export | RG-7 (shadow comparison events) | yes |
| **R8** | Analytics export (OTLP / Prometheus) | RG-8 (analytics snapshot) | yes |
| **R9** | Mutation + extension-based invalidation | RG-9 (invalidation config + callbacks) | needs R3 or R4 |
| **R10** | Subscription cache populate/invalidate | RG-10 (subscription callbacks) | needs R3 or R4 |
| **R11** | Documentation + config reference | none | needs all |

Total: 11 PRs replacing the single 96K-line squash.
Each is independently reviewable in well under a day.

---

## R1 — Foundation: config schema + cache wiring (no backend)

**Title:** `feat(cache): entity caching foundation — config schema and wiring (no backend)`

**Goal.**
Stand up the entire configuration surface and all the wiring seams, with **no functional caching**.
After this PR, an operator can write entity-caching YAML and the router parses, validates, and threads it through to the engine — but because no backend is registered and the feature defaults to disabled, behavior is byte-for-byte identical to today.
This is the keystone every other router PR stacks on.

**Scope.**

- **YAML config schema.**
  Add `EntityCachingConfiguration` to the router config struct and the JSON schema (`config.schema.json`):
  - `Enabled` (default `false`).
  - `GlobalCacheKeyPrefix` (string, used for schema-versioning of keys).
  - `L1{ Enabled, MaxSize (default 100MB) }`.
  - `L2{ Enabled, Storage{ ProviderID, KeyPrefix (default cosmo_entity_cache) }, CircuitBreaker{ FailureThreshold (default 5), CooldownPeriod (default 10s) } }`.
  - `SubgraphCacheOverrides[]{ Name, StorageProviderID, Entities[]{ Type, StorageProviderID } }`.
  - Extend `StorageProviders` with `Redis{ ID, URLs, ClusterEnabled }` and `Memory{ ID, MaxSize }` blocks.
- **Provider-ID resolution seam.**
  Add `resolveEntityCacheProviderID` with the documented 3-tier precedence: entity override → subgraph override → `default`.
  This one function is shared between instance-build time and plan-config time so the naming can never drift; it is the single source of truth for which named cache a config points at.
- **Instance map build.**
  Add `Router.buildEntityCacheInstances()` returning `map[providerID]resolve.LoaderCache`.
  In R1 it returns an **empty (or nil-valued) map**: no backend types exist yet.
  Establish the contract that the map MUST contain a `"default"` key whenever any plan config references it.
- **Engine wiring.**
  In the executor build path, set `resolve.ResolverOptions.Caches` from the instance map and pass it to the single `resolve.New(...)` entry point.
  Wire the (empty) `EntityCacheConfigs` map placeholder.
- **Composition → plan translation.**
  In `factoryresolver.go`, translate the composition protobuf into `plan.FederationMetaData`:
  populate `EntityCacheConfiguration`, `RootFieldCacheConfiguration` (with `EntityKeyMappings`/`FieldMapping`), `MutationFieldCacheConfiguration`, `MutationCacheInvalidationConfiguration`, `SubscriptionEntityPopulationConfiguration`, and `RequestScopedField`.
  Build `engine.SubgraphCachingConfigs` and pass them via `engine.WithSubgraphEntityCachingConfigs(...)` to `NewFederationEngineConfigFactory`.
- **Per-request options plumbing (disabled).**
  Add the code path that constructs `resolve.CachingOptions` per request from `EntityCachingHandlerOptions`, but with all flags defaulting to `false`.
  No headers honored yet.
- **Lifecycle.**
  `Router.Shutdown` closes any cache instance that implements `io.Closer` (no-op while the map is empty).

**Exclusions (explicitly NOT in R1).**

- No Redis backend (R3).
- No in-memory backend (R4).
- No circuit breaker (R5).
- No header handling (R6).
- No analytics export (R8).
- No invalidation deletes wired to subgraph extensions (R9).
- No subscription callbacks (R10).
- Demo apps, benchmark harnesses, and regenerated protobuf files are split into their own non-blocking chore PRs (see [07-UNRELATED-FINDINGS.md](07-UNRELATED-FINDINGS.md)).

**Dependency on a gqtools PR (version bump).**

> **Waits on gqtools release:** RG-1 — the gqtools **foundation** PR (G1 in [03-PR-PLAN-graphql-go-tools.md](03-PR-PLAN-graphql-go-tools.md)).
> That PR must export: `plan.EntityCacheConfiguration` and the four sibling config types + their `FindBy*` collection methods; `engine.SubgraphCachingConfig` + `WithSubgraphEntityCachingConfigs`; `resolve.LoaderCache`, `resolve.CacheEntry`, `resolve.EntityCacheInvalidationConfig`; `resolve.CachingOptions` and `ctx.ExecutionOptions.Caching`; and `resolve.ResolverOptions.Caches`/`EntityCacheConfigs`.
> Bump **both** `v2` and `execution` modules to the tag that includes RG-1.

**Critical contract notes (decided once, here).**
The clean re-implementation MUST standardize on the **current code** signatures, not the stale doc:

- `LoaderCache.Set(ctx, entries []*resolve.CacheEntry) error` — **no** `ttl` parameter; TTL is per-entry on `CacheEntry.TTL`.
  Zero = backend default, negative = indefinite.
- `CacheEntry` has fields `Key`, `Value []byte`, `TTL`, `RemainingTTL`, `WriteReason`.
- The router never implements `CacheKeyTemplate` and never sees `arena.Arena` / `astjson` — those stay engine-internal.
  See the doc-drift discussion in [01-ARCHITECTURE-SPEC.md](01-ARCHITECTURE-SPEC.md).

**Acceptance criteria.**

1. With entity caching absent from YAML, the router boots and serves requests with **zero** behavioral change (regression suite green).
2. With `entity_caching.enabled: true` but no storage provider configured, the router boots, L2 stays disabled, and a single clear warning is logged.
3. JSON-schema validation rejects an `L2` block that names a non-existent `storage.provider_id`.
4. `resolveEntityCacheProviderID` unit tests cover all three precedence tiers plus the missing-`default` edge case.
5. Composition fixtures translate to the expected `plan.*` structs — assert the **entire** `FederationMetaData` cache slices with `assert.Equal` against inline expected values (per the repo's exact-assertion rule).
6. `go vet` and the type checker pass; `go.mod` pins the RG-1 release tags with no `replace` directive.

**Reviewer-guide doc.**
`docs/entity-caching/reviewer-guides/R1-foundation.md` — explains the four router responsibilities, walks the config struct → JSON schema → plan translation path, shows the provider-ID precedence table, and states the "no backend, no behavior change" guarantee with the exact regression command to run.

---

## R2 — L1 per-request cache wiring

**Title:** `feat(cache): enable per-request L1 entity cache`

**Goal.**
Let an operator turn on the per-request in-memory L1 entity cache.
L1 lives entirely inside the engine; the router only flips the `EnableL1Cache` flag per request based on config.

**Scope.**

- Map `entity_caching.l1.enabled` → `resolve.CachingOptions.EnableL1Cache` in the per-request options builder.
- Document that L1 also gates `@requestScoped` coordinate L1 (shared flag).
- No new backend, no storage provider — L1 needs none.

**Exclusions.**

- No L2 (R3/R4).
- No header overrides (R6) — that PR will let dev requests toggle this flag.

**Dependency on a gqtools PR (version bump).**

> **Waits on gqtools release:** none beyond R1.
> `EnableL1Cache` ships in RG-1.
> If the L1 engine implementation is split into its own gqtools PR (G-L1), this router PR waits on **that** release instead; coordinate the exact tag with [03-PR-PLAN-graphql-go-tools.md](03-PR-PLAN-graphql-go-tools.md).

**Acceptance criteria.**

1. With `l1.enabled: true`, a single request that fetches the same entity twice issues **one** subgraph fetch — assert exact fetch count via the federation test-service tracker.
2. With `l1.enabled: false` (default), behavior matches R1 exactly.
3. L1 is per-request: two sequential requests do **not** share L1 state (assert two fetches across two requests).

**Reviewer-guide doc.**
`docs/entity-caching/reviewer-guides/R2-l1.md` — one paragraph: what L1 is (dedup within a request), why it needs no backend, the single flag it flips, and the `@requestScoped` shared-flag note.

---

## R3 — L2 Redis backend

**Title:** `feat(cache): Redis L2 entity cache backend`

**Goal.**
Ship a production `resolve.LoaderCache` backed by Redis, registered as a named instance, so entities cache across requests.

**Scope.**

- `router/pkg/entitycache`: `RedisEntityCache` implementing `resolve.LoaderCache`:
  - Constructor `NewRedisEntityCache(client redis.UniversalClient, keyPrefix string)`.
  - `Get` = `MGET` over `keyPrefix:key`; a nil reply maps to a **miss** (nil slot), and the returned slice MUST be the same length as the input keys.
  - `Set` = pipelined `SET` per entry with that entry's `CacheEntry.TTL`; clamp `TTL < 0` to `0` (Redis "no expiry"), `TTL == 0` uses backend default if configured.
  - `Delete` = `DEL`.
  - Thread-safe (Redis client is); document that the engine bulk-reads on the main thread today but the contract still requires concurrency safety.
- Build-path integration: when `l2.enabled` and a Redis storage provider is configured, `buildEntityCacheInstances` constructs a `RedisEntityCache` and registers it under the resolved provider ID **and** aliases it as `"default"` when it is the default provider.
- Per-request: map `l2.enabled` → `resolve.CachingOptions.EnableL2Cache`.
- `GlobalCacheKeyPrefix` from config flows into `CachingOptions.GlobalCacheKeyPrefix`; the backend's own `keyPrefix` is separate (storage namespacing vs. engine key transform).

**Exclusions.**

- No circuit breaker (R5).
- No in-memory backend (R4).
- No analytics on Redis errors yet (R8).
- No invalidation deletes from subgraph extensions (R9) — `Delete` exists but is only exercised by tests here.

**Dependency on a gqtools PR (version bump).**

> **Waits on gqtools release:** none beyond R1.
> `LoaderCache`/`CacheEntry` ship in RG-1; this PR only implements them.

**Risk to surface in the PR description.**
The engine's bulk L2 lookup fails the **whole batch** back to subgraph on a single `Get` error.
A raw Redis backend therefore turns one transient Redis blip into a full cache-miss for that batch.
This is the motivation for R5 (the breaker masks `Get` errors as a clean miss).
Call this out so reviewers understand R3 is intentionally "fail-open but noisy" until R5.

**Acceptance criteria.**

1. Round-trip test against a real Redis (testcontainers or miniredis): `Set` then `Get` returns the exact bytes and a positive `RemainingTTL`; `Get` of an unknown key returns a nil slot.
2. `Get` of N keys with M present returns a slice of length N with exactly the M present slots non-nil — assert the full slice.
3. `TTL < 0` results in a key with no expiry; `TTL > 0` results in the expected expiry (assert exact seconds).
4. End-to-end: two identical requests across the request boundary issue **one** subgraph fetch (second served from Redis) — assert exact tracker counts.
5. Key shape is exactly `keyPrefix:<engine-key>` — assert the full Redis key string.

**Reviewer-guide doc.**
`docs/entity-caching/reviewer-guides/R3-redis.md` — the three-method contract, the nil-slot miss convention, the TTL clamping table, the "one Get error fails the batch" risk and why R5 fixes it, and the exact key-shape example.

---

## R4 — L2 in-memory backend

**Title:** `feat(cache): in-memory L2 entity cache backend`

**Goal.**
Ship a single-node in-memory `resolve.LoaderCache` (ristretto) for deployments without Redis, and for tests.

**Scope.**

- `MemoryEntityCache` in `router/pkg/entitycache`:
  - Constructor `NewMemoryEntityCache(maxSizeBytes int64)`; `MaxCost = maxSizeBytes`.
  - `Set` via `SetWithTTL`; `Get` reads value + remaining TTL (`GetTTL`) into `CacheEntry.RemainingTTL`; `Delete` via `Del`.
  - Expose `Metrics()` and `MaxSizeBytes()` for the analytics PR to register a metric source.
  - Atomic `Len()`, `OnEvict` hook for eviction accounting.
  - Implements `io.Closer` so `Router.Shutdown` closes it.
- Build-path: register a `MemoryEntityCache` when the storage provider is `Memory`.

**Exclusions.**

- Same as R3: no breaker, no analytics export wiring (only the `Metrics()` getter), no invalidation flows.

**Dependency on a gqtools PR (version bump).**

> **Waits on gqtools release:** none beyond R1.

**Acceptance criteria.**

1. `Set`/`Get`/`Delete` round-trip returns exact bytes; eviction respects `MaxCost` (assert `Len` after overflowing the budget).
2. `RemainingTTL` is populated and decreases over time (assert exact value at write time, monotonic decrease at read time — normalize the timestamp, do not use a fuzzy comparison).
3. `Router.Shutdown` closes the instance (assert the `io.Closer` is invoked).
4. Same end-to-end cross-request single-fetch assertion as R3, using the in-memory backend.

**Reviewer-guide doc.**
`docs/entity-caching/reviewer-guides/R4-memory.md` — when to choose in-memory vs. Redis (single-node vs. shared), the ristretto cost model, and the `Metrics()`/`io.Closer` hooks R8 and Shutdown rely on.

---

## R5 — Circuit breaker decorator

**Title:** `feat(cache): circuit breaker for L2 cache backends`

**Goal.**
Wrap any `resolve.LoaderCache` so that repeated L2 failures stop hammering a sick backend and degrade to clean cache-misses instead of erroring the request batch.

**Scope.**

- `CircuitBreakerCache` decorator in `router/pkg/entitycache`:
  - States closed / open / half-open (atomic).
  - Opens after `FailureThreshold` **consecutive** failures.
  - While open: `Get` returns a slice of nil entries (a clean miss, **never** an error); `Set`/`Delete` are no-ops.
  - After `CooldownPeriod`, one half-open probe; success closes, failure re-opens.
  - Constructor `NewCircuitBreakerCache(cache resolve.LoaderCache, cfg{ Enabled, FailureThreshold, CooldownPeriod })`.
- Build-path: when `l2.circuit_breaker.enabled`, wrap the constructed backend (Redis or memory) before registering it.
- Config: `L2.CircuitBreaker{ FailureThreshold (default 5), CooldownPeriod (default 10s) }` from R1's schema.

**Why this matters.**
This directly addresses the R3 risk: by masking `Get` errors as clean misses, one Redis blip no longer fails the whole engine batch.
The decorator is the boundary where "fail-open" is made safe.

**Exclusions.**

- No analytics on breaker state transitions yet (R8 may add a counter).
- Decorates an existing backend only — no breaker without R3 or R4.

**Dependency on a gqtools PR (version bump).**

> **Waits on gqtools release:** none.
> The decorator only depends on `resolve.LoaderCache` from RG-1.
> **Depends on a sibling router PR:** R3 or R4 must be merged so there is a backend to wrap.

**Acceptance criteria.**

1. After `FailureThreshold` consecutive `Get` errors from a fake backend, the breaker is open and the next `Get` returns all-nil with **no** error — assert the full nil slice and a nil error.
2. While open, `Set` and `Delete` do not call the wrapped backend (assert the fake backend's call counters are unchanged).
3. After `CooldownPeriod`, exactly **one** probe reaches the wrapped backend; success transitions to closed (assert the state and that subsequent calls pass through).
4. A single failure below the threshold does **not** open the breaker (assert state still closed and call passed through).

**Reviewer-guide doc.**
`docs/entity-caching/reviewer-guides/R5-breaker.md` — the state machine diagram in words, the "errors become clean misses" guarantee, why it pairs with the R3 batch-failure risk, and the consecutive-vs-cumulative failure semantics.

---

## R6 — Per-request cache-control headers

**Title:** `feat(cache): per-request cache-control headers (dev/debug)`

**Goal.**
Allow per-request overrides of caching for debugging and the playground, gated so they cannot be abused in production.

**Scope.**

- Honor these request headers, mapping to `resolve.CachingOptions`:
  - `X-WG-Disable-Entity-Cache: true` → disable L1 **and** L2.
  - `X-WG-Disable-Entity-Cache-L1: true` → disable L1 only (also disables `@requestScoped` coordinate L1, shared flag).
  - `X-WG-Disable-Entity-Cache-L2: true` → disable L2 only.
  - `X-WG-Cache-Key-Prefix: <value>` → prepend to keys as `header:global` (header prefix in front of the configured `GlobalCacheKeyPrefix`).
- **Gate:** all of the above are honored **only** when `reqCtx.operation.traceOptions.Enable` is true (dev mode or a valid studio request token).
  In production without trace, the headers are ignored.
  Place the gate in `GraphQLHandler.cachingOptions` (`graphql_handler.go`).

**Exclusions.**

- No new config keys; this is purely a per-request override layer over R2/R3.

**Dependency on a gqtools PR (version bump).**

> **Waits on gqtools release:** none beyond R1.
> The flags and `GlobalCacheKeyPrefix` ship in RG-1.

**Risk to surface in the PR description.**
The trace gate is load-bearing.
Moving or weakening it turns these headers into a production DoS / cache-poisoning vector (any client could force-disable caching or shard the key space).
The reviewer guide and a regression test must lock the gate.

**Acceptance criteria.**

1. With trace **enabled**, each header has its exact documented effect — assert the resulting `CachingOptions` value (full struct) for each header.
2. With trace **disabled**, every header is ignored — assert `CachingOptions` equals the config-derived value unchanged.
3. `X-WG-Cache-Key-Prefix` composes as `header:global` (assert the exact resulting prefix string).

**Reviewer-guide doc.**
`docs/entity-caching/reviewer-guides/R6-headers.md` — the header table, the exact trace gate condition and where it lives, the prod-DoS rationale for the gate, and the prefix composition order.

---

## R7 — Shadow mode wiring + export

**Title:** `feat(cache): shadow mode wiring and staleness export`

**Goal.**
Let operators run L2 in "shadow" mode: read/write the cache but always serve fresh data and compare, to measure staleness before trusting the cache.
The comparison happens in the engine; the router only sets the flag and exports the staleness signal.

**Scope.**

- Translate `ShadowMode` from composition into the `plan.EntityCacheConfiguration.ShadowMode` field (already wired structurally in R1; R7 makes it functional end-to-end and adds the export).
- After resolution, read `ShadowComparisons` from `ctx.GetCacheStats()` and export `shadowStaleness` (fresh vs. stale counts) to observability.
  (This depends on the analytics read path; if R8 lands first, R7 reuses it; otherwise R7 ships the minimal snapshot read needed for staleness only.)

**Exclusions.**

- The router does **not** implement the fresh-vs-cached comparison — that is engine-side (RG-7).
- Full analytics export is R8; R7 exports staleness only.

**Dependency on a gqtools PR (version bump).**

> **Waits on gqtools release:** RG-7 — the gqtools shadow-mode PR (G-shadow) that serves fresh, performs the comparison, and emits `resolve.ShadowComparisonEvent` into the analytics snapshot.
> Bump to the tag that includes RG-7.

**Open question to resolve before coding (carry from findings).**
`NegativeCacheTTL` interaction with shadow mode is unspecified.
Confirm with the engine PR whether negative-cache sentinels participate in shadow comparison, and document the answer in the reviewer guide rather than guessing.

**Acceptance criteria.**

1. With `shadow_mode: true`, a cache **hit** still issues a subgraph fetch (fresh always served) — assert exact fetch count.
2. The served response equals the fresh subgraph response, never the cached bytes — assert the full response body.
3. `ShadowComparisons` are exported as `shadowStaleness` with the correct fresh/stale split for a fixture where cache and fresh differ — assert exact counts.

**Reviewer-guide doc.**
`docs/entity-caching/reviewer-guides/R7-shadow.md` — what shadow mode is for (de-risking a rollout), the "always serve fresh" guarantee, the single flag the router sets, what `ShadowComparisonEvent` carries, and the open `NegativeCacheTTL` question with its resolution.

---

## R8 — Analytics export (OTLP / Prometheus)

**Title:** `feat(cache): entity cache analytics export`

**Goal.**
Export the full cache analytics snapshot to OTLP and Prometheus so operators can see hit rates, bytes served, fetch timings, mutation impact, and errors.

**Scope.**

- Per request: set `resolve.CachingOptions.EnableCacheAnalytics` from config (`metrics.{otlp,prometheus}.entity_caching_stats`).
- After `ResolveGraphQLResponse`, call `ctx.GetCacheStats()` **exactly once** (it snapshots and releases the pooled collector).
- `EntityCacheMetrics.RecordSnapshot(resolve.CacheAnalyticsSnapshot)` maps snapshot fields to metrics:
  `L1Reads/L2Reads`, `L1Writes/L2Writes`, `FetchTimings`, `ShadowComparisons`, `MutationEvents`, `CacheOpErrors` → request/keys/latency/invalidations/populations/shadow-staleness/operation-error metrics.
- Register ristretto `Metrics()` (from R4's `MemoryEntityCache`) via a `cacheMetricSource` when configured.
- Gate the whole export on the metrics config flags; when disabled, `EnableCacheAnalytics` is false and `GetCacheStats()` returns empty with zero overhead.

**Exclusions.**

- No new analytics **types** invented router-side — consume only what the snapshot exposes.
- Do **not** call the low-level `CacheAnalyticsCollector.Record*`/`Merge*` methods; `GetCacheStats()` is the only sanctioned read path (those methods are flagged for possible internalization upstream — see open questions).

**Dependency on a gqtools PR (version bump).**

> **Waits on gqtools release:** RG-8 — the gqtools analytics PR (G-analytics) exporting `CacheAnalyticsSnapshot` and all event types (`CacheKeyEvent`, `CacheWriteEvent`, `FetchTimingEvent`, `ShadowComparisonEvent`, `MutationEvent`, `CacheOperationError`, `HeaderImpactEvent`).
> Bump to the tag that includes RG-8.

**Open questions to confirm before coding (carry from findings).**

- Does the router actually consume `HeaderImpactEvents` and `CacheOpErrors`?
  If not needed now, R8 may export a smaller subset and add them later — but the snapshot type from RG-8 should still carry them so no re-release is needed.
- Confirm `GetCacheStats()` is the only contract the router needs (vs. the exported collector methods).
  Settle this against the engine PR and document the decision.

**Acceptance criteria.**

1. `GetCacheStats()` is called exactly once per request (assert via a spy that a second call returns empty).
2. With analytics **disabled**, no snapshot is read and there is no measurable overhead (assert the collector is never initialized).
3. For a fixture with 2 L1 hits, 1 L2 hit, 1 miss: the exported metrics show exactly those counts — assert the full metric set (no fuzzy comparisons).
4. `MutationEvents` and `CacheOpErrors` map to their metrics with exact values for a crafted fixture.

**Reviewer-guide doc.**
`docs/entity-caching/reviewer-guides/R8-analytics.md` — the snapshot field → metric mapping table, the "call `GetCacheStats()` exactly once" rule and why (pooled collector release), the disabled-path zero-overhead guarantee, and the open questions on `HeaderImpactEvents`/collector methods.

---

## R9 — Mutation + extension-based invalidation

**Title:** `feat(cache): mutation and extension-based cache invalidation`

**Goal.**
Wire cache deletes triggered by mutations and by subgraph-returned `extensions.cacheInvalidation.keys`.

**Scope.**

- Populate `resolve.ResolverOptions.EntityCacheConfigs` (subgraphName → entityType → `*EntityCacheInvalidationConfig{ CacheName, IncludeSubgraphHeaderPrefix }`) from composition.
  This is the minimal config the engine needs to rebuild keys and call `LoaderCache.Delete`.
- Ensure the executor builds this map via `buildEntityCacheInvalidationConfigs` from the subgraph/type cache config.
- Translate `MutationFieldCacheConfiguration` (`EnableEntityL2CachePopulation`) and `MutationCacheInvalidationConfiguration` (`FieldName`, `EntityTypeName`) from composition (structurally wired in R1; R9 makes the delete path functional and tested end-to-end).
- The router does **not** build keys or call `Delete` itself for the engine-driven paths — the engine does, using `EntityCacheConfigs` + the per-request key transform pipeline (`GlobalCacheKeyPrefix → header prefix → L2CacheKeyInterceptor`).

**Exclusions.**

- No subscription invalidation (R10).
- No public "manual invalidation" router API in this PR; if added later it must reconstruct the **full** key including `GlobalCacheKeyPrefix` (call this out as a footgun in the guide).

**Dependency on a gqtools PR (version bump).**

> **Waits on gqtools release:** RG-9 — the gqtools invalidation PR (G-invalidation) that parses `extensions.cacheInvalidation.keys`, rebuilds L2 keys through the full transform pipeline, and calls `Delete`, requiring `EntityCacheConfigs` populated and `EnableL2Cache` true.
> Bump to the tag that includes RG-9.
> **Depends on a sibling router PR:** R3 or R4 (a backend whose `Delete` is exercised).

**Acceptance criteria.**

1. A subgraph response with `extensions.cacheInvalidation.keys` causes exactly the matching L2 keys to be deleted — assert the exact set of deleted keys (full slice).
2. A delete for a key being **written** in the same fetch is skipped (assert that key is not deleted).
3. A mutation configured with `EntityCacheInvalidationConfiguration` deletes the returned entity's L2 key after the mutation — assert the exact key.
4. Mutations skip L2 **reads** unconditionally and skip L2 writes unless `EnableEntityL2CachePopulation` — assert fetch/write behavior for both settings.
5. The deleted key includes `GlobalCacheKeyPrefix` and the header prefix when configured — assert the full key string (this guards the "forgot the prefix" footgun).

**Reviewer-guide doc.**
`docs/entity-caching/reviewer-guides/R9-invalidation.md` — the two invalidation triggers (mutation, subgraph extension), why `EntityCacheConfigs` is a separate minimal struct (to avoid a `resolve → plan` dependency), the full key-transform pipeline that any manual deleter must mirror, and the write-skip rule.

---

## R10 — Subscription cache populate / invalidate

**Title:** `feat(cache): subscription-driven cache populate and invalidate`

**Goal.**
Use subscription events to keep the L2 cache warm (populate) or to evict (invalidate), and surface the write/invalidate callbacks for observability.

**Scope.**

- Translate `SubscriptionEntityPopulationConfiguration` from composition, setting **both** `TypeName` **and** `FieldName`.
  The engine lookup `FindByTypeAndFieldName` matches on both; an empty `FieldName` silently no-ops populate/invalidate.
  The router config factory MUST always set `FieldName: cp.FieldName` (populate) and `FieldName: ci.FieldName` (invalidate).
- Map `EnableInvalidationOnKeyOnly`: populate mode writes entity data per event; invalidate mode deletes L2 when the event carries only `@key` fields.
- Optionally set `resolve.ResolverOptions.OnSubscriptionCacheWrite` and `OnSubscriptionCacheInvalidate` callbacks to feed subscription cache activity into analytics/observability (these are real public API the original integration doc omitted).

**Exclusions.**

- No new transport work; this rides the existing subscription path.

**Dependency on a gqtools PR (version bump).**

> **Waits on gqtools release:** RG-10 — the gqtools subscription-cache PR (G-subscriptions) exposing the two `OnSubscription*` callbacks and the per-event populate/invalidate engine behavior.
> Bump to the tag that includes RG-10.
> **Depends on a sibling router PR:** R3 or R4 (a backend to write/delete against).

**Critical footgun to lock with a test (carry from CLAUDE.md).**
If `FieldName` is empty, the lookup always fails and populate/invalidate become silent no-ops.
A regression test must assert that a config missing `FieldName` is rejected at build time (or at minimum logs a clear error and the test asserts the no-op is detected).

**Acceptance criteria.**

1. A populate-mode subscription event writes the entity to L2 — assert the exact L2 entry (key + value bytes).
2. An invalidate-mode event carrying only `@key` fields deletes the L2 key — assert the exact deleted key.
3. The config factory sets both `TypeName` and `FieldName`; a fixture with empty `FieldName` is caught (assert the validation error or the detected no-op).
4. `OnSubscriptionCacheWrite`/`OnSubscriptionCacheInvalidate` fire with the expected arguments — assert the full callback payloads.

**Reviewer-guide doc.**
`docs/entity-caching/reviewer-guides/R10-subscriptions.md` — populate vs. invalidate modes, the mandatory-`FieldName` footgun and its guard test, and the two observability callbacks.

---

## R11 — Documentation + config reference

**Title:** `docs(cache): entity caching operator + integration reference`

**Goal.**
Replace the stale integration doc with an accurate operator-facing reference and an internal integration guide, reflecting the **current** contracts.

**Scope.**

- Operator guide: every YAML field with defaults, the provider-ID precedence, the three dev headers and their gate, when to choose Redis vs. in-memory, shadow mode, analytics, invalidation.
- Integration/architecture note: the four router responsibilities, the thin seam, and the corrected contracts:
  - `LoaderCache.Set(ctx, entries)` (per-entry TTL, no `ttl` param).
  - The two subscription callbacks.
  - `SubscriptionEntityPopulationConfiguration.FieldName` is mandatory.
  - `EntityCacheConfiguration` has `NegativeCacheTTL` and `HashAnalyticsKeys`.
  - Cache trace is gated on `ctx.TracingOptions` (Enable + not ExcludeCacheStats), not a nonexistent `WithRequestTraceOptions` helper.
- Cross-link to [02-DIRECTIVE-INVENTORY.md](02-DIRECTIVE-INVENTORY.md) for the composition-side directives that produce these configs.

**Exclusions.**

- No code changes (docs only); CI must still pass markdown lint.

**Dependency on a gqtools PR (version bump).**

> **Waits on gqtools release:** none (docs).
> **Depends on:** all prior router PRs so the documented behavior is accurate.

**Acceptance criteria.**

1. Every YAML key in `config.schema.json` appears in the operator guide with its default.
2. Every corrected contract above is documented; the stale `Set(ctx, entries, ttl)` and `RenderCacheKeys(ctx, fetch, keys)` signatures do not appear anywhere.
3. Markdown follows the repo line-breaking convention (new line after each sentence-ending period, and after commas in long sentences).

**Reviewer-guide doc.**
This PR *is* documentation; the reviewer guide is a one-paragraph changelog of what was corrected from the old integration doc, with a diff-style before/after of the two load-bearing signatures.

---

## Dependency graph

```text
                 RG-1 release
                     │
                     ▼
                ┌── R1 (foundation) ──┐
                │                     │
   ┌────────────┼──────────┬─────────┼───────────┬───────────┐
   ▼            ▼          ▼          ▼           ▼           ▼
  R2 (L1)   R3 (Redis)  R4 (Memory) R6 (headers) R7 (shadow) R8 (analytics)
                │          │                       ▲ RG-7        ▲ RG-8
                └────┬─────┘
                     ▼
                 R5 (breaker)
                     │
        ┌────────────┴────────────┐
        ▼                         ▼
   R9 (invalidation)         R10 (subscriptions)
     ▲ RG-9 + (R3|R4)          ▲ RG-10 + (R3|R4)
        └───────────┬───────────┘
                    ▼
              R11 (docs, after all)
```

Edges marked `RG-n` are the **gqtools release gates**: that router PR cannot open until the named gqtools release ships and `go.mod` is bumped.
Edges marked `(R3|R4)` are router-internal: the PR needs at least one L2 backend present.

---

## Where a router PR must wait on a gqtools release (summary)

| Router PR | Blocking gqtools release | Why |
|-----------|--------------------------|-----|
| R1 | RG-1 (foundation) | needs all `plan.*` config types, `LoaderCache`, `CachingOptions`, factory option |
| R2 | (RG-1; or G-L1 if split) | needs `EnableL1Cache` (and the L1 engine impl if separately released) |
| R3 | RG-1 | implements `LoaderCache`; no new API |
| R4 | RG-1 | implements `LoaderCache`; no new API |
| R5 | none | decorates `LoaderCache` |
| R6 | RG-1 | flags + `GlobalCacheKeyPrefix` already shipped |
| R7 | RG-7 (shadow) | needs `ShadowComparisonEvent` + engine serve-fresh-and-compare |
| R8 | RG-8 (analytics) | needs `CacheAnalyticsSnapshot` + event types |
| R9 | RG-9 (invalidation) | needs `EntityCacheConfigs`-driven `Delete` + extension parsing |
| R10 | RG-10 (subscriptions) | needs `OnSubscription*` callbacks + per-event behavior |
| R11 | none | docs only |

The router stack therefore tracks the gqtools stack one release behind at each gated step.
The recommended cadence: land the gqtools PR, cut a release, bump and open the matching router PR.
R2–R6 can all proceed in parallel as soon as RG-1 ships; R7/R8/R9/R10 each unlock with their named gqtools release.

---

## Out-of-scope cleanups (do not bundle)

These were folded into the original 96K-line squash and must be split into their own non-blocking chore PRs so they never block the caching review.
See [07-UNRELATED-FINDINGS.md](07-UNRELATED-FINDINGS.md) for the full list:

- Regenerated protobuf files.
- Demo applications and example configs.
- Benchmark harnesses.

Keeping these out of the stack is what lets each caching PR stay small and reviewable.
