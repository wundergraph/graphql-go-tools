package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"sort"
)

func SortSelectionSets(walker *astvisitor.Walker) {
	visitor := &sortSelectionSets{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterSelectionSetVisitor(visitor)
}

type sortSelectionSets struct {
	*astvisitor.Walker
	operation *ast.Document
}

func (s *sortSelectionSets) EnterDocument(operation, _ *ast.Document) {
	s.operation = operation
}

func (s *sortSelectionSets) EnterSelectionSet(ref int) {
	selectionSet := s.operation.SelectionSets[ref]
	selectionRefs := selectionSet.SelectionRefs

	type sortable struct {
		ref int
		key string
	}

	sortables := make([]sortable, len(selectionRefs))
	for i, selRef := range selectionRefs {
		selection := s.operation.Selections[selRef]
		var key string
		switch selection.Kind {
		case ast.SelectionKindField:
			key = s.fieldKey(selection.Ref)
		case ast.SelectionKindFragmentSpread:
			key = s.fragmentSpreadKey(selection.Ref)
		case ast.SelectionKindInlineFragment:
			key = s.inlineFragmentKey(selection.Ref)
		default:
			key = ""
		}

		sortables[i] = sortable{selRef, key}
	}

	sort.SliceStable(sortables, func(i, j int) bool {
		return sortables[i].key < sortables[j].key
	})

	sortedSelectionRefs := make([]int, len(sortables))
	for i, item := range sortables {
		sortedSelectionRefs[i] = item.ref
	}

	s.operation.SelectionSets[ref].SelectionRefs = sortedSelectionRefs
}

func (s *sortSelectionSets) fieldKey(fieldRef int) string {
	field := s.operation.Fields[fieldRef]
	if field.Alias.IsDefined {
		return s.operation.FieldAliasString(fieldRef)
	}

	return string(s.operation.FieldNameBytes(fieldRef))
}

func (s *sortSelectionSets) fragmentSpreadKey(spreadRef int) string {
	return string(s.operation.FragmentSpreadNameBytes(spreadRef))
}

func (s *sortSelectionSets) inlineFragmentKey(fragmentRef int) string {
	inlineFragment := s.operation.InlineFragments[fragmentRef]
	if inlineFragment.TypeCondition.Type == -1 {
		return ""
	}
	typeName := s.operation.TypeNameBytes(inlineFragment.TypeCondition.Type)

	return string(typeName)
}
