# MultiFetch: merging same-subgraph entity fetches into one request

Status: spec v2 — revised after an adversarial verification pass (90 findings
checked against the code; all corrections incorporated).
Scope: `v2` engine — packages `plan`, `postprocess`, `resolve`, `astimport`,
`datasource/graphql_datasource`.

Naming: the feature is called **MultiFetch** (flag, postprocess stage). The
concrete resolve types use the **MultiEntity\*** family (`MultiEntityFetch`,
`FetchKindMultiEntity`, `MultiEntityInput`, `MultiEntityFetchEntry`,
`MultiEntityFetchVariable`) because v1 merges entity fetches only — parallel to
`BatchEntityFetch`.

## 1. Problem

When a single GraphQL operation resolves entities from the same subgraph at
multiple points of the query plan (e.g. a list field plus a single-entity field
that both extend types owned by one subgraph), the engine issues separate
`_entities` requests:

```graphql
{
  employees { id products }     # BatchEntityFetch -> products subgraph
  employee(id: 1) { id notes }  # EntityFetch      -> products subgraph
}
```

Today this produces two HTTP requests to the products subgraph that execute in
the same parallel wave. They should be merged into one request using aliased
`_entities` fields:

```graphql
query($representations_f1: [_Any!]!, $includeF1: Boolean!,
      $representations_f2: [_Any!]!, $includeF2: Boolean!) {
  f1: _entities(representations: $representations_f1) @include(if: $includeF1) {
    ... on Employee { __typename products { upc } }
  }
  f2: _entities(representations: $representations_f2) @include(if: $includeF2) {
    ... on Employee { __typename notes }
  }
}
```

The merge is a transport optimization: client-visible behavior (data, errors,
error paths, authorization semantics, skip semantics) must match the unmerged
execution, with the small documented divergences listed in section 5.1.

## 2. Design decisions (Q&A with user)

### Q1: Enablement
Opt-in, **default off**. The stored operation document is transient: it exists
only between planning and the postprocess merge stage. After the joined query
is printed, the document reference is **cleared** — no AST survives postprocess
into the cached/executable plan.

### Q2: Authorization / rate limiting / dynamic exclusion
Per sub-fetch authorization, implemented via GraphQL's built-in `@include`:

- The merged operation is printed **once at postprocess time** and is fully
  static (cacheable in the query plan). Each aliased `_entities` field carries
  `@include(if: $includeFN)` with its own `Boolean!` variable.
- At runtime only variables change — the operation is never rebuilt during
  execution.
- A sub-fetch that is auth-denied or has zero live representations gets
  `includeFN: false` (and an empty `representations_fN: []`, because the
  non-null variable must still pass variable coercion); the subgraph does not
  execute that field. A denied sub-fetch must **not** send its representations.
- If ALL sub-fetches are disabled, no request is sent.
- The MultiFetch stores the original fetches' metadata ("raw fetches":
  FetchInfo, dependencies, response paths — no rendered inputs) so
  authorization runs per sub-fetch exactly as today, with outcomes attributed
  to each sub-fetch's response path.
- Rate limiting guards subgraph calls: the merged fetch is **one** call → one
  `RateLimitPreFetch` check with the merged input and merged FetchInfo.
- `LoaderHooks.OnLoad`/`OnFinished` fire once per HTTP request.

Verified nuance (loader.go:1375-1451): for query-typed fetches — and entity
fetches are always query-typed (path_builder_visitor.go:1289-1300) —
`isFetchAuthorized` never calls `ctx.authorizer.AuthorizePreFetch`; the only
reachable per-fetch denial is the pre-fetch-field-authorizer cache path
(`isFetchAuthorizedFromCache`), which sets `fetchSkipped` only (no
`authorizationRejected`, no loader-rendered error — field-level errors come
from response resolution). Per-entry authorization therefore means: run the
same `isFetchAuthorized` logic per entry with the entry's `FetchInfo`; a
denied entry is excluded exactly like a skipped one. The
`authorizationRejected` machinery remains supported per entry for parity and
future authorizer changes, but is currently unreachable for entities.

### Q3: Merge scope
**Entity fetches only** (`EntityFetch`/`BatchEntityFetch` origins). Same-
subgraph parallel root fetches don't occur in practice — the planner already
merges same-subgraph root fields into a single fetch at planning time.

## 3. Existing architecture (verified facts)

### 3.1 Planning → fetch configuration

`graphql_datasource.Planner[T]` builds a private `upstreamOperation
*ast.Document` while visiting the client operation. Planner instances are
created fresh per `Plan()` call (`factory.Planner` in
`CreatePlannerConfiguration`, datasource_configuration.go:316) and the
planning walker runs once per plan (plan/planner.go:216), so `EnterDocument`'s
`Reset()` branch never executes for a document that `ConfigureFetch` has
stored. `ConfigureFetch` is invoked from `Visitor.LeaveDocument`
(visitor.go:1037-1045, 1224-1241); nothing touches the planner afterwards.

On `ConfigureFetch()` (graphql_datasource.go:323):

- `createInputForQuery()` (line 298) calls `printOperation()` (line 1452),
  which **normalizes** `upstreamOperation` in place against the upstream
  schema, copies out `upstreamOperation.Input.Variables` (literal variable
  values extracted by the normalizer) as `variablesBytes`, validates, prints,
  and optionally **minifies the printed bytes** (`SortAST: true`; the document
  itself is not mutated by minification).
