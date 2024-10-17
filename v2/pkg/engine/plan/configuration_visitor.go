package plan

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/argument_templates"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// configurationVisitor - walks through the operation multiple times to collect plannings paths
// to resolve fields from different datasources.
// we are revisiting query when we have:
// - missing path, which was not planned on the previuos walks
// - we have fields which are waiting for dependencies
type configurationVisitor struct {
	logger                             abstractlogger.Logger
	debug                              DebugConfiguration
	suggestionsSelectionReasonsEnabled bool

	operationName         string        // graphql query name
	operation, definition *ast.Document // graphql operation and schema documents
	walker                *astvisitor.Walker

	dataSources         []DataSource        // data sources configurations, which used by the current operation
	fieldConfigurations FieldConfigurations // field configuration from plan configuration

	planners []PlannerConfiguration // configurationVisitor is building this list of planners

	nodeSuggestions     *NodeSuggestions     // nodeSuggestions holds information about suggested data sources for each field
	nodeSuggestionHints []NodeSuggestionHint // nodeSuggestionHints holds information about suggested data sources for key fields

	parentTypeNodes               []ast.Node          // parentTypeNodes is a stack of parent type nodes - used to determine if the parent is abstract
	arrayFields                   []arrayField        // arrayFields is a stack of array fields - used to plan nested queries
	selectionSetRefs              []int               // selectionSetRefs is a stack of selection set refs - used to add a required fields
	skipFieldsRefs                []int               // skipFieldsRefs holds required field refs added by planner and should not be added to user response
	missingPathTracker            map[string]struct{} // missingPathTracker is a map of paths which will be added on secondary runs
	potentiallyMissingPathTracker map[string]struct{} // missingPathTracker is a map of paths which will be added on secondary runs
	addedPathTracker              []pathConfiguration // addedPathTracker is a list of paths which were added
	addedPathTrackerIndex         map[string][]int    // addedPathTrackerIndex is a map of path to index in addedPathTracker

	fieldDependenciesForPlanners map[int][]int // fieldDependenciesForPlanners is a map[FieldRef][]plannerIdx holds list of planner ids which depends on a field ref. Used for @key dependencies
	fieldsPlannedOn              map[int][]int // fieldsPlannedOn is a map[fieldRef][]plannerIdx holds list of planner ids which planned a field ref

	secondaryRun bool // secondaryRun is a flag to indicate that we're running the configurationVisitor not the first time
	fieldRef     int  // fieldRef is the reference for the current field; it is required by subscription filter to retrieve any variables

	fieldDependsOn           map[fieldIndexKey][]int // fieldDependsOn is a map[fieldRef][]fieldRef - holds list of field refs which are required by a field ref, e.g. field should be planned only after required fields were planned
	fieldRequirementsConfigs map[fieldIndexKey][]FederationFieldConfiguration
}

type FailedToCreatePlanningPathsError struct {
	MissingPaths                 []string
	HasFieldWaitingForDependency bool
}

func newFailedToCreatePlanningPathsError(missingPaths []string, hasFieldWaitingForDependency bool) *FailedToCreatePlanningPathsError {
	return &FailedToCreatePlanningPathsError{MissingPaths: missingPaths, HasFieldWaitingForDependency: hasFieldWaitingForDependency}
}

func (e *FailedToCreatePlanningPathsError) Error() string {
	return fmt.Sprintf("failed to create planning paths, missing paths: %v, has field waiting for dependency: %v", e.MissingPaths, e.HasFieldWaitingForDependency)
}

func (c *configurationVisitor) shouldRevisit() bool {
	return c.hasMissingPaths() || c.hasFieldsWaitingForDependency()
}

