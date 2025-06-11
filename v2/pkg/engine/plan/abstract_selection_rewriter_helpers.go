package plan

import (
	"bytes"
	"errors"
	"slices"
	"sort"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func (r *fieldSelectionRewriter) datasourceHasEntitiesWithName(typeNames []string) (entityNames []string, ok bool) {
	hasEntities := false
	for _, typeName := range typeNames {
		if r.dsConfiguration.HasEntity(typeName) {
			hasEntities = true
			break
		}
	}

	if !hasEntities {
		return nil, false
	}

	entityNames = make([]string, 0, len(typeNames))
	for _, typeName := range typeNames {
		if r.dsConfiguration.HasRootNodeWithTypename(typeName) {
			entityNames = append(entityNames, typeName)
		}
	}

	sort.Strings(entityNames)

	return entityNames, true
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

	return out
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

// TODO: looks like it is extensive unnecessary check
func (r *fieldSelectionRewriter) allEntitiesImplementsInterfaces(inlineFragmentsOnInterfaces []inlineFragmentSelectionOnInterface, entityNames []string) bool {
	for _, inlineFragmentsOnInterface := range inlineFragmentsOnInterfaces {
		entitiesImplementingInterface := r.entitiesImplementingInterface(inlineFragmentsOnInterface.typeNamesImplementingInterfaceInCurrentDS, entityNames)
		if len(entitiesImplementingInterface) == 0 {
			continue
		}

		if !r.allEntitiesHaveFieldsAsRootNode(entitiesImplementingInterface, inlineFragmentsOnInterface.selectionSetInfo.fields) {
			return false
		}
	}

	return true
}

// TODO: looks like it is extensive unnecessary check
func (r *fieldSelectionRewriter) allEntityFragmentsSatisfyInterfaces(inlineFragmentsOnInterfaces []inlineFragmentSelectionOnInterface, inlineFragmentsOnObjects []inlineFragmentSelection, entityNames []string) bool {
	for _, inlineFragmentsOnInterface := range inlineFragmentsOnInterfaces {
		entitiesImplementingInterface := r.entitiesImplementingInterface(inlineFragmentsOnInterface.typeNamesImplementingInterfaceInCurrentDS, entityNames)
		if len(entitiesImplementingInterface) == 0 {
			continue
		}

		entityFragments, _ := r.filterFragmentsByTypeNames(inlineFragmentsOnObjects, entitiesImplementingInterface)

		if len(entityFragments) > 0 {
			for _, entityFragment := range entityFragments {
				satisfies := r.inlineFragmentHasAllFieldsLocalToDatasource(entityFragment, inlineFragmentsOnInterface.selectionSetInfo.fields) ||
					r.entityHasFieldsAsRootNode(entityFragment.typeName, inlineFragmentsOnInterface.selectionSetInfo.fields)
				if !satisfies {
					return false
				}
			}
		}
	}

	return true
}

func (r *fieldSelectionRewriter) entityHasFieldsAsRootNode(entityName string, fields []fieldSelection) bool {
	for _, fieldSelection := range fields {
		if fieldSelection.fieldName == typeNameField {
			continue
		}

		if !r.dsConfiguration.HasRootNode(entityName, fieldSelection.fieldName) {
			return false
		}
	}

	return true
}

func (r *fieldSelectionRewriter) allEntitiesHaveFieldsAsRootNode(entityNames []string, fields []fieldSelection) bool {
	for _, entityName := range entityNames {
		if !r.entityHasFieldsAsRootNode(entityName, fields) {
			return false
		}
	}

	return true
}

func (r *fieldSelectionRewriter) interfaceFragmentsRequiresCleanup(inlineFragments []inlineFragmentSelectionOnInterface, parentSelectionValidTypes []string) bool {
	for _, fragment := range inlineFragments {
		if r.interfaceFragmentNeedCleanup(fragment, parentSelectionValidTypes) {
			return true
		}
	}

	return false
}

func (r *fieldSelectionRewriter) unionFragmentsRequiresCleanup(inlineFragments []inlineFragmentSelectionOnUnion, parentSelectionValidTypes []string) bool {
	for _, fragment := range inlineFragments {
		if r.unionFragmentNeedCleanup(fragment, parentSelectionValidTypes) {
			return true
		}
	}

	return false
}

func (r *fieldSelectionRewriter) objectFragmentsRequiresCleanup(inlineFragments []inlineFragmentSelection, parentSelectionValidTypes []string) bool {
	for _, fragmentOnObject := range inlineFragments {
		if r.objectFragmentNeedCleanup(fragmentOnObject, parentSelectionValidTypes) {
			return true
		}
	}

	return false
}

func (r *fieldSelectionRewriter) objectFragmentNeedCleanup(inlineFragmentOnObject inlineFragmentSelection, parentSelectionValidTypes []string) bool {
	if !r.hasTypeOnDataSource(inlineFragmentOnObject.typeName) {
		return true
	}

	if !slices.Contains(parentSelectionValidTypes, inlineFragmentOnObject.typeName) {
		return true
	}

	// check interface fragments
	if inlineFragmentOnObject.selectionSetInfo.hasInlineFragmentsOnInterfaces {
		if r.interfaceFragmentsRequiresCleanup(inlineFragmentOnObject.selectionSetInfo.inlineFragmentsOnInterfaces, []string{inlineFragmentOnObject.typeName}) {
			return true
		}
	}

	// check union fragments
	if inlineFragmentOnObject.selectionSetInfo.hasInlineFragmentsOnUnions {
		if r.unionFragmentsRequiresCleanup(inlineFragmentOnObject.selectionSetInfo.inlineFragmentsOnUnions, []string{inlineFragmentOnObject.typeName}) {
			return true
		}
	}

	return false
}

func (r *fieldSelectionRewriter) unionFragmentNeedCleanup(inlineFragmentOnUnion inlineFragmentSelectionOnUnion, parentSelectionValidTypes []string) bool {
	// check that union type exists on datasource
	if !r.hasTypeOnDataSource(inlineFragmentOnUnion.typeName) {
		return true
	}

	// We need to check if union type in the given datasource is implemented by parent selection valid types
	// because it could happen that in the given ds we have all types from union but not all of are part of the union
	// so we need to rewrite, because otherwise we won't get responses for all possible types, but only for part of union
	for _, typeName := range parentSelectionValidTypes {
		if !slices.Contains(inlineFragmentOnUnion.unionMemberTypeNamesInCurrentDS, typeName) {
			return true
		}
	}

	// if union fragment has inline fragments on objects
	// check that object type is present within parent selection valid types - e.g. members of union or parent interface
	// check each fragment for the presence of other interface fragments
	if inlineFragmentOnUnion.selectionSetInfo.hasInlineFragmentsOnObjects {
		if r.objectFragmentsRequiresCleanup(inlineFragmentOnUnion.selectionSetInfo.inlineFragmentsOnObjects, parentSelectionValidTypes) {
			return true
		}
	}

	// if union fragment has inline fragments on interfaces
	// recursively check each fragment for the presence of other interface or union fragments with the same parent selection valid types
	if inlineFragmentOnUnion.selectionSetInfo.hasInlineFragmentsOnInterfaces {
		if r.interfaceFragmentsRequiresCleanup(inlineFragmentOnUnion.selectionSetInfo.inlineFragmentsOnInterfaces, parentSelectionValidTypes) {
			return true
		}
	}

	// if union fragment has inline fragments on unions
	// recursively check each fragment for the presence of other interface or union fragments with the same parent selection valid types
	if inlineFragmentOnUnion.selectionSetInfo.hasInlineFragmentsOnUnions {
		if r.unionFragmentsRequiresCleanup(inlineFragmentOnUnion.selectionSetInfo.inlineFragmentsOnUnions, parentSelectionValidTypes) {
			return true
		}
	}

	return false
}

func (r *fieldSelectionRewriter) interfaceFragmentNeedCleanup(inlineFragmentOnInterface inlineFragmentSelectionOnInterface, parentSelectionValidTypes []string) bool {
	// check that interface type exists on datasource
	if !r.hasTypeOnDataSource(inlineFragmentOnInterface.typeName) {
		return true
	}

	// We need to check if interface type in the given datasource is implemented by parent selection valid types
	// because it could happen that in the given ds we have all types from interface but not all of them implements interface
	// so we need to rewrite, because otherwise we won't get responses for all possible types, but only for implementing interface
	for _, typeName := range parentSelectionValidTypes {
		if !slices.Contains(inlineFragmentOnInterface.typeNamesImplementingInterfaceInCurrentDS, typeName) {
			return true
		}
	}

	// if interface fragment has inline fragments on objects
	// check that object type is present within parent selection valid types - e.g. members of union or parent interface
	// check each fragment for the presence of other interface fragments
	if inlineFragmentOnInterface.selectionSetInfo.hasInlineFragmentsOnObjects {
		if r.objectFragmentsRequiresCleanup(inlineFragmentOnInterface.selectionSetInfo.inlineFragmentsOnObjects, parentSelectionValidTypes) {
			return true
		}
	}

	// if interface fragment has inline fragments on interfaces
	// recursively check each fragment for the presence of other interface fragments with the same parent selection valid types
	if inlineFragmentOnInterface.selectionSetInfo.hasInlineFragmentsOnInterfaces {
		if r.interfaceFragmentsRequiresCleanup(inlineFragmentOnInterface.selectionSetInfo.inlineFragmentsOnInterfaces, parentSelectionValidTypes) {
			return true
		}
	}

	if inlineFragmentOnInterface.selectionSetInfo.hasInlineFragmentsOnUnions {
		if r.unionFragmentsRequiresCleanup(inlineFragmentOnInterface.selectionSetInfo.inlineFragmentsOnUnions, parentSelectionValidTypes) {
			return true
		}
	}

	if inlineFragmentOnInterface.selectionSetInfo.hasFields {
		// NOTE: maybe we need to filter this typenames by parentSelectionValidTypes?
		for _, typeName := range inlineFragmentOnInterface.typeNamesImplementingInterfaceInCurrentDS {
			if !r.typeHasAllFieldLocal(typeName, inlineFragmentOnInterface.selectionSetInfo.fields) {
				return true
			}

			if r.hasRequiresConfigurationForField(typeName, inlineFragmentOnInterface.selectionSetInfo.fields) {
				return true
			}
		}
	}

	return false
}

func (r *fieldSelectionRewriter) typeHasAllFieldLocal(typeName string, fields []fieldSelection) bool {
	for _, field := range fields {
		if field.fieldName == typeNameField {
			continue
		}

		if !r.hasFieldOnDataSource(typeName, field.fieldName) {
			return false
		}
	}

	return true
}

func (r *fieldSelectionRewriter) hasRequiresConfigurationForField(typeName string, fields []fieldSelection) bool {
	return slices.ContainsFunc(r.dsConfiguration.FederationConfiguration().Requires, func(cfg FederationFieldConfiguration) bool {
		if cfg.TypeName != typeName {
			return false
		}

		return slices.ContainsFunc(fields, func(fieldSelection fieldSelection) bool {
			return cfg.FieldName == fieldSelection.fieldName
		})
	})
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

func (r *fieldSelectionRewriter) notSelectedFieldsForInlineFragment(inlineFragmentSelection inlineFragmentSelection, fields []fieldSelection) []fieldSelection {
	notSelectedFields := make([]fieldSelection, 0, len(fields))
	for _, fieldSelection := range fields {
		if fieldSelection.fieldName == typeNameField {
			continue
		}

		fieldIsSelected := false
		for _, fragmentField := range inlineFragmentSelection.selectionSetInfo.fields {
			if fragmentField.fieldName == typeNameField {
				continue
			}

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

func (r *fieldSelectionRewriter) inlineFragmentHasAllFieldsLocalToDatasource(inlineFragmentSelection inlineFragmentSelection, fields []fieldSelection) bool {
	notSelectedFields := r.notSelectedFieldsForInlineFragment(inlineFragmentSelection, fields)

	if len(notSelectedFields) == 0 {
		return true
	}

	for _, notSelectedField := range notSelectedFields {
		if notSelectedField.fieldName == typeNameField {
			continue
		}

		hasField := r.hasFieldOnDataSource(inlineFragmentSelection.typeName, notSelectedField.fieldName)

		if !hasField {
			return false
		}
	}

	return true
}

func (r *fieldSelectionRewriter) hasTypeOnDataSource(typeName string) bool {
	return r.dsConfiguration.HasRootNodeWithTypename(typeName) ||
		r.dsConfiguration.HasChildNodeWithTypename(typeName)
}

func (r *fieldSelectionRewriter) hasFieldOnDataSource(typeName string, fieldName string) bool {
	return fieldName == typeNameField ||
		r.dsConfiguration.HasRootNode(typeName, fieldName) ||
		r.dsConfiguration.HasChildNode(typeName, fieldName)
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

func (r *fieldSelectionRewriter) typeNameSelection() (selectionRef int, fieldRef int) {
	field := r.operation.AddField(ast.Field{
		Name: r.operation.Input.AppendInputString("__typename"),
	})
	return r.operation.AddSelectionToDocument(ast.Selection{
		Ref:  field.Ref,
		Kind: ast.SelectionKindField,
	}), field.Ref
}

func (r *fieldSelectionRewriter) fieldTypeNameFromUpstreamSchema(fieldRef int, enclosingTypeName ast.ByteSlice) (typeName string, ok bool) {
	fieldName := r.operation.FieldNameBytes(fieldRef)

	// if enclosing type was one of the root query types
	// we need to check if they were renamed in the upstream schema
	enclosingTypeName = r.typeNameWithRename(enclosingTypeName)

	node, hasNode := r.upstreamDefinition.NodeByName(enclosingTypeName)
	if !hasNode {
		return "", false
	}

	fieldTypeNode, ok := r.upstreamDefinition.FieldTypeNode(fieldName, node)
	if !ok {
		return "", false
	}

	return r.upstreamDefinition.NodeNameString(fieldTypeNode), true
}

func (r *fieldSelectionRewriter) getAllowedUnionMemberTypeNames(fieldRef int, unionDefRef int, enclosingTypeName ast.ByteSlice) ([]string, error) {
	unionTypeName := r.definition.UnionTypeDefinitionNameString(unionDefRef)
	unionTypeNamesFromDefinition, _ := r.definition.UnionTypeDefinitionMemberTypeNames(unionDefRef)

	// CurrentObject.field typename from the upstream schema
	fieldTypeName, ok := r.fieldTypeNameFromUpstreamSchema(fieldRef, enclosingTypeName)
	if !ok {
		return nil, errors.New("unexpected error: field type name is not found in the upstream schema")
	}

	// if typename of a field is not equal to the typename of the union type
	// then it should be a member of the union type
	if unionTypeName != fieldTypeName {
		if slices.Contains(unionTypeNamesFromDefinition, fieldTypeName) {
			return []string{fieldTypeName}, nil
		}

		// if it is not a member of the union type the config is corrupted
		return nil, errors.New("unexpected error: field type is not a member of the union type in the federated graph schema")
	}

	// when typename of a field is equal to the typename of the union type
	// we need to get allowed types from the upstream schema
	unionNode, ok := r.upstreamDefinition.NodeByNameStr(unionTypeName)
	if !ok {
		return nil, errors.New("unexpected error: union type is not found in the upstream schema")
	}

	if unionNode.Kind != ast.NodeKindUnionTypeDefinition {
		return nil, errors.New("unexpected error: node kind is not union type definition in the upstream schema")
	}

	unionTypeNames, ok := r.upstreamDefinition.UnionTypeDefinitionMemberTypeNames(unionNode.Ref)
	if !ok {
		return nil, errors.New("unexpected error: union type definition in the upstream schema do not have any members")
	}
	sort.Strings(unionTypeNames)

	return unionTypeNames, nil
}

func (r *fieldSelectionRewriter) getAllowedInterfaceMemberTypeNames(fieldRef int, interfaceDefRef int, enclosingTypeName ast.ByteSlice) (typeNames []string, isInterfaceObject bool, err error) {
	interfaceTypeName := r.definition.InterfaceTypeDefinitionNameString(interfaceDefRef)
	interfaceTypeNamesFromDefinition, _ := r.definition.InterfaceTypeDefinitionImplementedByObjectWithNames(interfaceDefRef)

	// CurrentObject.field typename from the upstream schema
	fieldTypeName, ok := r.fieldTypeNameFromUpstreamSchema(fieldRef, enclosingTypeName)
	if !ok {
		return nil, false, errors.New("unexpected error: field type name is not found in the upstream schema")
	}

	// if typename of a field is not equal to the typename of the interface type
	// then it should implement the interface type in the federated graph schema
	if interfaceTypeName != fieldTypeName {
		if slices.Contains(interfaceTypeNamesFromDefinition, fieldTypeName) {
			return []string{fieldTypeName}, false, nil
		}

		// if it is not a member of the union type the config is corrupted
		return nil, false, errors.New("unexpected error: field type do not implement the interface in the federated graph schema")
	}

	interfaceNode, hasNode := r.upstreamDefinition.NodeByNameStr(interfaceTypeName)
	if !hasNode {
		return nil, false, errors.New("unexpected error: interface type definition not found in the upstream schema")
	}

	// in case node kind is an interface type definition we just return the implementing types in this datasource
	if interfaceNode.Kind == ast.NodeKindInterfaceTypeDefinition {
		interfaceTypeNames, _ := r.upstreamDefinition.InterfaceTypeDefinitionImplementedByObjectWithNames(interfaceNode.Ref)
		sort.Strings(interfaceTypeNames)
		return interfaceTypeNames, false, nil
	}

	// otherwise we should get node kind object type definition
	// which means we are dealing with the interface object
	for _, k := range r.dsConfiguration.FederationConfiguration().InterfaceObjects {
		if k.InterfaceTypeName == interfaceTypeName {
			return k.ConcreteTypeNames, true, nil
		}
	}

	return nil, false, errors.New("unexpected error: node kind is not interface type definition in the upstream schema")
}

func (r *fieldSelectionRewriter) typeNameWithRename(typeName ast.ByteSlice) ast.ByteSlice {
	switch {
	case bytes.Equal(typeName, ast.DefaultQueryTypeName):
		if r.upstreamDefinition.Index.QueryTypeName != nil && !bytes.Equal(r.upstreamDefinition.Index.QueryTypeName, typeName) {
			return r.upstreamDefinition.Index.QueryTypeName
		}
	case bytes.Equal(typeName, ast.DefaultMutationTypeName):
		if r.upstreamDefinition.Index.MutationTypeName != nil && !bytes.Equal(r.upstreamDefinition.Index.MutationTypeName, typeName) {
			return r.upstreamDefinition.Index.MutationTypeName
		}
	case bytes.Equal(typeName, ast.DefaultSubscriptionTypeName):
		if r.upstreamDefinition.Index.SubscriptionTypeName != nil && !bytes.Equal(r.upstreamDefinition.Index.SubscriptionTypeName, typeName) {
			return r.upstreamDefinition.Index.SubscriptionTypeName
		}
	}

	return typeName
}
