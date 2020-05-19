package astnormalization

import (
	"bytes"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astimport"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func variablesExtraction(walker *astvisitor.Walker) {
	visitor := &variablesExtractionVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterArgumentVisitor(visitor)
}

type variablesExtractionVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
	importer              astimport.Importer
}

func (v *variablesExtractionVisitor) EnterArgument(ref int) {
	if v.operation.Arguments[ref].Value.Kind == ast.ValueKindVariable {
		return
	}

	variable := ast.VariableValue{
		Name: v.getNextVariableName(),
	}

	v.operation.VariableValues = append(v.operation.VariableValues, variable)

	varRef := len(v.operation.VariableValues) - 1

	v.operation.Arguments[ref].Value.Ref = varRef
	v.operation.Arguments[ref].Value.Kind = ast.ValueKindVariable

	defRef, ok := v.ArgumentInputValueDefinition(ref)
	if !ok {
		return
	}

	defType := v.definition.InputValueDefinitions[defRef].Type

	importedDefType := v.importer.ImportType(defType, v.definition, v.operation)

	v.operation.VariableDefinitions = append(v.operation.VariableDefinitions, ast.VariableDefinition{
		VariableValue: ast.Value{
			Kind: ast.ValueKindVariable,
			Ref:  varRef,
		},
		Type: importedDefType,
	})

	newVariableRef := len(v.operation.VariableDefinitions) - 1

	v.operation.OperationDefinitions[v.Ancestors[0].Ref].VariableDefinitions.Refs =
		append(v.operation.OperationDefinitions[v.Ancestors[0].Ref].VariableDefinitions.Refs, newVariableRef)
	v.operation.OperationDefinitions[v.Ancestors[0].Ref].HasVariableDefinitions = true
}

func (v *variablesExtractionVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation, v.definition = operation, definition
}

var (
	alphabet = []byte("abcdefghijklmnopqrstuvwxyz")
)

// TODO: this can be better - Need to be able to support more than 26 variables
func (v *variablesExtractionVisitor) getNextVariableName() ast.ByteSliceReference {
	for o := 1; o < len(alphabet)+1; o++ {
		for i := 0; i < len(alphabet); i++ {
			potentialName := alphabet[i : i+o]
			exists := false
			for _, j := range v.operation.OperationDefinitions[v.Ancestors[0].Ref].VariableDefinitions.Refs {
				if bytes.Equal(v.operation.VariableDefinitionNameBytes(j), potentialName) {
					exists = true
					break
				}
			}
			if exists {
				continue
			}
			return v.operation.Input.AppendInputBytes(potentialName)
		}
	}
	return ast.ByteSliceReference{}
}