- The final input template is a JSON-shaped string whose **top-level key
  order depends on the effective sjson version**: this repo's `go.work` pins
  `sjson v1.0.4` (a `replace` directive), which **prepends** new keys, so
  inputs here look like
  `{"method":"POST","url":...,["header":...,]"body":{"query":"...","variables":{...}}}`
  (envelope first; inside body, `query` precedes `variables` — verified
  against committed goldens, e.g.
  graphql_datasource_federation_test.go:459, and the BatchEntityFetch
  templates in loader_test.go, whose Header contains envelope+query and whose
  representations sit at the end). Downstream module consumers are NOT
  subject to the go.work replace and resolve sjson v1.2.5+ (append order),
  yielding the mirrored shape `{"body":{"variables":{...},"query":"..."},...}`.
  Any code locating the query/variables ranges must therefore handle **both
  shapes** and bail (skip merging) on anything else. `body.variables` is a
  **flat, one-level object**: each key is an upstream variable name, each
  value is a raw `$$N$$` token, the representations form `[$$N$$]`, or a
  literal JSON value (normalizer-extracted). `$$N$$` is a positional index
  into `FetchConfiguration.Variables` (variables.go:33-49; `AddVariable`
  dedupes by `Equals`). Caveat: literal values can themselves contain literal
  `$$` bytes (normalizer-extracted client strings) — `resolveInputTemplates`
  mis-parses those today; any new `$$`-splitter must reproduce the same
  blind-alternation semantics (bug-for-bug parity).
- The embedded query and header values are **not JSON-escaped**
  (`quotes.WrapBytes` only wraps; httpclient.go:152-163) — pre-existing
  behavior that the merge replicates, never "fixes".
- Writers of the variables blob, all via `sjson.SetRawBytes` with a top-level
  key: `addRepresentationsVariable` (line 849, always `[$$N$$]`),
  `configureFieldArgumentSource` (1157), `addVariableDefinitionsRecursively`
  (1241), `configureObjectFieldSource` (1286), `addDirectiveToNode` (247).
  The normalizer-extracted `variablesBytes` are merged inside
  `createInputForQuery` into a **local copy** of `p.upstreamVariables`
  (line 300); this merge can **overwrite** an existing token entry in place
  (e.g. a variable default extracted under the original name), preserving the
  key's position.
- Upstream **variable names reuse the client names** (or
  `GenerateUnusedVariableDefinitionName` products `a`, `b`, ... for object
  variables); types may be renamed, names are not. Two different fetches can
  therefore use the same name for different values — cross-fetch renaming is
  mandatory when merging.
- Known pre-existing quirk: the recorded blob can contain **stale keys**
  whose variable definitions were replaced during normalization (nested-
  variable extraction, see graphql_datasource_test.go:1783's skipped test).
  The blob, not the document, is the source of truth for `body.variables`
  keys.
- Entity fetches: `requiresEntityFetch()` (object path) /
  `requiresEntityBatchFetch()` (array path) set the flags and pick
  `SingleEntityPostProcessingConfiguration` (`["data","_entities","0"]`) or
  `EntitiesPostProcessingConfiguration` (`["data","_entities"]`). The
  representations variable is the only `ResolvableObjectVariable` of an
  entity fetch's Variables.
- **After** `ConfigureFetch` returns, `plan.Visitor.resolveInputTemplates`
  (visitor.go:1052-1158) post-processes the **whole input string**, converting
  `{{ .request.headers.X }}` occurrences (from datasource header/URL config)
  into `$$N$$` `HeaderVariable` tokens appended to the fetch's Variables. Any
  envelope material captured inside `ConfigureFetch` would still contain raw
  `{{ }}` templates — so envelope bytes must be derived from the
  **post-visitor** `FetchConfiguration.Input`, never stored at ConfigureFetch
  time.
- For gRPC (`p.config.grpc != nil`) the printed operation is re-parsed and the
  DataSource is the gRPC implementation — excluded from MultiFetch.
- Upstream operations contain only fields and inline fragments — the planner
  never creates fragment definitions or spreads, and upstream normalization
  inlines rather than extracts.

### 3.2 Postprocess pipeline

`postprocess.Processor.Process` (postprocess.go:203):
`mergeFields` (response tree) → `createFetchTree` (flat Sequence of Single
nodes from `RawFetches`) → `processFlatFetchTree`:

1. `collectAuthorizationCoordinates`
2. `dedupe` (`deduplicateSingleFetches`) — removes byte-identical fetches,
   merges FetchPath type conditions, rewrites `DependsOnFetchIDs` and
   `CoordinateDependencies` via `replaceDependsOnFetchId`. Model quirks the
   merge stage inherits knowingly: the CoordinateDependencies rewrite only
   happens for nodes whose `DependsOnFetchIDs` contained the removed ID, and
   the rewrite can leave duplicate survivor IDs in a dependent's list (all
   downstream consumers tolerate duplicates).
3. `appendFetchID` (`fetchIDAppender`) — renames propagated operation names in
   `fetch.Input` (string replace) and `QueryPlan.Query`
4. `resolveInputTemplates` — splits `Input` on `"$$"` (blind alternation),
   resolves each numeric chunk against `Variables[i].TemplateSegment()`, then
   **clears** `fetch.Input` and `fetch.Variables`; moves
   `SetTemplateOutputToNullOnVariableNull` onto the template. Its
   `traverseNode` does an **unchecked** cast to `*resolve.SingleFetch`.
5. `addMissingNestedDependencies` — adds dependency edges **only** for fetches
   with a non-empty `ResponsePath` and an **empty** `DependsOnFetchIDs` list,
   when their `ResponsePath` is prefixed by another fetch's provided path
   (`ResponsePath` + `PostProcessing.MergePath`). Reads only paths and
   dependency fields.
6. `createConcreteSingleFetchTypes` — converts `SingleFetch` to
   `EntityFetch`/`BatchEntityFetch` by locating the single
   `ResolvableObjectVariableKind` segment and splitting the template into
   Header/Item(s)/Footer. Sets `SetTemplateOutputToNullOnVariableNull` on
   Header, Items **and** Footer.

then `organizeFetchTree`: `orderSequenceByDependencies` (sort by transitive
dependency sets) → `createParallelNodes` (greedy: node j joins node i's
parallel group iff all of j's `DependsOnFetchIDs` are provided by nodes before
i). For `DeferResponsePlan`, `processFlatFetchTree` runs **before** defer
extraction (the flat tree mixes DeferIDs), and `organizeFetchTree` then runs
**per extracted defer group** (postprocess.go:212-231) — inside a defer group,
dependencies on out-of-group fetches are never "provided", so such fetches
stay serial there. Subscriptions run the same flat pipeline for the response
tree.

