package astvalidation

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// StreamAppliedToListFieldsOnly validates that the stream directive is used on list fields
func StreamAppliedToListFieldsOnly() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := streamAppliedToListFieldsVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterDirectiveVisitor(&visitor)
	}
}

type streamAppliedToListFieldsVisitor struct {
	*astvisitor.Walker

	operation, definition *ast.Document
}

func (s *streamAppliedToListFieldsVisitor) EnterDocument(operation, definition *ast.Document) {
	s.operation = operation
	s.definition = definition
}

func (s *streamAppliedToListFieldsVisitor) EnterDirective(ref int) {
	directiveName := s.operation.DirectiveNameBytes(ref)

	// Only validate @stream directives
	if !bytes.Equal(directiveName, literal.STREAM) {
		return
	}

	// Validate initialCount argument if present
	initialCountValue, hasCount := s.operation.DirectiveArgumentValueByName(ref, literal.INITIAL_COUNT)
	if hasCount {
		if initialCountValue.Kind == ast.ValueKindInteger {
			initialCount := s.operation.IntValueAsInt32(initialCountValue.Ref)
			if initialCount < 0 {
				directivePosition := s.operation.Directives[ref].At
				s.StopWithExternalErr(operationreport.ErrStreamInitialCountMustBeNonNegative(directiveName, directivePosition))
				return
			}
		}
	}

	if len(s.Ancestors) == 0 {
		return
	}
	ancestor := s.Ancestors[len(s.Ancestors)-1]

	// Get the field definition from the schema
	// We need to walk up the type definitions to find the field
	fieldName := s.operation.FieldNameBytes(ancestor.Ref)
	// Find the enclosing type by looking at TypeDefinitions in the walker.
	// Start from the item before the last one of typeDefinitions.
	var fieldDefinition int
	var exists bool
	for i := len(s.TypeDefinitions) - 2; i >= 0; i-- {
		fieldDefinition, exists = s.definition.NodeFieldDefinitionByName(s.TypeDefinitions[i], fieldName)
		if exists {
			break
		}
	}

	if !exists {
		// If the field doesn't exist in the schema, that's a different validation error
		// Skip this check
		return
	}

	fieldTypeRef := s.definition.FieldDefinitionType(fieldDefinition)

	if !s.definition.TypeIsList(fieldTypeRef) {
		directivePosition := s.operation.Directives[ref].At
		s.StopWithExternalErr(operationreport.ErrStreamDirectiveOnNonListField(directiveName, fieldName, directivePosition))
	}
}
