# Caching Implementation — Coding Guidelines

These are the rules every implementation session for the caching port MUST follow.
This file is the STANDING CONTRACT for Fable (Claude), who executes the plan end to end — re-read it at the start of every session and obey it verbatim.
The maintainer-feedback revision is FOLDED IN throughout (there is no override section to apply).
Ground truth, in priority order:

- `docs/caching/PLAN.md` + the task file you were handed (`docs/caching/tasks/NN-*.md`) — the contract for your session.
- This file.
- The RFC specs in `docs/caching/specs/` — background and rationale only; the task files record every deliberate deviation.
- The actual source: this branch, the base branch worktree (first-pass implementation, reference only), the `caching-base` worktree (OLD implementation).

Target: Go 1.25; code under `v2/` and `execution/`.

---

## 0. Workflow (single agent)

- Fable (Claude) does the orchestration, the coding, the tests, the self-review, and the progress tracking; there is no separate coding agent.
- Every task comes from `docs/caching/tasks/NN-*.md` with predefined, verifiable success goals (its ACCEPTANCE CRITERIA).
- Follow the execution protocol in PLAN §2: orient → implement → verify with real command output → self-review against the task's reviewer guidance → record in `PROGRESS.md` → commit gate.
- Update `PROGRESS.md` BEFORE ending any session; it and git history are the only durable state across sessions.
- Commit only under the approval granted in the kickoff prompt; absent a standing grant, finished code + passing tests means "ask for review", not "commit".

---

## 1. Modern Go (target 1.25) — do NOT reinvent the stdlib

Use modern idioms; legacy patterns are review-rejections.

- `any` (never `interface{}`).
- `slices`, `maps`, `cmp` for collection work (`slices.SortFunc`, `slices.EqualFunc`, `maps.Clone`, `cmp.Or`) — already used across the engine.
- `min`, `max`, `clear` builtins; `for i := range n`; range-over-func where it reads cleanly.
- `time.Since` / `time.Until` for elapsed/remaining math (TTLs, freshness).
- `errors.Is` / `errors.As` / `errors.Join`.
- `strings.Cut` instead of `Index`+slice.
- Typed `atomic.Bool` / `atomic.Int64` / `atomic.Pointer[T]`.
- `wg.Go(fn)` instead of manual `Add`/`Done`.
- Before writing ANY helper, assume the stdlib already has it — search, then reuse.
- When in doubt about an API signature, look it up; do not guess.

---

## 2. Search before you write — the mapped homes

Reuse these; do NOT hand-roll equivalents:

- Hashing (cache keys): `github.com/cespare/xxhash/v2` via `pkg/pool/hash64.go` (`pool.Hash64.Get()`/`.Put()`); follow the loader's existing batch-key hashing pattern.
- JSON: `github.com/wundergraph/astjson` (`MarshalTo`, `ParseBytesWithArena`, `MergeValuesWithPath`, `SetValue`, the `Type*` constants). astjson has NO built-in Hash/Equal — key hash = `MarshalTo` + xxhash.
- Arena: `github.com/wundergraph/go-arena`; build the `CacheTransaction` on top of it; do NOT introduce a new allocator.
- Buffers: `pkg/pool/bytesbuffer.go` / `pkg/pool/fastbuffer.go`.
- Sets: plain `map[string]struct{}` (repo convention; no generic Set type).
- Representation-variable building: the shared `plan/representationvariable` package (task 01) — never a copy.
- Test-side astjson building: `pkg/fastjsonext`; JSON normalization in tests: `compactJSONForAssert`.
- Clock: there is NO clock abstraction and we do NOT add one.
  Production code calls real `time.Now`/`time.Since`/`time.Until` directly — no injectable `now`, no clock seam.
  Time-dependent tests use the Go 1.25 stdlib `testing/synctest` (§4.5).
- DRY WITHIN the caching code too: before adding a helper, grep the caching packages for one an earlier task already added; when two tasks need the same predicate, keep ONE.

If you believe nothing exists, say so explicitly in the commit notes so the reviewer can double-check.

---

## 3. Architecture rules (folded-in maintainer feedback)

- ONE COMMON caching package: all cache logic lives in `v2/pkg/engine/cache` (controller with modular L1/L2/partial, key templates, transforms, pass logic, observer) + `cachetesting`.
  Contract types stay in `resolve`; `plan/cacheconfig` is the leaf policy package; `plan`/`postprocess` hold thin shims only.
- EXTENDING EXISTING PLANNERS/VISITORS IS ALLOWED — there are no forbidden files.
  Prefer NEW single-responsibility caching visitors/passes; extend an existing one when that is the simpler, clearer path.
