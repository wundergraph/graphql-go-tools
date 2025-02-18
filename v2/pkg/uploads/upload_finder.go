package uploads

import (
	"bytes"
	"fmt"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

var (
	uploadScalarName      = []byte("Upload")
	variablesPropertyName = []byte("variables")
)

type UploadFinder struct {
	visitor *uploadFinderVisitor
	walker  *astvisitor.Walker
}

func NewUploadFinder() *UploadFinder {
	walker := astvisitor.NewWalker(8)
	visitor := &uploadFinderVisitor{
		walker: &walker,
	}
	walker.RegisterEnterVariableDefinitionVisitor(visitor)
	walker.RegisterEnterDocumentVisitor(visitor)
	return &UploadFinder{
		walker:  &walker,
		visitor: visitor,
	}
}

func (v *UploadFinder) FindUploads(operation, definition *ast.Document, variables []byte) (paths []string, err error) {
	v.visitor.definition = definition
	v.visitor.operation = operation
	v.visitor.variables, err = astjson.ParseBytesWithoutCache(variables)
	if err != nil {
		return paths, err
	}
	report := &operationreport.Report{}
	v.walker.Walk(operation, definition, report)
	if report.HasErrors() {
		return paths, report
	}

	if len(v.visitor.uploadPaths) > 0 {
		return v.visitor.uploadPaths, nil
	}

	return nil, nil
}

type uploadFinderVisitor struct {
	walker               *astvisitor.Walker
	operation            *ast.Document
	definition           *ast.Document
	variables            *astjson.Value
	currentVariableName  []byte
	currentVariableValue *astjson.Value
	path                 []pathItem
	uploadPaths          []string
}

func (v *uploadFinderVisitor) renderPath() string {
	out := &bytes.Buffer{}
	for i, item := range v.path {
		if i > 0 {
			out.WriteString(".")
		}
		out.Write(item.name)
		if item.kind == pathItemKindArray {
			out.WriteString(fmt.Sprintf("%d", item.arrayIndex))
		}
	}
	return out.String()
}

type pathItemKind int

const (
	pathItemKindObject pathItemKind = iota
	pathItemKindArray
)

type pathItem struct {
	kind       pathItemKind
	name       []byte
	arrayIndex int
}

func (v *uploadFinderVisitor) pushObjectPath(name []byte) {
	v.path = append(v.path, pathItem{
		kind: pathItemKindObject,
		name: name,
	})
}

func (v *uploadFinderVisitor) pushArrayPath(index int) {
	v.path = append(v.path, pathItem{
		kind:       pathItemKindArray,
		arrayIndex: index,
	})
}

func (v *uploadFinderVisitor) popPath() {
	v.path = v.path[:len(v.path)-1]
}

func (v *uploadFinderVisitor) EnterDocument(operation, definition *ast.Document) {
	if v.uploadPaths != nil {
		v.uploadPaths = v.uploadPaths[:0]
	}
}

func (v *uploadFinderVisitor) EnterVariableDefinition(ref int) {
	varTypeRef := v.operation.VariableDefinitions[ref].Type
	operationVariableNameBytes := v.operation.VariableValueNameBytes(v.operation.VariableDefinitions[ref].VariableValue.Ref)
	operationVariableName := unsafebytes.BytesToString(operationVariableNameBytes)

	v.currentVariableName = operationVariableNameBytes
	v.currentVariableValue = v.variables.Get(operationVariableName)

	v.path = v.path[:0]
	v.pushObjectPath(variablesPropertyName)
	v.pushObjectPath(v.currentVariableName)
	v.traverseOperationType(v.currentVariableValue, varTypeRef)
}

func (v *uploadFinderVisitor) traverseOperationType(jsonValue *astjson.Value, operationTypeRef int) {
	if jsonValue == nil {
		return
	}

	if v.operation.TypeIsNonNull(operationTypeRef) {
		v.traverseOperationType(jsonValue, v.operation.Types[operationTypeRef].OfType)
		return
	}

	if v.operation.TypeIsList(operationTypeRef) {
		if jsonValue.Type() != astjson.TypeArray {
			return
		}
		values := jsonValue.GetArray()
		for i, arrayValue := range values {
			v.pushArrayPath(i)
			v.traverseOperationType(arrayValue, v.operation.Types[operationTypeRef].OfType)
			v.popPath()
			continue
		}
		return
	}

	varTypeName := v.operation.ResolveTypeNameBytes(operationTypeRef)
	if jsonValue.Type() == astjson.TypeNull && varTypeName.String() == "Upload" {
		v.uploadPaths = append(v.uploadPaths, v.renderPath())
		return
	}

	v.traverseNamedTypeNode(jsonValue, varTypeName)
}

func (v *uploadFinderVisitor) traverseFieldDefinitionType(fieldTypeDefinitionNodeKind ast.NodeKind, fieldName ast.ByteSlice, jsonValue *astjson.Value, typeRef, inputFieldRef int) {
	if v.definition.TypeIsNonNull(typeRef) {
		v.traverseFieldDefinitionType(fieldTypeDefinitionNodeKind, fieldName, jsonValue, v.definition.Types[typeRef].OfType, inputFieldRef)
		return
	}

	if v.definition.TypeIsList(typeRef) {
		if jsonValue.Type() != astjson.TypeArray {
			return
		}
		if len(jsonValue.GetArray()) == 0 {
			return
		}

		for i, arrayValue := range jsonValue.GetArray() {
			v.pushArrayPath(i)
			v.traverseFieldDefinitionType(fieldTypeDefinitionNodeKind, fieldName, arrayValue, v.definition.Types[typeRef].OfType, inputFieldRef)
			v.popPath()
			continue
		}
		return
	}

	v.traverseNamedTypeNode(jsonValue, v.definition.ResolveTypeNameBytes(typeRef))
}

func (v *uploadFinderVisitor) traverseNamedTypeNode(jsonValue *astjson.Value, typeName []byte) {
	fieldTypeDefinitionNode, ok := v.definition.NodeByName(typeName)
	if !ok {
		return
	}
	switch fieldTypeDefinitionNode.Kind {
	case ast.NodeKindInputObjectTypeDefinition:
		if jsonValue.Type() != astjson.TypeObject {
			return
		}
		inputFieldRefs := v.definition.NodeInputFieldDefinitions(fieldTypeDefinitionNode)
		for _, inputFieldRef := range inputFieldRefs {
			fieldName := v.definition.InputValueDefinitionNameBytes(inputFieldRef)
			fieldTypeRef := v.definition.InputValueDefinitionType(inputFieldRef)
			objectFieldValue := jsonValue.Get(unsafebytes.BytesToString(fieldName))

			v.pushObjectPath(fieldName)
			v.traverseFieldDefinitionType(fieldTypeDefinitionNode.Kind, fieldName, objectFieldValue, fieldTypeRef, inputFieldRef)
			v.popPath()
		}
	case ast.NodeKindScalarTypeDefinition:
		if bytes.Equal(typeName, uploadScalarName) && jsonValue.Type() == astjson.TypeNull {
			v.uploadPaths = append(v.uploadPaths, v.renderPath())
		}
	default:
	}
}
