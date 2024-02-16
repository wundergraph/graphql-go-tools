package openapi

import (
	"errors"
	"fmt"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/introspection"
	"github.com/getkin/kin-openapi/openapi3"
	"net/http"
	"sort"
)

func (c *converter) checkAndProcessOneOfKeyword(schema *openapi3.SchemaRef) error {
	if schema.Value.OneOf == nil {
		return nil
	}

	// Set name to unnamed schemas.
	var namedSchema = 1
	for _, oneOfSchema := range schema.Value.OneOf {
		if oneOfSchema.Ref == "" {
			if namedSchema <= 1 {
				oneOfSchema.Ref = fmt.Sprintf("%sMember", MakeTypeNameFromPathName(c.currentPathName))
			} else {
				oneOfSchema.Ref = fmt.Sprintf("%sMember%d", MakeTypeNameFromPathName(c.currentPathName), namedSchema)
			}
			namedSchema++
		}
	}

	for _, oneOfSchema := range schema.Value.OneOf {
		err := c.processSchema(oneOfSchema)
		if err != nil {
			return err
		}
	}
	// Create a UNION type here
	if len(schema.Value.OneOf) > 0 {
		unionName := MakeTypeNameFromPathName(c.currentPathName)
		if _, ok := c.knownUnions[unionName]; ok {
			// Already have the union definition.
			return nil
		}
		unionType := introspection.FullType{
			Kind:          introspection.UNION,
			Name:          unionName,
			PossibleTypes: []introspection.TypeRef{},
		}
		for _, oneOfSchema := range schema.Value.OneOf {
			fullTypeName, err := extractFullTypeNameFromRef(oneOfSchema.Ref)
			if errors.Is(err, errTypeNameExtractionImpossible) {
				fullTypeName = MakeTypeNameFromPathName(c.currentPathName)
				err = nil
			}
			if err != nil {
				return err
			}
			unionType.PossibleTypes = append(unionType.PossibleTypes, introspection.TypeRef{
				Kind: introspection.OBJECT,
				Name: &fullTypeName,
			})
		}
		c.knownUnions[unionName] = unionType
		c.fullTypes = append(c.fullTypes, unionType)
	}
	return nil
}

func (c *converter) mergedTypePostProcessing(mergedType introspection.FullType) {
	sort.Slice(mergedType.Fields, func(i, j int) bool {
		return mergedType.Fields[i].Name < mergedType.Fields[j].Name
	})
	sort.Slice(mergedType.InputFields, func(i, j int) bool {
		return mergedType.InputFields[i].Name < mergedType.InputFields[j].Name
	})
	sort.Slice(mergedType.EnumValues, func(i, j int) bool {
		return mergedType.EnumValues[i].Name < mergedType.EnumValues[j].Name
	})

	c.fullTypes = append(c.fullTypes, mergedType)
	sort.Slice(c.fullTypes, func(i, j int) bool {
		return c.fullTypes[i].Name < c.fullTypes[j].Name
	})
	c.knownFullTypes[mergedType.Name] = &knownFullTypeDetails{}
}

func (c *converter) checkAndProcessAllOfAnyOfCommon(ref string, items openapi3.SchemaRefs) error {
	var (
		err      error
		typeName string
	)

	// schema.Ref
	if ref != "" {
		typeName, err = extractFullTypeNameFromRef(ref)
		if errors.Is(err, errTypeNameExtractionImpossible) {
			typeName = MakeTypeNameFromPathName(c.currentPathName)
			err = nil
		}
		if err != nil {
			return err
		}
	} else {
		typeName = MakeTypeNameFromPathName(c.currentPathName)
	}
	if _, ok := c.knownFullTypes[typeName]; ok {
		// Already created, passing it.
		return nil
	}

	// Create a new converter here to process AllOf and AnyOf keywords and merge the types.
	// Then we move the merged type to the root converter.
	cc := newConverter(c.openapi)
	for i, item := range items {
		if item.Ref == "" {
			// Generate a name for the unnamed type. We just need the fields.
			item.Ref = fmt.Sprintf("unnamed-type-item-%d", i)
		}
		if err = cc.processSchema(item); err != nil {
			return err
		}
	}
	mergedType := introspection.FullType{
		Kind: introspection.OBJECT,
		Name: typeName,
	}
	knownFields := make(map[string]struct{})
	for _, fullType := range cc.fullTypes {
		if fullType.Kind == introspection.OBJECT {
			for _, field := range fullType.Fields {
				if _, ok := knownFields[field.Name]; !ok {
					knownFields[field.Name] = struct{}{}
					mergedType.Fields = append(mergedType.Fields, field)
				}
			}
		} else if fullType.Kind == introspection.ENUM {
			if _, ok := c.knownEnums[fullType.Name]; ok {
				continue
			} else {
				c.knownEnums[fullType.Name] = fullType
				c.fullTypes = append(c.fullTypes, fullType)
			}
		}
		mergedType.PossibleTypes = append(mergedType.PossibleTypes, fullType.PossibleTypes...)
		mergedType.Interfaces = append(mergedType.Interfaces, fullType.Interfaces...)
	}
	c.mergedTypePostProcessing(mergedType)
	return nil
}

// checkAndProcessAllOfKeyword checks for the "allOf" keyword in the schema and processes it if it exists.
// It merges the fields, enum values, input fields, possible types, and interfaces of the allOf schemas into one merged type.
// The merged type is then added to the list of full types and stored in the knownFullTypes map.
func (c *converter) checkAndProcessAllOfKeyword(schema *openapi3.SchemaRef) error {
	if schema.Value.AllOf == nil {
		return nil
	}
	return c.checkAndProcessAllOfAnyOfCommon(schema.Ref, schema.Value.AllOf)
}

// checkAndProcessAnyOfKeyword checks for the "anyOf" keyword in the schema and processes it if it exists.
// It calls the checkAndProcessAllOfAnyOfCommon method with the schema reference and the anyOf schemas.
// This method is used to handle schemas that have multiple possible types as defined by the anyOf keyword.
// It merges the fields, enum values, input fields, possible types, and interfaces of the anyOf schemas into one merged type.
// The merged type is then added to the list of full types and stored in the knownFullTypes map.
func (c *converter) checkAndProcessAnyOfKeyword(schema *openapi3.SchemaRef) error {
	if schema.Value.AnyOf == nil {
		return nil
	}
	return c.checkAndProcessAllOfAnyOfCommon(schema.Ref, schema.Value.AnyOf)
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

	err := c.checkAndProcessOneOfKeyword(schema)
	if err != nil {
		return err
	}

	err = c.checkAndProcessAllOfKeyword(schema)
	if err != nil {
		return err
	}

	err = c.checkAndProcessAnyOfKeyword(schema)
	if err != nil {
		return err
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

			_, schema, err := findSchemaRef(operation.Responses)
			if err != nil {
				return nil, err
			}
			if schema == nil {
				continue
			}
			err = c.processSchema(schema)
			if err != nil {
				return nil, err
			}
		}
	}
	sort.Slice(c.fullTypes, func(i, j int) bool {
		return c.fullTypes[i].Name < c.fullTypes[j].Name
	})
	return c.fullTypes, nil
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

// checkForNewKnownFullTypeDetails will return `true` if the `openapi3.SchemaRef` contains new type details and `false` if not.
func checkForNewKnownFullTypeDetails(schema *openapi3.SchemaRef, currentDetails *knownFullTypeDetails) bool {
	if !currentDetails.hasDescription && len(schema.Value.Description) > 0 {
		return true
	}
	return false
}
