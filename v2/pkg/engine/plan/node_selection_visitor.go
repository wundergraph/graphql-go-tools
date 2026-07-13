package plan

import (
	"bytes"
	"fmt"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

// nodeSelectionVisitor walks through the operation multiple times to rewrite it
// to be able to resolve fields from different data sources.
// If necessary, this visitor might add required fields and rewrite abstract selections.
//
// This visitor will walk the operation again if it has:
//   - added new required fields to the operation,
//   - rewritten an abstract field selection set.
type nodeSelectionVisitor struct {
	debug DebugConfiguration

	operationName         string        // graphql query name
	operation, definition *ast.Document // graphql operation and schema documents
	walker                *astvisitor.Walker

	dataSources     []DataSource     // data sources configurations, used by the current operation
	nodeSuggestions *NodeSuggestions // nodeSuggestions holds information about suggested data sources for each field

	selectionSetRefs []int // selectionSetRefs is a stack of selection set refs - used to add required fields
	skipFieldsRefs   []int // skipFieldsRefs holds required field refs added by planner and should not be added to user response

	pendingKeyRequirements   map[int]pendingKeyRequirements   // pendingKeyRequirements is a map[selectionSetRef][]keyRequirements
	pendingFieldRequirements map[int]pendingFieldRequirements // pendingFieldRequirements is a map[selectionSetRef]fieldRequirements

	visitedFieldsRequiresChecks map[fieldIndexKey]struct{}                       // visitedFieldsRequiresChecks is a map[fieldIndexKey] of already processed fields which we check for presence of @requires directive
	visitedFieldsKeyChecks      map[fieldIndexKey]struct{}                       // visitedFieldsKeyChecks is a map[fieldIndexKey] of already processed fields which we check for @key requirements
	visitedFieldsAbstractChecks map[int]struct{}                                 // visitedFieldsAbstractChecks is a map[fieldRef] of already processed fields which we check for abstract type, e.g. union or interface
	fieldDependsOn              map[fieldIndexKey][]int                          // fieldDependsOn is a map[fieldIndexKey][]fieldRef - holds list of field refs which are required by a field ref, e.g. field should be planned only after required fields were planned
	fieldRefDependsOn           map[int][]int                                    // fieldRefDependsOn is a map[fieldRef][]fieldRef - holds list of field refs which are required by a field ref, it is a second index without datasource hash
	fieldRequirementsConfigs    map[fieldIndexKey][]FederationFieldConfiguration // fieldRequirementsConfigs is a map[fieldIndexKey]FederationFieldConfiguration - holds a list of required configuratuibs for a field ref to later built representation variables
	fieldLandedTo               map[int]DSHash                                   // fieldLandedTo is a map[fieldRef]DSHash - holds a datasource hash where field was landed to
	fieldDependencyKind         map[fieldDependencyKey]fieldDependencyKind

	secondaryRun bool // secondaryRun is a flag to indicate that we're running the nodeSelectionVisitor not the first time
	hasNewFields bool // hasNewFields is used to determine if we need to run the planner again. It will be true in case required fields were added

	rewrittenFieldRefs          []int            // rewrittenFieldRefs holds field refs which had their selection sets rewritten during the current walk
	persistedRewrittenFieldRefs map[int]struct{} // persistedRewrittenFieldRefs holds field refs which had their selection sets rewritten during any of the walks

	// addTypenameInNestedSelections controls forced addition of __typename to nested selection sets
	// used by "requires" keys, not only when fragments are present.
	addTypenameInNestedSelections bool

	newFieldRefs map[int]struct{} // newFieldRefs is a set of field refs which were added by the visitor or was modified by a rewrite
}

func (c *nodeSelectionVisitor) addNewSkipFieldRefs(fieldRefs ...int) {
	c.addSkipFieldRefs(fieldRefs...)
	c.addNewFieldRefs(fieldRefs...)
}

func (c *nodeSelectionVisitor) addSkipFieldRefs(fieldRefs ...int) {
	c.skipFieldsRefs = append(c.skipFieldsRefs, fieldRefs...)
}

func (c *nodeSelectionVisitor) addNewFieldRefs(fieldRefs ...int) {
	for _, fieldRef := range fieldRefs {
		c.newFieldRefs[fieldRef] = struct{}{}
	}
}

type fieldDependencyKey struct {
	field, dependsOn int
}

type fieldIndexKey struct {
	fieldRef int
	dsHash   DSHash
}

// selectionSetPendingRequirements - is a wrapper to been able to have predictable order of keyRequirements but at the same time deduplicate keyRequirements
type pendingKeyRequirementExistsKey struct {
	dsHash  DSHash
	deferID int
}

type pendingKeyRequirements struct {
	existsTracker      map[pendingKeyRequirementExistsKey]int // maps an exists key to the index of its keyRequirements config (dedupe + direct lookup)
	requirementConfigs []keyRequirements                      // requirementConfigs is a list of keyRequirements which should be added to the selection set
}

// keyRequirements is a mapping between requestedByPlannerID or requestedByFieldRef, which requested required fields,
// and key selectionSet which should be added
type keyRequirements struct {
	targetDSHash         DSHash
	path                 string
	isInterfaceObject    bool
	sc                   SourceConnection
	requestedByFieldRefs []int
	typeName             string
	deferInfo            *DeferInfo
	parentFieldDeferID   int
}

type fieldRequirements struct {
	dsHash                       DSHash
	path                         string
	selectionSet                 string
	requestedByFieldRefs         []int
	isTypenameForEntityInterface bool
	deferInfo                    *DeferInfo
	parentFieldDeferID           int
}

type pendingFieldRequirements struct {
	existsTracker      map[pendingFieldRequirementExistsKey]int // maps an exists key to the index of its fieldRequirements config (dedupe + direct lookup)
	requirementConfigs []fieldRequirements                      // requirementConfigs is a list of fieldRequirements which should be added to the selection set
}

type pendingFieldRequirementExistsKey struct {
	dsHash                       DSHash
	selectionSet                 string
	isTypenameForEntityInterface bool
	deferID                      int
}

func (c *nodeSelectionVisitor) currentSelectionSet() int {
	if len(c.selectionSetRefs) == 0 {
		return ast.InvalidRef
	}

	return c.selectionSetRefs[len(c.selectionSetRefs)-1]
}

func (c *nodeSelectionVisitor) debugPrint(args ...any) {
	if !c.debug.ConfigurationVisitor {
		return
	}

	printArgs := []any{"[nodeSelectionVisitor]: "}
	printArgs = append(printArgs, args...)
	fmt.Println(printArgs...)
}

func (c *nodeSelectionVisitor) EnterDocument(operation, definition *ast.Document) {
	c.hasNewFields = false
	c.rewrittenFieldRefs = c.rewrittenFieldRefs[:0]

	if c.selectionSetRefs == nil {
		c.selectionSetRefs = make([]int, 0, 8)
	} else {
		c.selectionSetRefs = c.selectionSetRefs[:0]
	}

	if c.secondaryRun {
		return
	}

	c.operation, c.definition = operation, definition

	if c.skipFieldsRefs == nil {
		c.skipFieldsRefs = make([]int, 0, 8)
	}

	c.persistedRewrittenFieldRefs = make(map[int]struct{})
	c.visitedFieldsAbstractChecks = make(map[int]struct{})
	c.visitedFieldsRequiresChecks = make(map[fieldIndexKey]struct{})
	c.visitedFieldsKeyChecks = make(map[fieldIndexKey]struct{})
	c.pendingKeyRequirements = make(map[int]pendingKeyRequirements)
	c.pendingFieldRequirements = make(map[int]pendingFieldRequirements)
	c.fieldDependencyKind = make(map[fieldDependencyKey]fieldDependencyKind)

	c.fieldDependsOn = make(map[fieldIndexKey][]int)
	c.fieldRefDependsOn = make(map[int][]int)
	c.fieldRequirementsConfigs = make(map[fieldIndexKey][]FederationFieldConfiguration)
	c.fieldLandedTo = make(map[int]DSHash)
}

func (c *nodeSelectionVisitor) LeaveDocument(operation, definition *ast.Document) {}

func (c *nodeSelectionVisitor) EnterOperationDefinition(ref int) {
	operationName := c.operation.OperationDefinitionNameString(ref)
	if c.operationName != operationName {
		c.walker.SkipNode()
		return
	}
}

func (c *nodeSelectionVisitor) EnterSelectionSet(ref int) {
	c.debugPrint("EnterSelectionSet ref:", ref)
	c.selectionSetRefs = append(c.selectionSetRefs, ref)

	c.handleRequiresInSelectionSet(ref)
}

// handleRequiresInSelectionSet adds required fields for fields having @requires directive
// before walker actually walks into field selections
// this is needed for the case when requires configuration has deeply nested fields
// which will modify current field sublings
func (c *nodeSelectionVisitor) handleRequiresInSelectionSet(selectionSetRef int) {
	fieldSelectionsRefs := c.operation.SelectionSetFieldSelections(selectionSetRef)
	for _, fieldSelectionRef := range fieldSelectionsRefs {
		fieldRef := c.operation.Selections[fieldSelectionRef].Ref
		// process the field but handle only requires configurations
		c.handleEnterField(fieldRef, true)
	}

	// add required fields into operation right away
	// so when the walker walks into fields, required fields will be already present
	c.processPendingFieldRequirements(selectionSetRef)
}

func (c *nodeSelectionVisitor) LeaveSelectionSet(ref int) {
	c.debugPrint("LeaveSelectionSet ref:", ref)
	c.processPendingKeyRequirements(ref)
	c.selectionSetRefs = c.selectionSetRefs[:len(c.selectionSetRefs)-1]
}

func (c *nodeSelectionVisitor) EnterField(fieldRef int) {
	// process field to handle keys and do rewrites of abstract selections
	c.handleEnterField(fieldRef, false)
}

type fieldRequirementsContext struct {
	fieldRef           int
	parentPath         string
	typeName           string
	fieldName          string
	currentPath        string
	dsConfig           DataSource
	deferInfo          *DeferInfo
	parentFieldDeferID int
}

func (c *nodeSelectionVisitor) handleEnterField(fieldRef int, handleRequires bool) {
	root := c.walker.Ancestors[0]
	if root.Kind != ast.NodeKindOperationDefinition {
		return
	}

	fieldName := c.operation.FieldNameUnsafeString(fieldRef)
	fieldAliasOrName := c.operation.FieldAliasOrNameString(fieldRef)
	typeName := c.walker.EnclosingTypeDefinition.NameString(c.definition)

	c.debugPrint("EnterField ref:", fieldRef, "fieldName:", fieldName, "typeName:", typeName, "requires:", handleRequires)

	parentPath := c.walker.Path.DotDelimitedString()
	currentPath := parentPath + "." + fieldAliasOrName

	suggestions := c.nodeSuggestions.SuggestionsForPath(typeName, fieldName, currentPath)

	for _, suggestion := range suggestions {
		dsIdx := slices.IndexFunc(c.dataSources, func(d DataSource) bool {
			return d.Hash() == suggestion.DataSourceHash
		})
		if dsIdx == -1 {
			c.walker.StopWithInternalErr(fmt.Errorf("do not have a datasource for a field suggestion for field %s at path %s", fieldName, currentPath))
			return
		}

		fieldCtx := fieldRequirementsContext{
			fieldRef:           fieldRef,
			parentPath:         parentPath,
			typeName:           typeName,
			fieldName:          fieldName,
			currentPath:        currentPath,
			dsConfig:           c.dataSources[dsIdx],
			deferInfo:          suggestion.deferInfo,
			parentFieldDeferID: c.wrappingFieldDeferID(),
		}

		if handleRequires {
			// check if the field has @requires directive
			c.handleFieldRequiredByRequires(fieldCtx)
			// skip to the next suggestion as we only handle requires here
			continue
		}

		if suggestion.requiresKey != nil {
			// add @key requirements for the field
			c.handleFieldsRequiredByKey(fieldCtx, *suggestion.requiresKey)
		}

		// check if field selections are abstract and needs rewrites
		c.rewriteSelectionSetHavingAbstractFragments(fieldRef, fieldCtx.dsConfig)
	}
}

// wrappingFieldDeferID returns the defer id of the nearest wrapping field, or 0
// if the current field is not inside a defer scope.
//
// It is enough to inspect the nearest field ancestor: the defer normalization
// (astnormalization.deferExpandIntoInternal) stamps @__defer_internal onto every
// field in every selection set inside a deferred fragment. So if the nearest
// field ancestor carries the directive, this field is deferred under it; if it
// does not, this field is outside any defer scope. That is why we return 0
// immediately when the nearest field ancestor lacks the directive instead of
// continuing to climb.
func (c *nodeSelectionVisitor) wrappingFieldDeferID() int {
	for _, v := range slices.Backward(c.walker.Ancestors) {
		ancestor := v
		if ancestor.Kind != ast.NodeKindField {
			continue
		}
		id, exists := c.operation.FieldInternalDeferID(ancestor.Ref)
		if !exists {
			// nearest field ancestor is not deferred -> not in a defer scope
			return 0
		}
		return id
	}
	return 0
}

func (c *nodeSelectionVisitor) LeaveField(ref int) {
	// "__internal_typename" is an internal typename placeholder
	// added by astnormalization.directiveIncludeSkip or astnormalization.deferEnsureTypename normalization rule
	if bytes.Equal(c.operation.FieldAliasOrNameBytes(ref), literal.INTERNAL_TYPENAME) {
		// we should skip such typename as it was added as a placeholder to keep query valid
		// when normalizaion removed all other selections from the selection set
		c.addSkipFieldRefs(ref)
	}
}

func (c *nodeSelectionVisitor) handleFieldRequiredByRequires(fieldCtx fieldRequirementsContext) {
	fieldKey := fieldIndexKey{fieldCtx.fieldRef, fieldCtx.dsConfig.Hash()}
	_, visited := c.visitedFieldsRequiresChecks[fieldKey]
	if visited {
		return
	}
	c.visitedFieldsRequiresChecks[fieldKey] = struct{}{}

	if fieldCtx.fieldName == typeNameField {
		// the __typename field could not have @requires directive
		return
	}

	requiresConfiguration, exists := fieldCtx.dsConfig.RequiredFieldsByRequires(fieldCtx.typeName, fieldCtx.fieldName)

	if !exists {
		for _, io := range fieldCtx.dsConfig.FederationConfiguration().InterfaceObjects {
			if slices.Contains(io.ConcreteTypeNames, fieldCtx.typeName) {
				// we should check if we have a @requires configuration for the interface object
				requiresConfiguration, exists = fieldCtx.dsConfig.RequiredFieldsByRequires(io.InterfaceTypeName, fieldCtx.fieldName)
				if exists {
					requiresConfiguration.TypeName = fieldCtx.typeName
					break
				}
			}
		}
	}

	if !exists {
		// we do not have a @requires configuration for the field
		return
	}

	// check if the required fields are already provided
	input := areRequiredFieldsProvidedInput{
		typeName:          fieldCtx.typeName,
		requiredFields:    requiresConfiguration.SelectionSet,
		definition:        c.definition,
		dataSource:        fieldCtx.dsConfig,
		providedSelection: c.nodeSuggestions.providedSelectionForParentOfField(fieldCtx.dsConfig.Hash(), fieldCtx.fieldRef),
	}

	provided, report := areRequiredFieldsProvided(input)
	if report.HasErrors() {
		c.walker.StopWithInternalErr(fmt.Errorf("failed to check if required fields are provided for field %s at path %s: %w", fieldCtx.fieldName, fieldCtx.currentPath, report))
		return
	}

	if provided {
		// if all fields from requires configuration are provided, we do not need to add them to the operation
		return
	}

	// we should plan to add required fields for the field
	// they will be added in the on LeaveSelectionSet callback for the current selection set,
	// and the current field ref will be added to the fieldDependsOn map
	c.addPendingFieldRequirements(fieldCtx, requiresConfiguration, false)
	c.handleKeyRequirementsForBackJumpOnSameDataSource(fieldCtx)
}

func (c *nodeSelectionVisitor) handleFieldsRequiredByKey(fieldCtx fieldRequirementsContext, sc SourceConnection) {
	fieldKey := fieldIndexKey{fieldCtx.fieldRef, fieldCtx.dsConfig.Hash()}
	_, visited := c.visitedFieldsKeyChecks[fieldKey]
	if visited {
		return
	}
	c.visitedFieldsKeyChecks[fieldKey] = struct{}{}

	selectedParentsDSHashes := c.getSelectedParentsDSHashes(fieldCtx.fieldRef)

	isParentHasInterfaceObject := slices.ContainsFunc(selectedParentsDSHashes, func(dsHash DSHash) bool {
		dsIdx := slices.IndexFunc(c.dataSources, func(d DataSource) bool {
			return d.Hash() == dsHash
		})
		if dsIdx == -1 {
			return false
		}

		return c.dataSources[dsIdx].HasInterfaceObject(fieldCtx.typeName)
	})

	entityInterface := fieldCtx.dsConfig.HasEntityInterface(fieldCtx.typeName)
	interfaceObject := fieldCtx.dsConfig.HasInterfaceObject(fieldCtx.typeName)

	if fieldCtx.fieldName == typeNameField && !entityInterface {
		// the __typename field could not have @key directive
		// but for the interface object we have to plan it differently
		// e.g. we should get a __typename from a concrete type, not the interface object
		// it means for the entity interface we should evaluate key deps on a __typename field
		return
	}

	c.addPendingKeyRequirements(fieldCtx, sc, interfaceObject)

	if isParentHasInterfaceObject && !interfaceObject && !entityInterface {
		c.addPendingFieldRequirements(
			fieldCtx,
			FederationFieldConfiguration{
				TypeName:     fieldCtx.typeName,
				FieldName:    fieldCtx.fieldName,
				SelectionSet: typeNameField,
			},
			true,
		)
	}
}

func (c *nodeSelectionVisitor) getSelectedParentsDSHashes(fieldRef int) (out []DSHash) {
	treeNodeID := TreeNodeID(fieldRef)
	treeNode, ok := c.nodeSuggestions.responseTree.Find(treeNodeID)
	if !ok {
		return nil
	}

	parentIndexes := treeNode.GetParent().GetData()

	for _, itemID := range parentIndexes {
		if c.nodeSuggestions.items[itemID].Selected {
			out = append(out, c.nodeSuggestions.items[itemID].DataSourceHash)
		}
	}

	return out
}

func (c *nodeSelectionVisitor) handleKeyRequirementsForBackJumpOnSameDataSource(fieldCtx fieldRequirementsContext) {
	selectedParentsDSHashes := c.getSelectedParentsDSHashes(fieldCtx.fieldRef)

	// regularly keys are required only when the datasource hash differs from the parent datasource hash
	// one exception when the field has requires directive and planned on the same datasource as a parent
	// in this case we have to add a back jump on the same datasource to get required fields for the field resolver
	// but jump is possible only with keys, so we have to add any key for this datasource
	sameAsParentDS := len(selectedParentsDSHashes) == 1 && selectedParentsDSHashes[0] == fieldCtx.dsConfig.Hash()
	if !sameAsParentDS {
		return
	}

	keyConfigurations := fieldCtx.dsConfig.RequiredFieldsByKey(fieldCtx.typeName)

	if len(keyConfigurations) == 0 {
		// required fields could be of zero length in case type is not entity
		// or when entity has disabled entity resolver.
		// Usually we can't jump to the entity with disabled entity resolver, but there is one known exception
		// When entity has disabled entity resolver, but we have field with requires directive on this entity
		// we should add key fields for the field with requires - to pass them into field resolver

		keys := fieldCtx.dsConfig.FederationConfiguration().Keys
		keyConfigurations = keys.FilterByTypeAndResolvability(fieldCtx.typeName, false)
	}

	if len(keyConfigurations) == 0 {
		return
	}

	keyToUse := keyConfigurations[0]

	sc := SourceConnection{
		Type: SourceConnectionTypeDirect,
		Jumps: []KeyJump{
			{
				From:         fieldCtx.dsConfig.Hash(),
				To:           fieldCtx.dsConfig.Hash(),
				SelectionSet: keyToUse.SelectionSet,
				TypeName:     fieldCtx.typeName,
			},
		},
	}

	c.addPendingKeyRequirements(fieldCtx, sc, false)
}

func (c *nodeSelectionVisitor) addPendingFieldRequirements(fieldCtx fieldRequirementsContext, fieldConfiguration FederationFieldConfiguration, isTypenameForEntityInterface bool) {
	currentSelectionSet := c.currentSelectionSet()

	requirements, hasRequirements := c.pendingFieldRequirements[currentSelectionSet]
	if !hasRequirements {
		requirements = pendingFieldRequirements{
			existsTracker: make(map[pendingFieldRequirementExistsKey]int),
		}
	}

	deferID := 0
	if fieldCtx.deferInfo != nil {
		deferID = fieldCtx.deferInfo.ID
	}
	existsKey := pendingFieldRequirementExistsKey{fieldCtx.dsConfig.Hash(), fieldConfiguration.SelectionSet, isTypenameForEntityInterface, deferID}
	if idx, exists := requirements.existsTracker[existsKey]; exists {
		// already tracked for this scope (including deferID) - just record the requesting field ref
		cfg := &requirements.requirementConfigs[idx]
		if !slices.Contains(cfg.requestedByFieldRefs, fieldCtx.fieldRef) {
			cfg.requestedByFieldRefs = append(cfg.requestedByFieldRefs, fieldCtx.fieldRef)
		}
	} else {
		config := fieldRequirements{
			dsHash:                       fieldCtx.dsConfig.Hash(),
			path:                         fieldCtx.currentPath,
			selectionSet:                 fieldConfiguration.SelectionSet,
			requestedByFieldRefs:         []int{fieldCtx.fieldRef},
			isTypenameForEntityInterface: isTypenameForEntityInterface,
			deferInfo:                    fieldCtx.deferInfo,
			parentFieldDeferID:           fieldCtx.parentFieldDeferID,
		}

		requirements.existsTracker[existsKey] = len(requirements.requirementConfigs)
		requirements.requirementConfigs = append(requirements.requirementConfigs, config)
	}

	c.pendingFieldRequirements[currentSelectionSet] = requirements
}

func (c *nodeSelectionVisitor) addPendingKeyRequirements(fieldCtx fieldRequirementsContext, sc SourceConnection, isInterfaceObject bool) {
	currentSelectionSet := c.currentSelectionSet()

	requirements, hasRequirements := c.pendingKeyRequirements[currentSelectionSet]

	if !hasRequirements {
		requirements = pendingKeyRequirements{
			existsTracker: make(map[pendingKeyRequirementExistsKey]int),
		}
	}

	deferID := 0
	if fieldCtx.deferInfo != nil {
		deferID = fieldCtx.deferInfo.ID
	}
	existsKey := pendingKeyRequirementExistsKey{dsHash: fieldCtx.dsConfig.Hash(), deferID: deferID}
	if idx, exists := requirements.existsTracker[existsKey]; exists {
		// already tracked for this (dsHash, deferID) scope - just record the requesting field ref
		cfg := &requirements.requirementConfigs[idx]
		if !slices.Contains(cfg.requestedByFieldRefs, fieldCtx.fieldRef) {
			cfg.requestedByFieldRefs = append(cfg.requestedByFieldRefs, fieldCtx.fieldRef)
		}
	} else {
		config := keyRequirements{
			targetDSHash:         fieldCtx.dsConfig.Hash(),
			path:                 fieldCtx.parentPath,
			isInterfaceObject:    isInterfaceObject,
			sc:                   sc,
			requestedByFieldRefs: []int{fieldCtx.fieldRef},
			typeName:             fieldCtx.typeName,
			deferInfo:            fieldCtx.deferInfo,
			parentFieldDeferID:   fieldCtx.parentFieldDeferID,
		}

		requirements.existsTracker[existsKey] = len(requirements.requirementConfigs)
		requirements.requirementConfigs = append(requirements.requirementConfigs, config)
	}

	c.pendingKeyRequirements[currentSelectionSet] = requirements
}

func (c *nodeSelectionVisitor) processPendingFieldRequirements(selectionSetRef int) {
	configs, hasSelectionSet := c.pendingFieldRequirements[selectionSetRef]
	if !hasSelectionSet {
		return
	}
	delete(c.pendingFieldRequirements, selectionSetRef)

	for _, requiredFieldsCfg := range configs.requirementConfigs {
		c.addFieldRequirementsToOperation(selectionSetRef, requiredFieldsCfg)
	}
}

func (c *nodeSelectionVisitor) addFieldRequirementsToOperation(selectionSetRef int, requirements fieldRequirements) {
	typeName := c.walker.EnclosingTypeDefinition.NameString(c.definition)

	input := &addRequiredFieldsConfiguration{
		operation:                     c.operation,
		definition:                    c.definition,
		operationSelectionSetRef:      selectionSetRef,
		isTypeNameForEntityInterface:  requirements.isTypenameForEntityInterface,
		isKey:                         false,
		allowTypename:                 false,
		typeName:                      typeName,
		fieldSet:                      requirements.selectionSet,
		deferInfo:                     requirements.deferInfo,
		parentFieldDeferID:            requirements.parentFieldDeferID,
		addTypenameInNestedSelections: c.addTypenameInNestedSelections,
	}

	addFieldsResult, report := addRequiredFields(input)
	if report.HasErrors() {
		c.walker.StopWithInternalErr(fmt.Errorf("failed to add required fields %s for %s at path %s: %w", requirements.selectionSet, typeName, requirements.path, report))
		return
	}
	c.resetVisitedAbstractChecksForModifiedFields(addFieldsResult.modifiedFieldRefs)

	c.addNewSkipFieldRefs(addFieldsResult.skipFieldRefs...)
	// add mapping for the field dependencies
	for _, requestedByFieldRef := range requirements.requestedByFieldRefs {
		fieldKey := fieldIndexKey{requestedByFieldRef, requirements.dsHash}
		c.fieldDependsOn[fieldKey] = append(c.fieldDependsOn[fieldKey], addFieldsResult.requiredFieldRefs...)
		c.fieldRefDependsOn[requestedByFieldRef] = append(c.fieldRefDependsOn[requestedByFieldRef], addFieldsResult.requiredFieldRefs...)

		// TODO: actually we could probably build representations right away?
		// e.g. build it here, pass to path visitor and down to datasource, without the need to parse it there?
		fieldConfiguration := FederationFieldConfiguration{
			TypeName:               typeName,
			FieldName:              c.operation.FieldNameString(requestedByFieldRef),
			SelectionSet:           requirements.selectionSet,
			RemappedPaths:          addFieldsResult.remappedPaths,
			RequiredFieldArguments: addFieldsResult.requiredFieldArguments,
		}
		c.fieldRequirementsConfigs[fieldKey] = append(c.fieldRequirementsConfigs[fieldKey], fieldConfiguration)
		for _, requiredFieldRef := range addFieldsResult.requiredFieldRefs {
			c.fieldDependencyKind[fieldDependencyKey{field: requestedByFieldRef, dependsOn: requiredFieldRef}] = fieldDependencyKindRequires
		}
	}

	c.hasNewFields = true
}

func (c *nodeSelectionVisitor) processPendingKeyRequirements(selectionSetRef int) {
	configs, hasSelectionSet := c.pendingKeyRequirements[selectionSetRef]
	if !hasSelectionSet {
		return
	}
	delete(c.pendingKeyRequirements, selectionSetRef)

	for _, requirement := range configs.requirementConfigs {
		c.addKeyRequirementsToOperation(selectionSetRef, requirement)
	}

	// TODO: raise an error in configuration visitor? if fetch is nested, but don't have requirements attached
	// c.walker.StopWithInternalErr(fmt.Errorf("could not plan key requirements on a path '%s'", c.walker.Path.DotDelimitedString()))

}

func (c *nodeSelectionVisitor) addKeyRequirementsToOperation(selectionSetRef int, pendingKey keyRequirements) {
	requirementsFromInterfaceObject := pendingKey.isInterfaceObject
	requirementsToInterfaceObject := false

	if requirementsFromInterfaceObject {
		i := slices.IndexFunc(c.dataSources, func(d DataSource) bool {
			return d.Hash() == pendingKey.sc.Source
		})

		if i != -1 {
			targetDsConfiguration := c.dataSources[i]
			requirementsToInterfaceObject = targetDsConfiguration.HasInterfaceObject(pendingKey.typeName)
		}
	}

	// when we jump from interface object to interface object, we don't need a concrete type __typename to do the jump,
	// we also dissalow adding typename because for the interface object type we intentionally do not add root node for a typename
	// if we will add a typename it will be queried from the concrete types, which we don't want here
	// so we have to skip adding __typename field
	dissalowTypeName := requirementsFromInterfaceObject && requirementsToInterfaceObject

	// jumps represents a chain of keys needed to reach from source to the target, in simple case it is just one jump
	// each key from the next jump will have dependencies on a key fields from the previous jump

	var currentFieldRefs []int
	var previousJump *KeyJump
	for i, jump := range pendingKey.sc.Jumps {
		allowTypeName := !dissalowTypeName && i == 0
		lastJump := i == len(pendingKey.sc.Jumps)-1

		input := &addRequiredFieldsConfiguration{
			operation:                c.operation,
			definition:               c.definition,
			operationSelectionSetRef: selectionSetRef,
			isKey:                    true,
			allowTypename:            allowTypeName,
			typeName:                 jump.TypeName,
			fieldSet:                 jump.SelectionSet,
			deferInfo:                pendingKey.deferInfo,
			parentFieldDeferID:       pendingKey.parentFieldDeferID,
		}

		addFieldsResult, report := addRequiredFields(input)
		if report.HasErrors() {
			c.walker.StopWithInternalErr(fmt.Errorf("failed to add required key fields %s for %s: %w", jump.SelectionSet, jump.TypeName, report))
			return
		}
		c.resetVisitedAbstractChecksForModifiedFields(addFieldsResult.modifiedFieldRefs)

		// op, _ := astprinter.PrintStringIndentDebug(c.operation, " ")
		// fmt.Println("operation: ", op)

		c.addNewSkipFieldRefs(addFieldsResult.skipFieldRefs...)

		// setup deps between key chain items
		if currentFieldRefs != nil && previousJump != nil {
			for _, requestedByFieldRef := range addFieldsResult.requiredFieldRefs {
				fieldKey := fieldIndexKey{requestedByFieldRef, jump.From}

				for _, requiredFieldRef := range currentFieldRefs {
					if requiredFieldRef == requestedByFieldRef {
						// we should not add field ref to fieldDependsOn map if it is part of a key,
						// e.g., if it depends on itself
						continue
					}
					c.fieldDependsOn[fieldKey] = append(c.fieldDependsOn[fieldKey], requiredFieldRef)
					c.fieldRefDependsOn[requestedByFieldRef] = append(c.fieldRefDependsOn[requestedByFieldRef], requiredFieldRef)
				}

				c.fieldRequirementsConfigs[fieldKey] = append(c.fieldRequirementsConfigs[fieldKey], FederationFieldConfiguration{
					TypeName:      previousJump.TypeName,
					SelectionSet:  previousJump.SelectionSet,
					RemappedPaths: addFieldsResult.remappedPaths,
				})
				for _, requiredFieldRef := range currentFieldRefs {
					c.fieldDependencyKind[fieldDependencyKey{field: requestedByFieldRef, dependsOn: requiredFieldRef}] = fieldDependencyKindKey
				}
			}
		}
		currentFieldRefs = addFieldsResult.requiredFieldRefs

		// setup deps for the field requested this chain
		if lastJump {
			// add mapping for the field dependencies
			for _, requestedByFieldRef := range pendingKey.requestedByFieldRefs {
				fieldKey := fieldIndexKey{requestedByFieldRef, pendingKey.targetDSHash}

				for _, requiredFieldRef := range currentFieldRefs {
					if requiredFieldRef == requestedByFieldRef {
						// we should not add field ref to fieldDependsOn map if it is part of a key,
						// e.g., if it depends on itself
						continue
					}
					c.fieldDependsOn[fieldKey] = append(c.fieldDependsOn[fieldKey], requiredFieldRef)
					c.fieldRefDependsOn[requestedByFieldRef] = append(c.fieldRefDependsOn[requestedByFieldRef], requiredFieldRef)
				}

				c.fieldRequirementsConfigs[fieldKey] = append(c.fieldRequirementsConfigs[fieldKey], FederationFieldConfiguration{
					TypeName:      jump.TypeName,
					SelectionSet:  jump.SelectionSet,
					RemappedPaths: addFieldsResult.remappedPaths,
				})
				for _, requiredFieldRef := range currentFieldRefs {
					c.fieldDependencyKind[fieldDependencyKey{field: requestedByFieldRef, dependsOn: requiredFieldRef}] = fieldDependencyKindKey
				}
			}
		}

		for _, requiredFieldRef := range currentFieldRefs {
			c.fieldLandedTo[requiredFieldRef] = jump.From
		}

		previousJump = &jump
	}

	c.hasNewFields = true
}

func (c *nodeSelectionVisitor) rewriteSelectionSetHavingAbstractFragments(fieldRef int, ds DataSource) {
	if _, ok := c.visitedFieldsAbstractChecks[fieldRef]; ok {
		return
	}
	c.visitedFieldsAbstractChecks[fieldRef] = struct{}{}

	_, ok := ds.UpstreamSchema()
	if !ok {
		return
	}

	var options []rewriterOption
	if _, wasRewritten := c.persistedRewrittenFieldRefs[fieldRef]; wasRewritten {
		// When field was already rewritten in previous walker runs,
		// but we are visiting it again - it means that we have appended more required fields to it.
		// So we have to force rewriting it again, because without force we could end up with duplicated fields outside of fragments.
		// When newly added fields are local - rewriter will consider that rewrite is not necessary.
		options = append(options, withForceRewrite())
	}

	rewriter, err := newFieldSelectionRewriter(c.operation, c.definition, ds, options...)
	if err != nil {
		c.walker.StopWithInternalErr(fmt.Errorf("failed to create field selection rewriter for field %s at path %s: %w", c.operation.FieldNameString(fieldRef), c.walker.Path.DotDelimitedString(), err))
		return
	}

	result, err := rewriter.RewriteFieldSelection(fieldRef, c.walker.EnclosingTypeDefinition)
	if err != nil {
		c.walker.StopWithInternalErr(fmt.Errorf("failed to rewrite field selection for field %s at path %s: %w", c.operation.FieldNameString(fieldRef), c.walker.Path.DotDelimitedString(), err))
		return
	}

	if !result.rewritten {
		return
	}

	c.addNewSkipFieldRefs(rewriter.skipFieldRefs...)
	c.hasNewFields = true
	c.rewrittenFieldRefs = append(c.rewrittenFieldRefs, fieldRef)
	c.persistedRewrittenFieldRefs[fieldRef] = struct{}{}

	c.updateFieldDependsOn(result.changedFieldRefs)
	c.updateSkipFieldRefs(result.fieldRefOrigins)

	// skip walking into a rewritten field instead of stoping the whole visitor
	// should allow to do fewer walks over the operation
	c.walker.SkipNode()
}

// resetVisitedAbstractChecksForModifiedFields - when we modify the operation by adding required fields
// we need to reset visitedFieldsAbstractChecks for modified fields to allow additional rewrites if necessary
func (c *nodeSelectionVisitor) resetVisitedAbstractChecksForModifiedFields(modifiedFields []int) {
	for _, fieldRef := range modifiedFields {
		delete(c.visitedFieldsAbstractChecks, fieldRef)
	}
}

func (c *nodeSelectionVisitor) updateFieldDependsOn(changedFieldRefs map[int][]int) {
	for key, fieldRefs := range c.fieldDependsOn {
		updatedFieldRefs := make([]int, 0, len(fieldRefs))
		for _, fieldRef := range fieldRefs {
			if newRefs := changedFieldRefs[fieldRef]; newRefs != nil {
				updatedFieldRefs = append(updatedFieldRefs, newRefs...)
			} else {
				updatedFieldRefs = append(updatedFieldRefs, fieldRef)
			}
		}

		c.fieldDependsOn[key] = updatedFieldRefs
	}

	for key, fieldRefs := range c.fieldRefDependsOn {
		updatedFieldRefs := make([]int, 0, len(fieldRefs))
		for _, fieldRef := range fieldRefs {
			if newRefs := changedFieldRefs[fieldRef]; newRefs != nil {
				updatedFieldRefs = append(updatedFieldRefs, newRefs...)
			} else {
				updatedFieldRefs = append(updatedFieldRefs, fieldRef)
			}
		}

		c.fieldRefDependsOn[key] = updatedFieldRefs
	}

	for _, newRefs := range changedFieldRefs {
		c.addNewFieldRefs(newRefs...)
	}
}

// updateSkipFieldRefs updates the skipFieldsRefs list after a rewrite of an abstract field selection set.
// A field ref present after the rewrite should be skipped only when all original field refs
// occupying the same response position were skipped.
// When at least one origin is a user-requested field, the field must stay in the response -
// including the case when a previously skipped ref survived a merge with a user-requested duplicate.
func (c *nodeSelectionVisitor) updateSkipFieldRefs(fieldRefOrigins map[int][]int) {
	if len(fieldRefOrigins) == 0 {
		return
	}

	skipped := make(map[int]struct{}, len(c.skipFieldsRefs))
	for _, fieldRef := range c.skipFieldsRefs {
		skipped[fieldRef] = struct{}{}
	}

	var unskipFieldRefs map[int]struct{}

	for fieldRef, originRefs := range fieldRefOrigins {
		allOriginsSkipped := true
		for _, originRef := range originRefs {
			if _, ok := skipped[originRef]; !ok {
				allOriginsSkipped = false
				break
			}
		}

		if allOriginsSkipped {
			if _, ok := skipped[fieldRef]; !ok {
				c.skipFieldsRefs = append(c.skipFieldsRefs, fieldRef)
			}
			continue
		}

		if _, ok := skipped[fieldRef]; ok {
			if unskipFieldRefs == nil {
				unskipFieldRefs = make(map[int]struct{})
			}
			unskipFieldRefs[fieldRef] = struct{}{}
		}
	}

	if len(unskipFieldRefs) > 0 {
		c.skipFieldsRefs = slices.DeleteFunc(c.skipFieldsRefs, func(fieldRef int) bool {
			_, ok := unskipFieldRefs[fieldRef]
			return ok
		})
	}
}
