package graphqlerrors

import (
	"encoding/json"
)

// Response is the GraphQL response object
// It should only be used to write errors that are happening before the execution of the query e.g. validation errors.
type Response struct {
	Errors Errors `json:"errors,omitempty"`
	// data: null will never be included in the response because this struct should be used for errors that happening before execution
	// This behaviour is compliant with the spec https://spec.graphql.org/draft/#sec-Data
	Data any `json:"data,omitempty"`
}

func (r Response) Marshal() ([]byte, error) {
	return json.Marshal(r)
}
