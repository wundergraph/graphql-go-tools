package openapi

import (
	"errors"
	"sort"

	"github.com/TykTechnologies/graphql-go-tools/pkg/introspection"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/iancoleman/strcase"
)

// makeTypeRefFromSchemaRef creates a TypeRef from a SchemaRef, name, inputType, and required flags.
// It converts the name to lower camel case and checks if the enum type exists in the EnumType map.
// If the SchemaRef has enum values, it returns the corresponding enum type reference.
// Otherwise, it gets the GraphQL type name and handles object and array types accordingly.
// If the required flag is true, it converts the type reference to a non-null type.
// For array types, it sets the inner type as the list item type.
// Returns the TypeRef and an error if any.
//
// If type name extraction is impossible, it falls back to making a type name from the property name.
// If inputType is true, it appends "Input" to the type name.
func (c *converter) makeTypeRefFromSchemaRef(schemaRef *openapi3.SchemaRef, name string, inputType, required bool) (*introspection.TypeRef, error) {
	name = strcase.ToLowerCamel(name)

	if len(schemaRef.Value.Enum) > 0 {
		enumType := c.createOrGetEnumType(name, schemaRef)
		typeRef := getEnumTypeRef()
		typeRef.Name = &enumType.Name
		return &typeRef, nil
	}

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
		if len(schemaRef.Value.Enum) > 0 {
			c.createOrGetEnumType(*typeRef.Name, schemaRef)
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
