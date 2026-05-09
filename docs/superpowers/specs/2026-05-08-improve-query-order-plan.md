# Improve Query Order — Plan (rev 5.10)

Revision history:
- rev 1: initial draft.
- rev 2: incorporates codex review (executor blocker, ASAP-roots optimality, dependency-completeness, migration safety).
- rev 3: codex round 2 — validator must flatten leaves and use fetch-kind-aware provided paths;
  executor refactor must serialize merges with a mutex so JSON tree / arena / taintedObjs are not raced;
  exhaustive enumeration for n≤6;
  type discipline (`Fetch.Dependencies()` + kind switch, no `*SingleFetch` assumption after concrete-type pass).
- rev 3.1: codex round 3 — executor refactor needs three phases, not two: input rendering reads selected items and runs before the network call,
  so prepare (select + render) goes under the lock,
  load is the pure network round-trip,
  and merge stays under the lock. Same three-phase split applies to `BatchEntityFetch`.
- rev 4: user feedback — (a) detect "flat" plans (no nested concurrency) at execution start and skip the mutex entirely on that fast path;
  (b) research real federation query patterns and add a scenario library to the plan before implementing,
  so the algorithm and validator are exercised on shapes we will actually see in production.
- rev 4.1: codex round 5 — scenario 13 (mutations) corrected: planner already chains `DependsOnFetchIDs` between root mutations (verified in `path_builder_visitor.go`), so no `mustSerialize` flag is needed;
  fixture must model the chained edges, not edgeless roots.
  Added scenarios 16 and 17 (composite-key fan-in and response-path-derived dependency).
  Cycle detection returns an error rather than panicking when running in normal postprocess flow.
- rev 5: widen scope — replace the simple "WCC + ASAP-roots" recurrence with full series-parallel decomposition (recursive per-root scheduling with multi-parent join detection).
  This collapses the previously-deferred "v2" optimisation into v1 so the algorithm achieves the critical-path lower bound on every SP-reducible DAG.
  Scenario 4 is no longer "known-suboptimal" — it now produces the SP-optimal output.
  Added scenarios 18 (asymmetric chain merge), 19 (deep multi-parent fan-in), 20 (non-SP "N-shape" DAG — the genuine limit of the SP-tree representation).
  Algorithm grounded in Valdes-Tarjan-Lawler (1982) on SP-digraph recognition;
  minimum-makespan scheduling on series-parallel DAGs with unbounded processors is polynomial and matches the critical path,
  so the upper bound is well-defined.
