package subscription

import (
	"errors"
)

//go:generate mockgen -destination=transport_client_mock_test.go -package=subscription . TransportClient

// ErrTransportClientClosedConnection is an error to indicate that the transport client is using closed connection.
var ErrTransportClientClosedConnection = errors.New("transport client has a closed connection")

// TransportClient provides an interface that can be implemented by any possible subscription client like websockets, mqtt, etc.
// It operates with raw byte slices.
type TransportClient interface {
	// ReadBytesFromClient will invoke a read operation from the client connection and return a byte slice.
	// This function should return ErrTransportClientClosedConnection when reading on a closed connection.
	ReadBytesFromClient() ([]byte, error)
	// WriteBytesToClient will invoke a write operation to the client connection using a byte slice.
	// This function should return ErrTransportClientClosedConnection when writing on a closed connection.
	WriteBytesToClient([]byte) error
	// IsConnected will indicate if a connection is still established.
	IsConnected() bool
	// Disconnect will close the connection between server and client.
	Disconnect() error
	// DisconnectWithReason will close the connection but is also able to process a reason for closure.
	DisconnectWithReason(reason interface{}) error
}
