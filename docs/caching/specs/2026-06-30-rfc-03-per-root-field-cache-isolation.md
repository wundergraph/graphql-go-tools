# RFC-3: Per-root-field cache isolation

Status: final for review.
Author: caching working group.
Branch under change: `feat/eng-7770-add-defer-support-part-4`, code under `v2/pkg/engine/plan/`.
Companion contracts: RFC-1 (`docs/caching/specs/2026-06-30-rfc-01-loader-cache-abstraction.md`, the runtime cache config) and RFC-2 (`docs/caching/specs/2026-06-30-rfc-02-caching-planner.md`, the caching planner passes).
Ground truth: `scratchpad/caching-rfc/DOSSIER.md` (esp. §5.5 "all root fields in a fetch must share identical policy or L2 is disabled") and `scratchpad/caching-rfc/explore-isolatedRootField.md` (the OLD `isolatedRootField` mechanism and the additive-approach analysis).

RFC-2 §7.2 ships the conservative, correct-but-coarse all-or-nothing rule for root-field fetches and explicitly forward-references THIS file for the per-field isolation optimization it deliberately defers (RFC-2 §7.2, §11 staging table line ~1025, appendix line ~1062).
RFC-3 is that follow-on.
It is a pure optimization layered on top of RFC-2 v1, and it changes NONE of RFC-2's v1 contract:
with RFC-3 off, the planner produces byte-identical plans to RFC-2 v1, which in turn produce byte-identical plans to the pre-caching branch.

---

## 1. Summary and motivation

### 1.1 The concrete problem

The path builder merges sibling query root fields from the SAME datasource into ONE fetch.
On the current branch this happens in `planWithExistingPlanners` (`path_builder_visitor.go:829-902`):
when `MergeAliasedRootNodes` is set (`datasource_configuration.go:425-433`), a second top-level root field on the same datasource joins the planner that already owns the first, so both render into one subgraph operation and one `SingleFetch`.

Caching then meets a hard rule, carried verbatim from the OLD implementation and reproduced additively by RFC-2.
The dossier states it (dossier §5.5, OLD `caching_planner_state.go:700-745`):

> `RootFieldCacheConfig` requires ALL root fields in a fetch to share an IDENTICAL policy, or L2 is silently disabled for that fetch.

RFC-2 §7.2 re-expresses this as `rootFieldPolicyForAllRootFields`, which walks `info.RootFields` and returns a policy ONLY when every root field resolves to the same one;
otherwise the stamper leaves `Cache` nil and the whole fetch is L2-declined.
This is the conservative "disable rather than mis-cache" fallback — it never serves wrong data, it just caches less.

A single merged fetch hits this rule in two ways:

- A cached root field merged with an UNCACHED sibling.
  The uncached field resolves to no policy, so `rootFieldPolicyForAllRootFields` returns `(_, false)` and L2 is disabled for BOTH.
  The cacheable field silently stops being cached.
- Two cached root fields with DIFFERENT policies.
  For example `me` configured to cache in `users`/30s and `cat` configured to cache in `pets`/60s:
  the policies differ, so L2 is disabled for BOTH.

### 1.2 Why this is also an optimization, not only a fix

Even when two cached root fields share an IDENTICAL policy — so the all-or-nothing rule is satisfied and L2 stays enabled — merging them is suboptimal.
A merged fetch can only be cached as a COARSE unit:
its single L2 entry covers the whole combined response, so an operation that selects only one of the two fields misses, and a change to either field's data invalidates the entry for both.

Isolating each cached root field into its own single-root-field fetch gives each field its OWN L2 key, so it reuses across operations that select different subsets, and so the work later in the implementation order — root fields that RE-USE the entity cache — can match a root-field fetch against an entity-cache entry one field at a time.

Isolation therefore does two things at once:
it recovers correctness (no accidental L2-disable when policies are mixed), and it improves hit rate (finer cache granularity and better reuse).

### 1.3 What RFC-3 delivers

