# MultiFetch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Merge same-subgraph, same-wave entity fetches into one aliased `_entities` request, per `docs/multi-fetch/spec.md`.

**Architecture:** The graphql datasource stores the normalized upstream document + ordered variables fragments on the fetch config (flag-gated). A new postprocess stage groups mergeable entity fetches by (datasource, defer, wave), merges their documents via new astimport primitives into one aliased+`@include`-guarded operation, and emits a concrete `resolve.MultiEntityFetch`. The loader renders per-entry representations/variables into one request and demuxes the response per alias through the existing `mergeResult` machinery.

**Tech Stack:** Go, existing repo tooling only (`ast`, `astimport`, `astprinter`, `astparser`, `sjson`, `astjson`, `gotestsum`).

## Global Constraints

- The spec is the source of truth: `docs/multi-fetch/spec.md`. Read it before starting any task. Section references below (e.g. "spec 4.6") point there.
- Default off. `plan.Configuration.EnableMultiFetch=false` and no `postprocess.EnableMultiFetch()` option ⇒ byte-identical plans and zero behavior change. This includes error paths: never remove or weaken an existing error check while routing code through a helper.
- **Fetch input shape (critical, spec 3.1):** this repo's `go.work` pins sjson v1.0.4, which PREPENDS keys, so fetch inputs here look like `{"method":"POST","url":"...",["header":{...},]"body":{"query":"...","variables":{...}}}` — envelope first, query before variables (see the golden at `graphql_datasource_federation_test.go` and the BatchEntityFetch templates in `loader_test.go`). Downstream consumers resolve sjson v1.2.5+ (append), giving the mirrored `{"body":{"variables":{...},"query":"..."},...}`. Code that locates the two ranges must handle both shapes and bail otherwise.
- Never "fix" pre-existing quirks in passing: unescaped query embedding, blind `$$` alternation, `replaceDependsOnFetchId` duplicate-ID tolerance, v1.0.4 blob-overwrite corruption are replicated or tolerated, not repaired.
- Naming: feature = MultiFetch (flags, stage); resolve types = `MultiEntity*`; synthetic include variables = `includeF<k>` (never `include_f<k>` — collision rules, spec 4.4 step 4); aliases = `f1..fn`.
- Run tests with `gotestsum --format=short -- ./path/... -run TestName` (never bare `go test`), from `v2/`.
- Go files use tabs; run `gofmt -w` on every touched file after editing.
- Comments: small, meaningful, ≤2-3 sentences, no implementation-plan leakage, no "why this change is correct" narration.
- No git add/commit steps inside tasks — the orchestrator handles commits between tasks.
- All work happens in this worktree: `/Users/neyasut/projects/wundergraph/graphql-go-tools/.claude/worktrees/multi-fetch`, Go module root `v2/`.

---

### Task 1: astimport — rename-aware imports and recursive selection-set import

**Files:**
- Modify: `v2/pkg/astimport/astimport.go`
- Test: `v2/pkg/astimport/astimport_test.go` (extend; the file has no shared helper — existing tests use inline tables or per-test closures. Write the new tests standalone: parse `from` with `astparser.ParseGraphqlDocumentString`, import into `ast.NewSmallDocument()` plus one added query operation definition, print with `astprinter.PrintString`, assert on the printed golden. Do NOT hand-build expected `ast.Document` structs the way `TestImporter_ImportVariableDefinitions` does.)

**Interfaces:**
- Consumes: existing `Importer` methods (`ImportType`, `ImportArguments`, `ImportValue`), `ast.Document` mutators. Note `ast.Document.AddField` returns `ast.Node` — use `.Ref`.
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
- Rename-aware value path: a private `importValueWithRename(fromValue ast.Value, from, to *ast.Document, rename map[string]string) ast.Value` duplicating `ImportValue`'s switch, except `ast.ValueKindVariable` looks the source name up in `rename` (miss ⇒ keep original name) and list/object values recurse with the map. `ImportValue` delegates with a nil map.
- Rename-aware argument/directive imports: private `importArgumentsWithRename`, `importDirectivesWithRename` used by the recursive copy (public `ImportArgument(s)`/`ImportDirective` delegate with nil map). These stay private — Task 6 reaches them only through `ImportSelectionSetWithVariableRename`.

Implementation skeleton for the recursive copy (follow the traversal shape of `ast.Document.CopySelectionSet`/`CopySelection`/`CopyField` in `v2/pkg/ast/ast_selection.go`, but write into `to`):

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

with `importSelection` switching on `from.Selections[selectionRef].Kind`, building `ast.Field{...}`/`ast.InlineFragment{...}` values, appending via the document's selection helpers (`AddSelectionToDocument` in `ast_selection.go`), and setting `HasSelections`/`SelectionSet: -1` correctly for leaf fields.

- [ ] **Step 1: Write failing tests.** `TestImportSelectionSetWithVariableRename` parses this `from` operation:

```graphql
query($representations: [_Any!]!, $first: Int) {
  _entities(representations: $representations) {
    ... on Employee { __typename p: products(first: $first) @custom(arg: $first) { upc nested { id } } }
  }
}
```

  Import the operation's root selection set (`from.OperationDefinitions[0].SelectionSet`) into a fresh target with rename map `{"representations": "representations_f1", "first": "first_f1"}`; attach to the target operation; print. Assert the golden contains `_entities(representations: $representations_f1)`, `p: products(first: $first_f1)`, `@custom(arg: $first_f1)`, `... on Employee`, `nested {id}` (the alias `p:` covers the alias-verbatim path). Add `TestImportSelectionSetFragmentSpreadError` (a doc with a spread → error). Add `TestImportVariableDefinitionWithVariableNameRename`: import the `$representations` definition with new name `representations_f1`, attach via `AddImportedVariableDefinitionToOperationDefinition`, print, assert `query($representations_f1: [_Any!]!)`.
- [ ] **Step 2: Run to verify failure.** `gotestsum --format=short -- ./pkg/astimport/... -run 'TestImportSelectionSet|TestImportVariableDefinitionWithVariableNameRename'` — expect compile errors (methods undefined).
- [ ] **Step 3: Implement** per the contract above.
- [ ] **Step 4: Run to green**, then the whole package: `gotestsum --format=short -- ./pkg/astimport/...`.
- [ ] **Step 5: gofmt** touched files.

---

### Task 2: postprocess plumbing — stage reorder, checked casts, shared helpers

**Files:**
- Modify: `v2/pkg/engine/postprocess/postprocess.go` (reorder `processFlatFetchTree`)
- Modify: `v2/pkg/engine/postprocess/resolve_input_templates.go` (checked type switch + promote helper)
- Modify: `v2/pkg/engine/postprocess/deduplicate_single_fetches.go` (promote `replaceDependsOnFetchId`)
- Test: `v2/pkg/engine/postprocess/postprocess_test.go` (behavior-lock test)

**Interfaces:**
- Produces (used by Tasks 5-7):
  - `func resolveInputTemplate(variables resolve.Variables, input string, template *resolve.InputTemplate)` — package-level, body moved verbatim from the method (`resolve_input_templates.go:57-89`); the method delegates. It fills the out-param and returns nothing.
  - `func replaceDependsOnFetchID(root *resolve.FetchTreeNode, oldID, newID int)` — package-level, body moved verbatim from `deduplicateSingleFetches.replaceDependsOnFetchId` (`deduplicate_single_fetches.go:40-66`); the method callsite updated.

Changes:
1. In `processFlatFetchTree` (`postprocess.go:42-51`) move `p.addMissingNestedDependencies.ProcessFetchTree(fetches)` to directly after `p.appendFetchID.ProcessFetchTree(fetches)` (before `resolveInputTemplates`). Safe per spec 4.1.
2. In `resolveInputTemplates.traverseNode` (`resolve_input_templates.go:36-37`) replace the unchecked cast:

```go
	case resolve.FetchTreeNodeKindSingle:
		if fetch, ok := node.Item.Fetch.(*resolve.SingleFetch); ok {
			r.traverseSingleFetch(fetch)
		}
```

