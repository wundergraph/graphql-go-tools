# Caching onboarding guide

This is the handover document for an engineer taking over the response-caching feature.
It explains the mental model, walks the actual code path of a cached request,
gives a deep dive into every area with the reasoning behind its design,
and ends with how to work on the code day to day.

How to use the doc set:

- Read this document once, top to bottom — it is the tour.
- [2-behavior.md](2-behavior.md) is the behavior contract with worked wire-level examples;
  use it as the reference when you need to know "what exactly happens when…".
- [1-readme.md](1-readme.md) is the index: configuration surface, file map, test map.
- The working notes and reports live in this folder (see §8) — read them before planning new work.

## 1. The mental model

### 1.1 What is cached

The engine caches **subgraph fetches**, never whole client responses.
A federated query plan is a tree of fetches (root fetches and `_entities` fetches),
and each fetch is independently cacheable.
This is the same granularity Apollo Router's response cache uses,
and it is what makes partial reuse work:
one request's inventory data serves another request's completely different query,
as long as both plans produce an inventory fetch with the same identity.

Two layers hold values:

- **L1** is a per-request, in-memory map of decoded `*astjson.Value`s.
  It exists because one request can plan the same entity fetch several times —
  most importantly across `@defer` groups — and the second occurrence
  should cost neither network nor a store round trip.
  It dies with the request, so it needs no versioning, no privacy partitions, no TTL.
- **L2** is the external store (Redis in production, fakes in tests)
  behind the two-method `Store` interface.
  It is the only place bytes are marshaled,
  and every entry is wrapped in a self-describing envelope.

### 1.2 The identity idea: the key is what the fetch sends

The single most important design decision:
**a fetch's cache key is derived from the exact representation object the fetch sends to the subgraph**.
Not from the query text, not from a hand-configured key template, not from `@key` metadata alone —
from the merged representation node that the planner builds
and that the loader renders into the `representations` variable.

Consequences worth internalizing:

- Read key == write key == fetch identity, structurally guaranteed.
  There is no way for a lookup to use a different identity than the write,
  because both render from the same frozen node against the same item.
- `@requires` values are part of the key.
  A field computed from a `@requires` input (say, `shippingEstimate` requiring `weight`)
  is cached per input value, so a changed input can never serve a stale derived value.
- Anything the client does that does not change what the fetch sends —
  aliases, selection order, operation names, fragment layout — cannot fragment the cache.

### 1.3 The seam: the loader is blind

The loader (the component that executes fetches and merges results)
knows almost nothing about caching.
It calls four hooks around each fetch and branches on exactly one thing:
the `Decision` enum (`Fetch` / `SkipFullHit` / `FetchPartial` / `FetchShadow`).
Everything else — keys, envelopes, TTLs, privacy, tags — lives behind the
`resolve.CacheController` / `resolve.RequestCache` interfaces,
implemented by the `cache` package.

This seam is load-bearing:

- The `resolve` package holds only contract types (`cache_controller.go`)
  and never imports the `cache` package, so there is no cycle
  and the loader compiles without any cache logic.
- With a nil controller the loader never allocates a handle, never takes a lock,
  never enters cache code — the **runtime no-op gate**.
- All cache behavior is testable through the seam with recording fakes,
  without touching loader internals.

### 1.4 The three planes

Work happens at three times, and each plane only produces data for the next:

1. **Configuration** (integrator startup):
   the declarative cascade in `plan/cacheconfig` — global defaults,
   per-subgraph overrides, per-type declarations, per-root-field entries.
   Pure data, no cache logic, importable by everything.
2. **Plan time** (per operation, cacheable):
   a second planner walk records what each fetch returns (`ProvidesData`),
   the path builder isolates cached root fields into their own fetches,
   and postprocess resolves the cascade into one `resolve.FetchCacheConfig` per fetch
   and freezes the key spec from the fetch's representation node.
   Only static data is produced — plans stay shareable across requests.
3. **Runtime** (per request):
   the controller renders keys against the request's items,
   reads L1 then the store, decides, splices or writes.
   Everything request-specific (argument values, requester identity, header hashes)
   is derived here and only here.

