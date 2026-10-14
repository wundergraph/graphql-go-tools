# Optional nullability relaxation for field merging

The default validator enforces matching nullability for fields with the same
response name, including fields in mutually exclusive branches. Calling
`WithRelaxFieldSelectionMergingNullability()` deliberately relaxes that rule for
fields whose immediate enclosing types cannot overlap.

For example, given an `Entity` union of `User` and `Organization`, where
`User.email` is `String!` and `Organization.email` is `String`, this query fails
strict validation and passes with the option enabled:

```graphql
{
  entity {
    ... on User { email }
    ... on Organization { email }
  }
}
```

Each entity resolves to one concrete type, so only one of these branches applies
to any response object. Clients using the option must allow a nullable email.
This execution property motivates the option; it does not make the query valid
under strict GraphQL validation.

## Specification and deliberate deviations

[FieldsInSetCanMerge](https://spec.graphql.org/September2025/#FieldsInSetCanMerge())
requires compatible response shapes for every pair of selections sharing a
response name. Field names and arguments must also match when the enclosing
types are the same or either type is not an object. Distinct concrete object
branches permit different field identities, including in their descendants.

[SameResponseShape](https://spec.graphql.org/September2025/#SameResponseShape())
requires matching non-null wrappers, list structure, and named leaf types.
Composite type names may differ, provided their selected subfields have
compatible response shapes. These shape requirements apply even to mutually
exclusive branches.

The option skips only the non-null wrapper requirement for eligible pairs.
List nesting must still match, scalar and enum names must still match, and
composite subfields are still checked. It does not relax field-name or argument
checks.

Separately, the validator preserves an existing compatibility exception: an
interface and an object that does not implement it may select different names
or arguments under the same response name. These branches cannot apply to the
same runtime object. Their descendants inherit that exclusivity, but response
shapes are still checked. For example, when `Photo` does not implement `Node`,
`{ search { ... on Node { x: id } ... on Photo { x: url } } }` remains accepted
if the selected fields have compatible response shapes. This deliberately
deviates from FieldsInSetCanMerge even when nullability relaxation is disabled.
An interface and an implementing object still require matching field identities.

## Implementation

`fieldSelectionMergingVisitor` derives `ignoreNullability` from the option and
`potentiallySameObject` for the two immediate enclosing types:

| Enclosing types | Potential overlap | Nullability with the option |
| --- | --- | --- |
| Same object type | Yes | Strict |
| Different object types | No | Relaxed |
| Two interfaces | Assumed possible | Strict |
| Interface and implementing object | Yes | Strict |
| Interface and non-implementing object | No | Relaxed |
| Other combinations | No, by the helper's existing policy | Relaxed |

`Document.TypesHaveCompatibleResponseShape` shares the AST type-wrapper
comparison and uses the already resolved named-type definitions. It compares
leaf names exactly and permits differing composite names. The visitor checks
selected subfields separately. `__typename` participates as an implicit
`String!` field, even without the base schema.

The validator does not use `TypesAreCompatibleDeep` or
`TypesAreCompatibleIgnoringNullability`: their object/interface and object/union
compatibility rules are not response-shape validation. Inherited branch
exclusivity, including the compatibility exception above, controls identity checks
separately from the immediate-parent policy
for nullability relaxation.

## Subgraph compatibility

Accepting a query at the router does not ensure a strict subgraph will accept it.
The abstract-selection planner adds internal aliases to conflicting fields on
concrete branches, preserving the client's response names. This includes
composite fields such as `[[X!]!]!` and `[[Y]]` when their selected subfields are
compatible. Planner regression tests validate the generated operation using the
strict validator and check that the response plan retains the client field names.
