package asttransform

import (
	"bytes"
	_ "embed"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

var (
	//go:embed base.graphql
	baseSchema []byte

	//go:embed defer_internal.graphql
	deferInternal []byte

	//go:embed defer.graphql
	deferRegular []byte
)

type Options struct {
	InternalDefer bool
}

func MergeDefinitionWithBaseSchema(definition *ast.Document) error {
	return MergeDefinitionWithBaseSchemaWithOptions(definition, Options{})
}

func MergeDefinitionWithBaseSchemaWithOptions(definition *ast.Document, options Options) error {
	definition.Input.AppendInputBytes(baseSchema)
	if options.InternalDefer {
		definition.Input.AppendInputBytes(deferInternal)
	} else {
		definition.Input.AppendInputBytes(deferRegular)
	}

	parser := astparser.NewParser()
	report := operationreport.Report{}
	parser.Parse(definition, &report)
	if report.HasErrors() {
		return report
	}
	return handleSchema(definition)
}

func handleSchema(definition *ast.Document) error {
	var queryNodeRef int
	queryNode, hasQueryNode := findQueryNode(definition)
	if hasQueryNode {
		queryNodeRef = queryNode.Ref
	} else {
		queryNodeRef = definition.ImportObjectTypeDefinition("Query", "", nil, nil)
	}

	addSchemaDefinition(definition)
	addMissingRootOperationTypeDefinitions(definition)
	addIntrospectionQueryFields(definition, queryNodeRef)

	typeNamesVisitor := NewTypeNameVisitor()

	return typeNamesVisitor.ExtendSchema(definition)
}

func addSchemaDefinition(definition *ast.Document) {
	if definition.HasSchemaDefinition() {
		return
	}

	schemaDefinition := ast.SchemaDefinition{}
	definition.AddSchemaDefinitionRootNode(schemaDefinition)
}

func addMissingRootOperationTypeDefinitions(definition *ast.Document) {
	var rootOperationTypeRefs []int

	for i := range definition.RootNodes {
		if definition.RootNodes[i].Kind == ast.NodeKindObjectTypeDefinition {
			typeName := definition.ObjectTypeDefinitionNameBytes(definition.RootNodes[i].Ref)

			switch {
			case bytes.Equal(typeName, ast.DefaultQueryTypeName):
				rootOperationTypeRefs = createRootOperationTypeIfNotExists(definition, rootOperationTypeRefs, ast.OperationTypeQuery, i)
			case bytes.Equal(typeName, ast.DefaultMutationTypeName):
				rootOperationTypeRefs = createRootOperationTypeIfNotExists(definition, rootOperationTypeRefs, ast.OperationTypeMutation, i)
			case bytes.Equal(typeName, ast.DefaultSubscriptionTypeName):
				rootOperationTypeRefs = createRootOperationTypeIfNotExists(definition, rootOperationTypeRefs, ast.OperationTypeSubscription, i)
			default:
				continue
			}
		}
	}

	definition.SchemaDefinitions[definition.SchemaDefinitionRef()].AddRootOperationTypeDefinitionRefs(rootOperationTypeRefs...)
}

func createRootOperationTypeIfNotExists(definition *ast.Document, rootOperationTypeRefs []int, operationType ast.OperationType, nodeRef int) []int {
	for i := range definition.RootOperationTypeDefinitions {
		if definition.RootOperationTypeDefinitions[i].OperationType == operationType {
			return rootOperationTypeRefs
		}
	}

	ref := definition.CreateRootOperationTypeDefinition(operationType, nodeRef)
	return append(rootOperationTypeRefs, ref)
}

func addIntrospectionQueryFields(definition *ast.Document, objectTypeDefinitionRef int) {
	var fieldRefs []int
	if !definition.ObjectTypeDefinitionHasField(objectTypeDefinitionRef, []byte("__schema")) {
		fieldRefs = append(fieldRefs, addSchemaField(definition))
	}

	if !definition.ObjectTypeDefinitionHasField(objectTypeDefinitionRef, []byte("__type")) {
		fieldRefs = append(fieldRefs, addTypeField(definition))
	}

	definition.ObjectTypeDefinitions[objectTypeDefinitionRef].FieldsDefinition.Refs = append(definition.ObjectTypeDefinitions[objectTypeDefinitionRef].FieldsDefinition.Refs, fieldRefs...)
	definition.ObjectTypeDefinitions[objectTypeDefinitionRef].HasFieldDefinitions = true
}

func addSchemaField(definition *ast.Document) (ref int) {
	fieldNameRef := definition.Input.AppendInputBytes([]byte("__schema"))
	fieldTypeRef := definition.AddNonNullNamedType([]byte("__Schema"))

	return definition.AddFieldDefinition(ast.FieldDefinition{
		Name: fieldNameRef,
		Type: fieldTypeRef,
	})
}

func addTypeField(definition *ast.Document) (ref int) {
	fieldNameRef := definition.Input.AppendInputBytes([]byte("__type"))
	fieldTypeRef := definition.AddNamedType([]byte("__Type"))

	argumentNameRef := definition.Input.AppendInputBytes([]byte("name"))
	argumentTypeRef := definition.AddNonNullNamedType([]byte("String"))

	argumentRef := definition.AddInputValueDefinition(ast.InputValueDefinition{
		Name: argumentNameRef,
		Type: argumentTypeRef,
	})

	return definition.AddFieldDefinition(ast.FieldDefinition{
		Name: fieldNameRef,
		Type: fieldTypeRef,

		HasArgumentsDefinitions: true,
		ArgumentsDefinition: ast.InputValueDefinitionList{
			Refs: []int{argumentRef},
		},
	})
}

func findQueryNode(definition *ast.Document) (queryNode ast.Node, ok bool) {
	queryNode, ok = definition.Index.FirstNodeByNameBytes(definition.Index.QueryTypeName)
	if !ok {
		queryNode, ok = definition.Index.FirstNodeByNameStr("Query")
	}

	return queryNode, ok
}
