package netpoll

import (
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"
)

var (
	ErrUnsupported = errors.New("epoll/kqueue is not supported on this system")
)

// Poller is the interface for epoll/kqueue poller, special for network connections.
type Poller interface {
	// Add adds the connection to poller.
	Add(conn net.Conn) error
	// Remove removes the connection from poller and closes it.
	Remove(conn net.Conn) error
	// Wait waits for at most count events and returns the connections.
	Wait(count int) ([]net.Conn, error)
	// Close closes the poller. If closeConns is true, it will close all the connections.
	Close(closeConns bool) error
}

func SocketFD(conn net.Conn) int {
	if con, ok := conn.(syscall.Conn); ok {
		raw, err := con.SyscallConn()
		if err != nil {
			return 0
		}
		sfd := 0
		raw.Control(func(fd uintptr) { // nolint: errcheck
			sfd = int(fd)
		})
		return sfd
	} else if con, ok := conn.(ConnImpl); ok {
		return con.fd
	}
	return 0
}

// Supported checks if the more efficient network poll library is functional on the current system.
// It returns ErrUnsupported error if implementation is not supported. Any other error indicates non-functionality.
// This function does an integration test of the concrete network implementation
// to ensure that system calls are working as expected.
func Supported() error {
	// Create an instance of the poller
	poller, err := NewPoller(1, 10*time.Millisecond)
	if err != nil {
		return ErrUnsupported
	}
	defer poller.Close(true)

	// Create a dummy in-memory connection for testing
	conn1, conn2 := net.Pipe()
	defer conn1.Close()
	defer conn2.Close()

	// Add the connection to the poller
	if err := poller.Add(conn1); err != nil {
		return fmt.Errorf("failed to add connection to poller: %w", err)
	}

	// Test the wait functionality
	_, err = poller.Wait(1)
	if err != nil {
		return fmt.Errorf("failed to wait for events: %w", err)
	}

	// Remove the connection from the poller
	if err := poller.Remove(conn1); err != nil {
		return fmt.Errorf("failed to remove connection from poller: %w", err)
	}

	// If all operations succeed, implementation is functional
	return nil
}
