# Commit D2 — request-lifetime shared L1 store + L1 controller path

Plan item: `docs/caching/PLAN.md` §6, D2.
RFC sections: RFC-1 §5 (modes), §6 (thread-safety under defer; §6.2 shared state on `Context`, §6.3 clone isolation, §6.4 lock discipline), §3.7 (L1 keys prefix-free); appendix §5.9 (J), §6 (M), §7 (N).
Phase: D (L1 caching).

## Problem

There was no L1 entity store shared across a request's per-defer-group loaders.

## Solution

Add the runtime L1 path to the controller. The `requestCache` is created once per request and shared by reference across per-defer-group loaders (it lives on `Context.requestCache`), so an L1 map on it is automatically request-lifetime and shared across defer groups; `Context.clone` already nils `requestCache` (S2), so subscription events isolate.

- `requestCache.l1 map[string][]byte`, lazy-initialized inside a hook (under the `MergeSession`/`DataBuffer.Lock`), values heap-cloned.
- `ModeL1`/`ModeL1L2` enabled. `l1Enabled = cfg.L1 && mode∈{L1,L1L2}`; `l2Enabled = cfg.L2 && store != nil && mode∈{L2,L1L2}`.
- L1 KEY is PREFIX-FREE: `xxhash64(entity representation JSON)` (no cache name, no header prefix — L1 is per-request); L2 keys stay prefixed.
- `PrepareFetch` ordering L1 -> L2 -> subgraph: a covering L1 hit serves with NO L2 `Get`; on L1 miss, the L2 path runs as before.
- `OnFetchResult`: writes the normalized entity into L1 (when L1 enabled) AND accumulates the deferred L2 `Set` (when L2 enabled).
- `OnFetchSkipped`: splices from the chosen `FromCache` (L1 or L2).
- All L1 access is inside a hook's `MergeSession` (under `DataBuffer.Lock`) — no internal mutex (RFC-1 §6.4).
- `cachetesting.RealishCache` supports `ModeL1`/`ModeL1L2`.

## Key decisions

- The shared L1 store reaches defer groups through the by-reference-shared `Context.requestCache`; the lock guard is the scoped `MergeSession` (no extra mutex).
- L1 keys are prefix-free (per-request, no vary-by needed); L2 keys remain prefixed.

## Tests

- White-box `controller_l1_test.go`: J1-J7 mode matrix (NoOp/L1/L2/L1+L2) over the same entity input — L1 hit short-circuits with NO L2 `Get`; an L2 hit populates L1 so a later same-type fetch hits L1; responses are data-equal across modes (mode-blindness); store ops show L2 `Get`/`Set` only where expected. An explicit L1 write-on-fetch row.
- `resolve/cache_context_clone_test.go`: N6 clone isolation — `Context.clone` yields a fresh `requestCache` (no shared L1 across subscription events).

Verification:

- `cd v2 && go test ./pkg/engine/resolve/cache/... -count=1 -race` — PASS.
- `cd v2 && go test ./pkg/engine/resolve/ -count=1` — PASS.
- `cd execution && go test ./engine/ -run 'Caching' -count=1 -race` — PASS.
- `cd v2 && go build ./pkg/... && go vet ./pkg/engine/resolve/cache/...` — clean.

## Deferred (follow-up D3)

The deterministic PUBLIC defer/concurrency execution rows — N1/N2 (entity cached by the initial fetch served to a deferred fetch; deferred fetch populates L1 visible to a later group) and M1-M3 (lazy-init race-free, parallel L1 writes, per-defer-group loaders share one L1) — are NOT in this commit: they require gate-ordered `ResolveGraphQLDeferResponse` inside a `synctest` bubble, and were deliberately NOT faked with latency. The L1 mechanism is proven by the white-box J rows; the cross-defer-group SHARING is structurally guaranteed (the `requestCache` on the by-reference `Context`) and `-race`-clean. The end-to-end defer+L1 proofs land in D3.

## Reviewer guidance

- One `BeginRequest` per request; the L1 map lives on the shared `Context.requestCache`; clone resets it.
- L1 keys prefix-free; L2 keys prefixed; the loader branches only on `Decision` (mode-blindness, J rows).
- No internal mutex; all L1 access under the `MergeSession` lock.
