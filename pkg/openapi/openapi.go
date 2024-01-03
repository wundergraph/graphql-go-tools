package openapi

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/TykTechnologies/graphql-go-tools/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/pkg/introspection"
	"github.com/TykTechnologies/graphql-go-tools/pkg/operationreport"
	"github.com/getkin/kin-openapi/openapi3"
)

var (
	errTypeNameExtractionImpossible = errors.New("type name extraction is impossible")
	errNotPrimitiveType             = errors.New("not a primitive type")
)

type converter struct {
	openapi         *openapi3.T
	knownFullTypes  map[string]*knownFullTypeDetails
	knownEnums      map[string]*introspection.FullType
	fullTypes       []introspection.FullType
	currentPathName string
	currentPathItem *openapi3.PathItem
}

type knownFullTypeDetails struct {
	hasDescription bool
}

func ImportParsedOpenAPIv3Document(document *openapi3.T, report *operationreport.Report) *ast.Document {
	c := &converter{
		openapi:        document,
		knownFullTypes: make(map[string]*knownFullTypeDetails),
		knownEnums:     make(map[string]*introspection.FullType),
		fullTypes:      make([]introspection.FullType, 0),
	}
	data := introspection.Data{}

	queryType, err := c.importQueryType()
	if err != nil {
		report.AddInternalError(err)
		return nil
	}
	if len(queryType.Fields) > 0 {
		data.Schema.QueryType = &introspection.TypeName{
			Name: "Query",
		}
		data.Schema.Types = append(data.Schema.Types, *queryType)
	}

	mutationType, err := c.importMutationType()
	if err != nil {
		report.AddInternalError(err)
		return nil
	}
	if len(mutationType.Fields) > 0 {
		data.Schema.MutationType = &introspection.TypeName{
			Name: "Mutation",
		}
		data.Schema.Types = append(data.Schema.Types, *mutationType)
	}

	fullTypes, err := c.importFullTypes()
	if err != nil {
		report.AddInternalError(err)
		return nil
	}
	data.Schema.Types = append(data.Schema.Types, fullTypes...)

	outputPretty, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		report.AddInternalError(err)
		return nil
	}

	jc := introspection.JsonConverter{}
	buf := bytes.NewBuffer(outputPretty)
	doc, err := jc.GraphQLDocument(buf)
	if err != nil {
		report.AddInternalError(err)
		return nil
	}
	return doc
}

func ParseOpenAPIDocument(input []byte) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	document, err := loader.LoadFromData(input)
	if err != nil {
		return nil, err
	}
	if err = document.Validate(loader.Context); err != nil {
		return nil, err
	}
	return document, nil
}

func ImportOpenAPIDocumentByte(input []byte) (*ast.Document, operationreport.Report) {
	report := operationreport.Report{}
	document, err := ParseOpenAPIDocument(input)
	if err != nil {
		report.AddInternalError(err)
		return nil, report
	}
	return ImportParsedOpenAPIv3Document(document, &report), report
}

func ImportOpenAPIDocumentString(input string) (*ast.Document, operationreport.Report) {
	return ImportOpenAPIDocumentByte([]byte(input))
}
