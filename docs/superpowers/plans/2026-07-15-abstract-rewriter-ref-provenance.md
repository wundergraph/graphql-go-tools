# Abstract Rewriter Field Ref Provenance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the walk-and-match field ref tracking in the abstract selection rewriter with exact provenance recorded at the moments fields are copied and merged.

**Architecture:** Two optional observation callbacks on `ast.Document` (`OnCopyField`, `OnMergeFields`) record a copy log and a merge log during a rewrite. A pure function composes the logs into the existing `RewriteResult` maps (`changedFieldRefs`, `fieldRefOrigins`). The `AbstractFieldPathCollector` / scope-chain machinery is deleted; `nodeSelectionVisitor.updateSkipFieldRefs` loses its unskip branch (impossible by construction).

**Tech Stack:** Go, testify, gotestsum.

**Spec:** `docs/superpowers/specs/2026-07-15-abstract-rewriter-ref-provenance-design.md`

## Global Constraints

- Working directory for all commands: the repository root (branch `sergiy/router-152-fix-override-of-response-fields-with-a-skipped-fields`); test commands run from the `v2/` module directory.
- **Use the Edit/Write tools for ALL file modifications. Never use python, sed, awk, or any script to edit files.**
- Go files use tabs. When an Edit fails with "String not found", re-read the exact lines first; start `old_string` from within a line (after leading whitespace) when anchoring. Run `gofmt -w <file>` after editing a file if indentation of inserted code used spaces.
- Run tests with `gotestsum --format=short -- <packages> -run <TestName>` — never bare `go test`.
- Commit after each task (message style: `refactor: ...` / `test: ...`). Do NOT push.
- Do not add new dependencies.

---

### Task 1: Observation hooks on ast.Document

**Files:**
- Modify: `v2/pkg/ast/ast.go` (Document struct ~line 57, `Reset()` ~line 163)
- Modify: `v2/pkg/ast/ast_field.go` (`CopyField` ~line 23, `MergeFieldsDefer` ~line 189)
- Create: `v2/pkg/ast/ast_field_hooks_test.go`

**Interfaces:**
- Produces: `Document.OnCopyField func(fieldRef, copyRef int)` and `Document.OnMergeFields func(survivorRef, removedRef int)` — nil-able callback fields, invoked by `CopyField` / `MergeFieldsDefer`, cleared by `Reset()`. Task 3 consumes these.

- [ ] **Step 1: Write the failing test**

Create `v2/pkg/ast/ast_field_hooks_test.go`:

```go
package ast_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func TestDocumentFieldHooks(t *testing.T) {
	t.Run("OnCopyField is called with the source and the copied field ref", func(t *testing.T) {
		doc := ast.NewDocument()
		fieldRef := doc.AddField(ast.Field{Name: doc.Input.AppendInputString("id")}).Ref

		var gotFrom, gotTo int
		calls := 0
		doc.OnCopyField = func(fieldRef, copyRef int) {
			gotFrom, gotTo = fieldRef, copyRef
			calls++
		}

		copyRef := doc.CopyField(fieldRef)

		assert.Equal(t, 1, calls)
		assert.Equal(t, fieldRef, gotFrom)
		assert.Equal(t, copyRef, gotTo)
		assert.NotEqual(t, fieldRef, copyRef)
	})

	t.Run("CopyField without hook does not panic", func(t *testing.T) {
		doc := ast.NewDocument()
		fieldRef := doc.AddField(ast.Field{Name: doc.Input.AppendInputString("id")}).Ref

		copyRef := doc.CopyField(fieldRef)
		assert.NotEqual(t, fieldRef, copyRef)
	})

	t.Run("OnMergeFields is called with the survivor and the removed field ref", func(t *testing.T) {
		doc := ast.NewDocument()
		left := doc.AddField(ast.Field{Name: doc.Input.AppendInputString("id")}).Ref
		right := doc.AddField(ast.Field{Name: doc.Input.AppendInputString("id")}).Ref

		var gotSurvivor, gotRemoved int
		calls := 0
		doc.OnMergeFields = func(survivorRef, removedRef int) {
			gotSurvivor, gotRemoved = survivorRef, removedRef
			calls++
		}

		doc.MergeFieldsDefer(left, right)

		assert.Equal(t, 1, calls)
		assert.Equal(t, left, gotSurvivor)
		assert.Equal(t, right, gotRemoved)
	})

	t.Run("Reset clears the hooks", func(t *testing.T) {
		doc := ast.NewDocument()
		doc.OnCopyField = func(fieldRef, copyRef int) {}
		doc.OnMergeFields = func(survivorRef, removedRef int) {}

		doc.Reset()

		assert.Nil(t, doc.OnCopyField)
		assert.Nil(t, doc.OnMergeFields)
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd v2 && gotestsum --format=short -- ./pkg/ast/... -run TestDocumentFieldHooks`
Expected: compile FAIL — `doc.OnCopyField undefined`.

- [ ] **Step 3: Add the callback fields to Document**

