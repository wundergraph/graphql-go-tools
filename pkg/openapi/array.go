package openapi

import (
	"errors"

	"github.com/TykTechnologies/graphql-go-tools/pkg/introspection"
	"github.com/getkin/kin-openapi/openapi3"
)

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
