package plan

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jensneuse/abstractlogger"
	"github.com/pkg/errors"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type configurationVisitor struct {
	logger abstractlogger.Logger
	debug  bool

	operationName         string        // graphql query name
	operation, definition *ast.Document // graphql operation and schema documents
	walker                *astvisitor.Walker

	dataSources         []DataSource        // data sources configurations, which used by the current operation
	fieldConfigurations FieldConfigurations // field configuration from plan configuration

	planners []PlannerConfiguration // configurationVisitor is building this list of planners

	nodeSuggestions     *NodeSuggestions     // nodeSuggestions holds information about suggested data sources for each field
	nodeSuggestionHints []NodeSuggestionHint // nodeSuggestionHints holds information about suggested data sources for key fields

	parentTypeNodes    []ast.Node             // parentTypeNodes is a stack of parent type nodes - used to determine if the parent is abstract
	arrayFields        []arrayField           // arrayFields is a stack of array fields - used to plan nested queries
	selectionSetRefs   []int                  // selectionSetRefs is a stack of selection set refs - used to add a required fields
	skipFieldsRefs     []int                  // skipFieldsRefs holds required field refs added by planner and should not be added to user response
	missingPathTracker map[string]missingPath // missingPathTracker is a map of paths which will be added on secondary runs
	addedPathTracker   []pathConfiguration    // addedPathTracker is a list of paths which were added

	pendingRequiredFields        map[int]map[fieldsRequirementConfig]struct{} // pendingRequiredFields is a map[selectionSetRef][]fieldsRequirementConfig
	visitedFieldsRequiresChecks  map[int]struct{}                             // visitedFieldsRequiresChecks is a map[FieldRef] of already processed fields which we check for @requires directive
	visitedFieldsAbstractChecks  map[int]struct{}                             // visitedFieldsAbstractChecks is a map[FieldRef] of already processed fields which we check for abstract type, e.g. union or interface
	fieldDependenciesForPlanners map[int][]int                                // fieldDependenciesForPlanners is a map[FieldRef][]plannerIdx holds list of planner ids which depends on a field ref. Used for @key dependencies
	fieldsPlannedOn              map[int][]int                                // fieldsPlannedOn is a map[fieldRef][]plannerIdx holds list of planner ids which planned a field ref
	fieldWaitingForDependency    map[int][]int                                // fieldWaitingForDependency is a map[fieldRef][]fieldRef holds list of field refs which are waiting for a dependency to be planned. Used for @requires directive dependencies

	secondaryRun bool // secondaryRun is a flag to indicate that we're running the configurationVisitor not the first time
	hasNewFields bool // hasNewFields is used to determine if we need to run the planner again. It will be true in case required fields were added
}

// fieldsRequirementConfig is a mapping between requestedByPlannerID or requestedByFieldRef, which requested required fields,
// and fieldSelections which should be added
type fieldsRequirementConfig struct {
	path            string
	fieldSelections string
	skipTypename    bool

	requestedByFieldRef int // requestedByFieldRef is a field ref which requested fields via @requires directive

	requestedByPlannerID int // requestedByPlannerID is a planner id which requested fields from @key directive
	providedByPlannerID  int // providedByPlannerID is a planner id which should provide fields for the requestedByPlannerID planner
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
	planner            DataSourceFetchPlanner
	isSubscription     bool
	fieldRef           int
	fieldDefinitionRef int
	sourceID           string
	fetchID            int
	dependsOnFetchIDs  []int
	rootFields         []resolve.GraphCoordinate
	operationType      ast.OperationType
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
	c.planners[plannerIdx].AddPath(configuration)

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
	// NOTE: it will found first occurence of such path
	for i := range c.addedPathTracker {
		if c.addedPathTracker[i].path == path {
			return c.addedPathTracker[i].dsHash, true
		}
	}
	return 0, false
}