In `v2/pkg/ast/ast.go`, the Document struct ends with (lines 55-58):

```go
	Refs                         [][8]int
	RefIndex                     int
	Index                        Index
}
```

Edit to:

```go
	Refs                         [][8]int
	RefIndex                     int
	Index                        Index

	// OnCopyField, when set, is called by CopyField with the source field ref and the new field ref.
	OnCopyField func(fieldRef, copyRef int)
	// OnMergeFields, when set, is called by MergeFieldsDefer with the surviving (left)
	// and the removed (right) field ref when two fields are merged.
	OnMergeFields func(survivorRef, removedRef int)
}
```

In `Reset()` (v2/pkg/ast/ast.go), the function ends with:

```go
	d.RefIndex = -1
	d.Index.Reset()
	d.Input.Reset()
}
```

Edit to:

```go
	d.RefIndex = -1
	d.Index.Reset()
	d.Input.Reset()

	d.OnCopyField = nil
	d.OnMergeFields = nil
}
```

- [ ] **Step 4: Invoke the hooks**

In `v2/pkg/ast/ast_field.go`, `CopyField` currently ends with:

```go
	return d.AddField(Field{
		Name:          d.copyByteSliceReference(d.Fields[ref].Name),
		Alias:         d.CopyAlias(d.Fields[ref].Alias),
		HasArguments:  d.Fields[ref].HasArguments,
		Arguments:     arguments,
		HasDirectives: d.Fields[ref].HasDirectives,
		Directives:    directives,
		HasSelections: d.Fields[ref].HasSelections,
		SelectionSet:  selectionSet,
	}).Ref
}
```

Edit to:

```go
	copyRef := d.AddField(Field{
		Name:          d.copyByteSliceReference(d.Fields[ref].Name),
		Alias:         d.CopyAlias(d.Fields[ref].Alias),
		HasArguments:  d.Fields[ref].HasArguments,
		Arguments:     arguments,
		HasDirectives: d.Fields[ref].HasDirectives,
		Directives:    directives,
		HasSelections: d.Fields[ref].HasSelections,
		SelectionSet:  selectionSet,
	}).Ref
	if d.OnCopyField != nil {
		d.OnCopyField(ref, copyRef)
	}
	return copyRef
}
```

In `v2/pkg/ast/ast_field.go`, `MergeFieldsDefer` begins with:

```go
func (d *Document) MergeFieldsDefer(left, right int) {
	leftDeferDirectiveRef, leftDeferExists := d.Fields[left].Directives.HasDirectiveByNameBytes(d, literal.DEFER_INTERNAL)
```

Edit to:

```go
func (d *Document) MergeFieldsDefer(left, right int) {
	if d.OnMergeFields != nil {
		d.OnMergeFields(left, right)
	}

	leftDeferDirectiveRef, leftDeferExists := d.Fields[left].Directives.HasDirectiveByNameBytes(d, literal.DEFER_INTERNAL)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd v2 && gotestsum --format=short -- ./pkg/ast/... -run TestDocumentFieldHooks`
Expected: PASS (4 subtests).

- [ ] **Step 6: Run the whole ast package**

Run: `cd v2 && gotestsum --format=short -- ./pkg/ast/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add v2/pkg/ast/ast.go v2/pkg/ast/ast_field.go v2/pkg/ast/ast_field_hooks_test.go && git commit -m "feat(ast): add OnCopyField and OnMergeFields observation hooks to Document"
```

---

### Task 2: Provenance log composition function

**Files:**
- Create: `v2/pkg/engine/plan/abstract_selection_rewriter_provenance.go`
- Create: `v2/pkg/engine/plan/abstract_selection_rewriter_provenance_test.go`

**Interfaces:**
- Produces: `type refPair struct { from, to int }` and
  `func buildRefMappings(copyLog, mergeLog []refPair) (changedFieldRefs, fieldRefOrigins map[int][]int)` in package `plan`. Task 3 consumes both.
- Semantics: `copyLog` entries are `(originalRef -> newRef)`; `mergeLog` entries are `(removedRef -> survivorRef)` in chronological order. `changedFieldRefs` maps each original ref to the final refs its copies became (after resolving merge chains, deduplicated). `fieldRefOrigins` maps each surviving new ref to all original refs it represents (deduplicated); refs merged away are absent.

- [ ] **Step 1: Write the failing test**

Create `v2/pkg/engine/plan/abstract_selection_rewriter_provenance_test.go`:

