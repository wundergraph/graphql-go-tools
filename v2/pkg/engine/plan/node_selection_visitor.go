package plan

import (
	"errors"
	"fmt"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

// nodeSelectionVisitor - walks through the operation multiple times to rewrite operation
// to be able to resolve fields from different datasources.
// During walks, it is adding required fields and rewrites abstract selection if it is necessary.
// We are revisiting query when we have:
// - new required fields were added to operation
// - when we have rewritten abstract field selection set
type nodeSelectionVisitor struct {
	debug DebugConfiguration

	operationName         string        // graphql query name
	operation, definition *ast.Document // graphql operation and schema documents
	walker                *astvisitor.Walker

	dataSources     []DataSource     // data sources configurations, which used by the current operation
	nodeSuggestions *NodeSuggestions // nodeSuggestions holds information about suggested data sources for each field

	selectionSetRefs []int // selectionSetRefs is a stack of selection set refs - used to add a required fields
	skipFieldsRefs   []int // skipFieldsRefs holds required field refs added by planner and should not be added to user response

	pendingKeyRequirements   map[int]pendingKeyRequirements   // pendingKeyRequirements is a map[selectionSetRef][]keyRequirements
	pendingFieldRequirements map[int]pendingFieldRequirements // pendingFieldRequirements is a map[selectionSetRef]fieldRequirements

	visitedFieldsRequiresChecks map[fieldIndexKey]struct{}                       // visitedFieldsRequiresChecks is a map[fieldIndexKey] of already processed fields which we check for presence of @requires directive
	visitedFieldsKeyChecks      map[fieldIndexKey]struct{}                       // visitedFieldsKeyChecks is a map[fieldIndexKey] of already processed fields which we check for @key requirements
	visitedFieldsAbstractChecks map[int]struct{}                                 // visitedFieldsAbstractChecks is a map[FieldRef] of already processed fields which we check for abstract type, e.g. union or interface
	fieldDependsOn              map[fieldIndexKey][]int                          // fieldDependsOn is a map[fieldIndexKey][]fieldRef - holds list of field refs which are required by a field ref, e.g. field should be planned only after required fields were planned
	fieldRefDependsOn           map[int][]int                                    // fieldRefDependsOn is a map[fieldRef][]fieldRef - holds list of field refs which are required by a field ref, it is a second index without datasource hash
	fieldRequirementsConfigs    map[fieldIndexKey][]FederationFieldConfiguration // fieldRequirementsConfigs is a map[fieldIndexKey]FederationFieldConfiguration - holds a list of required configuratuibs for a field ref to later built representation variables
	fieldLandedTo               map[int]DSHash                                   // fieldLandedTo is a map[fieldRef]DSHash - holds a datasource hash where field was landed to

	secondaryRun        bool // secondaryRun is a flag to indicate that we're running the nodeSelectionVisitor not the first time
	hasNewFields        bool // hasNewFields is used to determine if we need to run the planner again. It will be true in case required fields were added
	hasUnresolvedFields bool // hasUnresolvedFields is used to determine if we need to run the planner again. We should set it to true in case we have unresolved fields

	fieldPathCoordinates []KeyConditionCoordinate // currentFieldPathCoordinates is a stack of field path coordinates // TODO: remove me
	rewrittenFieldRefs   []int
}

func (c *nodeSelectionVisitor) shouldRevisit() bool {
	return c.hasNewFields || c.hasUnresolvedFields
}

type fieldIndexKey struct {
	fieldRef int
	dsHash   DSHash
}

// selectionSetPendingRequirements - is a wrapper to been able to have predictable order of keyRequirements but at the same time deduplicate keyRequirements
type pendingKeyRequirements struct {
	existsTracker      map[DSHash]struct{} // existsTracker allows us to not add duplicated keyRequirements
	requirementConfigs []keyRequirements   // requirementConfigs is a list of keyRequirements which should be added to the selection set
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
}

type fieldRequirements struct {
	dsHash                       DSHash
	path                         string
	selectionSet                 string
	requestedByFieldRefs         []int
	isTypenameForEntityInterface bool
}

type pendingFieldRequirements struct {
	existsTracker      map[pendingFieldRequirementExistsKey]struct{} // existsTracker allows us to not add duplicated fieldRequirements
	requirementConfigs []fieldRequirements                           // requirementConfigs is a list of fieldRequirements which should be added to the selection set
}

type pendingFieldRequirementExistsKey struct {
	dsHash                       DSHash
	selectionSet                 string
	isTypenameForEntityInterface bool
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
	c.hasUnresolvedFields = false
	c.rewrittenFieldRefs = c.rewrittenFieldRefs[:0]

	if c.selectionSetRefs == nil {
		c.selectionSetRefs = make([]int, 0, 8)
	} else {
		c.selectionSetRefs = c.selectionSetRefs[:0]
	}

	if c.fieldPathCoordinates == nil {
		c.fieldPathCoordinates = make([]KeyConditionCoordinate, 0, 8)
	} else {
		c.fieldPathCoordinates = c.fieldPathCoordinates[:0]
	}

	if c.secondaryRun {
		return
	}

	c.operation, c.definition = operation, definition

	if c.skipFieldsRefs == nil {
		c.skipFieldsRefs = make([]int, 0, 8)
	}

	c.visitedFieldsAbstractChecks = make(map[int]struct{})
	c.visitedFieldsRequiresChecks = make(map[fieldIndexKey]struct{})
	c.visitedFieldsKeyChecks = make(map[fieldIndexKey]struct{})
	c.pendingKeyRequirements = make(map[int]pendingKeyRequirements)
	c.pendingFieldRequirements = make(map[int]pendingFieldRequirements)

	c.fieldDependsOn = make(map[fieldIndexKey][]int)
	c.fieldRefDependsOn = make(map[int][]int)
	c.fieldRequirementsConfigs = make(map[fieldIndexKey][]FederationFieldConfiguration)
	c.fieldLandedTo = make(map[int]DSHash)
}

func (c *nodeSelectionVisitor) LeaveDocument(operation, definition *ast.Document) {

}

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
}

