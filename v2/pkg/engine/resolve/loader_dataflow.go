package resolve

import (
	"context"
	stderrors "errors"
	"maps"
	"slices"

	"github.com/pkg/errors"
	"github.com/wundergraph/astjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// resolveDataflow executes the fetch DAG WITHOUT per-wave barriers. Each fetch's
// network load starts as soon as its OWN dependencies (DependsOnFetchIDs) have
// merged, rather than waiting for the slowest fetch of the previous Parallel wave
// (which resolveSerial/resolveParallel do via g.Wait()). Under skewed subgraph
// latency this collapses the wall-clock toward the true critical path.
//
// INVARIANTS (each falsifiable; codex is explicitly tasked to attack them):
//
//  1. Single-coordinator arena ownership: EVERY arena read (selectItemsForPath,
//     itemsData, input-template rendering, batch dedup) and EVERY arena
//     write (response parse, merge) runs on this coordinator goroutine, via
//     preparePhase (prepare) and mergeResult (merge).
//     The spawned worker closure contains ONLY
//     executeSourceLoad, which is arena-free (no l.jsonArena / l.resolvable /
//     selectItemsForPath / itemsData / MustParse references — greppable).
//     No mutex is needed; there is nothing concurrent to lock against.
//
//  2. Deterministic error ordering via swap-capture staging: the three audited
//     order-bearing sinks — l.resolvable.errors (all appends inside mergeErrors,
//     addApolloRouterCompatibilityError, renderErrorsFailedDeps/FailedToFetch/
//     StatusFallback, renderAuthorizationRejectedErrors,
//     renderRateLimitRejectedErrors, all reachable only from mergeResult /
//     mergeErrors), l.ctx.subgraphErrors
//     (ctx.appendSubgraphErrors, same reachability), and
//     l.resolvable.subgraphExtensions (mergeResult) — are
//     nil-swapped around each merge and replayed in ascending LEAF order after
//     the drain. Both swap targets are nil-gated lazy-init
//     (Resolvable.ensureErrorsInitialized, Context.appendSubgraphErrors), so the
//     swap is transparent to the merge code: ZERO diff to the loader.go
//     error paths. If a fourth order-bearing sink exists, this
//     staging silently misses it — that is the falsification target.
//
//  3. Leaf order == wave merge order: leaves are indexed by their position in
//     collectDataflowLeaves' depth-first flatten, which equals the wave
//     executor's merge order (resolveSerial walks Sequence children in order;
//     resolveParallel merges by node index after g.Wait()). Flushing staged
//     sinks and OnFinished hooks in ascending leaf index therefore reproduces
//     the wave executor's error/extension array order byte-for-byte.
//     Flags written during merge (taintedObjs, skipValueCompletion) stay
//     UN-staged: they are order-free, and dependents' selectItemsForPath must
//     observe parent taint (DAG order guarantees parents merged first).
//
//  4. Tools lifetime: batch input bytes live on the pooled res.tools
//     arena, so tools are collected into toolsToPut AT PREPARE TIME (abort-safe)
//     and Put ONLY after the drain loop exits — on every path including
//     prepare-fatal and merge-fatal drains. Put(nil) is a no-op.
//
//  5. Exactly-once completion: ch is buffered to n and every spawned worker
//     sends exactly once, unconditionally; the coordinator never returns before
//     inflight == 0. No goroutine can leak or block, even after cancel().
//     cancel() propagates to in-flight HTTP via the derived ctx. A worker panic
//     crashes the process exactly like the wave executor's errgroup — no
//     recover(), by parity.
//
//  6. Fatal selection: prepare/merge errors record per-leaf fatals and cancel();
//     dispatching stops, in-flight loads drain, staged sinks still flush (hook/
//     telemetry consistency), and the LOWEST-leaf-index fatal is returned —
//     deterministic, though not guaranteed bitwise wave-identical when multiple
//     fetches fail fatally (the wave executor short-circuits dispatch instead;
//     its own errgroup error selection was already completion-order
//     nondeterministic). The flush may therefore fire OnFinished for staged
//     leaves the wave executor would never have merged after ITS first merge
//     error — a deterministic superset, telemetry-only (codex P2, accepted).
//     Acceptable: a fatal LoadGraphQLResponseData error discards the resolvable.
//
//  7. OnFinished hooks fire during the flush, in leaf order — each hook sees
//     ctx.subgraphErrors in exactly the post-leaf-i state the wave executor
//     would show it. Wall-clock timing is later than the wave executor's
//     per-merge hooks; cosmo's hooks consume the already-captured responseInfo,
//     so this is timing-only.
//
//  8. Complete-DependsOnFetchIDs: taint visibility for concurrently dispatched
//     fetches relies on every fetch listing ALL fetches whose data it reads —
//     the same invariant resolveParallel/createParallelNodes already require.
//
//  9. Pre-fetch hook call order (Authorizer/RateLimiter via validatePreFetch,
//     which runs at PREPARE time and whose implementations can render
//     accumulated state into response extensions): every dispatch pops the
//     GLOBAL leaf-index minimum among ready fetches (not FIFO — an inline
//     skipLoad completion may enqueue a lower-leaf dependent behind an
//     already-queued sibling), so hook call order equals the wave executor's
//     spawn order whenever the DAG permits it. Residual contract: when a
//     WORKER completion makes a lower-leaf fetch ready after a higher-leaf
//     fetch was already dispatched, hook calls interleave earlier than the
//     wave BARRIER would allow — the wave executor already calls these hooks
//     concurrently (unordered) WITHIN a wave, so implementations must already
//     be order-tolerant; dataflow extends that tolerance requirement across
//     waves. Order-SENSITIVE extension renderers are not byte-stable under
//     ENGINE_ENABLE_DATAFLOW.
//
// validatePreFetch (authorization + rate limiting) runs inside the prepare
// functions on the coordinator, exactly as in the nested 3-phase protocol
// (upstream holds it under mergeMu there). Rate-limit I/O therefore serializes
// on the coordinator; if that ever matters, splitting it back into the worker
// is the named lever — do not do it speculatively.
//
// Eligible only for queries whose fetch tree is the flat federation shape
// (see collectDataflowLeaves) with UNIQUE FetchIDs forming a DAG. Mutations,
// subscriptions, non-unique FetchIDs, cyclic deps, nested (schedule-tree)
// plans, or any unexpected node kind fall back to the wave executor.
//
// STATUS: experimental, default-off (ResolverOptions.EnableDataflowExecution /
// ENGINE_ENABLE_DATAFLOW). Recovers up to ~37% wall-clock under skewed
// subgraph latency.
func (l *Loader) resolveDataflow(root *FetchTreeNode) error {
	leaves, ok := collectDataflowLeaves(root)
	if !ok {
		return l.resolveFetchNode(root)
	}
	n := len(leaves)
	switch n {
	case 0:
		return nil
	case 1:
		return l.resolveSingle(leaves[0].Item)
	}

	byID := make(map[int]*FetchTreeNode, n)
	leafIndexByID := make(map[int]int, n)
	for i, lf := range leaves {
		id := lf.Item.Fetch.Dependencies().FetchID
		if _, dup := byID[id]; dup {
			// Non-unique FetchIDs mean ordering is expressed by the tree STRUCTURE,
			// not by the FetchID dependency edges (e.g. hand-built plans, or any plan
			// where the planner did not assign distinct FetchIDs). The dataflow
			// scheduler keys solely on FetchID deps, so it cannot honor structural
			// ordering — fall back to the wave executor. Real planner output always
			// has unique FetchIDs with complete deps (createParallelNodes groups BY
			// those deps), so production plans take the dataflow path.
			return l.resolveFetchNode(root)
		}
		byID[id] = lf
		leafIndexByID[id] = i
	}
	// Count only dependencies that actually exist in this leaf set, and record the
	// reverse edges so a completed fetch can unblock its dependents.
	remaining := make(map[int]int, n)
	dependents := make(map[int][]int, n)
	for _, lf := range leaves {
		id := lf.Item.Fetch.Dependencies().FetchID
		cnt := 0
		for _, dep := range lf.Item.Fetch.Dependencies().DependsOnFetchIDs {
			if _, exists := byID[dep]; exists {
				cnt++
				dependents[dep] = append(dependents[dep], id)
			}
		}
		remaining[id] = cnt
	}

	// Schedulability pre-check (Kahn's algorithm on a copy of the in-degrees). If the
	// in-set dependency graph is not a DAG, a cycle leaves some fetches permanently
	// unschedulable; without this guard the coordinator loop would exit with those
	// fetches never dispatched and return a silently-incomplete response (codex P1).
	// Real planner output is always a DAG, so this falls back rather than executes.
	{
		indeg := make(map[int]int, n)
		maps.Copy(indeg, remaining)
		queue := make([]int, 0, n)
		for id, d := range indeg {
			if d == 0 {
				queue = append(queue, id)
			}
		}
		scheduled := 0
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			scheduled++
			for _, dep := range dependents[id] {
				indeg[dep]--
				if indeg[dep] == 0 {
					queue = append(queue, dep)
				}
			}
		}
		if scheduled != n {
			// Not a DAG (cycle / unschedulable). Fall back before merging anything.
			return l.resolveFetchNode(root)
		}
	}

	// mergeStage holds one leaf's captured error sinks until the leaf-order flush
	// (invariants 2 and 3).
	type mergeStage struct {
		errors       *astjson.Value
		extensions   []*astjson.Object
		subgraphErrs map[string]error
		res          *result
		merged       bool
	}
	stages := make([]mergeStage, n)

	type completion struct {
		id  int
		idx int
	}

	ctx, cancel := context.WithCancel(l.ctx.ctx)
	defer cancel()
	ch := make(chan completion, n)
	inflight := 0
	toolsToPut := make([]*batchEntityTools, 0, n)
	preparedByIdx := make([]*preparedFetch, n)

	fatalByIdx := make([]error, n)
	hasFatal := false
	recordFatal := func(idx int, err error) {
		if fatalByIdx[idx] == nil {
			fatalByIdx[idx] = err
		}
		hasFatal = true
		cancel()
	}

	// stagedMerge runs the wave executor's merge dispatch for one leaf on the
	// coordinator, with the three audited sinks nil-swapped and captured
	// (invariant 2). callOnFinished is deliberately NOT called here — it moves to
	// the leaf-order flush (invariant 7).
	stagedMerge := func(idx int, p *preparedFetch) error {
		savedErrors := l.resolvable.errors
		savedExtensions := l.resolvable.subgraphExtensions
		savedSubgraphErrors := l.ctx.subgraphErrors
		l.resolvable.errors = nil
		l.resolvable.subgraphExtensions = nil
		l.ctx.subgraphErrors = nil

		var err error
		switch {
		case p.res.nestedMergeItems != nil:
			// Vestigial in this fork — nestedMergeItems is never assigned — but kept
			// for parity with resolveParallel's merge dispatch (surgical-changes rule).
			for j := range p.res.nestedMergeItems {
				if err = l.mergeResult(p.item, p.res.nestedMergeItems[j], p.items[j:j+1]); err != nil {
					break
				}
			}
		default:
			err = l.mergeResult(p.item, p.res, p.items)
		}

		stages[idx] = mergeStage{
			errors:       l.resolvable.errors,
			extensions:   l.resolvable.subgraphExtensions,
			subgraphErrs: l.ctx.subgraphErrors,
			res:          p.res,
			merged:       true,
		}
		l.resolvable.errors = savedErrors
		l.resolvable.subgraphExtensions = savedExtensions
		l.ctx.subgraphErrors = savedSubgraphErrors
		return err
	}

	// Dispatch ordering is LEAF order (= tree order = the wave executor's spawn
	// order), NOT FetchID order: validatePreFetch calls user-supplied
	// Authorizer/RateLimiter hooks at prepare time, and those hooks can render
	// order-sensitive accumulated state into response extensions (codex P1 on
	// this hardening). Leaf-ordered seeding and unblock batches reproduce the
	// wave executor's call order whenever the DAG permits.
	byLeafIndex := func(a, b int) int { return leafIndexByID[a] - leafIndexByID[b] }
	var ready []int
	unblock := func(id int) {
		for _, dep := range dependents[id] {
			remaining[dep]--
			if remaining[dep] == 0 {
				ready = append(ready, dep)
			}
		}
	}

	// dispatchOne prepares one fetch on the coordinator (all arena reads,
	// invariant 1) and either completes it inline (skip paths) or hands ONLY the
	// arena-free load to a worker goroutine.
	dispatchOne := func(id int) {
		node := byID[id]
		idx := leafIndexByID[id]
		p, err := l.preparePhase(node.Item)
		if p != nil && p.res.tools != nil {
			// Collect at prepare time so abort paths still Put (invariant 4).
			toolsToPut = append(toolsToPut, p.res.tools)
		}
		if err != nil {
			recordFatal(idx, err)
			return
		}
		if p == nil {
			// Unknown fetch kind: the wave executor's resolveSingle default case
			// performs no load and no merge. Just unblock dependents.
			unblock(id)
			return
		}
		if p.skipLoad {
			// Skip paths still merge (fetchSkipped / rendered-error / denial state
			// lives on res), inline on the coordinator — no goroutine, no recursion.
			if mErr := stagedMerge(idx, p); mErr != nil {
				recordFatal(idx, mErr)
				return
			}
			unblock(id)
			return
		}
		preparedByIdx[idx] = p
		inflight++
		go func() {
			// Worker: arena-free by invariant 1. executeSourceLoad stores failures
			// in res.err (never a Go return); merge renders them deterministically.
			l.executeSourceLoad(ctx, p.item, p.source, p.input, p.res, p.trace)
			ch <- completion{id: id, idx: idx}
		}()
	}

	// Seed every dependency-free fetch.
	for id, r := range remaining {
		if r == 0 {
			ready = append(ready, id)
		}
	}

	for {
		for len(ready) > 0 && !hasFatal {
			// Pop the GLOBAL leaf-index minimum, not FIFO: an inline skipLoad
			// completion can enqueue a lower-leaf dependent behind an already-queued
			// higher-leaf sibling, and FIFO would call its pre-fetch hooks out of
			// wave order even though the DAG permits wave order (codex P1 round 2).
			// n is small; a sort per pop is cheaper than a heap at this size.
			slices.SortFunc(ready, byLeafIndex)
			id := ready[0]
			ready = ready[1:]
			dispatchOne(id)
		}
		if inflight == 0 {
			break
		}
		c := <-ch
		inflight--
		if hasFatal {
			continue // draining after a fatal
		}
		if err := stagedMerge(c.idx, preparedByIdx[c.idx]); err != nil {
			recordFatal(c.idx, err)
			continue
		}
		unblock(c.id)
	}

	// Flush staged sinks + OnFinished hooks in ascending leaf index — the wave
	// executor's merge order (invariants 3 and 7). Runs even after a fatal, for
	// hook/telemetry consistency; the resolvable is discarded on fatal anyway.
	for i := range stages {
		st := &stages[i]
		if !st.merged {
			continue
		}
		if st.errors != nil && len(st.errors.GetArray()) > 0 {
			l.resolvable.ensureErrorsInitialized()
			l.resolvable.errors.AppendArrayItems(l.jsonArena, st.errors)
		}
		if len(st.extensions) > 0 {
			l.resolvable.subgraphExtensions = append(l.resolvable.subgraphExtensions, st.extensions...)
		}
		if len(st.subgraphErrs) > 0 {
			if l.ctx.subgraphErrors == nil {
				l.ctx.subgraphErrors = make(map[string]error, len(st.subgraphErrs))
			}
			// Sorted for determinism (one key per fetch in practice). When the key is
			// new, the captured value is assigned directly — exact wave parity, since
			// the capture started from nil exactly like a fresh map entry. When the
			// key repeats across leaves, errors.Join nests the captured chain one
			// level deeper than the wave executor's sequential appends; the flattened
			// Error() string and errors.Is/As behavior are identical.
			for _, k := range slices.Sorted(maps.Keys(st.subgraphErrs)) {
				if existing, exists := l.ctx.subgraphErrors[k]; exists {
					l.ctx.subgraphErrors[k] = stderrors.Join(existing, st.subgraphErrs[k])
				} else {
					l.ctx.subgraphErrors[k] = st.subgraphErrs[k]
				}
			}
		}
		// The hook sees ctx.subgraphErrors in exactly the post-leaf-i state the
		// wave executor would show it (invariant 7).
		if st.res.nestedMergeItems != nil {
			for j := range st.res.nestedMergeItems {
				l.callOnFinished(st.res.nestedMergeItems[j])
			}
		} else {
			l.callOnFinished(st.res)
		}
	}

	for _, t := range toolsToPut {
		batchEntityToolPool.Put(t)
	}
	if hasFatal {
		for i := range fatalByIdx {
			if fatalByIdx[i] != nil {
				// Lowest-leaf-index fatal: deterministic regardless of completion order
				// (invariant 6).
				return errors.WithStack(fatalByIdx[i])
			}
		}
	}
	return nil
}

