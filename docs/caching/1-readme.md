# Entity and root-field caching

This is the reference for what the caching feature consists of,
how to configure it, how it behaves as a black box, and how it is tested.
Design history lives in git.

The full document set:

| Document | What it is for |
|---|---|
| this README | index: black-box contract, configuration, file map, test map |
| [2-behavior.md](2-behavior.md) | the detailed behavior spec: wire traffic, worked key derivations, store/serve decision rules |
| [3-onboarding.md](3-onboarding.md) | the handover deep dive: mental model, code-path walkthrough, per-area internals, change recipes, gotchas |
| [6-open-issues.md](6-open-issues.md) | open design notes and known bugs: the root-field key bug, schema-deploy invalidation, `s-maxage`, entity-interface verification |
| [5-testing-on-js-subgraphs.md](5-testing-on-js-subgraphs.md) | findings from running the engine against live Apollo-federation JS subgraphs |
| [4-redis-adapter-report.md](4-redis-adapter-report.md) | researched design for the router-side Redis store adapter |

## 1. What it does

The engine caches **subgraph fetches** — never whole client responses — in two layers:

- **L1** — request-lifetime, in-memory `*astjson.Value` store.
  Shared across defer groups; zero marshaling.
- **L2** — an external store behind a batched two-method interface (Redis, memory, anything).
  The only layer that marshals bytes.

Freshness follows the HTTP model.
A subgraph's `Cache-Control` response header is the runtime truth:
`max-age` sets the entry TTL, `no-store` forbids caching, `private` triggers the privacy rules.
A static configuration cascade fills in when the header is silent.

Entries carry stable tags for external invalidation.
Each request exposes an aggregated client cache policy the router can emit as a response header.

With no cache controller attached, the runtime path is byte-identical to a non-caching build.
With no caching configuration, plans are byte-identical too.
These are the two **no-op gates**.

## 2. Black-box behavior contract

The one-paragraph version; every rule below is expanded with worked examples in 2-behavior.md.

