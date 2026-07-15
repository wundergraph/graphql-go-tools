# MultiFetch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Merge same-subgraph, same-wave entity fetches into one aliased `_entities` request, per `docs/multi-fetch/spec.md`.

**Architecture:** The graphql datasource stores the normalized upstream document + ordered variables fragments on the fetch config (flag-gated). A new postprocess stage groups mergeable entity fetches by (datasource, defer, wave), merges their documents via new astimport primitives into one aliased+`@include`-guarded operation, and emits a concrete `resolve.MultiEntityFetch`. The loader renders per-entry representations/variables into one request and demuxes the response per alias through the existing `mergeResult` machinery.

**Tech Stack:** Go, existing repo tooling only (`ast`, `astimport`, `astprinter`, `astparser`, `sjson`, `astjson`, `gotestsum`).

## Global Constraints

- The spec is the source of truth: `docs/multi-fetch/spec.md`. Read it before starting any task. Section references below (e.g. "spec 4.6") point there.
- Default off. `plan.Configuration.EnableMultiFetch=false` and no `postprocess.EnableMultiFetch()` option ⇒ byte-identical plans and zero behavior change.
- Never "fix" pre-existing quirks in passing: unescaped query embedding, blind `$$` alternation, `replaceDependsOnFetchId` duplicate-ID tolerance are replicated, not repaired.
- Naming: feature = MultiFetch (flags, stage); resolve types = `MultiEntity*`; synthetic include variables = `includeF<k>` (never `include_f<k>` — collision rules, spec 4.4 step 4); aliases = `f1..fn`.
- Run tests with `gotestsum --format=short -- ./path/... -run TestName` (never bare `go test`).
- Go files use tabs; run `gofmt -w` on every touched file after editing.
- Comments: small, meaningful, ≤2-3 sentences, no implementation-plan leakage, no "why this change is correct" narration.
- No git add/commit steps inside tasks — the orchestrator handles commits between tasks.
- All work happens in this worktree: `/Users/neyasut/projects/wundergraph/graphql-go-tools/.claude/worktrees/multi-fetch`, Go module root `v2/`.

---

### Task 1: astimport — rename-aware imports and recursive selection-set import

**Files:**
- Modify: `v2/pkg/astimport/astimport.go`
- Test: `v2/pkg/astimport/astimport_test.go` (extend; the file exists and uses a table-driven `runTestCase` style — follow it or add standalone tests in the same style as the existing ones)

**Interfaces:**
- Consumes: existing `Importer` methods (`ImportType`, `ImportArguments`, `ImportValue`), `ast.Document` mutators.
- Produces (used by Task 6):
  - `func (i *Importer) ImportVariableDefinitionWithVariableNameRename(ref int, from, to *ast.Document, newName string) int`
  - `func (i *Importer) ImportSelectionSetWithVariableRename(ref int, from, to *ast.Document, rename map[string]string) (int, error)`
  - `func (i *Importer) ImportSelectionSet(ref int, from, to *ast.Document) (int, error)` — thin wrapper, nil map.

Behavior contract (spec 4.9):
- `ImportVariableDefinitionWithVariableNameRename` mirrors `ImportVariableDefinition` but writes `newName` as the variable value name (`to.ImportVariableValue([]byte(newName))`); the TYPE is imported unchanged via `ImportType` (do not confuse with the existing `ImportVariableDefinitionWithRename`, which renames the type).
- `ImportSelectionSetWithVariableRename` deep-copies a selection set across documents:
  - fields: name, alias (verbatim), arguments (values through the rename-aware value path), directives, nested selection sets;
  - inline fragments: type condition (via `ImportType`), directives, nested selection sets;
  - fragment spreads: return `fmt.Errorf("astimport: fragment spreads are not supported")` — upstream operations never contain them (spec 3.1).
- Rename-aware value path: a private `importValueWithRename(fromValue ast.Value, from, to *ast.Document, rename map[string]string) ast.Value` that duplicates `ImportValue`'s switch, except `ast.ValueKindVariable` looks the source name up in `rename` (miss ⇒ keep original name) and recurses through list/object values with the map. `ImportValue` becomes `importValueWithRename(..., nil)`.
- Rename-aware argument/directive imports: private `importArgumentsWithRename`, `importDirectivesWithRename` used by the recursive copy (public `ImportArgument(s)`/`ImportDirective` delegate with nil map).

Implementation skeleton for the recursive copy (follow `ast.Document.CopySelectionSet`/`CopySelection`/`CopyField` in `v2/pkg/ast/ast_selection.go:32-117` for the traversal shape, but write into `to`):

```go
func (i *Importer) ImportSelectionSetWithVariableRename(ref int, from, to *ast.Document, rename map[string]string) (int, error) {
	refs := make([]int, 0, len(from.SelectionSets[ref].SelectionRefs))
	for _, selectionRef := range from.SelectionSets[ref].SelectionRefs {
		imported, err := i.importSelection(selectionRef, from, to, rename)
		if err != nil {
			return -1, err
		}
		refs = append(refs, imported)
	}
	return to.AddSelectionSetToDocument(ast.SelectionSet{SelectionRefs: refs}), nil
}
```

with `importSelection` switching on `from.Selections[selectionRef].Kind`, building `ast.Field{...}`/`ast.InlineFragment{...}` values, appending via `to.AddSelectionToDocument` (check exact helper names in `ast_selection.go`; `AddSelectionToDocument` exists at `ast_selection.go:134`), and setting `HasSelections`/`SelectionSet: -1` correctly for leaf fields.

- [ ] **Step 1: Write failing tests.** In `astimport_test.go` add `TestImportSelectionSetWithVariableRename` parsing a `from` operation with `astparser.ParseGraphqlDocumentString`:

```graphql
query($representations: [_Any!]!, $first: Int) {
  _entities(representations: $representations) {
    ... on Employee { __typename products(first: $first) @custom(arg: $first) { upc nested { id } } }
  }
}
```

  Import the root field's enclosing selection set of `_entities` (locate the operation's selection set ref: `from.OperationDefinitions[0].SelectionSet`) into a fresh `ast.NewSmallDocument()` that has one query operation definition added; rename map `{"representations": "representations_f1", "first": "first_f1"}`. Attach the imported set to the operation and print with `astprinter.PrintString`; assert the golden string contains `_entities(representations: $representations_f1)`, `products(first: $first_f1)`, `@custom(arg: $first_f1)`, `... on Employee`, `nested {id}`. Add `TestImportSelectionSetFragmentSpreadError` (a doc with a spread → error). Add `TestImportVariableDefinitionWithVariableNameRename`: import the `$representations` definition with new name `representations_f1`, attach via `AddImportedVariableDefinitionToOperationDefinition`, print, assert `query($representations_f1: [_Any!]!)`.
- [ ] **Step 2: Run to verify failure.** `gotestsum --format=short -- ./pkg/astimport/... -run 'TestImportSelectionSet|TestImportVariableDefinitionWithVariableNameRename'` — expect compile errors (methods undefined).
- [ ] **Step 3: Implement** the methods per the contract above.
- [ ] **Step 4: Run tests to green**, then run the whole package: `gotestsum --format=short -- ./pkg/astimport/...`.
- [ ] **Step 5: gofmt** `gofmt -w v2/pkg/astimport/astimport.go v2/pkg/astimport/astimport_test.go`.

