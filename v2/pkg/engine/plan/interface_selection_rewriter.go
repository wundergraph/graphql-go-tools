package plan

import (
	"errors"
	"sort"

	"golang.org/x/exp/slices"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
)

var (
	FieldDoesntHaveSelectionSetErr          = errors.New("unexpected error: field does not have a selection set")
	InlineFragmentDoesntHaveSelectionSetErr = errors.New("unexpected error: inline fragment does not have a selection set")
	InlineFragmentTypeIsNotExistsErr        = errors.New("unexpected error: inline fragment type condition does not exists")
)

type fieldSelectionRewriter struct {
	operation          *ast.Document
	definition         *ast.Document
	upstreamDefinition *ast.Document

	skipTypeNameFieldRef int
}

func newFieldSelectionRewriter(operation *ast.Document, definition *ast.Document, upstreamDefinition *ast.Document) *fieldSelectionRewriter {
	return &fieldSelectionRewriter{
		operation:            operation,
		definition:           definition,
		upstreamDefinition:   upstreamDefinition,
		skipTypeNameFieldRef: ast.InvalidRef,
	}
}

func (r *fieldSelectionRewriter) fieldTypeNode(fieldRef int, enclosingNode ast.Node) (node ast.Node, ok bool) {
	var (
		fieldDefRef int
		hasField    bool
	)

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

	return node, true
}

func (r *fieldSelectionRewriter) datasourceHasEntitiesWithName(dsConfiguration *DataSourceConfiguration, typeNames []string) (entityNames []string, ok bool) {
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

	sort.Strings(entityNames)

	return entityNames, true
}

type fieldSelectionInfo struct {
	hasTypeNameSelection        bool // __typename is selected
	sharedFields                []fieldSelection
	inlineFragmentsOnObjects    []inlineFragmentSelection
	inlineFragmentsOnInterfaces []inlineFragmentSelection
}

type fieldSelection struct {
	fieldSelectionRef int
	fieldName         string
}

type inlineFragmentSelection struct {
	selectionRef               int
	typeName                   string
	nodeKind                   ast.NodeKind
	nodeRef                    int
	hasTypeNameSelection       bool // __typename is selected
	fields                     []fieldSelection
	typesImplementingInterface []string
}

func (s inlineFragmentSelection) hasTypeImplementingInterface(typeName string) bool {
	if len(s.typesImplementingInterface) == 0 {
		return false
	}

	return slices.Contains(s.typesImplementingInterface, typeName)
}

func (r *fieldSelectionRewriter) selectionSetFieldSelections(selectionSetRef int, includeTypename bool) (fieldSelections []fieldSelection, hasTypename bool) {
	fieldSelectionRefs := r.operation.SelectionSetFieldSelections(selectionSetRef)
	fieldSelections = make([]fieldSelection, 0, len(fieldSelectionRefs))
	for _, fieldSelectionRef := range fieldSelectionRefs {
		fieldRef := r.operation.Selections[fieldSelectionRef].Ref
		fieldName := r.operation.FieldNameString(fieldRef)
		if fieldName == "__typename" {
			hasTypename = true
			if !includeTypename {
				continue
			}
		}

		fieldSelections = append(fieldSelections, fieldSelection{
			fieldSelectionRef: fieldSelectionRef,
			fieldName:         fieldName,
		})
	}

	return fieldSelections, hasTypename
}

