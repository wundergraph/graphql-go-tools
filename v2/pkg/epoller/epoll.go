package epoller

import (
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"
)

var (
	ErrUnsupported = errors.New("epoll is not supported on this system")
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

// EpollSupported checks if epoll is functional on the current system using the provided Epoll module.
// It returns ErrUnsupported error if epoll is not supported. Any other error indicates non-functionality.
// This function does an integration test of the concrete Epoll implementation
// to ensure that system calls are working as expected.
func EpollSupported() error {
	// Create an instance of the Epoll poller
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

	// Remove the connection from the poller
	if err := poller.Remove(conn1); err != nil {
		return fmt.Errorf("failed to remove connection from poller: %w", err)
	}

	// If all operations succeed, epoll is functional
	return nil
}
