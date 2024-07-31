package resolve

import (
	"strings"
)

type FetchTreeNode struct {
	Kind          FetchTreeNodeKind
	Item          *FetchItem
	SerialNodes   []*FetchTreeNode
	ParallelNodes []*FetchTreeNode
}

type FetchTreeNodeKind string

const (
	FetchTreeNodeKindSingle   FetchTreeNodeKind = "Single"
	FetchTreeNodeKindSequence FetchTreeNodeKind = "Sequence"
	FetchTreeNodeKindParallel FetchTreeNodeKind = "Parallel"
)

func Sequence(children ...*FetchTreeNode) *FetchTreeNode {
	return &FetchTreeNode{
		Kind:        FetchTreeNodeKindSequence,
		SerialNodes: children,
	}
}

func Parallel(children ...*FetchTreeNode) *FetchTreeNode {
	return &FetchTreeNode{
		Kind:          FetchTreeNodeKindParallel,
		ParallelNodes: children,
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
