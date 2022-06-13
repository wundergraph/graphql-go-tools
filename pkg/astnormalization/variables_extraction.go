package astnormalization

import (
	"bytes"
	"fmt"
	"github.com/tidwall/sjson"
	"math"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafebytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astimport"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func extractVariables(walker *astvisitor.Walker) *variablesExtractionVisitor {
	visitor := &variablesExtractionVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterArgumentVisitor(visitor)
	walker.RegisterEnterOperationVisitor(visitor)
	return visitor
}

type variablesExtractionVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
	importer              astimport.Importer
	operationName         []byte
	skip                  bool
}

func (v *variablesExtractionVisitor) EnterOperationDefinition(ref int) {
	if len(v.operationName) == 0 {
		v.skip = false
		return
	}
	operationName := v.operation.OperationDefinitionNameBytes(ref)
	v.skip = !bytes.Equal(operationName, v.operationName)
}

func (v *variablesExtractionVisitor) EnterArgument(ref int) {
	if v.skip {
		return
	}
	if v.operation.Arguments[ref].Value.Kind == ast.ValueKindVariable {
		return
	}
	if len(v.Ancestors) == 0 || v.Ancestors[0].Kind != ast.NodeKindOperationDefinition {
		return
	}

	for i := range v.Ancestors {
		if v.Ancestors[i].Kind == ast.NodeKindDirective {
			return // skip all directives in any case
		}
	}

	inputValueDefinition, ok := v.Walker.ArgumentInputValueDefinition(ref)
	if !ok {
		return
	}

	v.appendArgumentDefaultInputFields(ref)

	containsVariable := v.operation.ValueContainsVariable(v.operation.Arguments[ref].Value)
	if containsVariable {
		v.traverseValue(v.operation.Arguments[ref].Value, ref, inputValueDefinition)
		return
	}

	variableNameBytes := v.operation.GenerateUnusedVariableDefinitionName(v.Ancestors[0].Ref)
	valueBytes, err := v.operation.ValueToJSON(v.operation.Arguments[ref].Value)
	if err != nil {
		return
	}
	v.operation.Input.Variables, err = sjson.SetRawBytes(v.operation.Input.Variables, unsafebytes.BytesToString(variableNameBytes), valueBytes)
	if err != nil {
		v.StopWithInternalErr(err)
		return
	}

	variable := ast.VariableValue{
		Name: v.operation.Input.AppendInputBytes(variableNameBytes),
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

func (v *variablesExtractionVisitor) appendArgumentDefaultInputFields(argumentRef int) {
	objectVal := v.operation.Arguments[argumentRef].Value
	if objectVal.Kind != ast.ValueKindObject {
		return
	}
	// get the argument definition
	parentNode := v.Ancestors[len(v.Ancestors)-1]
	if parentNode.Kind != ast.NodeKindField {
		return
	}
	inputValueDefinition, exists := v.ArgumentInputValueDefinition(argumentRef)
	if !exists {
		return
	}
	inputType := v.definition.InputValueDefinitions[inputValueDefinition].Type
	inputType = v.definition.BaseType(inputType)
	node, found := v.definition.Index.FirstNodeByNameBytes(v.definition.TypeNameBytes(inputType))
	if !found {
		return
	}
	if node.Kind == ast.NodeKindInputObjectTypeDefinition {
		v.recursiveInjectInputDefaultField(objectVal.Ref, node.Ref)
	}
	fmt.Println(node, found)
}

// TODO pass error
func (v *variablesExtractionVisitor) recursiveInjectInputDefaultField(objectValueRef, inputObjectDefRef int) {
	inputObjectDef := v.definition.InputObjectTypeDefinitions[inputObjectDefRef]
	for i := 0; i <= len(inputObjectDef.InputFieldsDefinition.Refs)-1; i++ {
		//get the object value of the field that corresponds to the input value name
		inputValueDefinitionRef := inputObjectDef.InputFieldsDefinition.Refs[i]

		// check if not null and append value
		inputFieldName := v.definition.InputValueDefinitionNameBytes(inputValueDefinitionRef)

		typeRef := v.definition.InputValueDefinitions[inputValueDefinitionRef].Type
		typeIsScalar := v.definition.TypeIsScalar(typeRef, v.definition)
		if v.definition.Types[typeRef].TypeKind != ast.TypeKindNonNull {
			continue
		}

		objectFieldRef := v.operation.ObjectValueObjectFieldByName(objectValueRef, inputFieldName)
		if objectFieldRef >= 0 {
			// field exists in query, check if object and move on if not
			// it is not -1, so it is present, move on to the next InputObjectField
			if typeIsScalar || v.definition.TypeIsList(typeRef) {
				continue
			}
			baseDef := v.definition.BaseType(typeRef)
			node, found := v.definition.Index.FirstNodeByNameBytes(v.definition.TypeNameBytes(baseDef))
			if found {
				valKind := v.operation.ObjectFields[objectFieldRef].Value.Kind
				if node.Kind == ast.NodeKindInputObjectTypeDefinition && valKind == ast.ValueKindObject {
					v.recursiveInjectInputDefaultField(v.operation.ObjectFields[objectFieldRef].Value.Ref, node.Ref)
				}
			}
		}

		if !v.definition.InputValueDefinitions[inputValueDefinitionRef].DefaultValue.IsDefined {
			return
		}
		if typeIsScalar {
			v.addDefaultValueToInput(objectValueRef, inputObjectDefRef, inputValueDefinitionRef)
			continue
		}
	}

}

func (v *variablesExtractionVisitor) addDefaultValueToInput(objectValueRef, inputObjectDefRef, inputValueDefinitionRef int) {
	defaultValue := v.definition.InputValueDefinitionDefaultValue(inputValueDefinitionRef)
	var valueRef int
	inputObjectName := v.definition.InputObjectTypeDefinitionNameString(inputObjectDefRef)
	inputValueName := v.definition.InputValueDefinitionNameString(inputValueDefinitionRef)
	switch defaultValue.Kind {
	case ast.ValueKindString:
		strVal := v.definition.InputObjectTypeDefinitionInputValueDefinitionDefaultValueString(inputObjectName, inputValueName)
		valueRef = v.operation.AddStringValue(ast.StringValue{
			BlockString: false,
			Content:     v.operation.Input.AppendInputString(strVal),
		})
	case ast.ValueKindInteger:
		intVal := v.definition.InputObjectTypeDefinitionInputValueDefinitionDefaultValueInt64(inputObjectName, inputValueName)
		valueRef = v.operation.AddIntValue(ast.IntValue{
			Negative: intVal < 0,
			Raw:      v.operation.Input.AppendInputString(fmt.Sprintf("%d", int(math.Abs(float64(intVal))))),
		})
	case ast.ValueKindBoolean:
		boolVal := v.definition.InputObjectTypeDefinitionInputValueDefinitionDefaultValueBool(inputObjectName, inputValueName)
		if boolVal {
			valueRef = 1
		} else {
			valueRef = 0
		}
	case ast.ValueKindFloat:
		floatVal := v.definition.InputObjectTypeDefinitionInputValueDefinitionDefaultValueFloat32(inputObjectName, inputValueName)
		valueRef = v.operation.AddFloatValue(ast.FloatValue{
			Negative: floatVal < 0,
			Raw:      v.operation.Input.AppendInputString(fmt.Sprintf("%f", math.Abs(float64(floatVal)))),
		})
	default:
		return
	}
	fieldRef := v.operation.AddObjectField(ast.ObjectField{
		Name: v.operation.Input.AppendInputString(inputValueName),
		Value: ast.Value{
			Kind: defaultValue.Kind,
			Ref:  valueRef,
		},
	})
	v.operation.ObjectValues[objectValueRef].Refs = append(v.operation.ObjectValues[objectValueRef].Refs, fieldRef)
}

func (v *variablesExtractionVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation, v.definition = operation, definition
}

func (v *variablesExtractionVisitor) traverseValue(value ast.Value, argRef, inputValueDefinition int) {
	switch value.Kind {
	case ast.ValueKindList:
		for _, ref := range v.operation.ListValues[value.Ref].Refs {
			listValue := v.operation.Value(ref)
			v.traverseValue(listValue, argRef, inputValueDefinition)
		}
	case ast.ValueKindObject:
		objectValueRefs := make([]int, len(v.operation.ObjectValues[value.Ref].Refs))
		copy(objectValueRefs, v.operation.ObjectValues[value.Ref].Refs)
		for _, ref := range objectValueRefs {
			fieldName := v.operation.Input.ByteSlice(v.operation.ObjectFields[ref].Name)
			fieldValue := v.operation.ObjectFields[ref].Value
			switch fieldValue.Kind {
			case ast.ValueKindVariable:
				continue
			default:

				typeName := v.definition.ResolveTypeNameString(v.definition.InputValueDefinitions[inputValueDefinition].Type)
				typeDefinitionNode, ok := v.definition.Index.FirstNodeByNameStr(typeName)
				if !ok {
					continue
				}
				objectFieldDefinition, ok := v.definition.NodeInputFieldDefinitionByName(typeDefinitionNode, fieldName)
				if !ok {
					continue
				}

				if v.operation.ValueContainsVariable(fieldValue) {
					v.traverseValue(fieldValue, argRef, objectFieldDefinition)
					continue
				}
				v.extractObjectValue(ref, fieldValue, objectFieldDefinition)
			}
		}
	}
}

func (v *variablesExtractionVisitor) extractObjectValue(objectField int, fieldValue ast.Value, inputValueDefinition int) {

	variableNameBytes := v.operation.GenerateUnusedVariableDefinitionName(v.Ancestors[0].Ref)
	valueBytes, err := v.operation.ValueToJSON(fieldValue)
	if err != nil {
		return
	}
	v.operation.Input.Variables, err = sjson.SetRawBytes(v.operation.Input.Variables, unsafebytes.BytesToString(variableNameBytes), valueBytes)
	if err != nil {
		v.StopWithInternalErr(err)
		return
	}

	variable := ast.VariableValue{
		Name: v.operation.Input.AppendInputBytes(variableNameBytes),
	}

	v.operation.VariableValues = append(v.operation.VariableValues, variable)

	varRef := len(v.operation.VariableValues) - 1

	v.operation.ObjectFields[objectField].Value.Kind = ast.ValueKindVariable
	v.operation.ObjectFields[objectField].Value.Ref = varRef

	defType := v.definition.InputValueDefinitions[inputValueDefinition].Type

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
