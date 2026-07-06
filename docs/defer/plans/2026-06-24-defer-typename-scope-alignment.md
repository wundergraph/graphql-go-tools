# Defer `__typename` Scope Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **User preferences:** No `git add`/`git commit` steps — the user manages commits. Run Go tests with `gotestsum --format=short -- <pkg> -run <Test>`. Use the Edit tool for all file edits; run `gofmt -w` after editing Go files. Avoid novel Bash command shapes (see project memory).

**Goal:** Make a `__typename` selected inside `@defer` take the defer scope of the object it describes (its enclosing object field), so the planner never aliases a structurally-required `__typename` and federated entity jumps off a deferred `__typename` work.

**Architecture:** A new normalization rule, `deferAlignTypenameScope`, runs after field merging (`deduplicateFields`) and before `deferPopulateParentIds`. For each `@__defer_internal`-stamped `__typename` field it rewrites the directive to mirror its enclosing object field's defer scope, or removes it when the enclosing object is non-deferred (or there is no enclosing field). The planner and resolver are untouched: with a plain non-deferred `__typename` present, the existing key/representation handling emits a literal `__typename`, which the resolver reads for type discrimination.

**Tech Stack:** Go; `v2/pkg/astnormalization` (rule + wiring), `v2/pkg/ast` (existing helpers), `execution/engine` (integration tests).

## Global Constraints

- `__typename`'s defer id MUST equal its enclosing object field's defer id; never the innermost textual `@defer`.
- Rule is normalization-only. Do NOT modify `required_fields_visitor.go` or `resolvable.go`.
- Rule stage runs **after** the `deduplicateFields` cleanup stage and **before** the `deferPopulateParentIds` stage, gated by the same `skipCondition: !o.inlineDeferVisitor.hasDefers()`.
- Reuse existing AST helpers: `FieldNameBytes`, `FieldInternalDeferIDWithDirectiveRef`, `FieldDeferInfo`, `AddDeferInternalDirectiveToField`, `DirectiveList.RemoveDirectiveByRef`.

---

### Task 1: `deferAlignTypenameScope` normalization rule

**Files:**
- Create: `v2/pkg/astnormalization/defer_align_typename_scope.go`
- Test: `v2/pkg/astnormalization/defer_align_typename_scope_test.go`

**Interfaces:**
- Produces: `func deferAlignTypenameScope(walker *astvisitor.Walker)` — registers the rule on a walker, same shape as `deferPopulateParentIds`.

- [ ] **Step 1: Write the failing tests**

Create `v2/pkg/astnormalization/defer_align_typename_scope_test.go`:

```go
package astnormalization

import "testing"

func TestDeferAlignTypenameScope(t *testing.T) {
	t.Run("deferred __typename on non-deferred object - defer stripped", func(t *testing.T) {
		run(t, deferAlignTypenameScope, testDefinition, `
			query dog {
				dog {
					__typename @__defer_internal(id: 1)
					name @__defer_internal(id: 1)
				}
			}`,
			`
			query dog {
				dog {
					__typename
					name @__defer_internal(id: 1)
				}
			}`, withIndent())
	})

	t.Run("deferred __typename on object in same defer - unchanged", func(t *testing.T) {
		run(t, deferAlignTypenameScope, testDefinition, `
			query dog {
				dog @__defer_internal(id: 1) {
					__typename @__defer_internal(id: 1)
					name @__defer_internal(id: 1)
				}
			}`,
			`
			query dog {
				dog @__defer_internal(id: 1) {
					__typename @__defer_internal(id: 1)
					name @__defer_internal(id: 1)
				}
			}`, withIndent())
	})

	t.Run("__typename deferred deeper than its object - realigned to object scope", func(t *testing.T) {
		run(t, deferAlignTypenameScope, testDefinition, `
			query dog {
				dog @__defer_internal(id: 1) {
					extra @__defer_internal(id: 1) {
						__typename @__defer_internal(id: 2, parentDeferId: 1)
						string @__defer_internal(id: 2, parentDeferId: 1)
					}
				}
			}`,
			`
			query dog {
				dog @__defer_internal(id: 1) {
					extra @__defer_internal(id: 1) {
						__typename @__defer_internal(id: 1)
						string @__defer_internal(id: 2, parentDeferId: 1)
					}
				}
			}`, withIndent())
	})

	t.Run("root-level deferred __typename - defer stripped", func(t *testing.T) {
		run(t, deferAlignTypenameScope, testDefinition, `
			query dog {
				__typename @__defer_internal(id: 1)
				dog {
					name
				}
			}`,
			`
			query dog {
				__typename
				dog {
					name
				}
			}`, withIndent())
	})

	t.Run("no deferred fields - no change", func(t *testing.T) {
		run(t, deferAlignTypenameScope, testDefinition, `
			query dog {
				dog {
					__typename
					name
				}
			}`,
			`
			query dog {
				dog {
					__typename
					name
				}
			}`, withIndent())
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `gotestsum --format=short -- ./v2/pkg/astnormalization/ -run TestDeferAlignTypenameScope`
Expected: FAIL — `undefined: deferAlignTypenameScope`.

- [ ] **Step 3: Write the rule**

Create `v2/pkg/astnormalization/defer_align_typename_scope.go`:

```go
package astnormalization

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

