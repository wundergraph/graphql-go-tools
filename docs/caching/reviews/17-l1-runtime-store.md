# Reviewer notes — task 17: request-lifetime shared L1 store

Commit: (hash recorded in PROGRESS.md).
Task file: [tasks/17-l1-runtime-store.md](../tasks/17-l1-runtime-store.md).
Spec background: RFC-1 §6; maintainer feedback R2.4; appendix J1–J7, M1–M2 (M3/N complete in task 18).

## What this commit adds

The request-lifetime L1 entity store: a `map[string]*astjson.Value` on `requestCache` (POINTERS, never bytes, never marshaled — R2.4), keyed by the SAME derived keys as L2 (one derivation per fetch feeds both layers), read L1 → L2 → subgraph with coverage at each layer, written normalized once per response (only the L2 write marshals), guarded by the hook's transaction under the external-lock invariant (no internal mutex).

## Decisions made

- L1 lookup runs over the already-rendered keys BEFORE any L2 `Get`; a covering (or negative-sentinel) L1 hit early-returns with NO missed keys and NO write-back duties — an L1 hit can never emit L2 traffic (the reviewer-guidance ordering hazard, pinned by the exact op logs).
- An L2-served value populates L1 under ALL of the item's rendered keys (`populateL1`); the fetch write path stores one structural copy under rendered + pending-backfilled keys, so multi-key aliases hit L1 too (pinned by the sku→upc row).
- The `PrepareFetch` gate widened from `useL2` to `L1 || useL2` — with the L2 loop and every `deferSet` now individually gated, an L1-only config produces ZERO store ops end to end.
- Negative L1 sentinel: an `EmptyEntity` result writes `tx.Null()` into L1 unconditionally under `cfg.L1` (within one request nonexistence is a fact; the `NegativeCacheTTL` knob gates only the L2 sentinel).
- H4 resolution (deferred from task 12): shadow stashes an L1-SELECTED value exactly like an L2 one — read-never-serve stays absolute (the stash ladder runs after selection, so this fell out of the task-12 structure; pinned).
- FIXED A LATENT ISOLATION BUG in `resolve.CacheTransaction.StructuralCopy`: `astjson.DeepCopy(nil, v)` is an identity PASSTHROUGH in heap mode (nil arena), and one production resolve entry (`resolve.go:361`) runs the loader with a nil arena — stored L1 values would have aliased live response values there.
  Heap mode now forces a real copy via a marshal round-trip; the arena path (the main production paths) is unchanged and keeps the zero-marshal guarantee.
- OPTIMIZE-PASS FIX surfaced by the e2e: the transitive-order walk consulted only cache-configured entity fetches, so a chain passing THROUGH an unconfigured fetch (products → reviews → products) broke at the middle hop and the pair was narrowed off.
  The pass now builds a dependency index over EVERY fetch in the trees (`collectFetchDependencies`).
- Superseded pins updated: the task-07 "L1-only untouched" gate row (now participates, zero store ops), G2 (isolates the L2 TTL rule with `L1=false`), H7 (split into NO-OP and L1-only-shadow rows).
- N6 (subscription event isolation): VERIFIED, not re-implemented — `clone` resets `requestCache` (pinned since task 02 in `cache_noop_test.go`), and the "no bleed across requests" unit row pins fresh-L1-per-BeginRequest at the controller level.

## What was implemented

- `controller.go` — the `l1` map + `l1Put`/`populateL1`; the L1-first read in `prepareItemState`; the L1/L2-split write paths in `OnFetchResult` (positive and negative); the widened gate.
- `resolve/cache_transaction.go` — the heap-mode `StructuralCopy` fix.
- `optimize_l1_cache.go` — the full-tree dependency index.

Tests:

- `controller_l1_test.go` (9 rows) — in-request reuse with EXACT op log (miss Get + flush Set only; shared-key equality asserted); L1-only zero-store round-trip; write and read isolation (mutate source / mutate served value → stored value pinned unaffected); L2-hit-populates-L1 (no second Get); multi-key backfill reaching L1; negative L1; H4; per-request isolation.
- `l1_e2e_test.go` — the chain fixture (`deal → product(sku) → reviews(upc) → product(upc)`: a genuinely dependency-ordered same-type pair, the only sync shape the narrowing correctly keeps); in-request reuse with a TAMPERED canned response for fetch B (accidental network use fails loudly) and zero store ops; the J mode matrix (NO-OP / L1-only / L1+L2 byte-identical, second request L2+L1); M1 (exactly one `BeginRequest` across parallel eligible fetches) + M2 (parallel single+batch writes, uncorrupted response) under `-race`.

## What to look into (review focus)

- The heap-mode `StructuralCopy` fix — confirm `resolve.go:361` (nil-arena loader) is a real production path; if it is ever removed, the marshal fallback can go with it.
- The GC/arena hazard (reviewer guidance): every value entering `l1` flows through `tx.StructuralCopy`/`tx.Null` — grep `l1Put` call sites.
- The optimize-pass dependency-index fix changes narrowing results for chain shapes — the task-16 rows still pass unchanged; the e2e chain is the new coverage.
- L1 population under ALL rendered keys (not just the selected hit's key) — safe because every rendered key identifies the same item, and readers re-run coverage.

## Verification evidence

- All unit + e2e rows pass; `-race` clean over `engine/cache`, `cachetesting`, `resolve`, and the execution harness.
- Full `v2` and `execution` suites pass, exit 0.
- `golangci-lint` (v2.5.0, repo config minus `modernize`): 0 issues in BOTH modules; `gci`/`gofmt` clean.