func (c *nodeSelectionVisitor) LeaveSelectionSet(ref int) {
	c.debugPrint("LeaveSelectionSet ref:", ref)
	c.processPendingFieldRequirements(ref)
	c.processPendingKeyRequirements(ref)
	c.selectionSetRefs = c.selectionSetRefs[:len(c.selectionSetRefs)-1]
}

func (c *nodeSelectionVisitor) EnterField(fieldRef int) {
	root := c.walker.Ancestors[0]
	if root.Kind != ast.NodeKindOperationDefinition {
		return
	}

	fieldName := c.operation.FieldNameUnsafeString(fieldRef)
	fieldAliasOrName := c.operation.FieldAliasOrNameString(fieldRef)
	typeName := c.walker.EnclosingTypeDefinition.NameString(c.definition)

	c.debugPrint("EnterField ref:", fieldRef, "fieldName:", fieldName, "typeName:", typeName)

	parentPath := c.walker.Path.DotDelimitedString()
	currentPath := parentPath + "." + fieldAliasOrName

	suggestions := c.nodeSuggestions.SuggestionsForPath(typeName, fieldName, currentPath)

	for _, suggestion := range suggestions {
		if suggestion.IsRequiredKeyField {
			// it was already selected as a key field
			// no need to process required fields for it
			continue
		}

		dsIdx := slices.IndexFunc(c.dataSources, func(d DataSource) bool {
			return d.Hash() == suggestion.DataSourceHash
		})
		if dsIdx == -1 {
			c.walker.StopWithInternalErr(errors.New("we should always have a datasource for a suggestion"))
			return
		}
		ds := c.dataSources[dsIdx]

		// check if the field has @requires directive
		c.handleFieldRequiredByRequires(fieldRef, parentPath, typeName, fieldName, currentPath, ds)

		if suggestion.requiresKey != nil {
			// add @key requirements for the field
			c.handleFieldsRequiredByKey(fieldRef, parentPath, typeName, fieldName, currentPath, ds, *suggestion.requiresKey)
		}

		// check if a field type is abstract and need rewrites
		c.rewriteSelectionSetOfFieldWithInterfaceType(fieldRef, ds)
	}

	c.fieldPathCoordinates = append(c.fieldPathCoordinates, KeyConditionCoordinate{
		FieldName: fieldName,
		TypeName:  typeName,
	})
}

func (c *nodeSelectionVisitor) LeaveField(ref int) {
	if len(c.fieldPathCoordinates) > 0 {
		c.fieldPathCoordinates = c.fieldPathCoordinates[:len(c.fieldPathCoordinates)-1]
	}
}