If you remember one rule when extending the feature:
**push work to the earliest plane that can hold it**.
Plan-time work is amortized across every request that reuses the plan.

## 2. Life of a cached request — the code path

The walkthrough below names the actual functions, in call order.
Scenario: `{ me { favoriteProduct { upc stock } } }`,
where the `inventory` subgraph (datasource `1`) is configured with `DefaultTTL: time.Minute`.
The plan chains three fetches: `users` root → `products` entity → `inventory` entity.

### 2.1 Plan time

1. `plan.Planner.Plan` finishes its normal planning walks, then —
   only when `Configuration.Caching != nil` — runs `cacheProvidesDataVisitor.walk`
   on a dedicated walker (`plan/cache_provides_data_visitor.go`).
   For every planner (= every fetch) it builds the exact field tree that fetch returns:
   the inventory fetch's tree is `{stock}` with `OnTypeNames: [Product]`.
   Aliases (`Field.OriginalName`), argument bindings (`Field.CacheArgs`),
   and the entity-boundary reset are recorded here.
   The trees attach to the plan as a side table keyed by `*FetchInfo`
   (`GraphQLResponse.CacheProvidesData()`), shared by the initial response and all defer groups.
2. Postprocess calls `cache.Configurator.ConfigureCaching`
   (wired in `postprocess/postprocess.go`, entry in `cache/configure_caching.go`).
   For each fetch, `fetch_cache_configurator.go` resolves the cascade:
   `caching.Resolve("inventory")` yields the effective subgraph config,
   `Entities([Product])` says "cached, TTL 1m, public",
   and a `resolve.FetchCacheConfig` is assembled and attached to the fetch.
   `cache_key_builder.go` freezes the key spec:
   a pointer to the fetch's own representation node plus scope metadata.
3. `optimize_l1_cache.go` walks all fetch trees of the response
   (root plus every defer group) and narrows `cfg.L1`:
   only fetches that provably have an L1 provider/consumer partner keep it,
   so a lone entity fetch does not pay L1 write costs for nothing.

### 2.2 Request start

4. The loader reaches the inventory fetch in `resolveSingle`
   and calls `Loader.cachePrepare` (`resolve/loader.go`).
   The two uncached fetches before it took the other branch:
   a configured controller with a config-less fetch fires `RequestCache.OnUncachedFetch`,
   which only tells the client-policy aggregate
   "part of this response came from outside the cache".
5. `Loader.cacheRequest` lazily creates the per-request surface:
   `Controller.BeginRequest` (`cache/controller.go`) runs at most once per request,
   under the request's data lock,
   and the returned `requestCache` is shared by reference across all defer-group loaders.

### 2.3 The lookup

