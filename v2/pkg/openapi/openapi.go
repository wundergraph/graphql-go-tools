package openapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/iancoleman/strcase"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/introspection"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/operationreport"
)

type converter struct {
	openapi        *openapi3.T
	knownFullTypes map[string]*knownFullTypeDetails
	fullTypes      []introspection.FullType
}

type knownFullTypeDetails struct {
	hasDescription bool
}

func isValidResponse(status int) bool {
	if status >= 200 && status < 300 {
		return true
	}
	return false
}

func MakeInputTypeName(name string) string {
	parsed := strings.Split(name, "/")
	return fmt.Sprintf("%sInput", strcase.ToCamel(parsed[len(parsed)-1]))
}

func MakeFieldNameFromOperationID(operationID string) string {
	return strcase.ToLowerCamel(operationID)
}

func MakeFieldNameFromEndpoint(method, endpoint string) string {
	endpoint = strings.Replace(endpoint, "/", " ", -1)
	endpoint = strings.Replace(endpoint, "{", " ", -1)
	endpoint = strings.Replace(endpoint, "}", " ", -1)
	endpoint = strings.TrimSpace(endpoint)
	return strcase.ToLowerCamel(fmt.Sprintf("%s %s", strings.ToLower(method), endpoint))
}

func MakeParameterName(name string) string {
	return strcase.ToLowerCamel(name)
}

func getOperationDescription(operation *openapi3.Operation) string {
	var sb = strings.Builder{}
	sb.WriteString(operation.Summary)
	sb.WriteString("\n")
	sb.WriteString(operation.Description)
	return strings.TrimSpace(sb.String())
}

// __TypeKind of introspection is an unexported type. In order to overcome the problem,
// this function creates and returns a TypeRef for a given kind. kind is a AsyncAPI type.
func getTypeRef(kind string) (introspection.TypeRef, error) {
	// See introspection_enum.go
	switch kind {
	case "string", "integer", "number", "boolean":
		return introspection.TypeRef{Kind: 0}, nil
	case "object":
		return introspection.TypeRef{Kind: 3}, nil
	case "array":
		return introspection.TypeRef{Kind: 1}, nil
	}
	return introspection.TypeRef{}, fmt.Errorf("unknown type: %s", kind)
}

func isNonNullable(name string, required []string) bool {
	for _, item := range required {
		if item == name {
			return true
		}
	}
	return false
}

func convertToNonNull(ofType *introspection.TypeRef) introspection.TypeRef {
	copiedOfType := *ofType
	nonNullType := introspection.TypeRef{Kind: 2}
	nonNullType.OfType = &copiedOfType
	nonNullType.Name = copiedOfType.Name
	return nonNullType
}

func getParamTypeRef(kind string) (introspection.TypeRef, error) {
	// See introspection_enum.go
	switch kind {
	case "string", "integer", "number", "boolean":
		return introspection.TypeRef{Kind: 0}, nil
	case "object":
		// InputType
		return introspection.TypeRef{Kind: 7}, nil
	case "array":
		return introspection.TypeRef{Kind: 1}, nil
	}
	return introspection.TypeRef{}, fmt.Errorf("unknown type: %s", kind)
}

func getPrimitiveGraphQLTypeName(openapiType string) (string, error) {
	switch openapiType {
	case "string":
		return string(literal.STRING), nil
	case "integer":
		return string(literal.INT), nil
	case "number":
		return string(literal.FLOAT), nil
	case "boolean":
		return string(literal.BOOLEAN), nil
	default:
		return "", fmt.Errorf("unknown type: %s", openapiType)
	}
}

func (c *converter) getGraphQLTypeName(schemaRef *openapi3.SchemaRef) (string, error) {
	if schemaRef.Value.Type == "object" {
		gqlType := extractFullTypeNameFromRef(schemaRef.Ref)
		if gqlType == "" {
			return "", errors.New("schema reference is empty")
		}
		err := c.processObject(schemaRef)
		if err != nil {
			return "", err
		}
		return gqlType, nil
	}
	return getPrimitiveGraphQLTypeName(schemaRef.Value.Type)
}