- NO `switch` over concrete fetch types (`*SingleFetch`/`*EntityFetch`/`*BatchEntityFetch`): use the `Fetch` interface methods (`CacheConfig()`, `SetCacheConfig`, entity/batch predicates).
  Improve OTHER fetch-type-switch sites you encounter in code you touch.
- All astjson mutation happens inside a held `CacheTransaction` (`Begin()` … ops … `Commit()`); one transaction per hook = one `DataBuffer.Lock` acquisition; never from the off-lock load phase.
- L1 stores `*astjson.Value` (never `[]byte`); only L2 marshals; parse each response ONCE; L1 and L2 SHARE the same keys (derive once).
- Normalize to schema field names (+ argument-suffix keys) on WRITE; denormalize to the query's aliases on READ.
- NAMES: plain, immediately understandable — `cacheKeyBuilder`, `fetchCacheConfigurator`, `ConfigureCaching`, `CacheTransaction`.
  No "freezer"/"stamper" vocabulary anywhere.
- REFACTOR IN PLACE — never copy-paste to modify; when extracting code into a reusable package, MOVE THE TESTS too.
- Surgical-change discipline: touch only what the task requires; match existing style; do not "improve" adjacent code beyond the sanctioned fetch-switch cleanups.
- Dead-code hygiene across the task chain: a task that supersedes an earlier helper DELETES it in the same commit.

---

## 4. Tests

### 4.1 Package placement

- NEVER append tests to an existing `*_test.go` file; group tests in dedicated files/packages per logical unit.
- Controller/transform/pass unit tests live in `v2/pkg/engine/cache` (in-package where unexported access is needed — the engine convention — noted in the commit).
- Plan-driven / e2e tests live in the EXECUTION module (built on `execution` + the `federationtesting` approach; `wgc`/cosmo-proto assets live there and `v2` must NOT take that dependency).
- Drive the loader through the PUBLIC `resolve` entry points; assert seams OBSERVABLY (recording fakes, store-op logs, `-race`) — no `export_test.go` bridge.

### 4.2 Structure to adopt

- testify: `require.*` for aborting preconditions, `assert.*` for checks; look the API up before writing assertions.
- Table tests + descriptive `t.Run`; scenario-matrix IDs in subtest names (e.g. `"[E3] backfill all after response"`).
- `t.Context()` (never `context.Background()` in tests); `t.Helper()` in harness functions.
- gomock for interface fakes where the repo already does; reuse the existing `MockDataSource`/`mockedDS` helpers.

### 4.3 Assertions

