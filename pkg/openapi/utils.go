package openapi

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/TykTechnologies/graphql-go-tools/pkg/introspection"
	"github.com/TykTechnologies/graphql-go-tools/pkg/lexer/literal"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/iancoleman/strcase"
)

const JsonScalarType = "JSON"

var errNoValidResponse = errors.New("no valid response found")

var preDefinedScalarTypes = map[string]string{
	JsonScalarType: "The `JSON` scalar type represents JSON values as specified by [ECMA-404](http://www.ecma-international.org/publications/files/ECMA-ST/ECMA-404.pdf).",
}

// From the OpenAPI spec: To define a range of response codes, you may use the
// following range definitions: 1XX, 2XX, 3XX, 4XX, and 5XX.
//
// See https://swagger.io/docs/specification/describing-responses/
var statusCodeRanges = map[string]int{
	"1XX": 100,
	"2XX": 200,
	"3XX": 300,
	"4XX": 400,
	"5XX": 500,
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

// cleanupEndpoint takes a string `name` and splits it by the forward slash ('/').
// It creates an empty slice `result` to store the cleaned-up words.
// For each word in the `parsed` slice, it checks if the word has length zero or starts with '{'.
// If either of these conditions is true, it skips to the next word.
// Otherwise, it appends the word to the `result` slice.
// Finally, it returns the `result` slice containing the cleaned-up words.
func cleanupEndpoint(name string) []string {
	parsed := strings.Split(name, "/")
	var result []string
	for _, word := range parsed {
		if len(word) == 0 {
			continue
		}
		if strings.HasPrefix(word, "{") {
			continue
		}
		result = append(result, word)
	}
	return result
}

func MakeTypeNameFromPathName(name string) string {
	result := cleanupEndpoint(name)
	return strcase.ToCamel(strings.Join(result, " "))
}

func MakeInputTypeName(name string) string {
	parsed := strings.Split(name, "/")
	return fmt.Sprintf("%sInput", strcase.ToCamel(parsed[len(parsed)-1]))
}

func MakeFieldNameFromOperationID(operationID string) string {
	return strcase.ToLowerCamel(operationID)
}

func MakeFieldNameFromEndpointForMutation(method, endpoint string) string {
	result := []string{strings.ToLower(method)}
	result = append(result, cleanupEndpoint(endpoint)...)
	return strcase.ToLowerCamel(strings.Join(result, " "))
}

func MakeFieldNameFromEndpoint(endpoint string) string {
	result := cleanupEndpoint(endpoint)
	return strcase.ToLowerCamel(strings.Join(result, " "))
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

func findSchemaRef(responses openapi3.Responses) (int, *openapi3.SchemaRef, error) {
	statusCode, responseRef, err := getValidResponse(responses)
	if err != nil {
		return 0, nil, err
	}
	return statusCode, getJSONSchemaFromResponseRef(responseRef), nil
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

func toCamelIfNotPredefinedScalar(typeName string) string {
	// Don't convert the type name to CamelCase if it is a predefined
	// scalar such as JSON.
	if _, ok := preDefinedScalarTypes[typeName]; !ok {
		return strcase.ToCamel(typeName)
	}
	return typeName
}

// statusCodeToRange returns a string representing the range of the given status code.
// The function categorizes the status code into different ranges: 1XX, 2XX, 3XX, 4XX, 5XX.
// If the status code is not within any of these ranges, an error is returned.
func statusCodeToRange(status int) (string, error) {
	if status >= 100 && status < 200 {
		return "1XX", nil
	} else if status >= 200 && status < 300 {
		return "2XX", nil
	} else if status >= 300 && status < 400 {
		return "3XX", nil
	} else if status >= 400 && status < 500 {
		return "4XX", nil
	} else if status >= 500 && status < 600 {
		return "5XX", nil
	} else {
		return "", fmt.Errorf("unknown status code: %d", status)
	}
}

func convertStatusCode(statusCode string) (int, error) {
	// The spec advises to use ranges as '2XX' but the OpenAPI parser accepts
	// '2xx' as a valid status code range.
	statusCode = strings.ToUpper(statusCode)
	if code, ok := statusCodeRanges[statusCode]; ok {
		return code, nil
	}
	return strconv.Atoi(statusCode)
}

// getResponseFromOperation returns the response for the given status code from the operation's responses.
// If a response with the given status code is not found, it tries to find the range for the status
// code and returns the response for that range.
func getResponseFromOperation(status int, operation *openapi3.Operation) *openapi3.ResponseRef {
	response := operation.Responses.Get(status)
	if response != nil {
		return response
	}
	// Try to find the range this time.
	statusCodeRange, err := statusCodeToRange(status)
	if err != nil {
		// Invalid status code. It's okay to return nil here. We couldn't find
		// a response for the given status code.
		return nil
	}
	return operation.Responses[statusCodeRange]
}

// isStatusCodeRange checks if the given statusCode is a valid status code range.
func isStatusCodeRange(statusCode string) bool {
	// The spec advises to use ranges as '2XX' but the OpenAPI parser accepts
	// '2xx' as a valid status code range.
	statusCode = strings.ToUpper(statusCode)
	_, has := statusCodeRanges[statusCode]
	return has
}

// sanitizeResponses cleans up responses. If a response range is defined using an
// explicit code, the explicit code definition takes precedence over the range definition for that code.
func sanitizeResponses(responses openapi3.Responses) (openapi3.Responses, error) {
	// OpenAPI specification:
	//
	// To define a range of response codes, you may use the following range definitions:
	// 1XX, 2XX, 3XX, 4XX, and 5XX.
	// If a response range is defined using an explicit code, the explicit code definition
	//takes precedence over the range definition for that code.

	result := make(openapi3.Responses)
	occupiedStatusCodeRange := make(map[string]struct{})

	// First pass, select the explicit code definitions.
	for stringStatusCode, response := range responses {
		// 'default' is not a valid response, ignore it.
		if stringStatusCode == "default" {
			continue
		}

		if isStatusCodeRange(stringStatusCode) {
			// 2XX, 3XX, etc.
			continue
		}

		result[stringStatusCode] = response

		// Calculate the occupied status code range.
		statusCode, err := strconv.Atoi(stringStatusCode)
		if err != nil {
			return nil, err
		}
		statusCodeRange, err := statusCodeToRange(statusCode)
		if err != nil {
			return nil, err
		}
		occupiedStatusCodeRange[statusCodeRange] = struct{}{}
	}

	// Use the status code ranges if not occupied.
	for stringStatusCode, response := range responses {
		// 'default' is not a valid response, ignore it.
		if stringStatusCode == "default" {
			continue
		}

		if !isStatusCodeRange(stringStatusCode) {
			continue
		}
		if _, ok := occupiedStatusCodeRange[stringStatusCode]; !ok {
			result[stringStatusCode] = response
		}
	}

	return result, nil
}

// getValidResponse returns the first valid response from the given responses map.
// It iterates over the responses map and checks if the status code of each response is a valid status.
//
// OpenAPI-to-GraphQL translator mimics IBM/openapi-to-graphql tool. This tool accepts HTTP code 200-299 or 2XX
// as valid responses. Other status codes are simply ignored. Currently, we follow the same convention.
func getValidResponse(responses openapi3.Responses) (int, *openapi3.ResponseRef, error) {
	responses, err := sanitizeResponses(responses)
	if err != nil {
		return 0, nil, err
	}

	var validStatusCodes []int
	validResponseRefs := make(map[int]*openapi3.ResponseRef)
	for stringStatusCode, responseRef := range responses {
		statusCode, err := convertStatusCode(stringStatusCode)
		if err != nil {
			return 0, nil, err
		}

		if isValidResponse(statusCode) {
			validStatusCodes = append(validStatusCodes, statusCode)
			validResponseRefs[statusCode] = responseRef
		}
	}

	if len(validStatusCodes) == 0 {
		return 0, nil, errNoValidResponse
	}

	// If the OpenAPI document contains multiple possible successful response object
	// (HTTP code 200-299 or 2XX). Only one can be chosen.
	// Select first response object with successful status code (200-299).
	// The response object with the HTTP code 200 will be selected
	sort.Ints(validStatusCodes)
	validStatusCode := validStatusCodes[0]
	return validStatusCode, validResponseRefs[validStatusCode], nil
}
