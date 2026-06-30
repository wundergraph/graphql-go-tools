# Caching Implementation — Coding Guidelines

These are the rules every coding session for the caching port MUST follow.
This file is INJECTED INTO EVERY CODEX SESSION as the standing contract, so keep it open and obey it verbatim.
Ground truth lives in the two REVISED, verified RFCs and the dossier, which override anything here on a conflict:

- RFC-1 (loader cache abstraction): `docs/caching/specs/2026-06-30-rfc-01-loader-cache-abstraction.md`
- RFC-2 (caching planner): `docs/caching/specs/2026-06-30-rfc-02-caching-planner.md`
- Dossier (file:line evidence): `scratchpad/caching-rfc/DOSSIER.md`
- Conventions survey (what to reuse, do NOT reinvent): `scratchpad/caching-rfc/explore-conventions.md`

Target: Go 1.25, all code under `v2/`.

---

## 0. Collaboration workflow (who does what)

- Claude ORCHESTRATES, REVIEWS, and gives FEEDBACK.
- Codex does ALL the coding.
- Every task arrives with clear instructions and PREDEFINED, verifiable success goals (tests to pass, plan output to match).
- After each Codex pass, Claude reviews against those goals and the RFCs.
- Iterate in fresh Codex sessions until BOTH Claude and Codex agree the predefined goals are met.
- This file is injected into every Codex session, so Codex always has the guidelines in front of it.
- Do not commit without explicit human approval (finishing code plus passing tests means "ask for review", not "commit").

---

## 1. Modern Go (target 1.25) — do NOT reinvent the stdlib

Use modern idioms; legacy patterns are review-rejections.

- `any` (never `interface{}`).
- `slices`, `maps`, `cmp` for collection work (`slices.Sort`, `slices.SortFunc`, `slices.Contains`, `slices.Equal`, `slices.EqualFunc`, `maps.Clone`, `cmp.Or`, `cmp.Compare`) — these are already used across the engine.
- `min`, `max`, `clear` builtins; `for i := range n` for counted loops; range-over-func where it reads cleanly.
- `time.Since` / `time.Until` for elapsed/remaining math (TTLs, remaining-TTL freshness).
- `errors.Is` / `errors.As` / `errors.Join` for error handling (`errors.Is(err, ErrCircuitBreakerOpen)` is the existing pattern).
- `strings.Cut` instead of `Index`+slice.
- Type-safe `atomic.Bool` / `atomic.Int64` / `atomic.Pointer[T]`, not the untyped `atomic.AddInt64` family.
- `wg.Go(fn)` (Go 1.25) instead of manual `wg.Add(1)` + `go func(){ defer wg.Done() }()`.
- JSON tags: `omitzero` for zero-value omission where appropriate.
- Before writing ANY helper, assume the stdlib already has it — search, then reuse.
- When in doubt about an API signature, look it up; do not guess.

---

## 2. Search before you write

Before adding a helper, function, or type, grep `v2/...` for one that already solves it.
The conventions survey already mapped the homes — reuse these, do NOT hand-roll equivalents:

- Hashing (cache keys): `github.com/cespare/xxhash/v2` via the pool `pkg/pool/hash64.go` (`pool.Hash64.Get()` / `.Put()`).
  The loader already hashes batch items exactly the way cache keys should be hashed (`loader.go` `keyGen.Reset(); keyGen.Write(bytes); h := keyGen.Sum64()`) — follow that pattern.
- JSON: `github.com/wundergraph/astjson` — serialize a key/representation with `(*astjson.Value).MarshalTo(dst)`; reuse the existing `astjson.ParseBytesWithArena`, `astjson.MergeValuesWithPath`, `astjson.SetValue`, `astjson.ValueIsNonNull/ValueIsNull`, the `astjson.Type*` constants.
  astjson has NO built-in Hash/Equal — cache-key hash = `MarshalTo` + xxhash; do not write a JSON walker for keys.
