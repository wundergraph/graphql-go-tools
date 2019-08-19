package astvisitor

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
)

type Walker struct {
	err              error
	document         *ast.Document
	definition       *ast.Document
	visitor          Visitor
	depth            int
	ancestors        []ast.Node
	parentDefinition ast.Node
}

func NewWalker(ancestorSize int) Walker {
	return Walker{
		ancestors: make([]ast.Node, 0, ancestorSize),
	}
}

type Info struct {
	Depth                   int
	Ancestors               []ast.Node
	SelectionSet            int
	SelectionsBefore        []int
	SelectionsAfter         []int
	ArgumentsBefore         []int
	ArgumentsAfter          []int
	HasSelections           bool
	FieldTypeDefinition     ast.Node
	EnclosingTypeDefinition ast.Node
	IsLastRootNode          bool
}

type Action int

const (
	NOOP Action = iota
	RevisitCurrentNode
)

type Instruction struct {
	Action Action
}

type Visitor interface {
	EnterOperationDefinition(ref int, info Info)
	LeaveOperationDefinition(ref int, info Info)
	EnterSelectionSet(ref int, info Info) (instruction Instruction)
	LeaveSelectionSet(ref int, info Info)
	EnterField(ref int, info Info)
	LeaveField(ref int, info Info)
	EnterArgument(ref int, definition int, info Info)
	LeaveArgument(ref int, definition int, info Info)
	EnterFragmentSpread(ref int, info Info)
	LeaveFragmentSpread(ref int, info Info)
	EnterInlineFragment(ref int, info Info)
	LeaveInlineFragment(ref int, info Info)
	EnterFragmentDefinition(ref int, info Info)
	LeaveFragmentDefinition(ref int, info Info)
}

func (w *Walker) Visit(document, definition *ast.Document, visitor Visitor) error {
	w.err = nil
	w.ancestors = w.ancestors[:0]
	w.document = document
	w.definition = definition
	w.depth = 0
	w.visitor = visitor
	w.walk()
	return w.err
}

func (w *Walker) walk() {

	if w.document == nil {
		w.err = fmt.Errorf("document must not be nil")
		return
	}

	for i := range w.document.RootNodes {
		isLast := i == len(w.document.RootNodes)-1
		ref := w.document.RootNodes[i].Ref
		switch w.document.RootNodes[i].Kind {
		case ast.NodeKindOperationDefinition:
			if w.definition == nil {
				w.err = fmt.Errorf("definition must not be nil when walking operations")
				return
			}
			w.walkOperationDefinition(ref, isLast)
		case ast.NodeKindFragmentDefinition:
			if w.definition == nil {
				w.err = fmt.Errorf("definition must not be nil when walking operations")
				return
			}
			w.walkFragmentDefinition(ref, isLast)
		}
	}
}

func (w *Walker) appendAncestor(ref int, kind ast.NodeKind) {
	w.ancestors = append(w.ancestors, ast.Node{
		Kind: kind,
		Ref:  ref,
	})
}

func (w *Walker) removeLastAncestor() {
	w.ancestors = w.ancestors[:len(w.ancestors)-1]
}

func (w *Walker) increaseDepth() {
	w.depth++
}

func (w *Walker) decreaseDepth() {
	w.depth--
}

func (w *Walker) walkOperationDefinition(ref int, isLastRootNode bool) {
	w.increaseDepth()

	info := Info{
		Depth:                   w.depth,
		Ancestors:               nil,
		SelectionSet:            -1,
		SelectionsBefore:        nil,
		SelectionsAfter:         nil,
		HasSelections:           false,
		EnclosingTypeDefinition: w.operationDefinitionTypeDefinition(ref),
		IsLastRootNode:          isLastRootNode,
	}

	w.visitor.EnterOperationDefinition(ref, info)

	w.appendAncestor(ref, ast.NodeKindOperationDefinition)

	if w.document.OperationDefinitions[ref].HasSelections {
		w.walkSelectionSet(w.document.OperationDefinitions[ref].SelectionSet, info.EnclosingTypeDefinition)
	}

	w.visitor.LeaveOperationDefinition(ref, info)
	w.decreaseDepth()
	w.removeLastAncestor()
}

func (w *Walker) operationDefinitionTypeDefinition(ref int) (typeDefinition ast.Node) {
	switch w.document.OperationDefinitions[ref].OperationType {
	case ast.OperationTypeQuery:
		typeDefinition = w.definition.Index.Nodes[string(w.definition.Index.QueryTypeName)]
	case ast.OperationTypeMutation:
		typeDefinition = w.definition.Index.Nodes[string(w.definition.Index.MutationTypeName)]
	case ast.OperationTypeSubscription:
		typeDefinition = w.definition.Index.Nodes[string(w.definition.Index.SubscriptionTypeName)]
	}
	return
}

