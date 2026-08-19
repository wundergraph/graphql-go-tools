package resolve

import (
	"fmt"
	"strings"
)

type FetchTreeNode struct {
	Kind FetchTreeNodeKind `json:"kind"`
	// Trigger is only set for subscription
	Trigger         *FetchTreeNode   `json:"trigger"`
	Item            *FetchItem       `json:"item"`
	ChildNodes      []*FetchTreeNode `json:"child_nodes"`
	NormalizedQuery string           `json:"normalized_query"`

	// deferMetadata is set only on synthetic sequence wrappers in a composite
	// defer execution tree. It is intentionally excluded from direct FetchTreeNode
	// JSON; QueryPlan and Trace expose their own stable representations.
	deferMetadata *fetchTreeDeferMetadata
}

type FetchTreeNodeKind string

const (
	FetchTreeNodeKindSingle   FetchTreeNodeKind = "Single"
	FetchTreeNodeKindSequence FetchTreeNodeKind = "Sequence"
	FetchTreeNodeKindParallel FetchTreeNodeKind = "Parallel"
	FetchTreeNodeKindTrigger  FetchTreeNodeKind = "Trigger"
)

func Sequence(children ...*FetchTreeNode) *FetchTreeNode {
	return &FetchTreeNode{
		Kind:       FetchTreeNodeKindSequence,
		ChildNodes: children,
	}
}

func Parallel(children ...*FetchTreeNode) *FetchTreeNode {
	return &FetchTreeNode{
		Kind:       FetchTreeNodeKindParallel,
		ChildNodes: children,
	}
}

func ObjectPath(path ...string) FetchItemPathElement {
	return FetchItemPathElement{
		Kind: FetchItemPathElementKindObject,
		Path: path,
	}
}

func PathElementWithTypeNames(element FetchItemPathElement, typeNames []string) FetchItemPathElement {
	element.TypeNames = typeNames
	return element
}

func ArrayPath(path ...string) FetchItemPathElement {
	return FetchItemPathElement{
		Kind: FetchItemPathElementKindArray,
		Path: path,
	}
}

func Single(fetch Fetch, path ...FetchItemPathElement) *FetchTreeNode {
	return &FetchTreeNode{
		Kind: FetchTreeNodeKindSingle,
		Item: &FetchItem{
			Fetch:     fetch,
			FetchPath: path,
		},
	}
}

func SingleWithPath(fetch Fetch, responsePath string, path ...FetchItemPathElement) *FetchTreeNode {
	node := &FetchTreeNode{
		Kind: FetchTreeNodeKindSingle,
		Item: &FetchItem{
			Fetch:        fetch,
			FetchPath:    path,
			ResponsePath: responsePath,
		},
	}
	if responsePath != "" {
		node.Item.ResponsePathElements = strings.Split(responsePath, ".")
	}
	return node
}

type FetchTreeTraceNode struct {
	Kind     FetchTreeNodeKind     `json:"kind"`
	Children []*FetchTreeTraceNode `json:"children,omitempty"`
	Fetch    *FetchTraceNode       `json:"fetch,omitempty"`
	Defer    *FetchTreeDeferTrace  `json:"defer,omitempty"`
}

// FetchTreeDeferTrace describes the deferred fragment represented by a trace
// wrapper. Status is populated only by a request-local execution trace tree.
type FetchTreeDeferTrace struct {
	ID     int                  `json:"id"`
	Label  string               `json:"label"`
	Path   []string             `json:"path"`
	Status DeferExecutionStatus `json:"status,omitempty"`
}

type FetchTraceNode struct {
	Kind       string                 `json:"kind"`
	Path       string                 `json:"path"`
	SourceID   string                 `json:"source_id"`
	SourceName string                 `json:"source_name"`
	Trace      *DataSourceLoadTrace   `json:"trace,omitempty"`
	Traces     []*DataSourceLoadTrace `json:"traces,omitempty"`
	Entries    []FetchTraceEntry      `json:"entries,omitempty"`
}

