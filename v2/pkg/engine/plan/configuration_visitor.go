package plan

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/pkg/errors"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
)

type configurationVisitor struct {
	ctx   context.Context
	debug bool

	operationName         string
	operation, definition *ast.Document
	walker                *astvisitor.Walker

	dataSources []DataSourceConfiguration
	planners    []*plannerConfiguration

	nodeSuggestions     NodeSuggestions      // nodeSuggestions holds information about suggested data sources for each field
	nodeSuggestionHints []NodeSuggestionHint // NodeSuggestionHints holds information about suggested data sources for key fields

	parentTypeNodes    []ast.Node             // parentTypeNodes is a stack of parent type nodes - used to determine if the parent is abstract
	arrayFields        []arrayField           // arrayFields is a stack of array fields - used to plan nested queries
	selectionSetRefs   []int                  // selectionSetRefs is a stack of selection set refs - used to add a required fields
	skipFieldsRefs     []int                  // skipFieldsRefs holds required field refs added by planner and should not be added to user response
	missingPathTracker map[string]missingPath // missingPathTracker is a map of paths which will be added on secondary runs
	addedPathTracker   []pathConfiguration    // addedPathTracker is a list of paths which were added

	pendingRequiredFields        map[int][]fieldsRequiredByPlanner // pendingRequiredFields is a map[selectionSetRef][]fieldsRequiredByPlanner
	handledRequires              map[int]struct{}                  // handledRequires is a map[FieldRef] of already processed fields which has @requires directive
	visitedFields                map[int]struct{}                  // visitedFields is a map[FieldRef] of already processed fields which we check for abstract type, e.g. union or interface
	fieldDependenciesForPlanners map[int][]int                     // fieldDependenciesForPlanners is a map[FieldRef][]plannerIdx holds dependencies between fields and planners

	secondaryRun        bool // secondaryRun is a flag to indicate that we're running the planner not the first time
	hasNewFields        bool // hasNewFields is used to determine if we need to run the planner again. It will be true in case required fields were added
	fieldConfigurations FieldConfigurations
}

// fieldsRequiredByPlanner is a mapping between planner id which requested required fields
// and a list of fields which should be added
type fieldsRequiredByPlanner struct {
	fieldSelections       string
	requestedByPlannerIDs []int
	providedByPlannerID   int
	skipTypename          bool
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

func (p *missingPath) String() string {
	return fmt.Sprintf(`{"ds":%d,"path":"%s","precedingRootNodePath":"%s"}`, p.dsHash, p.path, p.precedingRootNodePath)
}

type objectFetchConfiguration struct {
	object             *resolve.Object
	trigger            *resolve.GraphQLSubscriptionTrigger
	planner            DataSourcePlanner
	isSubscription     bool
	fieldRef           int
	fieldDefinitionRef int
	sourceID           string
	fetchID            int
	dependsOnFetchIDs  []int
	rootFields         []resolve.GraphCoordinate
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
	c.planners[plannerIdx].addPath(configuration)

	c.saveAddedPath(configuration)
}

func (c *configurationVisitor) saveAddedPath(configuration pathConfiguration) {
	if c.debug {
		c.debugPrint("saveAddedPath", configuration.String())
	}

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
		hasMatch :=
			strings.HasPrefix(currentPath, c.addedPathTracker[i].path) &&
				currentPath != c.addedPathTracker[i].path &&
				c.addedPathTracker[i].isRootNode

		if hasMatch {
			return c.addedPathTracker[i].path, true
		}
	}
	return "", false
}

