package plan

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type configurationVisitor struct {
	ctx   context.Context
	debug bool

	operationName         string
	operation, definition *ast.Document
	walker                *astvisitor.Walker

	dataSources []DataSourceConfiguration
	planners    []*plannerConfiguration
	fetches     []objectFetchConfiguration

	nodeSuggestions    NodeSuggestions        // nodeSuggestions holds information about suggested data sources for each field
	currentFetchID     int                    // currentFetchID is used to generate serial fetch IDs for each fetch
	parentTypeNodes    []ast.Node             // parentTypeNodes is a stack of parent type nodes - used to determine if the parent is abstract
	arrayFields        []arrayField           // arrayFields is a stack of array fields - used to plan nested queries
	selectionSetRefs   []int                  // selectionSetRefs is a stack of selection set refs - used to add a required fields
	skipFieldsRefs     []int                  // skipFieldsRefs holds required field refs which should not be added to user response
	missingPathTracker map[string]missingPath // missingPathTracker is a map of paths which will be added on secondary runs
	addedPathTracker   []pathConfiguration    // addedPathTracker is a list of paths which were added

	pendingRequiredFields map[int][]string // pendingRequiredFields is a map[selectionSetRef][]RequiredFieldsSelectionSet
	handledRequires       map[int]struct{} // handledRequires is a map[FieldRef] of already processed fields which has @requires directive

	secondaryRun bool // secondaryRun is a flag to indicate that we're running the planner not the first time
	hasNewFields bool // hasNewFields is used to determine if we need to run the planner again. It will be true in case required fields were added
}

type arrayField struct {
	fieldRef  int
	fieldPath string
}

type missingPath struct {
	path                  string
	precedingRootNodePath string
	dsHash                DSHash
}

type objectFetchConfiguration struct {
	object             *resolve.Object
	trigger            *resolve.GraphQLSubscriptionTrigger
	planner            DataSourcePlanner
	isSubscription     bool
	fieldRef           int
	fieldDefinitionRef int
	fetchID            int
}

func (c *configurationVisitor) currentSelectionSet() int {
	if len(c.selectionSetRefs) == 0 {
		return ast.InvalidRef
	}

	return c.selectionSetRefs[len(c.selectionSetRefs)-1]
}

func (c *configurationVisitor) plannerPathType(path string) PlannerPathType {
	for i := len(c.arrayFields) - 1; i >= 0; i-- {
		arrayPath := c.arrayFields[i].fieldPath
		switch {
		case path == arrayPath:
			return PlannerPathArrayItem
		case strings.HasPrefix(path, arrayPath+"."):
			return PlannerPathNestedInArray
		}
	}

	return PlannerPathObject
}

func (c *configurationVisitor) addArrayField(fieldRef int, currentPath string) {
	var (
		fieldDefRef int
		ok          bool
	)

	switch c.walker.EnclosingTypeDefinition.Kind {
	case ast.NodeKindObjectTypeDefinition:
		fieldDefRef, ok = c.definition.ObjectTypeDefinitionFieldWithName(c.walker.EnclosingTypeDefinition.Ref, c.operation.FieldNameBytes(fieldRef))
		if !ok {
			return
		}
	case ast.NodeKindInterfaceTypeDefinition:
		fieldDefRef, ok = c.definition.InterfaceTypeDefinitionFieldWithName(c.walker.EnclosingTypeDefinition.Ref, c.operation.FieldNameBytes(fieldRef))
		if !ok {
			return
		}
	default:
		return
	}

	if c.definition.TypeIsList(c.definition.FieldDefinitionType(fieldDefRef)) {
		c.arrayFields = append(c.arrayFields, arrayField{
			fieldRef:  fieldRef,
			fieldPath: currentPath,
		})
	}
}

func (c *configurationVisitor) removeArrayField(fieldRef int) {
	if len(c.arrayFields) == 0 {
		return
	}

	if c.arrayFields[len(c.arrayFields)-1].fieldRef == fieldRef {
		c.arrayFields = c.arrayFields[:len(c.arrayFields)-1]
	}
}

