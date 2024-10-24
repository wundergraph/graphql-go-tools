package graphql_datasource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"

	"github.com/buger/jsonparser"
	"github.com/jensneuse/abstractlogger"
)

// gqlTWSConnectionHandler is responsible for handling a connection to an origin
// it is responsible for managing all subscriptions using the underlying WebSocket connection
// if all Subscriptions are complete or cancelled/unsubscribed the handler will terminate
type gqlTWSConnectionHandler struct {
	// The underlying net.Conn. Only used for epoll. Should not be used to shutdown the connection.
	conn                          net.Conn
	requestContext, engineContext context.Context
	log                           abstractlogger.Logger
	options                       GraphQLSubscriptionOptions
	updater                       resolve.SubscriptionUpdater
}

func (h *gqlTWSConnectionHandler) ServerClose() {
	h.updater.Done()
	_ = ws.WriteFrame(h.conn, ws.MaskFrame(ws.NewCloseFrame(ws.NewCloseFrameBody(ws.StatusNormalClosure, "Normal Closure"))))
	_ = h.conn.Close()
}

func (h *gqlTWSConnectionHandler) ClientClose() {
	h.updater.Done()
	_ = wsutil.WriteClientText(h.conn, []byte(`{"id":"1","type":"complete"}`))
	_ = ws.WriteFrame(h.conn, ws.MaskFrame(ws.NewCloseFrame(ws.NewCloseFrameBody(ws.StatusNormalClosure, "Normal Closure"))))
	_ = h.conn.Close()
}

func (h *gqlTWSConnectionHandler) Subscribe() error {
	return h.subscribe()
}

func (h *gqlTWSConnectionHandler) ReadMessage() (done bool) {

	rw := readWriterPool.Get(h.conn)
	defer readWriterPool.Put(rw)

	for {
		err := h.conn.SetReadDeadline(time.Now().Add(time.Second))
		if err != nil {
			return handleConnectionError(err)
		}
		data, err := wsutil.ReadServerText(rw)
		if err != nil {
			return handleConnectionError(err)
		}
		messageType, err := jsonparser.GetString(data, "type")
		if err != nil {
			return false
		}
		switch messageType {
		case messageTypePing:
			h.handleMessageTypePing()
			continue
		case messageTypeNext:
			h.handleMessageTypeNext(data)
			continue
		case messageTypeComplete:
			h.handleMessageTypeComplete(data)
			return true
		case messageTypeError:
			h.handleMessageTypeError(data)
			continue
		case messageTypeConnectionKeepAlive:
			continue
		case messageTypeData, messageTypeConnectionError:
			h.log.Error("Invalid subprotocol. The subprotocol should be set to graphql-transport-ws, but currently it is set to graphql-ws")
			return true
		default:
			h.log.Error("unknown message type", abstractlogger.String("type", messageType))
			return false
		}
	}
}

func (h *gqlTWSConnectionHandler) NetConn() net.Conn {
	return h.conn
}

func newGQLTWSConnectionHandler(requestContext, engineContext context.Context, conn net.Conn, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater, l abstractlogger.Logger) *connection {
	handler := &gqlTWSConnectionHandler{
		conn:           conn,
		requestContext: requestContext,
		engineContext:  engineContext,
		log:            l,
		updater:        updater,
		options:        options,
	}
	return &connection{
		handler: handler,
		conn:    conn,
	}
}

func (h *gqlTWSConnectionHandler) StartBlocking() error {
	readCtx, cancel := context.WithCancel(h.requestContext)
	dataCh := make(chan []byte)
	errCh := make(chan error)

	defer func() {
		cancel()
		h.unsubscribeAllAndCloseConn()
	}()

	err := h.subscribe()
	if err != nil {
		return err
	}

	go h.readBlocking(readCtx, dataCh, errCh)

	for {
		select {
		case <-h.engineContext.Done():
			return h.engineContext.Err()
		case <-readCtx.Done():
			return readCtx.Err()
		case err := <-errCh:
			h.log.Error("gqlWSConnectionHandler.StartBlocking", abstractlogger.Error(err))
			h.broadcastErrorMessage(err)
			return err
		case data := <-dataCh:
			messageType, err := jsonparser.GetString(data, "type")
			if err != nil {
				continue
			}

			switch messageType {
			case messageTypePing:
				h.handleMessageTypePing()
				continue
			case messageTypeNext:
				h.handleMessageTypeNext(data)
				continue
			case messageTypeComplete:
				h.handleMessageTypeComplete(data)
				return nil
			case messageTypeError:
				h.handleMessageTypeError(data)
				continue
			case messageTypeConnectionKeepAlive:
				continue
			case messageTypeData, messageTypeConnectionError:
				h.log.Error("Invalid subprotocol. The subprotocol should be set to graphql-transport-ws, but currently it is set to graphql-ws")
				return errors.New("invalid subprotocol")
			default:
				h.log.Error("unknown message type", abstractlogger.String("type", messageType))
				continue
			}
		}
	}
}

