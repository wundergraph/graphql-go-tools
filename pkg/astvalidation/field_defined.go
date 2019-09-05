package astvalidation

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
)

type fieldDefined struct {
	*astvisitor.Walker
	operation  *ast.Document
	definition *ast.Document
}

func (f *fieldDefined) EnterDocument(operation, definition *ast.Document) {
	f.operation = operation
	f.definition = definition
}

func (f *fieldDefined) ValidateUnionField(ref int, enclosingTypeDefinition ast.Node) error {
	if bytes.Equal(f.operation.FieldName(ref), literal.TYPENAME) {
		return nil
	}
	fieldName := f.operation.FieldNameString(ref)
	typeName := f.definition.NodeTypeNameString(enclosingTypeDefinition)
	return fmt.Errorf("field with name: %s not defined on union: %s", fieldName, typeName)
}

func (f *fieldDefined) ValidateInterfaceObjectTypeField(ref int, enclosingTypeDefinition ast.Node) error {
	fieldName := f.operation.FieldName(ref)
	hasSelections := f.operation.FieldHasSelections(ref)
	definitions := f.definition.NodeFieldDefinitions(enclosingTypeDefinition)
	for _, i := range definitions {
		definitionName := f.definition.FieldDefinitionNameBytes(i)
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

	typeName := f.definition.NodeTypeNameString(enclosingTypeDefinition)
	return fmt.Errorf("field with name: %s not defined on type: %s", string(fieldName), typeName)
}

func (f *fieldDefined) ValidateScalarField(ref int, enclosingTypeDefinition ast.Node) error {
	fieldName := f.operation.FieldNameString(ref)
	typeName := f.operation.NodeTypeNameString(enclosingTypeDefinition)
	return fmt.Errorf("cannot select field: %s on scalar type: %s", fieldName, typeName)
}

func (f *fieldDefined) EnterField(ref int) {
	var err error
	switch f.EnclosingTypeDefinition.Kind {
	case ast.NodeKindUnionTypeDefinition:
		err = f.ValidateUnionField(ref, f.EnclosingTypeDefinition)
	case ast.NodeKindInterfaceTypeDefinition, ast.NodeKindObjectTypeDefinition:
		err = f.ValidateInterfaceObjectTypeField(ref, f.EnclosingTypeDefinition)
	case ast.NodeKindScalarTypeDefinition:
		err = f.ValidateScalarField(ref, f.EnclosingTypeDefinition)
	}

	if err != nil {
		f.StopWithErr(err)
	}
}
