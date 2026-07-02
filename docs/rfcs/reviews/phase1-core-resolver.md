# Phase 1 Review — Core Resolver Error Behavior

**Scope:** Implements client-selectable GraphQL error behavior (`PROPAGATE` / `NULL` / `HALT`) in the resolver.
Design: [docs/rfcs/2026-07-01-onerror-error-behavior.md](../2026-07-01-onerror-error-behavior.md).
Plan: [docs/rfcs/plans/2026-07-01-onerror-phase1-core-resolver.md](../plans/2026-07-01-onerror-phase1-core-resolver.md).

## What changed

| File | Change |
|---|---|
| `v2/pkg/engine/resolve/error_behavior.go` | New. `ErrorBehavior` enum + `MapErrorBehavior` validator/normalizer. |
| `v2/pkg/engine/resolve/context.go` | New `ExecutionOptions.ErrorBehavior` field (opt-in; empty ⇒ PROPAGATE). |
| `v2/pkg/engine/resolve/resolvable.go` | `errorBehavior` field + reset/normalize; `erroredPosition` and `keepFirstErrorOnly` helpers; NULL/HALT wiring across all walkers. |
| `v2/pkg/engine/resolve/*_test.go` | Enum tests + behavior tests (leaf/object/list-item/type-mismatch NULL, HALT single error, default-is-PROPAGATE). |

## Key decisions

1. **Empty value normalizes to PROPAGATE**, at both `Resolve` and `ResolveNode` entry points, and cleared in `Reset`.
`MapErrorBehavior("")` also returns PROPAGATE so router/gateway callers get the spec default for free.
The operator default is applied by the caller before this layer (RFC §5.2); this layer only sees the already-resolved value.

2. **NULL maps onto the existing single-bool propagation model** via one helper, `erroredPosition(path, parent)`.
Under non-NULL behaviors it returns `r.err()` (unchanged bubbling), so PROPAGATE/HALT paths are byte-identical to before.
Under NULL, on the validation pass it sets the errored position to `null` in place (via `astjson.SetNull`) and returns `false` (no propagation); on the print pass it prints `null` via `walkNull()`.

3. **Error is recorded once.**
Because NULL nulls the position in place (instead of bubbling), the print pass re-descends into the now-null position.
Every error-adding site is therefore guarded with `if !r.print { ... }` so the error is appended only on the validation pass.

4. **HALT is render-time only** (no loader/fetch cancellation, per the RFC refinement).
The walk runs exactly like PROPAGATE; then in `Resolve`, if the behavior is HALT and any error exists, `data` is forced to `null` and `keepFirstErrorOnly` trims the errors array to a single element (RFC §5.4 / decision 8.2).

5. **Array-item nulling reuses the empty-path branch.**
For non-null list items (`Item` has empty `Path`), `erroredPosition([], parent)` does not mutate the tree (nothing to address by path); the print pass renders `null` directly.
Whole-array nulls use `arr.Path` as normal.

## Where to focus review

- **`erroredPosition` (resolvable.go, near `err()`):** the correctness pivot.
Confirm the two-pass invariant: validation pass mutates/records, print pass renders `null` and never re-records.
- **The `if !r.print` guards** on all ~14 converted sites (null-in-non-null for 10 walkers + coercion/invalid-enum sites in Task 4).
A missing guard would double-record an error under NULL.
- **`ResolveNode` nil-`ctx` guard (resolvable.go:~217):** `ResolveNode` is reused to render a variable-that-is-a-node with `r.ctx == nil`.
The normalization must not dereference a nil ctx — this was caught by `TestAuthorization` and fixed to default PROPAGATE.
- **HALT wiring in `Resolve` (after `walkObject`):** ordering relative to `authorizationError` and the existing `hasErrors`/`printErrors` branches.
- **Regression surface:** the entire `resolve` package and `./pkg/engine/...` pass unchanged, which is the guarantee that PROPAGATE output is untouched.

## Test / verification

- `cd v2 && go test ./pkg/engine/resolve/` — green (incl. new behavior tests).
- `cd v2 && go test ./pkg/engine/...` — green.
- `go vet ./pkg/engine/resolve/` — clean.

## Out of scope for Phase 1

- Introspection service capabilities → Phase 2.
- Full behavior×position×nullability matrix + e2e with an erroring subgraph → Phase 3.
- Authorization-denial null-setting keeps its existing behavior (unchanged).
