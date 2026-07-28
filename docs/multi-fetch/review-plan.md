# MultiFetch Review Follow-ups — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task, dispatching one Opus subagent per task. Steps use checkbox (`- [ ]`) syntax for tracking. The user manages all commits — no task contains commit steps.

**Goal:** Address all 14 review comments on the MultiFetch branch: close test gaps, fix the pre-existing `representations` collision for all entity fetches, refactor for readability, and rework the architecture toward lazy input rendering with the merge stage relocated after parallel-node creation.

**Architecture:** Three phases. Phase 0 investigates the laziness/placement questions (review items 3, 5, 6, plus item 4 placement and items 7/8 final naming) and produces a design addendum for user sign-off. Phase 1 lands everything architecture-independent — test coverage, the planner collision fix, and runtime refactors — creating the regression net Phase 2 relies on. Phase 2 (detailed only after the Phase 0 addendum is approved) implements the lazy-input/stage-move rework under the constraint that executable plans stay functionally identical, so Phase 1 goldens act as the safety net.

**Tech Stack:** Go, `gotestsum` for all test runs, `datasourcetesting.RunTest` for full-plan goldens, `execution/engine` federation test harness for integration tests.

## Global Constraints

- Run tests with `gotestsum --format=short -- <pkg> -run <TestName>`, never bare `go test`.
- No behavior change for any code path with MultiFetch disabled (existing non-multifetch goldens must stay byte-identical), **except** the item-2 collision fix, which intentionally changes behavior for the currently-broken collision case on all entity fetches.
- Full-equality assertions in all new/rewritten tests: `assert.Equal` on whole strings, `assert.JSONEq` on whole JSON documents. `assert.Contains` only for genuinely unordered fragments, with a comment justifying it.
- All file paths in docs and code comments are repo-relative.
- Working naming decision (finalized at the Phase 0 checkpoint): `MergeableOperation` → `SubgraphOperation`, `NamedVariableFragment` → `SubgraphVariable`. Phase 1 tasks keep the old names; the rename is a Phase 2 task so it lands together with the type's role change.

## Original review comments (reference)

Verbatim user review of the MultiFetch branch, 2026-07-29; item numbers are referenced throughout this plan.