type FetchTraceEntry struct {
	Alias string `json:"alias"`
	Path  string `json:"path"`
}

func (n *FetchTreeNode) Trace() *FetchTreeTraceNode {
	return n.trace(false)
}

func (n *FetchTreeNode) trace(suppressLoadTrace bool) *FetchTreeTraceNode {
	if n == nil {
		return nil
	}
	trace := &FetchTreeTraceNode{
		Kind: n.Kind,
	}
	if n.deferMetadata != nil {
		status := n.deferMetadata.executionStatus()
		trace.Defer = &FetchTreeDeferTrace{
			ID:     n.deferMetadata.descriptor.ID,
			Label:  n.deferMetadata.descriptor.Label,
			Path:   append([]string{}, n.deferMetadata.descriptor.Path...),
			Status: status,
		}
		// A request-local skipped branch may still point at a cached fetch node
		// whose Trace field was populated by an earlier execution. Preserve the
		// planned fetch metadata, but never present stale timing as work performed
		// by this request.
		if status == DeferExecutionStatusSkipped {
			suppressLoadTrace = true
		}
	}
	switch n.Kind {
	case FetchTreeNodeKindSingle:
		switch f := n.Item.Fetch.(type) {
		case *SingleFetch:
			loadTrace := f.Trace
			if suppressLoadTrace {
				loadTrace = nil
			}
			trace.Fetch = &FetchTraceNode{
				Kind:       "Single",
				SourceID:   f.Info.DataSourceID,
				SourceName: f.Info.DataSourceName,
				Trace:      loadTrace,
				Path:       n.Item.ResponsePath,
			}
		case *EntityFetch:
			loadTrace := f.Trace
			if suppressLoadTrace {
				loadTrace = nil
			}
			trace.Fetch = &FetchTraceNode{
				Kind:       "Entity",
				SourceID:   f.Info.DataSourceID,
				SourceName: f.Info.DataSourceName,
				Trace:      loadTrace,
				Path:       n.Item.ResponsePath,
			}
		case *BatchEntityFetch:
			loadTrace := f.Trace
			if suppressLoadTrace {
				loadTrace = nil
			}
			trace.Fetch = &FetchTraceNode{
				Kind:       "BatchEntity",
				SourceID:   f.Info.DataSourceID,
				SourceName: f.Info.DataSourceName,
				Trace:      loadTrace,
				Path:       n.Item.ResponsePath,
			}
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
		default:
		}
	case FetchTreeNodeKindSequence, FetchTreeNodeKindParallel:
		trace.Children = make([]*FetchTreeTraceNode, len(n.ChildNodes))
		for i, c := range n.ChildNodes {
			trace.Children[i] = c.trace(suppressLoadTrace)
		}
	}
	return trace
}

type FetchTreeQueryPlanNode struct {
	Version         string                    `json:"version,omitempty"`
	Kind            FetchTreeNodeKind         `json:"kind"`
	Trigger         *FetchTreeQueryPlan       `json:"trigger,omitempty"`
	Children        []*FetchTreeQueryPlanNode `json:"children,omitempty"`
	Fetch           *FetchTreeQueryPlan       `json:"fetch,omitempty"`
	NormalizedQuery string                    `json:"normalizedQuery,omitempty"`
	Defer           *FetchTreeDeferDescriptor `json:"defer,omitempty"`
}

type FetchTreeQueryPlan struct {
	Kind              string            `json:"kind"`
	Path              string            `json:"path,omitempty"`
	SubgraphName      string            `json:"subgraphName"`
	SubgraphID        string            `json:"subgraphId"`
	FetchID           int               `json:"fetchId"`
	DependsOnFetchIDs []int             `json:"dependsOnFetchIds,omitempty"`
	Representations   []Representation  `json:"representations,omitempty"`
	Query             string            `json:"query,omitempty"`
	Dependencies      []FetchDependency `json:"dependencies,omitempty"`
	MergedFetchIDs    []int             `json:"mergedFetchIds,omitempty"`
	Entries           []QueryPlanEntry  `json:"entries,omitempty"`
}

