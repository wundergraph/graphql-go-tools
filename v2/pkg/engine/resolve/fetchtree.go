package resolve

import (
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
	Kind     string                 `json:"kind"`
	Path     string                 `json:"path"`
	SourceID string                 `json:"source_id"`
	Trace    *DataSourceLoadTrace   `json:"trace,omitempty"`
	Traces   []*DataSourceLoadTrace `json:"traces,omitempty"`
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
				Kind:     "Single",
				SourceID: f.Info.DataSourceID,
				Trace:    f.Trace,
				Path:     n.Item.ResponsePath,
			}
		case *EntityFetch:
			trace.Fetch = &FetchTraceNode{
				Kind:     "Entity",
				SourceID: f.Info.DataSourceID,
				Trace:    f.Trace,
				Path:     n.Item.ResponsePath,
			}
		case *BatchEntityFetch:
			trace.Fetch = &FetchTraceNode{
				Kind:     "BatchEntity",
				SourceID: f.Info.DataSourceID,
				Trace:    f.Trace,
				Path:     n.Item.ResponsePath,
			}
		case *ParallelListItemFetch:
			trace.Fetch = &FetchTraceNode{
				Kind:     "ParallelList",
				SourceID: f.Fetch.Info.DataSourceID,
				Traces:   make([]*DataSourceLoadTrace, len(f.Traces)),
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
