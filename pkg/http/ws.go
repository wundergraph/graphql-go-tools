package http

import (
	"context"
	"encoding/json"
	"net"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/jensneuse/abstractlogger"

	"github.com/jensneuse/graphql-go-tools/pkg/subscription"
)

type WebsocketSubscriptionClient struct {
	logger     abstractlogger.Logger
	clientConn net.Conn
}

func NewWebsocketSubscriptionClient(logger abstractlogger.Logger, clientConn net.Conn) *WebsocketSubscriptionClient {
	return &WebsocketSubscriptionClient{
		logger:     logger,
		clientConn: clientConn,
	}
}

func (w *WebsocketSubscriptionClient) ReadFromClient() (message subscription.Message, err error) {
	data := make([]byte, 0, 1024)
	var opCode ws.OpCode

	data, opCode, err = wsutil.ReadClientData(w.clientConn)
	if err != nil {
		w.logger.Error("http.WebsocketSubscriptionClient.ReadFromClient()",
			abstractlogger.Error(err),
			abstractlogger.ByteString("data", data),
			abstractlogger.Any("opCode", opCode),
		)

		return message, err
	}

	err = json.Unmarshal(data, &message)
	if err != nil {
		w.logger.Error("http.WebsocketSubscriptionClient.ReadFromClient()",
			abstractlogger.Error(err),
			abstractlogger.ByteString("data", data),
			abstractlogger.Any("opCode", opCode),
		)

		return message, err
	}

	return message, nil
}

func (w *WebsocketSubscriptionClient) WriteToClient(message subscription.Message) error {
	messageBytes, err := json.Marshal(message)
	if err != nil {
		w.logger.Error("http.WebsocketSubscriptionClient.WriteToClient()",
			abstractlogger.Error(err),
			abstractlogger.Any("message", message),
		)

		return err
	}

	err = wsutil.WriteServerMessage(w.clientConn, ws.OpText, messageBytes)
	if err != nil {
		w.logger.Error("http.WebsocketSubscriptionClient.WriteToClient()",
			abstractlogger.Error(err),
			abstractlogger.ByteString("messageBytes", messageBytes),
		)

		return err
	}

	return nil
}

func (w *WebsocketSubscriptionClient) IsConnected() bool {
	return true // TODO: Find a solution
}

func (w *WebsocketSubscriptionClient) Disconnect() error {
	return w.clientConn.Close()
}

func (g *GraphQLHTTPRequestHandler) handleWebsocket(conn net.Conn) {
	defer func() {
		err := conn.Close()
		if err != nil {
			g.log.Error("http.GraphQLHTTPRequestHandler.handleWebsocket()",
				abstractlogger.String("message", "could not close connection to client"),
				abstractlogger.Error(err),
			)
		}
	}()

	websocketClient := NewWebsocketSubscriptionClient(g.log, conn)
	subscriptionHandler, err := subscription.NewHandler(g.log, websocketClient, g.executionHandler)
	if err != nil {
		g.log.Error("http.GraphQLHTTPRequestHandler.handleWebsocket()",
			abstractlogger.String("message", "could not create subscriptionHandler"),
			abstractlogger.Error(err),
		)

		return
	}

	subscriptionHandler.Handle(context.Background())
}
