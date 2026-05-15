# Query Order Baseline Mock Test — Design

## Goal

Build an isolated, repeatable test in `v2/` that produces a printed federation query plan matching the target shape below.
The test locks in current planner behavior so that upcoming changes on the `improve-query-order` branch can be evaluated by a string-diff against this baseline.

Target plan shape (printer-faithful: all fetch nodes use `Fetch`, no `SingleFetch`/`EntityFetch` labels; the `Flatten(path: "...")` wrapper only appears when the fetch path contains a dot):

```
QueryPlan {
  Sequence {
    Parallel {
      Fetch(service: "user-subgraph") {
        {
          me {
            firstName
            lastName
            id
            __typename
          }
        }
      }
      Fetch(service: "organisation-subgraph") {
        {
          organisations(ids: $a) {
            name
            shortCode
            id
          }
        }
      }
    }
    Fetch(service: "practice-subgraph") {
      {
        fragment Key on User {
          __typename
          id
        }
      } =>
      {
        _entities(representations: $representations) {
          ... on User {
            __typename
            currentPractice {
              id
            }
          }
        }
      }
    }
  }
}
```

Note: the user-supplied plan was a stylized view that wrote `SingleFetch` and `EntityFetch` as separate labels.
The actual `PlanPrinter.Print()` output (see `v2/pkg/engine/resolve/fetchtree.go:333`) emits `Fetch(...)` for every fetch node and only differentiates entity fetches by the `fragment Key on … => …` representations block printed before the query.

The exact final golden string is set after first run.
The first run captures whatever `PrettyPrint()` actually emits today;
that becomes the baseline.

## Scope

- One new file: `v2/pkg/engine/datasource/graphql_datasource/query_order_baseline_test.go`.
- Three minimal federated subgraphs: `user-subgraph`, `organisation-subgraph`, `practice-subgraph`.
- One operation with a client-forwarded `$a: [ID!]!` variable.
- Full-string assertion on `Fetches.QueryPlan().PrettyPrint()`.

## Non-goals

- Modifying the planner.
- Adding multiple variants (those come after the baseline is stable).
- Asserting on the resolve `SynchronousResponsePlan` struct in addition to the printed plan.
- Touching `RunTest()` in `datasourcetesting`.

## Subgraph schemas

**user-subgraph:**

```graphql
type Query {
  me: User
}

type User @key(fields: "id") {
  id: ID!
  firstName: String!
  lastName: String!
}
```

**organisation-subgraph:**

```graphql
type Query {
  organisations(ids: [ID!]!): [Organisation!]!
}

type Organisation {
  id: ID!
  name: String!
  shortCode: String!
}
```

**practice-subgraph:**

```graphql
type User @key(fields: "id") {
  id: ID! @external
  currentPractice: Practice
}

type Practice {
  id: ID!
}
```

## Operation

```graphql
query Baseline($a: [ID!]!) {
  me {
    firstName
    lastName
    currentPractice {
      id
    }
  }
  organisations(ids: $a) {
    name
    shortCode
    id
  }
}
```

## Implementation outline

The test follows the planner-direct path used by `datasourcetesting.go` lines 194–222, but bypasses the struct assertion.

### Schema layout

The test needs **two distinct kinds of schema input**, and conflating them would produce duplicate-type compile errors:

1. **Operation `definition`** — a single composed (supergraph-style) GraphQL schema containing the union of all types from all three subgraphs, with no federation directives.
   This is what `unsafeparser.ParseGraphqlDocumentString` parses into `def`, then `asttransform.MergeDefinitionWithBaseSchema` adds the base schema to.
   It is what the operation is validated against.
2. **Per-datasource `ServiceSDL`** — each subgraph's federated SDL, with its own `@key`/`@external` directives, passed only to that datasource's `SchemaConfiguration` via `FederationConfiguration{Enabled: true, ServiceSDL: ...}`.

The three subgraph SDLs in this design must NOT be concatenated to form `definition` — that would duplicate `type Query` and `type User`.
The composed `definition` is built once and looks roughly like:

```graphql
type Query {
  me: User
  organisations(ids: [ID!]!): [Organisation!]!
}

type User {
  id: ID!
  firstName: String!
  lastName: String!
  currentPractice: Practice
}

type Organisation {
  id: ID!
  name: String!
  shortCode: String!
}

type Practice {
  id: ID!
}
```

