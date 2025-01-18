package astnormalization

import (
	"cmp"
	"math"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

func remapVariables(walker *astvisitor.Walker) *variablesMappingVisitor {
	visitor := &variablesMappingVisitor{
		Walker: walker,
	}
	walker.RegisterDocumentVisitor(visitor)
	walker.RegisterEnterOperationVisitor(visitor)
	walker.RegisterEnterArgumentVisitor(visitor)
	return visitor
}

type variablesMappingVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
	mapping               map[string]string
	variables             []*variableItem
	operationRef          int
}

type variableItem struct {
	variableName          string
	valueRefs             []int
	variableDefinitionRef int
}

func (v *variablesMappingVisitor) LeaveDocument(operation, definition *ast.Document) {
	for _, variableItem := range v.variables {
		mappingName := v.generateUnusedVariableMappingName()
		v.mapping[string(mappingName)] = variableItem.variableName

		newVariableName := v.operation.Input.AppendInputBytes(mappingName)

		// set new variable name for all variable values
		for _, variableValueRef := range variableItem.valueRefs {
			v.operation.VariableValues[variableValueRef].Name = newVariableName
		}

		// set new variable name for variable definition
		v.operation.VariableValues[v.operation.VariableDefinitions[variableItem.variableDefinitionRef].VariableValue.Ref].Name = newVariableName
	}

	slices.SortFunc(v.operation.OperationDefinitions[v.operationRef].VariableDefinitions.Refs, func(i, j int) int {
		return cmp.Compare(
			v.operation.VariableValueNameString(v.operation.VariableDefinitions[i].VariableValue.Ref),
			v.operation.VariableValueNameString(v.operation.VariableDefinitions[j].VariableValue.Ref),
		)
	})
}

func (v *variablesMappingVisitor) EnterArgument(ref int) {
	if v.operation.Arguments[ref].Value.Kind != ast.ValueKindVariable {
		return
	}
	if len(v.Ancestors) == 0 || v.Ancestors[0].Kind != ast.NodeKindOperationDefinition {
		return
	}

	varValueRef := v.operation.Arguments[ref].Value.Ref
	varNameBytes := v.operation.VariableValueNameBytes(varValueRef)
	// explicitly convert to string to convert unsafe
	varName := string(varNameBytes)

	variableDefinitionRef, exists := v.operation.VariableDefinitionByNameAndOperation(v.operationRef, varNameBytes)
	if !exists {
		return
	}

	idx := slices.IndexFunc(v.variables, func(i *variableItem) bool {
		return i.variableName == varName
	})
	if idx == -1 {
		v.variables = append(v.variables, &variableItem{
			variableName:          varName,
			valueRefs:             []int{varValueRef},
			variableDefinitionRef: variableDefinitionRef,
		})
		return
	}

	v.variables[idx].valueRefs = append(v.variables[idx].valueRefs, varValueRef)
}

func (v *variablesMappingVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation, v.definition = operation, definition
	v.mapping = make(map[string]string, len(operation.VariableDefinitions))
	v.variables = make([]*variableItem, 0, len(operation.VariableDefinitions))
}

func (v *variablesMappingVisitor) EnterOperationDefinition(ref int) {
	v.operationRef = ref
}

const alphabet = `abcdefghijklmnopqrstuvwxyz`

func (v *variablesMappingVisitor) generateUnusedVariableMappingName() []byte {
	var i, k int64

	for i = 1; i < math.MaxInt16; i++ {
		out := make([]byte, i)
		for j := range alphabet {
			for k = 0; k < i; k++ {
				out[k] = alphabet[j]
			}
			_, exists := v.mapping[string(out)]
			if !exists {
				return out
			}
		}
	}

	return nil
}
