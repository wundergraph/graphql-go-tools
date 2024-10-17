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
	"github.com/jensneuse/abstractlogger"
)

// gqlWSConnectionHandler is responsible for handling a connection to an origin
// it is responsible for managing all subscriptions using the underlying WebSocket connection
// if all Subscriptions are complete or cancelled/unsubscribed the handler will terminate
type gqlWSConnectionHandler struct {
	conn net.Conn
	ctx  context.Context
	log  abstractlogger.Logger
	// log                slog.Logger
	subscribeCh        chan Subscription
	nextSubscriptionID int
	subscriptions      map[string]Subscription
	readTimeout        time.Duration
}

func (h *gqlWSConnectionHandler) ServerClose() {
	for _, sub := range h.subscriptions {
		sub.updater.Done()
	}
	_ = h.conn.Close()
}

func (h *gqlWSConnectionHandler) ClientClose() {
	for k, v := range h.subscriptions {
		v.updater.Done()
		delete(h.subscriptions, k)
		stopRequest := fmt.Sprintf(stopMessage, k)
		_ = wsutil.WriteClientText(h.conn, []byte(stopRequest))
	}
	_ = h.conn.Close()
}

func (h *gqlWSConnectionHandler) Subscribe(sub Subscription) {
	h.subscribe(sub)
}

func (h *gqlWSConnectionHandler) ReadMessage() (done, timeout bool) {

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
		case messageTypeConnectionKeepAlive:
			continue
		case messageTypeData:
			h.handleMessageTypeData(data)
			continue
		case messageTypeComplete:
			h.handleMessageTypeComplete(data)
			return true, false
		case messageTypeConnectionError:
			h.handleMessageTypeConnectionError()
			return true, false
		case messageTypeError:
			h.handleMessageTypeError(data)
			continue
		default:
			return true, false
		}
	}
}

func (h *gqlWSConnectionHandler) handleConnectionError(err error) (closed, timeout bool) {
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

func (h *gqlWSConnectionHandler) NetConn() net.Conn {
	return h.conn
}

func newGQLWSConnectionHandler(ctx context.Context, conn net.Conn, readTimeout time.Duration, log abstractlogger.Logger) *gqlWSConnectionHandler {
	return &gqlWSConnectionHandler{
		conn:               conn,
		ctx:                ctx,
		log:                log,
		nextSubscriptionID: 0,
		subscriptions:      map[string]Subscription{},
		readTimeout:        readTimeout,
	}
}

// StartBlocking starts the single threaded event loop of the handler
// if the global context returns or the websocket connection is terminated, it will stop
func (h *gqlWSConnectionHandler) StartBlocking(sub Subscription) {
	dataCh := make(chan []byte)
	errCh := make(chan error)
	readCtx, cancel := context.WithCancel(h.ctx)

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
			if !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
				h.log.Error("gqlWSConnectionHandler.StartBlocking", abstractlogger.Error(err))
			}
			h.broadcastErrorMessage(err)
			return
		}
		hasActiveSubscriptions := h.checkActiveSubscriptions()
		if !hasActiveSubscriptions {
			return
		}
		select {
		case <-readCtx.Done():
			return
		case <-time.After(h.readTimeout):
			continue
		case err = <-errCh:
			if !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				h.log.Error("gqlWSConnectionHandler.StartBlocking", abstractlogger.Error(err))
			}
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
			case messageTypeData:
				h.handleMessageTypeData(data)
			case messageTypeComplete:
				h.handleMessageTypeComplete(data)
			case messageTypeConnectionError:
				h.handleMessageTypeConnectionError()
				return
			case messageTypeError:
				h.handleMessageTypeError(data)
				continue
			case messageTypeConnectionKeepAlive:
				continue
			case messageTypePing, messageTypeNext:
				h.log.Error("Invalid subprotocol. The subprotocol should be set to graphql-ws, but currently it is set to graphql-transport-ws")
				return
			default:
				h.log.Error("unknown message type", abstractlogger.String("type", messageType))
				continue
			}
		}
	}
}

// readBlocking is a dedicated loop running in a separate goroutine
// because the library "nhooyr.io/websocket" doesn't allow reading with a context with Timeout
// we'll block forever on reading until the context of the gqlWSConnectionHandler stops
func (h *gqlWSConnectionHandler) readBlocking(ctx context.Context, dataCh chan []byte, errCh chan error) {
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

func (h *gqlWSConnectionHandler) unsubscribeAllAndCloseConn() {
	for id := range h.subscriptions {
		h.unsubscribe(id)
	}
	_ = h.conn.Close()
}

// subscribe adds a new Subscription to the gqlWSConnectionHandler and sends the startMessage to the origin
func (h *gqlWSConnectionHandler) subscribe(sub Subscription) {
	graphQLBody, err := json.Marshal(sub.options.Body)
	if err != nil {
		return
	}

	h.nextSubscriptionID++

	subscriptionID := strconv.Itoa(h.nextSubscriptionID)

	startRequest := fmt.Sprintf(startMessage, subscriptionID, string(graphQLBody))
	err = wsutil.WriteClientText(h.conn, []byte(startRequest))
	if err != nil {
		return
	}

	h.subscriptions[subscriptionID] = sub
}

func (h *gqlWSConnectionHandler) handleMessageTypeData(data []byte) {
	id, err := jsonparser.GetString(data, "id")
	if err != nil {
		return
	}
	sub, ok := h.subscriptions[id]
	if !ok {
		return
	}
	payload, _, _, err := jsonparser.Get(data, "payload")
	if err != nil {
		return
	}

	sub.updater.Update(payload)
}

func (h *gqlWSConnectionHandler) handleMessageTypeConnectionError() {
	for _, sub := range h.subscriptions {
		sub.updater.Update([]byte(connectionError))
	}
}

func (h *gqlWSConnectionHandler) broadcastErrorMessage(err error) {
	errMsg := fmt.Sprintf(errorMessageTemplate, err)
	for _, sub := range h.subscriptions {
		sub.updater.Update([]byte(errMsg))
	}
}

func (h *gqlWSConnectionHandler) handleMessageTypeComplete(data []byte) {
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

func (h *gqlWSConnectionHandler) handleMessageTypeError(data []byte) {
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
		sub.updater.Update([]byte(internalError))
		return
	}
	switch valueType {
	case jsonparser.Array:
		response := []byte(`{}`)
		response, err = jsonparser.Set(response, value, "errors")
		if err != nil {
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

func (h *gqlWSConnectionHandler) unsubscribe(subscriptionID string) {
	sub, ok := h.subscriptions[subscriptionID]
	if !ok {
		return
	}
	sub.updater.Done()
	delete(h.subscriptions, subscriptionID)
	stopRequest := fmt.Sprintf(stopMessage, subscriptionID)
	_ = wsutil.WriteClientText(h.conn, []byte(stopRequest))
}

func (h *gqlWSConnectionHandler) checkActiveSubscriptions() (hasActiveSubscriptions bool) {
	for id, sub := range h.subscriptions {
		if sub.ctx.Err() != nil {
			h.unsubscribe(id)
		}
	}
	return len(h.subscriptions) != 0
}
