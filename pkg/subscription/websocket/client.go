package websocket

import (
	"errors"
	"io"
	"net"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/jensneuse/abstractlogger"

	"github.com/TykTechnologies/graphql-go-tools/pkg/subscription"
)

// Client is an actual implementation of the subscription client interface.
type Client struct {
	logger abstractlogger.Logger
	// clientConn holds the actual connection to the client.
	clientConn net.Conn
	// isClosedConnection indicates if the websocket connection is closed.
	isClosedConnection bool
}

// NewClient will create a new websocket subscription client.
func NewClient(logger abstractlogger.Logger, clientConn net.Conn) *Client {
	return &Client{
		logger:     logger,
		clientConn: clientConn,
	}
}

// ReadBytesFromClient will read a subscription message from the websocket client.
func (c *Client) ReadBytesFromClient() ([]byte, error) {
	var data []byte
	var opCode ws.OpCode

	data, opCode, err := wsutil.ReadClientData(c.clientConn)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, io.ErrUnexpectedEOF) {
		c.isClosedConnection = true
		return nil, subscription.ErrTransportClientClosedConnection
	} else if err != nil {
		if c.isClosedConnectionError(err) {
			return nil, subscription.ErrTransportClientClosedConnection
		}

		c.logger.Error("websocket.Client.ReadBytesFromClient: after reading from client",
			abstractlogger.Error(err),
			abstractlogger.ByteString("data", data),
			abstractlogger.Any("opCode", opCode),
		)

		c.isClosedConnectionError(err)

		return nil, err
	}

	return data, nil
}

// WriteBytesToClient will write a subscription message to the websocket client.
func (c *Client) WriteBytesToClient(message []byte) error {
	if c.isClosedConnection {
		return subscription.ErrTransportClientClosedConnection
	}

	err := wsutil.WriteServerMessage(c.clientConn, ws.OpText, message)
	if errors.Is(err, io.ErrClosedPipe) {
		c.isClosedConnection = true
		return subscription.ErrTransportClientClosedConnection
	} else if err != nil {
		c.logger.Error("websocket.Client.WriteBytesToClient: after writing to client",
			abstractlogger.Error(err),
			abstractlogger.ByteString("message", message),
		)

		return err
	}

	return nil
}

// IsConnected will indicate if the websocket connection is still established.
func (c *Client) IsConnected() bool {
	return !c.isClosedConnection
}

// Disconnect will close the websocket connection.
func (c *Client) Disconnect() error {
	c.logger.Debug("websocket.Client.Disconnect: before disconnect",
		abstractlogger.String("message", "disconnecting client"),
	)
	c.isClosedConnection = true
	return c.clientConn.Close()
}

// isClosedConnectionError will indicate if the given error is a connection closed error.
func (c *Client) isClosedConnectionError(err error) bool {
	var closedErr wsutil.ClosedError
	if errors.As(err, &closedErr) {
		c.isClosedConnection = true
	}
	return c.isClosedConnection
}

// Interface Guard
var _ subscription.TransportClient = (*Client)(nil)
