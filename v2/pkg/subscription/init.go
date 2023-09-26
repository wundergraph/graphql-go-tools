package subscription

import "encoding/json"

// InitPayload is a structure that is parsed from the websocket init message payload.
type InitPayload json.RawMessage

// GetString safely gets a string value from the payload. It returns an empty string if the
// payload is nil or the value isn't set.
func (p InitPayload) GetString(key string) string {
	if p == nil {
		return ""
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(p, &payload); err != nil {
		return ""
	}

	if value, ok := payload[key]; ok {
		res, _ := value.(string)
		return res
	}

	return ""
}

// Authorization is a short hand for getting the Authorization header from the
// payload.
func (p InitPayload) Authorization() string {
	if value := p.GetString("Authorization"); value != "" {
		return value
	}

	if value := p.GetString("authorization"); value != "" {
		return value
	}

	return ""
}
