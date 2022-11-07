package astvalidation

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

// KnownArguments validates if all arguments are known
func KnownArguments() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := knownArgumentsVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterArgumentVisitor(&visitor)
		walker.RegisterEnterFieldVisitor(&visitor)
	}
}

type knownArgumentsVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
	enclosingNode         ast.Node
}

func (v *knownArgumentsVisitor) EnterField(ref int) {
	_, exists := v.FieldDefinition(ref)
	if !exists {
		v.SkipNode() // ignore arguments of not existing fields
		return
	}

	v.enclosingNode = v.EnclosingTypeDefinition
}

func (v *knownArgumentsVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation = operation
	v.definition = definition
}

func (v *knownArgumentsVisitor) EnterArgument(ref int) {
	_, exists := v.ArgumentInputValueDefinition(ref)
	if exists {
		return
	}

	ancestor := v.Ancestor()
	ancestorName := v.AncestorNameBytes()

	argumentName := v.operation.ArgumentNameBytes(ref)
	argumentPosition := v.operation.Arguments[ref].Position

	switch ancestor.Kind {
	case ast.NodeKindField:
		objectTypeDefName := v.definition.ObjectTypeDefinitionNameBytes(v.enclosingNode.Ref)

		v.Report.AddExternalError(operationreport.ErrArgumentNotDefinedOnField(argumentName, objectTypeDefName, ancestorName, argumentPosition))
	case ast.NodeKindDirective:
		v.Report.AddExternalError(operationreport.ErrArgumentNotDefinedOnDirective(argumentName, ancestorName, argumentPosition))
	}
}
