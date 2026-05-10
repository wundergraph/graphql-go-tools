# ADR 0001: Query Plan Scheduler

Status: Accepted.
Date: 2026-05-09.

## Context

The query planner emits fetches as a flat sequence and then runs postprocessors that sort by dependencies and group eligible siblings into parallel nodes.
That design is simple, but it leaves latency on the table when independent dependency chains coexist in the same operation.
The motivating shape is a query that fetches `me` from one subgraph, fetches `organisations` from another subgraph, and then fetches `currentPractice` through an entity hop that depends only on `me`.
The old postprocessor emits `Sequence(Parallel(user, organisation), practice)`.
That forces `practice` to wait for `organisation`, even though `organisation` does not provide any dependency required by `practice`.
The better tree is `Parallel(Sequence(user, practice), organisation)`.
That tree has the critical path `max(user + practice, organisation)` instead of `max(user, organisation) + practice`.

The old executor could not safely run that better tree.
Its parallel implementation assumed every child of a `Parallel` node was a leaf fetch.
It preselected merge targets, ran network loads concurrently, and then merged results serially after all loads completed.
If a `Parallel` child became a nested `Sequence`, the loader would have to recurse inside a goroutine.
That would make selection, input rendering, merge, JSON arena writes, and tainted-object map writes happen concurrently.
Those operations are not thread safe.

The scheduler therefore had two coupled requirements.
First, the executor needed to support nested sequence and parallel fetch trees without racing shared response state.
Second, the postprocessor needed a dependency-aware tree builder that could emit nested plans while preserving existing default behavior until the new path is explicitly enabled.

## Decision

We keep the legacy postprocessors wired by default.
The new scheduler is enabled only with `WithBuildScheduleTree()`.
When enabled, it replaces the `orderSequenceByDependencies` and `createParallelNodes` pair after missing nested dependencies are added and after concrete fetch types are created.
The old processors remain in the codebase and continue to support the existing options.

The new scheduler builds two candidate trees from the same fetch DAG.
It builds a component-aware level scheduler and a component-aware eager-inline scheduler.
It then picks the eager-inline tree only when a symbolic dominance check proves that it can never be slower than the level tree for any non-negative duration vector.
If the dominance check fails, the scheduler returns the level tree.
This is deliberately conservative.
It guarantees no regression against the level scheduler while still taking the stronger nested-chain shape when the proof applies.

The executor now supports nested parallel plans with a three-phase leaf protocol.
Prepare selects merge targets and renders input while holding the merge mutex when nested concurrency is possible.
Load performs the network round trip without the merge mutex.
Merge writes the result into the response while holding the merge mutex when nested concurrency is possible.
For flat plans, the existing fast path remains in use and does not acquire the mutex.

## Algorithms

### scheduleLevel

The level scheduler receives a set of fetch nodes and the dependency DAG.
It first partitions the set into weakly connected components.
Independent components are scheduled recursively and wrapped in a `Parallel` node sorted by minimum reachable fetch ID.
Inside a single component, it finds all roots whose parents are not present in the current node set.
Those roots form the current level.
The remaining nodes are scheduled recursively after stripping the root level from the component.
A single root is emitted as a leaf.
Multiple roots are emitted as `Parallel(root_1, root_2, ...)`.
When there is a recursive remainder, the result is `Sequence(current_level, remainder)`.
If a non-empty component has no root, the DAG is cyclic and scheduling fails with a clear error.

This algorithm matches the old level-barrier intuition but adds component awareness.
It is optimal under uniform fetch durations because every node is scheduled at its earliest possible topological level.
It can be suboptimal under skewed durations because it synchronizes all chains at every level boundary.

### scheduleSP

The eager-inline scheduler also begins with weakly connected component partitioning.
That step prevents unrelated components from being serialized together by an outer batch.
For a single component, the scheduler processes ready roots in batches.
Each processed node emits itself, then recursively consumes descendants that are uniquely ready within that branch.
Children with multiple pending parents are recorded in an unhandled map rather than emitted immediately.
When sibling branches are merged, the remaining-parent sets are intersected.
If the intersection becomes empty, the child becomes ready at the outer level.

