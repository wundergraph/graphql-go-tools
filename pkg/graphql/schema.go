package graphql

import (
	"io"
)

type Schema struct {
	Content []byte
}

func NewSchemaFromReader(reader io.Reader) (*Schema, error) {
	return &Schema{Content: nil}, nil
}

func NewSchemaFromString(schema string) (*Schema, error) {
	return &Schema{Content: nil}, nil
}

func (s *Schema) Validate() (valid bool, errors SchemaValidationErrors) {
	return true, nil
}

func (s *Schema) Normalize() (*Schema, error) {
	return s, nil
}
