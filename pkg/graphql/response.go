package graphql

import (
	"encoding/json"
)

type Response struct {
	Errors     Errors                 `json:"errors,omitempty"`
	Data       map[string]interface{} `json:"data,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

func (r Response) Marshal() ([]byte, error) {
	return json.Marshal(r)
}
