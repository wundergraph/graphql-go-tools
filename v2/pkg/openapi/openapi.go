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

var (
	errTypeNameExtractionImpossible = errors.New("type name extraction is impossible")
	errNotPrimitiveType             = errors.New("not a primitive type")
)

type converter struct {
	openapi         *openapi3.T
	knownFullTypes  map[string]*knownFullTypeDetails
	fullTypes       []introspection.FullType
	currentPathName string
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

func MakeTypeNameFromPathName(name string) string {
	parsed := strings.Split(name, "/")
	return strcase.ToCamel(parsed[len(parsed)-1])
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
		return "", fmt.Errorf("%w: %s", errNotPrimitiveType, openapiType)
	}
}

func extractTypeName(status int, operation *openapi3.Operation) (string, error) {
	response := operation.Responses.Get(status)
	if response == nil {
		return "", fmt.Errorf("%w: response is nil for operation id: %s, status code: %d", errTypeNameExtractionImpossible, operation.OperationID, status)
	}

	schema := getJSONSchema(status, operation)
	if schema == nil {
		return "", fmt.Errorf("%w: schema is nil for operation id: %s, status code: %d", errTypeNameExtractionImpossible, operation.OperationID, status)
	}

	if schema.Value.Type == "array" {
		return extractFullTypeNameFromRef(schema.Value.Items.Ref)
	}
	return extractFullTypeNameFromRef(schema.Ref)
}

// tryExtractTypeName attempts to extract the GraphQL type name from the given OpenAPI schema reference.
// Returns the GraphQL type name and any error encountered.
func (c *converter) tryExtractTypeName(schemaRef *openapi3.SchemaRef) (graphqlTypeName string, err error) {
	if schemaRef.Value.Type == "object" {
		// If the schema value doesn't have any properties, the object will be stored in an arbitrary JSON type.
		if len(schemaRef.Value.Properties) == 0 {
			graphqlTypeName = "JSON"
			c.addScalarType(graphqlTypeName, preDefinedScalarTypes[graphqlTypeName])
		} else {
			// Unnamed object
			graphqlTypeName = MakeTypeNameFromPathName(c.currentPathName)
		}
	} else if schemaRef.Value.Type == "array" {
		typeOf := schemaRef.Value.Items.Value.Type
		if typeOf == "object" {
			// Array of unnamed objects
			graphqlTypeName = makeListItemFromTypeName(MakeTypeNameFromPathName(c.currentPathName))
		} else {
			// Array of primitive types
			graphqlTypeName, err = getPrimitiveGraphQLTypeName(typeOf)
		}
	}
	return
}

func (c *converter) getReturnType(schemaRef *openapi3.SchemaRef) (graphqlTypeName string, err error) {
	if schemaRef.Value.Type != "object" && schemaRef.Value.Type != "array" {
		if schemaRef.Ref != "" {
			return extractFullTypeNameFromRef(schemaRef.Ref)
		}
		return getPrimitiveGraphQLTypeName(schemaRef.Value.Type)
	}

	if schemaRef.Value.Type == "object" {
		graphqlTypeName, err = extractFullTypeNameFromRef(schemaRef.Ref)
	} else if schemaRef.Value.Type == "array" {
		graphqlTypeName, err = extractFullTypeNameFromRef(schemaRef.Value.Items.Ref)
	}
	if errors.Is(err, errTypeNameExtractionImpossible) {
		return c.tryExtractTypeName(schemaRef)
	}
	return graphqlTypeName, err
}

// getGraphQLTypeName returns the GraphQL type name corresponding to the given OpenAPI schema reference.
// Returns the GraphQL type name and any error encountered.
func (c *converter) getGraphQLTypeName(schemaRef *openapi3.SchemaRef, inputType bool) (graphqlTypeName string, err error) {
	if schemaRef.Value.Type != "object" && schemaRef.Value.Type != "array" {
		return getPrimitiveGraphQLTypeName(schemaRef.Value.Type)
	}

	if schemaRef.Value.Type == "object" {
		graphqlTypeName, err = extractFullTypeNameFromRef(schemaRef.Ref)
	} else if schemaRef.Value.Type == "array" {
		graphqlTypeName, err = extractFullTypeNameFromRef(schemaRef.Value.Items.Ref)
	}
	if err != nil {
		return "", err
	}
	if inputType {
		return MakeInputTypeName(graphqlTypeName), nil
	}

	return graphqlTypeName, nil
}

func extractFullTypeNameFromRef(ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("%w: schema reference is empty", errTypeNameExtractionImpossible)
	}
	parsed := strings.Split(ref, "/")
	return strcase.ToCamel(parsed[len(parsed)-1]), nil
}

func makeTypeNameFromPropertyName(name string, schemaRef *openapi3.SchemaRef) (string, error) {
	if schemaRef.Value.Type == "array" {
		return makeListItemFromTypeName(name), nil
	}
	return "", fmt.Errorf("error while making type name from property name: %s is a unsupported type", name)
}