// selectionSetPendingRequirements - is a wrapper to been able to have predictable order of fieldsRequirementConfig but at the same time deduplicate fieldsRequirementConfig
type selectionSetPendingRequirements struct {
	existsTracker      map[fieldsRequirementConfig]struct{} // existsTracker allows us to not add duplicated fieldsRequirementConfig
	requirementConfigs []fieldsRequirementConfig            // requirementConfigs is a list of fieldsRequirementConfig which should be added to the selection set
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

type objectFetchConfiguration struct {
	object             *resolve.Object
	trigger            *resolve.GraphQLSubscriptionTrigger
	filter             *resolve.SubscriptionFilter
	planner            DataSourceFetchPlanner
	isSubscription     bool
	fieldRef           int
	fieldDefinitionRef int
	sourceID           string
	sourceName         string
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
	if c.debug.ConfigurationVisitor {
		c.debugPrint("saveAddedPath", configuration.String())
	}

	c.addedPathTracker = append(c.addedPathTracker, configuration)
	c.addedPathTrackerIndex[configuration.path] = append(c.addedPathTrackerIndex[configuration.path], len(c.addedPathTracker)-1)

	c.removeMissingPath(configuration.path)
}

func (c *configurationVisitor) addedPathDSHash(path string) (hash DSHash, ok bool) {
	indexes, ok := c.addedPathTrackerIndex[path]
	if !ok {
		return 0, false
	}

	// NOTE: it returns first occurence of such path
	if len(indexes) == 0 {
		return 0, false
	}

	return c.addedPathTracker[indexes[0]].dsHash, true
}

func (c *configurationVisitor) isPathAddedFor(path string, hash DSHash) bool {
	indexes, ok := c.addedPathTrackerIndex[path]
	if !ok {
		return false
	}

	for _, i := range indexes {
		if c.addedPathTracker[i].dsHash == hash {
			return true
		}
	}
	return false
}

func (c *configurationVisitor) addMissingPath(path string) {
	c.missingPathTracker[path] = struct{}{}
}

func (c *configurationVisitor) hasMissingPaths() bool {
	return len(c.missingPathTracker) > 0
}

// handleMissingPath - checks if the path was planned and if not, adds the path to the missing path tracker
func (c *configurationVisitor) populateMissingPahts() {
	for path := range c.potentiallyMissingPathTracker {
		if _, ok := c.addedPathDSHash(path); ok {
			continue
		}
		c.addMissingPath(path)
	}
	c.potentiallyMissingPathTracker = make(map[string]struct{})
}

func (c *configurationVisitor) removeMissingPath(path string) {
	delete(c.missingPathTracker, path)
}

func (c *configurationVisitor) debugPrint(args ...any) {
	if !c.debug.ConfigurationVisitor {
		return
	}

	printArgs := []any{"[configurationVisitor]: "}
	printArgs = append(printArgs, args...)
	fmt.Println(printArgs...)
}

func (c *configurationVisitor) EnterDocument(operation, definition *ast.Document) {
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

	c.missingPathTracker = make(map[string]struct{})
	c.potentiallyMissingPathTracker = make(map[string]struct{})
	c.addedPathTracker = make([]pathConfiguration, 0, 8)
	c.addedPathTrackerIndex = make(map[string][]int)

	c.fieldDependenciesForPlanners = make(map[int][]int)
	c.fieldsPlannedOn = make(map[int][]int)
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
	// We have to add a fragment path to the planner paths,
	// otherwise concrete planner will not pick up any path from such fragment
	// because we always check for the planner does it have a parent path for the current path
	// NOTE: in some cases datasource could have parent path, but no fields were planned within the fragment
	// such fragment paths do not have any nested fields, so they are obsolete and will be removed
	// when all paths for the query are planned. It happens in Planner.removeUnnecessaryFragmentPaths method
	ancestor := c.walker.Ancestor()
	if ancestor.Kind != ast.NodeKindInlineFragment {
		return
	}

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
			// we need to check also if an enclosing type is a union
			// because we don't have root/child node for a union type
			if c.walker.EnclosingTypeDefinition.Kind != ast.NodeKindUnionTypeDefinition {
				continue
			}
		}

		if planner.HasPath(currentPath) {
			continue
		}

		path := pathConfiguration{
			parentPath:       parentPath,
			path:             currentPath,
			shouldWalkFields: true,
			dsHash:           planner.DataSourceConfiguration().Hash(),
			fieldRef:         ast.InvalidRef,
			fragmentRef:      ancestor.Ref,
			pathType:         PathTypeFragment,
		}

		c.addPath(i, path)
	}
}

