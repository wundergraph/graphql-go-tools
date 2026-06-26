package plan

import (
	"slices"

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

	fieldDependsOn            map[int][]int
	newFieldRefs              map[int]struct{}
	skipUnionRewriteFieldRefs map[int]struct{}
	dataSources               []DataSource

	allowFallbackKeyJumps   bool
	jumpsForPathForTypename map[KeyIndex]*DataSourceJumpsGraph
	dsHashesHavingKeys      map[DSHash]struct{}

	maxDataSourceCollectorsConcurrency uint
	nodesCollector                     *nodesCollector

	abstractFieldRequestedMembers map[string][]string
}

func NewDataSourceFilter(operation, definition *ast.Document, report *operationreport.Report, dataSources []DataSource, newFieldRefs map[int]struct{}, skipUnionRewriteFieldRefs ...map[int]struct{}) *DataSourceFilter {
	var skipUnionRewrite map[int]struct{}
	if len(skipUnionRewriteFieldRefs) > 0 {
		skipUnionRewrite = skipUnionRewriteFieldRefs[0]
	}

	return &DataSourceFilter{
		operation:                 operation,
		definition:                definition,
		report:                    report,
		dataSources:               dataSources,
		nodes:                     NewNodeSuggestions(),
		newFieldRefs:              newFieldRefs,
		skipUnionRewriteFieldRefs: skipUnionRewrite,
	}
}

func (f *DataSourceFilter) EnableSelectionReasons() {
	f.enableSelectionReasons = true
}

func (f *DataSourceFilter) EnableFallbackKeyJumps() {
	f.allowFallbackKeyJumps = true
}

// WithMaxDataSourceCollectorsConcurrency sets the maximum number of concurrent data source collectors
func (f *DataSourceFilter) WithMaxDataSourceCollectorsConcurrency(maxConcurrency uint) *DataSourceFilter {
	f.maxDataSourceCollectorsConcurrency = maxConcurrency
	return f
}

func (f *DataSourceFilter) FilterDataSources(landedTo map[int]DSHash, fieldDependsOn map[int][]int) (used []DataSource, suggestions *NodeSuggestions) {
	var dsInUse map[DSHash]struct{}

	f.fieldDependsOn = fieldDependsOn

	suggestions, dsInUse = f.findBestDataSourceSet(landedTo)
	if f.report.HasErrors() {
		return
	}

	used = make([]DataSource, 0, len(dsInUse))
	for i := range f.dataSources {
		_, inUse := dsInUse[f.dataSources[i].Hash()]
		if inUse {
			used = append(used, f.dataSources[i])
		}
	}

	f.secondaryRun = true
	return used, suggestions
}

func (f *DataSourceFilter) findBestDataSourceSet(landedTo map[int]DSHash) (*NodeSuggestions, map[DSHash]struct{}) {
	f.collectNodes()
	if f.report.HasErrors() {
		return nil, nil
	}

	// f.nodes.printNodes("initial nodes")
	f.applyLandedTo(landedTo) // FAILING TEST IF REMOVE: single key - double key - double key - single key

	f.selectUniqueNodes()
	f.selectDuplicateNodes(false)
	f.selectDuplicateNodes(true)
	f.selectClosestDatasourceForAbstractFields()

	uniqueDataSourceHashes := f.nodes.populateHasSuggestions()

	// add ds hashes from the keys
	for dsHash := range f.dsHashesHavingKeys {
		uniqueDataSourceHashes[dsHash] = struct{}{}
	}

	return f.nodes, uniqueDataSourceHashes
}

