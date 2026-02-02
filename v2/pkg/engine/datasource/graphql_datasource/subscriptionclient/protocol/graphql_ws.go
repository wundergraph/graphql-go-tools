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
	gwsTypeConnectionInit      = "connection_init"
	gwsTypeConnectionAck       = "connection_ack"
	gwsTypeConnectionError     = "connection_error"
	gwsTypeConnectionKeepAlive = "ka"
	gwsTypeConnectionTerminate = "connection_terminate"
	gwsTypeStart               = "start"
	gwsTypeData                = "data"
	gwsTypeError               = "error"
	gwsTypeComplete            = "complete"
	gwsTypeStop                = "stop"
)

// GraphQLWS implements the legacy graphql-ws protocol.
// See: https://github.com/apollographql/subscriptions-transport-ws/blob/master/PROTOCOL.md
type GraphQLWS struct {
	AckTimeout time.Duration
}

func NewGraphQLWS() *GraphQLWS {
	return &GraphQLWS{
		AckTimeout: 30 * time.Second,
	}
}

// Init implements Protocol.
func (p *GraphQLWS) Init(ctx context.Context, conn *websocket.Conn, payload map[string]any) error {
	initMsg := outgoingMessage{
		Type:    gwsTypeConnectionInit,
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
		case gwsTypeConnectionAck:
			return nil
		case gwsTypeConnectionKeepAlive:
			// Keep-alive messages can arrive before ack, ignore them
			continue
		case gwsTypeConnectionError:
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
func (p *GraphQLWS) Subscribe(ctx context.Context, conn *websocket.Conn, id string, req *common.Request) error {
	msg := outgoingMessage{
		ID:   id,
		Type: gwsTypeStart,
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
		Type: gwsTypeStop,
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

// Ping implements Protocol.
// Legacy protocol doesn't support client-initiated ping, this is a no-op.
func (p *GraphQLWS) Ping(ctx context.Context, conn *websocket.Conn) error {
	// Legacy protocol doesn't have client ping - only server sends ka
	return nil
}

// Pong implements Protocol.
// Legacy protocol doesn't support pong messages, this is a no-op.
func (p *GraphQLWS) Pong(ctx context.Context, conn *websocket.Conn) error {
	// Legacy protocol doesn't have pong
	return nil
}

func (p *GraphQLWS) decode(raw incomingMessage) (*Message, error) {
	msg := &Message{
		ID: raw.ID,
	}

	switch raw.Type {
	case gwsTypeData:
		msg.Type = MessageData
		if raw.Payload != nil {
			var resp common.ExecutionResult
			if err := json.Unmarshal(raw.Payload, &resp); err != nil {
				return nil, fmt.Errorf("unmarshal data payload: %w", err)
			}
			msg.Payload = &resp
		}

	case gwsTypeError:
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

	case gwsTypeComplete:
		msg.Type = MessageComplete

	case gwsTypeConnectionKeepAlive:
		// Map keep-alive to ping for consistent handling
		msg.Type = MessagePing

	case gwsTypeConnectionError:
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

var _ Protocol = (*GraphQLWS)(nil)
