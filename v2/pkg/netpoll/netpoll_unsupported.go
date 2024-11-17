//go:build windows
// +build windows

package netpoll

import (
	"time"
)

// NewPoller creates a new epoll poller.
func NewPoller(connBufferSize int, _ time.Duration) (Poller, error) {
	return nil, ErrUnsupported
}
