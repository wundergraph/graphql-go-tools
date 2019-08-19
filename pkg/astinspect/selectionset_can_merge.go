package astinspect

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
)

func SelectionSetCanMerge(set int, enclosingNode ast.Node, operation, definition *ast.Document) bool {
	selectionSetCanMerge := &selectionSetCanMerge{}
	return selectionSetCanMerge.Do(set, enclosingNode, operation, definition)
}

type selectionSetCanMerge struct {
	operation, definition *ast.Document
}

func (s *selectionSetCanMerge) Do(set int, enclosingNode ast.Node, operation, definition *ast.Document) bool {

	s.operation = operation
	s.definition = definition

	if !s.selectionSetInlineFragmentsCanMerge(set) {
		return false
	}

	if !s.selectionSetFieldsCanMerge(set, enclosingNode) {
		return false
	}

	return true
}

func (s *selectionSetCanMerge) selectionSetFieldsCanMerge(set int, enclosingNode ast.Node) bool {
	for _, i := range s.operation.SelectionSets[set].SelectionRefs {
		left := s.operation.Selections[i]
		if !s.shouldEvaluateField(left) {
			continue
		}
		for _, j := range s.operation.SelectionSets[set].SelectionRefs {
			right := s.operation.Selections[j]
			if i == j || i > j {
				continue
			}
			if !s.shouldEvaluateField(right) {
				continue
			}
			if !s.fieldsCanMerge(left.Ref, right.Ref, enclosingNode) {
				return false
			}
		}
	}
	return true
}

func (s *selectionSetCanMerge) selectionSetInlineFragmentsCanMerge(set int) bool {
	for _, i := range s.operation.SelectionSets[set].SelectionRefs {
		left := s.operation.Selections[i]
		if !s.shouldEvaluateInlineFragment(left) {
			continue
		}
		for _, j := range s.operation.SelectionSets[set].SelectionRefs {
			right := s.operation.Selections[j]
			if i == j || i > j {
				continue
			}
			if !s.shouldEvaluateInlineFragment(right) {
				continue
			}
			if !s.inlineFragmentsCanMerge(left.Ref, right.Ref) {
				return false
			}
		}
	}

	return true
}

func (s *selectionSetCanMerge) inlineFragmentsCanMerge(left, right int) bool {

	leftTypeName := s.operation.InlineFragmentTypeConditionName(left)
	rightTypeName := s.operation.InlineFragmentTypeConditionName(right)

	leftDefinition, ok := s.definition.Index.Nodes[string(leftTypeName)]
	if !ok {
		return false
	}
	rightDefinition, ok := s.definition.Index.Nodes[string(rightTypeName)]
	if !ok {
		return false
	}

	leftSelections := s.operation.InlineFragmentSelections(left)
	rightSelections := s.operation.InlineFragmentSelections(right)

	if len(leftSelections) != len(rightSelections) {
		return false
	}

	for i := 0; i < len(leftSelections); i++ {
		left := s.operation.Selections[leftSelections[i]]
		right := s.operation.Selections[rightSelections[i]]
		if !s.selectionsCanMerge(left, right, leftDefinition, rightDefinition) {
			return false
		}
	}

	return true
}

