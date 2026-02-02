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
	gtwsTypeConnectionInit = "connection_init"
	gtwsTypeConnectionAck  = "connection_ack"
	gtwsTypePing           = "ping"
	gtwsTypePong           = "pong"
	gtwsTypeSubscribe      = "subscribe"
	gtwsTypeNext           = "next"
	gtwsTypeError          = "error"
	gtwsTypeComplete       = "complete"
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

type GraphQLTransportWS struct {
	AckTimeout time.Duration
}

func NewGraphQLTransportWS() *GraphQLTransportWS {
	return &GraphQLTransportWS{
		AckTimeout: 30 * time.Second,
	}
}

// Init implements Protocol.
func (p *GraphQLTransportWS) Init(ctx context.Context, conn *websocket.Conn, payload map[string]any) error {
	initMsg := outgoingMessage{
		Type:    gtwsTypeConnectionInit,
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
		case gtwsTypeConnectionAck:
			return nil
		case gtwsTypePing:
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
func (p *GraphQLTransportWS) Ping(ctx context.Context, conn *websocket.Conn) error {
	msg := outgoingMessage{
		Type: gtwsTypePing,
	}
	return wsjson.Write(ctx, conn, msg)
}

// Pong implements Protocol.
func (p *GraphQLTransportWS) Pong(ctx context.Context, conn *websocket.Conn) error {
	msg := outgoingMessage{
		Type: gtwsTypePong,
	}
	return wsjson.Write(ctx, conn, msg)
}

// Read implements Protocol.
func (p *GraphQLTransportWS) Read(ctx context.Context, conn *websocket.Conn) (*Message, error) {
	var raw incomingMessage
	if err := wsjson.Read(ctx, conn, &raw); err != nil {
		return nil, fmt.Errorf("read message: %w", err)
	}

	return p.decode(raw)
}

// Subscribe implements Protocol.
func (p *GraphQLTransportWS) Subscribe(ctx context.Context, conn *websocket.Conn, id string, req *common.Request) error {
	msg := outgoingMessage{
		ID:   id,
		Type: gtwsTypeSubscribe,
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
func (p *GraphQLTransportWS) Unsubscribe(ctx context.Context, conn *websocket.Conn, id string) error {
	msg := outgoingMessage{
		ID:   id,
		Type: gtwsTypeComplete,
	}
	return wsjson.Write(ctx, conn, msg)
}

func (p *GraphQLTransportWS) decode(raw incomingMessage) (*Message, error) {
	msg := &Message{
		ID: raw.ID,
	}

	switch raw.Type {
	case gtwsTypeNext:
		msg.Type = MessageData
		if raw.Payload != nil {
			var resp common.ExecutionResult
			if err := json.Unmarshal(raw.Payload, &resp); err != nil {
				return nil, fmt.Errorf("unmarshal next payload: %w", err)
			}
			msg.Payload = &resp
		}
	case gtwsTypeError:
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

	case gtwsTypeComplete:
		msg.Type = MessageComplete

	case gtwsTypePing:
		msg.Type = MessagePing

	case gtwsTypePong:
		msg.Type = MessagePong

	default:
		return nil, fmt.Errorf("unknown message type: %s", raw.Type)
	}

	return msg, nil
}

var _ Protocol = (*GraphQLTransportWS)(nil)
