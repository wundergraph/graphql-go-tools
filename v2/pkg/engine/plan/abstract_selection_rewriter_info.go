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
	typeNamesImplementingInterfaceInCurrentDS []string
	entityNamesImplementingInterface          []string
}

func (s *inlineFragmentSelectionOnInterface) hasTypeImplementingInterface(typeName string) bool {
	if len(s.typeNamesImplementingInterfaceInCurrentDS) == 0 {
		return false
	}

	return slices.Contains(s.typeNamesImplementingInterfaceInCurrentDS, typeName)
}

func (s *inlineFragmentSelection) isFragmentOnInterface() bool {
	return s.definitionNodeKind == ast.NodeKindInterfaceTypeDefinition
}

func (r *fieldSelectionRewriter) selectionSetFieldSelections(selectionSetRef int, skipTypeName bool) (fieldSelections []fieldSelection, hasTypename bool) {
	fieldSelectionRefs := r.operation.SelectionSetFieldSelections(selectionSetRef)
	fieldSelections = make([]fieldSelection, 0, len(fieldSelectionRefs))
	for _, fieldSelectionRef := range fieldSelectionRefs {
		fieldRef := r.operation.Selections[fieldSelectionRef].Ref
		fieldName := r.operation.FieldNameString(fieldRef)

		if fieldName == "__typename" {
			hasTypename = true
			if skipTypeName {
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

func (r *fieldSelectionRewriter) collectFieldInformation(fieldRef int) (selectionSetInfo, error) {
	fieldSelectionSetRef, ok := r.operation.FieldSelectionSet(fieldRef)
	if !ok {
		return selectionSetInfo{}, FieldDoesntHaveSelectionSetErr
	}

	// we should skip typename field in the list of fields for the root field information
	// it should not be included when we are checking list of fields in the selection set
	// it will be added later in case it was selected
	return r.collectSelectionSetInformation(fieldSelectionSetRef, true)
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

	// Note: We are getting inline fragment type from the FEDERATED graph definition,
	// because it could be absent in the current SUBGRAPH document
	node, hasNode := r.definition.NodeByNameStr(typeCondition)
	if !hasNode {
		return InlineFragmentTypeIsNotExistsErr
	}

	selectionSetInfo, err := r.collectSelectionSetInformation(inlineFragmentSelectionSetRef, false)
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

	// Note: We are getting type names implementing interface from the current SUBGRAPH definion
	typeNamesImplementingInterface, _ := r.upstreamDefinition.InterfaceTypeDefinitionImplementedByObjectWithNames(node.Ref)
	entityNames, _ := r.datasourceHasEntitiesWithName(typeNamesImplementingInterface)

	inlineFragmentSelectionOnInterface := inlineFragmentSelectionOnInterface{
		inlineFragmentSelection:                   inlineFragmentSelection,
		typeNamesImplementingInterfaceInCurrentDS: typeNamesImplementingInterface,
		entityNamesImplementingInterface:          entityNames,
	}

	*inlineFragmentsOnInterfaces = append(*inlineFragmentsOnInterfaces, inlineFragmentSelectionOnInterface)

	return nil
}

func (r *fieldSelectionRewriter) collectSelectionSetInformation(selectionSetRef int, skipTypeName bool) (selectionSetInfo, error) {
	fieldSelections, hasSharedTypename := r.selectionSetFieldSelections(selectionSetRef, skipTypeName)

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