type QueryPlanEntry struct {
	Alias string `json:"alias"`
	Path  string `json:"path,omitempty"`
	// Representations are the representations of this entry alone. The enclosing
	// FetchTreeQueryPlan.Representations holds the merged representations of all entries.
	Representations []Representation `json:"representations,omitempty"`
}

func (n *FetchTreeNode) QueryPlan() *FetchTreeQueryPlanNode {
	if n == nil {
		return nil
	}

	plan := n.queryPlan()
	plan.Version = "1"

	if n.Trigger != nil && n.Trigger.Item != nil && n.Trigger.Item.Fetch != nil {
		if f, ok := n.Trigger.Item.Fetch.(*SingleFetch); ok {
			plan.Trigger = &FetchTreeQueryPlan{
				Kind:         "Trigger",
				Path:         n.Trigger.Item.ResponsePath,
				SubgraphName: f.Info.DataSourceName,
				SubgraphID:   f.Info.DataSourceID,
				FetchID:      f.FetchDependencies.FetchID,
				Query:        f.Info.QueryPlan.Query,
			}
		}
	}

	return plan
}

func (n *FetchTreeNode) queryPlan() *FetchTreeQueryPlanNode {
	if n == nil {
		return nil
	}
	queryPlan := &FetchTreeQueryPlanNode{
		Kind:            n.Kind,
		NormalizedQuery: n.NormalizedQuery,
	}
	if n.deferMetadata != nil {
		descriptor := n.deferMetadata.descriptor
		descriptor.Path = append([]string{}, descriptor.Path...)
		queryPlan.Defer = &descriptor
	}
	switch n.Kind {
	case FetchTreeNodeKindSingle:
		switch f := n.Item.Fetch.(type) {
		case *SingleFetch:
			queryPlan.Fetch = &FetchTreeQueryPlan{
				Kind:              "Single",
				FetchID:           f.FetchDependencies.FetchID,
				DependsOnFetchIDs: f.FetchDependencies.DependsOnFetchIDs,
				SubgraphName:      f.Info.DataSourceName,
				SubgraphID:        f.Info.DataSourceID,
				Path:              n.Item.ResponsePath,
				Dependencies:      f.Info.CoordinateDependencies,
			}

			if f.Info.QueryPlan != nil {
				queryPlan.Fetch.Query = f.Info.QueryPlan.Query
				queryPlan.Fetch.Representations = f.Info.QueryPlan.DependsOnFields
			}
		case *EntityFetch:
			queryPlan.Fetch = &FetchTreeQueryPlan{
				Kind:              "Entity",
				FetchID:           f.FetchDependencies.FetchID,
				DependsOnFetchIDs: f.FetchDependencies.DependsOnFetchIDs,
				SubgraphName:      f.Info.DataSourceName,
				SubgraphID:        f.Info.DataSourceID,
				Path:              n.Item.ResponsePath,
				Dependencies:      f.Info.CoordinateDependencies,
			}

			if f.Info.QueryPlan != nil {
				queryPlan.Fetch.Query = f.Info.QueryPlan.Query
				queryPlan.Fetch.Representations = f.Info.QueryPlan.DependsOnFields
			}
		case *BatchEntityFetch:
			queryPlan.Fetch = &FetchTreeQueryPlan{
				Kind:              "BatchEntity",
				FetchID:           f.FetchDependencies.FetchID,
				DependsOnFetchIDs: f.FetchDependencies.DependsOnFetchIDs,
				SubgraphName:      f.Info.DataSourceName,
				SubgraphID:        f.Info.DataSourceID,
				Path:              n.Item.ResponsePath,
				Dependencies:      f.Info.CoordinateDependencies,
			}

			if f.Info.QueryPlan != nil {
				queryPlan.Fetch.Query = f.Info.QueryPlan.Query
				queryPlan.Fetch.Representations = f.Info.QueryPlan.DependsOnFields
			}
		case *MultiEntityFetch:
			entries := make([]QueryPlanEntry, len(f.Input.Entries))
			for i, e := range f.Input.Entries {
				entries[i] = QueryPlanEntry{Alias: e.Alias, Path: e.Item.ResponsePath}
				if e.Info != nil && e.Info.QueryPlan != nil {
					entries[i].Representations = e.Info.QueryPlan.DependsOnFields
				}
			}
			queryPlan.Fetch = &FetchTreeQueryPlan{
				Kind:              "MultiEntity",
				FetchID:           f.FetchDependencies.FetchID,
				DependsOnFetchIDs: f.FetchDependencies.DependsOnFetchIDs,
				SubgraphName:      f.Info.DataSourceName,
				SubgraphID:        f.Info.DataSourceID,
				Path:              n.Item.ResponsePath,
				Dependencies:      f.Info.CoordinateDependencies,
				MergedFetchIDs:    f.MergedFetchIDs,
				Entries:           entries,
			}

			if f.Info.QueryPlan != nil {
				queryPlan.Fetch.Query = f.Info.QueryPlan.Query
				queryPlan.Fetch.Representations = f.Info.QueryPlan.DependsOnFields
			}
		default:
		}
	case FetchTreeNodeKindSequence, FetchTreeNodeKindParallel:
		queryPlan.Children = make([]*FetchTreeQueryPlanNode, len(n.ChildNodes))
		for i, c := range n.ChildNodes {
			queryPlan.Children[i] = c.queryPlan()
		}
	}
	return queryPlan
}

