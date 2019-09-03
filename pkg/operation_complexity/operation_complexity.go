/*
	package operation_complexity implements two common algorithms used by GitHub to calculate GraphQL query complexity

	1. Node count, the maximum number of Nodes a query may return
	2. Complexity, the maximum number of Node requests that might be needed to execute the query

	OperationComplexityEstimator takes a schema definition and a query and then walks recursively through the query to calculate both variables.

	The calculation can be influenced by integer arguments on fields that indicate the amount of Nodes returned by a field.

	To help the algorithm understand your schema you could make use of these two directives:

	- directive @nodeCountMultiply on ARGUMENT_DEFINITION
	- directive @nodeCountSkip on FIELD

	nodeCountMultiply:
	Indicates that the Int value the directive is applied on should be used as a Node multiplier

	nodeCountSkip:
	Indicates that the algorithm should skip this Node. This is useful to whitelist certain query paths, e.g. for introspection.
*/
package operation_complexity

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

var (
	nodeCountMultiply = []byte("nodeCountMultiply")
	nodeCountSkip     = []byte("nodeCountSkip")
)

type OperationComplexityEstimator struct {
	walker  *astvisitor.Walker
	visitor *complexityVisitor
}

func NewOperationComplexityEstimator() *OperationComplexityEstimator {

	walker := &astvisitor.Walker{}
	visitor := &complexityVisitor{
		multipliers: make([]multiplier, 0, 16),
	}

	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterArgumentVisitor(visitor)
	walker.RegisterLeaveFieldVisitor(visitor)
	walker.RegisterEnterFieldVisitor(visitor)
	walker.RegisterEnterSelectionSetVisitor(visitor)
	walker.RegisterEnterFragmentDefinitionVisitor(visitor)

	return &OperationComplexityEstimator{
		walker:  walker,
		visitor: visitor,
	}
}

func (n *OperationComplexityEstimator) Do(operation, definition *ast.Document) (nodeCount, complexity int, err error) {
	n.visitor.count = 0
	n.visitor.complexity = 0
	n.visitor.multipliers = n.visitor.multipliers[:0]
	err = n.walker.Walk(operation, definition)
	return n.visitor.count, n.visitor.complexity, err
}

func CalculateOperationComplexity(operation, definition *ast.Document) (nodeCount, complexity int, err error) {
	estimator := NewOperationComplexityEstimator()
	return estimator.Do(operation, definition)
}

type complexityVisitor struct {
	operation, definition *ast.Document
	count                 int
	complexity            int
	multipliers           []multiplier
}

type multiplier struct {
	fieldRef int
	multi    int
}

func (c *complexityVisitor) calculateMultiplied(i int) int {
	for _, j := range c.multipliers {
		i = i * j.multi
	}
	return i
}

func (c *complexityVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	c.operation = operation
	c.definition = definition
	return astvisitor.Instruction{}
}

func (c *complexityVisitor) EnterArgument(ref int, info astvisitor.Info) astvisitor.Instruction {

	if info.Ancestors[len(info.Ancestors)-1].Kind != ast.NodeKindField {
		return astvisitor.Instruction{}
	}

	if !c.definition.InputValueDefinitionHasDirective(info.Definition.Ref, nodeCountMultiply) {
		return astvisitor.Instruction{}
	}

	value := c.operation.ArgumentValue(ref)
	if value.Kind == ast.ValueKindInteger {
		multi := c.operation.IntValueAsInt(value.Ref)
		c.multipliers = append(c.multipliers, multiplier{
			fieldRef: info.Ancestors[len(info.Ancestors)-1].Ref,
			multi:    multi,
		})
	}

	return astvisitor.Instruction{}
}

func (c *complexityVisitor) EnterField(ref int, info astvisitor.Info) astvisitor.Instruction {

	if info.Definition.Kind != ast.NodeKindFieldDefinition {
		return astvisitor.Instruction{}
	}

	if _, exits := c.definition.FieldDefinitionDirectiveByName(info.Definition.Ref, nodeCountSkip); exits {
		return astvisitor.Instruction{
			Action: astvisitor.Skip,
		}
	}

	if !info.HasSelections {
		return astvisitor.Instruction{}
	}

	c.complexity = c.complexity + c.calculateMultiplied(1)

	return astvisitor.Instruction{}
}

func (c *complexityVisitor) LeaveField(ref int, info astvisitor.Info) astvisitor.Instruction {

	if len(c.multipliers) == 0 {
		return astvisitor.Instruction{}
	}

	if c.multipliers[len(c.multipliers)-1].fieldRef == ref {
		c.multipliers = c.multipliers[:len(c.multipliers)-1]
	}

	return astvisitor.Instruction{}
}

func (c *complexityVisitor) EnterSelectionSet(ref int, info astvisitor.Info) astvisitor.Instruction {

	if info.Ancestors[len(info.Ancestors)-1].Kind != ast.NodeKindField {
		return astvisitor.Instruction{}
	}

	c.count = c.count + c.calculateMultiplied(1)

	return astvisitor.Instruction{}
}

func (c *complexityVisitor) EnterFragmentDefinition(ref int, info astvisitor.Info) astvisitor.Instruction {
	return astvisitor.Instruction{
		Action: astvisitor.Skip,
	}
}
