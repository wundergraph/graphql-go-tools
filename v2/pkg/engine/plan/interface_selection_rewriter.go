package plan

import "github.com/wundergraph/graphql-go-tools/v2/pkg/ast"

type interfaceSelectionRewriter struct {
	operation  *ast.Document
	definition *ast.Document

	fieldRef        int
	enclosingNode   ast.Node
	dsConfiguration DataSourceConfiguration
}

func newInterfaceSelectionRewriter(operation *ast.Document, definition *ast.Document) *interfaceSelectionRewriter {
	return &interfaceSelectionRewriter{
		operation:  operation,
		definition: definition,
	}
}

func (r *interfaceSelectionRewriter) isFieldReturnsInterface(fieldRef int, enclosingNode ast.Node) (interfaceDefRef int, isInterface bool) {
	var (
		fieldDefRef int
		hasField    bool
	)

	interfaceDefRef = ast.InvalidRef

	switch enclosingNode.Kind {
	case ast.NodeKindObjectTypeDefinition:
		fieldDefRef, hasField = r.definition.ObjectTypeDefinitionFieldWithName(enclosingNode.Ref, r.operation.FieldNameBytes(fieldRef))
		if !hasField {
			return
		}
	case ast.NodeKindInterfaceTypeDefinition:
		fieldDefRef, hasField = r.definition.InterfaceTypeDefinitionFieldWithName(enclosingNode.Ref, r.operation.FieldNameBytes(fieldRef))
		if !hasField {
			return
		}
	default:
		return
	}

	fieldDefTypeName := r.definition.FieldDefinitionTypeNameBytes(fieldDefRef)
	node, hasNode := r.definition.NodeByName(fieldDefTypeName)
	if !hasNode {
		return
	}

	if node.Kind != ast.NodeKindInterfaceTypeDefinition {
		return
	}

	return node.Ref, true
}

func (r *interfaceSelectionRewriter) objectTypesImplementingInterface(interfaceDefRef int) (typeNames []string, ok bool) {
	implementedByNodes := r.definition.InterfaceTypeDefinitionImplementedByRootNodes(interfaceDefRef)

	typeNames = make([]string, 0, len(implementedByNodes))
	for _, implementedByNode := range implementedByNodes {
		if implementedByNode.Kind != ast.NodeKindObjectTypeDefinition {
			continue
		}

		typeNames = append(typeNames, implementedByNode.NameString(r.definition))
	}

	if len(typeNames) > 0 {
		return typeNames, true

	}

	return nil, false
}

func (r *interfaceSelectionRewriter) datasourceHasEntitiesWithName(dsConfiguration *DataSourceConfiguration, typeNames []string) (entityNames []string, ok bool) {
	hasEntities := false
	for _, typeName := range typeNames {
		if len(dsConfiguration.RequiredFieldsByKey(typeName)) > 0 {
			hasEntities = true
			break
		}
	}

	if !hasEntities {
		return nil, false
	}

	entityNames = make([]string, 0, len(typeNames))
	for _, typeName := range typeNames {
		if dsConfiguration.HasRootNodeWithTypename(typeName) {
			entityNames = append(entityNames, typeName)
		}
	}

	return entityNames, true
}

type interfaceFieldSelectionInfo struct {
	sharedFieldNames []string
	inlineFragments  []inlineFragmentSelection
}

type inlineFragmentSelection struct {
	selectionRef int
	typeName     string
	fieldNames   []string
}

func (r *interfaceSelectionRewriter) collectFieldInformation(fieldRef int) interfaceFieldSelectionInfo {
	fieldSelectionSetRef, ok := r.operation.FieldSelectionSet(fieldRef)
	if !ok {
		panic("unexpected error: field does not have a selection set")
	}

	fieldNames := r.operation.SelectionSetFieldNames(fieldSelectionSetRef)

	inlineFragmentSelectionRefs := r.operation.SelectionSetInlineFragmentSelections(fieldSelectionSetRef)
	inlineFragmentSelections := make([]inlineFragmentSelection, 0, len(inlineFragmentSelectionRefs))
	for _, inlineFragmentSelectionRef := range inlineFragmentSelectionRefs {
		typeCondition := r.operation.InlineFragmentTypeConditionNameString(inlineFragmentSelectionRef)
		inlineFragmentSelectionSetRef, ok := r.operation.InlineFragmentSelectionSet(inlineFragmentSelectionRef)
		if !ok {
			panic("unexpected error: inline fragment does not have a selection set")
		}

		// For now, we care only about field selections on inline fragment
		// potentially there could be another nested inline fragments - but we do not yet have such use case
		fieldNames := r.operation.SelectionSetFieldNames(inlineFragmentSelectionSetRef)

		inlineFragmentSelections = append(inlineFragmentSelections, inlineFragmentSelection{
			selectionRef: inlineFragmentSelectionRef,
			typeName:     typeCondition,
			fieldNames:   fieldNames,
		})
	}

	return interfaceFieldSelectionInfo{
		sharedFieldNames: fieldNames,
		inlineFragments:  inlineFragmentSelections,
	}
}