Latency consequence: for sync/subscription plans, children of one Parallel
group all start together after every preceding sequence step completes
(loader.go:291) — merging same-wave fetches adds zero latency. For defer
groups the executed grouping can be more serial than the full-list simulation;
merging is still safe (no intra-group dependencies) and can only reduce
latency there.

### 3.3 Loader

`resolveSingle` (loader.go:387) = `preparePhase` (under `dataBuffer.Lock()`;
type switch on fetch type) → `loadPhase` (`executeSourceLoad`: one HTTP call,
trace population, `OnLoad` hook, `body.extensions` + `fetch_reasons`
injection) → `mergePhase` (under lock; `mergeResult` + `callOnFinished`).

- `prepareBatchEntityFetch` (loader.go:1658) is the model: renders Header,
  loops items rendering `Input.Items` per item with skip flags, dedupes
  identical representations by xxhash into `res.batchStats` (unique index →
  merge targets), renders Separator/Footer, collects undefined variables,
  `SetInputUndefinedVariables`, then `validatePreFetch`. Tooling: **one**
  pooled `batchEntityTools` per result (xxhash digest, hash map, arena); the
  arena hosts the prepared-input buffers and the batchStats slices until
  batchStats are **copied to the heap** (loader.go:1767-1773; the
  `*astjson.Value` targets themselves live on the loader's jsonArena, not the
  tools arena); `resolveSingle` returns `prepared.res.tools` to the pool.
- `mergeResult` (loader.go:606): guards in order err → authorizationRejected →
  rateLimitRejected → fetchSkipped → empty out; parse response once
  (loader.go:634); collect response extensions into `l.subgraphExtensions`
  (643-649); select data at `SelectResponseDataPath`; merge errors from
  `SelectResponseErrorsPath` (`mergeErrors`: wrap or pass-through mode;
  `rewriteErrorPaths` runs **only when `rewriteSubgraphErrorPaths` is
  enabled** and matches the literal `"_entities"`); `isEmptyEntityFetch` is
  consulted **only when the selected data is null**; then merge data: single
  item, or `batchStats` fan-out (response array length must equal
  `len(batchStats)`, else `invalidBatchItemCount`).
- Unmerged single-entity semantics: `SelectResponseDataPath
  ["data","_entities","0"]` on an empty `_entities` array selects null →
  `isEmptyEntityFetch` → benign no-op.
- `getTaintedIndices` (tainted_objects.go:22) matches `"_entities"` in error
  paths and reads `fetch.FetchInfo().FetchReasons`.
- `renderContextVariable` (inputtemplate.go:135) records the **client-side**
  variable name when a context variable is undefined;
  `compactAndUnNullVariables` (graphql_datasource.go:1908-1952) later deletes
  `variables.<name>` keys **whose whole value is null**. With renamed upstream
  keys this name-based stripping cannot work — the MultiFetch prepare strips
  inline (4.6).
- Errored-fetch cascade: **only** `loadPhase` on `res.err` records the fetch
  ID (loader.go:355-364); invalid/empty/unparseable responses do not — their
  dependents still run, exactly as unmerged.
- Single flight: keyed by fetch item + input bytes; `singleFlightAllowed` and
  `headersForSubgraphRequest` read `fetchItem.Fetch.FetchInfo()`.

### 3.4 Flag threading precedent

`plan.Configuration.EnableOperationNamePropagation` →
`plannerConfigurationOptions` (planner_configuration.go:21) →
`DataSourcePlannerConfiguration.Options` (datasource_configuration.go:325,369)
→ readable by the datasource planner. The MultiFetch flag follows this path.

## 4. Design

### 4.1 Overview of the pipeline change

```
plan (graphql_datasource):
  ConfigureFetch stores MergeableOperation{Document, Variables}            [flag-gated]
        │
postprocess (new order):
  collectAuthorizationCoordinates
  dedupe
  appendFetchID
  addMissingNestedDependencies      ← moved BEFORE resolveInputTemplates
  createMultiFetch (NEW)            ← consumes MergeableOperation, emits resolve.MultiEntityFetch,
        │                             clears MergeableOperation on ALL fetches
  resolveInputTemplates             ← checked type switch; skips MultiEntityFetch
  createConcreteSingleFetchTypes    ← already skips non-SingleFetch
  orderSequenceByDependencies / createParallelNodes   (unchanged; interface-based)
        │
resolve (loader):
  preparePhase: case *MultiEntityFetch → prepareMultiEntityFetch
  loadPhase: unchanged (one executeSourceLoad)
  mergePhase: multi branch → per-entry demux + mergeResult reuse
```

Moving `addMissingNestedDependencies` before `resolveInputTemplates` is safe:
it reads only `ResponsePath`, `PostProcessing.MergePath`, and
`FetchDependencies` — none produced by `resolveInputTemplates` — and at that
point every child is still a `SingleFetch`, so its unchecked casts hold. The
"must go after dedupe" constraint is preserved.

### 4.2 Enablement flag

- `plan.Configuration.EnableMultiFetch bool` (default false) →
  `plannerConfigurationOptions.EnableMultiFetch` →
  `DataSourcePlannerConfiguration.Options`.
- `graphql_datasource.Planner.ConfigureFetch` populates
  `FetchConfiguration.MergeableOperation` only when the option is set, the
  fetch is an entity fetch, and `p.config.grpc == nil`.
- `postprocess.EnableMultiFetch()` ProcessorOption (default off) sets
  `processorOptions.enableMultiFetch` and activates the stage, which lives in
  `postprocess/create_multi_fetch.go` as `createMultiFetch` with its own slot
  in `FetchTreeProcessors`, invoked in `processFlatFetchTree` between
  `addMissingNestedDependencies` and `resolveInputTemplates`.
