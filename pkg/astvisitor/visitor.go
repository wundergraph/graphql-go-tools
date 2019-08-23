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
	visitors         visitors
	depth            int
	ancestors        []ast.Node
	parentDefinition ast.Node
	stop             bool
}

func NewWalker(ancestorSize int) Walker {
	return Walker{
		ancestors: make([]ast.Node, 0, ancestorSize),
	}
}

type Info struct {
	Depth                     int
	Ancestors                 []ast.Node
	SelectionSet              int
	SelectionsBefore          []int
	SelectionsAfter           []int
	ArgumentsBefore           []int
	ArgumentsAfter            []int
	VariableDefinitionsBefore []int
	VariableDefinitionsAfter  []int
	DirectivesBefore          []int
	DirectivesAfter           []int
	InputValueDefinitions     []int
	HasSelections             bool
	FieldTypeDefinition       ast.Node
	EnclosingTypeDefinition   ast.Node
	InputValueDefinition      int
	IsLastRootNode            bool
}

type Action int

const (
	NoOp Action = iota
	RevisitCurrentNode
	Stop
	StopWithError
)

type Instruction struct {
	Action  Action
	Message string
}

type (
	EnterOperationDefinitionVisitor interface {
		EnterOperationDefinition(ref int, info Info) Instruction
	}
	LeaveOperationDefinitionVisitor interface {
		LeaveOperationDefinition(ref int, info Info) Instruction
	}
	OperationDefinitionVisitor interface {
		EnterOperationDefinitionVisitor
		LeaveOperationDefinitionVisitor
	}
	EnterSelectionSetVisitor interface {
		EnterSelectionSet(ref int, info Info) Instruction
	}
	LeaveSelectionSetVisitor interface {
		LeaveSelectionSet(ref int, info Info) Instruction
	}
	SelectionSetVisitor interface {
		EnterSelectionSetVisitor
		LeaveSelectionSetVisitor
	}
	EnterFieldVisitor interface {
		EnterField(ref int, info Info) Instruction
	}
	LeaveFieldVisitor interface {
		LeaveField(ref int, info Info) Instruction
	}
	FieldVisitor interface {
		EnterFieldVisitor
		LeaveFieldVisitor
	}
	EnterArgumentVisitor interface {
		EnterArgument(ref int, info Info) Instruction
	}
	LeaveArgumentVisitor interface {
		LeaveArgument(ref int, info Info) Instruction
	}
	ArgumentVisitor interface {
		EnterArgumentVisitor
		LeaveArgumentVisitor
	}
	EnterFragmentSpreadVisitor interface {
		EnterFragmentSpread(ref int, info Info) Instruction
	}
	LeaveFragmentSpreadVisitor interface {
		LeaveFragmentSpread(ref int, info Info) Instruction
	}
	FragmentSpreadVisitor interface {
		EnterFragmentSpreadVisitor
		LeaveFragmentSpreadVisitor
	}
	EnterInlineFragmentVisitor interface {
		EnterInlineFragment(ref int, info Info) Instruction
	}
	LeaveInlineFragmentVisitor interface {
		LeaveInlineFragment(ref int, info Info) Instruction
	}
	InlineFragmentVisitor interface {
		EnterInlineFragmentVisitor
		LeaveInlineFragmentVisitor
	}
	EnterFragmentDefinitionVisitor interface {
		EnterFragmentDefinition(ref int, info Info) Instruction
	}
	LeaveFragmentDefinitionVisitor interface {
		LeaveFragmentDefinition(ref int, info Info) Instruction
	}
	FragmentDefinitionVisitor interface {
		EnterFragmentDefinitionVisitor
		LeaveFragmentDefinitionVisitor
	}
	EnterVariableDefinitionVisitor interface {
		EnterVariableDefinition(ref int, info Info) Instruction
	}
	LeaveVariableDefinitionVisitor interface {
		LeaveVariableDefinition(ref int, info Info) Instruction
	}
	VariableDefinitionVisitor interface {
		EnterVariableDefinitionVisitor
		LeaveVariableDefinitionVisitor
	}
	EnterDirectiveVisitor interface {
		EnterDirective(ref int, info Info) Instruction
	}
	LeaveDirectiveVisitor interface {
		LeaveDirective(ref int, info Info) Instruction
	}
	DirectiveVisitor interface {
		EnterDirectiveVisitor
		LeaveDirectiveVisitor
	}
	AllNodesVisitor interface {
		OperationDefinitionVisitor
		SelectionSetVisitor
		FieldVisitor
		ArgumentVisitor
		FragmentSpreadVisitor
		InlineFragmentVisitor
		FragmentDefinitionVisitor
		VariableDefinitionVisitor
		DirectiveVisitor
	}
	EnterDocumentVisitor interface {
		EnterDocument(operation, definition *ast.Document) Instruction
	}
	LeaveDocumentVisitor interface {
		LeaveDocument(operation, definition *ast.Document) Instruction
	}
	DocumentVisitor interface {
		EnterDocumentVisitor
		LeaveDocumentVisitor
	}
)

