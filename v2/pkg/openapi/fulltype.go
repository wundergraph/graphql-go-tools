package openapi

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/introspection"
	"github.com/getkin/kin-openapi/openapi3"
)

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
				if schema == nil {
					continue
				}

				err = c.processSchema(schema)
				if err != nil {
					return nil, err
				}
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
