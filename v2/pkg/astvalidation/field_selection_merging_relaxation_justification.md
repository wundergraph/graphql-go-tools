# Justification: Relaxed Field Selection Merging on Non-Overlapping Concrete Types

## Problem Statement

The Router rejects valid-in-practice queries where inline fragments on different
concrete union/interface member types select the same field with different
nullability:

```graphql
# Schema:
union Entity = User | Organization
type User { email: String! }
type Organization { email: String }

# Query (rejected):
{
  entity {
    ... on User { email }
    ... on Organization { email }
  }
}
# Error: "fields 'email' conflict because they return conflicting types 'String!' and 'String'"
```

## What the Spec Says

The GraphQL spec ([sec 5.3.2 "Field Selection Merging"](https://spec.graphql.org/October2021/#sec-Field-Selection-Merging))
defines two algorithms:

1. **FieldsInSetCanMerge**: For each pair of fields with the same response name,
   calls `SameResponseShape`. If parent types are the same (or either is not an
   Object Type), also checks that field names and arguments match.

2. **SameResponseShape**: Checks structural type compatibility. Step 2 states:
   "If typeA or typeB is Non-Null: if the other is nullable, return false."
   This check runs **unconditionally** — it has no concept of parent type
   mutual exclusivity.

The `FieldsInSetCanMerge` algorithm *does* have a concept of mutual exclusivity
(step 3: "if parentType1 is not equal to parentType2, and both are Object Types"),
but this flag **only** relaxes the field-name and argument-equality checks — not
the `SameResponseShape` check.

**Per the literal spec text, our query is invalid.**

## What the Reference Implementation Does

The [graphql-js reference implementation](https://github.com/graphql/graphql-js/blob/main/src/validation/rules/OverlappingFieldsCanBeMergedRule.ts)
mirrors the spec exactly. The `doTypesConflict` function runs **outside** the
`!areMutuallyExclusive` guard:

```typescript
// In findConflict():
if (!areMutuallyExclusive) {
  // Only check field name + argument equality when types could overlap
}

// TYPE CHECK — ALWAYS RUNS, regardless of areMutuallyExclusive
if (type1 && type2 && doTypesConflict(type1, type2)) {
  return [/* conflict */];
}
```

**graphql-js also rejects the query.**

## Known Issues About This Behavior

This has been reported multiple times across the ecosystem:

| Issue | Status |
|-------|--------|
| [graphql-js #1361](https://github.com/graphql/graphql-js/issues/1361): "Fragment safe divergence does not consider field nullability" | Closed as "working as designed". Maintainer suggested filing a spec change proposal. |
| [graphql-js #1065](https://github.com/graphql/graphql-js/issues/1065): "GraphQL union and conflicting types" | Closed with same explanation |
| [Netflix DGS #1583](https://github.com/Netflix/dgs-framework/issues/1583) | Closed as "working as designed per spec" |

No spec change proposal has been filed or merged.

## Why the Relaxation Is Safe

Despite deviating from the spec, our relaxation is safe for the following reasons:

### 1. Non-overlapping types can never co-resolve

`User` and `Organization` are distinct concrete object types. At runtime, the
`entity` field resolves to exactly one of them. The `... on User` branch and
the `... on Organization` branch can **never** both apply to the same response
object. Therefore, a client will only ever see one nullability variant per object.

### 2. The base types are still required to match

Our relaxation **only** strips `NonNull` wrappers. It still requires:
- Same list structure (`[String]` vs `String` is still rejected)
- Same base named type (`String` vs `Int` is still rejected)
- Same abstract/concrete compatibility

A nullable `String` is always a valid superset of a non-null `String!`. There is
no type safety issue.

### 3. Conservative for interfaces

When either enclosing type is an **interface**, we conservatively enforce the
strict check. This is because a concrete type could implement both interfaces,
making overlap possible. The relaxation only applies when both enclosing types
are **different concrete Object types**.

### 4. The original spec rationale doesn't apply here

The [original commit](https://github.com/graphql/graphql-js/commit/c034de91acce10d5c06d03bd332c6ebd45e2213c)
that introduced strict type checking for mutually exclusive types gave this
motivating example:

```graphql
... on Person { foo: birthday { bar: year } }   # bar: Int
... on Business { foo: location { bar: street } } # bar: String
```

Here `data.foo.bar` could be `Int` or `String` — genuinely ambiguous. But that
is a **different base type** conflict (`Int` vs `String`), which our fix still
rejects. Nullability differences (`String!` vs `String`) do not create this kind
of ambiguity — a client can always use the nullable type (`String`) as the field
type.

## What We Changed

In `operation_rule_field_selection_merging.go`, when `TypesAreCompatibleDeep`
fails (e.g. `String!` vs `String`), we now check:

1. **Can the enclosing types overlap?** (`potentiallySameObject`)
   - Interface + anything: YES (conservative, strict check)
   - Same object type: YES (strict check)
   - Different object types: NO (relaxed check)

2. **If they can't overlap** (nullability relaxation only): use
   `TypesAreCompatibleIgnoringNullability` which strips `NonNull` wrappers at
   every nesting level but still requires the same list structure and base named
   type. Note: this "base types must match" guarantee applies only to
   `RelaxNullabilityCheck`. The broader `RelaxTypeMismatchCheck` permits
   entirely different base named types on non-overlapping concrete types.

Existing test cases are unaffected:

| Existing Test | Types Involved | Result |
|---------------|---------------|--------|
| `NonNullStringBox1` (interface) + `StringBox` (object) | Interface overlap possible | **Still Invalid** |
| `IntBox.scalar: Int` + `StringBox.scalar: String` | Different base types | **Still Invalid** |
| All "112" tests with `String` vs `Int` on Dog/Cat | Different base types | **Still Invalid** |

## Type Mismatch Relaxation (RelaxTypeMismatchCheck)

In addition to the nullability relaxation above, a broader relaxation is
available via `RelaxTypeMismatchCheck`. This allows **completely different types**
(e.g. `IssueState` vs `PullRequestReviewState`) on fields in non-overlapping
concrete object types.

### Problem

Schemas like GitHub's define interfaces (e.g. `Updatable`) whose implementing
types have same-named fields returning different leaf types:

```graphql
interface Updatable { id: ID! }
type Issue implements Updatable { id: ID!, state: IssueState }
type PullRequestReview implements Updatable { id: ID!, state: PullRequestReviewState }
```

The spec's `SameResponseShape` algorithm rejects `state` across inline fragments
even though only one branch executes at runtime.

### Safety

The same `potentiallySameObject` guard applies: the relaxation only fires when
both enclosing types are different concrete Object types (never for interfaces).
At runtime, the response object can only match one concrete type, so different
field types cannot both appear for the same object.

### Relationship to nullability relaxation

`RelaxTypeMismatchCheck` is strictly broader than `RelaxNullabilityCheck`. When
the type mismatch flag is enabled on non-overlapping types, **all** type
differences (including nullability) are skipped because the code `continue`s
before reaching the nullability check. Both flags exist because callers may want
the narrower nullability-only relaxation without the broader type mismatch
relaxation.

## Conclusion

These are **deliberate, targeted deviations** from the GraphQL specification.
They address real-world pain points that have been independently reported across
multiple GraphQL implementations. Both relaxations are narrowly scoped (only
different concrete object types) and preserve all existing rejection behavior
for genuinely conflicting types on overlapping or same types.
