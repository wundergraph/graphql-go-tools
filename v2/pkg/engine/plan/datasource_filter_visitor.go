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

	jumpsForPathForTypename map[KeyIndex]*DataSourceJumpsGraph
	dsHashesHavingKeys      map[DSHash]struct{}
	seenKeys                map[SeenKeyPath]struct{}

	maxDataSourceCollectorsConcurrency uint
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

// WithMaxDataSourceCollectorsConcurrency sets the maximum number of concurrent data source collectors
func (f *DataSourceFilter) WithMaxDataSourceCollectorsConcurrency(maxConcurrency uint) *DataSourceFilter {
	f.maxDataSourceCollectorsConcurrency = maxConcurrency
	return f
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
	f.collectNodes(f.dataSources, existingNodes)
	if f.report.HasErrors() {
		return nil, nil
	}

	// f.nodes.printNodes("initial nodes")
	f.applyLandedTo(landedTo) // FAILING TEST IF REMOVE: single key - double key - double key - single key

	f.selectUniqueNodes()
	f.selectDuplicateNodes(false)
	f.selectDuplicateNodes(true)

	uniqueDataSourceHashes := f.nodes.populateHasSuggestions()

	// add ds hashes from the keys
	for dsHash := range f.dsHashesHavingKeys {
		uniqueDataSourceHashes[dsHash] = struct{}{}
	}

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

				// we need to unselect this node as we are changing initial node selection when we had less information
				// but in case there are any selected child nodes - selection should be untouched
				hasSelectedChildNodes := false
				if childNodes := f.nodes.childNodesOnSameSource(itemID); childNodes != nil {
					for _, childID := range childNodes {
						if f.nodes.items[childID].Selected {
							hasSelectedChildNodes = true
						}
					}
				}
				if !hasSelectedChildNodes {
					f.nodes.items[itemID].unselect()
				}

			}
		}
	}

}

func (f *DataSourceFilter) collectNodes(dataSources []DataSource, existingNodes *NodeSuggestions) {
	if existingNodes == nil {
		existingNodes = NewNodeSuggestions()
	}

	if f.seenKeys == nil {
		f.seenKeys = make(map[SeenKeyPath]struct{})
	}

	nodesCollector := &nodesCollector{
		operation:      f.operation,
		definition:     f.definition,
		dataSources:    dataSources,
		nodes:          existingNodes,
		report:         f.report,
		maxConcurrency: f.maxDataSourceCollectorsConcurrency,
		seenKeys:       f.seenKeys,
	}

	var keysInfo []DSKeyInfo
	f.nodes, keysInfo = nodesCollector.CollectNodes()

	if f.dsHashesHavingKeys == nil {
		f.dsHashesHavingKeys = make(map[DSHash]struct{})
	}

	keysForPathForTypename := make(map[KeyIndex]map[DSHash][]KeyInfo)
	for _, keyInfo := range keysInfo {
		keyIndex := KeyIndex{keyInfo.Path, keyInfo.TypeName}
		keysPerDS, ok := keysForPathForTypename[keyIndex]
		if !ok {
			keysPerDS = make(map[DSHash][]KeyInfo)
		}

		keysPerDS[keyInfo.DSHash] = keyInfo.Keys
		keysForPathForTypename[keyIndex] = keysPerDS

		f.dsHashesHavingKeys[keyInfo.DSHash] = struct{}{}
	}

	if f.jumpsForPathForTypename == nil {
		f.jumpsForPathForTypename = make(map[KeyIndex]*DataSourceJumpsGraph)
	}

	usedDsHashes := make([]DSHash, 0, len(f.dsHashesHavingKeys))
	// iterate over datasources to have deterministic order
	for _, ds := range dataSources {
		if _, ok := f.dsHashesHavingKeys[ds.Hash()]; ok {
			usedDsHashes = append(usedDsHashes, ds.Hash())
		}
	}

	for keyIndex, keysPerDS := range keysForPathForTypename {
		f.jumpsForPathForTypename[KeyIndex{Path: keyIndex.Path, TypeName: keyIndex.TypeName}] = NewDataSourceJumpsGraph(usedDsHashes, keysPerDS, keyIndex.TypeName)
	}
}

