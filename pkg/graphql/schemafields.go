package graphql

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/introspection"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

var DefaultSchemaFieldsGenerator SchemaFieldsGenerator = schemaFieldsGenerator{}

type (
	SchemaType struct {
		Name   string        `json:"name"`
		Fields []SchemaField `json:"fields"`
	}

	SchemaField struct {
		Name    string  `json:"name"`
		Type    string  `json:"type"`
		TypeRef *string `json:"type_ref"`
	}

	SchemaFieldsGenerator interface {
		Generate(schema *ast.Document) (SchemaFieldsResult, error)
	}
)

type schemaFieldsGenerator struct{}

func (g schemaFieldsGenerator) Generate(schema *ast.Document) (SchemaFieldsResult, error) {
	if schema == nil {
		return SchemaFieldsResult{}, ErrNilSchema
	}

	var (
		report operationreport.Report
		data   introspection.Data
	)

	generator := introspection.NewGenerator()
	generator.Generate(schema, &report, &data)

	if report.HasErrors() {
		return schemaFieldsResult(nil, report)
	}

	types := g.extractTypes(&data)
	return schemaFieldsResult(types, report)
}

func (g schemaFieldsGenerator) extractTypes(data *introspection.Data) []SchemaType {
	var types []SchemaType

	return types
}

type SchemaFieldsResult struct {
	Types  []SchemaType
	Errors Errors
}

func schemaFieldsResult(types []SchemaType, report operationreport.Report) (SchemaFieldsResult, error) {
	result := SchemaFieldsResult{
		Types:  types,
		Errors: nil,
	}

	if !report.HasErrors() {
		return result, nil
	}

	result.Errors = operationValidationErrorsFromOperationReport(report)

	var err error
	if len(report.InternalErrors) > 0 {
		err = report.InternalErrors[0]
	}

	return result, err
}
