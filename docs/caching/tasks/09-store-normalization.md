# Task 09 — Store-time normalization + argument-suffix keys

Phase: A (L2 entities).
Dependencies: task 07 (works with 08; sequencing with 08 is flexible).
References: maintainer feedback R2.5; OLD `loader_cache_transform.go` on the `caching-base` worktree; appendix row D6.

## Problem

A value cached by one fetch must be reusable by another fetch that selects the same fields under DIFFERENT aliases, and fields selected with different ARGUMENTS must never collide.
Without normalization, alias-shaped values poison the cache for other operations; without argument-suffix keys, `friends(first:5)` can serve `friends(first:20)` — a stale-data correctness hole.

## Scope

The transform module in `v2/pkg/engine/cache` (port + adapt the OLD transform pipeline):

- WRITE path: normalize values to SCHEMA field names before caching, using the `ProvidesData` tree's `OriginalName` (alias → schema name); fold each field's arguments into the stored object keys as a deterministic argument suffix (from `CacheArgs`, sorted, hashed with the repo xxhash pattern).
- READ path: denormalize a cached value back to the REQUESTING operation's aliases before splice, again driven by the requesting fetch's `ProvidesData`.
- The coverage walk (task 07) matches on schema-name + arg-suffix, so an arg-mismatched cached field is a MISS, not a hit.
- `HasAliases` is the fast-path gate: trees without aliases/args skip the transform entirely.
- All transforms run inside the hook's `CacheTransaction` (arena-backed values, `StructuralCopy` where aliasing could corrupt).
- Keep the transform a separable module (partial fetching in task 19 reuses it per-field).

## Tests

Controller unit tests (constructed astjson):

- Round-trip: alias-shaped response → normalized stored form (schema names + arg suffixes, asserted as the FULL stored bytes) → denormalized back under a DIFFERENT alias set (full response asserted).
- D6 arg-awareness: value cached under `friends_<hashA>` does NOT satisfy `friends(first:20)` = `friends_<hashB>` → miss.
- No-alias fast path: `HasAliases == false` skips the transform (assert via the recorded ops / unchanged bytes).
- Nested objects and lists normalize recursively; `__typename` preserved.

Plan-driven e2e rows (task 04 fixtures):

- Operation A caches an entity under aliases; operation B with different aliases over the same fields is served from cache; COMPLETE responses asserted for both.
- Same field with different argument values: B is a MISS and fetches.

## Acceptance criteria

- [ ] Alias-independent reuse proven end to end (the e2e row above).
- [ ] Argument-suffix keys proven both ways (same args → hit; different args → miss).
- [ ] Each response still parsed once; transforms allocate via the transaction only.
- [ ] The transform module has full unit coverage as a standalone unit.
- [ ] Lint-clean.

## Reviewer guidance

- The arg-suffix derivation must be deterministic (sorted args) and shared between the write path and the coverage walk — one implementation, not two.
- Watch allocation: normalization runs per cached entity; keep it arena-based and copy-minimal.