This intersection rule is the critical recurrence.
If `A` and `B` both feed `X`, processing `A` reports `X` pending on `B`, and processing `B` reports `X` pending on `A`.
The intersection is empty, so `X` is ready after the parallel batch containing `A` and `B`.
This preserves joins while allowing independent chains such as `A -> C` and `B -> D` to run as `Parallel(Sequence(A, C), Sequence(B, D))`.

The eager-inline tree is often better for skewed chains that join later.
It is not universally better.
On non-series-parallel shapes with a leaf side branch and a shared join, eager inlining can place the shared join behind unrelated work.
That is why the driver does not pick it based on uniform makespan alone.

### Dominance Theorem

For a fetch tree `T`, define `paths(T)` recursively.
For a single fetch, `paths(Single(n)) = {{n}}`.
For a sequence, `paths(Sequence(c1, ..., ck))` is the cross product of child paths with set union.
For a parallel node, `paths(Parallel(c1, ..., ck))` is the union of the child path sets.
For any non-negative duration vector `d`, `makespan(T, d)` is the maximum sum of durations over any path in `paths(T)`.

Tree `A` dominates tree `B` if and only if every path in `A` is set-contained in some path in `B`.
If that condition holds, choose the heaviest path in `A` under any duration vector.
By containment, there is a path in `B` containing every node from that path.
Because durations are non-negative, the `B` path has weight at least the `A` path.
Therefore `makespan(A, d) <= makespan(B, d)` for all `d`.
For the converse, suppose some path in `A` is not contained in any path in `B`.
Assign duration one to nodes on that `A` path and zero to all other nodes.
Every `B` path misses at least one node from the `A` path, so every `B` path is strictly lighter.
That contradicts dominance for all duration vectors.

The driver uses this theorem directly.
It computes `scheduleSP` and `scheduleLevel`.
It validates both.
It returns `scheduleSP` only when `dominates(scheduleSP, scheduleLevel)` is true.
Otherwise it returns `scheduleLevel`.

### Validation

The validator walks the produced tree and enforces dependency order.
For a sequence, each child is validated with the fetch IDs provided by all previous children.
For a parallel node, every branch is validated with the same incoming dependency set.
The validator flattens every leaf under each parallel branch and checks all cross-branch leaf pairs.
It rejects fetch-ID dependencies across branches.
It also rejects response-path containment across branches because that indicates one branch may need data produced by the other.
Provided paths are computed by fetch kind using `SingleFetch`, `EntityFetch`, or `BatchEntityFetch` postprocessing merge paths.
The scheduler and validator use the `Fetch.Dependencies()` interface rather than assuming a concrete single fetch type.

### Executor Split

The loader now computes `useMergeMu` once at `LoadGraphQLResponseData` entry.
The predicate is true when any `Parallel` node has at least one non-`Single` child.
Flat plans keep the old path.
The old path preselects items, runs leaf loads concurrently, and merges serially after the errgroup completes.
Nested plans recurse through the fetch tree inside errgroup branches.
For those plans, each leaf runs prepare, load, and merge.
Prepare and merge call `maybeLock`, which acquires `mergeMu` only when `useMergeMu` is true.
Load does not hold the mutex.

This keeps network I/O concurrent while serializing access to `resolvable.data`, the JSON arena, and tainted-object tracking.
It also avoids adding mutex overhead to the only tree shape produced by the default legacy postprocessors.

## Consequences

The new option can produce nested fetch trees that reduce latency for independent chains.
Default behavior is unchanged, so existing golden query-plan fixtures remain stable.
The executor is more capable, but flat plans still use the existing fast path.
The validator may surface previously hidden dependency omissions.
That is intentional.
If a fetch needs data from another branch, the planner should declare that dependency rather than relying on a level barrier to hide the missing edge.

The scheduler performs more work than the legacy pair because it builds two trees and enumerates path sets.
The expected federation DAG sizes are small enough for this to be negligible.
The implementation keeps deterministic ordering by sorting branches by minimum reachable fetch ID.

## Alternatives Considered

We considered replacing the legacy scheduler outright.
That would force fixture churn and make executor changes harder to review.
We rejected that in favor of opt-in migration.

We considered choosing the smaller uniform-duration makespan.
That is unsound under skewed durations.
A tree can be better under uniform weights while worse under realistic latency distributions.
The path-set dominance theorem gives a stronger and exact condition.

