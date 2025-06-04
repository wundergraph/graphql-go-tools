package astvalidation

import (
	"bytes"
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/apollocompatibility"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// FieldSelections validates if all FieldSelections are possible and valid
func FieldSelections(options OperationValidatorOptions) Rule {
	return func(walker *astvisitor.Walker) {
		fieldDefined := fieldDefined{
			Walker:                   walker,
			apolloCompatibilityFlags: options.ApolloCompatibilityFlags,
		}
		walker.RegisterEnterDocumentVisitor(&fieldDefined)
		walker.RegisterEnterFieldVisitor(&fieldDefined)
	}
}

type fieldDefined struct {
	*astvisitor.Walker
	operation                *ast.Document
	definition               *ast.Document
	apolloCompatibilityFlags apollocompatibility.Flags
}

func (f *fieldDefined) EnterDocument(operation, definition *ast.Document) {
	f.operation = operation
	f.definition = definition
}

func (f *fieldDefined) ValidateUnionField(ref int, enclosingTypeDefinition ast.Node) {
	if bytes.Equal(f.operation.FieldNameBytes(ref), literal.TYPENAME) {
		return
	}
	fieldName := f.operation.FieldNameBytes(ref)
	unionName := f.definition.NodeNameBytes(enclosingTypeDefinition)
	f.StopWithExternalErr(operationreport.ErrFieldSelectionOnUnion(fieldName, unionName))
}

func (f *fieldDefined) ValidateInterfaceOrObjectTypeField(ref int, enclosingTypeDefinition ast.Node) {
	fieldName := f.operation.FieldNameBytes(ref)
	if bytes.Equal(fieldName, literal.TYPENAME) {
		return
	}
	typeName := f.definition.NodeNameBytes(enclosingTypeDefinition)
	hasSelections := f.operation.FieldHasSelections(ref)
	definitions := f.definition.NodeFieldDefinitions(enclosingTypeDefinition)
	for _, i := range definitions {
		definitionName := f.definition.FieldDefinitionNameBytes(i)
		definitionTypeName := f.definition.FieldDefinitionTypeNameBytes(i)

		if bytes.Equal(fieldName, definitionName) {
			// field is defined
			fieldDefinitionTypeKind := f.definition.FieldDefinitionTypeNode(i).Kind

			if hasSelections && fieldDefinitionTypeKind == ast.NodeKindEnumTypeDefinition {
				if f.apolloCompatibilityFlags.UseGraphQLValidationErrors {
					f.StopWithExternalErr(operationreport.ErrApolloCompatibleFieldSelectionOnEnum(definitionName, definitionTypeName))
				} else {
					f.StopWithExternalErr(operationreport.ErrFieldSelectionOnEnum(definitionName))
				}
			}

			if hasSelections && fieldDefinitionTypeKind == ast.NodeKindScalarTypeDefinition {
				if f.apolloCompatibilityFlags.UseGraphQLValidationErrors {
					f.StopWithExternalErr(operationreport.ErrApolloCompatibleFieldSelectionOnScalar(definitionName, definitionTypeName))
				} else {
					f.StopWithExternalErr(operationreport.ErrFieldSelectionOnScalar(definitionName))
				}
			}

			if !hasSelections && (fieldDefinitionTypeKind != ast.NodeKindScalarTypeDefinition && fieldDefinitionTypeKind != ast.NodeKindEnumTypeDefinition) {
				if f.apolloCompatibilityFlags.UseGraphQLValidationErrors {
					f.StopWithExternalErr(operationreport.ErrApolloCompatibleMissingFieldSelectionOnNonScalar(fieldName, definitionTypeName))
				} else {
					f.StopWithExternalErr(operationreport.ErrMissingFieldSelectionOnNonScalar(fieldName, typeName))
				}
			}

			return
		}
	}
	if f.apolloCompatibilityFlags.UseGraphQLValidationErrors {
		f.StopWithExternalErr(operationreport.ErrApolloCompatibleFieldUndefinedOnType(fieldName, typeName))
		return
	}
	f.StopWithExternalErr(operationreport.ErrFieldUndefinedOnType(fieldName, typeName))
}

func (f *fieldDefined) ValidateScalarField(ref int, enclosingTypeDefinition ast.Node) {
	scalarTypeName := f.operation.NodeNameBytes(enclosingTypeDefinition)
	f.StopWithExternalErr(operationreport.ErrFieldSelectionOnScalar(scalarTypeName))
}

func (f *fieldDefined) EnterField(ref int) {
	switch f.EnclosingTypeDefinition.Kind {
	case ast.NodeKindUnionTypeDefinition:
		f.ValidateUnionField(ref, f.EnclosingTypeDefinition)
	case ast.NodeKindInterfaceTypeDefinition, ast.NodeKindObjectTypeDefinition:
		f.ValidateInterfaceOrObjectTypeField(ref, f.EnclosingTypeDefinition)
	case ast.NodeKindScalarTypeDefinition:
		f.ValidateScalarField(ref, f.EnclosingTypeDefinition)
	default:
		fieldName := f.operation.FieldNameBytes(ref)
		typeName := f.operation.NodeNameBytes(f.EnclosingTypeDefinition)
		f.StopWithInternalErr(fmt.Errorf("astvalidation/fieldDefined/EnterField: field: %s selection on type: %s unhandled", fieldName, typeName))
	}
}
