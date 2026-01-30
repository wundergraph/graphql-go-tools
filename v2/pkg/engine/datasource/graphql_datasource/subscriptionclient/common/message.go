package common

import (
	"encoding/json"
	"fmt"
)

type Message struct {
	// Payload contains the GraphQL response payload.
	Payload *ExecutionResult

	// Err is a transport/protocol level error.
	Err error

	// Done indicates the subscription has completed.
	Done bool
}

type ExecutionResult struct {
	Data       json.RawMessage `json:"data"`
	Errors     []GraphQLError  `json:"errors,omitempty"`
	Extensions map[string]any  `json:"extensions,omitempty"`
}

// GraphQLError represents a GraphQL execution error.
type GraphQLError struct {
	Message    string         `json:"message"`
	Path       []any          `json:"path,omitempty"`
	Locations  []Location     `json:"locations,omitempty"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

type Location struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// Request represents a GraphQL subscription request.
type Request struct {
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables,omitempty"`
	OperationName string         `json:"operationName,omitempty"`
	Extensions    map[string]any `json:"extensions,omitempty"`
}

type SubscriptionError struct {
	Errors []GraphQLError
}

func (e *SubscriptionError) Error() string {
	if len(e.Errors) == 0 {
		return "subscription error"
	}
	if len(e.Errors) == 1 {
		return e.Errors[0].Message
	}
	return fmt.Sprintf("%s (and %d more errors)", e.Errors[0].Message, len(e.Errors)-1)
}
