package http

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/jensneuse/graphql-go-tools/pkg/execution"
	"go.uber.org/zap"
	"net"
	"net/http"
)

const (
	CONNECTION_INIT       = "connection_init"
	CONNECTION_ACK        = "connection_ack"
	CONNECTION_ERROR      = "connection_error"
	CONNECTION_KEEP_ALIVE = "ka"
	START                 = "start"
	STOP                  = "stop"
	CONNECTION_TERMINATE  = "connection_terminate"
	DATA                  = "data"
	ERROR                 = "error"
	COMPLETE              = "complete"
)

type WebsocketMessage struct {
	Id      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func (g *GraphQLHTTPRequestHandler) handleWebsocket(r *http.Request, conn net.Conn) {
	defer conn.Close()

	subscriptions := map[string]context.CancelFunc{}

	defer func() {
		for _, cancel := range subscriptions {
			cancel()
		}
	}()

	for {
		data, op, err := wsutil.ReadClientData(conn)
		if err != nil {
			g.log.Error("GraphQLHTTPRequestHandler.handleWebsocket",
				zap.Error(err),
				zap.ByteString("message", data),
			)
			return
		}
		g.log.Debug("GraphQLHTTPRequestHandler.handleWebsocket",
			zap.ByteString("message", data),
			zap.String("opCode", string(op)),
		)
		var message WebsocketMessage
		err = json.Unmarshal(data, &message)
		if err != nil {
			g.log.Debug("GraphQLHTTPRequestHandler.handleClientMessage",
				zap.ByteString("message", data),
				zap.String("opCode", string(op)),
			)
			return
		}

		switch message.Type {
		case CONNECTION_INIT:
			err = g.sendAck(conn, op)
			if err != nil {
				g.log.Debug("GraphQLHTTPRequestHandler.sendAck",
					zap.ByteString("message", data),
					zap.String("opCode", string(op)),
				)
				return
			}
		case START:
			ctx, cancel := context.WithCancel(context.Background())
			subscriptions[message.Id] = cancel
			go g.startSubscription(r, ctx, message.Payload, conn, op, message.Id)
		case STOP:
			cancel, ok := subscriptions[message.Id]
			if !ok {
				continue
			}
			cancel()
			delete(subscriptions, message.Id)
		}
	}
}

func (g *GraphQLHTTPRequestHandler) sendAck(conn net.Conn, op ws.OpCode) error {
	data, err := json.Marshal(WebsocketMessage{
		Type: CONNECTION_ACK,
	})
	if err != nil {
		return err
	}
	return wsutil.WriteServerMessage(conn, op, data)
}

func (g *GraphQLHTTPRequestHandler) startSubscription(r *http.Request, ctx context.Context, data []byte, conn net.Conn, op ws.OpCode, id string) {

	extra := &bytes.Buffer{}
	_ = g.extraVariables(r, extra)

	executor, node, executionContext, err := g.executionHandler.Handle(data, extra.Bytes())
	if err != nil {
		g.log.Error("GraphQLHTTPRequestHandler.startSubscription.executionHandler.Handle",
			zap.Error(err),
			zap.ByteString("data", data),
		)
		return
	}

	executionContext.Context = ctx

	buf := bytes.NewBuffer(make([]byte, 0, 1024))

	for {
		buf.Reset()
		select {
		case <-ctx.Done():
			return
		default:
			instructions, err := executor.Execute(executionContext, node, buf)
			if err != nil {
				g.log.Error("GraphQLHTTPRequestHandler.startSubscription.executor.Execute",
					zap.Error(err),
					zap.ByteString("data", data),
				)
			}

			g.log.Debug("GraphQLHTTPRequestHandler.startSubscription",
				zap.ByteString("execution_result", buf.Bytes()),
			)

			response := WebsocketMessage{
				Type:    DATA,
				Id:      id,
				Payload: buf.Bytes(),
			}

			responseData, err := json.Marshal(response)
			if err != nil {
				g.log.Error("GraphQLHTTPRequestHandler.startSubscription.json.Marshal",
					zap.Error(err),
				)
				return
			}

			err = wsutil.WriteServerMessage(conn, op, responseData)
			if err != nil {
				g.log.Error("GraphQLHTTPRequestHandler.startSubscription.wsutil.WriteServerMessage",
					zap.Error(err),
					zap.ByteString("data", data),
				)
				return
			}

			for i := 0; i < len(instructions); i++ {
				switch instructions[i] {
				case execution.CloseConnection:
					err = g.sendCloseMessage(id, conn, op)
					if err != nil {
						g.log.Error("GraphQLHTTPRequestHandler.startSubscription.sendCloseMessage",
							zap.Error(err),
						)
					}
					return
				}
			}
		}
	}
}

func (g *GraphQLHTTPRequestHandler) sendCloseMessage(id string, conn net.Conn, op ws.OpCode) error {
	data, err := json.Marshal(WebsocketMessage{
		Id:   id,
		Type: STOP,
	})
	if err != nil {
		return err
	}
	return wsutil.WriteServerMessage(conn, op, data)
}
