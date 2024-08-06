package postprocess

import (
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
				root.ChildNodes = append(root.ChildNodes[:j], root.ChildNodes[j+1:]...)
				j--
			}
		}
	}
}

func (d *deduplicateSingleFetches) samePath(a, b *resolve.FetchItem) bool {
	if len(a.FetchPath) != len(b.FetchPath) {
		return false
	}
	for i := range a.FetchPath {
		if a.FetchPath[i].Kind != b.FetchPath[i].Kind {
			return false
		}
		if len(a.FetchPath[i].Path) != len(b.FetchPath[i].Path) {
			return false
		}
		for j := range a.FetchPath[i].Path {
			if a.FetchPath[i].Path[j] != b.FetchPath[i].Path[j] {
				return false
			}
		}
	}
	return true
}