- [ ] **Step 1: Write the behavior-lock test** in `postprocess_test.go`: a `plan.SynchronousResponsePlan` whose `RawFetches` contain a root fetch (`FetchID: 0`, `ResponsePath: ""`, `PostProcessing.MergePath: []string{"user"}`) and a nested fetch (`FetchID: 1`, empty `DependsOnFetchIDs`, `ResponsePath: "user"`); run `NewProcessor().Process`; assert the nested fetch gained `DependsOnFetchIDs: []int{0}` and the tree is `Sequence(Single(0), Single(1))`.
- [ ] **Step 2: Run it BEFORE any change** — it must already PASS (this is a lock, not red-green: `addMissingNestedDependencies` already runs today, merely later in the pipeline). Also run the full package green: `gotestsum --format=short -- ./pkg/engine/postprocess/...`.
- [ ] **Step 3: Apply the three modifications.** Bodies move verbatim; methods become one-line delegates so existing callers are untouched.
- [ ] **Step 4: Run the lock test and full package again** — identical results, zero golden changes.
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

`fetch_multi.go` — full content (plus package clause and imports):

```go
// MergeableOperation is the planner hand-off for MultiFetch merging.
type MergeableOperation struct {
	// Document is the normalized and validated upstream operation. Ownership
	// transfers to the plan; the planner nils its own reference after storing.
	Document *ast.Document
	// Variables lists the top-level body.variables entries in write order
	// (value replaced in place on duplicate name). Values are raw fragments
	// that may contain $$N$$ placeholders referring to
	// FetchConfiguration.Variables.
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
	// MergedFetchIDs are the original fetch IDs merged into this fetch, in
	// wave order; surfaced in query-plan output.
	MergedFetchIDs []int
	Info           *FetchInfo
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

- In `queryPlan()`'s inner switch add a `*MultiEntityFetch` case mirroring the `*BatchEntityFetch` case with `Kind: "MultiEntity"`, `MergedFetchIDs: f.MergedFetchIDs`, and `Entries` built from `f.Input.Entries` (alias + `e.Item.ResponsePath`). Per-entry QueryPlans are NOT rendered (spec 4.8).

- [ ] **Step 1: Write failing tests** `TestFetchTreeNode_Trace_MultiEntity` and `TestFetchTreeNode_QueryPlan_MultiEntity`: a `Single`-kind node whose `Item.Fetch` is a `*MultiEntityFetch` with two entries (`f1`/`employees.products`, `f2`/`employee`), `Info: &FetchInfo{DataSourceID: "products-id", DataSourceName: "products", QueryPlan: &QueryPlan{Query: "query {...}"}}`, `MergedFetchIDs: []int{1, 2}`, `FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}}`. Marshal `node.Trace()` / `node.QueryPlan()` to JSON; assert goldens contain `"kind":"MultiEntity"`, `"entries":[{"alias":"f1","path":"employees.products"},{"alias":"f2","path":"employee"}]`, `"mergedFetchIds":[1,2]`.
- [ ] **Step 2: Run to verify failure** (compile errors): `gotestsum --format=short -- ./pkg/engine/resolve/... -run 'TestFetchTreeNode_Trace_MultiEntity|TestFetchTreeNode_QueryPlan_MultiEntity'`.
- [ ] **Step 3: Implement** all additions above.
- [ ] **Step 4: Green**, then full package: `gotestsum --format=short -- ./pkg/engine/resolve/...`.
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
1. Planner fields, reset in `EnterDocument`: `upstreamVariablesList []resolve.NamedVariableFragment`, `recordUpstreamVariables bool`, `upstreamVariableCollision bool`. Set `recordUpstreamVariables = p.dataSourcePlannerConfig.Options.EnableMultiFetch && p.config.grpc == nil` (in `EnterDocument`, after config is available; `Register` stores the config first).
2. Recording helper — note the error return so existing error handling is preserved:

```go
// setUpstreamVariable writes a top-level body.variables entry and, when
// MultiFetch recording is on, mirrors it into upstreamVariablesList in write
// order with replace-in-slot semantics on duplicate names. A duplicate write
// to the "representations" slot marks the fetch as non-mergeable.
func (p *Planner[T]) setUpstreamVariable(target []byte, name string, raw []byte) ([]byte, error) {
	out, err := sjson.SetRawBytes(target, name, raw)
	if err != nil {
		return out, err
	}
	if p.recordUpstreamVariables {
		value := make([]byte, len(raw))
		copy(value, raw)
		for i := range p.upstreamVariablesList {
			if p.upstreamVariablesList[i].Name == name {
				if name == "representations" {
					p.upstreamVariableCollision = true
				}
				p.upstreamVariablesList[i].Value = value
				return out, nil
			}
		}
		p.upstreamVariablesList = append(p.upstreamVariablesList, resolve.NamedVariableFragment{Name: name, Value: value})
	}
	return out, nil
}
```

3. Route all six write sites through it, preserving each site's current error handling exactly: `addRepresentationsVariable` (graphql_datasource.go:849), `configureFieldArgumentSource` (:1157), `addVariableDefinitionsRecursively` (:1241), `configureObjectFieldSource` (:1286), `addDirectiveToNode` (:247) currently discard the error (`p.upstreamVariables, _ = ...` → `p.upstreamVariables, _ = p.setUpstreamVariable(p.upstreamVariables, name, raw)`); the opVars merge loop in `createInputForQuery` (:302-315) currently propagates the error into `stopWithError` — keep that: `upstreamVariables, err = p.setUpstreamVariable(upstreamVariables, string(key), value)` (it writes to the LOCAL copy; pass that local as `target`). Keep the existing quote-wrapping for string values, applied before the call.
4. In `ConfigureFetch`, after `createInputForQuery` and before building the return value:

```go
	var mergeableOperation *resolve.MergeableOperation
	if p.recordUpstreamVariables && !p.upstreamVariableCollision && (requiresEntityFetch || requiresEntityBatchFetch) {
		mergeableOperation = &resolve.MergeableOperation{
			Document:  p.upstreamOperation,
			Variables: p.upstreamVariablesList,
		}
		p.upstreamOperation = nil
	}
```

   and set `MergeableOperation: mergeableOperation` on the returned `resolve.FetchConfiguration`. (Nil-ing `p.upstreamOperation` transfers ownership; `EnterDocument` allocates fresh on any reuse.)

- [ ] **Step 1: Write failing tests** in a function named `TestConfigureFetch_MergeableOperation` (subtests per bullet — Step 2's `-run` pattern targets this name). Drive the plan-level path: pick an existing federation planning test fixture in this package (one that plans an entity fetch), run `plan.NewPlanner` with `Configuration.EnableMultiFetch: true`, take the produced `*plan.SynchronousResponsePlan` WITHOUT postprocessing, and inspect `Response.RawFetches`:
  - entity fetch ⇒ `MergeableOperation != nil`; `Variables` names in write order include `representations`; that fragment matches `^\[\$\$\d+\$\$\]$`; `astprinter.PrintString(Document)` contains `_entities`;
  - root fetch ⇒ `MergeableOperation == nil`;
  - flag off ⇒ nil everywhere AND every fetch `Input` string byte-identical to the flag-on run;
  - collision: a fixture whose client operation declares `$representations` used as an ordinary field argument on the entity path ⇒ `MergeableOperation == nil` (the `upstreamVariableCollision` guard).
- [ ] **Step 2: Run to verify failure**: `gotestsum --format=short -- ./pkg/engine/datasource/graphql_datasource/... -run TestConfigureFetch_MergeableOperation`.
- [ ] **Step 3: Implement** items 1-4 plus the plan-package threading (three edits mirroring `EnableOperationNamePropagation`).
- [ ] **Step 4: Green**; then `gotestsum --format=short -- ./pkg/engine/datasource/graphql_datasource/...` (slow; once) and `gotestsum --format=short -- ./pkg/engine/plan/...`.
- [ ] **Step 5: gofmt** touched files.

---

### Task 5: postprocess createMultiFetch — stage skeleton, candidates, waves, grouping, clearing

**Files:**
- Create: `v2/pkg/engine/postprocess/create_multi_fetch.go`
- Modify: `v2/pkg/engine/postprocess/postprocess.go` (option + slot + invocation)
- Test: `v2/pkg/engine/postprocess/create_multi_fetch_test.go`

**Interfaces:**
- Consumes: `replaceDependsOnFetchID`, `resolveInputTemplate` (Task 2), resolve types (Task 3).
- Produces (Tasks 6-7 fill in): `createMultiFetch` struct with `ProcessFetchTree`, private `collectGroups(root *resolve.FetchTreeNode) [][]*resolve.FetchTreeNode`, `clearMergeableOperations(root *resolve.FetchTreeNode)`, `partitionByDeferID(children []*resolve.FetchTreeNode) [][]*resolve.FetchTreeNode`.

Wiring (spec 4.2): add `enableMultiFetch bool` to `processorOptions`; option

```go
func EnableMultiFetch() ProcessorOption {
	return func(o *processorOptions) {
		o.enableMultiFetch = true
	}
}
```

Do NOT touch `DisableResolveInputTemplates()`. Enforce the interaction once in `NewProcessor`: `enableMultiFetch := opts.enableMultiFetch && !opts.disableResolveInputTemplates`; construct the stage with `disable: !enableMultiFetch`. Stage field `createMultiFetch *createMultiFetch` in `FetchTreeProcessors`; invoke in `processFlatFetchTree` between `addMissingNestedDependencies` and `resolveInputTemplates` — UNCONDITIONALLY (the stage itself handles `disable`; clearing must always run):

```go
	p.createMultiFetch.ProcessFetchTree(fetches)
