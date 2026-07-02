# Reviewer notes — task 15: root fields re-using the entity cache

Commit: (hash recorded in PROGRESS.md).
Task file: [tasks/15-entity-cache-reuse.md](../tasks/15-entity-cache-reuse.md).
Spec background: RFC-2 §6 derivation note; RFC-1 §3.6; appendix E3/E5; deviation D10.

## What this commit adds

By-key root fields (`product(upc:)`, `user(id:)`) now hit entity entries: the builder derives `EntityKeyMappings` STRUCTURALLY (D10 — definition + federation only, no external mapping config) and freezes the entity's FULL candidate set onto the root-field spec; the runtime renders the arg-derived candidate at lookup, serves the entity value at the field's response key, and backfills the data-derived keys after a fetch or read-hit — all through the SAME best-effort multi-key machinery.

## Decisions made

- KEY STRUCTURAL DIFFERENCE from the first pass: the spec carries the entity's WHOLE candidate set (arg-coverable keys render at lookup; the rest become `PendingCandidates`), not just mapping-derived candidates.
  The first pass built candidates only from the mappings, which made the E3 data-derived backfill (`sku` written after the response) impossible for root reuse.
  This is also what routes everything through `prepareItemState` unchanged — the reviewer-guidance requirement of "no separate lookup path".
- Candidate representations get `TypeName` set at freeze time: the arg-derived lookup item has no `__typename`, and the task-07 template already falls back to `representation.TypeName` (the seam was reserved for exactly this).
- Serving shape: the item state carries `EntityMergePath = [field's RESPONSE key]` — the splice lands the denormalized ENTITY at `data.<alias>`, and the write side extracts the entity below that key before normalizing; coverage/transforms run against the FIELD SUBTREE (`reuseProvides` per-handle override), because the cached value is the entity, not the whole response.
  This finally populates `ItemCacheState.EntityMergePath` (reserved since task 02/D4).
- The `CacheName` constraint is enforced BY CONSTRUCTION: the key prefix is the policy's cache name, so a mismatched name renders a different key and simply misses (pinned both ways in the e2e).
- v1 argument-binding constraint (documented at `deriveEntityKeyMappings`): the runtime reads the argument value from a request variable NAMED LIKE THE KEY FIELD (`query($upc:...){ product(upc:$upc) }`).
  Inline literals (extracted to synthetic variable names) and differently-named variables leave the candidate unrenderable — a plain fetch with post-response backfill, never wrong data.
  Operator-declared bindings are the recorded follow-up.
- Fallbacks fail open to the PLAIN root-field path: a non-object subtree (list-returning fields), a missing subtree, or zero templates.
- The root-field shadow asymmetry (task 13) applies to reuse too: shadow clears the served value; plain `DecisionFetch`, no compare possible.

## What was implemented

- `cache_key_builder.go` — `buildRootFieldSpec` extension, `deriveEntityKeyMappings`, `keySelectionSetFieldNames` (ported from the first pass's derivation with the candidate-set change above).
- `controller.go` — `prepareRootFieldEntityReuse` (+ `rootFieldSubtree`, `entityLookupItem`), the `reuseProvides` per-handle override, the per-item `EntityMergePath` splice/extract arms in both merge hooks.

Tests:

- `entity_reuse_test.go` — builder rows (full mappings + both candidates with `TypeName`, arg-mismatch/no-entity/multi-root-field declines); E3 with the EXACT ordered ops (`[Get upcKey, Set upcKey refresh, Set skuKey backfill]` in the ENTITY key space, cross-derived from a real entity fetch so read key == write key is proven by construction); the hit row (entity value spliced at `data.product`); E5 read-hit backfill; the missing-variable fallback (zero lookups, both candidates pending).
- `entity_reuse_e2e_test.go` — the full loop: `featuredReview.product` primes the entity (upc + sku keys), `product(upc:$upc)` is served from that entry with ZERO products loads (canned network response deliberately different, so accidental network use fails loudly), and the mismatched-`CacheName` variant fetches from the network.

## What to look into (review focus)

- The whole-candidate-set decision (vs the first pass's mapping-only candidates) — it is what makes E3 satisfiable; confirm no downside for lookup cost (one extra unrenderable candidate per non-arg key).
- `entityWriteKeys` in the unit tests derives expected keys from a REAL entity fetch through the controller — the strongest read==write proof available without duplicating key derivation in tests.
- The v1 variable-name constraint: confirm the documented behavior (miss, fetch, backfill — never wrong data) is acceptable until operator bindings land.
- `reuseProvides` joins the per-handle side maps under the same external-lock invariant.

## Verification evidence

- All builder/runtime/e2e rows pass (e2e first run); `-race` clean.
- Full `v2` and `execution` suites pass, exit 0.
- `golangci-lint` (v2.5.0, repo config minus `modernize`): 0 issues in BOTH modules; `gci`/`gofmt` clean.
