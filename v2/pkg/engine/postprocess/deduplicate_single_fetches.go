package postprocess

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// deduplicateSingleFetches is a post-processing step that removes duplicate single fetches
// from the initial fetch tree. It merges their fetch paths and updates dependencies accordingly.
// NOTE: initial tree structure should be flat and contain a single root item with all fetches as children.
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

				newId := root.ChildNodes[i].Item.Fetch.Dependencies().FetchID
				oldId := root.ChildNodes[j].Item.Fetch.Dependencies().FetchID

				root.ChildNodes = append(root.ChildNodes[:j], root.ChildNodes[j+1:]...)
				j--

				// when we merge duplicated fetches, we need to update the dependencies of the other fetches
				// because they might depend on the fetch that we are removing
				d.replaceDependsOnFetchId(root, oldId, newId)
			}
		}
	}
}

// replaceDependsOnFetchId replaces all occurrences of oldId with newId in the dependencies of the fetch tree.
func (d *deduplicateSingleFetches) replaceDependsOnFetchId(root *resolve.FetchTreeNode, oldId, newId int) {
	for i := range root.ChildNodes {
		replaced := false
		for j := range root.ChildNodes[i].Item.Fetch.Dependencies().DependsOnFetchIDs {
			if root.ChildNodes[i].Item.Fetch.Dependencies().DependsOnFetchIDs[j] == oldId {
				root.ChildNodes[i].Item.Fetch.Dependencies().DependsOnFetchIDs[j] = newId
				replaced = true
			}
		}

		if !replaced {
			continue
		}

		for j := range root.ChildNodes[i].Item.Fetch.DependenciesCoordinates() {
			for k := range root.ChildNodes[i].Item.Fetch.DependenciesCoordinates()[j].DependsOn {
				if root.ChildNodes[i].Item.Fetch.DependenciesCoordinates()[j].DependsOn[k].FetchID == oldId {
					root.ChildNodes[i].Item.Fetch.DependenciesCoordinates()[j].DependsOn[k].FetchID = newId
				}
			}
		}
	}
}

// mergeFetchPath merges the fetch paths of two single fetches.
// The goal of this method is to merge typename conditions.
// When fetches originate from different parent fragments -
// they will have different typenames, but the same path in response.
func (d *deduplicateSingleFetches) mergeFetchPath(left, right []resolve.FetchItemPathElement) []resolve.FetchItemPathElement {
	for i := range left {
		left[i].TypeNames = d.mergeTypeNames(left[i].TypeNames, right[i].TypeNames)
	}

	return left
}

func (d *deduplicateSingleFetches) mergeTypeNames(left []string, right []string) []string {
	if len(left) == 0 || len(right) == 0 {
		return nil // if either side is empty, fetch is unscoped
	}

	out := append(left, right...)

	slices.Sort(out)
	return slices.Compact(out) // removes consecutive duplicates from the sorted slice
}
