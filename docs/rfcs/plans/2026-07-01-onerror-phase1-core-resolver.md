# onError Phase 1 — Core Resolver Behavior — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement client-selectable GraphQL error behavior (`PROPAGATE` / `NULL` / `HALT`) in the graphql-go-tools resolver, defaulting to `PROPAGATE` with zero behavior change for existing callers.

**Architecture:** Add an `ErrorBehavior` enum + validation helper, thread the selected behavior through `ExecutionOptions` into the `Resolvable`, and change the resolver's execution-error handling: under `NULL`, an execution error at a position sets that position to `null` in place instead of propagating; under `HALT`, any error collapses the whole response to `data: null` with a single error. `PROPAGATE` is unchanged.

**Tech Stack:** Go, `github.com/wundergraph/astjson`, `testify/assert`. Package `github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve`.

## Global Constraints

- Module root for all `go` commands: `v2/` (run `cd v2` first).
- Follow the repo test convention: assert the **full** response string with `assert.Equal`, never `assert.Contains`.
- `PROPAGATE` output must remain byte-identical to today — the existing `resolvable_test.go` / `resolve_test.go` suites must pass unchanged.
- The empty `ErrorBehavior` value means the default and must normalize to `PROPAGATE`.
- Match existing code style in `resolvable.go`. Do not refactor unrelated code.
- Spec reference: request attribute values `NULL`, `PROPAGATE`, `HALT` (graphql-spec#1163). See `docs/rfcs/2026-07-01-onerror-error-behavior.md`.

---

### Task 1: `ErrorBehavior` type and `MapErrorBehavior` helper

**Files:**
- Create: `v2/pkg/engine/resolve/error_behavior.go`
- Test: `v2/pkg/engine/resolve/error_behavior_test.go`

**Interfaces:**
- Produces: `type ErrorBehavior string`; consts `ErrorBehaviorPropagate="PROPAGATE"`, `ErrorBehaviorNull="NULL"`, `ErrorBehaviorHalt="HALT"`; `func MapErrorBehavior(s string) (ErrorBehavior, bool)`.

- [ ] **Step 1: Write the failing test**

```go
// v2/pkg/engine/resolve/error_behavior_test.go
package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapErrorBehavior(t *testing.T) {
	cases := []struct {
		in   string
		want ErrorBehavior
		ok   bool
	}{
		{"", ErrorBehaviorPropagate, true}, // empty => default
		{"PROPAGATE", ErrorBehaviorPropagate, true},
		{"NULL", ErrorBehaviorNull, true},
		{"HALT", ErrorBehaviorHalt, true},
		{"null", "", false},  // case-sensitive per spec
		{"BOGUS", "", false},
	}
	for _, c := range cases {
		got, ok := MapErrorBehavior(c.in)
		assert.Equal(t, c.ok, ok, "ok for %q", c.in)
		assert.Equal(t, c.want, got, "value for %q", c.in)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd v2 && go test ./pkg/engine/resolve/ -run TestMapErrorBehavior -v`
Expected: FAIL — `undefined: MapErrorBehavior` / `undefined: ErrorBehaviorPropagate`.

- [ ] **Step 3: Write minimal implementation**

```go
// v2/pkg/engine/resolve/error_behavior.go
package resolve

// ErrorBehavior selects how execution errors interact with non-null positions
// in the response, per the GraphQL onError proposal (graphql-spec#1163).
type ErrorBehavior string

const (
	// ErrorBehaviorPropagate is the spec default and current behavior: an
	// execution error in a non-null position propagates to the nearest nullable
	// ancestor (or data: null at the root).
	ErrorBehaviorPropagate ErrorBehavior = "PROPAGATE"
	// ErrorBehaviorNull sets the errored position to null in place (even if
	// non-nullable) without propagating; the error is still recorded.
	ErrorBehaviorNull ErrorBehavior = "NULL"
	// ErrorBehaviorHalt aborts response assembly on any error: data is null and
	// a single error is returned.
	ErrorBehaviorHalt ErrorBehavior = "HALT"
)

// MapErrorBehavior validates and normalizes an incoming onError value.
// The empty string maps to the default (PROPAGATE). Any other unknown value
// returns ok=false so the caller can raise a request error.
func MapErrorBehavior(s string) (ErrorBehavior, bool) {
	switch ErrorBehavior(s) {
	case "":
		return ErrorBehaviorPropagate, true
	case ErrorBehaviorPropagate, ErrorBehaviorNull, ErrorBehaviorHalt:
		return ErrorBehavior(s), true
	default:
		return "", false
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd v2 && go test ./pkg/engine/resolve/ -run TestMapErrorBehavior -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add v2/pkg/engine/resolve/error_behavior.go v2/pkg/engine/resolve/error_behavior_test.go
git commit -m "feat(resolve): add ErrorBehavior enum and MapErrorBehavior helper"
```

---

### Task 2: Thread `ErrorBehavior` through `ExecutionOptions` into `Resolvable`

**Files:**
- Modify: `v2/pkg/engine/resolve/context.go` (`ExecutionOptions` struct, ~line 113)
- Modify: `v2/pkg/engine/resolve/resolvable.go` (`Resolvable` struct ~line 28, `Reset` ~line 123, `Resolve` ~line 229, `ResolveNode` ~line 208)
- Test: `v2/pkg/engine/resolve/resolvable_test.go`

**Interfaces:**
- Consumes: `ErrorBehavior` (Task 1).
- Produces: `ExecutionOptions.ErrorBehavior ErrorBehavior`; unexported `Resolvable.errorBehavior ErrorBehavior`, populated at the start of `Resolve`/`ResolveNode` and normalized so empty ⇒ `ErrorBehaviorPropagate`.

- [ ] **Step 1: Write the failing test** (proves default is PROPAGATE and the field is read)

```go
// append to v2/pkg/engine/resolve/resolvable_test.go
func TestResolvable_ErrorBehaviorDefaultsToPropagate(t *testing.T) {
	res := NewResolvable(nil, ResolvableOptions{})
	ctx := &Context{} // ExecutionOptions zero value
	err := res.Init(ctx, []byte(`{"hero":{"name":"R2D2"}}`), ast.OperationTypeQuery)
	assert.NoError(t, err)
	object := &Object{Fields: []*Field{{
		Name: []byte("hero"),
		Value: &Object{Path: []string{"hero"}, Fields: []*Field{{
			Name:  []byte("name"),
			Value: &String{Path: []string{"name"}},
		}}},
	}}}
	out := &bytes.Buffer{}
	err = res.Resolve(context.Background(), object, nil, out)
	assert.NoError(t, err)
	assert.Equal(t, `{"data":{"hero":{"name":"R2D2"}}}`, out.String())
	assert.Equal(t, ErrorBehaviorPropagate, res.errorBehavior)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd v2 && go test ./pkg/engine/resolve/ -run TestResolvable_ErrorBehaviorDefaultsToPropagate -v`
Expected: FAIL — `res.errorBehavior undefined`.

- [ ] **Step 3: Add the field to `ExecutionOptions`**

In `v2/pkg/engine/resolve/context.go`, inside `type ExecutionOptions struct { ... }`, add:

```go
	// ErrorBehavior selects how execution errors interact with non-null
	// positions. The empty value is treated as PROPAGATE (spec default).
	ErrorBehavior ErrorBehavior
```

- [ ] **Step 4: Add the field to `Resolvable` and reset it**

In `resolvable.go`, add to the `Resolvable` struct (near `operationType`):

```go
	errorBehavior ErrorBehavior
```

In `Reset()` add:

```go
	r.errorBehavior = ""
```

- [ ] **Step 5: Populate + normalize at the resolve entry points**

At the very top of `Resolve` (after `r.authorizationError = nil`, before `SkipLoader`) and at the top of `ResolveNode` (after `r.errors = nil`), add:

```go
	r.errorBehavior = r.ctx.ExecutionOptions.ErrorBehavior
	if r.errorBehavior == "" {
		r.errorBehavior = ErrorBehaviorPropagate
	}
```

- [ ] **Step 6: Run test + full resolve suite**

Run: `cd v2 && go test ./pkg/engine/resolve/ -run TestResolvable_ErrorBehaviorDefaultsToPropagate -v && go test ./pkg/engine/resolve/`
Expected: PASS, and the entire `resolve` package still green (PROPAGATE unchanged).

- [ ] **Step 7: Commit**

```bash
git add v2/pkg/engine/resolve/context.go v2/pkg/engine/resolve/resolvable.go v2/pkg/engine/resolve/resolvable_test.go
git commit -m "feat(resolve): thread ErrorBehavior from ExecutionOptions into Resolvable"
```

---

### Task 3: `NULL` behavior at null-in-non-null positions

**Files:**
- Modify: `v2/pkg/engine/resolve/resolvable.go` (add helper; edit `walkObject`, `walkArray`, `walkString`, `walkBoolean`, `walkInteger`, `walkFloat`, `walkBigInt`, `walkScalar`, `walkCustom`, `walkEnum`)
- Test: `v2/pkg/engine/resolve/resolvable_test.go`

**Interfaces:**
- Consumes: `r.errorBehavior` (Task 2).
- Produces: `func (r *Resolvable) erroredPosition(path []string, parent *astjson.Value) bool` — returns `true` to propagate (PROPAGATE/HALT) or, under `NULL`, sets the position to `null` on the validation pass / prints `null` on the print pass and returns `false`.

**Background:** every non-null leaf/object/array walker currently does `addNonNullableFieldError(x.Path, parent); return r.err()`. `err()` returns `true`, which bubbles. Under `NULL` we must (a) record the error once (validation pass only, since we no longer bubble and the print pass re-descends), and (b) render `null` in place.

- [ ] **Step 1: Write the failing tests** (leaf, object, list-item, nested, siblings preserved, with and without upstream error)

```go
// append to v2/pkg/engine/resolve/resolvable_test.go

func newNullResolvable(t *testing.T, data string) *Resolvable {
	t.Helper()
	res := NewResolvable(nil, ResolvableOptions{})
	err := res.Init(&Context{ExecutionOptions: ExecutionOptions{ErrorBehavior: ErrorBehaviorNull}},
		[]byte(data), ast.OperationTypeQuery)
	assert.NoError(t, err)
	return res
}

// non-null leaf returns null -> stays null, sibling preserved, error recorded
func TestResolvable_NullBehavior_NonNullLeaf(t *testing.T) {
	res := newNullResolvable(t, `{"hero":{"id":"1","name":null}}`)
	object := &Object{Fields: []*Field{{
		Name: []byte("hero"),
		Value: &Object{Path: []string{"hero"}, Fields: []*Field{
			{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
			{Name: []byte("name"), Value: &String{Path: []string{"name"}}}, // non-null
		}},
	}}}
	out := &bytes.Buffer{}
	err := res.Resolve(context.Background(), object, nil, out)
	assert.NoError(t, err)
	assert.Equal(t, `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.hero.name'.","path":["hero","name"]}],"data":{"hero":{"id":"1","name":null}}}`, out.String())
}