func (c *configurationVisitor) LeaveSelectionSet(ref int) {
	c.debugPrint("LeaveSelectionSet ref:", ref)
	// c.processPendingFieldRequirements(ref)
	c.selectionSetRefs = c.selectionSetRefs[:len(c.selectionSetRefs)-1]
	c.parentTypeNodes = c.parentTypeNodes[:len(c.parentTypeNodes)-1]
}

func (c *configurationVisitor) EnterField(fieldRef int) {
	fieldName := c.operation.FieldNameUnsafeString(fieldRef)
	fieldAliasOrName := c.operation.FieldAliasOrNameString(fieldRef)
	typeName := c.walker.EnclosingTypeDefinition.NameString(c.definition)

	c.debugPrint("EnterField ref:", fieldRef, "fieldName:", fieldName, "typeName:", typeName)

	parentPath := c.walker.Path.DotDelimitedString()
	// we need to also check preceding path for inline fragments
	// as for the field within inline fragment the parent path will include type condition in a path
	// but planner path still will not include it
	// this required to not produce multiple planners for the inline fragments
	precedingParentPath := parentPath

	var precedingPath ast.Path
	// we evaluate here the chain of inline fragments to get the preceding parent path
	// we will need to skip all consecutive inline fragments to get the correct parent path
	for i := len(c.walker.Path); i > 1; i-- {
		if c.walker.Path[i-1].Kind != ast.InlineFragmentName {
			break
		}

		precedingPath = c.walker.Path[:i-1]
	}
	if precedingPath != nil {
		precedingParentPath = precedingPath.DotDelimitedString()
	}

	currentPath := parentPath + "." + fieldAliasOrName

	c.addArrayField(fieldRef, currentPath)

	root := c.walker.Ancestors[0]
	if root.Kind != ast.NodeKindOperationDefinition {
		return
	}
	isSubscription := c.isSubscription(root.Ref, currentPath)

	suggestions := c.nodeSuggestions.SuggestionsForPath(typeName, fieldName, currentPath)
	shareable := len(suggestions) > 1
	for _, suggestion := range suggestions {
		dsIdx := slices.IndexFunc(c.dataSources, func(d DataSource) bool {
			return d.Hash() == suggestion.DataSourceHash
		})
		if dsIdx == -1 {
			c.walker.StopWithInternalErr(errors.New("we should always have a datasource for a suggestion"))
			return
		}
		ds := c.dataSources[dsIdx]

		if !c.couldPlanField(fieldRef, ds.Hash()) {
			c.handleMissingPath(false, typeName, fieldName, currentPath, shareable)

			// if we could not plan the field, we should skip walking into it
			// as the dependencies conditions are tight to this field,
			// and we could mistakenly plan the nested fields on this datasource without current field
			// It could happen when there are the same field as current on another datasource, and it is allowed to plan it
			c.walker.SkipNode()
			return
		}

		c.handlePlanningField(fieldRef, typeName, fieldName, currentPath, parentPath, precedingParentPath, isSubscription, suggestion, ds, shareable)
	}
}

func (c *configurationVisitor) handlePlanningField(fieldRef int, typeName, fieldName, currentPath, parentPath, precedingParentPath string, isSubscription bool, suggestion *NodeSuggestion, ds DataSource, shareable bool) {
	plannerIdx, planned := c.planWithExistingPlanners(fieldRef, typeName, fieldName, currentPath, parentPath, precedingParentPath, suggestion)
	if !planned {
		plannerIdx, planned = c.addNewPlanner(fieldRef, typeName, fieldName, currentPath, parentPath, isSubscription, ds)
	}

	if planned {
		c.recordFieldPlannedOn(fieldRef, plannerIdx)
		c.addFieldDependencies(fieldRef, typeName, fieldName, plannerIdx)
		c.addRootField(fieldRef, plannerIdx)
	}

	c.handleMissingPath(planned, typeName, fieldName, currentPath, shareable)
}

