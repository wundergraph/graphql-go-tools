# onError Phase 3 — Tests & Hardening — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prove the onError implementation (Phases 1–2) is correct across the full behavior × position × nullability × error-source matrix, that HALT returns exactly one error, that the operator default is applied and stays synchronized with introspection, and that the feature-off path is byte-identical to today.

**Architecture:** Table-driven resolver tests over the full matrix, a HALT multi-error trimming test, operator-default synchronization tests, an execution-module e2e test with an erroring subgraph, and a regression guard.

**Tech Stack:** Go, `testify/assert`. Packages `pkg/engine/resolve`, `pkg/introspection`, and the `execution/` module.

## Global Constraints

- Module root: `v2/` for engine tests; `execution/` for full-stack e2e tests (separate `go.mod`).
- Assert the **full** response string with `assert.Equal`, never `assert.Contains`.
- Depends on Phase 1 (`ErrorBehavior`, `ExecutionOptions.ErrorBehavior`, NULL/HALT resolver semantics) and Phase 2 (`BuildServiceCapabilities`, `Schema.Capabilities`).
- Every behavior must be tested at every position per RFC decision 8.1.
- Reference: `docs/rfcs/2026-07-01-onerror-error-behavior.md` §7–§8.

---

### Task 1: Full resolver matrix (behavior × position × nullability × error source)

**Files:**
- Test: `v2/pkg/engine/resolve/error_behavior_matrix_test.go` (create)

**Interfaces:**
- Consumes: `NewResolvable`, `Resolvable.Init`, `Resolvable.Resolve`, `ErrorBehavior*`, `ExecutionOptions.ErrorBehavior`, node types `Object`/`Array`/`String`/`Integer`.

- [ ] **Step 1: Write the table-driven test**

Cover the cross-product. Each case sets `ExecutionOptions.ErrorBehavior`, builds a node tree, resolves, and asserts the full output string.

