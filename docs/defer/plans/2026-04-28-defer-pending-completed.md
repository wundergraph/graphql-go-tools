# Defer Incremental Delivery: pending / incremental / completed

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring the streamed defer response format in line with the GraphQL incremental-delivery spec (matching graphql-js's reference implementation), with one deliberate simplification noted below:

- Initial response declares **every** `@defer` in the operation under a single `pending` array — both top-level and nested. (The spec releases nested defers later, alongside the parent's `completed` event; we go simpler for this iteration. See "Deviation from spec" below.)
- Each subsequent response carries `incremental` (data items with an `id`) and `completed` (defers that finished in this payload), plus `hasNext`. **No `pending` is ever emitted in subsequent responses.**
- **Labels move from each `incremental` envelope to the `pending` entry** — they're metadata about the defer, not about the data slice.

### Deviations from spec

Two simplifications relative to the graphql-js reference implementation:

**1. All defers in initial `pending`.** The spec calls for nested defers to be announced lazily — entering `pending` only on the subsequent response that completes their parent (see investigation notes; `IncrementalPublisher.ts:203-207`, "Initiates deferred grouped field sets only if they have been released as pending" test). Implementing that requires a parent-success tracking machinery in our resolver that we don't have today. Putting every defer in initial `pending` is a strict superset of information (clients learn about them sooner) and simplifies the implementation substantially. The spec doesn't forbid early disclosure of nested defers; the lazy form is the reference implementation's choice, not a wire-format requirement, and our envelope is still parseable by spec-conformant clients.

**2. One id per AST `@defer`, not per list-item application.** The spec's reference implementation allocates a fresh id for every runtime application of a deferred fragment, so a `@defer` mounted inside a list field produces N pending entries with indexed paths (`['hero', 'friends', 0]`, `[..., 1]`, `[..., 2]`) — each list item is an independent unit of progress (`defer-test.ts:1198-1216`, `IncrementalPublisher.ts:93` `_ensureId`). We do not have that capability: our defer ids are allocated at plan time per AST defer, and our `DeferFetchGroup` covers a single network fetch returning all list items' deferred fields together. We emit one `pending` entry per AST defer, with a path that stops at the field level (no integer indices). Consequently `DeferDescriptor.Path` is `[]string` (no list indices). All items' deferred data lands in a single `incremental` envelope. This matches the granularity of our subgraph-fetch architecture and does not lose information clients can act on.

Both are tagged for follow-up under "Out of scope".

## Reference format (from graphql-js defer-test.ts)

Initial response:
```json
{
  "data": { "hero": { "id": "1" } },
  "pending": [
    { "id": "0", "path": ["hero"], "label": "DeferTop" }
  ],
  "hasNext": true
}
```

Subsequent (single payload, in our simplified model — single defer group per payload):
```json
{
  "incremental": [
    { "data": { "name": "Luke" }, "id": "0" }
  ],
  "completed": [{ "id": "0" }],
  "hasNext": true
}
```

Initial response when nested defers exist (note: BOTH the outer defer `id: 0` AND the nested defer `id: 1` are listed in initial pending; spec would only list `id: 0` here and announce `id: 1` later — see Deviation):
```json
{
  "data": { "hero": {} },
  "pending": [
    { "id": "0", "path": ["hero"], "label": "DeferTop" },
    { "id": "1", "path": ["hero"], "label": "DeferNested" }
  ],
  "hasNext": true
}
```

Key shape rules:
- `pending` entry: `{ id: string, path: string[], label?: string }`. `label` is omitted (not empty-string) when no label was supplied.
- `incremental` entry: `{ data: object, id: string, errors?: [...] }`. **No `path`, no `label`, no `subPath`** on individual items.
- `completed` entry: `{ id: string }` (success) or `{ id: string, errors: [...] }` (failure). The recoverable-vs-non-recoverable error split is the spec's: recoverable errors stay in `incremental[].errors`, non-recoverable (null propagated to fragment root) skip `incremental` entirely and surface in `completed[].errors`.
- `id` is always a string. Our internal `int` defer ids stringify directly (e.g. `1` → `"1"`).
- `subPath` (graphql-js spec field for split-data envelopes) is **out of scope** — explicitly not implemented. Path precompute (Task 4) makes future implementation mechanical, but no `subPath` keys are produced or consumed in this iteration.

### Error semantics (both forms supported)

Two distinct error paths, drawn from the spec:

**Recoverable error** — a nullable field within the deferred fragment errored. The fragment still delivers, with the errored field set to `null` and the error attached to the `incremental` item:
```json
{
  "incremental": [
    { "data": { "hero": { "name": null } }, "errors": [{ "message": "bad", ... }], "id": "0" }
  ],
  "completed": [{ "id": "0" }],
  "hasNext": false
}
```

**Non-recoverable error** — a non-nullable field errored and the null propagated to the fragment root, invalidating the entire fragment. **No `incremental` item is emitted.** The error is reported in `completed[].errors`:
```json
{
  "completed": [
    {
      "id": "1",
      "errors": [{ "message": "Cannot return null for non-nullable field …", ... }]
    }
  ],
  "hasNext": true
}
```

Both forms must be supported. The resolver already knows the fragment outcome (the same null-propagation rule that decides whether the response data envelope contains `data` or `null`). The new wrinkle: route errors to either `incremental[].errors` (recoverable) or `completed[].errors` (non-recoverable) based on whether the fragment root survived null-propagation.

## Current state in this repo

- Single envelope per defer group: `{"incremental":[{"data":...,"path":[...],"label":"...","errors":[...]}],"hasNext":...}`.
- Initial response has no `pending`. No `completed` ever emitted.
- Labels are looked up from `GraphQLDeferResponse.DeferLabels` (refactor we just landed) and rendered alongside `path` in each incremental item.
- Defer ids and labels are known at plan time. Defer **paths** (response-path of the deferred fragment) are NOT collected today; the resolver only knows path while it's walking the response tree.

## Defer paths must be precomputed

Today, response paths only exist while the resolver walks. To emit `pending` in the initial response we need each defer's path **before resolution starts**. We compute it once during the same `prepareOperationWalker` pass that already gathers labels.

**Rule.** The deferred fragment's response path equals the field-alias-or-name chain from the operation root down to the field that immediately encloses the inline fragment. Equivalently, for the **first** field in document order carrying a given `@__defer_internal(id: X)` directive, the defer's path is that field's ancestor field names (excluding the field itself).

- `hero { ... @defer { name } }`: visitor enters `name` first (and only) field with id 1; ancestors-excluding-current = `["hero"]`. Defer path = `["hero"]`. ✓
- `... @defer { hero { name } }`: visitor enters `hero` first (id 1 attached to BOTH `hero` and `name`); ancestors-excluding-current = `[]`. Defer path = `[]`. ✓
- `hero { ... @defer { friends { ... @defer { name } } } }`: visitor enters `friends` first for id 1 (path `["hero"]`); then `name` first for id 2 (path `["hero","friends"]`). ✓

The "first occurrence wins" rule works because the inline-fragment expander applies `@__defer_internal` to every field within the fragment (including nested fields), and document-order traversal visits the topmost deferred field of each fragment before any of its descendants. The collector records each id once and ignores subsequent fields with the same id.

This precompute also enables two later capabilities (out of scope here, but explicitly unlocked):

1. **`subPath` rendering** — once we have the defer's `path` plus the resolver's current path, the suffix is mechanical to compute.
2. **Defer-walk pruning** — knowing each defer's path in advance lets the per-defer-group walk skip subtrees that aren't on the path, reducing iterations when looking for fields belonging to a specific defer.

## Approach

1. Extend the planning-time collection to capture, per defer id, a `DeferDescriptor { ID int, Label string, Path []string }`. Build during `prepareOperationWalker` in the same pass that already collects labels. (No `ParentID` needed — see below.)
2. Replace `GraphQLDeferResponse.DeferLabels map[int]string` with `Defers []DeferDescriptor` (ordered by id, ascending). The resolver looks up by id at render time.
3. Re-shape the resolver's render entry points:
   - **Initial**: After `data`, emit `pending: [...]` listing **every** descriptor (regardless of nesting). Order by id ascending.
   - **Per defer group, success path**: Emit `incremental: [{ data, id, errors? }]` (errors here are recoverable / nullable-field errors); emit `completed: [{ id }]`; then `hasNext`. **No `pending`.**
   - **Per defer group, fragment-root failure**: **Omit `incremental` entirely**; emit `completed: [{ id, errors: [...] }]`; then `hasNext`. **No `pending`.**
4. Drop `path`/`label` writes from per-item rendering (fold/rename `printDeferPathAndErrors`). The per-item envelope shrinks to `{ data, id, errors? }` and is omitted entirely on root failure.
5. Update integration tests in `execution/engine/execution_engine_defer_test.go` to the new shape (every `expectedResponse` will change). Add at least two new test cases — one for recoverable error, one for non-recoverable — to lock in the routing.

(Out of scope and Verification checklist appear at the end of the document.)

**Tech Stack:** Go 1.21+, graphql-go-tools v2, gotestsum.

---

## Implementation order

The compile state is intentionally allowed to be broken between Tasks 3–8; once Task 8 lands the build is green again. Task 9 cleans up remaining call sites of removed helpers. Task 10 (integration tests) is last per agreement.

---

## Task 1: AST — add combined `FieldDeferInfo` helper

**File:** `v2/pkg/ast/ast_field.go`

Today there are two helpers walking the field's directive list independently: `FieldInternalDeferID(ref)` and `FieldInternalDeferLabel(ref)`. We replace them with a single combined helper that walks once and returns id + label + parentID.

### Step 1.1: Add `FieldDeferInfo`

- [ ] Add a new method that returns all three values from a single directive lookup. Keep `FieldInternalDeferID` / `FieldInternalDeferLabel` in place for now — they'll be removed in Task 9 once all callers are migrated.

**Edit (anchor on the closing brace of `FieldInternalDeferLabel`):**

`old_string`:
```
	return d.StringValueContentString(labelValue.Ref), true
}
```

`new_string`:
```
	return d.StringValueContentString(labelValue.Ref), true
}

// FieldDeferInfo reads the @__defer_internal directive on the given field and
// returns its (id, label, parentID) triple in a single directive lookup.
// `exists` is true iff the directive is present and `id` argument is a
// non-zero integer. `label` may be empty (no label argument or empty string);
// `parentID` is 0 when the directive has no `parentDeferId` argument.
func (d *Document) FieldDeferInfo(fieldRef int) (id int, label string, parentID int, exists bool) {
	directiveRef, ok := d.Fields[fieldRef].Directives.HasDirectiveByNameBytes(d, literal.DEFER_INTERNAL)
	if !ok {
		return 0, "", 0, false
	}
	idValue, idOK := d.DirectiveArgumentValueByName(directiveRef, []byte("id"))
	if !idOK || idValue.Kind != ValueKindInteger {
		return 0, "", 0, false
	}
	id = int(d.IntValueAsInt(idValue.Ref))
	if id == 0 {
		return 0, "", 0, false
	}
	if labelValue, lOK := d.DirectiveArgumentValueByName(directiveRef, []byte("label")); lOK && labelValue.Kind == ValueKindString {
		label = d.StringValueContentString(labelValue.Ref)
	}
	if parentValue, pOK := d.DirectiveArgumentValueByName(directiveRef, []byte("parentDeferId")); pOK && parentValue.Kind == ValueKindInteger {
		parentID = int(d.IntValueAsInt(parentValue.Ref))
	}
	return id, label, parentID, true
}
```

- [ ] Run: `gofmt -w v2/pkg/ast/ast_field.go`
- [ ] Run: `gotestsum --format=short -- ./v2/pkg/ast/... -count=1`

---

## Task 2: Add new envelope literals

**File:** `v2/pkg/engine/resolve/const.go`

### Step 2.1: Add `pending`, `completed`, `id`

- [ ] Add three new literal byte slices alongside the existing ones.

**Edit:**

`old_string`:
```
	literalHasNext            = []byte("hasNext")
	literalLabel              = []byte("label")
```

`new_string`:
```
	literalHasNext            = []byte("hasNext")
	literalLabel              = []byte("label")
	literalPending            = []byte("pending")
	literalCompleted          = []byte("completed")
	literalId                 = []byte("id")
```

- [ ] Run: `gofmt -w v2/pkg/engine/resolve/const.go`

---

## Task 3: Define `DeferDescriptor` on `GraphQLDeferResponse`

**File:** `v2/pkg/engine/resolve/response.go`

### Step 3.1: Replace `DeferLabels` with `DeferDescriptors`

- [ ] Drop `DeferLabels map[int]string`. Add `DeferDescriptors map[int]DeferDescriptor`. Add the `DeferDescriptor` struct.

**Edit:**

`old_string`:
```
type GraphQLDeferResponse struct {
	Response    *GraphQLResponse
	Defers      []*DeferFetchGroup
	DeferLabels map[int]string
}
```

`new_string`:
```
type GraphQLDeferResponse struct {
	Response *GraphQLResponse
	Defers   []*DeferFetchGroup

	// DeferDescriptors lists every @defer fragment in the operation, keyed by ID.
	// Used to render `pending` entries in the initial response and to look up the
	// path / label of a defer at envelope-render time.
	DeferDescriptors map[int]DeferDescriptor
}

// DeferDescriptor describes a single @defer fragment for the incremental-delivery
// envelope. Path is the response path of the fragment (where it was mounted in
// the operation); Label is the user-supplied label (empty when none); ParentID
// is the id of the enclosing @defer (0 for top-level).
type DeferDescriptor struct {
	ID       int
	ParentID int
	Label    string
	Path     []string
}
```

- [ ] Run: `gofmt -w v2/pkg/engine/resolve/response.go`

After this task, packages that read `response.DeferLabels` will stop compiling. Fixed in Tasks 7 and 8.

---

## Task 4: Rewrite the defer info collector

**Files:**
- `v2/pkg/engine/plan/defer_label_collector.go` → rename to `v2/pkg/engine/plan/defer_info_collector.go`

### Step 4.1: Rename the file

- [ ] `git mv v2/pkg/engine/plan/defer_label_collector.go v2/pkg/engine/plan/defer_info_collector.go`

### Step 4.2: Rewrite the collector

- [ ] Replace the file contents. Type renamed to `deferInfoCollector`. Reads via combined `FieldDeferInfo`. Computes path via `walker.Path` filtered to `FieldName` items (excluding the current field). Captures `ParentID`.

**Edit (whole-file replacement):**

`old_string`:
```
package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

// deferLabelCollector walks all fields of an operation and records the
// user-supplied defer label keyed by defer id. It reads from the
// @__defer_internal directives that the inline_fragment_expand_defer
// normalizer placed on every field of a deferred fragment.
//
// Multiple fields share the same (id, label) pair (the normalizer applies
// the same directive to every field in a deferred fragment); the collector
// records the first non-empty label it encounters and ignores subsequent
// duplicates. Fields with empty labels are skipped — a missing entry in
// the resulting map means "no label was supplied".
type deferLabelCollector struct {
	operation *ast.Document
	labels    map[int]string
}

func registerDeferLabelCollector(walker *astvisitor.Walker) *deferLabelCollector {
	c := &deferLabelCollector{}
	walker.RegisterEnterDocumentVisitor(c)
	walker.RegisterEnterFieldVisitor(c)
	return c
}

func (c *deferLabelCollector) EnterDocument(operation, _ *ast.Document) {
	c.operation = operation
	c.labels = make(map[int]string)
}

func (c *deferLabelCollector) EnterField(ref int) {
	id, _ := c.operation.FieldInternalDeferID(ref)
	if id == 0 {
		return
	}
	if _, seen := c.labels[id]; seen {
		return
	}
	label, _ := c.operation.FieldInternalDeferLabel(ref)
	if label == "" {
		return
	}
	c.labels[id] = label
}
```

`new_string`:
```
package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// deferInfoCollector walks every field of an operation and records the
// per-defer-id descriptor (label, response path, parent id) by reading from
// the @__defer_internal directives that the inline_fragment_expand_defer
// normalizer placed on every field of a deferred fragment.
//
// Multiple fields within the same deferred fragment carry identical
// (id, label, parentID) and the same enclosing-fragment path; the collector
// records the first occurrence per id (in document order) and ignores the
// rest. The first deferred field encountered in document order is the
// topmost field of its fragment, so its enclosing-Field-ancestor chain
// equals the response-path where the @defer fragment was mounted.
type deferInfoCollector struct {
	*astvisitor.Walker
	operation   *ast.Document
	descriptors map[int]resolve.DeferDescriptor
}

func registerDeferInfoCollector(walker *astvisitor.Walker) *deferInfoCollector {
	c := &deferInfoCollector{Walker: walker}
	walker.RegisterEnterDocumentVisitor(c)
	walker.RegisterEnterFieldVisitor(c)
	return c
}

func (c *deferInfoCollector) EnterDocument(operation, _ *ast.Document) {
	c.operation = operation
	c.descriptors = make(map[int]resolve.DeferDescriptor)
}

func (c *deferInfoCollector) EnterField(ref int) {
	id, label, parentID, ok := c.operation.FieldDeferInfo(ref)
	if !ok {
		return
	}
	if _, seen := c.descriptors[id]; seen {
		return
	}
	c.descriptors[id] = resolve.DeferDescriptor{
		ID:       id,
		ParentID: parentID,
		Label:    label,
		Path:     c.deferPath(),
	}
}

func (c *deferInfoCollector) deferPath() []string {
	path := c.Walker.Path
	if len(path) == 0 {
		return nil
	}
	out := make([]string, 0, len(path))
	for _, item := range path {
		if item.Kind != ast.FieldName {
			continue
		}
		out = append(out, string(item.FieldName))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
```

- [ ] Run: `gofmt -w v2/pkg/engine/plan/defer_info_collector.go`

---

## Task 5: Wire collector through `Planner`

**File:** `v2/pkg/engine/plan/planner.go`

### Step 5.1: Rename field, registration, and handoff

- [ ] Three small edits.

**Edit A (struct field):**

`old_string`:
```
	prepareOperationWalker *astvisitor.Walker
	deferLabelCollector    *deferLabelCollector
}
```

`new_string`:
```
	prepareOperationWalker *astvisitor.Walker
	deferInfoCollector     *deferInfoCollector
}
```

**Edit B (registration):**

`old_string`:
```
	deferLabelCollector := registerDeferLabelCollector(&prepareOperationWalker)
```

`new_string`:
```
	deferInfoCollector := registerDeferInfoCollector(&prepareOperationWalker)
```

**Edit C (Planner literal):**

`old_string`:
```
		prepareOperationWalker: &prepareOperationWalker,
		deferLabelCollector:    deferLabelCollector,
	}
```

`new_string`:
```
		prepareOperationWalker: &prepareOperationWalker,
		deferInfoCollector:     deferInfoCollector,
	}
```

**Edit D (handoff in `Plan()`):**

`old_string`:
```
	p.planningVisitor.deferLabels = p.deferLabelCollector.labels
```

`new_string`:
```
	p.planningVisitor.deferDescriptors = p.deferInfoCollector.descriptors
```

- [ ] Run: `gofmt -w v2/pkg/engine/plan/planner.go`

---

## Task 6: Visitor — carry descriptors and attach to response

**File:** `v2/pkg/engine/plan/visitor.go`

### Step 6.1: Replace field

- [ ] Drop `deferLabels`, add `deferDescriptors`.

**Edit:**

`old_string`:
```
	deferLabels                  map[int]string
```

`new_string`:
```
	deferDescriptors             map[int]resolve.DeferDescriptor
```

### Step 6.2: Attach in `LeaveDocument`

- [ ] In the `DeferResponsePlan` literal, replace `DeferLabels: ...` with `DeferDescriptors: ...`.

**Edit:**

`old_string`:
```
		v.plan = &DeferResponsePlan{
			Response: &resolve.GraphQLDeferResponse{
				Response:    v.response,
				DeferLabels: v.deferLabels,
			},
		}
```

`new_string`:
```
		v.plan = &DeferResponsePlan{
			Response: &resolve.GraphQLDeferResponse{
				Response:         v.response,
				DeferDescriptors: v.deferDescriptors,
			},
		}
```

- [ ] Run: `gofmt -w v2/pkg/engine/plan/visitor.go`

---

## Task 7: Resolvable — pending emission, ResolveDefer rewrite, dead-code removal

**File:** `v2/pkg/engine/resolve/resolvable.go`

This is the largest task. Three concerns:
1. Replace per-Resolvable `deferLabel` field with `deferDescriptors` map.
2. Add `printPendingEntries`, `printPathArray`, `printDeferIdAndErrors`. Update `printDeferEnvelopeClose` to use the new helper. Drop `printDeferEnvelopeNullData`.
3. Modify `Resolve()` to emit `pending` between extensions and `hasNext`. Rewrite `ResolveDefer()` for the new envelope shape.
4. Drop the dead `if r.deferItemDataNull → printDeferEnvelopeNullData` branch inside the walker.

### Step 7.1: Field swap on `Resolvable`

**Edit A (struct field):**

`old_string`:
```
	deferMode           bool
	deferID             int
	deferLabel          string
```

`new_string`:
```
	deferMode           bool
	deferID             int
	deferDescriptors    map[int]DeferDescriptor
```

**Edit B (in `Reset()`):**

`old_string`:
```
	r.deferMode = false
	r.deferID = 0
	r.deferLabel = ""
	r.enableDeferRender = false
```

`new_string`:
```
	r.deferMode = false
	r.deferID = 0
	r.deferDescriptors = nil
	r.enableDeferRender = false
```

### Step 7.2: New helpers — `printPendingEntries`, `printPathArray`, `printDeferIdAndErrors`

- [ ] Add three helpers. `printPendingEntries` sorts ids ascending before iterating (deterministic JSON order). `printPathArray` writes a `[]string` as a JSON string array. `printDeferIdAndErrors` is the new per-incremental-item suffix.

**Edit (anchor on the closing brace of `renderPath`):**

`old_string`:
```
func (r *Resolvable) renderPath() {
	r.printBytes(lBrack)
	for i, p := range r.path {
		if i > 0 {
			r.printBytes(comma)
		}
		if p.Name != "" {
			r.printBytes(quote)
			r.printBytes(unsafebytes.StringToBytes(p.Name))
			r.printBytes(quote)
		} else {
			r.printBytes(unsafebytes.StringToBytes(strconv.Itoa(p.Idx)))
		}
	}
	r.printBytes(rBrack)
}
```

`new_string`:
```
func (r *Resolvable) renderPath() {
	r.printBytes(lBrack)
	for i, p := range r.path {
		if i > 0 {
			r.printBytes(comma)
		}
		if p.Name != "" {
			r.printBytes(quote)
			r.printBytes(unsafebytes.StringToBytes(p.Name))
			r.printBytes(quote)
		} else {
			r.printBytes(unsafebytes.StringToBytes(strconv.Itoa(p.Idx)))
		}
	}
	r.printBytes(rBrack)
}

// printPendingEntries writes `,"pending":[...]` listing every descriptor in
// the map, sorted by id ascending. Writes nothing if the map is empty/nil.
func (r *Resolvable) printPendingEntries(descriptors map[int]DeferDescriptor) {
	if len(descriptors) == 0 {
		return
	}
	ids := make([]int, 0, len(descriptors))
	for id := range descriptors {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	r.printBytes(comma)
	r.printBytes(quote)
	r.printBytes(literalPending)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(lBrack)
	for i, id := range ids {
		if i > 0 {
			r.printBytes(comma)
		}
		d := descriptors[id]
		r.printBytes(lBrace)
		// "id":"<n>"
		r.printBytes(quote)
		r.printBytes(literalId)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printBytes(quote)
		r.printBytes([]byte(strconv.Itoa(d.ID)))
		r.printBytes(quote)
		// "path":[...]
		r.printBytes(comma)
		r.printBytes(quote)
		r.printBytes(literalPath)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printPathArray(d.Path)
		// "label":"<l>"  — only if non-empty
		if d.Label != "" {
			r.printBytes(comma)
			r.printBytes(quote)
			r.printBytes(literalLabel)
			r.printBytes(quote)
			r.printBytes(colon)
			r.printBytes(strconv.AppendQuote(nil, d.Label))
		}
		r.printBytes(rBrace)
	}
	r.printBytes(rBrack)
}

// printPathArray writes a precomputed []string path as a JSON string array.
func (r *Resolvable) printPathArray(path []string) {
	r.printBytes(lBrack)
	for i, segment := range path {
		if i > 0 {
			r.printBytes(comma)
		}
		r.printBytes(strconv.AppendQuote(nil, segment))
	}
	r.printBytes(rBrack)
}
```

NOTE: `sort` package may not yet be imported in `resolvable.go`. If absent, add `"sort"` to the import block.

### Step 7.3: Replace `printDeferPathAndErrors` with `printDeferIdAndErrors`

- [ ] Rewrite the function. New shape: `"id":"<n>","errors":[...]?`. Manual quoting for the digit-only id; errors via existing `r.printNode`.

**Edit:**

`old_string`:
```
func (r *Resolvable) printDeferPathAndErrors() {
	r.printBytes(quote)
	r.printBytes(literalPath)
	r.printBytes(quote)
	r.printBytes(colon)
	r.renderPath()
	if r.deferLabel != "" {
		r.printBytes(comma)
		r.printBytes(quote)
		r.printBytes(literalLabel)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printBytes(strconv.AppendQuote(nil, r.deferLabel))
	}
	if r.hasErrors() {
		r.printBytes(comma)
		r.printBytes(quote)
		r.printBytes(literalErrors)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printNode(r.errors)
	}
}
```

`new_string`:
```
// printDeferIdAndErrors writes "id":"<n>" optionally followed by
// ,"errors":[...] when recoverable errors are pending on this incremental item.
func (r *Resolvable) printDeferIdAndErrors() {
	r.printBytes(quote)
	r.printBytes(literalId)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(quote)
	r.printBytes([]byte(strconv.Itoa(r.deferID)))
	r.printBytes(quote)
	if r.hasErrors() {
		r.printBytes(comma)
		r.printBytes(quote)
		r.printBytes(literalErrors)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printNode(r.errors)
	}
}
```

### Step 7.4: Update `printDeferEnvelopeClose` to use the new helper

**Edit:**

`old_string`:
```
func (r *Resolvable) printDeferEnvelopeClose() {
	if !r.render() {
		return
	}

	r.printBytes(rBrace)
	r.printBytes(comma)
	r.printDeferPathAndErrors()
	r.printBytes(rBrace)
}
```

`new_string`:
```
func (r *Resolvable) printDeferEnvelopeClose() {
	if !r.render() {
		return
	}

	r.printBytes(rBrace)
	r.printBytes(comma)
	r.printDeferIdAndErrors()
	r.printBytes(rBrace)
}
```

### Step 7.5: Delete `printDeferEnvelopeNullData`

The function is no longer reachable.

**Why it's only the non-recoverable case (and why it can't fire for nullable+error):** `printDeferEnvelopeNullData` is invoked exclusively when `r.deferItemDataNull` is true (today, at `resolvable.go:828`). That flag is set in exactly one place (`resolvable.go:847`): during the **pre-walk** (`!r.enableRender`), when `walkObject` returns `hasErrors=true`. The walker only returns `hasErrors=true` for **null-propagation through a non-nullable chain** — the case where the deferred fragment root cannot deliver any data.

A nullable field that errors is the recoverable case: the field renders as `null`, the error lands in `r.errors`, but `walkObject` returns `hasErrors=false` for the fragment as a whole. `deferItemDataNull` stays false, the regular envelope path runs, and the resolver emits `{data: {…with null fields…}, errors: [...], id: ...}` — exactly the recoverable spec shape. So `printDeferEnvelopeNullData` is **never** reached in the nullable+error path.

In the new envelope shape, the non-recoverable case skips `incremental` entirely and emits errors via `completed[].errors` (Step 7.8). The function has no callers and is deleted.

**Edit:**

`old_string`:
```
func (r *Resolvable) printDeferEnvelopeNullData() {
	if !r.render() {
		return
	}
	r.printBytes(lBrace)
	r.printBytes(quote)
	r.printBytes(literalData)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(null)
	r.printBytes(comma)
	r.printDeferPathAndErrors()
	r.printBytes(rBrace)
}

```

`new_string`: (empty — delete the function and its trailing blank line)

### Step 7.6: Drop the failure branch in `walkObject`

The `if r.deferItemDataNull → printDeferEnvelopeNullData` block is dead code once `ResolveDefer` skips the render walk on failure (Step 7.7). Remove it; the open path remains.

**Edit (anchor on the unique block at lines ~827-837):**

`old_string`:
```
				if r.deferID != 0 {
					if r.deferItemDataNull {
						// Pre-walk detected null propagating through non-nullable chain;
						// render {"data":null,"path":[...],"errors":[...]} without walking fields.
						r.printDeferEnvelopeNullData()
						r.incrementalItemWritten = true
						r.enableDeferRender = false
						return true
					}
					r.printDeferEnvelopeOpen()
				}
```

`new_string`:
```
				if r.deferID != 0 {
					r.printDeferEnvelopeOpen()
				}
```

### Step 7.7: `Resolve()` — emit `pending` before `hasNext`

**Edit:**

`old_string`:
```
	if r.deferMode && !r.hasErrors() {
		r.printHasNext(true)
	}
```

`new_string`:
```
	if r.deferMode && !r.hasErrors() {
		r.printPendingEntries(r.deferDescriptors)
		r.printHasNext(true)
	}
```

### Step 7.8: Rewrite `ResolveDefer()` for the new envelope shape

Today's `ResolveDefer` always writes `{"incremental":[...],"hasNext":...}`. The new shape branches on `r.deferItemDataNull` (set during pre-walk):
- **Failure**: `{"completed":[{"id":"<n>","errors":[...]}],"hasNext":...}` — no `incremental`.
- **Success**: `{"incremental":[<rendered>],"completed":[{"id":"<n>"}],"hasNext":...}`.

`hasNext` is no longer gated on `!r.hasErrors()` — defer-internal errors are scoped to the failed defer and don't terminate the response.

**Edit:**

`old_string`:
```
func (r *Resolvable) ResolveDefer(rootData *Object, out io.Writer, hasNext bool) error {
	r.out = out
	r.printErr = nil
	r.authorizationError = nil

	// This method acts as a generator for the incremental response
	// It will print the incremental response envelope and then use walkObject to find and render the deferred fields

	// First pass: validate and check for authorization errors
	r.enableRender = false
	r.deferMode = true
	r.enableDeferRender = false
	r.incrementalItemWritten = false
	r.deferItemDataNull = false

	_ = r.walkObject(rootData, r.data)
	if r.authorizationError != nil {
		return r.authorizationError
	}

	// Second pass: render the incremental response
	r.enableRender = true
	r.incrementalItemWritten = false
	r.enableDeferRender = false // reset: first pass may have left it true on early return

	r.printBytes(lBrace)
	r.printBytes(quote)
	r.printBytes(literalIncremental)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(lBrack)

	_ = r.walkObject(rootData, r.data)

	r.printBytes(rBrack)

	r.printHasNext(hasNext && !r.hasErrors())

	r.printBytes(rBrace)

	return r.printErr
}
```

`new_string`:
```
func (r *Resolvable) ResolveDefer(rootData *Object, out io.Writer, hasNext bool) error {
	r.out = out
	r.printErr = nil
	r.authorizationError = nil

	// First pass (pre-walk): validate, collect errors, decide whether the
	// fragment root survived null-propagation. r.deferItemDataNull is set
	// inside walkObject when null propagated through a non-nullable chain.
	r.enableRender = false
	r.deferMode = true
	r.enableDeferRender = false
	r.incrementalItemWritten = false
	r.deferItemDataNull = false

	_ = r.walkObject(rootData, r.data)
	if r.authorizationError != nil {
		return r.authorizationError
	}

	fragmentInvalid := r.deferItemDataNull

	// Open the per-defer envelope.
	r.printBytes(lBrace)

	if !fragmentInvalid {
		// Second pass: render incremental data.
		r.printBytes(quote)
		r.printBytes(literalIncremental)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printBytes(lBrack)

		r.enableRender = true
		r.incrementalItemWritten = false
		r.enableDeferRender = false

		_ = r.walkObject(rootData, r.data)

		r.printBytes(rBrack)
		r.printBytes(comma)
	}

	// Always emit completed for this defer id.
	r.printBytes(quote)
	r.printBytes(literalCompleted)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(lBrack)
	r.printBytes(lBrace)
	// "id":"<n>"
	r.printBytes(quote)
	r.printBytes(literalId)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(quote)
	r.printBytes([]byte(strconv.Itoa(r.deferID)))
	r.printBytes(quote)
	if fragmentInvalid && r.hasErrors() {
		r.printBytes(comma)
		r.printBytes(quote)
		r.printBytes(literalErrors)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printNode(r.errors)
	}
	r.printBytes(rBrace)
	r.printBytes(rBrack)

	// hasNext is independent of internal defer errors — they're scoped
	// to this defer's `completed.errors` and do not terminate the response.
	r.printHasNext(hasNext)

	r.printBytes(rBrace)

	return r.printErr
}
```

- [ ] Run: `gofmt -w v2/pkg/engine/resolve/resolvable.go`

---

## Task 8: Resolve loop — drop early-return, reset errors per iteration

**File:** `v2/pkg/engine/resolve/resolve.go`

### Step 8.1: Initial render setup

- [ ] Replace the `deferLabel = ""` line with `deferDescriptors = response.DeferDescriptors`.

**Edit:**

`old_string`:
```
		t.resolvable.deferMode = true
		t.resolvable.deferID = 0
		t.resolvable.deferLabel = ""
```

`new_string`:
```
		t.resolvable.deferMode = true
		t.resolvable.deferID = 0
		t.resolvable.deferDescriptors = response.DeferDescriptors
```

### Step 8.2: Per-defer loop changes

- [ ] Drop `t.resolvable.deferLabel = response.DeferLabels[deferGroup.DeferID]`.
- [ ] Drop the early-return on per-defer errors (we want the response to continue across defer-internal failures).
- [ ] Reset `t.resolvable.errors` between iterations so a previous defer's errors don't leak into the next one's `completed`.

**Edit (whole loop body):**

`old_string`:
```
		for i, deferGroup := range response.Defers {
			if err := t.loader.ResolveFetchNode(deferGroup.Fetches); err != nil {
				return nil, err
			}

			t.resolvable.deferID = deferGroup.DeferID
			t.resolvable.deferLabel = response.DeferLabels[deferGroup.DeferID]

			err = t.resolvable.ResolveDefer(response.Response.Data, writer, i < len(response.Defers)-1)
			if err != nil {
				return nil, err
			}

			// flush after each deferred response

			err = writer.Flush()
			if err != nil {
				return nil, err
			}

			if t.resolvable.hasErrors() {
				return resolveInfo, nil
			}
		}
```

`new_string`:
```
		for i, deferGroup := range response.Defers {
			// Reset per-iteration error state. Errors collected during one
			// deferred fragment must NOT leak into the next iteration's
			// completed.errors envelope.
			t.resolvable.errors = nil

			if err := t.loader.ResolveFetchNode(deferGroup.Fetches); err != nil {
				return nil, err
			}

			t.resolvable.deferID = deferGroup.DeferID

			err = t.resolvable.ResolveDefer(response.Response.Data, writer, i < len(response.Defers)-1)
			if err != nil {
				return nil, err
			}

			// flush after each deferred response

			err = writer.Flush()
			if err != nil {
				return nil, err
			}

			// Defer-internal errors (recoverable or non-recoverable) are
			// scoped to this defer's envelope and do NOT terminate the
			// response. Continue to the next defer regardless.
		}
```

- [ ] Run: `gofmt -w v2/pkg/engine/resolve/resolve.go`

After this task the build should be green again (assuming Task 9 has not yet run; old AST helpers still exist as fallback).

---

## Task 9: Migrate label callers, delete obsolete `FieldInternalDeferLabel`

`FieldInternalDeferID` stays — multiple call sites need just the id (parent-id lookups in `defer_ensure_typename.go`, `wrappingFieldDeferID` in `node_selection_visitor.go`, scope-checks in the abstract-selection rewriter). `FieldInternalDeferLabel` retires: any caller that wants the label also wants the id (and possibly parent id), and that's exactly what `FieldDeferInfo` (Task 1) returns in one call.

### Step 9.1: Inventory `FieldInternalDeferLabel` call sites

- [ ] Run: `grep -rn "FieldInternalDeferLabel" v2/ execution/`

Expected hits after Task 4:
- `v2/pkg/engine/plan/abstract_selection_rewriter.go` and helpers / info — reads label alongside id when handling typename rewrites.
- (Any others surfaced by grep)

### Step 9.2: Migrate each `FieldInternalDeferLabel` caller to `FieldDeferInfo`

- [ ] For each call site that today reads `FieldInternalDeferID` followed by `FieldInternalDeferLabel` (or vice versa) on the same field, fold both into a single `FieldDeferInfo` call. For sites that read ONLY the id, leave them on `FieldInternalDeferID`. Example:

`old_string`:
```
	deferID, _ := r.operation.FieldInternalDeferID(fieldRef)
	deferLabel, _ := r.operation.FieldInternalDeferLabel(fieldRef)
```

`new_string`:
```
	deferID, deferLabel, _, _ := r.operation.FieldDeferInfo(fieldRef)
```

(Sites that need the parent id alongside also use `FieldDeferInfo`'s third return value.)

### Step 9.3: Verify and delete `FieldInternalDeferLabel`

- [ ] Run `grep -rn "FieldInternalDeferLabel" v2/ execution/` again — must return zero hits before deletion.
- [ ] Delete the helper.

**Edit (delete `FieldInternalDeferLabel`):**

`old_string`:
```
func (d *Document) FieldInternalDeferLabel(fieldRef int) (label string, exists bool) {
	directiveRef, exists := d.Fields[fieldRef].Directives.HasDirectiveByNameBytes(d, literal.DEFER_INTERNAL)
	if !exists {
		return "", false
	}
	labelValue, exists := d.DirectiveArgumentValueByName(directiveRef, []byte("label"))
	if !exists {
		return "", false
	}
	if labelValue.Kind != ValueKindString {
		return "", false
	}
	return d.StringValueContentString(labelValue.Ref), true
}

```

`new_string`: (empty)

- [ ] Run: `gofmt -w v2/pkg/ast/ast_field.go`
- [ ] Run: `go build ./...` — must succeed.

---

## Task 10: Run package tests, fix unit-test breakage

Integration test rewrites are deferred to the LAST step (Task 11). Here we confirm the lower-level packages still compile and pass; any unit tests that hard-coded the old envelope shape will fail and need touch-ups.

### Step 10.1: Run

- [ ] Run: `gotestsum --format=short -- ./v2/pkg/ast/... ./v2/pkg/astnormalization/... ./v2/pkg/engine/plan/... ./v2/pkg/engine/postprocess/... ./v2/pkg/engine/resolve/... -count=1`

### Step 10.2: Fix unit tests that hard-coded envelope strings

Most resolve-package tests that exercise `ResolveDefer` / initial-render output will need their expected JSON updated. Sweep `v2/pkg/engine/resolve/resolve_test.go` and `resolvable_test.go` for `"path":[`, `"label":`, `"incremental":[{"data"` — patch each to the new shape.

---

## Task 11: Integration test rewrite

**File:** `execution/engine/execution_engine_defer_test.go`

Run last, after lower-level packages and tests are green.

### Step 11.1: Convert each test case

Pattern to apply (recap):
- Initial response gains `"pending":[{ "id":"<n>", "path":[...], "label":"…" (optional) }, ...]`. Every defer (top-level and nested) listed here, sorted by id ascending.
- Each `incremental` item loses `path` and `label`, gains `id`.
- Each subsequent envelope gains `"completed":[{ "id":"<n>" }]` before `hasNext`. No `pending` in subsequent.

(See examples already in this plan — preserved below for reference.)

**Old**:
```
{"data":{"user":{"name":"Black"}},"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"path":["user"]}],"hasNext":false}
```

**New**:
```
{"data":{"user":{"name":"Black"}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
```

**Old (labeled)**:
```
{"data":{"user":{"name":"Black"}},"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"path":["user"],"label":"titleLabel"}],"hasNext":false}
```

**New (labeled)**:
```
{"data":{"user":{"name":"Black"}},"pending":[{"id":"1","path":["user"],"label":"titleLabel"}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
```

**Old (nested)**:
```
{"data":{"user":{"name":"Black"}},"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"path":["user"]}],"hasNext":false}
```

**New (nested) — both defers in initial `pending`**:
```
{"data":{"user":{"name":"Black"}},"pending":[{"id":"1","path":["user"]},{"id":"2","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
```

**Old (sibling)**:
```
{"data":{"user":{}},"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"path":["user"],"label":"a"}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"path":["user"]}],"hasNext":false}
```

**New (sibling)**:
```
{"data":{"user":{}},"pending":[{"id":"1","path":["user"],"label":"a"},{"id":"2","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
```

NOTE: Defer ids start at 1 in our planner (0 is the "no defer" sentinel). `id` is rendered as a string (`"1"`, `"2"`, …). Order in `pending` is by id ascending.

### Step 11.2: Run

- [ ] Run: `gotestsum --format=short -- ./execution/engine/... -count=1 -run TestExecutionEngine_Execute_Defer`

Comprehensive parity with graphql-js spec fixtures (recoverable / non-recoverable error routing, list-item defers, etc.) is **next iteration** per agreement — not added here.

---

## Out of scope

- Spec-lazy nested `pending` (release nested in subsequent payload). All defers in initial `pending` for now.
- `subPath` rendering. Path precompute unblocks it; not implemented this iteration.
- Per-list-item defer-id allocation (graphql-js's `_ensureId` per runtime DeliveryGroup). One AST defer = one id, regardless of how many list items the deferred fragment applies to.
- Comprehensive spec-fixture parity in tests.

---

## Verification checklist

- [ ] `go build ./...` succeeds.
- [ ] All four labeled integration tests in `execution_engine_defer_test.go` pass with the new envelope shape.
- [ ] All four pre-existing unlabeled defer tests pass with the new envelope shape.
- [ ] `grep -rn "FieldInternalDeferLabel" v2/ execution/` returns nothing. (`FieldInternalDeferID` stays — used standalone where only the id is needed.)
- [ ] `grep -rn "DeferLabels" v2/ execution/` returns nothing.
- [ ] `grep -rn "deferLabel" v2/ execution/` returns nothing (the field is gone).
- [ ] `grep -rn "printDeferPathAndErrors\|printDeferEnvelopeNullData" v2/` returns nothing.
- [ ] `grep -rn "subPath" v2/ execution/` returns nothing.

