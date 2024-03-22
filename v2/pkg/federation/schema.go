package federation

import (
	"fmt"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func BuildFederationSchema(baseSchema, serviceSDL string) (string, error) {
	builder := schemaBuilder{}
	return builder.buildFederationSchema(baseSchema, serviceSDL)
}

// schemaBuilder makes GraphQL schemas compliant with the Apollo Federation Specification
type schemaBuilder struct {
}

// BuildFederationSchema takes a baseSchema plus the service sdl and turns it into a fully compliant federation schema
func (s *schemaBuilder) buildFederationSchema(baseSchema, serviceSDL string) (string, error) {
	unionTypes := s.entityUnionTypes(serviceSDL)
	hasEntities := len(unionTypes) != 0

	federatedSchema := federationTemplate
	if hasEntities {
		allUnionTypes := strings.Join(unionTypes, " | ")
		federatedSchema += fmt.Sprintf("\nunion _Entity = %s\n", allUnionTypes)
	}

	baseSchemaWithFederationFields := s.extendQueryTypeWithFederationFields(baseSchema, hasEntities)
	federatedSchema += "\n" + baseSchemaWithFederationFields

	return federatedSchema, nil
}

func (s *schemaBuilder) extendQueryTypeWithFederationFields(schema string, hasEntities bool) string {
	doc := ast.NewSmallDocument()
	doc.Input.ResetInputString(schema)
	parser := astparser.NewParser()
	report := &operationreport.Report{}
	parser.Parse(doc, report)
	if report.HasErrors() {
		return schema
	}

	if err := asttransform.MergeDefinitionWithBaseSchema(doc); err != nil {
		return schema
	}

	queryTypeName := doc.Index.QueryTypeName.String()
	if queryTypeName == "" {
		queryTypeName = "Query"
	}
	for i := range doc.ObjectTypeDefinitions {
		name := doc.ObjectTypeDefinitionNameString(i)
		if name == queryTypeName {
			s.extendQueryType(doc, i, hasEntities)
			out, err := astprinter.PrintStringIndent(doc, nil, "  ")
			if err != nil {
				return schema
			}
			return out
		}
	}
	return schema
}

func (s *schemaBuilder) extendQueryType(doc *ast.Document, ref int, hasEntities bool) {
	serviceType := doc.AddNonNullNamedType([]byte("_Service"))

	serviceFieldDefRef := doc.ImportFieldDefinition(
		"_service",
		"",
		serviceType,
		nil,
		nil,
	)

	doc.ObjectTypeDefinitions[ref].HasFieldDefinitions = true
	doc.ObjectTypeDefinitions[ref].FieldsDefinition.Refs = append(doc.ObjectTypeDefinitions[ref].FieldsDefinition.Refs, serviceFieldDefRef)

	if !hasEntities {
		return
	}

	anyType := doc.AddNonNullNamedType([]byte("_Any"))
	entityType := doc.AddNamedType([]byte("_Entity"))
	listOfAnyType := doc.AddListType(anyType)
	nonNullListOfAnyType := doc.AddNonNullType(listOfAnyType)
	listOfEntityType := doc.AddListType(entityType)
	nonNullListOfEntityType := doc.AddNonNullType(listOfEntityType)

	representationsArg := doc.ImportInputValueDefinition(
		"representations",
		"",
		nonNullListOfAnyType,
		ast.DefaultValue{})

	entitiesFDRef := doc.ImportFieldDefinition(
		"_entities",
		"",
		nonNullListOfEntityType,
		[]int{representationsArg},
		nil)

	doc.ObjectTypeDefinitions[ref].FieldsDefinition.Refs = append(doc.ObjectTypeDefinitions[ref].FieldsDefinition.Refs, entitiesFDRef)
}

// _entities(representations: [_Any!]!): [_Entity]!
// _service: _Service!

func (s *schemaBuilder) entityUnionTypes(serviceSDL string) []string {
	doc := ast.NewSmallDocument()
	doc.Input.ResetInputString(serviceSDL)
	parser := astparser.NewParser()
	report := &operationreport.Report{}
	parser.Parse(doc, report)
	if report.HasErrors() {
		return nil
	}

	walker := astvisitor.NewWalker(4)
	visitor := &schemaBuilderVisitor{}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterObjectTypeDefinitionVisitor(visitor)
	walker.RegisterEnterObjectTypeExtensionVisitor(visitor)
	walker.Walk(doc, nil, report)
	if report.HasErrors() {
		return nil
	}
	return visitor.entityUnionTypes
}

type schemaBuilderVisitor struct {
	definition       *ast.Document
	entityUnionTypes []string
}

func (s *schemaBuilderVisitor) addEntity(entity string) {
	for i := range s.entityUnionTypes {
		if s.entityUnionTypes[i] == entity {
			return
		}
	}
	s.entityUnionTypes = append(s.entityUnionTypes, entity)
}

func (s *schemaBuilderVisitor) EnterDocument(operation, _ *ast.Document) {
	s.definition = operation
}

func (s *schemaBuilderVisitor) EnterObjectTypeExtension(ref int) {
	for _, i := range s.definition.ObjectTypeExtensions[ref].Directives.Refs {
		if s.definition.DirectiveNameString(i) == "key" {
			s.addEntity(s.definition.ObjectTypeExtensionNameString(ref))
		}
	}
}

func (s *schemaBuilderVisitor) EnterObjectTypeDefinition(ref int) {
	for _, i := range s.definition.ObjectTypeDefinitions[ref].Directives.Refs {
		if s.definition.DirectiveNameString(i) == "key" {
			s.addEntity(s.definition.ObjectTypeDefinitionNameString(ref))
		}
	}
}

const federationTemplate = `scalar _Any
scalar _FieldSet

type _Service {
    sdl: String
}

directive @external on FIELD_DEFINITION
directive @requires(fields: _FieldSet!) on FIELD_DEFINITION
directive @provides(fields: _FieldSet!) on FIELD_DEFINITION
directive @key(fields: _FieldSet!) on OBJECT | INTERFACE
directive @extends on OBJECT | INTERFACE
directive @interfaceObject on OBJECT
`
