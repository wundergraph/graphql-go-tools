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
	secondaryRun           bool
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

func (f *DataSourceFilter) FilterDataSources(dataSources []DataSource, existingNodes *NodeSuggestions, landedTo map[int]DSHash) (used []DataSource, suggestions *NodeSuggestions) {
	var dsInUse map[DSHash]struct{}

	suggestions, dsInUse = f.findBestDataSourceSet(dataSources, existingNodes, landedTo)
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

	f.secondaryRun = true
	return used, suggestions
}

func (f *DataSourceFilter) findBestDataSourceSet(dataSources []DataSource, existingNodes *NodeSuggestions, landedTo map[int]DSHash) (*NodeSuggestions, map[DSHash]struct{}) {
	f.nodes = f.collectNodes(dataSources, existingNodes)
	if f.report.HasErrors() {
		return nil, nil
	}

	// f.nodes.printNodes("initial nodes")
	f.applyLandedTo(landedTo)

	f.selectUniqueNodes()
	// f.nodes.printNodes("unique nodes")
	f.selectDuplicateNodes(false)
	// f.nodes.printNodes("duplicate nodes")
	f.selectDuplicateNodes(true)
	// f.nodes.printNodes("duplicate nodes after second run")

	uniqueDataSourceHashes := f.nodes.populateHasSuggestions()
	return f.nodes, uniqueDataSourceHashes
}