- `DisableResolveInputTemplates()` (the test convention that keeps readable
  `Input` strings and also disables concrete fetch types) additionally forces
  the multi stage off — merged fetches have no `Input` string and would break
  plan-test golden assertions (`datasourcetesting.WithDefaultPostProcessor`).
- Document clearing is unconditional: `createMultiFetch.ProcessFetchTree`
  clears `MergeableOperation` on every `SingleFetch` even when `disable` is
  true (the only work done in that case).

### 4.3 Planner artifacts: `resolve.MergeableOperation`

New type in `resolve` (fetch.go already imports `pkg/ast`):

```go
// MergeableOperation carries planner artifacts that allow the postprocess
// MultiFetch stage to join sibling entity fetches to the same subgraph into
// one MultiEntityFetch. It is cleared during postprocessing and never reaches
// the executable plan. Requires postprocessing to run before the plan is
// cached or executed.
type MergeableOperation struct {
    // Document is the normalized and validated upstream operation. Ownership
    // transfers to the plan: ConfigureFetch nils the planner's reference
    // after storing, so no later planner activity can mutate it.
    Document *ast.Document
    // Variables lists the top-level body.variables entries in write order
    // (replace value in place on duplicate name). Write order is NOT the
    // blob's key order under sjson v1.0.4, which prepends new keys; nothing
    // downstream depends on blob order. Values are raw fragments that may
    // contain $$N$$ placeholders referring to FetchConfiguration.Variables
    // (and, in literal fragments, incidental "$$" bytes — see 3.1).
    Variables []NamedVariableFragment
}

type NamedVariableFragment struct {
    Name  string
    Value []byte
}
```

`FetchConfiguration` gets `MergeableOperation *MergeableOperation`.
`FetchConfiguration.Equals` does not compare it (consistent with its
DataSource exclusion): equal `Input` strings imply *semantically equivalent*
documents (selection order may differ under `SortAST` minification), which is
sufficient because the merged operation is re-printed from the surviving
member's document and responses are demuxed by alias.

Datasource changes:

- The variables-blob write sites (3.1) go through a helper that both performs
  `sjson.SetRawBytes` (propagating the error where today's code checks it)
  and records `(name, raw)` in the ordered slice with
  **replace-in-existing-slot** semantics on duplicate names. The recorded
  value on a duplicate is the last-written clean fragment even where the
  v1.0.4 blob overwrite corrupts token-bearing values in the blob itself
  (pre-existing quirk; the blob write path stays byte-identical with the
  flag off). The `createInputForQuery` opVars merge — which writes to a
  local copy — records through the same mechanism (safe:
  `createInputForQuery` runs once per planner). A duplicate write to the
  `representations` slot marks the planner's fetch as non-mergeable (a client
  variable literally named `representations` collides with the synthetic key
  — pre-existing planner defect; `MergeableOperation` is not stored).
- `ConfigureFetch` stores `p.upstreamOperation` (post-normalization,
  post-validation) into `MergeableOperation.Document` and then sets
  `p.upstreamOperation = nil`, making later mutation structurally impossible
  (a hypothetical reuse allocates a fresh document in `EnterDocument`).
- No envelope is stored (see 3.1: header/URL `{{ }}` conversion happens after
  ConfigureFetch). Envelope bytes are derived at merge time from the
  surviving member's post-visitor `Input`.

### 4.4 New postprocess stage: `createMultiFetch`

Runs on the flat tree (root Sequence, all children Single/SingleFetch).

**Candidate selection.** A child is a candidate iff its fetch is a
`*resolve.SingleFetch` with (`RequiresEntityFetch || RequiresEntityBatchFetch`),
`MergeableOperation != nil` (the planner already refuses to store it on a
`representations` name collision — see 4.3), `Info != nil` (with
`plan.DisableIncludeInfo` the feature silently disables — grouping and
per-entry auth need FetchInfo), and a well-formed variables record: exactly
one recorded fragment containing a `$$N$$` token whose variable is the
`ResolvableObjectVariable` (structural defense-in-depth on top of the
record-time guard).

**Grouping.** Candidates are partitioned by
`(FetchInfo.DataSourceID, FetchDependencies.DeferID, wave)`. Waves are
computed per **DeferID partition** of the flat child list (mirroring the real
per-group organize for defer plans): apply `orderSequenceByDependencies`'
comparator to the partition, then `createParallelNodes`' greedy grouping over
fetch-ID/dependency sets. The sort/grouping logic is extracted into shared
helpers reused by the organize stages. Two candidates merge only if the
simulation puts them in the same parallel group. Groups of size 1 are left
untouched. Same simulated group implies no mutual (transitive) dependency —
that is the safety requirement; for sync/subscription plans it also equals
the executed wave (zero added latency).

**Merged document construction.** For a group `[s1..sn]` (wave order, aliases
`f1..fn`, 1-based):

1. Create the merged document with `ast.NewSmallDocument()` (a zero-value
   `ast.Document` panics in the Add* helpers); add a query
   `OperationDefinition`, named
   `<propagatedName>__multi_<id1>_<id2>...` when all subs share a non-empty
   `OperationName`, otherwise anonymous. (`fetchIDAppender` already renamed
   sub `Input` strings; the merged operation prints fresh from documents.)
2. Per sub `k`, build the rename map `name → name_fk` over the **union** of
   the sub document's variable-definition names and the recorded
   `NamedVariableFragment` names (the recorded names are the source of truth
   for `body.variables` keys and can include stale keys absent from the
   document — see 3.1).
3. Import all of sub k's variable definitions with the variable **name**
   renamed (new astimport capability; the existing `WithRename` renames the
   type, not the name).
4. Add a `$includeF<k>: Boolean!` variable definition. The synthetic name is
   deliberately **outside the rename image**: every renamed name ends in
   `_f<digits>` (lowercase, underscore), which `includeF<k>` never does, so it
   cannot collide with any renamed client variable (including pathological
   client names like `include` or `representations_f1`). Rename-vs-rename is
   collision-free because within a sub renaming is injective and across subs
   `a+"_fi" == b+"_fj"` has no solution for `i != j` (digits cannot contain
   `_f`).