func (c *configurationVisitor) addPath(plannerIdx int, configuration pathConfiguration) {
	if c.debug {
		if pp, ok := c.planners[plannerIdx].planner.(DataSourceDebugger); ok {
			pp.DebugPrint("[configurationVisitor.addPath] parentPath:", "path:", configuration.String())
		}
	}

	configuration.depth = c.walker.Depth

	c.planners[plannerIdx].addPath(configuration)

	c.saveAddedPath(configuration)
}

func (c *configurationVisitor) saveAddedPath(configuration pathConfiguration) {
	c.addedPathTracker = append(c.addedPathTracker, configuration)

	c.removeMissingPath(configuration.path)
}

func (c *configurationVisitor) addedPathDSHash(path string) (hash DSHash, ok bool) {
	for i := range c.addedPathTracker {
		if c.addedPathTracker[i].path == path {
			return c.addedPathTracker[i].dsHash, true
		}
	}
	return 0, false
}

func (c *configurationVisitor) isPathAddedFor(path string, hash DSHash) bool {
	for i := range c.addedPathTracker {
		if c.addedPathTracker[i].path == path && c.addedPathTracker[i].dsHash == hash {
			return true
		}
	}
	return false
}

func (c *configurationVisitor) findPreviousRootPath(currentPath string) (previousRootPath string, found bool) {
	if len(c.addedPathTracker) == 0 {
		return "", false
	}

	for i := len(c.addedPathTracker) - 1; i >= 0; i-- {
		if strings.HasPrefix(currentPath, c.addedPathTracker[i].path) && c.addedPathTracker[i].isRootNode {
			return c.addedPathTracker[i].path, true
		}
	}
	return "", false
}

func (c *configurationVisitor) addMissingPath(path string, parentPath string, hash DSHash) {
	c.missingPathTracker[path] = missingPath{
		path:                  path,
		precedingRootNodePath: parentPath,
		dsHash:                hash,
	}
}

func (c *configurationVisitor) hasMissingPaths() bool {
	return len(c.missingPathTracker) > 0
}

func (c *configurationVisitor) removeMissingPath(path string) {
	delete(c.missingPathTracker, path)
}

func (c *configurationVisitor) hasMissingPathWithParentPath(parentPath string) bool {
	if !c.hasMissingPaths() {
		return false
	}

	for _, missingPath := range c.missingPathTracker {
		if missingPath.precedingRootNodePath == parentPath {
			return true
		}
	}

	return false
}

func (c *configurationVisitor) debugPrint(args ...any) {
	if !c.debug {
		return
	}

	printArgs := []any{"[configurationVisitor]: "}
	printArgs = append(printArgs, args...)
	fmt.Println(printArgs...)
}

func (c *configurationVisitor) EnterDocument(operation, definition *ast.Document) {
	c.hasNewFields = false

	if c.selectionSetRefs == nil {
		c.selectionSetRefs = make([]int, 0, 8)
	} else {
		c.selectionSetRefs = c.selectionSetRefs[:0]
	}

	if c.arrayFields == nil {
		c.arrayFields = make([]arrayField, 0, 4)
	} else {
		c.arrayFields = c.arrayFields[:0]
	}

	if c.secondaryRun {
		return
	}

	c.operation, c.definition = operation, definition
	c.currentFetchID = -1
	c.parentTypeNodes = c.parentTypeNodes[:0]
	if c.planners == nil {
		c.planners = make([]*plannerConfiguration, 0, 8)
	} else {
		c.planners = c.planners[:0]
	}
	if c.fetches == nil {
		c.fetches = []objectFetchConfiguration{}
	} else {
		c.fetches = c.fetches[:0]
	}
	if c.skipFieldsRefs == nil {
		c.skipFieldsRefs = make([]int, 0, 8)
	} else {
		c.skipFieldsRefs = c.skipFieldsRefs[:0]
	}

	c.missingPathTracker = make(map[string]missingPath)
	c.addedPathTracker = make([]pathConfiguration, 0, 8)

	c.pendingRequiredFields = make(map[int][]string)
	c.handledRequires = make(map[int]struct{})
}

func (c *configurationVisitor) LeaveDocument(operation, definition *ast.Document) {
}

