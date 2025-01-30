package resolve

import (
	"bytes"
	"fmt"
	"slices"
)

type GraphQLError struct {
	Message   string     `json:"message"`
	Locations []Location `json:"locations,omitempty"`
	// Path is a list of path segments that lead to the error, can be number or string
	Path       []any          `json:"path"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

type Location struct {
	Line   uint32 `json:"line"`
	Column uint32 `json:"column"`
}

type SubgraphError struct {
	DataSourceInfo DataSourceInfo
	Path           string
	Reason         string
	ResponseCode   int

	DownstreamErrors []*GraphQLError
}

func NewSubgraphError(ds DataSourceInfo, path, reason string, responseCode int) *SubgraphError {
	return &SubgraphError{
		Path:           path,
		Reason:         reason,
		ResponseCode:   responseCode,
		DataSourceInfo: ds,
	}
}

func (e *SubgraphError) AppendDownstreamError(error *GraphQLError) {
	e.DownstreamErrors = append(e.DownstreamErrors, error)
}

func (e *SubgraphError) Codes() []string {
	codes := make([]string, 0, len(e.DownstreamErrors))

	for _, downstreamError := range e.DownstreamErrors {
		if downstreamError.Extensions != nil {
			if ok := downstreamError.Extensions["code"]; ok != nil {
				if code, ok := downstreamError.Extensions["code"].(string); ok && !slices.Contains(codes, code) {
					codes = append(codes, code)
				}
			}
		}
	}

	return codes
}

// Error returns the high-level error without downstream errors. For more details, call Summary().
func (e *SubgraphError) Error() string {
	var bf bytes.Buffer

	if e.DataSourceInfo.Name != "" && e.Path != "" {
		fmt.Fprintf(&bf, "Failed to fetch from Subgraph '%s' at Path: '%s'", e.DataSourceInfo.Name, e.Path)
	} else {
		fmt.Fprintf(&bf, "Failed to fetch from Subgraph '%s'", e.DataSourceInfo.Name)
	}

	if e.Reason != "" {
		fmt.Fprintf(&bf, ", Reason: %s.", e.Reason)
	} else {
		fmt.Fprintf(&bf, ".")
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
