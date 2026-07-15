# MultiFetch — implementation guide

This document walks through how the MultiFetch feature works at each level of
the engine, in execution order: planning → postprocessing → runtime →
observability. It complements the design spec (`spec.md`, the behavioral
source of truth) by mapping the design onto the actual code.

## What it does

When one operation resolves entities from the same subgraph at several places
in the query plan — and those fetches would execute in the same parallel wave —
the engine merges them into **one** HTTP request with aliased `_entities`
fields, each guarded by its own `@include` variable:

```graphql
# instead of two requests to the products subgraph:
query($representations_f1: [_Any!]!, $includeF1: Boolean!,
      $representations_f2: [_Any!]!, $includeF2: Boolean!) {
  f1: _entities(representations: $representations_f1)@include(if: $includeF1) {
    ... on Employee { __typename products }
  }
  f2: _entities(representations: $representations_f2)@include(if: $includeF2) {
    ... on Employee { __typename notes }
  }
}
```

The merged operation is printed **once at plan time** and cached with the
plan; at runtime only variables change. A sub-fetch that has nothing to send
(no live representations, or denied by pre-fetch authorization) is switched
off with `includeFN: false` — the operation is never rebuilt during execution.

Enablement is opt-in and requires **both** flags:

- `plan.Configuration.EnableMultiFetch = true` — the graphql datasource
  records merge artifacts on entity fetches;
- `postprocess.EnableMultiFetch()` processor option — the merge stage runs.

With either flag off there is zero behavior change; plans are byte-identical.

## Level 1: planning (`datasource/graphql_datasource`)

The datasource planner already builds a private upstream operation AST and
prints it into the fetch input string
(`{"method":"POST","url":...,"body":{"query":"...","variables":{...}}}`).
Merging two printed strings later is not reliably possible, so when the flag
is on the planner keeps the structured artifacts a merge needs, on
`resolve.FetchConfiguration.MergeableOperation`:

- **`Document`** — the normalized, validated upstream operation AST
  (`*ast.Document`). `ConfigureFetch` transfers ownership by nil-ing its own
  reference (`p.upstreamOperation = nil`), so nothing can mutate it afterwards.
- **`Variables`** — the top-level `body.variables` entries `(name, raw
  fragment)` in write order. Fragments are raw bytes that may contain `$$N$$`
  placeholders indexing `FetchConfiguration.Variables` (e.g. the
  representations entry is always `[$$N$$]`).

Recording happens in `setUpstreamVariable` (graphql_datasource.go), a thin
wrapper around the existing `sjson.SetRawBytes` write sites (field arguments,
directive arguments, object-source arguments, the representations variable,
and the normalizer-extracted literals merged in `createInputForQuery`). It
replaces in place on duplicate names; a duplicate write to the
`representations` slot marks the fetch non-mergeable (a client variable
literally named `representations` collides with the synthetic key — a
pre-existing planner defect that MultiFetch refuses to inherit).

Artifacts are stored only for entity fetches (`RequiresEntityFetch` /
`RequiresEntityBatchFetch`) on plain HTTP GraphQL datasources (gRPC is
excluded). They are transient: postprocessing consumes and clears them (see
below), so no AST ever reaches the cached executable plan.

Flag threading follows the `EnableOperationNamePropagation` precedent:
`plan.Configuration.EnableMultiFetch` → `plannerConfigurationOptions` →
`DataSourcePlannerConfiguration.Options` → read in `EnterDocument`.

## Level 2: postprocessing (`postprocess/create_multi_fetch*.go`)

The pipeline order in `processFlatFetchTree` is:

```
collectAuthorizationCoordinates → dedupe → appendFetchID
→ addMissingNestedDependencies   (moved before template resolution)
→ createMultiFetch               (NEW — needs complete dependency edges
                                  and untouched Input/Variables)
→ resolveInputTemplates → createConcreteSingleFetchTypes
→ orderSequenceByDependencies → createParallelNodes
```

`createMultiFetch.ProcessFetchTree` always runs; when the option is off it
only clears `MergeableOperation` from every fetch (unconditional cleanup).
When on, it works in four steps:

### 2.1 Candidates and grouping (`create_multi_fetch.go`)

A fetch is a candidate iff it is a `SingleFetch` entity fetch with a non-nil
`MergeableOperation`, a non-nil `FetchInfo` (needed for grouping and per-entry
authorization; `plan.DisableIncludeInfo` silently disables the feature), and a
well-formed variables record (exactly one `[$$N$$]` fragment whose variable is
the `ResolvableObjectVariable`).

Grouping must answer "which fetches would execute simultaneously?" without
duplicating scheduler logic. The stage partitions the flat child list by
`DeferID` (defer groups are organized separately later, and out-of-partition
dependencies are never satisfiable there) and, per partition, runs the REAL
organize stages — `orderSequenceByDependencies` + `createParallelNodes` — on a
scratch tree. Nodes that land in the same parallel group form a wave; a wave's
candidates are then bucketed by `Info.DataSourceID`. Buckets with ≥ 2 members
merge. Zero drift from the actual scheduler by construction.

### 2.2 Merged document (`create_multi_fetch_document.go`)

`buildMergedOperation(members)` builds a fresh `ast.Document` with one query
operation (named `<OperationName>__multi_<id1>_<id2>` when every member shares
a propagated operation name, anonymous otherwise). Per member k (1-based):

- a rename map `name → name_f<k>` is built over the union of the member
  document's variable definitions AND the recorded fragment names (the blob
  can contain stale keys the document no longer declares);
- variable definitions are imported with the **name** renamed
  (`astimport.ImportVariableDefinitionWithVariableNameRename`);
- a synthetic `$includeF<k>: Boolean!` definition is added — the camel-case
  name is deliberately outside the rename image (every renamed name ends in
  `_f<digits>`), so it can never collide with a renamed client variable;
- the member's root selection set is imported via
  `astimport.ImportSelectionSetWithVariableRename` (recursive, cross-document,
  renaming every variable reference inside arguments and directives; fragment
  spreads are rejected — upstream operations never contain them);
- the imported `_entities` field gets alias `f<k>` and an appended
  `@include(if: $includeF<k>)` directive, and is attached to the merged
  operation.

The document prints twice: compact (embedded into the input) and pretty (the
merged `Info.QueryPlan.Query` when every member carries a query plan). No
re-normalization or re-validation happens — members are already normalized and
validated, aliases keep root fields disjoint, and the renaming is total.

### 2.3 Merged input (`create_multi_fetch_input.go` + `mergeGroup`)

The first member's post-visitor `Input` string supplies the envelope.
`splitEntityFetchInput` locates the `body.query` string value and the
`body.variables` object value, supporting both real-world key orders:

- **repo shape** (sjson v1.0.4 via `go.work` replace — prepends keys):
  `{"method":...,["header":...,]"body":{"query":"...","variables":{...}}}`.
  The query is found via the last `"body":{"query":"` anchor; the variables
  object via a backward, quote-and-escape-aware brace scan from the `}}}`
  tail (backslash-run parity decides whether a quote is escaped).
- **append shape** (sjson v1.2.5+ downstream):
  `{"body":{"variables":{...},"query":"..."},...}`. The variables object is
  scanned forward; the query end is the **unique** `"}`-then-`,` position
  preceded by `}` — ambiguity (e.g. header values ending in raw `}`) fails
  safe and the group is left unmerged.

The raw, unescaped query text is never scanned through — both shapes bound it
by its neighbors. Before merging, every member's "envelope remainder" (input
minus the two value ranges) must be byte-identical to the first member's, and
every `$$K$$` token inside the remainder must reference `.Equals()`-equal
variables — otherwise the group is not merged.

`mergeGroup` then assembles the concrete `resolve.MultiEntityFetch`:

- **Header** template: everything up to the variables object, with the query
  range replaced by the merged compact operation, ending in `"variables":{`;
