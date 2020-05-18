package introspection

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astimport"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type JsonConverter struct {
	schema *Schema
	doc    *ast.Document
	parser *astparser.Parser
}

func (j *JsonConverter) GraphQLDocument(introspectionJSON io.Reader) (*ast.Document, error) {
	var data Data
	if err := json.NewDecoder(introspectionJSON).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse inrospection json: %v", err)
	}

	j.schema = &data.Schema
	j.doc = ast.NewDocument()
	j.parser = astparser.NewParser()

	if err := j.importSchema(); err != nil {
		return nil, fmt.Errorf("failed to convert graphql schema: %v", err)
	}

	return j.doc, nil
}

func (j *JsonConverter) importSchema() error {
	j.doc.ImportSchemaDefinition(j.schema.TypeNames())

	for _, fullType := range j.schema.Types {
		if err := j.importFullType(fullType); err != nil {
			return err
		}
	}

	for _, directive := range j.schema.Directives {
		j.importDirective(directive)
	}

	return nil
}

func (j *JsonConverter) importFullType(fullType FullType) error {
	switch fullType.Kind {
	case SCALAR:
		j.importScalar(fullType)
	case OBJECT:
		return j.importObject(fullType)
	case ENUM:
		j.importEnum(fullType)
	case INTERFACE:
		j.importInterface(fullType)
	case UNION:
		j.importUnion(fullType)
	case INPUTOBJECT:
		j.importInputObject(fullType)
	}

	return nil
}

func (j *JsonConverter) importDirective(directive Directive) {
	// TODO: implement
}

func (j *JsonConverter) importObject(fullType FullType) error {
	fieldRefs := make([]int, 0, len(fullType.Fields))
	for _, field := range fullType.Fields {
		fieldRef, err := j.importField(field)
		if err != nil {
			return err
		}
		fieldRefs = append(fieldRefs, fieldRef)
	}

	iRefs := make([]int, 0, len(fullType.Interfaces))
	for _, ref := range fullType.Interfaces {
		iRefs = append(iRefs, j.importType(ref))
	}

	j.doc.ImportObjectTypeDefinition(
		fullType.Name,
		fullType.Description,
		fieldRefs,
		iRefs)

	return nil
}

func (j *JsonConverter) importField(field Field) (ref int, err error) {
	typeRef := j.importType(field.Type)

	argRefs := make([]int, 0, len(field.Args))
	for _, arg := range field.Args {
		argRef, err := j.importArgument(arg)
		if err != nil {
			return -1, err
		}
		argRefs = append(argRefs, argRef)
	}

	return j.doc.ImportFieldDefinition(
		field.Name, field.Description, typeRef, argRefs), nil
}

func (j *JsonConverter) importArgument(value InputValue) (ref int, err error) {
	typeRef := j.importType(value.Type)

	defaultValue, err := j.importDefaultValue(value.DefaultValue)
	if err != nil {
		return -1, err
	}

	return j.doc.ImportInputValueDefinition(
		value.Name, value.Description, typeRef, defaultValue), nil
}

func (j *JsonConverter) importType(typeRef TypeRef) (ref int) {
	switch typeRef.Kind {
	case LIST:
		listType := ast.Type{
			TypeKind: ast.TypeKindList,
			OfType:   j.importType(*typeRef.OfType),
		}
		return j.doc.AddType(listType)
	case NONNULL:
		nonNullType := ast.Type{
			TypeKind: ast.TypeKindNonNull,
			OfType:   j.importType(*typeRef.OfType),
		}
		return j.doc.AddType(nonNullType)
	}

	return j.doc.AddNamedType([]byte(*typeRef.Name))
}

func (j *JsonConverter) importScalar(fullType FullType) {
	// TODO: implement
}

func (j *JsonConverter) importEnum(fullType FullType) {
	// TODO: implement
}

func (j *JsonConverter) importInterface(fullType FullType) {
	// TODO: implement
}

func (j *JsonConverter) importUnion(fullType FullType) {
	// TODO: implement
}

func (j *JsonConverter) importInputObject(fullType FullType) {
	// TODO: implement
}

func (j *JsonConverter) importDefaultValue(defaultValue *string) (out ast.DefaultValue, err error) {
	if defaultValue == nil {
		return
	}

	from := ast.NewDocument()
	from.Input.AppendInputString(*defaultValue)

	report := &operationreport.Report{}

	j.parser.PrepareImport(from, report)
	value := j.parser.ParseValue()

	if report.HasErrors() {
		err = report
		return
	}

	importer := &astimport.Importer{}
	return ast.DefaultValue{
		IsDefined: true,
		Value:     importer.ImportValue(value, from, j.doc),
	}, nil
}