A DEDICATED, gated isolation mechanism that forces each qualifying cached query root field into its OWN planner, hence its own `SingleFetch`, and prevents any other top-level root field from being folded into it.
Each isolated fetch then carries its own `*resolve.FetchCacheConfig` (stamped by RFC-2's existing stamper, §7) with its own `CacheName`/`TTL`/`KeySpec`, and the isolated fetches run in parallel.

The mechanism is driven by RFC-1's self-contained `cacheconfig.CacheConfigProvider`, never by `FederationMetaData`, and it is gated so that when no provider is configured it is provably never entered:
the path builder takes its existing branches and produces byte-identical plans.
RFC-2 §7.2's all-or-nothing stamper rule is RETAINED as the residual safety net for any fetch that still ends up mixed.

---

## 2. Background

### 2.1 What OLD `caching-base` did

Per-root-field isolation lived entirely in the path builder and the caching planner state, not in the loader.
It had two cooperating halves (explore §1).

The planner half — a flag plus three sites (OLD `path_builder_visitor.go`):

1. `objectFetchConfiguration.isolatedRootField bool` (OLD `:116-119`):
   "marks planners for cached query root fields that must not merge with other root fields."
2. `handlePlanningField` (OLD `:554-599`) computed `isMutationRoot` and `isCachedQueryRoot := c.isCachedQueryRootField(...)`;
   when either was true it went straight to `addNewPlanner` (a brand-new isolated planner) instead of `planWithExistingPlanners`, and for the cached-root case it stamped the flag.
3. `isCachedQueryRootField` (OLD `:1340-1358`) was the gate:
   false when entity caching was globally disabled;
   only for `OperationTypeQuery`;
   only for DIRECT children of the root (`strings.Count(currentPath, ".") == 1`);
   and only when the field had explicit config (`RootFieldCacheConfig(typeName, fieldName) != nil`).
4. The merge guard in `planWithExistingPlanners` (OLD `:783-791`):
   ```go
   // Don't merge other query root fields into isolated planners (cached root fields).
   if c.isParentPathIsRootOperationPath(parentPath) && plannerConfig.ObjectFetchConfiguration().isolatedRootField {
       continue
   }
   ```

The config half — `configureFetchCaching` (OLD `caching_planner_state.go:594-746`) produced the per-fetch config and applied the all-or-nothing rule (OLD `:697-745`):
for a root-field fetch, ANY uncached root field, or any two root fields whose policies differed in `CacheName`/`TTL`/`IncludeSubgraphHeaderPrefix`, disabled L2 for the whole fetch.

Net effect:
each cached query root field became its own planner, hence its own `SingleFetch`, with no other top-level root field folded in, so the all-or-nothing rule was trivially satisfied (one field, one policy) and each fetch got its own independent `Enabled`/`CacheName`/`TTL`.

### 2.2 Why the current defer branch needs a fresh additive approach

The current branch has NO `isolatedRootField` at all (verified: `objectFetchConfiguration` at `path_builder_visitor.go:112-126` has no such field, and `grep isolat pkg/engine/plan/*.go` is empty), and NO `RootFieldCacheConfig` / `DisableEntityCaching` (also empty).
So the OLD mechanism is entirely absent; it would have to be re-introduced.

RFC-2's architecture deliberately does NOT re-introduce it in the core plan.
RFC-2 PR1 puts `path_builder_visitor.go` on the forbidden list and keeps the planner-visitor diff at zero, because the whole RFC-2 philosophy is additive passes with a byte-identical planner visitor (RFC-2 §5, §11.1, appendix §16).
RFC-2 reproduces only the SAFE half — the all-or-nothing decline, re-expressed additively in the post-planning stamper as `rootFieldPolicyForAllRootFields` (RFC-2 §7.2) — and forward-references this RFC for the isolation optimization, calling out that it is "not dropped" but layered on later (RFC-2 §7.2, §11, §16).

This RFC honors that split.
It re-introduces isolation as its own focused, gated unit so the one seam it needs gets its own review, while RFC-2's "existing planners untouched" guarantee holds for everyone who does not enable caching.

---

## 3. Design

### 3.1 The central tension: isolation is a merge decision, not a post-pass

The rest of caching (RFC-2) is shaped as passes over the FINISHED fetch tree, after `createConcreteSingleFetchTypes`.
Isolation cannot take that shape.

Whether two root fields share a fetch is a PLANNING-MERGE decision made during path building.
By the time the fetch tree is concrete, the two fields' input templates, response-tree attribution, and dependencies are already FUSED into one `SingleFetch`:
the subgraph operation string renders both fields, the response `Data` tree attributes both subtrees to one `FetchID`, and there is one dependency edge set.
A post-planning pass that tried to "un-merge" that fetch back into two would have to re-split the rendered operation, re-derive per-field response attribution, and recompute dependencies — exactly the work the planner already did, redone against a lossier data structure (explore §4, §5.3).

So isolation must influence the merge decision UP FRONT.
That is the one place caching genuinely needs to touch `path_builder_visitor.go`, and it is the reason this is a separate, deferred RFC rather than part of RFC-2's zero-diff core (§7).

The design below makes that touch as additive as the concern allows:
all decision logic and all tests live in a NEW file;
the seam into the forbidden visitor is two small guarded call sites plus one additive flag field;
and the seam is gated so it is provably never entered when caching is off (byte-identical plans).

### 3.2 The dedicated isolation unit

The mechanism is a single, named, independently-testable unit, `rootFieldIsolation`, in a NEW file `plan/root_field_isolation.go`.
It owns the gate predicate and nothing else.
The path builder holds one `*rootFieldIsolation` field (nil when caching is not configured) and calls it at the two cooperating sites.

```go
// plan/root_field_isolation.go (NEW)
//
// rootFieldIsolation decides whether a query root field must be planned into its
// own fetch so it can carry an independent cache policy (RFC-3). It is the SOLE
// owner of the isolation gate. When the caching provider set is empty the whole
// unit is nil and the path builder takes its existing branches unchanged, so plans
// are byte-identical to RFC-2 v1.
type rootFieldIsolation struct {
	// providers is the per-datasource caching provider set (RFC-1 §7.3), the SAME
	// map RFC-2 wires into the postprocess stamper. Reading RootFieldPolicy here,
	// not FederationMetaData, keeps the federation->caching decoupling intact.
	providers map[string]cacheconfig.CacheConfigProvider
}

// shouldIsolate reports whether field is a cached query root field that must be
// isolated into its own planner. It mirrors the OLD isCachedQueryRootField gate
// (explore §1.1, site 2), reading the self-contained provider instead of
// FederationMetaData.
func (r *rootFieldIsolation) shouldIsolate(field *currentFieldInfo, operationType ast.OperationType) bool {
	if r == nil || len(r.providers) == 0 {
		return false // caching not configured -> never isolate (byte-identical plans)
	}
	if operationType != ast.OperationTypeQuery {
		return false // queries only; mutations isolate via the existing isMutationRoot path (§5, D2)
	}
	if strings.Count(field.currentPath, ".") != 1 {
		return false // direct children of the root operation only
	}
	provider := r.providers[field.ds.Hash().String()] // datasource of the suggestion
	if provider == nil {
		return false
	}
	_, ok := provider.RootFieldPolicy(field.typeName, field.fieldName)
	return ok // isolate only when this exact root field has a cache policy
}
```

(The datasource-key lookup mirrors how RFC-2 keys its provider map by DataSourceID;
the exact accessor is settled in PLAN against `dataSourceConfiguration[T].Caching()`, RFC-1 §7.3, RFC-2 §11.2 — the point is it reads the provider, never `FederationConfiguration()`.)

The two cooperating halves wire into the existing path builder.

Half one — start a new planner instead of merging (the additive branch in `handlePlanningField`, `path_builder_visitor.go:657-673`).
Current code:

```go
isMutationRoot := c.isMutationRoot(field.currentPath)

if isMutationRoot {
	plannerIdx, planned = c.addNewPlanner(field, isMutationRoot)
} else {
	plannerIdx, planned = c.planWithExistingPlanners(field)
	if !planned {
		plannerIdx, planned = c.addNewPlanner(field, isMutationRoot)
	}
}
```

After (only the `// RFC-3` lines are added; every existing line is byte-identical, and the new branch is dead code when `c.rootFieldIsolation` is nil):

```go
isMutationRoot := c.isMutationRoot(field.currentPath)
isCachedQueryRoot := c.rootFieldIsolation.shouldIsolate(field, c.operationType()) // RFC-3; false when caching off

if isMutationRoot || isCachedQueryRoot { // RFC-3 adds the second disjunct
	plannerIdx, planned = c.addNewPlanner(field, isMutationRoot)
	if planned && isCachedQueryRoot { // RFC-3
		c.planners[plannerIdx].ObjectFetchConfiguration().isolatedRootField = true
	}
} else {
	plannerIdx, planned = c.planWithExistingPlanners(field)
	if !planned {
		plannerIdx, planned = c.addNewPlanner(field, isMutationRoot)
	}
}
```

Half two — refuse to merge another top-level root field into an isolated planner (the additive guard at the top of the `planWithExistingPlanners` loop, `path_builder_visitor.go:830`).
`isParentPathIsRootOperationPath` ALREADY EXISTS on this branch (`path_builder_visitor.go:904-906`), so the guard reuses it verbatim:

```go
for plannerIdx, plannerConfig := range c.planners {
	// RFC-3: never fold another top-level operation field into an isolated
	// (cached) root-field planner; its own subtree still merges normally.
	if c.isParentPathIsRootOperationPath(field.parentPath) && plannerConfig.ObjectFetchConfiguration().isolatedRootField {
		continue
	}
	// ... existing body unchanged ...
}
```

The flag itself is one additive field on `objectFetchConfiguration` (`path_builder_visitor.go:112-126`):

```go
type objectFetchConfiguration struct {
	// ... existing fields ...
	isolatedRootField bool // RFC-3: cached query root field; reject other top-level root fields
}
```

### 3.3 The entity-root-node trap (a first-class invariant)

The merge guard keys off `field.parentPath` being a root operation path (`query`/`mutation`/`subscription`), NOT off `field.suggestion.IsRootNode`.
This is load-bearing, and getting it wrong is the subtle correctness bug the OLD comment warned about (explore §1.1, site 3).

Entity types are ALSO datasource root nodes:
a `Product` reachable by `@key` is an `IsRootNode` at its own coordinate.
If the guard used `IsRootNode`, it would wrongly block NESTED entity and child fields from merging into the isolated planner that legitimately needs them — the isolated root field's own subtree would be torn apart.
`isParentPathIsRootOperationPath` blocks ONLY fields whose parent is the top-level operation, so the isolated root field's own subtree (including nested entities) still merges normally.
RFC-3 treats this as a named invariant with a dedicated test (§6).

### 3.4 Per-field isolation (the granularity decision)

RFC-3 isolates PER FIELD:
every cached query root field gets its own planner, even two cached root fields that happen to share an identical policy.
This mirrors OLD and is the deliberate choice over per-policy-group batching (the alternative is weighed and rejected in §5, D1).
Per-field maximizes the runtime benefit that motivates the optimization — each field gets its own L2 key, reuses across operations that select different subsets, and can be matched against the entity cache one field at a time — and it keeps the merge guard a single, simple predicate with no policy comparison in the hot path.

### 3.5 Runtime shape

Each isolated root field becomes a distinct `*SingleFetch`.
RFC-2's stamper (§7) then runs over the now-isolated tree exactly as it does today and stamps each fetch's `*resolve.FetchCacheConfig` from its single root field's policy;
because each isolated fetch has exactly one root field, `rootFieldPolicyForAllRootFields` returns `(policy, true)` and L2 is enabled per fetch with that field's own `CacheName`/`TTL`/`KeySpec`.

The isolated fetches carry NO `DependsOnFetchIDs` between each other (they are independent top-level roots), so they execute in PARALLEL (the OLD planner test confirmed FetchID 0 and FetchID 1 with no dependency edge, explore §3).
At runtime RFC-1's controller does an independent L2 `Get`/`Set` per fetch, under that fetch's own cache name, TTL, and keys (RFC-1 §3).

The cost is the cold-cache amplification:
N single-field fetches mean N subgraph round-trips where one batched operation would have served all N (explore §3).
The benefit is warm-cache reuse:
each field is served from L2 independently and its network fetch is skipped.
This cost/benefit (more round-trips on cold cache, finer reuse on warm cache) is exactly why isolation is an optimization that can be deferred, not a correctness requirement (§7).

### 3.6 Rejected alternative: a post-planning fetch-tree split pass

A design that WOULD be purely additive in RFC-2's literal sense — a new `FetchTreeProcessor`, zero edits to any visitor body — is a post-planning pass that re-partitions a merged root-field `SingleFetch` into per-policy-group fetches.
RFC-3 considered and rejected it (explore §4, §5.3).

To split a merged `SingleFetch` after `createConcreteSingleFetchTypes`, the pass would have to:
re-derive a separate input template (subgraph operation) for each policy group, by re-splitting the already-rendered operation;
re-attribute response-tree fields to the new per-group `FetchID`s;
and recompute `DependsOnFetchIDs`.
That is high complexity and high risk — it redoes the planner's merge/attribution work against the lossier finished tree, precisely the data structure that no longer carries per-field provenance cleanly (RFC-2 §9.1 documents the analogous attribution loss for `ProvidesData`).

Up-front merge PREVENTION is strictly simpler and lower-risk:
it lets the planner never fuse the fields in the first place, so no input template, attribution, or dependency ever needs to be re-derived.
RFC-3 accepts the single narrow, gated, separately-reviewed visitor seam (§3.2) in exchange for avoiding the split pass entirely.

---

## 4. Interaction with RFC-2 and RFC-1

### 4.1 RFC-2's passes are shape-agnostic and need no change

RFC-3 changes the SHAPE of the fetch tree (more, smaller root-field fetches);
it does not change the CONTENT of any pass.
RFC-2's passes all operate on whatever tree the planner produced:

- P1 `cacheProvidesDataVisitor` (RFC-2 §9) reads per-field fetch ownership from `planningVisitor.fieldPlanners` AFTER path building, so it sees the isolated planners and builds correct per-fetch `ProvidesData` automatically.
- The stamper `cacheConfigStamper` (RFC-2 §7) walks the finished tree and stamps each fetch; with isolation on, each root-field fetch has one root field, so the all-or-nothing rule is trivially satisfied and each gets its own `Cache`.
- `rootFieldPolicyForAllRootFields` (RFC-2 §7.2) is RETAINED as the residual safety net.
  With RFC-3 on it almost always sees a single-field fetch and returns `(policy, true)`;
  if any fetch somehow remains mixed (a datasource where merging is forced by other rules), it still declines L2 for that fetch rather than mis-caching.
  RFC-3 makes the rule's "disable" branch rare, it does not remove the rule.
- `optimizeL1Cache` (RFC-2 §10) narrows `cfg.L1` cross-tree from `ProvidesData` + `DependsOnFetchIDs`;
  more, smaller fetches are just more nodes to consider, and narrowing is conservative-safe either way.

Ordering:
RFC-3 acts during path building, BEFORE RFC-2's P1 walk and BEFORE the postprocess stamper.
So the sequence is: RFC-3 isolates planners -> P1 builds `ProvidesData` over the isolated planners -> postprocess stamps the isolated fetches.
No RFC-2 pass needs to be reordered or modified.

### 4.2 RFC-1's runtime config

Each isolated fetch carries its own `*resolve.FetchCacheConfig` (RFC-1 §3.6) with its own `L2`, `CacheName`, `TTL`, and frozen multi-key `KeySpec`.
The loader and controller (RFC-1 §3) treat each isolated fetch independently;
because there are no `DependsOnFetchIDs` between them, the loader schedules them in parallel and the controller does an independent lookup/write per fetch.
RFC-3 adds NO new runtime type and NO new field to `FetchCacheConfig`;
it only changes how many fetches exist to be stamped.

### 4.3 The federation decoupling holds

The gate reads `cacheconfig.CacheConfigProvider.RootFieldPolicy(typeName, fieldName)` (RFC-1 §7.3), the SAME provider RFC-2 consumes, reached parallel to `FederationInfo`.
It never calls `field.ds.FederationConfiguration()` for the isolation decision.
So no federation pointer re-enters the planning decision, consistent with RFC-1 §7.4 and RFC-2 PR2.

---

## 5. Decisions and alternatives

- D1. Isolate PER FIELD, not per policy group.
  Per-field gives every cached root field its own L2 key (maximum reuse and the cleanest path to root-fields-reusing-the-entity-cache), and keeps the merge guard a single predicate with no policy comparison.
  The alternative, per-policy-group (same-policy cached root fields share one batched fetch), saves cold-cache round-trips but reintroduces coarse-granularity caching — the very thing §1.2 wants to fix — and complicates the guard with a per-policy equality check in the path-building hot path.
  RFC-3 picks per-field, matching OLD;
  per-policy-group batching can be revisited later if cold-cache amplification proves to dominate in practice (D4).

- D2. Query-only;
  composes cleanly with mutation isolation.
  `shouldIsolate` returns false for any non-`Query` operation, and mutations continue to isolate through the existing `isMutationRoot` branch (`path_builder_visitor.go:657-673`, `:1391-1399`).
  A field can never be both:
  `isMutationRoot` requires `OperationTypeMutation`, `shouldIsolate` requires `OperationTypeQuery`.
  So `addNewPlanner` is still called once, and there is no double-isolation.
  Subscriptions are out of scope (the subscription branch in the path builder is separate, and subscription caching is staged to RFC-2 v2).

- D3. Inside defer groups, isolation composes with the per-defer-group fetch trees.
  A cached query root field is a direct child of the operation root (`strings.Count(currentPath, ".") == 1`);
  defer applies to sub-selections, so the isolation decision is made at the top level and the isolated planner's deferred sub-fields still merge into its subtree under the existing `field.deferID` checks (`path_builder_visitor.go:839-848`).
  `extractDeferFetches` (postprocess) then splits those into per-`DeferID` group trees as usual.
  The merge guard only blocks OTHER top-level operation fields, so it does not interfere with defer extraction.
  A test (§6) pins this.

- D4. No cold-cache amplification cap in v1.
  Isolation only triggers for root fields the operator EXPLICITLY configured with a cache policy, so the amplification is opt-in and bounded by the number of cache-configured root fields in one operation.
  A cap would add config surface and a behavior cliff for a cost that only appears on cold cache.
  RFC-3 ships without a cap (matching OLD) and flags amplification as a monitored risk;
  if it dominates, the per-policy-group batching of D1 is the natural mitigation.

- D5. The gate reads the self-contained provider, not `FederationMetaData` (§4.3).
  This is a hard rule, verified in reviewer guidance.

- Alternative rejected: the post-planning fetch-tree split pass (§3.6).
  It is the only design that would keep `path_builder_visitor.go` at a literal zero diff, but it is rejected as high-complexity and high-risk;
  up-front merge prevention is simpler and lower-risk.

---

## 6. Reviewer guidance

- NO-OP proof (the PR-equivalent of RFC-2's zero-impact gate).
  With no `CacheConfigProvider` wired, assert the postprocessed plans are BYTE-IDENTICAL to pre-RFC-3 (full-plan `assert.Equal`, golden plans).
  This proves the new branch in `handlePlanningField` and the new guard in `planWithExistingPlanners` are never entered when caching is off.

- The entity-root-node trap (§3.3).
  Add a test with a NESTED entity (e.g. `Product` by `@key`) under an isolated cached root field, and assert the nested entity/child fields still merge into the isolated planner's subtree — confirming the guard keys off `parentPath` being a root operation path, NEVER off `IsRootNode`.

- Parallelism.
  Assert isolated fetches carry no `DependsOnFetchIDs` between each other (they run in parallel).

- The all-or-nothing safety net still applies.
  Assert that RFC-2 §7.2's `rootFieldPolicyForAllRootFields` is unchanged and still declines L2 for any residual mixed-policy fetch (a defensive case isolation makes rare, not impossible).

- Port the three OLD planner tests (explore §3.1) and assert FULL plans with `assert.Equal` (per the project rule: never `assert.Contains`, never fuzzy comparison):
  1. `query Q { me { id username } cat { name } }` with `me`->users/30s, `cat`->pets/60s produces TWO parallel raw fetches, FetchID 0 `{L2, CacheName:"users", TTL:30s}` and FetchID 1 `{L2, CacheName:"pets", TTL:60s}`, neither with `DependsOnFetchIDs`.
  2. `query Q { me { id } user(id:"1") { username } }` with only `me` cached produces FetchID 0 (cached `me`) isolated from FetchID 1 (uncached `user`, no `Cache`).
  3. With caching not configured, isolation is skipped, the fields merge into ONE fetch, and the all-or-nothing rule declines L2 — byte-identical to RFC-2 v1.

- Defer composition (D3).
  Add a test with a cached root field whose subtree contains a deferred fragment, and assert the isolated root field and its deferred sub-fetch land in the correct per-`DeferID` trees.

- Federation decoupling (D5).
  Confirm `shouldIsolate` reads `CacheConfigProvider.RootFieldPolicy`, never `field.ds.FederationConfiguration()`;
  no federation pointer enters the isolation decision.

- Surgical-diff review.
  The change to `path_builder_visitor.go` must be exactly: one additive field on `objectFetchConfiguration`, the second disjunct + flag-set in `handlePlanningField`, and the one guard at the top of `planWithExistingPlanners`.
  All decision logic and all tests live in `plan/root_field_isolation.go` and its test file.

---

## 7. Why it is deferred from the core plan

RFC-3 is intentionally NOT part of the core caching plan, for three reasons.

- It is a pure optimization, not correctness.
  RFC-2 v1's all-or-nothing decline never mis-caches;
  it just caches less when root-field policies are mixed.
  RFC-3 recovers the lost caching and improves granularity, but the system is correct without it.

- It is the ONE caching feature that must touch `path_builder_visitor.go`, which RFC-2 PR1 forbids to keep the planner-visitor diff at zero (RFC-2 §5, §16).
  Carving it into its own RFC preserves RFC-2's "existing planners untouched" guarantee for every integrator who does not enable caching, and gives the single visitor seam its own focused review.
  The rest of caching reaches runtime with ZERO planner-visitor edits;
  this one optimization is where, and only where, that bar is relaxed — under a gate that makes the relaxation invisible when caching is off.

- It layers cleanly on top and changes none of RFC-2's v1 contract (RFC-2 §7.2, §11).
  With RFC-3 off, plans are byte-identical to RFC-2 v1.
  RFC-2's passes need no modification to accommodate it (§4.1).

Where it slots in the mandated implementation order:
structure first, then L2 for ENTITIES, then L2 for ROOT FIELDS, then L2 for ROOT FIELDS that RE-USE the entity cache, then L1.
Per-root-field granularity is an enhancement to the L2-root-fields stage — it only matters once root fields cache at all — so it naturally trails into that stage and after it, ahead of the L1 work.
It ships LAST among the L2-root-field items:
RFC-2 v1's conservative all-or-nothing decline is correct from the start, and RFC-3 upgrades it to per-field caching once the L2-root-field machinery is in place.