func (n *FetchTreeQueryPlanNode) PrettyPrint() string {
	if n == nil {
		return ""
	}
	printer := PlanPrinter{}
	return printer.Print(n)
}

type PlanPrinter struct {
	depth int
	buf   strings.Builder
}

func (p *PlanPrinter) Print(plan *FetchTreeQueryPlanNode) string {
	p.buf.Reset()

	p.print("QueryPlan {")
	p.printPlanNode(plan, true)
	p.print("}")

	return p.buf.String()
}

func (p *PlanPrinter) printPlanNode(plan *FetchTreeQueryPlanNode, increaseDepth bool) {
	if plan == nil {
		return
	}
	if increaseDepth {
		p.depth++
	}
	if plan.Defer != nil {
		if plan.Defer.Label == "" {
			p.print("Defer {")
		} else {
			p.print(fmt.Sprintf("Defer(label: %q) {", plan.Defer.Label))
		}
		p.depth++
	}
	switch plan.Kind {
	case FetchTreeNodeKindSingle:
		p.printFetchInfo(plan.Fetch)
	case FetchTreeNodeKindSequence:
		isSubscription := plan.Trigger != nil
		hasChildren := len(plan.Children) > 0
		// Special case for Subscriptions:
		// "Primary" key has plan.Trigger. "Rest" has plan.Children
		if isSubscription {
			p.print("Subscription {")
			p.depth++
			p.print("Primary: {")
			p.depth++
			p.printFetchInfo(plan.Trigger)
			p.depth--
			p.print("},") // Primary
			if hasChildren {
				p.print("Rest: {")
				p.depth++
			}
		}

		isSequence := len(plan.Children) > 1
		if isSequence {
			p.print("Sequence {")
		}
		for _, child := range plan.Children {
			p.printPlanNode(child, isSequence)
		}
		if isSequence {
			p.print("}")
		}

		if isSubscription {
			if hasChildren {
				p.depth--
				p.print("},") // Rest
			}
			p.depth--
			p.print("}") // Subscription
		}
	case FetchTreeNodeKindParallel:
		p.print("Parallel {")
		for _, child := range plan.Children {
			p.printPlanNode(child, true)
		}
		p.print("}")
	}
	if plan.Defer != nil {
		p.depth--
		p.print("}")
	}
	if increaseDepth {
		p.depth--
	}
}