type visitors struct {
	enterOperation          []EnterOperationDefinitionVisitor
	leaveOperation          []LeaveOperationDefinitionVisitor
	enterSelectionSet       []EnterSelectionSetVisitor
	leaveSelectionSet       []LeaveSelectionSetVisitor
	enterField              []EnterFieldVisitor
	leaveField              []LeaveFieldVisitor
	enterArgument           []EnterArgumentVisitor
	leaveArgument           []LeaveArgumentVisitor
	enterFragmentSpread     []EnterFragmentSpreadVisitor
	leaveFragmentSpread     []LeaveFragmentSpreadVisitor
	enterInlineFragment     []EnterInlineFragmentVisitor
	leaveInlineFragment     []LeaveInlineFragmentVisitor
	enterFragmentDefinition []EnterFragmentDefinitionVisitor
	leaveFragmentDefinition []LeaveFragmentDefinitionVisitor
	enterDocument           []EnterDocumentVisitor
	leaveDocument           []LeaveDocumentVisitor
	enterVariableDefinition []EnterVariableDefinitionVisitor
	leaveVariableDefinition []LeaveVariableDefinitionVisitor
	enterDirective          []EnterDirectiveVisitor
	leaveDirective          []LeaveDirectiveVisitor
}

func (w *Walker) ResetVisitors() {
	w.visitors.enterOperation = w.visitors.enterOperation[:0]
	w.visitors.leaveOperation = w.visitors.leaveOperation[:0]
	w.visitors.enterSelectionSet = w.visitors.enterSelectionSet[:0]
	w.visitors.leaveSelectionSet = w.visitors.leaveSelectionSet[:0]
	w.visitors.enterField = w.visitors.enterField[:0]
	w.visitors.leaveField = w.visitors.leaveField[:0]
	w.visitors.enterArgument = w.visitors.enterArgument[:0]
	w.visitors.leaveArgument = w.visitors.leaveArgument[:0]
	w.visitors.enterFragmentSpread = w.visitors.enterFragmentSpread[:0]
	w.visitors.leaveFragmentSpread = w.visitors.leaveFragmentSpread[:0]
	w.visitors.enterInlineFragment = w.visitors.enterInlineFragment[:0]
	w.visitors.leaveInlineFragment = w.visitors.leaveInlineFragment[:0]
	w.visitors.enterFragmentDefinition = w.visitors.enterFragmentDefinition[:0]
	w.visitors.leaveFragmentDefinition = w.visitors.leaveFragmentDefinition[:0]
	w.visitors.enterDocument = w.visitors.enterDocument[:0]
	w.visitors.leaveDocument = w.visitors.leaveDocument[:0]
	w.visitors.enterVariableDefinition = w.visitors.enterVariableDefinition[:0]
	w.visitors.leaveVariableDefinition = w.visitors.leaveVariableDefinition[:0]
	w.visitors.enterDirective = w.visitors.enterDirective[:0]
	w.visitors.leaveDirective = w.visitors.leaveDirective[:0]
}

func (w *Walker) RegisterEnterFieldVisitor(visitor EnterFieldVisitor) {
	w.visitors.enterField = append(w.visitors.enterField, visitor)
}

func (w *Walker) RegisterLeaveFieldVisitor(visitor LeaveFieldVisitor) {
	w.visitors.leaveField = append(w.visitors.leaveField, visitor)
}