func extractFullTypeNameFromRef(ref string) string {
	parsed := strings.Split(ref, "/")
	return strcase.ToCamel(parsed[len(parsed)-1])
}

func (c *converter) processSchemaProperties(fullType *introspection.FullType, schema openapi3.Schema) error {
	for name, schemaRef := range schema.Properties {
		gqlType, err := c.getGraphQLTypeName(schemaRef)
		if err != nil {
			return err
		}

		typeRef, err := getTypeRef(schemaRef.Value.Type)
		if err != nil {
			return err
		}
		typeRef.Name = &gqlType
		if isNonNullable(name, schema.Required) {
			typeRef = convertToNonNull(&typeRef)
		}

		field := introspection.Field{
			Name:        name,
			Type:        typeRef,
			Description: schemaRef.Value.Description,
		}

		fullType.Fields = append(fullType.Fields, field)
		sort.Slice(fullType.Fields, func(i, j int) bool {
			return fullType.Fields[i].Name < fullType.Fields[j].Name
		})
	}
	return nil
}

func (c *converter) processInputFields(ft *introspection.FullType, schemaRef *openapi3.SchemaRef) error {
	for propertyName, property := range schemaRef.Value.Properties {
		gqlType, err := getPrimitiveGraphQLTypeName(property.Value.Type)
		if err != nil {
			return err
		}
		typeRef, err := getTypeRef(property.Value.Type)
		if err != nil {
			return err
		}
		typeRef.Name = &gqlType
		if isNonNullable(propertyName, schemaRef.Value.Required) {
			typeRef = convertToNonNull(&typeRef)
		}
		f := introspection.InputValue{
			Name: propertyName,
			Type: typeRef,
		}
		ft.InputFields = append(ft.InputFields, f)
		sort.Slice(ft.InputFields, func(i, j int) bool {
			return ft.InputFields[i].Name < ft.InputFields[j].Name
		})
	}
	return nil
}

func (c *converter) processArray(schema *openapi3.SchemaRef) error {
	fullTypeName := extractFullTypeNameFromRef(schema.Value.Items.Ref)
	_, ok := c.knownFullTypes[fullTypeName]
	if ok {
		return nil
	}
	c.knownFullTypes[fullTypeName] = &knownFullTypeDetails{}

	ft := introspection.FullType{
		Kind: introspection.OBJECT,
		Name: fullTypeName,
	}
	typeOfElements := schema.Value.Items.Value.Type
	if typeOfElements == "object" {
		err := c.processSchemaProperties(&ft, *schema.Value.Items.Value)
		if err != nil {
			return err
		}
	} else {
		for _, item := range schema.Value.Items.Value.AllOf {
			if item.Value.Type == "object" {
				err := c.processSchemaProperties(&ft, *item.Value)
				if err != nil {
					return err
				}
			}
		}
	}
	c.fullTypes = append(c.fullTypes, ft)
	return nil
}

func (c *converter) processObject(schema *openapi3.SchemaRef) error {
	fullTypeName := extractFullTypeNameFromRef(schema.Ref)
	details, ok := c.knownFullTypes[fullTypeName]
	if ok {
		needsUpdate := checkForNewKnownFullTypeDetails(schema, details)
		if !needsUpdate {
			return nil
		}

		ok = c.updateFullTypeDetails(schema, fullTypeName)
		if ok {
			return nil
		}
	}
	c.knownFullTypes[fullTypeName] = &knownFullTypeDetails{
		hasDescription: len(schema.Value.Description) > 0,
	}

	ft := introspection.FullType{
		Kind:        introspection.OBJECT,
		Name:        fullTypeName,
		Description: schema.Value.Description,
	}
	err := c.processSchemaProperties(&ft, *schema.Value)
	if err != nil {
		return err
	}
	c.fullTypes = append(c.fullTypes, ft)
	return nil
}