func (f *DataSourceFilter) selectClosestDatasourceForAbstractFields() {
	for _, treeNode := range TraverseBFS(f.nodes.responseTree) {
		itemIDs := treeNode.GetData()
		if len(itemIDs) < 2 {
			continue
		}

		selectedItems := make([]int, 0, len(itemIDs))
		for _, itemID := range itemIDs {
			if f.nodes.items[itemID].Selected {
				selectedItems = append(selectedItems, itemID)
			}
		}

		if len(selectedItems) < 2 {
			continue
		}

		unionDefRef, ok := f.fieldReturnsUnionType(selectedItems[0])
		if !ok {
			continue
		}

		requestedMembers := f.fullRequestedUnionMemberTypeNames(selectedItems[0], unionDefRef)
		if len(requestedMembers) == 0 {
			continue
		}

		closestItem := selectedItems[0]
		closestScore := f.datasourceJumpScore(closestItem)
		for _, itemID := range selectedItems[1:] {
			score := f.datasourceJumpScore(itemID)
			if score < closestScore {
				closestItem = itemID
				closestScore = score
			}
		}

		closestMembers, ok := f.localUnionMemberTypeNames(closestItem, unionDefRef)
		if !ok {
			continue
		}

		missingMembers := f.missingUnionMembers(requestedMembers, closestMembers)
		if len(missingMembers) == 0 {
			for _, itemID := range selectedItems {
				if itemID == closestItem {
					continue
				}
				f.unselectAbstractFieldBranch(itemID)
			}
			continue
		}

		f.restrictChildrenToUnionMembers(closestItem, f.intersectUnionMembers(requestedMembers, closestMembers))

		fallbackCandidates := make([]nodeJump, 0, len(selectedItems)-1)
		for _, itemID := range selectedItems {
			if itemID == closestItem {
				continue
			}
			fallbackCandidates = append(fallbackCandidates, nodeJump{
				nodeIdx:   itemID,
				jumpCount: f.datasourceJumpScore(itemID),
			})
		}

		slices.SortFunc(fallbackCandidates, func(a, b nodeJump) int {
			return a.jumpCount - b.jumpCount
		})

		for _, candidate := range fallbackCandidates {
			itemID := candidate.nodeIdx
			if !f.withinSingleKeyJump(itemID) {
				f.unselectAbstractFieldBranch(itemID)
				continue
			}

			localMembers, ok := f.localUnionMemberTypeNames(itemID, unionDefRef)
			if !ok {
				f.unselectAbstractFieldBranch(itemID)
				continue
			}

			contributedMembers := f.intersectUnionMembers(missingMembers, localMembers)
			if len(contributedMembers) == 0 {
				f.unselectAbstractFieldBranch(itemID)
				continue
			}

			f.restrictChildrenToUnionMembers(itemID, contributedMembers)
			if f.skipUnionRewriteFieldRefs != nil {
				f.skipUnionRewriteFieldRefs[f.nodes.items[itemID].FieldRef] = struct{}{}
			}
			missingMembers = f.missingUnionMembers(missingMembers, contributedMembers)
		}
	}
}

func (f *DataSourceFilter) fullRequestedUnionMemberTypeNames(itemID int, unionDefRef int) []string {
	item := f.nodes.items[itemID]
	requestedMembers := f.requestedUnionMemberTypeNames(item.FieldRef, unionDefRef)

	if f.abstractFieldRequestedMembers == nil {
		f.abstractFieldRequestedMembers = make(map[string][]string)
	}

	existingMembers := f.abstractFieldRequestedMembers[item.Path]
	if len(existingMembers) > len(requestedMembers) {
		return existingMembers
	}

	if len(requestedMembers) > len(existingMembers) {
		f.abstractFieldRequestedMembers[item.Path] = requestedMembers
	}

	return requestedMembers
}

func (f *DataSourceFilter) fieldReturnsUnionType(itemID int) (unionDefRef int, ok bool) {
	item := f.nodes.items[itemID]
	if item.FieldName == typeNameField {
		return -1, false
	}

	enclosingNode, ok := f.definition.NodeByNameStr(item.TypeName)
	if !ok {
		return -1, false
	}

	fieldTypeNode, ok := f.definition.FieldTypeNode([]byte(item.FieldName), enclosingNode)
	if !ok || fieldTypeNode.Kind != ast.NodeKindUnionTypeDefinition {
		return -1, false
	}

	return fieldTypeNode.Ref, true
}

func (f *DataSourceFilter) requestedUnionMemberTypeNames(fieldRef int, unionDefRef int) []string {
	selectionSetRef, ok := f.operation.FieldSelectionSet(fieldRef)
	if !ok {
		return nil
	}

	unionMemberTypeNames, ok := f.definition.UnionTypeDefinitionMemberTypeNames(unionDefRef)
	if !ok {
		return nil
	}

	allowedMembers := make(map[string]struct{}, len(unionMemberTypeNames))
	for _, typeName := range unionMemberTypeNames {
		allowedMembers[typeName] = struct{}{}
	}

	out := make([]string, 0, len(unionMemberTypeNames))
	f.collectRequestedUnionMemberTypeNames(selectionSetRef, allowedMembers, &out)
	return out
}

