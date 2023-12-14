package openapi

import (
	"fmt"

	"github.com/TykTechnologies/graphql-go-tools/pkg/introspection"
	"github.com/iancoleman/strcase"
)

var preDefinedScalarTypes = map[string]string{
	"JSON": "The `JSON` scalar type represents JSON values as specified by [ECMA-404](http://www.ecma-international.org/publications/files/ECMA-ST/ECMA-404.pdf).",
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