- A cached entity fetch is keyed by exactly what it sends:
  subgraph, `@key` fields plus `@requires` values, argument values, requester partition.
  Read key == write key == fetch identity ([BEHAVIOR §3](2-behavior.md#3-cache-keys)).
- A value is written only after a successful, storable, TTL-positive fetch;
  it is served only when it decodes, matches the scope, and covers the selection
  ([BEHAVIOR §4](2-behavior.md#4-when-a-value-is-cached--and-when-not),
  [§6](2-behavior.md#6-when-a-cached-entry-is-served--and-when-it-is-a-miss)).
- Stored values are normalized (schema names, argument-suffixed fields),
  so aliases and query shape never fragment or corrupt entries
  ([BEHAVIOR §5](2-behavior.md#5-what-is-stored)).
- Batches render one key per unique representation;
  with partial loading the subgraph receives only the missing representations
  ([BEHAVIOR §7](2-behavior.md#7-batch-entity-fetches)).
- Empty entities cache as a negative sentinel under `NegativeCacheTTL`.
- Private data is partitioned per requester or not stored at all
  ([BEHAVIOR §9](2-behavior.md#9-privacy-behavior)).
- Cache failures never fail a request; the origin is the fallback
  ([BEHAVIOR §14](2-behavior.md#14-store-interaction-contract)).
- Caching never changes a response.
  A cached and an uncached run of the same operation produce identical bodies.

## 3. Components and file map

The feature is split at a hard seam:
the `resolve` package holds only contract types and hook call sites,
and the `cache` package holds all cache logic.
Plan and postprocess hold thin shims.
The tables below are the index;
each area's internals, with the reasoning behind its design,
are explained in [ONBOARDING §3](3-onboarding.md#3-deep-dives),
and the code path connecting them in [ONBOARDING §2](3-onboarding.md#2-life-of-a-cached-request--the-code-path).

### Plan time (per plan, cacheable work)

| Area | Files |
|---|---|
| Configuration model: cascade, `Resolve`, `@cache` directive extraction | `v2/pkg/engine/plan/cacheconfig/cacheconfig.go`, `cache_directive.go` |
| ProvidesData walk: per-fetch field tree (aliases, args, type conditions) | `v2/pkg/engine/plan/cache_provides_data_visitor.go` |
| Per-root-field planner isolation gate | `v2/pkg/engine/plan/root_field_isolation.go` |
| `@key`-vs-`@requires` field marking on representation nodes | `v2/pkg/engine/plan/representationvariable/representation_variable.go` |
| Planner wiring (gated second walk) | `v2/pkg/engine/plan/planner.go` |
| Postprocess wiring into the caching passes | `v2/pkg/engine/postprocess/postprocess.go` |
| Pass entry: fetch config assembly + L1 narrowing | `v2/pkg/engine/cache/configure_caching.go`, `fetch_cache_configurator.go`, `optimize_l1_cache.go` |
| Key spec freezing from the fetch's representation node | `v2/pkg/engine/cache/cache_key_builder.go` |

### Runtime (per request)

| Area | Files |
|---|---|
| Loader-facing contract: `CacheController`, `RequestCache`, `Decision`, handle types | `v2/pkg/engine/resolve/cache_controller.go` |
| Per-fetch config carried on the plan | `v2/pkg/engine/resolve/cache_config.go` |
| Arena/lock transaction the hooks run under | `v2/pkg/engine/resolve/cache_transaction.go` |
| Client cache answer type | `v2/pkg/engine/resolve/cache_response.go` |
| Deferred batch input assembly (reduced representations) | `v2/pkg/engine/resolve/batch_input_assembly.go` |
| Hook call sites in the loader; ports on the Context | `v2/pkg/engine/resolve/loader.go`, `context.go`, `resolve.go` |
| The controller: lookup, decisions, merge hooks, deferred writes | `v2/pkg/engine/cache/controller.go` |
| Key rendering: entity/root-field keys, args digest, partitions | `v2/pkg/engine/cache/cache_key_template.go` |
| `Cache-Control` parsing + TTL/storability ladder | `v2/pkg/engine/cache/cache_control.go` |
| Value envelope encode/decode | `v2/pkg/engine/cache/envelope.go` |
| Coverage walk (can this entry serve this fetch) | `v2/pkg/engine/cache/coverage.go` |
| Normalize/denormalize between query shape and stored shape | `v2/pkg/engine/cache/transform.go` |
| Partial batch splice/realign | `v2/pkg/engine/cache/partial.go` |
| Default invalidation tags | `v2/pkg/engine/cache/cache_tags.go` |
| Client cache answer folding | `v2/pkg/engine/cache/cache_response.go` |
| ART trace / analytics observer | `v2/pkg/engine/cache/observer.go` |
| Test doubles (fake store with op log, recording observers/controllers) | `v2/pkg/engine/cache/cachetesting/` |

### Integration surface

| Area | Files |
|---|---|
| `Configuration.SetCaching` — the one config entry point | `execution/engine/engine_config.go` |
| `WithCacheController`, `WithPrivatePartitionProvider`, `CacheResponseInfo` | `execution/engine/execution_engine.go` |

## 4. Configuring it

### 4.1 Plan side — the configuration cascade

`Configuration.SetCaching(cacheconfig.CachingConfiguration)`.
Three levels, most specific wins, every knob optional:

```go
engineConfig.SetCaching(cacheconfig.CachingConfiguration{
    Global: cacheconfig.GlobalCacheConfig{
        DefaultTTL:       5 * time.Minute,  // static fallback when no header and no narrower config
        MaxTTL:           time.Hour,        // clamp on ANY resolved TTL, header-driven included; 0 = no clamp
        NegativeCacheTTL: 30 * time.Second, // empty-entity sentinel lifetime; header-independent
        EnablePartialCacheLoad: true,       // mixed batch hits fetch only the missing entities
        ShadowMode:       false,            // read-never-serve staleness measurement
        HashAnalyticsKeys: false,           // hash key material in traces
        EmitCdnTags:      false,            // accumulate the CDN tag union (default off)
        // EmitClientCacheControl *bool — client policy aggregation, default true
    },
    Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
        "products": { // keyed by datasource ID; absent subgraphs inherit pure global values
            DefaultTTL: ptr(time.Minute),   // pointer fields: nil = inherit from global
            Enabled:    ptr(false),         // explicit veto for one subgraph
            IncludeSubgraphHeaders: ptr(true), // vary-by-headers (public) or partition source (private)
            Scope: ptr(cacheconfig.CacheScopePrivate), // every entry partitioned per requester
            Types: map[string]cacheconfig.TypeCacheConfig{ // most specific static tier
                "Product": {MaxAge: 30 * time.Second},                     // beats the subgraph default
                "Secret":  {MaxAge: 0},                                    // declared uncacheable, sentinel included
                "Profile": {MaxAge: time.Minute, Scope: cacheconfig.CacheScopePrivate},
            },
            RootFields: []cacheconfig.RootFieldCacheConfig{
                {TypeName: "Query", FieldName: "topProducts", TTL: time.Minute},
                // TTL 0 falls back to subgraph then global DefaultTTL; the entry itself is the opt-in
            },
        },
    },
})
```

Enablement rules:

- An entity fetch is cached when its type declaration says so (`MaxAge > 0`).
  Without a declaration, it is cached when the effective subgraph config yields
  `DefaultTTL > 0 || NegativeCacheTTL > 0`.
  A declared `MaxAge: 0` vetoes the type entirely.
- A root field is cached only when a coordinate entry exists.
- There are no L1/L2 switches.
  L2 follows the resolved TTLs; L1 eligibility is computed by the `optimizeL1Cache` postprocess pass.

### 4.2 Per-type declarations from subgraph SDL

Subgraph authors can declare per-type caching in their SDL;
the router extracts it into the `Types` map:

```graphql
directive @cache(maxAge: Int!, scope: CacheControlScope = PUBLIC) on OBJECT

type Product @key(fields: "upc") @cache(maxAge: 60) {
  upc: String!
}
```

```go
types, warnings := cacheconfig.ExtractTypeCacheConfigs(subgraphSDL)
// merge into SubgraphCacheConfig.Types
// surface warnings — malformed declarations are skipped, never errors
```

`cacheconfig.CacheDirectiveDefinition` exports the directive SDL for composition tooling.

### 4.3 Runtime side

```go
controller := cache.NewController(store, observer, cache.WithGlobalConfig(caching.Global))
engine.Execute(ctx, operation, writer,
    engine.WithCacheController(controller),
    engine.WithPrivatePartitionProvider(partitions), // only needed for PRIVATE scopes
)
```

The store the integrator supplies:

```go
type Store interface {
    GetMany(ctx context.Context, keys []string) ([]Entry, error)
    SetMany(ctx context.Context, items []Item) error
}
// Item{Key, Value, TTL, Tags}
// Entry{Value, RemainingTTL, OK} — positional with keys
```

Tag index maintenance and delete-by-tag are the store implementation's job;
the engine only attaches tags.

The partition provider names the requester for private scopes:

```go
type PrivatePartitionProvider interface {
    PrivatePartition(ctx *resolve.Context, subgraphName string) (value string, ok bool)
}
```

### 4.4 Reading the client cache policy

After resolution, the router reads one value and decides what headers to emit:

```go
info := resolveContext.CacheResponseInfo()
// CacheResponseInfo{HasPolicy, MaxAge, Private, NoStore, Tags}
```

Emission rules and the folding semantics: [BEHAVIOR §10](2-behavior.md#10-the-client-cache-answer).

## 5. Architecture

Plan time does all cacheable work; runtime only renders and decides:

1. **Plan** — a gated second walk (`cacheProvidesDataVisitor`) records,
   per fetch, the exact field tree that fetch returns.
   The path builder isolates cached QUERY root fields into their own planners.
2. **Postprocess** — `cache.ConfigureCaching` resolves the config cascade per fetch,
   freezes the key spec from the fetch's own representation node,
   attaches a `resolve.FetchCacheConfig` to each cache-eligible fetch,
   and narrows L1 to provider/consumer pairs across the response's fetch trees.
3. **Runtime** — the loader calls four hooks around each fetch
   (`PrepareFetch`, `OnFetchSkipped`, `OnFetchResult`, `EndRequest`,
   plus `OnUncachedFetch` for config-less fetches).
   The controller renders keys, reads L1 then one batched `GetMany`,
   decides (`Fetch` / `SkipFullHit` / `FetchPartial` / `FetchShadow`),
   splices or write-gates on merge, and flushes one `SetMany` at request end.
   The loader branches on nothing but the `Decision`.

Concurrency: every hook opens exactly one `CacheTransaction`,
which holds the request's single data lock for the whole arena sequence;
all per-request cache state is guarded by that external lock.
`EndRequest` runs once, single-threaded, arena-free.

Only static data is written at plan time;
digests, partitions, and header hashes derive per request,
so plans stay safely shareable across requests (plan-cache safety).

## 6. Invariants (must survive any change)

- Both no-op gates: no controller / no config ⇒ byte-identical behavior / plans.
- Key fidelity: one template per fetch renders read and write keys from the fetch's own representation;
  the L1/L2 keys share one preimage core.
- Write gate: never reducible to `!HasErrors`; store failures never fail a request.
- Lock discipline: one `CacheTransaction` per hook;
  the arena is never touched outside a held transaction.
  The client-policy aggregate carries its own mutex because it folds on failure paths outside transactions.
- Privacy: no identity ⇒ no L2; partition values are sha256-hashed and source-tagged;
  tags and traces never carry identity material.
- Plan-cache safety: only static data is written at plan time.

## 7. How it is tested

### 7.1 Conventions

- Full-value `assert.Equal` only — complete op logs, complete envelopes, complete responses;
  no partial contains-checks and no golden files.
- Time via `testing/synctest` (unit suites); `-race` on the cache/resolve/cachingtesting suites.
- e2e fixtures are wgc-composed SDL with the composed `config.json` committed
  (`execution/cachingtesting/compose.sh`, `graph.yaml`, `subgraphs/`);
  composition reruns only when fixtures change, plans are never hand-written.
- Subgraph doubles are real `httptest` servers;
  `SubgraphRule.Headers` sets response headers per fixture without recomposition.
- Self-contained subtests; scenario-matrix IDs in subtest names where a spec matrix exists.

### 7.2 Unit suites — `v2/pkg/engine/cache`

Run against a fake store with a full op log, under `synctest` clocks.

| File | Behavior under test |
|---|---|
| `controller_test.go` | decision dispatch, the write-gate rows (transport/parse/GraphQL-error/null variants), handle lifecycle |
| `controller_batch_test.go` | one key per unique representation, batch full-hit/full-miss, bucket alignment |
| `controller_cache_control_test.go` | header-driven storability and TTL flowing through the controller |
| `controller_l1_test.go` | L1 writes/serves, structural isolation, sentinel behavior |
| `controller_negative_test.go` | negative sentinel: write rules, TTL source, serve-as-null |
| `controller_privacy_test.go` | partitioned access, no-identity L2 skip, scope guard both directions |
| `controller_rootfield_test.go` | root-field whole-response entries, shadow asymmetry |
| `controller_shadow_test.go` | stash → compare → overwrite; metadata excluded from the compare |
| `controller_store_test.go` | store-error degradation to miss / dropped writes, `OnStoreError` |
| `cache_control_test.go` | `Cache-Control` parser rows + the `resolveCaching` ladder |
| `cache_key_template_test.go` | entity key rendering: type conditions, unrenderable inputs, number/string unification |
| `cache_key_args_test.go` | argument digest and per-field argument suffixes |
| `cache_key_partition_test.go` | partition and header-hash segments, byte-for-byte prefixes |
| `cache_key_builder_test.go` | plan-time key spec freezing from representation nodes |
| `cache_tags_test.go` | tag vocabulary, `@key`-only entity digest |
| `envelope_test.go` | envelope encode/decode, undecodable-bytes-as-miss |
| `coverage_test.go` | coverage walk: nullability, type conditions, argument-suffixed names |
| `transform_test.go` | normalize/denormalize round trips across alias sets |
| `cache_response_test.go` | client-answer folding: min-freshness, zero rules, defer suppression |
| `optimize_l1_cache_test.go` | L1 narrowing: provider/consumer proof, defer-tree ancestry, cycle bounds |
| `configure_caching_test.go` | pass wiring and the planner no-op gate |
| `fetch_cache_configurator_test.go` (+ `_rootfield`, `_types`) | cascade resolution into per-fetch configs, vetoes, mixed-root-field decline |
| `observer_test.go` | trace assembly from handles |

### 7.3 Plan and contract suites

| File | Behavior under test |
|---|---|
| `v2/pkg/engine/plan/cache_provides_data_visitor_test.go` | the no-op gate; plans byte-identical with an empty caching config |
| `v2/pkg/engine/plan/cache_provides_data_visitor_port_test.go` | full ProvidesData tree fidelity per fetch |
| `v2/pkg/engine/plan/root_field_isolation_test.go` | cached root fields planned into isolated fetches |
| `v2/pkg/engine/plan/cacheconfig/*_test.go` | cascade `Resolve`, `@cache` extraction warnings, type-tier semantics |
| `v2/pkg/engine/resolve/cache_config_test.go` | `FetchCacheConfig.Equals` — plan dedup can never conflate cache policy |
| `v2/pkg/engine/resolve/cache_controller_test.go` | contract types (decisions, handle strings) |
| `v2/pkg/engine/resolve/cache_noop_test.go` | the runtime no-op gate: nil controller costs nothing |
| `v2/pkg/engine/resolve/cache_fetch_test.go`, `cache_node_copy_test.go` | fetch-interface accessors; field/node copies carry cache metadata |
| `v2/pkg/engine/resolve/batch_input_assembly_test.go` | deferred batch input assembly, reduced-keep rendering |

### 7.4 End-to-end suites — `execution/cachingtesting`

Every suite drives the REAL execution engine over real `httptest` subgraph doubles
and asserts complete client responses plus complete store-op logs.
`cachingtesting.go` is the plan harness, `enginetesting.go` the engine harness
(doubles, request counting, URL/clock normalizers).

| File | Behavior under test (2-behavior.md section) |
|---|---|
| `entity_l2_test.go` | the miss → write → hit lifecycle, zero network on hit, hook dispatch (§2) |
| `entity_config_test.go` | the per-fetch cache config landing in real plans |
| `envelope_e2e_test.go` | stored bytes, corrupted-entry recovery, per-subgraph keyspace separation (§5) |
| `batch_e2e_test.go` | batch full-hit and mixed-run-without-partial refetch (§7) |
| `partial_e2e_test.go` | the reduced representations request, mixed-TTL expiry (§7) |
| `negative_e2e_test.go` | empty entities served as null from the sentinel (§5) |
| `cache_control_e2e_test.go` | response-header TTL beating and vetoing the static tiers (§8) |
| `cascade_e2e_test.go` | global/subgraph inheritance and vetoes (§4) |
| `type_declaration_e2e_test.go` | per-type lifetimes, `MaxAge: 0` veto, private type partitioning (§4, §9) |
| `args_key_e2e_test.go` | argument-variant entries, no ping-pong, aliased multi-variant reuse (§3.2) |
| `normalization_e2e_test.go` | alias-different operations sharing one entry (§5) |
| `rootfield_e2e_test.go` | root-field entries, alias-variant reuse, different-arguments miss (§3.5) |
| `isolation_e2e_test.go` | cached root fields fetch-isolated, independent expiry (§3.5) |
| `privacy_e2e_test.go` | partitions per requester, no-identity skip, runtime-private drop (§9) |
| `tags_e2e_test.go` | the tag vocabulary on every write, shared entity tags across variants (§11) |
| `client_headers_e2e_test.go` | `CacheResponseInfo`: countdown, no-store forcing, defer suppression (§10) |
| `store_batching_e2e_test.go` | one `GetMany` per fetch / one `SetMany` per request, store-error fallback (§14) |
| `l1_e2e_test.go` | in-request L1 reuse: a later fetch of the same entity, no second store lookup (§13) |
| `defer_l1_e2e_test.go` | cross-defer-group L1 serving, superset shapes (§13) |
| `shadow_e2e_test.go` | shadow read-compare-overwrite, origin always served (§12) |
| `art_e2e_test.go` | the cache section of the ART trace, key hashing (§15) |
| `provides_data_test.go` | ProvidesData trees over real federation plans, deferred fetches |
| `loader_bench_test.go` | allocation budget of the hit and miss paths |

## 8. Out of scope / follow-ups

- Mutation- and subscription-driven invalidation; user-defined cache tags;
  `stale-while-revalidate` / `stale-if-error`; request-level `no-cache` bypass;
  `s-maxage` (see notes); a cache debugger (ART + shadow mode cover the operator story).
- The invalidation endpoint and the Redis store implementation
  (indexes, delete-by-tag, circuit breaking) live in the router repository,
  bound by the tag vocabulary and the store contract above.
- Store-outage behavior is deliberate origin fallback:
  pair caching with subgraph rate limiting operationally.