1. We are missing tests when the operation has additional variables passed to the fetches.
2. graphql_datasource: "A duplicate write to the `representations` slot marks the fetch as non-mergeable" — a client variable literally named `representations`. We should handle it for all kinds of entity fetches, not only MultiFetch. *(Clarified: rename the planner-added synthetic variable, keep the client's name.)*
3. `ConfigureFetch()` in the graphql datasource still creates input with a printed query and variables. I would expect this to be lazy when MultiFetch is enabled. It could mean that `generateQueryPlansForFetchConfiguration` also should be lazy. Investigation is needed on how to properly implement it.
4. `buildMergedOperation` is too big; it should be refactored to be small, with meaningful functions representing functional parts — and it feels like it could belong to the asttransform package. *(Clarified: investigate which placement is better; both acceptable; generic AST manipulation feels like a transform thing.)*
5. `splitEntityFetchInput` is a sign that there is no laziness — the initial input was built in the graphql datasource, so operating on string inputs is an inconvenient and messy approach.
6. The `createMultiFetch` postprocessor has all-custom code to build mergeable fetch groups; it feels wired at the wrong place. For example, we could postpone rendering input templates until the end of postprocessing; maybe MultiFetch could be wired after fetch-tree node creation — fetches are mergeable when they are in the same parallel group and are entity fetches (single or batch).
7. `resolve.NamedVariableFragment` — badly named, could be confused with GraphQL fragments; something like `SubgraphVariable` or `SubgraphVariableValue`. *(Clarified: leaning `SubgraphVariable`; final name signed off in Phase 0.)*
8. `resolve.FetchConfiguration.MergeableOperation` — not every fetch will be merged, so the name is not representative; something like a raw fetch operation. *(Clarified: leaning `SubgraphOperation`; final name signed off in Phase 0.)*
9. Fetchtree tests use contains, while they should use full assertions.
10. Missing pretty-printed plan test.
11. `v2/pkg/engine/resolve/loader_multi_entity_test.go` also uses contains; the test is very hard to read — too many small pieces. Refactor and use only meaningful helpers; small duplication is fine if it is more readable.
12. `v2/pkg/engine/resolve/loader_multi_entity.go` — rather hard to read.
13. `loader.go` uses too many if/else to support MultiFetch; some helpers could be refactored to be MultiFetch-aware.
14. We do not have a full integration test (they live in the execution pkg, e.g. `execution/engine/execution_engine_defer_test.go`).

## Review-item → task map

| Review item | Task |
|---|---|
| 1 — missing extra-variables plan tests | Task 1.2 |
| 2 — `representations` collision (all entity fetches) | Task 1.5 |
| 3 — lazy input construction | Phase 0 → Phase 2 |
| 4 — `buildMergedOperation` refactor + placement | Phase 0 (placement decision) → Phase 2 |
| 5 — delete `splitEntityFetchInput` via laziness | Phase 0 → Phase 2 |
| 6 — merge stage placement after parallel nodes | Phase 0 → Phase 2 |
| 7 — rename `NamedVariableFragment` | Phase 2 (name confirmed in Phase 0 checkpoint) |
| 8 — rename `MergeableOperation` | Phase 2 (name confirmed in Phase 0 checkpoint) |
| 9 — fetchtree tests use contains | Task 1.3 |
| 10 — missing PrettyPrint test | Task 1.3 |
| 11 — loader test readability / contains | Task 1.4 |
| 12 — `loader_multi_entity.go` readability | Task 1.6 |
| 13 — `loader.go` if/else sprawl | Task 1.6 |
| 14 — full integration test in `execution` pkg | Task 1.1 |

---

## Phase 0 — Investigation and design addendum (items 3, 4, 5, 6, 7, 8)

### Task 0.1: Laziness and stage-placement design addendum

**Files:**
- Create: `docs/multi-fetch/design-v2.md`
- Read-only inputs: `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource.go` (`ConfigureFetch`, `createInputForQuery`, `generateQueryPlansForFetchConfiguration`), `v2/pkg/engine/postprocess/postprocess.go`, `v2/pkg/engine/postprocess/create_multi_fetch.go`, `v2/pkg/engine/postprocess/create_multi_fetch_input.go`, `v2/pkg/engine/postprocess/resolve_input_templates.go`, `v2/pkg/engine/postprocess/create_concrete_single_fetch_types.go`, `v2/pkg/engine/postprocess/order_sequence_by_dependencies.go`, `v2/pkg/engine/postprocess/create_parallel_nodes.go`, `docs/multi-fetch/spec.md`

**Deliverable:** a design doc that answers, with code evidence (file:line references), each question below, and ends with a task-by-task Phase 2 breakdown in the same format as Phase 1 of this plan.

- [ ] **Step 1: Answer the input-laziness questions (items 3 + 5)**

The doc must answer:

1. What consumes `FetchConfiguration.Input` (the printed string) between `ConfigureFetch` and `resolveInputTemplates`? Known consumers to verify: `deduplicateSingleFetches` (string equality), plan tests under `DisableResolveInputTemplates`, subscription trigger inputs. Are there others?
2. Can the planner, when `EnableMultiFetch` is on and the fetch is an entity fetch, skip printing `body.query` into the input and instead carry the envelope **structurally** (method, URL, header JSON, extensions, plus the ordered variables list already recorded)? Target: `splitEntityFetchInput` (all of `v2/pkg/engine/postprocess/create_multi_fetch_input.go`) is deleted; merged and unmerged inputs are rendered from structured parts at the end of postprocessing.
3. What does `generateQueryPlansForFetchConfiguration` print eagerly, and can the pretty query for merged fetches be produced only for the survivors (lazy query-plan printing)?
4. For fetches that end up **not** merged, where is their input string rendered under the lazy scheme, and is it byte-identical to today's output? (Hard requirement: existing goldens for unmerged plans must not change.)

- [ ] **Step 2: Answer the stage-placement questions (item 6)**

1. Can `resolveInputTemplates` move to the end of postprocessing (after `orderSequenceByDependencies` / `createParallelNodes` / defer-tree building)? Enumerate every stage between its current and proposed position and state whether it reads `InputTemplate` segments or the raw `Input` string.
2. Can `createMultiFetch` run after `createParallelNodes`, replacing the scratch-tree wave simulation with the real parallel groups? Determine: (a) which fetch representation exists at that point (`SingleFetch` vs concrete `EntityFetch`/`BatchEntityFetch` — depends on whether `createConcreteSingleFetchTypes` also moves), (b) how defer-tree grouping interacts, (c) how the subscription path (`processTrigger`) is affected.
3. Decide the target pipeline order and list every ordering constraint with its reason, in the style of the current comment block in `processFlatFetchTree`.

- [ ] **Step 3: Answer the document-merge placement question (item 4)**

1. Sketch the refactored `buildMergedOperation` decomposition (functions ≤ ~25 lines each: rename-map construction, per-member variable-definition import, include-directive injection, per-member selection import + aliasing, printing).
2. Determine whether the core is expressible as pure `ast.Document` in/out (no `resolve.*` types). If yes, specify the `asttransform` API (proposed: `asttransform.MergeOperations(members []asttransform.OperationMergeMember) (*ast.Document, error)` with `OperationMergeMember{Document *ast.Document, Alias string, IncludeVariableName string, VariableRename map[string]string}`) and the thin postprocess adapter. If no, state precisely which coupling prevents it and keep the split inside postprocess.

- [ ] **Step 4: Confirm final names (items 7 + 8)**

State the final names with one-paragraph rationale each, defaulting to `SubgraphOperation` + `SubgraphVariable` unless the laziness design gives the types a different role (e.g. if the structured envelope joins the type, a name like `SubgraphRequest` may fit better). List every declaration and usage site the rename touches.

- [ ] **Step 5: Write the Phase 2 task breakdown**

Same task format as Phase 1 below: exact files, interfaces, failing-test-first steps, `gotestsum` commands with expected outcomes. Constraint to state explicitly in every Phase 2 task: final executable plans (inputs, templates, postprocessing configs) must be functionally identical to the pre-rework branch so that Phase 1's goldens pass unchanged, except where the design doc explicitly documents a divergence.

- [ ] **Step 6: AUTONOMOUS CHECKPOINT — the design doc is adversarially reviewed (independent review agent + orchestrator self-review) and revised until findings are resolved. Phase 2 does not start until the review passes. Phase 1 may proceed in parallel.**

---

## Phase 1 — Architecture-independent fixes

Task order below is dependency-driven: 1.1 and 1.2 build the regression net; 1.3 and 1.4 make existing tests trustworthy; 1.5 changes planner behavior (guarded by the net); 1.6 refactors runtime code (guarded by 1.1 + 1.4).

### Task 1.1: Execution-engine plumbing + full integration test (item 14)

**Files:**
- Modify: `execution/engine/execution_engine.go` (postProcessor construction, `ExecutionEngine` config surface)
- Create: `execution/engine/execution_engine_multi_fetch_test.go`
- Reference for harness patterns: `execution/engine/execution_engine_defer_test.go`, `execution/engine/execution_engine_test.go`

**Interfaces:**
- Produces: a public way to enable MultiFetch on the execution engine, following the existing option pattern in that file (investigate first: if `NewExecutionEngine` takes functional options, add `WithMultiFetch()`; if configuration is struct-based, add an `EnableMultiFetch bool` field). It must set **both** `plan.Configuration.EnableMultiFetch = true` and construct the processor as `postprocess.NewProcessor(postprocess.EnableMultiFetch())`.

- [ ] **Step 1: Investigate the option surface.** Read `execution/engine/execution_engine.go` and how `plan.Configuration` is built there. Pick the option mechanism that matches existing style; record the choice in the test file's header comment.
- [ ] **Step 2: Write the failing integration test — happy path** (`TestExecutionEngine_MultiFetch`, subtest `merges same-subgraph entity fetches into one request`). Federation setup with two subgraphs where one subgraph (`products`) receives two entity fetches in the same wave (pattern: query selects `employees { products }` and `employee(id:) { notes }`, both resolved by `products`). Use a counting round-tripper / test server so the test can assert the subgraph received **exactly one** request, and `assert.Equal` the full request body JSON (merged aliased operation + `representations_f1`/`includeF1` variables) and the full client response JSON.
- [ ] **Step 3: Run it, verify it fails** (engine has no MultiFetch option yet): `gotestsum --format=short -- ./execution/engine/... -run TestExecutionEngine_MultiFetch` — expected: compile error or single-request assertion failure (two requests observed).
- [ ] **Step 4: Implement the plumbing** from Step 1's investigation. Minimal: thread the two flags; no other engine changes.
- [ ] **Step 5: Run the test, verify it passes.**
- [ ] **Step 6: Add the remaining scenarios**, each with full-equality assertions on the client response:
  - subgraph returns errors under one alias only (`f1`) — errors attribute to that entry's response path, other entry's data intact;
  - one entry has no live representations (parent returned null) — request body contains `"includeF2":false` and empty representations array; response correct;
  - MultiFetch disabled (control) — same query produces two subgraph requests and a byte-identical client response to the merged run.
- [ ] **Step 7: Run the full package:** `gotestsum --format=short -- ./execution/engine/...` — expected: PASS, no regressions.

### Task 1.2: Full-plan tests with additional variables (item 1)

**Files:**
- Modify: `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_multi_fetch_test.go`

**Interfaces:**
- Consumes: existing `TestGraphQLDataSourceFederation_MultiFetch` schema/setup in that file; extend the federation SDL only if it lacks a field with an argument on an entity type.

- [ ] **Step 1: Write failing full-plan golden tests** (RunTest full-equality, as the existing cases in this file):
  - **client variable through one member:** member 1 selects `products(first: $first)`; expect merged operation declaring `$first_f1: Int` and entry 1 carrying `Variables: [{KeyPrefix: ',"first_f1":', Value: <context-variable template>}]`, entry 2 without it;
  - **same client variable through both members:** both members use `$first`; expect independent `$first_f1` and `$first_f2` definitions and per-entry variable statics;
  - **variable with a default value:** `$first: Int = 10` — expect the default preserved on the renamed definition (exercises `ImportVariableDefinitionWithVariableNameRename` default handling end-to-end).
- [ ] **Step 2: Run to see the real planner output:** `gotestsum --format=short -- ./v2/pkg/engine/datasource/graphql_datasource/... -run TestGraphQLDataSourceFederation_MultiFetch` — expected: FAIL with a plan diff. Verify the diff shows *correct* planner behavior before adjusting the expected plan to match; if behavior is wrong, stop and report instead of goldening the bug.
- [ ] **Step 3: Finalize expected plans, run again, verify PASS.**

### Task 1.3: Fetchtree full assertions + PrettyPrint test (items 9, 10)

**Files:**
- Modify: `v2/pkg/engine/resolve/fetchtree_test.go`

- [ ] **Step 1: Rewrite `TestFetchTreeNode_Trace_MultiEntity` and `TestFetchTreeNode_QueryPlan_MultiEntity`** to marshal the full node and compare with `assert.JSONEq` against one complete expected JSON document each (kind, source id/name, entries, mergedFetchIds, query, representations, dependencies — every populated field).
- [ ] **Step 2: Add `TestFetchTreeQueryPlanNode_PrettyPrint_MultiEntity`**, modeled on `TestFetchTreeQueryPlanNode_PrettyPrint_Trigger`: build a query plan containing a `MultiEntity` node with a pretty merged query and representations, `assert.Equal` the entire `PrettyPrint()` output string.
- [ ] **Step 3: Run:** `gotestsum --format=short -- ./v2/pkg/engine/resolve/... -run 'TestFetchTree'` — expected: PASS. If the PrettyPrint output for MultiEntity is malformed (this is its first full-output test), fix `v2/pkg/engine/resolve/fetchtree.go` rendering, not the expectation.

### Task 1.4: Loader multi-entity test refactor (item 11)

**Files:**
- Modify: `v2/pkg/engine/resolve/loader_multi_entity_test.go`

**Refactor contract:** behavior coverage must not shrink — every scenario currently asserted keeps a full-equality assertion.

- [ ] **Step 1: Introduce exactly two helpers** (small duplication elsewhere is acceptable per review guidance):
  - `multiFetchFixture(t, opts...)` — builds the `MultiEntityFetch`, loader, and parent data in one call, returning a struct with named fields (`loader`, `fetch`, `item`) so each test reads top-to-bottom;
  - `assertMergedErrors(t, loader, expectedJSON string)` — marshals accumulated errors and `assert.JSONEq`s the whole document.
- [ ] **Step 2: Replace fragment assertions with full ones:** every `assert.Contains(t, string(prepared.input), ...)` becomes one `assert.Equal` on the entire input string; every error/data `Contains` cluster becomes one `assert.JSONEq`/`assert.Equal` on the whole document. `NotContains` checks for alias leakage (`"f1"`, `"f2"`) become implied by full-document equality and are dropped.
- [ ] **Step 3: Run:** `gotestsum --format=short -- ./v2/pkg/engine/resolve/... -run 'TestPrepareMultiEntityFetch|TestMergeMultiEntityResult|TestLoadGraphQLResponseData_MultiEntity'` — expected: PASS with identical coverage (same test names, same scenario count or better).

### Task 1.5: Planner `representations` collision fix for all entity fetches (item 2)

**Files:**
- Modify: `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource.go`
- Test: `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_test.go` (planner-level), plus one MultiFetch full-plan case in `graphql_datasource_multi_fetch_test.go`

**Design (rename the planner-added variable, never the client's):**
The synthetic name is used at three coupled sites: the variable definition (`addRepresentationsVariableDefinition`), the `_entities(representations: $...)` argument literal (the `representationsLiteral` write in the entities-selection builder), and the `body.variables` key (`addRepresentationsVariable`). Introduce a single source of truth: a planner method `representationsVariableName() string` that returns `"representations"` unless the **downstream** operation declares a client variable of that name (check `p.visitor.Operation` variable definitions for the operation being planned), in which case it probes `_representations`, `_representations2`, … until free. All three sites read this method; the resolved name is computed once per document and cached on the planner (reset in `EnterDocument`).

Detection must use the downstream operation, not the upstream one: the synthetic definition is added in the `EnterDocument` scaffold *before* client variables are imported, so at write time the upstream operation cannot yet reveal the collision. This makes the check deliberately conservative — an operation that declares `$representations` but never passes it to this subgraph still triggers the rename. That is harmless (the renamed synthetic is internal to the subgraph request) but behavior-visible; before implementing, grep existing planner test operations for a declared `$representations` to confirm no current golden is affected.

**Consequences to implement in the same task:**
- Delete `upstreamVariableCollision` and its poison behavior — with the rename, the collision cannot occur, so MultiFetch no longer refuses these fetches.
- `create_multi_fetch.go` candidate validation and entry-static construction must identify the representations entry by its recorded name matching the fetch's resolved synthetic name (or, simpler and name-agnostic: by being the single `[$$N$$]` fragment bound to the `ResolvableObjectVariable` — verify which identification the code already uses and keep it name-agnostic).

- [ ] **Step 1: Write failing planner tests** (`TestGraphQLDataSource_RepresentationsCollision`) in `graphql_datasource_test.go`: federation entity fetch where the client operation declares and uses `$representations: String!` as a field argument. Assert the full printed fetch input: client variable keeps its name and value; the synthetic entry appears as `"_representations":[$$N$$]`; the upstream operation uses `_entities(representations: $_representations)` with both variables declared. Do not hand-guess definition order — after Step 3, derive the full expected strings from observed output, verifying the *behavior* (both variables present, correct bindings, no clobbering) before goldening, as in Task 1.2 Step 2. Cover both `RequiresEntityFetch` and `RequiresEntityBatchFetch` shapes.
- [ ] **Step 2: Run to verify failure:** `gotestsum --format=short -- ./v2/pkg/engine/datasource/graphql_datasource/... -run TestGraphQLDataSource_RepresentationsCollision` — expected: FAIL (today one write clobbers the other).
- [ ] **Step 3: Implement `representationsVariableName()`** and thread it through the three sites; delete the collision flag and its check in `setUpstreamVariable`/`ConfigureFetch`.
- [ ] **Step 4: Add the MultiFetch full-plan case:** same collision situation with MultiFetch enabled — the fetch **is now merged**, with the renamed synthetic flowing through (`_representations_f1` after member renaming).
- [ ] **Step 5: Run the full datasource + postprocess packages:** `gotestsum --format=short -- ./v2/pkg/engine/datasource/graphql_datasource/... ./v2/pkg/engine/postprocess/...` — expected: PASS; no existing golden may change (no collision → name stays `representations`).

### Task 1.6: Runtime readability refactor (items 12, 13)

**Files:**
- Modify: `v2/pkg/engine/resolve/loader.go`, `v2/pkg/engine/resolve/loader_multi_entity.go`

**Refactor contract:** pure refactor — zero behavior change; the suite from Tasks 1.1/1.4 plus the existing resolve tests is the guard. Run the full resolve package before starting to confirm a green baseline.

- [ ] **Step 1: Collapse the six `res.multi != nil` branches in `mergeResult`** by giving `result` (or a small interface over `result`/`multiEntryMergeConfig`) intention-revealing accessors, so `mergeResult` reads linearly:
  - `res.parsedResponse(l *Loader) (*astjson.Value, error)` — pre-parsed multi response or parse-with-fallback;
  - `res.responseErrors(response *astjson.Value) *astjson.Value` — pre-partitioned entry errors or path lookup;
  - `res.errorPathRoot() string` — entry alias or `"_entities"` (feeds both `getTaintedIndices` and `rewriteErrorPaths`, and decides `hideAliasInErrorPaths`);
  - `res.collectExtensions() bool`, `res.checkEmptyEntityFetch() bool`, `res.emptyAliasIsBenign(responseData *astjson.Value) bool`.
- [ ] **Step 2: Split `prepareMultiEntityFetch` in `loader_multi_entity.go`** into named phases matching the spec vocabulary, each ≤ ~40 lines: `selectEntryTargets`, `authorizeEntry`, `renderEntryRepresentations`, `renderEntryVariables`, `assembleMultiEntityInput`. Same for the merge side: `partitionResponseErrors`, `applyTransportStateToEntries`, `mergeEntryResults`. Keep comments that state lock invariants (everything under `dataBuffer.Lock()`).
- [ ] **Step 3: Run:** `gotestsum --format=short -- ./v2/pkg/engine/resolve/...` — expected: PASS, identical test set.

---

## Phase 2 — Architecture rework (items 3, 4, 5, 6, 7, 8)

Blocked on the Phase 0 checkpoint. `docs/multi-fetch/design-v2.md` will contain the detailed task breakdown in this plan's format; after user approval those tasks are appended here (or executed from the design doc directly, user's choice). Fixed constraints already known:

- Lazy input rendering: planner stops printing `body.query`/`body.variables` for entity fetches when MultiFetch is enabled; envelope carried structurally; `splitEntityFetchInput` and its tests are **deleted**, not adapted.
- Merge stage relocates after real parallel-group creation; the scratch-tree wave simulation in `create_multi_fetch.go` is deleted.
- `buildMergedOperation` is decomposed per the design doc's Step 3 decision (asttransform core + postprocess adapter, or postprocess-internal split).
- Renames land here: `MergeableOperation` → final name, `NamedVariableFragment` → final name, including doc updates (`spec.md`, `implementation.md`).
- Acceptance gate for the whole phase: full test suite green, and every Phase 1 golden (datasource plans, integration request bodies, loader inputs) passes **unchanged** except divergences explicitly listed in the design doc.

## Execution model

- Fully autonomous: design and decisions by the orchestrator (Fable), implementation by one Opus subagent per task, each given this file plus only its task section.
- Each task's diff is reviewed by the orchestrator (independent review agent for risky tasks: 1.1, 1.5, 1.6, all of Phase 2) and its tests re-run before commit. Phase 0's checkpoint is an adversarial agent review instead of a user review.
- Subagents never commit; the orchestrator commits once per task, keeping commits splittable by topic.
- Formatting gate before every commit: `gofmt -w` + `goimports -w` on changed files; `go vet` on changed packages.
