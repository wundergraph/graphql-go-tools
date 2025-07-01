package postprocess

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type deduplicateSingleFetches struct {
	disable bool
}

func (d *deduplicateSingleFetches) ProcessFetchTree(root *resolve.FetchTreeNode) {
	if d.disable {
		return
	}
	for i := range root.ChildNodes {
		for j := i + 1; j < len(root.ChildNodes); j++ {
			if root.ChildNodes[i].Item.Equals(root.ChildNodes[j].Item) {
				root.ChildNodes[i].Item.FetchPath = d.mergeFetchPath(root.ChildNodes[i].Item.FetchPath, root.ChildNodes[j].Item.FetchPath)

				root.ChildNodes = append(root.ChildNodes[:j], root.ChildNodes[j+1:]...)
				j--
			}
		}
	}
}

func (d *deduplicateSingleFetches) mergeFetchPath(left, right []resolve.FetchItemPathElement) []resolve.FetchItemPathElement {
	for i := range left {
		left[i].TypeNames = d.mergeTypeNames(left[i].TypeNames, right[i].TypeNames)
	}

	return left
}

func (d *deduplicateSingleFetches) mergeTypeNames(left []string, right []string) []string {
	if len(left) == 0 {
		return left
	}
	if len(right) == 0 {
		return right
	}

	out := append(left, right...)

	slices.Sort(out)
	slices.Compact(out) // deduplicate

	return out
}
