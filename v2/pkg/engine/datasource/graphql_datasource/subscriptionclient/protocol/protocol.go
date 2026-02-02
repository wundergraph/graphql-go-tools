package protocol

import (
	"context"
	"errors"

	"github.com/coder/websocket"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
)

type Protocol interface {
	Init(ctx context.Context, conn *websocket.Conn, payload map[string]any) error

	Subscribe(ctx context.Context, conn *websocket.Conn, id string, req *common.Request) error

	Unsubscribe(ctx context.Context, conn *websocket.Conn, id string) error

	Read(ctx context.Context, conn *websocket.Conn) (*Message, error)

	Ping(ctx context.Context, conn *websocket.Conn) error

	Pong(ctx context.Context, conn *websocket.Conn) error
}

var (
	ErrAckTimeout      = errors.New("connection_ack timeout")
	ErrAckNotReceived  = errors.New("expected connection_ack")
	ErrConnectionError = errors.New("connection error from server")
)

type Message struct {
	ID      string
	Type    MessageType
	Payload *common.ExecutionResult
	Err     error
}

func (m *Message) IntoClientMessage() *common.Message {
	switch m.Type {
	case MessageData:
		return &common.Message{Payload: m.Payload}
	case MessageError:
		return &common.Message{Err: m.Err, Done: true}
	case MessageComplete:
		return &common.Message{Done: true}
	default:
		return &common.Message{}
	}
}

// MessageType identifies the message type.
type MessageType int

const (
	MessageData MessageType = iota
	MessageError
	MessageComplete
	MessagePing
	MessagePong
)

func (t MessageType) String() string {
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