```

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
			c.mergeGroup(root, group) // Tasks 6-7
		}
	}
	c.clearMergeableOperations(root)
}
```

Candidate predicate (spec 4.4): fetch is `*resolve.SingleFetch`; `RequiresEntityFetch || RequiresEntityBatchFetch`; `MergeableOperation != nil`; `Info != nil`; well-formed record: no duplicate `Name`s and exactly one fragment whose `Value` matches **exactly** `^\[\$\$\d+\$\$\]$` (the form `addRepresentationsVariable` always writes) with `N < len(Variables)` and `Variables[N]` being a `*resolve.ResolvableObjectVariable`. Helper `representationsFragmentIndex(fetch *resolve.SingleFetch) int` returns that fragment's index, -1 when malformed — the exact-match rule makes Task 7's bracket-strip unconditionally safe.

`partitionByDeferID(children)`: buckets keyed by `Item.Fetch.Dependencies().DeferID`, buckets emitted in first-seen order, nodes inside each bucket preserving original child order.

Wave computation — reuse the real stages on a scratch tree per partition (zero drift):

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

(The stages only mutate the scratch root's ChildNodes slice, never the nodes.) Groups = candidates bucketed by `(Info.DataSourceID, DeferID, wave)`; keep buckets with ≥2 members, members in the order they appear inside the scratch wave.

`clearMergeableOperations` walks `root.ChildNodes` and nils `MergeableOperation` on every `*resolve.SingleFetch`.

- [ ] **Step 1: Write failing tests** targeting `collectGroups`/`clearMergeableOperations` directly (package-internal), plus two pipeline-level tests. Build flat Sequence trees of `resolve.Single(&resolve.SingleFetch{...})` nodes in the style of `create_parallel_nodes_test.go`. IMPORTANT: every grouping tree also contains a non-candidate root node `FetchID: 0` (empty `DependsOnFetchIDs`, no MergeableOperation) so that candidate dependencies `{0}` are actually provided in the wave simulation. Candidate template: `FetchConfiguration{RequiresEntityBatchFetch: true, MergeableOperation: &resolve.MergeableOperation{Variables: []resolve.NamedVariableFragment{{Name: "representations", Value: []byte("[$$0$$]")}}}, Variables: resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{}))}` with `Info: &resolve.FetchInfo{DataSourceID: "ds1"}`. Cases:
  1. root 0 + two same-ds candidates (deps `{0}`) ⇒ one group of 2;
  2. two candidates with empty deps (no root needed) ⇒ one group (wave 0);
  3. different DataSourceID ⇒ no group;
  4. candidate 2 depends on candidate 1 ⇒ no group (different waves);
  5. per-DeferID partition: root 0 (DeferID 0), candidates 1,2 (DeferID 0, deps {0}), candidates 3,4 (DeferID 7, EMPTY deps) ⇒ two groups {1,2} and {3,4} (waves computed inside each partition; an out-of-partition dependency is never "provided" there, so the defer candidates must be satisfiable within their own partition);
  5b. negative companion: candidates 3,4 with DeferID 7 and deps {0} (provider lives in the DeferID-0 partition) ⇒ NOT grouped — they stay serial inside their defer group, mirroring the real per-group organize (spec 3.2/4.4);
  6. nil Info / nil MergeableOperation / non-entity ⇒ not candidates;
  7. malformed record (duplicate names; or fragment token pointing at a non-ResolvableObject variable) ⇒ not a candidate;
  8. `clearMergeableOperations` via `(&createMultiFetch{disable: true}).ProcessFetchTree(tree)` ⇒ artifacts nil'd;
  9. pipeline-level unconditional clearing: `postprocess.NewProcessor()` (NO EnableMultiFetch) over a plan whose RawFetches carry MergeableOperation ⇒ after `Process`, every fetch's artifact is nil;
  10. `NewProcessor(EnableMultiFetch(), DisableResolveInputTemplates())` over two valid candidates ⇒ no `MultiEntityFetch` emitted, fetches keep readable `Input` strings, artifacts still cleared.
- [ ] **Step 2: Run to verify failure**: `gotestsum --format=short -- ./pkg/engine/postprocess/... -run TestCreateMultiFetch`.
- [ ] **Step 3: Implement** skeleton + wiring, with `mergeGroup` a temporary no-op (filled by Tasks 6-7; the stage stays dark until the option is used, and Tasks 5-7 land as one reviewed sequence).
- [ ] **Step 4: Green** new tests + full package.
- [ ] **Step 5: gofmt.**

---

### Task 6: createMultiFetch — merged document construction

**Files:**
- Create: `v2/pkg/engine/postprocess/create_multi_fetch_document.go`
- Test: extend `v2/pkg/engine/postprocess/create_multi_fetch_test.go`

**Interfaces:**
- Consumes: Task 1 astimport methods (public surface only).
- Produces (used by Task 7, exact signature — member fetch IDs come from `members[i].FetchDependencies.FetchID`):

```go
// buildMergedOperation merges the group members' stored documents into one
// aliased, @include-guarded operation and returns the compact and pretty
// printed forms.
func buildMergedOperation(members []*resolve.SingleFetch) (compact string, pretty string, err error)
```

Algorithm (spec 4.4 steps 1-6), per member k (1-based; `kStr := strconv.Itoa(k)`):
1. `merged := ast.NewSmallDocument()` (a zero-value `ast.Document` panics in the Add* helpers). Add the operation: fresh selection set via `merged.AddSelectionSet()` (returns `ast.Node`; keep its `.Ref`), then `merged.AddOperationDefinitionToRootNodes(ast.OperationDefinition{OperationType: ast.OperationTypeQuery, HasSelections: true, SelectionSet: opSetRef})` and keep the operation ref. Name it `<OperationName>__multi_<id1>_<id2>...` iff every member's `FetchConfiguration.OperationName` is equal and non-empty (IDs from `members[i].FetchDependencies.FetchID`; name bytes via `merged.Input.AppendInputString`, set `Name` on the op def); otherwise anonymous.
2. Rename map for member k over the UNION of (a) the member document's operation variable-definition names (`doc.VariableDefinitionNameString`) and (b) every `NamedVariableFragment.Name` (the blob can contain stale keys absent from the document, spec 3.1): `rename[name] = name + "_f" + kStr`.
3. Import each of the member document's variable definitions: `importedRef := importer.ImportVariableDefinitionWithVariableNameRename(ref, doc, merged, rename[origName])`; attach with `merged.AddImportedVariableDefinitionToOperationDefinition(opRef, importedRef)`.
4. Add the include variable: `boolType := merged.AddNamedType([]byte("Boolean"))`; `nonNullBool := merged.AddNonNullType(boolType)`; `includeVar := merged.ImportVariableValue([]byte("includeF" + kStr))`; `merged.AddVariableDefinitionToOperationDefinition(opRef, includeVar, nonNullBool)`.
5. Import the member's root field USING ONLY THE PUBLIC API: `importedSetRef, err := importer.ImportSelectionSetWithVariableRename(doc.OperationDefinitions[0].SelectionSet, doc, merged, rename)`. Then assert the imported set has exactly one selection of kind Field whose name is `_entities` (else return an error — the member is malformed). Take `fieldRef := merged.Selections[merged.SelectionSets[importedSetRef].SelectionRefs[0]].Ref` and mutate the imported field in place:

```go
	merged.Fields[fieldRef].Alias = ast.Alias{IsDefined: true, Name: merged.Input.AppendInputString("f" + kStr)}
	includeArgValue := ast.Value{Kind: ast.ValueKindVariable, Ref: merged.AddVariableValue(ast.VariableValue{Name: merged.Input.AppendInputString("includeF" + kStr)})}
	includeArg := merged.AddArgument(ast.Argument{Name: merged.Input.AppendInputString("if"), Value: includeArgValue})
	directiveRef := merged.AddDirective(ast.Directive{Name: merged.Input.AppendInputString("include"), HasArguments: true, Arguments: ast.ArgumentList{Refs: []int{includeArg}}})
	merged.Fields[fieldRef].HasDirectives = true
	merged.Fields[fieldRef].Directives.Refs = append(merged.Fields[fieldRef].Directives.Refs, directiveRef)
```

   then append the selection to the operation's set: `merged.AddSelection(opSetRef, ast.Selection{Kind: ast.SelectionKindField, Ref: fieldRef})`. (Note: the field arrives inside `importedSetRef`; only the field ref is re-attached to the operation's own selection set — the imported wrapper set is simply left unused.)
