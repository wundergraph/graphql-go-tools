package resolve

import (
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

	var sb strings.Builder

	if e.SubgraphName == "" {
		sb.WriteString(fmt.Sprintf("Failed to fetch Subgraph at path: '%s'", e.Path))
	} else {
		sb.WriteString(fmt.Sprintf("Failed to fetch Subgraph '%s' at path: '%s'", e.SubgraphName, e.Path))
	}

	if e.Reason != "" {
		sb.WriteString(fmt.Sprintf(", Reason: %s.", e.Reason))
	} else {
		sb.WriteString(".")
	}

	if len(e.DownstreamErrors) > 0 {

		sb.WriteString("\n")
		sb.WriteString("Downstream errors:\n")

		for i, downstreamError := range e.DownstreamErrors {
			extensionCodeErrorString := ""
			if downstreamError.Extensions != nil && downstreamError.Extensions.Code != "" {
				extensionCodeErrorString = downstreamError.Extensions.Code
			}

			if len(downstreamError.Path) > 0 {
				sb.WriteString(fmt.Sprintf("%d. Subgraph error at path '%s', Message: %s", i+1, strings.Join(downstreamError.Path, ","), downstreamError.Message))
			} else {
				sb.WriteString(fmt.Sprintf("%d. Subgraph error, Message: %s", i+1, downstreamError.Message))
			}

			if extensionCodeErrorString != "" {
				sb.WriteString(fmt.Sprintf(", Extension Code: %s", extensionCodeErrorString))
			}

			sb.WriteString("\n")
		}
	}

	return sb.String()
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