- Arena: `github.com/wundergraph/go-arena` (`arena.NewMonotonicArena`, `arena.NewArenaBuffer`, `arena.AllocateSlice[T]`).
  Build the RFC-1 `MergeArena`/`MergeSession` on top of this; do NOT introduce a new allocator.
- Buffers: `pkg/pool/bytesbuffer.go` (`pool.BytesBuffer`) and `pkg/pool/fastbuffer.go` (`pool.FastBuffer`).
- Sets: plain `map[string]struct{}` is the repo convention (no generic Set type exists, do not add one).
- Representation-variable building: `pkg/engine/datasource/graphql_datasource/representation_variable.go` — to be EXTRACTED, not copied (see §3).
- Test-side astjson value building: `pkg/fastjsonext`.
- JSON normalization in tests: `compactJSONForAssert` (`resolve/json_assert_test.go`); do not hand-roll normalization.
- Plan/struct diffing in tests: `github.com/kylelemons/godebug/pretty` (the de-facto snapshot mechanism; no golden files in resolve/postprocess/plan).
- Clock: there is NO clock abstraction in the repo, and we do NOT add one.
  Production code calls real `time` directly (`time.Now`, `time.Since`, `time.Until`, TTL `time.Duration` comparisons) — do NOT introduce an injectable `now func() time.Time` or any clock seam "for testability".
  Time-dependent tests fake the clock with the Go 1.25 stdlib `testing/synctest` instead (see §4.5).

If you believe nothing exists, state that explicitly in the commit notes so the reviewer can double-check before you add a new helper.

---

## 3. Refactor in place — never copy-paste to modify

When you need existing code in a new place, REFACTOR IT IN PLACE and make it multi-purpose, then have both callers use the one implementation.
Copy-paste-and-edit creates two divergent implementations and is a review-rejection.

Worked example (RFC-2 §6.1, mandated):
the representation-variable builder is unexported and lives in `graphql_datasource` (`representation_variable.go`).
Do NOT copy it into the caching code and do NOT reach into another package's private helpers.
Instead:

1. EXTRACT `BuildRepresentationVariableNode` / `MergeRepresentationVariableNodes` (and the internal visitor + merge helpers) into a NEW shared, exported package `v2/pkg/engine/plan/representationvariable` (does not exist yet).
2. REFACTOR `graphql_datasource` IN PLACE to import and call the exported functions instead of its local unexported copies.
3. The caching `cacheKeySpecFreezer` then reuses the SAME builder.

This refactor is its own early structural commit, must preserve `graphql_datasource` behavior, and is guarded by the existing `graphql_datasource/representation_variable_test.go` (which pins the output byte-for-byte).
No import cycle: `representationvariable` imports `plan`/`resolve`/`ast`/`astvisitor`; `plan` does not import it.

Surgical-change discipline applies everywhere:
touch only what the task requires, match existing style, and do not "improve" adjacent code.
RFC-2 additions are ADDITIVE — zero body edits to the five forbidden planner/visitor files (`node_selection_visitor.go`, `path_builder_visitor.go`, `required_fields_visitor.go`, `node_selection_builder.go`, `visitor.go`).

---

## 4. Tests

### 4.1 Package placement

- NEVER append tests to an existing `*_test.go` file, and NEVER drop new test files into an existing package as a grab-bag.
- Group tests by creating SEPARATE, dedicated packages/files for each logical unit.
- The caching IMPLEMENTATION lives in the NEW package `v2/pkg/engine/resolve/cache` (RFC-1 §7.1), so its tests are naturally a new package — put them there, driving through the exported `resolve` API (`SetCacheController`, the loader entry points) wherever that reaches the needed coverage.
- The RFC-2 passes (`cacheKeySpecFreezer`, `cacheProvidesDataVisitor`, `cacheConfigStamper`, `optimizeL1Cache`) are each independently unit-testable units — give each its own test file, named for the unit.
- The shared `representationvariable` package gets its own tests (and the extraction is additionally guarded by the existing `graphql_datasource` test).
- Plan-driven federation tests (real plan -> loader -> fake responses -> cache) do NOT live in `v2`: the `wgc`/config_factory assets live in the EXECUTION module and `v2` must NOT take the cosmo-proto dependency, so those tests live in a NEW test package in the execution module — see §4.6.

