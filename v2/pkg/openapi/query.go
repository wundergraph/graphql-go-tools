package openapi

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/introspection"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/iancoleman/strcase"
)

func (c *converter) importQueryType() (*introspection.FullType, error) {
	queryType := &introspection.FullType{
		Kind: introspection.OBJECT,
		Name: "Query",
	}
	for pathName, pathItem := range c.openapi.Paths {
		c.currentPathName = pathName
		c.currentPathItem = pathItem
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

				var (
					kind     string
					typeName string
				)
				if schema == nil {
					typeName = c.tryMakeTypeNameFromOperation(status, operation)
				} else {
					kind = schema.Value.Type
					typeName, err = c.getReturnType(schema)
					if err != nil {
						return nil, err
					}
					if len(schema.Value.Enum) > 0 {
						c.createOrGetEnumType(typeName, schema)
					}
				}

				if kind == "" {
					// We assume that it is an object type.
					kind = "object"
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

func (c *converter) processParameter(field *introspection.Field, parameter *openapi3.ParameterRef) error {
	schema := parameter.Value.Schema
	if schema == nil {
		mediaType := parameter.Value.Content.Get("application/json")
		if mediaType != nil {
			schema = mediaType.Schema
		}
	}
	if schema == nil {
		return nil
	}
	return c.importQueryTypeFieldParameter(field, parameter.Value, schema)
}

func (c *converter) importQueryTypeFields(typeRef *introspection.TypeRef, operation *openapi3.Operation) (*introspection.Field, error) {
	field := &introspection.Field{
		Name:        strcase.ToLowerCamel(operation.OperationID),
		Type:        *typeRef,
		Description: getOperationDescription(operation),
	}

	for _, parameter := range operation.Parameters {
		if err := c.processParameter(field, parameter); err != nil {
			return nil, err
		}
	}

	for _, parameter := range c.currentPathItem.Parameters {
		if err := c.processParameter(field, parameter); err != nil {
			return nil, err
		}
	}

	return field, nil
}

func (c *converter) importQueryTypeFieldParameter(field *introspection.Field, parameter *openapi3.Parameter, schema *openapi3.SchemaRef) error {
	var (
		err     error
		gqlType string
		typeRef introspection.TypeRef
	)

	if len(schema.Value.Enum) > 0 {
		enumType := c.createOrGetEnumType(parameter.Name, schema)
		typeRef = getEnumTypeRef()
		gqlType = enumType.Name
	} else {
		paramType := schema.Value.Type
		if paramType == "array" {
			paramType = schema.Value.Items.Value.Type
		}

		typeRef, err = getTypeRef(paramType)
		if err != nil {
			return err
		}
		gqlType, err = getPrimitiveGraphQLTypeName(paramType)
		if err != nil {
			return err
		}
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
		Name: strcase.ToLowerCamel(parameter.Name),
		Type: typeRef,
	}

	field.Args = append(field.Args, iv)
	sort.Slice(field.Args, func(i, j int) bool {
		return field.Args[i].Name < field.Args[j].Name
	})
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