func (c *configurationVisitor) EnterOperationDefinition(ref int) {
	operationName := c.operation.OperationDefinitionNameString(ref)
	if c.operationName != operationName {
		c.walker.SkipNode()
		return
	}
}

func (c *configurationVisitor) EnterSelectionSet(ref int) {
	c.debugPrint("EnterSelectionSet ref:", ref)
	c.selectionSetRefs = append(c.selectionSetRefs, ref)
	c.parentTypeNodes = append(c.parentTypeNodes, c.walker.EnclosingTypeDefinition)

	// When selection is the inline fragment
	// We have to add a fragment path to the planner paths
	ancestor := c.walker.Ancestor()
	if ancestor.Kind == ast.NodeKindInlineFragment {
		parentPath := c.walker.Path[:len(c.walker.Path)-1].DotDelimitedString()
		currentPath := c.walker.Path.DotDelimitedString()
		typeName := c.operation.InlineFragmentTypeConditionNameString(ancestor.Ref)

		for i, planner := range c.planners {
			if !planner.hasPath(parentPath) {
				continue
			}

			hasRootNode := planner.dataSourceConfiguration.HasRootNodeWithTypename(typeName)
			hasChildNode := planner.dataSourceConfiguration.HasChildNodeWithTypename(typeName)
			if !(hasRootNode || hasChildNode) {
				continue
			}

			if planner.hasPath(currentPath) {
				continue
			}

			path := pathConfiguration{
				path:             currentPath,
				shouldWalkFields: true,
				dsHash:           planner.dataSourceConfiguration.Hash(),
				fieldRef:         ast.InvalidRef,
				pathType:         PathTypeFragment,
			}

			c.addPath(i, path)
		}
	}
}

func (c *configurationVisitor) LeaveSelectionSet(ref int) {
	c.debugPrint("LeaveSelectionSet ref:", ref)
	c.processPendingRequiredFields(ref)
	c.selectionSetRefs = c.selectionSetRefs[:len(c.selectionSetRefs)-1]
	c.parentTypeNodes = c.parentTypeNodes[:len(c.parentTypeNodes)-1]
}

func (c *configurationVisitor) EnterField(ref int) {
	fieldName := c.operation.FieldNameUnsafeString(ref)
	fieldAliasOrName := c.operation.FieldAliasOrNameString(ref)
	typeName := c.walker.EnclosingTypeDefinition.NameString(c.definition)

	c.debugPrint("EnterField ref:", ref, "fieldName:", fieldName, "typeName:", typeName)

	parentPath := c.walker.Path.DotDelimitedString()
	// we need to also check preceding path for inline fragments
	// as for the field within inline fragment the parent path will include type condition in a path
	// but planner path still will not include it
	// this required to not produce multiple planners for the inline fragments
	precedingParentPath := parentPath
	if c.walker.Path[len(c.walker.Path)-1].Kind == ast.InlineFragmentName {
		precedingParentPath = c.walker.Path[:len(c.walker.Path)-1].DotDelimitedString()
	}

	currentPath := parentPath + "." + fieldAliasOrName

	c.addArrayField(ref, currentPath)
	c.handleProvidesSuggestions(ref, typeName, fieldName, currentPath)

	root := c.walker.Ancestors[0]
	if root.Kind != ast.NodeKindOperationDefinition {
		return
	}
	isSubscription := c.isSubscription(root.Ref, currentPath)

	plannerIdx, planned := c.planWithExistingPlanners(ref, typeName, fieldName, currentPath, parentPath, precedingParentPath)
	if planned {
		c.handleRequirements(plannerIdx, parentPath, currentPath, typeName, fieldName, ref)
		return
	}

	plannerIdx, planned = c.addNewPlanner(ref, typeName, fieldName, currentPath, parentPath, isSubscription)
	if planned {
		c.handleRequirements(plannerIdx, parentPath, currentPath, typeName, fieldName, ref)
		return
	}

	c.handleMissingPath(typeName, fieldName, currentPath)
}

