# Phase 3 Review — Tests & Hardening

**Scope:** Prove Phases 1–2 correct across the behavior × position × nullability matrix, that HALT returns exactly one error, that the operator default is applied and synchronized with introspection, that a real erroring subgraph behaves correctly under each behavior end-to-end, and that the feature-off path is byte-identical.
Plan: [docs/rfcs/plans/2026-07-01-onerror-phase3-tests-hardening.md](../plans/2026-07-01-onerror-phase3-tests-hardening.md).

## What changed

| File | Change |
|---|---|
| `v2/pkg/engine/resolve/error_behavior_matrix_test.go` | New. Full behavior×position×nullability matrix, HALT single-error, unset==PROPAGATE regression guard. |
| `v2/pkg/engine/resolve/error_behavior_test.go` | `TestErrorBehavior_OperatorDefaultApplied`. |
| `v2/pkg/introspection/introspection_capabilities_test.go` | `TestCapabilities_DefaultSyncsWithConfig`. |
| `execution/engine/execution_engine.go` | New `WithErrorBehavior` execution option (the only non-test change). |
| `execution/engine/error_behavior_e2e_test.go` | New. End-to-end test with an erroring subgraph under PROPAGATE / NULL / HALT. |

## Key decisions

1. **The only production change is `WithErrorBehavior`** in the execution engine — a one-line option mirroring `WithRequestTraceOptions` that sets `resolveContext.ExecutionOptions.ErrorBehavior`.
Everything else in Phase 3 is tests.

2. **Expected strings are pinned to observed output, never `Contains`.**
The e2e revealed two truthful details I pinned rather than papered over:
   - The raw subgraph error (`boom`) is replaced by `"Failed to fetch from Subgraph 'id'."` — the **default** subgraph-error propagation mode wraps upstream errors. This is orthogonal to onError (RFC §5.6); the test documents it.
   - Under PROPAGATE/NULL the response carries **two** errors (the wrapped fetch error + the resolver's non-null violation); HALT trims both to the single first error. `data` serializes before `errors` in the engine's writer.

3. **The e2e is the strongest proof of the whole stack.** It exercises real planning + fetching + resolving with `ExecutionOptions.ErrorBehavior`, confirming:
   - `NULL` → `{"data":{"hero":{"id":"1","name":null}},…}` (sibling `id` preserved, non-null `name` nulled in place).
   - `HALT` → `{"data":null,"errors":[<one>]}`.
   - `PROPAGATE` → `{"data":{"hero":null},…}` (bubbles to the nullable `hero`).

4. **Operator-default synchronization is asserted from both sides.**
`TestErrorBehavior_OperatorDefaultApplied` models the router's `effective = request ?? operatorDefault ?? PROPAGATE` resolution; `TestCapabilities_DefaultSyncsWithConfig` asserts `graphql.defaultErrorBehavior` echoes the same configured value for all three behaviors (RFC §5.8 / decision 8.3).

## Where to focus review

- **`error_behavior_matrix_test.go` expected strings:** these are the behavioral contract. Each is a full-string `assert.Equal`; confirm the NULL rows keep sibling/other data and the PROPAGATE rows collapse per nullability.
- **`WithErrorBehavior`:** trivial, but confirm it targets `resolveContext.ExecutionOptions.ErrorBehavior` (not a copy).
- **e2e wrapped-error nuance:** confirm reviewers are comfortable that the raw subgraph error is wrapped by the default propagation mode; if a passthrough demonstration is preferred, the datasource can be reconfigured — the onError behavior itself is unaffected.
- **`TestErrorBehavior_UnsetEqualsPropagate`:** the byte-identical guarantee — unset behavior must equal explicit PROPAGATE.

## Test / verification

- `cd v2 && go test ./pkg/engine/resolve/ ./pkg/introspection/ ./pkg/engine/datasource/introspection_datasource/ ./pkg/asttransform/` — green.
- `cd v2 && go test ./pkg/engine/...` — green.
- `cd execution && go test ./engine/` — green (incl. the new e2e).

## Coverage vs plan

- Task 1 (matrix), Task 2 (HALT single error), Task 3 (operator default + introspection sync), Task 4 (e2e erroring subgraph), Task 5 (feature-off regression guard) — all delivered.
- Combined with Phase 2's execution-layer note, the full `{ __schema { capabilities } }` **execution** assertion could be added here later; Phase 2 already proves that path's three layers directly.