func (w *Walker) RegisterFieldVisitor(visitor FieldVisitor) {
	w.RegisterEnterFieldVisitor(visitor)
	w.RegisterLeaveFieldVisitor(visitor)
}

func (w *Walker) RegisterEnterSelectionSetVisitor(visitor EnterSelectionSetVisitor) {
	w.visitors.enterSelectionSet = append(w.visitors.enterSelectionSet, visitor)
}

func (w *Walker) RegisterLeaveSelectionSetVisitor(visitor LeaveSelectionSetVisitor) {
	w.visitors.leaveSelectionSet = append(w.visitors.leaveSelectionSet, visitor)
}

func (w *Walker) RegisterSelectionSetVisitor(visitor SelectionSetVisitor) {
	w.RegisterEnterSelectionSetVisitor(visitor)
	w.RegisterLeaveSelectionSetVisitor(visitor)
}

func (w *Walker) RegisterEnterArgumentVisitor(visitor EnterArgumentVisitor) {
	w.visitors.enterArgument = append(w.visitors.enterArgument, visitor)
}

func (w *Walker) RegisterLeaveArgumentVisitor(visitor LeaveArgumentVisitor) {
	w.visitors.leaveArgument = append(w.visitors.leaveArgument, visitor)
}

func (w *Walker) RegisterArgumentVisitor(visitor ArgumentVisitor) {
	w.RegisterEnterArgumentVisitor(visitor)
	w.RegisterLeaveArgumentVisitor(visitor)
}

func (w *Walker) RegisterEnterFragmentSpreadVisitor(visitor EnterFragmentSpreadVisitor) {
	w.visitors.enterFragmentSpread = append(w.visitors.enterFragmentSpread, visitor)
}

func (w *Walker) RegisterLeaveFragmentSpreadVisitor(visitor LeaveFragmentSpreadVisitor) {
	w.visitors.leaveFragmentSpread = append(w.visitors.leaveFragmentSpread, visitor)
}

func (w *Walker) RegisterFragmentSpreadVisitor(visitor FragmentSpreadVisitor) {
	w.RegisterEnterFragmentSpreadVisitor(visitor)
	w.RegisterLeaveFragmentSpreadVisitor(visitor)
}

func (w *Walker) RegisterEnterInlineFragmentVisitor(visitor EnterInlineFragmentVisitor) {
	w.visitors.enterInlineFragment = append(w.visitors.enterInlineFragment, visitor)
}

func (w *Walker) RegisterLeaveInlineFragmentVisitor(visitor LeaveInlineFragmentVisitor) {
	w.visitors.leaveInlineFragment = append(w.visitors.leaveInlineFragment, visitor)
}

func (w *Walker) RegisterInlineFragmentVisitor(visitor InlineFragmentVisitor) {
	w.RegisterEnterInlineFragmentVisitor(visitor)
	w.RegisterLeaveInlineFragmentVisitor(visitor)
}

func (w *Walker) RegisterEnterFragmentDefinitionVisitor(visitor EnterFragmentDefinitionVisitor) {
	w.visitors.enterFragmentDefinition = append(w.visitors.enterFragmentDefinition, visitor)
}

func (w *Walker) RegisterLeaveFragmentDefinitionVisitor(visitor LeaveFragmentDefinitionVisitor) {
	w.visitors.leaveFragmentDefinition = append(w.visitors.leaveFragmentDefinition, visitor)
}

func (w *Walker) RegisterFragmentDefinitionVisitor(visitor FragmentDefinitionVisitor) {
	w.RegisterEnterFragmentDefinitionVisitor(visitor)
	w.RegisterLeaveFragmentDefinitionVisitor(visitor)
}

func (w *Walker) RegisterEnterVariableDefinitionVisitor(visitor EnterVariableDefinitionVisitor) {
	w.visitors.enterVariableDefinition = append(w.visitors.enterVariableDefinition, visitor)
}

func (w *Walker) RegisterLeaveVariableDefinitionVisitor(visitor LeaveVariableDefinitionVisitor) {
	w.visitors.leaveVariableDefinition = append(w.visitors.leaveVariableDefinition, visitor)
}

