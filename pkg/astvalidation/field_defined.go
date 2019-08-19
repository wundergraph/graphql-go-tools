package astvalidation

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
)

type fieldDefined struct {
	operation  *ast.Document
	definition *ast.Document
	err        error
}

func (f *fieldDefined) ValidateUnionField(ref int, info astvisitor.Info) error {
	if bytes.Equal(f.operation.FieldName(ref), literal.TYPENAME) {
		return nil
	}
	fieldName := f.operation.FieldNameString(ref)
	typeName := f.definition.NodeTypeNameString(info.EnclosingTypeDefinition)
	return fmt.Errorf("field with name: %s not defined on union: %s", fieldName, typeName)
}

func (f *fieldDefined) ValidateInterfaceObjectTypeField(ref int, info astvisitor.Info) error {
	fieldName := f.operation.FieldName(ref)
	hasSelections := f.operation.FieldHasSelections(ref)
	definitions := f.definition.NodeFieldDefinitions(info.EnclosingTypeDefinition)
	for _, i := range definitions {
		definitionName := f.definition.FieldDefinitionName(i)
		if bytes.Equal(fieldName, definitionName) {
			// field is defined
			fieldDefinitionTypeKind := f.definition.FieldDefinitionTypeNodeKind(i)
			switch {
			case hasSelections && fieldDefinitionTypeKind == ast.NodeKindScalarTypeDefinition:
				return fmt.Errorf("field cannot have selections on scalar type")
			case !hasSelections && fieldDefinitionTypeKind != ast.NodeKindScalarTypeDefinition:
				return fmt.Errorf("field must have selections on non scalar type")
			default:
				return nil
			}
		}
	}

	typeName := f.definition.NodeTypeNameString(info.EnclosingTypeDefinition)
	return fmt.Errorf("field with name: %s not defined on type: %s", string(fieldName), typeName)
}

func (f *fieldDefined) ValidateScalarField(ref int, info astvisitor.Info) error {
	fieldName := f.operation.FieldNameString(ref)
	typeName := f.operation.NodeTypeNameString(info.EnclosingTypeDefinition)
	return fmt.Errorf("cannot select field: %s on scalar type: %s", fieldName, typeName)
}

func (f *fieldDefined) EnterOperationDefinition(ref int, info astvisitor.Info) {

}

func (f *fieldDefined) LeaveOperationDefinition(ref int, info astvisitor.Info) {

}

func (f *fieldDefined) EnterSelectionSet(ref int, info astvisitor.Info) (instruction astvisitor.Instruction) {
	return
}

func (f *fieldDefined) LeaveSelectionSet(ref int, info astvisitor.Info) {

}

func (f *fieldDefined) EnterField(ref int, info astvisitor.Info) {

	if f.err != nil {
		return
	}

	switch info.EnclosingTypeDefinition.Kind {
	case ast.NodeKindUnionTypeDefinition:
		f.err = f.ValidateUnionField(ref, info)
	case ast.NodeKindInterfaceTypeDefinition, ast.NodeKindObjectTypeDefinition:
		f.err = f.ValidateInterfaceObjectTypeField(ref, info)
	case ast.NodeKindScalarTypeDefinition:
		f.err = f.ValidateScalarField(ref, info)
	}
}

func (f *fieldDefined) LeaveField(ref int, info astvisitor.Info) {

}

func (f *fieldDefined) EnterArgument(ref int, definition int, info astvisitor.Info) {

}

func (f *fieldDefined) LeaveArgument(ref int, definition int, info astvisitor.Info) {

}

func (f *fieldDefined) EnterFragmentSpread(ref int, info astvisitor.Info) {

}

func (f *fieldDefined) LeaveFragmentSpread(ref int, info astvisitor.Info) {

}

func (f *fieldDefined) EnterInlineFragment(ref int, info astvisitor.Info) {

}

func (f *fieldDefined) LeaveInlineFragment(ref int, info astvisitor.Info) {

}

func (f *fieldDefined) EnterFragmentDefinition(ref int, info astvisitor.Info) {

}

func (f *fieldDefined) LeaveFragmentDefinition(ref int, info astvisitor.Info) {

}