### Steps

1. Parse the composed `definition` SDL and the operation via `unsafeparser.ParseGraphqlDocumentString`.
2. Merge base schema via `asttransform.MergeDefinitionWithBaseSchema(&def)`.
3. Normalize the operation via `astnormalization.NewWithOpts(astnormalization.WithExtractVariables(), astnormalization.WithInlineFragmentSpreads(), astnormalization.WithRemoveFragmentDefinitions(), astnormalization.WithRemoveUnusedVariables())`.
   The client-supplied `$a` is preserved here; `WithExtractVariables` only renames inline-literal extractions, not existing `ValueKindVariable` arguments (`v2/pkg/astnormalization/variables_extraction.go:62`).
4. Validate via `astvalidation.DefaultOperationValidator()`.
5. Build a `plan.Configuration` with three datasources via `mustDataSourceConfiguration` (defined in `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_test.go:47`, package-private, accessible because the new test file lives in the same `package graphql_datasource` test package):
   - Each datasource gets `plan.DataSourceMetadata` with `RootNodes`, `ChildNodes`, and `FederationMetaData.Keys`.
     Per the type's own comments (`v2/pkg/engine/plan/datasource_configuration.go:39-50`): for federation, `RootNodes` contain root query type fields, entity type fields, and entity object fields; `ChildNodes` contain non-entity type fields.
     Concretely:
     - **user-subgraph** `RootNodes`: `Query.me`, and `User.{id, firstName, lastName}` (User is the entity).
       `ChildNodes`: none.
       `Keys`: `{TypeName: "User", SelectionSet: "id"}`.
     - **organisation-subgraph** `RootNodes`: `Query.organisations`.
       `ChildNodes`: `Organisation.{id, name, shortCode}` (Organisation is not an entity).
       `Keys`: none.
     - **practice-subgraph** `RootNodes`: `User.{id, currentPractice}` (User is an entity in this subgraph too).
       `ChildNodes`: `Practice.{id}` (Practice is not an entity).
       `Keys`: `{TypeName: "User", SelectionSet: "id"}`.
   - Each datasource gets a `Configuration` with `FetchConfiguration{URL: "..."}` and a `SchemaConfiguration` built via `mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: <subgraph-sdl>}, ...)`.
   - **Do not set `RequiresEntityFetch` in metadata** — that flag lives on the generated `resolve.FetchConfiguration` and is set by the planner when it decides to issue an `_entities` fetch. Metadata only provides root nodes, child nodes, and keys; the planner picks entity fetches based on which subgraph owns which fields.
   - **`plan.Configuration.Fields` must declare argument forwarding for `Query.organisations(ids)`.** Without this, validation fails because the planner does not forward variable arguments to root fields by default. Concretely:
     ```go
     Fields: plan.FieldConfigurations{
         {TypeName: "Query", FieldName: "organisations", Arguments: plan.ArgumentsConfigurations{
             {Name: "ids", SourceType: plan.FieldArgumentSource},
         }},
     },
     ```
6. Construct `plan.NewPlanner(config)` and call `p.Plan(&op, &def, "Baseline", &report, plan.IncludeQueryPlanInResponse())`.
7. Run `postprocess.NewProcessor().Process(actualPlan)`. **This is required.** `Plan()` leaves fetches in `RawFetches` until the postprocessor materializes the fetch tree at `Response.Fetches`. Without postprocessing, `Fetches.QueryPlan()` returns nil.
8. Extract printed plan via `actualPlan.(*plan.SynchronousResponsePlan).Response.Fetches.QueryPlan().PrettyPrint()`.
9. Assert against the inline expected string after `strings.TrimSpace` on both sides.
   The PrettyPrint output ends with a trailing newline; existing tests in `v2/pkg/engine/resolve/fetchtree_test.go` trim before comparing, and the new test does the same.

### Sanity-check assertions alongside the golden

A pure golden-string assertion can bless a structurally wrong plan (the planner could regress in a way that produces a different but equally-pinned output, and the test would still pass once the golden is updated).
To catch this, the test asserts a few structural invariants on the `*FetchTreeQueryPlanNode` returned by `Fetches.QueryPlan()` (the same projection used for printing).

