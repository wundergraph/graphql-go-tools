package fastastvisitor

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
)

type Walker struct {
	Ancestors               []ast.Node
	EnclosingTypeDefinition ast.Node
	SelectionsBefore        []int
	SelectionsAfter         []int
	err                     error
	document                *ast.Document
	definition              *ast.Document
	visitors                visitors
	depth                   int
	typeDefinitions         []ast.Node
	stop                    bool
	skip                    bool
	revisit                 bool
}

func NewWalker(ancestorSize int) Walker {
	return Walker{
		Ancestors:       make([]ast.Node, 0, ancestorSize),
		typeDefinitions: make([]ast.Node, 0, ancestorSize),
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
	IsLastRootNode            bool
	Definition                ast.Node
}

type Action int

const (
	NoOp Action = iota
	RevisitCurrentNode
	Stop
	StopWithError
	Skip
)

type Instruction struct {
	Action  Action
	Message string
}

type (
	EnterOperationDefinitionVisitor interface {
		EnterOperationDefinition(ref int)
	}
	LeaveOperationDefinitionVisitor interface {
		LeaveOperationDefinition(ref int)
	}
	OperationDefinitionVisitor interface {
		EnterOperationDefinitionVisitor
		LeaveOperationDefinitionVisitor
	}
	EnterSelectionSetVisitor interface {
		EnterSelectionSet(ref int)
	}
	LeaveSelectionSetVisitor interface {
		LeaveSelectionSet(ref int)
	}
	SelectionSetVisitor interface {
		EnterSelectionSetVisitor
		LeaveSelectionSetVisitor
	}
	EnterFieldVisitor interface {
		EnterField(ref int)
	}
	LeaveFieldVisitor interface {
		LeaveField(ref int)
	}
	FieldVisitor interface {
		EnterFieldVisitor
		LeaveFieldVisitor
	}
	EnterArgumentVisitor interface {
		EnterArgument(ref int)
	}
	LeaveArgumentVisitor interface {
		LeaveArgument(ref int)
	}
	ArgumentVisitor interface {
		EnterArgumentVisitor
		LeaveArgumentVisitor
	}
	EnterFragmentSpreadVisitor interface {
		EnterFragmentSpread(ref int)
	}
	LeaveFragmentSpreadVisitor interface {
		LeaveFragmentSpread(ref int)
	}
	FragmentSpreadVisitor interface {
		EnterFragmentSpreadVisitor
		LeaveFragmentSpreadVisitor
	}
	EnterInlineFragmentVisitor interface {
		EnterInlineFragment(ref int)
	}
	LeaveInlineFragmentVisitor interface {
		LeaveInlineFragment(ref int)
	}
	InlineFragmentVisitor interface {
		EnterInlineFragmentVisitor
		LeaveInlineFragmentVisitor
	}
	EnterFragmentDefinitionVisitor interface {
		EnterFragmentDefinition(ref int)
	}
	LeaveFragmentDefinitionVisitor interface {
		LeaveFragmentDefinition(ref int)
	}
	FragmentDefinitionVisitor interface {
		EnterFragmentDefinitionVisitor
		LeaveFragmentDefinitionVisitor
	}
	EnterVariableDefinitionVisitor interface {
		EnterVariableDefinition(ref int)
	}
	LeaveVariableDefinitionVisitor interface {
		LeaveVariableDefinition(ref int)
	}
	VariableDefinitionVisitor interface {
		EnterVariableDefinitionVisitor
		LeaveVariableDefinitionVisitor
	}
	EnterDirectiveVisitor interface {
		EnterDirective(ref int)
	}
	LeaveDirectiveVisitor interface {
		LeaveDirective(ref int)
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
		DocumentVisitor
	}
	EnterDocumentVisitor interface {
		EnterDocument(operation, definition *ast.Document)
	}
	LeaveDocumentVisitor interface {
		LeaveDocument(operation, definition *ast.Document)
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
	w.Ancestors = w.Ancestors[:0]
	w.typeDefinitions = w.typeDefinitions[:0]
	w.document = document
	w.definition = definition
	w.depth = 0
	w.stop = false
	w.walk()
	return w.err
}

func (w *Walker) appendAncestor(ref int, kind ast.NodeKind) {

	w.Ancestors = append(w.Ancestors, ast.Node{
		Kind: kind,
		Ref:  ref,
	})

	var typeName []byte

	switch kind {
	case ast.NodeKindOperationDefinition:
		switch w.document.OperationDefinitions[ref].OperationType {
		case ast.OperationTypeQuery:
			typeName = w.definition.Index.QueryTypeName
		case ast.OperationTypeMutation:
			typeName = w.definition.Index.MutationTypeName
		case ast.OperationTypeSubscription:
			typeName = w.definition.Index.SubscriptionTypeName
		default:
			return
		}
	case ast.NodeKindInlineFragment:
		if !w.document.InlineFragmentHasTypeCondition(ref) {
			return
		}
		typeName = w.document.InlineFragmentTypeConditionName(ref)
	case ast.NodeKindFragmentDefinition:
		typeName = w.document.FragmentDefinitionTypeName(ref)
	case ast.NodeKindField:
		fieldName := w.document.FieldName(ref)
		if bytes.Equal(fieldName, literal.TYPENAME) {
			typeName = literal.STRING
		}
		fields := w.definition.NodeFieldDefinitions(w.typeDefinitions[len(w.typeDefinitions)-1])
		for _, i := range fields {
			if bytes.Equal(fieldName, w.definition.FieldDefinitionNameBytes(i)) {
				typeName = w.definition.ResolveTypeName(w.definition.FieldDefinitionType(i))
				break
			}
		}
		if typeName == nil {
			w.StopWithErr(fmt.Errorf("field: '%s' not defined on type: %s", string(fieldName), w.definition.NodeTypeNameString(w.typeDefinitions[len(w.typeDefinitions)-1])))
			return
		}
	default:
		return
	}

	var exists bool
	w.EnclosingTypeDefinition, exists = w.definition.Index.Nodes[string(typeName)]
	if !exists {
		w.StopWithErr(fmt.Errorf("type: '%s' not defined", string(typeName)))
		return
	}

	w.typeDefinitions = append(w.typeDefinitions, w.EnclosingTypeDefinition)
}

func (w *Walker) removeLastAncestor() {

	ancestor := w.Ancestors[len(w.Ancestors)-1]
	w.Ancestors = w.Ancestors[:len(w.Ancestors)-1]

	switch ancestor.Kind {
	case ast.NodeKindOperationDefinition, ast.NodeKindFragmentDefinition:
		w.typeDefinitions = w.typeDefinitions[:len(w.typeDefinitions)-1]
		w.EnclosingTypeDefinition.Kind = ast.NodeKindUnknown
		w.EnclosingTypeDefinition.Ref = -1
	case ast.NodeKindInlineFragment:
		if w.document.InlineFragmentHasTypeCondition(ancestor.Ref) {
			w.typeDefinitions = w.typeDefinitions[:len(w.typeDefinitions)-1]
			w.EnclosingTypeDefinition = w.typeDefinitions[len(w.typeDefinitions)-1]
		}
	case ast.NodeKindField:
		w.typeDefinitions = w.typeDefinitions[:len(w.typeDefinitions)-1]
		w.EnclosingTypeDefinition = w.typeDefinitions[len(w.typeDefinitions)-1]
	}
}

func (w *Walker) increaseDepth() {
	w.depth++
}

func (w *Walker) decreaseDepth() {
	w.depth--
}

func (w *Walker) walk() {

	if w.document == nil {
		w.err = fmt.Errorf("document must not be nil")
		return
	}

	for i := 0; i < len(w.visitors.enterDocument); {
		w.visitors.enterDocument[i].EnterDocument(w.document, w.definition)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			return
		}
		i++
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
			w.walkFragmentDefinition(ref)
		}

		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			return
		}
	}

	for i := 0; i < len(w.visitors.leaveDocument); {
		w.visitors.leaveDocument[i].LeaveDocument(w.document, w.definition)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			return
		}
		i++
	}
}