5. Import the sub operation's single root field (`_entities` with its full
   selection subtree, arguments, directives, inline fragments) using the new
   recursive cross-document import with the rename map applied to every
   `ValueKindVariable` occurrence. Set `Alias: fk` on the imported root field
   and append `@include(if: $includeF<k>)` to its directive list.
6. Print with `astprinter.PrintString` (no re-normalization or re-validation:
   subs are already normalized/validated, aliases make root fields disjoint,
   renaming is total per step 2/4). Minification is skipped in v1 (documented
   future work). A pretty print (`PrintStringIndent`) feeds the merged
   `QueryPlan.Query` when subs carry query plans.

**Merged input assembly.** The stage outputs the concrete
`resolve.MultiEntityFetch` directly. The input skeleton derives from the
surviving member's post-visitor `Input` string via a small quote-aware scanner
(handles `$$N$$` tokens transparently — tokens contain no braces or quotes):

- Locate the `body.variables` **object value** byte range and the
  `body.query` **string value** byte range in `s1.Input`, supporting **both**
  key orders (3.1): the repo shape (envelope first, `"body":{"query":"...",
  "variables":{...}}` at the end — variables object found by a backward
  brace-balanced scan from the input tail, query anchored by
  `"body":{"query":"`) and the append shape (`{"body":{"variables":{...},
  "query":"..."},...}` — variables found by a forward balanced scan, query
  end located by the last `"}` followed by `,`). The raw, unescaped query
  string is never scanned through — both shapes bound it by its neighbors.
  A candidate whose input fails these anchors or a round-trip check is left
  unmerged.
- Replace the query value with the printed merged operation (embedded raw,
  matching today's unescaped embedding) and the variables object with the
  authored per-entry material below.
- Precondition: all group members' envelope bytes (input minus the two
  replaced ranges) must be equal (`bytes.Equal`); otherwise the group is not
  merged (defensive — same DataSourceID implies same config in practice).
- Header template = `$$`-split of the prefix (up to and including
  `"variables":` plus `{`) against `s1.Variables`; Footer template =
  `$$`-split of `}` + everything after the variables object (includes the
  replaced query and the header/url section, whose `$$K$$` HeaderVariable
  tokens resolve against `s1.Variables`). Header/Footer contain no context
  variables (header variables never collect undefined), so no undefined-
  variable bookkeeping is needed there.

Authored per-entry material, per sub k (templates built with the shared
`$$`-split helper against the **sub's own** Variables slice):

- `Representations InputTemplate` — the sub's single ResolvableObject
  segment; `SetTemplateOutputToNullOnVariableNull: true` on this template
  only.
- `RepresentationsPrefix []byte` — `"representations_fk":[`, with a leading
  `,` for k ≥ 2 (the inter-entry separator lives here; no dangling commas
  regardless of omitted pairs).
- `IncludePrefix []byte` — `],"includeFk":`.
- `Variables []MultiEntityFetchVariable` — for every other recorded fragment:
  `{KeyPrefix: []byte(",\"<name>_fk\":"), Value: InputTemplate}`. The value
  template is the `$$`-split of the raw fragment. These templates do **not**
  set `SetTemplateOutputToNullOnVariableNull` — a deliberate, documented
  divergence from `createConcreteSingleFetchTypes` (which sets it on
  Header/Items/Footer so a null object variable nulls the whole input): in
  the merged design a null value renders as a literal `null` in place, and
  entry-level exclusion replaces whole-input nulling.

Where `SingleEntityPostProcessingConfiguration` applied
(`RequiresEntityFetch`), the entry records `OriginKind: single`; batch origins
record `OriginKind: batch` (drives the empty-response edge, 4.7).

**The fetch node.** The group's first member node is replaced by a Single node
whose `FetchItem` is `{Fetch: multi, FetchPath: nil, ResponsePath: ""}`; the
other members are removed. Bookkeeping mirrors `deduplicateSingleFetches`
(including its two known-harmless quirks, 3.2):

- `FetchDependencies`: `FetchID` = lowest member ID; `DependsOnFetchIDs` =
  union of member dependencies minus member IDs (duplicates tolerated);
  `DeferID` = shared value.
- `replaceDependsOnFetchId(root, memberID, multi.FetchID)` for **every**
  member ID ≠ the surviving FetchID — including the first member's original
  ID when it is not the group minimum (wave order sorts by dependency count,
  so the first member need not carry the lowest ID; a dependent referencing
  its vanished ID would otherwise break ordering and the skip cascade).
- `DataSource` and `DataSourceIdentifier` are taken from s1 (matching the
  envelope choice).
- Merged `FetchInfo`: same `DataSourceID`/`DataSourceName`;
  `OperationType: ast.OperationTypeQuery`; `RootFields` = deduplicated union;
  `CoordinateDependencies`, `FetchReasons`, `PropagatedFetchReasons` =
  concatenations; `QueryPlan` = `{Query: prettyMergedQuery, DependsOnFields:
  union}` when subs have query plans. Per-entry `Info` keeps each sub's
  original FetchInfo (whose `QueryPlan.Query` was already `__<fetchID>`-
  suffixed by `fetchIDAppender`; per-entry query plans are **not** rendered in
  query-plan output — only the merged one, see 4.8).
- Entry `Item` = a copy of the sub's original `FetchItem` with `Item.Fetch`
  set to **nil**: a backpointer to the multi would make the plan cyclic
  (breaking structural plan comparison in tests), and retaining the replaced
  member fetch would keep its `MergeableOperation` document alive inside the
  cached plan. The only `fetchItem.Fetch` consumer on the per-entry merge
  path (`isEmptyEntityFetch`) is skipped for multi entries (see 4.7).

**Document clearing.** Final step (and only step when disabled): set
`MergeableOperation = nil` on every `SingleFetch` in the tree.

### 4.5 New fetch type: `resolve.MultiEntityFetch`

```go
type MultiEntityFetch struct {
    FetchDependencies

    Input                MultiEntityInput
    DataSource           DataSource
    DataSourceIdentifier []byte
    Trace                *DataSourceLoadTrace
    Info                 *FetchInfo   // merged info (transport-level identity)
}

func (*MultiEntityFetch) FetchKind() FetchKind { return FetchKindMultiEntity }
// Dependencies/FetchInfo as usual; compile-time assertion added.

type MultiEntityInput struct {
    Header  InputTemplate
    Entries []MultiEntityFetchEntry
    Footer  InputTemplate
}

// MultiEntityFetchEntry is one original entity fetch inside the merged
// request: it carries the "raw fetch" identity (placement, info,
// post-processing) plus the template material to render its slice of
// body.variables. It has no rendered input of its own.
type MultiEntityFetchEntry struct {
    Alias string                    // "f1"
    Item  *FetchItem                 // original FetchPath/ResponsePath; Fetch stays nil (no cycles)
    Info  *FetchInfo                 // original fetch's info (auth, taint checks)
    PostProcessing PostProcessingConfiguration // SelectResponseDataPath: ["data", Alias];
                                               // SelectResponseErrorsPath: ["errors"];
                                               // MergePath: original
    OriginKind EntityFetchOriginKind // single | batch (empty-response edge)

    RepresentationsPrefix []byte     // `"representations_f1":[` (leading ',' for k>=2)
    Representations InputTemplate    // single ResolvableObject segment; null-on-var-null
    IncludePrefix         []byte     // `],"includeF1":`
    Variables []MultiEntityFetchVariable

    SkipNullItems, SkipEmptyObjectItems, SkipErrItems bool // uniform batch semantics
}

type MultiEntityFetchVariable struct {
    KeyPrefix []byte        // `,"first_f1":`
    Value     InputTemplate // token segment(s) and/or static literal bytes
}
```

Kind constant `FetchKindMultiEntity` is appended to the enum. Type switches
gaining a case / hardening:

- `postprocess.resolveInputTemplates.traverseNode` — replace the unchecked
  cast with a checked type switch (skip non-SingleFetch).
- `postprocess.createConcreteSingleFetchTypes.traverseFetch` — already skips.
- `fetchtree.go` `Trace()` and `queryPlan()` — see 4.8.
- `loader.preparePhase` — new case.
- `isEmptyEntityFetch`, `getTaintedIndices`, `rewriteErrorPaths` — generalized
  (4.7).

### 4.6 Loader: `prepareMultiEntityFetch`

Runs inside `preparePhase` under the data lock. Tools lifecycle: **one**
pooled `batchEntityTools` object stored on the shared `prepared.res` (so the
existing `resolveSingle` defer returns it); per-entry scratch buffers are
allocated on that tools arena and must survive until final assembly — between
entries only `keyGen` and `batchHashToIndex` are cleared (per-entry dedup
scope), **never** `a.Reset()`; each entry's `batchStats` is copied to the heap
before the next entry starts (mirroring loader.go:1768-1773).

Steps:

1. Per entry k: `items_k := l.selectItemsForPath(entry.Item.FetchPath)`
   (type conditions and tainted-object filtering per entry).
2. Per entry k: authorization first (input-free in all reachable paths, see
   Q2): run the `isFetchAuthorized` logic with `entry.Info`. Denied →
   per-entry result gets today's outcome (`fetchSkipped` from the cache path;
   `authorizationRejected(+Reasons)` kept for parity if the authorizer path
   ever becomes reachable) and the entry is *excluded*.
