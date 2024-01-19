package openapi

import (
	"errors"
	"github.com/getkin/kin-openapi/openapi3"
)

// tryExtractTypeName attempts to extract the GraphQL type name from the given OpenAPI schema reference.
// Returns the GraphQL type name and any error encountered.
func (c *converter) tryExtractTypeName(schemaRef *openapi3.SchemaRef) (graphqlTypeName string, err error) {
	if schemaRef.Value.Type == "object" {
		// If the schema value doesn't have any properties, the object will be stored in an arbitrary JSON type.
		if len(schemaRef.Value.Properties) == 0 {
			graphqlTypeName = JsonScalarType
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

func (c *converter) getTypeNameFromSchema(schemaRef *openapi3.SchemaRef) (graphqlTypeName string, err error) {
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

func (c *converter) getReturnType(schemaRef *openapi3.SchemaRef) (string, error) {
	typeName, err := c.getTypeNameFromSchema(schemaRef)
	if err == nil {
		return typeName, nil
	}

	if schemaRef.Value.OneOf != nil && len(schemaRef.Value.OneOf) > 0 {
		return MakeTypeNameFromPathName(c.currentPathName), nil
	}

	if schemaRef.Value.AllOf != nil && len(schemaRef.Value.AllOf) > 0 {
		return MakeTypeNameFromPathName(c.currentPathName), nil
	}

	if schemaRef.Value.AnyOf != nil && len(schemaRef.Value.AnyOf) > 0 {
		return MakeTypeNameFromPathName(c.currentPathName), nil
	}
	return "", err
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