---

### Task 2: postprocess plumbing — stage reorder, checked casts, shared helpers

**Files:**
- Modify: `v2/pkg/engine/postprocess/postprocess.go` (reorder `processFlatFetchTree`)
- Modify: `v2/pkg/engine/postprocess/resolve_input_templates.go` (checked type switch + promote helper)
- Modify: `v2/pkg/engine/postprocess/deduplicate_single_fetches.go` (promote `replaceDependsOnFetchId` to a package-level function)
- Test: existing package tests must stay green; add reorder-safety test in `v2/pkg/engine/postprocess/postprocess_test.go`

**Interfaces:**
- Produces (used by Tasks 5-7):
  - `func resolveInputTemplate(variables resolve.Variables, input string, template *resolve.InputTemplate)` — package-level, extracted verbatim from the method (`resolve_input_templates.go:57-89`); the method delegates to it.
  - `func replaceDependsOnFetchID(root *resolve.FetchTreeNode, oldID, newID int)` — package-level, body moved verbatim from `deduplicateSingleFetches.replaceDependsOnFetchId` (`deduplicate_single_fetches.go:40-66`); the method callsite updated.

Changes:
1. In `processFlatFetchTree` (`postprocess.go:42-51`) move `p.addMissingNestedDependencies.ProcessFetchTree(fetches)` to directly after `p.appendFetchID.ProcessFetchTree(fetches)` (i.e. before `resolveInputTemplates`). Safe per spec 4.1: it reads only ResponsePath/MergePath/FetchDependencies.
2. In `resolveInputTemplates.traverseNode` (`resolve_input_templates.go:36-37`) replace the unchecked cast:

```go
	case resolve.FetchTreeNodeKindSingle:
		if fetch, ok := node.Item.Fetch.(*resolve.SingleFetch); ok {
			r.traverseSingleFetch(fetch)
		}
```

- [ ] **Step 1: Write the reorder-safety test** in `postprocess_test.go`: construct a `plan.SynchronousResponsePlan` whose `RawFetches` contain a root fetch (`FetchID: 0`, `ResponsePath: ""`, `PostProcessing.MergePath: nil`) and a nested fetch (`FetchID: 1`, empty `DependsOnFetchIDs`, `ResponsePath: "user"`, root provides path `user` via `MergePath: []string{"user"}`), run `NewProcessor().Process`, and assert the nested fetch ends up with `DependsOnFetchIDs: []int{0}` and the tree is `Sequence(Single(0), Single(1))`. (This locks the moved stage still fires; mirror existing test style in that file.)
- [ ] **Step 2: Run the new test and the full package**: `gotestsum --format=short -- ./pkg/engine/postprocess/...` — expect the new test to pass only after the changes; existing tests green before and after (the reorder must not change any golden).
- [ ] **Step 3: Apply the three modifications.** Extract the two package-level functions with bodies moved verbatim; keep the methods as one-line delegates so existing callers are untouched.
- [ ] **Step 4: Full package run**: `gotestsum --format=short -- ./pkg/engine/postprocess/...` — all green.
- [ ] **Step 5: gofmt** touched files.

---

### Task 3: resolve types — MergeableOperation + MultiEntityFetch family + tree rendering

**Files:**
- Create: `v2/pkg/engine/resolve/fetch_multi.go`
- Modify: `v2/pkg/engine/resolve/fetch.go` (kind constant, `FetchConfiguration.MergeableOperation` field, compile-time assertion)
- Modify: `v2/pkg/engine/resolve/fetchtree.go` (`Trace()`, `queryPlan()`, new fields)
- Test: `v2/pkg/engine/resolve/fetchtree_test.go` (extend; create if absent)

**Interfaces:**
- Produces (used by Tasks 4-9): everything below, verbatim.

`fetch.go`: append `FetchKindMultiEntity` to the `FetchKind` enum; add to `FetchConfiguration`:

```go
	// MergeableOperation carries planner artifacts consumed by the postprocess
	// MultiFetch stage; it is cleared during postprocessing and never reaches
	// the executable plan. Nil unless plan.Configuration.EnableMultiFetch is set.
	MergeableOperation *MergeableOperation
```

`FetchConfiguration.Equals` is NOT changed (spec 4.3). Add `_ Fetch = (*MultiEntityFetch)(nil)` to the assertion block.

`fetch_multi.go` — full content (plus package/imports):

```go
// MergeableOperation is the planner hand-off for MultiFetch merging.
type MergeableOperation struct {
	// Document is the normalized and validated upstream operation. Ownership
	// transfers to the plan; the planner nils its own reference after storing.
	Document *ast.Document
	// Variables lists the top-level body.variables entries in blob order.
	// Values are raw fragments that may contain $$N$$ placeholders referring
	// to FetchConfiguration.Variables.
	Variables []NamedVariableFragment
}

type NamedVariableFragment struct {
	Name  string
	Value []byte
}

type EntityFetchOriginKind int

const (
	EntityFetchOriginSingle EntityFetchOriginKind = iota + 1
	EntityFetchOriginBatch
)

// MultiEntityFetch merges several same-subgraph entity fetches into one
// request with aliased _entities fields guarded by @include variables.
type MultiEntityFetch struct {
	FetchDependencies

	Input                MultiEntityInput
	DataSource           DataSource
	DataSourceIdentifier []byte
	Trace                *DataSourceLoadTrace
	Info                 *FetchInfo
}

func (m *MultiEntityFetch) Dependencies() *FetchDependencies { return &m.FetchDependencies }
func (m *MultiEntityFetch) FetchInfo() *FetchInfo            { return m.Info }
func (*MultiEntityFetch) FetchKind() FetchKind               { return FetchKindMultiEntity }

type MultiEntityInput struct {
	Header  InputTemplate
	Entries []MultiEntityFetchEntry
	Footer  InputTemplate
}

// MultiEntityFetchEntry is one original entity fetch inside the merged
// request: raw-fetch identity plus the template material for its slice of
// body.variables. It has no rendered input of its own.
type MultiEntityFetchEntry struct {
	Alias          string
	Item           *FetchItem // original FetchPath/ResponsePath; Fetch is the parent MultiEntityFetch
	Info           *FetchInfo
	PostProcessing PostProcessingConfiguration
	OriginKind     EntityFetchOriginKind

	RepresentationsPrefix []byte // `"representations_f1":[` with a leading ',' for entries after the first
	Representations       InputTemplate
	IncludePrefix         []byte // `],"includeF1":`
	Variables             []MultiEntityFetchVariable

	SkipNullItems        bool
	SkipEmptyObjectItems bool
	SkipErrItems         bool
}

type MultiEntityFetchVariable struct {
	KeyPrefix []byte // `,"first_f1":`
	Value     InputTemplate
}
```

`fetchtree.go`:
- Add to `FetchTraceNode`: `Entries []FetchTraceEntry \`json:"entries,omitempty"\`` and

```go
type FetchTraceEntry struct {
	Alias string `json:"alias"`
	Path  string `json:"path"`
}
```

- In `Trace()`'s inner switch add:

```go
		case *MultiEntityFetch:
			entries := make([]FetchTraceEntry, len(f.Input.Entries))
			for i, e := range f.Input.Entries {
				entries[i] = FetchTraceEntry{Alias: e.Alias, Path: e.Item.ResponsePath}
			}
			trace.Fetch = &FetchTraceNode{
				Kind:       "MultiEntity",
				SourceID:   f.Info.DataSourceID,
				SourceName: f.Info.DataSourceName,
				Trace:      f.Trace,
				Path:       n.Item.ResponsePath,
				Entries:    entries,
			}
```

- Add to `FetchTreeQueryPlan`: `MergedFetchIDs []int \`json:"mergedFetchIds,omitempty"\`` and `Entries []QueryPlanEntry \`json:"entries,omitempty"\`` with

```go
type QueryPlanEntry struct {
	Alias string `json:"alias"`
	Path  string `json:"path,omitempty"`
}
```

- In `queryPlan()`'s inner switch add a `*MultiEntityFetch` case mirroring the `*BatchEntityFetch` case (`Kind: "MultiEntity"`), plus `MergedFetchIDs` (populated by Task 7 from the original fetch IDs stored on entries — carry them as `MergedFetchIDs []int` on `MultiEntityFetch`; add that field: `MergedFetchIDs []int` next to `Info`) and `Entries` built from `Input.Entries` aliases/paths. Per-entry QueryPlans are NOT rendered (spec 4.8).

- [ ] **Step 1: Write failing tests** `TestFetchTreeNode_Trace_MultiEntity` and `TestFetchTreeNode_QueryPlan_MultiEntity` in `fetchtree_test.go`: build a `Single`-kind node whose `Item.Fetch` is a `*MultiEntityFetch` with two entries (`f1`/`employees.products`, `f2`/`employee`), `Info: &FetchInfo{DataSourceID: "products-id", DataSourceName: "products", QueryPlan: &QueryPlan{Query: "query {...}"}}`, `MergedFetchIDs: []int{1, 2}`, `FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}`. Marshal `node.Trace()` and `node.QueryPlan()` to JSON and assert golden strings (exact tags: `"kind":"MultiEntity"`, `"entries":[{"alias":"f1","path":"employees.products"}...]`, `"mergedFetchIds":[1,2]`).
- [ ] **Step 2: Run to verify failure** (compile errors): `gotestsum --format=short -- ./pkg/engine/resolve/... -run 'TestFetchTreeNode_Trace_MultiEntity|TestFetchTreeNode_QueryPlan_MultiEntity'`.
- [ ] **Step 3: Implement** all type/field additions above.
- [ ] **Step 4: Green** the two tests, then full package: `gotestsum --format=short -- ./pkg/engine/resolve/...`.
- [ ] **Step 5: gofmt** touched files.

---

### Task 4: plan flag threading + graphql_datasource artifact recording

**Files:**
- Modify: `v2/pkg/engine/plan/configuration.go` (add `EnableMultiFetch bool` with doc comment)
- Modify: `v2/pkg/engine/plan/planner_configuration.go:21-23` (add `EnableMultiFetch bool` to `plannerConfigurationOptions`)
- Modify: `v2/pkg/engine/plan/datasource_configuration.go:325-327` (thread `configuration.EnableMultiFetch`)
- Modify: `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource.go`
- Test: `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_test.go` (extend)

**Interfaces:**
- Consumes: `resolve.MergeableOperation`/`NamedVariableFragment` (Task 3).
- Produces: `FetchConfiguration.MergeableOperation` populated for entity fetches when the flag is on.

Datasource changes (spec 4.3):
1. Planner fields: `upstreamVariablesList []resolve.NamedVariableFragment` (nil unless recording) plus `recordUpstreamVariables bool`, reset in `EnterDocument`. `recordUpstreamVariables = p.dataSourcePlannerConfig.Options.EnableMultiFetch && p.config.grpc == nil` (set in `Register` or `EnterDocument`).
2. Recording helper on the planner:

```go
// setUpstreamVariable writes a top-level body.variables entry and, when
// MultiFetch recording is on, mirrors it into upstreamVariablesList with
// replace-in-slot semantics so the slice reproduces the blob's key order.
func (p *Planner[T]) setUpstreamVariable(target []byte, name string, raw []byte) []byte {
	out, _ := sjson.SetRawBytes(target, name, raw)
	if p.recordUpstreamVariables {
		value := make([]byte, len(raw))
		copy(value, raw)
		for i := range p.upstreamVariablesList {
			if p.upstreamVariablesList[i].Name == name {
				p.upstreamVariablesList[i].Value = value
				return out
			}
		}
		p.upstreamVariablesList = append(p.upstreamVariablesList, resolve.NamedVariableFragment{Name: name, Value: value})
	}
	return out
}
```

3. Route all six write sites through it (spec 3.1): `addRepresentationsVariable` (:849), `configureFieldArgumentSource` (:1157 — note it writes with `sjson.SetRawBytes(p.upstreamVariables, variableNameStr, []byte(contextVariableName))`), `addVariableDefinitionsRecursively` (:1241), `configureObjectFieldSource` (:1286), `addDirectiveToNode` (:247), and the opVars merge loop in `createInputForQuery` (:302-315 — it writes to the local `upstreamVariables`; pass that local as `target`). Keep the quote-wrapping logic for string values exactly as-is, applied before calling the helper.
4. In `ConfigureFetch`, after `createInputForQuery` and before returning, when `p.recordUpstreamVariables && (requiresEntityFetch || requiresEntityBatchFetch)`:

```go
	var mergeableOperation *resolve.MergeableOperation
	if p.recordUpstreamVariables && (requiresEntityFetch || requiresEntityBatchFetch) {
		mergeableOperation = &resolve.MergeableOperation{
			Document:  p.upstreamOperation,
			Variables: p.upstreamVariablesList,
		}
		p.upstreamOperation = nil
	}
```

   and set `MergeableOperation: mergeableOperation` in the returned `resolve.FetchConfiguration`. (Nil-ing `p.upstreamOperation` transfers ownership; `EnterDocument` allocates fresh on any reuse.)

- [ ] **Step 1: Write failing test** `TestConfigureFetch_MergeableOperation` (place near other planner unit tests; if the harness makes direct ConfigureFetch testing awkward, use the plan-level path: run `plan.NewPlanner` over a small federation config from an existing federation test with `Configuration.EnableMultiFetch: true`, take the produced `SynchronousResponsePlan` BEFORE postprocessing, and inspect `RawFetches`): assert the entity fetch's `FetchConfiguration.MergeableOperation != nil`, its `Variables` names equal `["representations"]` (plus any context variables in the chosen query, in blob order), the representations value is `[$$0$$]`-shaped (regex `^\[\$\$\d+\$\$\]$`), and `astprinter.PrintString(Document)` prints an `_entities` query. Also assert: flag off ⇒ `MergeableOperation == nil`; root (non-entity) fetch ⇒ nil; and the fetch `Input` string is byte-identical with the flag on vs off.
- [ ] **Step 2: Run to verify failure**: `gotestsum --format=short -- ./pkg/engine/datasource/graphql_datasource/... -run TestConfigureFetch_MergeableOperation`.
- [ ] **Step 3: Implement** items 1-4 plus the plan-package threading (three small edits mirroring `EnableOperationNamePropagation`).
- [ ] **Step 4: Green** the test; then the datasource package: `gotestsum --format=short -- ./pkg/engine/datasource/graphql_datasource/...` (slow; run once) and `gotestsum --format=short -- ./pkg/engine/plan/...`.
- [ ] **Step 5: gofmt** touched files.