```go
package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildRefMappings(t *testing.T) {
	t.Run("empty logs produce empty maps", func(t *testing.T) {
		changed, origins := buildRefMappings(nil, nil)
		assert.Empty(t, changed)
		assert.Empty(t, origins)
	})

	t.Run("copies without merges map one to one", func(t *testing.T) {
		copyLog := []refPair{{from: 0, to: 5}, {from: 0, to: 6}, {from: 1, to: 7}}

		changed, origins := buildRefMappings(copyLog, nil)

		assert.Equal(t, map[int][]int{0: {5, 6}, 1: {7}}, changed)
		assert.Equal(t, map[int][]int{5: {0}, 6: {0}, 7: {1}}, origins)
	})

	t.Run("merge transfers origins to the survivor", func(t *testing.T) {
		// user id copied to A (5) and B (6); planner id in B copied to 8; 8 merged into 6
		copyLog := []refPair{{from: 0, to: 5}, {from: 0, to: 6}, {from: 1, to: 7}, {from: 2, to: 8}}
		mergeLog := []refPair{{from: 8, to: 6}}

		changed, origins := buildRefMappings(copyLog, mergeLog)

		assert.Equal(t, map[int][]int{0: {5, 6}, 1: {7}, 2: {6}}, changed)
		assert.Equal(t, map[int][]int{5: {0}, 6: {0, 2}, 7: {1}}, origins)
	})

	t.Run("merge chain resolves to the final survivor", func(t *testing.T) {
		copyLog := []refPair{{from: 0, to: 5}, {from: 1, to: 6}, {from: 2, to: 7}}
		mergeLog := []refPair{{from: 6, to: 5}, {from: 5, to: 7}}

		changed, origins := buildRefMappings(copyLog, mergeLog)

		assert.Equal(t, map[int][]int{0: {7}, 1: {7}, 2: {7}}, changed)
		assert.Equal(t, map[int][]int{7: {2, 0, 1}}, origins)
	})

	t.Run("copies of one original merged together are deduplicated", func(t *testing.T) {
		copyLog := []refPair{{from: 0, to: 5}, {from: 0, to: 6}}
		mergeLog := []refPair{{from: 6, to: 5}}

		changed, origins := buildRefMappings(copyLog, mergeLog)

		assert.Equal(t, map[int][]int{0: {5}}, changed)
		assert.Equal(t, map[int][]int{5: {0}}, origins)
	})

	t.Run("merge of refs unknown to the copy log is ignored", func(t *testing.T) {
		copyLog := []refPair{{from: 0, to: 5}}
		mergeLog := []refPair{{from: 9, to: 8}}

		changed, origins := buildRefMappings(copyLog, mergeLog)

		assert.Equal(t, map[int][]int{0: {5}}, changed)
		assert.Equal(t, map[int][]int{5: {0}}, origins)
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd v2 && gotestsum --format=short -- ./pkg/engine/plan/... -run TestBuildRefMappings`
Expected: compile FAIL — `undefined: buildRefMappings` / `undefined: refPair`.

- [ ] **Step 3: Write the implementation**

Create `v2/pkg/engine/plan/abstract_selection_rewriter_provenance.go`:

```go
package plan

import "slices"

// refPair is a single provenance record of a field rewrite.
// In a copy log it is (original field ref -> new field ref).
// In a merge log it is (removed field ref -> surviving field ref).
type refPair struct {
	from int
	to   int
}

// buildRefMappings composes the copy and merge logs of a single rewrite
// into the two RewriteResult maps.
//
// The copy log holds one entry per field created during the rewrite, pointing at
// the pre-rewrite field ref it was created from. The merge log holds one entry per
// field merged away during the post-rewrite normalization, pointing at the field it
// was merged into, in chronological order - a survivor of an earlier merge can be
// removed by a later one.
func buildRefMappings(copyLog, mergeLog []refPair) (changedFieldRefs, fieldRefOrigins map[int][]int) {
	fieldRefOrigins = make(map[int][]int, len(copyLog))
	for _, c := range copyLog {
		fieldRefOrigins[c.to] = appendUniqueRef(fieldRefOrigins[c.to], c.from)
	}

	// a removed field transfers its origins to the survivor
	redirects := make(map[int]int, len(mergeLog))
	for _, m := range mergeLog {
		for _, originRef := range fieldRefOrigins[m.from] {
			fieldRefOrigins[m.to] = appendUniqueRef(fieldRefOrigins[m.to], originRef)
		}
		delete(fieldRefOrigins, m.from)
		redirects[m.from] = m.to
	}

	changedFieldRefs = make(map[int][]int, len(copyLog))
	for _, c := range copyLog {
		newRef := c.to
		for {
			survivorRef, ok := redirects[newRef]
			if !ok {
				break
			}
			newRef = survivorRef
		}
		changedFieldRefs[c.from] = appendUniqueRef(changedFieldRefs[c.from], newRef)
	}

	return changedFieldRefs, fieldRefOrigins
}

func appendUniqueRef(refs []int, ref int) []int {
	if slices.Contains(refs, ref) {
		return refs
	}
	return append(refs, ref)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd v2 && gotestsum --format=short -- ./pkg/engine/plan/... -run TestBuildRefMappings`
Expected: PASS (6 subtests).

- [ ] **Step 5: Commit**

```bash
git add v2/pkg/engine/plan/abstract_selection_rewriter_provenance.go v2/pkg/engine/plan/abstract_selection_rewriter_provenance_test.go && git commit -m "feat(plan): add provenance log composition for the abstract selection rewriter"
```

