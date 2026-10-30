package resolve

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

// GraphQLError is a GraphQL error with a message and optional response metadata.
// Values decoded into Path, Extensions, and AdditionalFields use json.Number
// to preserve numeric precision. Extensions may contain nested maps and slices.
type GraphQLError struct {
	Message   string     `json:"message"`
	Locations []Location `json:"locations,omitempty"`
	// Path contains field names and array indices in the client-visible response.
	Path       []any          `json:"path,omitempty"`
	Extensions map[string]any `json:"extensions,omitempty"`
	// AdditionalFields holds non-standard top-level fields. Standard fields must
	// be set using the corresponding fields above.
	AdditionalFields map[string]any `json:"-"`
}

func (e *GraphQLError) Error() string { return e.Message }

type Location struct {
	Line   uint32 `json:"line"`
	Column uint32 `json:"column"`
}

// UnmarshalJSON preserves custom fields and numeric precision in arbitrary JSON values.
func (e *GraphQLError) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	var decoded GraphQLError
	for key, raw := range fields {
		var target any
		switch key {
		case "message":
			target = &decoded.Message
		case "locations":
			target = &decoded.Locations
		case "path":
			target = &decoded.Path
		case "extensions":
			target = &decoded.Extensions
		default:
			var value any
			if err := decodeErrorJSON(raw, &value); err != nil {
				return err
			}
			if decoded.AdditionalFields == nil {
				decoded.AdditionalFields = make(map[string]any)
			}
			decoded.AdditionalFields[key] = value
			continue
		}
		if err := decodeErrorJSON(raw, target); err != nil {
			return err
		}
	}
	*e = decoded
	return nil
}

// MarshalJSON includes custom fields alongside the standard GraphQL error fields.
// A nil optional field is omitted; a non-nil empty map or slice is preserved.
func (e GraphQLError) MarshalJSON() ([]byte, error) {
	fields := make(map[string]any, len(e.AdditionalFields)+4)
	for key, value := range e.AdditionalFields {
		switch key {
		case "message", "locations", "path", "extensions":
			continue
		}
		fields[key] = value
	}
	fields["message"] = e.Message
	if e.Locations != nil {
		fields["locations"] = e.Locations
	}
	if e.Path != nil {
		fields["path"] = e.Path
	}
	if e.Extensions != nil {
		fields["extensions"] = e.Extensions
	}
	return json.Marshal(fields)
}

func decodeErrorJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode(target)
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
		if downstreamError == nil {
			continue
		}
		if code, ok := downstreamError.Extensions["code"].(string); ok && !slices.Contains(codes, code) {
			codes = append(codes, code)
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
