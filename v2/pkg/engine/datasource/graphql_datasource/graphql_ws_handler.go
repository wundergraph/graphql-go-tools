package graphql_datasource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"

	"github.com/buger/jsonparser"
	"github.com/jensneuse/abstractlogger"
)

// gqlWSConnectionHandler is responsible for handling a connection to an origin
// it is responsible for managing all subscriptions using the underlying WebSocket connection
// if all Subscriptions are complete or cancelled/unsubscribed the handler will terminate
type gqlWSConnectionHandler struct {
	// The underlying net.Conn. Only used for netPoll. Should not be used to shutdown the connection.
	conn                          net.Conn
	requestContext, engineContext context.Context
	log                           abstractlogger.Logger
	options                       GraphQLSubscriptionOptions
	updater                       resolve.SubscriptionUpdater
}

func (h *gqlWSConnectionHandler) ServerClose() {
	h.updater.Done()
	_ = ws.WriteFrame(h.conn, ws.MaskFrame(ws.NewCloseFrame(ws.NewCloseFrameBody(ws.StatusNormalClosure, "Normal Closure"))))
	_ = h.conn.Close()
}

func (h *gqlWSConnectionHandler) ClientClose() {
	h.updater.Done()
	_ = wsutil.WriteClientText(h.conn, []byte(`{"type":"stop","id":"1"}`))
	_ = ws.WriteFrame(h.conn, ws.MaskFrame(ws.NewCloseFrame(ws.NewCloseFrameBody(ws.StatusNormalClosure, "Normal Closure"))))
	_ = h.conn.Close()
}

func (h *gqlWSConnectionHandler) Subscribe() error {
	return h.subscribe()
}

func (h *gqlWSConnectionHandler) HandleMessage(data []byte) (done bool) {
	messageType, err := jsonparser.GetString(data, "type")
	if err != nil {
		return false
	}
	switch messageType {
	case messageTypeConnectionKeepAlive:
		return false
	case messageTypeData:
		h.handleMessageTypeData(data)
		return false
	case messageTypeComplete:
		h.handleMessageTypeComplete(data)
		return true
	case messageTypeConnectionError:
		h.handleMessageTypeConnectionError()
		return true
	case messageTypeError:
		h.handleMessageTypeError(data)
		return false
	default:
		return false
	}
}

func (h *gqlWSConnectionHandler) NetConn() net.Conn {
	return h.conn
}

func newGQLWSConnectionHandler(requestContext, engineContext context.Context, conn net.Conn, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater, log abstractlogger.Logger) *connection {
	handler := &gqlWSConnectionHandler{
		conn:           conn,
		requestContext: requestContext,
		engineContext:  engineContext,
		log:            log,
		updater:        updater,
		options:        options,
	}
	return &connection{
		handler: handler,
		netConn: conn,
	}
}

// StartBlocking starts the single threaded event loop of the handler
// if the global context returns or the websocket connection is terminated, it will stop
func (h *gqlWSConnectionHandler) StartBlocking() error {
	dataCh := make(chan []byte)
	errCh := make(chan error)
	readCtx, cancel := context.WithCancel(h.requestContext)

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
			if !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				h.log.Error("gqlWSConnectionHandler.StartBlocking", abstractlogger.Error(err))
			}
			h.broadcastErrorMessage(err)
			return err
		case data := <-dataCh:
			messageType, err := jsonparser.GetString(data, "type")
			if err != nil {
				continue
			}
			switch messageType {
			case messageTypeData:
				h.handleMessageTypeData(data)
				continue
			case messageTypeComplete:
				h.handleMessageTypeComplete(data)
				return nil
			case messageTypeConnectionError:
				h.handleMessageTypeConnectionError()
				return nil
			case messageTypeError:
				h.handleMessageTypeError(data)
				continue
			case messageTypeConnectionKeepAlive:
				continue
			case messageTypePing, messageTypeNext:
				h.log.Error("Invalid subprotocol. The subprotocol should be set to graphql-ws, but currently it is set to graphql-transport-ws")
				return errors.New("invalid subprotocol")
			default:
				h.log.Error("unknown message type", abstractlogger.String("type", messageType))
				continue
			}
		}
	}
}

// readBlocking is a dedicated loop running in a separate goroutine
// because the library "github.com/coder/websocket" doesn't allow reading with a context with Timeout
// we'll block forever on reading until the context of the gqlWSConnectionHandler stops
func (h *gqlWSConnectionHandler) readBlocking(ctx context.Context, dataCh chan []byte, errCh chan error) {
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

func (h *gqlWSConnectionHandler) unsubscribeAllAndCloseConn() {
	h.unsubscribe()
	_ = ws.WriteFrame(h.conn, ws.MaskFrame(ws.NewCloseFrame(ws.NewCloseFrameBody(ws.StatusNormalClosure, "Normal Closure"))))
	_ = h.conn.Close()
}

// subscribe adds a new Subscription to the gqlWSConnectionHandler and sends the startMessage to the origin
func (h *gqlWSConnectionHandler) subscribe() error {
	graphQLBody, err := json.Marshal(h.options.Body)
	if err != nil {
		return err
	}
	startRequest := fmt.Sprintf(startMessage, "1", string(graphQLBody))
	err = wsutil.WriteClientText(h.conn, []byte(startRequest))
	if err != nil {
		return err
	}
	return nil
}

func (h *gqlWSConnectionHandler) handleMessageTypeData(data []byte) {
	id, err := jsonparser.GetString(data, "id")
	if err != nil {
		return
	}
	if id != "1" {
		return
	}
	payload, _, _, err := jsonparser.Get(data, "payload")
	if err != nil {
		return
	}

	h.updater.Update(payload)
}

func (h *gqlWSConnectionHandler) handleMessageTypeConnectionError() {
	h.updater.Update([]byte(connectionError))
}

func (h *gqlWSConnectionHandler) broadcastErrorMessage(err error) {
	errMsg := fmt.Sprintf(errorMessageTemplate, err)
	h.updater.Update([]byte(errMsg))
}

func (h *gqlWSConnectionHandler) handleMessageTypeComplete(data []byte) {
	id, err := jsonparser.GetString(data, "id")
	if err != nil {
		return
	}
	if id != "1" {
		return
	}
	h.updater.Done()
}

func (h *gqlWSConnectionHandler) handleMessageTypeError(data []byte) {
	id, err := jsonparser.GetString(data, "id")
	if err != nil {
		return
	}
	if id != "1" {
		return
	}
	value, valueType, _, err := jsonparser.Get(data, "payload")
	if err != nil {
		h.updater.Update([]byte(internalError))
		return
	}
	switch valueType {
	case jsonparser.Array:
		response := []byte(`{}`)
		response, err = jsonparser.Set(response, value, "errors")
		if err != nil {
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

func (h *gqlWSConnectionHandler) unsubscribe() {
	h.updater.Done()
	stopRequest := fmt.Sprintf(stopMessage, "1")
	_ = wsutil.WriteClientText(h.conn, []byte(stopRequest))
}
