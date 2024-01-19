package openapi

import (
	"strings"

	"github.com/TykTechnologies/graphql-go-tools/pkg/introspection"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/iancoleman/strcase"
)

func getEnumTypeRef() introspection.TypeRef {
	return introspection.TypeRef{Kind: 4}
}

// createOrGetEnumType creates or retrieves an enum type based on the given name and schema.
// If the enum type already exists, it is returned.
// Otherwise, a new enum type is created and stored in the knownEnums map and fullTypes slice.
// It populates the enum values of the enum type based on the enum values in the schema.
// Finally, it returns the created or retrieved enum type.
func (c *converter) createOrGetEnumType(name string, schema *openapi3.SchemaRef) introspection.FullType {
	name = strcase.ToCamel(name)
	if enumType, ok := c.knownEnums[name]; ok {
		return enumType
	}

	enumType := introspection.FullType{
		Kind:        introspection.ENUM,
		Name:        name,
		Description: schema.Value.Description,
	}

	for _, enum := range schema.Value.Enum {
		enumValue := introspection.EnumValue{
			Name: strings.ToUpper(strcase.ToSnake(enum.(string))),
		}
		enumType.EnumValues = append(enumType.EnumValues, enumValue)
	}
	c.fullTypes = append(c.fullTypes, enumType)
	c.knownEnums[name] = enumType
	return enumType
}