func (f *DataSourceFilter) collectRequestedUnionMemberTypeNames(selectionSetRef int, allowedMembers map[string]struct{}, out *[]string) {
	inlineFragmentSelectionRefs := f.operation.SelectionSetInlineFragmentSelections(selectionSetRef)
	for _, inlineFragmentSelectionRef := range inlineFragmentSelectionRefs {
		inlineFragmentRef := f.operation.Selections[inlineFragmentSelectionRef].Ref
		typeCondition := f.operation.InlineFragmentTypeConditionNameString(inlineFragmentRef)
		definitionNode, ok := f.definition.NodeByNameStr(typeCondition)
		if !ok {
			continue
		}

		switch definitionNode.Kind {
		case ast.NodeKindObjectTypeDefinition:
			if _, ok := allowedMembers[typeCondition]; ok && !slices.Contains(*out, typeCondition) {
				*out = append(*out, typeCondition)
			}
		case ast.NodeKindInterfaceTypeDefinition:
			implementingTypeNames, _ := f.definition.InterfaceTypeDefinitionImplementedByObjectWithNames(definitionNode.Ref)
			for _, typeName := range implementingTypeNames {
				if _, ok := allowedMembers[typeName]; ok && !slices.Contains(*out, typeName) {
					*out = append(*out, typeName)
				}
			}
		case ast.NodeKindUnionTypeDefinition:
			memberTypeNames, _ := f.definition.UnionTypeDefinitionMemberTypeNames(definitionNode.Ref)
			for _, typeName := range memberTypeNames {
				if _, ok := allowedMembers[typeName]; ok && !slices.Contains(*out, typeName) {
					*out = append(*out, typeName)
				}
			}
		}

		nestedSelectionSetRef, ok := f.operation.InlineFragmentSelectionSet(inlineFragmentRef)
		if ok {
			f.collectRequestedUnionMemberTypeNames(nestedSelectionSetRef, allowedMembers, out)
		}
	}
}

func (f *DataSourceFilter) localUnionMemberTypeNames(itemID int, unionDefRef int) ([]string, bool) {
	item := f.nodes.items[itemID]
	ds, ok := f.dataSourceByHash(item.DataSourceHash)
	if !ok {
		return nil, false
	}

	upstreamDefinition, ok := ds.UpstreamSchema()
	if !ok {
		return nil, false
	}

	enclosingNode, ok := upstreamDefinition.NodeByNameStr(item.TypeName)
	if !ok {
		return nil, false
	}

	fieldTypeNode, ok := upstreamDefinition.FieldTypeNode([]byte(item.FieldName), enclosingNode)
	if !ok {
		return nil, false
	}

	unionTypeName := f.definition.UnionTypeDefinitionNameString(unionDefRef)
	unionTypeNamesFromDefinition, _ := f.definition.UnionTypeDefinitionMemberTypeNames(unionDefRef)
	fieldTypeName := upstreamDefinition.NodeNameString(fieldTypeNode)
	if unionTypeName != fieldTypeName {
		if slices.Contains(unionTypeNamesFromDefinition, fieldTypeName) {
			return []string{fieldTypeName}, true
		}
		return nil, false
	}

	unionNode, ok := upstreamDefinition.NodeByNameStr(unionTypeName)
	if !ok || unionNode.Kind != ast.NodeKindUnionTypeDefinition {
		return nil, false
	}

	unionMemberTypeNames, ok := upstreamDefinition.UnionTypeDefinitionMemberTypeNames(unionNode.Ref)
	if !ok {
		return nil, false
	}

	return unionMemberTypeNames, true
}

func (f *DataSourceFilter) dataSourceByHash(hash DSHash) (DataSource, bool) {
	for _, ds := range f.dataSources {
		if ds.Hash() == hash {
			return ds, true
		}
	}
	return nil, false
}

