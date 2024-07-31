package postprocess

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type deduplicateSingleFetches struct {
}

func (d *deduplicateSingleFetches) ProcessFetchTree(root *resolve.FetchTreeNode) {
	for i := range root.SerialNodes {
		for j := i + 1; j < len(root.SerialNodes); j++ {
			if root.SerialNodes[i].Item.Equals(root.SerialNodes[j].Item) {
				root.SerialNodes = append(root.SerialNodes[:j], root.SerialNodes[j+1:]...)
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