6. `requestCache.PrepareFetch` → `prepareFetch`.
   It opens ONE `CacheTransaction` (`in.Arena.Begin()`),
   which takes the request's single `DataBuffer` lock for the whole hook body —
   this is the entire concurrency story, there are no internal mutexes
   (except the client-policy aggregate's own, see §3.9).
7. `newFetchKeyTemplate` builds the per-fetch key template ONCE:
   `resolveL2Access` decides store access and the privacy partition (§3.7),
   `argsDigest` folds parameterized-field argument values (§3.3).
8. `prepareItems` renders each item's keys via `cacheKeyTemplate.render`:
   the L1 key is the raw preimage, the L2 key the versioned digest.
   `serveFromL1` checks the request-lifetime map first;
   remaining items go into ONE `storeGetMany` call.
9. `applyStoreEntry` accepts or rejects each returned entry:
   decode (`decodeEnvelope`), scope check (`servableScope`),
   then either the negative-sentinel short-circuit or the coverage walk (`covers`).
   Accepted values are stored on the handle in NORMALIZED form and populate L1.
10. The decision falls out of the item states:
    every item covered → `DecisionSkipFullHit`;
    some covered and `EnablePartialCacheLoad` → `DecisionFetchPartial`
    (with `handle.BatchFetchKeep` marking the still-missing buckets);
    a shadow-mode read that found something to compare → `DecisionFetchShadow`
    (the loader treats it exactly like a miss); otherwise `DecisionFetch`.

### 2.4 The fetch (or its absence)

11. On `SkipFullHit` the loader skips the network load entirely,
    and `mergeResult` early-returns into `OnFetchSkipped`,
    where `spliceCachedItem` denormalizes each cached value
    to the requesting operation's aliases (`denormalizeToSelection`)
    and merges it at the fetch's merge path.
    A pure hit owes no writes.
12. On `FetchPartial` the loader's `assembleBatchInput` renders the final batch input
    from pre-rendered per-bucket segments (`resolve/batch_input_assembly.go`),
    keeping only the missing representations — the reduced list is never parsed back from bytes.
13. On `Fetch`/`FetchShadow` the fetch runs unchanged.

### 2.5 The merge and the write

14. After merge processing, `Loader.cacheMerge` dispatches on the decision:
    `OnFetchSkipped` for full hits, `OnFetchResult` for everything else
    (the partial arm splices covered buckets AND realigns + merges the fetched ones
    in `onPartialBatchResult`, one hook, one lock acquisition).
15. `OnFetchResult` applies the write gate —
    `!FetchFailed && !HasErrors && ResponseData != nil && != null`;
    `EmptyEntity` is the one non-failure that still writes (the negative sentinel) —
    then `cachingDecisionFor` runs the storability ladder (§3.6)
    over the response's `Cache-Control` header and the static config.
16. `writeFetchedValue` normalizes the value to schema shape (`normalizeToSchema`),
    puts it into L1, and `deferSet` encodes the envelope and queues the write.
    Nothing hits the store yet.
17. Shadow mode inserts one step before the write:
    `CacheObserver.CompareShadow` byte-compares the stashed would-have-served value
    against the fresh data.

### 2.6 Request end

18. After the whole tree (root + every defer group) resolves,
    `requestCache.EndRequest` flushes all queued writes in ONE `SetMany`
    and finalizes observation.
    It runs after the request arenas are released,
    so it must never touch arena-owned values — everything it consumes is plain heap data
    (this is a real trap, see §6).
19. The router reads `Context.CacheResponseInfo()` for the client cache answer (§3.9).

## 3. Deep dives

### 3.1 The configuration cascade (`plan/cacheconfig`)

The cascade is three levels of pure data:
`GlobalCacheConfig` → `SubgraphCacheConfig` (pointer fields, `nil` = inherit) →
`TypeCacheConfig` / `RootFieldCacheConfig`.
Consumers never walk the levels themselves;
`CachingConfiguration.Resolve(subgraphID)` produces an `EffectiveSubgraphConfig`,
and its two methods answer the only two questions the planner asks:

- `Entities(typeNames)` — for an entity fetch over one or more concrete types
  (an abstract-type batch fetch can resolve several).
  Each type contributes its declaration or the subgraph default;
  the fetch takes the MINIMUM lifetime and turns private if ANY contribution is private.
  A `MaxAge: 0` declaration is a veto for the whole fetch, sentinel included.
- `RootField(typeName, fieldName)` — an entry must exist for the exact coordinate;
  a root field is never cached by a bare `DefaultTTL`.
  The TTL ladder (coordinate → subgraph → global) fills a zero TTL in the entry.

Why enablement is derived rather than switched:
a positive resolved TTL, a `NegativeCacheTTL`, or shadow mode IS the intent to cache;
a separate boolean would just be one more thing to keep consistent.
`ExtractTypeCacheConfigs` (`cache_directive.go`) turns `@cache(maxAge:, scope:)`
directives from a subgraph SDL into the `Types` map,
skip-and-warn on malformed declarations — configuration reading never fails a boot.

### 3.2 The provides-data walk (`plan/cache_provides_data_visitor.go`)

The cache needs to know, per fetch, the exact tree of fields that fetch RETURNS —
for coverage checks, for normalization, and for the argument digest.
The main planning walk cannot give this cheaply
(its visitor state is deep in fetch construction),
so a second, filter-free walk runs after planning,
reading `fieldPlanners` (the field → planner-IDs attribution the main walk left behind).

Mechanics worth knowing before touching it:

- It maintains a per-planner frame stack (`currentFields`) mirroring the walk depth,
  and pops frames by field ref (`popFields`).
- The **entity-boundary reset**: a nested entity fetch provides the ENTITY's fields,
  not the path from the query root down to it.
  When the walk enters the field where a nested fetch's entity begins,
  the tree for that planner restarts at the entity object,
  and its direct children get `OnTypeNames` (the entity type condition).
- Aliases: the tree's `Field.Name` is the RESPONSE key (the alias);
  `OriginalName` carries the schema name when they differ.
  `ComputeHasAliases` then marks every object with aliased or parameterized fields below,
  which is the gate the transform walks and the args digest run on —
  unaliased, argument-free fetches skip all of that work.
- Arguments: `captureFieldCacheArgs` records `(argName, variableName)` pairs —
  the VALUES are resolved per request, plan-time only records the binding.
  Root-operation-field arguments are deliberately not captured per field;
  they are part of the root-field key instead.

The walk is gated on `Caching != nil` and proven plan-identical when the config enables nothing.

### 3.3 Keys (`cache/cache_key_template.go`, `cache_key_builder.go`)

`buildEntitySpec` / `buildRootFieldSpec` freeze the plan-time spec:
for entities, literally a pointer to the fetch's representation node
(plan-owned, read-only at runtime).
At request time `newCacheKeyTemplate` derives one template per fetch,
and `render` produces both keys per item from ONE canonical rendering:

```text
body     = {"__typename":"Product","representation":{"upc":"1"}[,"args":"<digest>"]
L1       = 1:<body>}
preimage = v1:1:<body>[,"partition":"<sha256>"]}
L2       = v1:1:<xxhash64(preimage) as 16-hex>
```

Details that answer most "why" questions:

- The canonical rendering writes bytes directly (no intermediate astjson values) —
  this path is the hit path's hot loop and was profiled into this shape.
- The rendering rules mirror the fetch input rules exactly:
  conditioned fields skip non-matching types, null/absent key fields abort,
  and an aborted key means "this item is simply not cached", never an error.
- Numbers are unified with strings of the same literal (`1` == `"1"`),
  but no float parsing happens (`1` != `1.0`) — a conservative extra miss
  beats corrupting a 64-bit integer beyond 2^53.
- The `args` digest exists to stop interleaved traffic with different argument values
  from replacing each other's entries on every write (the "ping-pong" problem).
  It is a digest of sorted normalized field PATHS with per-field value-hash suffixes,
  so it is order-, alias-, and variable-name-independent.
- The format version (`cacheFormatVersion = "v1"`) leads the key AND its hashed preimage.
  Changing anything about the layout or envelope means bumping it,
  which makes every old entry unreachable instead of misreadable.
  There is deliberately no migration story — caches refill.
- Root-field keys hash `{TypeName}.{FieldName}:{name-sorted variables}`.
  Query text is excluded so alias variants share entries;
  this is safe because operations are normalized with variable extraction,
  so literals cannot hide in the query text.
  KNOWN BUG: the variable set is the request's, not the fetch's —
  two calls of one cached root field in one operation collide on one key,
  and client variable names fragment entries (see `6-open-issues.md`).

### 3.4 Transforms and coverage (`cache/transform.go`, `coverage.go`)

Values are stored NORMALIZED and served DENORMALIZED:

- `normalizeToSchema` (write path) rewrites response keys to schema names
  (via `OriginalName`) with argument suffixes folded in
  (`stockHistory(days:3)` → `stockHistory_<16-hex>`),
  recursively, preserving unselected fields (`__typename`, key fields) as-is.
- `denormalizeToSelection` (serve path) rebuilds the value
  in the requesting operation's alias shape, in selection order,
  appending cached-only extras so no data is lost on write-back.
  It always builds a fresh transaction-owned value,
  which doubles as the aliasing-safe copy for the splice.
- `normalizedFieldName` is the single derivation both walks and the coverage walk share —
  the three key spaces cannot diverge because there is only one function.

`covers` is the always-on serve guard:
an entry is served only if it contains every field of the fetch's ProvidesData tree,
null accepted only where the schema allows it,
conditioned fields checked against the value's own `__typename`.
Because stored names carry argument suffixes,
an argument-mismatched field fails coverage — a miss, never a wrong hit.

### 3.5 The envelope (`cache/envelope.go`)

Every L2 value is `{"data":…,"cc":{"ttl":…,"created":…,"scope":…}}`.
The `cc` record makes an entry self-describing:
remaining freshness is computable from the entry alone
(`ttl - (now - created)`), so the store's own `RemainingTTL` report is optional.
Decode failures of any kind are a miss the next write repairs — never an error.
The negative sentinel is `"data":null` with the same `cc` record.

The envelope used to carry a `types` map as a schema-drift guard;
it was removed 2026-08-10.
**There is currently NO schema-drift protection**:
an entry written before a schema change is served as long as it decodes and covers.
The open answer is deploy-time purging by `type:` tags (see `6-open-issues.md`).

### 3.6 Freshness (`cache/cache_control.go`)

`parseResponseCacheControl` reads `max-age`, `no-store`, `no-cache`, `private`
(first well-formed `max-age` wins, storability flags sticky, malformed dropped),
and `resolveCaching` runs the two-tier ladder:
header `max-age` is the runtime truth, the static cascade the fallback,
`no-store`/`no-cache` kill both layers, `MaxTTL` clamps the winner.
The decision is per fetch RESULT — one HTTP response, one header, one TTL,
shared by every entity entry written from a batch.

Two subtleties:

- `no-cache` maps to "do not store" because this cache cannot revalidate.
- A runtime-only `private` (header says private, statics say public)
  drops the store writes but keeps L1 — inside one request the data is one requester's anyway.

### 3.7 Privacy (`resolveL2Access`, `cache_key_template.go` partitions)

Statically-private fetches key every entry under a partition segment:
sha256 of the requester identity, source-tagged before hashing
(`i:` for the `PrivatePartitionProvider` hook, `h:` for the forwarded-header hash)
so the two sources can never forge each other.
The provider is called at most once per subgraph per request (`hookIdentity` caches it),
because it is an integrator callback that may parse a JWT.

The load-bearing rule: **no identity ⇒ no L2 at all** for that fetch —
no read, no write, sentinel included —
because a shared key would leak one requester's data to everyone.
L1 stays fully functional unpartitioned (one request = one requester),
so privacy costs cross-request reuse, never in-request reuse.
Envelopes record their scope and `servableScope` discards mismatches in both directions —
that guard catches configuration drift between deployments.

### 3.8 Tags (`cache/cache_tags.go`)

Keys are identity and never externally addressable
(they contain hashes of representation values);
tags are the deliberate addressing layer for invalidation.
Every write carries `subgraph:{name}`, `type:{name}:{TypeName}`,
and for entities `entity:{name}:{TypeName}:{digest of the @key subset ONLY}`.
The entity digest excluding `@requires`/args/partitions is the point:
every variant of one entity shares one tag,
so "this entity changed" is a single purge that clears values,
argument variants, private copies, and sentinels alike.
Tags ride on the store `Item`, never inside the envelope —
index maintenance and delete-by-tag are the store implementation's business.

### 3.9 The client cache answer (`cache/cache_response.go`)

`responseAggregate` folds one contribution per fetch at the point where
that fetch's cacheability is DECIDED (including failure paths and prepare give-ups),
which is why it carries its own mutex instead of the transaction lock —
some folds happen outside any transaction.
The rules: min remaining freshness wins, any uncacheable part forces `NoStore`,
an L1-served part contributes nothing (its origin fetch already folded),
a config-less fetch is a zero that cannot stand alone
(an operation the cache never touched has NO policy, not `no-store`),
and `@defer` suppresses the whole answer
(headers ship with the first frame and cannot describe the rest).
The aggregate outlives the request cache
(`SetCacheResponseInfoSource` survives `endCacheRequest`)
so the router can read it after resolution.

### 3.10 Observability (`cache/observer.go`, ART)

`CacheObserver` is composed inside the controller — the loader never sees it.
`TraceObserver` assembles a per-fetch `CacheTrace`
(decision, served-from layer, per-item key/hit/negative/remaining-TTL)
and attaches it to the fetch's ART trace.
Because the trace extension serializes DURING `Resolve`
while `EndRequest` runs after the response is written,
the resolver calls `FlushTraces` right before rendering when tracing is on —
idempotent per handle, same no-arena contract as `EndRequest`.

## 4. Testing: how to run, what exists, how to extend

Commands (run from the module directory, `v2/` or `execution/`):

```sh
gotestsum --format=short -- ./pkg/engine/cache/... -race        # unit suites (v2/)
gotestsum --format=short -- ./cachingtesting/... -race          # e2e suites (execution/)
golangci-lint run ./pkg/engine/cache/...                        # lint, config resolves from repo root
```

The complete file-by-file test map is in [README §7](1-readme.md#7-how-it-is-tested).
The layering logic:

- **Unit suites** (`v2/pkg/engine/cache/*_test.go`) drive the controller directly
  with a fake store that records a full op log,
  under `testing/synctest` so TTL arithmetic is exact
  (sleep past an expiry inside the bubble — instant in fake time).
- **Plan/contract suites** pin plan shapes and the seam contract
  (no-op gates, `Equals` dedup safety, fetch-interface polymorphism).
- **e2e suites** (`execution/cachingtesting`) run the REAL engine
  over real `httptest` subgraph doubles and assert complete responses,
  complete store-op logs, and subgraph request counts.
  The doubles are rule-based (`Rule(match, response)`) and count requests,
  so "zero network on hit" is a first-class assertion.

Adding an e2e scenario:

1. Check whether the committed fixture schema already supports it
   (`execution/cachingtesting/subgraphs/*.graphql`, composed `config.json`).
2. If not, edit the subgraph SDL and rerun `compose.sh` (needs `npx wgc`);
   commit both the SDL and the regenerated `config.json` —
   tests never compose at runtime.
3. Write a new `*_e2e_test.go` file per logical area — never append to an existing one.
   Fresh `FakeStore` per subtest; assert full op logs with `NormalizeStoreOpsClock`;
   pin envelope values literally.
4. Every pinned key digest embeds the format version —
   if you change key material, expect to re-pin digests
   (get the new values from the failure output, they are authoritative).

Conventions that are enforced in review:
full-value equality only, no `.golden` files, no `Contains`,
`t.Context()` everywhere, `require` for preconditions vs `assert` for checks,
fresh test files per unit, self-contained subtests over shared helpers.

## 5. Change recipes

**Add a config knob** —
add the field to `GlobalCacheConfig` (+ pointer twin on `SubgraphCacheConfig` if per-subgraph),
thread it through `Resolve`/`EffectiveSubgraphConfig`,
consume it in `fetch_cache_configurator.go` into `FetchCacheConfig`,
extend `FetchCacheConfig.Equals` AND its per-field mutation rows in
`resolve/cache_config_test.go` (a field missing from `Equals` silently conflates
plans under dedup), and cover it in `cacheconfig` unit tests plus one e2e.

**Add an observability signal** —
extend `resolve.CacheObserver` (interface change: update `TraceObserver`
and the `RecordingObserver` double in `cachetesting/fakes.go`),
fire it from the controller at the decision point,
and pin it in `observer_test.go` plus the relevant e2e op log.

**Change anything about key material or the envelope** —
bump `cacheFormatVersion`.
Old entries become unreachable by construction; that is the migration story.
Then re-pin every digest in the test suites.

**Touch the loader hooks** —
re-read the concurrency comment on `requestCache` first.
Every mutable field is guarded by the external `CacheTransaction` lock;
a new hook must either open a transaction or provably touch only its own data.
Run the resolve and cachingtesting suites with `-race` — they are built to catch this.

**Anything involving `@defer`** —
remember the request cache is SHARED across defer-group loaders,
L1 is the mechanism that makes cross-group reuse work,
and the client cache answer is suppressed for deferred responses.

## 6. Gotchas — the traps that already bit

- **The EndRequest arena trap.**
  `EndRequest` (and `FlushTraces`) run after the request arena is released.
  Dereferencing any arena-owned `astjson.Value` still on a handle
  (`ItemCacheState.Item`, `FromCache`, shadow stash values) reads reused memory.
  Everything consumed at request end must be plain heap data — strings, durations, copied bytes.
  The contract is written on `RequestCache.EndRequest`; take it literally.
- **Structural copies at L1 boundaries.**
  Values entering and leaving L1 are `StructuralCopy`-isolated.
  Skipping a copy "because it looks safe" lets a later merge mutate a cached value in place;
  the corruption shows up in an unrelated request's response.
- **`Equals` and plan dedup.**
  Fetch dedup compares `FetchCacheConfig` values.
  A field added to the config but not to `Equals` means two fetches with different
  cache policies can be merged into one — silently, and only under specific plans.
- **Stale LSP diagnostics.**
  This codebase's history includes several rounds of diagnostics claiming
  compile errors that did not exist on disk.
  Trust `go build` / `go vet`, not the editor overlay.
- **Tabs vs the Edit tool.**
  Go files use tabs; anchor edits on unique in-line substrings,
  write new code with any indentation, and `gofmt -w` after.
- **The subgraph header is per HTTP response.**
  Batch entity fetches get ONE `Cache-Control` for N entities;
  do not design anything assuming per-entity freshness from the origin.
- **No schema-drift guard exists** (§3.5).
  Do not assume stale-shaped entries are detected; they are not, by design, for now.

## 7. Debugging toolbox

- **The op log is the ground truth.**
  `cachetesting.NewFakeStore()` records every `GetMany`/`SetMany` with keys, hit flags,
  values, TTLs, and tags; most questions ("why did this miss?") are answered by
  printing `store.Ops()` in a scratch test.
- **ART traces**: enable the trace extension in a cachingtesting scenario
  (see `art_e2e_test.go`) to see the cache section per fetch —
  decision, layer, per-item keys and hits — exactly as an operator would.
- **Shadow mode** answers "would the cache have been correct?" in production shape:
  reads and compares without ever serving.
- **Recording controllers** (`cachetesting.NewRecordingController`) script decisions
  per fetch path and record every hook call with its inputs —
  the tool for loader-seam questions.
- When a key mystifies you, remember the L1 key IS the readable preimage;
  log it instead of reverse-engineering the L2 digest.

## 8. Current state and open work

Branch state at handover (2026-08-11):
the feature is complete in this repository and lives on `tmp-caching-squashed`
(one squashed feature commit plus follow-ups; the original 13-commit history
is preserved on `tmp-caching`).
Nothing is pushed.

Working documents in this folder:

- [6-open-issues.md](6-open-issues.md) — open design notes and known bugs:
  the root-field cache-key bug (§3.3; fix it before enabling root-field caching broadly),
  schema-deploy invalidation-need detection (now the only drift answer, see §3.5),
  `s-maxage` support (needs a second freshness number through the pipeline),
  entity-interface caching verification (fixtures missing; key renderer analysis done).
- [5-testing-on-js-subgraphs.md](5-testing-on-js-subgraphs.md) — findings from running the engine against live
  Apollo-federation JS subgraphs (what works, what does not).
- [4-redis-adapter-report.md](4-redis-adapter-report.md) — the researched design for the router-side Redis store
  (tag indexes, delete-by-tag, cluster behavior, implementation checklist).

Remaining work lives in the **router repository** by design:

- The Redis `Store` implementation (see `4-redis-adapter-report.md`).
- The invalidation endpoint bound to the tag vocabulary.
- HTTP emission of `Cache-Control` / `Cache-Tag` from `CacheResponseInfo`.
- Composition carrying the `@cache` directive to the engine configuration.

Known dead code: `FetchCacheConfig.PopulateL2OnMutation` and `MutationTTLOverride`
are declared and compared in `Equals` but never set or read — delete or implement.
