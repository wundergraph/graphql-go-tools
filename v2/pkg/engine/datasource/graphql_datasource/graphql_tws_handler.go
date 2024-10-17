package graphql_datasource

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gobwas/ws/wsutil"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"

	"github.com/buger/jsonparser"
	log "github.com/jensneuse/abstractlogger"
)

// gqlTWSConnectionHandler is responsible for handling a connection to an origin
// it is responsible for managing all subscriptions using the underlying WebSocket connection
// if all Subscriptions are complete or cancelled/unsubscribed the handler will terminate
type gqlTWSConnectionHandler struct {
	conn               net.Conn
	ctx                context.Context
	log                log.Logger
	subscribeCh        chan Subscription
	nextSubscriptionID int
	subscriptions      map[string]Subscription
	readTimeout        time.Duration
}

func (h *gqlTWSConnectionHandler) ServerClose() {
	for _, sub := range h.subscriptions {
		sub.updater.Done()
	}
	_ = h.conn.Close()
}

func (h *gqlTWSConnectionHandler) ClientClose() {
	for k, v := range h.subscriptions {
		v.updater.Done()
		delete(h.subscriptions, k)

		req := fmt.Sprintf(completeMessage, k)
		err := wsutil.WriteClientText(h.conn, []byte(req))
		if err != nil {
			h.log.Error("failed to write complete message", log.Error(err))
		}
	}
	_ = h.conn.Close()
}

func (h *gqlTWSConnectionHandler) Subscribe(sub Subscription) error {
	return h.subscribe(sub)
}

func (h *gqlTWSConnectionHandler) ReadMessage() (done, timeout bool) {

	r := bufio.NewReader(h.conn)
	wr := bufio.NewWriter(h.conn)
	rwr := bufio.NewReadWriter(r, wr)

	for {

		err := h.conn.SetReadDeadline(time.Now().Add(time.Second))
		if err != nil {
			return h.handleConnectionError(err)
		}

		data, err := wsutil.ReadServerText(rwr)
		if err != nil {
			return h.handleConnectionError(err)
		}

		messageType, err := jsonparser.GetString(data, "type")
		if err != nil {
			return false, false
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
			return true, false
		case messageTypeError:
			h.handleMessageTypeError(data)
			continue
		case messageTypeConnectionKeepAlive:
			continue
		case messageTypeData, messageTypeConnectionError:
			h.log.Error("Invalid subprotocol. The subprotocol should be set to graphql-transport-ws, but currently it is set to graphql-ws")
			return true, false
		default:
			h.log.Error("unknown message type", log.String("type", messageType))
			return false, false
		}
	}
}

func (h *gqlTWSConnectionHandler) handleConnectionError(err error) (closed, timeout bool) {
	if errors.Is(err, context.DeadlineExceeded) {
		return false, true
	}
	netOpErr := &net.OpError{}
	if errors.As(err, &netOpErr) {
		if netOpErr.Timeout() {
			return false, true
		}
		return true, false
	}
	if errors.As(err, &wsutil.ClosedError{}) {
		return true, false
	}
	if strings.HasSuffix(err.Error(), "use of closed network connection") {
		return true, false
	}
	return false, false
}

func (h *gqlTWSConnectionHandler) NetConn() net.Conn {
	return h.conn
}

func newGQLTWSConnectionHandler(ctx context.Context, conn net.Conn, rt time.Duration, l log.Logger) *gqlTWSConnectionHandler {
	return &gqlTWSConnectionHandler{
		conn:               conn,
		ctx:                ctx,
		log:                l,
		nextSubscriptionID: 0,
		subscriptions:      map[string]Subscription{},
		readTimeout:        rt,
	}
}

func (h *gqlTWSConnectionHandler) StartBlocking(sub Subscription) {
	readCtx, cancel := context.WithCancel(h.ctx)
	dataCh := make(chan []byte)
	errCh := make(chan error)

	defer func() {
		h.unsubscribeAllAndCloseConn()
		cancel()
	}()

	h.subscribe(sub)

	go h.readBlocking(readCtx, dataCh, errCh)

	ticker := time.NewTicker(resolve.HearbeatInterval)
	defer ticker.Stop()

	for {
		err := h.ctx.Err()
		if err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				h.log.Error("gqlWSConnectionHandler.StartBlocking", log.Error(err))
			}
			h.broadcastErrorMessage(err)
			return
		}

		hasActiveSubscriptions := h.hasActiveSubscriptions()
		if !hasActiveSubscriptions {
			return
		}

		select {
		case <-readCtx.Done():
			return
		case <-time.After(h.readTimeout):
			continue
		case err := <-errCh:
			h.log.Error("gqlWSConnectionHandler.StartBlocking", log.Error(err))
			h.broadcastErrorMessage(err)
			return
		case <-ticker.C:
			sub.updater.Heartbeat()
		case data := <-dataCh:
			ticker.Reset(resolve.HearbeatInterval)
			messageType, err := jsonparser.GetString(data, "type")
			if err != nil {
				continue
			}

			switch messageType {
			case messageTypePing:
				h.handleMessageTypePing()
			case messageTypeNext:
				h.handleMessageTypeNext(data)
			case messageTypeComplete:
				h.handleMessageTypeComplete(data)
			case messageTypeError:
				h.handleMessageTypeError(data)
				continue
			case messageTypeConnectionKeepAlive:
				continue
			case messageTypeData, messageTypeConnectionError:
				h.log.Error("Invalid subprotocol. The subprotocol should be set to graphql-transport-ws, but currently it is set to graphql-ws")
				return
			default:
				h.log.Error("unknown message type", log.String("type", messageType))
				continue
			}
		}
	}
}

