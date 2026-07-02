# RFC: Client-controlled error behavior (`onError`) in graphql-go-tools

- Status: Draft
- Author: (RFC drafted 2026-07-01)
- Branch: `on-error-spec-update`
- Tracking spec: [graphql/graphql-spec#1163 — Service capabilities / error behaviors](https://github.com/graphql/graphql-spec/pull/1163) (open, last updated 2026-05)
- Reference implementation: [graphql/graphql-js#4364 — Implement onError proposal](https://github.com/graphql/graphql-js/pull/4364) (draft)

## 1. Summary

The GraphQL spec is gaining a client-controlled error-handling mechanism.
A client can send an optional `onError` request attribute with the value `NULL`, `PROPAGATE`, or `HALT` to choose how execution errors interact with non-null positions in the response.
The long-term goal of the working group is to decouple nullability from errors ("true nullability"): a non-null (`!`) marker should mean "this value is never semantically null", not "this value never errors".

This RFC proposes how to implement the three error behaviors in graphql-go-tools, how to plumb the selected behavior from the caller (the router) into the resolver, and how to advertise support via the new introspection Service Capabilities surface.

## 2. Background: the spec change

### 2.1 The three error behaviors

The spec defines three values for the request `onError` attribute (see the "Handling Execution Errors" section of PR #1163).
When a response position raises an *execution error*:

- **`PROPAGATE`** — today's behavior.
  The execution error propagates to the parent response position (the parent list item, in the case of a list position).
  The parent resolves to `null` if the schema allows it, otherwise the error propagates further up until it reaches a nullable position or the root, in which case `data` becomes `null`.
  The error is added to the `errors` list.

- **`NULL`** — the errored response position is set to `null`, *even if the schema marks it non-nullable*.
  There is no propagation.
  The execution error is still added to the `errors` list.
  The client is responsible for reconciling the `null` value against the `errors` list to distinguish an error result from an intentional `null`.

- **`HALT`** — execution is aborted immediately.
  `data` is set to `null` and the response contains only that single execution error.
  Intended for ad-hoc clients that discard the whole response on any error, and to let the server stop doing unnecessary work.

If `onError` is provided but its value is not one of `NULL`, `PROPAGATE`, or `HALT`, a *request error* must be raised (the whole request fails).

### 2.2 Defaults and discovery (Service Capabilities)

- The default behavior when `onError` is absent is **`PROPAGATE`**, for backwards compatibility.
  `NULL` is the recommended default for new services.
- Support is discovered through a new introspection surface: **Service Capabilities**.
  A service exposes a list of `__Capability` entries (each with `identifier`, `description`, `value`).
  Two capabilities are relevant:
  - `graphql.onError` — the service accepts the `onError` request attribute.
  - `graphql.defaultErrorBehavior` — its `value` names the service's default error behavior; absent means `PROPAGATE`.
- If a client sees no `graphql.onError` capability (or introspection has no capabilities at all), it must not send `onError` and must assume `PROPAGATE`.

### 2.3 Proposal churn (why we pin to #1163)

This area has moved a lot.
Earlier drafts (#1050, #1145, #1153) and the graphql-js PR at various points used an SDL directive `@behavior(onError: __ErrorBehavior! = PROPAGATE)` and an introspection field `__Schema.defaultErrorBehavior`, and even echoed `onError` back in the response.
PR #1163 supersedes those: it drops the response echo and the `@behavior` directive in favor of the generic Service Capabilities mechanism, and keeps the request attribute values as `NULL` / `PROPAGATE` / `HALT`.
Because the spec is still **open and unmerged**, this RFC treats the request-attribute semantics (Section 2.1) as stable enough to build on, and treats the introspection/capabilities surface (Section 2.2) as the most likely-to-change part — hence it is the last, optional phase.

## 3. Current behavior in graphql-go-tools

All error/null-propagation logic lives in the resolver, in `v2/pkg/engine/resolve/resolvable.go`.
The resolver runs the response tree in **two passes** over the same `astjson` value tree (`Resolvable.Resolve`, `resolvable.go:229`):

1. A **validation pass** (`r.print == false`): `walkObject`/`walkArray`/leaf walkers validate values, append errors, and — crucially — mutate the data tree to `null` where propagation applies.
2. A **print pass** (`r.print == true`): the same walkers re-walk the (now-mutated) tree and emit JSON (`printData`, `resolvable.go:308`).

Propagation is driven by a single boolean return value from `walkNode`: `true` means "an error occurred here, bubble up"; `false` means "handled, stop".
`r.err()` (`resolvable.go:294`) simply returns `true`.

The decision points that implement propagation today:

- **Object field** (`walkObject`, `resolvable.go:691`):
  - Null value in a non-null object → `addNonNullableFieldError` + `return r.err()` (`resolvable.go:697-703`).
  - A child field bubbled up (`err == true`) → if `obj.Nullable` and it has a path, set the object to `null` and stop (`SetNull`, `resolvable.go:801-808`); otherwise keep bubbling.
- **List** (`walkArray`, `resolvable.go:942`):
  - Null in a non-null list → `addNonNullableFieldError` + `err()` (`resolvable.go:945-951`).
  - An item bubbled up → if the item is a nullable object, null just that item and continue; else if the list is nullable, null the whole list and stop; else keep bubbling (`resolvable.go:1005-1015`).
- **Leaf walkers** (`walkString` `:1072`, `walkBoolean` `:1118`, `walkInteger`, `walkFloat`, `walkBigInt`, `walkScalar`, `walkEnum` `:~1300`):
  - Null in a non-null leaf → `addNonNullableFieldError` + `err()`.
- **Root** (`Resolve`, `resolvable.go:254-271`):
  - If the root walk returns `true`, `data` is written as `null`.

The synthetic error message for a null in a non-null position is `Cannot return null for non-nullable field '<path>'.` (`addNonNullableFieldError`, `resolvable.go:1372`).

Nullability is represented by a `Nullable bool` on each resolve node: `Object` (`node_object.go:9`), `Array` (`node_array.go:11`), and all scalar/enum leaf nodes.

Error collection: errors are accumulated into the `r.errors` astjson array via `fastjsonext.AppendError*` helpers; the response is assembled in `Resolve` / `writeGraphqlResponse` (`response.go:77`).
Note this is **separate** from *subgraph* error propagation, which is configured by the existing `SubgraphErrorPropagationMode` (`resolve.go:114`, values `Wrapped` / `PassThrough`) and governs how errors *reported by subgraphs* are surfaced — an orthogonal concern (see Section 5.6).

There is currently **no** notion of `onError`, `errorBehavior`, or client-selectable propagation (confirmed by search).

## 4. Goals / Non-goals

**Goals**

- Implement all three error behaviors (`PROPAGATE`, `NULL`, `HALT`) in the resolver.
- Provide a typed, validated way for the caller (the router) to select the behavior per request, defaulting to `PROPAGATE` (no behavior change for existing callers).
- Advertise support and the default via the introspection Service Capabilities surface.
- Keep the change surgical and localized to the resolve layer; preserve existing behavior exactly when the behavior is `PROPAGATE`.

**Non-goals**

- Parsing the HTTP GraphQL request body. graphql-go-tools does not own HTTP request decoding; the router extracts `onError` from the request and passes it into the engine. This RFC provides the enum type + validation helper the router calls.
- Semantic-non-null (`@semanticNonNull` / the `*` type marker, graphql-spec#1065). Related but a separate feature.
- Changing subgraph-level error propagation (`SubgraphErrorPropagationMode`).

## 5. Proposed design

### 5.1 The `ErrorBehavior` type

Introduce a typed enum in `pkg/engine/resolve`:

```go
type ErrorBehavior string

const (
    ErrorBehaviorPropagate ErrorBehavior = "PROPAGATE" // default; current behavior
    ErrorBehaviorNull      ErrorBehavior = "NULL"      // set errored position to null, no propagation
    ErrorBehaviorHalt      ErrorBehavior = "HALT"      // abort on first error, data: null
)

// MapErrorBehavior validates and normalizes an incoming value.
// The empty string maps to the default (PROPAGATE). Any other unknown value
// returns ok=false so the caller can raise a request error.
func MapErrorBehavior(s string) (ErrorBehavior, bool)
```

`MapErrorBehavior` gives the router a single place to validate the request attribute and to decide whether to raise a request error on an invalid value (per Section 2.1).
The zero value (empty string) deliberately means "default / PROPAGATE" so that every existing caller is unaffected.

### 5.2 Where the behavior is configured

Add a field to `ExecutionOptions` (`context.go:113`), which already carries per-request execution toggles and is threaded through `Context` into the `Resolvable`:

```go
type ExecutionOptions struct {
    // ...existing fields...

    // ErrorBehavior selects how execution errors interact with non-null
    // positions. The empty value is treated as PROPAGATE (spec default).
    ErrorBehavior ErrorBehavior
}
```

The `Resolvable` reads `r.ctx.ExecutionOptions.ErrorBehavior` at the start of `Resolve` and stores it on the struct (e.g. `r.errorBehavior`), so the hot walk loop does one field read rather than repeatedly reaching through the context.
Default resolution: an empty value is normalized to `ErrorBehaviorPropagate` in `Resolve`/`Init` (the engine's ultimate safety-net default).

**Operator-defined default (effective behavior resolution).**
The behavior applied to a request is resolved by the router as:

```
effective = requestOnError  (if the request explicitly set onError and the feature is enabled)
          ?? operatorDefault (a router config value: PROPAGATE | NULL | HALT)
          ?? PROPAGATE       (spec / engine safety-net default)
```

The operator default is a router-side configuration value of type `ErrorBehavior`.
If the operator does not configure one, it is `PROPAGATE` (no change from today).
The router computes `effective` and sets it on `ExecutionOptions.ErrorBehavior`, so from the engine's point of view that field is always the concrete behavior to apply for this request.
The same operator default value feeds the `graphql.defaultErrorBehavior` introspection capability (Section 5.7), keeping "what the server does by default" and "what introspection reports as the default" identical.

### 5.3 `NULL` behavior in the resolver

Observation: `NULL` behavior is *exactly* "treat every non-null position as if it were nullable, for the purpose of propagation, while still recording the error".
This maps cleanly onto the existing single-boolean propagation model.

Introduce a helper that replaces the raw `return r.err()` at the non-null-null decision points:

```go
// nonNullFieldError records the non-null violation and returns whether the
// error should propagate. Under NULL behavior it never propagates.
func (r *Resolvable) nonNullFieldError(fieldPath []string, parent *astjson.Value) bool {
    r.addNonNullableFieldError(fieldPath, parent)
    if r.errorBehavior == ErrorBehaviorNull {
        // set this position to null in place and stop propagation
        if len(fieldPath) > 0 {
            astjson.SetNull(r.astjsonArena, parent, fieldPath...)
        }
        return false
    }
    return true
}
```

Touch points to convert (all currently `addNonNullableFieldError(...) + return r.err()`):

- `walkObject` `resolvable.go:701-702`
- `walkArray` `resolvable.go:949-950`
- `walkString` `resolvable.go:1079-1080`
- `walkBoolean`, `walkInteger`, `walkFloat`, `walkBigInt`, `walkScalar`, `walkEnum` (same shape)

Additionally, the *bubble-up* decision points must not bubble under `NULL`.
Today, when a child returns `true`, `walkObject`/`walkArray` bubble the null upward if the parent is nullable.
Under `NULL` the child never returns `true` for a nullability violation, so these bubble-up branches (`resolvable.go:801-808`, `1005-1015`) are naturally not exercised for that cause — no change required there, but tests must confirm it.

**Decided — "null with an error" vs "null without an error": uniform behavior (option A).**
Under `NULL`, the resolver always sets the errored/null position to `null` in place and records the "Cannot return null for non-nullable field" error, regardless of whether an upstream execution error already exists for that path.
Rationale: it is simple, keeps the existing error message, and never loses information — the client always receives a `null` plus an error to reconcile, which is exactly the contract of `NULL` mode.
A subgraph that returns `null` for a non-null field with no accompanying error *is* an error, so emitting the synthetic error is correct.

This must be covered by tests for **all** positions and shapes (see Section 7): non-null leaf, non-null object, non-null list, non-null list item, deeply nested non-null chains, and the mix of "null with an upstream error already present" and "null with no upstream error".

### 5.4 `HALT` behavior in the resolver

`HALT` means: as soon as an error is present, produce `data: null`.

**Scope decision: HALT is handled entirely inside `Resolvable`, at render time.**
We do **not** touch the loader and we do **not** cancel or skip subgraph fetches.
All fetches run as normal; HALT only changes how the response is assembled.
This keeps the change fully contained to `resolvable.go` and avoids any concurrency/cancellation complexity in the loader.

Implementation:

- At the top of `Resolve` (after `SkipLoader` handling, around `resolvable.go:252`): if `errorBehavior == HALT && r.hasErrors()`, skip the walk, write `data: null`, and emit a **single** error (see below).
  (Errors from subgraph loading are already present in `r.errors` at this point.)
- If no errors exist yet when `Resolve` begins, run the walk normally.
  Under `HALT` the walk behaves like `PROPAGATE`, so the first null-in-non-null violation bubbles straight to the root and `data` is written as `null` (the existing root-level collapse in `Resolve`, `resolvable.go:263-268`, already does this).
  If the walk itself is the source of the (single) error, that walk error is the one reported.

In effect, HALT = "if there is any error, render `data: null` with a single error", decided at render time in the resolver.

**Decided — HALT reports exactly one error.**
Per the spec, the HALT response contains a single execution error.
Because HALT is render-time and fetches are not cancelled, `r.errors` may hold several errors from parallel subgraph loading.
We trim to a single error: the **first** entry in `r.errors` (index 0).
Concretely, when short-circuiting for HALT, print an `errors` array containing only `r.errors[0]` (or, if the single error originates from the walk, that error).
Note: subgraph errors are appended to `r.errors` as loads complete, so "first" is defined by append order; this is deterministic enough for the single-error contract, and tests assert the exact single-error response.

### 5.5 Interaction with the two-pass walk

Both passes must observe the same behavior, since the validation pass mutates the tree and the print pass renders it.
`r.errorBehavior` is set once before the passes, so this is automatic.
For `NULL`, because the validation pass sets positions to `null` in place (via `SetNull`), the print pass simply renders those nulls — no special print-pass logic needed.
This reuses the exact mechanism the resolver already uses for legitimately-nullable fields.

### 5.6 Relationship to `SubgraphErrorPropagationMode`

`SubgraphErrorPropagationMode` (`resolve.go:114`) controls how errors *reported by subgraphs* are shaped into the client `errors` array (wrapped vs pass-through).
`ErrorBehavior` controls how *execution errors interact with non-null positions* in `data`.
They are orthogonal and compose: e.g. `onError=NULL` with `SubgraphErrorPropagationModePassThrough` yields the subgraph's original error entries plus in-place `null`s.
The RFC introduces no coupling between them; tests should cover the cross-product for at least the common cases.

### 5.7 Introspection: Service Capabilities (required, config-gated)

Support must be advertised so that a client (and the router itself) can discover it via introspection.
Extend the introspection generation in `v2/pkg/introspection` to expose the Service Capabilities surface:

- Add the `__Capability` type (`identifier`, `description`, `value`) to the introspection model (`introspection.go`, `introspection_enum.go`, `converter.go`).
- Wire the `service { capabilities { ... } }` introspection surface per the spec's introspection schema additions.
- Emit two capabilities **when enabled**:
  - `graphql.onError` (no value) — the service accepts the `onError` request attribute.
  - `graphql.defaultErrorBehavior` with `value` = the operator-configured default behavior (Section 5.2); `PROPAGATE` if the operator did not configure one.

Do **not** implement the superseded `@behavior` directive or `__Schema.defaultErrorBehavior` field.

**Configurability is mandatory (see Section 5.8).**
The capabilities must be emitted **only when the feature is enabled**.
When the feature is disabled, introspection must render exactly as it does today — no `service`/`capabilities` surface — so that clients do not attempt to send `onError` to a service that will not honor it.
Concretely, introspection generation must support two outcomes from a single configuration decision:

1. **Enabled** → introspection includes the Service Capabilities with `graphql.onError` and `graphql.defaultErrorBehavior`.
2. **Disabled** → introspection omits the capabilities entirely (current behavior, byte-identical golden output).

Design: thread an optional capabilities descriptor into introspection generation.
Today `Generator.Generate(definition, report, data)` (`generator.go:54`) populates a `Data` value.
Add an optional capabilities input (nil ⇒ omit), for example:

```go
// nil => do not emit any service capabilities (current behavior)
type ServiceCapabilities struct {
    OnError              bool   // emit graphql.onError
    DefaultErrorBehavior string // e.g. "PROPAGATE"; "" => omit graphql.defaultErrorBehavior
}
```

carried on the `Data`/`Schema` model with `json:"...,omitempty"` (or a sibling field to `__schema`) so the marshalled introspection response is unchanged when it is nil.
The caller (router) constructs this descriptor from its own config: when it enables `onError`, it passes a non-nil descriptor; otherwise nil.
This is the single decision point that keeps "engine honors `onError`" and "introspection advertises `onError`" in lockstep.

### 5.8 Configuration and caller (router) responsibilities

The feature is **opt-in and configurable at the router**.
There is a single "enable `onError`" switch on the Cosmo router side that must control **both** halves of the feature together:

- Whether the engine **honors** an incoming `onError` request attribute (otherwise it is ignored and behavior stays `PROPAGATE`).
- Whether **introspection advertises** the Service Capabilities (Section 5.7).

These must never diverge: if the engine honors `onError` but introspection hides it, clients can't discover it; if introspection advertises it but the engine ignores it, clients are misled.
Tying both to one config value is the core requirement from this refinement.

graphql-go-tools provides the primitives to make the router wiring small and type-safe:

- The `ErrorBehavior` enum + `MapErrorBehavior` validation helper (Section 5.1).
- The `ExecutionOptions.ErrorBehavior` field (Section 5.2).
- The optional `ServiceCapabilities` descriptor for introspection generation (Section 5.7).

Router flow when the feature is **enabled**:

1. Read the operator's configured default behavior (`PROPAGATE` if unset).
2. Extract `onError` from the incoming GraphQL request; if present, call `resolve.MapErrorBehavior(value)` and on `ok == false` return a GraphQL *request error* (fail the whole request) and do not execute (spec: invalid value ⇒ request error).
3. Resolve the effective behavior = request `onError` (if present) else operator default (Section 5.2) and set it on `ExecutionOptions.ErrorBehavior`.
4. Serve introspection with a non-nil `ServiceCapabilities`, where `DefaultErrorBehavior` = the operator default — advertising `graphql.onError` + `graphql.defaultErrorBehavior` consistently with (1).

Router flow when the feature is **disabled** (default):

- Ignore any incoming `onError` (do not set `ExecutionOptions.ErrorBehavior`; it stays the `PROPAGATE` default).
- Serve introspection with a nil `ServiceCapabilities` (no capabilities surface — byte-identical to today).

HTTP request-body parsing itself remains the router's job; this RFC ships the enum, validation helper, execution option, and introspection descriptor so the router only wires them to its config switch.

## 6. Phased implementation plan

1. **Phase 1 — core resolver behavior.**
   `ErrorBehavior` type + `MapErrorBehavior`; `ExecutionOptions.ErrorBehavior`; `r.errorBehavior` on `Resolvable`; `NULL` and `HALT` (render-time) semantics at the touch points in Sections 5.3–5.4.
   Verify: `PROPAGATE` output byte-identical to today.
2. **Phase 2 — introspection Service Capabilities (config-gated).**
   `__Capability` type + `service { capabilities }` surface; optional `ServiceCapabilities` descriptor threaded into introspection generation; emit `graphql.onError` + `graphql.defaultErrorBehavior` only when enabled.
   Verify: with the descriptor nil, introspection golden output is byte-identical to today.
3. **Phase 3 — tests & hardening.**
   Resolver unit tests + datasource pipeline tests across the behavior × nullability × list/leaf/object matrix, plus introspection golden tests for both enabled/disabled (Section 7).

## 7. Testing strategy

Tests must cover **all** behaviors at **all** positions (per decision 8.1). Assert the entire response string with `assert.Equal` (not `Contains`), per repo convention.

- **Resolver unit tests** (`pkg/engine/resolve/resolvable_test.go`, `resolve_test.go`): construct response trees with non-null violations and assert the *full* rendered response for each `ErrorBehavior`.
  Full matrix:
  - **Position:** non-null leaf, non-null object, non-null list, non-null list item, deeply nested non-null chain to root.
  - **Behavior:** `PROPAGATE`, `NULL`, `HALT`.
  - **Parent nullability:** nullable parent present vs non-null chain all the way to root (→ `data: null`).
  - **Error source:** null *with* an upstream execution error already present, and null *without* any upstream error (subgraph returned a bare null).
- **`NULL` assertions:** errored position is `null` in place, sibling fields preserved, no bubbling, and the "Cannot return null…" error present (option A).
- **`HALT` assertions:** `data: null` and exactly **one** error in `errors` (decision 8.2) — including the case where multiple subgraph errors were collected, asserting only `r.errors[0]` is emitted.
- **Pipeline / e2e tests** using `RunTest()` (`datasourcetesting`) and the `execution/` module: subgraph returns an error + null for a non-null field; assert `data` retains sibling values under `NULL`, collapses under `PROPAGATE`, and is a single-error `data: null` under `HALT`.
- **Operator-default tests (decision 8.3):** with the operator default set to `NULL` (and `HALT`) and no request `onError`, assert the effective behavior is the default; and assert the introspection `graphql.defaultErrorBehavior` value equals the configured default (the two stay synchronized).
- **Golden/introspection tests:** with the `ServiceCapabilities` descriptor enabled, assert the introspection response includes `graphql.onError` + `graphql.defaultErrorBehavior` (with the configured default value); with it nil, assert the introspection golden output is byte-identical to today.
- **Regression guard:** with the feature disabled (`ErrorBehavior` unset, nil capabilities) both `data` and introspection output are identical to the pre-change golden output.

## 8. Resolved decisions

1. **NULL synthetic errors (Section 5.3): uniform (option A).**
   Under `NULL`, always set the position to `null` in place and record the "Cannot return null…" error, whether or not an upstream error already exists.
   Full test coverage across all positions/shapes is required.
2. **HALT reports a single error (Section 5.4).**
   `data: null` plus exactly one error — `r.errors[0]` (or the walk error if that is the source). HALT stays render-time only; fetches are **not** cancelled.
3. **Operator-configurable default (Section 5.2, 5.7).**
   The default behavior is operator-configurable at the router. Unset ⇒ `PROPAGATE`; the operator may set `NULL` or `HALT`.
   The configured default is used when a request omits `onError`, and the same value is reported via the `graphql.defaultErrorBehavior` introspection capability — the two must stay synchronized.

## 9. Risks & backwards compatibility

- **No behavior change by default.** Feature disabled → unset `ErrorBehavior` → `PROPAGATE` → byte-identical `data`, and nil `ServiceCapabilities` → byte-identical introspection. Both enforced by regression/golden tests.
- **Spec instability.** The request-attribute semantics are stable across recent drafts; the introspection capabilities surface is less settled, so it is implemented but strictly config-gated (off by default) — an operator only opts in when they accept the current shape.
- **HALT is render-time only.** Fetches are not cancelled, so there is no loader/concurrency complexity; HALT emits `data: null` with a single error (`r.errors[0]`), asserted by tests.
- **Client contract.** `NULL` shifts error-vs-null reconciliation to the client; this is by design and matches the working-group direction (`graphql-toe`, Relay `@throwOnFieldError`).

## 10. References

- graphql-spec#1163 — Service capabilities / error behaviors: https://github.com/graphql/graphql-spec/pull/1163
- graphql-js#4364 — Implement onError proposal (reference impl): https://github.com/graphql/graphql-js/pull/4364
- graphql-spec#1050 — Directive proposal for opting out of null bubbling (superseded): https://github.com/graphql/graphql-spec/pull/1050
- graphql-spec#719 — "error propagation considered harmful": https://github.com/graphql/graphql-spec/issues/719
- nullability-wg#85 — Uncoupling nullability and errors: https://github.com/graphql/nullability-wg/discussions/85
- graphql-toe (client-side reconciliation): https://www.npmjs.com/package/graphql-toe

### Key codebase anchors

- `v2/pkg/engine/resolve/resolvable.go` — `Resolve` (`:229`), `walkObject` (`:691`), `walkArray` (`:942`), leaf walkers (`:1072`+), `addNonNullableFieldError` (`:1372`), `err` (`:294`).
- `v2/pkg/engine/resolve/context.go` — `Context` (`:19`), `ExecutionOptions` (`:113`).
- `v2/pkg/engine/resolve/resolve.go` — `SubgraphErrorPropagationMode` (`:114`, orthogonal).
- `v2/pkg/engine/resolve/node_object.go`, `node_array.go`, `node_scalar.go` — `Nullable` fields.
- `v2/pkg/introspection/` — introspection generation (Phase 3).
