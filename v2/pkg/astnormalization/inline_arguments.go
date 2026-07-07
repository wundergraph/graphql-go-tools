package astnormalization

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/position"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// InlineArgument describes a single argument in an operation whose value was
// supplied inline (a literal) instead of as a variable.
type InlineArgument struct {
	ArgumentName  string
	EnclosingName string
	EnclosingKind ast.NodeKind
	ValueKind     ast.ValueKind
	Position      position.Position
}

func (a InlineArgument) QualifiedName() string {
	switch a.EnclosingKind {
	case ast.NodeKindField:
		return a.EnclosingName + "." + a.ArgumentName
	case ast.NodeKindDirective:
		return "@" + a.EnclosingName + "." + a.ArgumentName
	default:
		return a.ArgumentName
	}
}

type InlineArgumentsValidationOptions struct {
	Enforce      bool
	ErrorMessage string
	ErrorCode    string
	StatusCode   int
	// ReturnInResponseExtensions, when true and enforcing, names the offending
	// inline argument in the rejection error message (e.g. `... argument "user.id".`).
	// In non-enforcing mode the router reports findings via the top-level response
	// extensions instead, so this option has no effect on the walker there.
	ReturnInResponseExtensions bool
}

type InlineArgumentsValidator struct {
	Options  InlineArgumentsValidationOptions
	Findings []InlineArgument
	Disabled bool
}

func (v *InlineArgumentsValidator) ClearFindings() {
	v.Findings = v.Findings[:0]
}

func (v *InlineArgumentsValidator) HadInlineArguments() bool {
	return len(v.Findings) > 0
}

// InlineArgumentsRule returns a prevalidation rule that flags every argument
// whose value is an inline literal instead of a variable, in any context: field
// arguments, directive arguments (@skip/@include and any custom directive), and
// introspection-field arguments. Register it via WithPrevalidationRules; results
// land on the given validator.
//
// Variable-definition default values (e.g. `$x: Int = 5`) are naturally excluded
// — they are not arguments and are never visited as one.
func InlineArgumentsRule(validator *InlineArgumentsValidator) func(walker *astvisitor.Walker) {
	return func(walker *astvisitor.Walker) {
		visitor := &inlineArgumentsVisitor{
			Walker:    walker,
			validator: validator,
		}
		walker.RegisterEnterDocumentVisitor(visitor)
		walker.RegisterEnterArgumentVisitor(visitor)
	}
}

type inlineArgumentsVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
	validator             *InlineArgumentsValidator
}

func (v *inlineArgumentsVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation = operation
	v.definition = definition
}

func (v *inlineArgumentsVisitor) EnterArgument(ref int) {
	if v.validator.Disabled {
		return
	}
	valueKind := v.operation.Arguments[ref].Value.Kind
	if valueKind == ast.ValueKindVariable {
		return
	}

	finding := InlineArgument{
		ArgumentName: v.operation.ArgumentNameString(ref),
		ValueKind:    valueKind,
		Position:     v.operation.Arguments[ref].Position,
	}

	if len(v.Ancestors) > 0 {
		parent := v.Ancestors[len(v.Ancestors)-1]
		finding.EnclosingKind = parent.Kind
		switch parent.Kind {
		case ast.NodeKindField:
			finding.EnclosingName = v.operation.FieldNameString(parent.Ref)
		case ast.NodeKindDirective:
			finding.EnclosingName = v.operation.DirectiveNameString(parent.Ref)
		}
	}

	if v.validator.Options.Enforce {
		// Reject on the first inline argument and stop the walk — a single error is
		// enough to signal that the operation is non-compliant. When configured, the
		// offending argument is named in the message rather than in a dedicated
		// extension.
		message := v.validator.Options.ErrorMessage
		if v.validator.Options.ReturnInResponseExtensions {
			message = fmt.Sprintf("%s Inline value provided for argument %q.", message, finding.QualifiedName())
		}
		v.StopWithExternalErr(operationreport.ExternalError{
			Message:       message,
			ExtensionCode: v.validator.Options.ErrorCode,
			StatusCode:    v.validator.Options.StatusCode,
			Locations:     operationreport.LocationsFromPosition(finding.Position),
		})
		return
	}

	v.validator.Findings = append(v.validator.Findings, finding)
}
