package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
)

const (
	typeConnectionInit = "connection_init"
	typeConnectionAck  = "connection_ack"
	typePing           = "ping"
	typePong           = "pong"
	typeSubscribe      = "subscribe"
	typeNext           = "next"
	typeError          = "error"
	typeComplete       = "complete"
)

var (
	ErrAckTimeout      = errors.New("connection_ack timeout")
	ErrAckNotReceived  = errors.New("expected connection_ack")
	ErrConnectionError = errors.New("connection error from server")
)

type outgoingMessage struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

type incomingMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type subscribePayload struct {
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables,omitempty"`
	OperationName string         `json:"operationName,omitempty"`
	Extensions    map[string]any `json:"extensions,omitempty"`
}

type GraphQLWS struct {
	AckTimeout time.Duration
}

func NewGraphQLWS() *GraphQLWS {
	return &GraphQLWS{
		AckTimeout: 30 * time.Second,
	}
}

func (p *GraphQLWS) Subprotocol() string {
	return "graphql-transport-ws"
}

// Init implements Protocol.
func (p *GraphQLWS) Init(ctx context.Context, conn *websocket.Conn, payload map[string]any) error {
	initMsg := outgoingMessage{
		Type:    typeConnectionInit,
		Payload: payload,
	}
	if err := wsjson.Write(ctx, conn, initMsg); err != nil {
		return fmt.Errorf("write connection_init: %w", err)
	}

	timeout := p.AckTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ackCtx, ackCancel := context.WithTimeout(ctx, timeout)
	defer ackCancel()

	for {
		var ackMessage incomingMessage
		if err := wsjson.Read(ackCtx, conn, &ackMessage); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return ErrAckTimeout
			}
			return fmt.Errorf("read connection_ack: %w", err)
		}

		switch ackMessage.Type {
		case typeConnectionAck:
			return nil
		case typePing:
			if err := p.Pong(ctx, conn); err != nil {
				return fmt.Errorf("pre-init pong: %w", err)
			}
			continue
		default:
			return fmt.Errorf("%w: got %q", ErrAckNotReceived, ackMessage.Type)
		}
	}

}

// Ping implements Protocol.
func (p *GraphQLWS) Ping(ctx context.Context, conn *websocket.Conn) error {
	msg := outgoingMessage{
		Type: typePing,
	}
	return wsjson.Write(ctx, conn, msg)
}

// Pong implements Protocol.
func (p *GraphQLWS) Pong(ctx context.Context, conn *websocket.Conn) error {
	msg := outgoingMessage{
		Type: typePong,
	}
	return wsjson.Write(ctx, conn, msg)
}

// Read implements Protocol.
func (p *GraphQLWS) Read(ctx context.Context, conn *websocket.Conn) (*Message, error) {
	var raw incomingMessage
	if err := wsjson.Read(ctx, conn, &raw); err != nil {
		return nil, fmt.Errorf("read message: %w", err)
	}

	return p.decode(raw)
}

// Subscribe implements Protocol.
func (p *GraphQLWS) Subscribe(ctx context.Context, conn *websocket.Conn, id string, req *common.Request) error {
	msg := outgoingMessage{
		ID:   id,
		Type: typeSubscribe,
		Payload: subscribePayload{
			Query:         req.Query,
			Variables:     req.Variables,
			OperationName: req.OperationName,
			Extensions:    req.Extensions,
		},
	}
	return wsjson.Write(ctx, conn, msg)
}

// Unsubscribe implements Protocol.
func (p *GraphQLWS) Unsubscribe(ctx context.Context, conn *websocket.Conn, id string) error {
	msg := outgoingMessage{
		ID:   id,
		Type: typeComplete,
	}
	return wsjson.Write(ctx, conn, msg)
}

func (p *GraphQLWS) decode(raw incomingMessage) (*Message, error) {
	msg := &Message{
		ID: raw.ID,
	}

	switch raw.Type {
	case typeNext:
		msg.Type = MessageData
		if raw.Payload != nil {
			var resp common.ExecutionResult
			if err := json.Unmarshal(raw.Payload, &resp); err != nil {
				return nil, fmt.Errorf("unmarshal next payload: %w", err)
			}
			msg.Payload = &resp
		}
	case typeError:
		msg.Type = MessageError
		if raw.Payload != nil {
			var errs []common.GraphQLError
			if err := json.Unmarshal(raw.Payload, &errs); err != nil {
				return nil, fmt.Errorf("unmarshal error payload: %w", err)
			}
			msg.Err = &common.SubscriptionError{Errors: errs}
		} else {
			msg.Err = errors.New("subscription error")
		}

	case typeComplete:
		msg.Type = MessageComplete

	case typePing:
		msg.Type = MessagePing

	case typePong:
		msg.Type = MessagePong

	default:
		return nil, fmt.Errorf("unknown message type: %s", raw.Type)
	}

	return msg, nil
}

var _ Protocol = (*GraphQLWS)(nil)
