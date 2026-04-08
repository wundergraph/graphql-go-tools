package protocol

import (
	"context"
	"errors"

	"github.com/coder/websocket"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
)

// Protocol defines the message framing and behaviour used on a WS connection.
type Protocol interface {
	// Init performs the connection handshake with the server.
	Init(ctx context.Context, conn *websocket.Conn, payload map[string]any) error

	// Subscribe starts a subscription for the given operation.
	Subscribe(ctx context.Context, conn *websocket.Conn, id string, req *common.Request) error

	// Unsubscribe ends a subscription.
	Unsubscribe(ctx context.Context, conn *websocket.Conn, id string) error

	// Read blocks until the next message arrives and decodes it.
	Read(ctx context.Context, conn *websocket.Conn) (*WireMessage, error)

	// Ping requests a liveness check from the server. No-op for protocols that don't support it.
	Ping(ctx context.Context, conn *websocket.Conn) error

	// Pong responds to a server liveness check. No-op for protocols that don't support it.
	Pong(ctx context.Context, conn *websocket.Conn) error
}

var (
	ErrAckTimeout      = errors.New("connection_ack timeout")
	ErrAckNotReceived  = errors.New("expected connection_ack")
	ErrConnectionError = errors.New("connection error from server")
)

// WireMessage is a decoded wire-level protocol message.
// It is different from the common message format because it still contains the ID and internal type,
// which is not exposed to consumers.
type WireMessage struct {
	ID      string
	Type    WireMessageType
	Payload *common.ExecutionResult
	Err     error
}

func (m *WireMessage) IntoClientMessage() *common.Message {
	switch m.Type {
	case MessageData:
		return &common.Message{Type: common.MessageTypeData, Payload: m.Payload}
	case MessageError:
		if m.Payload != nil {
			return &common.Message{Type: common.MessageTypeError, Payload: m.Payload}
		}
		return &common.Message{Type: common.MessageTypeConnectionError, Err: m.Err}
	case MessageComplete:
		return &common.Message{Type: common.MessageTypeComplete}
	default:
		return &common.Message{Type: common.MessageTypeUnknown}
	}
}

// WireMessageType identifies the message type.
type WireMessageType int

const (
	MessageData WireMessageType = iota
	MessageError
	MessageComplete
	MessagePing
	MessagePong
)

func (t WireMessageType) String() string {
	switch t {
	case MessageData:
		return "data"
	case MessageError:
		return "error"
	case MessageComplete:
		return "complete"
	case MessagePing:
		return "ping"
	case MessagePong:
		return "pong"
	default:
		return "unknown"
	}
}
