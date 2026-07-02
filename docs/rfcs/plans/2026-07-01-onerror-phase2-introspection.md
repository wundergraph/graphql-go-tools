# onError Phase 2 — Introspection Service Capabilities — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let graphql-go-tools advertise onError support via the GraphQL Service Capabilities introspection surface (`graphql.onError`, `graphql.defaultErrorBehavior`), emitted **only when the feature is enabled** so introspection is byte-identical to today when disabled.

**Architecture:** Extend the `introspection` package model with a `Capability` type and an optional `Capabilities` list on the introspection `Schema`, plus a config-gated builder. When a caller (the router) enables onError, it attaches capabilities to the introspection `Data`; when disabled it attaches nothing (`omitempty` ⇒ absent). The `__Capability` type is registered in the introspected type list. The end-to-end query field is wired last, gated on confirming the final spec shape.

**Tech Stack:** Go, `encoding/json`, `testify/assert`. Package `github.com/wundergraph/graphql-go-tools/v2/pkg/introspection`.

## Global Constraints

- Module root for all `go` commands: `v2/`.
- Assert full values with `assert.Equal`; for JSON, assert the exact marshaled string.
- **Disabled = byte-identical to today.** With no capabilities attached, the marshaled introspection `Schema` must be unchanged (existing `pkg/introspection` golden fixtures must pass untouched).
- Capability identifiers are exact, case-sensitive strings: `graphql.onError`, `graphql.defaultErrorBehavior`. Error-behavior values: `PROPAGATE` / `NULL` / `HALT`.
- Do **not** implement the superseded `@behavior` SDL directive or a `__Schema.defaultErrorBehavior` field.
- Spec reference: graphql-spec#1163 (Service Capabilities). Spec is open; the introspection *query shape* is provisional (see Task 4). See `docs/rfcs/2026-07-01-onerror-error-behavior.md` §5.7.

---

### Task 1: `Capability` type + optional `Capabilities` on the introspection `Schema`

**Files:**
- Modify: `v2/pkg/introspection/introspection.go` (`Schema` struct ~line 14; add `Capability` type + constructor)
- Test: `v2/pkg/introspection/introspection_capabilities_test.go` (create)

**Interfaces:**
- Produces:
  - `type Capability struct { Identifier string; Description *string; Value *string; TypeName string }` with JSON tags `identifier`, `description`, `value`, `__typename`.
  - `Schema.Capabilities []Capability` with tag `json:"capabilities,omitempty"`.

- [ ] **Step 1: Write the failing test**

```go
// v2/pkg/introspection/introspection_capabilities_test.go
package introspection

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSchema_Capabilities_OmittedWhenEmpty(t *testing.T) {
	s := NewSchema() // no capabilities
	b, err := json.Marshal(&s)
	assert.NoError(t, err)
	// capabilities key must be absent when empty (byte-identical to today)
	assert.NotContains(t, string(b), `"capabilities"`)
}

func TestSchema_Capabilities_Marshaled(t *testing.T) {
	s := NewSchema()
	val := "PROPAGATE"
	s.Capabilities = []Capability{
		{Identifier: "graphql.onError", TypeName: "__Capability"},
		{Identifier: "graphql.defaultErrorBehavior", Value: &val, TypeName: "__Capability"},
	}
	b, err := json.Marshal(&s)
	assert.NoError(t, err)
	assert.Contains(t, string(b), `"capabilities":[{"identifier":"graphql.onError","description":null,"value":null,"__typename":"__Capability"},{"identifier":"graphql.defaultErrorBehavior","description":null,"value":"PROPAGATE","__typename":"__Capability"}]`)
}
```