func (r *fieldSelectionRewriter) collectFieldInformation(fieldRef int) (fieldSelectionInfo, error) {
	fieldSelectionSetRef, ok := r.operation.FieldSelectionSet(fieldRef)
	if !ok {
		return fieldSelectionInfo{}, FieldDoesntHaveSelectionSetErr
	}

	sharedFields, hasSharedTypename := r.selectionSetFieldSelections(fieldSelectionSetRef, false)

	inlineFragmentSelectionRefs := r.operation.SelectionSetInlineFragmentSelections(fieldSelectionSetRef)
	inlineFragmentSelectionsOnObjects := make([]inlineFragmentSelection, 0, len(inlineFragmentSelectionRefs))
	inlineFragmentsOnInterfaces := make([]inlineFragmentSelection, 0, len(inlineFragmentSelectionRefs))

	for _, inlineFragmentSelectionRef := range inlineFragmentSelectionRefs {
		inlineFragmentRef := r.operation.Selections[inlineFragmentSelectionRef].Ref
		typeCondition := r.operation.InlineFragmentTypeConditionNameString(inlineFragmentRef)
		inlineFragmentSelectionSetRef, ok := r.operation.InlineFragmentSelectionSet(inlineFragmentRef)
		if !ok {
			return fieldSelectionInfo{}, InlineFragmentDoesntHaveSelectionSetErr
		}

		node, hasNode := r.definition.NodeByNameStr(typeCondition)
		if !hasNode {
			return fieldSelectionInfo{}, InlineFragmentTypeIsNotExistsErr
		}

		// For now, we care only about field selections on inline fragment
		// potentially there could be another nested inline fragments - but we do not yet have such use case
		fields, hasTypeName := r.selectionSetFieldSelections(inlineFragmentSelectionSetRef, true)

		fragmentSelectionInfo := inlineFragmentSelection{
			selectionRef:         inlineFragmentSelectionRef,
			typeName:             typeCondition,
			hasTypeNameSelection: hasTypeName,
			fields:               fields,

			nodeKind: node.Kind,
			nodeRef:  node.Ref,
		}

		switch node.Kind {
		case ast.NodeKindObjectTypeDefinition:
			inlineFragmentSelectionsOnObjects = append(inlineFragmentSelectionsOnObjects, fragmentSelectionInfo)
		case ast.NodeKindInterfaceTypeDefinition:
			typesImplementingInterface := r.typesImplementingInterface(node.Ref)
			fragmentSelectionInfo.typesImplementingInterface = typesImplementingInterface

			inlineFragmentsOnInterfaces = append(inlineFragmentsOnInterfaces, fragmentSelectionInfo)
		}
	}

	return fieldSelectionInfo{
		sharedFields:                sharedFields,
		hasTypeNameSelection:        hasSharedTypename,
		inlineFragmentsOnObjects:    inlineFragmentSelectionsOnObjects,
		inlineFragmentsOnInterfaces: inlineFragmentsOnInterfaces,
	}, nil
}

func (r *fieldSelectionRewriter) entityNamesWithoutFragments(inlineFragments []inlineFragmentSelection, entityNames []string) []string {
	entityNamesNotIncludedInFragments := make([]string, 0, len(entityNames))
	for _, typeName := range entityNames {
		idx := slices.IndexFunc(inlineFragments, func(fragmentSelection inlineFragmentSelection) bool {
			return fragmentSelection.typeName == typeName
		})

		if idx == -1 {
			entityNamesNotIncludedInFragments = append(entityNamesNotIncludedInFragments, typeName)
		}
	}

	if len(entityNamesNotIncludedInFragments) > 0 {
		return entityNamesNotIncludedInFragments
	}

	return nil
}

func (r *fieldSelectionRewriter) entityNamesWithFragments(inlineFragments []inlineFragmentSelection, entityNames []string) []string {
	entityNamesWithFragments := make([]string, 0, len(entityNames))
	for _, typeName := range entityNames {
		idx := slices.IndexFunc(inlineFragments, func(fragmentSelection inlineFragmentSelection) bool {
			return fragmentSelection.typeName == typeName
		})

		if idx != -1 {
			entityNamesWithFragments = append(entityNamesWithFragments, typeName)
		}
	}

	if len(entityNamesWithFragments) > 0 {
		return entityNamesWithFragments
	}

	return nil
}

func (r *fieldSelectionRewriter) allFragmentTypesExistsOnDatasource(inlineFragments []inlineFragmentSelection, configuration *DataSourceConfiguration) bool {
	for _, inlineFragment := range inlineFragments {
		if !r.hasTypeOnDataSource(configuration, inlineFragment.typeName) {
			return false
		}
	}

	return true
}