- **Entries** (one per member, in wave order): precomputed statics
  `RepresentationsPrefix` (`"representations_f1":[`, with a leading comma from
  the second entry on), `IncludePrefix` (`],"includeF1":`), the
  representations item template (the member's `ResolvableObjectVariable`
  segment, null-propagating), the member's other recorded variables as
  `{KeyPrefix: ',"name_fk":', Value: template}`, the original `FetchItem`
  placement (with `Fetch` deliberately nil — a backpointer would make the plan
  cyclic and would keep the merged-away member fetch and its document alive),
  the member's `FetchInfo`, per-alias `PostProcessing`
  (`["data","fN"]`/`["errors"]` + original merge path), and the origin kind
  (single vs batch — needed for one response edge case);
- **Footer** template: the variables-object close plus the envelope tail;
- merged `FetchDependencies` (lowest member ID; deduplicated dependency union
  minus member IDs) and merged `FetchInfo` (same subgraph identity; union
  root fields; concatenated coordinate dependencies / fetch reasons; merged
  query plan);
- tree surgery: the first member node is replaced, the others removed, and
  `replaceDependsOnFetchID` rewrites EVERY member ID (including the first
  member's own, when it is not the group minimum) to the surviving ID across
  the tree, so dependents and the loader's skip cascade stay correct.

## Level 3: runtime (`resolve/loader_multi_entity.go`)

### 3.1 Prepare (`prepareMultiEntityFetch`, under the data lock)

One pooled `batchEntityTools` (hash + arena) serves all entries; between
entries only the dedup state is cleared (`clearDedupState`) — never the arena,
whose per-entry buffers must survive until assembly. Per entry:

1. select merge targets via the entry's own `FetchPath`
   (type-condition- and taint-aware);
2. authorization first, input-free: `isFetchAuthorized(nil, entry.Info, res)`
   — for query-typed entity fetches the only reachable denial is the
   pre-fetch field-authorization cache, which marks the entry skipped;
   a denied entry's representations are **never rendered into the request**;
3. render representations exactly like a batch entity fetch: per item, skip
   flags, xxhash dedup, comma separators, per-entry `batchStats` copied to
   the heap. Zero unique items ⇒ entry excluded.

Assembly writes one buffer: Header, then per entry
`"representations_fN":[<items or empty>],"includeFN":<bool>` plus its other
variables — a variable pair is omitted entirely iff it rendered `null`
**because** an undefined client variable was collected during that render
(explicit client `null` stays; this reproduces the engine's undefined-variable
stripping, which cannot match renamed keys) — then Footer. If every entry is
excluded the whole fetch is skipped before the rate limiter runs. Otherwise
rate limiting runs **once** with the merged input and merged info.

### 3.2 Load

Unchanged single-fetch machinery: one `executeSourceLoad`, one HTTP request,
one `LoaderHooks.OnLoad`, one trace, one `body.extensions` / fetch-reasons
injection, single-flight compatible (the merged fetch is query-typed).

### 3.3 Merge (`mergeMultiEntityResult`, under the data lock)

- All entries excluded ⇒ nothing to do.
- Transport error / auth / rate-limit rejection / empty body ⇒ the transport
  state is copied onto every entry **except** those excluded at prepare time
  (their unmerged counterparts were never sent), and each entry's ordinary
  `mergeResult` guards render the failed-to-fetch / rate-limit errors at that
  entry's response path. Only a genuine transport error (`res.err`) records
  the fetch ID for the dependent-skip cascade — invalid responses do not,
  matching unmerged behavior.
- Success: the body is parsed **once**; response extensions are collected
  once; `response.errors` is partitioned by leading path element (`"f1"` → 
  entry 1; unmatched errors attribute once to the merged fetch). Each entry
  then runs the standard `mergeResult` with a small per-entry view
  (`result.multi`): pre-parsed response, pre-partitioned errors, alias data
  path, the entry's taint info, and its `batchStats` fan-out. Two multi-aware
  branches inside `mergeResult` keep client-visible parity:
  - error paths: with subgraph error-path rewriting on, the alias is treated
    as the entity root (index dropped, response path prefixed); with it off,
    a leading alias is still rewritten to `_entities` so internal aliases
    never leak in pass-through mode;
  - a single-origin entry whose alias returned an empty array is a benign
    no-op (unmerged `["data","_entities","0"]` selects null → silence), while
    a batch-origin count mismatch keeps today's error.
- Per-entry subgraph errors are joined into the shared result and
  `OnFinished` fires once.

## Level 4: observability

- **ART traces** (`fetchtree.go Trace()`): a `MultiEntity` trace node with the
  single load trace plus `entries: [{alias, path}]` listing every merged
  sub-fetch.
- **Query plans** (`fetchtree.go queryPlan()`): a `MultiEntity` fetch node
  carrying the merged query (pretty form), the union representations,
  `mergedFetchIds`, and per-entry `{alias, path}` pairs. `PrettyPrint` renders
  it like a single fetch — one `Fetch(service: ...)` with more variables.

## Behavior guarantees

The merge is a transport optimization: data, errors, error paths,
authorization outcomes, and skip semantics match unmerged execution. The
deliberate, documented divergences (rate limiting counts one call instead of
N; a variable-coercion failure poisons the whole merged request; a denied
entry's non-representations variables are transmitted but never executed;
non-representations null variables render in place instead of nulling the
whole input) are listed in spec section 5.1.

## File map

| Area | Files |
|---|---|
| Flag threading | `plan/configuration.go`, `plan/planner_configuration.go`, `plan/datasource_configuration.go` |
| Artifact recording | `datasource/graphql_datasource/graphql_datasource.go` (`setUpstreamVariable`, `ConfigureFetch`) |
| Artifact types + fetch type | `resolve/fetch_multi.go`, `resolve/fetch.go` |
| AST merging primitives | `astimport/astimport.go` (`ImportSelectionSetWithVariableRename`, `ImportVariableDefinitionWithVariableNameRename`) |
| Merge stage | `postprocess/create_multi_fetch.go` (stage, candidates, waves, tree surgery), `create_multi_fetch_document.go` (document merge), `create_multi_fetch_input.go` (dual-shape input scanner) |
| Pipeline wiring | `postprocess/postprocess.go` (`EnableMultiFetch()`, stage order), `resolve_input_templates.go` / `deduplicate_single_fetches.go` (shared helpers) |
| Runtime | `resolve/loader_multi_entity.go` (prepare + merge), `resolve/loader.go` (dispatch, `mergeResult` multi branches, alias hiding), `resolve/tainted_objects.go` |
| Observability | `resolve/fetchtree.go` |

## Test map

| Concern | Tests |
|---|---|
| AST import primitives | `astimport/astimport_test.go` (`TestImportSelectionSet*`, `TestImportVariableDefinitionWithVariableNameRename`) |
| Artifact recording | `graphql_datasource_test.go` (`TestConfigureFetch_MergeableOperation`) |
| Grouping/waves/clearing | `postprocess/create_multi_fetch_test.go` (`TestCreateMultiFetch_CollectGroups`, `_PipelineClearingUnconditional`, `_PipelineDisableResolveInputTemplates`) |
| Document merge | `TestBuildMergedOperation` (full compact + pretty golden equality) |
| Input scanner + assembly | `TestSplitEntityFetchInput`, `TestCreateMultiFetch_MergeGroup` (+ aborts, survivor-ID rewrite, append shape, three members) |
| Loader prepare | `resolve/loader_multi_entity_test.go` (`TestPrepareMultiEntityFetch_*`: assembly golden, dedup isolation, empty/denied/all-excluded, undefined variables) |
| Loader merge | `TestMergeMultiEntityResult_*` (fan-out, error partitioning in both propagation modes, empty-array origins, transport/invalid/rate-limit, taint, extensions/hooks once) |
| End-to-end plans (full-plan equality via `datasourcetesting.RunTest`) | `graphql_datasource_multi_fetch_test.go` (`TestGraphQLDataSourceFederation_MultiFetch` + `_ThreeFetchGroup`, `_WaveSeparation`, `_Subscription`) |
| Runtime integration | `TestLoadGraphQLResponseData_MultiEntity` (one Load call, input golden, byte-identical result vs unmerged run), `_SingleFlight` |
