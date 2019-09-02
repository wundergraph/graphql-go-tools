package operation_complexity

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

var (
	nodeCountMultiply = []byte("nodeCountMultiply")
)

type NodeCounter struct {
	walker  *astvisitor.Walker
	visitor *nodeCountVisitor
}

func NewNodeCounter() *NodeCounter {

	walker := &astvisitor.Walker{}
	visitor := &nodeCountVisitor{
		multipliers: make([]multiplier, 0, 16),
	}

	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterArgumentVisitor(visitor)
	walker.RegisterLeaveFieldVisitor(visitor)
	walker.RegisterEnterSelectionSetVisitor(visitor)

	return &NodeCounter{
		walker:  walker,
		visitor: visitor,
	}
}

func (n *NodeCounter) Do(operation, definition *ast.Document) (int, error) {
	n.visitor.count = 0
	n.visitor.multipliers = n.visitor.multipliers[:0]
	err := n.walker.Walk(operation, definition)
	return n.visitor.count, err
}

func NodeCount(operation, definition *ast.Document) (int, error) {
	counter := NewNodeCounter()
	return counter.Do(operation, definition)
}

type nodeCountVisitor struct {
	operation, definition *ast.Document
	count                 int
	multipliers           []multiplier
}

type multiplier struct {
	fieldRef int
	multi    int
}

func (n *nodeCountVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	n.operation = operation
	n.definition = definition
	return astvisitor.Instruction{}
}

func (n *nodeCountVisitor) EnterArgument(ref int, info astvisitor.Info) astvisitor.Instruction {

	if info.Ancestors[len(info.Ancestors)-1].Kind != ast.NodeKindField {
		return astvisitor.Instruction{}
	}

	if !n.definition.InputValueDefinitionHasDirective(info.Definition.Ref, nodeCountMultiply) {
		return astvisitor.Instruction{}
	}

	value := n.operation.ArgumentValue(ref)
	if value.Kind == ast.ValueKindInteger {
		multi := n.operation.IntValueAsInt(value.Ref)
		n.multipliers = append(n.multipliers, multiplier{
			fieldRef: info.Ancestors[len(info.Ancestors)-1].Ref,
			multi:    multi,
		})
	}

	return astvisitor.Instruction{}
}

func (n *nodeCountVisitor) LeaveField(ref int, info astvisitor.Info) astvisitor.Instruction {
	if len(n.multipliers) == 0 {
		return astvisitor.Instruction{}
	}

	if n.multipliers[len(n.multipliers)-1].fieldRef == ref {
		n.multipliers = n.multipliers[:len(n.multipliers)-1]
	}

	return astvisitor.Instruction{}
}

func (n *nodeCountVisitor) EnterSelectionSet(ref int, info astvisitor.Info) astvisitor.Instruction {

	if info.Ancestors[len(info.Ancestors)-1].Kind != ast.NodeKindField {
		return astvisitor.Instruction{}
	}

	count := 1
	for _, i := range n.multipliers {
		count = count * i.multi
	}

	n.count = n.count + count

	return astvisitor.Instruction{}
}
