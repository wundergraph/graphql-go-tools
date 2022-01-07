package astnormalization

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astimport"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

// TODO: Take a look at this https://www.graphql.de/blog/scalars-in-depth/#input-coercion

var errIncorrectItemValue = fmt.Errorf("incorrect item value")

func inputCoercionForList(walker *astvisitor.Walker) {
	visitor := inputCoercionForListVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterArgumentVisitor(&visitor)
}

type inputCoercionForListVisitor struct {
	*astvisitor.Walker
	importer   astimport.Importer
	operation  *ast.Document
	definition *ast.Document
}

func (i *inputCoercionForListVisitor) valueSatisfiesScalar(value ast.Value, scalar int) bool {
	scalarName := i.definition.ScalarTypeDefinitionNameString(scalar)
	if value.Kind == ast.ValueKindVariable {
		variableName := i.operation.VariableValueNameBytes(value.Ref)
		variableDefinition, exists := i.operation.VariableDefinitionByNameAndOperation(i.Ancestors[0].Ref, variableName)
		if !exists {
			return false
		}
		variableTypeRef := i.operation.VariableDefinitions[variableDefinition].Type
		typeName := i.operation.ResolveTypeNameString(variableTypeRef)
		return scalarName == typeName
	}
	switch scalarName {
	case "Boolean":
		return value.Kind == ast.ValueKindBoolean
	case "Int":
		return value.Kind == ast.ValueKindInteger
	case "Float":
		return value.Kind == ast.ValueKindFloat || value.Kind == ast.ValueKindInteger
	default:
		return value.Kind == ast.ValueKindString
	}
}

func (i *inputCoercionForListVisitor) valueSatisfiesTypeDefinitionNode(value ast.Value, node ast.Node) bool {
	switch node.Kind {
	//case ast.NodeKindEnumTypeDefinition:
	//	return v.valueSatisfiesEnum(value, node)
	case ast.NodeKindScalarTypeDefinition:
		return i.valueSatisfiesScalar(value, node.Ref)
	//case ast.NodeKindInputObjectTypeDefinition:
	//	return v.valueSatisfiesInputObjectTypeDefinition(value, node.Ref)
	default:
		return false
	}
}

func (i *inputCoercionForListVisitor) valueSatisfiesListType(value ast.Value, listType int) bool {
	if value.Kind == ast.ValueKindVariable {
		variableName := i.operation.VariableValueNameBytes(value.Ref)
		variableDefinition, exists := i.operation.VariableDefinitionByNameAndOperation(i.Ancestors[0].Ref, variableName)
		if !exists {
			return false
		}
		actualType := i.operation.VariableDefinitions[variableDefinition].Type
		expectedType := i.importer.ImportType(listType, i.definition, i.operation)
		if i.operation.Types[actualType].TypeKind == ast.TypeKindNonNull {
			actualType = i.operation.Types[actualType].OfType
		}
		if i.operation.Types[actualType].TypeKind == ast.TypeKindList {
			actualType = i.operation.Types[actualType].OfType
		}
		return i.operation.TypesAreEqualDeep(expectedType, actualType)
	}

	if value.Kind != ast.ValueKindList {
		return false
	}

	if i.definition.Types[listType].TypeKind == ast.TypeKindNonNull {
		if len(i.operation.ListValues[value.Ref].Refs) == 0 {
			return false
		}
		listType = i.definition.Types[listType].OfType
	}

	for _, ref := range i.operation.ListValues[value.Ref].Refs {
		listValue := i.operation.Value(ref)
		if !i.valueSatisfiesInputValueDefinitionType(listValue, listType) {
			return false
		}
	}

	return true
}

func (i *inputCoercionForListVisitor) valueSatisfiesInputValueDefinitionType(value ast.Value, definitionTypeRef int) bool {
	switch i.definition.Types[definitionTypeRef].TypeKind {
	case ast.TypeKindNonNull:
		switch value.Kind {
		case ast.ValueKindNull:
			return false
		case ast.ValueKindVariable:
			variableName := i.operation.VariableValueNameBytes(value.Ref)
			variableDefinition, exists := i.operation.VariableDefinitionByNameAndOperation(i.Ancestors[0].Ref, variableName)
			if !exists {
				return false
			}
			variableTypeRef := i.operation.VariableDefinitions[variableDefinition].Type
			importedDefinitionType := i.importer.ImportType(definitionTypeRef, i.definition, i.operation)
			if !i.operation.TypesAreEqualDeep(importedDefinitionType, variableTypeRef) {
				return false
			}
		}
		return i.valueSatisfiesInputValueDefinitionType(value, i.definition.Types[definitionTypeRef].OfType)
	case ast.TypeKindNamed:
		typeName := i.definition.ResolveTypeNameBytes(definitionTypeRef)
		node, exists := i.definition.Index.FirstNodeByNameBytes(typeName)
		if !exists {
			return false
		}
		return i.valueSatisfiesTypeDefinitionNode(value, node)
	case ast.TypeKindList:
		return i.valueSatisfiesListType(value, i.definition.Types[definitionTypeRef].OfType)
	default:
		return false
	}
}

func (i *inputCoercionForListVisitor) inputCoercionForList(value ast.Value, ref, defType int) {
	l := ast.ListValue{}
	if !i.valueSatisfiesInputValueDefinitionType(value, defType) {
		i.StopWithInternalErr(errIncorrectItemValue)
		return
	}
	l.Refs = append(l.Refs, i.operation.ListValues[value.Ref].Refs...)
	listRef := i.operation.AddListValue(l)
	listValue := ast.Value{
		Kind: ast.ValueKindList,
		Ref:  listRef,
	}
	i.operation.AddValue(i.operation.Arguments[ref].Value)
	i.operation.Arguments[ref].Value = listValue
}

func (i *inputCoercionForListVisitor) EnterArgument(ref int) {
	defRef, ok := i.ArgumentInputValueDefinition(ref)
	if !ok {
		return
	}

	defType := i.definition.InputValueDefinitions[defRef].Type
	typeKind := i.definition.Types[defType].TypeKind
	if typeKind != ast.TypeKindList {
		return
	}

	value := i.operation.Arguments[ref].Value

	l := ast.ListValue{}
	switch i.operation.Arguments[ref].Value.Kind {
	case ast.ValueKindNull:
		return
	case ast.ValueKindList:
		i.inputCoercionForList(value, ref, defType)
	default:
		var latestRef = i.operation.AddValue(i.operation.Arguments[ref].Value)
		var definitionTypeRef = defType
		for {
			definitionTypeRef = i.definition.Types[definitionTypeRef].OfType
			if i.definition.Types[definitionTypeRef].TypeKind == ast.TypeKindList {
				innerList := ast.ListValue{}
				innerList.Refs = []int{latestRef}
				listRef := i.operation.AddListValue(innerList)
				listValue := ast.Value{
					Kind: ast.ValueKindList,
					Ref:  listRef,
				}
				latestRef = i.operation.AddValue(listValue)
				continue
			}
			break
		}
		if !i.valueSatisfiesInputValueDefinitionType(value, definitionTypeRef) {
			i.StopWithInternalErr(errIncorrectItemValue)
			return
		}
		l.Refs = []int{latestRef}
		listRef := i.operation.AddListValue(l)
		listValue := ast.Value{
			Kind: ast.ValueKindList,
			Ref:  listRef,
		}
		i.operation.Arguments[ref].Value = listValue
	}
}

func (i *inputCoercionForListVisitor) EnterDocument(operation, definition *ast.Document) {
	i.operation, i.definition = operation, definition
}
