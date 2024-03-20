package graphql

import (
	"encoding/json"
)

type Response struct {
	Errors Errors `json:"errors,omitempty"`
	Data   any    `json:"data"` // we add this here to ensure that "data":null is added to an error response
}

func (r Response) Marshal() ([]byte, error) {
	return json.Marshal(r)
}
