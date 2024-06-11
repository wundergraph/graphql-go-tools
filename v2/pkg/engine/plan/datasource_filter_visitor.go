package plan

import (
	"github.com/kingledion/go-tools/tree"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

const typeNameField = "__typename"

var typeNameFieldBytes = []byte(typeNameField)

type DataSourceFilter struct {
	operation  *ast.Document
	definition *ast.Document
	report     *operationreport.Report

	nodes *NodeSuggestions

	enableSelectionReasons bool
}

func NewDataSourceFilter(operation, definition *ast.Document, report *operationreport.Report) *DataSourceFilter {
	return &DataSourceFilter{
		operation:  operation,
		definition: definition,
		report:     report,
	}
}

func (f *DataSourceFilter) EnableSelectionReasons() {
	f.enableSelectionReasons = true
}

func (f *DataSourceFilter) FilterDataSources(dataSources []DataSource, existingNodes *NodeSuggestions, keyHints ...NodeSuggestionHint) (used []DataSource, suggestions *NodeSuggestions) {
	var dsInUse map[DSHash]struct{}

	suggestions, dsInUse = f.findBestDataSourceSet(dataSources, existingNodes, keyHints...)
	if f.report.HasErrors() {
		return
	}

	used = make([]DataSource, 0, len(dsInUse))
	for i := range dataSources {
		_, inUse := dsInUse[dataSources[i].Hash()]
		if inUse {
			used = append(used, dataSources[i])
		}
	}

	return used, suggestions
}

func (f *DataSourceFilter) findBestDataSourceSet(dataSources []DataSource, existingNodes *NodeSuggestions, keyHints ...NodeSuggestionHint) (*NodeSuggestions, map[DSHash]struct{}) {
	f.nodes = f.collectNodes(dataSources, existingNodes)
	if f.report.HasErrors() {
		return nil, nil
	}

	// f.nodes.printNodes("initial nodes")

	f.applySuggestionHints(keyHints)
	// f.nodes.printNodes("nodes after applying hints")

	f.selectUniqueNodes()
	// f.nodes.printNodes("unique nodes")
	f.selectDuplicateNodes(false)
	// f.nodes.printNodes("duplicate nodes")
	f.selectDuplicateNodes(true)
	// f.nodes.printNodes("duplicate nodes after second run")

	uniqueDataSourceHashes := f.nodes.populateHasSuggestions()
	return f.nodes, uniqueDataSourceHashes
}

func (f *DataSourceFilter) collectNodes(dataSources []DataSource, existingNodes *NodeSuggestions, hints ...NodeSuggestionHint) (nodes *NodeSuggestions) {
	if existingNodes == nil {
		existingNodes = NewNodeSuggestions()
	}

	nodesCollector := &nodesCollector{
		operation:   f.operation,
		definition:  f.definition,
		dataSources: dataSources,
		nodes:       existingNodes,
		report:      f.report,
	}

	return nodesCollector.CollectNodes()
}

const (
	ReasonStage1Unique                = "stage1: unique"
	ReasonStage1SameSourceParent      = "stage1: same source parent of unique node"
	ReasonStage1SameSourceLeafChild   = "stage1: same source leaf child of unique node"
	ReasonStage1SameSourceLeafSibling = "stage1: same source leaf sibling of unique node"

	ReasonStage2SameSourceNodeOfSelectedParent  = "stage2: node on the same source as selected parent"
	ReasonStage2SameSourceNodeOfSelectedChild   = "stage2: node on the same source as selected child"
	ReasonStage2SameSourceNodeOfSelectedSibling = "stage2: node on the same source as selected sibling"

	ReasonStage3SelectAvailableLeafNode                        = "stage3: select first available leaf node"
	ReasonStage3SelectNodeHavingPossibleChildsOnSameDataSource = "stage3: select non leaf node which have possible child selections on the same source"
	ReasonStage3SelectRootNodeWithEnabledEntityResolver        = "stage3: first available node with enabled entity resolver"
	ReasonStage3SelectFirstParentRootNode                      = "stage3: first available parent node with enabled entity resolver"

	ReasonKeyRequirementProvidedByPlanner = "provided by planner as required by @key"
	ReasonProvidesProvidedByPlanner       = "@provides"
)

func (f *DataSourceFilter) applySuggestionHints(hints []NodeSuggestionHint) {
	if len(hints) == 0 {
		return
	}

	for _, hint := range hints {
		treeNodeID := TreeNodeID(hint.fieldRef)
		treeNode, ok := f.nodes.responseTree.Find(treeNodeID)
		if !ok {
			continue
		}

		itemIndexes := treeNode.GetData()
		for _, itemIdx := range itemIndexes {
			if f.nodes.items[itemIdx].DataSourceHash != hint.dsHash {
				if f.nodes.items[itemIdx].Selected {
					// if the node was already selected by another datasource
					// we unselect it
					f.nodes.items[itemIdx].Selected = false
					f.nodes.items[itemIdx].SelectionReasons = nil
				}
			} else {
				f.nodes.items[itemIdx].selectWithReason(ReasonKeyRequirementProvidedByPlanner, f.enableSelectionReasons)
			}
		}
	}
}

// selectUniqueNodes - selects nodes (e.g. fields) which are unique to a single datasource
// In addition we select:
//   - parent of such node if the node is a leaf and not nested under the fragment
//   - siblings nodes
func (f *DataSourceFilter) selectUniqueNodes() {

	for i := range f.nodes.items {
		if f.nodes.items[i].Selected {
			continue
		}

		isNodeUnique := f.nodes.isNodeUnique(i)
		if !isNodeUnique {
			continue
		}

		// unique nodes always have priority
		f.nodes.items[i].selectWithReason(ReasonStage1Unique, f.enableSelectionReasons)

		if !f.nodes.items[i].onFragment { // on a first stage do not select parent of nodes on fragments
			// if node parents of the unique node is on the same source, prioritize it too
			f.selectUniqNodeParentsUpToRootNode(i)
		}

		// if node has leaf children on the same source, prioritize them too
		children := f.nodes.childNodesOnSameSource(i)
		for _, child := range children {
			if f.nodes.isLeaf(child) && f.nodes.isNodeUnique(child) {
				f.nodes.items[child].selectWithReason(ReasonStage1SameSourceLeafChild, f.enableSelectionReasons)
			}
		}

		// prioritize leaf siblings of the node on the same source
		siblings := f.nodes.siblingNodesOnSameSource(i)
		for _, sibling := range siblings {
			if f.nodes.isLeaf(sibling) && f.nodes.isNodeUnique(sibling) {
				f.nodes.items[sibling].selectWithReason(ReasonStage1SameSourceLeafSibling, f.enableSelectionReasons)
			}
		}
	}
}

func (f *DataSourceFilter) selectUniqNodeParentsUpToRootNode(i int) {
	// When we have a chain of datasource child nodes, we should select every parent until we reach the root node
	// as root node is a starting point from where we could get all theese child nodes

	if f.nodes.items[i].IsRootNode {
		// no need to select parent of a root node here
		// as it could be resolved by itself
		return
	}

	current := i
	for {
		parentIdx, ok := f.nodes.parentNodeOnSameSource(current)
		if !ok {
			break
		}
		f.nodes.items[parentIdx].selectWithReason(ReasonStage1SameSourceParent, f.enableSelectionReasons)

		current = parentIdx
		if f.nodes.items[current].IsRootNode {
			break
		}
	}
}

// selectDuplicateNodes - selects nodes (e.g. fields) which are not unique to a single datasource,
// e.g. could be resolved by multiple datasources
// This method checks only nodes not already selected on the other datasource
// On a first run we are doing set of checks of surrounding nodes selection for the current analyzed node and each of its duplicates:
//   - check for selected parent of a current node or its duplicates
//   - check for selected childs of a current node or its duplicates
//   - check for selected siblings of a current node or its duplicates
//
// On a second run in additional to all the checks from the first run
// we select nodes which was not choosen by previous stages, so we just pick first available datasource
func (f *DataSourceFilter) selectDuplicateNodes(secondRun bool) {

	treeNodes := f.nodes.responseTree.Traverse(tree.TraverseBreadthFirst)

	for treeNode := range treeNodes {
		if treeNode.GetID() == treeRootID {
			continue
		}

		itemIDs := treeNode.GetData()

		for _, i := range itemIDs {
			if f.nodes.items[i].Selected {
				continue
			}

			if f.nodes.isSelectedOnOtherSource(i) {
				continue
			}

			nodeDuplicates := f.nodes.duplicatesOf(i)

			// check for selected parent of a current node or its duplicates
			if f.checkNodeParent(i) {
				continue
			}
			if f.checkNodeDuplicates(nodeDuplicates, f.checkNodeParent) {
				continue
			}

			if f.nodes.items[i].FieldName == typeNameField {
				// we should select typename predictable depending on 2 conditions:
				// - parent was selected
				// - provided by key

				// in case of entity interface we could select __typename from datasource containing this entity interface
				// but not from the datasource containing the interface object
				if !f.nodes.items[i].IsEntityInterfaceTypeName {
					continue
				}
			}

			// check for selected childs of a current node or its duplicates
			if f.checkNodeChilds(i) {
				continue
			}
			if f.checkNodeDuplicates(nodeDuplicates, f.checkNodeChilds) {
				continue
			}

			if f.checkNodeSiblings(i) {
				continue
			}
			if f.checkNodeDuplicates(nodeDuplicates, f.checkNodeSiblings) {
				continue
			}

			if !secondRun {
				continue
			}

			if f.nodes.items[i].IsKeyField {
				continue
			}

			// if after all checks node was not selected
			// we need a couple more checks

			// 1. Lookup in duplicates for root nodes with enabled reference resolver
			// in case current node suggestion is an entity root node, and it contains a key with disabled resolver
			// we could not select such node, because we could not jump to the subgraph which do not have reference resolver,
			// so we need to find a possible duplicate which has enabled entity resolver

			if f.nodes.items[i].IsRootNode && f.nodes.items[i].DisabledEntityResolver {
				foundPossibleDuplicate := false
				for _, duplicateIdx := range nodeDuplicates {
					if !f.nodes.items[duplicateIdx].DisabledEntityResolver {
						f.nodes.items[duplicateIdx].selectWithReason(ReasonStage3SelectRootNodeWithEnabledEntityResolver, f.enableSelectionReasons)
						foundPossibleDuplicate = true
						break
					}
				}
				if foundPossibleDuplicate {
					// continue to the next node
					continue
				}
			}

			// 2. Lookup for the first parent root node with enabled entity resolver
			// when we haven't found a possible duplicate
			// we need to find parent node which is a root node and has enabled entity resolver, e.g. the point in the query from where we could jump

			treeNode := f.nodes.treeNode(i)
			parent := treeNode.GetParent()

			foundPossibleParent := false
			for parent != nil {
				parentNodeIndexes := parent.GetData()

				for _, parentIdx := range parentNodeIndexes {
					if f.nodes.items[parentIdx].IsRootNode && !f.nodes.items[parentIdx].DisabledEntityResolver {
						f.nodes.items[parentIdx].selectWithReason(ReasonStage3SelectFirstParentRootNode, f.enableSelectionReasons)
						foundPossibleParent = true
						break
					}
				}

				if foundPossibleParent {
					break
				}

				parent = parent.GetParent()
			}

			if foundPossibleParent {
				continue
			}

			// If we still haven't selected the node -
			// 3. we could select first available node only in case it is a leaf node
			if f.nodes.isLeaf(i) {
				f.nodes.items[i].selectWithReason(ReasonStage3SelectAvailableLeafNode, f.enableSelectionReasons)
				continue
			}

			// 4. then we try to select a node which could provide more selections on the same source
			currentIdx := i
			currentChildNodeCount := len(f.nodes.childNodesOnSameSource(i))

			for _, duplicateIdx := range nodeDuplicates {
				duplicateChildNodeCount := len(f.nodes.childNodesOnSameSource(duplicateIdx))
				if duplicateChildNodeCount > currentChildNodeCount {
					currentIdx = duplicateIdx
					currentChildNodeCount = duplicateChildNodeCount
				}
			}

			f.nodes.items[currentIdx].selectWithReason(ReasonStage3SelectNodeHavingPossibleChildsOnSameDataSource, f.enableSelectionReasons)
		}
	}
}

func (f *DataSourceFilter) checkNodeDuplicates(duplicates []int, callback func(nodeIdx int) (nodeIsSelected bool)) (nodeIsSelected bool) {
	for _, duplicate := range duplicates {
		if callback(duplicate) {
			nodeIsSelected = true
			break
		}
	}
	return
}

func (f *DataSourceFilter) checkNodeChilds(i int) (nodeIsSelected bool) {
	childs := f.nodes.childNodesOnSameSource(i)
	for _, child := range childs {
		if f.nodes.items[child].Selected {
			f.nodes.items[i].selectWithReason(ReasonStage2SameSourceNodeOfSelectedChild, f.enableSelectionReasons)
			nodeIsSelected = true
			break
		}
	}
	return
}

func (f *DataSourceFilter) checkNodeSiblings(i int) (nodeIsSelected bool) {
	if f.nodes.items[i].IsKeyField {
		// we select based on siblings only in case node is not part of a key
		// check for selected siblings of a current node or its duplicates
		return false
	}

	siblings := f.nodes.siblingNodesOnSameSource(i)
	for _, sibling := range siblings {
		if f.nodes.items[sibling].Selected {
			f.nodes.items[i].selectWithReason(ReasonStage2SameSourceNodeOfSelectedSibling, f.enableSelectionReasons)
			nodeIsSelected = true
			break
		}
	}
	return
}

func (f *DataSourceFilter) checkNodeParent(i int) (nodeIsSelected bool) {
	parentIdx, ok := f.nodes.parentNodeOnSameSource(i)
	if ok && f.nodes.items[parentIdx].Selected {
		f.nodes.items[i].selectWithReason(ReasonStage2SameSourceNodeOfSelectedParent, f.enableSelectionReasons)
		nodeIsSelected = true
	}

	return
}
