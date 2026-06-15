# 05 — ASTJSON Primitives: Dependency Spec

Status: normative.
Audience: a reader with NO prior knowledge of entity caching or astjson internals.

This document answers three questions:

1. Which `astjson` APIs does the entity-caching foundation actually require, and why?
2. What are the contracts and safety semantics of each API?
3. Are those APIs released in the `astjson` version `graphql-go-tools` depends on, or do they exist ONLY in an open PR — and therefore, is "land the astjson primitives" a hard prerequisite (PR #0) for everything else?

Short answer to #3: the APIs are NOT released.
They exist only on the open `astjson` PR #16 branch `feat/two-pass-parser`, pinned via a pseudo-version.
"Land + release the astjson primitives" MUST be PR #0 of the stacked plan.
Section 5 gives a concrete dependency strategy.

Cross-links:

- Architecture and the single integration seam: [01-ARCHITECTURE-SPEC.md](01-ARCHITECTURE-SPEC.md)
- Foundation ADR: [adr/0001-foundation.md](adr/0001-foundation.md)
- graphql-go-tools stacked PRs (this is where PR #0 lives): [03-PR-PLAN-graphql-go-tools.md](03-PR-PLAN-graphql-go-tools.md)
- Test + benchmark plan (astjson regression tests to carry): [06-TEST-AND-BENCH-PLAN.md](06-TEST-AND-BENCH-PLAN.md)
- Out-of-scope findings, including doc drift: [07-UNRELATED-FINDINGS.md](07-UNRELATED-FINDINGS.md)

---

## 1. Why astjson at all — the one-paragraph mental model

`astjson` is the JSON value library the resolver uses for all response data.
A parsed JSON document is a tree of `*astjson.Value` nodes (objects, arrays, strings, numbers, bools, nulls).
The resolver allocates those nodes on an arena (a bump-pointer memory block from `go-arena`) so the whole tree can be freed in one shot at the end of a request, with no garbage-collector pressure.

Entity caching's entire job is to take a piece of that tree (an entity, e.g. a `User` object), stash it somewhere (a per-request map for L1, an external store for L2), and later splice it back into a response tree — possibly a DIFFERENT response tree, in a DIFFERENT request.
Doing that safely, cheaply, and without re-parsing JSON is exactly what the astjson primitives below provide.

If you remember one thing: the load-bearing primitive is `StructuralCopy`.
Everything else supports it or is used at exactly one site.

---

## 2. The five required primitives (and the two that ride along but are NOT required)

### 2.1 Required by the foundation

| Primitive | Form | Required because |
|---|---|---|
| `StructuralCopy` | `Parser` method + package func | Isolates a cache value from the live response tree on every cache write, read, and merge. THE core of L1/L2 correctness. |
| `StructuralCopyWithTransform` | `Parser` method + package func | Same isolation PLUS per-field rename / project / passthrough — powers alias normalization and arg-aware cache keys. |
| `Transform` / `TransformEntry` (with `Passthrough`) | types | The data that drives `StructuralCopyWithTransform`. `Passthrough` is the L1-vs-L2 switch. |
| `DeepCopy` (package-level, heap mode) | package func | Used at exactly ONE site: isolate per-request `Variables` onto the heap. |
| `MergeValues` / `MergeValuesWithPath` (2-return form) | package func | Folds cached + fetched data into the response tree (~12 call sites). The signature CHANGED — see 2.3. |
| value constructors + `value.MarshalTo` | methods | Build cache-key objects and serialize L2 entries to bytes. (Pre-existing, stable.) |

### 2.2 Ships in PR #16 but NOT required by the foundation (droppable from a minimal extraction)

- `DeepCopyWithTransform` — exists in astjson, never called by the foundation.
- `CoerceToString` — added in the same PR, not used anywhere in `resolve`.
- The two-pass arena parser is a perf optimization, NOT an API the foundation calls directly — see 2.4. It is coupled to `StructuralCopy` via shared internal slab machinery, so it cannot be cleanly dropped, but the foundation never names it.

### 2.3 Does NOT exist in PR #16 (do not re-introduce — see doc drift)

- `Transform.Forced` — the real struct has only `Entries`, `ArrayItem`, `Passthrough`.
- `MarshalToWithTransform` / `ParseBytesWithTransform` — not present, not used.
  L2 writes use plain `MarshalTo` on an already-structurally-normalized value.

`resolve/CLAUDE.md` references `Transform.Forced` and `MarshalToWithTransform`.
That is stale aspirational documentation, not the shipping API.
A clean extraction MUST follow the code, not the CLAUDE.md surface.
Tracked in [07-UNRELATED-FINDINGS.md](07-UNRELATED-FINDINGS.md).

---

## 2.4 Contracts and semantics

#### `StructuralCopy(a arena.Arena, v *Value) *Value`

Also available as a `Parser` method `func (p *Parser) StructuralCopy(a, v) *Value`.
The foundation uses the Parser method (via `l.parser`) so it reuses the parser's own scratch and avoids a pool mutex.

Semantics:
clones ONLY container nodes (objects and arrays) onto arena `a`,
while ALIASING every scalar leaf (string, number, bool, null) AND every object key string directly from the source `v`.

Properties:

- Cheap — a single tree walk, no byte round-trip, no re-parse.
- The result shares NO mutable container memory with the source, so mutating one tree's object/array does not corrupt the other.
- The result DOES share immutable leaf payloads (string bytes, number bits) with the source.

Safety precondition (critical):
source and destination MUST share the same arena lifetime — same request, reset together.
If a `StructuralCopy`'d value ever outlives its source arena, the shared leaves are freed underneath it: a use-after-free.
This is why L2 writes never hand a `StructuralCopy`'d value to the external store directly — they serialize it to heap bytes via `MarshalTo` first (see 2.6).

`StructuralCopy` is NOT a substitute for `DeepCopy` when crossing an arena/heap boundary.

#### `StructuralCopyWithTransform(a arena.Arena, v *Value, t *Transform) *Value`

Same container-clone / leaf-alias semantics as `StructuralCopy`,
but applies a `*Transform` to objects during the copy.

- `t == nil` falls back to plain `StructuralCopy`.
- `a == nil` falls back to a heap-mode transform copy.
- `OutputKey` strings are copied ONTO the arena (not aliased).
  This is deliberate and load-bearing: arena memory is `noscan`, so a heap string referenced only by an arena key-value pair would be invisible to the GC and could be collected.

This single primitive powers all four Loader cache helpers (see 2.5).

#### `Transform` / `TransformEntry`

`Transform` has exactly three fields:

- `Entries []TransformEntry` — rename rules (`InputKey` -> `OutputKey`); when `Passthrough == false` they ALSO project (unlisted source fields dropped).
- `ArrayItem *Transform` — applied per array element; mutually exclusive with `Entries`.
- `Passthrough bool` — the L1-vs-L2 switch.

`TransformEntry`:

- `InputKey string` — source field name to read.
- `OutputKey string` — destination field name to write (rename / arg-hash suffix); copied onto the arena for GC safety.
- `Child *Transform` — `nil` = plain value copy; non-`nil` = recurse.

There is NO `Forced` field.

`Passthrough` semantics:

- `Passthrough == false` (L2 write):
  output contains ONLY the fields listed in `Entries` (rename + project; unlisted dropped).
  L2 entries are minimal and self-contained.
- `Passthrough == true` (L1 write):
  listed fields are renamed, and UNLISTED source fields pass through verbatim.
  This preserves `@key` fields not in `ProvidesData`, plus fields accumulated by sibling fetches across the request.

Collision rule (important for correctness):
rename wins.
If an `OutputKey` was emitted and a same-named source field also exists, the source field is dropped to avoid a duplicate JSON key.
If the rename could NOT emit (source missing the `InputKey`), the slot is not claimed and the source field passes through.

Plan/fill alignment hazard (silent corruption):
`StructuralCopyWithTransform` internally runs a counting pass and a fill pass that MUST make byte-for-byte identical structural decisions, including the passthrough field-skip and the de-duplication of duplicate source keys under `Passthrough`.
The exact pinned commit `f600d161463f` IS the fix for a misalignment bug here (duplicate keys under `Passthrough` panicking during marshal).
A clean re-implementation must port that dedupe logic and the consumption guard, and carry its regression tests.
See [06-TEST-AND-BENCH-PLAN.md](06-TEST-AND-BENCH-PLAN.md).

#### `DeepCopy(a arena.Arena, v *Value) *Value`

Clones EVERYTHING, including scalar string/number payloads, onto the destination.
The result shares NO mutable memory with `v`.

- `a != nil` -> arena-allocated copy.
- `a == nil` -> heap-allocated, always independent.
- Immutable singletons (`valueTrue`/`valueFalse`/`valueNull`) are shared, not cloned.
- `nil` in -> `nil` out.

Purpose: safely move a value across a heap<->arena boundary.
Without it, an arena (which is `noscan`) could hold the only reference to a heap value and the GC would collect it -> use-after-free.

The foundation uses `DeepCopy` in EXACTLY ONE place:
`context.go:398` — `astjson.DeepCopy(nil, c.Variables)` — heap-mode deep copy to isolate per-request variables.
Verified present in the worktree.
The `DeepCopyWithTransform` variant exists in astjson but is NOT used by the foundation.

#### `MergeValues` / `MergeValuesWithPath` — BREAKING signature change

New form (what the foundation compiles against):

```go
func MergeValues(ar arena.Arena, a, b *Value) (*Value, error)
func MergeValuesWithPath(ar arena.Arena, a, b *Value, path ...string) (*Value, error)
```

Previous form (released `v1.0.0` / `v1.1.0`):
`(*Value, changed bool, error)` — the `changed bool` return was DROPPED.

These functions are pre-existing (not new in PR #16), but the dropped return value means the foundation will NOT compile against `astjson v1.0.0` / `v1.1.0`.
The foundation calls them ~12 times in `loader.go` (verified) to fold cached + fetched data into the response tree.

Two semantics matter for caching:

- `MergeValues` ALIASES nested container nodes from `b` into `a`.
  This is precisely why `StructuralCopy` must isolate a cache value BEFORE merging it — otherwise the longer-lived response tree (`a`) ends up aliasing cache-owned containers (`b`).
- `MergeValues` is NON-ATOMIC on failure: a partial mutation can corrupt the destination.
  This is why the L1 merge-into-existing path uses working-copy-and-swap (StructuralCopy the live entry, merge into the copy, store the copy on success or the fresh incoming value on failure) — never mutate a live cache entry in place.

#### value constructors + `MarshalTo`

Pre-existing, stable API surface:
`ObjectValue` / `ArrayValue` / `StringValue` / `StringValueBytes` / `NumberValue` / `IntValue` / `FloatValue` / `TrueValue` / `FalseValue(a arena.Arena)`, plus the `NullValue` global const, plus `func (v *Value) MarshalTo([]byte) []byte`.

The foundation builds cache-key objects with the constructors and serializes L2 entries to heap bytes with `MarshalTo` (verified: `loader_cache.go` uses plain `MarshalTo`, NOT a `WithTransform` variant — the value is structurally normalized FIRST, then marshaled).

#### Two-pass arena parser (internal — not called by the foundation)

`ParseWithArena` / `ParseBytesWithArena` are UNCHANGED public entry points.
When `arena != nil` they now run two internal passes (`parseArenaTwoPass`, `planArenaParse`):
Pass 1 plans (validates, records exact totals and per-container counts, allocates no tree nodes); Pass 2 fills (one exactly-sized arena slab per node kind, bump-pointer fill, strings sliced from recorded spans).
Heap mode (`a == nil`) stays single-pass.
`true`/`false`/`null` produce shared singletons consuming no slab slot.
`finish()` verifies every slab/plan list was consumed exactly and returns a BUG error otherwise.

Why it is in scope here even though the foundation never names it:
`StructuralCopy` REUSES the same plan+fill slab architecture, so the two are coupled in the same source file and the same PR.
A minimal extraction cannot ship `StructuralCopy` without the shared machinery.

---

## 2.5 The single integration seam in graphql-go-tools

All transform-driven copies are funneled through one file:
`v2/pkg/engine/resolve/loader_cache_transform.go`.
Four Loader helpers wrap the two Parser copy methods (all verified present in the worktree):

| Helper | Path | Transform config |
|---|---|---|
| `structuralCopyNormalized` | L2 write | rename alias->schema + arg-hash, `Passthrough=false`, project to `ProvidesData` |
| `structuralCopyDenormalized` | L2 read | schema->alias, project |
| `structuralCopyNormalizedPassthrough` | L1 write | rename but keep all fields, `Passthrough=true` |
| `structuralCopyDenormalizedPassthrough` | L1 read | restore alias, keep all, `Passthrough=true` |

`buildNormalizeTransform` / `buildDenormalizeTransform` construct ephemeral `*Transform` trees on reusable `l.transformEntries []astjson.TransformEntry` and `l.transforms []astjson.Transform` slabs.

Other consuming sites:

- `loader.go` — `StructuralCopy` at every cache<->response-tree merge boundary (entity splice into batch arrays, full/partial L1 hit merge, working-copy-and-swap); plus the ~12 `MergeValues` / `MergeValuesWithPath` call sites.
- `loader_cache.go` — L2 write path: structural-normalize then `value.MarshalTo(nil)` to produce heap bytes for the external cache; working-copy-and-swap L1 merge-into-existing.
- `context.go:398` — the sole `DeepCopy(nil, ...)` site.

Architecture detail of these seams: [01-ARCHITECTURE-SPEC.md](01-ARCHITECTURE-SPEC.md).

---

## 3. Release status — the crux

The released astjson tags are `v1.0.0` and `v1.1.0` (latest, 2026-02-20).
The three API-bearing source files — `parser_deep_copy.go`, `parser_arena_twopass.go`, `transform.go` — DO NOT EXIST at the `v1.1.0` tag (404 on the tag).

Both `graphql-go-tools` modules pin an UNRELEASED pseudo-version (verified in the worktree):

- `v2/go.mod:31` -> `github.com/wundergraph/astjson v1.1.1-0.20260419105127-f600d161463f`
- `execution/go.mod:18` -> same pseudo-version.

That pseudo-version resolves to commit `f600d161463f` dated 2026-04-19, which lives on the OPEN PR #16 branch `feat/two-pass-parser`.
`master` of graphql-go-tools pins `astjson v1.0.0`.

Consequence:
the entity-caching foundation cannot be reviewed or merged against `master`'s `astjson v1.0.0`.
It depends on code that exists only in an open upstream PR, referenced by commit hash.
This is the single highest-priority risk in the whole project.

`go-arena` is consistent: PR #16 bumps `go-arena` to `v1.2.0` and both graphql-go-tools modules already pin `go-arena v1.2.0` (verified) — no coordinated bump needed there, but confirm it stays aligned when the astjson tag is cut.

---

## 4. Therefore: "land astjson primitives" is PR #0

Because the required APIs are unreleased, the FIRST step of the stacked plan — before any caching code can be reviewed or merged on its own dependency graph — is to land and release the astjson primitives upstream, then bump graphql-go-tools to that real tag.

PR #0 must deliver, in `wundergraph/astjson`:

1. `StructuralCopy` + `StructuralCopyWithTransform` as `Parser` methods (and package-level funcs).
2. `Transform` / `TransformEntry` with `Entries` / `ArrayItem` / `Passthrough` (and NO `Forced`).
3. Package-level `DeepCopy` (heap-mode behavior required).
4. The 2-return `MergeValues` / `MergeValuesWithPath`.
5. Value constructors + `MarshalTo` (already present; confirm stable).
6. The two-pass arena parser, including the `f600d161463f` Passthrough-dedupe fix and the `finish()` BUG-check consumption guard, with regression tests.

PR #0 may DROP, to shrink the reviewable surface (none are transitively required by the kept pieces — confirm via the open question in section 7):

- `DeepCopyWithTransform`, `CoerceToString`, and any escape-flag / coerce / benchmark churn that is not needed by `StructuralCopy` / `DeepCopy` / `Transform` / two-pass.

Sequencing in the broader plan lives in [03-PR-PLAN-graphql-go-tools.md](03-PR-PLAN-graphql-go-tools.md); the foundation ADR records the decision in [adr/0001-foundation.md](adr/0001-foundation.md).

---

## 5. Recommended dependency strategy (concrete)

Three options, in order of preference.

### Option A — Land astjson PR #16, cut a real tag, bump (RECOMMENDED)

1. Review and merge the astjson primitives upstream (the PR #0 content from section 4).
2. Cut a real, immutable astjson tag.
   This is a minor/major bump, NOT a patch — the `MergeValues` `changed bool` removal is a breaking change for any external consumer on the 3-return form.
3. Bump `astjson` in both `v2/go.mod` and `execution/go.mod` from the pseudo-version `v1.1.1-0.20260419105127-f600d161463f` to the new real tag.
4. Confirm `go-arena v1.2.0` stays aligned across the workspace.

Why preferred:
clean, reviewable, reproducible; no pseudo-versions or `replace` directives in merged `master`; other astjson consumers migrate intentionally at the tag boundary.

### Option B — Pin the pseudo-version (the CURRENT state — acceptable only as interim)

The repo already does this.
It compiles and resolves to a real commit, so development can proceed.
But a pseudo-version pointing at an OPEN PR branch must NOT be the state in which the foundation merges to `master`:
the upstream branch can be force-pushed, rebased, or closed, breaking the build retroactively.
Treat this strictly as the pre-PR-#0 development state and gate merge of the foundation on Option A completing.

### Option C — `replace` directive to a local/fork checkout (development only)

Use a `go.mod` `replace` pointing at a local astjson checkout while iterating on both repos simultaneously.
Never merge a `replace` to `master`.
Useful only when actively co-developing the astjson primitives and the caching foundation in the same session; remove before review.

### Decision

Adopt Option A as the merge gate.
Option B is the legitimate development-time state (and is what the worktree is in today).
Option C is for local co-development only.
Do not merge the caching foundation while astjson is referenced by pseudo-version or `replace`.

---

## 6. Safety invariants checklist (carry into review)

- Every cache WRITE `StructuralCopy`s INTO the cache.
- Every cache READ `StructuralCopy`s OUT of the cache before use.
- Every merge-into-response-tree `StructuralCopy`s the read value BEFORE `MergeValues` (because `MergeValues` aliases `b`'s containers into `a`).
- `StructuralCopy`'d values NEVER outlive their source arena.
  L2 hand-off to an external store goes through `MarshalTo` to heap bytes first.
- L1 merge-into-existing uses working-copy-and-swap, never in-place mutation (because `MergeValues` is non-atomic on failure).
- `Transform` `OutputKey` strings are arena-copied, not aliased (GC safety on `noscan` arena memory).
- Two-pass plan and fill, and `StructuralCopyWithTransform` count and fill, make byte-identical structural decisions (port the `f600d161463f` dedupe + the `finish()` BUG check; carry the regression tests).

---

## 7. Open questions (resolve before/during PR #0)

1. Cut the astjson release from PR #16 as-is (DeepCopy + StructuralCopy + two-pass + Transform + the 2-return MergeValues together), or split further (two-pass parser as one release, copy APIs as another)?
   The foundation needs StructuralCopy + Transform + 2-return MergeValues; the two-pass parser is coupled via shared slab code but is conceptually separable.
2. Is the `MergeValues` `changed bool` removal actually required by the foundation, or incidental cleanup?
   A grep shows all ~12 foundation call sites use the 2-tuple form, so none NEED the dropped `changed` value.
   If kept backward-compatible, the astjson release could be a non-breaking minor and other consumers would not be forced to migrate.
   Confirm and decide whether to keep the breaking change or restore compatibility.
3. Can the minimal extraction drop `DeepCopyWithTransform`, `CoerceToString`, and the escape-flag / coerce / benchmark churn, keeping only StructuralCopy(+WithTransform), DeepCopy, Transform/TransformEntry, the two-pass parser, and the MergeValues signature?
   Confirm none of the dropped pieces are transitively required by the kept ones.
4. Does `go-arena` need a coordinated bump?
   PR #16's release notes bump `go-arena` to `v1.2.0`; graphql-go-tools already pins `v1.2.0`.
   Confirm the astjson release and graphql-go-tools agree on the `go-arena` version to avoid a workspace mismatch.
