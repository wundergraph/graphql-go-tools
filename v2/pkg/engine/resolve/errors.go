package resolve

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/wundergraph/astjson"
)

type GraphQLError struct {
	Message   string     `json:"message"`
	Locations []Location `json:"locations,omitempty"`
	// Path is a list of path segments that lead to the error, can be number or string
	Path       []any          `json:"path"`
	Extensions *astjson.Value `json:"extensions,omitempty"`
}

type Location struct {
	Line   uint32 `json:"line"`
	Column uint32 `json:"column"`
}

func (e *GraphQLError) UnmarshalJSON(data []byte) error {
	type Alias GraphQLError

	aux := &struct {
		*Alias

		Extensions json.RawMessage `json:"extensions,omitempty"`
	}{
		Alias: (*Alias)(e),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	if len(aux.Extensions) > 0 {
		e.Extensions = astjson.MustParseBytes(aux.Extensions)
	}

	return nil
}

func (e GraphQLError) MarshalJSON() ([]byte, error) {
	aux := &struct {
		*GraphQLError

		Extensions json.RawMessage `json:"extensions,omitempty"`
	}{
		GraphQLError: &e,
	}

	if e.Extensions != nil {
		aux.Extensions = e.Extensions.MarshalTo(nil)
	}

	return json.Marshal(aux)
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
		if code := downstreamError.Extensions.Get("code"); code != nil {
			if !slices.Contains(codes, code.String()) {
				codes = append(codes, code.String())
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