func (c *configurationVisitor) handleRequirements(plannerIdx int, parentPath string, currentPath string, typeName, fieldName string, fieldRef int) {
	plannerConfig := c.planners[plannerIdx]
	dsHash := plannerConfig.dataSourceConfiguration.Hash()

	parentDSHash, ok := c.addedPathDSHash(parentPath)
	if ok && dsHash != parentDSHash {
		// add required fields for type (@key)
		c.handleFieldsRequiredByKey(plannerIdx, plannerConfig, parentPath, typeName)
	}

	// add required fields for field and type (@requires)
	c.handleFieldRequiredByRequires(plannerConfig, currentPath, typeName, fieldName, fieldRef)
}

func (c *configurationVisitor) handleProvidesSuggestions(ref int, typeName, fieldName, currentPath string) {
	dsHash, ok := c.nodeSuggestions.HasSuggestionForPath(typeName, fieldName, currentPath)
	if !ok {
		return
	}

	var providesCfg *FederationFieldConfiguration
	for _, ds := range c.dataSources {
		if ds.Hash() != dsHash {
			continue
		}

		found := false
		for _, provide := range ds.FederationMetaData.Provides {
			if provide.TypeName == typeName && provide.FieldName == fieldName {
				providesCfg = &provide
				found = true
				break
			}
		}
		if found {
			break
		}

	}

	if providesCfg == nil {
		return
	}

	if c.walker.EnclosingTypeDefinition.Kind != ast.NodeKindObjectTypeDefinition {
		return
	}

	fieldDefRef, ok := c.definition.ObjectTypeDefinitionFieldWithName(c.walker.EnclosingTypeDefinition.Ref, c.operation.FieldNameBytes(ref))
	if !ok {
		return
	}
	fieldTypeName := c.definition.FieldDefinitionTypeNameString(fieldDefRef)

	key, report := RequiredFieldsFragment(fieldTypeName, providesCfg.SelectionSet, false)
	if report.HasErrors() {
		c.walker.StopWithInternalErr(fmt.Errorf("failed to parse provides fields for %s", typeName))
	}

	input := &providesInput{
		key:        key,
		definition: c.definition,
		report:     report,
		parentPath: currentPath,
		DSHash:     dsHash,
	}
	suggestions := providesSuggestions(input)
	if report.HasErrors() {
		c.walker.StopWithInternalErr(fmt.Errorf("failed to get provides suggestions for %s", typeName))
	}

	for i := range c.planners {
		if c.planners[i].dataSourceConfiguration.Hash() == dsHash {
			c.planners[i].providedFields = append(c.planners[i].providedFields, suggestions...)
			break
		}
	}
}