func (r *fieldSelectionRewriter) allFragmentTypesImplementsInterfaceTypes(inlineFragments []inlineFragmentSelection, interfaceTypes []string) bool {
	for _, inlineFragment := range inlineFragments {
		if !slices.Contains(interfaceTypes, inlineFragment.typeName) {
			return false
		}
	}

	return true
}

func (r *fieldSelectionRewriter) filterFragmentsByTypeNames(inlineFragments []inlineFragmentSelection, typeNames []string) (fragments []inlineFragmentSelection, missingTypes []string) {
	fragments = make([]inlineFragmentSelection, 0, len(typeNames))
	for _, typeName := range typeNames {
		idx := slices.IndexFunc(inlineFragments, func(fragmentSelection inlineFragmentSelection) bool {
			return fragmentSelection.typeName == typeName
		})

		if idx != -1 {
			fragments = append(fragments, inlineFragments[idx])
		} else {
			missingTypes = append(missingTypes, typeName)
		}
	}

	if len(fragments) > 0 {
		return fragments, missingTypes
	}

	return nil, missingTypes
}

func (r *fieldSelectionRewriter) entityHasFieldsAsRootNode(configuration *DataSourceConfiguration, entityName string, fields []fieldSelection) bool {
	for _, fieldSelection := range fields {
		if !configuration.HasRootNode(entityName, fieldSelection.fieldName) {
			return false
		}
	}

	return true
}

func (r *fieldSelectionRewriter) allEntitiesHaveFieldsAsRootNode(configuration *DataSourceConfiguration, entityNames []string, fields []fieldSelection) bool {
	for _, entityName := range entityNames {
		if !r.entityHasFieldsAsRootNode(configuration, entityName, fields) {
			return false
		}
	}

	return true
}

func (r *fieldSelectionRewriter) notSelectedFieldsForInlineFragment(inlineFragmentSelection inlineFragmentSelection, fields []fieldSelection) []fieldSelection {
	notSelectedFields := make([]fieldSelection, 0, len(fields))
	for _, fieldSelection := range fields {
		fieldIsSelected := false
		for _, fragmentField := range inlineFragmentSelection.fields {
			if fieldSelection.fieldName == fragmentField.fieldName {
				fieldIsSelected = true
				break
			}
		}
		if !fieldIsSelected {
			notSelectedFields = append(notSelectedFields, fieldSelection)
		}
	}

	return notSelectedFields
}

func (r *fieldSelectionRewriter) inlineFragmentHasAllFieldsLocalToDatasource(dsConfiguration *DataSourceConfiguration, inlineFragmentSelection inlineFragmentSelection, fields []fieldSelection) bool {
	notSelectedFields := r.notSelectedFieldsForInlineFragment(inlineFragmentSelection, fields)

	if len(notSelectedFields) == 0 {
		return true
	}

	for _, notSelectedField := range notSelectedFields {
		hasField := dsConfiguration.HasRootNode(inlineFragmentSelection.typeName, notSelectedField.fieldName) ||
			dsConfiguration.HasChildNode(inlineFragmentSelection.typeName, notSelectedField.fieldName)

		if !hasField {
			return false
		}
	}

	return true
}

