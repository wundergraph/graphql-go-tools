package websocket

import (
	"errors"
	"io"
	"net"
	"sync"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/pkg/subscription"
)

// CloseReason is type that is used to provide a close reason to Client.DisconnectWithReason.
type CloseReason ws.Frame

// CompiledCloseReason is a pre-compiled close reason to be provided to Client.DisconnectWithReason.
type CompiledCloseReason []byte

var (
	CompiledCloseReasonNormal CompiledCloseReason = ws.MustCompileFrame(
		ws.NewCloseFrame(ws.NewCloseFrameBody(
			ws.StatusNormalClosure, "Normal Closure",
		)),
	)
	CompiledCloseReasonInternalServerError CompiledCloseReason = ws.MustCompileFrame(
		ws.NewCloseFrame(ws.NewCloseFrameBody(
			ws.StatusInternalServerError, "Internal Server Error",
		)),
	)
)

// NewCloseReason is used to compose a close frame with code and reason message.
func NewCloseReason(code uint16, reason string) CloseReason {
	wsCloseFrame := ws.NewCloseFrame(ws.NewCloseFrameBody(
		ws.StatusCode(code), reason,
	))
	return CloseReason(wsCloseFrame)
}

// Client is an actual implementation of the subscription client interface.
type Client struct {
	logger abstractlogger.Logger
	// clientConn holds the actual connection to the client.
	clientConn net.Conn
	// isClosedConnection indicates if the websocket connection is closed.
	isClosedConnection bool
	mu                 *sync.RWMutex
}

// NewClient will create a new websocket subscription client.
func NewClient(logger abstractlogger.Logger, clientConn net.Conn) *Client {
	return &Client{
		logger:     logger,
		clientConn: clientConn,
		mu:         &sync.RWMutex{},
	}
}

// ReadBytesFromClient will read a subscription message from the websocket client.
func (c *Client) ReadBytesFromClient() ([]byte, error) {
	if !c.IsConnected() {
		return nil, subscription.ErrTransportClientClosedConnection
	}

	data, opCode, err := wsutil.ReadClientData(c.clientConn)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, io.ErrUnexpectedEOF) {
		c.changeConnectionStateToClosed()
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
	if !c.IsConnected() {
		return subscription.ErrTransportClientClosedConnection
	}

	err := wsutil.WriteServerMessage(c.clientConn, ws.OpText, message)
	if errors.Is(err, io.ErrClosedPipe) {
		c.changeConnectionStateToClosed()
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
	c.mu.RLock()
	defer c.mu.RUnlock()
	return !c.isClosedConnection
}

// Disconnect will close the websocket connection.
func (c *Client) Disconnect() error {
	c.logger.Debug("websocket.Client.Disconnect: before disconnect",
		abstractlogger.String("message", "disconnecting client"),
	)
	c.changeConnectionStateToClosed()
	return c.clientConn.Close()
}

// DisconnectWithReason will close the websocket and provide the close code and reason.
// It can only consume CloseReason or CompiledCloseReason.
func (c *Client) DisconnectWithReason(reason interface{}) error {
	var err error
	switch reason := reason.(type) {
	case CloseReason:
		err = c.writeFrame(ws.Frame(reason))
	case CompiledCloseReason:
		err = c.writeCompiledFrame(reason)
	default:
		c.logger.Error("websocket.Client.DisconnectWithReason: on reason/frame parsing",
			abstractlogger.String("message", "unknown reason provided"),
		)
		frame := NewCloseReason(4400, "unknown reason")
		err = c.writeFrame(ws.Frame(frame))
	}

	c.logger.Debug("websocket.Client.DisconnectWithReason: before sending close frame",
		abstractlogger.String("message", "disconnecting client"),
	)

	if err != nil {
		c.logger.Error("websocket.Client.DisconnectWithReason: after writing close reason",
			abstractlogger.Error(err),
		)
		return err
	}

	return c.Disconnect()
}

func (c *Client) writeFrame(frame ws.Frame) error {
	return ws.WriteFrame(c.clientConn, frame)
}

func (c *Client) writeCompiledFrame(compiledFrame []byte) error {
	_, err := c.clientConn.Write(compiledFrame)
	return err
}

// isClosedConnectionError will indicate if the given error is a connection closed error.
func (c *Client) isClosedConnectionError(err error) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	var closedErr wsutil.ClosedError
	if errors.As(err, &closedErr) {
		c.isClosedConnection = true
	}
	return c.isClosedConnection
}

func (c *Client) changeConnectionStateToClosed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.isClosedConnection = true
}

// Interface Guard
var _ subscription.TransportClient = (*Client)(nil)