---

### Task 3: Record provenance in the rewriter, delete the path collector

**Files:**
- Modify: `v2/pkg/engine/plan/abstract_selection_rewriter.go`
- Modify: `v2/pkg/engine/plan/abstract_selection_rewriter_helpers.go` (`preserveTypeNameSelection` ~line 452)
- Modify: `v2/pkg/engine/plan/abstract_selection_rewriter_info.go` (`selectionSetInfo` struct, `selectionSetFieldSelections`, `collectSelectionSetInformation`)
- Modify: `v2/pkg/engine/plan/abstract_selection_rewriter_changed_refs_test.go`

**Interfaces:**
- Consumes: `Document.OnCopyField` / `Document.OnMergeFields` (Task 1), `refPair` / `buildRefMappings` (Task 2).
- Produces: `RewriteResult{changedFieldRefs, fieldRefOrigins}` with tightened semantics — both maps contain only refs participating in the rewrite; no root-field identity entry. Task 4 relies on: every key of `fieldRefOrigins` is a freshly created field ref, never present in `skipFieldsRefs` before the rewrite.

- [ ] **Step 1: Update the changed-refs test expectations**

In `v2/pkg/engine/plan/abstract_selection_rewriter_changed_refs_test.go`:

1. In subtest `"user field in one fragment - duplicated field in another fragment"`, edit the expected origins map from:

```go
			map[int][]int{
				4: {4},
				5: {0}, // id in A originates only from the user field - must not inherit skip status from ref 2
				6: {1},
				7: {2}, // id in B originates only from the planner field - stays skipped
			},
```

to:

```go
			map[int][]int{
				5: {0}, // id in A originates only from the user field - must not inherit skip status from ref 2
				6: {1},
				7: {2}, // id in B originates only from the planner field - stays skipped
			},
```

2. In subtest `"user field on interface level - duplicated field in fragment"`, edit the expected origins map from:

```go
			map[int][]int{
				4: {4},
				5: {0},
				6: {0, 2}, // merged user and planner fields - user field wins, must stay in the response
				7: {1},
			},
```

to:

```go
			map[int][]int{
				5: {0},
				6: {0, 2}, // merged user and planner fields - user field wins, must stay in the response
				7: {1},
			},
```

3. In subtest `"aliased duplicate does not conflate with the field name"`, edit the expected origins map from:

```go
			map[int][]int{
				4: {4},
				5: {0},
				6: {0},
				7: {1},
				8: {2},
			},
```

to:

```go
			map[int][]int{
				5: {0},
				6: {0},
				7: {1},
				8: {2},
			},
```

4. Update the doc comment of `TestFieldSelectionRewriter_ChangedFieldRefs` from:

```go
// TestFieldSelectionRewriter_ChangedFieldRefs verifies that after a rewrite the mapping
// between original and new field refs respects inline fragment type condition scopes.
// Fields with the same name at the same query depth but under non-intersecting type conditions
// must not be mapped to each other - otherwise a planner-added (skipped) field and
// a user-requested field are conflated and skip status propagates to the wrong refs.
```

to:

```go
// TestFieldSelectionRewriter_ChangedFieldRefs verifies the exact provenance mapping
// between original and new field refs produced by a rewrite.
// A planner-added (skipped) field and a user-requested field with the same name
// under different type conditions must map to distinct new refs - otherwise
// skip status propagates to the wrong refs.
```

5. Append a new test function at the end of the file covering `__typename` recreation through `preserveTypeNameSelection` (union rewrite):

