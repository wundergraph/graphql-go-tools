package graphql_datasource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
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
	// The underlying net.Conn. Only used for netPoll. Should not be used to shutdown the connection.
	conn                          net.Conn
	requestContext, engineContext context.Context
	log                           abstractlogger.Logger
	options                       GraphQLSubscriptionOptions
	updater                       resolve.SubscriptionUpdater
	lastPingSentUnix              atomic.Int64 // Unix timestamp in nanoseconds, 0 means no ping in flight
	pingTimeout                   time.Duration
}

func (h *gqlTWSConnectionHandler) ServerClose() {
	// Because the server closes the connection, we need to send a close frame to the event loop.
	h.updater.Done()
	_ = h.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	_ = ws.WriteFrame(h.conn, ws.MaskFrame(ws.NewCloseFrame(ws.NewCloseFrameBody(ws.StatusNormalClosure, "Normal Closure"))))
	_ = h.conn.Close()
}

// ClientClose is called when the client closes the connection. Is called when the trigger is shutdown with all subscriptions.
func (h *gqlTWSConnectionHandler) ClientClose() {
	_ = h.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	_ = wsutil.WriteClientText(h.conn, []byte(`{"id":"1","type":"complete"}`))
	_ = h.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	_ = ws.WriteFrame(h.conn, ws.MaskFrame(ws.NewCloseFrame(ws.NewCloseFrameBody(ws.StatusNormalClosure, "Normal Closure"))))
	_ = h.conn.Close()
}

func (h *gqlTWSConnectionHandler) Subscribe() error {
	return h.subscribe()
}

func (h *gqlTWSConnectionHandler) HandleMessage(data []byte) (done bool) {
	messageType, err := jsonparser.GetString(data, "type")
	if err != nil {
		return false
	}
	switch messageType {
	case messageTypePing:
		h.handleMessageTypePing()
		return false
	case messageTypeNext:
		h.handleMessageTypeNext(data)
		return false
	case messageTypeComplete:
		h.handleMessageTypeComplete(data)
		return true
	case messageTypeError:
		h.handleMessageTypeError(data)
		return false
	case messageTypeConnectionKeepAlive:
		return false
	case messageTypeData, messageTypeConnectionError:
		h.log.Error("Invalid subprotocol. The subprotocol should be set to graphql-transport-ws, but currently it is set to graphql-ws")
		return true
	case messageTypePong:
		h.handleMessageTypePong()
		return false
	default:
		h.log.Error("unknown message type", abstractlogger.String("type", messageType))
		return false
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
		pingTimeout:    options.pingTimeout,
	}

	// Initialize atomic field to 0 (no ping in flight)
	handler.lastPingSentUnix.Store(0)

	return &connection{
		handler: handler,
		netConn: conn,
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

	pingTicker := time.NewTicker(h.options.pingInterval)
	defer pingTicker.Stop()

	go h.readBlocking(readCtx, h.options.readTimeout, dataCh, errCh)

	for {
		select {
		case <-h.engineContext.Done():
			return h.engineContext.Err()
		case <-readCtx.Done():
			return readCtx.Err()
		case <-pingTicker.C:
			h.Ping()
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
			case messageTypePong:
				h.handleMessageTypePong()
				continue
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
	_ = h.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	_ = ws.WriteFrame(h.conn, ws.MaskFrame(ws.NewCloseFrame(ws.NewCloseFrameBody(ws.StatusNormalClosure, "Normal Closure"))))
	_ = h.conn.Close()
}

func (h *gqlTWSConnectionHandler) unsubscribe() {
	h.updater.Done()
	req := fmt.Sprintf(completeMessage, "1")
	_ = h.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
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
	if err = h.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	if err = wsutil.WriteClientText(h.conn, []byte(subscribeRequest)); err != nil {
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
	_ = h.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	err := wsutil.WriteClientText(h.conn, []byte(pongMessage))
	if err != nil {
		h.log.Error("failed to write pong message", abstractlogger.Error(err))
	}
}

func (h *gqlTWSConnectionHandler) handleMessageTypePong() {
	// Reset timestamp to indicate no ping in flight
	h.lastPingSentUnix.Store(0)
}

func (h *gqlTWSConnectionHandler) Ping() {
	// Get current timestamp of last ping
	lastPingTimestamp := h.lastPingSentUnix.Load()

	// If a ping is in flight, check if it has timed out
	if lastPingTimestamp > 0 {
		pingTime := time.Unix(0, lastPingTimestamp)
		if time.Since(pingTime) > h.pingTimeout {
			h.log.Error("ping timeout exceeded. Closing connection")
			// Reset timestamp to avoid duplicate closes if Ping gets called again
			h.lastPingSentUnix.Store(0)
			h.ServerClose()
			return
		}
	}

	// Only send a new ping if we haven't sent one yet, or if we received a pong for the previous one
	// (indicated by lastPingSentUnix being 0)
	if lastPingTimestamp == 0 {
		_ = h.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		err := wsutil.WriteClientText(h.conn, []byte(pingMessage))

		if err != nil {
			h.log.Error("failed to write ping message", abstractlogger.Error(err))
			return
		}

		// Store current time as Unix timestamp in nanoseconds
		h.lastPingSentUnix.Store(time.Now().UnixNano())
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
func (h *gqlTWSConnectionHandler) readBlocking(ctx context.Context, readTimeout time.Duration, dataCh chan []byte, errCh chan error) {
	netOpErr := &net.OpError{}
	for {
		_ = h.conn.SetReadDeadline(time.Now().Add(readTimeout))
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