3. Per entry k (not excluded): render representations exactly like
   `prepareBatchEntityFetch` — per item, render `entry.Representations` with
   the entry's skip flags, xxhash dedup into `batchStats_k`, `,` between
   unique items — into the entry's scratch buffer. Zero unique items → entry
   is *empty* → excluded.
4. Entry state: `include_k = !excluded`. Excluded entries emit
   `"representations_fk":[]` and `"includeFk":false`; a denied entry's
   rendered representations are discarded, never sent. Excluded entries'
   **non-representations variables are still rendered** — variable coercion
   of non-null variables happens before `@include` evaluation on the subgraph
   (GraphQL `CoerceVariableValues` precedes execution), so every declared
   variable must receive a value.
5. Assemble once (no optimistic re-assembly — authorization precedes
   assembly): render Header (`RenderAndCollectUndefinedVariables`); per entry:
   `RepresentationsPrefix` + items (or nothing) + `IncludePrefix` + boolean +
   per variable `KeyPrefix` + rendered value. Undefined-variable stripping is
   inline: a variable pair is **omitted** iff its value template's rendered
   output is exactly `null` **and** an undefined context variable was
   collected during that render (an explicit client `null` renders null but
   is defined → pair kept). This reproduces `compactAndUnNullVariables`
   semantics for renamed keys. Render Footer. No `SetInputUndefinedVariables`
   call (already stripped; Header/Footer collect nothing).
6. All entries excluded → `fetchSkipped`, `skipLoad`; tracing input recorded
   when enabled (mirrors batch behavior).
7. Rate limiting: one `rateLimitFetch(mergedInput, multi.Info, res)` call.
   Rejection blocks the whole request; the rate-limit error is rendered per
   entry path in the merge phase. Note: a budget-style limiter could have
   allowed some of the N unmerged requests — the merged single check is a
   documented, accepted divergence (5.1).
8. `prepared.source/input/trace` set from the multi fetch. `preparedFetch`
   gains `multiEntries []preparedMultiEntry` where

   ```go
   type preparedMultiEntry struct {
       items []*astjson.Value // heap-copied targets for this entry
       res   *result          // per-entry view: init(entry.PostProcessing, entry.Info)
   }
   ```

   The multi merge branch does **not** reuse `nestedMergeItems` (its
   `items[j:j+1]` pairing is wrong for per-entry item sets). Per-entry
   results share `statusCode`/`ds`/`out`/`httpResponseContext` copied from
   the parent after load; `tools` stays only on the parent result.

Tracing: when enabled, `fetch.Trace = &DataSourceLoadTrace{}`; `RawInputData`
is the per-entry items data keyed by alias (`{"f1":[...],"f2":[...]}`).