func (c *configurationVisitor) planWithExistingPlanners(ref int, typeName, fieldName, currentPath, parentPath, precedingParentPath string) (plannerIdx int, planned bool) {
	dsHash, hasSuggestion := c.nodeSuggestions.HasSuggestionForPath(typeName, fieldName, currentPath)

	for plannerIdx, plannerConfig := range c.planners {
		currentPlannerDSHash := plannerConfig.dataSourceConfiguration.Hash()

		providedDsHash, isProvided := plannerConfig.providedFields.HasSuggestionForPath(typeName, fieldName, currentPath)
		if isProvided && currentPlannerDSHash != providedDsHash {
			continue
		}

		if !isProvided && hasSuggestion && currentPlannerDSHash != dsHash {
			continue
		}

		hasRootNode := plannerConfig.dataSourceConfiguration.HasRootNode(typeName, fieldName)
		hasChildNode := plannerConfig.dataSourceConfiguration.HasChildNode(typeName, fieldName)

		if c.secondaryRun && plannerConfig.hasPath(currentPath) {
			if c.hasMissingPathWithParentPath(currentPath) {
				continue
			}
			// on the second run we need to process only new fields added by the first run
			return plannerIdx, true
		}

		planningBehaviour := plannerConfig.planner.DataSourcePlanningBehavior()

		if (plannerConfig.hasParent(parentPath) || plannerConfig.hasParent(precedingParentPath)) &&
			hasRootNode &&
			planningBehaviour.MergeAliasedRootNodes {
			// same parent + root node = root sibling

			c.addPath(plannerIdx, pathConfiguration{
				path:             currentPath,
				shouldWalkFields: true,
				typeName:         typeName,
				fieldRef:         ref,
				enclosingNode:    c.walker.EnclosingTypeDefinition,
				dsHash:           currentPlannerDSHash,
				isRootNode:       true,
			})

			return plannerIdx, true
		}
		if plannerConfig.hasPath(parentPath) || plannerConfig.hasPath(precedingParentPath) {
			if pathAdded := c.addPlannerPathForTypename(plannerIdx, currentPath, ref, fieldName, typeName, planningBehaviour); pathAdded {
				return plannerIdx, true
			}

			if isProvided || hasChildNode || (hasRootNode && planningBehaviour.MergeAliasedRootNodes) {

				// has parent path + has child node = child
				c.addPath(plannerIdx, pathConfiguration{
					path:             currentPath,
					shouldWalkFields: true,
					typeName:         typeName,
					fieldRef:         ref,
					enclosingNode:    c.walker.EnclosingTypeDefinition,
					dsHash:           currentPlannerDSHash,
					isRootNode:       hasRootNode,
				})

				return plannerIdx, true
			}

			if pathAdded := c.addPlannerPathForUnionChildOfObjectParent(plannerIdx, currentPath, ref, fieldName, typeName, planningBehaviour); pathAdded {
				return plannerIdx, true
			}

			if pathAdded := c.addPlannerPathForChildOfAbstractParent(plannerIdx, currentPath, ref, fieldName, typeName, planningBehaviour); pathAdded {
				return plannerIdx, true
			}
		}
	}

	return -1, false
}

func (c *configurationVisitor) findSuggestedDataSourceConfiguration(typeName, fieldName, currentPath string) *DataSourceConfiguration {
	dsHash, ok := c.nodeSuggestions.HasSuggestionForPath(typeName, fieldName, currentPath)
	if !ok {
		return nil
	}

	for _, dsCfg := range c.dataSources {
		if dsCfg.Hash() == dsHash {
			return &dsCfg
		}
	}

	return nil
}

func (c *configurationVisitor) findAlternativeDataSourceConfiguration(typeName, fieldName, currentPath string) *DataSourceConfiguration {
	for _, dsCfg := range c.dataSources {
		if dsCfg.HasRootNode(typeName, fieldName) && !c.isPathAddedFor(currentPath, dsCfg.Hash()) {
			return &dsCfg
		}
	}

	return nil
}