func (s *selectionSetCanMerge) selectionsCanMerge(leftSelection, rightSelection ast.Selection, leftDefinitionNode, rightDefinitionNode ast.Node) bool {

	if leftSelection.Kind != rightSelection.Kind {
		return false
	}

	switch leftSelection.Kind {
	case ast.SelectionKindInlineFragment:
		return s.inlineFragmentsCanMerge(leftSelection.Ref, rightSelection.Ref)
	case ast.SelectionKindField:

		if !s.operation.FieldsHaveSameShape(leftSelection.Ref, rightSelection.Ref) {
			return false
		}

		left := s.operation.Fields[leftSelection.Ref]
		right := s.operation.Fields[rightSelection.Ref]

		leftDefinition, err := s.definition.NodeFieldDefinitionByName(leftDefinitionNode, s.operation.Input.ByteSlice(left.Name))
		if err != nil {
			return false
		}

		rightDefinition, err := s.definition.NodeFieldDefinitionByName(rightDefinitionNode, s.operation.Input.ByteSlice(right.Name))
		if err != nil {
			return false
		}

		leftType := s.definition.FieldDefinitionType(leftDefinition)
		rightType := s.definition.FieldDefinitionType(rightDefinition)

		if left.HasSelections != right.HasSelections {
			return false
		}

		if !left.HasSelections {
			if !s.definition.TypesAreEqualDeep(leftType, rightType) {
				return false
			}
			return true
		}

		leftSelections := s.operation.SelectionSets[left.SelectionSet].SelectionRefs
		rightSelections := s.operation.SelectionSets[right.SelectionSet].SelectionRefs

		if len(leftSelections) != len(rightSelections) {
			return false
		}

		leftTypeName := s.definition.ResolveTypeName(leftType)
		rightTypeName := s.definition.ResolveTypeName(rightType)

		for i := 0; i < len(leftSelections); i++ {
			leftSelection = s.operation.Selections[leftSelections[i]]
			rightSelection = s.operation.Selections[rightSelections[i]]
			leftDefinitionNode = s.definition.Index.Nodes[string(leftTypeName)]
			rightDefinitionNode = s.definition.Index.Nodes[string(rightTypeName)]
			if !s.selectionsCanMerge(leftSelection, rightSelection, leftDefinitionNode, rightDefinitionNode) {
				return false
			}
		}

		return true

	default:
		return false
	}
}

func (s *selectionSetCanMerge) shouldEvaluateField(selection ast.Selection) bool {
	if selection.Kind != ast.SelectionKindField {
		return false
	}
	return true
}

func (s *selectionSetCanMerge) shouldEvaluateInlineFragment(selection ast.Selection) bool {
	if selection.Kind != ast.SelectionKindInlineFragment {
		return false
	}
	if !s.operation.InlineFragments[selection.Ref].HasSelections {
		return false
	}
	if s.operation.InlineFragments[selection.Ref].TypeCondition.Type == -1 {
		return false
	}
	return true
}

func (s *selectionSetCanMerge) fieldsCanMerge(left int, right int, enclosingNode ast.Node) bool {
	leftName := s.operation.FieldName(left)
	rightName := s.operation.FieldName(right)
	leftAlias := s.operation.FieldAlias(left)
	rightAlias := s.operation.FieldAlias(right)
	leftAliasDefined := s.operation.FieldAliasIsDefined(left)
	rightAliasDefined := s.operation.FieldAliasIsDefined(right)

	if !bytes.Equal(leftAlias, rightAlias) {
		if leftAliasDefined && !rightAliasDefined {
			return !bytes.Equal(leftAlias, rightName)
		}
		if rightAliasDefined && !leftAliasDefined {
			return !bytes.Equal(rightAlias, leftName)
		}
		return true
	}

	if !bytes.Equal(leftName, rightName) {
		return false
	}

	if !s.operation.FieldsAreEqualFlat(left, right) {
		return false
	}

	/*
		if s.operation.FieldHasSelections(left) != s.operation.FieldHasSelections(right) {
			return false
		}

		if s.operation.FieldHasSelections(left) {
			return true
		}

			leftSelectionSet := s.operation.Fields[left].SelectionSet
			rightSelectionSet := s.operation.Fields[right].SelectionSet

			leftSelections := s.operation.SelectionSets[leftSelectionSet].SelectionRefs
			rightSelections := s.operation.SelectionSets[rightSelectionSet].SelectionRefs

			if len(leftSelections) != len(rightSelections) {
				return false
			}

			definition, err := s.definition.NodeFieldDefinitionByName(enclosingNode, leftName)
			if err != nil {
				return false
			}

			definitionNode := s.definition.Index.Nodes[string(s.definition.FieldDefinitionName(definition))]

			for i := 0; i < len(leftSelections); i++ {
				leftSelection := s.operation.Selections[leftSelections[i]]
				rightSelection := s.operation.Selections[rightSelections[i]]

				if !s.selectionsCanMerge(leftSelection, rightSelection, definitionNode, definitionNode) {
					return false
				}
			}*/

	return true
}
