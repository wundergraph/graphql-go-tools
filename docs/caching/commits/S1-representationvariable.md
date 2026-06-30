# Commit S1 — Extract the representation-variable builder into a shared package

Plan item: `docs/caching/PLAN.md` §2, S1.
RFC sections: RFC-2 §6.1 (representation-variable builder extraction), CODING_GUIDELINES §3 (refactor in place, never copy-paste).
Phase: 0 (Structure, pure no-op).

## Problem

The `@key` -> representation-node builder that caching needs is unexported and lives in `graphql_datasource`
(`buildRepresentationVariableNode` / `mergeRepresentationVariableNodes` and their internal visitor + merge helpers).
The later caching key-spec freezer must reuse this exact builder.
Copy-pasting it would create two divergent `@key` -> representation walkers — the silent key-skew hazard RFC-2 §6 exists to prevent.

## Solution

Refactor-in-place extraction (not a re-add) into a new shared, exported package.

- New package `v2/pkg/engine/plan/representationvariable` (verified absent before this commit).
- `buildRepresentationVariableNode` -> `BuildRepresentationVariableNode`;
  `mergeRepresentationVariableNodes` -> `MergeRepresentationVariableNodes`;
  the internal `representationVariableVisitor`, `objectFields`, and the merge helpers move with them, unexported in the new package.
- The implementation is moved byte-for-byte; only the package declaration and the two exported identifiers change.
- `graphql_datasource.go` is refactored in place: its two call sites
  (`buildRepresentationsVariable`, at the build and merge points) now call the exported functions.
- No import cycle: `representationvariable` imports `plan`/`resolve`/`ast`/`astvisitor`; `plan` does not import it.

## Key decisions

- Extract, do not re-add: one implementation, two callers (the data source today, the caching freezer later).
- `MergeRepresentationVariableNodes` is retained for the data source's single `representations` variable;
  the freezer will NOT call it — multi-key caching keeps candidates separate (RFC-2 §6.1).
- No backward-compat wrapper functions are kept in `graphql_datasource`; the call sites use the exported names directly.
- No caching code is introduced in this commit; it is a pure, behavior-preserving move.

## Tests

- The existing `graphql_datasource/representation_variable_test.go` is the behavior-preservation guard.
  It is updated only by repointing the call identifiers to the exported package-qualified names;
  every input, expected value, and assertion is unchanged, so it pins the output byte-for-byte.
- New `v2/pkg/engine/plan/representationvariable/representationvariable_test.go` (own external test package)
  table-tests `(definition, one @key set, federation)` and asserts the FULL `*resolve.Object` with `assert.Equal`:
  single `@key`, composite `@key`, nested-object `@key`, interfaceObject, and entityInterface (`__typename` `OnTypeNames` baked in).

Verification (run from `v2/`):

- `go build ./pkg/...` — clean.
- `go test ./pkg/engine/datasource/graphql_datasource/ -run 'TestBuildRepresentationVariableNode|TestMergeRepresentationVariableNodes' -count=1` — pass (9 subtests).
- `go test ./pkg/engine/plan/representationvariable/... -count=1` — pass (5 subtests).
- `go vet ./pkg/engine/plan/representationvariable/... ./pkg/engine/datasource/graphql_datasource/...` — clean.

Note: `go build ./...` fails at the repo-root `package main` (`v2/doc.go` has no `main`), which is pre-existing and unrelated (`doc.go` is untouched by this commit).

## Reviewer guidance

- Confirm zero behavior change via the existing datasource test (assertions untouched, only call identifiers repointed).
- Confirm no new import cycle (the build proves it).
- Confirm both `graphql_datasource.go` call sites now use the exported names.
- Confirm the moved file is identical to the original except the package declaration and the two exported names.