const (
	ReasonStage1Unique           = "stage1: unique"
	ReasonStage1SameSourceParent = "stage1: same source parent of unique node"

	ReasonStage2SameSourceNodeOfSelectedParent  = "stage2: node on the same source as selected parent"
	ReasonStage2SameSourceNodeOfSelectedChild   = "stage2: node on the same source as selected child"
	ReasonStage2SameSourceNodeOfSelectedSibling = "stage2: node on the same source as selected sibling"

	ReasonStage3SelectAvailableLeafNode                        = "stage3: select first available leaf node"
	ReasonStage3SelectNodeHavingPossibleChildsOnSameDataSource = "stage3: select non leaf node which have possible child selections on the same source"
	ReasonStage3SelectFirstAvailableRootNode                   = "stage3: first available root node"
	ReasonStage3SelectParentRootNodeWithEnabledEntityResolver  = "stage3: first available parent node with enabled entity resolver"
	ReasonStage3SelectNodeUnderFirstParentRootNode             = "stage3: node under first available parent node"
	ReasonStage3SelectParentNodeWhichCouldGiveKeys             = "stage3: select parent node which could provide keys for the child node"

	ReasonKeyRequirementProvidedByPlanner = "provided by planner as required by @key"
	ReasonProvidesProvidedByPlanner       = "@provides"
)

// selectUniqueNodes - selects nodes (e.g. fields) which are unique to a single datasource
func (f *DataSourceFilter) selectUniqueNodes() {

	for i := range f.nodes.items {
		if f.nodes.items[i].Selected {
			continue
		}

		if !f.nodes.isNodeUnique(i) {
			continue
		}

		// unique nodes always have priority
		f.nodes.items[i].selectWithReason(ReasonStage1Unique, f.enableSelectionReasons)

		f.selectUniqNodeParentsUpToRootNode(i)
	}
}

func (f *DataSourceFilter) selectUniqNodeParentsUpToRootNode(i int) {
	// When we have a chain of datasource child nodes, we should select every parent until we reach the root node
	// as a root node is a starting point from where we could get all these child nodes

	if f.nodes.items[i].IsRootNode {
		// no need to select the parent of a root node here
		// as it could be resolved by itself
		return
	}

	rootNodeFound := false
	nodesIdsToSelect := make([]int, 0, 2)
	current := i
	for {
		parentIdx, ok := f.nodes.parentNodeOnSameSource(current)
		if !ok {
			break
		}
		nodesIdsToSelect = append(nodesIdsToSelect, parentIdx)

		if f.nodes.items[parentIdx].IsExternal && !f.nodes.items[i].IsProvided {
			// such a parent can't be selected,
			// so we skip this parent but continue looking for a potential root node higher
			current = parentIdx
			continue
		}

		// TODO: there could be a potential situation when we have selected root node with enabled entity resolver,
		// but we can't jump to it because no parent could provide a key for it
		// Need to consider how to move this logic into the selection process of duplicated nodes maybe?

		if f.nodes.items[parentIdx].IsRootNode && !f.nodes.items[parentIdx].DisabledEntityResolver {
			rootNodeFound = true
			break
		}

		current = parentIdx
	}

	if !rootNodeFound {
		return
	}

	for _, parentIdx := range nodesIdsToSelect {
		f.nodes.items[parentIdx].selectWithReason(ReasonStage1SameSourceParent, f.enableSelectionReasons)
	}
}

func hasPathBetweenDs(jumps *DataSourceJumpsGraph, from, to DSHash) (bestPath *SourceConnection, exists bool) {
	possiblePaths, exists := jumps.GetPaths(from, to)
	if !exists {
		return nil, false
	}

	var directs []SourceConnection
	var indirects []SourceConnection

	for _, path := range possiblePaths {
		if path.Type == SourceConnectionTypeDirect {
			directs = append(directs, path)
			continue
		}
		indirects = append(indirects, path)
	}

	if len(directs) > 0 {
		return &directs[0], true
	}

	// TODO: indirect path should take into consideration existing nodes?

	for _, path := range indirects {
		if bestPath == nil {
			bestPath = &path
			continue
		}

		if len(path.Jumps) < len(bestPath.Jumps) {
			bestPath = &path
		}
	}

	return bestPath, bestPath != nil
}

