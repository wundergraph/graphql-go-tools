package postprocess

import (
	"fmt"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// fetchIDAppender is a processor to append fetchIDs to the operation names propagated downstream.
type fetchIDAppender struct {
	disable bool
}

func (f *fetchIDAppender) ProcessFetchTree(root *resolve.FetchTreeNode) {
	if f.disable {
		return
	}
	f.traverseNode(root)
}

func (f *fetchIDAppender) traverseNode(node *resolve.FetchTreeNode) {
	if node == nil {
		return
	}
	switch node.Kind {
	case resolve.FetchTreeNodeKindSingle:
		if singleFetch, ok := node.Item.Fetch.(*resolve.SingleFetch); ok {
			f.traverseSingleFetch(singleFetch)
		}
	case resolve.FetchTreeNodeKindParallel, resolve.FetchTreeNodeKindSequence:
		for i := range node.ChildNodes {
			f.traverseNode(node.ChildNodes[i])
		}
	}
}

func (f *fetchIDAppender) traverseSingleFetch(fetch *resolve.SingleFetch) {
	if fetch.OperationName != "" {
		expandedName := fmt.Sprintf("%s__%d", fetch.OperationName, fetch.FetchID)
		fetch.Input = strings.Replace(fetch.Input, fetch.OperationName, expandedName, 1)
		if fetch.QueryPlan != nil {
			// Needed for debugging of query plans
			fetch.QueryPlan.Query = strings.Replace(fetch.QueryPlan.Query, fetch.OperationName, expandedName, 1)
		}
	}
}