func (w *Walker) walkOperationDefinition(ref int, isLastRootNode bool) {
	w.increaseDepth()

	for i := 0; i < len(w.visitors.enterOperation); {
		w.visitors.enterOperation[i].EnterOperationDefinition(ref)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindOperationDefinition)

	if w.document.OperationDefinitions[ref].HasVariableDefinitions {
		for _, i := range w.document.OperationDefinitions[ref].VariableDefinitions.Refs {
			w.walkVariableDefinition(i)
			if w.stop {
				return
			}
		}
	}

	if w.document.OperationDefinitions[ref].HasDirectives {
		for _, i := range w.document.OperationDefinitions[ref].Directives.Refs {
			w.walkDirective(i)
			if w.stop {
				return
			}
		}
	}

	if w.document.OperationDefinitions[ref].HasSelections {
		w.walkSelectionSet(w.document.OperationDefinitions[ref].SelectionSet)
		if w.stop {
			return
		}
	}

	w.removeLastAncestor()

	for i := 0; i < len(w.visitors.leaveOperation); {
		w.visitors.leaveOperation[i].LeaveOperationDefinition(ref)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.decreaseDepth()
}

func (w *Walker) walkVariableDefinition(ref int) {
	w.increaseDepth()

	for i := 0; i < len(w.visitors.enterVariableDefinition); {
		w.visitors.enterVariableDefinition[i].EnterVariableDefinition(ref)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindVariableDefinition)

	if w.document.VariableDefinitions[ref].HasDirectives {
		for _, i := range w.document.VariableDefinitions[ref].Directives.Refs {
			w.walkDirective(i)
			if w.stop {
				return
			}
		}
	}

	w.removeLastAncestor()

	for i := 0; i < len(w.visitors.leaveVariableDefinition); {
		w.visitors.leaveVariableDefinition[i].LeaveVariableDefinition(ref)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.decreaseDepth()
}

func (w *Walker) walkSelectionSet(ref int) {
	w.increaseDepth()

	for i := 0; i < len(w.visitors.enterSelectionSet); {
		w.visitors.enterSelectionSet[i].EnterSelectionSet(ref)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindSelectionSet)

RefsChanged:
	for {
		refs := w.document.SelectionSets[ref].SelectionRefs
		for i, j := range refs {

			w.SelectionsBefore = refs[:i]
			w.SelectionsAfter = refs[i+1:]

			switch w.document.Selections[j].Kind {
			case ast.SelectionKindField:
				w.walkField(w.document.Selections[j].Ref)
			case ast.SelectionKindFragmentSpread:
				w.walkFragmentSpread(w.document.Selections[j].Ref)
			case ast.SelectionKindInlineFragment:
				w.walkInlineFragment(w.document.Selections[j].Ref)
			}

			if w.stop {
				return
			}
			if !w.refsEqual(refs, w.document.SelectionSets[ref].SelectionRefs) {
				continue RefsChanged
			}
		}
		break
	}

	w.removeLastAncestor()

	for i := 0; i < len(w.visitors.leaveSelectionSet); {
		w.visitors.leaveSelectionSet[i].LeaveSelectionSet(ref)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.decreaseDepth()
}

func (w *Walker) walkField(ref int) {
	w.increaseDepth()

	selectionsBefore := w.SelectionsBefore
	selectionsAfter := w.SelectionsAfter

	for i := 0; i < len(w.visitors.enterField); {
		w.visitors.enterField[i].EnterField(ref)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindField)
	if w.stop {
		return
	}

	if len(w.document.Fields[ref].Arguments.Refs) != 0 {
		for _, i := range w.document.Fields[ref].Arguments.Refs {
			w.walkArgument(i)
			if w.stop {
				return
			}
		}
	}

	if w.document.Fields[ref].HasDirectives {
		for _, i := range w.document.Fields[ref].Directives.Refs {
			w.walkDirective(i)
			if w.stop {
				return
			}
		}
	}

	if w.document.Fields[ref].HasSelections {
		w.walkSelectionSet(w.document.Fields[ref].SelectionSet)
	}

	w.removeLastAncestor()

	w.SelectionsBefore = selectionsBefore
	w.SelectionsAfter = selectionsAfter

	for i := 0; i < len(w.visitors.leaveField); {
		w.visitors.leaveField[i].LeaveField(ref)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.decreaseDepth()
}

func (w *Walker) walkDirective(ref int) {
	w.increaseDepth()

	for i := 0; i < len(w.visitors.enterDirective); {
		w.visitors.enterDirective[i].EnterDirective(ref)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindDirective)

	if w.document.Directives[ref].HasArguments {
		for _, i := range w.document.Directives[ref].Arguments.Refs {
			w.walkArgument(i)
		}
	}

	w.removeLastAncestor()

	for i := 0; i < len(w.visitors.leaveDirective); {
		w.visitors.leaveDirective[i].LeaveDirective(ref)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.decreaseDepth()
}

func (w *Walker) walkArgument(ref int) {
	w.increaseDepth()

	for i := 0; i < len(w.visitors.enterArgument); {
		w.visitors.enterArgument[i].EnterArgument(ref)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	for i := 0; i < len(w.visitors.leaveArgument); {
		w.visitors.leaveArgument[i].LeaveArgument(ref)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.decreaseDepth()
}

func (w *Walker) walkFragmentSpread(ref int) {
	w.increaseDepth()

	for i := 0; i < len(w.visitors.enterFragmentSpread); {
		w.visitors.enterFragmentSpread[i].EnterFragmentSpread(ref)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	for i := 0; i < len(w.visitors.leaveFragmentSpread); {
		w.visitors.leaveFragmentSpread[i].LeaveFragmentSpread(ref)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.decreaseDepth()
}

func (w *Walker) walkInlineFragment(ref int) {
	w.increaseDepth()

	selectionsBefore := w.SelectionsBefore
	selectionsAfter := w.SelectionsAfter

	for i := 0; i < len(w.visitors.enterInlineFragment); {
		w.visitors.enterInlineFragment[i].EnterInlineFragment(ref)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindInlineFragment)

	if w.document.InlineFragments[ref].HasDirectives {
		for _, i := range w.document.InlineFragments[ref].Directives.Refs {
			w.walkDirective(i)
		}
	}

	if w.document.InlineFragments[ref].HasSelections {
		w.walkSelectionSet(w.document.InlineFragments[ref].SelectionSet)
	}

	w.removeLastAncestor()

	w.SelectionsBefore = selectionsBefore
	w.SelectionsAfter = selectionsAfter

	for i := 0; i < len(w.visitors.leaveInlineFragment); {
		w.visitors.leaveInlineFragment[i].LeaveInlineFragment(ref)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
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

func (w *Walker) walkFragmentDefinition(ref int) {
	w.increaseDepth()

	for i := 0; i < len(w.visitors.enterFragmentDefinition); {
		w.visitors.enterFragmentDefinition[i].EnterFragmentDefinition(ref)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindFragmentDefinition)

	if w.document.FragmentDefinitions[ref].HasSelections {
		w.walkSelectionSet(w.document.FragmentDefinitions[ref].SelectionSet)
		if w.stop {
			return
		}
	}

	w.removeLastAncestor()

	for i := 0; i < len(w.visitors.leaveFragmentDefinition); {
		w.visitors.leaveFragmentDefinition[i].LeaveFragmentDefinition(ref)
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.decreaseDepth()
}

func (w *Walker) refsEqual(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (w *Walker) SkipNode() {
	w.skip = true
}

func (w *Walker) Stop() {
	w.stop = true
}

func (w *Walker) RevisitNode() {
	w.revisit = true
}

func (w *Walker) StopWithErr(err error) {
	w.stop = true
	if w.err != nil {
		return
	}
	w.err = err
}

func (w *Walker) ArgumentInputValueDefinition(argument int) (definition int, exits bool) {
	argumentName := w.document.ArgumentName(argument)
	ancestor := w.Ancestors[len(w.Ancestors)-1]
	switch ancestor.Kind {
	case ast.NodeKindField:
		fieldName := w.document.FieldName(ancestor.Ref)
		fieldTypeDef := w.typeDefinitions[len(w.typeDefinitions)-2]
		definition = w.definition.NodeFieldDefinitionArgumentDefinitionByName(fieldTypeDef, fieldName, argumentName)
		exits = definition != -1
	case ast.NodeKindDirective:
		directiveName := w.document.DirectiveNameBytes(ancestor.Ref)
		definition = w.definition.DirectiveArgumentInputValueDefinition(directiveName, argumentName)
		exits = definition != -1
	}
	return
}