`FetchTreeQueryPlanNode` (`v2/pkg/engine/resolve/fetchtree.go:144-163`) exposes these directly: `Kind`, `Children`, and `Fetch *FetchTreeQueryPlan`, where `FetchTreeQueryPlan` has `Kind` (`"Single"`, `"Entity"`, or `"BatchEntity"` — the latter only for batched entity fetches, which are not expected in this baseline), `SubgraphName`, `Path`, and `Representations`.
These are not on the underlying `Fetch` interface — that's only reachable via `FetchInfo()` — so the structural assertions are written against the query-plan projection, not the raw fetch tree.

Invariants:

- The root node is `FetchTreeNodeKindSequence` with exactly two children.
- The first child is `FetchTreeNodeKindParallel` with two children whose `Fetch.SubgraphName` set equals `{"user-subgraph", "organisation-subgraph"}` (order-independent).
   Both have `Fetch.Kind == "Single"`.
   The two `Fetch` pointers are kept in a `map[string]*FetchTreeQueryPlan` keyed by `SubgraphName` so the next invariant can reference the user fetch's `FetchID`.
- The second child is `FetchTreeNodeKindSingle` with:
  - `Fetch.Kind == "Entity"`,
  - `Fetch.SubgraphName == "practice-subgraph"`,
  - `Fetch.DependsOnFetchIDs == [user-subgraph fetch ID]` exactly (not the organisation fetch, not both — proves the entity fetch is wired to the correct upstream),
  - `Fetch.Representations` of length 1, with `Kind == resolve.RepresentationKindKey`, `TypeName == "User"`, and `Fragment` containing both `__typename` and `id`.

These invariants encode "what the plan SHOULD look like" independently of formatting.
They are short.
The string golden remains the primary diff surface; the structural checks are insurance.

### Establishing the expected string

The expected string is established by capturing the actual output on first run.
The first run also confirms the structural invariants pass — only then is the captured string committed as the golden.

## Resolved questions

These were open in earlier drafts; verified during design review against the source.

- **`mustDataSourceConfiguration` accessibility.** Defined in `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_test.go:47`. Package-private (lowercase), but the new test file is in `package graphql_datasource` (same test-package), so it has direct access.
- **`$a` is preserved by normalization.** `WithExtractVariables` returns early when the argument is already `ValueKindVariable` (`v2/pkg/astnormalization/variables_extraction.go:62`). It only generates `$a`/`$b` for inline-literal extraction. The client's `$a` survives unchanged.
- **No `Flatten` wrapper for the practice fetch.** `Flatten(path: "...")` is emitted only when `fetch.Path` contains a dot (`v2/pkg/engine/resolve/fetchtree.go:333-339`). For an entity fetch under `me`, the path is `"me"` (no dot), so it prints as a bare `Fetch(...)`. A `Flatten("me.currentPractice")` would only appear if the planner pushed the fetch deeper.
- **Composition.** Both `wgc router compose` and `rover supergraph compose` were run against the three subgraph SDLs (with `@link` directives added) and produced clean output.

## Open risks

- **Field order inside `me`.** The printed selection set order is planner-determined. The user-supplied target shows `firstName lastName id __typename`, but existing planner output in similar entity-enabling fetches sometimes appends federation key fields after user fields (e.g. `firstName lastName __typename id`). First-run output is authoritative; the structural invariants do not depend on field order, so a small drift here is fine.
- **Order inside the `Parallel` block.** `createParallelNodes` (`v2/pkg/engine/postprocess/create_parallel_nodes.go:17`) groups dependency-free fetches, but the order of children within a `Parallel` is the planner's emission order, not a stable rule. The structural invariants match by `SubgraphName` rather than position to absorb this.
- **Baseline blesses current behavior.** A pure golden-string assertion will pass for any output the test captures on first run, including a regressed one. The structural invariants in the implementation outline mitigate this — they encode what the plan *should* look like (Sequence around two children; Parallel grouping user + organisation; entity fetch on User into practice) and would fail loudly if the planner produces a categorically wrong shape.

## Validation

- Run the new test with `-v` and capture the actual printed plan output.
- Update the inline `expected` constant to match the captured output.
- Re-run; the test must now pass.
- Manually compare the captured output against the user-supplied target.
  Document any structural differences (e.g. fetch order, field order) as findings, not as failures.
- Commit the test file plus the captured baseline.
- Future planner changes that produce a different plan now fail this test with a readable string diff, which is the whole point of the baseline.
