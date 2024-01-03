package openapi

import (
	"fmt"
	"strings"

	"github.com/TykTechnologies/graphql-go-tools/pkg/introspection"
	"github.com/TykTechnologies/graphql-go-tools/pkg/lexer/literal"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/iancoleman/strcase"
)

var preDefinedScalarTypes = map[string]string{
	"JSON": "The `JSON` scalar type represents JSON values as specified by [ECMA-404](http://www.ecma-international.org/publications/files/ECMA-ST/ECMA-404.pdf).",
}

// addScalarType adds a new scalar type to the converter's known full types list.
// It checks if the type is already known and returns immediately if so.
// Otherwise, it creates a new introspection.FullType instance with the given type name and description.
// It also updates the known full type details to track if the type has a description or not.
// Finally, it adds the new scalar type to the converter's full types slice.
func (c *converter) addScalarType(typeName, description string) {
	if _, ok := c.knownFullTypes[typeName]; ok {
		return
	}
	scalarType := introspection.FullType{
		Kind:        introspection.SCALAR,
		Name:        typeName,
		Description: description,
	}
	typeDetails := &knownFullTypeDetails{}
	if len(description) > 0 {
		typeDetails.hasDescription = true
	}
	c.knownFullTypes[typeName] = typeDetails
	c.fullTypes = append(c.fullTypes, scalarType)
}

// makeListItemFromTypeName returns a formatted string by concatenating the given typeName with "ListItem",
// using the ToCamel function from the strcase package to convert the typeName to camel case.
func makeListItemFromTypeName(typeName string) string {
	return fmt.Sprintf("%sListItem", strcase.ToCamel(typeName))
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

func isValidResponse(status int) bool {
	if status >= 200 && status < 300 {
		return true
	}
	return false
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

func getOperationDescription(operation *openapi3.Operation) string {
	var sb = strings.Builder{}
	sb.WriteString(operation.Summary)
	sb.WriteString("\n")
	sb.WriteString(operation.Description)
	return strings.TrimSpace(sb.String())
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