func (c *converter) makeTypeRefFromSchemaRef(schemaRef *openapi3.SchemaRef, name string, inputType, required bool) (*introspection.TypeRef, error) {
	name = strcase.ToLowerCamel(name)

	graphQLTypeName, err := c.getGraphQLTypeName(schemaRef, inputType)
	if errors.Is(err, errTypeNameExtractionImpossible) {
		graphQLTypeName, err = makeTypeNameFromPropertyName(name, schemaRef)
		if inputType {
			graphQLTypeName = MakeInputTypeName(graphQLTypeName)
		}
	}
	if err != nil {
		return nil, err
	}

	switch schemaRef.Value.Type {
	case "object":
		err = c.processObject(schemaRef)
	case "array":
		err = c.processArrayWithFullTypeName(graphQLTypeName, schemaRef)
	}
	if err != nil {
		return nil, err
	}

	typeRef, err := getTypeRef(schemaRef.Value.Type)
	if err != nil {
		return nil, err
	}
	typeRef.Name = &graphQLTypeName
	if required {
		typeRef = convertToNonNull(&typeRef)
	}

	if schemaRef.Value.Type == "array" {
		typeRef.OfType = &introspection.TypeRef{Kind: 3, Name: &graphQLTypeName}
	}
	return &typeRef, nil
}

func (c *converter) processSchemaProperties(fullType *introspection.FullType, schema openapi3.Schema) error {
	for name, schemaRef := range schema.Properties {
		typeRef, err := c.makeTypeRefFromSchemaRef(schemaRef, name, false, isNonNullable(name, schema.Required))
		if err != nil {
			return err
		}
		field := introspection.Field{
			Name:        name,
			Type:        *typeRef,
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
	for name, property := range schemaRef.Value.Properties {
		typeRef, err := c.makeTypeRefFromSchemaRef(property, name, true, isNonNullable(name, schemaRef.Value.Required))
		if err != nil {
			return err
		}
		f := introspection.InputValue{
			Name: name,
			Type: *typeRef,
		}
		ft.InputFields = append(ft.InputFields, f)
		sort.Slice(ft.InputFields, func(i, j int) bool {
			return ft.InputFields[i].Name < ft.InputFields[j].Name
		})
	}
	return nil
}

func (c *converter) processArray(schema *openapi3.SchemaRef) error {
	fullTypeName, err := extractFullTypeNameFromRef(schema.Value.Items.Ref)
	if errors.Is(err, errTypeNameExtractionImpossible) {
		fullTypeName = makeListItemFromTypeName(MakeTypeNameFromPathName(c.currentPathName))
		err = nil
	}
	if err != nil {
		return err
	}
	return c.processArrayWithFullTypeName(fullTypeName, schema)
}

func (c *converter) processArrayWithFullTypeName(fullTypeName string, schema *openapi3.SchemaRef) error {
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
	// If the schema value doesn't have any properties, the object will be stored in an arbitrary JSON type.
	if len(schema.Value.Properties) == 0 {
		return nil
	}

	fullTypeName, err := extractFullTypeNameFromRef(schema.Ref)
	if errors.Is(err, errTypeNameExtractionImpossible) {
		fullTypeName = MakeTypeNameFromPathName(c.currentPathName)
		err = nil
	}
	if err != nil {
		return err
	}

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
	err = c.processSchemaProperties(&ft, *schema.Value)
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
		arrayOf := schema.Value.Items.Value.Type
		if arrayOf == "string" || arrayOf == "integer" || arrayOf == "number" || arrayOf == "boolean" {
			return nil
		}
		return c.processArray(schema)
	} else if schema.Value.Type == "object" {
		return c.processObject(schema)
	}

	return nil
}

func (c *converter) importFullTypes() ([]introspection.FullType, error) {
	for pathName, pathItem := range c.openapi.Paths {
		c.currentPathName = pathName
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
	for pathName, pathItem := range c.openapi.Paths {
		c.currentPathName = pathName
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
				typeName, err := c.getReturnType(schema)
				if err != nil {
					return nil, err
				}
				typeName = strcase.ToCamel(typeName)
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
					queryField.Name = strings.Trim(pathName, "/")
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
	for pathName, pathItem := range c.openapi.Paths {
		c.currentPathName = pathName
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

				typeName, err := extractTypeName(status, operation)
				if errors.Is(err, errTypeNameExtractionImpossible) {
					// Try to make a new type name for unnamed objects.
					responseRef := operation.Responses.Get(status)
					if responseRef != nil && responseRef.Value != nil {
						mediaType := responseRef.Value.Content.Get("application/json")
						if mediaType != nil && mediaType.Schema != nil && mediaType.Schema.Value != nil {
							if mediaType.Schema.Value.Type == "object" {
								typeName = MakeTypeNameFromPathName(c.currentPathName)
								err = nil
							}
						}
					}

					if typeName == "" {
						// IBM/openapi-to-graphql uses String as return type.
						// TODO: https://stackoverflow.com/questions/44737043/is-it-possible-to-not-return-any-data-when-using-a-graphql-mutation/44773532#44773532
						typeName = "String"
						err = nil
					}
				}
				if err != nil {
					return nil, err
				}
				typeName = strcase.ToCamel(typeName)
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
					f.Name = MakeFieldNameFromEndpoint(method, pathName)
				}

				var inputValue *introspection.InputValue
				if operation.RequestBody != nil {
					schema := getJSONSchemaFromRequestBody(operation)
					fullTypeName, err := extractFullTypeNameFromRef(schema.Ref)
					if err != nil {
						return nil, err
					}
					inputValue, err = c.addParameters(fullTypeName, schema)
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
