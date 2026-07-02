# Phase 2 Review — Introspection Service Capabilities

**Scope:** Advertise onError support via GraphQL Service Capabilities introspection (`graphql.onError`, `graphql.defaultErrorBehavior`), **config-gated** so introspection is byte-identical to today when the feature is disabled.
Design: [docs/rfcs/2026-07-01-onerror-error-behavior.md](../2026-07-01-onerror-error-behavior.md) §5.7.
Plan: [docs/rfcs/plans/2026-07-01-onerror-phase2-introspection.md](../plans/2026-07-01-onerror-phase2-introspection.md).

## What changed

| File | Change |
|---|---|
| `v2/pkg/introspection/introspection.go` | `Capability` type; optional `Schema.Capabilities` (`omitempty`); `BuildServiceCapabilities(enabled, default)`; `Schema.AddCapabilityType()` (idempotent). |
| `v2/pkg/asttransform/baseschema.go` | `MergeOptions{ServiceCapabilities}`; `MergeDefinitionWithBaseSchemaOptions`; `baseSchemaServiceCapabilities` (`type __Capability`); `addCapabilitiesField` adds `__Schema.capabilities: [__Capability!]`. |
| `v2/pkg/engine/datasource/introspection_datasource/config_factory.go` | `IntrospectionOptions`; `NewIntrospectionConfigFactoryWithOptions`; populates `Schema.Capabilities`; registers `capabilities` / `__Capability` child nodes when enabled. |
| `*_capabilities_test.go`, `capabilities_test.go` | Model marshaling, builder, type registration, SDL gating, Load serialization, child-node registration. |

## Key decisions

1. **Gated/additive, not a global schema change** (per your selected approach).
`MergeDefinitionWithBaseSchema` and `NewIntrospectionConfigFactory` are unchanged and delegate to the new `…Options` variants with the feature off, so every existing caller is byte-identical.
Only the new opt-in paths add the `__Capability` type, the `__Schema.capabilities` field, the child-node registrations, and the runtime capability values.

2. **`capabilities` is a nullable list `[__Capability!]`, not `[__Capability!]!`.**
This is deliberate: a server with the feature enabled but advertising nothing (or the `omitempty` disabled path) yields `null`, which must not trip a non-null violation.
Individual capability entries are non-null.

3. **The `__Schema.capabilities` field is added programmatically** (`addCapabilitiesField`), not via `extend type __Schema` in SDL.
The base-schema merge flow here does not merge object-type extensions (its `ExtendSchema` only injects `__typename`), so `extend` would silently not attach the field.
The `type __Capability` itself is plain SDL appended only when enabled.

4. **Two independent registrations must both be gated together.**
For `__schema { capabilities }` to plan and resolve, three things must line up: (a) the SDL declares the field+type (asttransform), (b) the datasource declares `capabilities`/`__Capability` as child nodes (config factory), (c) the runtime `Schema.Capabilities` values are populated (config factory).
All three are driven by the same `ServiceCapabilities` flag.

5. **`source.go` unchanged.**
It already marshals the whole `Schema`, so once `Schema.Capabilities` is populated the values serialize with no datasource-Load change.
Note the Load response marshals `Schema` directly (capabilities is top-level in that payload, not under `__schema`).

## Where to focus review

- **`addCapabilitiesField` (baseschema.go):** confirm the field is appended to the correct `__Schema` object-type definition and that `[__Capability!]` is built correctly (`AddListType(AddNonNullNamedType(...))`).
Verify it is a no-op if `__Schema` is somehow absent.
- **Gating symmetry:** the SDL merge option and the config-factory option must be set together by the caller.
If a caller enables one but not the other, planning (`HasChildNode`) or SDL validity will disagree — worth a guard or doc note for the router integration.
- **`omitempty` disabled path:** `TestServiceCapabilities_Load_Disabled` and `TestMergeBaseSchema_Capabilities_Disabled` are the byte-identical guarantees.
- **`BuildServiceCapabilities` default:** empty `defaultErrorBehavior` ⇒ `"PROPAGATE"`, which must stay synchronized with Phase 1's `ExecutionOptions.ErrorBehavior` operator-default resolution (RFC §5.8).

## Test / verification

- `cd v2 && go test ./pkg/introspection/ ./pkg/asttransform/ ./pkg/engine/datasource/introspection_datasource/ ./pkg/engine/plan/` — green.
- `cd v2 && go test ./pkg/engine/...` — green (no golden drift).

## Deliberately deferred to Phase 3

- A full **execution** e2e (`{ __schema { capabilities { identifier value } } }` → asserted JSON response) belongs in the execution module and lands in Phase 3.
Phase 2 proves the three wiring layers directly (SDL gating via AST, child-node registration via `HasChildNode`, value serialization via `source.Load`) rather than hand-authoring a brittle expected plan.
- **Router integration** (the single config switch feeding both `MergeOptions` and `IntrospectionOptions` from operator config) lives in the router repo; this phase supplies the primitives.

## Spec caveat

The introspection **query shape** for Service Capabilities (`graphql-spec#1163`) is still open; this phase assumes capabilities under `__schema { capabilities }`.
If the spec settles on a separate `service { capabilities }` root, the change is localized to `input.go`/`planner.go`/`source.go` plus the SDL location — the model and builder are unaffected.
