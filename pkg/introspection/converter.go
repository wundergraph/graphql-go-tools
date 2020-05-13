package introspection

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
)

type JsonConverter struct {
	schema *Schema
	doc    *ast.Document
}

func (i *JsonConverter) GraphQLDocument(introspectionJSON io.Reader) (*ast.Document, error) {
	var data Data
	if err := json.NewDecoder(introspectionJSON).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse inrospection json: %v", err)
	}

	i.schema = &data.Schema
	i.doc = ast.NewDocument()

	i.importSchema()

	return i.doc, nil
}

func (i *JsonConverter) importSchema() {
	i.doc.ImportSchemaDefinition(i.schema.TypeNames())

	for _, fullType := range i.schema.Types {
		i.importFullType(fullType)
	}

	for _, directive := range i.schema.Directives {
		i.importDirective(directive)
	}
}

func (i *JsonConverter) importFullType(fullType FullType) {
	switch fullType.Kind {
	case SCALAR:
		i.importScalar(fullType)
	case OBJECT:
		i.importObject(fullType)
	case ENUM:
		i.importEnum(fullType)
	case INTERFACE:
		i.importInterface(fullType)
	case UNION:
		i.importUnion(fullType)
	case INPUTOBJECT:
		i.importInputObject(fullType)
	}
}

func (i *JsonConverter) importDirective(directive Directive) {
	// TODO: implement
}

func (i *JsonConverter) importObject(fullType FullType) {
	fieldRefs := make([]int, 0, len(fullType.Fields))
	for _, field := range fullType.Fields {
		fieldRefs = append(fieldRefs, i.importField(field))
	}

	// TODO: import description
	// TODO: import implements
	i.importDescription()

	objectName := i.doc.Input.AppendInputString(fullType.Name)
	objectTypeDef := ast.ObjectTypeDefinition{
		Name:                objectName,
		HasFieldDefinitions: len(fieldRefs) > 0,
		FieldsDefinition: ast.FieldDefinitionList{
			Refs: fieldRefs,
		},
	}

	i.doc.ObjectTypeDefinitions = append(i.doc.ObjectTypeDefinitions, objectTypeDef)
	ref := len(i.doc.ObjectTypeDefinitions) - 1

	objectTypeNode := ast.Node{
		Kind: ast.NodeKindObjectTypeDefinition,
		Ref:  ref,
	}

	i.doc.RootNodes = append(i.doc.RootNodes, objectTypeNode)
	i.doc.Index.Add(fullType.Name, objectTypeNode)
}

func (i *JsonConverter) importField(field Field) (ref int) {
	// TODO: import description
	// TODO: import args
	typeRef := i.importType(field.Type)

	return i.doc.ImportFieldDefinition(field.Name, typeRef)
}

func (i *JsonConverter) importType(typeRef TypeRef) (ref int) {
	switch typeRef.Kind {
	case LIST:
		listType := ast.Type{
			TypeKind: ast.TypeKindList,
			OfType:   i.importType(*typeRef.OfType),
		}
		return i.doc.AddType(listType)
	case NONNULL:
		nonNullType := ast.Type{
			TypeKind: ast.TypeKindNonNull,
			OfType:   i.importType(*typeRef.OfType),
		}
		return i.doc.AddType(nonNullType)
	}

	return i.doc.AddNamedType([]byte(*typeRef.Name))
}

func (i *JsonConverter) importDescription() {
	// TODO: implement
}

func (i *JsonConverter) importScalar(fullType FullType) {
	// TODO: implement
}

func (i *JsonConverter) importEnum(fullType FullType) {
	// TODO: implement
}

func (i *JsonConverter) importInterface(fullType FullType) {
	// TODO: implement
}

func (i *JsonConverter) importUnion(fullType FullType) {
	// TODO: implement
}

func (i *JsonConverter) importInputObject(fullType FullType) {
	// TODO: implement
}
