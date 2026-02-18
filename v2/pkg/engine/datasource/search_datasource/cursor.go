package search_datasource

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// EncodeCursor encodes sort values into an opaque cursor string.
func EncodeCursor(sortValues []string) string {
	data, _ := json.Marshal(sortValues)
	return base64.RawURLEncoding.EncodeToString(data)
}

// DecodeCursor decodes an opaque cursor string into sort values.
func DecodeCursor(cursor string) ([]string, error) {
	data, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor encoding: %w", err)
	}
	var sortValues []string
	if err := json.Unmarshal(data, &sortValues); err != nil {
		return nil, fmt.Errorf("invalid cursor data: %w", err)
	}
	return sortValues, nil
}