func (c *nodeSelectionVisitor) handleFieldRequiredByRequires(fieldRef int, parentPath, typeName, fieldName, currentPath string, dsConfig DataSource) {
	fieldKey := fieldIndexKey{fieldRef, dsConfig.Hash()}
	_, visited := c.visitedFieldsRequiresChecks[fieldKey]
	if visited {
		return
	}
	c.visitedFieldsRequiresChecks[fieldKey] = struct{}{}

	if fieldName == typeNameField {
		// the __typename field could not have @requires directive
		return
	}

	requiresConfiguration, exists := dsConfig.RequiredFieldsByRequires(typeName, fieldName)

	if !exists {
		for _, io := range dsConfig.FederationConfiguration().InterfaceObjects {
			if slices.Contains(io.ConcreteTypeNames, typeName) {
				// we should check if we have a @requires configuration for the interface object
				requiresConfiguration, exists = dsConfig.RequiredFieldsByRequires(io.InterfaceTypeName, fieldName)
				if exists {
					requiresConfiguration.TypeName = typeName
					break
				}
			}
		}
	}

	if !exists {
		// we do not have a @requires configuration for the field
		return
	}

	// we should plan adding required fields for the field
	// they will be added in the on LeaveSelectionSet callback for the current selection set
	// and current field ref will be added to fieldDependsOn map
	c.addPendingFieldRequirements(fieldRef, dsConfig.Hash(), requiresConfiguration, currentPath, false)
	c.handleKeyRequirementsForBackJumpOnSameDataSource(fieldRef, dsConfig, typeName, parentPath)
}