var deferTypenameLiteral = []byte("__typename")

// deferAlignTypenameScope rewrites every @__defer_internal-stamped __typename
// field so its defer scope matches the object it describes (its enclosing object
// field), not the innermost @defer it was textually written in.
//
// __typename is a meta-field whose value is available exactly when its object is
// materialized, so it belongs in that object's defer scope:
//   - enclosing object is non-deferred (or there is no enclosing field): the
//     __typename must not be deferred — remove the directive so it stays in the
//     initial response and remains a literal `__typename` for type discrimination
//     and entity representation building.
//   - enclosing object is deferred under a different id: re-stamp the __typename
//     to mirror the enclosing object field's defer scope.
//   - already in the enclosing object's scope: leave as-is.
//
// Must run after deduplicateFields (so the enclosing object's final defer id is
// known) and before deferPopulateParentIds (so parents are computed from the
// aligned value).
func deferAlignTypenameScope(walker *astvisitor.Walker) {
	visitor := &deferAlignTypenameScopeVisitor{Walker: walker}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)
}

type deferAlignTypenameScopeVisitor struct {
	*astvisitor.Walker

	operation *ast.Document
}

func (v *deferAlignTypenameScopeVisitor) EnterDocument(operation, _ *ast.Document) {
	v.operation = operation
}

func (v *deferAlignTypenameScopeVisitor) EnterField(ref int) {
	if !bytes.Equal(v.operation.FieldNameBytes(ref), deferTypenameLiteral) {
		return
	}
	curID, directiveRef, exists := v.operation.FieldInternalDeferIDWithDirectiveRef(ref)
	if !exists {
		return
	}

	enclosingID, enclosingLabel, enclosingParent, enclosingDeferred := v.enclosingObjectFieldDefer()

	if !enclosingDeferred {
		// enclosing object is non-deferred (or there is no enclosing field):
		// __typename must stay in the initial scope.
		v.removeFieldDeferDirective(ref, directiveRef)
		return
	}

	if curID == enclosingID {
		// already in its object's defer group
		return
	}

	// align __typename to its enclosing object's defer scope
	v.removeFieldDeferDirective(ref, directiveRef)
	v.operation.AddDeferInternalDirectiveToField(ref, enclosingID, enclosingLabel, enclosingParent)
}

func (v *deferAlignTypenameScopeVisitor) LeaveField(ref int) {}

// enclosingObjectFieldDefer returns the defer info of the nearest ancestor field
// (the object the current field belongs to). deferred is false when there is no
// ancestor field or that field carries no @__defer_internal.
func (v *deferAlignTypenameScopeVisitor) enclosingObjectFieldDefer() (id int, label string, parentID int, deferred bool) {
	for i := len(v.Walker.Ancestors) - 1; i >= 0; i-- {
		ancestor := v.Walker.Ancestors[i]
		if ancestor.Kind != ast.NodeKindField {
			continue
		}
		return v.operation.FieldDeferInfo(ancestor.Ref)
	}
	return 0, "", 0, false
}