`loadPhase` unchanged: one `executeSourceLoad` — one `OnLoad`, one trace, one
`body.extensions`/fetch-reasons injection (merged union).

### 4.7 Loader: merging the multi result

`mergePhase` gets a multi branch (`mergeMultiEntityResult`), under the data
lock:

1. Transport-level failures once per entry for client-visible parity:
   `res.err` / empty body / unparseable body / status fallback →
   `renderErrorsFailedToFetch`-family with each entry's `FetchItem` (N errors
   at N paths, as N separate fetches would produce) — EXCEPT entries excluded
   at prepare time, which get no transport/rate-limit error (their unmerged
   counterparts were never sent). Dependent-fetch skipping
   applies **only** to the `res.err` case (`loadPhase` recorded the surviving
   ID); invalid-response shapes do not cascade — same as unmerged.
2. Parse the response body **once**; collect response `extensions` into
   `l.subgraphExtensions` **once** (hoisted out of the per-entry path — N
   `mergeResult` calls over one response must not duplicate them).
3. Partition `response.errors` by first path element (`"f1"` → entry 1).
   Errors without an alias-shaped first element are attributed once to the
   multi fetch (merged `FetchItem`, empty response path).
4. Per entry k: run the (refactored) `mergeResult` with the entry's
   `FetchItem`, per-entry result view, and `items_k`. The refactor:
   - accepts a pre-parsed response value and pre-selected errors (nil = parse
     and select as today);
   - parameterizes the entity root name for `rewriteErrorPaths` /
     `getTaintedIndices` (`"_entities"` for existing kinds, the alias for
     multi entries) and the taint-info source (`entry.Info`);
   - **error-path alias hiding**: for multi entries, the alias prefix in
     subgraph error paths is rewritten even when `rewriteSubgraphErrorPaths`
     is off — at minimum `alias → "_entities"` — so pass-through mode never
     exposes internal aliases (invariant 2);
   - **empty-array edge**: before the batch fan-out, an entry whose selected
     data is an empty array and whose `OriginKind` is `single` is treated as
     a benign empty entity fetch (return nil — preserves today's EntityFetch
     behavior where `["data","_entities","0"]` selects null); batch-origin
     entries keep the `invalidBatchItemCount` error on count mismatch.
     `isEmptyEntityFetch` itself needs **no** multi case: an entry's
     `SelectResponseDataPath` has no trailing index, so the empty-array case
     arrives as non-null `[]` (handled by the pre-fan-out edge), and a null
     `data.<alias>` follows the existing null-data fallthrough, matching
     unmerged behavior.
   All existing machinery is thereby reused per entry: skip/auth guards,
   `setSkipErrors`, batch fan-out, taint marking, wrap/pass-through error
   modes, status fallbacks.
5. Per-entry subgraph errors recorded via `recordSubgraphError` are joined
   into the shared result's `subgraphError`; `callOnFinished` fires once with
   the shared result.

### 4.8 ART tracing and query plans

`fetchtree.go` (JSON tags follow each struct's existing family — snake_case
for trace, camelCase for query plan; all new fields `omitempty`):

- `Trace()` gains `case *MultiEntityFetch`: `FetchTraceNode{Kind:
  "MultiEntity", SourceID/SourceName from Info, Trace: f.Trace, Path: ""}`
  plus `Entries []FetchTraceEntry` (`entries`, each `{alias, path}`) listing
  every merged sub-fetch — its own entries in ART, one load trace (one
  request).
- `queryPlan()` gains `case *MultiEntityFetch`: `FetchTreeQueryPlan{Kind:
  "MultiEntity", FetchID, DependsOnFetchIDs, SubgraphName/ID, Path: "",
  Dependencies: Info.CoordinateDependencies, Query: Info.QueryPlan.Query,
  Representations: Info.QueryPlan.DependsOnFields}` plus `MergedFetchIDs
  []int` (`mergedFetchIds`) and per-entry `{alias, path}` pairs (`entries`).
  Per-entry QueryPlans are not rendered. `PlanPrinter.printFetchInfo` renders
  it like a Single fetch (no `Flatten` wrapper; representations block +
  query) — "like a single fetch, just with more variables".

### 4.9 astimport extensions

New capabilities on `astimport.Importer` (existing methods untouched —
`ImportField` hardcodes `SelectionSet: -1` and drops directives, so recursive
methods are additions, not fixes):

- `ImportVariableDefinitionWithVariableNameRename(ref int, from, to
  *ast.Document, newName string) int` — renames the variable **name** (the
  rename hooks into the `ValueKindVariable` path of a rename-aware
  `ImportValue`).
- `ImportSelectionSetWithVariableRename(ref int, from, to *ast.Document,
  rename map[string]string) int` (nil map = plain copy; a thin
  `ImportSelectionSet` wrapper) — recursive cross-document copy of selection
  sets: fields (name, alias, arguments, directives, nested sets), inline
  fragments (type condition, directives, nested sets). Fragment spreads
  return an error (upstream operations never contain them, 3.1). Variable
  renaming applies to every `ValueKindVariable` inside argument and directive
  values.
- Directive import during the recursive copy uses `ImportDirective` extended
  to the rename-aware value path.

The merge stage composes these with existing `ast` helpers
(`AddOperationDefinitionToRootNodes`,
`AddImportedVariableDefinitionToOperationDefinition`,
`AddVariableDefinitionToOperationDefinition`, `AddNamedType`/`AddNonNullType`
for `Boolean!`, `field.Alias = ast.Alias{...}` as in
`required_fields_visitor.go:568`, `AddDirective` + appending to the field's
`Directives.Refs` with `HasDirectives: true`).

## 5. Behavior details and edge cases