func (r *fieldSelectionRewriter) interfaceFieldSelectionNeedsRewrite(selectionSetInfo fieldSelectionInfo, dsConfiguration *DataSourceConfiguration, entityNames []string, interfaceTypeNames []string) (entitiesWithoutFragment []string, needRewrite bool) {
	entitiesWithoutFragment = r.entityNamesWithoutFragments(selectionSetInfo.inlineFragmentsOnObjects, entityNames)

	// TODO: we are not checking inline fragments on interfaces - this is the case when we have on interface fragment within interface field selection

	// case 1. we do not have fragments
	if len(selectionSetInfo.inlineFragmentsOnObjects) == 0 {
		// check that all types implementing the interface have a root node with the requested fields
		return entitiesWithoutFragment, !r.allEntitiesHaveFieldsAsRootNode(dsConfiguration, entityNames, selectionSetInfo.sharedFields)
	}

	// check that all inline fragments types are implementing the interface in the current datasource
	if !r.allFragmentTypesImplementsInterfaceTypes(selectionSetInfo.inlineFragmentsOnObjects, interfaceTypeNames) {
		return entitiesWithoutFragment, true
	}

	// check that all inline fragments types are present in the current datasource
	if !r.allFragmentTypesExistsOnDatasource(selectionSetInfo.inlineFragmentsOnObjects, dsConfiguration) {
		return entitiesWithoutFragment, true
	}

	// case 2. we do not have shared fields, but only fragments
	if len(selectionSetInfo.sharedFields) == 0 {
		// if we do not have shared fields but do have fragments - we do not need to rewrite
		return entitiesWithoutFragment, false
	}

	// case 3. we have both shared fields and inline fragments
	// 3.1 check first case for types for which we do not have inline fragments

	if !r.allEntitiesHaveFieldsAsRootNode(dsConfiguration, entitiesWithoutFragment, selectionSetInfo.sharedFields) {
		return entitiesWithoutFragment, true
	}

	// 3.2 check that fragment types have all requested fields or all not selected fields are local for the datasource
	for _, inlineFragmentSelection := range selectionSetInfo.inlineFragmentsOnObjects {
		if !r.inlineFragmentHasAllFieldsLocalToDatasource(dsConfiguration, inlineFragmentSelection, selectionSetInfo.sharedFields) {
			return entitiesWithoutFragment, true
		}
	}

	return entitiesWithoutFragment, false
}

func (r *fieldSelectionRewriter) typesImplementingInterface(interfaceDefinitionRef int) (out []string) {
	typeNames, ok := r.definition.InterfaceTypeDefinitionImplementedByObjectWithNames(interfaceDefinitionRef)
	if !ok {
		return nil
	}

	return typeNames
}

func (r *fieldSelectionRewriter) entitiesImplementingInterface(typesImplementingInterface []string, entityNames []string) (out []string) {
	if len(typesImplementingInterface) == 0 {
		return nil
	}

	for _, typeName := range typesImplementingInterface {
		if slices.Contains(entityNames, typeName) {
			out = append(out, typeName)
		}
	}

	return entityNames
}

func (r *fieldSelectionRewriter) allEntitiesImplementsInterfaces(inlineFragmentsOnInterfaces []inlineFragmentSelection, dsConfiguration *DataSourceConfiguration, entityNames []string) bool {
	for _, inlineFragmentsOnInterface := range inlineFragmentsOnInterfaces {
		entitiesImplementingInterface := r.entitiesImplementingInterface(inlineFragmentsOnInterface.typesImplementingInterface, entityNames)
		if len(entitiesImplementingInterface) == 0 {
			continue
		}

		if !r.allEntitiesHaveFieldsAsRootNode(dsConfiguration, entitiesImplementingInterface, inlineFragmentsOnInterface.fields) {
			return false
		}
	}

	return true
}

func (r *fieldSelectionRewriter) allEntityFragmentsSatisfyInterfaces(inlineFragmentsOnInterfaces, inlineFragmentsOnObjects []inlineFragmentSelection, dsConfiguration *DataSourceConfiguration, entityNames []string) bool {
	for _, inlineFragmentsOnInterface := range inlineFragmentsOnInterfaces {
		entitiesImplementingInterface := r.entitiesImplementingInterface(inlineFragmentsOnInterface.typesImplementingInterface, entityNames)
		if len(entitiesImplementingInterface) == 0 {
			continue
		}

		entityFragments, _ := r.filterFragmentsByTypeNames(inlineFragmentsOnObjects, entitiesImplementingInterface)

		if len(entityFragments) > 0 {
			for _, entityFragment := range entityFragments {
				satisfies := r.inlineFragmentHasAllFieldsLocalToDatasource(dsConfiguration, entityFragment, inlineFragmentsOnInterface.fields) ||
					r.entityHasFieldsAsRootNode(dsConfiguration, entityFragment.typeName, inlineFragmentsOnInterface.fields)
				if !satisfies {
					return false
				}
			}
		}
	}

	return true
}