func (w *Walker) walkSelectionSet(ref int, enclosingTypeDefinition ast.Node) {
	w.increaseDepth()

	info := Info{
		Depth:                   w.depth,
		Ancestors:               w.ancestors,
		SelectionSet:            ref,
		SelectionsBefore:        nil,
		SelectionsAfter:         nil,
		HasSelections:           true,
		EnclosingTypeDefinition: enclosingTypeDefinition,
	}

	w.enterSelectionSet(ref, info)

	w.appendAncestor(ref, ast.NodeKindSelectionSet)

	for i, j := range w.document.SelectionSets[ref].SelectionRefs {

		info.SelectionsBefore = w.document.SelectionSets[ref].SelectionRefs[:i]
		info.SelectionsAfter = w.document.SelectionSets[ref].SelectionRefs[i+1:]

		switch w.document.Selections[j].Kind {
		case ast.SelectionKindField:
			w.walkField(w.document.Selections[j].Ref, info)
		case ast.SelectionKindFragmentSpread:
			w.walkFragmentSpread(w.document.Selections[j].Ref, info)
		case ast.SelectionKindInlineFragment:
			w.walkInlineFragment(w.document.Selections[j].Ref, info)
		}
	}

	info.SelectionsBefore = nil
	info.SelectionsAfter = nil

	w.visitor.LeaveSelectionSet(ref, info)
	w.removeLastAncestor()
	w.decreaseDepth()
}

func (w *Walker) enterSelectionSet(ref int, info Info) {
	for {
		instruction := w.visitor.EnterSelectionSet(ref, info)
		switch instruction.Action {
		case RevisitCurrentNode:
			continue
		default:
			return
		}
	}
}

func (w *Walker) walkField(ref int, enclosing Info) {
	w.increaseDepth()

	info := Info{
		Depth:                   w.depth,
		Ancestors:               w.ancestors,
		SelectionSet:            enclosing.SelectionSet,
		SelectionsBefore:        enclosing.SelectionsBefore,
		SelectionsAfter:         enclosing.SelectionsAfter,
		HasSelections:           w.document.Fields[ref].HasSelections,
		FieldTypeDefinition:     w.fieldTypeDefinition(ref, enclosing.EnclosingTypeDefinition),
		EnclosingTypeDefinition: enclosing.EnclosingTypeDefinition,
	}
	w.visitor.EnterField(ref, info)

	w.appendAncestor(ref, ast.NodeKindField)

	if len(w.document.Fields[ref].Arguments.Refs) != 0 {
		w.walkArguments(w.document.Fields[ref].Arguments.Refs, info)
	}

	if w.document.Fields[ref].HasSelections {
		w.walkSelectionSet(w.document.Fields[ref].SelectionSet, info.FieldTypeDefinition)
	}

	w.visitor.LeaveField(ref, info)
	w.removeLastAncestor()
	w.decreaseDepth()
}

func (w *Walker) fieldTypeDefinition(ref int, enclosingTypeDefinition ast.Node) ast.Node {

	fieldName := w.document.Input.ByteSlice(w.document.Fields[ref].Name)
	fieldDefinitions := w.definition.NodeFieldDefinitions(enclosingTypeDefinition)
	for _, i := range fieldDefinitions {
		if bytes.Equal(w.definition.Input.ByteSlice(w.definition.FieldDefinitions[i].Name), fieldName) {
			typeName := w.definition.ResolveTypeName(w.definition.FieldDefinitions[i].Type)
			node, exists := w.definition.Index.Nodes[string(typeName)]
			if !exists {
				w.err = fmt.Errorf("node not found in index for key: %s", string(typeName))
			}
			return node
		}
	}

	//typeName := w.definition.NodeTypeNameString(enclosingTypeDefinition)
	//w.err = fmt.Errorf("field definition not found for field: %s on type: %s", string(fieldName), typeName)
	return ast.Node{}
}

func (w *Walker) walkArguments(refs []int, enclosing Info) {

	info := Info{
		Depth:                   w.depth,
		Ancestors:               w.ancestors,
		SelectionSet:            -1,
		FieldTypeDefinition:     enclosing.FieldTypeDefinition,
		EnclosingTypeDefinition: enclosing.EnclosingTypeDefinition,
	}

	for i, j := range refs {
		info.ArgumentsBefore = refs[:i]
		info.ArgumentsAfter = refs[i+1:]
		w.walkArgument(j, info)
	}
}

