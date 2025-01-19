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

// VariablesMapper is a visitor which remaps variables in the operation to have more cache hits for the queries of the same shape
// but with different variables/inline values combinations
// e.g.
//
//	query MyQuery($a: String!, $b: String!) {
//		field(a: $a, b: $b)
//	}
//
//	query MyQuery($e: String!, $d: String!) {
//		field(a: $e, b: $d)
//	}
//
//	query MyQuery($b: String!, $a: String!) {
//		field(a: $a, b: $b)
//	}
//
//	query MyQuery {
//		field(a: "a", b: "b")
//	}
//
//	query MyQuery($b: String!) {
//		field(a: "a", b: $b)
//	}
//
//	query MyQuery($a: String!) {
//		field(a: $a, b: "b")
//	}
//
// All of the example queries above will be normalized to the same query:
//
//	query MyQuery($a: String!, $b: String!) {
//		field(a: $a, b: $b)
//	}
//
// The important consideration - the main requirement is to have same amount of variables/inline values used in a query in the same places
// otherwise the queries will be considered different
// e.g. field(a: "a", b: "a") will be the same as field(a: $a, b: $a) but different from field(a: $a, b: $b) or field(a: $a, b: $a)
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

	// it is important to sort the variable definitions of operation by name to ensure that the order is deterministic
	// which allows to produce the same query string for the queries with different order of variables used in the same places
	// e.g.
	// query MyQuery($e: String!, $d: String!) {
	// 	field(a: $e, b: $d)
	// }
	//
	// query MyQuery($b: String!, $a: String!) {
	// 	field(a: $a, b: $b)
	// }
	// both of this queries will be normalized to the same query:
	// query MyQuery($a: String!, $b: String!) {
	// 	field(a: $a, b: $b)
	// }
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

	variableDefinitionRef, exists := v.operation.VariableDefinitionByNameAndOperation(v.operationRef, varNameBytes)
	if !exists {
		return
	}

	// explicitly convert to string to convert unsafe
	varName := string(varNameBytes)

	// here we collect occurrences of the variables in the operation in depth-first order
	// if the variable is the same we save the ref to the variable value
	// if we haven't seen the variable - we save the ref to the variable definition and its name
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

// generateUnusedVariableMappingName generates a new name for the variable mapping
// right now it will generate the next variable names
// 0-25: a, b, c, ..., z
// 26-51: aa, bb, cc, ..., zz
// 52-77: aaa, bbb, ccc, ..., zzz
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
