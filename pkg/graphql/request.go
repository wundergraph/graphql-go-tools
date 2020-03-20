package graphql

import (
	"encoding/json"
	"io"
)

type Request struct {
	OperationName string          `json:"operation_name"`
	Variables     json.RawMessage `json:"variables"`
	Query         string          `json:"query"`
}

func UnmarshalRequest(reader io.Reader) (*Request, error) {
	return &Request{}, nil
}

func (r Request) ValidateForSchema(schema *Schema) (valid bool, errors OperationValidationErrors) {
	return true, nil
}

func (r *Request) Normalize(schema *Schema) error {
	return nil
}

func (r Request) CalculateComplexity(complexityCalculator ComplexityCalculator) int {
	return 1
}

func (r Request) Print(writer io.Writer) (n int, err error) {
	return writer.Write(nil)
}