func (f *DataSourceFilter) jumpsForPathAndTypeName(path string, typeName string) (*DataSourceJumpsGraph, bool) {
	jumpsForTypename, exists := f.jumpsForPathForTypename[KeyIndex{Path: path, TypeName: typeName}]
	if !exists {
		return nil, false
	}

	return jumpsForTypename, true
}

func (f *DataSourceFilter) assignKeys(itemIdx int, parentNodeIndexes []int) {
	currentNode := f.nodes.items[itemIdx]

	if currentNode.requiresKey != nil {
		return
	}

	currentNodeDsHash := currentNode.DataSourceHash
	currentNodeTypeName := currentNode.TypeName

	selectedParentHashes := make([]DSHash, 0, len(parentNodeIndexes))
	hasSelectedParentOnSameDataSource := false

	for _, parentIdx := range parentNodeIndexes {
		if !f.nodes.items[parentIdx].Selected {
			continue
		}

		if f.nodes.items[parentIdx].DataSourceHash == currentNodeDsHash {
			hasSelectedParentOnSameDataSource = true
			break
		}

		selectedParentHashes = append(selectedParentHashes, f.nodes.items[parentIdx].DataSourceHash)
	}

	if hasSelectedParentOnSameDataSource {
		return
	}

	jumpsForTypename, exists := f.jumpsForPathAndTypeName(currentNode.ParentPath, currentNodeTypeName)
	if !exists {
		return
	}

	for _, selectedParentHash := range selectedParentHashes {
		path, exists := hasPathBetweenDs(jumpsForTypename, selectedParentHash, currentNodeDsHash)
		if exists {
			currentNode.requiresKey = path
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
		// f.nodes.printNodes("nodes")
		// fmt.Println()

		if treeNode.GetID() == treeRootID {
			continue
		}

		itemIDs := treeNode.GetData()
		if len(itemIDs) == 0 {
			// no available nodes to select
			continue
		}

		if len(itemIDs) == 1 {
			// such node already selected as unique

			parentNodeIndexes := treeNode.GetParent().GetData()

			f.assignKeys(itemIDs[0], parentNodeIndexes)
			continue
		}

		if !secondPass && f.nodes.items[itemIDs[0]].isTypeName {
			// we want to select typename only after some fields were selected
			continue
		}

		// if any item on the given node is already selected, we could skip it
		skip := false
		for _, itemID := range itemIDs {
			if !f.nodes.items[itemID].Selected {
				continue
			}

			skip = true

			currentNode := f.nodes.items[itemID]

			if currentNode.requiresKey != nil {
				continue
			}

			parentNodeIndexes := treeNode.GetParent().GetData()

			f.assignKeys(itemID, parentNodeIndexes)
		}

		if skip {
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
			// we skip non leaf nodes which could not provide any child selections
			if !f.nodes.items[i].IsLeaf && !f.couldProvideChildFields(i) {
				return true
			}

			// we should not select a __typename field based on a siblings, unless it is on a root query type
			return f.nodes.items[i].isTypeName && !IsMutationOrQueryRootType(f.nodes.items[i].TypeName)
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
				return f.selectWithExternalCheck(i, ReasonStage3SelectFirstAvailableRootNode)
			},
			func(i int) (skip bool) {
				if !f.nodes.items[i].IsRootNode {
					return true
				}

				if f.nodes.items[i].DisabledEntityResolver {
					return true
				}

				// when node is a root query node we will not have parent
				// so we need to check if parent node id is not a root of a tree
				if treeNode.GetParentID() != treeRootID {
					// we need to check if the node with enabled resolver could actually get a key from the parent node
					if !f.isSelectedParentCouldProvideKeysForCurrentNode(i) {
						return true
					}
				}

				// if node is not a leaf we need to check if it is possible to get any fields (not counting __typename) from this datasource
				// it may be the case that all fields belongs to different datasource, but this node could serve as the connection point
				// to the other datasources, so we check if parent could provide a key
				// and that we could provide a key to the next childs
				if f.nodes.items[i].IsLeaf {
					return false
				}

				// if current query node has only typename child field, we could select it
				if f.childsHasOnlyTypename(i) {
					return false
				}

				if f.couldProvideChildFields(i) {
					return false
				}

				if f.nodeCouldProvideKeysToChildNodes(i) {
					return false
				}

				return true
			}) {
			continue
		}

		// stages 2,3,4 - are stages when choices are equal, and we should select first available node

		// 2. we choose the first available leaf node
		if f.checkNodes(itemIDs,
			func(i int) bool {
				return f.selectWithExternalCheck(i, ReasonStage3SelectAvailableLeafNode)
			},
			func(i int) bool {
				if !f.nodes.isLeaf(i) {
					return true
				}

				// when node is a root query node we will not have parent
				// so we need to check if parent node id is not a root of a tree
				if treeNode.GetParentID() != treeRootID {
					// we need to check if the node with enabled resolver could actually get a key from the parent node
					if !f.isSelectedParentCouldProvideKeysForCurrentNode(i) {
						return true
					}
				}

				return false
			}) {
			continue
		}

		// 3. if node is not a leaf we select a node which could provide more selections on the same source
		currentChildNodeCount := -1
		currentItemIDx := -1

		for _, duplicateIdx := range itemIDs {
			// we need to check if the node with enabled resolver could actually get a key from the parent node
			if !f.isSelectedParentCouldProvideKeysForCurrentNode(duplicateIdx) {
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
			continue
		}

		// 4. We check here not leaf nodes which could provide keys to the child nodes
		// this rule one of the rules responsible for the shareable nodes
		if f.checkNodes(itemIDs,
			func(i int) bool {
				return f.selectWithExternalCheck(i, ReasonStage3SelectParentNodeWhichCouldGiveKeys)
			},
			func(i int) (skip bool) {
				// do not evaluate childs for the leaf nodes
				if f.nodes.items[i].IsLeaf {
					return true
				}

				// when node is a root query node we will not have parent
				// so we need to check if parent node id is not a root of a tree
				if treeNode.GetParentID() != treeRootID {
					// when node is not a root query node we also check if node could actually get a key from the parent node
					if !f.isSelectedParentCouldProvideKeysForCurrentNode(i) {
						return true
					}
				}

				return !f.nodeCouldProvideKeysToChildNodes(i)
			}) {
			continue
		}

		// 5. Lookup for the first parent root node with the enabled entity resolver.
		// When we haven't found a possible duplicate -
		// we need to find the parent node which is a root node and has enabled entity resolver,
		// e.g., the point in the query from where we could jump.
		// It is a parent entity jump case, and it is less preferable,
		// than direct entity jump, so it should go last.

		// TODO: replace with all nodes check - select smallest parent entity chain
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

	for _, parentIdx := range parentNodeIndexes {
		if !f.nodes.items[parentIdx].Selected {
			continue
		}

		if f.parentNodeCouldProvideKeysForCurrentNode(parentIdx, idx, true) {
			return true
		}
	}

	return false
}

func (f *DataSourceFilter) nodeCouldProvideKeysToChildNodes(idx int) bool {
	childIds := f.nodes.childNodesIdsOnOtherDS(idx)

	for _, childId := range childIds {
		if f.nodes.items[childId].isTypeName {
			// we have to omit __typename field
			// to not be in a situation when all fields are external but __typename is selectable
			continue
		}

		if f.parentNodeCouldProvideKeysForCurrentNode(idx, childId, false) {
			return true
		}
	}

	return false
}

func (f *DataSourceFilter) parentNodeCouldProvideKeysForCurrentNode(parentIdx, idx int, withSetKey bool) bool {
	// this check is widely used for all selection rules
	// one thing to know, on the first iterations abstract selections are not rewritten, yet
	// they could be rewritten when we see that on the given datasource some field is external
	// This check could happen earlier when the type of the field on parent will be abstract,
	// but the actual field on one of the concrete types, so there will be no match
	// We handle this by adding possible keys for each possible type during nodes collecting

	// first we need to check a concrete type, because it could be an entity interface type which is possible to use for jump
	if f.parentNodeCouldProvideKeysForCurrentNodeWithTypename(parentIdx, idx, f.nodes.items[idx].TypeName, withSetKey) {
		return true
	}

	// possible type names are used for the union and interface types
	// for the interface objects could be used one of the possible types or the interface object type itself
	if len(f.nodes.items[idx].possibleTypeNames) > 0 {
		for _, typeName := range f.nodes.items[idx].possibleTypeNames {
			if f.parentNodeCouldProvideKeysForCurrentNodeWithTypename(parentIdx, idx, typeName, false) {
				return true
			}
		}
	}

	return false
}

// parentNodeCouldProvideKeysForCurrentNodeWithTypename - checks if the parent node could provide keys for the current node
// e.g. if there is a jump path between the parent node and the current node
// NOTE: method has side effects, it sets requiresKey for the current node
func (f *DataSourceFilter) parentNodeCouldProvideKeysForCurrentNodeWithTypename(parentIdx, idx int, typeName string, setRequiresKey bool) bool {
	if f.nodes.items[parentIdx].DataSourceHash == f.nodes.items[idx].DataSourceHash {
		return true
	}

	jumpsForTypename, exists := f.jumpsForPathAndTypeName(f.nodes.items[idx].ParentPath, typeName)
	if !exists {
		return false
	}

	path, exists := hasPathBetweenDs(jumpsForTypename, f.nodes.items[parentIdx].DataSourceHash, f.nodes.items[idx].DataSourceHash)
	if !exists {
		return false
	}

	if setRequiresKey {
		f.nodes.items[idx].requiresKey = path
	}

	return true
}

type nodeJump struct {
	// nodeIdx is the index of the node in the nodes slice
	nodeIdx int
	// jumpCount is the number of jumps to the node
	jumpCount int
}

func (f *DataSourceFilter) checkNodes(duplicates []int, callback func(nodeIdx int) (nodeIsSelected bool), skip func(nodeIdx int) (skip bool)) (nodeIsSelected bool) {
	allowedToSelect := make([]int, 0, len(duplicates))

	for _, i := range duplicates {
		if f.nodes.items[i].IsExternal && !f.nodes.items[i].IsProvided {
			continue
		}

		if skip != nil && skip(i) {
			continue
		}

		allowedToSelect = append(allowedToSelect, i)
	}

	jumpsCounts := make([]nodeJump, 0, len(allowedToSelect))

	for _, i := range allowedToSelect {
		jumpCount := 0

		if f.nodes.items[i].requiresKey != nil {
			jumpCount = len(f.nodes.items[i].requiresKey.Jumps)
		}

		jumpsCounts = append(jumpsCounts, nodeJump{
			nodeIdx:   i,
			jumpCount: jumpCount,
		})
	}

	// sort by the number of jumps
	slices.SortFunc(jumpsCounts, func(a, b nodeJump) int {
		// acs order from 0 to n
		return a.jumpCount - b.jumpCount
	})

	for _, j := range jumpsCounts {
		if callback(j.nodeIdx) {
			return true
		}
	}

	return false
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
		if f.nodes.items[i].isTypeName {
			// we have to omit __typename field
			// to not be in a situation when all fields are external but __typename is selectable
			continue
		}

		if f.nodes.items[i].IsExternal && !f.nodes.items[i].IsProvided {
			continue
		}

		hasFields = true
	}

	return hasFields
}

func (f *DataSourceFilter) childsHasOnlyTypename(i int) (hasOnlyTypename bool) {
	treeNode := f.nodes.treeNode(i)
	children := treeNode.GetChildren()

	if len(children) == 0 {
		return false
	}

	hasTypeName := false
	hasFields := false

	for _, child := range children {
		itemIds := child.GetData()
		firstItem := itemIds[0]

		if f.nodes.items[firstItem].isTypeName {
			hasTypeName = true
			continue
		}

		hasFields = true
	}

	return !hasFields && hasTypeName
}