func (f *DataSourceFilter) datasourceJumpScore(itemID int) int {
	score := 0
	current := itemID

	for {
		if f.nodes.items[current].requiresKey != nil {
			score += len(f.nodes.items[current].requiresKey.Jumps) + 1
		}
		if f.nodes.items[current].IsRootNode && !IsMutationOrQueryRootType(f.nodes.items[current].TypeName) {
			score++
		}

		parentIdx, ok := f.nodes.parentNodeOnSameSource(current)
		if !ok {
			return score
		}

		current = parentIdx
	}
}

func (f *DataSourceFilter) withinSingleKeyJump(itemID int) bool {
	current := itemID
	for {
		if f.nodes.items[current].requiresKey != nil && len(f.nodes.items[current].requiresKey.Jumps) > 1 {
			return false
		}

		parentIdx, ok := f.nodes.parentNodeOnSameSource(current)
		if !ok {
			return true
		}
		current = parentIdx
	}
}

func (f *DataSourceFilter) intersectUnionMembers(a, b []string) []string {
	out := make([]string, 0, len(a))
	for _, typeName := range a {
		if slices.Contains(b, typeName) && !slices.Contains(out, typeName) {
			out = append(out, typeName)
		}
	}
	return out
}

func (f *DataSourceFilter) missingUnionMembers(requested, covered []string) []string {
	out := make([]string, 0, len(requested))
	for _, typeName := range requested {
		if !slices.Contains(covered, typeName) {
			out = append(out, typeName)
		}
	}
	return out
}

func (f *DataSourceFilter) unselectAbstractFieldBranch(itemID int) {
	f.nodes.items[itemID].unselect()
	f.unselectChildrenOnDatasource(itemID)
	f.unselectEmptyAncestorsOnDatasource(itemID)
}

func (f *DataSourceFilter) restrictChildrenToUnionMembers(itemID int, allowedMembers []string) {
	allowed := make(map[string]struct{}, len(allowedMembers))
	for _, typeName := range allowedMembers {
		allowed[typeName] = struct{}{}
	}
	f.restrictChildTreeToUnionMembers(itemID, allowed)
}

func (f *DataSourceFilter) restrictChildTreeToUnionMembers(itemID int, allowedMembers map[string]struct{}) {
	item := f.nodes.items[itemID]
	treeNode := f.nodes.treeNode(itemID)

	for _, child := range treeNode.GetChildren() {
		for _, childID := range child.GetData() {
			childItem := f.nodes.items[childID]
			if childItem.DataSourceHash != item.DataSourceHash {
				continue
			}

			_, allowedMember := allowedMembers[childItem.TypeName]
			if childItem.TypeName != item.TypeName && !allowedMember && !childItem.isTypeName && !childItem.IsRequiredKeyField {
				childItem.unselect()
				f.unselectChildrenOnDatasource(childID)
				continue
			}

			if !childItem.IsExternal || childItem.IsProvided {
				childItem.selectWithReason(ReasonStage2SameSourceNodeOfSelectedParent, f.enableSelectionReasons)
			}

			f.restrictChildTreeToUnionMembers(childID, allowedMembers)
		}
	}
}

func (f *DataSourceFilter) unselectChildrenOnDatasource(itemID int) {
	item := f.nodes.items[itemID]
	treeNode := f.nodes.treeNode(itemID)

	for _, child := range treeNode.GetChildren() {
		for _, childID := range child.GetData() {
			if f.nodes.items[childID].DataSourceHash != item.DataSourceHash {
				continue
			}

			f.nodes.items[childID].unselect()
			f.unselectChildrenOnDatasource(childID)
		}
	}
}

func (f *DataSourceFilter) unselectEmptyAncestorsOnDatasource(itemID int) {
	parentIdx, ok := f.nodes.parentNodeOnSameSource(itemID)
	for ok {
		if !f.shouldUnselectEmptyAncestor(parentIdx) {
			return
		}

		f.nodes.items[parentIdx].unselect()
		parentIdx, ok = f.nodes.parentNodeOnSameSource(parentIdx)
	}
}

func (f *DataSourceFilter) shouldUnselectEmptyAncestor(i int) bool {
	item := f.nodes.items[i]
	if item.IsOrphan || !item.Selected || item.IsLeaf || item.isTypeName || item.IsRequiredKeyField {
		return false
	}

	for _, child := range f.nodes.childNodesOnSameSource(i) {
		if f.nodes.items[child].Selected {
			return false
		}
	}

	return !f.nodeCouldProvideKeysToChildNodes(i)
}

