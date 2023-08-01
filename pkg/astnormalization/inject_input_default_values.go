package astnormalization

import (
	"errors"
	"fmt"

	"github.com/buger/jsonparser"
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
)

func injectInputFieldDefaults(walker *astvisitor.Walker) *inputFieldDefaultInjectionVisitor {
	visitor := &inputFieldDefaultInjectionVisitor{
		Walker:   walker,
		jsonPath: make([]string, 0),
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterVariableDefinitionVisitor(visitor)
	return visitor
}

type inputFieldDefaultInjectionVisitor struct {
	*astvisitor.Walker

	operation  *ast.Document
	definition *ast.Document

	variableName string
	jsonPath     []string
}

func (v *inputFieldDefaultInjectionVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation, v.definition = operation, definition
}

func (v *inputFieldDefaultInjectionVisitor) EnterVariableDefinition(ref int) {
	v.variableName = v.operation.VariableDefinitionNameString(ref)

	variableVal, _, _, err := jsonparser.Get(v.operation.Input.Variables, v.variableName)
	if err == jsonparser.KeyPathNotFoundError {
		return
	}
	if err != nil {
		v.StopWithInternalErr(err)
		return
	}

	typeRef := v.operation.VariableDefinitions[ref].Type
	if v.isScalarTypeOrExtension(typeRef, v.operation) {
		return
	}
	newVal, replaced, err := v.processObjectOrListInput(typeRef, variableVal, v.operation)
	if err != nil {
		v.StopWithInternalErr(err)
		return
	}
	if replaced {
		newVariables, err := jsonparser.Set(v.operation.Input.Variables, newVal, v.variableName)
		if err != nil {
			v.StopWithInternalErr(err)
			return
		}
		v.operation.Input.Variables = newVariables
	}
}

// recursiveInjectInputFields injects default values in input types starting from
// inputObjectRef and walking to its descendants. If no replacements are done it
// returns (varValue, false). If injecting a default value caused varValue to change
// it returns (newValue, true).
func (v *inputFieldDefaultInjectionVisitor) recursiveInjectInputFields(inputObjectRef int, varValue []byte) ([]byte, bool, error) {
	objectDef := v.definition.InputObjectTypeDefinitions[inputObjectRef]
	if !objectDef.HasInputFieldsDefinition {
		return varValue, false, nil
	}
	finalVal := varValue
	hasDoneAnyReplacements := false
	for _, ref := range objectDef.InputFieldsDefinition.Refs {
		valDef := v.definition.InputValueDefinitions[ref]
		fieldName := v.definition.InputValueDefinitionNameString(ref)
		isTypeScalarOrEnum := v.isScalarTypeOrExtension(valDef.Type, v.definition)
		hasDefault := valDef.DefaultValue.IsDefined

		varVal, _, _, err := jsonparser.Get(varValue, fieldName)
		if err != nil && err != jsonparser.KeyPathNotFoundError {
			v.StopWithInternalErr(err)
			return nil, false, err
		}
		existsInVal := err != jsonparser.KeyPathNotFoundError

		if !isTypeScalarOrEnum {
			var valToUse []byte
			if existsInVal {
				valToUse = varVal
			} else if hasDefault {
				defVal, err := v.definition.ValueToJSON(valDef.DefaultValue.Value)
				if err != nil {
					return nil, false, err
				}
				valToUse = defVal
			} else {
				continue
			}
			fieldValue, replaced, err := v.processObjectOrListInput(valDef.Type, valToUse, v.definition)
			if err != nil {
				return nil, false, err
			}
			if (!existsInVal && hasDefault) || replaced {
				finalVal, err = jsonparser.Set(finalVal, fieldValue, fieldName)
				if err != nil {
					return nil, false, err
				}
				hasDoneAnyReplacements = true
			}
			continue
		}

		if !hasDefault && isTypeScalarOrEnum {
			continue
		}
		if existsInVal {
			continue
		}
		defVal, err := v.definition.ValueToJSON(valDef.DefaultValue.Value)
		if err != nil {
			return nil, false, err
		}

		finalVal, err = jsonparser.Set(finalVal, defVal, fieldName)
		if err != nil {
			return nil, false, err
		}
		hasDoneAnyReplacements = true
	}
	return finalVal, hasDoneAnyReplacements, nil
}

func (v *inputFieldDefaultInjectionVisitor) isScalarTypeOrExtension(typeRef int, typeDoc *ast.Document) bool {
	if typeDoc.TypeIsScalar(typeRef, v.definition) || typeDoc.TypeIsEnum(typeRef, v.definition) {
		return true
	}
	typeName := typeDoc.TypeNameBytes(typeRef)
	node, found := v.definition.Index.FirstNonExtensionNodeByNameBytes(typeName)
	if !found {
		return false
	}
	switch node.Kind {
	case ast.NodeKindScalarTypeDefinition, ast.NodeKindEnumTypeDefinition:
		return true
	}
	return false
}

// processObjectOrListInput walks over an input object or list, assigning default values
// from the schema if necessary. If there are no changes to be made it (defaultValue, false),
// and if any value is replaced by its default in the schema it returns (newValue, true).
func (v *inputFieldDefaultInjectionVisitor) processObjectOrListInput(fieldType int, defaultValue []byte, typeDoc *ast.Document) ([]byte, bool, error) {
	fieldIsList := typeDoc.TypeIsList(fieldType)
	varVal, valType, _, err := jsonparser.Get(defaultValue)
	if err != nil {
		return nil, false, err

	}
	node, found := v.definition.Index.FirstNodeByNameBytes(typeDoc.ResolveTypeNameBytes(fieldType))
	if !found {
		return defaultValue, false, nil
	}
	if node.Kind == ast.NodeKindScalarTypeDefinition {
		return defaultValue, false, nil
	}
	finalVal := defaultValue
	replaced := false
	valIsList := valType == jsonparser.Array
	if fieldIsList && valIsList {
		_, err := jsonparser.ArrayEach(varVal, v.jsonWalker(typeDoc.ResolveListOrNameType(fieldType), defaultValue, &node, typeDoc, &finalVal, &replaced))
		if err != nil {
			return nil, false, err

		}
	} else if !fieldIsList && !valIsList {
		finalVal, replaced, err = v.recursiveInjectInputFields(node.Ref, defaultValue)
		if err != nil {
			return nil, false, err
		}
	} else {
		return nil, false, errors.New("mismatched input value")
	}
	return finalVal, replaced, nil
}

// jsonWalker returns a function for visiting an array using jsonparser.ArrayEach that recursively applies
// default values from the schema if necessary, using v.processObjectOrListInput() and v.recursiveInjectInputFields().
// If any changes are made, it returns (newValue, true), otherwise it returns (defaultValue, false).
func (v *inputFieldDefaultInjectionVisitor) jsonWalker(fieldType int, defaultValue []byte, node *ast.Node, typeDoc *ast.Document, finalVal *[]byte, finalValueReplaced *bool) func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
	i := 0
	listOfList := typeDoc.TypeIsList(typeDoc.Types[fieldType].OfType)
	return func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		if err != nil {
			return
		}
		if listOfList && dataType == jsonparser.Array {
			newVal, replaced, err := v.processObjectOrListInput(typeDoc.Types[fieldType].OfType, value, typeDoc)
			if err != nil {
				return
			}
			if replaced {
				*finalVal, err = jsonparser.Set(defaultValue, newVal, fmt.Sprintf("[%d]", i))
				if err != nil {
					return
				}
				*finalValueReplaced = true
			}
		} else if !listOfList && dataType == jsonparser.Object {
			newVal, replaced, err := v.recursiveInjectInputFields(node.Ref, value)
			if err != nil {
				return
			}
			if replaced {
				*finalVal, err = jsonparser.Set(defaultValue, newVal, fmt.Sprintf("[%d]", i))
				if err != nil {
					return
				}
				*finalValueReplaced = true
			}
		} else {
			return
		}
		i++
	}

}
func (v *inputFieldDefaultInjectionVisitor) LeaveVariableDefinition(ref int) {
	v.variableName = ""
	v.jsonPath = make([]string, 0)
}