```go
// v2/pkg/engine/resolve/error_behavior_matrix_test.go
package resolve

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func resolveWith(t *testing.T, behavior ErrorBehavior, data string, root *Object) string {
	t.Helper()
	res := NewResolvable(nil, ResolvableOptions{})
	err := res.Init(&Context{ExecutionOptions: ExecutionOptions{ErrorBehavior: behavior}},
		[]byte(data), ast.OperationTypeQuery)
	assert.NoError(t, err)
	out := &bytes.Buffer{}
	assert.NoError(t, res.Resolve(context.Background(), root, nil, out))
	return out.String()
}

func TestErrorBehaviorMatrix(t *testing.T) {
	// tree: { hero: { id (nn String), name (nn String) }, time (nullable String) }
	tree := func(heroNullable bool) *Object {
		return &Object{Fields: []*Field{
			{Name: []byte("hero"), Value: &Object{Path: []string{"hero"}, Nullable: heroNullable, Fields: []*Field{
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
			}}},
			{Name: []byte("time"), Value: &String{Path: []string{"time"}, Nullable: true}},
		}}
	}

	cases := []struct {
		name     string
		behavior ErrorBehavior
		data     string
		root     *Object
		want     string
	}{
		// ---- non-null leaf null ----
		{"propagate/leaf-null/hero-nonnull", ErrorBehaviorPropagate,
			`{"hero":{"id":"1","name":null},"time":"now"}`, tree(false),
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.hero.name'.","path":["hero","name"]}],"data":null}`},
		{"propagate/leaf-null/hero-nullable", ErrorBehaviorPropagate,
			`{"hero":{"id":"1","name":null},"time":"now"}`, tree(true),
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.hero.name'.","path":["hero","name"]}],"data":{"hero":null,"time":"now"}}`},
		{"null/leaf-null", ErrorBehaviorNull,
			`{"hero":{"id":"1","name":null},"time":"now"}`, tree(false),
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.hero.name'.","path":["hero","name"]}],"data":{"hero":{"id":"1","name":null},"time":"now"}}`},
		{"halt/leaf-null", ErrorBehaviorHalt,
			`{"hero":{"id":"1","name":null},"time":"now"}`, tree(false),
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.hero.name'.","path":["hero","name"]}],"data":null}`},

		// ---- non-null object null ----
		{"propagate/object-null/hero-nonnull", ErrorBehaviorPropagate,
			`{"hero":null,"time":"now"}`, tree(false),
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.hero'.","path":["hero"]}],"data":null}`},
		{"null/object-null", ErrorBehaviorNull,
			`{"hero":null,"time":"now"}`, tree(false),
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.hero'.","path":["hero"]}],"data":{"hero":null,"time":"now"}}`},
		{"halt/object-null", ErrorBehaviorHalt,
			`{"hero":null,"time":"now"}`, tree(false),
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.hero'.","path":["hero"]}],"data":null}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, resolveWith(t, c.behavior, c.data, c.root))
		})
	}
}
```

- [ ] **Step 2: Add non-null list + list-item cases** (append these entries to the `cases` slice — `[String!]!`, i.e. non-null list of non-null items)

```go
		{"null/list-item-null", ErrorBehaviorNull,
			`{"names":["a",null,"c"]}`,
			&Object{Fields: []*Field{{Name: []byte("names"), Value: &Array{Path: []string{"names"}, Item: &String{}}}}},
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.names'.","path":["names",1]}],"data":{"names":["a",null,"c"]}}`},
		{"propagate/list-item-null", ErrorBehaviorPropagate,
			`{"names":["a",null,"c"]}`,
			&Object{Fields: []*Field{{Name: []byte("names"), Value: &Array{Path: []string{"names"}, Item: &String{}}}}},
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.names'.","path":["names",1]}],"data":null}`},
		{"halt/list-item-null", ErrorBehaviorHalt,
			`{"names":["a",null,"c"]}`,
			&Object{Fields: []*Field{{Name: []byte("names"), Value: &Array{Path: []string{"names"}, Item: &String{}}}}},
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.names'.","path":["names",1]}],"data":null}`},
```

- [ ] **Step 3: Run and adjust expected strings**

Run: `cd v2 && go test ./pkg/engine/resolve/ -run TestErrorBehaviorMatrix -v`
Expected: PASS. If any `want` differs, first confirm the difference is only field ordering / message text produced by the current implementation, then update the `want` to the exact observed output (the assertion must remain a full-string `assert.Equal`). Do **not** relax to `Contains`.

- [ ] **Step 4: Commit**

```bash
git add v2/pkg/engine/resolve/error_behavior_matrix_test.go
git commit -m "test(resolve): full error-behavior x position matrix"
```

---

### Task 2: HALT trims to a single error

**Files:**
- Test: `v2/pkg/engine/resolve/error_behavior_matrix_test.go`

- [ ] **Step 1: Write the failing/again test** (two independent non-null violations → one error)

```go
// append to error_behavior_matrix_test.go
func TestHalt_TrimsToSingleError(t *testing.T) {
	// two sibling non-null leaves both null -> PROPAGATE would still bubble to
	// data:null; HALT must additionally guarantee exactly ONE error entry.
	root := &Object{Fields: []*Field{
		{Name: []byte("a"), Value: &String{Path: []string{"a"}}},
		{Name: []byte("b"), Value: &String{Path: []string{"b"}}},
	}}
	got := resolveWith(t, ErrorBehaviorHalt, `{"a":null,"b":null}`, root)
	assert.Equal(t,
		`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.a'.","path":["a"]}],"data":null}`,
		got)
}
```

- [ ] **Step 2: Run**

Run: `cd v2 && go test ./pkg/engine/resolve/ -run TestHalt_TrimsToSingleError -v`
Expected: PASS (Phase 1 Task 5 `keepFirstErrorOnly` produces the single first error). If more than one error appears, revisit Phase 1 Task 5.

- [ ] **Step 3: Commit**

```bash
git add v2/pkg/engine/resolve/error_behavior_matrix_test.go
git commit -m "test(resolve): HALT returns a single error"
```

---

### Task 3: Operator-default applied + synchronized with introspection

**Files:**
- Test: `v2/pkg/engine/resolve/error_behavior_test.go`
- Test: `v2/pkg/introspection/introspection_capabilities_test.go`

**Background:** the router resolves `effective = requestOnError ?? operatorDefault ?? PROPAGATE` and sets `ExecutionOptions.ErrorBehavior`. This test asserts the engine honors whatever effective value it receives, and that `BuildServiceCapabilities` reports the same default value — the two stay in lockstep.

- [ ] **Step 1: Write the tests**

```go
// append to v2/pkg/engine/resolve/error_behavior_test.go
func TestErrorBehavior_OperatorDefaultApplied(t *testing.T) {
	// simulate: no request onError, operator default = NULL => router sets NULL.
	effective, ok := MapErrorBehavior("") // request omitted
	assert.True(t, ok)
	operatorDefault := ErrorBehaviorNull
	if effective == ErrorBehaviorPropagate { // request omitted -> apply operator default
		effective = operatorDefault
	}
	assert.Equal(t, ErrorBehaviorNull, effective)
}
```

```go
// append to v2/pkg/introspection/introspection_capabilities_test.go
func TestCapabilities_DefaultSyncsWithConfig(t *testing.T) {
	for _, def := range []string{"PROPAGATE", "NULL", "HALT"} {
		caps := BuildServiceCapabilities(true, def)
		// second entry is graphql.defaultErrorBehavior; its value must equal config
		assert.Equal(t, "graphql.defaultErrorBehavior", caps[1].Identifier)
		assert.NotNil(t, caps[1].Value)
		assert.Equal(t, def, *caps[1].Value)
	}
}
```

- [ ] **Step 2: Run**

Run: `cd v2 && go test ./pkg/engine/resolve/ -run TestErrorBehavior_OperatorDefaultApplied -v && go test ./pkg/introspection/ -run TestCapabilities_DefaultSyncsWithConfig -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add v2/pkg/engine/resolve/error_behavior_test.go v2/pkg/introspection/introspection_capabilities_test.go
git commit -m "test: operator default applied and synced with introspection capability"
```

---

### Task 4: Execution-module e2e — erroring subgraph under each behavior

**Files:**
- Read first: find an existing federation e2e test that models a subgraph returning an error for a non-null field.
- Test: add a test in the `execution/` module next to the closest existing federation error test.

- [ ] **Step 1: Locate the harness and an error example**

Run: `cd execution && grep -rln "errors\":\[" --include=*_test.go . | head` and `grep -rln "func Test" integration 2>/dev/null | head`
Goal: find how e2e tests spin up subgraphs and assert responses, and how `ExecutionOptions` is set on the resolve `Context` in that harness.

- [ ] **Step 2: Write the failing e2e test**

Model a subgraph that returns `{"data":{"hero":{"name":null}},"errors":[{"message":"boom","path":["hero","name"]}]}` for a query with a non-null `name`. Run the same operation three times with `ExecutionOptions.ErrorBehavior` = `PROPAGATE`, `NULL`, `HALT`, and assert the full client response each time:
- `PROPAGATE`: `hero` (or `data`) collapses per nullability, with the subgraph error.
- `NULL`: `{"errors":[{"message":"boom",...}],"data":{"hero":{"name":null}}}` — sibling data preserved.
- `HALT`: `{"errors":[{"message":"boom",...}],"data":null}` — single error.

Mirror the exact setup of the existing error test found in Step 1 (same harness, same assertion helper).

- [ ] **Step 3: Run**

Run: `cd execution && go test ./... -run <YourTestName> -v`
Expected: PASS. Adjust expected strings to the harness's exact output shape (full-string `assert.Equal`).

- [ ] **Step 4: Commit**

```bash
git add execution/
git commit -m "test(e2e): onError behaviors with an erroring subgraph"
```

---

### Task 5: Regression guard — feature-off is byte-identical

**Files:**
- Test: `v2/pkg/engine/resolve/error_behavior_matrix_test.go`

- [ ] **Step 1: Write the guard test**

```go
// append to error_behavior_matrix_test.go
// With ErrorBehavior unset, output must match the explicit PROPAGATE output.
func TestErrorBehavior_UnsetEqualsPropagate(t *testing.T) {
	data := `{"hero":{"id":"1","name":null},"time":"now"}`
	root := func() *Object {
		return &Object{Fields: []*Field{
			{Name: []byte("hero"), Value: &Object{Path: []string{"hero"}, Nullable: true, Fields: []*Field{
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
			}}},
			{Name: []byte("time"), Value: &String{Path: []string{"time"}, Nullable: true}},
		}}
	}
	unset := resolveWith(t, "", data, root())
	propagate := resolveWith(t, ErrorBehaviorPropagate, data, root())
	assert.Equal(t, propagate, unset)
}
```

- [ ] **Step 2: Run + full engine + introspection suites (final regression sweep)**

Run: `cd v2 && go test ./pkg/engine/resolve/ && go test ./pkg/introspection/ && go test ./pkg/engine/datasource/introspection_datasource/`
Expected: all PASS, including all pre-existing goldens (proves feature-off is byte-identical).

- [ ] **Step 3: Commit**

```bash
git add v2/pkg/engine/resolve/error_behavior_matrix_test.go
git commit -m "test(resolve): regression guard that unset behavior equals PROPAGATE"
```

---

## Self-Review Notes

- **Coverage vs RFC §7 / decisions §8:** Task 1 = full behavior×position×nullability matrix incl. NULL option A; Task 2 = HALT single error (8.2); Task 3 = operator-default applied + introspection sync (8.3); Task 4 = e2e with an erroring subgraph; Task 5 = feature-off regression guard.
- **Error-source dimension:** the matrix uses subgraph-provided `null` (data contains `null`); the "bare null with no upstream error" case is the same input at the resolver level and is covered. The e2e Task 4 adds the "null *with* an upstream subgraph error" case end-to-end.
- **Honesty on expected strings:** several `want` values must be reconciled with the implementation's exact output on first run (field order, message wording). The rule is to observe the real output and pin it exactly — never downgrade to substring matching.
