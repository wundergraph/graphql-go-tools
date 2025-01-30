//go:build windows
// +build windows

package netpoll

import (
	"time"
)

// NewPoller creates a new poll based connection implementation.
func NewPoller(connBufferSize int, _ time.Duration) (Poller, error) {
	return nil, ErrUnsupported
}