6. Print: `compact, err := astprinter.PrintString(merged)`; `pretty, err := astprinter.PrintStringIndent(merged, "  ")`. No re-normalization/re-validation (spec 4.4).

- [ ] **Step 1: Write failing test** `TestBuildMergedOperation`. Member documents parsed with `astparser.ParseGraphqlDocumentString` from — note BOTH declare `$first` (cross-member same-name coverage):
  - m1: `query($representations: [_Any!]!, $first: Int){_entities(representations: $representations){... on Employee {__typename products(first: $first) {upc}}}}`
  - m2: `query($representations: [_Any!]!, $first: Int){_entities(representations: $representations){... on Employee {__typename notes(first: $first)}}}`

  Fragments: m1 `[{representations,[$$0$$]},{first,$$1$$},{stale,1}]` (the `stale` name has NO matching variable definition — stale-key coverage); m2 `[{representations,[$$0$$]},{first,$$1$$}]`. Members are `&resolve.SingleFetch{FetchConfiguration: resolve.FetchConfiguration{MergeableOperation: ..., OperationName: ""}, FetchDependencies: resolve.FetchDependencies{FetchID: 3 /* and 5 */}}`. Assertions on `compact`:
  - declares `$representations_f1: [_Any!]!`, `$first_f1: Int`, `$includeF1: Boolean!`, `$representations_f2`, `$first_f2`, `$includeF2` — and does NOT declare `$stale_f1` (stale keys get no definition);
  - contains `f1: _entities(representations: $representations_f1)@include(if: $includeF1)` and `f2: _entities(representations: $representations_f2)@include(if: $includeF2)` — note NO space before `@`: astprinter writes directives on fields without a leading space (see the committed golden `)@onOperation` in graphql_datasource_test.go); the spec section-1 example's spacing is illustrative GraphQL, not printer output;
  - m1's subtree references `$first_f1`, m2's references `$first_f2`.
  Freeze the full golden on first green run (printer spacing), then assert equality. Second case: both members `OperationName: "Q"` ⇒ compact starts `query Q__multi_3_5(`. Third case: member root selection not a single `_entities` field ⇒ error.
- [ ] **Step 2: Run to verify failure.**
- [ ] **Step 3: Implement** `buildMergedOperation` exactly with the Produces signature.
- [ ] **Step 4: Green**; full package run.
- [ ] **Step 5: gofmt.**

---

### Task 7: createMultiFetch — input split, entry assembly, fetch construction, integration

**Files:**
- Create: `v2/pkg/engine/postprocess/create_multi_fetch_input.go`
- Modify: `v2/pkg/engine/postprocess/create_multi_fetch.go` (`mergeGroup` real implementation)
- Test: extend `v2/pkg/engine/postprocess/create_multi_fetch_test.go`

**Interfaces:**
- Consumes: Task 2 (`resolveInputTemplate`, `replaceDependsOnFetchID`), Task 3 (resolve types), Task 5 (`representationsFragmentIndex(fetch *resolve.SingleFetch) int` in create_multi_fetch.go), Task 6 (`buildMergedOperation(members []*resolve.SingleFetch) (compact, pretty string, err error)`).
- Produces: the complete stage; the loader consumes the emitted `*resolve.MultiEntityFetch`.

`create_multi_fetch_input.go` — the scanner supports BOTH input shapes (Global Constraints; spec 3.1/4.4):

```go
// fetchInputSplit locates the body.query string value and the body.variables
// object value inside a fetch input, supporting both sjson key orders:
// repo shape   {"method":...,["header":...,]"body":{"query":"...","variables":{...}}}
// append shape {"body":{"variables":{...},"query":"..."},...}
// ok is false when the input matches neither; such groups are not merged.
type fetchInputSplit struct {
	queryStart, queryEnd         int // query string value content range (between the quotes)
	variablesStart, variablesEnd int // variables object value range including braces
}

func splitEntityFetchInput(input string) (s fetchInputSplit, ok bool)
```

Algorithm:
- **Repo shape** (input does NOT start with `{"body":`): find the LAST occurrence of the anchor `"body":{"query":"` — body is the final top-level key in this shape, printed string literals inside the query escape their quotes so the query cannot contain the raw anchor, and last-occurrence guards against pathological unescaped header values. `queryStart` = end of anchor. The variables object is at the very end: require the input to end with `}}}`; backward brace-balanced, quote-aware scan starting at the `}` at `len(input)-3` to find the variables object start. Backward quote detection: a `"` is escaped iff preceded by an odd-length run of backslashes (literal fragments are well-formed JSON with escapes; `$$N$$` tokens contain neither braces nor quotes). If brace balance does not reach zero before reaching `queryStart`, return `ok=false`. Require the bytes immediately before `variablesStart` to be `,"variables":`; `queryEnd` = start of that marker. Round-trip: `input[queryEnd-1]` must be the query's closing quote position, i.e. the marker is preceded by `"` — set `queryEnd` to that quote's index and check `input[queryEnd] == '"'`.
- **Append shape** (input starts with `{"body":{"variables":`): `variablesStart` = position of that `{`; forward brace-balanced quote-aware scan for `variablesEnd`; require `,"query":"` next; `queryStart` after it; `queryEnd` = the greatest q such that `input[q:q+2] == "\"}"` AND `q+2 < len(input)` AND `input[q+2] == ','` AND `input[q-1] == '}'` (a printed operation always ends with a closing selection-set brace; the extra check rejects `"},` byte sequences inside JSON-escaped envelope header values, and a trailing `"POST"}` at EOF never matches because nothing follows it). No such q ⇒ `ok=false`.
- Anything else ⇒ `ok = false`.