// non-null object returns null -> object null, sibling field of parent preserved
func TestResolvable_NullBehavior_NonNullObject(t *testing.T) {
	res := newNullResolvable(t, `{"hero":null,"time":"now"}`)
	object := &Object{Fields: []*Field{
		{Name: []byte("hero"), Value: &Object{Path: []string{"hero"}, Fields: []*Field{
			{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
		}}}, // non-null object
		{Name: []byte("time"), Value: &String{Path: []string{"time"}}},
	}}
	out := &bytes.Buffer{}
	err := res.Resolve(context.Background(), object, nil, out)
	assert.NoError(t, err)
	assert.Equal(t, `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.hero'.","path":["hero"]}],"data":{"hero":null,"time":"now"}}`, out.String())
}

// non-null list item null -> that item null, other items preserved
func TestResolvable_NullBehavior_NonNullListItem(t *testing.T) {
	res := newNullResolvable(t, `{"names":["a",null,"c"]}`)
	object := &Object{Fields: []*Field{{
		Name:  []byte("names"),
		Value: &Array{Path: []string{"names"}, Item: &String{}}, // non-null items
	}}}
	out := &bytes.Buffer{}
	err := res.Resolve(context.Background(), object, nil, out)
	assert.NoError(t, err)
	assert.Equal(t, `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.names'.","path":["names",1]}],"data":{"names":["a",null,"c"]}}`, out.String())
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd v2 && go test ./pkg/engine/resolve/ -run 'TestResolvable_NullBehavior' -v`
Expected: FAIL — current behavior propagates (e.g. `"data":{"hero":null}` for the leaf case, or `data:null`), not the in-place null above.

- [ ] **Step 3: Add the helper**

Add near `err()` in `resolvable.go`:

```go
// erroredPosition decides how an execution error at the current position is
// handled under the active error behavior. The caller must have already
// recorded the error (guarded to the validation pass). Under NULL the position
// is rendered as null and the error does not propagate; otherwise it propagates.
func (r *Resolvable) erroredPosition(path []string, parent *astjson.Value) bool {
	if r.errorBehavior != ErrorBehaviorNull {
		return r.err()
	}
	if r.print {
		return r.walkNull() // prints null on the print pass, returns false
	}
	if len(path) > 0 {
		astjson.SetNull(r.astjsonArena, parent, path...)
	}
	return false
}
```

- [ ] **Step 4: Convert the null-in-non-null sites**

For each site below, replace the two lines
```go
		r.addNonNullableFieldError(<PATH>, parent)
		return r.err()
```
with
```go
		if !r.print {
			r.addNonNullableFieldError(<PATH>, parent)
		}
		return r.erroredPosition(<PATH>, parent)
```

Apply at (path arg shown per function):
- `walkObject` (`obj.Path`) — the `value == nil || TypeNull` branch (~line 701).
- `walkArray` (`arr.Path`) — the `ValueIsNull` branch (~line 949).
- `walkString` (`s.Path`) — ~line 1079.
- `walkBoolean` (`b.Path`) — ~line 1125.
- `walkInteger` (`i.Path`) — ~line 1146.
- `walkFloat` (`f.Path`) — ~line 1167.
- `walkBigInt` (`b.Path`) — ~line 1197.
- `walkScalar` (`s.Path`) — ~line 1213.
- `walkCustom` (`c.Path`) — ~line 1245.
- `walkEnum` (`e.Path`) — ~line 1324.

- [ ] **Step 5: Run the NULL tests + full suite**

Run: `cd v2 && go test ./pkg/engine/resolve/ -run 'TestResolvable_NullBehavior' -v && go test ./pkg/engine/resolve/`
Expected: PASS, and the whole `resolve` package green (PROPAGATE paths unchanged because `erroredPosition` returns `r.err()` for non-NULL).

- [ ] **Step 6: Commit**

```bash
git add v2/pkg/engine/resolve/resolvable.go v2/pkg/engine/resolve/resolvable_test.go
git commit -m "feat(resolve): implement NULL error behavior for non-null positions"
```

---

### Task 4: `NULL` behavior for value-coercion and invalid-enum errors

**Files:**
- Modify: `v2/pkg/engine/resolve/resolvable.go` (`walkObject` non-object, `walkArray` non-array, `walkString`/`walkBoolean`/`walkInteger`/`walkFloat` type mismatch, `walkCustom` resolve error, `walkEnum` type mismatch + invalid value)
- Test: `v2/pkg/engine/resolve/resolvable_test.go`

**Interfaces:**
- Consumes: `erroredPosition` (Task 3).

**Background:** coercion failures (wrong JSON type) and invalid enum values are also execution errors; under `NULL` they must render as null in place, not bubble. These sites currently call an `addError*` then `return r.err()` (some already guard the add with `!r.print`).

- [ ] **Step 1: Write the failing test**

```go
// append to v2/pkg/engine/resolve/resolvable_test.go

// non-null leaf with a type mismatch under NULL -> null in place + error, sibling kept
func TestResolvable_NullBehavior_TypeMismatch(t *testing.T) {
	res := newNullResolvable(t, `{"hero":{"id":"1","name":123}}`)
	object := &Object{Fields: []*Field{{
		Name: []byte("hero"),
		Value: &Object{Path: []string{"hero"}, Fields: []*Field{
			{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
			{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
		}},
	}}}
	out := &bytes.Buffer{}
	err := res.Resolve(context.Background(), object, nil, out)
	assert.NoError(t, err)
	assert.Equal(t, `{"errors":[{"message":"String cannot represent non-string value: \"123\"","path":["hero","name"]}],"data":{"hero":{"id":"1","name":null}}}`, out.String())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd v2 && go test ./pkg/engine/resolve/ -run TestResolvable_NullBehavior_TypeMismatch -v`
Expected: FAIL — today the type-mismatch bubbles (`data:null` at root, since `hero` is non-null).

- [ ] **Step 3: Convert the coercion sites**

For each coercion/invalid-value site, guard the existing `addError*(...)` call with `if !r.print { ... }` (if not already guarded) and replace the trailing `return r.err()` with `return r.erroredPosition(<PATH>, parent)`. Sites and their path arg:

- `walkObject` non-object branch (~line 707-709): `addError("Object cannot represent non-object value.", obj.Path)` → path `obj.Path`.
- `walkArray` non-array branch (~line 954-956): path `arr.Path`.
- `walkString` type mismatch (~line 1082-1086): path `s.Path`.
- `walkBoolean` type mismatch (~line 1128-1131): path `b.Path`.
- `walkInteger` type mismatch (~line 1149-1152): path `i.Path`.
- `walkFloat` type mismatch (~line 1170-1176, already `!r.print`-guarded): path `f.Path`.
- `walkCustom` resolve error (~line 1250-1252): path `c.Path`.
- `walkEnum` type mismatch (~line 1327-1330, add already writes with path) → replace `return r.err()` with `return r.erroredPosition(e.Path, parent)`.
- `walkEnum` invalid value (~line 1333-1365): the non-Apollo branch ends `return r.err()` — replace with `return r.erroredPosition(e.Path, parent)`; the error add is already guarded by `if !r.print`.

Example (walkString), before:
```go
	if value.Type() != astjson.TypeString {
		r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
		r.addError(fmt.Sprintf("String cannot represent non-string value: \"%s\"", string(r.marshalBuf)), s.Path)
		return r.err()
	}
```
after:
```go
	if value.Type() != astjson.TypeString {
		if !r.print {
			r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
			r.addError(fmt.Sprintf("String cannot represent non-string value: \"%s\"", string(r.marshalBuf)), s.Path)
		}
		return r.erroredPosition(s.Path, parent)
	}
```

- [ ] **Step 4: Run the test + full suite**

Run: `cd v2 && go test ./pkg/engine/resolve/ -run 'TestResolvable_NullBehavior' -v && go test ./pkg/engine/resolve/`
Expected: PASS, full `resolve` package green.

- [ ] **Step 5: Commit**

```bash
git add v2/pkg/engine/resolve/resolvable.go v2/pkg/engine/resolve/resolvable_test.go
git commit -m "feat(resolve): apply NULL error behavior to coercion and invalid-enum errors"
```

---

### Task 5: `HALT` behavior — data null with a single error

**Files:**
- Modify: `v2/pkg/engine/resolve/resolvable.go` (`Resolve` ~line 254-271; add `keepFirstErrorOnly` helper)
- Test: `v2/pkg/engine/resolve/resolvable_test.go`

**Interfaces:**
- Consumes: `r.errorBehavior` (Task 2).
- Produces: `func (r *Resolvable) keepFirstErrorOnly()` — trims `r.errors` to its first element.

**Background:** HALT is render-time only (no loader/fetch changes). Under HALT the walk behaves like PROPAGATE (any non-null violation bubbles to the root). We additionally force `data: null` whenever any error exists (including pre-existing subgraph errors) and trim the errors list to one.

- [ ] **Step 1: Write the failing tests**

```go
// append to v2/pkg/engine/resolve/resolvable_test.go

func newHaltResolvable(t *testing.T, data string) *Resolvable {
	t.Helper()
	res := NewResolvable(nil, ResolvableOptions{})
	err := res.Init(&Context{ExecutionOptions: ExecutionOptions{ErrorBehavior: ErrorBehaviorHalt}},
		[]byte(data), ast.OperationTypeQuery)
	assert.NoError(t, err)
	return res
}

// a single non-null violation -> data:null, single error
func TestResolvable_HaltBehavior_SingleViolation(t *testing.T) {
	res := newHaltResolvable(t, `{"hero":{"name":null}}`)
	object := &Object{Fields: []*Field{{
		Name: []byte("hero"),
		Value: &Object{Path: []string{"hero"}, Fields: []*Field{
			{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
		}},
	}}}
	out := &bytes.Buffer{}
	err := res.Resolve(context.Background(), object, nil, out)
	assert.NoError(t, err)
	assert.Equal(t, `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.hero.name'.","path":["hero","name"]}],"data":null}`, out.String())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd v2 && go test ./pkg/engine/resolve/ -run TestResolvable_HaltBehavior_SingleViolation -v`
Expected: FAIL — without HALT handling the error bubbles to `data:null` but the errors list is not guaranteed trimmed, and multi-error cases would differ (covered in Phase 3). For the single-error case it may already pass; if so, proceed — Step 3 makes multi-error correct and is verified in Phase 3.

- [ ] **Step 3: Add `keepFirstErrorOnly` and wire HALT into `Resolve`**

Add the helper near `printErrors`:

```go
// keepFirstErrorOnly trims r.errors down to its first element (used by HALT).
func (r *Resolvable) keepFirstErrorOnly() {
	if r.errors == nil {
		return
	}
	items := r.errors.GetArray()
	if len(items) <= 1 {
		return
	}
	first := items[0]
	r.errors = astjson.ArrayValue(r.astjsonArena)
	r.errors.SetArrayItem(r.astjsonArena, 0, first)
}
```

In `Resolve`, immediately after `hasErrors := r.walkObject(rootData, r.data)` (line ~254) and before `if r.authorizationError != nil`, add:

```go
	if r.errorBehavior == ErrorBehaviorHalt && r.hasErrors() {
		hasErrors = true // force data: null
		r.keepFirstErrorOnly()
	}
```

(The existing `if r.hasErrors() { r.printErrors() }` and `if hasErrors { data:null }` branches then render the single error and `data: null`.)

- [ ] **Step 4: Run the test + full suite**

Run: `cd v2 && go test ./pkg/engine/resolve/ -run 'TestResolvable_HaltBehavior' -v && go test ./pkg/engine/resolve/`
Expected: PASS, full `resolve` package green.

- [ ] **Step 5: Commit**

```bash
git add v2/pkg/engine/resolve/resolvable.go v2/pkg/engine/resolve/resolvable_test.go
git commit -m "feat(resolve): implement HALT error behavior (data null + single error)"
```

---

## Self-Review Notes

- **Coverage vs RFC §5.1–5.4:** Task 1 = enum/helper (§5.1); Task 2 = plumbing/normalization (§5.2); Tasks 3–4 = NULL semantics incl. option A (§5.3); Task 5 = HALT single-error, render-time (§5.4).
- **Out of scope for Phase 1 (documented, not deferred silently):** authorization-denial null-setting (`walkObject` ~lines 769-786) keeps its existing behavior; introspection capabilities are Phase 2; the full behavior×position test matrix and pipeline/e2e tests are Phase 3.
- **Regression guard:** every task ends by running the full `resolve` package so any `PROPAGATE` drift fails immediately.
