package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
)

// graphqlWS implements the legacy graphql-ws protocol.
// See: https://github.com/apollographql/subscriptions-transport-ws/blob/master/PROTOCOL.md
type graphqlWS struct{}

const (
	gwsTypeConnectionInit      = "connection_init"
	gwsTypeConnectionAck       = "connection_ack"
	gwsTypeConnectionError     = "connection_error"
	gwsTypeConnectionKeepAlive = "ka"
	gwsTypeStart               = "start"
	gwsTypeData                = "data"
	gwsTypeError               = "error"
	gwsTypeComplete            = "complete"
	gwsTypeStop                = "stop"
)

func NewGraphQLWS() *graphqlWS {
	return &graphqlWS{}
}

// Init implements Protocol.
func (p *graphqlWS) Init(ctx context.Context, conn *websocket.Conn, payload map[string]any) error {
	initMsg := outgoingMessage{
		Type: gwsTypeConnectionInit,
	}
	if payload != nil {
		initMsg.Payload = payload
	}
	if err := wsjson.Write(ctx, conn, initMsg); err != nil {
		return fmt.Errorf("write connection_init: %w", err)
	}

	for {
		var ackMessage incomingMessage
		if err := wsjson.Read(ctx, conn, &ackMessage); err != nil {
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
				// If this fails, the error will have nil errors anyway, handling it does nothing unique
				_ = json.Unmarshal(ackMessage.Payload, &errPayload)
			}
			return fmt.Errorf("%w: %v", ErrConnectionError, errPayload)
		default:
			return fmt.Errorf("%w: got %q", ErrAckNotReceived, ackMessage.Type)
		}
	}
}

// Subscribe implements Protocol.
func (p *graphqlWS) Subscribe(ctx context.Context, conn *websocket.Conn, id string, req *common.Request) error {
	msg := outgoingMessage{
		ID:      id,
		Type:    gwsTypeStart,
		Payload: req,
	}
	return wsjson.Write(ctx, conn, msg)
}

// Unsubscribe implements Protocol.
func (p *graphqlWS) Unsubscribe(ctx context.Context, conn *websocket.Conn, id string) error {
	msg := outgoingMessage{
		ID:   id,
		Type: gwsTypeStop,
	}
	return wsjson.Write(ctx, conn, msg)
}

// Read implements Protocol.
func (p *graphqlWS) Read(ctx context.Context, conn *websocket.Conn) (*WireMessage, error) {
	var raw incomingMessage
	if err := wsjson.Read(ctx, conn, &raw); err != nil {
		return nil, fmt.Errorf("read message: %w", err)
	}

	return p.decode(raw)
}

func (p *graphqlWS) decode(raw incomingMessage) (*WireMessage, error) {
	msg := &WireMessage{
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
			msg.Payload = &common.ExecutionResult{Errors: raw.Payload}
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
			_ = json.Unmarshal(raw.Payload, &errPayload)
		}
		msg.Err = fmt.Errorf("%w: %v", ErrConnectionError, errPayload)

	default:
		return nil, fmt.Errorf("unknown message type: %s", raw.Type)
	}

	return msg, nil
}

var _ Protocol = (*graphqlWS)(nil)
