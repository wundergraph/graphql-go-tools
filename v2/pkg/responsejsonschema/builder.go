// Package responsejsonschema builds JSON Schemas for selected GraphQL response values.
package responsejsonschema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type options struct {
	customScalarMappings map[string]json.RawMessage
}

// Option configures response schema construction.
type Option func(*options)

// WithCustomScalarMappings supplies JSON Schemas for custom GraphQL scalars.
func WithCustomScalarMappings(mappings map[string]json.RawMessage) Option {
	ownedMappings := cloneCustomScalarMappings(mappings)

	return func(options *options) {
		options.customScalarMappings = cloneCustomScalarMappings(ownedMappings)
	}
}

func cloneCustomScalarMappings(mappings map[string]json.RawMessage) map[string]json.RawMessage {
	if mappings == nil {
		return nil
	}

	clonedMappings := make(map[string]json.RawMessage, len(mappings))
	for scalarName, schema := range mappings {
		clonedMappings[scalarName] = append(json.RawMessage(nil), schema...)
	}
	return clonedMappings
}

// Build returns the JSON Schema for the response value at fieldPath.
//
// Each path segment is a response name, so aliases take precedence over field
// names. Inline fragments do not add path segments.
func Build(operation *ast.Document, definition *ast.Document, fieldPath []string, opts ...Option) (json.RawMessage, error) {
	if operation == nil {
		return nil, fmt.Errorf("build response JSON schema: operation document is nil")
	}
	if definition == nil {
		return nil, fmt.Errorf("build response JSON schema: definition document is nil")
	}
	if len(fieldPath) == 0 {
		return nil, fmt.Errorf("build response JSON schema: field path is empty")
	}
	if len(operation.OperationDefinitions) == 0 {
		return nil, fmt.Errorf("build response JSON schema: operation document has no operation definition")
	}

	appliedOptions := &options{}
	for _, option := range opts {
		option(appliedOptions)
	}
	if err := validateCustomScalarMappings(definition, appliedOptions.customScalarMappings); err != nil {
		return nil, fmt.Errorf("build response JSON schema: %w", err)
	}

	operationDefinition := operation.OperationDefinitions[0]
	if !operationDefinition.HasSelections {
		return nil, fmt.Errorf("build response JSON schema: operation definition has no selection set")
	}

	fieldCandidates, err := fieldCandidatesByResponsePath(operation, definition, &operationDefinition, fieldPath)
	if err != nil {
		return nil, fmt.Errorf(
			"build response JSON schema: resolve response path %q: %w",
			strings.Join(fieldPath, "."),
			err,
		)
	}

	schemas := make([]json.RawMessage, 0, len(fieldCandidates))
	for _, candidate := range fieldCandidates {
		var selectionSetRefs []int
		for _, field := range candidate.fields {
			if operation.Fields[field.ref].HasSelections {
				selectionSetRefs = append(selectionSetRefs, operation.Fields[field.ref].SelectionSet)
			}
		}
		schema, err := schemaForTypeRef(operation, definition, definition.FieldDefinitions[candidate.fieldDefinitionRef].Type, selectionSetRefs, appliedOptions)
		if err != nil {
			return nil, fmt.Errorf(
				"build response JSON schema at response path %q: %w",
				strings.Join(fieldPath, "."),
				err,
			)
		}
		schemas = append(schemas, schema)
	}

	sort.Slice(schemas, func(left, right int) bool {
		return bytes.Compare(schemas[left], schemas[right]) < 0
	})
	uniqueSchemas := schemas[:0]
	for _, schema := range schemas {
		if len(uniqueSchemas) == 0 || !bytes.Equal(uniqueSchemas[len(uniqueSchemas)-1], schema) {
			uniqueSchemas = append(uniqueSchemas, schema)
		}
	}
	if len(uniqueSchemas) == 1 {
		return uniqueSchemas[0], nil
	}

	combined, err := marshalSchema(anyOfSchema{AnyOf: uniqueSchemas})
	if err != nil {
		return nil, fmt.Errorf("build response JSON schema: combine mutually exclusive response schemas: %w", err)
	}
	return combined, nil
}

func validateCustomScalarMappings(definition *ast.Document, mappings map[string]json.RawMessage) error {
	scalarNames := make([]string, 0, len(mappings))
	for scalarName := range mappings {
		scalarNames = append(scalarNames, scalarName)
	}
	sort.Strings(scalarNames)

	for _, scalarName := range scalarNames {
		typeNode, ok := definition.Index.FirstNodeByNameStr(scalarName)
		if !ok || typeNode.Kind != ast.NodeKindScalarTypeDefinition {
			return fmt.Errorf("custom scalar mapping %q does not name a defined custom scalar", scalarName)
		}

		schema := mappings[scalarName]
		if !json.Valid(schema) {
			return fmt.Errorf("custom scalar mapping %q is not valid JSON", scalarName)
		}
		var decoded any
		if err := json.Unmarshal(schema, &decoded); err != nil {
			return fmt.Errorf("custom scalar mapping %q is not valid JSON: %w", scalarName, err)
		}
		switch decoded.(type) {
		case map[string]any, bool:
		default:
			return fmt.Errorf("custom scalar mapping %q is not a JSON Schema", scalarName)
		}
		if _, err := jsonschema.CompileString("custom-scalar-"+scalarName+".schema.json", string(schema)); err != nil {
			return fmt.Errorf("custom scalar mapping %q is not a valid JSON Schema: %w", scalarName, err)
		}
	}
	return nil
}