func (r *interfaceSelectionRewriter) allEntitiesHaveSharedFieldsAsRootNode(configuration *DataSourceConfiguration, entityNames []string, sharedFieldNames []string) bool {
	for _, typeName := range entityNames {
		for _, fieldName := range sharedFieldNames {
			if !configuration.HasRootNode(typeName, fieldName) {
				return false
			}
		}
	}

	return true
}

func (r *interfaceSelectionRewriter) inlineFragmentHasAllSharedFields(dsConfiguration *DataSourceConfiguration, inlineFragmentSelection inlineFragmentSelection, sharedFieldNames []string) bool {
	notSelectedFields := make([]string, 0, len(sharedFieldNames))
	selectedSharedFieldsCount := 0
	for _, fieldName := range sharedFieldNames {
		fieldIsSelected := false
		for _, fragmentFieldName := range inlineFragmentSelection.fieldNames {
			if fieldName == fragmentFieldName {
				selectedSharedFieldsCount++
				fieldIsSelected = true
				break
			}
		}
		if !fieldIsSelected {
			notSelectedFields = append(notSelectedFields, fieldName)
		}
	}

	if selectedSharedFieldsCount == len(sharedFieldNames) {
		return true
	}

	for _, fieldName := range notSelectedFields {
		if !dsConfiguration.HasRootNode(inlineFragmentSelection.typeName, fieldName) {
			return false
		}
	}

	return true
}

func (r *interfaceSelectionRewriter) interfaceFieldSelectionNeedsRewrite(fieldRef int, dsConfiguration *DataSourceConfiguration, entityNames []string) (interfaceFieldSelectionInfo, bool) {
	fieldInfo := r.collectFieldInformation(fieldRef)

	// case 1. we do not have fragments
	if len(fieldInfo.inlineFragments) == 0 {
		// check that all types implementing the interface have a root node with the requested fields
		return fieldInfo, !r.allEntitiesHaveSharedFieldsAsRootNode(dsConfiguration, entityNames, fieldInfo.sharedFieldNames)
	}

	// case 2. we do not have shared fields, but only fragments
	if len(fieldInfo.sharedFieldNames) == 0 {
		// if we do not have shared fields but do have fragments - we do not need to rewrite
		return fieldInfo, false
	}

	// case 3. we have both shared fields and inline fragments
	// 3.1 check first case for types for which we do not have inline fragments

	entityNamesNotIncludedInFragments := make([]string, 0, len(entityNames))
	for _, typeName := range entityNames {
		hasType := false
		for _, fragmentSelection := range fieldInfo.inlineFragments {
			if fragmentSelection.typeName == typeName {
				hasType = true
				break
			}
		}
		if !hasType {
			entityNamesNotIncludedInFragments = append(entityNamesNotIncludedInFragments, typeName)
		}
	}
	if !r.allEntitiesHaveSharedFieldsAsRootNode(dsConfiguration, entityNamesNotIncludedInFragments, fieldInfo.sharedFieldNames) {
		return fieldInfo, true
	}

	// 3.2 check that fragment types have all requested fields or all not selected fields are local for the datasource
	for _, inlineFragmentSelection := range fieldInfo.inlineFragments {
		if !r.inlineFragmentHasAllSharedFields(dsConfiguration, inlineFragmentSelection, fieldInfo.sharedFieldNames) {
			return fieldInfo, true
		}
	}

	return fieldInfo, false
}

func (r *interfaceSelectionRewriter) RewriteOperation(fieldRef int, enclosingNode ast.Node, dsConfiguration *DataSourceConfiguration) bool {
	interfaceDefRef, isInterface := r.isFieldReturnsInterface(fieldRef, enclosingNode)
	if !isInterface {
		return false
	}

	typeNames, ok := r.objectTypesImplementingInterface(interfaceDefRef)
	if !ok {
		return false
	}

	entityNames, ok := r.datasourceHasEntitiesWithName(dsConfiguration, typeNames)
	if !ok {
		return false
	}

	info, needRewrite := r.interfaceFieldSelectionNeedsRewrite(fieldRef, dsConfiguration, entityNames)
	if !needRewrite {
		return false
	}

	r.rewriteOperation(fieldRef, entityNames, info)

	return true
}

func (r *interfaceSelectionRewriter) rewriteOperation(fieldRef int, entityNames []string, fieldInfo interfaceFieldSelectionInfo) {
	/*
		1) extract selections which is not inline-fragments - e.g. shared selections
		2) extract selections for each inline fragment
		3) for types which do not have inline-fragment - add inline fragment with shared fields
		4) for types which have inline-fragment - add not selected shared fields to existing inline fragment
	*/

	if len(fieldInfo.inlineFragments) == 0 {
		r.createSelectionSetFromSharedFields(fieldRef, entityNames, fieldInfo.sharedFieldNames)
		// create new selection set with shared fields
		// add inline fragment for each type
		// copy selection set into inline fragment
	}

	// when we have both shared fields and inline fragments
	// for types which do not have inline-fragment - add inline fragment with shared fields
	// for types which have inline-fragment - add not selected shared fields to existing inline fragment
}
