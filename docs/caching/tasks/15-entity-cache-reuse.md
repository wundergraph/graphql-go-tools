# Task 15 — Root fields re-using the entity cache (EntityKeyMappings)

Phase: C.
Dependencies: tasks 08, 13.
References: RFC-2 §6 derivation note; RFC-1 §3.6 (`EntityKeyMappings` as additional candidates); appendix rows E3, E5; deviation D10 (PLAN §7).

## Problem

A by-key root field (e.g. `product(upc:)`, `user(id:)`) returns exactly the entity the entity cache may already hold, but its root ARGUMENTS are not linked to the entity `@key`, so it cannot reuse those entries.

## Scope

Plan side (extends task 06's `cacheKeyBuilder`):

- DERIVE `EntityKeyMappings` structurally (D10): resolve the root field's return type from the definition, collect its argument names, and for each resolvable `@key` set emit a mapping ONLY when every key field name matches a root-arg name (`product(upc:)` maps `@key(upc)` but not `@key(sku)`).
  This branch carries no external mapping config; operator-declared overrides are a recorded follow-up.
- Mappings are frozen BY VALUE onto the ROOT-FIELD spec (entity fetches key by representation and need none).
- v1 reuse constraint (D10): reuse works only when the by-key root-field policy shares `CacheName` with the entity policy (read key == write key); document this at the policy structs.

Runtime (extends the task 08 multi-key model):

- At `PrepareFetch` for a root field with `EntityKeyMappings`: render an ENTITY candidate from the root args (mappings are additional candidates, not a separate key space) and look up the entity key space; a hit serves the root field from the entity entry (denormalized per task 09).
- Backfill: after a fetch (or on a read-hit), re-render the remaining entity candidates from the returned data and write them (`OnFetchResult` / `OnFetchSkipped` with `MustWriteBack`).

## Tests

- Builder rows: FULL `EntityKeyMappings` asserted for `product(upc:)`/`user(id:)`-shaped fixtures; no mapping when args do not cover a key set; no federation pointer retained.
- E3: lookup renders ONLY the arg-derived candidate; after the response the data-derived key (e.g. `sku`) is backfilled — exact ordered store ops asserted.
- E5: read-hit backfill via `OnFetchSkipped`.
- e2e (task 04 fixtures): `{ product(upc:"1") { name } }` served from an entity entry primed by a list field (e.g. `topProducts`) — zero network on request 2, COMPLETE response asserted; a mismatched-`CacheName` variant does NOT reuse (the constraint is enforced, not accidental).

## Acceptance criteria

- [ ] By-key root fields hit entity entries end to end.
- [ ] The exact ordered `Get`/`Set` sequence asserted (a wrong key or missing backfill fails).
- [ ] Mappings derived from definition + federation only; never from the policy struct.
- [ ] The `CacheName` constraint tested both ways.
- [ ] Lint-clean.

## Reviewer guidance

- Mappings must flow through the SAME best-effort candidate machinery as multi-key (no separate lookup path in the controller).