func (w *Walker) RegisterVariableDefinitionVisitor(visitor VariableDefinitionVisitor) {
	w.RegisterEnterVariableDefinitionVisitor(visitor)
	w.RegisterLeaveVariableDefinitionVisitor(visitor)
}

func (w *Walker) RegisterEnterOperationVisitor(visitor EnterOperationDefinitionVisitor) {
	w.visitors.enterOperation = append(w.visitors.enterOperation, visitor)
}

func (w *Walker) RegisterLeaveOperationVisitor(visitor LeaveOperationDefinitionVisitor) {
	w.visitors.leaveOperation = append(w.visitors.leaveOperation, visitor)
}

func (w *Walker) RegisterOperationVisitor(visitor OperationDefinitionVisitor) {
	w.RegisterEnterOperationVisitor(visitor)
	w.RegisterLeaveOperationVisitor(visitor)
}

func (w *Walker) RegisterEnterDirectiveVisitor(visitor EnterDirectiveVisitor) {
	w.visitors.enterDirective = append(w.visitors.enterDirective, visitor)
}

func (w *Walker) RegisterLeaveDirectiveVisitor(visitor LeaveDirectiveVisitor) {
	w.visitors.leaveDirective = append(w.visitors.leaveDirective, visitor)
}

func (w *Walker) RegisterDirectiveVisitor(visitor DirectiveVisitor) {
	w.RegisterEnterDirectiveVisitor(visitor)
	w.RegisterLeaveDirectiveVisitor(visitor)
}

func (w *Walker) RegisterAllNodesVisitor(visitor AllNodesVisitor) {
	w.RegisterOperationVisitor(visitor)
	w.RegisterSelectionSetVisitor(visitor)
	w.RegisterFieldVisitor(visitor)
	w.RegisterArgumentVisitor(visitor)
	w.RegisterFragmentSpreadVisitor(visitor)
	w.RegisterInlineFragmentVisitor(visitor)
	w.RegisterFragmentDefinitionVisitor(visitor)
	w.RegisterVariableDefinitionVisitor(visitor)
	w.RegisterDirectiveVisitor(visitor)
}

func (w *Walker) RegisterEnterDocumentVisitor(visitor EnterDocumentVisitor) {
	w.visitors.enterDocument = append(w.visitors.enterDocument, visitor)
}

func (w *Walker) RegisterLeaveDocumentVisitor(visitor LeaveDocumentVisitor) {
	w.visitors.leaveDocument = append(w.visitors.leaveDocument, visitor)
}

func (w *Walker) RegisterDocumentVisitor(visitor DocumentVisitor) {
	w.RegisterEnterDocumentVisitor(visitor)
	w.RegisterLeaveDocumentVisitor(visitor)
}

func (w *Walker) Walk(document, definition *ast.Document) error {
	w.err = nil
	w.ancestors = w.ancestors[:0]
	w.document = document
	w.definition = definition
	w.depth = 0
	w.stop = false
	w.walk()
	return w.err
}

func (w *Walker) setImmutableErr(err error) {
	if w.err != nil {
		return
	}
	w.err = err
}