func (c *configurationVisitor) addMissingPath(path string, parentPath string) {
	c.missingPathTracker[path] = missingPath{
		path:                  path,
		precedingRootNodePath: parentPath,
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
	c.parentTypeNodes = c.parentTypeNodes[:0]
	if c.planners == nil {
		c.planners = make([]*plannerConfiguration, 0, 8)
	} else {
		c.planners = c.planners[:0]
	}

	if c.skipFieldsRefs == nil {
		c.skipFieldsRefs = make([]int, 0, 8)
	} else {
		c.skipFieldsRefs = c.skipFieldsRefs[:0]
	}

	c.missingPathTracker = make(map[string]missingPath)
	c.addedPathTracker = make([]pathConfiguration, 0, 8)

	c.pendingRequiredFields = make(map[int][]fieldsRequiredByPlanner)
	c.fieldDependenciesForPlanners = make(map[int][]int)
	c.handledRequires = make(map[int]struct{})
	c.visitedFields = make(map[int]struct{})
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
	if !planned {
		plannerIdx, planned = c.addNewPlanner(ref, typeName, fieldName, currentPath, parentPath, isSubscription)
	}

	if planned {
		c.handleRequirements(plannerIdx, parentPath, typeName, fieldName, ref)
		c.rewriteSelectionSetOfFieldWithInterfaceType(ref, plannerIdx)
		c.addPlannerDependencies(ref, plannerIdx)
		c.addRootField(ref, plannerIdx)
		return
	}

	c.handleMissingPath(typeName, fieldName, currentPath)
}

func (c *configurationVisitor) addRootField(fieldRef, plannerIdx int) {

	if c.fieldIsChildNode(plannerIdx) {
		return
	}

	enclosingTypeName := c.walker.EnclosingTypeDefinition.NameString(c.definition)
	fieldName := c.operation.FieldNameString(fieldRef)
	fieldHasAuthorizationRule := c.fieldHasAuthorizationRule(enclosingTypeName, fieldName)

	coordinate := resolve.GraphCoordinate{
		TypeName:             enclosingTypeName,
		FieldName:            fieldName,
		HasAuthorizationRule: fieldHasAuthorizationRule,
	}

	if slices.Contains(c.planners[plannerIdx].objectFetchConfiguration.rootFields, coordinate) {
		return
	}

	c.planners[plannerIdx].objectFetchConfiguration.rootFields = append(c.planners[plannerIdx].objectFetchConfiguration.rootFields, coordinate)
}

func (c *configurationVisitor) fieldHasAuthorizationRule(typeName, fieldName string) bool {
	fieldConfig := c.fieldConfigurations.ForTypeField(typeName, fieldName)
	return fieldConfig != nil && fieldConfig.HasAuthorizationRule
}

func (c *configurationVisitor) fieldIsChildNode(plannerIdx int) bool {
	path := c.walker.Path.DotDelimitedString()
	plannerPath := c.planners[plannerIdx].parentPath
	fieldPath := strings.TrimPrefix(path, plannerPath)
	return strings.ContainsAny(fieldPath, ".")
}

func (c *configurationVisitor) addPlannerDependencies(fieldRef int, currentPlannerIdx int) {
	plannerIds, mappingExists := c.fieldDependenciesForPlanners[fieldRef]
	if !mappingExists {
		return
	}

	for _, notifyPlannerIdx := range plannerIds {
		notified := false
		for _, existingPlannerId := range c.planners[notifyPlannerIdx].objectFetchConfiguration.dependsOnFetchIDs {
			if existingPlannerId == currentPlannerIdx {
				notified = true
				break
			}
		}
		if !notified {
			if notifyPlannerIdx == currentPlannerIdx {
				continue
				// c.walker.StopWithInternalErr(fmt.Errorf("wrong fetch dependencies planner %d depends on itself", notifyPlannerIdx))
			}

			c.planners[notifyPlannerIdx].objectFetchConfiguration.dependsOnFetchIDs = append(c.planners[notifyPlannerIdx].objectFetchConfiguration.dependsOnFetchIDs, currentPlannerIdx)
		}
	}
}

func (c *configurationVisitor) handleRequirements(plannerIdx int, parentPath string, typeName, fieldName string, fieldRef int) {
	plannerConfig := c.planners[plannerIdx]
	dsHash := plannerConfig.dataSourceConfiguration.Hash()

	parentDSHash, ok := c.addedPathDSHash(parentPath)
	if ok && dsHash != parentDSHash {
		// add required fields for type (@key)
		c.handleFieldsRequiredByKey(plannerIdx, plannerConfig, typeName, parentPath)
	}

	// add required fields for field and type (@requires)
	c.handleFieldRequiredByRequires(plannerIdx, plannerConfig, typeName, fieldName, fieldRef)
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
	dsHashes := c.nodeSuggestions.SuggestionsForPath(typeName, fieldName, currentPath)

	for plannerIdx, plannerConfig := range c.planners {
		planningBehaviour := plannerConfig.planner.DataSourcePlanningBehavior()
		currentPlannerDSHash := plannerConfig.dataSourceConfiguration.Hash()
		_, isProvided := plannerConfig.providedFields.HasSuggestionForPath(typeName, fieldName, currentPath)

		hasSuggestion := false
		for _, dsHash := range dsHashes {
			if dsHash == currentPlannerDSHash {
				hasSuggestion = true
				break
			}
		}

		// On a union we will never get a node suggestion because union type is not in the root or child nodes
		shouldHandleTypeNameOnUnion :=
			!hasSuggestion && fieldName == typeNameField &&
				c.walker.EnclosingTypeDefinition.Kind == ast.NodeKindUnionTypeDefinition &&
				plannerConfig.hasPath(parentPath)

		if shouldHandleTypeNameOnUnion {
			pathAdded, halted := c.addPlannerPathForTypename(plannerIdx, currentPath, parentPath, ref, fieldName, typeName, planningBehaviour)
			switch {
			case pathAdded:
				return plannerIdx, true
			case halted:
				return -1, false
			}
		}

		if !isProvided && !hasSuggestion {
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

		if (plannerConfig.hasParent(parentPath) || plannerConfig.hasParent(precedingParentPath)) &&
			hasRootNode &&
			planningBehaviour.MergeAliasedRootNodes {
			// same parent + root node = root sibling

			if !c.couldHandleFieldsRequiredByKey(plannerConfig.dataSourceConfiguration, typeName, parentPath) {
				return -1, false
			}

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
			pathAdded, halted := c.addPlannerPathForTypename(plannerIdx, currentPath, parentPath, ref, fieldName, typeName, planningBehaviour)
			switch {
			case pathAdded:
				return plannerIdx, true
			case halted:
				return -1, false
			}

			if isProvided || hasChildNode || (hasRootNode && planningBehaviour.MergeAliasedRootNodes) {
				if !c.couldHandleFieldsRequiredByKey(plannerConfig.dataSourceConfiguration, typeName, parentPath) {
					return -1, false
				}

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

			pathAdded, halted = c.addPlannerPathForUnionChildOfObjectParent(plannerIdx, currentPath, parentPath, ref, fieldName, typeName, planningBehaviour)
			switch {
			case pathAdded:
				return plannerIdx, true
			case halted:
				return -1, false
			}

			pathAdded, halted = c.addPlannerPathForChildOfAbstractParent(plannerIdx, currentPath, parentPath, ref, fieldName, typeName, planningBehaviour)
			switch {
			case pathAdded:
				return plannerIdx, true
			case halted:
				return -1, false
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

	isEntityInterface := false
	for _, interfaceObjCfg := range config.FederationMetaData.EntityInterfaces {
		hasMatch :=
			interfaceObjCfg.InterfaceTypeName == typeName ||
				slices.Contains(interfaceObjCfg.ConcreteTypeNames, typeName)

		if hasMatch {
			isEntityInterface = true
			break
		}
	}

	// we should handle a new planner for a __typename
	// only when it is the first field on a query
	// or we are on the entity interface object
	shouldHandleTypeName :=
		fieldName == typeNameField && parentPath == "query" ||
			isEntityInterface

	if !shouldHandleTypeName && !config.HasRootNode(typeName, fieldName) {
		return -1, false
	}

	if !c.couldHandleFieldsRequiredByKey(*config, typeName, parentPath) {
		return -1, false
	}

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

	fieldDefinition, ok := c.walker.FieldDefinition(ref)
	if !ok {
		return -1, false
	}

	// fetch id is an index of the current planner
	fetchID := len(c.planners)

	fetchConfiguration := objectFetchConfiguration{
		planner:            planner,
		isSubscription:     isSubscription,
		fieldRef:           ref,
		fieldDefinitionRef: fieldDefinition,
		fetchID:            fetchID,
		sourceID:           config.ID,
	}

	plannerConfig := &plannerConfiguration{
		dataSourceConfiguration:  *config,
		objectFetchConfiguration: fetchConfiguration,
		parentPath:               plannerPath,
		planner:                  planner,
		paths:                    paths,
		parentPathType:           c.plannerPathType(plannerPath),
	}

	c.planners = append(c.planners, plannerConfig)

	for _, pathConfiguration := range paths {
		c.saveAddedPath(pathConfiguration)
	}

	return len(c.planners) - 1, true
}

// handleMissingPath - records missing path for the case when we don't yet have a planner for the field
func (c *configurationVisitor) handleMissingPath(typeName string, fieldName string, currentPath string) {
	suggestedDataSourceHashes := c.nodeSuggestions.SuggestionsForPath(typeName, fieldName, currentPath)
	if len(suggestedDataSourceHashes) > 0 {
		parentPath, found := c.findPreviousRootPath(currentPath)
		if found {
			c.addMissingPath(currentPath, parentPath)
			return
		}

		allPlannersHasPath := true
		for i := range c.planners {
			if slices.Contains(suggestedDataSourceHashes, c.planners[i].dataSourceConfiguration.Hash()) {
				if !c.planners[i].hasPath(currentPath) {
					allPlannersHasPath = false
					break
				}
			}
		}

		if allPlannersHasPath {
			return
		}
	}

	c.walker.StopWithInternalErr(errors.Wrap(fmt.Errorf("could not plan field %s.%s on path %s", typeName, fieldName, currentPath), "configurationVisitor.handleMissingPath"))
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
	plannerIndex int, currentPath string, parentPath string, fieldRef int, fieldName string, typeName string, planningBehaviour DataSourcePlanningBehavior,
) (pathAdded bool, halted bool) {

	if c.walker.EnclosingTypeDefinition.Kind != ast.NodeKindObjectTypeDefinition {
		return false, false
	}
	fieldDefRef, exists := c.definition.NodeFieldDefinitionByName(c.walker.EnclosingTypeDefinition, c.operation.FieldNameBytes(fieldRef))
	if !exists {
		return false, false
	}

	fieldDefTypeName := c.definition.FieldDefinitionTypeNameBytes(fieldDefRef)
	node, ok := c.definition.NodeByName(fieldDefTypeName)
	if !ok {
		return false, false
	}

	if node.Kind == ast.NodeKindUnionTypeDefinition {
		if !c.couldHandleFieldsRequiredByKey(c.planners[plannerIndex].dataSourceConfiguration, typeName, parentPath) {
			return false, true
		}

		c.addPath(plannerIndex, pathConfiguration{
			path:             currentPath,
			shouldWalkFields: true,
			typeName:         typeName,
			fieldRef:         fieldRef,
			enclosingNode:    c.walker.EnclosingTypeDefinition,
			dsHash:           c.planners[plannerIndex].dataSourceConfiguration.Hash(),
		})
		return true, false
	}

	return false, false
}

func (c *configurationVisitor) addPlannerPathForChildOfAbstractParent(
	plannerIndex int, currentPath string, parentPath string, fieldRef int, fieldName string, typeName string, planningBehaviour DataSourcePlanningBehavior,
) (pathAdded bool, halted bool) {

	if !c.isParentTypeNodeAbstractType() {
		return false, false
	}

	pathAdded, halted = c.addPlannerPathForTypename(plannerIndex, currentPath, parentPath, fieldRef, fieldName, typeName, planningBehaviour)
	switch {
	case pathAdded:
		return true, false
	case halted:
		return false, true
	}

	// If the field is a root node in any of the data sources, the path shouldn't be handled here
	// NOTE: previously we were checking all ds, not sure if we need now
	for _, d := range c.dataSources {
		if d.HasRootNode(typeName, fieldName) {
			return false, false
		}
	}

	if !c.couldHandleFieldsRequiredByKey(c.planners[plannerIndex].dataSourceConfiguration, typeName, parentPath) {
		return false, true
	}

	c.addPath(plannerIndex, pathConfiguration{
		path:             currentPath,
		shouldWalkFields: true,
		typeName:         typeName,
		fieldRef:         fieldRef,
		enclosingNode:    c.walker.EnclosingTypeDefinition,
		dsHash:           c.planners[plannerIndex].dataSourceConfiguration.Hash(),
	})

	return true, false
}

// addPlannerPathForTypename adds a path for the __typename field
// adding __typename should be done only in case particular planner has parent path
// otherwise it will be added to all planners and will cause visiting of incorrect selection sets
func (c *configurationVisitor) addPlannerPathForTypename(
	plannerIndex int, currentPath string, parentPath string, fieldRef int, fieldName string, typeName string,
	planningBehaviour DataSourcePlanningBehavior,
) (pathAdded bool, halted bool) {
	if fieldName != typeNameField {
		return false, false
	}
	if !planningBehaviour.IncludeTypeNameFields {
		return false, false
	}

	if c.planners[plannerIndex].hasPath(currentPath) {
		// do not add a path for __typename if it already exists
		return true, false
	}

	if !c.couldHandleFieldsRequiredByKey(c.planners[plannerIndex].dataSourceConfiguration, typeName, parentPath) {
		return false, true
	}

	c.addPath(plannerIndex, pathConfiguration{
		path:             currentPath,
		shouldWalkFields: true,
		typeName:         typeName,
		fieldRef:         fieldRef,
		dsHash:           c.planners[plannerIndex].dataSourceConfiguration.Hash(),
	})
	return true, false
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

func (c *configurationVisitor) handleFieldRequiredByRequires(plannerIdx int, config *plannerConfiguration, typeName, fieldName string, fieldRef int) {
	if _, ok := c.handledRequires[fieldRef]; ok {
		return
	}
	c.handledRequires[fieldRef] = struct{}{}

	requiredFieldsForTypeAndField := config.dataSourceConfiguration.RequiredFieldsByRequires(typeName, fieldName)
	for _, requiredFieldsConfiguration := range requiredFieldsForTypeAndField {
		c.planAddingRequiredFields(plannerIdx, -1, requiredFieldsConfiguration, false)
		var added bool
		config.requiredFields, added = appendRequiredFieldsConfigurationIfNotPresent(config.requiredFields, requiredFieldsConfiguration)
		if added {
			c.hasNewFields = true
		}
	}
}

func (c *configurationVisitor) handleFieldsRequiredByKey(plannerIdx int, config *plannerConfiguration, typeName string, parentPath string) {
	requiredFieldsForType := config.dataSourceConfiguration.RequiredFieldsByKey(typeName)
	if len(requiredFieldsForType) > 0 {
		isInterfaceObject := false
		for _, interfaceObjCfg := range config.dataSourceConfiguration.FederationMetaData.InterfaceObjects {
			if slices.Contains(interfaceObjCfg.ConcreteTypeNames, typeName) {
				isInterfaceObject = true
				break
			}
		}

		requiredFieldsConfiguration, planned := c.planKeyRequiredFields(plannerIdx, typeName, parentPath, requiredFieldsForType, isInterfaceObject)
		if planned {
			var added bool
			config.requiredFields, added = appendRequiredFieldsConfigurationIfNotPresent(config.requiredFields, requiredFieldsConfiguration)
			if added {
				c.hasNewFields = true
			}
		}
	}
}

// couldHandleFieldsRequiredByKey - checks wether we could plan the field now according to it's key requirements
// if no existing planners datasources could provide us with the required fields we should postpone planning of the field
func (c *configurationVisitor) couldHandleFieldsRequiredByKey(dsConfig DataSourceConfiguration, typeName string, parentPath string) bool {
	possibleRequiredFields := dsConfig.RequiredFieldsByKey(typeName)
	if len(possibleRequiredFields) == 0 {
		return true
	}

	for i := range c.planners {
		if !c.planners[i].hasPath(parentPath) {
			continue
		}

		if c.planners[i].dataSourceConfiguration.Hash() == dsConfig.Hash() {
			return true
		}

		for _, possibleRequiredFieldConfig := range possibleRequiredFields {
			if c.planners[i].dataSourceConfiguration.HasKeyRequirement(typeName, possibleRequiredFieldConfig.SelectionSet) {
				return true
			}
		}
	}

	return false
}

func (c *configurationVisitor) planKeyRequiredFields(currentPlannerIdx int, typeName string, parentPath string, possibleRequiredFields []FederationFieldConfiguration, forInterfaceObject bool) (config FederationFieldConfiguration, planned bool) {
	if len(possibleRequiredFields) == 0 {
		return
	}

	for i := range c.planners {
		if i == currentPlannerIdx {
			continue // skip current planner
		}
		// we need to filter out the planners which do not have such parent path
		// because we could have a planner with mathing datasource but on the wrong path
		if !c.planners[i].hasPath(parentPath) {
			continue
		}
		for _, possibleRequiredFieldConfig := range possibleRequiredFields {
			if c.planners[i].dataSourceConfiguration.HasKeyRequirement(typeName, possibleRequiredFieldConfig.SelectionSet) {

				isInterfaceObject := false
				for _, interfaceObjCfg := range c.planners[i].dataSourceConfiguration.FederationMetaData.InterfaceObjects {
					if slices.Contains(interfaceObjCfg.ConcreteTypeNames, typeName) {
						isInterfaceObject = true
						break
					}
				}
				skipTypename := forInterfaceObject && isInterfaceObject

				c.planAddingRequiredFields(currentPlannerIdx, i, possibleRequiredFieldConfig, skipTypename)
				return possibleRequiredFieldConfig, true
			}
		}
	}

	return FederationFieldConfiguration{}, false
}

func (c *configurationVisitor) planAddingRequiredFields(currentPlannerIdx int, providedByPlannerIdx int, fieldConfiguration FederationFieldConfiguration, skipTypename bool) {
	currentSelectionSet := c.currentSelectionSet()

	requirements, hasRequirements := c.pendingRequiredFields[currentSelectionSet]
	// no requirement was added for the current selection set
	if !hasRequirements {
		requirements = make([]fieldsRequiredByPlanner, 0, 2)
		c.pendingRequiredFields[currentSelectionSet] = append(requirements, fieldsRequiredByPlanner{
			requestedByPlannerIDs: []int{currentPlannerIdx},
			providedByPlannerID:   providedByPlannerIdx,
			fieldSelections:       fieldConfiguration.SelectionSet,
			skipTypename:          skipTypename,
		})
		return
	}

	requirementExists := false
	for i := range requirements {
		if requirements[i].fieldSelections == fieldConfiguration.SelectionSet {
			requirementExists = true

			plannerIdExists := false
			for _, plannerId := range requirements[i].requestedByPlannerIDs {
				if plannerId == currentPlannerIdx {
					plannerIdExists = true
					break
				}
			}

			// when we have already added the same requirements for the current selection set
			// but not for such planner id
			if !plannerIdExists {
				requirements[i].requestedByPlannerIDs = append(requirements[i].requestedByPlannerIDs, currentPlannerIdx)
			}
			break
		}
	}

	// when no such requirement was added for the current selection set
	if !requirementExists {
		requirements = append(requirements, fieldsRequiredByPlanner{
			requestedByPlannerIDs: []int{currentPlannerIdx},
			providedByPlannerID:   providedByPlannerIdx,
			fieldSelections:       fieldConfiguration.SelectionSet,
		})
	}

	c.pendingRequiredFields[currentSelectionSet] = requirements
}

func (c *configurationVisitor) processPendingRequiredFields(selectionSetRef int) {
	configs, hasSelectionSet := c.pendingRequiredFields[selectionSetRef]
	if !hasSelectionSet {
		return
	}

	for _, requiredFieldsCfg := range configs {
		c.addRequiredFields(selectionSetRef, requiredFieldsCfg)
	}

	delete(c.pendingRequiredFields, selectionSetRef)
}

func (c *configurationVisitor) addRequiredFields(selectionSetRef int, requiredFieldsCfg fieldsRequiredByPlanner) {
	typeName := c.walker.EnclosingTypeDefinition.NameString(c.definition)
	key, report := RequiredFieldsFragment(typeName, requiredFieldsCfg.fieldSelections, !requiredFieldsCfg.skipTypename)
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
		parentPath:            parentPath,
	}

	skipFieldRefs, requiredFieldRefs := addRequiredFields(input)
	if report.HasErrors() {
		c.walker.StopWithInternalErr(fmt.Errorf("failed to add required fields for %s", typeName))
	}

	c.skipFieldsRefs = append(c.skipFieldsRefs, skipFieldRefs...)

	for _, fieldRef := range requiredFieldRefs {
		c.fieldDependenciesForPlanners[fieldRef] = append(c.fieldDependenciesForPlanners[fieldRef], requiredFieldsCfg.requestedByPlannerIDs...)
		if requiredFieldsCfg.providedByPlannerID != -1 {
			c.addNodeSuggestionHint(fieldRef, requiredFieldsCfg.providedByPlannerID)
		}
	}
}

func (c *configurationVisitor) addNodeSuggestionHint(fieldRef int, plannerIdx int) {
	/*
		Here we add hints for the node suggestions filter
		to be able to select a proper datasource for the key field
		It is required to make jumps via diffrerent keys between different subgraphs
	*/

	// but we should not add hints for a __typename field cause it collides with
	// entity interfaces functionality
	fieldName := c.operation.FieldNameString(fieldRef)
	if fieldName == typeNameField {
		return
	}

	dsHash := c.planners[plannerIdx].dataSourceConfiguration.Hash()
	parentPath := c.walker.Path.DotDelimitedString()

	c.nodeSuggestionHints = append(c.nodeSuggestionHints, NodeSuggestionHint{
		fieldRef:   fieldRef,
		dsHash:     dsHash,
		fieldName:  fieldName,
		parentPath: parentPath,
	})
}

func (c *configurationVisitor) rewriteSelectionSetOfFieldWithInterfaceType(fieldRef int, plannerIdx int) {
	if _, ok := c.visitedFields[fieldRef]; ok {
		return
	}
	c.visitedFields[fieldRef] = struct{}{}

	upstreamSchema := c.planners[plannerIdx].planner.UpstreamSchema(c.planners[plannerIdx].dataSourceConfiguration)

	rewriter := newFieldSelectionRewriter(c.operation, c.definition)
	rewriter.SetUpstreamDefinition(upstreamSchema)
	rewriter.SetDatasourceConfiguration(&c.planners[plannerIdx].dataSourceConfiguration)

	rewritten, err := rewriter.RewriteFieldSelection(fieldRef, c.walker.EnclosingTypeDefinition)

	if err != nil {
		c.walker.StopWithInternalErr(err)
	}

	if !rewritten {
		return
	}

	c.hasNewFields = true
	c.walker.Stop()
}