---

### Task 5: postprocess createMultiFetch — stage skeleton, candidates, waves, grouping, clearing

**Files:**
- Create: `v2/pkg/engine/postprocess/create_multi_fetch.go`
- Modify: `v2/pkg/engine/postprocess/postprocess.go` (option + slot + invocation)
- Test: `v2/pkg/engine/postprocess/create_multi_fetch_test.go`

**Interfaces:**
- Consumes: `replaceDependsOnFetchID`, `resolveInputTemplate` (Task 2), resolve types (Task 3).
- Produces (Tasks 6-7 fill in): `createMultiFetch` struct with `ProcessFetchTree`, private `collectGroups(root *resolve.FetchTreeNode) [][]*resolve.FetchTreeNode`, `clearMergeableOperations(root *resolve.FetchTreeNode)`.

Wiring (spec 4.2): `processorOptions.enableMultiFetch bool`; option

```go
func EnableMultiFetch() ProcessorOption {
	return func(o *processorOptions) {
		o.enableMultiFetch = true
	}
}
```

`DisableResolveInputTemplates()` additionally sets `o.enableMultiFetch = false`... it runs before/after other options in caller order, so instead enforce at `NewProcessor`: `enableMultiFetch = opts.enableMultiFetch && !opts.disableResolveInputTemplates`. Stage field `createMultiFetch *createMultiFetch` in `FetchTreeProcessors`, constructed with `disable: !enableMultiFetch`; invoked in `processFlatFetchTree` between `addMissingNestedDependencies` and `resolveInputTemplates`.

Stage skeleton:

```go
// createMultiFetch merges same-subgraph entity fetches that would execute in
// the same parallel wave into a single MultiEntityFetch with aliased
// _entities fields. It always clears MergeableOperation artifacts, even when
// disabled, so no AST survives postprocessing.
type createMultiFetch struct {
	disable bool
}

func (c *createMultiFetch) ProcessFetchTree(root *resolve.FetchTreeNode) {
	if !c.disable {
		for _, group := range c.collectGroups(root) {
			c.mergeGroup(root, group) // Task 6/7
		}
	}
	c.clearMergeableOperations(root)
}
```

Candidate predicate (spec 4.4): fetch is `*resolve.SingleFetch`; `RequiresEntityFetch || RequiresEntityBatchFetch`; `MergeableOperation != nil`; `Info != nil`; variables record well-formed: no duplicate `Name`s, and exactly one fragment whose `Value` contains a `$$N$$` token whose `Variables[N]` is a `*resolve.ResolvableObjectVariable` (helper `representationsFragmentIndex(fetch) int` returning -1 when malformed).

Wave computation (spec 4.4) — reuse the real stages on a scratch tree per DeferID partition, zero drift:

```go
func (c *createMultiFetch) wavesByFetchID(root *resolve.FetchTreeNode) map[int]int {
	waves := map[int]int{}
	wave := 0
	for _, partition := range c.partitionByDeferID(root.ChildNodes) {
		scratch := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSequence, ChildNodes: append([]*resolve.FetchTreeNode(nil), partition...)}
		(&orderSequenceByDependencies{}).ProcessFetchTree(scratch)
		(&createParallelNodes{}).ProcessFetchTree(scratch)
		for _, child := range scratch.ChildNodes {
			switch child.Kind {
			case resolve.FetchTreeNodeKindParallel:
				for _, member := range child.ChildNodes {
					waves[member.Item.Fetch.Dependencies().FetchID] = wave
				}
			default:
				waves[child.Item.Fetch.Dependencies().FetchID] = wave
			}
			wave++
		}
	}
	return waves
}
```

