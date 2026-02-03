package common

import "encoding/json"

type Message struct {
	Payload *ExecutionResult
	Err     error
	Done    bool
}

type ExecutionResult struct {
	Data       json.RawMessage `json:"data,omitempty"`
	Errors     json.RawMessage `json:"errors,omitempty"`
	Extensions json.RawMessage `json:"extensions,omitempty"`
}

type Request struct {
	Query         string          `json:"query"`
	OperationName string          `json:"operationName,omitempty"`
	Variables     json.RawMessage `json:"variables,omitempty"`
	Extensions    json.RawMessage `json:"extensions,omitempty"`
}
