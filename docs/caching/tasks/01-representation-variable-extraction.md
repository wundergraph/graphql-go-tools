# Task 01 — Extract the representation-variable builder into a shared package

Phase: 0 (Structure).
Dependencies: none.
References: RFC-2 §6.1; CODING_GUIDELINES §3.

## Problem

The `@key` → representation-node builder that cache-key construction needs is unexported and lives in `graphql_datasource`.
Copy-pasting it would create two divergent `@key`→representation walkers — the silent key-skew hazard the whole key model exists to prevent.

## Scope

Extract, do not re-add; no caching code yet.

- New exported package `v2/pkg/engine/plan/representationvariable`.
- Move `buildRepresentationVariableNode` → `BuildRepresentationVariableNode` and `mergeRepresentationVariableNodes` → `MergeRepresentationVariableNodes` (from `v2/pkg/engine/datasource/graphql_datasource/representation_variable.go`, plus the internal `representationVariableVisitor` and merge helpers).
- Refactor `graphql_datasource` IN PLACE to call the exported functions at its two call sites (`graphql_datasource.go`, build + merge).
- MOVE the tests too: the representation-variable tests move into the new package (per-package test ownership); the datasource-level behavior guard stays where it exercises the call sites.

## Implementation outline

- Pure move + re-export + two-call-site rewrite.
- No import cycle: `representationvariable` imports `plan`/`resolve`/`ast`/`astvisitor`; `plan` does not import it.
- Signatures:
  - `BuildRepresentationVariableNode(definition *ast.Document, cfg plan.FederationFieldConfiguration, federationCfg plan.FederationMetaData) (*resolve.Object, error)` — one `@key` selection set → one federation-pointer-free `*resolve.Object` with the interfaceObject/entityInterface `__typename` remap baked in.
  - `MergeRepresentationVariableNodes(objects []*resolve.Object) *resolve.Object` — used by the datasource for its single `representations` variable; the caching `cacheKeyBuilder` (task 06) will NOT call it (multi-key keeps candidates separate).

## Tests

- The EXISTING `graphql_datasource` representation-variable behavior must still pass byte-for-byte (behavior-preservation guard).
- New `representationvariable` package tests (moved + extended): table tests over `(definition, one @key set, federation)` asserting the FULL `*resolve.Object` node with `assert.Equal` — single `@key`, composite, nested-object, interfaceObject/entityInterface `__typename` baked in.

## Acceptance criteria

- [ ] Zero behavior change: the full existing datasource suite passes unmodified.
- [ ] Both call sites use the exported names; no unexported copy remains in `graphql_datasource`.
- [ ] No new import cycle.
- [ ] Tests moved with the code, not left behind.
- [ ] Lint-clean in `v2`.

## Reviewer guidance

- Confirm this is a pure move (diff should read as relocation + rename, not rewrite).
- Confirm the merge helper is exported but unused by caching (it exists for the datasource only).