func (c *configurationVisitor) addNewPlanner(ref int, typeName, fieldName, currentPath, parentPath string, isSubscription bool) (plannerIdx int, planned bool) {
	config := c.findSuggestedDataSourceConfiguration(typeName, fieldName, currentPath)
	if config == nil {
		return -1, false
	}

	if c.isPathAddedFor(currentPath, config.Hash()) {
		config = c.findAlternativeDataSourceConfiguration(typeName, fieldName, currentPath)
		if config == nil {
			return -1, false
		}
	}

	// we should handle a new planner for a __typename
	// only when it is the first field on a query
	shouldHandleTypeName := fieldName == typeNameField && parentPath == "query"

	if !shouldHandleTypeName && !config.HasRootNode(typeName, fieldName) {
		return -1, false
	}

	fetchID := c.nextFetchID()
	planner := config.Factory.Planner(c.ctx)
	isParentAbstract := c.isParentTypeNodeAbstractType()

	currentPathConfiguration := pathConfiguration{
		path:             currentPath,
		shouldWalkFields: true,
		typeName:         typeName,
		fieldRef:         ref,
		enclosingNode:    c.walker.EnclosingTypeDefinition,
		dsHash:           config.Hash(),
		isRootNode:       true,
	}

	paths := []pathConfiguration{
		currentPathConfiguration,
	}

	if isParentAbstract {
		// if the parent is abstract, we add the parent path as well
		// this will ensure that we're walking into and out of the root inline fragments
		// otherwise, we'd only walk into the fields inside the inline fragments in the root,
		// so we'd miss the selection sets and inline fragments in the root
		paths = append([]pathConfiguration{
			{
				path:             parentPath,
				shouldWalkFields: false,
				dsHash:           config.Hash(),
				fieldRef:         ast.InvalidRef,
				pathType:         PathTypeFragment,
			},
		}, paths...)
	} else {
		// add potentially missing parent path
		// this could happen when the parent is a fragment and we walking nested selection sets
		paths = append([]pathConfiguration{
			{
				path:             parentPath,
				shouldWalkFields: true,
				dsHash:           config.Hash(),
				fieldRef:         ast.InvalidRef,
				pathType:         PathTypeParent,
			},
		}, paths...)
	}

	plannerPath := parentPath

	isParentFragment := c.walker.Path[len(c.walker.Path)-1].Kind == ast.InlineFragmentName
	if isParentFragment {
		precedingFragmentPath := c.walker.Path[:len(c.walker.Path)-1].DotDelimitedString()
		// if the parent is a fragment, we add the preceding parent path as well
		// to be able to walk selection sets in the fragment
		paths = append([]pathConfiguration{
			{
				path:             precedingFragmentPath,
				shouldWalkFields: false,
				dsHash:           config.Hash(),
				fieldRef:         ast.InvalidRef,
				pathType:         PathTypeParent,
			},
		}, paths...)

		// if the parent is a fragment, we use the preceding parent path as the planner path
		// to avoid creating multiple planners for the same upstream
		plannerPath = precedingFragmentPath
	}

	plannerConfig := &plannerConfiguration{
		dataSourceConfiguration: *config,
		parentPath:              plannerPath,
		planner:                 planner,
		paths:                   paths,
		parentPathType:          c.plannerPathType(plannerPath),
	}

	fieldDefinition, ok := c.walker.FieldDefinition(ref)
	if !ok {
		return -1, false
	}

	c.planners = append(c.planners, plannerConfig)
	c.fetches = append(c.fetches, objectFetchConfiguration{
		planner:            planner,
		isSubscription:     isSubscription,
		fieldRef:           ref,
		fieldDefinitionRef: fieldDefinition,
		fetchID:            fetchID,
	})

	c.saveAddedPath(currentPathConfiguration)
	return len(c.planners) - 1, true
}

// handleMissingPath - records missing path for the case when we don't yet have a planner for the field
func (c *configurationVisitor) handleMissingPath(typeName string, fieldName string, currentPath string) {
	suggestedDataSourceHash, ok := c.nodeSuggestions.HasSuggestionForPath(typeName, fieldName, currentPath)
	if ok {
		parentPath, found := c.findPreviousRootPath(currentPath)
		if found {
			c.addMissingPath(currentPath, parentPath, suggestedDataSourceHash)
			return
		}
	}

	c.walker.StopWithInternalErr(errors.Wrap(fmt.Errorf("could not plan a field %s.%s on a path %s", typeName, fieldName, currentPath), "configurationVisitor.handleMissingPath"))
}

func (c *configurationVisitor) LeaveField(ref int) {
	c.removeArrayField(ref)

	fieldName := c.operation.FieldNameUnsafeString(ref)
	fieldAliasOrName := c.operation.FieldAliasOrNameString(ref)
	typeName := c.walker.EnclosingTypeDefinition.NameString(c.definition)
	c.debugPrint("LeaveField ref:", ref, "fieldName:", fieldName, "typeName:", typeName)

	if !c.secondaryRun {
		// we should evaluate exit paths only on the second run
		return
	}

	// fieldAliasOrName := c.operation.FieldAliasOrNameString(ref)
	parent := c.walker.Path.DotDelimitedString()
	current := parent + "." + fieldAliasOrName
	for i, planner := range c.planners {
		if planner.hasPath(current) && !planner.hasPathPrefix(current) {
			c.planners[i].setPathExit(current)
			return
		}
	}
}