func (h *gqlTWSConnectionHandler) unsubscribeAllAndCloseConn() {
	for id := range h.subscriptions {
		h.unsubscribe(id)
	}
	_ = h.conn.Close()
}

func (h *gqlTWSConnectionHandler) unsubscribe(subscriptionID string) {
	sub, ok := h.subscriptions[subscriptionID]
	if !ok {
		return
	}
	sub.updater.Done()
	delete(h.subscriptions, subscriptionID)

	req := fmt.Sprintf(completeMessage, subscriptionID)
	err := wsutil.WriteClientText(h.conn, []byte(req))
	if err != nil {
		h.log.Error("failed to write complete message", log.Error(err))
	}
}

// subscribe adds a new Subscription to the gqlTWSConnectionHandler and sends the subscribeMessage to the origin
func (h *gqlTWSConnectionHandler) subscribe(sub Subscription) error {
	graphQLBody, err := json.Marshal(sub.options.Body)
	if err != nil {
		return err
	}

	h.nextSubscriptionID++

	subscriptionID := strconv.Itoa(h.nextSubscriptionID)
	subscribeRequest := fmt.Sprintf(subscribeMessage, subscriptionID, string(graphQLBody))
	err = wsutil.WriteClientText(h.conn, []byte(subscribeRequest))
	if err != nil {
		return err
	}

	h.subscriptions[subscriptionID] = sub
	return nil
}

func (h *gqlTWSConnectionHandler) broadcastErrorMessage(err error) {
	errMsg := fmt.Sprintf(errorMessageTemplate, err)
	for _, sub := range h.subscriptions {
		sub.updater.Update([]byte(errMsg))
	}
}

func (h *gqlTWSConnectionHandler) handleMessageTypeComplete(data []byte) {
	id, err := jsonparser.GetString(data, "id")
	if err != nil {
		return
	}
	sub, ok := h.subscriptions[id]
	if !ok {
		return
	}
	sub.updater.Done()
	delete(h.subscriptions, id)
}

func (h *gqlTWSConnectionHandler) handleMessageTypeError(data []byte) {
	id, err := jsonparser.GetString(data, "id")
	if err != nil {
		return
	}
	sub, ok := h.subscriptions[id]
	if !ok {
		return
	}

	value, valueType, _, err := jsonparser.Get(data, "payload")
	if err != nil {
		h.log.Error(
			"failed to get payload from error message",
			log.Error(err),
			log.ByteString("raw message", data),
		)
		sub.updater.Update([]byte(internalError))
		return
	}

	switch valueType {
	case jsonparser.Array:
		response := []byte(`{}`)
		response, err = jsonparser.Set(response, value, "errors")
		if err != nil {
			h.log.Error(
				"failed to set errors response",
				log.Error(err),
				log.ByteString("raw message", value),
			)
			sub.updater.Update([]byte(internalError))
			return
		}
		sub.updater.Update(response)
	case jsonparser.Object:
		response := []byte(`{"errors":[]}`)
		response, err = jsonparser.Set(response, value, "errors", "[0]")
		if err != nil {
			sub.updater.Update([]byte(internalError))
			return
		}
		sub.updater.Update(response)
	default:
		sub.updater.Update([]byte(internalError))
	}
}

func (h *gqlTWSConnectionHandler) handleMessageTypePing() {
	err := wsutil.WriteClientText(h.conn, []byte(pongMessage))
	if err != nil {
		h.log.Error("failed to write pong message", log.Error(err))
	}
}

func (h *gqlTWSConnectionHandler) handleMessageTypeNext(data []byte) {
	id, err := jsonparser.GetString(data, "id")
	if err != nil {
		return
	}
	sub, ok := h.subscriptions[id]
	if !ok {
		return
	}

	value, _, _, err := jsonparser.Get(data, "payload")
	if err != nil {
		h.log.Error(
			"failed to get payload from next message",
			log.Error(err),
		)
		sub.updater.Update([]byte(internalError))
		return
	}

	sub.updater.Update(value)
}

// readBlocking is a dedicated loop running in a separate goroutine
// because the library "nhooyr.io/websocket" doesn't allow reading with a context with Timeout
// we'll block forever on reading until the context of the gqlTWSConnectionHandler stops
func (h *gqlTWSConnectionHandler) readBlocking(ctx context.Context, dataCh chan []byte, errCh chan error) {
	for {
		data, err := wsutil.ReadServerText(h.conn)
		if err != nil {
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

func (h *gqlTWSConnectionHandler) hasActiveSubscriptions() (hasActiveSubscriptions bool) {
	for id, sub := range h.subscriptions {
		if sub.ctx.Err() != nil {
			h.unsubscribe(id)
		}
	}
	return len(h.subscriptions) != 0
}
