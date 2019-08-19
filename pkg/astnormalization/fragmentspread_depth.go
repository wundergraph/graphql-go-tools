package astnormalization

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

type FragmentSpreadDepth struct {
	walker  astvisitor.Walker
	visitor fragmentSpreadDepthVisitor
	calc    NestedDepthCalc
}

type Depth struct {
	SpreadRef          int
	Depth              int
	SpreadName         ast.ByteSlice
	isNested           bool
	parentFragmentName ast.ByteSlice
}

type Depths []Depth

func (d Depths) ByRef(ref int) (int, bool) {
	for i := range d {
		if d[i].SpreadRef == ref {
			return d[i].Depth, true
		}
	}
	return -1, false
}

func (r *FragmentSpreadDepth) Get(operation, definition *ast.Document, depths *Depths) error {

	r.visitor.operation = operation
	r.visitor.definition = definition
	r.visitor.depths = depths

	err := r.walker.Visit(operation, definition, &r.visitor)
	if err != nil {
		return err
	}

	r.calc.calculatedNestedDepths(depths)

	return r.visitor.err
}

type NestedDepthCalc struct {
	depths *Depths
}

func (n *NestedDepthCalc) calculatedNestedDepths(depths *Depths) {
	n.depths = depths

	for i := range *depths {
		(*depths)[i].Depth = n.calculateNestedDepth(i)
	}
}

func (n *NestedDepthCalc) calculateNestedDepth(i int) int {
	if !(*n.depths)[i].isNested {
		return (*n.depths)[i].Depth
	}
	return (*n.depths)[i].Depth + n.depthForFragment((*n.depths)[i].parentFragmentName)
}

func (n *NestedDepthCalc) depthForFragment(name ast.ByteSlice) int {
	for i := range *n.depths {
		if bytes.Equal(name, (*n.depths)[i].SpreadName) {
			return n.calculateNestedDepth(i)
		}
	}
	return 0
}

type fragmentSpreadDepthVisitor struct {
	operation  *ast.Document
	definition *ast.Document
	depths     *Depths
	err        error
}

func (r *fragmentSpreadDepthVisitor) EnterArgument(ref int, definition int, info astvisitor.Info) {

}

func (r *fragmentSpreadDepthVisitor) LeaveArgument(ref int, definition int, info astvisitor.Info) {

}

func (r *fragmentSpreadDepthVisitor) EnterOperationDefinition(ref int, info astvisitor.Info) {

}

func (r *fragmentSpreadDepthVisitor) LeaveOperationDefinition(ref int, info astvisitor.Info) {

}

func (r *fragmentSpreadDepthVisitor) EnterSelectionSet(ref int, info astvisitor.Info) (instruction astvisitor.Instruction) {
	return
}

func (r *fragmentSpreadDepthVisitor) LeaveSelectionSet(ref int, info astvisitor.Info) {

}

func (r *fragmentSpreadDepthVisitor) EnterField(ref int, info astvisitor.Info) {

}

func (r *fragmentSpreadDepthVisitor) LeaveField(ref int, info astvisitor.Info) {

}

func (r *fragmentSpreadDepthVisitor) EnterFragmentSpread(ref int, info astvisitor.Info) {

	depth := Depth{
		SpreadRef:  ref,
		Depth:      info.Depth,
		SpreadName: r.operation.FragmentSpreadName(ref),
	}

	if info.Ancestors[0].Kind == ast.NodeKindFragmentDefinition {
		depth.isNested = true
		depth.parentFragmentName = r.operation.FragmentDefinitionName(info.Ancestors[0].Ref)
	}

	*r.depths = append(*r.depths, depth)
}

func (r *fragmentSpreadDepthVisitor) LeaveFragmentSpread(ref int, info astvisitor.Info) {

}

func (r *fragmentSpreadDepthVisitor) EnterInlineFragment(ref int, info astvisitor.Info) {

}

func (r *fragmentSpreadDepthVisitor) LeaveInlineFragment(ref int, info astvisitor.Info) {

}

func (r *fragmentSpreadDepthVisitor) EnterFragmentDefinition(ref int, info astvisitor.Info) {

}

func (r *fragmentSpreadDepthVisitor) LeaveFragmentDefinition(ref int, info astvisitor.Info) {

}