`mergeGroup(root, group)` (spec 4.4). Private helpers defined alongside: `minID(ids []int) int` (lowest member fetch ID) and `unionDependencies(members []*resolve.SingleFetch, ids []int) []int` (union of members' `DependsOnFetchIDs` minus member IDs; duplicates tolerated).
1. Members `s1..sn` (`*resolve.SingleFetch`), `ids[i] = members[i].FetchID`. Split every member's `Input`; any `!ok` ⇒ abort (leave nodes untouched). Envelope precondition, shape-neutral: order each member's two value ranges by start offset — `(aStart,aEnd)` the earlier, `(bStart,bEnd)` the later (query first in repo shape, variables first in append shape); envelope remainder = `input[:aStart] + input[aEnd:bStart] + input[bEnd:]`. Each member's remainder must be byte-equal to s1's; additionally every `$$K$$` token inside the remainder must satisfy `si.Variables[K].Equals(s1.Variables[K])` (split the remainder with the blind `$$` alternation); else abort.
2. `compact, pretty, err := buildMergedOperation(members)`; error ⇒ abort.
3. Header/Footer templates from s1's split, replacing the query range with `compact` and splitting at the variables object:
   - repo shape (query before variables): Header source = `s1.Input[:queryStart] + compact + s1.Input[queryEnd:variablesStart] + "{"`; Footer source = `"}" + s1.Input[variablesEnd:]` (i.e. `}}}` tail).
   - append shape (variables before query): Header source = `s1.Input[:variablesStart] + "{"`; Footer source = `"}" + s1.Input[variablesEnd:queryStart] + compact + s1.Input[queryEnd:]`.
   Resolve both with `var header resolve.InputTemplate; resolveInputTemplate(s1.Variables, headerSource, &header)` (out-param; returns nothing) and likewise for the footer — envelope `$$K$$` header tokens become proper segments.
4. Entries, per member k: `alias := "f" + kStr`; `OriginKind` from `RequiresEntityFetch` (single) / `RequiresEntityBatchFetch` (batch). Representations: fragment at `representationsFragmentIndex`; its value is `[$$N$$]` — strip the surrounding brackets and resolve the inner token: `var reps resolve.InputTemplate; resolveInputTemplate(sk.Variables, inner, &reps); reps.SetTemplateOutputToNullOnVariableNull = true`. Prefixes:

```go
	prefix := `"representations_f` + kStr + `":[`
	if k > 1 {
		prefix = "," + prefix
	}
	includePrefix := `],"includeF` + kStr + `":`
```

   Other variables — every non-representations fragment, in record order:

```go
	var tpl resolve.InputTemplate
	resolveInputTemplate(sk.Variables, string(frag.Value), &tpl)
	variables = append(variables, resolve.MultiEntityFetchVariable{
		KeyPrefix: []byte(`,"` + frag.Name + "_f" + kStr + `":`),
		Value:     tpl,
	})
```

   (no null-flag on these — spec 4.4). Skip flags all true. `PostProcessing`: `SelectResponseDataPath: []string{"data", alias}`, `SelectResponseErrorsPath: []string{"errors"}`, `MergePath: sk.PostProcessing.MergePath`. `Item`: copy of the member node's `*resolve.FetchItem` (same FetchPath/ResponsePath/ResponsePathElements) — its `Fetch` field is set to the multi after construction. `Info: sk.Info`.
5. The fetch:

```go
	multi := &resolve.MultiEntityFetch{
		FetchDependencies: resolve.FetchDependencies{
			FetchID:           minID(ids),
			DependsOnFetchIDs: unionDependencies(members, ids), // union of members' DependsOnFetchIDs minus member IDs; duplicates tolerated
			DeferID:           members[0].FetchDependencies.DeferID,
		},
		Input:                resolve.MultiEntityInput{Header: header, Entries: entries, Footer: footer},
		DataSource:           members[0].DataSource,
		DataSourceIdentifier: members[0].DataSourceIdentifier,
		MergedFetchIDs:       ids,
		Info:                 mergedInfo,
	}
```

   `mergedInfo` (spec 4.4): `DataSourceID`/`DataSourceName` from s1; `OperationType: ast.OperationTypeQuery`; `RootFields` = deduplicated union; `CoordinateDependencies`/`FetchReasons`/`PropagatedFetchReasons` = concatenations; `QueryPlan: &resolve.QueryPlan{Query: pretty, DependsOnFields: <concat of members' Info.QueryPlan.DependsOnFields>}` iff every member's `Info.QueryPlan != nil`, else nil. Then repoint every entry: `entries[i].Item.Fetch = multi`.
6. Tree surgery: replace the first member's node with `&resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: multi}}`; remove the other member nodes from `root.ChildNodes`; then for EACH `id` in `ids` with `id != multi.FetchID`: `replaceDependsOnFetchID(root, id, multi.FetchID)`. This must include the first member's original ID when it is not the group minimum — wave order sorts by dependency count, so s1 need not carry the lowest ID, and a dependent referencing its vanished ID would otherwise break ordering and the loader's skip cascade (spec 4.4).

- [ ] **Step 1: Write failing tests.**
  - `TestSplitEntityFetchInput` table — repo shape canonical: `{"method":"POST","url":"http://x","header":{"Auth":["$$2$$"]},"body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename}}","variables":{"representations":[$$0$$],"a":$$1$$}}}` ⇒ assert the extracted query text and variables object; repo shape with escapes in a literal fragment: variables `{"a":"x\"}","b":"y\\","representations":[$$0$$]}` ⇒ correct variables range (backslash-parity handling); append shape: `{"body":{"variables":{"representations":[$$0$$],"a":$$1$$},"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename}}"},"header":{"Auth":["$$2$$"]},"url":"http://x","method":"POST"}` ⇒ same extractions; append shape with a JSON-escaped header value whose raw bytes contain `"},` (e.g. header value `{"a":"b"},x` marshaled) ⇒ query end still lands at the operation's closing brace (the `input[q-1] == '}'` check); deviants (no body.query anchor; input not ending `}}}` in repo shape; truncated input) ⇒ `ok=false`.
  - `TestCreateMultiFetch_MergeGroup` with EXACT member fixtures (repo shape, token-free envelope for the happy path). Every candidate gets `Info: &resolve.FetchInfo{DataSourceID: "products-id", DataSourceName: "products", OperationType: ast.OperationTypeQuery}` (identical across members — candidacy requires `Info != nil` and grouping keys on `Info.DataSourceID`); fetches 0 and 3 need no Info (non-candidates):
    - m1 document source (no `$first` here, unlike Task 6's m1 — its fragments must match its declarations plus the stale extra): `query($representations: [_Any!]!){_entities(representations: $representations){... on Employee {__typename products {upc}}}}`; m1 Input: `{"method":"POST","url":"http://x","body":{"query":"<m1 source>","variables":{"representations":[$$0$$],"stale":1}}}`, `Variables: resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{}))`, fragments `[{representations,[$$0$$]},{stale,1}]` (the stale fragment has no matching variable definition), `PostProcessing: resolve.PostProcessingConfiguration{MergePath: []string{"a"}}`, ResponsePath `employees.@`, `FetchID: 1`, deps `{0}`, `RequiresEntityBatchFetch: true`;
    - m2 document source: `query($representations: [_Any!]!, $first: Int){_entities(representations: $representations){... on Employee {__typename notes(first: $first)}}}`; m2 Input: `{"method":"POST","url":"http://x","body":{"query":"<m2 source>","variables":{"representations":[$$0$$],"first":$$1$$}}}`, `Variables: resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{}), &resolve.ContextVariable{Path: []string{"first"}, Renderer: resolve.NewJSONVariableRenderer()})`, fragments `[{representations,[$$0$$]},{first,$$1$$}]`, `PostProcessing: resolve.PostProcessingConfiguration{MergePath: []string{"b"}}`, ResponsePath `employee`, `FetchID: 2`, deps `{0}`, `RequiresEntityFetch: true`;
    - root fetch 0 (non-candidate) and fetch 3 with deps `{2}`.
    Run the full `Processor.Process` with `EnableMultiFetch()`. Assert: final tree `Sequence(Single(0), Single(multi), Single(3))`; `multi.FetchID == 1`, `MergedFetchIDs == [1,2]`; fetch 3 deps now `{1}`; entries: aliases f1/f2, `RepresentationsPrefix` exactly `"representations_f1":[` and `,"representations_f2":[`, `IncludePrefix` `],"includeF1":`/`],"includeF2":`, PostProcessing data paths `["data","f1"]`/`["data","f2"]`, origins batch/single, m1's entry has one variable with KeyPrefix `,"stale_f1":` rendering the literal `1` (a recorded fragment with no variable definition still flows into body.variables — spec 3.1), m2's entry has one variable with KeyPrefix `,"first_f2":`; Header's first static segment starts `{"method":"POST"` and contains `"query":"query(` (the merged compact operation) and ends with `"variables":{`; Footer static is `}}}`; every fetch's `MergeableOperation == nil` afterwards.
  - Append-shape variant: same two members but Inputs in the mirrored append shape (`{"body":{"variables":{...},"query":"<source>"},"url":"http://x","method":"POST"}`) ⇒ merge succeeds; Header static is `{"body":{"variables":{`; Footer static starts `},"query":"query(` and ends `"},"url":"http://x","method":"POST"}`.
  - Three-member group variant: add m3 (same shape as m1 minus the stale fragment, `FetchID: 4`, deps `{0}`, ResponsePath `contractors.@`, `MergePath: ["c"]`) ⇒ one multi with entries f1/f2/f3, `,"representations_f3":[` prefix, `],"includeF3":` and `MergedFetchIDs == [1,2,4]`.
  - Survivor-ID rewrite case: providers 0 and 1 (non-candidates), members `FetchID: 7` (deps `{0}`) and `FetchID: 4` (deps `{0,1}`) — wave order puts 7 first (fewer deps) — plus fetch 9 with deps `{7}` ⇒ `multi.FetchID == 4`, `MergedFetchIDs == [7,4]`, and fetch 9's deps rewritten to `{4}` (the first member's vanished ID must be rewritten too).
  - Abort cases: different `url` envelope ⇒ untouched; same envelope BYTES with a `$$2$$` header token but `s2.Variables[2]` a `HeaderVariable` with a different path than s1's ⇒ untouched (token cross-check); malformed input ⇒ untouched.