func (f *DataSourceFilter) applyLandedTo(landedTo map[int]DSHash) {
	if landedTo == nil {
		return
	}

	for fieldRef, dsHash := range landedTo {
		treeNodeID := TreeNodeID(fieldRef)

		node, ok := f.nodes.responseTree.Find(treeNodeID)
		if !ok {
			// no such node in the tree
			// we may removed it as orphaned
			continue
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

// collectNodes collects and organizes information about
// which data sources (subgraphs) can resolve each field and
// builds a "jump graph" to enable navigation between data sources using federation @key directives.
func (f *DataSourceFilter) collectNodes() {
	if f.nodesCollector == nil {
		f.nodesCollector = &nodesCollector{
			operation:      f.operation,
			definition:     f.definition,
			dataSources:    f.dataSources,
			nodes:          f.nodes,
			report:         f.report,
			maxConcurrency: f.maxDataSourceCollectorsConcurrency,
			seenKeys:       make(map[SeenKeyPath]struct{}),
			fieldInfo:      make(map[int]fieldInfo),
			newFieldRefs:   f.newFieldRefs,
		}

		f.nodesCollector.initVisitors()
	}

	keysInfo := f.nodesCollector.CollectNodes()

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

		keysPerDS[keyInfo.DSHash] = append(keysPerDS[keyInfo.DSHash], keyInfo.Keys...)
		keysForPathForTypename[keyIndex] = keysPerDS

		f.dsHashesHavingKeys[keyInfo.DSHash] = struct{}{}
	}

	if f.jumpsForPathForTypename == nil {
		f.jumpsForPathForTypename = make(map[KeyIndex]*DataSourceJumpsGraph)
	}

	usedDsHashes := make([]DSHash, 0, len(f.dsHashesHavingKeys))
	// iterate over datasources to have a deterministic order
	for _, ds := range f.dataSources {
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
		if f.nodes.items[i].IsOrphan {
			continue
		}

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
			// parent can't be selected because it is fully external
			// we will have to find another way to get to this node
			break
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

func hasPathBetweenDs(jumps *DataSourceJumpsGraph, from, to DSHash, includeFallback bool) (bestPath *SourceConnection, exists bool) {
	possiblePaths, exists := jumps.getPaths(from, to, includeFallback)
	if !exists {
		return nil, false
	}

	var directs []SourceConnection
	var indirects []SourceConnection
	var fallbackDirects []SourceConnection
	var fallbackIndirects []SourceConnection

	for _, path := range possiblePaths {
		if sourceConnectionUsesFallback(path) {
			if path.Type == SourceConnectionTypeDirect {
				fallbackDirects = append(fallbackDirects, path)
				continue
			}
			fallbackIndirects = append(fallbackIndirects, path)
			continue
		}

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

	if bestPath := shortestConnection(indirects); bestPath != nil {
		return bestPath, true
	}

	if len(fallbackDirects) > 0 {
		return &fallbackDirects[0], true
	}

	if bestPath := shortestConnection(fallbackIndirects); bestPath != nil {
		return bestPath, true
	}

	return nil, false
}

func shortestConnection(paths []SourceConnection) *SourceConnection {
	var bestPath *SourceConnection

	for _, path := range paths {
		if bestPath == nil {
			bestPath = &path
			continue
		}

		if len(path.Jumps) < len(bestPath.Jumps) {
			bestPath = &path
		}
	}

	return bestPath
}

func sourceConnectionUsesFallback(path SourceConnection) bool {
	for _, jump := range path.Jumps {
		if jump.Fallback {
			return true
		}
	}

	return false
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
	if len(selectedParentHashes) == 0 && currentNode.onFragment {
		selectedParentHashes, hasSelectedParentOnSameDataSource = f.selectedAncestorHashes(itemIdx, currentNodeDsHash)
		if hasSelectedParentOnSameDataSource {
			return
		}
	}

	jumpsForTypename, exists := f.jumpsForPathAndTypeName(currentNode.ParentPath, currentNodeTypeName)
	if !exists {
		return
	}

	for _, selectedParentHash := range selectedParentHashes {
		path, exists := hasPathBetweenDs(jumpsForTypename, selectedParentHash, currentNodeDsHash, f.allowFallbackKeyJumps)
		if exists {
			currentNode.requiresKey = path
			currentNode.requiresFallbackKey = false
			break
		}

		if f.allowFallbackKeyJumps {
			continue
		}
		fallbackPath, fallbackExists := hasPathBetweenDs(jumpsForTypename, selectedParentHash, currentNodeDsHash, true)
		if !fallbackExists {
			continue
		}
		if !sourceConnectionUsesFallback(*fallbackPath) {
			continue
		}
		if sourceConnectionRequiresMissingFallbackKeyField(fallbackPath, currentNode) {
			continue
		}
		currentNode.requiresFallbackKey = true
	}
}

func (f *DataSourceFilter) selectedAncestorHashes(itemIdx int, currentNodeDsHash DSHash) (selectedParentHashes []DSHash, hasSelectedParentOnSameDataSource bool) {
	node := f.nodes.treeNode(itemIdx)
	for parent := node.GetParent(); parent != nil && parent.GetID() != treeRootID; parent = parent.GetParent() {
		for _, parentIdx := range parent.GetData() {
			if !f.nodes.items[parentIdx].Selected {
				continue
			}
			if f.nodes.items[parentIdx].DataSourceHash == currentNodeDsHash {
				return nil, true
			}
			selectedParentHashes = append(selectedParentHashes, f.nodes.items[parentIdx].DataSourceHash)
		}
		if len(selectedParentHashes) > 0 {
			return selectedParentHashes, false
		}
	}
	return nil, false
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
	for id, treeNode := range TraverseBFS(f.nodes.responseTree) {
		// f.nodes.printNodes("nodes")
		// fmt.Println()

		if id == treeRootID {
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

		// Select a node based on a check for the selected parent of a current node or its duplicates
		if f.checkNodes(itemIDs, f.checkNodeParent, func(i int) (skip bool) {
			if f.nodes.items[i].IsLeaf {
				return false
			}

			if !f.nodes.items[i].IsRootNode {
				return false
			}

			if !f.couldProvideChildFields(i) && !f.nodeCouldProvideKeysToChildNodes(i) {
				return true
			}

			return false
		}) {
			continue
		}

		// we are checking if we have selected any child fields on the same datasource
		// it means that the child was unique, so we could select the parent of such a child
		if f.checkNodes(itemIDs, f.checkNodeChilds, func(i int) (skip bool) {
			// do not evaluate childs for the leaf nodes
			return f.nodes.items[i].IsLeaf
		}) {
			continue
		}

		// we are checking if we have selected any sibling fields on the same datasource
		// if a sibling is selected on the same datasource, we could select a current node
		if f.checkNodes(itemIDs, f.checkNodeSiblings, func(i int) (skip bool) {
			// we skip non-leaf nodes which could not provide any child selections
			if !f.nodes.items[i].IsLeaf && !f.couldProvideChildFields(i) {
				return true
			}

			// we should not select a __typename field based on a sibling, unless it is on a root query type
			return f.nodes.items[i].isTypeName && !IsMutationOrQueryRootType(f.nodes.items[i].TypeName)
		}) {
			continue
		}

		// if after all checks node was not selected,
		// we need a couple more checks

		// Prefer a datasource that explicitly provides fields below an abstract root field.
		// This keeps interface-typed @provides selections together before falling back to root order.
		if f.checkNodes(itemIDs,
			func(i int) bool {
				return f.selectWithExternalCheck(i, ReasonStage3SelectNodeHavingPossibleChildsOnSameDataSource)
			},
			func(i int) (skip bool) {
				if !f.nodes.items[i].IsRootNode {
					return true
				}
				if treeNode.GetParentID() != treeRootID {
					return true
				}
				if !f.fieldReturnsAbstractType(f.nodes.items[i].TypeName, f.nodes.items[i].FieldName) {
					return true
				}
				return !f.hasProvidedChildOnSameSource(i)
			}) {
			continue
		}

		// 1. Lookup in duplicates for root nodes with enabled reference resolver
		// in case current node suggestion is an entity root node, and it contains a key with disabled resolver
		// we could not select such a node, because we could not jump to the subgraph which do not have a reference resolver,
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

				// when a node is a root query node, we will not have a parent
				// so we need to check if the parent node id is not a root of a tree
				if treeNode.GetParentID() != treeRootID {
					// we need to check if the node with the enabled resolver could actually get a key from the parent node
					if !f.isSelectedParentCouldProvideKeysForCurrentNode(i) {
						return true
					}
				}

				// if node is not a leaf, we need to check if it is possible to get any fields (not counting __typename) from this datasource
				// it may be the case that all fields belong to different datasource, but this node could serve as the connection point
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

func (f *DataSourceFilter) fieldReturnsAbstractType(typeName, fieldName string) bool {
	node, exists := f.definition.NodeByNameStr(typeName)
	if !exists {
		return false
	}

	var fieldDefinitionRef int
	var ok bool
	switch node.Kind {
	case ast.NodeKindObjectTypeDefinition:
		fieldDefinitionRef, ok = f.definition.ObjectTypeDefinitionFieldWithName(node.Ref, []byte(fieldName))
	case ast.NodeKindInterfaceTypeDefinition:
		fieldDefinitionRef, ok = f.definition.InterfaceTypeDefinitionFieldWithName(node.Ref, []byte(fieldName))
	default:
		return false
	}
	if !ok {
		return false
	}

	fieldTypeName := f.definition.FieldDefinitionTypeNameBytes(fieldDefinitionRef)
	fieldTypeNode, exists := f.definition.NodeByName(fieldTypeName)
	if !exists {
		return false
	}

	return fieldTypeNode.Kind == ast.NodeKindInterfaceTypeDefinition || fieldTypeNode.Kind == ast.NodeKindUnionTypeDefinition
}

func (f *DataSourceFilter) hasProvidedChildOnSameSource(idx int) bool {
	for _, childIdx := range f.nodes.childNodesOnSameSource(idx) {
		if f.nodes.items[childIdx].IsProvided {
			return true
		}
	}
	return false
}

func (f *DataSourceFilter) findPossibleParents(i int) (parentIds []int) {
	nodesIdsToSelect := make([]int, 0, 2)

	parentIdx, _ := f.nodes.parentNodeOnSameSource(i)
	for parentIdx != -1 {
		if f.nodes.items[parentIdx].IsExternal && !f.nodes.items[i].IsProvided {
			break
		}

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

	path, exists := hasPathBetweenDs(jumpsForTypename, f.nodes.items[parentIdx].DataSourceHash, f.nodes.items[idx].DataSourceHash, f.allowFallbackKeyJumps)
	if !exists {
		return false
	}

	if setRequiresKey {
		f.nodes.items[idx].requiresKey = path
	}

	return true
}

func sourceConnectionRequiresMissingFallbackKeyField(path *SourceConnection, node *NodeSuggestion) bool {
	if path == nil || node == nil {
		return false
	}

	fieldPath := node.Path

	for _, jump := range path.Jumps {
		if !jump.Fallback {
			continue
		}

		targetContainsField := false
		for _, keyPath := range jump.FieldPaths {
			if keyPath.Path == fieldPath {
				targetContainsField = true
				break
			}
		}
		if !targetContainsField {
			continue
		}

		for _, keyPath := range jump.SourcePaths {
			if keyPath.Path == fieldPath {
				return false
			}
		}

		return true
	}

	return false
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

	hasSelectableFields := false
	for _, i := range nodesIds {
		if f.nodes.items[i].isTypeName {
			// we have to omit __typename field
			// to not be in a situation when all fields are external but __typename is selectable
			continue
		}

		if f.nodes.items[i].IsExternal && !f.nodes.items[i].IsProvided {
			continue
		}

		if f.nodes.items[i].IsLeaf {
			// when the node is a leaf, it could be selected
			hasSelectableFields = true
			break
		}

		// for non-leaf nodes, we need to check again if they could provide any child fields
		// or keys to the nested child nodes
		// this ensures bigger chains of non-leaf nodes selected
		if f.couldProvideChildFields(i) || f.nodeCouldProvideKeysToChildNodes(i) {
			hasSelectableFields = true
			break
		}
	}

	return hasSelectableFields
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
