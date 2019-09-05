package astnormalization

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/fastastvisitor"
)

type FragmentSpreadDepth struct {
	walker             fastastvisitor.Walker
	visitor            fragmentSpreadDepthVisitor
	calc               NestedDepthCalc
	visitorsRegistered bool
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

	if !r.visitorsRegistered {
		r.walker.RegisterEnterFragmentSpreadVisitor(&r.visitor)
		r.visitorsRegistered = true
	}

	r.visitor.operation = operation
	r.visitor.definition = definition
	r.visitor.depths = depths
	r.visitor.Walker = &r.walker

	err := r.walker.Walk(operation, definition)
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
	*fastastvisitor.Walker
	operation  *ast.Document
	definition *ast.Document
	depths     *Depths
	err        error
}

func (r *fragmentSpreadDepthVisitor) EnterFragmentSpread(ref int) {

	depth := Depth{
		SpreadRef:  ref,
		Depth:      r.Depth,
		SpreadName: r.operation.FragmentSpreadName(ref),
	}

	if r.Ancestors[0].Kind == ast.NodeKindFragmentDefinition {
		depth.isNested = true
		depth.parentFragmentName = r.operation.FragmentDefinitionName(r.Ancestors[0].Ref)
	}

	*r.depths = append(*r.depths, depth)
}
