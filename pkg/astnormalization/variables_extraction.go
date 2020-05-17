package astnormalization

import (
	"fmt"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func variablesExtraction(walker *astvisitor.Walker){
	visitor := &variablesExtractionVisitor{
		Walker:walker,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterOperationVisitor(visitor)
	walker.RegisterEnterVariableDefinitionVisitor(visitor)
}

type variablesExtractionVisitor struct {
	*astvisitor.Walker
	operation,definition *ast.Document
}

func (v *variablesExtractionVisitor) EnterVariableDefinition(ref int) {
	name := v.operation.VariableValueNameString(ref)
	fmt.Println(name)
}

func (v *variablesExtractionVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation,v.definition = operation,definition
}

func (v *variablesExtractionVisitor) EnterOperationDefinition(ref int) {
	name := v.operation.OperationDefinitionNameString(ref)
	fmt.Println(name)
}