func (w *Walker) walk() {

	if w.document == nil {
		w.err = fmt.Errorf("document must not be nil")
		return
	}

	for i := range w.visitors.enterDocument {
		w.enterDocument(i)
		if w.stop {
			return
		}
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

		if w.stop {
			return
		}
	}

	for i := range w.visitors.leaveDocument {
		w.leaveDocument(i)
		if w.stop {
			return
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

	for i := range w.visitors.enterOperation {
		w.enterOperationDefinition(i, ref, info)
		if w.stop {
			return
		}
	}

	w.appendAncestor(ref, ast.NodeKindOperationDefinition)

	if w.document.OperationDefinitions[ref].HasVariableDefinitions {
		w.walkVariableDefinitions(w.document.OperationDefinitions[ref].VariableDefinitions.Refs, info)
		if w.stop {
			return
		}
	}

	if w.document.OperationDefinitions[ref].HasDirectives {
		w.walkDirectives(w.document.OperationDefinitions[ref].Directives.Refs, info)
		if w.stop {
			return
		}
	}

	if w.document.OperationDefinitions[ref].HasSelections {
		w.walkSelectionSet(w.document.OperationDefinitions[ref].SelectionSet, info.EnclosingTypeDefinition)
		if w.stop {
			return
		}
	}

	w.removeLastAncestor()

	for i := range w.visitors.leaveOperation {
		w.leaveOperationDefinition(i, ref, info)
		if w.stop {
			return
		}
	}

	w.decreaseDepth()
}

func (w *Walker) walkVariableDefinitions(refs []int, enclosed Info) {
	info := Info{
		Depth:                   w.depth,
		Ancestors:               w.ancestors,
		SelectionSet:            -1,
		SelectionsBefore:        nil,
		SelectionsAfter:         nil,
		ArgumentsBefore:         nil,
		ArgumentsAfter:          nil,
		HasSelections:           false,
		FieldTypeDefinition:     enclosed.FieldTypeDefinition,
		EnclosingTypeDefinition: enclosed.EnclosingTypeDefinition,
		IsLastRootNode:          false,
	}

	for _, i := range refs {
		info.VariableDefinitionsBefore = refs[:i]
		info.VariableDefinitionsAfter = refs[i+1:]
		w.walkVariableDefinition(i, info)
	}
}

func (w *Walker) walkVariableDefinition(ref int, enclosing Info) {
	w.increaseDepth()

	info := Info{
		Depth:                     w.depth,
		Ancestors:                 w.ancestors,
		SelectionSet:              -1,
		VariableDefinitionsBefore: enclosing.VariableDefinitionsBefore,
		VariableDefinitionsAfter:  enclosing.VariableDefinitionsAfter,
		FieldTypeDefinition:       enclosing.FieldTypeDefinition,
		EnclosingTypeDefinition:   enclosing.EnclosingTypeDefinition,
	}

	for i := range w.visitors.enterVariableDefinition {
		w.enterVariableDefinition(i, ref, info)
		if w.stop {
			return
		}
	}

	w.appendAncestor(ref, ast.NodeKindVariableDefinition)

	if w.document.VariableDefinitions[ref].HasDirectives {
		w.walkDirectives(w.document.VariableDefinitions[ref].Directives.Refs, info)
		if w.stop {
			return
		}
	}

	w.removeLastAncestor()

	for i := range w.visitors.leaveVariableDefinition {
		w.leaveVariableDefinition(i, ref, info)
		if w.stop {
			return
		}
	}

	w.decreaseDepth()
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

	for i := range w.visitors.enterSelectionSet {
		w.enterSelectionSet(i, ref, info)
		if w.stop {
			return
		}
	}

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

		if w.stop {
			return
		}
	}

	info.SelectionsBefore = nil
	info.SelectionsAfter = nil

	w.removeLastAncestor()

	for i := range w.visitors.leaveSelectionSet {
		w.leaveSelectionSet(i, ref, info)
		if w.stop {
			return
		}
	}

	w.decreaseDepth()
}

func (w *Walker) walkField(ref int, enclosing Info) {
	w.increaseDepth()

	argumentDefinitions := w.definition.NodeFieldDefinitionArgumentsDefinitions(enclosing.EnclosingTypeDefinition, w.document.FieldName(ref))

	info := Info{
		Depth:                   w.depth,
		Ancestors:               w.ancestors,
		SelectionSet:            enclosing.SelectionSet,
		SelectionsBefore:        enclosing.SelectionsBefore,
		SelectionsAfter:         enclosing.SelectionsAfter,
		HasSelections:           w.document.Fields[ref].HasSelections,
		FieldTypeDefinition:     w.fieldTypeDefinition(ref, enclosing.EnclosingTypeDefinition),
		InputValueDefinitions:   argumentDefinitions,
		EnclosingTypeDefinition: enclosing.EnclosingTypeDefinition,
	}

	for i := range w.visitors.enterField {
		w.enterField(i, ref, info)
		if w.stop {
			return
		}
	}

	w.appendAncestor(ref, ast.NodeKindField)

	if len(w.document.Fields[ref].Arguments.Refs) != 0 {
		w.walkArguments(w.document.Fields[ref].Arguments.Refs, info)
	}

	if w.document.Fields[ref].HasDirectives {
		w.walkDirectives(w.document.Fields[ref].Directives.Refs, info)
	}

	if w.document.Fields[ref].HasSelections {
		w.walkSelectionSet(w.document.Fields[ref].SelectionSet, info.FieldTypeDefinition)
	}

	w.removeLastAncestor()

	for i := range w.visitors.leaveField {
		w.leaveField(i, ref, info)
		if w.stop {
			return
		}
	}

	w.decreaseDepth()
}

func (w *Walker) walkDirectives(refs []int, enclosing Info) {

	for i, j := range refs {
		enclosing.DirectivesBefore = refs[:i]
		enclosing.DirectivesAfter = refs[i+1:]
		w.walkDirective(j, enclosing)
		if w.stop {
			return
		}
	}
}

func (w *Walker) walkDirective(ref int, enclosing Info) {
	w.increaseDepth()

	info := Info{
		Depth:                   w.depth,
		Ancestors:               w.ancestors,
		SelectionSet:            enclosing.SelectionSet,
		DirectivesBefore:        enclosing.DirectivesBefore,
		DirectivesAfter:         enclosing.DirectivesAfter,
		FieldTypeDefinition:     enclosing.FieldTypeDefinition,
		EnclosingTypeDefinition: enclosing.EnclosingTypeDefinition,
	}

	for i := range w.visitors.enterDirective {
		w.enterDirective(i, ref, info)
		if w.stop {
			return
		}
	}

	w.appendAncestor(ref, ast.NodeKindDirective)

	if w.document.Directives[ref].HasArguments {
		w.walkArguments(w.document.Directives[ref].Arguments.Refs, info)
	}

	w.removeLastAncestor()

	for i := range w.visitors.leaveDirective {
		w.leaveDirective(i, ref, info)
		if w.stop {
			return
		}
	}

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
		if w.stop {
			return
		}
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
		InputValueDefinition:    w.inputValueDefinition(ref, enclosing),
	}

	for i := range w.visitors.enterArgument {
		w.enterArgument(i, ref, info)
		if w.stop {
			return
		}
	}

	for i := range w.visitors.leaveArgument {
		w.leaveArgument(i, ref, info)
		if w.stop {
			return
		}
	}

	w.decreaseDepth()
}