```go
// TestFieldSelectionRewriter_ChangedFieldRefs_UnionTypename verifies that an explicitly
// requested __typename recreated by a union rewrite keeps its provenance -
// a planner-added skipped __typename must stay skipped after a rewrite recreates it.
func TestFieldSelectionRewriter_ChangedFieldRefs_UnionTypename(t *testing.T) {
	definition := `
		type A {
			id: ID!
		}

		type B {
			id: ID!
		}

		type C {
			id: ID!
		}

		union Nodes = A | B | C

		type Query {
			unodes: Nodes
		}
	`

	// type C is not a member of the union in the upstream schema - a fragment on C triggers the rewrite
	upstreamDefinition := `
		type A @key(fields: "id") {
			id: ID!
		}

		type B @key(fields: "id") {
			id: ID!
		}

		union Nodes = A | B

		type Query {
			unodes: Nodes
		}
	`

	// refs before: 0 - __typename, 1 - id in B, 2 - id in C, 3 - unodes
	// refs after: 4 - recreated __typename, 5 - id in B
	operation := `query { unodes { __typename ... on B { id } ... on C { id } } }`
	expectedOperation := `query {
		unodes {
			__typename
			... on B {
				id
			}
		}
	}`

	op := unsafeparser.ParseGraphqlDocumentString(operation)
	def := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(definition)

	fieldRef := ast.InvalidRef
	for ref := range op.Fields {
		if op.FieldNameString(ref) == "unodes" {
			fieldRef = ref
			break
		}
	}
	require.NotEqual(t, ast.InvalidRef, fieldRef)

	ds := dsb().
		RootNode("Query", "unodes").
		RootNode("A", "id").
		RootNode("B", "id").
		KeysMetadata(FederationFieldConfigurations{
			{TypeName: "A", SelectionSet: "id"},
			{TypeName: "B", SelectionSet: "id"},
		}).
		SchemaMergedWithBase(upstreamDefinition).
		DS()

	node, _ := def.Index.FirstNodeByNameStr("Query")

	rewriter, err := newFieldSelectionRewriter(&op, &def, ds)
	require.NoError(t, err)

	result, err := rewriter.RewriteFieldSelection(fieldRef, node)
	require.NoError(t, err)
	require.True(t, result.rewritten)

	assert.Equal(t, unsafeprinter.Prettify(expectedOperation), unsafeprinter.PrettyPrint(&op))
	assert.Equal(t, map[int][]int{
		0: {4},
		1: {5},
	}, result.changedFieldRefs)
	assert.Equal(t, map[int][]int{
		4: {0}, // recreated __typename inherits the origin of the original __typename
		5: {1},
	}, result.fieldRefOrigins)

	// the provenance hooks must not outlive the rewrite
	assert.Nil(t, op.OnCopyField)
	assert.Nil(t, op.OnMergeFields)
}
```

- [ ] **Step 2: Run the test to verify current expectations fail**

Run: `cd v2 && gotestsum --format=short -- ./pkg/engine/plan/... -run 'TestFieldSelectionRewriter_ChangedFieldRefs'`
Expected: FAIL — the three updated subtests fail on the removed `4: {4}` entries (old implementation still emits them); the union test may fail on maps.

- [ ] **Step 3: Add provenance logs to the rewriter struct**

In `v2/pkg/engine/plan/abstract_selection_rewriter.go`, the struct:

```go
	skipFieldRefs []int
	alwaysRewrite bool
}
```

Edit to:

```go
	skipFieldRefs []int
	alwaysRewrite bool

	copyLog  []refPair // (original field ref -> new field ref) for each field created during the rewrite
	mergeLog []refPair // (removed field ref -> surviving field ref) for each field merged away during the post-rewrite normalization, chronological
}
```

Update the `RewriteResult` comments from:

```go
type RewriteResult struct {
	rewritten        bool
	changedFieldRefs map[int][]int // map[fieldRef][]fieldRef - for each original fieldRef list of new fieldRefs; identity mappings are omitted
	fieldRefOrigins  map[int][]int // map[fieldRef][]fieldRef - for each fieldRef present after the rewrite, all original fieldRefs occupying the same response position, including itself
}
```

to:

```go
type RewriteResult struct {
	rewritten        bool
	changedFieldRefs map[int][]int // map[originalFieldRef][]newFieldRef - for each original field ref, the new field refs it was rewritten into
	fieldRefOrigins  map[int][]int // map[newFieldRef][]originalFieldRef - for each field ref created by the rewrite, the original field refs it represents
}
```

- [ ] **Step 4: Set the hooks for the duration of RewriteFieldSelection**

In `RewriteFieldSelection`, after:

```go
	fieldName := r.operation.FieldNameBytes(fieldRef)
	fieldTypeNode, ok := r.definition.FieldTypeNode(fieldName, enclosingNode)
	if !ok {
		return resultNotRewritten, nil
	}
```

insert:

```go
	// Record provenance of field refs touched by the rewrite. Every new field is copied
	// from an original field (createFragmentSelection) or recreated from one
	// (preserveTypeNameSelection, which appends to copyLog directly). Fields merged away
	// during the post-rewrite normalization transfer their origins to the surviving field.
	r.operation.OnCopyField = func(originalFieldRef, copyRef int) {
		r.copyLog = append(r.copyLog, refPair{from: originalFieldRef, to: copyRef})
	}
	r.operation.OnMergeFields = func(survivorRef, removedRef int) {
		r.mergeLog = append(r.mergeLog, refPair{from: removedRef, to: survivorRef})
	}
	defer func() {
		r.operation.OnCopyField = nil
		r.operation.OnMergeFields = nil
	}()
```

- [ ] **Step 5: Replace path collection with log composition in the three process functions**

In `processUnionSelection`, delete:

```go
	fieldPaths, err := collectFieldPaths(r.operation, r.definition, fieldRef)
	if err != nil {
		return resultNotRewritten, err
	}
```

and replace:

```go
	changedRefs, originRefs, err := r.collectChangedRefs(fieldRef, fieldPaths)
	if err != nil {
		return resultNotRewritten, err
	}

	return RewriteResult{
		rewritten:        true,
		changedFieldRefs: changedRefs,
		fieldRefOrigins:  originRefs,
	}, nil
```

with:

```go
	changedRefs, originRefs := buildRefMappings(r.copyLog, r.mergeLog)

	return RewriteResult{
		rewritten:        true,
		changedFieldRefs: changedRefs,
		fieldRefOrigins:  originRefs,
	}, nil
```

Apply the exact same two edits in `processObjectSelection` and `processInterfaceSelection` (each has an identical `fieldPaths, err := collectFieldPaths(...)` block before its `rewriteXxxSelection` call and an identical `collectChangedRefs` block after it).

- [ ] **Step 6: Delete the path collector machinery**

In `v2/pkg/engine/plan/abstract_selection_rewriter.go`, delete entirely (they form the contiguous tail of the file, from `func (r *fieldSelectionRewriter) collectChangedRefs(` to the end):

- `func (r *fieldSelectionRewriter) collectChangedRefs`
- `type collectedFieldPath`
- `type scopeChain`
- `func scopeChainsIntersect`
- `func scopesIntersect`
- `func intersectScopes`
- `type AbstractFieldPathCollector` and all its methods (`EnterField`, `LeaveField`, `EnterInlineFragment`, `LeaveInlineFragment`, `resolveTypeCondition`)
- `func collectFieldPaths`
- `type FieldLimitedVisitor` and its methods (`AllowVisitor`, `EnterField`, `LeaveField`)

Then fix imports: remove `"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"` and `"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"` from the import block (they were used only by the deleted code; verify with the compiler in Step 9).

- [ ] **Step 7: Track the original __typename field ref in selectionSetInfo**

In `v2/pkg/engine/plan/abstract_selection_rewriter_info.go`:

1. Struct field — edit:

```go
	hasInlineFragmentsOnUnions     bool
	typenameFieldDeferId           int
}
```

to:

```go
	hasInlineFragmentsOnUnions     bool
	typenameFieldDeferId           int
	typenameFieldRef               int // field ref of the __typename selection; only meaningful when hasTypeNameSelection is true, ast.InvalidRef otherwise
}
```

2. `selectionSetFieldSelections` — edit the whole function from:

```go
func (r *fieldSelectionRewriter) selectionSetFieldSelections(selectionSetRef int) (fieldSelections []fieldSelection, hasTypename bool, typeNameFieldDeferID int) {
	fieldSelectionRefs := r.operation.SelectionSetFieldSelections(selectionSetRef)
	fieldSelections = make([]fieldSelection, 0, len(fieldSelectionRefs))
	for _, fieldSelectionRef := range fieldSelectionRefs {
		fieldRef := r.operation.Selections[fieldSelectionRef].Ref
		fieldName := r.operation.FieldNameString(fieldRef)

		if fieldName == typeNameField {
			hasTypename = true
			typeNameFieldDeferID, _ = r.operation.FieldInternalDeferID(fieldRef)
		}

		fieldSelections = append(fieldSelections, fieldSelection{
			fieldSelectionRef: fieldSelectionRef,
			fieldName:         fieldName,
		})
	}

	return fieldSelections, hasTypename, typeNameFieldDeferID
}
```

to:

```go
func (r *fieldSelectionRewriter) selectionSetFieldSelections(selectionSetRef int) (fieldSelections []fieldSelection, hasTypename bool, typeNameFieldDeferID int, typenameFieldRef int) {
	typenameFieldRef = ast.InvalidRef

	fieldSelectionRefs := r.operation.SelectionSetFieldSelections(selectionSetRef)
	fieldSelections = make([]fieldSelection, 0, len(fieldSelectionRefs))
	for _, fieldSelectionRef := range fieldSelectionRefs {
		fieldRef := r.operation.Selections[fieldSelectionRef].Ref
		fieldName := r.operation.FieldNameString(fieldRef)

		if fieldName == typeNameField {
			hasTypename = true
			typenameFieldRef = fieldRef
			typeNameFieldDeferID, _ = r.operation.FieldInternalDeferID(fieldRef)
		}

		fieldSelections = append(fieldSelections, fieldSelection{
			fieldSelectionRef: fieldSelectionRef,
			fieldName:         fieldName,
		})
	}

	return fieldSelections, hasTypename, typeNameFieldDeferID, typenameFieldRef
}
```

3. `collectSelectionSetInformation` — edit:

```go
	fieldSelections, hasSharedTypename, typenameFieldDeferId := r.selectionSetFieldSelections(selectionSetRef)
```

to:

```go
	fieldSelections, hasSharedTypename, typenameFieldDeferId, typenameFieldRef := r.selectionSetFieldSelections(selectionSetRef)
```

and in the returned struct literal, edit:

```go
		hasTypeNameSelection:           hasSharedTypename,
		typenameFieldDeferId:           typenameFieldDeferId,
```

to:

```go
		hasTypeNameSelection:           hasSharedTypename,
		typenameFieldDeferId:           typenameFieldDeferId,
		typenameFieldRef:               typenameFieldRef,
```

- [ ] **Step 8: Record provenance for the recreated __typename**

In `v2/pkg/engine/plan/abstract_selection_rewriter_helpers.go`, edit `preserveTypeNameSelection` from:

