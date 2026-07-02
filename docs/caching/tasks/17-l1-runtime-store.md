# Task 17 — Request-lifetime shared L1 store

Phase: D (L1).
Dependencies: tasks 09, 16.
References: RFC-1 §6; maintainer feedback R2.4; appendix rows J1–J7, M1–M2 (M3/N rows complete in task 18).

## Problem

There is no L1 entity store shared across a request's per-defer-group loaders.
An entity fetched by the initial tree should serve a deferred fetch of the same entity later in the SAME request, with zero marshaling.

## Scope

- The L1 store lives on the `RequestCache` on the by-reference-shared `Context` (created lazily once per request under `DataBuffer.Lock`), guarded by the hook's `CacheTransaction` — no internal mutex (document the external-lock invariant at the struct).
- Storage model (R2.4): L1 stores `*astjson.Value` — NEVER `[]byte`, NEVER marshals; isolation via `tx.StructuralCopy` on write and on read (so merges cannot corrupt the stored value).
- SHARED KEYS: L1 uses the SAME derived keys as L2 (the task 07 `CacheKeyTemplate` output) — derive each key once per fetch, use for both layers; no prefix-free L1 keyspace, no second derivation.
- `PrepareFetch`: L1 → L2 → subgraph ordering with coverage at each layer; an L1 hit short-circuits the L2 `Get`.
- `OnFetchResult`: write NORMALIZED entities (task 09) to L1 (structural copy) and to L2 when enabled — parse once, one normalized form feeds both layers (only the L2 write marshals).
- `OnFetchSkipped`: splice from L1 (denormalized to the requesting fetch's aliases).
- Subscription isolation: `clone` resets `requestCache` (task 02), so each event builds its own L1 — verify, don't re-implement.
- This request-lifetime store is NOT the removed `@requestScoped` directive feature (D11).

## Tests

Controller unit tests: L1 map behavior (structural-copy isolation — mutate the source after write, the stored value is unaffected; and mutate a served value, the stored value is unaffected), shared-key assertion (the L1 key string equals the L2 key string for the same fetch), parse-once (op log shows a single parse per response).

Plan-driven e2e rows:

- J mode matrix J1–J7 over the same query (NO-OP / L1 / L2 / L1+L2): loader branches only on `Decision`; L1 hit short-circuits L2; L2 hit populates L1; data-equal responses across modes.
- M1 lazy init race-free (N parallel eligible fetches → exactly one `BeginRequest`, `-race`, gates).
- M2 parallel writes to the shared L1 (two same-type entity fetches, both miss, both write; no corruption, `-race`).
- N6 subscription event isolation (two events via clone; no cross-event bleed).
- An in-request reuse row on the sync path: fetch A populates L1, dependent fetch B (same entity, subset selection) serves from L1 with zero network.

## Acceptance criteria

- [ ] Zero marshaling on the L1 path (assert via the op logs: no `Set` bytes for L1, no parse on an L1 hit).
- [ ] One key derivation per fetch feeding both layers.
- [ ] All rows pass under `-race` with gate-based ordering (no latency sleeps).
- [ ] The runtime no-op and mode-blindness gates still hold (J rows).
- [ ] Lint-clean.

## Reviewer guidance

- The GC/arena hazard: every `*astjson.Value` stored in the L1 map must be produced via the transaction (`ParseBytes`/`StructuralCopy`), never a heap value smuggled into arena-noscan memory.
- Watch the L1→L2 ordering: an L1 hit must not emit an L2 read or write-back unless coverage genuinely required L2.
