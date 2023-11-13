package plan

import (
	"golang.org/x/exp/slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type selectionSetInfo struct {
	hasTypeNameSelection           bool // __typename is selected
	fields                         []fieldSelection
	hasFields                      bool
	inlineFragmentsOnObjects       []inlineFragmentSelection
	hasInlineFragmentsOnObjects    bool
	inlineFragmentsOnInterfaces    []inlineFragmentSelectionOnInterface
	hasInlineFragmentsOnInterfaces bool
}

type fieldSelection struct {
	fieldSelectionRef int
	fieldName         string
}

type inlineFragmentSelection struct {
	selectionRef       int
	typeName           string
	hasDirectives      bool
	definitionNodeKind ast.NodeKind
	definitionNodeRef  int
	selectionSetInfo   selectionSetInfo
}

type inlineFragmentSelectionOnInterface struct {
	inlineFragmentSelection
	typeNamesImplementingInterface   []string
	entityNamesImplementingInterface []string
}

func (s *inlineFragmentSelectionOnInterface) hasTypeImplementingInterface(typeName string) bool {
	if len(s.typeNamesImplementingInterface) == 0 {
		return false
	}

	return slices.Contains(s.typeNamesImplementingInterface, typeName)
}

func (s *inlineFragmentSelection) isFragmentOnInterface() bool {
	return s.definitionNodeKind == ast.NodeKindInterfaceTypeDefinition
}

func (r *fieldSelectionRewriter) selectionSetFieldSelections(selectionSetRef int) (fieldSelections []fieldSelection, hasTypename bool) {
	fieldSelectionRefs := r.operation.SelectionSetFieldSelections(selectionSetRef)
	fieldSelections = make([]fieldSelection, 0, len(fieldSelectionRefs))
	for _, fieldSelectionRef := range fieldSelectionRefs {
		fieldRef := r.operation.Selections[fieldSelectionRef].Ref
		fieldName := r.operation.FieldNameString(fieldRef)
		if fieldName == "__typename" {
			hasTypename = true
			continue
		}

		fieldSelections = append(fieldSelections, fieldSelection{
			fieldSelectionRef: fieldSelectionRef,
			fieldName:         fieldName,
		})
	}

	return fieldSelections, hasTypename
}

func (r *fieldSelectionRewriter) collectFieldInformation(fieldRef int) (selectionSetInfo, error) {
	fieldSelectionSetRef, ok := r.operation.FieldSelectionSet(fieldRef)
	if !ok {
		return selectionSetInfo{}, FieldDoesntHaveSelectionSetErr
	}

	return r.collectSelectionSetInformation(fieldSelectionSetRef)
}

func (r *fieldSelectionRewriter) collectInlineFragmentInformation(
	inlineFragmentSelectionRef int,
	inlineFragmentSelectionsOnObjects *[]inlineFragmentSelection,
	inlineFragmentsOnInterfaces *[]inlineFragmentSelectionOnInterface,
) error {

	inlineFragmentRef := r.operation.Selections[inlineFragmentSelectionRef].Ref

	typeCondition := r.operation.InlineFragmentTypeConditionNameString(inlineFragmentRef)
	inlineFragmentSelectionSetRef, ok := r.operation.InlineFragmentSelectionSet(inlineFragmentRef)
	if !ok {
		return InlineFragmentDoesntHaveSelectionSetErr
	}

	hasDirectives := r.operation.InlineFragmentHasDirectives(inlineFragmentRef)

	node, hasNode := r.definition.NodeByNameStr(typeCondition)
	if !hasNode {
		return InlineFragmentTypeIsNotExistsErr
	}

	selectionSetInfo, err := r.collectSelectionSetInformation(inlineFragmentSelectionSetRef)
	if err != nil {
		return err
	}

	inlineFragmentSelection := inlineFragmentSelection{
		selectionRef:       inlineFragmentSelectionRef,
		typeName:           typeCondition,
		hasDirectives:      hasDirectives,
		definitionNodeKind: node.Kind,
		definitionNodeRef:  node.Ref,
		selectionSetInfo:   selectionSetInfo,
	}

	if inlineFragmentSelection.definitionNodeKind == ast.NodeKindObjectTypeDefinition {
		*inlineFragmentSelectionsOnObjects = append(*inlineFragmentSelectionsOnObjects, inlineFragmentSelection)
		return nil
	}

	typeNamesImplementingInterface, _ := r.definition.InterfaceTypeDefinitionImplementedByObjectWithNames(node.Ref)
	entityNames, _ := r.datasourceHasEntitiesWithName(typeNamesImplementingInterface)

	inlineFragmentSelectionOnInterface := inlineFragmentSelectionOnInterface{
		inlineFragmentSelection:          inlineFragmentSelection,
		typeNamesImplementingInterface:   typeNamesImplementingInterface,
		entityNamesImplementingInterface: entityNames,
	}

	*inlineFragmentsOnInterfaces = append(*inlineFragmentsOnInterfaces, inlineFragmentSelectionOnInterface)

	return nil
}

func (r *fieldSelectionRewriter) collectSelectionSetInformation(selectionSetRef int) (selectionSetInfo, error) {
	fieldSelections, hasSharedTypename := r.selectionSetFieldSelections(selectionSetRef)

	inlineFragmentSelectionRefs := r.operation.SelectionSetInlineFragmentSelections(selectionSetRef)
	inlineFragmentSelectionsOnObjects := make([]inlineFragmentSelection, 0, len(inlineFragmentSelectionRefs))
	inlineFragmentsOnInterfaces := make([]inlineFragmentSelectionOnInterface, 0, len(inlineFragmentSelectionRefs))

	for _, inlineFragmentSelectionRef := range inlineFragmentSelectionRefs {
		err := r.collectInlineFragmentInformation(inlineFragmentSelectionRef, &inlineFragmentSelectionsOnObjects, &inlineFragmentsOnInterfaces)
		if err != nil {
			return selectionSetInfo{}, err
		}
	}

	return selectionSetInfo{
		fields:                         fieldSelections,
		hasFields:                      len(fieldSelections) > 0,
		hasTypeNameSelection:           hasSharedTypename,
		inlineFragmentsOnObjects:       inlineFragmentSelectionsOnObjects,
		hasInlineFragmentsOnObjects:    len(inlineFragmentSelectionsOnObjects) > 0,
		inlineFragmentsOnInterfaces:    inlineFragmentsOnInterfaces,
		hasInlineFragmentsOnInterfaces: len(inlineFragmentsOnInterfaces) > 0,
	}, nil
}