func (c *converter) processInputObject(schema *openapi3.SchemaRef) error {
	fullTypeName := MakeInputTypeName(schema.Ref)
	_, ok := c.knownFullTypes[fullTypeName]
	if ok {
		return nil
	}
	c.knownFullTypes[fullTypeName] = &knownFullTypeDetails{}

	ft := introspection.FullType{
		Kind: introspection.INPUTOBJECT,
		Name: fullTypeName,
	}
	err := c.processInputFields(&ft, schema)
	if err != nil {
		return err
	}
	c.fullTypes = append(c.fullTypes, ft)
	return nil
}

func (c *converter) processSchema(schema *openapi3.SchemaRef) error {
	if schema.Value.Type == "array" {
		return c.processArray(schema)
	} else if schema.Value.Type == "object" {
		return c.processObject(schema)
	}

	sort.Slice(c.fullTypes, func(i, j int) bool {
		return c.fullTypes[i].Name < c.fullTypes[j].Name
	})
	return nil
}

func (c *converter) importFullTypes() ([]introspection.FullType, error) {
	for _, pathItem := range c.openapi.Paths {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodPut} {
			operation := pathItem.GetOperation(method)
			if operation == nil {
				continue
			}

			for statusCodeStr := range operation.Responses {
				if statusCodeStr == "default" {
					continue
				}
				status, err := strconv.Atoi(statusCodeStr)
				if err != nil {
					return nil, err
				}
				if !isValidResponse(status) {
					continue
				}

				schema := getJSONSchema(status, operation)
				if schema == nil {
					continue
				}

				err = c.processSchema(schema)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	sort.Slice(c.fullTypes, func(i, j int) bool {
		return c.fullTypes[i].Name < c.fullTypes[j].Name
	})
	return c.fullTypes, nil
}

func extractTypeName(status int, operation *openapi3.Operation) string {
	response := operation.Responses.Get(status)
	if response == nil {
		// Nil response?
		return ""
	}
	schema := getJSONSchema(status, operation)
	if schema == nil {
		return ""
	}
	if schema.Value.Type == "array" {
		return extractFullTypeNameFromRef(schema.Value.Items.Ref)
	}
	return extractFullTypeNameFromRef(schema.Ref)
}

func getJSONSchemaFromResponseRef(response *openapi3.ResponseRef) *openapi3.SchemaRef {
	if response == nil {
		return nil
	}
	var schema *openapi3.SchemaRef
	for _, mime := range []string{"application/json"} {
		mediaType := response.Value.Content.Get(mime)
		if mediaType != nil {
			return mediaType.Schema
		}
	}
	return schema
}

func getJSONSchema(status int, operation *openapi3.Operation) *openapi3.SchemaRef {
	response := operation.Responses.Get(status)
	if response == nil {
		return nil
	}
	return getJSONSchemaFromResponseRef(response)
}

func getJSONSchemaFromRequestBody(operation *openapi3.Operation) *openapi3.SchemaRef {
	for _, mime := range []string{"application/json"} {
		mediaType := operation.RequestBody.Value.Content.Get(mime)
		if mediaType != nil {
			return mediaType.Schema
		}
	}
	return nil
}

func (c *converter) importQueryTypeFieldParameter(field *introspection.Field, parameter *openapi3.Parameter, schema *openapi3.SchemaRef) error {
	paramType := schema.Value.Type
	if paramType == "array" {
		paramType = schema.Value.Items.Value.Type
	}

	typeRef, err := getTypeRef(paramType)
	if err != nil {
		return err
	}
	gqlType, err := getPrimitiveGraphQLTypeName(paramType)
	if err != nil {
		return err
	}

	if schema.Value.Items != nil {
		ofType := schema.Value.Items.Value.Type
		ofTypeRef, err := getTypeRef(ofType)
		if err != nil {
			return err
		}
		typeRef.OfType = &ofTypeRef
		gqlType = fmt.Sprintf("[%s]", gqlType)
	}

	typeRef.Name = &gqlType
	if parameter.Required {
		typeRef = convertToNonNull(&typeRef)
	}
	iv := introspection.InputValue{
		Name: parameter.Name,
		Type: typeRef,
	}

	field.Args = append(field.Args, iv)
	sort.Slice(field.Args, func(i, j int) bool {
		return field.Args[i].Name < field.Args[j].Name
	})
	return nil
}

func (c *converter) importQueryTypeFields(typeRef *introspection.TypeRef, operation *openapi3.Operation) (*introspection.Field, error) {
	f := introspection.Field{
		Name:        strcase.ToLowerCamel(operation.OperationID),
		Type:        *typeRef,
		Description: getOperationDescription(operation),
	}

	for _, parameter := range operation.Parameters {
		schema := parameter.Value.Schema
		if schema == nil {
			mediaType := parameter.Value.Content.Get("application/json")
			if mediaType != nil {
				schema = mediaType.Schema
			}
		}
		if schema == nil {
			continue
		}
		err := c.importQueryTypeFieldParameter(&f, parameter.Value, schema)
		if err != nil {
			return nil, err
		}
	}
	return &f, nil
}

func (c *converter) importQueryType() (*introspection.FullType, error) {
	queryType := &introspection.FullType{
		Kind: introspection.OBJECT,
		Name: "Query",
	}
	for key, pathItem := range c.openapi.Paths {
		// We only support HTTP GET operation.
		for _, method := range []string{http.MethodGet} {
			operation := pathItem.GetOperation(method)
			if operation == nil {
				continue
			}
			for statusCodeStr := range operation.Responses {
				if statusCodeStr == "default" {
					continue
				}
				status, err := strconv.Atoi(statusCodeStr)
				if err != nil {
					return nil, err
				}

				if !isValidResponse(status) {
					continue
				}

				schema := getJSONSchema(status, operation)
				if schema == nil {
					continue
				}
				kind := schema.Value.Type
				if kind == "" {
					// We assume that it is an object type.
					kind = "object"
				}

				typeName := strcase.ToCamel(extractTypeName(status, operation))
				typeRef, err := getTypeRef(kind)
				if err != nil {
					return nil, err
				}
				if kind == "array" {
					// Array of some type
					typeRef.OfType = &introspection.TypeRef{Kind: 3, Name: &typeName}
				}
				typeRef.Name = &typeName
				queryField, err := c.importQueryTypeFields(&typeRef, operation)
				if err != nil {
					return nil, err
				}
				if queryField.Name == "" {
					queryField.Name = strings.Trim(key, "/")
				}
				queryType.Fields = append(queryType.Fields, *queryField)
			}
		}
	}
	sort.Slice(queryType.Fields, func(i, j int) bool {
		return queryType.Fields[i].Name < queryType.Fields[j].Name
	})
	return queryType, nil
}

func (c *converter) addParameters(name string, schema *openapi3.SchemaRef) (*introspection.InputValue, error) {
	paramType := schema.Value.Type
	if paramType == "array" {
		paramType = schema.Value.Items.Value.Type
	}

	typeRef, err := getParamTypeRef(paramType)
	if err != nil {
		return nil, err
	}

	gqlType := name
	if paramType != "object" {
		gqlType, err = getPrimitiveGraphQLTypeName(paramType)
		if err != nil {
			return nil, err
		}
	} else {
		name = MakeInputTypeName(name)
		gqlType = name
		err = c.processInputObject(schema)
		if err != nil {
			return nil, err
		}
	}

	if schema.Value.Items != nil {
		ofType := schema.Value.Items.Value.Type
		ofTypeRef, err := getParamTypeRef(ofType)
		if err != nil {
			return nil, err
		}
		typeRef.OfType = &ofTypeRef
		gqlType = fmt.Sprintf("[%s]", gqlType)
	}

	typeRef.Name = &gqlType
	return &introspection.InputValue{
		Name: MakeParameterName(name),
		Type: typeRef,
	}, nil
}

func (c *converter) importMutationType() (*introspection.FullType, error) {
	mutationType := &introspection.FullType{
		Kind: introspection.OBJECT,
		Name: "Mutation",
	}
	for key, pathItem := range c.openapi.Paths {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
			operation := pathItem.GetOperation(method)
			if operation == nil {
				continue
			}
			for statusCodeStr := range operation.Responses {
				if statusCodeStr == "default" {
					continue
				}
				status, err := strconv.Atoi(statusCodeStr)
				if err != nil {
					return nil, err
				}

				if !isValidResponse(status) {
					continue
				}

				typeName := strcase.ToCamel(extractTypeName(status, operation))
				if typeName == "" {
					// IBM/openapi-to-graphql uses String as return type.
					// TODO: https://stackoverflow.com/questions/44737043/is-it-possible-to-not-return-any-data-when-using-a-graphql-mutation/44773532#44773532
					typeName = "String"
				}

				typeRef, err := getTypeRef("object")
				if err != nil {
					return nil, err
				}
				typeRef.Name = &typeName

				f := introspection.Field{
					Name:        MakeFieldNameFromOperationID(operation.OperationID),
					Type:        typeRef,
					Description: getOperationDescription(operation),
				}
				if f.Name == "" {
					f.Name = MakeFieldNameFromEndpoint(method, key)
				}

				var inputValue *introspection.InputValue
				if operation.RequestBody != nil {
					schema := getJSONSchemaFromRequestBody(operation)
					inputValue, err = c.addParameters(extractFullTypeNameFromRef(schema.Ref), schema)
					if err != nil {
						return nil, err
					}
					if operation.RequestBody.Value.Required {
						inputValue.Type = convertToNonNull(&inputValue.Type)
					}
					f.Args = append(f.Args, *inputValue)
				}

				for _, parameter := range operation.Parameters {
					inputValue, err = c.addParameters(parameter.Value.Name, parameter.Value.Schema)
					if err != nil {
						return nil, err
					}
					if parameter.Value.Required {
						inputValue.Type = convertToNonNull(&inputValue.Type)
					}
					f.Args = append(f.Args, *inputValue)
				}

				sort.Slice(f.Args, func(i, j int) bool {
					return f.Args[i].Name < f.Args[j].Name
				})
				mutationType.Fields = append(mutationType.Fields, f)
			}
		}
	}
	sort.Slice(mutationType.Fields, func(i, j int) bool {
		return mutationType.Fields[i].Name < mutationType.Fields[j].Name
	})
	return mutationType, nil
}

func (c *converter) updateFullTypeDetails(schema *openapi3.SchemaRef, typeName string) (ok bool) {
	var introspectionFullType *introspection.FullType
	for i := 0; i < len(c.fullTypes); i++ {
		if c.fullTypes[i].Name == typeName {
			introspectionFullType = &c.fullTypes[i]
			break
		}
	}

	if introspectionFullType == nil {
		return false
	}

	if !c.knownFullTypes[typeName].hasDescription {
		introspectionFullType.Description = schema.Value.Description
		c.knownFullTypes[typeName].hasDescription = true
	}

	return true
}

func ImportParsedOpenAPIv3Document(document *openapi3.T, report *operationreport.Report) *ast.Document {
	c := &converter{
		openapi:        document,
		knownFullTypes: make(map[string]*knownFullTypeDetails),
		fullTypes:      make([]introspection.FullType, 0),
	}
	data := introspection.Data{}

	data.Schema.QueryType = &introspection.TypeName{
		Name: "Query",
	}
	queryType, err := c.importQueryType()
	if err != nil {
		report.AddInternalError(err)
		return nil
	}
	data.Schema.Types = append(data.Schema.Types, *queryType)

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

// checkForNewKnownFullTypeDetails will return `true` if the `openapi3.SchemaRef` contains new type details and `false` if not.
func checkForNewKnownFullTypeDetails(schema *openapi3.SchemaRef, currentDetails *knownFullTypeDetails) bool {
	if !currentDetails.hasDescription && len(schema.Value.Description) > 0 {
		return true
	}
	return false
}
