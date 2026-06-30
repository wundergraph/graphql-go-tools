# Commit A1b — port the P1 `cacheProvidesDataVisitor`

Plan item: `docs/caching/PLAN.md` §3, A1 (second of two parts; completes A1).
RFC sections: RFC-2 §9 (re-add the dedicated ProvidesData visitor), §5.2 (planner registration), §5.4 (node_object fields), §9.3 (the *FetchInfo side-table).
Phase: A (L2 entities).
OLD reference: `caching-base` worktree `plan/caching_planner_state.go` + `plan/visitor.go`.

## Problem

The entity stamper (A1a) attached `cfg.ProvidesData = pd[info]`, but the P1 side-table was the inert S3 skeleton, so `ProvidesData` was nil and the runtime coverage walk would be disabled.
A nil/too-small `ProvidesData` silently disables hits; an arg-blind one serves stale data (RFC-2 §9.1).
So P1 must build, per fetch, the alias-and-arg-aware field tree the fetch returns.

## Solution

Port the OLD per-planner `ProvidesData` builder into the standalone peer visitor:

- `trackField`/`popField`/`createFieldValue`/`captureFieldCacheArgs` ported faithfully: entity-boundary reset, `__typename` dedup, inline-fragment `OnTypeNames`, alias -> `Field.OriginalName`, and `CacheArgs` (variable arguments only, sorted by name, skipped on root operation fields).
- Per-planner state on the visitor (`objects map[int]*resolve.Object`, `currentFields map[int][]objectFields`), grown via the walker's `RunAfterEnterField` deferred push (the current-branch equivalent of the OLD deferred enter hook).
- `attachTo` resolves each `plannerID -> *FetchInfo` via `planners[plannerID].ObjectFetchConfiguration().fetchItem.Fetch.FetchInfo()` (iterated in sorted plannerID order for determinism) and stores the `map[*FetchInfo]*Object` side-table on the response. That `*FetchInfo` is the SAME pointer `createConcreteSingleFetchTypes` copies by reference onto the concrete entity fetch, so the stamper's `pd[fetch.FetchInfo()]` resolves the right tree.

### Architecture deviation (necessary, documented)

RFC-2 §5.2/§9.2 assumed P1 could register as a peer on the SAME planning walk, mirroring `CostVisitor`.
On the actual branch this does NOT hold: `Visitor.AllowVisitor` filters out non-planner peers (so P1 would not run on the main walk), and `fieldPlanners` is populated during `LeaveField` (so an `EnterField`-time peer would see it empty).
Editing `visitor.go` to change either is forbidden (PR1).
So P1 runs as a SECOND, filter-free walk on the same `planningWalker` AFTER the main walk: `ResetVisitors()` + `SetVisitorFilter(nil)` + register ONLY P1 + `Walk(...)` + `attachTo`.
This is gated entirely by `p.cacheProvidesData != nil` (constructed only when `Configuration.CacheConfigProviders` is non-empty), so:
- the NO-OP path and ALL existing planner tests never trigger the second walk (no caching configured) -> plans byte-identical;
- the second walk runs ONLY P1, which mutates only its own per-planner state, never `planningVisitor.plan`, so the built plan is not corrupted;
- the cost is one extra plan-time walk per unique cached operation (plans are cached/reused across requests).

## Key decisions

- Second-walk over editing forbidden visitor files: preserves the zero-forbidden-file-edit invariant at the cost of one extra plan-time walk; correctness is bounded because only P1 runs and the plan is already finalized.
- `resolve.CacheFieldArg` keeps the S3 shape `{Name, VariableName string}`; variable args sorted by `Name`.

## Tests

- `plan/cache_provides_data_visitor_test.go`: drives the REAL planner and asserts the FULL per-fetch `*resolve.Object` tree (the PR5 fidelity gate) for a single-`@key` entity, a nested object, an ALIASED field (`OriginalName` set), a field with a VARIABLE argument (`CacheArgs` captured + sorted; root-operation-field args NOT captured), and `__typename` dedup. Full-value `assert.Equal`.
- The execution `StageL2Entities` golden now shows the entity fetch with `providesData:true`.

Verification:

- `cd v2 && go build ./pkg/...` — clean.
- `cd v2 && go test ./pkg/engine/plan/... ./pkg/engine/postprocess/... ./pkg/engine/resolve/... -count=1` — PASS; the StageNoop planner no-op golden is byte-identical (P1 never constructed without providers).
- `cd execution && go test ./engine/ -run 'Caching' -count=1` — PASS (StageL2Entities golden updated to providesData:true; StageNoop unchanged).
- `cd v2 && go vet ./pkg/engine/plan/...` — clean.

## Reviewer guidance

- The no-op golden is byte-identical (no provider -> no P1 -> no second walk).
- The second walk runs ONLY P1 and never rebuilds `planningVisitor.plan` (verify the existing planner goldens are unaffected — they are, because they configure no providers).
- The `ProvidesData` trees match the OLD semantics (the unit-test trees ARE the fidelity gate): alias -> OriginalName, variable args -> sorted CacheArgs, `__typename` dedup, entity-boundary reset.
- Minor cleanup opportunity (not blocking): the pre-main-walk P1 registration block is now vestigial (the second walk re-registers after `reset()`); it is harmless (P1 is filtered on the main walk and its state is reset before the second walk) and can be removed in a later tidy.
