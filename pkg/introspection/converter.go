package introspection

import (
	"encoding/json"
	"io"

	"github.com/cespare/xxhash"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
)

type JsonConverter struct {
	schema *Schema
	doc    *ast.Document
}

func (i *JsonConverter) GraphQLDocument(introspectionJSON io.Reader) *ast.Document {
	var data Data
	err := json.NewDecoder(introspectionJSON).Decode(&data)
	if err != nil {
		// TODO: handle error
	}
	doc := ast.NewDocument()

	i.importSchema(&data.Schema, doc)

	return doc
}

func (i *JsonConverter) importSchema(schema *Schema, doc *ast.Document) {
	i.schema = schema
	i.doc = doc

	i.importRootOperations()

	for _, fullType := range i.schema.Types {
		i.importFullType(fullType)
	}

	for _, directive := range i.schema.Directives {
		i.importDirective(directive)
	}
}

func (i *JsonConverter) importRootOperations() {
	var operationRefs []int

	if i.schema.QueryType != nil {
		operationRefs = append(operationRefs, i.importRootOperation(i.schema.QueryType.Name, ast.OperationTypeQuery))
	}
	if i.schema.MutationType != nil {
		operationRefs = append(operationRefs, i.importRootOperation(i.schema.MutationType.Name, ast.OperationTypeMutation))
	}
	if i.schema.SubscriptionType != nil {
		operationRefs = append(operationRefs, i.importRootOperation(i.schema.SubscriptionType.Name, ast.OperationTypeSubscription))
	}

	schemaDefinition := ast.SchemaDefinition{
		RootOperationTypeDefinitions: ast.RootOperationTypeDefinitionList{
			Refs: operationRefs,
		},
	}

	i.doc.SchemaDefinitions = append(i.doc.SchemaDefinitions, schemaDefinition)
	schemaDefinitionRef := len(i.doc.SchemaDefinitions) - 1

	// add the SchemaDefinition to the RootNodes
	// all root level nodes have to be added to the RootNodes slice in order to make them available to the Walker for traversal
	i.doc.RootNodes = append(i.doc.RootNodes, ast.Node{Kind: ast.NodeKindSchemaDefinition, Ref: schemaDefinitionRef})
}

func (i *JsonConverter) importRootOperation(name string, operationType ast.OperationType) int {
	typeName := i.doc.Input.AppendInputString(name)

	operationTypeDefinition := ast.RootOperationTypeDefinition{
		OperationType: operationType,
		NamedType: ast.Type{
			Name: typeName,
		},
	}

	i.doc.RootOperationTypeDefinitions = append(i.doc.RootOperationTypeDefinitions, operationTypeDefinition)
	ref := len(i.doc.RootOperationTypeDefinitions) - 1
	return ref
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
	i.doc.Index.Nodes[xxhash.Sum64String(fullType.Name)] = objectTypeNode
}

func (i *JsonConverter) importField(field Field) (ref int) {
	fieldName := i.doc.Input.AppendInputString(field.Name)

	// TODO: import description
	// TODO: import args

	typeRef := i.importType(field.Type)

	helloFieldDefinition := ast.FieldDefinition{
		Name: fieldName,
		Type: typeRef,
	}

	i.doc.FieldDefinitions = append(i.doc.FieldDefinitions, helloFieldDefinition)
	ref = len(i.doc.FieldDefinitions) - 1
	return
}

func (i *JsonConverter) importType(typeRef TypeRef) (ref int) {
	switch typeRef.Kind {
	case LIST:
		listType := ast.Type{
			TypeKind: ast.TypeKindList,
			OfType:   i.importType(*typeRef.OfType),
		}
		return i.importAstType(listType)
	case NONNULL:
		nonNullType := ast.Type{
			TypeKind: ast.TypeKindNonNull,
			OfType:   i.importType(*typeRef.OfType),
		}
		return i.importAstType(nonNullType)
	}

	name := i.doc.Input.AppendInputString(*typeRef.Name)
	astType := ast.Type{
		TypeKind: ast.TypeKindNamed,
		Name:     name,
	}

	return i.importAstType(astType)
}

func (i *JsonConverter) importAstType(t ast.Type) (ref int) {
	i.doc.Types = append(i.doc.Types, t)
	ref = len(i.doc.Types) - 1
	return
}

func (i *JsonConverter) importDescription() (ref int) {
	return 0
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