func (r *fieldSelectionRewriter) hasTypeOnDataSource(dsConfiguration *DataSourceConfiguration, typeName string) bool {
	return dsConfiguration.HasRootNodeWithTypename(typeName) ||
		dsConfiguration.HasChildNodeWithTypename(typeName)
}

func (r *fieldSelectionRewriter) unionFieldSelectionNeedsRewrite(selectionSetInfo fieldSelectionInfo, dsConfiguration *DataSourceConfiguration, entityNames []string) (needRewrite bool) {
	// when we have types not exists in the current datasource - we need to rewrite
	if !r.allFragmentTypesExistsOnDatasource(selectionSetInfo.inlineFragmentsOnObjects, dsConfiguration) {
		return true
	}

	// when we do not have fragments on interfaces, but only on objects - we do not need to rewrite
	if len(selectionSetInfo.inlineFragmentsOnInterfaces) == 0 {
		return false
	}

	// when we do not have fragments on objects, but only on interfaces
	// we need to check that all entities implementing each interface have a root node with the requested fields
	// e.g. { ... on Interface { a } }

	if len(selectionSetInfo.inlineFragmentsOnObjects) == 0 {
		return !r.allEntitiesImplementsInterfaces(selectionSetInfo.inlineFragmentsOnInterfaces, dsConfiguration, entityNames)
	}

	// when we have fragments on both interfaces and objects
	// we need to check that all entities without fragments implementing each interface have a root node with the requested fields

	entitiesWithoutFragment := r.entityNamesWithoutFragments(selectionSetInfo.inlineFragmentsOnObjects, entityNames)
	if len(entitiesWithoutFragment) > 0 {
		if !r.allEntitiesImplementsInterfaces(selectionSetInfo.inlineFragmentsOnInterfaces, dsConfiguration, entitiesWithoutFragment) {
			return true
		}
	}

	// for each existing fragment we need to check:
	// - is it entity
	// - is it implements each interface
	// - does it have all requested fields from this interface
	entityNamesWithFragments := r.entityNamesWithFragments(selectionSetInfo.inlineFragmentsOnObjects, entityNames)
	if len(entityNamesWithFragments) > 0 {
		if !r.allEntityFragmentsSatisfyInterfaces(selectionSetInfo.inlineFragmentsOnInterfaces, selectionSetInfo.inlineFragmentsOnObjects, dsConfiguration, entityNamesWithFragments) {
			return true
		}
	}

	return false
}

func (r *fieldSelectionRewriter) RewriteFieldSelection(fieldRef int, enclosingNode ast.Node, dsConfiguration *DataSourceConfiguration) (rewritten bool, err error) {
	fieldTypeNode, ok := r.fieldTypeNode(fieldRef, enclosingNode)
	if !ok {
		return false, nil
	}

	switch fieldTypeNode.Kind {
	case ast.NodeKindInterfaceTypeDefinition:
		return r.processInterfaceSelection(fieldRef, fieldTypeNode.Ref, dsConfiguration)
	case ast.NodeKindUnionTypeDefinition:
		return r.processUnionSelection(fieldRef, fieldTypeNode.Ref, dsConfiguration)
	default:
		return false, nil
	}
}