```go
func (r *fieldSelectionRewriter) preserveTypeNameSelection(selectionSetInfo selectionSetInfo, selectionRefs *[]int) {
	// we should preserve __typename if it was in the original query as it is explicitly requested
	if !selectionSetInfo.hasTypeNameSelection {
		return
	}

	selectionRef, _ := r.typeNameSelection(selectionSetInfo.typenameFieldDeferId)
	*selectionRefs = append(*selectionRefs, selectionRef)
}
```

to:

```go
func (r *fieldSelectionRewriter) preserveTypeNameSelection(selectionSetInfo selectionSetInfo, selectionRefs *[]int) {
	// we should preserve __typename if it was in the original query as it is explicitly requested
	if !selectionSetInfo.hasTypeNameSelection {
		return
	}

	selectionRef, fieldRef := r.typeNameSelection(selectionSetInfo.typenameFieldDeferId)
	if selectionSetInfo.typenameFieldRef != ast.InvalidRef {
		// the recreated __typename replaces the original one - record it as a copy
		r.copyLog = append(r.copyLog, refPair{from: selectionSetInfo.typenameFieldRef, to: fieldRef})
	}
	*selectionRefs = append(*selectionRefs, selectionRef)
}
```

- [ ] **Step 9: Build and run the rewriter tests**

Run: `cd v2 && go build ./... && go vet ./pkg/engine/plan/ ./pkg/ast/`
Expected: clean build. If imports were over- or under-trimmed in Step 6, fix them now.

Run: `cd v2 && gotestsum --format=short -- ./pkg/engine/plan/... -run 'TestFieldSelectionRewriter'`
Expected: PASS, including all `TestFieldSelectionRewriter_ChangedFieldRefs*` tests.

- [ ] **Step 10: Run the full plan package**

Run: `cd v2 && gotestsum --format=short -- ./pkg/engine/plan/...`
Expected: PASS. `TestNodeSelectionVisitor_UpdateSkipFieldRefs` still passes — the current visitor code handles the new maps (the unskip branch just never triggers); it is simplified in Task 4.

- [ ] **Step 11: gofmt and commit**

```bash
gofmt -l v2/pkg/engine/plan/ v2/pkg/ast/
```
Expected: no output (fix any listed file with `gofmt -w`).

```bash
git add -A v2/pkg/engine/plan/ && git commit -m "refactor(plan): replace path-matching ref tracking with copy/merge provenance in the abstract rewriter"
```

---

### Task 4: Simplify updateSkipFieldRefs

**Files:**
- Modify: `v2/pkg/engine/plan/node_selection_visitor.go` (`updateSkipFieldRefs` ~line 861)
- Modify: `v2/pkg/engine/plan/abstract_selection_rewriter_changed_refs_test.go` (`TestNodeSelectionVisitor_UpdateSkipFieldRefs`)

**Interfaces:**
- Consumes: `RewriteResult.fieldRefOrigins` semantics from Task 3 — every key is a freshly created field ref, never already present in `skipFieldsRefs`.

- [ ] **Step 1: Update the test**

Edit `TestNodeSelectionVisitor_UpdateSkipFieldRefs` from:

```go
// TestNodeSelectionVisitor_UpdateSkipFieldRefs verifies the skip propagation semantics:
// a field ref present after a rewrite is skipped only when all original refs
// occupying the same response position were skipped.
func TestNodeSelectionVisitor_UpdateSkipFieldRefs(t *testing.T) {
	c := &nodeSelectionVisitor{
		skipFieldsRefs: []int{2, 9},
	}

	c.updateSkipFieldRefs(map[int][]int{
		5: {0},    // origin is a user field - stays visible
		6: {0, 2}, // user and planner fields merged - stays visible
		7: {2},    // origin is a planner field - becomes skipped
		9: {0, 9}, // previously skipped ref survived a merge with a user field - must be unskipped
	})

	assert.ElementsMatch(t, []int{2, 7}, c.skipFieldsRefs)
}
```

to:

```go
// TestNodeSelectionVisitor_UpdateSkipFieldRefs verifies the skip propagation semantics:
// a field ref created by a rewrite is skipped only when all original refs
// it represents were skipped. Refs created by a rewrite are always fresh,
// so an already skipped ref can never appear among the keys.
func TestNodeSelectionVisitor_UpdateSkipFieldRefs(t *testing.T) {
	c := &nodeSelectionVisitor{
		skipFieldsRefs: []int{2, 9},
	}

	c.updateSkipFieldRefs(map[int][]int{
		5: {0},    // origin is a user field - stays visible
		6: {0, 2}, // user and planner fields merged - stays visible
		7: {2},    // origin is a planner field - becomes skipped
	})

	assert.ElementsMatch(t, []int{2, 9, 7}, c.skipFieldsRefs)
}
```

- [ ] **Step 2: Run the test to confirm it passes against the current implementation**

