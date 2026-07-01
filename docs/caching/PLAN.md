# Caching port — unified implementation plan

Status: authoritative execution plan for the clean re-implementation.
Branch under change: `caching-fable-on-defer` (branched from the defer branch BEFORE the first-pass caching commits), all code under `v2/` (Go 1.25) plus the `execution/` module for integration tests.

This plan supersedes the first-pass PLAN.md on the base branch.
The maintainer feedback (formerly the "R2" overlay) is FOLDED IN here — there is no override layer to mentally apply.
Where this plan conflicts with the RFC specs in `specs/`, THIS PLAN WINS; the specs are background reference and design rationale, not the contract.

## 1. Ground truth, in priority order

1. This `PLAN.md` and the task files in `tasks/` (one file per subtask; each is a self-contained work order), plus `PROGRESS.md` (the live execution state — where any session resumes from).
2. `CODING_GUIDELINES.md` (re-read at the start of EVERY implementation session).
3. The RFC specs in `specs/` (RFC-1 loader abstraction, RFC-2 caching planner, RFC-3 root-field isolation, RFC-1 testing appendix) — background and rationale; superseded by the task files where they differ (the task files record every deliberate deviation).
4. The actual source: this branch (`v2/`, `execution/`), the base branch worktree (`feat/eng-7770-add-defer-support-part-4`, which carries the FIRST-PASS implementation — consult it as a reference, do not cherry-pick blindly), and the OLD implementation (`caching-base` worktree).

The scratchpad DOSSIER the RFCs cite (`scratchpad/caching-rfc/DOSSIER.md`) is NOT committed and is NOT required to execute this plan; every load-bearing fact from it has been folded into the task files.

## 2. Workflow and execution protocol (single agent: Fable)

Fable (Claude) executes this plan END TO END — implementation, tests, verification, self-review, and progress tracking.
There is no separate coding agent.

Per task, in order:

1. ORIENT: read `PLAN.md`, `PROGRESS.md`, `CODING_GUIDELINES.md`, and the task file; open the referenced spec sections only where the task file points at them.
2. IMPLEMENT the task exactly as scoped; stay surgical; test-first where the task defines behavior (write the failing test, then make it pass).
3. VERIFY: run the task's tests, the full affected suites, and `golangci-lint` for EVERY touched module (v2 AND execution); every ACCEPTANCE CRITERIA checkbox must be proven with real command output, never assumed.
4. SELF-REVIEW against the task's "Reviewer guidance" section and both no-op gates before declaring the task done.
5. RECORD: update `PROGRESS.md` (status, commit hash, deviations, anything the next task must know) BEFORE moving on or ending the session.
6. COMMIT: one reviewable commit per task (or per commit split the task file defines), linking the task file, with notes per CODING_GUIDELINES §8.
   Commit only under the approval granted in the kickoff prompt; absent a standing grant, stop at each task boundary and ask for review.

Continuity rules (what makes follow-through survive session and context boundaries):

- `PROGRESS.md` plus git history are the ONLY durable state; never rely on conversation memory across sessions.
- Never end a session mid-task without writing the exact stopping point and the next concrete step into `PROGRESS.md`.
- A task's status is what `PROGRESS.md` says; if git reality and `PROGRESS.md` disagree, reconcile `PROGRESS.md` against git FIRST, then continue.
- Execute tasks in the §5 dependency order unless `PROGRESS.md` records an approved re-ordering.
- Never skip a failing acceptance criterion to move on: fix it, or record a precise blocker in `PROGRESS.md` and stop for input.
- The kickoff prompt is idempotent by design: "read PROGRESS.md, resume the next incomplete step" always lands in a well-defined state.

## 3. Architecture overview (the abstractions, post-feedback)

Runtime (loader side):

- The loader talks to ONE mode-blind working surface, `RequestCache` (`PrepareFetch` / `OnFetchSkipped` / `OnFetchResult` / `EndRequest`), obtained lazily from a `CacheController` set on `Context` (mirroring `SetAuthorizer`).
- The loader branches on NOTHING but the returned `Decision` (`Fetch`, `SkipFullHit`, `FetchPartial`, `FetchShadow`) and a nil-check on the opaque two-level `*FetchCacheHandle`.
- All astjson reads/merges/writes by the cache happen inside an explicit `CacheTransaction`: `tx := arena.Begin()` … ops on `tx` … `tx.Commit()`.
  One transaction per hook == one `DataBuffer.Lock` acquisition (this replaces the RFC-1 `MergeArena`/`MergeSession` naming with the same locking semantics).
- Per-fetch config is a self-contained, federation-free `*FetchCacheConfig` on the fetch; nil means "not cached".
  Fetches expose it polymorphically via methods on the `Fetch` interface (`CacheConfig()`, `SetCacheConfig(...)`, entity/batch predicates) — NO `switch` over concrete fetch types.