func (h *gqlTWSConnectionHandler) unsubscribeAllAndCloseConn() {
	h.unsubscribe()
	_ = ws.WriteFrame(h.conn, ws.MaskFrame(ws.NewCloseFrame(ws.NewCloseFrameBody(ws.StatusNormalClosure, "Normal Closure"))))
	_ = h.conn.Close()
}

func (h *gqlTWSConnectionHandler) unsubscribe() {
	h.updater.Done()
	req := fmt.Sprintf(completeMessage, "1")
	err := wsutil.WriteClientText(h.conn, []byte(req))
	if err != nil {
		h.log.Error("failed to write complete message", abstractlogger.Error(err))
	}
}

// subscribe adds a new Subscription to the gqlTWSConnectionHandler and sends the subscribeMessage to the origin
func (h *gqlTWSConnectionHandler) subscribe() error {
	graphQLBody, err := json.Marshal(h.options.Body)
	if err != nil {
		return err
	}
	subscribeRequest := fmt.Sprintf(subscribeMessage, "1", string(graphQLBody))
	err = wsutil.WriteClientText(h.conn, []byte(subscribeRequest))
	if err != nil {
		return err
	}
	return nil
}

func (h *gqlTWSConnectionHandler) broadcastErrorMessage(err error) {
	errMsg := fmt.Sprintf(errorMessageTemplate, err)
	h.updater.Update([]byte(errMsg))
}

func (h *gqlTWSConnectionHandler) handleMessageTypeComplete(data []byte) {
	id, err := jsonparser.GetString(data, "id")
	if err != nil {
		return
	}
	if id != "1" {
		return
	}
	h.updater.Done()
}

func (h *gqlTWSConnectionHandler) handleMessageTypeError(data []byte) {
	id, err := jsonparser.GetString(data, "id")
	if err != nil {
		return
	}
	if id != "1" {
		return
	}
	value, valueType, _, err := jsonparser.Get(data, "payload")
	if err != nil {
		h.log.Error(
			"failed to get payload from error message",
			abstractlogger.Error(err),
			abstractlogger.ByteString("raw message", data),
		)
		h.updater.Update([]byte(internalError))
		return
	}

	switch valueType {
	case jsonparser.Array:
		response := []byte(`{}`)
		response, err = jsonparser.Set(response, value, "errors")
		if err != nil {
			h.log.Error(
				"failed to set errors response",
				abstractlogger.Error(err),
				abstractlogger.ByteString("raw message", value),
			)
			h.updater.Update([]byte(internalError))
			return
		}
		h.updater.Update(response)
	case jsonparser.Object:
		response := []byte(`{"errors":[]}`)
		response, err = jsonparser.Set(response, value, "errors", "[0]")
		if err != nil {
			h.updater.Update([]byte(internalError))
			return
		}
		h.updater.Update(response)
	default:
		h.updater.Update([]byte(internalError))
	}
}

func (h *gqlTWSConnectionHandler) handleMessageTypePing() {
	err := wsutil.WriteClientText(h.conn, []byte(pongMessage))
	if err != nil {
		h.log.Error("failed to write pong message", abstractlogger.Error(err))
	}
}

func (h *gqlTWSConnectionHandler) handleMessageTypeNext(data []byte) {
	id, err := jsonparser.GetString(data, "id")
	if err != nil {
		return
	}
	if id != "1" {
		return
	}
	value, _, _, err := jsonparser.Get(data, "payload")
	if err != nil {
		h.log.Error(
			"failed to get payload from next message",
			abstractlogger.Error(err),
		)
		h.updater.Update([]byte(internalError))
		return
	}

	h.updater.Update(value)
}

// readBlocking is a dedicated loop running in a separate goroutine
// because the library "github.com/coder/websocket" doesn't allow reading with a context with Timeout
// we'll block forever on reading until the context of the gqlTWSConnectionHandler stops
func (h *gqlTWSConnectionHandler) readBlocking(ctx context.Context, dataCh chan []byte, errCh chan error) {
	netOpErr := &net.OpError{}
	for {
		data, err := wsutil.ReadServerText(h.conn)
		if err != nil {
			if errors.As(err, &netOpErr) {
				if netOpErr.Timeout() {
					select {
					case <-ctx.Done():
						return
					default:
						continue
					}
				}
			}
			select {
			case errCh <- err:
			case <-ctx.Done():
			}
			return
		}
		select {
		case dataCh <- data:
		case <-ctx.Done():
			return
		}
	}
}
