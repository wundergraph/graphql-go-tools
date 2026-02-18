package search_datasource

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

// ExtractEntities extracts entity documents from a GraphQL response JSON.
// The path parameter (e.g. "data.products") specifies where to find the entity array.
func ExtractEntities(responseBody []byte, path string, entityTypeName string, keyFields []string) ([]searchindex.EntityDocument, error) {
	var raw any
	if err := json.Unmarshal(responseBody, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}

	// Navigate to the path
	current := raw
	for _, segment := range strings.Split(path, ".") {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected object at path segment %q, got %T", segment, current)
		}
		current, ok = obj[segment]
		if !ok {
			return nil, fmt.Errorf("path segment %q not found", segment)
		}
	}

	// Handle both single object and array
	switch v := current.(type) {
	case []any:
		return extractFromArray(v, entityTypeName, keyFields)
	case map[string]any:
		doc, err := extractSingleEntity(v, entityTypeName, keyFields)
		if err != nil {
			return nil, err
		}
		return []searchindex.EntityDocument{doc}, nil
	default:
		return nil, fmt.Errorf("expected array or object at path, got %T", current)
	}
}

func extractFromArray(items []any, entityTypeName string, keyFields []string) ([]searchindex.EntityDocument, error) {
	docs := make([]searchindex.EntityDocument, 0, len(items))
	for i, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected object at index %d, got %T", i, item)
		}
		doc, err := extractSingleEntity(obj, entityTypeName, keyFields)
		if err != nil {
			return nil, fmt.Errorf("entity at index %d: %w", i, err)
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

func extractSingleEntity(obj map[string]any, entityTypeName string, keyFields []string) (searchindex.EntityDocument, error) {
	identity := searchindex.DocumentIdentity{
		TypeName:  entityTypeName,
		KeyFields: make(map[string]any, len(keyFields)),
	}

	for _, kf := range keyFields {
		val, ok := obj[kf]
		if !ok {
			return searchindex.EntityDocument{}, fmt.Errorf("key field %q not found in entity", kf)
		}
		identity.KeyFields[kf] = val
	}

	// Copy all fields (except vectors, which are handled by the embedding pipeline)
	fields := make(map[string]any, len(obj))
	vectors := make(map[string][]float32)

	for k, v := range obj {
		// Check if this is a vector field (array of numbers)
		if arr, ok := v.([]any); ok && len(arr) > 0 {
			if _, isNum := arr[0].(float64); isNum {
				vec := make([]float32, len(arr))
				allNum := true
				for i, elem := range arr {
					num, ok := elem.(float64)
					if !ok {
						allNum = false
						break
					}
					vec[i] = float32(num)
				}
				if allNum {
					vectors[k] = vec
					continue
				}
			}
		}
		fields[k] = v
	}

	return searchindex.EntityDocument{
		Identity: identity,
		Fields:   fields,
		Vectors:  vectors,
	}, nil
}

// EntityFieldMaps extracts fields as maps for batch embedding processing.
func EntityFieldMaps(docs []searchindex.EntityDocument) []map[string]any {
	result := make([]map[string]any, len(docs))
	for i, doc := range docs {
		result[i] = doc.Fields
	}
	return result
}