- L1 stores `*astjson.Value` in memory (never marshals); only L2 marshals/unmarshals; each response is parsed ONCE; L1 and L2 SHARE the same derived keys (derive once, use for both layers).
- Values are NORMALIZED to schema field names (with argument-suffix keys) on write and DENORMALIZED back to the query's aliases on read, so a value cached by one fetch is reusable by another with different aliases/args.
- Partial fetching is IN CORE: resolve layer by layer — serve what L1 has, look up the still-missing keys in L2, fetch only the keys still missing from the subgraph, then splice + realign (batch: refetch only the missing representations).
- A `CacheObserver` port quarantines analytics/trace/shadow-compare off the lookup/write surface; verbose caching behavior (L1/L2, hits/misses, shadow compares, backfills, key derivations, remaining TTLs) is exposed through ART.

Plan side (producer):

- A leaf policy package `plan/cacheconfig` (`CachingConfiguration`, the `*CachePolicy` structs, `CacheConfigProvider`), reached via `dataSourceConfiguration[T].Caching()`.
- One plan-time visitor, `cacheProvidesDataVisitor` (P1), run as a gated SECOND, filter-free walk on the `planningWalker` after the main walk, building the per-fetch `ProvidesData` tree (alias→`OriginalName`, `CacheArgs`, entity boundaries).
- Postprocess passes over the finished fetch tree: `cacheKeyBuilder` (the SOLE federation reader; freezes every resolvable `@key` set into multi-key `CacheKeySpec.Candidates` by value) and `fetchCacheConfigurator` (assembles + sets `*FetchCacheConfig` on the concrete fetch types after `createConcreteSingleFetchTypes`), orchestrated by a thin `ConfigureCaching` facade holding the single no-op gate; then `optimizeL1Cache` narrows `cfg.L1` cross-tree.
  (These are the RFC-2 "freezer"/"stamper" under plain names.)
- Per-root-field cache isolation is IN CORE: sibling query root fields on the same datasource with different cache configs are split into separate fetches during path building (`path_builder_visitor.go` MAY be edited; there are no forbidden files).
- Public entry point: the engine `Configuration` gains `SetCaching(...)` alongside `SetDataSources`/`SetFieldConfigurations`; `postprocess.EnableCaching(...)` stays an internal detail.

Packaging:

- Contract types (`FetchCacheConfig`, `CacheKeySpec`, `Decision`, the handle, the hook inputs, `CacheTransaction`) live in `resolve` (the loader references them; `resolve` imports no cache logic).
- ALL cache logic — controller (L1/L2/partial as clean, separable modules), key templates and rendering, normalization transforms, the postprocess pass logic, observer/ART glue — consolidates into ONE common package `v2/pkg/engine/cache` (plus its `cachetesting` support subpackage).
- `plan` keeps only the P1 visitor and the isolation seam (they need walk-time planner context); `postprocess` keeps only thin facade calls into `engine/cache`.

## 4. Invariants every task must protect

- RUNTIME NO-OP: with no `SetCacheController`, the loader path is byte-identical to today.
- PLANNER NO-OP: with no caching configuration supplied, postprocessed plans are byte-identical to today.
  Phase 0 as a whole is a pure no-op end to end; every later phase keeps both gates intact for integrators who do not opt in.
- WRITE GATE: a fetched value is persisted only when `!FetchFailed && !HasErrors && ResponseData != nil && Type() != Null`; `EmptyEntity` is the one non-failure that still writes (the negative sentinel). The gate can never reduce to `!HasErrors`.
- KEY FIDELITY: cache keys derive from the canonical pre-injection input + header hash; read key == write key, byte-identical; one `CacheKeyTemplate` per candidate is the sole source of read/write/invalidate keys.
- LOCK DISCIPLINE: one `CacheTransaction` (one `DataBuffer.Lock` acquisition) per hook; the arena is never touched outside a held transaction; never from the off-lock load phase.
- PLAN CACHE SAFETY: only static, request-independent config is written at plan time; per-request key material derives at runtime.

## 5. Dependency order at a glance

