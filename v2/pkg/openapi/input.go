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
