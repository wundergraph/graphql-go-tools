package netpoll

import (
	"errors"
	"fmt"
	"golang.org/x/sync/errgroup"
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

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	defer ln.Close()

	var addConnErrGroup errgroup.Group

	addConnErrGroup.Go(func() error {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return fmt.Errorf("failed to accept connection: %w", err)
			}

			// Add the connection to the poller
			if err := poller.Add(conn); err != nil {
				return fmt.Errorf("failed to add connection to poller: %w", err)
			}

			// Test the wait functionality
			_, err = poller.Wait(1)
			if err != nil {
				return fmt.Errorf("failed to wait for events: %w", err)
			}

			// Remove the connection from the poller
			if err := poller.Remove(conn); err != nil {
				return fmt.Errorf("failed to remove connection from poller: %w", err)
			}

			return nil //nolint intentionally return nil
		}
	})

	var dialErrGroup errgroup.Group

	dialErrGroup.Go(func() error {

		conn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			return err
		}

		defer conn.Close()

		if err := addConnErrGroup.Wait(); err != nil {
			return err
		}

		_, err = conn.Write([]byte("hello"))
		if err != nil {
			return err
		}

		return nil
	})

	// If all operations succeed, implementation is functional
	return dialErrGroup.Wait()
}