We considered a cost-based scheduler using observed subgraph durations.
That could close more gaps, but the planner does not currently have reliable runtime duration inputs.
Adding that feedback loop is a separate product and operational decision.

We considered changing the runtime model to support per-parent dependencies inside a parallel block.
That would address non-series-parallel representation limits.
It would also be a larger executor contract change, so it is outside this decision.

## Acknowledged Residuals

Gap A is the non-series-parallel representation limit.
Some DAGs cannot be represented as a `Sequence` and `Parallel` tree that matches the critical path for every duration vector.
The scheduler falls back to the level tree when SP does not dominate, but that is still a representation-level compromise.

Gap B is incomparable trees under skew.
When SP and Level each win under different duration assignments, the hybrid picks Level.
That preserves the no-regression guarantee but can leave a runtime win on the table.
Closing this gap requires runtime statistics or a richer execution model.

## Worked Examples

Each example below shows the same five pieces:
the dependency graph drawn in ASCII,
the two candidate trees produced by `scheduleSP` and `scheduleLevel`,
the root-to-leaf path sets used by the dominance check,
the dominance verdict and which tree the hybrid returns,
and a numerical makespan comparison under at least one skewed duration vector that explains why the choice matters at runtime.

### Example 1 — the baseline (independent components)

Federation pattern: a query selects `me { firstName lastName currentPractice { id } }` plus `organisations(...)`.
`me` lives in the user subgraph.
`currentPractice` is an entity hop into a third subgraph that depends on `me.id`.
`organisations` is in a fourth subgraph that has nothing to do with the user.

DAG:

```
user ───▶ practice
organisation         (no edges)
```

Candidate trees:

```
scheduleSP    : Parallel(Sequence(user, practice), organisation)
scheduleLevel : Parallel(Sequence(user, practice), organisation)
```

Both schedulers see two weakly connected components — `{user, practice}` and `{organisation}` — and emit a `Parallel` over the components.
The trees are identical.

Path sets:

```
SP    paths : {user, practice}, {organisation}
Level paths : {user, practice}, {organisation}
```

Every SP path is contained in some Level path (each path is contained in itself).
`dominates(SP, Level)` returns `true`.
The hybrid returns `scheduleSP`, which is the same shape as `scheduleLevel` here.

Makespan with `user = 100ms`, `practice = 50ms`, `organisation = 200ms`:

```
runtime makespan = max(100 + 50, 200) = 200ms
old plan makespan = max(100, 200) + 50 = 250ms
```

Why this matters: the legacy postprocessor would have emitted `Sequence(Parallel(user, organisation), practice)` and forced `practice` to wait for `organisation` even though `practice` does not need `organisation`'s data.
The new scheduler reads the dependency graph instead of the topological levels and lets the unrelated branches run end-to-end in parallel.

### Example 2 — two chains joining at a third subgraph (SP dominates)

Federation pattern: two independent entity chains feed a third subgraph.
For instance, `User → Address` and `Order → Discount` both contribute fields that a downstream `Recommendation` resolver needs.

DAG:

```
A ────▶ C ─┐
           ├─▶ E
B ────▶ D ─┘
```

Edges: `A→C`, `B→D`, `C→E`, `D→E`.

Candidate trees:

```
scheduleSP    : Sequence(
                  Parallel(
                    Sequence(A, C),
                    Sequence(B, D)
                  ),
                  E
                )

scheduleLevel : Sequence(
                  Parallel(A, B),
                  Parallel(C, D),
                  E
                )
```

`scheduleSP` recognises that `A → C` and `B → D` are independent chains that meet at `E`.
It runs each chain end-to-end in its own parallel branch.
`scheduleLevel` strips the topological layers — first `{A, B}`, then `{C, D}`, then `E` — and synchronises every layer.

Path sets:

```
SP    paths : {A, C, E}, {B, D, E}
Level paths : {A, C, E}, {A, D, E}, {B, C, E}, {B, D, E}
```

Every SP path appears in the Level set as well.
`dominates(SP, Level)` returns `true`.
The hybrid returns the SP tree.

Why dominance is the right rule here: under uniform durations both trees finish in the same time (three "ticks"), but their behaviour diverges as soon as the chain durations become skewed.
With `A = 1ms`, `B = 100ms`, `C = 100ms`, `D = 1ms`, and `E = 10ms`:

```
SP makespan    : max(A+C, B+D) + E = max(101, 101) + 10 = 111ms
Level makespan : max(A, B) + max(C, D) + E = 100 + 100 + 10 = 210ms
```

The SP tree carries the long `A → C` arm and the long `B → D` arm in parallel.
The Level tree synchronises at every layer and ends up paying the maximum of each layer twice — once for the slow root and once for the slow middle node.
The dominance check guaranteed at plan time that this was safe, so the hybrid picked the win.

### Example 3 — asymmetric chain merge with a leaf side-branch (Level wins, SP regresses)

Federation pattern: a root entity `A` has a chain branch `A → B → D` and a sibling branch `A → C` where `C` both feeds the join `D` and produces an extra leaf `E`.

DAG:

```
        ┌───▶ B ───┐
        │          ├─▶ D
A ──────┼───▶ C ───┘
        │
        └───▶ C ─▶ E      (same C, drawn twice for clarity)
```

Edges: `A→B`, `A→C`, `B→D`, `C→D`, `C→E`.

Candidate trees:

```
scheduleSP    : Sequence(
                  A,
                  Parallel(
                    B,
                    Sequence(C, E)
                  ),
                  D
                )

scheduleLevel : Sequence(
                  A,
                  Parallel(B, C),
                  Parallel(D, E)
                )
```

`scheduleSP` eagerly inlines `C → E` into `C`'s branch.
That binds `E` to the parallel block, which means `D` cannot start until both `B` and `C → E` have finished.
`scheduleLevel` keeps `D` and `E` at the same topological level after `{B, C}` and lets them run in parallel.

Path sets:

```
SP    paths : {A, B, D}, {A, C, E, D}
Level paths : {A, B, D}, {A, B, E}, {A, C, D}, {A, C, E}
```

The SP path `{A, C, E, D}` has four nodes.
Every Level path has exactly three.
There is no Level path that contains all four SP nodes.
`dominates(SP, Level)` returns `false`.

The dominance-only driver therefore returns `scheduleLevel`.

Makespan under uniform durations, weight `1` for every node:

```
SP makespan    : 1 + max(1, 2) + 1 = 4 ticks
Level makespan : 1 + max(1, 1) + max(1, 1) = 3 ticks
```

The hybrid avoided a one-tick regression on uniform durations.
Under skewed durations the SP tree can lose by even more, because the chain `C → E` keeps growing while `D` sits idle behind it.

This is the canonical motivation for the dominance check: `scheduleSP` is not always better.
Picking it on a "uniform-tie" or "smaller uniform makespan" rule would silently regress this class of plans.
The path-set test catches the regression at plan time.

### Example 4 — independent leaf alongside a chain (component-awareness)

Federation pattern: a root `A` produces three children — two of them (`B` and `C`) form a chain that joins inside `C`, the third (`D`) is an unrelated leaf hanging off `A`.

DAG:

```
        ┌──▶ B ──▶ C
A ──────┼──▶ C   (multi-parent join via B)
        │
        └──▶ D
```

Edges: `A→B`, `A→C`, `A→D`, `B→C`.

Candidate trees:

```
scheduleSP    : Sequence(
                  A,
                  Parallel(B, D),
                  C
                )

scheduleLevel : Sequence(
                  A,
                  Parallel(
                    Sequence(B, C),
                    D
                  )
                )
```

`scheduleSP` batches everything ready after `A` into a single parallel block (`B` and `D`), then runs `C` once `B` is done.
`scheduleLevel` notices that after stripping `A`, the rest of the DAG splits into two components — `{B, C}` and `{D}` — and runs each as its own parallel arm.

Path sets:

```
SP    paths : {A, B, C}, {A, D, C}
Level paths : {A, B, C}, {A, D}
```

The SP path `{A, D, C}` is not contained in any Level path.
Level has `{A, D}` (no `C`) and `{A, B, C}` (no `D`).
`dominates(SP, Level)` returns `false`.

The hybrid returns `scheduleLevel`.

Why this matters under skew: with `A = 1ms`, `B = 1ms`, `C = 100ms`, `D = 50ms`,

```
SP makespan    : 1 + max(1, 50) + 100 = 151ms
Level makespan : 1 + max(1 + 100, 50) = 102ms
```