```
Phase 0 — STRUCTURE (pure no-op end to end)
  01 representationvariable extraction                  (no deps)
  02 runtime contract + loader seam + transaction       (no deps; independent of 01)
  03 planner wiring + cacheconfig + engine SetCaching   (deps: 01, 02)
  04 test infrastructure: caching subgraphs + harness   (deps: 02, 03)

Phase A — L2 ENTITIES
  05 ProvidesData visitor (P1)                          (deps: 03, 04)
  06 entity cache config: key builder + configurator    (deps: 01, 03, 05)
  07 entity L2 controller core (single candidate)       (deps: 02, 04, 06)
  08 multi-key: freshness, reorder, backfill            (deps: 07)
  09 store-time normalization + argument-suffix keys    (deps: 07)
  10 batch entity caching                               (deps: 08)
  11 negative caching                                   (deps: 07)
  12 shadow mode                                        (deps: 07)

Phase B — L2 ROOT FIELDS
  13 root-field L2 (plan + runtime)                     (deps: Phase A core: 07, 09)
  14 per-root-field isolation (RFC-3, in core)          (deps: 13)

Phase C — ROOT FIELDS RE-USING THE ENTITY CACHE
  15 entity-cache reuse via EntityKeyMappings           (deps: 08, 13)

Phase D — L1 CACHING
  16 optimizeL1Cache cross-tree narrowing               (deps: 06)
  17 request-lifetime shared L1 store (astjson values)  (deps: 09, 16)
  18 defer + concurrency scenario coverage              (deps: 17; needs the 04 fixtures)

Phase E — PARTIAL + OBSERVABILITY (in core per maintainer feedback)
  19 partial fetching (partial cache load + batch realign)  (deps: 10, 17)
  20 ART observability (verbose caching trace)              (deps: 12, 17)
```

Each task is a reviewable increment with its own tests; no task may regress the no-op gates.

## 6. Task index

| # | File | Phase | Deliverable |
|---|---|---|---|
| 01 | [tasks/01-representation-variable-extraction.md](tasks/01-representation-variable-extraction.md) | 0 | Shared exported `representationvariable` package; `graphql_datasource` refactored in place |
| 02 | [tasks/02-runtime-contract-and-loader-seam.md](tasks/02-runtime-contract-and-loader-seam.md) | 0 | Contract types, `CacheTransaction`, `Fetch` interface methods, no-op loader seam |
| 03 | [tasks/03-planner-wiring-and-engine-config.md](tasks/03-planner-wiring-and-engine-config.md) | 0 | `plan/cacheconfig`, postprocess pass skeletons, `Configuration.SetCaching(...)` |
| 04 | [tasks/04-test-infrastructure.md](tasks/04-test-infrastructure.md) | 0 | Dedicated caching subgraphs, execution-module e2e framework, `cachetesting` fakes |
| 05 | [tasks/05-provides-data-visitor.md](tasks/05-provides-data-visitor.md) | A | P1 visitor (second filter-free walk) + `*FetchInfo`-keyed side-table |
| 06 | [tasks/06-entity-cache-configuration.md](tasks/06-entity-cache-configuration.md) | A | `cacheKeyBuilder` (multi-key freeze) + `fetchCacheConfigurator` entity arm |
| 07 | [tasks/07-entity-l2-controller-core.md](tasks/07-entity-l2-controller-core.md) | A | Entity L2 controller: lookup → coverage → decision → gated write → flush |
| 08 | [tasks/08-multi-key-freshness-reorder.md](tasks/08-multi-key-freshness-reorder.md) | A | Multi-candidate render/lookup, freshness selection, reorder, backfill |
| 09 | [tasks/09-store-normalization.md](tasks/09-store-normalization.md) | A | Normalize-on-write / denormalize-on-read + argument-suffix keys |
| 10 | [tasks/10-batch-entity-caching.md](tasks/10-batch-entity-caching.md) | A | Batch entity shapes, per-item state, end-to-end L2 hit |
| 11 | [tasks/11-negative-caching.md](tasks/11-negative-caching.md) | A | Empty-entity null sentinel write + hit, TTL expiry |
| 12 | [tasks/12-shadow-mode.md](tasks/12-shadow-mode.md) | A | `DecisionFetchShadow`: read-never-serve, compare-before-write |
| 13 | [tasks/13-root-field-l2.md](tasks/13-root-field-l2.md) | B | Root-field policies, all-or-nothing safety net, root-field L2 runtime + shadow asymmetry |
| 14 | [tasks/14-root-field-isolation.md](tasks/14-root-field-isolation.md) | B | Per-root-field fetch isolation in the path builder |
| 15 | [tasks/15-entity-cache-reuse.md](tasks/15-entity-cache-reuse.md) | C | `EntityKeyMappings` derivation + by-key root fields served from the entity cache |
| 16 | [tasks/16-optimize-l1-pass.md](tasks/16-optimize-l1-pass.md) | D | Cross-tree L1 narrowing pass |
| 17 | [tasks/17-l1-runtime-store.md](tasks/17-l1-runtime-store.md) | D | Request-lifetime shared L1 (`*astjson.Value`, shared keys) across defer groups |
| 18 | [tasks/18-defer-concurrency-coverage.md](tasks/18-defer-concurrency-coverage.md) | D | Defer + concurrency scenario rows (N/M), incl. the cross-group L1 fixture |
| 19 | [tasks/19-partial-fetching.md](tasks/19-partial-fetching.md) | E | Partial cache load + partial batch realign (`DecisionFetchPartial`) |
| 20 | [tasks/20-art-observability.md](tasks/20-art-observability.md) | E | Verbose caching behavior through ART via `CacheObserver` |

