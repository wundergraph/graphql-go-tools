package astvalidation

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

// KnownArguments validates if all arguments are known
func KnownArguments() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := knownArgumentsVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterArgumentVisitor(&visitor)
	}
}

type knownArgumentsVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
}

func (v *knownArgumentsVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation = operation
	v.definition = definition
}

func (v *knownArgumentsVisitor) EnterArgument(ref int) {
	definitionRef, exists := v.ArgumentInputValueDefinition(ref)
	_ = definitionRef // TODO: provide location for this error

	if !exists {
		argumentName := v.operation.ArgumentNameBytes(ref)
		ancestorName := v.AncestorNameBytes()
		v.StopWithExternalErr(operationreport.ErrArgumentNotDefinedOnNode(argumentName, ancestorName))
	}
}
