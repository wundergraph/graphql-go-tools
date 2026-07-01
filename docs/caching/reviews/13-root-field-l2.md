# Reviewer notes — task 13: root-field L2 caching (plan + runtime)

Commits: two (plan side, then runtime; hashes in PROGRESS.md).
Task file: [tasks/13-root-field-l2.md](../tasks/13-root-field-l2.md).
Spec background: RFC-2 §7.2; RFC-1 §3.5, §5.1(b); appendix D/F/I/J root arms, H5.

## What this adds

Root-field fetches now cache: the configurator's root-field arm (commit 1) and the controller's root-field branch (commit 2) — whole-response-scoped entries per field coordinate, alias-independent reuse via the task-09 transforms, and the historical shadow ASYMMETRY (force-refetch, never compare).

## Decisions made

- Mixed-fetch safety net: `rootFieldPolicyForAllRootFields` compares policy VALUES excluding the coordinate (the first pass's `sameRootFieldCachePolicy` semantics) — a merged fetch whose fields all carry equal settings caches as ONE unit; any mix (different values, cached + uncached) leaves `Cache` nil.
  My first cut compared whole structs (coordinate included), which silently declined EVERY merged fetch — caught by the "identical values" row and corrected.
- THE KEY DESIGN (the load-bearing deviation from the RFC sketch): the root-field key preimage is the fetch's root-field COORDINATE plus the request variables in canonical (name-sorted) form — the QUERY TEXT and the rendered input are deliberately EXCLUDED.
  Keying on the canonical input (as RFC-1 sketched) would make alias-variant operations miss (different query text → different input bytes), defeating the task-09 reuse the e2e row requires.
  Safety: coverage guards sub-selection differences, normalization guards shapes, and inline argument literals cannot collide because the engine ALWAYS normalizes with variable extraction (precondition documented at `rootFieldCacheKey`).
  Two fetch shapes sharing a first coordinate share an entry ON PURPOSE (bigger values serve smaller selections; smaller entries fail coverage for bigger ones).
- Shadow asymmetry implementation: a root-field shadow hit yields a plain `DecisionFetch` — no stash, no `Shadow` flag — so a compare is STRUCTURALLY impossible (the first pass stashed and suppressed the compare with a scope check; this is simpler and cannot regress).
  The read still happens (hit-rate visible in store ops); the normal write path overwrites L2.
- Runtime reuse: the root branch reuses `covers`, `normalizeToSchema`/`denormalizeToSelection`, `deferSet`, and the generic `OnFetchSkipped` splice — the only new primitives are `rootFieldCacheKey`/`canonicalVariables`.
- `L1 = false` (root fields are L2 providers only); config declines entirely when nothing is enabled (the task-06 safety net is now reachable).

## What was implemented

- Commit 1 (plan): `cacheKeyBuilder.buildRootFieldSpec`; the configurator's root-field arm + `rootFieldPolicyForAllRootFields`/`sameRootFieldCachePolicy`; rows for full config, identical-values merge, mixed/cached+uncached declines, all-flags-false.
- Commit 2 (runtime): `prepareRootFieldFetch` (single key, single lookup, shared served value), the root-field write branch in `OnFetchResult` (whole response, normalized when aliased, written once), `rootFieldCacheKey` + `canonicalVariables`.

Tests (commit 2):

- `controller_rootfield_test.go` — D-root miss→hit with exact op log and read-key==write-key; coverage failure (smaller entry never serves a bigger selection); key rows (different variables → different keys; variable ORDER irrelevant); F-root write gate; H5 (shadow hit → plain Fetch, ZERO compares, L2 overwritten with the changed value); J (cache-served data byte-identical to network-merged data).
- `rootfield_e2e_test.go` — L2 hit with zero network; the ALIAS-VARIANT operation (`items: products { code: upc title: name }`) served from the SAME entry with its own shape; different arguments miss and fetch; stored bytes pinned (`{"products":[...]}` under the schema names).

## What to look into (review focus)

- The key-design deviation: agree/disagree with excluding the query text (the alternative kills alias reuse; the documented extraction precondition is what makes it safe — if an integrator can reach the planner with UNEXTRACTED inline literals, two operations could collide; flag if that path exists).
- The variables canonicalization is top-level-only (nested object key order inside a variable VALUE still distinguishes) — conservative misses only, never wrong data.
- H5: grep confirms no `CompareShadow` call is reachable for root scope (no stash exists to iterate even if it were).
- Duplication check: the runtime root branch adds no parallel implementations of coverage/normalization/write.

## Verification evidence

- All configurator + runtime rows and the e2e rows pass (e2e first run); `-race` clean.
- Full `v2` and `execution` suites pass, exit 0.
- `golangci-lint` (v2.5.0, repo config minus `modernize`): 0 issues in BOTH modules; `gci`/`gofmt` clean.
