// Package responsejsonschema builds JSON Schemas for selected GraphQL response values.
package responsejsonschema

import (
	"encoding/json"
	"fmt"

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

	operationDefinition := operation.OperationDefinitions[0]
	if !operationDefinition.HasSelections {
		return nil, fmt.Errorf("build response JSON schema: operation definition has no selection set")
	}

	fieldRefs, err := fieldRefsByResponsePath(operation, operationDefinition.SelectionSet, fieldPath)
	if err != nil {
		return nil, fmt.Errorf("build response JSON schema: %w", err)
	}

	fieldDefinitionRef, err := fieldDefinitionByResponsePath(operation, definition, &operationDefinition, fieldPath)
	if err != nil {
		return nil, fmt.Errorf("build response JSON schema: %w", err)
	}

	var selectionSetRefs []int
	for _, fieldRef := range fieldRefs {
		if operation.Fields[fieldRef].HasSelections {
			selectionSetRefs = append(selectionSetRefs, operation.Fields[fieldRef].SelectionSet)
		}
	}
	schema, err := schemaForTypeRef(operation, definition, definition.FieldDefinitions[fieldDefinitionRef].Type, selectionSetRefs, appliedOptions)
	if err != nil {
		return nil, fmt.Errorf("build response JSON schema: %w", err)
	}

	return schema, nil
}
