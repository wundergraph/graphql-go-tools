package transport

import "fmt"

// ErrFailedSubscriptionConnection reports an HTTP response that rejected a WS or SSE connection.
type ErrFailedSubscriptionConnection struct {
	URL        string
	StatusCode int
}

func (e ErrFailedSubscriptionConnection) Error() string {
	return fmt.Sprintf("failed to establish subscription connection to %s, status code: %d", e.URL, e.StatusCode)
}