- rev 5.1: codex round 7 caught a real defect in the rev 5 claim. The recursive eager-inline algorithm is *not* a strict improvement vs level-based: scenario 18 (`A→B, A→C, B→D, C→D, C→E`) produces uniform makespan 4 under rev 5 but uniform makespan 3 under level-based (the leaf side-branch `E` gets dragged into `C`'s sequence, blocking the join `D`).
  Scenario 18 is not series-parallel reducible, so the "critical-path-optimal on SP DAGs" claim was vacuous for that case.
  Resolution: ship a **hybrid** scheduler — compute both the eager-inline tree (rev 5) and the level-with-components tree (rev 4.1), pick the smaller uniform-makespan;
  deterministic tie-break by tree-shape preference.
  This guarantees no uniform-duration regression vs level-based,
  preserves the chain-merge skew win from rev 5,
  and does not require duration data at plan time.
- rev 5.2: codex round 8 found a tie-break counterexample. DAG `A→C, B→C, D` (D independent): under uniform, both schedulers tie at makespan 2; under skew `A=B=C=1, D=2`, level matches critical path while eager-inline regresses to 3 because it batches `D` with `A`/`B` in the parallel block, dragging the join `C` behind `D`.
  Root cause: `scheduleSP` did not partition by weakly-connected component before its inline recursion, so independent subgraphs got serialised against each other through the outer parallel block.
  Fix: add component-awareness to `scheduleSP` — at every recursion level, run WCC and recurse per component.
  Also corrected scenario 20 numbers (it is not a uniform-tie case;
  level wins by 1 even under uniform durations because the leaves can split into 3 independent sub-components after stripping roots).
  Property test extended with a **random-skew evaluation layer** that samples durations and verifies hybrid never regresses against `min(scheduleSP, scheduleLevel)` under any sampled distribution.
- rev 5.3: codex round 9 found a second tie-break counterexample even after the component-awareness fix. DAG `A→B, A→C, A→D, B→C` (single component, three children of `A`, two of which form a chain that joins at `C`): uniform makespans tie at 3, but under skew `A=1, B=1, C=100, D=50`, level matches the DAG critical path of 102 while eager-inline regresses to 151. The "prefer SP on uniform tie" rule is not dominant.
  Initial fix: a sampled-skew dominance check across a fixed set of profiles. Codex round 10 disproved this — DAG `A→D, A→E, B→C, B→E, C→D` ties uniform AND ties on every fixed profile, but Level still beats SP under specific skew (`A=25, B=227, C=647, D=3, E=5`).
- rev 5.4: codex round 10 — replace sampled-skew dominance with **symbolic path-set dominance**. For two SP-trees on the same node set, tree `A` dominates tree `B` iff every root-to-leaf path of `A` is set-contained in some root-to-leaf path of `B`. Equivalently: for every duration assignment, `makespan_A ≤ makespan_B`. This is a finite, polynomial-time check (paths are at most `b^k` where `b` is max parallel branching and `k` is parallel depth — for federation plans with `n ≤ 50` this is bounded and small).
  Tie-break: pick whichever tree dominates the other; if neither, default to Level (conservative — never worse than today's algorithm under uniform durations).
  Verified across all 22 prior scenarios plus codex's round-10 DAG (added as scenario 23).
  The sampled-skew property test remains as a *bound check* but is no longer relied on for soundness of the tie-break.
- rev 5.5: codex round 11 — three plan-text bugs in rev 5.4 (algorithm itself was sound; codex confirmed the theorem and path-set recurrence). Fixes:
  (1) Driver was picking SP whenever `m_sp < m_level` *without* applying dominance, breaking the "no regression vs Level" guarantee. Fix: drop uniform-makespan from the driver entirely. Only pick SP when `dominates(SP, Level)`; otherwise pick Level. This is unconditionally no-regression-vs-Level by the dominance theorem.
  (2) Property-test claim `hybrid_makespan(d) ≤ min(SP_makespan(d), Level_makespan(d))` was false on incomparable DAGs (scenario 23 itself: `A=100, B=1..` gives SP=101, Level=102, hybrid falls back to Level=102 > min=101). Weakened to `hybrid_makespan(d) ≤ Level_makespan(d)` — provable from dominance.
  (3) Removed stale rev 5.3 text that still referenced `spDominatesLevelUnderSkewProfiles` and "prefer SP on uniform-duration tie".
- rev 5.6: codex round 12 — confirmed dominance-only driver is sound. Fixed six remaining stale-text contradictions from earlier revisions:
  property-test claim about `min(scheduleSP, scheduleLevel)` matching uniform makespan;
  "picks the smaller" wording in the honest-claim section;
  scenario 18's "min(makespan) selection rule" wording;
  scenario 21's "no skew regression vs min(SP, Level)" assertion;
  scenario 20's "min(makespan) rule picks correctly under any duration distribution" wording (rewritten to call out incomparable-with-Level-fallback);
  log direction reversed in random-skew evaluation (was logging `Level < SP`, now logs the case where hybrid picked Level but SP would have been better).
- rev 5.7: codex round 13 — two more stale-text bugs.
  Scenario 18's validation entry referenced the rev-5.4 uniform-makespan short-circuit which no longer exists; rewrote with explicit path enumeration showing neither tree dominates → fallback to Level.
  Scenario 20's residual-gap paragraph had contradictory false-start text claiming `Level` path-set-dominates SP and naming a non-existent Level path;
  rewrote with full path enumeration confirming the two trees are incomparable.
- rev 5.8: codex round 14 — three more stale-text contradictions in worked examples and section intros.
  "Hybrid scheduler picks the better output" was wrong (it's dominance-only, falls back to Level on incomparable);
  worked example 1 said "tie + deterministic tie-break" instead of "trees identical → SP trivially dominates";
  worked example 4 said "SP dominates on tied DAGs" instead of explicit path-set-dominance;
  worked example 6 said "Hybrid picks Level (3 < 4)" instead of "incomparable → fallback Level";
  scenario 22 description said "Level dominates SP path-set-wise" — corrected to "SP does not path-set-dominate → fallback Level";
  acknowledged-optimality intro said "hybrid picks the better of two trees" — corrected to dominance-only language.
- rev 5.9: codex round 15 — three more wording inconsistencies vs the dominance-only driver semantics.
  Scenario 22 validation entry said "Level dominates SP → pick Level" (driver doesn't actually check `dominates(Level, SP)`);
  rephrased as "SP does not dominate Level → driver falls back to Level".
  Scenarios 19/21 validation entry said "fall back to Level"; for identical trees the driver returns SP (trivial dominance), output is functionally identical;
  rewrote accordingly.
  Worked examples 2, 3, 5 said "Tie. Same shape." inconsistent with example 1's "trees identical → SP trivially dominates → picks SP" wording;
  unified all four to the same phrasing.
- rev 5.10: codex round 16 — scenario 21 still narrated the rev-5.2-era buggy "tie-break preferred SP" outcome as if it were the current driver behaviour.
  Rewrote scenario 21 to: (a) describe the component-aware SP output and the identical Level output under the rev 5.9 dominance-only driver;
  (b) note that even without component-awareness, the dominance-only driver would catch the regression (since SP-without-components doesn't dominate Level on this DAG);
  (c) justify keeping component-awareness anyway for cleaner SP trees and simpler property tests.

## Goal

Replace the current "level-based" parallel grouping in postprocess with a DAG-aware scheduler that decomposes the fetch DAG into independent components, so that independent dependency chains run fully in parallel instead of being synchronised at every level boundary.

## Motivating example

The committed baseline test (`v2/pkg/engine/datasource/graphql_datasource/query_order_baseline_test.go`) pins the planner output for this operation:

```graphql
query Baseline($a: [ID!]!) {
  me {
    firstName
    lastName
    currentPractice { id }
  }
  organisations(ids: $a) {
    name
    shortCode
    id
  }
}
```

Three subgraphs:
`user-subgraph` owns `me` and `User`.
`organisation-subgraph` owns `organisations`/`Organisation` and is independent of `User`.
`practice-subgraph` extends `User` with `currentPractice`.

Today's plan:

```
Sequence {
  Parallel {
    Fetch(user-subgraph)
    Fetch(organisation-subgraph)
  }
  Fetch(practice-subgraph)        # entity fetch into User, depends on user
}
```

Critical path: `max(user, org) + practice`.

Target plan:

```
Parallel {
  Sequence {
    Fetch(user-subgraph)
    Fetch(practice-subgraph)
  }
  Fetch(organisation-subgraph)
}
```

Critical path: `max(user + practice, org)`.

Concrete numbers — `user=100ms, practice=50ms, org=200ms`:
today = `200 + 50 = 250ms`,
target = `max(150, 200) = 200ms`.
20% reduction on this small case;
larger when there are more independent components or longer chains.

## Why the current postprocessor produces the suboptimal plan

The fetch tree is built in two postprocess steps after the planner emits a flat sequence (`v2/pkg/engine/postprocess/postprocess.go:112-127`):

1. `orderSequenceByDependencies` (`v2/pkg/engine/postprocess/order_sequence_by_dependencies.go`) topologically sorts the flat children of the root `Sequence` so that every node appears after its transitive dependencies.
2. `createParallelNodes` (`v2/pkg/engine/postprocess/create_parallel_nodes.go`) walks the sorted children left-to-right and, for each position `i`, greedily pulls every later child whose dependencies are already satisfied by `ChildNodes[:i]` into a `Parallel` block at position `i`.

Trace for the baseline (`user`, `org`, `practice`):

- Sort order: `user, org, practice` (practice depends on user; user/org tie on equal deps and resolve by `FetchID`).
- `i=0`: `providedFetchIDs=[]`. `org.deps=[]` → joins parallel. `practice.deps=[user]` → cannot, user is not in `[:0]`. Parallel becomes `[user, org]`.
- `i=1` (now `practice` after splice): no later siblings.
- Result: `Sequence(Parallel(user, org), practice)`.

The shape is forced because `createParallelNodes` only groups *forward from the leftmost ungrouped node* and only checks dependencies against nodes appearing *strictly before* `i`.
It cannot move `practice` to run alongside `org`,
nor can it split the original sequence into two independent branches.
That is the whole class of optimisations we are leaving on the table.

## Hard prerequisite: executor cannot run nested Sequence inside Parallel today

`v2/pkg/engine/resolve/loader.go:226-273` (`resolveParallel`) iterates the children of a `Parallel` node and accesses `nodes[i].Item.FetchPath` and `nodes[i].Item.Fetch` directly,
launching one goroutine per child via `loadFetch`.
A `Sequence` child has `Item == nil`,
so emitting `Parallel(Sequence(...), Single(...))` from the postprocessor would nil-deref at runtime.

The optimal shape for the baseline (`Parallel(Sequence(user, practice), org)`) therefore cannot be executed by the current loader.
This is the executor work that must land before any postprocessor change.

### Executor change

The naive refactor — "have each goroutine call `resolveFetchNode` recursively" — is not correct.
Today `resolveParallel` (`loader.go:226-273`) preselects items, runs `loadFetch` concurrently inside the errgroup, and then merges results *serially* after `g.Wait()`.
Recursing into `resolveSingle` from inside the goroutine would call `selectItemsForPath`, `loadSingleFetch`, and `mergeResult` *inline* (`loader.go:285-301`). That introduces:

- Concurrent `astjson.MergeValuesWithPath` calls into `l.jsonArena` (`merge_result.go` calls `astjson.MergeValuesWithPath` repeatedly into the shared arena).
- Concurrent map writes into `l.taintedObjs` (`loader.go:589`) — Go panics on concurrent map writes.
- Concurrent reads of `l.resolvable.data` from `selectItemsForPath` while another goroutine merges into it — produces undefined JSON state and likely a panic in the astjson library.

The current code avoids all of this because *only `loadFetch` runs in the errgroup*; everything else happens on a single goroutine.

The correct invariant to preserve is:
**network I/O runs concurrently across branches; any read or mutation of `l.resolvable.data`, `l.jsonArena`, or `l.taintedObjs` is serialised.**

Concrete design — **three phases**, not two. `loadSingleFetch` (`loader.go:419` and `loader.go:1289` for input rendering) needs the selected `items` *before* the network call so it can render the request body, and `selectItemsForPath` (`loader.go:331`) reads `l.resolvable.data` and `l.taintedObjs`. Selecting and rendering must therefore happen under the lock; only the network round-trip is pure.

1. Add `l.mergeMu sync.Mutex` to `Loader`.
2. Refactor the leaf operation in `resolveSingle` into three phases:
   - `preparePhase(item) (items, renderedInput, error)` — runs `selectItemsForPath` and the input-template rendering. Reads `l.resolvable.data` / `l.taintedObjs`. **Acquires `l.mergeMu` for the whole body.**
   - `loadPhase(ctx, fetch, renderedInput) (*result, error)` — pure network call with the pre-rendered body. No shared state mutation. Safe to call from any goroutine without the lock.
   - `mergePhase(item, items, result) error` — wraps `mergeResult`, `callOnFinished`, and any `taintedObjs.add`. **Acquires `l.mergeMu` for the whole body.**
3. Rewrite `resolveSingle` to call `preparePhase` → `loadPhase` → `mergePhase` (single-goroutine path is unchanged in behaviour, just split internally).
4. Rewrite `resolveParallel` to dispatch children through a context-aware `resolveFetchNodeWithCtx(ctx, node)` running in `errgroup.Go`. Each call recurses uniformly:
   - For `Single`: `preparePhase` (lock) → `loadPhase` (no lock) → `mergePhase` (lock).
   - For `Sequence`: serial recursion within the goroutine. Each child enters its own three phases. Across-branch sequencing is naturally honoured because `preparePhase`/`mergePhase` acquire `mergeMu`.
   - For nested `Parallel`: spawn a sub-errgroup, same recursion.
5. Pre-existing top-level callers of `resolveParallel` (`loader.go:220`) still see the same semantics: load is concurrent, merges are serialised, no goroutine sees torn JSON state.

The previous "preselect items, merge after Wait" pattern in `resolveParallel` (`loader.go:237-271`) is replaced by per-leaf `prepare + load + merge` with the load step parallel and the prepare/merge steps under a shared mutex. Performance: the network load remains parallel (the win); the prepare and merge are O(n) serial today and stay O(n) serial.

The same three-phase split applies to `BatchEntityFetch` (`loader.go:303-327`). The plan covers it explicitly because the test plan calls out a batch-entity validator case.

### Mutex bypass for flat plans

The lock is only required if two goroutines can concurrently mutate `l.resolvable.data` / `l.jsonArena` / `l.taintedObjs`. That can happen iff some `Parallel` node has a non-`Single` child (a `Sequence` or nested `Parallel`), because then the per-branch goroutine recurses through `resolveFetchNode` and ends up calling `resolveSingle` from inside a goroutine.

If every `Parallel` in the tree has only `Single` children — call this a **flat plan** — the original "preselect items, parallel load, serial merge after `g.Wait`" pattern (`loader.go:226-273` today) is correct without any lock. This is also the only shape the current postprocessor produces, so the fast path matches today's behaviour exactly.

Concrete design:

1. At the start of `LoadGraphQLResponseData`, walk `response.Fetches` once and compute `l.useMergeMu bool`. The walk is `O(n)` over fetch-tree nodes;
   it is bounded by the same `n` as the rest of the loader, so cost is negligible.
2. The walk's predicate: any `Parallel` node has at least one child whose `Kind != FetchTreeNodeKindSingle` → `useMergeMu = true`.
3. The three-phase `preparePhase` / `mergePhase` helpers acquire `l.mergeMu` only if `l.useMergeMu` is set:

   ```go
   func (l *Loader) maybeLock() func() {
       if !l.useMergeMu { return func() {} }
       l.mergeMu.Lock()
       return l.mergeMu.Unlock
   }
   ```

4. `resolveParallel` keeps two paths:
   - **Fast path** (existing code, unchanged) when all children are `Single`. Used for every plan today and every plan the new postprocessor produces that happens to be flat.
   - **Nested path** (new, three-phase, lock-aware) when any child is `Sequence` / `Parallel`. Only triggered by the new postprocessor's output for non-flat DAGs.

5. The branch lives directly inside `resolveParallel`:
   ```go
   if allChildrenAreSingle(nodes) {
       return l.resolveParallelFlat(nodes)   // = today's body
   }
   return l.resolveParallelNested(ctx, nodes)
   ```

Result:

- Today's behaviour, today's plans → **same code path, no lock, no overhead.**
- New nested plans → three-phase path with mutex.
- The mutex is dead code (never acquired) for flat plans because `useMergeMu` is false, but kept on the `Loader` so the type doesn't change between paths.

This is what the user asked for: a real bypass, not just "uncontended mutex is cheap". Importantly, it preserves the *existing* fast path verbatim, which removes a class of "did the refactor regress my flat-plan throughput" risk.

### Mutex-bypass tests

- A flat plan executed via the new code path must take the fast path. Verify with a counter on `maybeLock`.
- A nested plan must take the nested path. Same counter.
- The pre-existing `loader_test.go` cases all assume flat plans;
  they must continue to pass under the new `resolveParallel` dispatch with no lock acquisition.

This refactor is a **separate PR** from the postprocessor change. It is behaviour-preserving for any tree the postprocessor produces today, and unlocks the trees the new postprocessor wants to produce.

### Race / correctness verification

- Run the existing parallel-fetch tests (`pkg/engine/resolve/loader_test.go` and `resolve_federation_test.go`) before any postprocessor change to confirm the executor refactor is behaviour-preserving.
- Add a new test that builds a hand-rolled `Parallel(Sequence(A, B), Single(C))` tree (no postprocessor involvement) and asserts that B sees A's data, C sees nothing else, and races are clean under `-race`. The test must exercise the merge interleaving — i.e. branch 1 calling `mergePhase` while branch 2 calls `loadPhase` — to validate the lock discipline. Use a fake datasource that injects controllable delays into `loadPhase` so the interleaving is reproducible.
- Add a `-race` smoke run of `pkg/engine/resolve/...` to CI for this PR (or run it locally and document the command).
- Subscription path: `Trigger` lives at `Fetches.Trigger`, not as a sibling of the root tree (`postprocess.go:225`). The executor refactor does not touch trigger handling. Verify with the existing subscription test.

## Algorithm

After the executor lands, replace `orderSequenceByDependencies` + `createParallelNodes` with a **hybrid scheduler** that runs two complementary algorithms and picks the eager-inline tree only when it provably dominates the level-based tree under any duration distribution; otherwise picks the level-based tree as the safe no-regression fallback.

### Theoretical grounding

For a DAG with precedence constraints, the minimum makespan under unbounded processors equals the **critical path length** (longest path in the DAG, weighted by node duration). For *series-parallel* DAGs, this lower bound is achievable by a Sequence/Parallel tree; recognition and decomposition run in linear time (Valdes, Tarjan, Lawler 1982). For non-SP DAGs, the optimal SP-tree makespan may strictly exceed the critical path — this is a representation limit, not an algorithm limit.

Two natural recurrences yield Sequence/Parallel trees but neither is universally optimal:

- **Level-based with components** (`scheduleLevel`): in the largest connected dependency component, take all ASAP-ready nodes as a `Parallel` block, then recurse on the rest. Multiple components run as `Parallel(component_a_schedule, component_b_schedule)`.
   Optimal under uniform durations on every DAG. Suboptimal under non-uniform (skewed) durations on chain-merge DAGs (e.g. scenario 4) because it forces a "wave" boundary at every level — even when two independent chains could simply run as `Parallel(Sequence(...), Sequence(...))`.
- **Eager-inline SP** (`scheduleSP`): recursively process each ready root and consume its uniquely-dependent descendant chain inline as a `Sequence`. Multi-parent children get deferred to the outer level via a "remaining-parents intersection" merge step.
   Optimal under any durations on series-parallel reducible DAGs. *Suboptimal under uniform durations* on certain non-SP DAGs (scenario 18) because eager-inlining a leaf side-branch of a chain forces an unrelated multi-parent join to wait for the side-branch.

The two regressions are non-overlapping: the DAGs where one is suboptimal, the other is optimal. The hybrid picks per query.

### Input / output

Input: a flat `Sequence` whose children are `Single` fetch nodes, each with `FetchID` and `DependsOnFetchIDs` populated.
Output: a tree of `Sequence`/`Parallel`/`Single` nodes whose execution order respects every dependency edge.

### Hybrid driver

The driver is **dominance-only**. Uniform-makespan comparison is dropped because picking SP based on a strictly-smaller uniform makespan does not by itself imply `makespan_SP ≤ makespan_Level` under arbitrary durations — that implication needs the dominance check, so we apply it unconditionally.

```
buildScheduleTree(roots, dag) -> Tree:
  treeLevel = scheduleLevel(roots, dag)
  treeSP    = scheduleSP(roots, dag)
  validate(treeLevel, dag); validate(treeSP, dag)

  if dominates(treeSP, treeLevel):     return treeSP
  return treeLevel        # SP doesn't provably dominate Level → conservative pick.
```

This driver guarantees `makespan_hybrid(d) ≤ makespan_Level(d)` for every non-negative `d` by the dominance theorem.

Cost: two scheduler runs + one dominance check, each `O(n + m)` to `O(n · (n + m))` worst case for the SP recurrence; dominance is `O(|paths_SP| · |paths_Level| · n)` where `|paths|` is bounded by `b^k`. For typical federation plans (`n ≤ 50`, `b ≤ 5`, `k ≤ 4`) this is well under a millisecond per query plan.

There is no `uniformMakespan` function in the driver. We considered using uniform makespan as a fast-path tie-breaker, but it can give the wrong answer (SP can be uniform-smaller yet not dominate Level), and the dominance check is fast enough that we don't need a fast path. Keeping the driver simple and provably correct trumps a few microseconds.

### Tie-break — symbolic path-set dominance

For two SP-trees `A` and `B` on the same set of leaves (fetch nodes), define `paths(T)` recursively:

- `paths(Single(n)) = { {n} }`
- `paths(Sequence(c1, c2, ..., ck)) = { p1 ∪ p2 ∪ ... ∪ pk : pi ∈ paths(ci) }` (cross product, set-union per element)
- `paths(Parallel(c1, c2, ..., ck)) = paths(c1) ∪ paths(c2) ∪ ... ∪ paths(ck)` (set-union of branch path sets)

`makespan(T, d) = max_{p ∈ paths(T)} sum(d_n for n ∈ p)`.

**Theorem.** Tree `A` dominates `B` (i.e. `makespan_A(d) ≤ makespan_B(d)` for every non-negative duration vector `d`) iff for every `pa ∈ paths(A)` there exists `pb ∈ paths(B)` such that `pa ⊆ pb`.

*Proof sketch.* (⇐) For any `d`, let `pa* = argmax_{pa} sum_d(pa)`. By assumption there is `pb*` with `pa* ⊆ pb*`, so `sum_d(pa*) ≤ sum_d(pb*) ≤ makespan_B(d)`. Hence `makespan_A(d) ≤ makespan_B(d)`. (⇒) If some `pa` is not contained in any `pb`, choose `d` to put weight only on `pa`'s nodes (1 each) and 0 elsewhere. Then `makespan_A(d) = |pa|` while every `pb` is missing at least one node of `pa`, so `makespan_B(d) < |pa|` — contradiction.

This gives the predicate used by the driver:

```
dominates(treeA, treeB) -> bool:
  paths_A = enumeratePaths(treeA)
  paths_B = enumeratePaths(treeB)
  for pa in paths_A:
    if no pb in paths_B with pa ⊆ pb:
      return false
  return true
```

Cost: enumeration is `O(b^k)` where `b` is maximum parallel branching and `k` is parallel depth. For federation plans with `n ≤ 50`, `b` is typically `≤ 5` and `k ≤ 4`, so paths ≤ 625 per tree. Pairwise containment check is `O(|paths_A| × |paths_B| × n)`. Total cost: well under a millisecond per query plan.

The dominance check is **provably correct** — it is not a heuristic. When it returns true, `makespan_A(d) ≤ makespan_B(d)` for every non-negative `d`. When it returns false, there exists at least one `d` for which `makespan_A(d) > makespan_B(d)`. In particular, when neither tree dominates the other, the trees are incomparable: either may win at runtime depending on the actual durations, and at plan time we conservatively pick Level (which is at least as good as today's algorithm under uniform durations by construction).

### Validation that the tie-break is correct on every scenario

For every uniform-tie pair across the 23 scenarios, the symbolic `dominates` check must produce the documented hybrid pick:

- **Scenario 4** (`A→C, B→D, C→E, D→E`): SP paths = `{{A,C,E}, {B,D,E}}`; Level paths = `{{A,C,E}, {A,D,E}, {B,C,E}, {B,D,E}}`. Every SP path is contained in a Level path → SP dominates Level. Pick SP. ✓ (Skew win preserved.)
- **Scenario 18** (`A→B, A→C, B→D, C→D, C→E`): SP tree `Sequence(A, Parallel(B, Sequence(C, E)), D)` has paths `{A, B, D}` (A then Parallel-pick B then D) and `{A, C, E, D}` (A then Parallel-pick Sequence(C, E) traversing C and E, then D). Level tree `Sequence(A, Parallel(B, C), Parallel(D, E))` has paths `{A, B, D}, {A, B, E}, {A, C, D}, {A, C, E}`. SP path `{A, C, E, D}` is not contained in any Level path (each Level path has three nodes; `{A, C, E, D}` has four). Level path `{A, B, E}` is not contained in any SP path. **Neither dominates**. Hybrid falls back to Level. ✓ (This is the canonical motivating case for the hybrid: SP alone would regress here vs Level.)
- **Scenario 22** (`A→B, A→C, A→D, B→C`): SP paths = `{{A,B,C}, {A,D,C}}`; Level paths = `{{A,B,C}, {A,D}}`. SP path `{A,D,C}` not contained in any Level path → SP does NOT path-set-dominate Level → driver falls back to Level. ✓ (Codex round-9 counterexample fixed.)
- **Scenario 23** (`A→D, A→E, B→C, B→E, C→D` — codex round-10 counterexample): SP paths = `{{A,D}, {A,E}, {B,C,D}, {B,C,E}}`; Level paths = `{{A,C,D}, {A,E}, {B,C,D}, {B,E}}`. SP path `{B,C,E}` is not in any Level path (Level has neither `{B,C,E}` nor a superset). Level path `{A,C,D}` is not in any SP path. **Neither dominates** → fall back to Level. ✓ (Avoids the round-10 regression because Level wins under codex's specific skew while SP wins under others; the conservative pick avoids regressing vs today.)
- **Scenarios 19, 21**: SP and Level produce identical trees → `dominates(SP, Level)` is trivially true → driver returns SP (functionally identical to Level since the trees are equal node-for-node).

The property test enforces this for every enumerated DAG.

### Recurrence — `scheduleSP` (eager-inline, component-aware)

`scheduleSP` runs `WCC` at the top of every recursion to partition independent subgraphs *before* the inline-merge step:

```
scheduleSP(nodes, dag):
  if len(nodes) == 0: return nil
  if len(nodes) == 1: return Single(nodes[0])
  components = weakly_connected_components(nodes, dag)
  if len(components) > 1:
    return Parallel(sorted([scheduleSP(c, dag) for c in components]))
  # single component
  return scheduleSP_inline(nodes, dag)
```

`scheduleSP_inline` is the recurrence below. The two mutually recursive functions, `processNode` and `processBatch`, plus a `ProcessingState` that tracks ready-to-process and unhandled (multi-parent-pending) nodes:

```
ProcessingState:
  ready:     ordered list of node IDs whose every parent has been processed
  unhandled: map from node ID -> set of remaining unprocessed parent IDs

processBatch(state, parallel_first, dag) -> (Tree, NewState):
  # Process every node in state.ready in one batch (parallel by default after
  # the first iteration; configurable via parallel_first for the very first call).
  branches = []
  merged   = state with ready=[]
  for r in state.ready (sorted):
    (subtree, sub_state) = processNode(r, dag)
    branches.append(subtree)
    merged = mergeStates(merged, sub_state)     # see below

  if len(branches) == 1:
    batch_node = branches[0]
  else if parallel_first:
    batch_node = Parallel(branches)
  else:
    batch_node = Sequence(branches)

  # `mergeStates` may have surfaced new ready nodes (those whose entire parent
  # set is now contained in `state.ready`); the caller will pick them up.
  return (batch_node, merged.update_for_processed(state.ready))

processNode(n, dag) -> (Tree, State):
  emit_n = Single(n)
  children = direct successors of n in dag
  child_state = ProcessingState{ready=[], unhandled={}}
  for c in children:
    if parents(c) == {n}:
      child_state.ready.append(c)
    else:
      child_state.unhandled[c] = parents(c) - {n}

  if child_state.ready is empty:
    # n has no children, OR all children are multi-parent and remain unhandled.
    return (emit_n, child_state)

  # Recurse — n's uniquely-dependent subtree is processed inline. Multi-parent
  # children stay unhandled and will be resolved at the outer call site by
  # mergeStates.
  inline_sequence = []
  s = child_state
  parallel = true
  while s.ready is not empty:
    (sub_tree, s_after) = processBatch(s, parallel, dag)
    inline_sequence.append(sub_tree)
    parallel = true       # after the first iteration we are always parallel
    s = s_after

  inner = inline_sequence[0] if len(inline_sequence) == 1 else Sequence(inline_sequence)
  return (Sequence(emit_n, inner), s)

mergeStates(a, b):
  # `ready` is the union of both ready lists (deduped, ordered).
  ready = unique(a.ready + b.ready)
  unhandled = {}
  for c in keys(a.unhandled) ∪ keys(b.unhandled):
    pending = a.unhandled.get(c, parents(c)) ∩ b.unhandled.get(c, parents(c))
    # ^ a node listed by only one side keeps its full parent set on the other,
    #   so the intersection across siblings tells us "still unprocessed by anyone".
    if pending is empty:
      ready.append(c)             # all parents processed by some sibling
    else:
      unhandled[c] = pending
  return ProcessingState{ready, unhandled}

schedule(roots, dag) -> Tree:
  state = ProcessingState{ready=roots, unhandled={}}
  main_sequence = []
  parallel = (len(roots) > 1)        # multiple roots → first batch is parallel
  while state.ready is not empty:
    (batch_tree, state_after) = processBatch(state, parallel, dag)
    main_sequence.append(batch_tree)
    parallel = true
    state = state_after
  if state.unhandled is not empty:
    error("cycle detected") — see scenario 15

  return main_sequence[0] if len(main_sequence) == 1 else Sequence(main_sequence)
```

The intersection rule in `mergeStates` is the critical piece. When two sibling roots `A` and `B` both have child `X` (with `parents(X) = {A, B}`), `processNode(A)` reports `X` as unhandled with pending `{B}`, and `processNode(B)` reports `X` as unhandled with pending `{A}`. The intersection `{B} ∩ {A} = ∅`, so `X` becomes ready in the merged state and runs *after* the parallel block of `A` and `B` — at the outer level, where it belongs.

Sort key for `Parallel` children: by the minimum `FetchID` reachable in each branch. Determinism is required by the structural assertions in `query_order_baseline_test.go`.

### Recurrence — `scheduleLevel` (level-based with components)

```
scheduleLevel(nodes, edges):
  if len(nodes) == 0: return nil
  if len(nodes) == 1: return Single(nodes[0])

  components = weakly_connected_components(nodes, edges)
  if len(components) > 1:
    return Parallel(sorted([scheduleLevel(c, edges_within(c)) for c in components]))

  # Single connected component, multiple nodes.
  roots = nodes with no parent in `nodes`
  rest  = nodes - roots
  rest_edges = edges restricted to rest

  rest_schedule = scheduleLevel(rest, rest_edges)
  if len(roots) == 1:
    return Sequence(Single(roots[0]), rest_schedule)
  return Sequence(Parallel(sorted([Single(r) for r in roots])), rest_schedule)
```

This is the rev 4.1 algorithm. It handles independent components but synchronises at every level inside a component. Optimal under uniform durations on every DAG.

### Worked examples (hybrid output)

For each, the table shows what each scheduler produces and what the hybrid picks.

1. **Baseline** — `user, org, practice; user→practice`.
   - `scheduleSP`: `Parallel(Sequence(user, practice), org)`. Paths: `{user, practice}, {org}`.
   - `scheduleLevel`: components `{user, practice}, {org}` → `Parallel(Sequence(user, practice), org)`. Paths: `{user, practice}, {org}`.
   - Identical trees; SP trivially dominates Level (every path on each side is in the other) → hybrid picks SP. Same shape either way. Critical path achieved.

2. **Diamond** — `A→B, A→C, B→D, C→D`.
   - `scheduleSP`: inline-recurse from A. Children {B, C} → parallel. Each emits D as unhandled; merge promotes D to outer iteration. Result: `Sequence(A, Parallel(B, C), D)`.
   - `scheduleLevel`: roots={A}; recurse on {B, C, D} → roots={B, C}; recurse on {D}. Result: `Sequence(A, Parallel(B, C), D)`.
   - Identical trees; `dominates(SP, Level)` trivially true → hybrid picks SP. Same shape either way.

3. **Independent chains** — `A→B, C→D`.
   - `scheduleSP`: `Parallel(Sequence(A, B), Sequence(C, D))`.
   - `scheduleLevel`: components `{A, B}, {C, D}` → same.
   - Identical trees; `dominates(SP, Level)` trivially true → hybrid picks SP. Same shape either way.

4. **Two-chains-join** — `A→C, B→D, C→E, D→E`.
   - `scheduleSP`: `Sequence(Parallel(Sequence(A, C), Sequence(B, D)), E)`. Paths: `{A, C, E}, {B, D, E}`.
   - `scheduleLevel`: one component, roots={A, B}, rest={C, D, E}; recurse → `Sequence(Parallel(A, B), Parallel(C, D), E)`. Paths: `{A, C, E}, {A, D, E}, {B, C, E}, {B, D, E}`.
   - Every SP path is contained in a Level path → **SP path-set-dominates Level** → hybrid picks SP. Under skewed durations (e.g. `A=1, B=100, C=100, D=1`) SP makespan is `101+E` and Level is `200+E`; SP wins by 99.

5. **Branch-and-merge** — `A→B, B→C, A→D` (D independent of B/C).
   - `scheduleSP`: `Sequence(A, Parallel(Sequence(B, C), D))`.
   - `scheduleLevel`: roots={A}; rest={B, C, D} with `B→C` only → components `{B, C}, {D}` → `Parallel(Sequence(B, C), D)`. Result: `Sequence(A, Parallel(Sequence(B, C), D))`.
   - Identical trees; `dominates(SP, Level)` trivially true → hybrid picks SP. Same shape either way.

6. **Asymmetric chain merge with leaf** — `A→B, A→C, B→D, C→D, C→E` (scenario 18).
   - `scheduleSP`: `Sequence(A, Parallel(B, Sequence(C, E)), D)`. Paths: `{A, B, D}, {A, C, E, D}`.
   - `scheduleLevel`: `Sequence(A, Parallel(B, C), Parallel(D, E))`. Paths: `{A, B, D}, {A, B, E}, {A, C, D}, {A, C, E}`.
   - SP path `{A, C, E, D}` not contained in any Level path → SP does NOT dominate Level. Level path `{A, B, E}` not contained in any SP path → Level does NOT dominate SP. **Incomparable** → hybrid falls back to Level. (Critical-path-optimal under uniform durations on this DAG via Level.)

### Acknowledged optimality gap

The hybrid picks SP only when SP path-set-dominates Level (provably never worse under any duration distribution); otherwise picks Level. Neither tree is always optimal across all duration distributions, so two residual gaps remain:

**Gap A — non-SP DAGs (representation limit).** The "N-shape" DAG `A→C, A→D, B→D, B→E` is not series-parallel reducible. Both schedulers must produce one of the two SP trees:
- `scheduleSP`: `Sequence(Parallel(Sequence(A, C), Sequence(B, E)), D)`. With `A=B=C=E=10, D=1000`: makespan 1020.
- `scheduleLevel`: roots={A, B}, rest={C, D, E}, components `{C}, {D}, {E}` → `Sequence(Parallel(A, B), Parallel(C, D, E))`. Same durations: makespan `max(A,B) + max(C, D, E) = 10 + 1000 = 1010`.

Critical path of the DAG = `max(A+D, B+D) = 1010`. `scheduleLevel` matches; `scheduleSP` is `D` units longer because it forces `D` to wait for the entire parallel block including `E`. Hybrid picks `scheduleLevel` here. This is captured as **scenario 20** with the level-output asserted.

There is no SP tree on this DAG that matches the critical path *under all duration distributions*. A different runtime model (per-parent dependencies on individual nodes inside a Parallel block) would be needed, and that is genuinely out of scope.

**Gap B — incomparable trees with skew-only differences.** On scenario 4 with `A=1, B=100, C=100, D=1`, `scheduleSP` produces makespan `101+E` and `scheduleLevel` produces `200+E`. SP path-set-dominates Level on this DAG, so the dominance check picks SP and the runtime makespan is `101+E` — matching critical path.

On scenario 23 (`A→D, A→E, B→C, B→E, C→D`), SP and Level are *incomparable*: each tree's path set has a path not contained in the other's. The dominance check returns false, the hybrid falls back to Level. Some duration distributions (like `A=100, B=1, C=1, D=1, E=1`) would have favoured SP at runtime (SP=101 vs Level=102), and we leave that 1-unit win on the table. This is the documented gap of the conservative fallback.

The hybrid never regresses against `scheduleLevel` (provable from the dominance theorem) but may underperform `min(scheduleSP, scheduleLevel)` on incomparable DAGs at runtime under some duration distributions. Closing that gap requires runtime statistics (which we do not collect today) or a richer runtime model (per-parent dependencies inside `Parallel` blocks, which would change the executor contract — out of scope).

Federation DAGs are nearly always SP-reducible in practice (entity-extension trees with occasional joins) and the SP/Level trees are very often comparable. When they are not, the runtime loss is bounded by the difference between the trees' makespans on the actual durations, which is small in typical federation workloads.

### Correctness sketch

Every emitted edge is honoured.
A node only appears in `rest_schedule` after it appears in `roots`,
so transitive dependencies are respected.
Two nodes are placed in the same `Parallel` only if there is no path between them in either direction (otherwise they would not both be in `roots` of the current call,
nor would they live in distinct components).
Therefore the schedule is a valid topological execution.

This sketch assumes `DependsOnFetchIDs` plus `addMissingNestedDependencies` together form a *complete* dependency graph. That assumption is fragile (see next section);
the plan adds a post-build validator to catch violations.

### Dependency completeness — the part that scared codex

`addMissingNestedDependencies.go:21` only adds response-path-derived dependencies when the node has *zero* existing fetch-ID deps:

```go
if len(node.Item.Fetch.(*resolve.SingleFetch).FetchDependencies.DependsOnFetchIDs) != 0 {
    continue
}
```

Implication: a node that already depends on an entity-key upstream (e.g. `practice → user`) will *not* additionally pick up a response-path-derived dependency on a sibling (e.g. an unrelated upstream that materialises a parent object on the same response path).
Today this is hidden by the level-barrier serialisation;
under the new scheduler an under-declared dependency could land on the wrong side of a `Parallel` boundary and execute before its real prerequisite.

We do not fix `addMissingNestedDependencies` in this change.
We *guard against* incomplete dependencies with a validator that runs after `buildScheduleTree`.

The validator must not just compare the immediate children of each `Parallel` node — that would miss `Parallel(Sequence(A, B), Sequence(C, D))` cases where `B → C` is the hidden edge. It must flatten *every leaf fetch* under each `Parallel` child and check every cross-branch leaf pair.

It must also use the right notion of "provided path." `addMissingNestedDependencies` (`add_missing_nested_dependencies.go:35-41`) computes `provided = ResponsePath + "." + MergePath` where `MergePath` lives on `SingleFetch.PostProcessing`. After `createConcreteSingleFetchTypes` runs, the leaf may be `*SingleFetch`, `*EntityFetch`, or `*BatchEntityFetch`. The validator therefore needs a fetch-kind-aware helper:

```
providedPath(node) →
  switch fetch := node.Item.Fetch.(type) {
  case *SingleFetch:        merge := fetch.PostProcessing.MergePath
  case *EntityFetch:        merge := fetch.PostProcessing.MergePath
  case *BatchEntityFetch:   merge := fetch.PostProcessing.MergePath
  }
  return joinDot(node.Item.ResponsePath, merge...)
```

The validator algorithm:

```
validate(tree):
  for every Parallel node P:
    branchLeaves := [flattenLeaves(child) for child in P.Children]
    for each pair of branches (i, j):
      for each leaf a in branchLeaves[i]:
        for each leaf b in branchLeaves[j]:
          assert a.FetchID not in b.Fetch.Dependencies().DependsOnFetchIDs
          assert b.FetchID not in a.Fetch.Dependencies().DependsOnFetchIDs
          assert providedPath(a) is not a strict prefix of b.Item.ResponsePath
          assert providedPath(b) is not a strict prefix of a.Item.ResponsePath
          assert providedPath(a) != providedPath(b)
            (matching paths suggest one fetch produces what the other consumes)
  for every Sequence node:
    each child must topologically follow its predecessors;
    assert by walking through and tracking provided fetch IDs in order.
```

Use `Fetch.Dependencies()` rather than a `*SingleFetch` cast — see the type-discipline note below.

If the validator fails on a real query, the planner has an undeclared dependency. Surface it with a descriptive panic during postprocess and add a regression test for the operation. Treating the symptom (re-serialising) would just hide the bug.

### Type discipline after `createConcreteSingleFetchTypes`

Both the scheduler and the validator run *after* `createConcreteSingleFetchTypes`, so leaf fetches are no longer guaranteed to be `*SingleFetch`. They may be `*EntityFetch` or `*BatchEntityFetch`. Use:

- `fetch.Dependencies()` (the interface method) for `FetchID` / `DependsOnFetchIDs`.
- A small fetch-kind switch helper for `MergePath` (`PostProcessing` lives on each concrete type, not on the interface).

Avoid any `node.Item.Fetch.(*resolve.SingleFetch)` cast in the new code.

## Implementation

### Files

- New: `v2/pkg/engine/postprocess/build_schedule_tree.go` — the new processor (`buildScheduleTree`) plus `validateSchedule`.
- New: `v2/pkg/engine/postprocess/build_schedule_tree_test.go` — unit tests covering the worked examples plus edge cases (single node, all-independent, all-chained, mixed components, deep nesting, the codex known-suboptimal case).
- Modified: `v2/pkg/engine/resolve/loader.go` — refactor `resolveParallel` to dispatch through `resolveFetchNode`. Add a context-aware `resolveFetchNodeWithCtx`.
- Modified: `v2/pkg/engine/postprocess/postprocess.go` — add a `disableBuildScheduleTree` option and a `BuildScheduleTree()` opt-in. **Keep the old processors wired up by default.** When `BuildScheduleTree()` is set, the new processor *replaces* the old pair (not aliases — explicit replacement).
- Untouched (this revision): `order_sequence_by_dependencies.go`, `create_parallel_nodes.go`, `add_missing_nested_dependencies.go`. Old processors stay until the new path is proven on a meaningful sample of fixtures.

### Wiring order

After `addMissingNestedDependencies` (so dependency edges are populated) and after `createConcreteSingleFetchTypes` (because the current order is `addMissingNestedDependencies → createConcreteSingleFetchTypes → orderSequenceByDependencies → createParallelNodes`, see `postprocess.go:112-127`).
The new processor takes the slot of the last two:

```
addMissingNestedDependencies → createConcreteSingleFetchTypes → buildScheduleTree → validateSchedule
```

(Old wiring stays as a parallel option behind the existing disable flags. We do *not* change the default.)

### Migration: opt-in first

Concretely:

1. Land the executor refactor with no postprocessor changes. Run the full repo suite. This is a behaviour-preserving change.
2. Land `buildScheduleTree` + `validateSchedule` + the new option, *off by default*. Run the new processor's unit tests.
3. Add a new option (likely a config flag on the planner or the processor: `WithBuildScheduleTree()`) plus a few targeted integration tests that opt in. Verify on the baseline, the codex case, and 2-3 representative federation tests.
4. **Stop here for the first PR.** No production fixtures change. No old processors deleted. No default behaviour change.

A follow-up PR flips the default, audits fixture diffs, and deletes the old processors. That PR's scope is "rewrite golden plans"; keeping it separate makes review tractable.

## Test strategy

### Unit tests for the new processor

Port every existing case from `create_parallel_nodes_test.go` and `order_sequence_by_dependencies_test.go` into `build_schedule_tree_test.go` with the *expected* output recomputed under the new algorithm.
Add new cases:

- Independent chains: `A→B, C→D` → `Parallel(Sequence(A, B), Sequence(C, D))`.
- Branch-and-merge: `A→B, B→C, A→D` → `Sequence(A, Parallel(Sequence(B, C), D))`.
- Three independent singletons: `A, B, C` (no edges) → `Parallel(A, B, C)` (matches today).
- Deep transitive: `A→B→C→D, plus E→F` → `Parallel(Sequence(A, B, C, D), Sequence(E, F))`.
- Empty input → no-op.
- Single node → unchanged.
- **Known-suboptimal** (codex): `A→C, B→D, C→E, D→E` → asserts the level-grouped output `Sequence(Parallel(A, B), Parallel(C, D), E)` and is annotated with a comment naming the better SP-decomposition. A future PR that improves the algorithm flips this fixture.

### Validator tests

- Hand-built tree with a missing fetch-id edge across a `Parallel` boundary must panic via `validateSchedule`. Include a "deep" variant where the edge crosses leaves nested inside `Sequence` children of `Parallel`, to confirm leaf-flattening.
- Hand-built tree with a response-path containment across a `Parallel` boundary must panic.
- Hand-built tree where two leaves on opposite branches share an identical `providedPath` must panic.
- Hand-built tree using `*EntityFetch` and `*BatchEntityFetch` leaves (with their own `PostProcessing.MergePath`) must apply the same checks correctly. Confirms the fetch-kind helper is wired.
- Hand-built tree where everything is consistent must not panic.
- Hand-built tree with a cycle (or with a non-empty connected component that has no roots) must panic with a clear "cycle detected" message — also covers the cycle-detection risk above.

### Property test (cross-check hybrid vs both individual schedulers vs critical path)

Three layers:

1. **Exhaustive enumeration for `n ≤ 6`.** Enumerate every DAG on up to 6 nodes (acyclic, no parallel edges) and assert:
   - `scheduleSP`, `scheduleLevel`, and `buildScheduleTree` outputs respect every edge.
   - `validateSchedule` accepts every output of all three.
   - `uniformMakespan(buildScheduleTree) ≤ uniformMakespan(scheduleLevel)` — the hybrid's no-regression-vs-Level guarantee under uniform durations (provable from the dominance theorem; this is the test-side check).
   - When `dominates(scheduleSP, scheduleLevel)` is true (computed by the test helper), `buildScheduleTree == scheduleSP`; otherwise `buildScheduleTree == scheduleLevel`. This pins the dominance-only driver.
   - For every DAG that is **series-parallel reducible**, `min(uniformMakespan(scheduleSP), uniformMakespan(scheduleLevel))` matches the DAG's uniform critical-path lower bound. SP-recognition uses the linear-time decomposition (Valdes-Tarjan-Lawler 1982) implemented in a small test helper. We do not assert that the hybrid itself matches CP under uniform on every SP DAG — it does so only when one of the two trees dominates the other. Incomparable SP-reducible DAGs (rare but possible) fall back to Level, which still matches CP under uniform but may not match it under skew.
   - For non-SP DAGs, the hybrid output's makespan may exceed the DAG's critical path; the gap is logged. Any DAG where the gap exceeds 1 is flagged for manual review.

   Cheap and catches embarrassing counterexamples.

2. **Random DAGs for `n=3..15`** with average degree ~1.5, fixed seed in CI. Same assertions.

3. **Random DAGs for `n=50, 200`** as smoke checks for the larger-graph runtime; assert no regression in uniform makespan vs the level-based output of today, and `validateSchedule` accepts every output.

4. **Random-skew evaluation.** For *every* DAG produced by layers 1, 2, and 3, sample 50 random duration vectors per DAG (each component drawn from a heavy-tailed distribution, e.g. `LogUniform(1, 1000)`). For each (DAG, durations) pair compute makespan of the hybrid output, makespan of `scheduleSP`, and makespan of `scheduleLevel`. Assert:
   - `hybrid_makespan(d) ≤ Level_makespan(d)` — the hybrid never regresses against `scheduleLevel`, which is the no-regression-vs-today guarantee. This is provable from the dominance theorem (hybrid returns SP only when it dominates Level; otherwise hybrid IS Level).
   - When the hybrid output is `scheduleSP` (which only happens when SP dominates Level): assert `SP_makespan(d) ≤ Level_makespan(d)` — also provable from dominance.
   - Log every `(DAG, d)` where the hybrid picked Level *and* `SP_makespan(d) < Level_makespan(d)` — these are the cases where the conservative fallback gave up a runtime win on an incomparable DAG. Track per-DAG-class to quantify the cost of the safety fallback.

   This catches any algorithmic bug or accidental tie-break regression at scale. The first assertion is *the* correctness invariant for the hybrid; the second is a sanity check on the dominance check; the log is observational.

   We explicitly do NOT assert `hybrid_makespan(d) ≤ min(SP_makespan(d), Level_makespan(d))` — that is false on incomparable DAGs (scenario 23 itself: with `A=100, B=1, C=1, D=1, E=1`, SP=101, Level=102, hybrid picks Level=102 > min=101). The hybrid trades that strict bound for the no-regression-vs-Level guarantee.

5. **Skew-stress for fixtures.** For each of scenarios 1-23, evaluate the hybrid's chosen output under a fixed list of skewed duration distributions (heavy on roots, heavy on leaves, heavy on inline-chain interior, heavy on multi-parent join). Assert `hybrid_makespan(d) ≤ Level_makespan(d)` (the no-regression-vs-Level guarantee, also provable from dominance and asserted at random scale in layer 4). Where the hybrid picked Level and the SP tree would have been better, log the gap; do not fail. Hand-curated complement to layer 4.

The honest claim, verified by the property test:

- `buildScheduleTree` is **critical-path-optimal on every DAG under uniform durations whenever one of `scheduleSP` or `scheduleLevel` dominates the other**. When the trees are incomparable the hybrid picks Level (which is critical-path-optimal under uniform durations on every DAG by construction), so under uniform durations the hybrid still matches CP. Under skewed durations on incomparable DAGs the hybrid may exceed CP because the dominance fallback is conservative.
- `buildScheduleTree` **never regresses against `scheduleLevel`** under any duration distribution. Proof: the symbolic dominance check returns SP only when SP path-set-dominates Level, which by the dominance theorem implies `makespan_SP(d) ≤ makespan_Level(d)` for every duration vector `d`. On non-dominance, the hybrid returns Level. So `makespan_hybrid(d) ≤ makespan_Level(d)` always.
- `buildScheduleTree` may also be **strictly better than `scheduleLevel`** on every DAG where SP dominates Level. This is the chain-merge family (scenario 4 and similar SP-reducible DAGs).
- For DAGs where SP and Level are *incomparable* (codex round-10 example, scenario 23), the hybrid picks Level conservatively. On those DAGs, runtime makespan may exceed the per-distribution optimum (sometimes SP would have been better, sometimes Level — without runtime statistics we cannot tell). The runtime gap is bounded by `max(makespan_SP(d), makespan_Level(d)) − min(makespan_SP(d), makespan_Level(d))` under the actual `d`, which is small in practice (typically 1-10% on incomparable DAGs).

### Baseline test update

`query_order_baseline_test.go` will need its `expectedQueryOrderBaselinePlan` constant updated to the target shape *only when the new processor is enabled*.
Implementation: parameterise the test on the option, run it twice — once with old wiring (asserting today's shape) and once with new wiring (asserting the target shape).
This keeps the baseline doing its job (regression guard) on both code paths.

The structural invariants (`assertQueryOrderBaselineShape`) become two functions, one per shape.

### Wider planner test fixtures

No fixture churn in this PR. The follow-up PR that flips the default will audit and update fixtures with one-line justifications per change.

`execution/` e2e tests should pass with the new wiring once enabled — they assert response shape, not plan shape. Verify by running the new wiring under a build tag or env var locally before flipping defaults.

### Option-precedence tests

`postprocess.NewProcessor()` accepts `DisableOrderSequenceByDependencies()`, `DisableCreateParallelNodes()`, and the new `BuildScheduleTree()`. Tests must cover:

- Old wiring (no opt-in flag): both old processors run.
- New wiring (`BuildScheduleTree()`): old pair is skipped regardless of the disable flags;
  new processor + validator run.
- `DisableOrderSequenceByDependencies()` alone (existing API): only `createParallelNodes` runs. Behaviour preserved.
- `DisableCreateParallelNodes()` alone (existing API): only `orderSequenceByDependencies` runs. Behaviour preserved.
- Combinations: `BuildScheduleTree() + DisableOrderSequenceByDependencies()` — the disable flag is a no-op when the new processor is on. Document this in the option's godoc.

### Performance

Add `BenchmarkBuildScheduleTree` covering 10, 50, 200 nodes with a representative DAG. Informational, not gating.

## Risks (post-codex)

- **Executor refactor regressions.** The change to `resolveParallel` is small but threads through the loader's context and merge logic.
  Mitigation: land it as a standalone PR with no postprocessor change and watch for race-test failures on `-race`.
- **Hidden dependency edges.** As above, `addMissingNestedDependencies` is conservative.
  Mitigation: `validateSchedule` panics loudly. The new processor stays opt-in until we have run it on enough representative queries.
- **Undefined behaviour with cyclic deps.** A planner bug could yield cycles. New algorithm would loop;
  validator would detect (no roots in non-empty connected component) and panic.
  Mitigation: explicit cycle check at the top of `buildScheduleTree`.
- **Subscription trigger.** Trigger lives at `Fetches.Trigger`, separate from the response tree.
  Mitigation: the new processor only operates on the fetch-tree root, not on the trigger. Existing subscription tests must still pass.
- **Batch entity fetches.** `BatchEntityFetch` participates in the same `FetchDependencies` model. The scheduler treats it the same as `SingleFetch`.
  Mitigation: include a batch-entity case in the unit tests.
- **API break.** The new option must coexist with `DisableOrderSequenceByDependencies` and `DisableCreateParallelNodes` (`postprocess.go:58, 83`). Some callers disable only one;
  the new option does not collapse them.
  Mitigation: keep the existing flags as separate controls and document the precedence (if `BuildScheduleTree` is set, the old pair is bypassed regardless of the disable flags).

## Phase 0 — Scenario library (do this before coding)

Before touching the executor or postprocessor, build a fixture library of federation query plans we will exercise the new algorithm and validator against. Without it, we will over-fit to the baseline.

Sources reviewed:

- Existing federation test scenarios in `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_federation_test.go` (composite keys, requires chains, provides, nested key fields, mutations with entity calls, complex requires across 3 subgraphs).
- Federation specification — confirms the canonical query-plan shapes are `Sequence`, `Parallel`, `Fetch`, `Flatten`, `Defer`, plus interface expansion. Notes that `@shareable` over-use and interface implementations expand the plan space, and that `@requires` chains force serial fetches.

The scenarios below are concrete shapes we want to verify. Each becomes a file under `v2/pkg/engine/postprocess/scenarios/` (one Go file per scenario) that builds a synthetic flat-`Sequence` `FetchTreeNode` matching the expected planner emission, runs `buildScheduleTree` + `validateSchedule`, and asserts the resulting tree shape.

Scenarios need not require running the planner end-to-end — building the input tree by hand keeps the test focused on the postprocessor and avoids fixture drift from unrelated planner changes. End-to-end coverage already exists in `graphql_datasource_federation_test.go` and will catch any post-flip regression.

### Scenario 1 — independent components (the baseline)

`user → practice; org`. Three subgraphs, two components.

Today: `Sequence(Parallel(user, org), practice)`.
New: `Parallel(Sequence(user, practice), org)`.

### Scenario 2 — single chain

`A → B → C → D`. One subgraph chain via @key entity hops.

Today and new: `Sequence(A, B, C, D)`. Sanity-check that the new algorithm does not introduce phantom parallelism on a pure chain.

### Scenario 3 — diamond join

`A → B, A → C, B+C → D`. (e.g. `User → Profile`, `User → Org`, `Profile + Org → Audit`.)

Today and new: `Sequence(A, Parallel(B, C), D)`. Sanity that within a single component the level-based pattern is still emitted when correct.

### Scenario 4 — two chains joining at a third subgraph

`A → C, B → D, C → E, D → E`. (Two parallel entity chains both feeding a third subgraph.)

Today: `Sequence(Parallel(A, B), Parallel(C, D), E)`. Critical path `max(A,B) + max(C,D) + E`.
New: `Sequence(Parallel(Sequence(A, C), Sequence(B, D)), E)`. Critical path `max(A+C, B+D) + E`. **SP-optimal.**

This is the case the multi-parent-merge step in `mergeStates` was added for. Validates that the inner merge intersection correctly identifies E as ready only after *both* sibling branches have processed their share of its parents.

### Scenario 5 — wide fan-out

`A → {B, C, D, E}`. (One root entity, four independent extensions.)

Today: `Sequence(A, Parallel(B, C, D, E))`.
New: same. The win for fan-out is already captured by the existing algorithm.

### Scenario 6 — wide fan-in

`{A, B, C, D} → E`. (Four root-level fetches whose entity keys all feed a fifth subgraph that resolves a join field.)

Today: `Sequence(Parallel(A, B, C, D), E)`.
New: same.

### Scenario 7 — independent diamonds

Two diamonds that share no nodes:
`A1 → {B1, C1}, B1+C1 → D1` and `A2 → {B2, C2}, B2+C2 → D2`.

Today: `Sequence(Parallel(A1, A2), Parallel(B1, C1, B2, C2), Parallel(D1, D2))`.
New: `Parallel(Sequence(A1, Parallel(B1, C1), D1), Sequence(A2, Parallel(B2, C2), D2))`. Major win: the two diamonds run fully in parallel instead of synchronising at every level.

### Scenario 8 — `@requires` chain

`U` has `email` resolved by subgraph A. Subgraph B's `User.shippingFee` `@requires email`.

Plan: `Sequence(Fetch(A: email), Fetch(B: shippingFee))` — strictly serial. The new algorithm must not "optimise" this into parallel: A's fetch ID must appear in B's `DependsOnFetchIDs`.

Validation: this scenario tests that the validator does **not** flag the serial sequence as suboptimal. Confirms the algorithm respects `@requires`-induced edges and does not need a special case for them.

### Scenario 9 — list-typed entity batched fetch

Query returns `users: [User!]!` from A, each User has `posts` from B. B's fetch is a `BatchEntityFetch`.

Plan: `Sequence(Fetch(A: users), BatchEntityFetch(B: posts for all users))`.
With another independent root-level fetch X: `Sequence(Parallel(A, X), BatchEntityFetch(B))` today; new: `Parallel(Sequence(A, BatchEntityFetch(B)), X)`. Tests that `BatchEntityFetch` participates in scheduling identically to `SingleFetch` and that the validator handles the batch fetch's `MergePath` via the kind helper.

### Scenario 10 — nested entity (`User.manager: User`)

Query: `me { manager { manager { name } } }`. Three sequential entity fetches into the same subgraph (or different subgraphs that all extend `User`).

Plan: `Sequence(A: me, B: manager, B: manager.manager)`. Tests deep response-path containment in the validator (each fetch's response path is a prefix of the next's).

### Scenario 11 — interface expansion

Query selects fields on an interface that has implementations across multiple subgraphs.

Plan typically expands into per-implementation fetches under a single `Flatten`. The flat-`Sequence` input may include several `_entities` fetches keyed by different concrete types. The algorithm should treat them as independent unless a dependency edge says otherwise.

### Scenario 12 — `@provides` skips a fetch

Subgraph A's root field `topUser: User` `@provides "name"`. Operation selects only `topUser.name` — the `B: User.name` fetch is *not* emitted.

Plan: `Fetch(A: topUser.name)` — single fetch, no entity hop.

This scenario tests that the new algorithm and validator behave correctly on a single-fetch input and do not introduce empty `Sequence`/`Parallel` wrappers.

### Scenario 13 — sequential mutation

Two mutations at the operation root. Federation forces serial execution per the GraphQL spec.

The current planner already encodes this in `DependsOnFetchIDs` (verified by codex's read of `path_builder_visitor.go`): each mutation root planner appends *all previous* mutation root planner IDs to its `DependsOnFetchIDs`. The existing `TestGraphQLDataSourceFederation_Mutations / serial mutations` case (`graphql_datasource_federation_test.go:528`) confirms the shape:

- `M0` (deps `[]`)
- `M1` (deps `[0]`)
- `M2` (deps `[0, 1]`)

So the new scheduler, given this input, emits `Sequence(M0, M1, M2)` naturally. No `mustSerialize` flag, no operation-type threading.

The fixture for this scenario must model the chained edges as the planner produces them. **Do not** test edgeless mutation roots — that input cannot occur, and asserting against it would push us to invent a flag that solves a non-problem.

Add an explicit fixture that asserts the chained-edge output remains `Sequence(M0, M1, M2)` and that the validator does not flag it.

### Scenario 14 — empty / single-fetch tree

No edges, single child. Algorithm returns the node unchanged. Validator no-ops.

### Scenario 15 — cycle (planner bug)

Two nodes with `A.deps=[B]` and `B.deps=[A]`. Algorithm must detect (no roots in non-empty connected component).

In normal postprocess flow this surfaces as an `error` returned from `buildScheduleTree.ProcessFetchTree` (or the equivalent error-returning entry point). It must *not* panic — a planner bug should fail the request with a clear diagnostic, not crash the router. Unit tests assert on the error type and message.

A `panic` is appropriate only inside the validator's hand-built test harness, where the test author intentionally constructs a malformed input to exercise the validator. Production code paths return errors.

### Scenario 16 — composite key fan-in (multi-parent dependency)

A common federation pattern: an entity with a composite key `@key(fields: "id sku")` resolved across two upstream subgraphs that each provide one of the key fields.

- `Subgraph A` resolves `id` via `Query.products`.
- `Subgraph B` resolves `sku` for that same product via an entity hop on `id`.
- `Subgraph C` resolves the final field via an entity hop that requires both `id` and `sku`.

DAG: `A → B, A → C, B → C`. (C depends on both A and B; B depends on A.)

Today: `Sequence(A, B, C)`. Note that today's level-based output would be `Sequence(A, Parallel(B, ?), C)` only if there were a sibling at B's level — there isn't, so it collapses to `Sequence(A, B, C)`.

New: `Sequence(A, B, C)`. Same shape. This scenario is included to confirm the algorithm handles composite-key chains cleanly and the validator accepts the multi-parent edge `B → C` and `A → C` simultaneously.

Reference: existing scenarios in `graphql_datasource_federation_test.go:1383` ("composite keys") and `1916` ("query having a fetch after fetch with composite key") show real planner output for this pattern;
mirror them when constructing the fixture.

### Scenario 17 — response-path-derived dependency (validator stress test)

This case targets the validator, not the algorithm, and probes the part of the system codex flagged as fragile (`addMissingNestedDependencies` and the conservative skip when a node already has any `DependsOnFetchIDs`).

Setup: two leaves `X` and `Y` with no fetch-ID edge between them, but `Y.ResponsePath` is a strict prefix of `X.ResponsePath` (i.e. Y produces a parent object that X needs to read from). Today's `addMissingNestedDependencies` would add `Y → X` only if X has zero existing deps. If X already depends on something else, that path-derived edge is *not* added — and a buggy planner emission could put X and Y on opposite sides of a `Parallel` boundary.

The validator must detect this: `providedPath(Y)` is a strict prefix of `X.Item.ResponsePath` across a `Parallel` boundary → error.

Fixture builds the tree by hand to bypass the planner and then asserts `validateSchedule` returns an error naming both fetch IDs. Confirms the validator's response-path check is wired correctly even when `addMissingNestedDependencies` would have skipped the input.

Reference: the conservative skip is at `add_missing_nested_dependencies.go:21`. The path construction (`ResponsePath + "." + MergePath`) is at `:35-41`.

### Scenario 18 — asymmetric chain merge with leaf side-branch

`A → B, A → C, B → D, C → D, C → E`. One root, one join (`D` depends on both `B` and `C`), and one extra leaf `E` only dependent on `C`.

This DAG is **not** series-parallel reducible — `C` has out-degree 2 (`D` and `E`) where one of its successors (`D`) is also reached from a sibling (`B`).

- `scheduleSP`: `Sequence(A, Parallel(B, Sequence(C, E)), D)`. Uniform makespan 4. The leaf side-branch `E` is dragged into `C`'s sequence, blocking `D`.
- `scheduleLevel`: roots `{A}`; rest `{B, C, D, E}` is one component → roots `{B, C}`, rest `{D, E}` (two components: `{D}`, `{E}`) → `Parallel(D, E)`. Result: `Sequence(A, Parallel(B, C), Parallel(D, E))`. Uniform makespan 3.
- Hybrid picks **Level**. Critical-path-optimal under uniform durations.

This scenario is the canonical motivating case for the hybrid: the eager-inline algorithm alone would regress here against today's level-based output. The hybrid's dominance-based selection rule rejects SP (it does not path-set-dominate Level) and falls back to Level, eliminating the regression.

### Scenario 19 — deep multi-parent fan-in

Three roots, all feeding a single deep node:
`A → X, B → X, C → X, X → Y, Y → Z`.

Trace: `processBatch({A, B, C}, parallel)`. Each child returns `Single(root)` with X unhandled (each missing two parents). Merge intersection: X unhandled-parent intersection is empty → X ready.
Outer second iteration: `processNode(X)` → child Y uniquely dependent → recurse → `Sequence(X, Y, Z)`.

Result: `Sequence(Parallel(A, B, C), X, Y, Z)`.

Confirms that 3-way (and N-way) intersections in `mergeStates` work, not just pairwise.

### Scenario 23 — incomparable SP and Level trees (symbolic dominance fallback test)

`A → D, A → E, B → C, B → E, C → D`. Two roots, multi-parent leaves `D` and `E`, with `C` as an intermediate join feeding `D`.

- `scheduleSP`: `Sequence(Parallel(A, Sequence(B, C)), Parallel(D, E))`. Uniform makespan `max(1, 2) + 1 = 3`. Paths: `{A,D}`, `{A,E}`, `{B,C,D}`, `{B,C,E}`.
- `scheduleLevel`: `Sequence(Parallel(A, B), Parallel(Sequence(C, D), E))`. Uniform makespan `1 + max(2, 1) = 3`. Paths: `{A,C,D}`, `{A,E}`, `{B,C,D}`, `{B,E}`.
- Uniform tie at 3. Symbolic dominance check:
  - SP path `{B, C, E}` is not contained in any Level path → SP does not dominate Level.
  - Level path `{A, C, D}` is not contained in any SP path → Level does not dominate SP.
  - **Incomparable** → fall back to Level.
- Hybrid pick: Level. Output `Sequence(Parallel(A, B), Parallel(Sequence(C, D), E))`.
- Verification: under skew `A=25, B=227, C=647, D=3, E=5`, Level makespan `max(25, 227) + max(647+3, 5) = 227 + 650 = 877`; SP makespan `max(25, 227+647) + max(3, 5) = 874 + 5 = 879`. Level wins by 2. Hybrid correctly picked Level.
- Under different skew where SP would win (e.g. `A=100, B=1, C=1, D=1, E=1`): SP makespan `max(100, 2) + 1 = 101`; Level `max(100, 1) + max(2, 1) = 102`. SP wins by 1.
- The fact that *each* tree wins on some duration distribution proves they are genuinely incomparable. The conservative tie-break avoids both the round-9 and round-10 regressions while accepting that we may leave skew wins on the table for some duration distributions. We log the runtime makespan gap on plans where this happens so we can observe the impact in production telemetry.

This scenario is the regression fixture for the symbolic dominance fallback rule. It also exposes the genuine residual gap: the SP-tree representation cannot achieve `min(SP_makespan, Level_makespan)` under arbitrary durations on incomparable DAGs without runtime statistics.

### Scenario 22 — independent leaf alongside chain (codex round-9 counterexample)

`A → B, A → C, A → D, B → C`. Root `A` has three children; one of them (`B`) chains into another sibling (`C`) which becomes a multi-parent join; the third (`D`) is an independent leaf.

- `scheduleSP`: `Sequence(A, Parallel(B, D), C)`. Uniform makespan 3. Paths: `{A, B, C}`, `{A, D, C}`. Under skew `A=1, B=1, C=100, D=50`: `1 + max(1, 50) + 100 = 151`.

- `scheduleLevel`: roots `{A}`; rest `{B, C, D}` → WCC: `{B, C}, {D}` → `Parallel(Sequence(B, C), D)`. Result: `Sequence(A, Parallel(Sequence(B, C), D))`. Uniform makespan 3. Paths: `{A, B, C}`, `{A, D}`. Skew: `1 + max(101, 50) = 102`. Matches DAG critical path.

- Hybrid: symbolic dominance check:
  - SP path `{A, D, C}` not contained in any Level path → SP does NOT path-set-dominate Level.
  - Driver falls back to Level (the dominance-only driver does not check `dominates(Level, SP)`; once SP fails to dominate, the safe-default Level is returned).
  - Pick Level. ✓

- Result: `Sequence(A, Parallel(Sequence(B, C), D))`. Critical-path-optimal under any duration distribution.

This scenario is the canonical regression-vs-scheduleLevel test for the tie-break logic. The plan's earlier "prefer SP on tie" rule (rev 5.2) and "sampled-skew dominance" rule (rev 5.3) would both have made the wrong call. The symbolic path-set dominance check (rev 5.4) is provably correct here.

### Scenario 21 — independent root with shared join (component-awareness regression test)

`A → C, B → C, D` (D independent; A and B both feed C).

This scenario pins the component-awareness behaviour of `scheduleSP`. Without WCC partitioning inside `scheduleSP`, the algorithm batches `D` with `A` and `B` in the outer parallel block (an unrelated leaf forced into a sibling batch). Component-awareness fixes that.

- `scheduleSP`-with-components: 2 WCCs `{A, B, C}` and `{D}` → `Parallel(scheduleSP({A, B, C}), scheduleSP({D}))` = `Parallel(Sequence(Parallel(A, B), C), D)`. Paths: `{A, C}, {B, C}, {D}`.
- `scheduleLevel`: 2 components `{A, B, C}`, `{D}` → `Parallel(Sequence(Parallel(A, B), C), D)`. Paths: `{A, C}, {B, C}, {D}`.
- Identical trees (same paths, same shape) → `dominates(SP, Level)` trivially true → hybrid picks SP. Output is `Parallel(Sequence(Parallel(A, B), C), D)`, critical-path-optimal under any duration distribution.

Note: even *without* component-awareness, the rev 5.9 dominance-only driver would still catch the bug. `scheduleSP`-without-components produces `Sequence(Parallel(A, B, D), C)` whose paths include `{D, C}`, which is not contained in any Level path → `dominates(SP, Level)` is false → driver falls back to Level. So the dominance check is a safety net even when the SP scheduler emits a suboptimal tree. We keep component-awareness anyway for two reasons: (a) it produces a structurally cleaner SP tree that matches Level on these inputs, simplifying the property test; (b) it removes a class of cases where the driver would always pick Level — better to have both schedulers agree than to rely on the dominance check to silently override.

Scenario 21 pins both behaviours: assert that `scheduleSP`-with-components produces the same tree as `scheduleLevel`, and assert that the dominance check would catch the regression even without component-awareness.

### Scenario 20 — non-SP "N-shape" DAG (representation limit)

`A → C, A → D, B → D, B → E`. (D has two parents; C and E are independent leaves under different roots.)

This DAG is **not** series-parallel reducible: `D` is a multi-parent join while `C` and `E` are leaf side-branches under each parent. There is no SP tree that achieves the DAG's critical path under arbitrary durations.

- `scheduleSP` (component-aware top-level → single component → inline recurse): `Sequence(Parallel(Sequence(A, C), Sequence(B, E)), D)`. Uniform makespan `1 + max(2, 2) + 1 = 4`. Wait, that flattens through `processNode(A)` returning `Sequence(A, C)` so let me retrace: `processNode(A)`: child `C` ready (uniquely dependent), `D` unhandled `{B}`. Recurse: `Single(C)`. Returns `Sequence(A, C)` with `unhandled={D:{B}}`. Similarly `Sequence(B, E)` with `unhandled={D:{A}}`. Merge: `D` ready. Outer second iter: `Single(D)`. Result: `Sequence(Parallel(Sequence(A, C), Sequence(B, E)), D)`. Uniform makespan: `max(2, 2) + 1 = 3`.
- `scheduleLevel`: roots `{A, B}`; rest `{C, D, E}` — components: `{C}`, `{D}`, `{E}` (all three are isolated after edges-from-roots are stripped) → `Parallel(C, D, E)`. Result: `Sequence(Parallel(A, B), Parallel(C, D, E))`. Uniform makespan: `1 + 1 = 2`.
- Hybrid picks **Level**. No tie. Critical path under uniform durations is 2 (longest path from any source to any sink); Level matches it.
- Under skew `A=B=C=E=10, D=1000`: SP `max(20, 20) + 1000 = 1020`. Level `10 + max(10, 1000, 10) = 1010`. Critical path `max(A+D, B+D) = 1010`. Level matches; SP off by 10. Hybrid picks Level. ✓

This scenario asserts the hybrid's chosen output, asserts both individual schedulers' outputs are valid (respect dependencies), and includes a comment naming the representation limit.

Path enumeration:
- `scheduleSP` for scenario 20 produces `Sequence(Parallel(Sequence(A, C), Sequence(B, E)), D)`. Paths: `{A, C, D}, {B, E, D}`.
- `scheduleLevel` produces `Sequence(Parallel(A, B), Parallel(C, D, E))`. Paths: `{A, C}, {A, D}, {A, E}, {B, C}, {B, D}, {B, E}`.
- SP path `{A, C, D}` is not contained in any Level path (no Level path has all three).
- Level path `{A, D}` ⊆ SP path `{A, C, D}` ✓ but Level path `{A, C}` is not contained in any SP path (SP has `{A, C, D}` which contains `{A, C}` ✓ — let me recheck — yes `{A, C}` ⊆ `{A, C, D}`). Actually checking each Level path: `{A, C}` ⊆ `{A, C, D}` ✓; `{A, D}` ⊆ `{A, C, D}` ✓; `{A, E}` — neither SP path contains both A and E ✗; `{B, C}` — neither SP path contains both B and C ✗; `{B, D}` ⊆ `{B, E, D}` ✓; `{B, E}` ⊆ `{B, E, D}` ✓.

Two Level paths (`{A, E}` and `{B, C}`) are not contained in any SP path → Level does NOT dominate SP. And SP path `{A, C, D}` is not contained in any Level path → SP does NOT dominate Level either. **Incomparable.** Hybrid falls back to Level conservatively.

Under heavy-`D` skew Level matches CP; under heavy-leaf skew SP would win — and the hybrid leaves that on the table. This is the bounded residual gap; closing it requires runtime statistics or a richer runtime model.

### Implementation order for the scenarios

1. Build scenarios 1, 2, 3, 14, 15 first — they are the smallest and cover the core algorithm correctness.
2. Then 4, 18, 19, 21, 22, 23 — these exercise the multi-parent-merge intersection, leaf-side-branch regression, component-awareness regression, the dominance-check tie-break (scenario 22 must pick Level), and the incomparable-fallback (scenario 23 must pick Level conservatively).
3. Then 7, 13 — these surface different shapes than today and the mutation chain.
4. Then 8, 9, 10, 11, 12, 16 — federation-specific cases that exercise the validator's leaf-flattening, fetch-kind helper, and provided-path checks.
5. Then 17 — validator stress test for the conservative skip in `addMissingNestedDependencies`.
6. Then 20 — the non-SP representation-limit case.
7. Finally 5, 6 — sanity that simple/common cases don't regress.

Each scenario file follows a small template (input flat-`Sequence`, expected output tree, expected validator outcome). Scenarios 8–12 will require constructing fetch nodes that look like what the planner produces for those federation patterns;
the existing `graphql_datasource_federation_test.go` cases are good references for the exact shapes (see lines 981, 3073, 3728, 4041 for `@requires` variants;
lines 6701+ for `@provides`).

## Out of scope for this PR

- Changes to how the planner emits `DependsOnFetchIDs`.
- Changes to `addMissingNestedDependencies` (its conservative skip).
- New fetch-tree node kinds (specifically: per-parent dependencies inside a `Parallel` block, which would be required to close the non-SP slack).
- Cost-based / duration-aware scheduling (e.g., putting expensive fetches first within a `Parallel` based on observed latencies). The current algorithm is critical-path-optimal *under uniform durations* on SP DAGs;
  a duration-aware refinement could matter for non-SP DAGs but requires runtime statistics we do not currently collect.
- Flipping the default and updating fixtures (separate PR).

## Validation

- Executor refactor PR: existing `pkg/engine/resolve/...` tests pass under `-race`. New hand-rolled `Parallel(Sequence(...), Single(...))` test added.
- Postprocessor PR: new unit tests + property test pass. Validator catches synthetic violations. Old fixtures unchanged because new path is opt-in.
- Manual verification: run `query_order_baseline_test.go` under both wirings.
- `execution/` smoke test under the new wiring (locally, before any default flip).