## 7. Resolved decisions (deviations from the RFCs, recorded once)

- D1. The maintainer-feedback revision is folded in; the RFCs are background.
  Every task file states its own deviations inline, so no reader needs to diff spec against plan.
- D2. `MergeArena`/`MergeSession` is renamed and reshaped into `CacheTransaction` (`Begin()` … ops … `Commit()`), identical locking semantics: one transaction per hook, one `DataBuffer.Lock` acquisition, arena untouchable outside a held transaction.
- D3. L2 enablement stays DERIVED from policy TTLs (`TTL > 0 || NegativeCacheTTL > 0` for entities, `TTL > 0` for root fields); no explicit L2 bool is added to the policy structs (closes RFC-2 open question R3).
- D4. `EntityMergePath` is a FIX, not a gap: the loader surfaces the fetch's post-processing merge path into the hook inputs so entity/batch values splice at the correct target (task 07/10); values are never silently spliced at the item root for non-root merge paths.
- D5. Packaging: contract types in `resolve`; ALL logic in the common `v2/pkg/engine/cache` package; `plan/cacheconfig` stays a leaf policy package (avoids the plan↔cache import cycle); `plan`/`postprocess` hold only thin shims.
- D6. Names: `cacheProvidesDataVisitor` (kept), `cacheKeyBuilder` (was "freezer"), `fetchCacheConfigurator` (was "stamper"), `ConfigureCaching` facade, `optimizeL1Cache` (kept). No "freeze"/"stamp" vocabulary in code or docs.
- D7. Testing: NO `.golden` snapshot files; assert COMPLETE responses/plans with full-value `assert.Equal`; the appendix scenario catalog (A–P, M, N rows) is retained as the coverage checklist, re-based onto the new harness; dedicated caching subgraphs (not `commerce` alone) with nesting and TTL/config variance, built on `execution` + the `federationtesting` approach.
- D8. `Fetch` interface methods (`CacheConfig()`, `SetCacheConfig(...)`, entity/batch predicates) replace all `switch`-over-concrete-fetch-type sites in caching code; other fetch-type switches encountered along the way are improved in the same spirit.
- D9. P1 runs as a gated SECOND, filter-free walk on the `planningWalker` after the main walk (the first-pass correction to RFC-2 §9.2); it never re-runs the planning visitor and never rebuilds the plan.
- D10. `EntityKeyMappings` are STRUCTURALLY DERIVED (root-field return type + args matched against the entity `@key` via definition + federation); this branch carries no external mapping config. By-key root-field reuse requires the root-field policy to share `CacheName` with the entity policy (read key == write key).
- D11. The `@requestScoped` directive feature is EXCLUDED entirely (no pass, no provider method, no config field). The request-LIFETIME shared L1 store is retained and is not that feature.
- D12. Subscription/mutation caching and the cosmo `FromFederation` migration shim remain out-of-core follow-ups; ART observability and partial fetching are IN core (Phase E).

## 8. Out-of-core follow-ups (sequenced after Phase E)

- Subscription/mutation caching (`cacheSubscriptionAnnotator`, trigger lifecycle, invalidation).
- Root-field → entity L1 promotion.
- cosmo migration shim (`cacheconfig.FromFederation(...)`).
- Per-defer-group early L2 flush (correctness-neutral latency optimization).

## 9. Completion checklist

- [ ] All 20 tasks merged, each meeting its own ACCEPTANCE CRITERIA.
- [ ] Both no-op gates hold at every commit (runtime byte-identical without a controller; plans byte-identical without caching config).
- [ ] All caching code consolidated per D5; no `switch` over concrete fetch types in caching code (D8).
- [ ] L1 stores `*astjson.Value`; single parse per response; L1/L2 share keys; normalization on write, denormalization on read (tasks 09, 17).
- [ ] Partial fetching works end to end for single and batch fetches (task 19).
- [ ] ART exposes verbose caching behavior (task 20).
- [ ] The full scenario matrix passes: coverage/freshness/reorder, multi-key, negative, shadow, root-field split, entity reuse, partial expiry, alias/argument reuse, defer + concurrency under `-race`.
- [ ] Every function of the modular cache interfaces has a meaningful unit test; every caching use case has a fully-implemented integration test; NO placeholder tests; NO dead code; HIGH coverage.
- [ ] All caching tests assert FULL values with `assert.Equal`; no `Contains`/`JSONEq`/fuzzy anywhere.
- [ ] Every new type and every important function carries a responsibility doc comment; complex logic carries explanatory inline comments.