func (r *fieldSelectionRewriter) processInterfaceSelection(fieldRef int, interfaceDefRef int, dsConfiguration *DataSourceConfiguration) (rewritten bool, err error) {
	/*
		1) extract selections which is not inline-fragments - e.g. shared selections
		2) extract selections for each inline fragment
		3) for types which do not have inline-fragment - add inline fragment with shared fields
		4) for types which have inline-fragment - add not selected shared fields to existing inline fragment
	*/

	interfaceTypeName := r.definition.InterfaceTypeDefinitionNameBytes(interfaceDefRef)
	node, hasNode := r.upstreamDefinition.NodeByName(interfaceTypeName)
	if !hasNode {
		return false, errors.New("unexpected error: interface type definition not found in the upstream schema")
	}
	if node.Kind != ast.NodeKindInterfaceTypeDefinition {
		return false, errors.New("unexpected error: node kind is not interface type definition in the upstream schema")
	}

	typeNames, ok := r.upstreamDefinition.InterfaceTypeDefinitionImplementedByObjectWithNames(node.Ref)
	if !ok {
		return false, nil
	}

	entityNames, _ := r.datasourceHasEntitiesWithName(dsConfiguration, typeNames)

	selectionSetInfo, err := r.collectFieldInformation(fieldRef)
	if err != nil {
		return false, err
	}

	entitiesWithoutFragment, needRewrite := r.interfaceFieldSelectionNeedsRewrite(selectionSetInfo, dsConfiguration, entityNames, typeNames)
	if !needRewrite {
		return false, nil
	}

	err = r.rewriteInterfaceSelection(fieldRef, selectionSetInfo, dsConfiguration, entitiesWithoutFragment, typeNames)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (r *fieldSelectionRewriter) processUnionSelection(fieldRef int, unionDefRef int, dsConfiguration *DataSourceConfiguration) (rewritten bool, err error) {
	/*
		1) extract inline fragments selections with interface types
		2) extract inline fragments selections with members of the union
		3) intersect inline fragments selections with interface types and members of the union
		4) create new inline fragments with types from the intersection which do not have inline fragments
		5) append existing inline fragments with fields from the interfaces
	*/

	unionTypeName := r.definition.UnionTypeDefinitionNameBytes(unionDefRef)
	node, hasNode := r.upstreamDefinition.NodeByName(unionTypeName)
	if !hasNode {
		return false, errors.New("unexpected error: union type definition not found in the upstream schema")
	}
	if node.Kind != ast.NodeKindUnionTypeDefinition {
		return false, errors.New("unexpected error: node kind is not union type definition in the upstream schema")
	}

	unionTypeNames, ok := r.upstreamDefinition.UnionTypeDefinitionMemberTypeNames(node.Ref)
	if !ok {
		return false, nil
	}

	entityNames, _ := r.datasourceHasEntitiesWithName(dsConfiguration, unionTypeNames)

	selectionSetInfo, err := r.collectFieldInformation(fieldRef)
	if err != nil {
		return false, err
	}

	needRewrite := r.unionFieldSelectionNeedsRewrite(selectionSetInfo, dsConfiguration, entityNames)
	if !needRewrite {
		return false, nil
	}

	err = r.rewriteUnionSelection(fieldRef, selectionSetInfo, dsConfiguration, unionTypeNames, entityNames)
	if err != nil {
		return false, err
	}

	return needRewrite, nil
}

func (r *fieldSelectionRewriter) rewriteUnionSelection(fieldRef int, fieldInfo fieldSelectionInfo, dsConfiguration *DataSourceConfiguration, unionTypeNames, entityNames []string) error {
	newSelectionRefs := make([]int, 0, len(unionTypeNames)+1) // 1 for __typename
	if fieldInfo.hasTypeNameSelection {
		// we should preserve __typename if it was in the original query as it is explicitly requested
		typeNameSelectionRef, _ := r.typeNameSelection()
		newSelectionRefs = append(newSelectionRefs, typeNameSelectionRef)
	}

	unionTypeNamesToProcess := make([]string, 0, len(unionTypeNames))
	for _, typeName := range unionTypeNames {
		hasTypeOnDatasource := dsConfiguration.HasRootNodeWithTypename(typeName) ||
			dsConfiguration.HasChildNodeWithTypename(typeName)

		if !hasTypeOnDatasource {
			// remove/skip fragments with type not exists in the current datasource
			continue
		}

		unionTypeNamesToProcess = append(unionTypeNamesToProcess, typeName)
	}

	existingObjectFragments, missingFragmentTypeNames := r.filterFragmentsByTypeNames(fieldInfo.inlineFragmentsOnObjects, unionTypeNamesToProcess)

	addedFragments := 0

	// handle existing fragments
	for _, existingObjectFragment := range existingObjectFragments {
		// check if it implements interface
		// if yes - add fields from the interface
		// if no - just copy fragment

		fieldsToAdd := make([]fieldSelection, 0, len(existingObjectFragment.fields))

		for _, fragmentSelectionOnInterface := range fieldInfo.inlineFragmentsOnInterfaces {
			if !fragmentSelectionOnInterface.hasTypeImplementingInterface(existingObjectFragment.typeName) {
				continue
			}

			fieldsToAdd = append(fieldsToAdd, fragmentSelectionOnInterface.fields...)
		}

		fragmentSelectionRef, err := r.copyFragmentSelectionWithAddingFields(existingObjectFragment, fieldsToAdd)
		if err != nil {
			return err
		}

		newSelectionRefs = append(newSelectionRefs, fragmentSelectionRef)

		addedFragments++
	}

	// handle missing fragments
	for _, missingFragmentTypeName := range missingFragmentTypeNames {
		// check if it implements each interface
		// and add field from each interface fragment selection

		fieldsToAdd := make([]fieldSelection, 0, 2)

		for _, fragmentSelectionOnInterface := range fieldInfo.inlineFragmentsOnInterfaces {
			if !fragmentSelectionOnInterface.hasTypeImplementingInterface(missingFragmentTypeName) {
				continue
			}

			fieldsToAdd = append(fieldsToAdd, fragmentSelectionOnInterface.fields...)
		}

		if len(fieldsToAdd) == 0 {
			continue
		}

		fragmentSelectionRef := r.createFragmentSelection(missingFragmentTypeName, fieldsToAdd)
		newSelectionRefs = append(newSelectionRefs, fragmentSelectionRef)
		addedFragments++
	}

	fieldSelectionSetRef, _ := r.operation.FieldSelectionSet(fieldRef)
	r.operation.EmptySelectionSet(fieldSelectionSetRef)

	for _, newSelectionRef := range newSelectionRefs {
		r.operation.AddSelectionRefToSelectionSet(fieldSelectionSetRef, newSelectionRef)
	}

	if addedFragments == 0 && !fieldInfo.hasTypeNameSelection {
		// we have to add __typename selection - but we should skip it in response
		typeNameSelectionRef, typeNameFieldRef := r.typeNameSelection()
		r.operation.AddSelectionRefToSelectionSet(fieldSelectionSetRef, typeNameSelectionRef)
		r.skipTypeNameFieldRef = typeNameFieldRef
	}

	return nil
}

func (r *fieldSelectionRewriter) rewriteInterfaceSelection(fieldRef int, fieldInfo fieldSelectionInfo, dsConfiguration *DataSourceConfiguration, entitiesWithoutFragment []string, interfaceTypeNames []string) error {
	newSelectionRefs := make([]int, 0, len(entitiesWithoutFragment)+len(fieldInfo.inlineFragmentsOnObjects)+1) // 1 for __typename

	if fieldInfo.hasTypeNameSelection {
		// we should preserve __typename if it was in the original query as it is explicitly requested
		typeNameSelectionRef, _ := r.typeNameSelection()
		newSelectionRefs = append(newSelectionRefs, typeNameSelectionRef)
	}

	addedFragments := 0

	if len(fieldInfo.sharedFields) > 0 {
		for _, entityName := range entitiesWithoutFragment {
			newSelectionRefs = append(newSelectionRefs, r.createFragmentSelection(entityName, fieldInfo.sharedFields))
			addedFragments++
		}
	}

	for _, inlineFragmentInfo := range fieldInfo.inlineFragmentsOnObjects {
		if !r.hasTypeOnDataSource(dsConfiguration, inlineFragmentInfo.typeName) {
			// remove fragments with type not exists in the current datasource
			continue
		}

		if !slices.Contains(interfaceTypeNames, inlineFragmentInfo.typeName) {
			// remove fragment which not implements interface in the current datasource
			continue
		}

		fragmentSelectionRef, err := r.copyFragmentSelectionWithAddingFields(inlineFragmentInfo, fieldInfo.sharedFields)
		if err != nil {
			return err
		}

		newSelectionRefs = append(newSelectionRefs, fragmentSelectionRef)
		addedFragments++
	}

	fieldSelectionSetRef, _ := r.operation.FieldSelectionSet(fieldRef)
	r.operation.EmptySelectionSet(fieldSelectionSetRef)

	for _, newSelectionRef := range newSelectionRefs {
		r.operation.AddSelectionRefToSelectionSet(fieldSelectionSetRef, newSelectionRef)
	}

	if addedFragments == 0 && !fieldInfo.hasTypeNameSelection {
		// we have to add __typename selection - but we should skip it in response
		typeNameSelectionRef, typeNameFieldRef := r.typeNameSelection()
		r.operation.AddSelectionRefToSelectionSet(fieldSelectionSetRef, typeNameSelectionRef)
		r.skipTypeNameFieldRef = typeNameFieldRef
	}

	return nil
}

func (r *fieldSelectionRewriter) typeNameSelection() (selectionRef int, fieldRef int) {
	field := r.operation.AddField(ast.Field{
		Name: r.operation.Input.AppendInputString("__typename"),
	})
	return r.operation.AddSelectionToDocument(ast.Selection{
		Ref:  field.Ref,
		Kind: ast.SelectionKindField,
	}), field.Ref
}

func (r *fieldSelectionRewriter) createFragmentSelection(typeName string, fields []fieldSelection) (selectionRef int) {
	selectionRefs := make([]int, 0, len(fields))
	for _, sharedField := range fields {
		newFieldSelectionRef := r.operation.CopySelection(sharedField.fieldSelectionRef)
		selectionRefs = append(selectionRefs, newFieldSelectionRef)
	}

	selectionSetRef := r.operation.AddSelectionSetToDocument(ast.SelectionSet{
		SelectionRefs: selectionRefs,
	})

	inlineFragment := ast.InlineFragment{
		TypeCondition: ast.TypeCondition{
			Type: r.operation.AddNamedType([]byte(typeName)),
		},
		SelectionSet:  selectionSetRef,
		HasSelections: true,
	}

	inlineFragmentRef := r.operation.AddInlineFragment(inlineFragment)

	return r.operation.AddSelectionToDocument(ast.Selection{
		Kind: ast.SelectionKindInlineFragment,
		Ref:  inlineFragmentRef,
	})
}

func (r *fieldSelectionRewriter) copyFragmentSelectionWithAddingFields(inlineFragmentInfo inlineFragmentSelection, fields []fieldSelection) (selectionRef int, err error) {
	notSelectedFields := r.notSelectedFieldsForInlineFragment(inlineFragmentInfo, fields)

	inlineFragmentSelectionCopyRef := r.operation.CopySelection(inlineFragmentInfo.selectionRef)
	inlineFragmentRef := r.operation.Selections[inlineFragmentSelectionCopyRef].Ref

	inlineFragmentSelectionSetRef, ok := r.operation.InlineFragmentSelectionSet(inlineFragmentRef)
	if !ok {
		return ast.InvalidRef, InlineFragmentDoesntHaveSelectionSetErr
	}

	for _, notSelectedField := range notSelectedFields {
		r.operation.AddSelectionRefToSelectionSet(inlineFragmentSelectionSetRef, r.operation.CopySelection(notSelectedField.fieldSelectionRef))
	}

	return inlineFragmentSelectionCopyRef, nil
}
