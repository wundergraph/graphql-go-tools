package graphql

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
)

var ErrEmptyRequest = errors.New("the provided request is empty")

type Request struct {
	OperationName string          `json:"operation_name"`
	Variables     json.RawMessage `json:"variables"`
	Query         string          `json:"query"`
}

func UnmarshalRequest(reader io.Reader) (*Request, error) {
	requestBytes, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	if len(requestBytes) == 0 {
		return nil, ErrEmptyRequest
	}

	var request Request
	err = json.Unmarshal(requestBytes, &request)
	if err != nil {
		return nil, err
	}

	if len(request.Query) == 0 {
		return nil, ErrEmptyRequest
	}

	return &request, nil
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
