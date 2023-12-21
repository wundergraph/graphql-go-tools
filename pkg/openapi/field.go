package openapi

import (
	"errors"
	"sort"

	"github.com/TykTechnologies/graphql-go-tools/pkg/introspection"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/iancoleman/strcase"
)

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
