package graphql_datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/buger/jsonparser"
	"github.com/jensneuse/abstractlogger"
	"nhooyr.io/websocket"
)

// gqlWSConnectionHandler is responsible for handling a connection to an origin
// it is responsible for managing all subscriptions using the underlying WebSocket connection
// if all Subscriptions are complete or cancelled/unsubscribed the handler will terminate
type gqlWSConnectionHandler struct {
	conn               *websocket.Conn
	ctx                context.Context
	log                abstractlogger.Logger
	subscribeCh        chan Subscription
	nextSubscriptionID int
	subscriptions      map[string]Subscription
	readTimeout        time.Duration
}

func newGQLWSConnectionHandler(ctx context.Context, conn *websocket.Conn, readTimeout time.Duration, log abstractlogger.Logger) *gqlWSConnectionHandler {
	return &gqlWSConnectionHandler{
		conn:               conn,
		ctx:                ctx,
		log:                log,
		subscribeCh:        make(chan Subscription),
		nextSubscriptionID: 0,
		subscriptions:      map[string]Subscription{},
		readTimeout:        readTimeout,
	}
}

func (h *gqlWSConnectionHandler) SubscribeCH() chan<- Subscription {
	return h.subscribeCh
}

// StartBlocking starts the single threaded event loop of the handler
// if the global context returns or the websocket connection is terminated, it will stop
func (h *gqlWSConnectionHandler) StartBlocking(sub Subscription) {
	readCtx, cancel := context.WithCancel(h.ctx)
	defer func() {
		h.unsubscribeAllAndCloseConn()
		cancel()
	}()
	h.subscribe(sub)
	dataCh := make(chan []byte)
	errCh := make(chan error)
	go h.readBlocking(readCtx, dataCh, errCh)
	for {
		err := h.ctx.Err()
		if err != nil {
			h.log.Error("gqlWSConnectionHandler.StartBlocking", abstractlogger.Error(err))
			h.broadcastErrorMessage(err)
			return
		}
		hasActiveSubscriptions := h.checkActiveSubscriptions()
		if !hasActiveSubscriptions {
			return
		}
		select {
		case <-time.After(h.readTimeout):
			continue
		case sub = <-h.subscribeCh:
			h.subscribe(sub)
		case err = <-errCh:
			h.log.Error("gqlWSConnectionHandler.StartBlocking", abstractlogger.Error(err))
			h.broadcastErrorMessage(err)
			return
		case data := <-dataCh:
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
			default:
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
		msgType, data, err := h.conn.Read(ctx)
		if ctx.Err() != nil {
			select {
			case errCh <- ctx.Err():
			case <-ctx.Done():
				return
			}
		}
		if err != nil {
			select {
			case errCh <- err:
			case <-ctx.Done():
				return
			}
		}
		if msgType != websocket.MessageText {
			continue
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
	_ = h.conn.Close(websocket.StatusNormalClosure, "")
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
	err = h.conn.Write(h.ctx, websocket.MessageText, []byte(startRequest))
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
	ctx, cancel := context.WithTimeout(h.ctx, time.Second*5)
	defer cancel()

	select {
	case <-ctx.Done():
	case sub.next <- payload:
	case <-sub.ctx.Done():
	}
}

func (h *gqlWSConnectionHandler) handleMessageTypeConnectionError() {
	for _, sub := range h.subscriptions {
		ctx, cancel := context.WithTimeout(h.ctx, time.Second*5)
		select {
		case sub.next <- []byte(connectionError):
			cancel()
			continue
		case <-ctx.Done():
			cancel()
			continue
		}
	}
}

func (h *gqlWSConnectionHandler) broadcastErrorMessage(err error) {
	errMsg := fmt.Sprintf(errorMessageTemplate, err)
	for _, sub := range h.subscriptions {
		ctx, cancel := context.WithTimeout(h.ctx, time.Second*5)
		select {
		case sub.next <- []byte(errMsg):
			cancel()
			continue
		case <-ctx.Done():
			cancel()
			continue
		}
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
	close(sub.next)
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
		sub.next <- []byte(internalError)
		return
	}
	switch valueType {
	case jsonparser.Array:
		response := []byte(`{}`)
		response, err = jsonparser.Set(response, value, "errors")
		if err != nil {
			sub.next <- []byte(internalError)
			return
		}
		sub.next <- response
	case jsonparser.Object:
		response := []byte(`{"errors":[]}`)
		response, err = jsonparser.Set(response, value, "errors", "[0]")
		if err != nil {
			sub.next <- []byte(internalError)
			return
		}
		sub.next <- response
	default:
		sub.next <- []byte(internalError)
	}
}

func (h *gqlWSConnectionHandler) unsubscribe(subscriptionID string) {
	sub, ok := h.subscriptions[subscriptionID]
	if !ok {
		return
	}
	close(sub.next)
	delete(h.subscriptions, subscriptionID)
	stopRequest := fmt.Sprintf(stopMessage, subscriptionID)
	_ = h.conn.Write(h.ctx, websocket.MessageText, []byte(stopRequest))
}

func (h *gqlWSConnectionHandler) checkActiveSubscriptions() (hasActiveSubscriptions bool) {
	for id, sub := range h.subscriptions {
		if sub.ctx.Err() != nil {
			h.unsubscribe(id)
		}
	}
	return len(h.subscriptions) != 0
}
