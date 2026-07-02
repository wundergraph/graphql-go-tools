# Reviewer notes — task 18: defer + concurrency scenario coverage

Commit: (hash recorded in PROGRESS.md).
Task file: [tasks/18-defer-concurrency-coverage.md](../tasks/18-defer-concurrency-coverage.md).
Spec background: appendix §6–§7 (M3–M5, N1–N5); CODING_GUIDELINES §4.5.

## What this commit adds

The defer × caching proofs the first pass could not complete: cross-defer-group L1 SERVING is now proven end to end through `ResolveGraphQLDeferResponse`, with complete-frame assertions, gated datasources, and `-race`.
One small runtime fix flushed out (as the task anticipated): the narrowing pass learned the DEFER-GROUP ANCESTRY as an ordering source.

## Decisions made

- THE FLUSHED-OUT FIX: `executesBefore`'s "tree 0 before all" rule could not order a deferred provider before a NESTED (child-group) consumer — N2's shape has NO dependency edge between the two inventory fetches, only group ancestry.
  `ConfigureCaching` now takes `treeParents` (derived in postprocess from `DeferDescriptors.ParentID`, mirroring `deferTrees`), and `treeEncloses` generalizes the rule: an ANCESTOR tree's fetches execute before every DESCENDANT tree's (the resolver resolves a parent group fully before its children — `resolveDeferTree`'s Sequence arm); SIBLING groups stay unordered (they run in parallel).
  Pinned by a new narrowing unit row plus N2's plan inspection.
- N2's fixture shape: a nested `@defer` whose selection is a SUBSET of the enclosing fragment gets normalized away (discovered by probing), so the nested group routes through the reviews hop back to the same Product (`... @defer { stock warehouse {...} ... @defer { reviews { product { stock } } } }`) — distinct path, same entity, ancestry-only ordering.
- N4 uses PURE channel synchronization, not synctest: `Plan` and `resolve.New` spawn request-lifetime goroutines (WS ping loops, the resolver heartbeat) that deadlock a synctest bubble at exit.
  The harness `deferFrameWriter` gained an optional `Flushed` signal channel — the test consumes exactly two flush signals while the gated sibling is provably blocked (its `Arrived` signal received, its `Release` still closed-off), so ordering is gate-based with zero latency dependence.
- M5's empirical behavior, now pinned: a cache-hook error surfaces in THAT group's `completed` entry as a frame error (`"errors":[{"message":"cache hook failed"}]`) while the sibling group's frame carries its complete data — sibling isolation holds.
- N5/M4 (per-group transactions, single lock per hook, no cross-group arena race) are covered by `-race` across N1–N4 plus the task-02 loader seam rows; no separate row can observe lock acquisition counts from outside without instrumenting production code.

## What was implemented

- `cache/configure_caching.go` + `optimize_l1_cache.go` — the `treeParents` ancestry ordering (+ unit row).
- `postprocess/postprocess.go` — `deferTreeParents` (ParentID chain → parent tree indices) threaded into the defer arm; other arms pass nil.
- Harness (`cachingtesting.go`) — `ResolveDeferResponse`/`ResolveDeferResponseWith` over a frame-capturing `DeferResponseWriter` (with the `Flushed` gate channel), and `PlanResult.Gate` (name-translated `SetGate` + re-swap).
- `defer_l1_e2e_test.go` — the rows:
  - N1 + M3: initial-fetch entity serves the deferred group — plan INSPECTED (the reviewer-guidance point: the defer group really carries a configured same-entity fetch), both frames pinned as complete strings, the deferred subgraph never hit (tampered canned response), zero store ops (L1-only), exactly ONE `BeginRequest` across all groups.
  - N2: a deferred fetch populates L1 served to the NESTED later group (ancestry-only ordering); all three frames pinned; tampered response; zero store ops.
  - N3: exactly one `EndRequest`; the single request-end flush carries the initial fetch's L2 write AND the deferred group's (all Gets before all Sets, both values asserted).
  - N4: a `SkipFullHit` sibling flushes its frame while the other sibling is still gated mid-`Load` — the skip neither waits nor reorders.
  - M5: the hook-error isolation row above.

## What to look into (review focus)

- The ancestry fix is the one production change — verify `deferTreeParents` against `buildDeferTree`'s actual execution semantics (Sequence = parent-then-children; Parallel = unordered), and that passing nil elsewhere degrades to the old root-before-defers rule.
- N4's synchronization: confirm the `Flushed` channel consumption can neither deadlock (buffered, exact counts) nor pass vacuously (the gated fetch's `Arrived` is received BEFORE the flush waits).
- Frame strings are pinned byte-exact except N4/M5's parallel-sibling frames (matched by content, order-independent) — the one place full-value ordering would be racy by construction.

## Verification evidence

- All defer rows pass; the FULL execution harness suite is `-race` clean; `v2` cache/postprocess suites `-race` clean.
- Full `v2` and `execution` suites pass, exit 0.
- `golangci-lint` (v2.5.0, repo config minus `modernize`): 0 issues in BOTH modules; `gci`/`gofmt` clean.