func (c *configurationVisitor) addPlannerPathForUnionChildOfObjectParent(
	plannerIndex int, currentPath string, fieldRef int, fieldName string, typeName string, planningBehaviour DataSourcePlanningBehavior,
) (pathAdded bool) {

	if c.walker.EnclosingTypeDefinition.Kind != ast.NodeKindObjectTypeDefinition {
		return false
	}
	fieldDefRef, exists := c.definition.NodeFieldDefinitionByName(c.walker.EnclosingTypeDefinition, c.operation.FieldNameBytes(fieldRef))
	if !exists {
		return false
	}

	fieldDefTypeRef := c.definition.FieldDefinitionType(fieldDefRef)
	fieldDefTypeName := c.definition.TypeNameBytes(fieldDefTypeRef)
	node, ok := c.definition.NodeByName(fieldDefTypeName)
	if !ok {
		return false
	}

	if node.Kind == ast.NodeKindUnionTypeDefinition {
		c.addPath(plannerIndex, pathConfiguration{
			path:             currentPath,
			shouldWalkFields: true,
			typeName:         typeName,
			fieldRef:         fieldRef,
			enclosingNode:    c.walker.EnclosingTypeDefinition,
			dsHash:           c.planners[plannerIndex].dataSourceConfiguration.Hash(),
		})
		return true
	}
	return false
}

func (c *configurationVisitor) addPlannerPathForChildOfAbstractParent(
	plannerIndex int, currentPath string, fieldRef int, fieldName string, typeName string, planningBehaviour DataSourcePlanningBehavior,
) (pathAdded bool) {

	if !c.isParentTypeNodeAbstractType() {
		return false
	}

	if c.addPlannerPathForTypename(plannerIndex, currentPath, fieldRef, fieldName, typeName, planningBehaviour) {
		return true
	}

	// If the field is a root node in any of the data sources, the path shouldn't be handled here
	// NOTE: previously we were checking all ds, not sure if we need now
	for _, d := range c.dataSources {
		if d.HasRootNode(typeName, fieldName) {
			return false
		}
	}
	// The path for this field should only be added if the parent path also exists on this planner

	c.addPath(plannerIndex, pathConfiguration{
		path:             currentPath,
		shouldWalkFields: true,
		typeName:         typeName,
		fieldRef:         fieldRef,
		enclosingNode:    c.walker.EnclosingTypeDefinition,
		dsHash:           c.planners[plannerIndex].dataSourceConfiguration.Hash(),
	})
	return true
}

// addPlannerPathForTypename adds a path for the __typename field
// adding __typename should be done only in case particular planner has parent path
// otherwise it will be added to all planners and will cause visiting of incorrect selection sets
func (c *configurationVisitor) addPlannerPathForTypename(
	plannerIndex int, currentPath string, fieldRef int, fieldName string, typeName string, planningBehaviour DataSourcePlanningBehavior,
) (pathAdded bool) {
	if fieldName != typeNameField {
		return false
	}
	if !planningBehaviour.IncludeTypeNameFields {
		return false
	}

	if c.planners[plannerIndex].hasPath(currentPath) {
		// do not add a path for __typename if it already exists
		return true
	}

	c.addPath(plannerIndex, pathConfiguration{
		path:             currentPath,
		shouldWalkFields: true,
		typeName:         typeName,
		fieldRef:         fieldRef,
		dsHash:           c.planners[plannerIndex].dataSourceConfiguration.Hash(),
	})
	return true
}

func (c *configurationVisitor) isParentTypeNodeAbstractType() bool {
	if len(c.parentTypeNodes) < 2 {
		return false
	}
	parentTypeNode := c.parentTypeNodes[len(c.parentTypeNodes)-2]
	return parentTypeNode.Kind.IsAbstractType()
}

func (c *configurationVisitor) isSubscription(root int, path string) bool {
	rootOperationType := c.operation.OperationDefinitions[root].OperationType
	if rootOperationType != ast.OperationTypeSubscription {
		return false
	}
	return strings.Count(path, ".") == 1
}

func (c *configurationVisitor) nextFetchID() int {
	c.currentFetchID++
	return c.currentFetchID
}