DOCUMENTED EXCEPTION (surface this, do not silently break the rule):
the engine packages (`resolve`/`postprocess`/`plan`) are tested IN-PACKAGE — there are ZERO external `_test` packages under `pkg/engine/`, deliberately, so tests can reach unexported types and fakes (conventions survey §a).
RFC-1 wants the loader plus caching unit-tested with fakes to ~100% coverage, which needs unexported access.
So when a test genuinely requires unexported `resolve` internals (the two-level handle fields, the merge-arena impl, loader seam wiring), put it in `package resolve` as its OWN new, clearly-named file (e.g. `cache_controller_test.go`) that groups the cache tests together — this honors "separate new file, logically grouped" while respecting the engine's in-package convention.
Prefer the new `resolve/cache` package and exported API first; only fall back to in-package `resolve` when coverage demands it, and say so in the commit notes.

### 4.2 Structure to adopt (cite, do not invent)

- testify is the assertion library: `require.*` for setup/preconditions that must abort, `assert.*` for the actual checks.
- gomock (`github.com/golang/mock/gomock`) is used for interface fakes, NOT testify/mock.
  For DataSource stubs reuse `mockedDS(t, ctrl, expectedInput, responseData)` and `MockDataSource`/`NewMockDataSource(ctrl)` (`resolve_federation_test.go`); reuse the `TestingTB` abstraction and the closure-style harnesses (`testFn...` at `resolve_test.go`); build fetch trees inline with `Sequence(Single(&SingleFetch{...}))` / `Parallel(...)` / `Single(&BatchEntityFetch{...})` as the loader tests do.
- Table tests: `tests := []struct{ name string; ... }` + `for _, tt := range tests { t.Run(tt.name, ...) }` (or descriptive nested `t.Run` subtests).
- postprocess/plan output is compared as a whole struct with `assert.Equal`, rendered on failure via godebug `pretty`.
- Plan-level integration proof: `datasourcetesting.RunTest` / `RunTestWithVariables` (dot-imported), extended with the caching post-processor; assert the WHOLE postprocessed plan tree.
  For FEDERATION plans do NOT hand-write the input `plan.Configuration`: source it from the real `wgc`-composed config via the config_factory and golden the generated plan (§4.6).
- Use `t.Context()` for context in tests (Go 1.25), not `context.Background()` / `context.TODO()`.
- Look up the testify API (resolve the library and query its docs) before writing assertions — do not guess signatures.

### 4.3 Assertions

- Assert on the FULL value with `assert.Equal` — the entire response/struct/map/slice/key inline, even when long.
- NEVER use `assert.Contains`, `assert.JSONEq` as a substring escape hatch, or fuzzy comparisons (`GreaterOrEqual`, `Less`, etc.).
  (The repo does use `Contains`/`JSONEq` elsewhere; for NEW caching tests, do not.)