- [ ] **Step 2: Run to verify failure.**
- [ ] **Step 3: Implement** scanner + `mergeGroup`.
- [ ] **Step 4: Green**; full postprocess package.
- [ ] **Step 5: gofmt.**

---

### Task 8: loader — prepareMultiEntityFetch

**Files:**
- Create: `v2/pkg/engine/resolve/loader_multi_entity.go`
- Modify: `v2/pkg/engine/resolve/loader.go` (`preparePhase` case; `preparedFetch.multiEntries` field; `batchEntityTools.clearDedupState()` method)
- Test: `v2/pkg/engine/resolve/loader_multi_entity_test.go`

**Interfaces:**
- Consumes: Task 3 types; loader internals (`selectItemsForPath`, `isFetchAuthorized`, `rateLimitFetch`, `batchEntityToolPool`, `setTracingInput`).
- Produces (Task 9 consumes):

```go
type preparedMultiEntry struct {
	entry *MultiEntityFetchEntry
	items []*astjson.Value // merge targets from selectItemsForPath (jsonArena-backed, same lifecycle as preparedFetch.items — no copy needed)
	res   *result          // per-entry view; init(entry.PostProcessing, entry.Info)
}
```

  `preparedFetch` gains `multiEntries []preparedMultiEntry`. `batchEntityTools` gains a new method (the existing `reset()` — called by the pool's `Put`, which also resets the arena — stays untouched):

```go
// clearDedupState resets the per-entry dedup scope without touching the
// arena, whose buffers must survive until final input assembly.
func (b *batchEntityTools) clearDedupState() {
	b.keyGen.Reset()
	for k := range b.batchHashToIndex {
		delete(b.batchHashToIndex, k)
	}
}
```

`preparePhase` new case:

```go
	case *MultiEntityFetch:
		err := l.prepareMultiEntityFetch(item, fetch, res, prepared)
		return prepared, err
```

(The generic `selectItemsForPath(item.FetchPath)` above the switch harmlessly selects the root; ignore its result for multi.)

`prepareMultiEntityFetch(fetchItem *FetchItem, fetch *MultiEntityFetch, res *result, prepared *preparedFetch) error` (spec 4.6, exact order):
1. `res.init(PostProcessingConfiguration{SelectResponseErrorsPath: []string{"errors"}}, fetch.Info)`; when tracing: `fetch.Trace = &DataSourceLoadTrace{}`. `res.tools = batchEntityToolPool.Get(...)` ONCE on the shared result (per-entry results keep `tools == nil`; the existing `resolveSingle` defer returns the shared one).
2. Per entry k: `entryRes := &result{}; entryRes.init(entry.PostProcessing, entry.Info)`; `items := l.selectItemsForPath(entry.Item.FetchPath)`. Authorization first (input-free — the authorizer path is unreachable for query-typed entries, spec Q2): `allowed, err := l.isFetchAuthorized(nil, entry.Info, entryRes)`; err ⇒ return; `!allowed` ⇒ entry excluded (entryRes now carries fetchSkipped and/or authorizationRejected exactly as the helper set them).
3. Representations for non-excluded entries into a per-entry arena buffer (`arena.NewArenaBuffer(res.tools.a)`): mirror `prepareBatchEntityFetch`'s loop (loader.go:1696-1744) — render `entry.Representations` per item honoring `SkipNullItems`/`SkipEmptyObjectItems`/`SkipErrItems`, xxhash dedup via `res.tools`, `,` between unique items, arena batchStats. After EACH entry: copy its batchStats to the heap into `entryRes.batchStats` (mirror loader.go:1768-1773) and call `res.tools.clearDedupState()`. Zero unique items ⇒ excluded (`entryRes.fetchSkipped = true`).
4. `include_k = !excluded`. Excluded entries contribute `[]` representations (a denied entry's rendered bytes are discarded — never sent) but their non-representations variables are STILL rendered (variable coercion precedes @include on the subgraph, spec 4.6 step 4).
5. Assemble once into a `bytes.Buffer`:
   - `fetch.Input.Header.RenderAndCollectUndefinedVariables(l.ctx, nil, buf, &undefined)`;
   - per entry: `buf.Write(entry.RepresentationsPrefix)`; included ⇒ the entry's representations bytes; `buf.Write(entry.IncludePrefix)`; `true`/`false`;
   - per entry variable: render `v.Value` into a scratch arena buffer with a FRESH undefined slice; omit the whole pair iff the rendered bytes equal `null` AND the fresh slice is non-empty (undefined collected during THIS render; explicit client null renders null with an empty slice ⇒ kept); else `buf.Write(v.KeyPrefix); buf.Write(scratch.Bytes())`;
   - `fetch.Input.Footer.RenderAndCollectUndefinedVariables(...)`. No `SetInputUndefinedVariables` call (Header/Footer collect nothing; entry pairs were stripped inline).
6. All entries excluded ⇒ `res.fetchSkipped = true; prepared.skipLoad = true`; when tracing, `l.setTracingInput(fetchItem, buf.Bytes(), fetch.Trace)`; then **return nil** — rate limiting must not run for a request that is never sent (mirror `prepareBatchEntityFetch` returning before `validatePreFetch`, loader.go:1746-1779).
7. Rate limit once: `allowed, err := l.rateLimitFetch(buf.Bytes(), fetch.Info, res)`; `!allowed` ⇒ `prepared.skipLoad = true` (flags on the shared res; Task 9 fans out).
8. `prepared.source, prepared.input, prepared.trace = fetch.DataSource, buf.Bytes(), fetch.Trace`; `prepared.multiEntries = ...`; when tracing and `!ExcludeRawInputData`: `fetch.Trace.RawInputData` = `{"f1":<itemsData(entry1 items)>,"f2":...}` marshaled per alias.

- [ ] **Step 1: Write failing tests** (mirror the Loader/Context/dataBuffer construction used by existing `TestLoader_*` tests in `loader_test.go`; seed the data buffer with `{"employees":[{"__typename":"Employee","id":1},{"__typename":"Employee","id":2},{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}` — employees[2] duplicates employees[0]):
  - `TestPrepareMultiEntityFetch_Assembly`: entry1 batch over `ArrayPath("employees")` with a representations renderer over `{__typename, id}`; entry2 single over `ObjectPath("employee")`; entry2 has one variable `{KeyPrefix: ",\"first_f2\":", Value: <ContextVariable path ["first"] template>}` and `ctx.Variables = {"first": 10}`. Assert the assembled input: Header statics, `"representations_f1":[{"__typename":"Employee","id":1},{"__typename":"Employee","id":2}]` (deduped to 2 uniques; `entryRes.batchStats == [[e0,e2],[e1]]`), `],"includeF1":true`, `,"representations_f2":[{"__typename":"Employee","id":9}]`, `],"includeF2":true`, `,"first_f2":10`, Footer statics.
  - `TestPrepareMultiEntityFetch_EmptyEntry`: `employee` null in seeded data ⇒ `,"representations_f2":[],"includeF2":false`, entry2 `fetchSkipped`, entry2's `first_f2` variable still rendered.
  - `TestPrepareMultiEntityFetch_DeniedEntry`: loader with the pre-fetch field-authorization cache seeded to deny ALL of entry2's `Info.RootFields` (mirror existing FieldAuthorization tests for construction) ⇒ entry2 excluded: `"representations_f2":[]`, `includeF2":false`, `fetchSkipped` on entry2's res, NO authorizationRejected error rendering expected, entry1 unaffected.
  - `TestPrepareMultiEntityFetch_AllExcluded`: both entries empty ⇒ `prepared.skipLoad`, shared `res.fetchSkipped`.
  - `TestPrepareMultiEntityFetch_UndefinedVariable`: `$first` ABSENT from ctx.Variables ⇒ the `"first_f2"` key/value pair omitted entirely (no comma, no key); explicit `ctx.Variables = {"first": null}` ⇒ `,"first_f2":null` kept.
  - `TestPrepareMultiEntityFetch_DedupStateIsolation`: identical representation bytes reachable from entry1 and entry2 ⇒ each entry's array still contains it (no cross-entry dedup).
- [ ] **Step 2: Run to verify failure**: `gotestsum --format=short -- ./pkg/engine/resolve/... -run TestPrepareMultiEntityFetch`.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Green** + full resolve package.
- [ ] **Step 5: gofmt.**

---

### Task 9: loader — mergeResult refactor + mergeMultiEntityResult

**Files:**
- Modify: `v2/pkg/engine/resolve/loader.go` (`result` field, `mergeResult` injection points, `mergePhase` multi branch, `rewriteErrorPaths`/`mergeErrors` root-name handling)
- Modify: `v2/pkg/engine/resolve/tainted_objects.go` (`getTaintedIndices` signature)
- Modify: `v2/pkg/engine/resolve/tainted_objects_test.go` (mechanical: direct `getTaintedIndices` callsites get the new signature, passing `fetch.FetchInfo(), "_entities"`)
- Modify: `v2/pkg/engine/resolve/loader_test.go` (mechanical: the direct `rewriteErrorPaths` callsite gains the new `rootName` argument, passing `"_entities"`)
- Extend: `v2/pkg/engine/resolve/loader_multi_entity.go` + `loader_multi_entity_test.go`

**Interfaces:**
- Consumes: Task 8's `preparedFetch.multiEntries`.
- Produces: complete runtime path.

`result` gains:

```go
	// multi is set on per-entry result views during MultiEntityFetch merging.
	multi *multiEntryMergeConfig

type multiEntryMergeConfig struct {
	alias        string
	originSingle bool
	info         *FetchInfo     // taint-info source
	response     *astjson.Value // pre-parsed shared response
	errors       *astjson.Value // pre-partitioned errors array for this entry (nil = none)
}
```

`mergeResult` injection points (each a small, behavior-preserving branch; `isEmptyEntityFetch` is NOT touched — its kind check already returns false for multi entries, which is correct: a null `data.<alias>` follows the existing null-data fallthrough, and the empty-array case never reaches it):
- parse (loader.go:634): `if res.multi != nil && res.multi.response != nil { response = res.multi.response } else { parse as today }`;
- extensions collection (643-649): skip when `res.multi != nil` (the parent collects once);
- errors (662-686): when `res.multi != nil`, use `res.multi.errors` in place of `response.Get(SelectResponseErrorsPath...)`;
- taint (670): new signature `getTaintedIndices(info *FetchInfo, rootName string, data, errors *astjson.Value) []int`; existing callsite passes `fetchItem.Fetch.FetchInfo(), "_entities"`; multi entries pass `res.multi.info, res.multi.alias`;
- error-path rewriting inside `mergeErrors`: `rewriteErrorPaths` gains a `rootName string` parameter (existing caller passes `"_entities"`). For multi entries: when `l.rewriteSubgraphErrorPaths` is true, call it with `rootName = res.multi.alias`; when false, still call a new tiny helper `hideAliasInErrorPaths(a arena.Arena, alias string, values []*astjson.Value)` that replaces a LEADING path element equal to the alias with the string `"_entities"` (pass-through mode must never leak internal aliases, spec 4.7);
- empty-array single-origin edge, immediately before the batch fan-out (743): `if res.multi != nil && res.multi.originSingle { if batch := responseData.GetArray(); batch != nil && len(batch) == 0 { return nil } }`.

`mergePhase` (loader.go:366-385) gains, immediately AFTER the existing `l.dataBuffer.Lock()` / `defer l.dataBuffer.Unlock()` pair (mergeMultiEntityResult assumes the lock is held, like mergeResult):

```go
	if prepared.multiEntries != nil {
		return l.mergeMultiEntityResult(prepared)
	}
```

`mergeMultiEntityResult(prepared *preparedFetch) error` (spec 4.7; res = prepared.res). Convention, stated once: `entryRes.multi` is set ONLY on the successfully-parsed path (step 4); every fan-out path leaves it nil so each entry's `mergeResult` replays today's unmerged guards against the copied transport state.
1. Ordered short-circuits:
   - `if res.fetchSkipped && !res.rateLimitRejected { return nil }` — the all-excluded case;
   - `if res.err != nil || res.authorizationRejected || res.rateLimitRejected || len(res.out) == 0`: copy `err`, `statusCode`, `ds`, `out`, `rateLimitRejected(+Reason)`, `authorizationRejected(+Reasons)` onto every entry res (`multi` stays nil) and jump straight to the step-4 merge loop (each entry's `mergeResult` guards render failed-to-fetch / rate-limit errors at that entry's path); skip steps 2-3. (loadPhase already recorded `erroredFetchIDs` for `res.err` only — invalid responses must NOT cascade, spec 4.7 step 1.)
2. Parse once: `response, err := astjson.ParseBytesWithArena(l.jsonArena, res.out)`; on parse failure, copy `out`/`statusCode`/`ds` to entries (`multi` stays nil) and run the step-4 merge loop (per-entry status-fallback/invalid-JSON errors, exactly today's guards). On success: collect response extensions once (replicate loader.go:643-649 against `response`).
3. Partition `response.Get("errors")` by first path element matching an entry alias; errors with no alias-shaped first element go to an `unmatched` array; if non-empty, `l.mergeErrors(res, prepared.item, unmatchedValue)` once (parent fetchItem, empty ResponsePath).
4. Per entry i (parsed path only): `entryRes.multi = &multiEntryMergeConfig{alias, originSingle, entry.Info, response, entryErrors}`; copy `statusCode`, `ds`, `out`, `httpResponseContext` from res — `out` MUST be copied or mergeResult's `len(res.out) == 0` guard (which runs BEFORE the parse injection point) fires `emptyGraphQLResponse` for every entry; the parse is still skipped because `multi.response` is non-nil. Then `err := l.mergeResult(prepared.multiEntries[i].entry.Item, entryRes, prepared.multiEntries[i].items)`; join `entryRes.subgraphError` into `res.subgraphError`; remember the first error. (The fan-out paths from steps 1-2 run this same loop, just without `multi`.)
5. `l.callOnFinished(res)` once; return the first error.

- [ ] **Step 1: Write failing tests** (stub DataSource returning canned bodies; drive through `resolveSingle` on a hand-built multi FetchItem so prepare+load+merge all run):
  - `TestMergeMultiEntityResult_FanOut`: response `{"data":{"f1":[{"products":[{"upc":"1"}]},{"products":[{"upc":"2"}]}],"f2":[{"notes":"n"}]}}` with entry1 batchStats `[[e0,e2],[e1]]` ⇒ e0/e2 get upc 1, e1 upc 2, employee gets notes; final data-buffer JSON golden.
  - `TestMergeMultiEntityResult_ErrorPartitioning`: errors `[{"message":"a","path":["f1",0,"products"]},{"message":"b","path":["f2"]},{"message":"c"}]` — wrap mode: "a" at entry1's response path (index dropped), "b" at entry2's, "c" once at the multi; pass-through mode with `rewriteSubgraphErrorPaths=false`: emitted paths contain `_entities`, never `f1`/`f2`.
  - `TestMergeMultiEntityResult_EmptyArraySingleOrigin`: `{"data":{"f2":[]}}` single-origin ⇒ no error, no merge; batch-origin with 1 representation ⇒ `invalidBatchItemCount` failed-to-fetch error.
  - `TestMergeMultiEntityResult_TransportError`: stub returns error ⇒ one failed-to-fetch error PER entry path; `erroredFetchIDs` contains the multi's FetchID.
  - `TestMergeMultiEntityResult_InvalidResponse`: body `not json` ⇒ per-entry failed-to-fetch errors AND `erroredFetchIDs` does NOT contain the multi's FetchID (dependents still run).
  - `TestMergeMultiEntityResult_RateLimitRejected`: rate limiter rejects at prepare ⇒ NO Load call, one rate-limit error at EACH entry's response path.
  - `TestMergeMultiEntityResult_TaintPerEntry`: `validateRequiredExternalFields` on; entry1.Info.FetchReasons has an `IsRequires`+`Nullable` coordinate; response error path `["f1",0,"<field>"]` with the field null in data ⇒ only entry1's matching target lands in `taintedObjs`.
  - `TestMergeMultiEntityResult_ExtensionsOnce`: response with `extensions` + `allowCustomExtensionProperties` ⇒ exactly one entry in `l.subgraphExtensions`.
  - `TestMergeMultiEntityResult_HooksOnce`: recording LoaderHooks ⇒ one OnLoad, one OnFinished.
  - `TestMergeMultiEntityResult_ExcludedEntry`: entry2 excluded at prepare (`data` has only `f1`) ⇒ entry2 merges nothing, no error, entry1 normal.
- [ ] **Step 2: Run to verify failure**: `gotestsum --format=short -- ./pkg/engine/resolve/... -run TestMergeMultiEntityResult`.
- [ ] **Step 3: Implement.** After the `getTaintedIndices`/`rewriteErrorPaths` signature changes, run the FULL resolve package — both have existing direct tests.
- [ ] **Step 4: Green** + full package.
- [ ] **Step 5: gofmt.**

---

### Task 10: end-to-end — datasource plan tests + resolve integration

**Files:**
- Test: `v2/pkg/engine/datasource/graphql_datasource/graphql_datasource_multi_fetch_test.go` (new)
- Test: `v2/pkg/engine/resolve/loader_multi_entity_test.go` (integration cases)

**Interfaces:** none new — verification only.

Harness requirements (critical, verified against `v2/pkg/engine/datasourcetesting/datasourcetesting.go`): `RunTest` force-sets `DisableIncludeInfo = true` unless a field-info option is used — with nil `Info`, NO fetch is a merge candidate and the merge silently never happens. `WithDefaultCustomPostProcessor(options ...postprocess.ProcessorOption)` exists (≈line 64) and `WithPrintPlan()` (≈line 77) enables field info + `plan.IncludeQueryPlanInResponse()` — BUT `RunTest` always deep-diffs the ENTIRE post-processed plan against `expectedPlan` and only `t.Log`s the pretty query plan; with `EnableMultiFetch()` the full pipeline runs (resolved templates, concrete types), making a full-plan golden impractical. Therefore write these tests HAND-ROLLED: build the plan config with `EnableMultiFetch: true` and `DisableIncludeInfo: false`; normalize + validate the operation the way `RunTestWithVariables` does (≈lines 199-229); plan via `plan.NewPlanner(config)` + `p.Plan(&op, &def, opName, &report, plan.IncludeQueryPlanInResponse())` (this populates `Info.QueryPlan`); run `postprocess.NewProcessor(postprocess.EnableMultiFetch()).Process(actualPlan)`; assert on `Response.Fetches.QueryPlan().PrettyPrint()`.

- [ ] **Step 1: Two-fetch merge test** `TestGraphQLDataSourceFederation_MultiFetch`: a federation config where a list field and a single-entity field both extend types from one subgraph at the same wave (the issue's `employees { id products }` + `employee(id: 1) { id notes }` pattern — reuse/adapt an existing federation fixture in this package). Assert on `Response.Fetches.QueryPlan().PrettyPrint()` golden: exactly one Fetch of kind MultiEntity for the products subgraph, whose query contains `f1: _entities(representations: $representations_f1)@include(if: $includeF1)` and `f2: _entities(representations: $representations_f2)@include(if: $includeF2)` (no space before `@` — astprinter directive spacing), plus `mergedFetchIds`. Then the SAME query with `EnableMultiFetch: false` ⇒ two separate entity fetch nodes (golden) — flag-off parity (invariant 5).
- [ ] **Step 2: Wave-separation test**: a query where the second entity fetch depends on the first's output ⇒ no merge (two fetch nodes remain).
- [ ] **Step 3: Three-fetch group + subscription.** (a) Three same-wave entity fetches to one subgraph ⇒ one MultiEntity fetch with `f1`/`f2`/`f3` and `$includeF3`; (b) a subscription whose response tree contains two same-wave entity fetches ⇒ merged (assert via the subscription response's fetch-tree QueryPlan).
- [ ] **Step 4: Resolve integration** `TestLoadGraphQLResponseData_MultiEntity`: hand-built post-postprocess tree — root SingleFetch (stub returns parent data) then a MultiEntityFetch (stub records received input, returns per-alias data). Assert exactly ONE Load call for the multi; the received input equals Task 8's assembly golden; and the final data-buffer JSON is byte-identical to an equivalent UNMERGED run (same tree with the two original Entity/Batch fetches). Build the unmerged comparison tree as a Sequence (serial merges ⇒ deterministic bytes) and keep fixtures targeting disjoint merge paths. Also assert single-flight compatibility: with subgraph request deduplication enabled, `singleFlightAllowed` treats the multi FetchItem as a query (merged `Info.OperationType == ast.OperationTypeQuery`) — two concurrent identical multi loads share one request (mirror the existing single-flight loader test setup).
- [ ] **Step 5: Run everything**: `gotestsum --format=short -- ./pkg/engine/... ./pkg/astimport/...` — all green.

---

### Task 11: lint, fmt, full sweep

- [ ] **Step 1:** `gofmt -l v2/pkg` — expect empty output; fix anything listed.
- [ ] **Step 2:** Run the repo linter: inspect the Makefile locations first (`v2/Makefile`, root `Makefile`) and run the `lint-fix` target from the right directory (the user's requirement is "run a linter with a make lint-fix"). Fix all reported issues in touched files.
- [ ] **Step 3:** Full test sweep from `v2/`: `gotestsum --format=short -- ./pkg/... -count=1` (long; run once). All green, including pre-existing tests.
- [ ] **Step 4:** Re-read every new/modified comment against the Global Constraints comment rules; delete narration.
