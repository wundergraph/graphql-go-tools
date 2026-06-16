package astnormalization

import (
	"fmt"

	"github.com/buger/jsonparser"
	"github.com/tidwall/sjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

func coerceNullVariablesWithDefaults(walker *astvisitor.Walker) {
	visitor := &nullVariableCoercionWithDefaultsVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterOperationVisitor(visitor)
	walker.RegisterEnterFieldVisitor(visitor)
}

// nullVariableCoercionWithDefaultsVisitor handles the case where a nullable variable
// is explicitly set to null and used in a non-null argument position that has a schema
// default value.
//
// Per the GraphQL spec, a nullable variable can be used in a non-null argument position
// only if the argument has a default value. When the variable is null at runtime, the
// argument should fall back to its schema default (matching Apollo's behavior).
//
// This visitor "splits" the variable: it replaces the argument's variable reference with
// a new variable that has no value in the variables JSON (making it "not provided" for
// the downstream subgraph, which then uses its schema default). The original variable
// is preserved for other usages where null may be a valid value.
type nullVariableCoercionWithDefaultsVisitor struct {
	*astvisitor.Walker
	operation    *ast.Document
	definition   *ast.Document
	operationRef int
	splitCounter int
}

func (v *nullVariableCoercionWithDefaultsVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation, v.definition = operation, definition
	v.splitCounter = 0
}

func (v *nullVariableCoercionWithDefaultsVisitor) EnterOperationDefinition(ref int) {
	v.operationRef = ref
}

func (v *nullVariableCoercionWithDefaultsVisitor) EnterField(ref int) {
	fieldName := v.operation.FieldNameBytes(ref)
	fieldDefRef, ok := v.definition.NodeFieldDefinitionByName(v.EnclosingTypeDefinition, fieldName)
	if !ok {
		return
	}

	if !v.definition.FieldDefinitions[fieldDefRef].HasArgumentsDefinitions {
		return
	}

	for _, definitionInputValueDefRef := range v.definition.FieldDefinitions[fieldDefRef].ArgumentsDefinition.Refs {
		defTypeRef := v.definition.InputValueDefinitions[definitionInputValueDefRef].Type

		if !v.definition.TypeIsNonNull(defTypeRef) {
			continue
		}
		if !v.definition.InputValueDefinitionHasDefaultValue(definitionInputValueDefRef) {
			continue
		}

		argName := v.definition.InputValueDefinitionNameBytes(definitionInputValueDefRef)
		operationArgRef, exists := v.operation.FieldArgument(ref, argName)
		if !exists {
			continue
		}

		operationArgValue := v.operation.ArgumentValue(operationArgRef)
		if operationArgValue.Kind != ast.ValueKindVariable {
			continue
		}

		variableName := v.operation.VariableValueNameString(operationArgValue.Ref)

		_, dataType, _, err := jsonparser.Get(v.operation.Input.Variables, variableName)
		if err != nil {
			continue
		}
		if dataType != jsonparser.Null {
			continue
		}

		// The variable is explicitly null and the argument is non-null with a default.
		// Split: create a new variable that won't have a value in the variables JSON,
		// so the subgraph treats it as "not provided" and uses its schema default.
		newVarName := fmt.Sprintf("%s_ndf_%d", variableName, v.splitCounter)
		v.splitCounter++

		// Create a new variable value referencing the new name
		newVarValueRef := v.operation.ImportVariableValue([]byte(newVarName))

		// Point the argument to the new variable
		v.operation.Arguments[operationArgRef].Value = ast.Value{
			Kind: ast.ValueKindVariable,
			Ref:  newVarValueRef,
		}

		// Find the original variable definition to copy its type
		varDefRef, varExists := v.operation.VariableDefinitionByNameAndOperation(v.operationRef, []byte(variableName))
		if !varExists {
			continue
		}

		// Add a new variable definition with the same type (nullable)
		newVarValueRefForDef := v.operation.ImportVariableValue([]byte(newVarName))
		typeRef := v.operation.VariableDefinitions[varDefRef].Type
		v.operation.AddVariableDefinitionToOperationDefinition(v.operationRef, newVarValueRefForDef, typeRef)
	}

	// After processing all arguments for this field, check if the original null
	// variables are still referenced anywhere. If not, clean them up from the JSON.
	v.cleanupUnreferencedNullVariables()
}

// cleanupUnreferencedNullVariables removes null variables from the variables JSON
// and their definitions from the operation if they are no longer referenced.
func (v *nullVariableCoercionWithDefaultsVisitor) cleanupUnreferencedNullVariables() {
	if !v.operation.OperationDefinitions[v.operationRef].HasVariableDefinitions {
		return
	}

	refs := v.operation.OperationDefinitions[v.operationRef].VariableDefinitions.Refs
	cleaned := refs[:0]
	for _, varDefRef := range refs {
		varName := v.operation.VariableDefinitionNameString(varDefRef)
		_, dataType, _, err := jsonparser.Get(v.operation.Input.Variables, varName)
		if err != nil || dataType != jsonparser.Null {
			cleaned = append(cleaned, varDefRef)
			continue
		}

		if v.variableIsReferenced(varName) {
			cleaned = append(cleaned, varDefRef)
			continue
		}

		// Variable is null and no longer referenced — remove from JSON and AST
		v.operation.Input.Variables, _ = sjson.DeleteBytes(v.operation.Input.Variables, varName)
	}
	v.operation.OperationDefinitions[v.operationRef].VariableDefinitions.Refs = cleaned
	if len(cleaned) == 0 {
		v.operation.OperationDefinitions[v.operationRef].HasVariableDefinitions = false
	}
}

// variableIsReferenced checks if a variable name is referenced by any argument
// in the operation's selection sets.
func (v *nullVariableCoercionWithDefaultsVisitor) variableIsReferenced(varName string) bool {
	for i := range v.operation.Arguments {
		if v.operation.Arguments[i].Value.Kind != ast.ValueKindVariable {
			continue
		}
		if v.operation.VariableValueNameString(v.operation.Arguments[i].Value.Ref) == varName {
			return true
		}
	}
	return false
}