- Assert the FULL value with `assert.Equal` — the entire response string, struct, map, slice, key, call log, store-op log — inline, even when long.
- NEVER `assert.Contains`, `assert.JSONEq`, or fuzzy comparisons (`GreaterOrEqual` etc.).
- NO `.golden` snapshot files.
- Non-deterministic bits are NORMALIZED first (synctest's fixed clock, `compactJSONForAssert`, sorting free-ordered records with `slices.SortFunc`), then asserted structurally.
- NO placeholder tests; every test asserts real, worked-out expected values.

### 4.4 Standing test gates

- NO-OP gates: with no caching configured, plans AND runtime behavior are byte-identical to the baseline — every task keeps both green.
- `ProvidesData` fidelity gate: P1 trees match the OLD branch's trees (task 05).
- Key-builder aliasing gate: mutate the source `FederationMetaData` after building and re-assert equality.
- Determinism: plan twice, byte-identical.
- Ports are tested ADVERSARIALLY: a verbatim port carries the OLD bugs; add rows beyond the OLD test set (partial overlap, irrelevant providers, empty unions).

### 4.5 Time — `testing/synctest`, never a custom clock

- Wrap time-dependent bodies in `synctest.Test(t, func(t *testing.T) { … })`; production keeps calling real `time` functions, which the bubble fakes.
- Test TTL expiry by sleeping PAST the TTL inside the bubble (instant in fake time).
- Order concurrent/defer fetches with GATE CHANNELS + `synctest.Wait()` — NEVER latency sleeps; sleeps exist only to advance fake time.
- Bubble constraints: start ALL goroutines inside the test body; ONLY in-process fakes (no sockets).

### 4.6 Federation fixtures — real wgc composition, no hand-written plans

- Subgraphs are SDL files + `graph.yaml`, composed by real `wgc` via `compose.sh`, with the composed `config.json` COMMITTED; re-running `compose.sh` is the composability guard (cross-validate with rover where available).
- Never hand-write `plan.Configuration` or fetch-tree literals; obtain plans from the committed config through the config factory + the real planner (the task 04 harness).
- Composition is the ONE network step: run `compose.sh` explicitly (it needs `npx wgc`, so expect a permission prompt) only when fixtures change, commit the resulting `config.json`, and build/test against the committed artifact.

---

## 5. Markdown

- One sentence per line in all `.md` files; break long sentences at commas.
- Applies to prose, bullets, and plan docs — not code blocks or inline code.

---

## 6. Scope guardrails (do NOT build these)

- The `@requestScoped` DIRECTIVE FEATURE is REMOVED entirely — not a pass, not a provider method, not a config field.
  (The request-LIFETIME shared L1 store is retained and is NOT that feature.)
- Subscription/mutation caching, root-field→entity L1 promotion, and the cosmo `FromFederation` shim are out-of-core follow-ups.
- Caching config is SELF-CONTAINED: no federation types or pointers at runtime; `@key` is a plan-time input frozen by value once, in `cacheKeyBuilder` only.
- `CacheKeySpec` is MULTI-KEY, best-effort: none required; render what you can at lookup, backfill the rest at write.

---

## 7. Implementation order (mandated)

Follow `PLAN.md` §5: Structure (01–04) → L2 entities (05–12) → L2 root fields incl. isolation (13–14) → entity-cache reuse (15) → L1 (16–18) → partial + ART (19–20).
Each task is a reviewable increment with its own tests; cross-reference your task file for the acceptance goals.

---

## 8. Definition of done (per commit)

- PROBLEM stated: the commit notes name the problem and the task-file goal it satisfies (link the task file and relevant spec sections).
- TESTS: new tests in a dedicated, logically-grouped file; full-value `assert.Equal`; deterministic; the existing suite still passes.
- IMPLEMENTATION: minimal, surgical, modern-Go, reuse-first; every changed line traces to the task.
- QUALITY: every new type and every important function carries a responsibility doc comment; no very large functions — decompose; complex logic carries explanatory inline comments; every intentional error-swallow/best-effort skip and every externally-lock-guarded field carries a WHY comment.
- NOTES: any design decision or deviation written down (one sentence per line) with short reviewer guidance on what to scrutinize.
- No commit without explicit human approval.

---

## 9. CI / lint (match CI exactly, for every module you touch)

CI runs `golangci-lint` (v2) SEPARATELY on `v2/` and `execution/`; a commit touching both must be clean in BOTH.
Enabled: `errcheck`, `govet`, `ineffassign`, `staticcheck`, `bodyclose`, `embeddedstructfieldcheck`, `tparallel`, `modernize`, plus `gci` and `gofmt` (`unused` disabled).
Known recurring failures — avoid up front:

- `gci` import grouping: `standard` → `default` (third-party) → `prefix(github.com/wundergraph)` → `prefix(github.com/wundergraph/graphql-go-tools)`, blank-line-separated (`astjson` and `graphql-go-tools` are DIFFERENT groups).
  Run `gci write -s standard -s default -s "prefix(github.com/wundergraph)" -s "prefix(github.com/wundergraph/graphql-go-tools)" <files>` before finishing.
- `embeddedstructfieldcheck`: blank line between an embedded field and regular fields.
- `staticcheck QF1012`: `fmt.Fprintf(&b, …)`, never `b.WriteString(fmt.Sprintf(…))`.
- CI caps duplicate reports (`max-same-issues: 3`) — fix EVERY occurrence in your diff, not just the printed ones.
- Emulate locally: `golangci-lint run` from each touched module's directory (if your local version lacks `modernize`, strip that line from a copy of the config and run the rest).

Environment realities:

- `compose.sh` needs network (`npx -y wgc@latest`); run it as an explicit step when fixtures change and commit `config.json` — never regenerate it implicitly inside a test run.
- The `execution` module resolves local `v2` via `go.work`; run its tests with `cd execution && go test ./...`.
- Long test runs (`-race`, both modules) belong in background shells; verify output, do not assume success.

---

## 10. Quick checklist (run before handing back)

- [ ] Modern Go idioms; no reinvented stdlib; reused the cited packages.
- [ ] No `switch` over concrete fetch types; no "freezer"/"stamper" names; all logic in `engine/cache` per the packaging rules.
- [ ] All astjson mutation inside a held `CacheTransaction`; one per hook.
- [ ] Tests in dedicated files; full-value `assert.Equal` only; no `.golden`, no `Contains`/`JSONEq`/fuzzy; no placeholders.
- [ ] Time via `testing/synctest`; ordering via gates, never latency.
- [ ] Fixtures wgc-composed and committed; no hand-written plans.
- [ ] Both NO-OP gates still green.
- [ ] Doc comments on every new type and important function; WHY comments on intentional skips and externally-locked fields.
- [ ] No helper duplicated across caching files; no dead code left behind.
- [ ] Stayed in scope (no `@requestScoped`, no subscription/mutation caching).
- [ ] Lint-clean per §9 in EVERY touched module.
- [ ] Markdown one sentence per line.
