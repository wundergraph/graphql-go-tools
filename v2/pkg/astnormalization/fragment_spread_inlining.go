package astnormalization

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func fragmentSpreadInline(walker *astvisitor.Walker, limits Limits, expand bool) {
	visitor := fragmentSpreadInlineVisitor{
		Walker: walker,
		limits: limits,
		expand: expand,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterSelectionSetVisitor(&visitor)
	walker.RegisterEnterFragmentDefinitionVisitor(&visitor)
	if limits.MaxFragmentExpansionSelections > 0 || limits.MaxExpandedSelectionSetFields > 0 {
		walker.RegisterEnterOperationVisitor(&visitor)
	}
	if limits.MaxExpandedSelectionSetFields > 0 {
		walker.RegisterLeaveSelectionSetVisitor(&visitor)
	}
}

type fragmentSpreadInlineVisitor struct {
	*astvisitor.Walker

	operation, definition *ast.Document
	limits                Limits
	expand                bool
	remainingCopies       int
	// Inline fragments share their enclosing field's sibling counter.
	fieldCounts []int
}

func (f *fragmentSpreadInlineVisitor) EnterFragmentDefinition(ref int) {
	f.SkipNode()
}

func (f *fragmentSpreadInlineVisitor) EnterDocument(operation, definition *ast.Document) {
	f.operation = operation
	f.definition = definition
}

func (f *fragmentSpreadInlineVisitor) EnterOperationDefinition(ref int) {
	f.remainingCopies = f.limits.MaxFragmentExpansionSelections
	f.fieldCounts = f.fieldCounts[:0]
}

func (f *fragmentSpreadInlineVisitor) EnterSelectionSet(ref int) {
	fields := 0
	for _, selection := range f.operation.SelectionSets[ref].SelectionRefs {
		if f.operation.Selections[selection].Kind == ast.SelectionKindField {
			fields++
		}
		if !f.expand || f.operation.Selections[selection].Kind != ast.SelectionKindFragmentSpread {
			continue
		}
		if f.replaceFragmentSpread(ref, f.operation.Selections[selection].Ref) {
			f.RevisitNode()
			return
		}
		if f.Report.HasErrors() {
			return
		}
	}
	if f.limits.MaxExpandedSelectionSetFields <= 0 {
		return
	}
	// RevisitNode retries the replacement loop. Charge fields only once all
	// spreads in this set have been replaced and the walker is about to descend.
	parent := f.Ancestors[len(f.Ancestors)-1].Kind
	if parent == ast.NodeKindField || parent == ast.NodeKindOperationDefinition {
		f.fieldCounts = append(f.fieldCounts, 0)
	}
	current := &f.fieldCounts[len(f.fieldCounts)-1]
	if fields > f.limits.MaxExpandedSelectionSetFields-*current {
		f.StopWithExternalErr(operationreport.ErrNormalizationLimitExceeded("max_expanded_selection_set_fields", f.limits.MaxExpandedSelectionSetFields))
		return
	}
	*current += fields
}

func (f *fragmentSpreadInlineVisitor) LeaveSelectionSet(ref int) {
	parent := f.Ancestors[len(f.Ancestors)-1].Kind
	if parent == ast.NodeKindField || parent == ast.NodeKindOperationDefinition {
		f.fieldCounts = f.fieldCounts[:len(f.fieldCounts)-1]
	}
}

func (f *fragmentSpreadInlineVisitor) replaceFragmentSpread(selectionSetRef int, ref int) (replaced bool) {
	parentTypeName := f.definition.NodeNameBytes(f.EnclosingTypeDefinition)

	spreadName := f.operation.FragmentSpreadNameBytes(ref)
	fragmentDefinitionRef, exists := f.operation.FragmentDefinitionRef(spreadName)
	if !exists {
		fragmentName := f.operation.FragmentSpreadNameBytes(ref)
		f.StopWithExternalErr(operationreport.ErrFragmentUndefined(fragmentName))
		return
	}

	fragmentTypeName := f.operation.FragmentDefinitionTypeName(fragmentDefinitionRef)
	fragmentNode, exists := f.definition.NodeByName(fragmentTypeName)
	if !exists {
		f.StopWithExternalErr(operationreport.ErrTypeUndefined(fragmentTypeName))
		return
	}

	fragmentTypeEqualsParentType := bytes.Equal(parentTypeName, fragmentTypeName)
	var enclosingTypeImplementsFragmentType bool
	var enclosingTypeIsMemberOfFragmentUnion bool
	var fragmentTypeImplementsEnclosingType bool
	var fragmentTypeIsMemberOfEnclosingUnionType bool
	var fragmentUnionIntersectsEnclosingInterface bool
	var fragmentInterfaceIntersectsEnclosingUnion bool
	var fragmentInterfaceIntersectsEnclosingInterface bool

	if fragmentNode.Kind == ast.NodeKindInterfaceTypeDefinition && f.EnclosingTypeDefinition.Kind == ast.NodeKindObjectTypeDefinition {
		enclosingTypeImplementsFragmentType =
			f.definition.NodeImplementsInterface(f.EnclosingTypeDefinition, fragmentTypeName) &&
				f.definition.NodeImplementsInterfaceFields(f.EnclosingTypeDefinition, fragmentNode)
	}

	if fragmentNode.Kind == ast.NodeKindInterfaceTypeDefinition && f.EnclosingTypeDefinition.Kind == ast.NodeKindInterfaceTypeDefinition {
		fragmentInterfaceIntersectsEnclosingInterface = f.definition.InterfacesIntersect(fragmentNode.Ref, f.EnclosingTypeDefinition.Ref)
	}

	if fragmentNode.Kind == ast.NodeKindUnionTypeDefinition {
		enclosingTypeIsMemberOfFragmentUnion = f.definition.NodeIsUnionMember(f.EnclosingTypeDefinition, fragmentNode)
	}

	if f.EnclosingTypeDefinition.Kind == ast.NodeKindInterfaceTypeDefinition {
		fragmentTypeImplementsEnclosingType =
			f.definition.NodeImplementsInterface(fragmentNode, parentTypeName) &&
				f.definition.NodeImplementsInterfaceFields(fragmentNode, f.EnclosingTypeDefinition)
	}

	if f.EnclosingTypeDefinition.Kind == ast.NodeKindInterfaceTypeDefinition && fragmentNode.Kind == ast.NodeKindUnionTypeDefinition {
		fragmentUnionIntersectsEnclosingInterface = f.definition.UnionNodeIntersectsInterfaceNode(fragmentNode, f.EnclosingTypeDefinition)
	}

	if f.EnclosingTypeDefinition.Kind == ast.NodeKindUnionTypeDefinition && fragmentNode.Kind == ast.NodeKindInterfaceTypeDefinition {
		fragmentInterfaceIntersectsEnclosingUnion = f.definition.UnionNodeIntersectsInterfaceNode(f.EnclosingTypeDefinition, fragmentNode)
	}

	if f.EnclosingTypeDefinition.Kind == ast.NodeKindUnionTypeDefinition {
		fragmentTypeIsMemberOfEnclosingUnionType = f.definition.NodeIsUnionMember(fragmentNode, f.EnclosingTypeDefinition)
	}

	replaceWith := f.operation.FragmentDefinitions[fragmentDefinitionRef].SelectionSet
	typeCondition := f.operation.FragmentDefinitions[fragmentDefinitionRef].TypeCondition

	directiveList := f.operation.FragmentSpreads[ref].Directives

	switch {
	case fragmentTypeEqualsParentType || enclosingTypeImplementsFragmentType:
		// NOTE: we always replace fragment with inline fragment
		// it is dangerous to fully inline fragment without checking for the compatibility
		// of possible nested fragments.
		// Such checks are performed in the inlineSelectionsFromInlineFragmentsVisitor
		// on the next stage of the normalization.

		fallthrough
	case fragmentTypeImplementsEnclosingType ||
		fragmentTypeIsMemberOfEnclosingUnionType ||
		enclosingTypeIsMemberOfFragmentUnion ||
		fragmentUnionIntersectsEnclosingInterface ||
		fragmentInterfaceIntersectsEnclosingUnion ||
		fragmentInterfaceIntersectsEnclosingInterface:

		var remaining *int
		if f.limits.MaxFragmentExpansionSelections > 0 {
			remaining = &f.remainingCopies
		}
		if !f.operation.ReplaceFragmentSpreadWithInlineFragmentWithBudget(selectionSetRef, ref, replaceWith, typeCondition, directiveList, remaining) {
			f.StopWithExternalErr(operationreport.ErrNormalizationLimitExceeded("max_fragment_expansion_selections", f.limits.MaxFragmentExpansionSelections))
			return false
		}

		return true
	default:
		// all other case are invalid and should be reported by validation
		return false
	}
}
