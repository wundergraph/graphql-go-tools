package resolve

import (
	"bytes"
	"fmt"
	"strings"
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

func (e *SubgraphError) Error() string {

	var bf bytes.Buffer

	if e.DataSourceInfo.Name == "" {
		fmt.Fprintf(&bf, "Failed to fetch Subgraph at Path: '%s'", e.Path)
	} else {
		fmt.Fprintf(&bf, "Failed to fetch from Subgraph '%s' at Path: '%s'", e.DataSourceInfo.Name, e.Path)
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
			if downstreamError.Extensions != nil {
				if ok := downstreamError.Extensions["code"]; ok != nil {
					if code, ok := downstreamError.Extensions["code"].(string); ok {
						extensionCodeErrorString = code
					}
				}
			}

			if len(downstreamError.Path) > 0 {
				builder := strings.Builder{}
				for i := range downstreamError.Path {
					switch t := downstreamError.Path[i].(type) {
					case string:
						builder.WriteString(t)
					case int:
						builder.WriteString(fmt.Sprintf("%d", t))
					}
					if i < len(downstreamError.Path)-1 {
						builder.WriteRune('.')
					}
				}
				path := builder.String()
				fmt.Fprintf(&bf, "%d. Subgraph error at Path '%s', Message: %s", i+1, path, downstreamError.Message)
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
