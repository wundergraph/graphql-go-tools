# MultiFetch v2 design — deferred input rendering and merge-stage relocation

Resolves review items 3 (lazy input construction), 4 (`buildMergedOperation` refactor/placement), 5 (delete `splitEntityFetchInput`), 6 (merge-stage placement), 7/8 (final naming). Companion to `docs/multi-fetch/review-plan.md` (Phase 0, Task 0.1). All findings verified against the branch; file:line references are to the current branch state.

## 1. Evidence summary

**E1 — readers of the raw `FetchConfiguration.Input` string.** Four, in pipeline order: (a) the plan visitor's own template rewrite (`plan/visitor.go:1235` → `visitor.go:1052`), which rewrites mustache (`{{ .arguments... }}`) segments — a no-op for the graphql datasource, whose input already carries `$$N$$` placeholders; (b) `deduplicateSingleFetches` via `FetchConfiguration.Equals` (`resolve/fetch.go:290`), which compares `Input` **byte-for-byte** as its primary discriminator; (c) `createMultiFetch`'s `splitEntityFetchInput` string scanner; (d) `resolveInputTemplates` itself, which consumes and clears it. The loader never reads `Input` — only `InputTemplate`.

**E2 — stage IO.** `fetchIDAppender` is an **Input-string writer**: it appends `__<fetchID>` to the propagated operation name via `strings.Replace(fetch.Input, ...)` and the same on `QueryPlan.Query` (`fetch_id_appender.go:38-46`) — it never touches the Document, so deferred rendering must compensate (see D1). `orderSequenceByDependencies` and `createParallelNodes` read only `FetchID`/`DependsOnFetchIDs`. `createConcreteSingleFetchTypes` reads the **resolved** `InputTemplate.Segments` (locates the `ResolvableObjectVariableKind` segment; `create_concrete_single_fetch_types.go:60-67,105-112`), so it must follow `resolveInputTemplates` but has no other position constraint. The defer tree is built **after** all `organizeFetchTree` calls (`postprocess.go:251`). Subscription triggers never pass through `createMultiFetch`; the trigger input is resolved separately (`postprocess.go:263`).

**E3 — what the planner prints eagerly.** `createInputForQuery` assembles `body.variables`, `body.query` (printed minified operation), then `header`/`url`/`method` from `p.config.fetch` via `httpclient` setters. `generateQueryPlansForFetchConfiguration` additionally pretty-prints every fetch's operation eagerly; on merge, the per-member pretty print is discarded and replaced by the merged pretty print (`create_multi_fetch.go:271-301`).

**E4 — representations identification.** The postprocess candidate check is already name-agnostic: it matches the single `^\[\$\$(\d+)\$\$\]$` variables fragment bound to a `ResolvableObjectVariable` (`create_multi_fetch.go:392-428`). Only the planner-side collision flag uses the literal name.

**E5 — `buildMergedOperation` coupling.** The merge core touches `resolve.*` only for: `MergeableOperation.Document`, `MergeableOperation.Variables[].Name`, `OperationName`, `FetchID`. Everything else is pure `ast`/`astimport`/`astprinter`. `asttransform`'s existing surface is document-level AST transforms (`MergeDefinitionWithBaseSchema`, `NewTypeNameVisitor`).

## 2. Decisions

### D1 — Deferred input rendering with print-once caching (items 3, 5)

When `EnableMultiFetch` is on and the fetch is an entity fetch, the planner **stops assembling the input JSON entirely**. `FetchConfiguration.Input` stays empty; the structured artifact carries everything needed to render it later:

```go
// resolve package (final names per D4)
type SubgraphOperation struct {
    Document  *ast.Document      // normalized, validated upstream operation (ownership transferred)
    Variables []SubgraphVariable // ordered top-level body.variables entries, raw fragments with $$N$$
    Envelope  SubgraphRequestEnvelope

    // printedQuery caches the compact operation print. Implementation amendment
    // (Task 2.4): the PLANNER seeds this cache with the exact bytes printOperation
    // produces (including minification, which needs the schema and is impossible
    // in postprocess); dedupe and the render stage only read it. The print-once
    // property holds — the planner printed the operation exactly once already.
    printedQuery []byte
}

type SubgraphVariable struct {
    Name  string
    Value []byte
}

type SubgraphRequestEnvelope struct {
    Method string
    URL    string
    Header []byte // raw JSON or nil — exactly what ConfigureFetch would have set
}
```