// collectDataflowLeaves returns the Single leaves of a FLAT fetch tree: nil, a
// bare Single, or a Sequence whose children are Single or Parallel-of-Single —
// exactly the shape createParallelNodes emits. Anything else, in particular the
// NESTED Parallel(Sequence(...)) trees built by the schedule-tree processor
// (WithBuildScheduleTree), reports ok=false and resolveDataflow falls back to the
// wave executor, which handles nested trees under mergeMu. This structural guard
// is what makes ENGINE_ENABLE_DATAFLOW + ENGINE_ENABLE_SCHEDULE_TREE safe to
// combine: the dataflow scheduler keys solely on FetchID dependency edges and
// would otherwise ignore the schedule tree's structural ordering.
func collectDataflowLeaves(node *FetchTreeNode) ([]*FetchTreeNode, bool) {
	if node == nil {
		return nil, true
	}
	switch node.Kind {
	case FetchTreeNodeKindSingle:
		return singleDataflowLeaf(node)
	case FetchTreeNodeKindSequence:
		out := make([]*FetchTreeNode, 0, len(node.ChildNodes))
		for _, child := range node.ChildNodes {
			if child == nil {
				return nil, false
			}
			switch child.Kind {
			case FetchTreeNodeKindSingle:
				leaf, ok := singleDataflowLeaf(child)
				if !ok {
					return nil, false
				}
				out = append(out, leaf...)
			case FetchTreeNodeKindParallel:
				for _, pc := range child.ChildNodes {
					if pc == nil || pc.Kind != FetchTreeNodeKindSingle {
						return nil, false
					}
					leaf, ok := singleDataflowLeaf(pc)
					if !ok {
						return nil, false
					}
					out = append(out, leaf...)
				}
			default:
				return nil, false
			}
		}
		return out, true
	default:
		return nil, false
	}
}

func singleDataflowLeaf(node *FetchTreeNode) ([]*FetchTreeNode, bool) {
	if node.Item == nil || node.Item.Fetch == nil {
		return nil, false
	}
	return []*FetchTreeNode{node}, true
}

// dataflowEligibleOperation reports whether the current operation may use the
// dataflow executor. Mutations are serialized by side effect (ordering not
// captured in DependsOnFetchIDs) and subscriptions resolve differently, so only
// queries are eligible. response.Info is authoritative when present; the
// context value (which defaults to Query when unset) is the fallback.
func (l *Loader) dataflowEligibleOperation() bool {
	if l.info != nil {
		return l.info.OperationType == ast.OperationTypeQuery
	}
	return GetOperationTypeFromContext(l.ctx.ctx) == ast.OperationTypeQuery
}