| Case | Behavior |
|---|---|
| Group of 1 candidate | Left untouched. |
| Different `DeferID` | Never merged; waves computed per DeferID partition. |
| Subscription response tree | Merged like sync; trigger untouched. |
| Mutation root fetches | Never candidates (entity fetches are query-typed). |
| gRPC datasource | Never candidates (no `MergeableOperation`). |
| `Info == nil` (DisableIncludeInfo) | Never candidates; feature silently off. |
| Client variable named `representations` | Sub excluded from candidacy (duplicate-name / representations-fragment guard; pre-existing planner defect not merged bug-for-bug). |
| Envelope bytes differ within a group | Group not merged (defensive). |
| Entry with all representations null/empty/skipped | `includeFN:false`, `representations_fN:[]`, no merge, no error — same as today's skipped fetch. |
| All entries excluded | No HTTP request (`fetchSkipped`). |
| Auth-denied entry (cache path) | Representations not sent; `include:false`; **no loader-rendered error** (field errors come from response resolution, as today); siblings unaffected. |
| Rate limit rejected | Whole request not sent; rate-limit error rendered at each entry path. |
| Transport error (`res.err`) | Failed-to-fetch error per entry path; dependents of the merged fetch ID skip. |
| Invalid/empty/unparseable response | Failed-to-fetch error per entry path; dependents still run (as unmerged). |
| Subgraph error with path `["f1", 0, "x"]` | Attributed to entry 1; alias prefix rewritten (full rewrite when `rewriteSubgraphErrorPaths` on, `alias→"_entities"` otherwise). |
| Subgraph error without alias path | Attributed once to the merged fetch. |
| `data.f1` null with errors | Entry-level null-data semantics (as today). |
| `data.f1 == []`, single-origin entry | Benign no-op (parity with `["data","_entities","0"]` selecting null). |
| `data.f1 == []`, batch-origin entry with ≥1 representation | `invalidBatchItemCount` (as today). |
| Duplicate representations across entries | Not deduplicated across entries in v1; dedup stays per entry. |
| Same client variable in two entries | Sent under both renamed keys; only request bytes duplicate. |
| Undefined client variable in an entry | Pair omitted iff rendered `null` AND undefined was collected (parity with `compactAndUnNullVariables`). |
| Interface objects / multiple keys | Unaffected: per-entry representations variables/renderers. |
| Minified merged operation | Not minified in v1. |
| `NormalizedQuery` on tree nodes | Unset for multi nodes. |

### 5.1 Documented divergences from unmerged execution

1. **Rate limiting**: one pre-fetch check instead of N. A budget-style limiter
   may allow/deny differently than N independent checks. Inherent to merging;
   accepted.
2. **Variable-coercion blast radius**: if an entry's `body.variables` value
   fails upstream coercion (e.g. an undefined *required* variable), the whole
   merged request fails, nulling sibling entries too. Unreachable for valid
   client operations — an undefined client variable implies a nullable client
   variable definition, and upstream context-variable definitions mirror
   client nullability — but noted for completeness.
3. **Denied-entry variable transmission**: a denied entry's
   non-representations variables (which may include parent-derived object
   variables) are still transmitted (never executed) because non-null
   variable coercion requires values. Representations — the entity keys — are
   never sent. Routers for which this residual transmission matters can keep
   the feature off; a candidacy filter for auth-ruled fetches is future work.
4. **Per-entry null object variables**: today a null object variable anywhere
   in an entity fetch's template nulls the whole input (whole fetch skipped);
   merged entries render `null` in place for non-representations variables
   (representations keep null-propagation → entry exclusion). Strictly more
   graceful; noted.

Client-visible invariants (test assertions):

1. Response data identical to unmerged execution (modulo 5.1).
2. Error objects identical modulo the single shared HTTP request; alias never
   leaks in any error-propagation mode.
3. Authorization outcomes identical.
4. Exactly one subgraph request per merged group; `OnLoad`/`OnFinished` once
   per request.
5. Flag off: zero behavior change, zero allocation change.

## 6. Testing strategy

- **astimport**: selection-set import (aliases, args, directives, inline
  fragments, nesting), variable-name rename in definitions and all value
  positions, fragment-spread error; golden printed output.
- **postprocess/createMultiFetch**: grouping by datasource/defer/wave
  (incl. per-DeferID wave partition), dependency-ID rewrite, merged-document
  goldens (include directives, renamed variables, synthetic names, stale
  blob keys), entry template layout (prefix commas, statics + segments),
  envelope-equality bail-out, `representations`-named-variable bail-out,
  document clearing with the stage enabled AND disabled, no-op for single
  candidates / non-entity fetches / nil Info; full `Process` test with the
  option on asserting the final tree; a subscription-plan test; interaction
  with `DisableResolveInputTemplates`.
- **loader prepare**: input-assembly goldens (multiple entries, per-entry
  dedup, empty entry, denied entry via pre-fetch-authorizer cache, undefined
  variable stripping incl. the explicit-null-kept case, all-excluded skip);
  tools lifecycle (no arena reset between entries).
- **loader merge**: per-alias fan-out, error partitioning + alias rewrite in
  both propagation modes, transport-error fan-out + dependent skip, invalid-
  response non-cascade, empty-array single-origin vs batch-origin, taint
  indices per entry, extensions collected once, hooks once, rate-limit
  rejection errors per path.
- **fetchtree**: golden `Trace()` and `QueryPlan()` JSON for a multi node
  (tags, omitempty).
- **graphql_datasource plan tests**: the issue's example producing one
  `MultiEntityFetch` with the exact merged query (asserted via
  `Info.QueryPlan.Query` with query plans enabled); a three-fetch group;
  different waves → no merge; flag off → byte-identical plans (invariant 5).
- **resolve integration**: full `LoadGraphQLResponseData` with a stub
  datasource asserting a single Load call, single-flight compatibility, and
  a final response identical to the unmerged run.

## 7. Future work (out of scope for v1)

- Root-field (non-entity) fetch merging.
- Cross-entry representation dedup and same-entity-type selection merging.
- Sharing identical client variables across entries.
- Minifying the merged operation.
- Candidacy filter for auth-ruled fetches (divergence 5.1.3).
- Cost/latency heuristics (e.g. cap on entries per merged request).