func (c *configurationVisitor) handleFieldRequiredByRequires(config *plannerConfiguration, currentPath string, typeName, fieldName string, fieldRef int) {
	if _, ok := c.handledRequires[fieldRef]; ok {
		return
	}
	c.handledRequires[fieldRef] = struct{}{}

	requiredFieldsForTypeAndField := config.dataSourceConfiguration.RequiredFieldsByRequires(typeName, fieldName)
	for _, requiredFieldsConfiguration := range requiredFieldsForTypeAndField {
		c.planAddingRequiredFields(requiredFieldsConfiguration)
		var added bool
		config.requiredFields, added = appendRequiredFieldsConfigurationIfNotPresent(config.requiredFields, requiredFieldsConfiguration)
		if added {
			c.hasNewFields = true
		}
	}
}

func (c *configurationVisitor) handleFieldsRequiredByKey(plannerIdx int, config *plannerConfiguration, parentPath string, typeName string) {
	requiredFieldsForType := config.dataSourceConfiguration.RequiredFieldsByKey(typeName)
	if len(requiredFieldsForType) > 0 {
		requiredFieldsConfiguration, planned := c.planKeyRequiredFields(plannerIdx, parentPath, typeName, requiredFieldsForType)
		if planned {
			var added bool
			config.requiredFields, added = appendRequiredFieldsConfigurationIfNotPresent(config.requiredFields, requiredFieldsConfiguration)
			if added {
				c.hasNewFields = true
			}
		}
	}
}

func (c *configurationVisitor) planKeyRequiredFields(plannerIdx int, parentPath string, typeName string, possibleRequiredFields []FederationFieldConfiguration) (config FederationFieldConfiguration, planned bool) {
	if len(possibleRequiredFields) == 0 {
		return
	}

	for i := range c.planners {
		if i == plannerIdx {
			continue // skip current planner
		}
		for _, possibleRequiredFieldConfig := range possibleRequiredFields {
			if c.planners[i].dataSourceConfiguration.HasKeyRequirement(typeName, possibleRequiredFieldConfig.SelectionSet) {
				c.planAddingRequiredFields(possibleRequiredFieldConfig)
				return possibleRequiredFieldConfig, true
			}
		}
	}

	return FederationFieldConfiguration{}, false
}

func (c *configurationVisitor) planAddingRequiredFields(fieldConfiguration FederationFieldConfiguration) {
	currentSelectionSet := c.currentSelectionSet()

	selectionSets, hasSelectionSet := c.pendingRequiredFields[currentSelectionSet]
	if !hasSelectionSet {
		selectionSets = make([]string, 0, 2)
		c.pendingRequiredFields[currentSelectionSet] = append(selectionSets, fieldConfiguration.SelectionSet)
		return
	}

	exists := false
	for _, existingSelectionSet := range selectionSets {
		if existingSelectionSet == fieldConfiguration.SelectionSet {
			exists = true
			break
		}
	}
	if !exists {
		c.pendingRequiredFields[currentSelectionSet] = append(selectionSets, fieldConfiguration.SelectionSet)
	}
}

func (c *configurationVisitor) processPendingRequiredFields(selectionSetRef int) {
	configs, hasSelectionSet := c.pendingRequiredFields[selectionSetRef]
	if !hasSelectionSet {
		return
	}

	for _, requiredFields := range configs {
		c.addRequiredFields(selectionSetRef, requiredFields)
	}

	delete(c.pendingRequiredFields, selectionSetRef)
}

func (c *configurationVisitor) addRequiredFields(selectionSetRef int, requiredFields string) {
	typeName := c.walker.EnclosingTypeDefinition.NameString(c.definition)
	key, report := RequiredFieldsFragment(typeName, requiredFields, true)
	if report.HasErrors() {
		c.walker.StopWithInternalErr(fmt.Errorf("failed to parse required fields for %s", typeName))
	}

	parentPath := c.walker.Path.DotDelimitedString()

	input := &addRequiredFieldsInput{
		key:                   key,
		operation:             c.operation,
		definition:            c.definition,
		report:                report,
		operationSelectionSet: selectionSetRef,
		skipFieldRefs:         &c.skipFieldsRefs,
		parentPath:            parentPath,
	}

	addRequiredFields(input)
	if report.HasErrors() {
		c.walker.StopWithInternalErr(fmt.Errorf("failed to add required fields for %s", typeName))
	}
}