func (c *configurationVisitor) isPathAddedFor(path string, hash DSHash) bool {
	// TODO: could be optimized by using map[struct{path,hash}]
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
		c.planners = make([]PlannerConfiguration, 0, 8)
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

	c.pendingRequiredFields = make(map[int]map[fieldsRequirementConfig]struct{})
	c.fieldDependenciesForPlanners = make(map[int][]int)
	c.fieldsPlannedOn = make(map[int][]int)
	c.fieldWaitingForDependency = make(map[int][]int)

	c.visitedFieldsRequiresChecks = make(map[int]struct{})
	c.visitedFieldsAbstractChecks = make(map[int]struct{})
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
			if !planner.HasPath(parentPath) {
				continue
			}

			hasRootNode := planner.DataSourceConfiguration().HasRootNodeWithTypename(typeName)
			hasChildNode := planner.DataSourceConfiguration().HasChildNodeWithTypename(typeName)
			if !(hasRootNode || hasChildNode) {
				continue
			}

			if planner.HasPath(currentPath) {
				continue
			}

			path := pathConfiguration{
				path:             currentPath,
				shouldWalkFields: true,
				dsHash:           planner.DataSourceConfiguration().Hash(),
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

	// we check if the field has @requires directive
	// and if we don't have required fields in the operation, we do not plan current field,
	// but schedule adding of required fields into the operation and record missing path
	if !c.handleFieldRequiredByRequires(ref, typeName, fieldName, currentPath) {
		c.handleMissingPath(typeName, fieldName, currentPath)
		return
	}

	plannerIdx, planned := c.planWithExistingPlanners(ref, typeName, fieldName, currentPath, parentPath, precedingParentPath)
	if !planned {
		plannerIdx, planned = c.addNewPlanner(ref, typeName, fieldName, currentPath, parentPath, isSubscription)
	}

	if planned {
		c.handleFieldsRequiredByKey(plannerIdx, parentPath, typeName)
		c.recordFieldPlannedOn(ref, plannerIdx)
		c.addPlannerDependencies(ref, plannerIdx)
		c.addFieldDependencies(ref, plannerIdx)

		c.rewriteSelectionSetOfFieldWithInterfaceType(ref, plannerIdx)
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

	fetchConfiguration := c.planners[plannerIdx].ObjectFetchConfiguration()
	if !slices.Contains(fetchConfiguration.rootFields, coordinate) {
		fetchConfiguration.rootFields = append(fetchConfiguration.rootFields, coordinate)
	}
}

func (c *configurationVisitor) fieldHasAuthorizationRule(typeName, fieldName string) bool {
	fieldConfig := c.fieldConfigurations.ForTypeField(typeName, fieldName)
	return fieldConfig != nil && fieldConfig.HasAuthorizationRule
}

func (c *configurationVisitor) fieldIsChildNode(plannerIdx int) bool {
	path := c.walker.Path.DotDelimitedString()
	plannerPath := c.planners[plannerIdx].ParentPath()
	fieldPath := strings.TrimPrefix(path, plannerPath)
	return strings.ContainsAny(fieldPath, ".")
}

// addPlannerDependencies adds dependencies between planners based on @key directive
func (c *configurationVisitor) addPlannerDependencies(fieldRef int, currentPlannerIdx int) {
	plannerIds, mappingExists := c.fieldDependenciesForPlanners[fieldRef]
	if !mappingExists {
		return
	}

	for _, notifyPlannerIdx := range plannerIds {
		fetchConfiguration := c.planners[notifyPlannerIdx].ObjectFetchConfiguration()

		notified := slices.Contains(fetchConfiguration.dependsOnFetchIDs, currentPlannerIdx)
		if !notified {
			if notifyPlannerIdx == currentPlannerIdx {
				return
				// c.walker.StopWithInternalErr(fmt.Errorf("wrong fetch dependencies planner %d depends on itself", notifyPlannerIdx))
			}

			fetchConfiguration.dependsOnFetchIDs = append(fetchConfiguration.dependsOnFetchIDs, currentPlannerIdx)
		}
	}
}

// handleFieldsRequiredByKey records on which planner id a field was planned
func (c *configurationVisitor) recordFieldPlannedOn(fieldRef int, plannerIdx int) {
	c.fieldsPlannedOn[fieldRef] = append(c.fieldsPlannedOn[fieldRef], plannerIdx)
}

func (c *configurationVisitor) addFieldDependencies(fieldRef int, currentPlannerIdx int) {
	fieldRefs, mappingExists := c.fieldWaitingForDependency[fieldRef]
	if !mappingExists {
		return
	}

	fetchConfiguration := c.planners[currentPlannerIdx].ObjectFetchConfiguration()

	for _, waitingForFieldRef := range fieldRefs {
		// we do not check if it exists, because we should not be able to plan a field with requires
		// in case we haven't planned all required fields
		plannerIds, _ := c.fieldsPlannedOn[waitingForFieldRef]

		for _, plannerIdx := range plannerIds {
			notified := slices.Contains(fetchConfiguration.dependsOnFetchIDs, plannerIdx)
			if !notified {
				fetchConfiguration.dependsOnFetchIDs = append(fetchConfiguration.dependsOnFetchIDs, plannerIdx)
			}
		}
	}
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
		for _, provide := range ds.FederationConfiguration().Provides {
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
		c.walker.StopWithInternalErr(fmt.Errorf("failed to parse provides fields for %s.%s at path %s", typeName, fieldName, currentPath))
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
		c.walker.StopWithInternalErr(fmt.Errorf("failed to get provides suggestions for %s.%s at path %s", typeName, fieldName, currentPath))
	}

	for i := range c.planners {
		if c.planners[i].DataSourceConfiguration().Hash() == dsHash {
			c.planners[i].ProvidedFields().AddItems(suggestions...)
			break
		}
	}
}

func (c *configurationVisitor) planWithExistingPlanners(ref int, typeName, fieldName, currentPath, parentPath, precedingParentPath string) (plannerIdx int, planned bool) {
	dsHashes := c.nodeSuggestions.SuggestionsForPath(typeName, fieldName, currentPath)

	for plannerIdx, plannerConfig := range c.planners {
		planningBehaviour := plannerConfig.DataSourcePlanningBehavior()
		currentPlannerDSHash := plannerConfig.DataSourceConfiguration().Hash()
		_, isProvided := plannerConfig.ProvidedFields().HasSuggestionForPath(typeName, fieldName, currentPath)
		hasSuggestion := slices.Contains(dsHashes, currentPlannerDSHash)

		// On a union we will never get a node suggestion because union type is not in the root or child nodes
		shouldHandleTypeNameOnUnion :=
			!hasSuggestion && fieldName == typeNameField &&
				c.walker.EnclosingTypeDefinition.Kind == ast.NodeKindUnionTypeDefinition &&
				plannerConfig.HasPath(parentPath)

		if shouldHandleTypeNameOnUnion {
			if pathAdded := c.addPlannerPathForTypename(plannerIdx, currentPath, parentPath, ref, fieldName, typeName, planningBehaviour); pathAdded {
				return plannerIdx, true
			}
		}

		if !isProvided && !hasSuggestion {
			continue
		}

		hasRootNode := plannerConfig.DataSourceConfiguration().HasRootNode(typeName, fieldName)
		hasChildNode := plannerConfig.DataSourceConfiguration().HasChildNode(typeName, fieldName)

		if c.secondaryRun && plannerConfig.HasPath(currentPath) {
			if c.hasMissingPathWithParentPath(currentPath) {
				// shareable case - we have planned this path for this plannerIdx, but we still have a missing path,
				// so we need to check other planner indexes
				continue
			}
			// on the second run we need to process only new fields added by the first run
			return plannerIdx, true
		}

		if !c.couldHandleFieldsRequiredByKey(plannerConfig.DataSourceConfiguration(), typeName, parentPath) {
			return -1, false
		}

		if plannerConfig.HasPath(parentPath) || plannerConfig.HasPath(precedingParentPath) {
			if pathAdded := c.addPlannerPathForTypename(plannerIdx, currentPath, parentPath, ref, fieldName, typeName, planningBehaviour); pathAdded {
				return plannerIdx, true
			}

			if isProvided || hasChildNode || (hasRootNode && planningBehaviour.MergeAliasedRootNodes) {
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
		}
	}

	return -1, false
}

func (c *configurationVisitor) findSuggestedDataSourceConfiguration(typeName, fieldName, currentPath string) DataSource {
	dsHashes := c.nodeSuggestions.SuggestionsForPath(typeName, fieldName, currentPath)
	if len(dsHashes) == 0 {
		return nil
	}

	for _, dsCfg := range c.dataSources {
		if !slices.Contains(dsHashes, dsCfg.Hash()) {
			continue
		}

		if c.isPathAddedFor(currentPath, dsCfg.Hash()) {
			continue
		}

		return dsCfg
	}

	return nil
}

func (c *configurationVisitor) isParentPathIsRootOperationPath(parentPath string) bool {
	return parentPath == "query" || parentPath == "mutation" || parentPath == "subscription"
}

func (c *configurationVisitor) allowNewPlannerForTypenameField(fieldName string, typeName string, parentPath string, dsCfg DataSource) bool {
	isEntityInterface := slices.ContainsFunc(dsCfg.FederationConfiguration().EntityInterfaces, func(interfaceObjCfg EntityInterfaceConfiguration) bool {
		return interfaceObjCfg.InterfaceTypeName == typeName || slices.Contains(interfaceObjCfg.ConcreteTypeNames, typeName)
	})

	if isEntityInterface {
		return true
	}

	// we should handle a new planner for a __typename
	// only when it is the first field on a query,
	// or we are on the entity interface object
	return fieldName == typeNameField && c.isParentPathIsRootOperationPath(parentPath)
}

func (c *configurationVisitor) addNewPlanner(ref int, typeName, fieldName, currentPath, parentPath string, isSubscription bool) (plannerIdx int, planned bool) {
	config := c.findSuggestedDataSourceConfiguration(typeName, fieldName, currentPath)
	if config == nil {
		return -1, false
	}

	shouldPlanTypenameField := c.allowNewPlannerForTypenameField(fieldName, typeName, parentPath, config)

	if !shouldPlanTypenameField && !config.HasRootNode(typeName, fieldName) {
		return -1, false
	}

	if !c.couldHandleFieldsRequiredByKey(config, typeName, parentPath) {
		return -1, false
	}

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

	isParentAbstract := c.isParentTypeNodeAbstractType()
	isParentFragment := c.walker.Path[len(c.walker.Path)-1].Kind == ast.InlineFragmentName

	if isParentAbstract && isParentFragment {
		// if the parent is abstract and path is on a fragment parent, we add the parent path of type fragment
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

	fetchConfiguration := &objectFetchConfiguration{
		isSubscription:     isSubscription,
		fieldRef:           ref,
		fieldDefinitionRef: fieldDefinition,
		fetchID:            fetchID,
		sourceID:           config.Id(),
		operationType:      c.resolveRootFieldOperationType(typeName),
	}

	plannerPathConfig := newPlannerPathsConfiguration(
		plannerPath,
		c.plannerPathType(plannerPath),
		paths,
	)

	plannerConfig := config.CreatePlannerConfiguration(c.logger, fetchConfiguration, plannerPathConfig)

	c.planners = append(c.planners, plannerConfig)

	for _, pathConfiguration := range paths {
		c.saveAddedPath(pathConfiguration)
	}

	return len(c.planners) - 1, true
}

func (c *configurationVisitor) resolveRootFieldOperationType(typeName string) ast.OperationType {
	if typeName == c.definition.Index.QueryTypeName.String() {
		return ast.OperationTypeQuery
	}
	if typeName == c.definition.Index.MutationTypeName.String() {
		return ast.OperationTypeMutation
	}
	if typeName == c.definition.Index.SubscriptionTypeName.String() {
		return ast.OperationTypeSubscription
	}
	return ast.OperationTypeQuery
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
			if slices.Contains(suggestedDataSourceHashes, c.planners[i].DataSourceConfiguration().Hash()) {
				if !c.planners[i].HasPath(currentPath) {
					allPlannersHasPath = false
					break
				}
			}
		}

		if allPlannersHasPath {
			// we have revisited field already planned by all existing planners
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
		if planner.HasPath(current) && !planner.HasPathPrefix(current) {
			c.planners[i].SetPathExit(current)
			return
		}
	}
}

// addPlannerPathForTypename adds a path for the __typename field
// adding __typename should be done only in case particular planner has parent path
// otherwise it will be added to all planners and will cause visiting of incorrect selection sets
func (c *configurationVisitor) addPlannerPathForTypename(
	plannerIndex int, currentPath string, parentPath string, fieldRef int, fieldName string, typeName string,
	planningBehaviour DataSourcePlanningBehavior,
) (pathAdded bool) {
	if fieldName != typeNameField {
		return false
	}
	if !planningBehaviour.IncludeTypeNameFields {
		return false
	}

	if c.planners[plannerIndex].HasPath(currentPath) {
		// do not add a path for __typename if it already exists
		return true
	}

	c.addPath(plannerIndex, pathConfiguration{
		path:             currentPath,
		shouldWalkFields: true,
		typeName:         typeName,
		fieldRef:         fieldRef,
		dsHash:           c.planners[plannerIndex].DataSourceConfiguration().Hash(),
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

func (c *configurationVisitor) handleFieldRequiredByRequires(fieldRef int, typeName, fieldName, currentPath string) (ok bool) {
	if _, ok := c.visitedFieldsRequiresChecks[fieldRef]; ok {
		// if we already visited this field, we should not check it again
		return true
	}
	c.visitedFieldsRequiresChecks[fieldRef] = struct{}{}

	if fieldName == typeNameField {
		// the __typename field could not have @requires directive
		return true
	}

	config := c.findSuggestedDataSourceConfiguration(typeName, fieldName, currentPath)
	if config == nil {
		// if we could not find a datasource for the field
		// something wrong, and we will not be able to plan a query
		return false
	}

	requiresConfiguration, exists := config.RequiredFieldsByRequires(typeName, fieldName)
	if !exists {
		// we do not have a @requires configuration for the field
		return true
	}

	currentSelectionSet := c.currentSelectionSet()
	if c.hasFieldsRequiredByRequires(currentSelectionSet, requiresConfiguration.SelectionSet, typeName, fieldName, currentPath) {
		// all field from @requires directive are already planned
		return true
	}

	// we should plan required fields for the field
	c.planAddingRequiredFields(-1, -1, fieldRef, requiresConfiguration, true, currentPath)
	c.hasNewFields = true
	return false
}

func (c *configurationVisitor) hasFieldsRequiredByRequires(selectionSetRef int, fieldSelections string, typeName, fieldName, currentPath string) bool {
	key, report := RequiredFieldsFragment(typeName, fieldSelections, false)
	if report.HasErrors() {
		c.walker.StopWithInternalErr(fmt.Errorf("failed to parse required fields for %s", typeName))
		return false
	}

	input := &addRequiredFieldsInput{
		key:                   key,
		operation:             c.operation,
		definition:            c.definition,
		report:                report,
		operationSelectionSet: selectionSetRef,
	}

	allPresent := testRequiredFields(input)
	if report.HasErrors() {
		// TODO: here should be also a field name
		c.walker.StopWithInternalErr(fmt.Errorf("failed to check field requirements for %s", typeName))
		return false
	}

	return allPresent
}

func (c *configurationVisitor) handleFieldsRequiredByKey(plannerIdx int, parentPath string, typeName string) {
	plannerConfig := c.planners[plannerIdx]
	dsHash := plannerConfig.DataSourceConfiguration().Hash()

	parentDSHash, ok := c.addedPathDSHash(parentPath)
	if !ok {
		return
	}

	// we should handle key requirements only when the datasource hash differs from the parent datasource hash
	// it means that this field should be resolved by another datasource
	if dsHash == parentDSHash {
		return
	}

	requiredFieldsForType := plannerConfig.DataSourceConfiguration().RequiredFieldsByKey(typeName)
	if len(requiredFieldsForType) > 0 {
		isInterfaceObject := false
		for _, interfaceObjCfg := range plannerConfig.DataSourceConfiguration().FederationConfiguration().InterfaceObjects {
			if slices.Contains(interfaceObjCfg.ConcreteTypeNames, typeName) {
				isInterfaceObject = true
				break
			}
		}

		requiredFieldsConfiguration, planned := c.planKeyRequiredFields(plannerIdx, typeName, parentPath, requiredFieldsForType, isInterfaceObject)
		if planned {
			if plannerConfig.RequiredFields().AppendIfNotPresent(requiredFieldsConfiguration) {
				c.hasNewFields = true
			}
		}
	}
}

// couldHandleFieldsRequiredByKey - checks wether we could plan the field now according to it's key requirements
// if no existing planners datasources could provide us with the required fields we should postpone planning of the field
func (c *configurationVisitor) couldHandleFieldsRequiredByKey(dsConfig DataSource, typeName string, parentPath string) bool {
	possibleRequiredFields := dsConfig.RequiredFieldsByKey(typeName)
	if len(possibleRequiredFields) == 0 {
		return true
	}

	for i := range c.planners {
		if !c.planners[i].HasPath(parentPath) {
			continue
		}

		if c.planners[i].DataSourceConfiguration().Hash() == dsConfig.Hash() {
			return true
		}

		for _, possibleRequiredFieldConfig := range possibleRequiredFields {
			if c.planners[i].DataSourceConfiguration().HasKeyRequirement(typeName, possibleRequiredFieldConfig.SelectionSet) {
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
		// because we could have a planner with matching datasource but on the wrong path
		if !c.planners[i].HasPath(parentPath) {
			continue
		}
		for _, possibleRequiredFieldConfig := range possibleRequiredFields {
			if c.planners[i].DataSourceConfiguration().HasKeyRequirement(typeName, possibleRequiredFieldConfig.SelectionSet) {

				isInterfaceObject := slices.ContainsFunc(c.planners[i].DataSourceConfiguration().FederationConfiguration().InterfaceObjects, func(interfaceObjCfg EntityInterfaceConfiguration) bool {
					return slices.Contains(interfaceObjCfg.ConcreteTypeNames, typeName)
				})

				skipTypename := forInterfaceObject && isInterfaceObject

				c.planAddingRequiredFields(currentPlannerIdx, i, -1, possibleRequiredFieldConfig, skipTypename, parentPath)

				return possibleRequiredFieldConfig, true
			}
		}
	}

	return FederationFieldConfiguration{}, false
}

func (c *configurationVisitor) planAddingRequiredFields(currentPlannerIdx int, providedByPlannerIdx int, requestedByFieldRef int, fieldConfiguration FederationFieldConfiguration, skipTypename bool, currentPath string) {
	currentSelectionSet := c.currentSelectionSet()

	requirements, hasRequirements := c.pendingRequiredFields[currentSelectionSet]

	if !hasRequirements {
		requirements = make(map[fieldsRequirementConfig]struct{})
	}

	requirements[fieldsRequirementConfig{
		path:                 currentPath,
		requestedByFieldRef:  requestedByFieldRef,
		requestedByPlannerID: currentPlannerIdx,
		providedByPlannerID:  providedByPlannerIdx,
		fieldSelections:      fieldConfiguration.SelectionSet,
		skipTypename:         skipTypename,
	}] = struct{}{}

	c.pendingRequiredFields[currentSelectionSet] = requirements
}

func (c *configurationVisitor) processPendingRequiredFields(selectionSetRef int) {
	configs, hasSelectionSet := c.pendingRequiredFields[selectionSetRef]
	if !hasSelectionSet {
		return
	}

	for requiredFieldsCfg := range configs {
		c.addRequiredFieldsToOperation(selectionSetRef, requiredFieldsCfg)
	}

	delete(c.pendingRequiredFields, selectionSetRef)
}

func (c *configurationVisitor) addRequiredFieldsToOperation(selectionSetRef int, requiredFieldsCfg fieldsRequirementConfig) {
	typeName := c.walker.EnclosingTypeDefinition.NameString(c.definition)
	key, report := RequiredFieldsFragment(typeName, requiredFieldsCfg.fieldSelections, !requiredFieldsCfg.skipTypename)
	if report.HasErrors() {
		c.walker.StopWithInternalErr(fmt.Errorf("failed to parse required fields %s for %s at path %s", requiredFieldsCfg.fieldSelections, typeName, requiredFieldsCfg.path))
		return
	}

	input := &addRequiredFieldsInput{
		key:                   key,
		operation:             c.operation,
		definition:            c.definition,
		report:                report,
		operationSelectionSet: selectionSetRef,
	}

	skipFieldRefs, requiredFieldRefs := addRequiredFields(input)
	if report.HasErrors() {
		c.walker.StopWithInternalErr(fmt.Errorf("failed to add required fields %s for %s at path %s", requiredFieldsCfg.fieldSelections, typeName, requiredFieldsCfg.path))
		return
	}

	c.skipFieldsRefs = append(c.skipFieldsRefs, skipFieldRefs...)

	for _, fieldRef := range requiredFieldRefs {
		// add mapping for the planner field dependecies
		c.fieldDependenciesForPlanners[fieldRef] = append(c.fieldDependenciesForPlanners[fieldRef], requiredFieldsCfg.requestedByPlannerID)

		// add suggestion hint to plan key fields on a proper data source
		if requiredFieldsCfg.providedByPlannerID != -1 {
			c.addNodeSuggestionHint(fieldRef, requiredFieldsCfg.providedByPlannerID)
		}
	}

	// add mapping for the field deoendencies
	if requiredFieldsCfg.requestedByFieldRef != -1 {
		c.fieldWaitingForDependency[requiredFieldsCfg.requestedByFieldRef] = requiredFieldRefs
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

	dsHash := c.planners[plannerIdx].DataSourceConfiguration().Hash()
	parentPath := c.walker.Path.DotDelimitedString()

	c.nodeSuggestionHints = append(c.nodeSuggestionHints, NodeSuggestionHint{
		fieldRef:   fieldRef,
		dsHash:     dsHash,
		fieldName:  fieldName,
		parentPath: parentPath,
	})
}

func (c *configurationVisitor) rewriteSelectionSetOfFieldWithInterfaceType(fieldRef int, plannerIdx int) {
	if _, ok := c.visitedFieldsAbstractChecks[fieldRef]; ok {
		return
	}
	c.visitedFieldsAbstractChecks[fieldRef] = struct{}{}

	upstreamSchema, ok := c.planners[plannerIdx].UpstreamSchema()
	if !ok {
		return
	}

	rewriter := newFieldSelectionRewriter(c.operation, c.definition)
	rewriter.SetUpstreamDefinition(upstreamSchema)
	rewriter.SetDatasourceConfiguration(c.planners[plannerIdx].DataSourceConfiguration())

	rewritten, err := rewriter.RewriteFieldSelection(fieldRef, c.walker.EnclosingTypeDefinition)

	if err != nil {
		c.walker.StopWithInternalErr(err)
		return
	}

	if !rewritten {
		return
	}

	c.hasNewFields = true
	c.walker.Stop()
}