(The stages only mutate the scratch root's ChildNodes slice, never the nodes; partitions preserve original slice order.) Groups = candidates bucketed by `(Info.DataSourceID, DeferID, wave)`, keeping only buckets with ≥2 members, members in wave order (the order they appear in the scratch parallel group).

`clearMergeableOperations` walks `root.ChildNodes`, and for every `*resolve.SingleFetch` sets `fetch.MergeableOperation = nil`.

- [ ] **Step 1: Write failing tests** in `create_multi_fetch_test.go` for grouping/clearing only (stub `mergeGroup` not yet doing anything — make Step 1 tests target `collectGroups` and `clearMergeableOperations` directly, package-internal). Test table (build flat Sequence trees out of `resolve.Single(&resolve.SingleFetch{...})` nodes like `create_parallel_nodes` tests do; give candidates `FetchConfiguration{RequiresEntityFetch: true, MergeableOperation: &resolve.MergeableOperation{...minimal valid: Variables: []resolve.NamedVariableFragment{{Name: "representations", Value: []byte("[$$0$$]")}}...}, Variables: resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{}))}` and `Info: &resolve.FetchInfo{DataSourceID: "ds1"}`):
  1. two same-ds entity fetches, same deps `{0}` ⇒ one group of 2;
  2. different DataSourceID ⇒ no group;
  3. one depends on the other ⇒ no group (different waves);
  4. different DeferID ⇒ no group;
  5. nil Info / nil MergeableOperation / non-entity ⇒ not candidates;
  6. duplicate fragment names ⇒ not a candidate;
  7. `clearMergeableOperations` nils artifacts on every fetch with the stage disabled (`ProcessFetchTree` with `disable: true`).
- [ ] **Step 2: Run to verify failure**: `gotestsum --format=short -- ./pkg/engine/postprocess/... -run TestCreateMultiFetch`.
- [ ] **Step 3: Implement** skeleton + wiring (with `mergeGroup` as a no-op placeholder that only Task 6/7 fills — acceptable here because Tasks 5-7 land as one reviewed sequence and the stage is dark until `EnableMultiFetch()` is used; the no-op must be replaced within this plan).
- [ ] **Step 4: Green** new tests + full package.
- [ ] **Step 5: gofmt.**

---

### Task 6: createMultiFetch — merged document construction

**Files:**
- Create: `v2/pkg/engine/postprocess/create_multi_fetch_document.go`
- Test: extend `v2/pkg/engine/postprocess/create_multi_fetch_test.go`

**Interfaces:**
- Consumes: Task 1 astimport methods.
- Produces (used by Task 7):

```go
// buildMergedOperation merges the group members' stored documents into one
// aliased, @include-guarded operation and returns the compact and pretty
// printed forms.
func buildMergedOperation(members []*resolve.SingleFetch) (compact string, pretty string, err error)
```

Algorithm (spec 4.4 steps 1-6), per member k (1-based):
1. `merged := ast.NewSmallDocument()`; add operation definition: `ast.OperationDefinition{OperationType: ast.OperationTypeQuery, HasSelections: true, SelectionSet: <fresh set>}` via `merged.AddOperationDefinitionToRootNodes`. Name it `<OperationName>__multi_<id1>_<id2>...` iff every member's `FetchConfiguration.OperationName` is equal and non-empty (name bytes via `merged.Input.AppendInputString`; set `Name` on the op def); otherwise anonymous.
2. Rename map for member k: for every variable definition name in `member.MergeableOperation.Document` (operation's `VariableDefinitions`, name via `doc.VariableDefinitionNameString`) AND every `NamedVariableFragment.Name`: `rename[name] = name + "_f" + strconv.Itoa(k)`.
3. Import each variable definition with `ImportVariableDefinitionWithVariableNameRename(ref, doc, merged, rename[origName])`, attach with `merged.AddImportedVariableDefinitionToOperationDefinition(opRef, importedRef)`.
4. Add the include variable: named type `Boolean` (`merged.AddNamedType([]byte("Boolean"))`), `merged.AddNonNullType(...)`, variable value `merged.ImportVariableValue([]byte("includeF"+strconv.Itoa(k)))`, `merged.AddVariableDefinitionToOperationDefinition(opRef, variableValueRef, nonNullBooleanRef)`.
5. Locate the member's root `_entities` field: the sub operation's selection set has exactly one selection, a field named `_entities` (assert; bail with error otherwise). Import it manually (do not use `ImportField` — it drops selections): build `ast.Field` with name `_entities` appended to merged input, `Alias: ast.Alias{IsDefined: true, Name: merged.Input.AppendInputString("f"+strconv.Itoa(k))}` (pattern of `required_fields_visitor.go:568`), arguments via the rename-aware argument import, selection set via `ImportSelectionSetWithVariableRename(subFieldSelectionSetRef, doc, merged, rename)`, plus an `@include` directive:

```go
	includeArgValue := ast.Value{Kind: ast.ValueKindVariable, Ref: merged.AddVariableValue(ast.VariableValue{Name: merged.Input.AppendInputString("includeF" + strconv.Itoa(k))})}
	includeArg := merged.AddArgument(ast.Argument{Name: merged.Input.AppendInputString("if"), Value: includeArgValue})
	directiveRef := merged.AddDirective(ast.Directive{Name: merged.Input.AppendInputString("include"), HasArguments: true, Arguments: ast.ArgumentList{Refs: []int{includeArg}}})
```

   set `HasDirectives: true, Directives: ast.DirectiveList{Refs: [...existing imported directives..., directiveRef]}` on the field, `merged.AddField(...)`, and append the selection to the operation's selection set (`merged.AddSelection(opSelectionSetRef, ast.Selection{Kind: ast.SelectionKindField, Ref: fieldRef})`).
6. Print: `compact, err := astprinter.PrintString(merged)`; `pretty, err := astprinter.PrintStringIndent(merged, "  ")`.

- [ ] **Step 1: Write failing test** `TestBuildMergedOperation`: two members whose `MergeableOperation.Document`s are parsed with `astparser.ParseGraphqlDocumentString` from
  - `query($representations: [_Any!]!){_entities(representations: $representations){... on Employee {__typename products {upc}}}}`
  - `query($representations: [_Any!]!, $first: Int){_entities(representations: $representations){... on Employee {__typename notes(first: $first)}}}`

  and `NamedVariableFragment` records `[{representations,[$$0$$]}]` / `[{representations,[$$0$$]},{first,$$1$$}]`. Expected compact golden (single line, printer spacing — adjust to actual printer output on first run, then freeze):

  `query($representations_f1: [_Any!]!, $includeF1: Boolean!, $representations_f2: [_Any!]!, $first_f2: Int, $includeF2: Boolean!){f1: _entities(representations: $representations_f1) @include(if: $includeF1){... on Employee {__typename products {upc}}} f2: _entities(representations: $representations_f2) @include(if: $includeF2){... on Employee {__typename notes(first: $first_f2)}}}`

  Plus: named-operation case (both members `OperationName: "Q"`, IDs 3 and 5 ⇒ `query Q__multi_3_5(...)`); error case (member root selection is not a single `_entities` field ⇒ error).
- [ ] **Step 2: Run to verify failure.**
- [ ] **Step 3: Implement** `buildMergedOperation` (parameterize member fetch IDs — signature may take `ids []int`; keep the Produces signature updated here and in Task 7 if changed — final: `buildMergedOperation(members []*resolve.SingleFetch, ids []int) (compact, pretty string, err error)` where `ids[i]` is member i's FetchID).
- [ ] **Step 4: Green**; full package run.
- [ ] **Step 5: gofmt.**

---

### Task 7: createMultiFetch — input split, entry assembly, fetch construction, integration

**Files:**
- Create: `v2/pkg/engine/postprocess/create_multi_fetch_input.go`
- Modify: `v2/pkg/engine/postprocess/create_multi_fetch.go` (`mergeGroup` real implementation)
- Test: extend `v2/pkg/engine/postprocess/create_multi_fetch_test.go`

**Interfaces:**
- Consumes: Tasks 2, 3, 6.
- Produces: the complete stage; loader consumes the emitted `*resolve.MultiEntityFetch`.

`create_multi_fetch_input.go`:

```go
// fetchInputSplit locates the body.variables object and body.query string
// values inside a fetch input of the deterministic shape
// {"body":{"variables":{...},"query":"..."},...envelope}. ok is false when
// the input deviates; such fetches are left unmerged.
type fetchInputSplit struct {
	variablesStart, variablesEnd int // the {...} object value byte range
	queryStart, queryEnd         int // the string value content byte range (between the quotes)
}

func splitEntityFetchInput(input string) (s fetchInputSplit, ok bool)
```

Algorithm: require prefix `{"body":{"variables":` (else `ok=false`); brace-balanced, quote-and-escape-aware scan for the variables object end (raw `$$N$$` tokens contain neither braces nor quotes; literal fragments are valid JSON); require the following bytes to be `,"query":"`; for the query end, scan from the END of the input for the last occurrence of `"}` whose following byte is `,` or the final `}` — take that as queryEnd/body-close; round-trip check: `input[s.queryEnd:s.queryEnd+2] == "\"}"`. Suffix = `input[s.queryEnd+2:]` (envelope tail), prefix-envelope = `input[:len("{\"body\":{\"variables\":")]`.

`mergeGroup(root, group)` (spec 4.4 "Merged input assembly" + "The fetch node"):
1. Members `s1..sn` = the group's `*resolve.SingleFetch`s, `ids` their FetchIDs. Split every member's `Input`; any `!ok` ⇒ abort merge for the group (leave nodes untouched). Envelope precondition: for all members, `input[queryEnd+2:]` byte-equal to s1's and every `$$K$$` token inside that suffix must reference `.Equals()`-equal variables across members (split the suffix with the blind `$$` alternation; on numeric segments compare `si.Variables[K].Equals(s1.Variables[K])`); else abort.
2. `compact, pretty, err := buildMergedOperation(members, ids)`; error ⇒ abort.
3. Header template: `resolveInputTemplate(s1.Variables, s1.Input[:variablesStart] + "{", &header)` — the prefix through `"variables":` plus the object opener.
4. Footer template: `resolveInputTemplate(s1.Variables, "}" + s1.Input[variablesEnd:queryStart] + compact + s1.Input[queryEnd:], &footer)` — closes the variables object, re-emits `,"query":"`, the merged operation (raw, unescaped — parity), and the envelope tail.
5. Entries, per member k: origin from `RequiresEntityFetch` (single) / `RequiresEntityBatchFetch` (batch); `alias := "f"+strconv.Itoa(k)`. Representations fragment index via `representationsFragmentIndex`; build `Representations` by `resolveInputTemplate(sk.Variables, "<inner of the [...] fragment>", &tpl)` — the fragment is `[$$N$$]`; strip the surrounding brackets and resolve; set `tpl.SetTemplateOutputToNullOnVariableNull = true`. Prefixes:

```go
	prefix := `"representations_f` + kStr + `":[`
	if k > 1 {
		prefix = "," + prefix
	}
	includePrefix := `],"includeF` + kStr + `":`
```

   Other variables: every other `NamedVariableFragment` ⇒ `MultiEntityFetchVariable{KeyPrefix: []byte(`,"` + frag.Name + "_f" + kStr + `":`), Value: resolveInputTemplate(sk.Variables, string(frag.Value), ...)}` (no null-flag). Skip flags all true. `PostProcessing`: `SelectResponseDataPath: []string{"data", alias}`, `SelectResponseErrorsPath: []string{"errors"}`, `MergePath: sk.PostProcessing.MergePath`. `Item`: copy of the member node's `*resolve.FetchItem` with `Fetch` repointed to the multi (set after constructing it). `Info: sk.Info`.
6. The fetch:

```go
	multi := &resolve.MultiEntityFetch{
		FetchDependencies: resolve.FetchDependencies{
			FetchID:           minID(ids),
			DependsOnFetchIDs: unionDependencies(members, ids), // union minus member IDs; duplicates tolerated
			DeferID:           members[0].DeferID,
		},
		Input:                resolve.MultiEntityInput{Header: header, Entries: entries, Footer: footer},
		DataSource:           members[0].DataSource,
		DataSourceIdentifier: members[0].DataSourceIdentifier,
		MergedFetchIDs:       ids,
		Info:                 mergedInfo, // spec 4.4: union RootFields (dedup), concat CoordinateDependencies/FetchReasons/PropagatedFetchReasons, OperationType Query, QueryPlan{Query: pretty, DependsOnFields: union} iff all members have QueryPlan
	}
```

7. Tree surgery: replace the first member's node with `&resolve.FetchTreeNode{Kind: Single, Item: &resolve.FetchItem{Fetch: multi}}` (FetchPath nil, ResponsePath ""); delete the other member nodes from `root.ChildNodes`; for every removed id ≠ survivor: `replaceDependsOnFetchID(root, removedID, multi.FetchID)`.

- [ ] **Step 1: Write failing tests.**
  - `TestSplitEntityFetchInput` table: the canonical shape `{"body":{"variables":{"representations":[$$0$$],"a":$$1$$},"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename}}"},"header":{"Auth":["$$2$$"]},"url":"http://x","method":"POST"}` ⇒ correct ranges (assert extracted variables object and query text); deviant inputs (`{"method":...` first; no variables key) ⇒ `ok=false`.
  - `TestCreateMultiFetch_MergeGroup`: build a flat tree with fetch 0 (root SingleFetch, non-candidate) and fetches 1, 2 (entity candidates, `DependsOnFetchIDs: []int{0}`, same ds, well-formed Inputs as above, documents parsed from the Task 6 goldens' sources, distinct `MergePath`s `["a"]`/`["b"]`, distinct ResponsePaths `employees.@`/`employee`); fetch 3 (`DependsOnFetchIDs: []int{2}`). Run the full `Processor.Process` with `EnableMultiFetch()`. Assert: tree is `Sequence(Single(0), Single(multi), Single(3))` (before organize — assert post-organize final shape `Sequence(Single(0), Single(multi), Single(3))`); `multi.FetchID == 1`, `multi.MergedFetchIDs == [1,2]`; fetch 3 now `DependsOnFetchIDs: []int{1}`; entries have aliases f1/f2, prefixes exactly `"representations_f1":[` and `,"representations_f2":[`, include prefixes `],"includeF1":`/`],"includeF2":`, entry PostProcessing paths `["data","f1"]`/`["data","f2"]`, origins single/batch as configured; Header template's first static segment starts with `{"body":{"variables":{`; Footer's static contains `,"query":"query(` and the envelope tail; every fetch's `MergeableOperation == nil` afterwards.
  - Abort cases: envelope mismatch (different `url`) ⇒ nodes untouched; malformed input ⇒ untouched.
- [ ] **Step 2: Run to verify failure.**
- [ ] **Step 3: Implement** scanner + `mergeGroup`.
- [ ] **Step 4: Green**; full postprocess package.
- [ ] **Step 5: gofmt.**

---

### Task 8: loader — prepareMultiEntityFetch

**Files:**
- Create: `v2/pkg/engine/resolve/loader_multi_entity.go`
- Modify: `v2/pkg/engine/resolve/loader.go` (`preparePhase` case; `preparedFetch.multiEntries` field; `batchEntityTools.clearDedupState()` helper)
- Test: `v2/pkg/engine/resolve/loader_multi_entity_test.go`

**Interfaces:**
- Consumes: Task 3 types; existing loader internals (`selectItemsForPath`, `isFetchAuthorized`, `rateLimitFetch`, `batchEntityToolPool`, `setTracingInput`).
- Produces (Task 9 consumes):

```go
type preparedMultiEntry struct {
	entry *MultiEntityFetchEntry
	items []*astjson.Value // heap-copied merge targets
	res   *result          // per-entry view; init(entry.PostProcessing, entry.Info)
}
```

  `preparedFetch` gains `multiEntries []preparedMultiEntry`.

`prepareMultiEntityFetch(fetchItem *FetchItem, fetch *MultiEntityFetch, res *result, prepared *preparedFetch) error` — called from the new `preparePhase` case (note: the generic `items := l.selectItemsForPath(item.FetchPath)` at loader.go:332 runs first and harmlessly selects the root; ignore it):

```go
	case *MultiEntityFetch:
		err := l.prepareMultiEntityFetch(item, fetch, res, prepared)
		return prepared, err
```

Algorithm (spec 4.6, exact order):
1. `res.init(PostProcessingConfiguration{SelectResponseErrorsPath: []string{"errors"}}, fetch.Info)`; tracing: `fetch.Trace = &DataSourceLoadTrace{}` when enabled; `res.tools = batchEntityToolPool.Get(...)` ONCE on the shared result (per-entry results keep `tools == nil`; the existing `resolveSingle` defer returns it).
2. Per entry k: per-entry `result` with `init(entry.PostProcessing, entry.Info)`; `items := l.selectItemsForPath(entry.Item.FetchPath)`; authorization: `allowed, err := l.isFetchAuthorized(nil, entry.Info, entryRes)` (input-free — unreachable authorizer path for query-typed entries, spec Q2); denied ⇒ excluded.
3. Representations rendering for non-excluded entries into a per-entry arena buffer (`arena.NewArenaBuffer(res.tools.a)`): mirror `prepareBatchEntityFetch`'s item loop (loader.go:1696-1744) — render `entry.Representations` per item, skip flags, xxhash dedup via `res.tools`, `,` separators, arena `batchStats`. Between entries call `res.tools.clearDedupState()` (new method: `keyGen.Reset()` + clear `batchHashToIndex`; NEVER `a.Reset()`), and copy the entry's batchStats to heap (`entryRes.batchStats`) immediately (mirror loader.go:1768-1773). Zero unique items ⇒ excluded.
4. Assembly into a single `bytes.Buffer` (heap; the merged input escapes prepare):
   - `fetch.Input.Header.RenderAndCollectUndefinedVariables(l.ctx, nil, buf, &undefined)` (undefined stays empty — no context vars, spec 4.4);
   - per entry: `buf.Write(entry.RepresentationsPrefix)`; included ⇒ the entry's rendered representations bytes; `buf.Write(entry.IncludePrefix)`; `true`/`false`;
   - per entry variable: render `v.Value` into a scratch arena buffer with `RenderAndCollectUndefinedVariables(l.ctx, nil, scratch, &entryUndefined)`; omit the pair iff `bytes.Equal(scratch.Bytes(), null) && len(entryUndefined) > entryUndefinedBefore` (undefined collected during THIS render — spec 4.6 step 5); else `buf.Write(v.KeyPrefix); buf.Write(scratch.Bytes())`;
   - `fetch.Input.Footer.RenderAndCollectUndefinedVariables(...)`. No `SetInputUndefinedVariables` call.
5. All entries excluded ⇒ `res.fetchSkipped = true; prepared.skipLoad = true`; tracing input recorded via `l.setTracingInput(fetchItem, buf.Bytes(), fetch.Trace)` when enabled (mirror batch, loader.go:1775-1779). Per-entry excluded state: `entryRes.fetchSkipped = true`.
6. Rate limit once: `allowed, err := l.rateLimitFetch(buf.Bytes(), fetch.Info, res)`; `!allowed` ⇒ `prepared.skipLoad = true` (flags land on the shared res; Task 9 fans out).
7. `prepared.source, prepared.input, prepared.trace = fetch.DataSource, buf.Bytes(), fetch.Trace`; `prepared.multiEntries = entries`.
8. Tracing `RawInputData` when enabled: `{"f1":<itemsData(entry1.items)>,...}` built with `MarshalTo` per alias.

- [ ] **Step 1: Write failing tests** (loader tests in this package build a `Loader` via existing helpers — mirror `TestLoader_*` setup in `loader_test.go`, using a stub `DataSource` and a seeded `dataBuffer`; keep entries' `FetchPath`s pointing at seeded objects):
  - `TestPrepareMultiEntityFetch_Assembly`: two entries over seeded data (`{"employees":[{"__typename":"Employee","id":1},{"__typename":"Employee","id":2},{"__typename":"Employee","id":1}], "employee":{"__typename":"Employee","id":9}}` — note employees[2] duplicates employees[0] for dedup); entry1 batch over `employees` (ObjectVariable renderer on `id`), entry2 single over `employee`; a context variable `$first_f2` present (`ctx.Variables = {"first": 10}`). Assert the assembled input golden: header, `"representations_f1":[{...1},{...2}]` (deduped, 2 uniques; batchStats `[[e0,e2],[e1]]`), `],"includeF1":true`, `,"representations_f2":[{...9}]`, `],"includeF2":true`, `,"first_f2":10`, footer.
  - `TestPrepareMultiEntityFetch_EmptyEntry`: entry2's parent object null ⇒ `"representations_f2":[],"includeF2":false`, entry2 res.fetchSkipped.
  - `TestPrepareMultiEntityFetch_AllExcluded`: both empty ⇒ `prepared.skipLoad`, `res.fetchSkipped`.
  - `TestPrepareMultiEntityFetch_UndefinedVariable`: `$first` absent from ctx.Variables ⇒ `,"first_f2":10` pair omitted entirely; explicit `{"first":null}` ⇒ `,"first_f2":null` kept.
  - `TestPrepareMultiEntityFetch_DedupStateIsolation`: identical representation bytes in entry1 and entry2 ⇒ NOT cross-deduped (each entry's array contains it).
- [ ] **Step 2: Run to verify failure**: `gotestsum --format=short -- ./pkg/engine/resolve/... -run TestPrepareMultiEntityFetch`.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Green** + full resolve package.
- [ ] **Step 5: gofmt.**

---

### Task 9: loader — mergeResult refactor + mergeMultiEntityResult

**Files:**
- Modify: `v2/pkg/engine/resolve/loader.go` (`result` fields, `mergeResult` injection points, `mergePhase` multi branch, `isEmptyEntityFetch`, `rewriteErrorPaths`, `mergeErrors` signature pass-through)
- Modify: `v2/pkg/engine/resolve/tainted_objects.go` (`getTaintedIndices` parameterization)
- Create/extend: `v2/pkg/engine/resolve/loader_multi_entity.go` (merge half) and `loader_multi_entity_test.go`

**Interfaces:**
- Consumes: Task 8's `preparedFetch.multiEntries`.
- Produces: complete runtime path.

`result` gains a multi-entry view config:

```go
	// multi is set on per-entry result views during MultiEntityFetch merging.
	multi *multiEntryMergeConfig

type multiEntryMergeConfig struct {
	alias        string
	originSingle bool
	info         *FetchInfo      // taint-info source
	response     *astjson.Value  // pre-parsed shared response
	errors       *astjson.Value  // pre-partitioned errors for this entry (nil = none)
}
```

`mergeResult` injection points (each a small, behavior-preserving branch):
- parse (loader.go:634): `if res.multi != nil && res.multi.response != nil { response = res.multi.response } else { parse as today }`;
- extensions collection (643-649): skip when `res.multi != nil` (parent collects once);
- errors (662-686): when `res.multi != nil`, use `res.multi.errors` directly instead of `response.Get(SelectResponseErrorsPath...)`;
- taint (670): `getTaintedIndices(taintInfoFor(res, fetchItem), entityRootNameFor(res), responseData, responseErrors)` — signature change to `getTaintedIndices(info *FetchInfo, rootName string, data, errors *astjson.Value)`; existing callsite passes `fetchItem.Fetch.FetchInfo(), "_entities"`; multi passes `res.multi.info, res.multi.alias`;
- `rewriteErrorPaths` (859): signature gains `rootName string`; existing caller passes `"_entities"`. For multi entries, alias hiding is unconditional (spec 4.7): in `mergeErrors`, when `res.multi != nil` and `l.rewriteSubgraphErrorPaths` is false, still rewrite ONLY the alias root element: replace a leading path element equal to the alias with `"_entities"` (a tiny helper `hideAliasInErrorPaths(a, alias, values)`); when the option is true, run the full `rewriteErrorPaths` with `rootName = alias`;
- empty-array single-origin edge (before the batch fan-out at 743): `if res.multi != nil && res.multi.originSingle { if batch := responseData.GetArray(); batch != nil && len(batch) == 0 { return nil } }`;
- `isEmptyEntityFetch` (794): add `if fetchItem.Fetch.FetchKind() == FetchKindMultiEntity { ... }` — for a multi entry (detected via the res.multi config passed alongside; simplest: fold the check into mergeResult's null-data branch: `if res.multi != nil { if v := response.Get("data", res.multi.alias); astjson.ValueIsNonNull(v) && v.Type() == astjson.TypeArray { return nil } }` before calling `isEmptyEntityFetch`).

`mergePhase` (loader.go:366-385) gains, before the existing paths:

```go
	if prepared.multiEntries != nil {
		return l.mergeMultiEntityResult(prepared)
	}
```

`mergeMultiEntityResult(prepared *preparedFetch)` (spec 4.7):
1. Shared res flags fan-out: if `res.err != nil || res.authorizationRejected || res.rateLimitRejected || len(res.out) == 0` (and not fetchSkipped-everything) — copy `err/statusCode/ds/rateLimitRejected(+reason)/out`-emptiness onto every entry res and run step 4's loop (each entry renders its own failed-to-fetch / rate-limit errors at its own path via `mergeResult`'s existing guards); `fetchSkipped` on the shared res (all excluded) ⇒ return nil.
2. Otherwise parse once: `response, err := astjson.ParseBytesWithArena(l.jsonArena, res.out)`; unparseable ⇒ per-entry status-fallback/failed-to-fetch fan-out exactly like today's single-fetch behavior but per entry (set per-entry `res.out = parent out` + statusCode and let `mergeResult` parse-fail per entry — simplest parity: give each entry res the parent's `out` and no `multi.response`, so each replays today's guards; only on successful parse do entries get `multi.response`). Collect extensions once (copy of loader.go:643-649 against the parsed response).
3. Partition `response.Get("errors")` array by first path element into per-alias buckets + an unmatched bucket. Unmatched non-empty ⇒ `l.mergeErrors(res, prepared.item, unmatchedValue)` once (parent fetchItem, ResponsePath "").
4. Per entry: populate `entryRes.multi = &multiEntryMergeConfig{alias, originSingle, entry.Info, response, entryErrors}`, copy `statusCode/ds/out/httpResponseContext` from parent, then `err := l.mergeResult(entry.Item, entryRes, prepared.multiEntries[i].items)`; join `entryRes.subgraphError` into `res.subgraphError`.
5. `l.callOnFinished(res)` once; return first error.

- [ ] **Step 1: Write failing tests** (`loader_multi_entity_test.go`), stub DataSource returning canned bodies:
  - `TestMergeMultiEntityResult_FanOut`: response `{"data":{"f1":[{"products":[{"upc":"1"}]},{"products":[{"upc":"2"}]}],"f2":[{"notes":"n"}]}}` with entry1 batchStats `[[e0,e2],[e1]]` ⇒ e0 and e2 get products upc 1, e1 upc 2, employee gets notes; final resolvable JSON golden.
  - `TestMergeMultiEntityResult_ErrorPartitioning`: errors `[{"message":"a","path":["f1",0,"products"]},{"message":"b","path":["f2"]},{"message":"c"}]` in wrap mode ⇒ error "a" attributed at entry1's response path (rewritten, alias hidden), "b" at entry2's, "c" once at the multi; pass-through mode with `rewriteSubgraphErrorPaths=false` ⇒ paths contain `_entities`, never `f1`.
  - `TestMergeMultiEntityResult_EmptyArraySingleOrigin`: `{"data":{"f2":[]}}` for a single-origin entry ⇒ no error, no merge; same shape for batch-origin with 1 representation ⇒ `invalidBatchItemCount` failed-to-fetch error.
  - `TestMergeMultiEntityResult_TransportError`: `res.err` set ⇒ one failed-to-fetch error PER entry path; `erroredFetchIDs` contains the multi's FetchID.
  - `TestMergeMultiEntityResult_ExtensionsOnce`: response with `extensions` and `allowCustomExtensionProperties` ⇒ exactly one entry in `l.subgraphExtensions`.
  - `TestMergeMultiEntityResult_HooksOnce`: LoaderHooks recording ⇒ one OnLoad, one OnFinished.
  - `TestMergeMultiEntityResult_ExcludedEntry`: entry2 excluded at prepare ⇒ its data absent, no error, entry1 merged normally.
- [ ] **Step 2: Run to verify failure**: `gotestsum --format=short -- ./pkg/engine/resolve/... -run TestMergeMultiEntityResult`.
- [ ] **Step 3: Implement** the refactor + branch. Run the FULL resolve package after the `mergeResult`/`getTaintedIndices`/`rewriteErrorPaths` signature changes — they have existing tests.
- [ ] **Step 4: Green** + full package.
- [ ] **Step 5: gofmt.**

---

### Task 10: end-to-end — datasource plan tests + resolve integration

**Files:**
- Test: `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_federation_test.go` (extend) or a new `graphql_datasource_multi_fetch_test.go`
- Test: `v2/pkg/engine/resolve/loader_multi_entity_test.go` (integration case)

**Interfaces:** none new — verification only.

- [ ] **Step 1: Plan-level test** `TestGraphQLDataSourceFederation_MultiFetch`: reuse an existing federation test schema where two fields resolve entities from one subgraph at the same wave (pattern: `employees { id products }` + `employee(id: 1) { id products }` from the issue; adapt to an existing test schema with a list + single entity extension — if none fits, add a minimal 2-subgraph config modeled on the existing `TestGraphQLDataSourceFederation` fixtures). Drive `plan.NewPlanner` with `EnableMultiFetch: true`, postprocess with `postprocess.NewProcessor(postprocess.EnableMultiFetch(), postprocess.CollectDataSourceInfo())`, then assert on `response.Fetches.QueryPlan().PrettyPrint()` golden: exactly one Fetch(service: products) node of kind MultiEntity containing both aliased `_entities` fields with `@include(if: $includeF1)` / `@include(if: $includeF2)` and renamed representations variables. Also: same query with `EnableMultiFetch: false` ⇒ two separate entity fetch nodes (golden), asserting flag-off parity.
- [ ] **Step 2: Wave-separation test**: a query where the second entity fetch depends on the first's output (different waves) ⇒ no merge (two fetch nodes remain).
- [ ] **Step 3: Subscription test**: subscription response tree with two same-wave entity fetches ⇒ merged (assert via QueryPlan of the subscription response fetches).
- [ ] **Step 4: Resolve integration** `TestLoadGraphQLResponseData_MultiEntity`: hand-built post-postprocess tree (root SingleFetch stub datasource returning the parent data, then a MultiEntityFetch whose stub records the received input and returns per-alias data). Assert exactly ONE Load call for the multi, the received input equals the assembly golden from Task 8, and the final `resolvable` data equals the equivalent unmerged run (construct the same tree with the two original Entity/Batch fetches and diff the JSON byte-for-byte).
- [ ] **Step 5: Run everything**: `gotestsum --format=short -- ./pkg/engine/... ./pkg/astimport/...` — all green.

---

### Task 11: lint, fmt, full sweep

- [ ] **Step 1:** `gofmt -l v2/pkg` — expect empty output; fix anything listed.
- [ ] **Step 2:** Run the repo linter: `cd v2 && make lint-fix` if `v2/Makefile` has the target, else `make lint-fix` at the repo root (inspect `Makefile` first; the user's requirement is "run a linter with a make lint-fix"). Fix all reported issues in touched files.
- [ ] **Step 3:** Full test sweep: `gotestsum --format=short -- ./pkg/... -count=1` from `v2/` (long; run once). All green, including pre-existing tests.
- [ ] **Step 4:** Re-read every new/modified comment against the Global Constraints comment rules; delete narration.
