# Commit A2c — batch entity shapes + execution end-to-end L2 + single EndRequest

Plan item: `docs/caching/PLAN.md` §3, A2 (third of three; completes A2).
RFC sections: RFC-1 §3.3 (BatchStats), §3.7 (BatchIndex/BatchEntityKey/EntityMergePath), §5.1(e), §9 (full-batch v1); appendix §5.9 (I), §7 (N3), §4 (Plan harness).
Phase: A (L2 entities).

## Problem

The controller handled only single (non-batch) entity fetches, and nothing proved the whole stack end to end through the real loader.

## Solution

- Batch path in the controller (`len(in.BatchStats) > 0`): per unique-representation bucket `i`, render the candidate key(s) from `BatchStats[i][0]`, `Get`, coverage/freshness/reorder (reusing the A2b helpers), and build one `ItemCacheState` per bucket with `BatchEntityKey=true` and `BatchIndex=i`. FULL-BATCH all-or-nothing: `DecisionSkipFullHit` only when EVERY bucket covers; any miss -> `DecisionFetch` (full refetch; no partial realign in v1). Empty batch short-circuits. On a full hit, the chosen value fans out to ALL targets in `BatchStats[BatchIndex]`; on a fetch, each bucket's entity is extracted by `BatchIndex` from the fresh `_entities` array and written/backfilled.
- `ItemCacheState.BatchEntityKey` added (additive; runtime handle state, not serialized -> no plan-golden impact).
- `cachetesting` datasource load counters for the end-to-end assertion.

## Key decisions

- Full-batch v1: a mixed batch (some hit, some miss) refetches the whole batch rather than splicing a partial subset (partial realign is staged to v2, RFC-1 §9).
- `EntityMergePath` is left empty in this commit: the controller input does not expose the fetch's post-processing merge path, so batch values splice at the target root. This is correct for the `commerce` `_entities` shape (the end-to-end test passes); revisit if an entity fetch needs a non-root merge path (likely alongside C2 root-field<->entity reuse).

## Tests

- Controller white-box `controller_batch_test.go`: batch full-hit (fan-out, two Gets), all-miss (Fetch + both written), mixed (full refetch, no partial), empty (short-circuit, no Get), and `BatchEntityKey`/`BatchIndex` per bucket. Full-value `assert.Equal` on `[]storeOp`.
- Execution end-to-end `caching_e2e_test.go` `TestCaching_EndToEnd_L2EntityHit` (the headline proof, via the public `ResolveGraphQLResponse`): ONE `FakeStore` shared across two `Plan(StageL2Entities, "{ topProducts { upc name reviews { body } } }")` requests. First request MISSES -> runs the faked subgraph -> writes L2 (`Get,Get,Set,Set`); second request HITS L2 (`Get,Get` only), returns the SAME response bytes, and the reviews entity datasource records ZERO loads (`LoadCount("2","topProducts")==0`). Exactly one `EndRequest` per resolve (N3). Full-value byte assertions.

Verification:

- `cd v2 && go test ./pkg/engine/resolve/cache/... -count=1 -race` — PASS.
- `cd v2 && go test ./pkg/engine/resolve/ -count=1` — PASS.
- `cd execution && go test ./engine/ -run 'Caching' -count=1` — PASS (new end-to-end + N3 rows; StageNoop + StageL2Entities goldens unchanged).
- `cd v2 && go build ./pkg/... && go vet ./pkg/engine/resolve/cache/...` — clean.

## Reviewer guidance

- Batch is all-or-nothing in v1; a mixed batch refetches wholly.
- The end-to-end test is the real proof: a second request is served from L2 with zero subgraph loads and identical bytes.
- Negative (A3), shadow (A4), L1 (D2), and partial-batch realign (v2) remain out of scope.