func (p *PlanPrinter) printFetchInfo(fetch *FetchTreeQueryPlan) {
	nested := strings.Contains(fetch.Path, ".")

	if nested {
		p.print(fmt.Sprintf(`Flatten(path: "%s") {`, fetch.Path))
		p.depth++
	}
	p.print(fmt.Sprintf(`Fetch(service: "%s") {`, fetch.SubgraphName))
	p.depth++

	switch {
	case fetch.Kind == "MultiEntity" && hasEntryRepresentations(fetch.Entries):
		// For a merged multi-entity fetch the top level representations are the concatenation of all
		// entries, which makes the fragments indistinguishable. Attribute them to their entry instead.
		p.printEntryRepresentations(fetch.Entries)
	case fetch.Representations != nil:
		p.printRepresentations(fetch.Representations)
	}
	p.printQuery(fetch.Query)

	p.depth--
	p.print("}")
	if nested {
		p.depth--
		p.print("}")
	}
}

// printQuery replaces the first line of a query with "{" and prints into p.
// It expects a multi-line formatted query. As a fallback for a single-line query,
// it will print such a query as it is.
func (p *PlanPrinter) printQuery(query string) {
	if query == "" {
		return
	}
	lines := strings.Split(query, "\n")
	if len(lines) == 1 {
		p.print(query)
		return
	}
	lines[0] = "{"
	lines[len(lines)-1] = "}"
	p.print(lines...)
}

func (p *PlanPrinter) printRepresentations(reps []Representation) {
	p.print("{")
	p.depth++
	for _, rep := range reps {
		lines := strings.Split(rep.Fragment, "\n")
		p.print(lines...)
	}
	p.depth--
	p.print("} =>")
}

// hasEntryRepresentations reports whether at least one entry carries its own representations.
func hasEntryRepresentations(entries []QueryPlanEntry) bool {
	for _, entry := range entries {
		if len(entry.Representations) > 0 {
			return true
		}
	}
	return false
}

// printEntryRepresentations prints the representations of a multi-entity fetch as named fragments,
// one per representation, named after the aliased sub-fetch they belong to, e.g.
//
//	fragment f1_Key on Product {
//	    __typename
//	    upc
//	}
//	=>
//
// The "=>" separator is kept so the overall "representations => query" shape is preserved.
func (p *PlanPrinter) printEntryRepresentations(entries []QueryPlanEntry) {
	for _, entry := range entries {
		seen := make(map[string]int, len(entry.Representations))
		for _, rep := range entry.Representations {
			name := representationKindName(rep.Kind)
			seen[name]++
			if count := seen[name]; count > 1 {
				name = fmt.Sprintf("%s%d", name, count)
			}
			// Reuse the brace block of the inline fragment verbatim to keep its own indentation.
			block := "{}"
			if idx := strings.Index(rep.Fragment, "{"); idx != -1 {
				block = rep.Fragment[idx:]
			}
			lines := strings.Split(block, "\n")
			lines[0] = fmt.Sprintf("fragment %s_%s on %s %s", entry.Alias, name, rep.TypeName, lines[0])
			p.print(lines...)
		}
	}
	p.print("=>")
}

// representationKindName turns a RepresentationKind into a fragment name segment by stripping all
// non-alphanumeric characters and capitalizing the first letter, e.g. "@key" becomes "Key".
func representationKindName(kind RepresentationKind) string {
	var sb strings.Builder
	for _, r := range string(kind) {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		}
	}
	name := sb.String()
	if name == "" {
		return "Representation"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func (p *PlanPrinter) print(lines ...string) {
	for _, l := range lines {
		fmt.Fprintf(&p.buf, "%s%s\n", strings.Repeat("  ", p.depth), l)
	}
}
