package plan

import (
	"slices"

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

	fieldDependsOn map[int][]int
	dataSources    []DataSource
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

func (f *DataSourceFilter) FilterDataSources(dataSources []DataSource, existingNodes *NodeSuggestions, landedTo map[int]DSHash, fieldDependsOn map[int][]int) (used []DataSource, suggestions *NodeSuggestions) {
	var dsInUse map[DSHash]struct{}

	f.fieldDependsOn = fieldDependsOn
	f.dataSources = dataSources

	suggestions, dsInUse = f.findBestDataSourceSet(existingNodes, landedTo)
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

func (f *DataSourceFilter) findBestDataSourceSet(existingNodes *NodeSuggestions, landedTo map[int]DSHash) (*NodeSuggestions, map[DSHash]struct{}) {
	f.nodes = f.collectNodes(f.dataSources, existingNodes)
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

		if len(itemIDs) == 0 {
			// no available nodes to select
			continue
		}

		if !secondPass && f.nodes.items[itemIDs[0]].isTypeName {
			// we want to select typename only after some fields were selected
			continue
		}

		// if any item on the given node is already selected, we could skip it
		if slices.ContainsFunc(itemIDs, func(i int) bool {
			return f.nodes.items[i].Selected
		}) {
			continue
		}

		// Select node based on a check for selected parent of a current node or its duplicates
		if f.checkNodes(itemIDs, f.checkNodeParent, nil) {
			continue
		}

		// we are checking if we have selected any child fields on the same datasource
		// it means that child was unique, so we could select parent of such child
		if f.checkNodes(itemIDs, f.checkNodeChilds, func(i int) bool {
			// do not evaluate childs for the leaf nodes
			return f.nodes.items[i].IsLeaf
		}) {
			continue
		}

		// we are checking if we have selected any sibling fields on the same datasource
		// if sibling is selected on the same datasource, we could select current node
		if f.checkNodes(itemIDs, f.checkNodeSiblings, func(i int) bool {
			// we should not select a __typename field based on a siblings, unless it is on a root query type
			return f.nodes.items[i].FieldName == typeNameField && !IsMutationOrQueryRootType(f.nodes.items[i].TypeName)
		}) {
			continue
		}

		// if after all checks node was not selected
		// we need a couple more checks

		// 1. Lookup in duplicates for root nodes with enabled reference resolver
		// in case current node suggestion is an entity root node, and it contains a key with disabled resolver
		// we could not select such node, because we could not jump to the subgraph which do not have reference resolver,
		// so we need to find a possible duplicate which has enabled entity resolver
		// The tricky part here is to check that the parent node could provide keys for the current node

		if f.checkNodes(itemIDs,
			func(i int) bool {
				return f.selectWithExternalCheck(i, ReasonStage3SelectFirstAvailableRootNodeWithEnabledEntityResolver)
			},
			func(i int) bool {
				if !f.nodes.items[i].IsRootNode {
					return true
				}

				if f.nodes.items[i].DisabledEntityResolver {
					return true
				}

				// we need to check if the node with enabled resolver could actually get a key from the parent node
				if !f.isSelectedParentCouldProvideKeysForCurrentNode(i) {
					return true
				}

				// if node is not a leaf we need to check if it is possible to get any fields (not counting __typename) from this datasource
				if !f.nodes.items[i].IsLeaf && !f.couldProvideChildFields(i) {
					return true
				}

				return false
			}) {
			continue
		}

		// 2. Lookup for the first parent root node with enabled entity resolver
		// when we haven't found a possible duplicate
		// we need to find parent node which is a root node and has enabled entity resolver, e.g. the point in the query from where we could jump
		// it is a parent entity jump case

		if f.checkNodes(itemIDs,
			func(i int) bool {
				if f.nodes.items[i].IsExternal && !f.nodes.items[i].IsProvided {
					return false
				}

				parents := f.findPossibleParents(i)
				if len(parents) > 0 {
					if f.selectWithExternalCheck(i, ReasonStage3SelectNodeUnderFirstParentRootNode) {
						for _, parent := range parents {
							f.nodes.items[parent].selectWithReason(ReasonStage3SelectParentRootNodeWithEnabledEntityResolver, f.enableSelectionReasons)
						}

						return true
					}
				}
				return false
			},
			nil) {
			continue
		}

		// 3 and 4 - are stages when choices are equal, and we should select first available node

		// 3. we choose first available leaf node
		if f.checkNodes(itemIDs,
			func(i int) bool {
				return f.selectWithExternalCheck(i, ReasonStage3SelectAvailableLeafNode)
			},
			func(i int) bool {
				if !f.nodes.isLeaf(i) {
					return true
				}

				if f.nodes.items[i].IsExternal && !f.nodes.items[i].IsProvided {
					return true
				}

				return false
			}) {
			continue
		}

		// 4. if node is not a leaf we select a node which could provide more selections on the same source
		currentItemIDx := itemIDs[0]
		currentChildNodeCount := len(f.nodes.childNodesOnSameSource(currentItemIDx))

		for k, duplicateIdx := range itemIDs {
			if k == 0 {
				continue
			}

			duplicateChildNodeCount := len(f.nodes.childNodesOnSameSource(duplicateIdx))
			if duplicateChildNodeCount > currentChildNodeCount {
				currentItemIDx = duplicateIdx
				currentChildNodeCount = duplicateChildNodeCount
			}
		}

		if currentChildNodeCount > 0 {
			// we can't select node if it doesn't have any child nodes to select
			f.selectWithExternalCheck(currentItemIDx, ReasonStage3SelectNodeHavingPossibleChildsOnSameDataSource)
		}
	}
}

func (f *DataSourceFilter) findPossibleParents(i int) (parentIds []int) {
	nodesIdsToSelect := make([]int, 0, 2)

	parentIdx, _ := f.nodes.parentNodeOnSameSource(i)
	for parentIdx != -1 {
		nodesIdsToSelect = append(nodesIdsToSelect, parentIdx)

		// for the parent node we are checking if it is a root node and has enabled entity resolver.
		// the last check is to ensure that the parent node could provide keys for the current node with parentIdx
		// if all conditions are met, we return full chain of the nodes to this parentIdx
		if f.nodes.items[parentIdx].IsRootNode &&
			!f.nodes.items[parentIdx].DisabledEntityResolver &&
			f.isSelectedParentCouldProvideKeysForCurrentNode(parentIdx) {

			return nodesIdsToSelect
		}

		parentIdx, _ = f.nodes.parentNodeOnSameSource(parentIdx)
	}

	return nil
}

func (f *DataSourceFilter) isSelectedParentCouldProvideKeysForCurrentNode(idx int) bool {
	treeNode := f.nodes.treeNode(idx)
	parentNodeIndexes := treeNode.GetParent().GetData()
	typeName := f.nodes.items[idx].TypeName

	currentDsHash := f.nodes.items[idx].DataSourceHash
	currenDsIdx := slices.IndexFunc(f.dataSources, func(ds DataSource) bool {
		return ds.Hash() == currentDsHash
	})
	if currenDsIdx == -1 {
		return false
	}
	currentDs := f.dataSources[currenDsIdx]
	currentDsKeys := currentDs.FederationConfiguration().Keys
	possibleKeys := currentDsKeys.FilterByTypeAndResolvability(typeName, true)

	for _, parentIdx := range parentNodeIndexes {
		if !f.nodes.items[parentIdx].Selected {
			continue
		}

		dsHash := f.nodes.items[parentIdx].DataSourceHash
		dsIdx := slices.IndexFunc(f.dataSources, func(ds DataSource) bool {
			return ds.Hash() == dsHash
		})
		if dsIdx == -1 {
			continue
		}

		ds := f.dataSources[dsIdx]
		keys := ds.FederationConfiguration().Keys

		keyConfigurations := keys.FilterByTypeAndResolvability(typeName, true)

		for _, possibleKey := range possibleKeys {
			if slices.ContainsFunc(keyConfigurations, func(keyCfg FederationFieldConfiguration) bool {
				return keyCfg.SelectionSet == possibleKey.SelectionSet
			}) {
				return true
			}
		}

		// second check is for the keys which do not have entity resolver
		// e.g. resolvable: false or implicit keys
		keyConfigurations = keys.FilterByTypeAndResolvability(typeName, false)

		// NOTE: logic here is limited currently to only resolvable keys
		// there also could be potential matches with implicit conditional keys
		// if there will be a need to support such cases, we need to extend this logic

		var (
			implicitKeys []FederationFieldConfiguration
		)

		for _, keyCfg := range keyConfigurations {
			if len(keyCfg.Conditions) == 0 {
				implicitKeys = append(implicitKeys, keyCfg)
				continue
			}
		}

		for _, possibleKey := range possibleKeys {
			if slices.ContainsFunc(implicitKeys, func(keyCfg FederationFieldConfiguration) bool {
				return keyCfg.SelectionSet == possibleKey.SelectionSet
			}) {
				return true
			}
		}
	}

	return false
}

func (f *DataSourceFilter) checkNodes(duplicates []int, callback func(nodeIdx int) (nodeIsSelected bool), skip func(nodeIdx int) bool) (nodeIsSelected bool) {
	for _, i := range duplicates {
		if skip != nil && skip(i) {
			continue
		}

		if f.nodes.items[i].IsExternal && !f.nodes.items[i].IsProvided {
			continue
		}

		if callback(i) {
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

		// we should not select a field on the datasource of sibling which depends on the current field
		dependsOnFieldRefs := f.fieldDependsOn[f.nodes.items[sibling].FieldRef]
		if slices.Contains(dependsOnFieldRefs, f.nodes.items[i].FieldRef) {
			continue
		}

		if f.nodes.items[sibling].Selected {
			if f.selectWithExternalCheck(i, ReasonStage2SameSourceNodeOfSelectedSibling) {
				return true
			}
		}
	}

	return false
}

func (f *DataSourceFilter) checkNodeParent(i int) (nodeIsSelected bool) {
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

	if f.selectWithExternalCheck(i, ReasonStage2SameSourceNodeOfSelectedParent) {
		if f.nodes.items[i].IsProvided && !f.nodes.items[i].IsLeaf {
			f.selectProvidedChildNodes(i)
		}

		return true
	}

	return false
}

func (f *DataSourceFilter) selectProvidedChildNodes(i int) {
	children := f.nodes.childNodesOnSameSource(i)
	for _, childId := range children {
		if f.nodes.items[childId].IsProvided {
			f.nodes.items[childId].selectWithReason(ReasonProvidesProvidedByPlanner, f.enableSelectionReasons)
			if !f.nodes.items[childId].IsLeaf {
				f.selectProvidedChildNodes(childId)
			}
		}
	}
}

func (f *DataSourceFilter) selectWithExternalCheck(i int, reason string) (nodeIsSelected bool) {
	if f.nodes.items[i].IsExternal && !f.nodes.items[i].IsProvided {
		return false
	}

	f.nodes.items[i].selectWithReason(reason, f.enableSelectionReasons)
	return true
}

// couldProvideChildFields - checks if the node could provide any selectable child fields on the same datasource
func (f *DataSourceFilter) couldProvideChildFields(i int) bool {
	nodesIds := f.nodes.childNodesOnSameSource(i)

	hasFields := false
	for _, i := range nodesIds {
		if f.nodes.items[i].FieldName == typeNameField {
			// we have to omit __typename field
			// to not be in a situation when all fields are external but __typename is selectable
			continue
		}

		hasFields = true
	}

	return hasFields
}
