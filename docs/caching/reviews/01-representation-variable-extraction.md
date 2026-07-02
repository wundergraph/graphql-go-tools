# Reviewer notes — task 01: representationvariable extraction

Commit: `ca0ec6fb`.
Task file: [tasks/01-representation-variable-extraction.md](../tasks/01-representation-variable-extraction.md).

## What this commit adds

A new exported package `v2/pkg/engine/plan/representationvariable` containing the `@key` → representation-node builder that was previously unexported in `graphql_datasource`.
Cache-key construction (task 06) will reuse it, so there is exactly ONE `@key`→representation walker in the codebase and read/write cache keys can never skew against the datasource's own representations.

## Decisions made

- Pure move: the visitor, the merge helpers, and both entry functions moved verbatim; only the two entry points were renamed to exported form (`BuildRepresentationVariableNode`, `MergeRepresentationVariableNodes`).
- The merge helpers (`mergeFields`, `mergeObjects`, `mergeArrays`, `fieldsHasField`, `isOnTypeEqual`) stay unexported inside the new package; nothing else in `graphql_datasource` used them (verified by grep).
- Doc comments were added to the package and the two exported functions (required by CODING_GUIDELINES §8); no logic lines changed.
- The pre-existing `// TODO: add support for remapping path` comment and the `unusedparams` hint on `resolveOnTypeNames(fieldRef …)` were left as-is — carrying them over unchanged keeps the diff a relocation.

## What was implemented

- `v2/pkg/engine/plan/representationvariable/representation_variable.go` — the moved code.
- `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource.go` — both call sites (`buildRepresentationsVariable`: build + merge) now call the exported names; import added.
- Old `representation_variable.go` / `_test.go` deleted from `graphql_datasource`; tests moved with the code.
- One NEW test case: `TestBuildRepresentationVariableNode/with entity interface` — the `entityInterfaceTypeName` → `OnTypeNames` remap branch was previously uncovered (only the interfaceObject branch had a test).

## What to look into (review focus)

- Confirm the diff reads as relocation + rename, not rewrite: git detected the renames at 89% (impl; the delta is doc comments) and 92% (tests; the delta is the new entity-interface case).
- Confirm no import cycle: `representationvariable` imports `plan`; `plan` does NOT import `representationvariable` (only `graphql_datasource` does).
- Confirm `MergeRepresentationVariableNodes` is exported for the datasource only; the task 06 `cacheKeyBuilder` must NOT call it (multi-key keeps candidates separate).

## Verification evidence

- `go test ./pkg/engine/plan/representationvariable/...` — ok.
- `go test ./pkg/engine/datasource/graphql_datasource/...` and `./pkg/engine/plan/...` — ok (behavior-preservation guard).
- Full `v2` suite: 40 packages ok, exit 0; `execution` module: 5 packages ok, exit 0.
- `golangci-lint` (v2.5.0, repo config minus `modernize`, which the local binary lacks): 0 issues; `gci`/`gofmt` clean.
