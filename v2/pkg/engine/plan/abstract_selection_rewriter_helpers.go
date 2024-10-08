package plan

import (
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

	return entityNames
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

func (r *fieldSelectionRewriter) allFragmentTypesExistsOnDatasource(inlineFragments []inlineFragmentSelection) bool {
	for _, inlineFragment := range inlineFragments {
		if !r.hasTypeOnDataSource(inlineFragment.typeName) {
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

func (r *fieldSelectionRewriter) objectFragmentNeedCleanup(inlineFragment inlineFragmentSelection) bool {
	if !r.hasTypeOnDataSource(inlineFragment.typeName) {
		return true
	}

	for _, fragmentOnInterface := range inlineFragment.selectionSetInfo.inlineFragmentsOnInterfaces {
		if r.interfaceFragmentNeedCleanup(fragmentOnInterface, []string{inlineFragment.typeName}) {
			return true
		}
	}

	return false
}

func (r *fieldSelectionRewriter) interfaceFragmentNeedCleanup(inlineFragment inlineFragmentSelectionOnInterface, parentSelectionValidTypes []string) bool {
	// check that interface type exists on datasource
	if !r.hasTypeOnDataSource(inlineFragment.typeName) {
		return true
	}

	// if interface fragment has inline fragments on objects
	// check that object type is present within parent selection valid types - e.g. members of union or parent interface
	// check each fragment for the presence of other interface fragments
	if inlineFragment.selectionSetInfo.hasInlineFragmentsOnObjects {
		for _, fragmentOnObject := range inlineFragment.selectionSetInfo.inlineFragmentsOnObjects {
			if !slices.Contains(parentSelectionValidTypes, fragmentOnObject.typeName) {
				return true
			}

			if r.objectFragmentNeedCleanup(fragmentOnObject) {
				return true
			}
		}
	}

	// if interface fragment has inline fragments on interfaces
	// recursively check each fragment for the presence of other interface fragments with the same parent selection valid types
	if inlineFragment.selectionSetInfo.hasInlineFragmentsOnInterfaces {
		for _, fragmentOnInterface := range inlineFragment.selectionSetInfo.inlineFragmentsOnInterfaces {
			if r.interfaceFragmentNeedCleanup(fragmentOnInterface, parentSelectionValidTypes) {
				return true
			}
		}
	}

	if inlineFragment.selectionSetInfo.hasFields {
		// NOTE: maybe we need to filter this typenames by parentSelectionValidTypes?
		for _, typeName := range inlineFragment.typeNamesImplementingInterfaceInCurrentDS {
			if !r.typeHasAllFieldLocal(typeName, inlineFragment.selectionSetInfo.fields) {
				return true
			}

			if r.hasRequiresConfigurationForField(typeName, inlineFragment.selectionSetInfo.fields) {
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