- For non-deterministic bits (timestamps, IDs, remaining-TTL), NORMALIZE first (`synctest`'s fixed fake-clock start, `compactJSONForAssert`, structural snapshot) then assert structural equality — never fall back to `Contains`.
- A fuzzy assertion is a smell that the author did not work out the real expected value.

### 4.4 Key test gates from the RFCs (predefined success goals)

- NO-OP golden: with no provider wired, the postprocessed plans are BYTE-IDENTICAL to today, and merging RFC-1 alone changes runtime behavior in ZERO ways.
- `ProvidesData` fidelity gate: golden-compare P1's per-fetch `ProvidesData` trees against the OLD branch's trees (a too-small tree silently disables hits; an arg-blind one serves stale data).
- Freezer: mutate the source `FederationMetaData` after freezing and re-assert equality (proves no pointer aliasing into federation).
- Determinism: run each plan twice and assert byte-identical output.

### 4.5 Time in tests — `testing/synctest`, never a custom clock

Any test whose outcome depends on time (TTL expiry, negative-cache expiry, remaining-TTL freshness, shadow CacheAge, timeouts) uses the Go 1.25 stdlib `testing/synctest` — there is NO custom clock and NO injectable `now` (see §2).

- Wrap the time-dependent body in `synctest.Test(t, func(t *testing.T) { ... })`: it runs in a "bubble" with a FAKE clock that starts at a fixed instant and only advances when EVERY goroutine in the bubble is durably blocked.
- Test TTL EXPIRY by sleeping PAST the TTL inside the bubble (`time.Sleep(ttl + time.Nanosecond)`), which is instantaneous in fake time, then asserting the entry is treated as expired; production still calls real `time.Since`/`time.Until`, which the bubble fakes.
- `synctest.Wait()` blocks until all bubble goroutines are durably blocked — use it to reach a stable point before asserting.
- Constraints for the bubble to work: start ALL goroutines INSIDE the test body so they join the bubble, and use ONLY in-process fakes (the scripted/gated fake datasource, the in-memory store) — NEVER a real network/socket, which is not bubble-aware.
- Order concurrent/defer fetches with CHANNELS or gates plus `synctest.Wait()`, NEVER real-latency sleeps; sleeps are reserved for advancing the fake clock to test TTL (consistent with commit e509453b's gates-not-latency rule).

### 4.6 Federation test fixtures — real `wgc` composition, no hand-written plans

Do NOT hand-maintain a router config and do NOT hand-write `plan.Configuration` / fetch trees for federation tests; both drift silently from the real planner.
Instead REUSE the existing composition + config-factory assets (do not reinvent them) — reference: `execution/engine/config_factory_federation.go`, `execution/engine/config_factory_federation_test.go`, and `execution/engine/testdata/config_factory_federation/` (`account_sdl.graphql`/`product_sdl.graphql`/`review_sdl.graphql`, `graph.yaml`, `compose.sh`, the committed `config.json`).

- DEFINE each test subgraph as an SDL file plus a `graph.yaml` listing `{ name, routing_url, schema.file }`, exactly like `testdata/config_factory_federation/graph.yaml`.
- COMPOSE with REAL `wgc` via a `compose.sh` running `npx -y wgc@latest router compose -i graph.yaml -o config.json` then `jq . config.json`, and COMMIT the resulting `config.json`.
  Re-running `compose.sh` is the composability guard — `wgc` FAILS if the subgraphs do not compose, so a committed `config.json` proves the test subgraphs compose AND that the planner configuration is valid.
- LOAD the committed config at test time exactly as `config_factory_federation_test.go` does: `os.ReadFile(".../config.json")` -> `protojson.Unmarshal(data, &rc)` into `nodev1.RouterConfig` -> `engine.NewFederationEngineConfigFactory(ctx, ...).BuildEngineConfiguration(&rc)`, which yields a valid `plan.Configuration` (real `FederationMetaData` Keys/Requires/Provides) + schema.
- GENERATE plans with the REAL v2 planner + postprocess (including the RFC-2 caching pass) from that `plan.Configuration`, then GOLDEN-snapshot the rendered plan so reviewers SEE it and it cannot drift — never assert against a hand-typed plan literal.
- MODULE BOUNDARY: `config_factory` and the cosmo proto live in the EXECUTION module (`execution/go.mod`), and `v2` MUST NOT take the cosmo-proto dependency.
  So the PLAN-DRIVEN loader+cache tests (real plan -> loader -> fake subgraph responses -> cache) live in a NEW test package in the execution module; obtain `plan.Configuration` via the config_factory (add a small EXPORTED test accessor in the execution `engine` package if `plannerConfig` is unexported — refactor in place per §3), then drive the v2 loader with the golden plan, a fake in-process datasource, and the cache controller.
- The pure controller unit tests (lookup decision, `ProvidesData` coverage, multi-candidate freshness, reorder, multi-key render/backfill, shadow compare, negative/write-back) need NO plan: they stay in the NEW `v2/pkg/engine/resolve/cache` test package, built from CONSTRUCTED astjson inputs + fakes, and use `synctest` for TTL (§4.5).

---

## 5. Markdown

- One sentence per line in all `.md` files (specs, PLAN.md, RFCs, this file).
- Break long sentences at commas onto new lines.
- Applies to prose, bullets, and plan docs — not to code blocks or inline code.
- Write markdown with the native Write tool.

---

## 6. Scope guardrails (do NOT build these)

- The `@requestScoped` DIRECTIVE FEATURE is REMOVED entirely — out of scope, not a v1 or v2 pass.
  (The request-LIFETIME shared L1 store across per-defer-group loaders is RETAINED, but it is NOT the `@requestScoped` feature — do not conflate them.)
- Per-root-field cache isolation (`isolatedRootField`) is its OWN separate RFC (RFC-03), not part of this plan; v1 uses the conservative all-or-nothing decline.
- Caching config is SELF-CONTAINED with no federation types at runtime; federation `@key` info is a plan-time INPUT only, frozen by value once.
- `CacheKeySpec` is MULTI-KEY, best-effort: a list of `CacheKeyCandidate{Representation *resolve.Object}`, none required; render every renderable candidate at lookup, re-render the rest from fresh data at write, backfill all renderable keys.
- Staged to v2 (config bits may be stamped, but do not build the runtime): partial L1 / partial batch realign, walker-inlined analytics, subscription/mutation caching.

---

## 7. Implementation order (mandated)

Build in this exact order; each stage is a reviewable increment with its own tests:

1. STRUCTURE first (RFC-1 contract types + no-op loader seam commit; RFC-2 additive wiring + `representationvariable` extraction).
2. L2 caching for ENTITIES only.
3. L2 caching for ROOT FIELDS.
4. L2 for ROOT FIELDS that RE-USE the entity cache.
5. L1 caching.

(No request-scoped caching at any stage.)

---

## 8. Definition of done (per commit)

Cross-reference the running plan in `docs/caching/PLAN.md` for which commit you are on and its acceptance goals.
A commit is DONE only when all of the following hold:

- PROBLEM stated: the commit message / notes name the problem and the predefined success goal it satisfies (link the PLAN.md item and the relevant RFC section).
- TESTS: new tests in a separate, logically-grouped package/file, full-value `assert.Equal`, deterministic, covering the RFC gates relevant to this commit (§4.4); the existing suite still passes.
- IMPLEMENTATION: minimal, surgical, modern-Go, reuse-first; every changed line traces to the task.
- LIGHTWEIGHT RFC / notes: any design decision or RFC deviation is written down (one sentence per line) so the reviewer can follow it.
- REVIEWER GUIDANCE: a short note on what to scrutinize (e.g. "no federation pointer escapes the freezer", "no-op golden unchanged", "MergeSession is the single lock acquisition").
- No commit without explicit human approval.

---

## 9. Quick checklist (run before handing back)

- [ ] Modern Go idioms; no reinvented stdlib.
- [ ] Searched `v2/...` for an existing helper before adding one; reused the cited packages.
- [ ] Refactored in place (multi-purpose) instead of copy-paste.
- [ ] Tests in a SEPARATE new package/file; in-package `resolve` only where unexported access is required (noted).
- [ ] Full-value `assert.Equal` only; `t.Context()`; testify API verified, gomock fakes reused.
- [ ] Time-dependent tests use `testing/synctest` (all goroutines + only in-process fakes inside the bubble); no custom clock or injectable `now` in production code.
- [ ] Federation fixtures are real-`wgc`-composed (committed `config.json` loaded via config_factory), plans are planner-generated + goldened — no hand-written router config or plan literals; plan-driven federation tests live in the execution module.
- [ ] Markdown is one sentence per line.
- [ ] Stayed in scope (no `@requestScoped`, no per-root-field isolation, no request-scoped caching).
- [ ] Followed the mandated implementation order and the per-commit definition of done; cross-referenced PLAN.md.