func (c *configurationVisitor) couldPlanField(fieldRef int, dsHash DSHash) (ok bool) {
	fieldKey := fieldIndexKey{fieldRef, dsHash}
	fieldRefs, ok := c.fieldDependsOn[fieldKey]
	if !ok {
		return true
	}

	for _, ref := range fieldRefs {
		_, planned := c.fieldsPlannedOn[ref]
		if !planned {
			return false
		}
	}

	return true
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
// e.g. when we have a record in a map, that this fieldRef is a dependency for the planner id
// we will notify that planner about the dependency on thecurrentPlannerIdx where this field is landed
func (c *configurationVisitor) addPlannerDependencies(fieldRef int, plannedOnPlannerId int) {
	plannerIds, mappingExists := c.fieldDependenciesForPlanners[fieldRef]
	if !mappingExists {
		return
	}

	for _, notifyPlannerIdx := range plannerIds {
		fetchConfiguration := c.planners[notifyPlannerIdx].ObjectFetchConfiguration()

		notified := slices.Contains(fetchConfiguration.dependsOnFetchIDs, plannedOnPlannerId)
		if !notified {
			if notifyPlannerIdx == plannedOnPlannerId {
				return
				// c.walker.StopWithInternalErr(fmt.Errorf("wrong fetch dependencies planner %d depends on itself", notifyPlannerIdx))
			}

			fetchConfiguration.dependsOnFetchIDs = append(fetchConfiguration.dependsOnFetchIDs, plannedOnPlannerId)
			slices.Sort(fetchConfiguration.dependsOnFetchIDs)
		}
	}
}

// recordFieldPlannedOn - records the planner id on which the field was planned
func (c *configurationVisitor) recordFieldPlannedOn(fieldRef int, plannerIdx int) {
	if !slices.Contains(c.fieldsPlannedOn[fieldRef], plannerIdx) {
		c.fieldsPlannedOn[fieldRef] = append(c.fieldsPlannedOn[fieldRef], plannerIdx)
	}
}

func (c *configurationVisitor) hasFieldsWaitingForDependency() bool {
	return len(c.fieldDependsOn) > 0
}

// addFieldDependencies adds dependencies between planners based on @requires directive
// in case current field has @requires directive, and we were able to plan it - it means that all fields from requires selection set was planned before that.
// So we need to notify planner of current fieldRef about dependencies on those other fields
// we know where fields were planned, because we record planner id of each planned field
func (c *configurationVisitor) addFieldDependencies(fieldRef int, typeName, fieldName string, currentPlannerIdx int) {
	dsHash := c.planners[currentPlannerIdx].DataSourceConfiguration().Hash()
	fieldKey := fieldIndexKey{fieldRef, dsHash}

	fieldRefs, mappingExists := c.fieldDependsOn[fieldKey]
	if !mappingExists {
		return
	}
	delete(c.fieldDependsOn, fieldKey)

	requiresConfigurations, ok := c.fieldRequirementsConfigs[fieldKey]
	if !ok {
		c.walker.StopWithInternalErr(fmt.Errorf("missing field requirements configuration for field %s.%s fieldRef %d", typeName, fieldName, fieldRef))
	}
	for _, requiresConfiguration := range requiresConfigurations {
		// add required fields to the current planner to pass it in the representation variables
		c.planners[currentPlannerIdx].RequiredFields().AppendIfNotPresent(requiresConfiguration)
	}

	// add dependency to current field planner for all fields which we were waiting for
	fetchConfiguration := c.planners[currentPlannerIdx].ObjectFetchConfiguration()
	for _, waitingForFieldRef := range fieldRefs {
		// we do not check if it exists, because we should not be able to plan a field with requires
		// in case we haven't planned all required fields
		plannerIds := c.fieldsPlannedOn[waitingForFieldRef]

		for _, plannerIdx := range plannerIds {
			// do not notify planner about itself
			// this could happen when we have requires directive on a field
			// but all fields from requires selection set were planned on the same planner because they are provided
			if plannerIdx == currentPlannerIdx {
				continue
			}

			notified := slices.Contains(fetchConfiguration.dependsOnFetchIDs, plannerIdx)
			if !notified {
				fetchConfiguration.dependsOnFetchIDs = append(fetchConfiguration.dependsOnFetchIDs, plannerIdx)
				slices.Sort(fetchConfiguration.dependsOnFetchIDs)
			}
		}
	}
}

func (c *configurationVisitor) isPlannerDependenciesAllowsToPlanField(fieldRef int, currentPlannerIdx int) bool {
	fieldKey := fieldIndexKey{fieldRef, c.planners[currentPlannerIdx].DataSourceConfiguration().Hash()}

	// we have a field which have `requires` directive and depends on some fields,
	// so we need to check is current planner already involved in this requires chain
	waitingFor := c.fieldDependsOn[fieldKey]

	// iterate over fields we depends on
	for _, waitingForFieldRef := range waitingFor {
		// get all planners which planned the field we depend on
		plannedOnPlannerIds := c.fieldsPlannedOn[waitingForFieldRef]

		// for each planner which has planned the field we depend on
		for _, plannedOnPlannerId := range plannedOnPlannerIds {
			// check if it has a dependency on the current planner id
			if slices.Contains(c.planners[plannedOnPlannerId].ObjectFetchConfiguration().dependsOnFetchIDs, currentPlannerIdx) {
				return false
			}
		}
	}

	return true
}

func (c *configurationVisitor) isAllFieldDependenciesOnSameDataSource(fieldRef int, currentPlannerIdx int) bool {
	fieldKey := fieldIndexKey{fieldRef, c.planners[currentPlannerIdx].DataSourceConfiguration().Hash()}

	// we have a field which have `requires` directive and depends on some fields,
	waitingFor := c.fieldDependsOn[fieldKey]

	// iterate over fields we depends on
	for _, waitingForFieldRef := range waitingFor {
		// get all planners which planned the field we depend on
		plannedOnPlannerIds := c.fieldsPlannedOn[waitingForFieldRef]

		for _, plannedOnPlannerId := range plannedOnPlannerIds {
			if plannedOnPlannerId != currentPlannerIdx {
				return false
			}
		}
	}

	return true
}

func (c *configurationVisitor) planWithExistingPlanners(fieldRef int, typeName, fieldName, currentPath, parentPath, precedingParentPath string, suggestion *NodeSuggestion) (plannerIdx int, planned bool) {
	for plannerIdx, plannerConfig := range c.planners {
		planningBehaviour := plannerConfig.DataSourcePlanningBehavior()
		dsConfiguration := plannerConfig.DataSourceConfiguration()
		currentPlannerDSHash := dsConfiguration.Hash()

		hasSuggestion := suggestion != nil
		if !hasSuggestion {
			continue
		}

		if suggestion.DataSourceHash != currentPlannerDSHash {
			continue
		}

		isProvided := suggestion.IsProvided
		isRootNode := suggestion.IsRootNode
		isChildNode := !isRootNode

		if c.secondaryRun && plannerConfig.HasPath(currentPath) {
			// on the secondary run we need to process only new fields added by the first run
			return plannerIdx, true
		}

		dsHash := dsConfiguration.Hash()
		fieldKey := fieldIndexKey{fieldRef, dsHash}
		requiresConfigurations := c.fieldRequirementsConfigs[fieldKey]
		fieldHasRequiresDirective := slices.ContainsFunc(requiresConfigurations, func(config FederationFieldConfiguration) bool {
			return config.FieldName != ""
		})

		if fieldHasRequiresDirective {
			// we should not plan fields with requires on a root level planner
			// because field with requires always will need an additional fetch before could be planned
			if !plannerConfig.IsNestedPlanner() && !c.isAllFieldDependenciesOnSameDataSource(fieldRef, plannerIdx) {
				continue
			}

			if !c.isPlannerDependenciesAllowsToPlanField(fieldRef, plannerIdx) {
				continue
			}
		}

		if plannerConfig.HasPath(parentPath) || plannerConfig.HasPath(precedingParentPath) {
			if pathAdded := c.addPlannerPathForTypename(plannerIdx, currentPath, parentPath, fieldRef, fieldName, typeName, planningBehaviour); pathAdded {
				return plannerIdx, true
			}

			if isProvided || (isRootNode && planningBehaviour.MergeAliasedRootNodes) || isChildNode {
				c.addPath(plannerIdx, pathConfiguration{
					parentPath:       parentPath,
					path:             currentPath,
					shouldWalkFields: true,
					typeName:         typeName,
					fieldRef:         fieldRef,
					fragmentRef:      ast.InvalidRef,
					enclosingNode:    c.walker.EnclosingTypeDefinition,
					dsHash:           currentPlannerDSHash,
					isRootNode:       isRootNode,
					pathType:         PathTypeField,
				})

				return plannerIdx, true
			}
		}
	}

	return -1, false
}

func (c *configurationVisitor) isParentPathIsRootOperationPath(parentPath string) bool {
	return parentPath == "query" || parentPath == "mutation" || parentPath == "subscription"
}

func (c *configurationVisitor) allowNewPlannerForTypenameField(fieldName string, typeName string, parentPath string, dsCfg DataSource) bool {
	fedCfg := dsCfg.FederationConfiguration()
	isEntityInterface := fedCfg.HasEntityInterface(typeName)

	if isEntityInterface {
		return true
	}

	// we should handle a new planner for a __typename
	// only when it is the first field on a query,
	// or we are on the entity interface object
	return c.isParentPathIsRootOperationPath(parentPath)
}

func (c *configurationVisitor) addNewPlanner(fieldRef int, typeName, fieldName, currentPath, parentPath string, isSubscription bool, dsConfig DataSource) (plannerIdx int, planned bool) {
	if !dsConfig.HasRootNode(typeName, fieldName) {
		if fieldName != typeNameField {
			return -1, false
		}

		if !c.allowNewPlannerForTypenameField(fieldName, typeName, parentPath, dsConfig) {
			return -1, false
		}
	}

	currentPathConfiguration := pathConfiguration{
		parentPath:       parentPath,
		path:             currentPath,
		shouldWalkFields: true,
		typeName:         typeName,
		fieldRef:         fieldRef,
		fragmentRef:      ast.InvalidRef,
		enclosingNode:    c.walker.EnclosingTypeDefinition,
		dsHash:           dsConfig.Hash(),
		isRootNode:       true,
		pathType:         PathTypeField,
	}

	paths := []pathConfiguration{
		currentPathConfiguration,
	}

	isParentAbstract := c.isParentTypeNodeAbstractType()
	isParentFragment := c.walker.Path[len(c.walker.Path)-1].Kind == ast.InlineFragmentName
	fragmentRef := ast.InvalidRef

	if isParentFragment {
		fragmentRef = c.walker.Ancestors[len(c.walker.Ancestors)-2].Ref
	}

	if isParentAbstract && isParentFragment {
		// if the parent is abstract and path is on a fragment parent, we add the parent path of type fragment
		// this will ensure that we're walking into and out of the root inline fragments
		// otherwise, we'd only walk into the fields inside the inline fragments in the root,
		// so we'd miss the selection sets and inline fragments in the root
		paths = append([]pathConfiguration{
			{
				path:             parentPath,
				shouldWalkFields: false,
				dsHash:           dsConfig.Hash(),
				fieldRef:         ast.InvalidRef,
				fragmentRef:      fragmentRef,
				pathType:         PathTypeFragment,
			},
		}, paths...)
	} else {
		pathType := PathTypeParent
		if isParentFragment {
			pathType = PathTypeFragment
		}

		// add potentially missing parent path
		// this could happen when the parent is a fragment and we walking nested selection sets
		paths = append([]pathConfiguration{
			{
				path:             parentPath,
				shouldWalkFields: true,
				dsHash:           dsConfig.Hash(),
				fieldRef:         ast.InvalidRef,
				fragmentRef:      fragmentRef,
				pathType:         pathType,
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
				dsHash:           dsConfig.Hash(),
				fieldRef:         ast.InvalidRef,
				fragmentRef:      ast.InvalidRef,
				pathType:         PathTypeParent,
			},
		}, paths...)

		// if the parent is a fragment, we use the preceding parent path as the planner path
		// to avoid creating multiple planners for the same upstream
		plannerPath = precedingFragmentPath
	}

	fieldDefinition, ok := c.walker.FieldDefinition(fieldRef)
	if !ok {
		return -1, false
	}

	// fetch id is an index of the current planner
	fetchID := len(c.planners)

	// the filter needs access to fieldRef to retrieve the field argument variable
	c.fieldRef = fieldRef

	fetchConfiguration := &objectFetchConfiguration{
		isSubscription:     isSubscription,
		fieldRef:           fieldRef,
		fieldDefinitionRef: fieldDefinition,
		fetchID:            fetchID,
		sourceID:           dsConfig.Id(),
		sourceName:         dsConfig.Name(),
		operationType:      c.resolveRootFieldOperationType(typeName),
		filter:             c.resolveSubscriptionFilterCondition(typeName, fieldName),
	}

	plannerPathConfig := newPlannerPathsConfiguration(
		plannerPath,
		c.plannerPathType(plannerPath),
		paths,
	)

	plannerConfig := dsConfig.CreatePlannerConfiguration(c.logger, fetchConfiguration, plannerPathConfig)

	c.planners = append(c.planners, plannerConfig)

	for _, pathConfiguration := range paths {
		c.saveAddedPath(pathConfiguration)
	}

	return len(c.planners) - 1, true
}

func (c *configurationVisitor) resolveSubscriptionFilterCondition(typeName, fieldName string) *resolve.SubscriptionFilter {
	fieldConfig := c.fieldConfigurations.ForTypeField(typeName, fieldName)
	if fieldConfig == nil {
		return nil
	}
	if fieldConfig.SubscriptionFilterCondition == nil {
		return nil
	}
	return c.buildSubscriptionFilterCondition(*fieldConfig.SubscriptionFilterCondition)
}

func (c *configurationVisitor) buildSubscriptionFilterCondition(condition SubscriptionFilterCondition) *resolve.SubscriptionFilter {
	filter := &resolve.SubscriptionFilter{}
	if condition.And != nil {
		for _, andCondition := range condition.And {
			and := c.buildSubscriptionFilterCondition(andCondition)
			if and != nil {
				filter.And = append(filter.And, *and)
			}
		}
	}
	if condition.Or != nil {
		for _, orCondition := range condition.Or {
			or := c.buildSubscriptionFilterCondition(orCondition)
			if or != nil {
				filter.Or = append(filter.Or, *or)
			}
		}
	}
	if condition.Not != nil {
		filter.Not = c.buildSubscriptionFilterCondition(*condition.Not)
	}
	if condition.In != nil {
		filter.In = c.buildSubscriptionFieldFilter(condition.In)
	}
	if filter.And == nil && filter.Or == nil && filter.Not == nil && filter.In == nil {
		return nil
	}
	return filter
}

func (c *configurationVisitor) buildSubscriptionFieldFilter(condition *SubscriptionFieldCondition) *resolve.SubscriptionFieldFilter {
	filter := &resolve.SubscriptionFieldFilter{}
	filter.FieldPath = condition.FieldPath
	filter.Values = make([]resolve.InputTemplate, len(condition.Values))
	for i, value := range condition.Values {
		matches := argument_templates.ArgumentTemplateRegex.FindAllStringSubmatchIndex(value, -1)
		if len(matches) == 0 {
			filter.Values[i].Segments = []resolve.TemplateSegment{
				{
					SegmentType: resolve.StaticSegmentType,
					Data:        []byte(value),
				},
			}
			continue
		}
		fieldNameBytes := c.operation.FieldNameBytes(c.fieldRef)
		fieldDefinitionRef, ok := c.definition.ObjectTypeDefinitionFieldWithName(c.walker.EnclosingTypeDefinition.Ref, fieldNameBytes)
		if !ok {
			c.walker.StopWithInternalErr(fmt.Errorf(`expected field definition to exist for field "%s"`, fieldNameBytes))
			return nil
		}
		groups := matches[0]
		/* The range value[0:groups[0]] is a prefix (if any—an empty prefix still provides an index)
		 * The range value[groups[1]:groups[2]] is the whole argument template
		 * The range value[groups[2]:groups[3]] is the argument path
		 * The range groups[1] to the end of value is the suffix (if any)
		 */
		if len(matches) != 1 || len(groups) != 4 {
			return nil
		}
		argumentPathGroup := value[groups[2]:groups[3]]
		validationResult, err := argument_templates.ValidateArgumentPath(c.definition, argumentPathGroup, fieldDefinitionRef)
		if err != nil {
			c.walker.StopWithInternalErr(fmt.Errorf(`argument template defined on field "%s" is invalid: %w`, fieldNameBytes, err))
			return nil
		}
		prefix := value[:groups[0]]
		hasPrefix := len(prefix) > 0
		argumentNameBytes := []byte(validationResult.ArgumentPath[0])
		argumentRef, ok := c.operation.FieldArgument(c.fieldRef, argumentNameBytes)
		if !ok {
			c.walker.StopWithInternalErr(fmt.Errorf(`operation field "%s" does not define argument "%s"`, fieldNameBytes, argumentNameBytes))
			return nil
		}
		variablePath, err := c.operation.VariablePathByArgumentRefAndArgumentPath(argumentRef, validationResult.ArgumentPath, c.walker.Ancestors[0].Ref)
		if err != nil {
			c.walker.StopWithInternalErr(fmt.Errorf(`failed to create template segment for argument "%s" defined on operation field "%s": %w`, argumentNameBytes, fieldNameBytes, err))
			return nil
		}
		suffix := value[groups[1]:]
		hasSuffix := len(suffix) > 0
		size := 1
		if hasPrefix {
			size++
		}
		if hasSuffix {
			size++
		}
		filter.Values[i].Segments = make([]resolve.TemplateSegment, size)
		idx := 0
		if hasPrefix {
			filter.Values[i].Segments[idx] = resolve.TemplateSegment{
				SegmentType: resolve.StaticSegmentType,
				Data:        []byte(prefix),
			}
			idx++
		}
		filter.Values[i].Segments[idx] = resolve.TemplateSegment{
			SegmentType:        resolve.VariableSegmentType,
			VariableKind:       resolve.ContextVariableKind,
			Renderer:           resolve.NewPlainVariableRenderer(),
			VariableSourcePath: variablePath,
		}
		if hasSuffix {
			filter.Values[i].Segments[idx+1] = resolve.TemplateSegment{
				SegmentType: resolve.StaticSegmentType,
				Data:        []byte(suffix),
			}
		}
		continue
	}
	return filter
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
func (c *configurationVisitor) handleMissingPath(planned bool, typeName string, fieldName string, currentPath string, shareable bool) {
	suggestions := c.nodeSuggestions.SuggestionsForPath(typeName, fieldName, currentPath)

	if len(suggestions) <= 1 {
		if planned {
			// __typename field on a union could not have a suggestion
			return
		} else {
			if c.debug.PrintPlanningPaths {
				fmt.Println("Adding potentially missing path", currentPath)
			}

			c.potentiallyMissingPathTracker[currentPath] = struct{}{}
		}
	}

	allSuggestionsPlanned := true

	for _, suggestion := range suggestions {
		hasPlannedSuggestion := false
		for i := range c.planners {
			if c.planners[i].DataSourceConfiguration().Hash() != suggestion.DataSourceHash {
				continue
			}
			if c.planners[i].HasPath(currentPath) {
				hasPlannedSuggestion = true
				break
			}
		}
		if !hasPlannedSuggestion {
			allSuggestionsPlanned = false
			break
		}
	}

	if allSuggestionsPlanned {
		// all suggestions were planned, so we should not record a missing path
		return
	}

	if !shareable {
		c.walker.SkipNode()
	}
}

func (c *configurationVisitor) LeaveField(ref int) {
	c.removeArrayField(ref)

	fieldAliasOrName := c.operation.FieldAliasOrNameString(ref)
	typeName := c.walker.EnclosingTypeDefinition.NameString(c.definition)
	c.debugPrint("LeaveField ref:", ref, "fieldName:", fieldAliasOrName, "typeName:", typeName)

	if !c.secondaryRun {
		// we should evaluate exit paths only on the second run
		return
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
		parentPath:       parentPath,
		path:             currentPath,
		shouldWalkFields: true,
		typeName:         typeName,
		fieldRef:         fieldRef,
		fragmentRef:      ast.InvalidRef,
		dsHash:           c.planners[plannerIndex].DataSourceConfiguration().Hash(),
		pathType:         PathTypeField,
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
