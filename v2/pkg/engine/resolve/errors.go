package resolve

import (
	"bytes"
	"fmt"
	"strings"
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

	var bf bytes.Buffer

	if e.SubgraphName == "" {
		fmt.Fprintf(&bf, "Failed to fetch Subgraph at Path: '%s'", e.Path)
	} else {
		fmt.Fprintf(&bf, "Failed to fetch from Subgraph '%s' at Path: '%s'", e.SubgraphName, e.Path)
	}

	if e.Reason != "" {
		fmt.Fprintf(&bf, ", Reason: %s.", e.Reason)
	} else {
		fmt.Fprintf(&bf, ".")
	}

	if len(e.DownstreamErrors) > 0 {

		fmt.Fprintf(&bf, "\n")
		fmt.Fprintf(&bf, "Downstream errors:\n")

		for i, downstreamError := range e.DownstreamErrors {
			extensionCodeErrorString := ""
			if downstreamError.Extensions != nil && downstreamError.Extensions.Code != "" {
				extensionCodeErrorString = downstreamError.Extensions.Code
			}

			if len(downstreamError.Path) > 0 {
				fmt.Fprintf(&bf, "%d. Subgraph error at Path '%s', Message: %s", i+1, strings.Join(downstreamError.Path, ","), downstreamError.Message)
			} else {
				fmt.Fprintf(&bf, "%d. Subgraph error with Message: %s", i+1, downstreamError.Message)
			}

			if extensionCodeErrorString != "" {
				fmt.Fprintf(&bf, ", Extension Code: %s.", extensionCodeErrorString)
			}

			fmt.Fprintf(&bf, "\n")
		}
	}

	return bf.String()
}

func NewRateLimitError(subgraphName, path, reason string) *RateLimitError {
	return &RateLimitError{
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
		return fmt.Sprintf("Rate limit rejected for Subgraph '%s' at Path '%s'.", e.SubgraphName, e.Path)
	}
	return fmt.Sprintf("Rate limit rejected for Subgraph '%s' at Path '%s', Reason: %s.", e.SubgraphName, e.Path, e.Reason)
}
