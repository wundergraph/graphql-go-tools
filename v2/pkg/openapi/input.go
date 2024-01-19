package openapi

import (
	"fmt"
	"sort"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/introspection"
	"github.com/getkin/kin-openapi/openapi3"
)

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

func (c *converter) makeInputObjectFromAllOfAnyOfCommon(items openapi3.SchemaRefs) (string, error) {
	// Create a new converter here to process AllOf and AnyOf keywords and merge the types.
	// Then we move the merged type to the root converter.
	cc := newConverter(c.openapi)
	for i, item := range items {
		if item.Ref == "" {
			// Generate a name for the unnamed type. We just need the fields.
			item.Ref = fmt.Sprintf("unnamed-type-item-%d", i)
		}
		if err := cc.processSchema(item); err != nil {
			return "", err
		}
	}

	typeName := MakeInputTypeName(MakeTypeNameFromPathName(c.currentPathName))
	if _, ok := c.knownFullTypes[typeName]; ok {
		// Already created, passing it.
		return typeName, nil
	}

	mergedType := introspection.FullType{
		Kind: introspection.INPUTOBJECT,
		Name: typeName,
	}
	knownFields := make(map[string]struct{})
	knownInputFields := make(map[string]struct{})
	for _, fullType := range cc.fullTypes {
		if fullType.Kind == introspection.OBJECT {
			for _, field := range fullType.Fields {
				if _, ok := knownFields[field.Name]; !ok {
					knownFields[field.Name] = struct{}{}
					// Convert a Field to a InputValue
					inputValue := introspection.InputValue{
						Name:        field.Name,
						Description: field.Description,
						Type:        field.Type,
					}
					mergedType.InputFields = append(mergedType.InputFields, inputValue)
				}
			}
		} else if fullType.Kind == introspection.ENUM {
			if _, ok := c.knownEnums[fullType.Name]; ok {
				continue
			} else {
				c.knownEnums[fullType.Name] = fullType
				c.fullTypes = append(c.fullTypes, fullType)
			}
		} else if fullType.Kind == introspection.INPUTOBJECT {
			for _, inputField := range fullType.InputFields {
				if _, ok := knownInputFields[inputField.Name]; !ok {
					knownInputFields[inputField.Name] = struct{}{}
					mergedType.InputFields = append(mergedType.InputFields, inputField)
				}
			}
		}
		mergedType.PossibleTypes = append(mergedType.PossibleTypes, fullType.PossibleTypes...)
		mergedType.Interfaces = append(mergedType.Interfaces, fullType.Interfaces...)
	}
	c.mergedTypePostProcessing(mergedType)
	return mergedType.Name, nil
}

// makeInputObjectFromAllOf converts a schema with multiple "allOf" properties into an input object.
func (c *converter) makeInputObjectFromAllOf(schema *openapi3.SchemaRef) (string, error) {
	return c.makeInputObjectFromAllOfAnyOfCommon(schema.Value.AllOf)
}

// makeInputObjectFromAllOf converts a schema with multiple "allOf" properties into an input object.
func (c *converter) makeInputObjectFromAnyOf(schema *openapi3.SchemaRef) (string, error) {
	return c.makeInputObjectFromAllOfAnyOfCommon(schema.Value.AnyOf)
}

func (c *converter) getInputValue(name string, schema *openapi3.SchemaRef) (*introspection.InputValue, error) {
	var (
		err     error
		gqlType string
		typeRef introspection.TypeRef
	)

	if len(schema.Value.Enum) > 0 {
		enumType := c.createOrGetEnumType(name, schema)
		typeRef = getEnumTypeRef()
		gqlType = enumType.Name
	} else if schema.Value.OneOf != nil && len(schema.Value.OneOf) > 0 {
		gqlType = name // JSON
		typeRef = introspection.TypeRef{Kind: 0}
	} else if schema.Value.AllOf != nil && len(schema.Value.AllOf) > 0 {
		gqlType, err = c.makeInputObjectFromAllOf(schema)
		if err != nil {
			return nil, err
		}
		typeRef = introspection.TypeRef{Kind: 7}
	} else if schema.Value.AnyOf != nil && len(schema.Value.AnyOf) > 0 {
		gqlType, err = c.makeInputObjectFromAnyOf(schema)
		if err != nil {
			return nil, err
		}
		typeRef = introspection.TypeRef{Kind: 7}
	} else {
		paramType := schema.Value.Type
		if paramType == "array" {
			paramType = schema.Value.Items.Value.Type
		}

		typeRef, err = getParamTypeRef(paramType)
		if err != nil {
			return nil, err
		}

		gqlType = name
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