func (c *nodeSelectionVisitor) handleFieldsRequiredByKey(fieldRef int, parentPath, typeName, fieldName, currentPath string, dsConfig DataSource, sc SourceConnection) {
	fieldKey := fieldIndexKey{fieldRef, dsConfig.Hash()}
	_, visited := c.visitedFieldsKeyChecks[fieldKey]
	if visited {
		return
	}
	c.visitedFieldsKeyChecks[fieldKey] = struct{}{}

	selectedParentsDSHashes := c.getSelectedParentsDSHashes(fieldRef)

	isParentHasInterfaceObject := slices.ContainsFunc(selectedParentsDSHashes, func(dsHash DSHash) bool {
		dsIdx := slices.IndexFunc(c.dataSources, func(d DataSource) bool {
			return d.Hash() == dsHash
		})
		if dsIdx == -1 {
			return false
		}

		return c.dataSources[dsIdx].HasInterfaceObject(typeName)
	})

	entityInterface := dsConfig.HasEntityInterface(typeName)
	interfaceObject := dsConfig.HasInterfaceObject(typeName)

	if fieldName == typeNameField && !entityInterface {
		// the __typename field could not have @key directive
		// but for the interface object we have to plan it differently
		// e.g. we should get a __typename from a concrete type, not the interface object
		// it means for the entity interface we should evaluate key deps on a __typename field
		return
	}

	c.addPendingKeyRequirements(fieldRef, dsConfig.Hash(), sc, interfaceObject, parentPath, typeName)

	if isParentHasInterfaceObject && !interfaceObject && !entityInterface {
		c.addPendingFieldRequirements(
			fieldRef,
			dsConfig.Hash(),
			FederationFieldConfiguration{
				TypeName:     typeName,
				FieldName:    fieldName,
				SelectionSet: "__typename",
			},
			currentPath,
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

func (c *nodeSelectionVisitor) handleKeyRequirementsForBackJumpOnSameDataSource(fieldRef int, dsConfig DataSource, typeName string, parentPath string) {
	selectedParentsDSHashes := c.getSelectedParentsDSHashes(fieldRef)

	// regularly keys are required only when the datasource hash differs from the parent datasource hash
	// one exception when the field has requires directive and planned on the same datasource as a parent
	// in this case we have to add a back jump on the same datasource to get required fields for the field resolver
	// but jump is possible only with keys, so we have to add any key for this datasource
	sameAsParentDS := len(selectedParentsDSHashes) == 1 && selectedParentsDSHashes[0] == dsConfig.Hash()
	if !sameAsParentDS {
		return
	}

	keyConfigurations := dsConfig.RequiredFieldsByKey(typeName)

	if len(keyConfigurations) == 0 {
		// required fields could be of zero length in case type is not entity
		// or when entity has disabled entity resolver.
		// Usually we can't jump to the entity with disabled entity resolver, but there is one known exception
		// When entity has disabled entity resolver, but we have field with requires directive on this entity
		// we should add key fields for the field with requires - to pass them into field resolver

		keys := dsConfig.FederationConfiguration().Keys
		keyConfigurations = keys.FilterByTypeAndResolvability(typeName, false)
	}

	if len(keyConfigurations) == 0 {
		return
	}

	keyToUse := keyConfigurations[0]

	sc := SourceConnection{
		Type: SourceConnectionTypeDirect,
		Jumps: []KeyJump{
			{
				From:         dsConfig.Hash(),
				To:           dsConfig.Hash(),
				SelectionSet: keyToUse.SelectionSet,
				TypeName:     typeName,
			},
		},
	}

	c.addPendingKeyRequirements(fieldRef, dsConfig.Hash(), sc, false, parentPath, typeName)
}

func (c *nodeSelectionVisitor) addPendingFieldRequirements(requestedByFieldRef int, dsHash DSHash, fieldConfiguration FederationFieldConfiguration, currentPath string, isTypenameForEntityInterface bool) {
	currentSelectionSet := c.currentSelectionSet()

	requirements, hasRequirements := c.pendingFieldRequirements[currentSelectionSet]
	if !hasRequirements {
		requirements = pendingFieldRequirements{
			existsTracker: make(map[pendingFieldRequirementExistsKey]struct{}),
		}
	}

	existsKey := pendingFieldRequirementExistsKey{dsHash, fieldConfiguration.SelectionSet, isTypenameForEntityInterface}
	if _, exists := requirements.existsTracker[existsKey]; !exists {
		config := fieldRequirements{
			dsHash:                       dsHash,
			path:                         currentPath,
			selectionSet:                 fieldConfiguration.SelectionSet,
			requestedByFieldRefs:         []int{requestedByFieldRef},
			isTypenameForEntityInterface: isTypenameForEntityInterface,
		}

		requirements.existsTracker[existsKey] = struct{}{}
		requirements.requirementConfigs = append(requirements.requirementConfigs, config)
	} else {
		for i := range requirements.requirementConfigs {
			if requirements.requirementConfigs[i].selectionSet == fieldConfiguration.SelectionSet && requirements.requirementConfigs[i].dsHash == dsHash && requirements.requirementConfigs[i].isTypenameForEntityInterface == isTypenameForEntityInterface {
				if slices.IndexFunc(requirements.requirementConfigs[i].requestedByFieldRefs, func(fieldRef int) bool {
					return fieldRef == requestedByFieldRef
				}) == -1 {
					requirements.requirementConfigs[i].requestedByFieldRefs = append(requirements.requirementConfigs[i].requestedByFieldRefs, requestedByFieldRef)
				}
				break
			}
		}
	}

	c.pendingFieldRequirements[currentSelectionSet] = requirements
	fieldKey := fieldIndexKey{requestedByFieldRef, dsHash}
	c.fieldRequirementsConfigs[fieldKey] = append(c.fieldRequirementsConfigs[fieldKey], fieldConfiguration)
}

func (c *nodeSelectionVisitor) addPendingKeyRequirements(requestedByFieldRef int, dsHash DSHash, sc SourceConnection, isInterfaceObject bool, parentPath string, typeName string) {
	currentSelectionSet := c.currentSelectionSet()

	requirements, hasRequirements := c.pendingKeyRequirements[currentSelectionSet]

	if !hasRequirements {
		requirements = pendingKeyRequirements{
			existsTracker: make(map[DSHash]struct{}),
		}
	}

	existsKey := dsHash
	if _, exists := requirements.existsTracker[existsKey]; !exists {
		config := keyRequirements{
			targetDSHash:         dsHash,
			path:                 parentPath,
			isInterfaceObject:    isInterfaceObject,
			sc:                   sc,
			requestedByFieldRefs: []int{requestedByFieldRef},
			typeName:             typeName,
		}

		requirements.existsTracker[existsKey] = struct{}{}
		requirements.requirementConfigs = append(requirements.requirementConfigs, config)
	} else {
		for i := range requirements.requirementConfigs {
			if requirements.requirementConfigs[i].targetDSHash == dsHash {
				if !slices.Contains(requirements.requirementConfigs[i].requestedByFieldRefs, requestedByFieldRef) {
					requirements.requirementConfigs[i].requestedByFieldRefs = append(requirements.requirementConfigs[i].requestedByFieldRefs, requestedByFieldRef)
				}
				break
			}
		}
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
	key, report := RequiredFieldsFragment(typeName, requirements.selectionSet, false)
	if report.HasErrors() {
		c.walker.StopWithInternalErr(fmt.Errorf("failed to parse required fields %s for %s at path %s", requirements.selectionSet, typeName, requirements.path))
		return
	}

	input := &addRequiredFieldsInput{
		key:                          key,
		operation:                    c.operation,
		definition:                   c.definition,
		report:                       report,
		operationSelectionSet:        selectionSetRef,
		isTypeNameForEntityInterface: requirements.isTypenameForEntityInterface,
	}

	skipFieldRefs, requiredFieldRefs := addRequiredFields(input)
	if report.HasErrors() {
		c.walker.StopWithInternalErr(fmt.Errorf("failed to add required fields %s for %s at path %s", requirements.selectionSet, typeName, requirements.path))
		return
	}

	c.skipFieldsRefs = append(c.skipFieldsRefs, skipFieldRefs...)
	// add mapping for the field dependencies
	for _, requestedByFieldRef := range requirements.requestedByFieldRefs {
		fieldKey := fieldIndexKey{requestedByFieldRef, requirements.dsHash}
		c.fieldDependsOn[fieldKey] = append(c.fieldDependsOn[fieldKey], requiredFieldRefs...)
		c.fieldRefDependsOn[requestedByFieldRef] = append(c.fieldRefDependsOn[requestedByFieldRef], requiredFieldRefs...)
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

		key, report := RequiredFieldsFragment(jump.TypeName, jump.SelectionSet, allowTypeName)
		if report.HasErrors() {
			c.walker.StopWithInternalErr(fmt.Errorf("failed to parse required fields %s for %s", jump.SelectionSet, jump.TypeName))
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
			c.walker.StopWithInternalErr(fmt.Errorf("failed to add required fields %s for %s", jump.SelectionSet, jump.TypeName))
			return
		}

		// op, _ := astprinter.PrintStringIndentDebug(c.operation, " ")
		// fmt.Println("operation: ", op)

		c.skipFieldsRefs = append(c.skipFieldsRefs, skipFieldRefs...)

		// setup deps between key chain items
		if currentFieldRefs != nil && previousJump != nil {
			for _, requestedByFieldRef := range requiredFieldRefs {
				if slices.Contains(currentFieldRefs, requestedByFieldRef) {
					// we should not add field ref to fieldDependsOn map if it is part of a key
					continue
				}

				fieldKey := fieldIndexKey{requestedByFieldRef, jump.From}
				c.fieldDependsOn[fieldKey] = append(c.fieldDependsOn[fieldKey], currentFieldRefs...)
				c.fieldRefDependsOn[requestedByFieldRef] = append(c.fieldRefDependsOn[requestedByFieldRef], currentFieldRefs...)
				c.fieldRequirementsConfigs[fieldKey] = append(c.fieldRequirementsConfigs[fieldKey], FederationFieldConfiguration{
					TypeName:     previousJump.TypeName,
					SelectionSet: previousJump.SelectionSet,
				})
			}
		}
		currentFieldRefs = requiredFieldRefs

		// setup deps for the field requested this chain
		if lastJump {
			// add mapping for the field dependencies
			for _, requestedByFieldRef := range pendingKey.requestedByFieldRefs {
				if slices.Contains(currentFieldRefs, requestedByFieldRef) {
					// we should not add field ref to fieldDependsOn map if it is part of a key
					continue
				}

				fieldKey := fieldIndexKey{requestedByFieldRef, pendingKey.targetDSHash}
				c.fieldDependsOn[fieldKey] = append(c.fieldDependsOn[fieldKey], requiredFieldRefs...)
				c.fieldRefDependsOn[requestedByFieldRef] = append(c.fieldRefDependsOn[requestedByFieldRef], requiredFieldRefs...)
				c.fieldRequirementsConfigs[fieldKey] = append(c.fieldRequirementsConfigs[fieldKey], FederationFieldConfiguration{
					TypeName:     jump.TypeName,
					SelectionSet: jump.SelectionSet,
				})
			}
		}

		for _, requiredFieldRef := range currentFieldRefs {
			c.fieldLandedTo[requiredFieldRef] = jump.From
		}

		previousJump = &jump
	}

	c.hasNewFields = true
}

func (c *nodeSelectionVisitor) rewriteSelectionSetOfFieldWithInterfaceType(fieldRef int, ds DataSource) {
	if _, ok := c.visitedFieldsAbstractChecks[fieldRef]; ok {
		return
	}
	c.visitedFieldsAbstractChecks[fieldRef] = struct{}{}

	upstreamSchema, ok := ds.UpstreamSchema()
	if !ok {
		return
	}

	rewriter := newFieldSelectionRewriter(c.operation, c.definition)
	rewriter.SetUpstreamDefinition(upstreamSchema)
	rewriter.SetDatasourceConfiguration(ds)

	rewritten, err := rewriter.RewriteFieldSelection(fieldRef, c.walker.EnclosingTypeDefinition)
	if err != nil {
		c.walker.StopWithInternalErr(err)
		return
	}

	if !rewritten {
		return
	}

	c.skipFieldsRefs = append(c.skipFieldsRefs, rewriter.skipFieldRefs...)

	c.hasNewFields = true
	c.rewrittenFieldRefs = append(c.rewrittenFieldRefs, fieldRef)

	// skip walking into a rewritten field instead of stoping the whole visitor
	// should allow to do fewer walks over the operation
	c.walker.SkipNode()
}
