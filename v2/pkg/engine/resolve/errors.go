package resolve

import (
	"fmt"
)

type GraphQLError struct {
	Message    string          `json:"message"`
	Locations  []Location      `json:"locations,omitempty"`
	Path       []string        `json:"path"`
	Extensions *ErrorExtension `json:"extensions,omitempty"`
}

type ErrorExtension struct {
	Code string `json:"code"`
}

type Location struct {
	Line   uint32 `json:"line"`
	Column uint32 `json:"column"`
}

type SubgraphError struct {
	SubgraphName string
	Path         string
	Reason       string
	ResponseCode int

	DownstreamErrors []*GraphQLError
}

func NewSubgraphError(subgraphName, path, reason string, responseCode int) *SubgraphError {
	return &SubgraphError{
		SubgraphName: subgraphName,
		Path:         path,
		Reason:       reason,
		ResponseCode: responseCode,
	}
}

func (e *SubgraphError) AppendDownstreamError(error *GraphQLError) {
	e.DownstreamErrors = append(e.DownstreamErrors, error)
}

func (e *SubgraphError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("Failed to fetch subgraph '%s' at path '%s'", e.SubgraphName, e.Path)
	}
	return fmt.Sprintf("Failed to fetch subgraph '%s' at path '%s'. Reason: %s", e.SubgraphName, e.Path, e.Reason)
}

func NewRateLimitError(subgraphName, path, reason string) error {
	return &SubgraphError{
		SubgraphName: subgraphName,
		Path:         path,
		Reason:       reason,
	}
}

type RateLimitError struct {
	SubgraphName string
	Path         string
	Reason       string
}

func (e *RateLimitError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("Rate limit rejected for subgraph '%s' at path '%s'", e.SubgraphName, e.Path)
	}
	return fmt.Sprintf("Rate limit rejected for subgraph '%s' at path '%s'. Reason: %s", e.SubgraphName, e.Path, e.Reason)
}
