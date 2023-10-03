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

func (r *interfaceSelectionRewriter) datasourceHasEntitiesWithName(dsConfiguration *DataSourceConfiguration, typeNames []string) (dsTypeNames []string, ok bool) {
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

	dsTypeNames = make([]string, 0, len(typeNames))
	for _, typeName := range typeNames {
		if dsConfiguration.HasRootNodeWithTypename(typeName) ||
			dsConfiguration.HasChildNodeWithTypename(typeName) {
			dsTypeNames = append(dsTypeNames, typeName)
		}
	}

	return dsTypeNames, true
}

func (r *interfaceSelectionRewriter) interfaceFieldSelectionNeedsRewrite(fieldRef int, dsConfiguration *DataSourceConfiguration, typeNames []string) bool {

	/*
		We do not need to rewrite the selection set if:
		- all types implementing the interface have a root node with the requested fields
		- or selections contains inline fragments for all types implementing the interface
	*/

	fieldSelectionSetRef := r.operation.FieldSelectionSet(fieldRef)

	fieldSelections := r.operation.SelectionSetFieldSelections(fieldSelectionSetRef)
	fieldNames := make([]string, 0, len(fieldSelections))
	for _, fieldSelectionRef := range fieldSelections {
		fieldNames = append(fieldNames, r.operation.FieldNameString(fieldSelectionRef))
	}

	inlineFragmentSelections := r.operation.SelectionSetInlineFragmentSelections(fieldSelectionSetRef)
	selectionsForTypes := make(map[string][]string)
	for _, inlineFragmentSelectionRef := range inlineFragmentSelections {
		typeCondition := r.operation.InlineFragmentTypeConditionNameString(inlineFragmentSelectionRef)
		selectionsRefs := r.operation.InlineFragmentSelections(inlineFragmentSelectionRef)

		selectionsForTypes[typeCondition] = make([]string, 0, len(selectionsRefs))

		for _, selectionRef := range selectionsRefs {
			_ = selectionRef
		}
	}

	return false
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

	dsTypeNames, ok := r.datasourceHasEntitiesWithName(dsConfiguration, typeNames)
	if !ok {
		return false
	}

	if !r.interfaceFieldSelectionNeedsRewrite(fieldRef, dsConfiguration, dsTypeNames) {
		return false
	}

	// TODO: implement rewrite

	/*
		1) extract selections which is not inline-fragments - e.g. shared selections
		2) extract selections for each inline fragment
		3) for types which do not have inline-fragment - add inline fragment with shared fields
		4) for types which have inline-fragment - add not selected shared fields to existing inline fragment
	*/

	return true
}