func (f *DataSourceFilter) applyLandedTo(landedTo map[int]DSHash) {
	if landedTo == nil {
		return
	}

	for fieldRef, dsHash := range landedTo {
		treeNodeID := TreeNodeID(fieldRef)

		node, ok := f.nodes.responseTree.Find(treeNodeID)
		if !ok {
			panic("node not found")
		}

		nodeData := node.GetData()
		for _, itemID := range nodeData {
			if f.nodes.items[itemID].DataSourceHash == dsHash {
				f.nodes.items[itemID].unselect()
				// we need to select this node
				f.nodes.items[itemID].selectWithReason(ReasonKeyRequirementProvidedByPlanner, f.enableSelectionReasons)
				f.nodes.items[itemID].IsRequiredKeyField = true
			} else if f.nodes.items[itemID].Selected {
				// we need to unselect this node
				f.nodes.items[itemID].unselect()
			}
		}
	}

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

	ReasonStage3SelectAvailableLeafNode                               = "stage3: select first available leaf node"
	ReasonStage3SelectNodeHavingPossibleChildsOnSameDataSource        = "stage3: select non leaf node which have possible child selections on the same source"
	ReasonStage3SelectFirstAvailableRootNodeWithEnabledEntityResolver = "stage3: first available node with enabled entity resolver"
	ReasonStage3SelectParentRootNodeWithEnabledEntityResolver         = "stage3: first available parent node with enabled entity resolver"
	ReasonStage3SelectNodeUnderFirstParentRootNode                    = "stage3: node under first available parent node with enabled entity resolver"

	ReasonKeyRequirementProvidedByPlanner = "provided by planner as required by @key"
	ReasonProvidesProvidedByPlanner       = "@provides"
)

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
	// as root node is a starting point from where we could get all these child nodes

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
// we select nodes which was not chosen by previous stages, so we just pick first available datasource
func (f *DataSourceFilter) selectDuplicateNodes(secondPass bool) {

	treeNodes := f.nodes.responseTree.Traverse(tree.TraverseBreadthFirst)

	for treeNode := range treeNodes {
		if treeNode.GetID() == treeRootID {
			continue
		}

		itemIDs := treeNode.GetData()
		if len(itemIDs) == 1 {
			// such node already selected as unique
			continue
		}

		// isKeyInSomeDatasource := false
		// for _, i := range itemIDs {
		// 	if f.nodes.items[i].IsKeyField {
		// 		isKeyInSomeDatasource = true
		// 		break
		// 	}
		// }

		for _, i := range itemIDs {
			if f.nodes.items[i].Selected {
				break
			}

			if f.nodes.isSelectedOnOtherSource(i) {
				break
			}

			if f.nodes.items[i].IsExternal && !f.nodes.items[i].IsProvided {
				continue
			}

			nodeDuplicates := f.nodes.duplicatesOf(i)

			// Select node based on a check for selected parent of a current node or its duplicates
			// Additional considerations, if node is not leaf, e.g. it has children
			// we need to check do we also already selected any child fields on the same datasource,
			// When there is no child selections on the same datasource, we need to skip selecting this node,
			// because it means we potentially planned all child fields on other datasources,
			// also it this is the first pass, we won't count __typename field as a child.
			// we will do it on the second pass, when we will have some other fields selected, or we only have __typename field selection
			if secondPass {
				if f.checkNodeParent(i) {
					continue
				}
				if f.checkNodeDuplicates(nodeDuplicates, f.checkNodeParent) {
					continue
				}
			} else {
				if f.checkNodeParentSkipTypeName(i) {
					continue
				}
				if f.checkNodeDuplicates(nodeDuplicates, f.checkNodeParentSkipTypeName) {
					continue
				}
			}

			if f.nodes.items[i].FieldName == typeNameField && !IsMutationOrQueryRootType(f.nodes.items[i].TypeName) {
				// we should select __typename predictable depending on 2 conditions:
				// - parent was selected
				// - provided by key
				// Exception: __typename is on a root operation type

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

			if !f.nodes.items[i].IsRequiredKeyField {
				if f.checkNodeSiblings(i) {
					continue
				}
				if f.checkNodeDuplicates(nodeDuplicates, f.checkNodeSiblings) {
					continue
				}
			}

			if !secondPass {
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
						if f.selectWithExternalCheck(duplicateIdx, ReasonStage3SelectFirstAvailableRootNodeWithEnabledEntityResolver) {
							foundPossibleDuplicate = true
							break
						}
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

			currentCheckIdx := i
			parents, ok := f.findPossibleParents(i)
			if !ok {
				for _, duplicateIdx := range nodeDuplicates {
					currentCheckIdx = duplicateIdx
					parents, ok = f.findPossibleParents(duplicateIdx)
					if ok {
						break
					}
				}
			}
			if len(parents) > 0 {
				if f.selectWithExternalCheck(currentCheckIdx, ReasonStage3SelectNodeUnderFirstParentRootNode) {
					for _, parent := range parents {
						f.nodes.items[parent].selectWithReason(ReasonStage3SelectParentRootNodeWithEnabledEntityResolver, f.enableSelectionReasons)
					}

					// continue to the next node
					continue
				}
			}

			// If we still haven't selected the node -
			// 3. we could select first available node only in case it is a leaf node
			if f.nodes.isLeaf(i) {
				if f.selectWithExternalCheck(i, ReasonStage3SelectAvailableLeafNode) {
					continue
				}
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

			f.selectWithExternalCheck(currentIdx, ReasonStage3SelectNodeHavingPossibleChildsOnSameDataSource)
		}
	}
}

func (f *DataSourceFilter) findPossibleParents(i int) (parentIds []int, ok bool) {
	nodesIdsToSelect := make([]int, 0, 2)

	parentIdx, ok := f.nodes.parentNodeOnSameSource(i)
	for parentIdx != -1 {
		nodesIdsToSelect = append(nodesIdsToSelect, parentIdx)

		if f.nodes.items[parentIdx].IsRootNode && !f.nodes.items[parentIdx].DisabledEntityResolver {
			return nodesIdsToSelect, true
		}

		parentIdx, ok = f.nodes.parentNodeOnSameSource(parentIdx)
	}

	return nil, false
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
			if f.selectWithExternalCheck(i, ReasonStage2SameSourceNodeOfSelectedChild) {
				return true
			}
		}
	}
	return false
}

func (f *DataSourceFilter) checkNodeSiblings(i int) (nodeIsSelected bool) {
	parentIdx, hasParentOnSameSource := f.nodes.parentNodeOnSameSource(i)
	if hasParentOnSameSource && f.nodes.items[parentIdx].IsExternal && !f.nodes.items[i].IsProvided {
		return false
	}

	siblings := f.nodes.siblingNodesOnSameSource(i)
	for _, sibling := range siblings {
		if f.nodes.items[sibling].Selected {
			if f.selectWithExternalCheck(i, ReasonStage2SameSourceNodeOfSelectedSibling) {
				return true
			}
		}
	}

	return false
}

func (f *DataSourceFilter) checkNodeParent(i int) (nodeIsSelected bool) {
	return f.checkNodeParentWithTypeNameField(i, false)
}

func (f *DataSourceFilter) checkNodeParentSkipTypeName(i int) (nodeIsSelected bool) {
	return f.checkNodeParentWithTypeNameField(i, true)
}

func (f *DataSourceFilter) checkNodeParentWithTypeNameField(i int, skipTypeNameField bool) (nodeIsSelected bool) {
	parentIdx, ok := f.nodes.parentNodeOnSameSource(i)
	if !ok {
		return false
	}

	if !f.nodes.items[parentIdx].Selected {
		return false
	}

	// if selected parent is external and selected
	// but current node is not provided, we can't select it as it is external
	// it means that there was provided some sibling, but not the current field
	parentIsExternal := f.nodes.items[parentIdx].IsExternal
	if parentIsExternal && !f.nodes.items[i].IsProvided {
		return false
	}

	if !f.nodes.items[i].IsLeaf && skipTypeNameField && len(f.nodes.withoutTypeName(f.nodes.childNodesOnSameSource(i))) == 0 {
		return false
	}

	return f.selectWithExternalCheck(i, ReasonStage2SameSourceNodeOfSelectedParent)
}

func (f *DataSourceFilter) selectWithExternalCheck(i int, reason string) (nodeIsSelected bool) {
	if f.nodes.items[i].IsExternal && !f.nodes.items[i].IsProvided {
		return false
	}

	f.nodes.items[i].selectWithReason(reason, f.enableSelectionReasons)
	return true
}