func (w *Walker) inputValueDefinition(argument int, enclosing Info) int {
	ancestor := w.ancestors[len(w.ancestors)-1]
	argName := w.document.ArgumentName(argument)
	switch ancestor.Kind {
	case ast.NodeKindField:
		fieldName := w.document.FieldName(ancestor.Ref)
		return w.definition.NodeFieldDefinitionArgumentDefinitionByName(enclosing.EnclosingTypeDefinition, fieldName, argName)
	case ast.NodeKindDirective:
		directiveName := w.document.DirectiveNameBytes(ancestor.Ref)
		return w.definition.DirectiveArgumentInputValueDefinition(directiveName, argName)
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

	for i := range w.visitors.enterFragmentSpread {
		w.enterFragmentSpread(i, ref, info)
		if w.stop {
			return
		}
	}

	for i := range w.visitors.leaveFragmentSpread {
		w.leaveFragmentSpread(i, ref, info)
		if w.stop {
			return
		}
	}

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

	for i := range w.visitors.enterInlineFragment {
		w.enterInlineFragment(i, ref, info)
		if w.stop {
			return
		}
	}

	w.appendAncestor(ref, ast.NodeKindInlineFragment)

	if w.document.InlineFragments[ref].HasSelections {
		inlineFragmentTypeDefinition := w.inlineFragmentTypeDefinition(ref, enclosing.EnclosingTypeDefinition)
		w.walkSelectionSet(w.document.InlineFragments[ref].SelectionSet, inlineFragmentTypeDefinition)
	}

	w.removeLastAncestor()

	for i := range w.visitors.leaveInlineFragment {
		w.leaveInlineFragment(i, ref, info)
		if w.stop {
			return
		}
	}

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

	for i := range w.visitors.enterFragmentDefinition {
		w.enterFragmentDefinition(i, ref, info)
		if w.stop {
			return
		}
	}

	w.appendAncestor(ref, ast.NodeKindFragmentDefinition)

	if w.document.FragmentDefinitions[ref].HasSelections {
		w.walkSelectionSet(w.document.FragmentDefinitions[ref].SelectionSet, fragmentDefinitionTypeDefinition)
	}

	w.removeLastAncestor()

	for i := range w.visitors.leaveFragmentDefinition {
		w.leaveFragmentDefinition(i, ref, info)
		if w.stop {
			return
		}
	}

	w.decreaseDepth()
}

func (w *Walker) fragmentDefinitionTypeDefinition(ref int) ast.Node {
	typeRef := w.document.FragmentDefinitions[ref].TypeCondition.Type
	typeNameRef := w.document.Types[typeRef].Name
	typeName := w.document.Input.ByteSlice(typeNameRef)
	return w.definition.Index.Nodes[string(typeName)]
}

func (w *Walker) handleInstruction(instruction Instruction) (retry bool) {
	switch instruction.Action {
	case RevisitCurrentNode:
		return true
	case StopWithError:
		w.stop = true
		w.setImmutableErr(fmt.Errorf(instruction.Message))
		return false
	case Stop:
		w.stop = true
		return false
	default:
		return false
	}
}

func (w *Walker) enterDirective(visitor, ref int, info Info) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.enterDirective[visitor].EnterDirective(ref, info))
	}
}

