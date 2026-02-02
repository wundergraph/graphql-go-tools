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

// Legacy graphql-ws protocol message types.
// See: https://github.com/apollographql/subscriptions-transport-ws/blob/master/PROTOCOL.md
const (
	legacyTypeConnectionInit      = "connection_init"
	legacyTypeConnectionAck       = "connection_ack"
	legacyTypeConnectionError     = "connection_error"
	legacyTypeConnectionKeepAlive = "ka"
	legacyTypeConnectionTerminate = "connection_terminate"
	legacyTypeStart               = "start"
	legacyTypeData                = "data"
	legacyTypeError               = "error"
	legacyTypeComplete            = "complete"
	legacyTypeStop                = "stop"
)

// GraphQLWSLegacy implements the legacy graphql-ws protocol.
// This is the older Apollo subscriptions-transport-ws protocol.
type GraphQLWSLegacy struct {
	AckTimeout time.Duration
}

// NewGraphQLWSLegacy creates a new legacy graphql-ws protocol handler.
func NewGraphQLWSLegacy() *GraphQLWSLegacy {
	return &GraphQLWSLegacy{
		AckTimeout: 30 * time.Second,
	}
}

func (p *GraphQLWSLegacy) Subprotocol() string {
	return "graphql-ws"
}

// Init implements Protocol.
func (p *GraphQLWSLegacy) Init(ctx context.Context, conn *websocket.Conn, payload map[string]any) error {
	initMsg := outgoingMessage{
		Type:    legacyTypeConnectionInit,
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
		case legacyTypeConnectionAck:
			return nil
		case legacyTypeConnectionKeepAlive:
			// Keep-alive messages can arrive before ack, ignore them
			continue
		case legacyTypeConnectionError:
			var errPayload map[string]any
			if ackMessage.Payload != nil {
				json.Unmarshal(ackMessage.Payload, &errPayload)
			}
			return fmt.Errorf("%w: %v", ErrConnectionError, errPayload)
		default:
			return fmt.Errorf("%w: got %q", ErrAckNotReceived, ackMessage.Type)
		}
	}
}

// Subscribe implements Protocol.
func (p *GraphQLWSLegacy) Subscribe(ctx context.Context, conn *websocket.Conn, id string, req *common.Request) error {
	msg := outgoingMessage{
		ID:   id,
		Type: legacyTypeStart,
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
func (p *GraphQLWSLegacy) Unsubscribe(ctx context.Context, conn *websocket.Conn, id string) error {
	msg := outgoingMessage{
		ID:   id,
		Type: legacyTypeStop,
	}
	return wsjson.Write(ctx, conn, msg)
}

// Read implements Protocol.
func (p *GraphQLWSLegacy) Read(ctx context.Context, conn *websocket.Conn) (*Message, error) {
	var raw incomingMessage
	if err := wsjson.Read(ctx, conn, &raw); err != nil {
		return nil, fmt.Errorf("read message: %w", err)
	}

	return p.decode(raw)
}

// Ping implements Protocol.
// Legacy protocol doesn't support client-initiated ping, this is a no-op.
func (p *GraphQLWSLegacy) Ping(ctx context.Context, conn *websocket.Conn) error {
	// Legacy protocol doesn't have client ping - only server sends ka
	return nil
}

// Pong implements Protocol.
// Legacy protocol doesn't support pong messages, this is a no-op.
func (p *GraphQLWSLegacy) Pong(ctx context.Context, conn *websocket.Conn) error {
	// Legacy protocol doesn't have pong
	return nil
}

func (p *GraphQLWSLegacy) decode(raw incomingMessage) (*Message, error) {
	msg := &Message{
		ID: raw.ID,
	}

	switch raw.Type {
	case legacyTypeData:
		msg.Type = MessageData
		if raw.Payload != nil {
			var resp common.ExecutionResult
			if err := json.Unmarshal(raw.Payload, &resp); err != nil {
				return nil, fmt.Errorf("unmarshal data payload: %w", err)
			}
			msg.Payload = &resp
		}

	case legacyTypeError:
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

	case legacyTypeComplete:
		msg.Type = MessageComplete

	case legacyTypeConnectionKeepAlive:
		// Map keep-alive to ping for consistent handling
		msg.Type = MessagePing

	case legacyTypeConnectionError:
		msg.Type = MessageError
		var errPayload map[string]any
		if raw.Payload != nil {
			json.Unmarshal(raw.Payload, &errPayload)
		}
		msg.Err = fmt.Errorf("connection error: %v", errPayload)

	default:
		return nil, fmt.Errorf("unknown message type: %s", raw.Type)
	}

	return msg, nil
}

var _ Protocol = (*GraphQLWSLegacy)(nil)