func (w *Walker) walkArgument(ref int, enclosing Info) {
	w.increaseDepth()

	info := Info{
		Depth:                   w.depth,
		Ancestors:               w.ancestors,
		SelectionSet:            -1,
		ArgumentsBefore:         enclosing.ArgumentsBefore,
		ArgumentsAfter:          enclosing.ArgumentsAfter,
		FieldTypeDefinition:     enclosing.FieldTypeDefinition,
		EnclosingTypeDefinition: enclosing.EnclosingTypeDefinition,
	}

	definition := w.argumentDefinition(ref, enclosing)

	w.visitor.EnterArgument(ref, definition, info)
	w.visitor.LeaveArgument(ref, definition, info)

	w.decreaseDepth()
}

func (w *Walker) argumentDefinition(argument int, enclosing Info) int {
	ancestor := w.ancestors[len(w.ancestors)-1]
	switch ancestor.Kind {
	case ast.NodeKindField:
		fieldName := w.document.FieldName(ancestor.Ref)
		argName := w.document.ArgumentName(argument)
		return w.definition.NodeFieldDefinitionArgumentDefinitionByName(enclosing.EnclosingTypeDefinition, fieldName, argName)
	default:
		return -1
	}
}

func (w *Walker) walkFragmentSpread(ref int, enclosing Info) {
	w.increaseDepth()

	info := Info{
		Depth:                   w.depth,
		Ancestors:               w.ancestors,
		SelectionSet:            enclosing.SelectionSet,
		SelectionsBefore:        enclosing.SelectionsBefore,
		SelectionsAfter:         enclosing.SelectionsAfter,
		HasSelections:           false,
		EnclosingTypeDefinition: enclosing.EnclosingTypeDefinition,
	}
	w.visitor.EnterFragmentSpread(ref, info)
	// no need to append self to ancestors because we're not traversing any deeper
	w.visitor.LeaveFragmentSpread(ref, info)
	w.decreaseDepth()
}

func (w *Walker) walkInlineFragment(ref int, enclosing Info) {
	w.increaseDepth()
	info := Info{
		Depth:                   w.depth,
		Ancestors:               w.ancestors,
		SelectionSet:            enclosing.SelectionSet,
		SelectionsBefore:        enclosing.SelectionsBefore,
		SelectionsAfter:         enclosing.SelectionsAfter,
		HasSelections:           w.document.InlineFragments[ref].HasSelections,
		EnclosingTypeDefinition: enclosing.EnclosingTypeDefinition,
	}
	w.visitor.EnterInlineFragment(ref, info)

	w.appendAncestor(ref, ast.NodeKindInlineFragment)

	if w.document.InlineFragments[ref].HasSelections {
		inlineFragmentTypeDefinition := w.inlineFragmentTypeDefinition(ref, enclosing.EnclosingTypeDefinition)
		w.walkSelectionSet(w.document.InlineFragments[ref].SelectionSet, inlineFragmentTypeDefinition)
	}

	w.visitor.LeaveInlineFragment(ref, info)
	w.removeLastAncestor()
	w.decreaseDepth()
}

func (w *Walker) inlineFragmentTypeDefinition(ref int, enclosingTypeDefinition ast.Node) ast.Node {
	typeRef := w.document.InlineFragments[ref].TypeCondition.Type
	if typeRef == -1 {
		return enclosingTypeDefinition
	}
	typeCondition := w.document.Types[w.document.InlineFragments[ref].TypeCondition.Type]
	return w.definition.Index.Nodes[string(w.document.Input.ByteSlice(typeCondition.Name))]
}

func (w *Walker) walkFragmentDefinition(ref int, isLastRootNode bool) {
	w.increaseDepth()
	fragmentDefinitionTypeDefinition := w.fragmentDefinitionTypeDefinition(ref)
	info := Info{
		Depth:                   w.depth,
		Ancestors:               nil,
		SelectionSet:            -1,
		SelectionsBefore:        nil,
		SelectionsAfter:         nil,
		HasSelections:           w.document.FragmentDefinitions[ref].HasSelections,
		EnclosingTypeDefinition: fragmentDefinitionTypeDefinition,
		IsLastRootNode:          isLastRootNode,
	}
	w.visitor.EnterFragmentDefinition(ref, info)

	w.appendAncestor(ref, ast.NodeKindFragmentDefinition)

	if w.document.FragmentDefinitions[ref].HasSelections {
		w.walkSelectionSet(w.document.FragmentDefinitions[ref].SelectionSet, fragmentDefinitionTypeDefinition)
	}

	w.visitor.LeaveFragmentDefinition(ref, info)
	w.removeLastAncestor()
	w.decreaseDepth()
}

func (w *Walker) fragmentDefinitionTypeDefinition(ref int) ast.Node {
	typeRef := w.document.FragmentDefinitions[ref].TypeCondition.Type
	typeNameRef := w.document.Types[typeRef].Name
	typeName := w.document.Input.ByteSlice(typeNameRef)
	return w.definition.Index.Nodes[string(typeName)]
}