func (v *deferAlignTypenameScopeVisitor) removeFieldDeferDirective(fieldRef, directiveRef int) {
	v.operation.Fields[fieldRef].Directives.RemoveDirectiveByRef(directiveRef)
	v.operation.Fields[fieldRef].HasDirectives = len(v.operation.Fields[fieldRef].Directives.Refs) > 0
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `gofmt -w v2/pkg/astnormalization/defer_align_typename_scope.go v2/pkg/astnormalization/defer_align_typename_scope_test.go` then
`gotestsum --format=short -- ./v2/pkg/astnormalization/ -run TestDeferAlignTypenameScope`
Expected: PASS (all 5 subtests).

---

### Task 2: Wire the rule into the normalization pipeline + full-pipeline test

**Files:**
- Modify: `v2/pkg/astnormalization/astnormalization.go` (insert a stage between the `deduplicateFields, deleteUnusedVariables` cleanup stage and the `populateDeferParentIds` stage)
- Test: `v2/pkg/astnormalization/astnormalization_test.go` (extend `TestNormalizeOperation`)

**Interfaces:**
- Consumes: `deferAlignTypenameScope` (Task 1), `o.inlineDeferVisitor.hasDefers()`.

- [ ] **Step 1: Write the failing full-pipeline test**

In `v2/pkg/astnormalization/astnormalization_test.go`, inside `TestNormalizeOperation`, add a subtest (place it next to the existing `"defer parent ids preserved across merged and discarded scopes"` subtest). It asserts the bug query normalizes with a plain, non-deferred `__typename` on the initially-materialized `article`:

```go
	t.Run("deferred __typename on initial entity becomes non-deferred", func(t *testing.T) {
		run(t, `
			type Query { article: Article }
			type Article { id: ID! title: String! reviews: [Review!]! }
			type Review { id: ID! }`, `
			query Q {
				article {
					id
					... @defer { __typename title }
					reviews { id }
				}
			}`, `
			query Q {
				article {
					id
					__typename
					title @__defer_internal(id: 1)
					reviews {
						id
					}
				}
			}`, "", "")
	})
```

Note: the exact field order and any `parentDeferId` are determined by the pipeline. If the captured output differs only in field ordering or an absent/explicit `parentDeferId` on `title` (top-level defer has no parent, so none is expected), adjust the expected to the verified output — but `__typename` MUST appear with no `@__defer_internal` directive, and `title` MUST keep `@__defer_internal(id: 1)`.

- [ ] **Step 2: Run to verify it fails**

Run: `gotestsum --format=short -- ./v2/pkg/astnormalization/ -run 'TestNormalizeOperation/deferred___typename_on_initial_entity_becomes_non-deferred'`
Expected: FAIL — actual still shows `__typename` aliased/deferred (e.g. `__internal___typename: __typename`) because the rule is not wired in yet.

- [ ] **Step 3: Register the stage**

In `v2/pkg/astnormalization/astnormalization.go`, locate the block that appends the cleanup stage:

```go
	o.operationWalkers = append(o.operationWalkers, walkerStage{
		name:   "deduplicateFields, deleteUnusedVariables",
		walker: &cleanup,
	})
```

Immediately after it (and before the `if o.options.enableDefer { ... populateDeferParentIds ... }` block), insert:

```go
	if o.options.enableDefer {
		alignTypename := astvisitor.NewWalkerWithID(8, "AlignDeferTypenameScope")
		deferAlignTypenameScope(&alignTypename)
		o.operationWalkers = append(o.operationWalkers, walkerStage{
			name:          "alignDeferTypenameScope",
			walker:        &alignTypename,
			skipCondition: func() bool { return !o.inlineDeferVisitor.hasDefers() },
		})
	}
```

- [ ] **Step 4: Run to verify it passes**

Run: `gofmt -w v2/pkg/astnormalization/astnormalization.go` then
`gotestsum --format=short -- ./v2/pkg/astnormalization/ -run TestNormalizeOperation`
Expected: PASS (new subtest and all existing `TestNormalizeOperation` subtests). If the new subtest fails only on field ordering / parentDeferId presence, update its expected to the verified output per the Step 1 note, then re-run to PASS.

- [ ] **Step 5: Run the whole astnormalization package**

Run: `gotestsum --format=short -- ./v2/pkg/astnormalization/`
Expected: PASS (no regressions — includes existing defer expand/populate/ensure-typename tests).

---

### Task 3: Engine integration — fill the 3-subgraph test and add the nested case

**Files:**
- Modify: `execution/engine/execution_engine_defer_test.go` — the existing section `t.Run("defer across three federated subgraphs - article reviews authors", ...)` (its subtest `expectedResponse` is currently the placeholder `` `TBD` ``).

**Interfaces:**
- Consumes: the normalization fix from Tasks 1–2 (no new Go symbols).

- [ ] **Step 1: Capture the now-working streaming output**

The fix makes `article.__typename` eager (initial). Temporarily set the section's datasource `conditionalTestCase` to `reportUnused: true, reportUsed: true` (DS1/DS2/DS3) and run:

Run: `go test ./execution/engine/ -run 'TestExecutionEngine_Execute_Defer/defer_across_three_federated_subgraphs' -v 2>&1 | grep -iE "Requested MOCK|unexpected body|actual  :"`

Read the logged `Requested MOCK [...]` bodies. For every upstream query the engine now issues (initial article fetch incl. plain `__typename`, the DS2 `reviews` entity fetch, the DS2 reviews+author defer fetch, the DS3 author `displayName` defer fetch), add a matching entry to the corresponding subgraph's `responses` map with believable data (`title`, `reviews[].id`, `author.id`, `displayName`). Re-run until there are no `received unexpected body` messages and you obtain a clean `actual:` streaming response.

- [ ] **Step 2: Verify the captured response is semantically correct**

Confirm the captured `actual:` satisfies ALL of:
- The initial `data` contains `article` with `id`, a literal `__typename: "Article"`, and `reviews` (each review's `id`) — i.e. no top-level `errors`, `reviews` is populated (not null).
- `title` is delivered in an incremental/deferred frame (defer for the `... @defer { __typename title }` group), NOT in the initial `data`.
- Each review's `author` (`__typename`, `id`, `displayName`) is delivered in its own deferred frame, with `displayName` sourced from DS3.
- No `Cannot return null` errors anywhere.

Only once these hold, set the subtest's `expectedResponse` to the captured multi-line string and set the datasources back to `reportUnused: false` (drop `reportUsed`). If deferred frames race non-deterministically, add per-fetch `latency` values to force a stable order (mirror the existing `extensive parallel defers` / nested-list sections), or use `expectedResponses: []string{...}` with the allowed orderings.

- [ ] **Step 3: Add the nested deferred-entity case**

Add a second subtest in the same section proving a `__typename` that legitimately belongs in a defer scope still works. Use a query shaped like:

```
{ article { id ... @defer { reviews { id author { __typename id displayName } } } } }
```

(here `reviews`/`author` are materialized inside the defer, so `author.__typename` stays in that defer scope). Discover its upstream fetches with `reportUsed: true` as in Step 1, fill responses, verify (no errors; `author.__typename`/`displayName` delivered in the deferred frame), and set `expectedResponse` to the captured, verified output. Reset `reportUsed`/`reportUnused` afterward.

- [ ] **Step 4: Run the section**

Run: `gofmt -w execution/engine/execution_engine_defer_test.go` then
`go test ./execution/engine/ -run 'TestExecutionEngine_Execute_Defer/defer_across_three_federated_subgraphs' -count=5`
Expected: `ok` on all 5 runs (stable, no flakiness).

---

### Task 4: Full regression

**Files:** none (verification only).

- [ ] **Step 1: Run the defer + federation suites**

Run: `gotestsum --format=short -- ./v2/pkg/astnormalization/ ./v2/pkg/engine/datasource/graphql_datasource/ ./execution/engine/`
Expected: PASS. Pay attention to the full `TestExecutionEngine_Execute_Defer` tree (incl. `merged/discarded defer parent`, `nested list entities`, `cross subgraph requires`) and `TestBuildRepresentationVariableNode` — all green.

- [ ] **Step 2: Confirm formatting**

Run: `gofmt -l v2/pkg/astnormalization/ execution/engine/`
Expected: no output (all formatted).

---

## Notes / edge cases (validated by tests above)

- **`__typename`-only defer fragment** on an initially-materialized object (e.g. `... @defer { __typename }` on `article`): after alignment `__typename` is non-deferred, so no field carries that defer id and the group simply does not exist — delivered in the initial response. (Covered by Task 1 strip tests + Task 2 pipeline behavior.)
- **Placeholders from `deferEnsureTypename`**: that rule already stamps its injected `__typename` with the enclosing object's defer id, so `curID == enclosingID` and `deferAlignTypenameScope` no-ops on them.
- **Deeper nesting** (`obj@1 { ... @defer { __typename } }`): `__typename` realigns to `obj`'s id (Task 1, test 3 shape).
