package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/position"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// InlineArgument describes a single argument in an operation whose value was
// supplied inline (a literal) instead of as a variable.
type InlineArgument struct {
	ArgumentName string
	AncestorName string
	AncestorKind ast.NodeKind
	ValueKind    ast.ValueKind
	Position     position.Position
}

func (a InlineArgument) QualifiedName() string {
	switch a.AncestorKind {
	case ast.NodeKindField:
		return a.AncestorName + "." + a.ArgumentName
	case ast.NodeKindDirective:
		return "@" + a.AncestorName + "." + a.ArgumentName
	default:
		return a.ArgumentName
	}
}

type InlineArgumentsValidationOptions struct {
	Enforce      bool
	ErrorMessage string
	ErrorCode    string
	StatusCode   int
}

// NormalizationResult carries per-run outputs of normalization beyond report errors.
type NormalizationResult struct {
	InlineArguments []InlineArgument
}

// RunOptions are per-call inputs to a normalization run.
type RunOptions struct {
	SkipInlineArguments bool
}

func registerInlineArgumentsValidation(walker *astvisitor.Walker, opts InlineArgumentsValidationOptions) *inlineArgumentsVisitor {
	visitor := &inlineArgumentsVisitor{
		Walker: walker,
		opts:   opts,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterArgumentVisitor(visitor)
	return visitor
}

type inlineArgumentsVisitor struct {
	*astvisitor.Walker

	operation, definition *ast.Document
	opts                  InlineArgumentsValidationOptions

	// disabled is set per run (see RunOptions.SkipInlineArguments) to exempt this
	// operation from detection/enforcement.
	disabled bool
	// result accumulates the findings for the current run. Reset by the normalizer
	// at the start of each run.
	result NormalizationResult
}

func (v *inlineArgumentsVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation = operation
	v.definition = definition
}

func (v *inlineArgumentsVisitor) EnterArgument(ref int) {
	if v.disabled {
		return
	}
	valueKind := v.operation.Arguments[ref].Value.Kind
	if valueKind == ast.ValueKindVariable {
		return
	}

	if v.opts.Enforce {
		// Reject on the first inline argument and stop the walk. A single generic
		// error is enough to signal that the operation is non-compliant; we don't
		// name the argument or point at its location.
		v.StopWithExternalErr(operationreport.ExternalError{
			Message:       v.opts.ErrorMessage,
			ExtensionCode: v.opts.ErrorCode,
			StatusCode:    v.opts.StatusCode,
		})
		return
	}

	finding := InlineArgument{
		ArgumentName: v.operation.ArgumentNameString(ref),
		ValueKind:    valueKind,
		Position:     v.operation.Arguments[ref].Position,
	}

	if len(v.Ancestors) > 0 {
		parent := v.Ancestors[len(v.Ancestors)-1]
		finding.AncestorKind = parent.Kind
		switch parent.Kind {
		case ast.NodeKindField:
			finding.ArgumentName = v.operation.FieldNameString(parent.Ref)
		case ast.NodeKindDirective:
			finding.AncestorName = v.operation.DirectiveNameString(parent.Ref)
		}
	}

	v.result.InlineArguments = append(v.result.InlineArguments, finding)
}
