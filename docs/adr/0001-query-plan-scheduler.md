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

For the baseline `user -> practice` plus independent `organisation`, both schedulers produce `Parallel(Sequence(user, practice), organisation)`.
SP trivially dominates Level because the trees are identical.

For two chains joining at a final fetch, `A -> C`, `B -> D`, `C -> E`, and `D -> E`, SP produces `Sequence(Parallel(Sequence(A, C), Sequence(B, D)), E)`.
Level produces `Sequence(Parallel(A, B), Parallel(C, D), E)`.
Every SP path is contained in a Level path, so SP dominates and the hybrid picks SP.

For the asymmetric leaf-side-branch shape `A -> B`, `A -> C`, `B -> D`, `C -> D`, and `C -> E`, SP does not dominate Level.
The SP tree has a path that includes `A`, `C`, `E`, and `D`.
No Level path contains that whole set.
The hybrid falls back to Level.

For the codex counterexample `A -> D`, `A -> E`, `B -> C`, `B -> E`, and `C -> D`, the two trees are incomparable.
SP wins under some duration vectors and Level wins under others.
The hybrid falls back to Level because SP does not dominate.

## References

Valdes, Tarjan, and Lawler, 1982, The Recognition of Series Parallel Digraphs.
GraphQL federation query planning shapes: Sequence, Parallel, Fetch, Flatten, and entity fetch dependencies.
Local implementation plan: `docs/superpowers/specs/2026-05-08-improve-query-order-plan.md`.