(Note: `assert.NotContains`/`Contains` are acceptable here because these assert *presence/absence of a key* in a large generated blob; the exact capability object is asserted in full inline.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd v2 && go test ./pkg/introspection/ -run TestSchema_Capabilities -v`
Expected: FAIL — `Capabilities`/`Capability` undefined.

- [ ] **Step 3: Add the type and field**

In `introspection.go`, add the field to `Schema` (after `Directives`):

```go
	Capabilities []Capability `json:"capabilities,omitempty"`
```

Add the type:

```go
// Capability describes a single GraphQL service capability (Service
// Capabilities introspection, graphql-spec#1163).
type Capability struct {
	Identifier  string  `json:"identifier"`
	Description *string `json:"description"`
	Value       *string `json:"value"`
	TypeName    string  `json:"__typename"`
}
```

- [ ] **Step 4: Run test + existing introspection suite**

Run: `cd v2 && go test ./pkg/introspection/ -run TestSchema_Capabilities -v && go test ./pkg/introspection/`
Expected: PASS, and existing golden tests unchanged (omitempty keeps output identical).

- [ ] **Step 5: Commit**

```bash
git add v2/pkg/introspection/introspection.go v2/pkg/introspection/introspection_capabilities_test.go
git commit -m "feat(introspection): add optional Service Capabilities to schema model"
```

---

### Task 2: Config-gated capabilities builder

**Files:**
- Modify: `v2/pkg/introspection/introspection.go` (add builder)
- Test: `v2/pkg/introspection/introspection_capabilities_test.go`

**Interfaces:**
- Produces: `func BuildServiceCapabilities(enabled bool, defaultErrorBehavior string) []Capability` — returns `nil` when `enabled` is false; otherwise `graphql.onError` plus `graphql.defaultErrorBehavior` (with `defaultErrorBehavior` value; defaults to `"PROPAGATE"` when empty).

- [ ] **Step 1: Write the failing test**

```go
// append to introspection_capabilities_test.go
func TestBuildServiceCapabilities(t *testing.T) {
	assert.Nil(t, BuildServiceCapabilities(false, "NULL")) // disabled => nothing

	got := BuildServiceCapabilities(true, "") // empty default => PROPAGATE
	pv := "PROPAGATE"
	assert.Equal(t, []Capability{
		{Identifier: "graphql.onError", TypeName: "__Capability"},
		{Identifier: "graphql.defaultErrorBehavior", Value: &pv, TypeName: "__Capability"},
	}, got)

	got = BuildServiceCapabilities(true, "NULL")
	nv := "NULL"
	assert.Equal(t, []Capability{
		{Identifier: "graphql.onError", TypeName: "__Capability"},
		{Identifier: "graphql.defaultErrorBehavior", Value: &nv, TypeName: "__Capability"},
	}, got)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd v2 && go test ./pkg/introspection/ -run TestBuildServiceCapabilities -v`
Expected: FAIL — `BuildServiceCapabilities` undefined.

- [ ] **Step 3: Implement the builder**

```go
// BuildServiceCapabilities returns the Service Capabilities to advertise, or nil
// when the onError feature is disabled. When enabled, defaultErrorBehavior is the
// operator-configured default ("" => "PROPAGATE").
func BuildServiceCapabilities(enabled bool, defaultErrorBehavior string) []Capability {
	if !enabled {
		return nil
	}
	if defaultErrorBehavior == "" {
		defaultErrorBehavior = "PROPAGATE"
	}
	def := defaultErrorBehavior
	return []Capability{
		{Identifier: "graphql.onError", TypeName: "__Capability"},
		{Identifier: "graphql.defaultErrorBehavior", Value: &def, TypeName: "__Capability"},
	}
}
```

- [ ] **Step 4: Run test**

Run: `cd v2 && go test ./pkg/introspection/ -run TestBuildServiceCapabilities -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add v2/pkg/introspection/introspection.go v2/pkg/introspection/introspection_capabilities_test.go
git commit -m "feat(introspection): add config-gated BuildServiceCapabilities helper"
```

---

### Task 3: Register the `__Capability` type in the introspected type list

**Files:**
- Modify: `v2/pkg/introspection/introspection.go` (extend `NewSchema` or add a registration helper)
- Test: `v2/pkg/introspection/introspection_capabilities_test.go`

**Interfaces:**
- Produces: `func (s *Schema) AddCapabilityType()` — appends the `__Capability` object type (fields `identifier: String!`, `description: String`, `value: String`) to `s.Types` so `__type(name:"__Capability")` and `__schema { types }` resolve it. Idempotent (no-op if already present).

**Rationale:** capabilities are only introspectable if `__Capability` is a known type. Only register it when capabilities are actually attached (called by the caller alongside `BuildServiceCapabilities`).

- [ ] **Step 1: Write the failing test**

```go
// append to introspection_capabilities_test.go
func TestSchema_AddCapabilityType(t *testing.T) {
	s := NewSchema()
	s.AddCapabilityType()
	ft := s.TypeByName("__Capability")
	assert.NotNil(t, ft)
	assert.Equal(t, "__Capability", ft.Name)
	assert.Equal(t, OBJECT, ft.Kind)
	names := make([]string, 0, len(ft.Fields))
	for _, f := range ft.Fields {
		names = append(names, f.Name)
	}
	assert.Equal(t, []string{"identifier", "description", "value"}, names)

	// idempotent
	s.AddCapabilityType()
	count := 0
	for _, tpe := range s.Types {
		if tpe.Name == "__Capability" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd v2 && go test ./pkg/introspection/ -run TestSchema_AddCapabilityType -v`
Expected: FAIL — `AddCapabilityType` undefined.

- [ ] **Step 3: Implement `AddCapabilityType`**

Build the `__Capability` object type using the existing `NewFullType`/`NewField`/`TypeRef` constructors (mirror how other `__`-types are shaped in `generator.go`). `identifier` is `String!` (NON_NULL wrapping SCALAR String); `description` and `value` are nullable `String`.

```go
func stringTypeRef(nonNull bool) TypeRef {
	name := "String"
	scalar := TypeRef{Kind: SCALAR, Name: &name, TypeName: "__Type"}
	if !nonNull {
		return scalar
	}
	return TypeRef{Kind: NON_NULL, OfType: &scalar, TypeName: "__Type"}
}

// AddCapabilityType registers the __Capability object type. Idempotent.
func (s *Schema) AddCapabilityType() {
	if s.TypeByName("__Capability") != nil {
		return
	}
	ft := NewFullType()
	ft.Kind = OBJECT
	ft.Name = "__Capability"
	idField := NewField()
	idField.Name = "identifier"
	idField.Type = stringTypeRef(true)
	descField := NewField()
	descField.Name = "description"
	descField.Type = stringTypeRef(false)
	valField := NewField()
	valField.Name = "value"
	valField.Type = stringTypeRef(false)
	ft.Fields = []Field{idField, descField, valField}
	s.AddType(ft)
}
```

(Verify `OBJECT`, `SCALAR`, `NON_NULL` enum identifiers against `introspection_enum.go` before running; adjust names if the generated enum differs.)

- [ ] **Step 4: Run test + full introspection suite**

Run: `cd v2 && go test ./pkg/introspection/ -run TestSchema_AddCapabilityType -v && go test ./pkg/introspection/`
Expected: PASS; existing goldens untouched (only affected when `AddCapabilityType` is explicitly called).

- [ ] **Step 5: Commit**

```bash
git add v2/pkg/introspection/introspection.go v2/pkg/introspection/introspection_capabilities_test.go
git commit -m "feat(introspection): register __Capability type on demand"
```

---

### Task 4: Wire capabilities into the introspection query path (spec-shape gated)

**Files:**
- Read/confirm: current spec introspection shape (graphql-spec#1163) — is `capabilities` exposed under `__schema { capabilities }` or a separate `service { capabilities }` root?
- Modify (likely): the introspection schema definition consumed by planning (locate first), and — only if a new root field is required — `v2/pkg/engine/datasource/introspection_datasource/{input.go,planner.go,source.go}`.
- Test: `v2/pkg/engine/datasource/introspection_datasource/*_test.go`

**Interfaces:**
- Consumes: `Schema.Capabilities` (Task 1), `BuildServiceCapabilities` (Task 2), `Schema.AddCapabilityType` (Task 3).

**Decision to record before coding:** confirm the final introspection field path in the spec and write it into this task (one line). Default assumption if unchanged: capabilities live under `__schema` (i.e. `__schema { capabilities { identifier value } }`). This is the cheapest to support because `source.go` (`Load`) already returns `json.Marshal(s.introspectionData.Schema)` for `__schema`, so once `Schema.Capabilities` is populated the field serializes with **no datasource change** — only the introspection SDL/type definition must declare `__Schema.capabilities: [__Capability!]!`.

- [ ] **Step 1: Locate the introspection schema definition**

Run: `cd v2 && grep -rn "__Schema" pkg --include=*.go | grep -iv "_test\|__schema\"" | head` and `grep -rln "queryType\|__Directive\|__Type" pkg/introspection pkg/ast* pkg/astparser 2>/dev/null | head`
Goal: find where the `__Schema`/`__Type` introspection type system is declared (SDL string or programmatic), which is where a `capabilities` field / `service` root must be added.

- [ ] **Step 2: Write a failing end-to-end test**

Add a datasource/pipeline test that executes `{ __schema { capabilities { identifier value } } }` (or the confirmed field) against a schema built with `Schema.Capabilities` populated + `AddCapabilityType()` called, and asserts the **full** response:

```
{"data":{"__schema":{"capabilities":[{"identifier":"graphql.onError","value":null},{"identifier":"graphql.defaultErrorBehavior","value":"PROPAGATE"}]}}}
```

Run it and confirm it fails (field unknown / not resolved).

- [ ] **Step 3: Implement the confirmed wiring**

- If capabilities live under `__schema`: extend the introspection type definition (from Step 1) to declare `capabilities: [__Capability!]!` on `__Schema` and the `__Capability` type; no `source.go` change needed (it already marshals the whole `Schema`). Ensure the code path that builds `introspectionData` calls `AddCapabilityType()` and sets `Schema.Capabilities = BuildServiceCapabilities(enabled, defaultBehavior)`.
- If a separate `service` root is required: add `serviceFieldName = "service"` in `input.go`, a `case serviceFieldName` in `planner.go` `EnterField`, a `ServiceRequestType` + input branch, and a `source.go` branch returning `json.Marshal(struct{ Capabilities []introspection.Capability }{...})`. Follow the exact `__schema` pattern in those three files.

- [ ] **Step 4: Run the e2e test + affected suites**

Run: `cd v2 && go test ./pkg/engine/datasource/introspection_datasource/ && go test ./pkg/introspection/`
Expected: PASS. Also confirm that with the feature disabled (capabilities not attached, `AddCapabilityType` not called) introspection output is byte-identical to today.

- [ ] **Step 5: Commit**

```bash
git add -A v2/pkg/engine/datasource/introspection_datasource v2/pkg/introspection
git commit -m "feat(introspection): expose Service Capabilities via introspection query"
```

---

## Self-Review Notes

- **Coverage vs RFC §5.7:** Tasks 1–3 deliver the config-gated model/builder/type-registration the library owns and can golden-test; Task 4 is the end-to-end query wiring, explicitly gated on confirming the (still-open) spec field shape.
- **Disabled path:** `omitempty` + on-demand `AddCapabilityType` guarantee byte-identical introspection when the feature is off — asserted in Task 1 Step 1 and Task 4 Step 4.
- **Router integration** (config switch, feeding `BuildServiceCapabilities` from operator config) lives in the router repo; this plan supplies the primitives (Phase 1 `ErrorBehavior`/`ExecutionOptions`, Phase 2 builder/type). Keep the operator default synchronized between `ExecutionOptions.ErrorBehavior` resolution and `graphql.defaultErrorBehavior` per RFC §5.8.
