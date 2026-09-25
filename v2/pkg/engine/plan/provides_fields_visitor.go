package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type providesInput struct {
	parentTypeName       string
	providesSelectionSet string
	definition           *ast.Document
}

type addTypenamesVisitor struct {
	walker                       *astvisitor.Walker
	providesFieldSet, definition *ast.Document
}

func (a *addTypenamesVisitor) EnterSelectionSet(ref int) {
	exist, _ := a.providesFieldSet.SelectionSetHasFieldSelectionWithExactName(ref, typeNameFieldBytes)
	if exist {
		return
	}

	// Add __typename to the selection set
	fieldNode := a.providesFieldSet.AddField(ast.Field{
		Name: a.providesFieldSet.Input.AppendInputBytes(typeNameFieldBytes),
	})
	selectionRef := a.providesFieldSet.AddSelectionToDocument(ast.Selection{
		Ref:  fieldNode.Ref,
		Kind: ast.SelectionKindField,
	})
	a.providesFieldSet.AddSelectionRefToSelectionSet(ref, selectionRef)
}

func addTypenames(operation, definition *ast.Document, report *operationreport.Report) {
	walker := astvisitor.NewWalkerWithID(32, "AddTypenamesVisitor")
	visitor := &addTypenamesVisitor{
		walker:           &walker,
		providesFieldSet: operation,
		definition:       definition,
	}
	walker.RegisterEnterSelectionSetVisitor(visitor)
	walker.Walk(operation, definition, report)
}

func providesFragment(fieldTypeName string, providesSelectionSet string, definition *ast.Document) (*ast.Document, *operationreport.Report) {
	providesFieldSet, report := RequiredFieldsFragment(fieldTypeName, providesSelectionSet, false)
	if report.HasErrors() {
		return nil, report
	}

	// prewalk provides selection set and add a typename to each selection set
	// as when we could select a field we could select __typename as well
	addTypenames(providesFieldSet, definition, report)
	if report.HasErrors() {
		return nil, report
	}

	return providesFieldSet, report
}

// providesSelection describes fields promised by a @provides directive at a single
// nesting level, keyed by field name. A field has multiple branches only when the
// provides selection mentions it under different inline fragments, e.g.
// "... on A { i { x } } ... on B { i { y } }" - each branch keeps its own nested
// selection, so which fields are provided deeper down stays correlated with the
// enclosing type.
type providesSelection map[string][]providesBranch

type providesBranch struct {
	// allowedTypes are enclosing types under which the field is provided at this position:
	// for a field on an abstract type - the type itself plus its implementers or members,
	// so a query matches whether it selects the field on the abstract type or on a
	// concrete one; for a field under a concrete inline fragment - just that type.
	allowedTypes map[string]struct{}
	// selection describes provided child fields, nil for leaf fields
	selection providesSelection
}

// providedTypeSelection returns the nested provided selection when the field is
// provided under the given enclosing type. Safe to call on a nil selection.
func (s providesSelection) providedTypeSelection(fieldName, typeName string) (providesSelection, bool) {
	for _, branch := range s[fieldName] {
		if _, ok := branch.allowedTypes[typeName]; ok {
			return branch.selection, true
		}
	}
	return nil, false
}

func providesSuggestions(input *providesInput) (providesSelection, *operationreport.Report) {
	providesFieldSet, report := providesFragment(input.parentTypeName, input.providesSelectionSet, input.definition)
	if report.HasErrors() {
		return nil, report
	}

	walker := astvisitor.NewWalkerWithID(48, "ProvidesVisitor")

	root := make(providesSelection)
	visitor := &providesVisitor{
		walker:           &walker,
		providesFieldSet: providesFieldSet,
		definition:       input.definition,
		selectionStack:   []providesSelection{root},
	}
	walker.RegisterFieldVisitor(visitor)

	walker.Walk(providesFieldSet, input.definition, report)

	return root, report
}

type providesVisitor struct {
	walker           *astvisitor.Walker
	providesFieldSet *ast.Document
	definition       *ast.Document

	// selectionStack holds the selection currently being filled at each nesting level,
	// selectionStack[0] is the root selection of the provides fragment
	selectionStack []providesSelection
}

func (v *providesVisitor) EnterField(ref int) {
	fieldName := v.providesFieldSet.FieldNameString(ref)

	branch := providesBranch{
		allowedTypes: v.expandedEnclosingTypes(v.walker.EnclosingTypeDefinition),
	}
	if v.providesFieldSet.FieldHasSelections(ref) {
		branch.selection = make(providesSelection)
	}

	currentSelection := v.selectionStack[len(v.selectionStack)-1]
	currentSelection[fieldName] = append(currentSelection[fieldName], branch)

	if branch.selection != nil {
		v.selectionStack = append(v.selectionStack, branch.selection)
	}
}

func (v *providesVisitor) LeaveField(ref int) {
	if v.providesFieldSet.FieldHasSelections(ref) {
		v.selectionStack = v.selectionStack[:len(v.selectionStack)-1]
	}
}

// expandedEnclosingTypes returns the enclosing type name plus, when it is abstract,
// every concrete object type it can resolve to (interface implementers / union members).
func (v *providesVisitor) expandedEnclosingTypes(enclosing ast.Node) map[string]struct{} {
	out := make(map[string]struct{})
	out[enclosing.NameString(v.definition)] = struct{}{}

	switch enclosing.Kind {
	case ast.NodeKindInterfaceTypeDefinition:
		implementers, _ := v.definition.InterfaceTypeDefinitionImplementedByObjectWithNames(enclosing.Ref)
		for _, name := range implementers {
			out[name] = struct{}{}
		}
	case ast.NodeKindUnionTypeDefinition:
		members, _ := v.definition.UnionTypeDefinitionMemberTypeNames(enclosing.Ref)
		for _, name := range members {
			out[name] = struct{}{}
		}
	}

	return out
}