The SP tree drags `C` behind whichever parallel sibling is slowest, so a slow but unrelated `D` blocks the join.
The Level tree sees that `D` does not feed `C` and lets the `B → C` chain run end-to-end alongside `D`.

This case was discovered during plan review.
It is the reason `scheduleSP` does its own weakly-connected-component pass at every recursion level — without that step, `scheduleSP` would have produced `Sequence(Parallel(A, B, D), C)` and lost even more time.
With the component pass, `scheduleSP` produces the shape above, which still loses to `scheduleLevel`, and the dominance check correctly falls back.

### Example 5 — incomparable trees (residual gap)

Federation pattern: a non-series-parallel DAG.
There is no `Sequence`/`Parallel` tree that is universally optimal; the best choice depends on the actual durations.

DAG:

```
        ┌──▶ D
A ──────┤
        └──▶ E ◀──┐
                  │
B ─▶ C ─▶ D       │  (E has parents A and B; D has parents A and C)
        └────▶ E ─┘
```

Edges: `A→D`, `A→E`, `B→C`, `B→E`, `C→D`.

Candidate trees:

```
scheduleSP    : Sequence(
                  Parallel(A, Sequence(B, C)),
                  Parallel(D, E)
                )

scheduleLevel : Sequence(
                  Parallel(A, B),
                  Parallel(
                    Sequence(C, D),
                    E
                  )
                )
```

Path sets:

```
SP    paths : {A, D}, {A, E}, {B, C, D}, {B, C, E}
Level paths : {A, C, D}, {A, E}, {B, C, D}, {B, E}
```

SP dominance check:
the SP path `{B, C, E}` is not contained in any Level path (Level has `{B, C, D}` without `E` and `{B, E}` without `C`).
`dominates(SP, Level)` returns `false`.

Level dominance is also false (the dominance-only driver does not check this, but for the analysis the symmetric case fails too:
Level path `{A, C, D}` is not contained in any SP path).

Neither tree dominates.
The hybrid returns `scheduleLevel`.

Why "incomparable" is real and not an artefact of the algorithm:
under one duration vector SP is faster, under another Level is faster.
Concrete example with `A=25, B=227, C=647, D=3, E=5`:

```
SP makespan    : max(A, B+C) + max(D, E) = max(25, 874) + max(3, 5) = 874 + 5 = 879ms
Level makespan : max(A, B) + max(C+D, E) = max(25, 227) + max(650, 5) = 227 + 650 = 877ms
```

Level is slightly better.

But under `A=100, B=1, C=1, D=1, E=1`:

```
SP makespan    : max(100, 2) + max(1, 1) = 100 + 1 = 101ms
Level makespan : max(100, 1) + max(1+1, 1) = 100 + 2 = 102ms
```

SP is slightly better.

The dominance theorem proves that no single SP tree on this DAG is universally optimal, so at plan time we have no way to choose correctly without runtime statistics.
The hybrid takes the conservative option — `scheduleLevel`, the same shape today's algorithm would produce — and accepts that some duration distributions leave a small win on the table.
This is the "incomparable trees" residual documented in Gap B above.

### Reading the decision table

The four examples cover the four interesting outcomes:

```
Example 1 : trees identical                                  → hybrid returns SP (same shape)
Example 2 : SP path-set-dominates Level                      → hybrid returns SP (skew win)
Example 3 : SP does not dominate, Level wins on uniform too  → hybrid returns Level (regression caught)
Example 4 : SP does not dominate, regression only under skew → hybrid returns Level (regression caught)
Example 5 : incomparable, neither tree universally better    → hybrid returns Level (conservative)
```

Production federation queries are dominated by Example 1 and Example 2 patterns — a single chain plus an unrelated subgraph, or two chains joining at a downstream entity.
Examples 3, 4, and 5 are uncommon but real, and the dominance check guarantees the hybrid never regresses against the legacy level-based shape on any of them.

## References

Valdes, Tarjan, and Lawler, 1982, The Recognition of Series Parallel Digraphs.
GraphQL federation query planning shapes: Sequence, Parallel, Fetch, Flatten, and entity fetch dependencies.
Local implementation plan: `docs/superpowers/specs/2026-05-08-improve-query-order-plan.md`.
