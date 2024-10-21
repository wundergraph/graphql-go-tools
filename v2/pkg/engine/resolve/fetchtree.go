package resolve

import (
	"fmt"
	"strings"
)

type FetchTreeNode struct {
	Kind       FetchTreeNodeKind `json:"kind"`
	Item       *FetchItem        `json:"item"`
	ChildNodes []*FetchTreeNode  `json:"child_nodes"`
}

type FetchTreeNodeKind string

const (
	FetchTreeNodeKindSingle   FetchTreeNodeKind = "Single"
	FetchTreeNodeKindSequence FetchTreeNodeKind = "Sequence"
	FetchTreeNodeKindParallel FetchTreeNodeKind = "Parallel"
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
}

type FetchTraceNode struct {
	Kind       string                 `json:"kind"`
	Path       string                 `json:"path"`
	SourceID   string                 `json:"source_id"`
	SourceName string                 `json:"source_name"`
	Trace      *DataSourceLoadTrace   `json:"trace,omitempty"`
	Traces     []*DataSourceLoadTrace `json:"traces,omitempty"`
}

func (n *FetchTreeNode) Trace() *FetchTreeTraceNode {
	if n == nil {
		return nil
	}
	trace := &FetchTreeTraceNode{
		Kind: n.Kind,
	}
	switch n.Kind {
	case FetchTreeNodeKindSingle:
		switch f := n.Item.Fetch.(type) {
		case *SingleFetch:
			trace.Fetch = &FetchTraceNode{
				Kind:       "Single",
				SourceID:   f.Info.DataSourceID,
				SourceName: f.Info.DataSourceName,
				Trace:      f.Trace,
				Path:       n.Item.ResponsePath,
			}
		case *EntityFetch:
			trace.Fetch = &FetchTraceNode{
				Kind:       "Entity",
				SourceID:   f.Info.DataSourceID,
				SourceName: f.Info.DataSourceName,
				Trace:      f.Trace,
				Path:       n.Item.ResponsePath,
			}
		case *BatchEntityFetch:
			trace.Fetch = &FetchTraceNode{
				Kind:       "BatchEntity",
				SourceID:   f.Info.DataSourceID,
				SourceName: f.Info.DataSourceName,
				Trace:      f.Trace,
				Path:       n.Item.ResponsePath,
			}
		case *ParallelListItemFetch:
			trace.Fetch = &FetchTraceNode{
				Kind:       "ParallelList",
				SourceID:   f.Fetch.Info.DataSourceID,
				SourceName: f.Fetch.Info.DataSourceName,
				Traces:     make([]*DataSourceLoadTrace, len(f.Traces)),
				Path:       n.Item.ResponsePath,
			}
			for i, t := range f.Traces {
				trace.Fetch.Traces[i] = t.Trace
			}
		default:
		}
	case FetchTreeNodeKindSequence, FetchTreeNodeKindParallel:
		trace.Children = make([]*FetchTreeTraceNode, len(n.ChildNodes))
		for i, c := range n.ChildNodes {
			trace.Children[i] = c.Trace()
		}
	}
	return trace
}

type FetchTreeQueryPlanNode struct {
	Version  string                    `json:"version,omitempty"`
	Kind     FetchTreeNodeKind         `json:"kind"`
	Children []*FetchTreeQueryPlanNode `json:"children,omitempty"`
	Fetch    *FetchTreeQueryPlan       `json:"fetch,omitempty"`
}

type FetchTreeQueryPlan struct {
	Kind              string           `json:"kind"`
	Path              string           `json:"path,omitempty"`
	SubgraphName      string           `json:"subgraphName"`
	SubgraphID        string           `json:"subgraphId"`
	FetchID           int              `json:"fetchId"`
	DependsOnFetchIDs []int            `json:"dependsOnFetchIds,omitempty"`
	Representations   []Representation `json:"representations,omitempty"`
	Query             string           `json:"query,omitempty"`
}

func (n *FetchTreeNode) QueryPlan() *FetchTreeQueryPlanNode {
	if n == nil {
		return nil
	}
	plan := n.queryPlan()
	plan.Version = "1"
	return plan
}

func (n *FetchTreeNode) queryPlan() *FetchTreeQueryPlanNode {
	if n == nil {
		return nil
	}
	queryPlan := &FetchTreeQueryPlanNode{
		Kind: n.Kind,
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
				Query:             f.Info.QueryPlan.Query,
				Representations:   f.Info.QueryPlan.DependsOnFields,
				Path:              n.Item.ResponsePath,
			}
		case *EntityFetch:
			queryPlan.Fetch = &FetchTreeQueryPlan{
				Kind:              "Entity",
				FetchID:           f.FetchDependencies.FetchID,
				DependsOnFetchIDs: f.FetchDependencies.DependsOnFetchIDs,
				SubgraphName:      f.Info.DataSourceName,
				SubgraphID:        f.Info.DataSourceID,
				Query:             f.Info.QueryPlan.Query,
				Representations:   f.Info.QueryPlan.DependsOnFields,
				Path:              n.Item.ResponsePath,
			}
		case *BatchEntityFetch:
			queryPlan.Fetch = &FetchTreeQueryPlan{
				Kind:              "BatchEntity",
				FetchID:           f.FetchDependencies.FetchID,
				DependsOnFetchIDs: f.FetchDependencies.DependsOnFetchIDs,
				SubgraphName:      f.Info.DataSourceName,
				SubgraphID:        f.Info.DataSourceID,
				Query:             f.Info.QueryPlan.Query,
				Representations:   f.Info.QueryPlan.DependsOnFields,
				Path:              n.Item.ResponsePath,
			}
		case *ParallelListItemFetch:
			queryPlan.Fetch = &FetchTreeQueryPlan{
				Kind:              "ParallelList",
				FetchID:           f.Fetch.FetchDependencies.FetchID,
				DependsOnFetchIDs: f.Fetch.FetchDependencies.DependsOnFetchIDs,
				SubgraphName:      f.Fetch.Info.DataSourceName,
				SubgraphID:        f.Fetch.Info.DataSourceID,
				Query:             f.Fetch.Info.QueryPlan.Query,
				Representations:   f.Fetch.Info.QueryPlan.DependsOnFields,
				Path:              n.Item.ResponsePath,
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
	if increaseDepth {
		p.depth++
	}
	switch plan.Kind {
	case FetchTreeNodeKindSingle:
		p.printFetchInfo(plan.Fetch)
	case FetchTreeNodeKindSequence:
		manyChilds := len(plan.Children) > 1
		if manyChilds {
			p.print("Sequence {")
		}
		for _, child := range plan.Children {
			p.printPlanNode(child, manyChilds)
		}
		if manyChilds {
			p.print("}")
		}
	case FetchTreeNodeKindParallel:
		p.print("Parallel {")
		for _, child := range plan.Children {
			p.printPlanNode(child, true)
		}
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

	if fetch.Representations != nil {
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

func (p *PlanPrinter) printQuery(query string) {
	lines := strings.Split(query, "\n")
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

func (p *PlanPrinter) print(lines ...string) {
	for _, l := range lines {
		p.buf.WriteString(fmt.Sprintf("%s%s\n", strings.Repeat("  ", p.depth), l))
	}
}
