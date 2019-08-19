package astvalidation

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astinspect"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

type FieldSelectionMergingVisitor struct {
	definition, operation *ast.Document
	err                   error
}

func (f *FieldSelectionMergingVisitor) EnterArgument(ref int, definition int, info astvisitor.Info) {

}

func (f *FieldSelectionMergingVisitor) LeaveArgument(ref int, definition int, info astvisitor.Info) {

}

func (f *FieldSelectionMergingVisitor) EnterOperationDefinition(ref int, info astvisitor.Info) {

}

func (f *FieldSelectionMergingVisitor) LeaveOperationDefinition(ref int, info astvisitor.Info) {

}

func (f *FieldSelectionMergingVisitor) EnterSelectionSet(ref int, info astvisitor.Info) (instruction astvisitor.Instruction) {

	if !astinspect.SelectionSetCanMerge(ref, info.EnclosingTypeDefinition, f.operation, f.definition) {
		f.err = fmt.Errorf("selectionset cannot merge")
	}

	/*	if f.err != nil {
			return
		}

		for _, i := range f.operation.SelectionSets[ref].SelectionRefs {
			left := f.operation.Selections[i].Ref
			if f.operation.Selections[i].Kind == ast.SelectionKindField {
				for _, j := range f.operation.SelectionSets[ref].SelectionRefs {
					if i == j {
						continue
					}
					if i > j {
						continue
					}
					if f.operation.Selections[j].Kind != ast.SelectionKindField {
						continue
					}
					right := f.operation.Selections[j].Ref
					if !astinspect.FieldsCanMerge(f.operation, left, right) {
						f.err = fmt.Errorf("cannot merge fields left: %+v, right: %+v", f.operation.Fields[left], f.operation.Fields[right])
						return
					}
				}
			}
			if f.operation.Selections[i].Kind == ast.SelectionKindInlineFragment {
				for _, j := range f.operation.SelectionSets[ref].SelectionRefs {
					if i == j {
						continue
					}
					if i > j {
						continue
					}
					if f.operation.Selections[j].Kind != ast.SelectionKindInlineFragment {
						continue
					}

					astinspect.SelectionSetCanMerge()
				}
			}
		}*/
	return
}

func (f *FieldSelectionMergingVisitor) LeaveSelectionSet(ref int, info astvisitor.Info) {

}

func (f *FieldSelectionMergingVisitor) EnterField(ref int, info astvisitor.Info) {

}

func (f *FieldSelectionMergingVisitor) LeaveField(ref int, info astvisitor.Info) {

}

func (f *FieldSelectionMergingVisitor) EnterFragmentSpread(ref int, info astvisitor.Info) {

}

func (f *FieldSelectionMergingVisitor) LeaveFragmentSpread(ref int, info astvisitor.Info) {

}

func (f *FieldSelectionMergingVisitor) EnterInlineFragment(ref int, info astvisitor.Info) {

}

func (f *FieldSelectionMergingVisitor) LeaveInlineFragment(ref int, info astvisitor.Info) {

}

func (f *FieldSelectionMergingVisitor) EnterFragmentDefinition(ref int, info astvisitor.Info) {

}

func (f *FieldSelectionMergingVisitor) LeaveFragmentDefinition(ref int, info astvisitor.Info) {

}