func (w *Walker) leaveDirective(visitor, ref int, info Info) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.leaveDirective[visitor].LeaveDirective(ref, info))
	}
}

func (w *Walker) enterVariableDefinition(visitor, ref int, info Info) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.enterVariableDefinition[visitor].EnterVariableDefinition(ref, info))
	}
}

func (w *Walker) leaveVariableDefinition(visitor, ref int, info Info) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.leaveVariableDefinition[visitor].LeaveVariableDefinition(ref, info))
	}
}

func (w *Walker) enterDocument(visitor int) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.enterDocument[visitor].EnterDocument(w.document, w.definition))
	}
}

func (w *Walker) leaveDocument(visitor int) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.leaveDocument[visitor].LeaveDocument(w.document, w.definition))
	}
}

func (w *Walker) enterOperationDefinition(visitor, ref int, info Info) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.enterOperation[visitor].EnterOperationDefinition(ref, info))
	}
}

func (w *Walker) leaveOperationDefinition(visitor, ref int, info Info) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.leaveOperation[visitor].LeaveOperationDefinition(ref, info))
	}
}

func (w *Walker) enterSelectionSet(visitor, ref int, info Info) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.enterSelectionSet[visitor].EnterSelectionSet(ref, info))
	}
}

func (w *Walker) leaveSelectionSet(visitor, ref int, info Info) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.leaveSelectionSet[visitor].LeaveSelectionSet(ref, info))
	}
}

func (w *Walker) enterField(visitor, ref int, info Info) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.enterField[visitor].EnterField(ref, info))
	}
}

func (w *Walker) leaveField(visitor, ref int, info Info) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.leaveField[visitor].LeaveField(ref, info))
	}
}

func (w *Walker) enterArgument(visitor, ref int, info Info) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.enterArgument[visitor].EnterArgument(ref, info))
	}
}

func (w *Walker) leaveArgument(visitor, ref int, info Info) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.leaveArgument[visitor].LeaveArgument(ref, info))
	}
}

func (w *Walker) enterFragmentSpread(visitor, ref int, info Info) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.enterFragmentSpread[visitor].EnterFragmentSpread(ref, info))
	}
}

func (w *Walker) leaveFragmentSpread(visitor, ref int, info Info) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.leaveFragmentSpread[visitor].LeaveFragmentSpread(ref, info))
	}
}

func (w *Walker) enterInlineFragment(visitor, ref int, info Info) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.enterInlineFragment[visitor].EnterInlineFragment(ref, info))
	}
}

func (w *Walker) leaveInlineFragment(visitor, ref int, info Info) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.leaveInlineFragment[visitor].LeaveInlineFragment(ref, info))
	}
}

func (w *Walker) enterFragmentDefinition(visitor, ref int, info Info) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.enterFragmentDefinition[visitor].EnterFragmentDefinition(ref, info))
	}
}

func (w *Walker) leaveFragmentDefinition(visitor, ref int, info Info) {
	for retry := true; retry; {
		retry = w.handleInstruction(w.visitors.leaveFragmentDefinition[visitor].LeaveFragmentDefinition(ref, info))
	}
}