Run: `cd v2 && gotestsum --format=short -- ./pkg/engine/plan/... -run TestNodeSelectionVisitor_UpdateSkipFieldRefs`
Expected: PASS (this is a behavior-preserving simplification for the reachable inputs; the test pins the contract before the code shrinks).

- [ ] **Step 3: Simplify the implementation**

In `v2/pkg/engine/plan/node_selection_visitor.go`, edit `updateSkipFieldRefs` from:

```go
// updateSkipFieldRefs updates the skipFieldsRefs list after a rewrite of an abstract field selection set.
// A field ref present after the rewrite should be skipped only when all original field refs
// occupying the same response position were skipped.
// When at least one origin is a user-requested field, the field must stay in the response -
// including the case when a previously skipped ref survived a merge with a user-requested duplicate.
func (c *nodeSelectionVisitor) updateSkipFieldRefs(fieldRefOrigins map[int][]int) {
	if len(fieldRefOrigins) == 0 {
		return
	}

	skipped := make(map[int]struct{}, len(c.skipFieldsRefs))
	for _, fieldRef := range c.skipFieldsRefs {
		skipped[fieldRef] = struct{}{}
	}

	var unskipFieldRefs map[int]struct{}

	for fieldRef, originRefs := range fieldRefOrigins {
		allOriginsSkipped := true
		for _, originRef := range originRefs {
			if _, ok := skipped[originRef]; !ok {
				allOriginsSkipped = false
				break
			}
		}

		if allOriginsSkipped {
			if _, ok := skipped[fieldRef]; !ok {
				c.skipFieldsRefs = append(c.skipFieldsRefs, fieldRef)
			}
			continue
		}

		if _, ok := skipped[fieldRef]; ok {
			if unskipFieldRefs == nil {
				unskipFieldRefs = make(map[int]struct{})
			}
			unskipFieldRefs[fieldRef] = struct{}{}
		}
	}

	if len(unskipFieldRefs) > 0 {
		c.skipFieldsRefs = slices.DeleteFunc(c.skipFieldsRefs, func(fieldRef int) bool {
			_, ok := unskipFieldRefs[fieldRef]
			return ok
		})
	}
}
```

to:

```go
// updateSkipFieldRefs updates the skipFieldsRefs list after a rewrite of an abstract field selection set.
// A field ref created by the rewrite should be skipped only when all original field refs
// it represents were skipped. When at least one origin is a user-requested field,
// the field must stay in the response.
// Field refs created by a rewrite are always fresh, so they are never present in skipFieldsRefs.
func (c *nodeSelectionVisitor) updateSkipFieldRefs(fieldRefOrigins map[int][]int) {
	if len(fieldRefOrigins) == 0 || len(c.skipFieldsRefs) == 0 {
		return
	}

	skipped := make(map[int]struct{}, len(c.skipFieldsRefs))
	for _, fieldRef := range c.skipFieldsRefs {
		skipped[fieldRef] = struct{}{}
	}

	for fieldRef, originRefs := range fieldRefOrigins {
		allOriginsSkipped := true
		for _, originRef := range originRefs {
			if _, ok := skipped[originRef]; !ok {
				allOriginsSkipped = false
				break
			}
		}

		if allOriginsSkipped {
			c.skipFieldsRefs = append(c.skipFieldsRefs, fieldRef)
		}
	}
}
```

(`slices` remains imported — it is used elsewhere in the file.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd v2 && gotestsum --format=short -- ./pkg/engine/plan/... -run TestNodeSelectionVisitor_UpdateSkipFieldRefs`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add v2/pkg/engine/plan/node_selection_visitor.go v2/pkg/engine/plan/abstract_selection_rewriter_changed_refs_test.go && git commit -m "refactor(plan): drop the impossible unskip branch from updateSkipFieldRefs"
```

---

### Task 5: Full verification

**Files:** none (verification only).

- [ ] **Step 1: Run the full plan, ast, astnormalization packages**

Run: `cd v2 && gotestsum --format=short -- ./pkg/engine/plan/... ./pkg/ast/... ./pkg/astnormalization/...`
Expected: PASS.

- [ ] **Step 2: Run the graphql datasource federation suite (contains the ROUTER-152 regression tests added on this branch)**

Run: `cd v2 && gotestsum --format=short -- ./pkg/engine/datasource/graphql_datasource/...`
Expected: PASS.

- [ ] **Step 3: Run the remaining engine packages for regressions**

Run: `cd v2 && gotestsum --format=short -- ./pkg/engine/... ./pkg/astminify/...`
Expected: PASS (pre-existing failures on master are acceptable only if reproducible with `git stash` — none are known on this branch).

- [ ] **Step 4: gofmt check over all touched packages**

Run: `gofmt -l v2/pkg/ast/ v2/pkg/engine/plan/`
Expected: no output.

- [ ] **Step 5: Commit any remaining changes (design doc supersession)**

```bash
git add docs/superpowers/ && git commit -m "docs: supersede scope-chain design with ref provenance design"
```

Do NOT push.