Rendering moves to postprocessing (see D2 for position): a `renderSubgraphInputs` stage assembles the input string for every fetch that carries a `SubgraphOperation` and no `Input`, using **the same assembly helper the planner uses today** — `createInputForQuery`'s envelope assembly is extracted into a shared function (home: `datasource/httpclient`, which both packages already import; postprocess ← httpclient introduces no cycle). Same helper + same sjson version ⇒ byte-identical output for unmerged fetches, which is the hard acceptance gate.

Consequences, each with its resolution:

- **Dedupe (E1b).** `FetchConfiguration.Equals` gains one branch: when both sides have empty `Input` and non-nil `SubgraphOperation`, compare `Envelope` fields, the `Variables` list (names + raw values), and the lazily-printed-and-cached `printedQuery`. The cached print is *reused* as `body.query` during rendering, so unmerged fetches pay exactly one print — the same count as today. Merged-away members may pay one print for a dedupe comparison that today they'd also pay in `ConfigureFetch`; never more.
- **Plan-visitor rewrite (E1a).** Operates on an empty string — no-op. A Phase 2 test asserts a mergeable-fetch plan built through the full `plan.Planner` still renders correctly (guarding the "no other visitor reader" assumption).
- **`DisableResolveInputTemplates` / datasourcetesting.** `renderSubgraphInputs` runs **unconditionally** (like today's artifact-clearing), even when template resolution and merging are disabled — so datasource-testing plans keep a readable printed `Input`. The stage renders the string; it does not resolve templates.
- **`splitEntityFetchInput` and all of `create_multi_fetch_input.go` are deleted**, including the dual sjson-shape handling and the envelope-remainder byte comparison. `mergeGroup` builds `Header`/`Footer` directly from `Envelope` + the merged printed operation; the "same envelope" merge guard becomes a comparison of `Envelope` structs and shared `$$K$$` variable references — no byte scanning.
- **Operation-name suffix (E2, review finding 1).** `fetchIDAppender`'s string replace no-ops on an empty `Input`, so `renderSubgraphInputs` must apply the identical transformation after assembly: `strings.Replace(assembledInput, operationName, operationName+"__<fetchID>", 1)` on the **whole input** (not just the query), replicating today's first-occurrence semantics exactly — including the pre-existing quirk that a URL/header containing the operation-name substring would be corrupted; we preserve the quirk for byte identity rather than silently fixing it. The same compensation applies to the deferred `QueryPlan.Query` print. Dedicated Task 2.4 gate: a flag-on plan with `EnableOperationNamePropagation` renders byte-identically to flag-off.
- **Query-plan laziness (E3, item 3's second half).** `generateQueryPlansForFetchConfiguration` keeps building `DependsOnFields` (needed per-member even after merge) but defers the pretty operation print the same way: `QueryPlan.Query` for mergeable entity fetches is rendered during `renderSubgraphInputs` for survivors only. Merged fetches already re-print the merged document; per-member eager prints are pure waste today.

**Rejected alternative — structured envelope alongside eager printing** (keep printing `Input`, record envelope statics only to kill the scanner): strictly less invasive (no dedupe change, no render stage), but it leaves item 3 unaddressed — the planner still prints inputs that merging throws away — and permanently maintains two sources of truth for the same bytes. The dedupe extension is the only real cost of full deferral, and it is small, well-testable, and confined to one method.

### D2 — Pipeline placement: merge on real parallel groups, render at the end (item 6)

Target orchestration (`postprocess.go`), per response — including each defer group, which today already gets its own `organizeFetchTree` pass:

```
mergeFields → createFetchTree
→ collectAuthorizationCoordinates → dedupe → appendFetchID
→ addMissingNestedDependencies
→ orderSequenceByDependencies → createParallelNodes        (real waves exist now)
→ createMultiFetch          (walks parallel groups; no scratch tree, no DeferID partition —
                             defer groups are organized separately, so waves are already correct)
→ renderSubgraphInputs      (unconditional; assembles Input for artifact-carrying fetches,
                             renders survivor query plans, clears artifacts)
→ resolveInputTemplates     (unchanged; now traverses the organized tree)
→ createConcreteSingleFetchTypes   (unchanged logic; runs last, per E2 it only needs segments)
```

What this dissolves: the scratch-tree wave simulation (`create_multi_fetch.go` re-running organize stages per DeferID partition) is replaced by walking the real `FetchTreeNodeKindParallel` groups — zero drift by *identity* instead of zero drift *by construction*. The merge operates on `SingleFetch` nodes still (concretization is last), so candidate logic is unchanged.

Constraints that keep this correct, each carried into Phase 2 tasks as an explicit test:
- `resolveInputTemplates`, `renderSubgraphInputs`, and `createConcreteSingleFetchTypes` must traverse sequence/parallel/defer node kinds, not just flat children (today's traversers already recurse; verify per stage).
- Merged-node tree surgery now edits an organized tree: replacing the first member inside its parallel group and deleting the others must preserve group invariants — `createParallelNodes` materializes a Parallel node only for >1 children (`create_parallel_nodes.go:27`), so a post-merge group of one collapses back to the bare Single node (explicit Task 2.5 test).
- Member/alias order: today's `f1`/`f2` assignment follows flat child order (ascending FetchIDs). To make the relocated stage independent of any ordering behavior in `orderSequenceByDependencies`, group members are **sorted by FetchID before aliasing** — reproducing today's order by construction (Task 2.5).
- The subscription response path gets the same reordered pipeline; the trigger keeps its separate `ProcessTrigger` template resolution and is never merged (E2).
- Defer: `buildDeferTree` runs after all of the above, reading only IDs — unaffected. The old DeferID partitioning code is deleted with the scratch tree.

### D3 — Operation merging moves to asttransform (item 4)

The merge core becomes a pure-AST API in `v2/pkg/asttransform`:

```go
package asttransform

type OperationMergeMember struct {
    Document        *ast.Document     // one operation definition, normalized
    Alias           string            // alias for the member's root field ("f1")
    IncludeVariable string            // synthetic Boolean! variable name ("includeF1")
    VariableRename  map[string]string // total rename applied to definitions and all references
}

// MergeOperationDocuments builds a new document with a single query operation
// (named operationName, anonymous if empty) containing every member's root
// selection aliased and guarded by @include(if: $IncludeVariable).
// Fragment spreads in member documents are rejected.
func MergeOperationDocuments(operationName string, members []OperationMergeMember) (*ast.Document, error)
```

Decomposition inside (each ≤ ~25 lines): `mergeVariableDefinitions` (renamed imports + synthetic include per member), `mergeMemberSelection` (selection-set import + alias + `@include` attachment), plus the top-level assembly. Validation split: the **single-root-field** check is generic and stays in asttransform; the **root field must be `_entities`** check is federation-specific and moves to the postprocess adapter. The adapter (`create_multi_fetch_document.go` shrinks to it) builds rename maps and operation name from `resolve` types and does the compact/pretty printing — printing stays out of asttransform since callers differ in which forms they need. Byte-identity of output is preserved: variable-definition order (member definitions in document order, then the synthetic include, sequentially per member) matches today's loop.

Fit check (E5): asttransform is document-level AST manipulation with no `resolve` coupling; this API matches. It differs from the existing mutate-in-place style by returning a new document — inherent to a merge of N inputs, and acceptable.

### D4 — Final names (items 7, 8)

- `resolve.MergeableOperation` → **`resolve.SubgraphOperation`**, field `FetchConfiguration.MergeableOperation` → **`SubgraphOperation`**. Rationale: after D1 the type is the structured form of the subgraph request's operation — what it *is*, independent of whether merging consumes it. "Mergeable"/"Raw" described one consumer.
- `resolve.NamedVariableFragment` → **`resolve.SubgraphVariable`**. "Fragment" collided with GraphQL fragments; the value keeps the field name `Value` (raw bytes documented on the field, not in the type name).
- New `resolve.SubgraphRequestEnvelope` per D1.
- Planner internals follow: `upstreamVariablesList []resolve.SubgraphVariable`, `MergeableOperation` mentions in docs/comments updated (`spec.md`, `implementation.md`).

### D5 — Acceptance gates for the whole rework

1. Every existing golden for **unmerged** plans is byte-identical (dedupe extension and shared assembly helper are behavior-preserving; laziness only activates with the flag on).
2. Every Phase 1 golden — merged full plans (Task 1.2), integration request bodies and client responses (Task 1.1), loader input assembly (Task 1.4) — passes **unchanged**. The merged input's `Header`/`Footer` produced from `Envelope` must equal what `splitEntityFetchInput` produced; the Task 1.1 full-request-body assertions are the executable proof.
3. Full `v2` and `execution` test suites green after every task (each task below leaves the tree green).

## 3. Phase 2 task breakdown

Ordered so the tree stays green and each commit is one topic. Tasks 2.1 and 2.2 are mechanical and independent; 2.3–2.5 are the coupled core in strict order; 2.6 is the cleanup that realizes the deletions.

### Task 2.1: Extract `asttransform.MergeOperationDocuments` (item 4)

**Files:** Create `v2/pkg/asttransform/merge_operations.go`, `v2/pkg/asttransform/merge_operations_test.go`; Modify `v2/pkg/engine/postprocess/create_multi_fetch_document.go` (shrink to adapter).
**Behavior-identical refactor**: move the loop body of `buildMergedOperation` into the D3 API with the D3 decomposition; adapter builds `OperationMergeMember` slices (rename maps from document variable definitions ∪ recorded variable names, exactly as today) and keeps compact/pretty printing. Port `TestBuildMergedOperation` goldens: full compact + pretty equality must not change by a byte. New asttransform unit tests cover: two members with overlapping variable names, default values, directives on fields/inline fragments, fragment-spread rejection, anonymous vs named operation.
**Gate:** `gotestsum --format=short -- ./v2/pkg/asttransform/... ./v2/pkg/engine/postprocess/...` green; `TestGraphQLDataSourceFederation_MultiFetch*` goldens unchanged.

### Task 2.2: Renames (items 7, 8)

**Files:** `v2/pkg/engine/resolve/fetch_multi.go`, `fetch.go`, all users (`graphql_datasource.go`, `create_multi_fetch*.go`, tests), `docs/multi-fetch/spec.md`, `docs/multi-fetch/implementation.md`.
Pure mechanical rename per D4 (`MergeableOperation` → `SubgraphOperation`, `NamedVariableFragment` → `SubgraphVariable`), via gopls rename or exhaustive grep; no behavior change. **Gate:** full `v2` suite green; `grep -rn "MergeableOperation\|NamedVariableFragment" v2/ docs/` returns nothing.

### Task 2.3: Shared input assembly + envelope recording (D1 groundwork)

**Files:** Modify `v2/pkg/engine/datasource/httpclient/httpclient.go` (or new `input_assembly.go` there), `graphql_datasource.go`, `v2/pkg/engine/resolve/fetch_multi.go` (add `Envelope`, `printedQuery` cache + `PrintedQuery()` accessor).
Extract the envelope assembly from `createInputForQuery`/`ConfigureFetch` into one shared function; the planner calls it (byte-identical output — assert by running the full existing datasource suite), and additionally records `SubgraphRequestEnvelope` on the artifact when recording is on. **Input is still printed eagerly in this task** — no consumer changes yet, tree fully green, goldens untouched.
**Gate:** full `v2/pkg/engine/datasource/...` suite byte-identical green.

### Task 2.4: `renderSubgraphInputs` stage + dedupe extension + planner flip (D1 core)

**Files:** Create `v2/pkg/engine/postprocess/render_subgraph_inputs.go` (+ test); Modify `postprocess.go`, `resolve/fetch.go` (`Equals` extension + unit tests), `graphql_datasource.go` (stop printing input/query-plan query for recorded entity fetches), `create_multi_fetch.go` (consume `Envelope` + `PrintedQuery` for the *unmerged* path only — the merged path still scans in this task if needed for one commit, or flips together if the diff stays reviewable; prefer flipping together).
Order inside the task: (1) render stage able to render from artifacts — including the `fetchIDAppender` suffix compensation (whole-input first-occurrence replace, quirk preserved) and deferred `QueryPlan.Query` printing — no-op when `Input` already set, so the tree stays green while the planner still prints; (2) `Equals` extension with unit tests for empty-Input artifact comparison (equal docs/envelopes ⇒ equal; differing query/envelope/variables ⇒ not equal); (3) planner flip **and** structured-envelope `mergeGroup` land together — this is mandatory, not preferred: once the planner stops printing, the scanner reads an empty `Input` and every merged plan breaks, so steps 3+4 form one atomic green commit (the unused scanner code left behind compiles and vets clean; its deletion is Task 2.6).
**Gate:** all Phase 1 goldens (1.1 integration bodies, 1.2 plans, 1.4 loader inputs) pass unchanged; a flag-on plan with `EnableOperationNamePropagation` renders byte-identically to flag-off; full `v2` + `execution` suites green.

### Task 2.5: Pipeline relocation (D2, item 6)

**Files:** Modify `v2/pkg/engine/postprocess/postprocess.go` (orchestration), `create_multi_fetch.go` (walk real parallel groups; delete scratch-tree simulation and DeferID partitioning; group-of-one normalization on surgery), traversal checks in `resolve_input_templates.go` / `render_subgraph_inputs.go` / `create_concrete_single_fetch_types.go` (recurse over organized trees incl. defer groups); Modify `postprocess_test.go`, `create_multi_fetch_test.go` (wave tests now construct organized trees).
Members are sorted by `FetchID` before aliasing (order-stability insurance per D2); an explicit test covers the group-of-one collapse after surgery.
**Gate:** full-plan goldens unchanged (same merges must happen — wave semantics identical by identity); defer plan tests (`postprocess` + `graphql_datasource` defer suites) green; subscription multi-fetch golden unchanged.

### Task 2.6: Delete the scanner and dead paths (item 5 realized)

**Files:** Delete `v2/pkg/engine/postprocess/create_multi_fetch_input.go` and its tests (`TestSplitEntityFetchInput` and merge-abort cases tied to byte scanning; port still-meaningful abort semantics — envelope mismatch, variable mismatch — to structured-guard tests); Modify `docs/multi-fetch/spec.md` §merge-guard and `implementation.md` §2.3 to describe the structured path.
**Gate:** `grep -rn "splitEntityFetchInput\|envelopeRemainder" v2/` empty; full suites green; docs updated.

## 4. Risks

| Risk | Mitigation |
|---|---|
| Dedupe equality regression (false merge/false split of entity fetches) | Dedicated `Equals` unit tests both ways + existing dedupe postprocess tests + full-plan goldens |
| Byte drift in rendered inputs vs today | Single shared assembly helper; Task 2.3 lands it under the eager path first, where the whole existing suite verifies byte identity |
| Organized-tree traversal misses a node kind (defer/trigger) | Per-stage traversal tests in 2.5; defer + subscription goldens in the gate |
| Hidden reader of `Input` between plan build and postprocess | E1 enumeration + Task 2.4 gate runs the *full* v2 and execution suites, not just multifetch tests |
| sjson shape drift for downstream consumers (v1.2.5 append shape) | Scanner deleted — shape dependence disappears entirely; assembly helper controls order explicitly |
