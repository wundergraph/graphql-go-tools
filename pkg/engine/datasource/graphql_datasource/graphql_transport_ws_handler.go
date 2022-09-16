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

// gqlTWSConnectionHandler is responsible for handling a connection to an origin
// it is responsible for managing all subscriptions using the underlying WebSocket connection
// if all Subscriptions are complete or cancelled/unsubscribed the handler will terminate
type gqlTWSConnectionHandler struct {
	conn               *websocket.Conn
	ctx                context.Context
	log                abstractlogger.Logger
	subscribeCh        chan Subscription
	nextSubscriptionID int
	subscriptions      map[string]Subscription
	readTimeout        time.Duration
}

func newGQLTWSConnectionHandler(ctx context.Context, conn *websocket.Conn, rt time.Duration, l abstractlogger.Logger) *gqlTWSConnectionHandler {
	return &gqlTWSConnectionHandler{
		conn:               conn,
		ctx:                ctx,
		log:                l,
		subscribeCh:        make(chan Subscription),
		nextSubscriptionID: 0,
		subscriptions:      map[string]Subscription{},
		readTimeout:        rt,
	}
}

func (h *gqlTWSConnectionHandler) SubscribeCH() chan<- Subscription {
	return h.subscribeCh
}

func (h *gqlTWSConnectionHandler) StartBlocking(sub Subscription) {
	readCtx, cancel := context.WithCancel(h.ctx)
	defer func() {
		h.unsubscribeAllAndCloseConn()
		cancel()
	}()

	h.subscribe(sub)
	dataCh := make(chan []byte)
	go h.readBlocking(readCtx, dataCh)

	for {
		if h.ctx.Err() != nil {
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
		case next := <-dataCh:
			messageType, err := jsonparser.GetString(next, "type")
			if err != nil {
				continue
			}

			switch messageType {
			case messageTypeNext:

			case messageTypeComplete:

			case messageTypeError:

			default:
				continue
			}

		default:
			continue
		}
	}
}

func (h *gqlTWSConnectionHandler) unsubscribeAllAndCloseConn() {
	for id := range h.subscriptions {
		h.unsubscribe(id)
	}
	_ = h.conn.Close(websocket.StatusNormalClosure, "")
}

func (h *gqlTWSConnectionHandler) unsubscribe(subscriptionID string) {
	sub, ok := h.subscriptions[subscriptionID]
	if !ok {
		return
	}
	close(sub.next)
	delete(h.subscriptions, subscriptionID)

	req := fmt.Sprintf(completeMessage, subscriptionID)
	_ = h.conn.Write(h.ctx, websocket.MessageText, []byte(req))
}

// subscribe adds a new Subscription to the gqlTWSConnectionHandler and sends the subscribeMessage to the origin
func (h *gqlTWSConnectionHandler) subscribe(sub Subscription) {
	graphQLBody, err := json.Marshal(sub.options.Body)
	if err != nil {
		return
	}

	h.nextSubscriptionID++

	subscriptionID := strconv.Itoa(h.nextSubscriptionID)

	subscribeRequest := fmt.Sprintf(subscribeMessage, subscriptionID, string(graphQLBody))
	err = h.conn.Write(h.ctx, websocket.MessageText, []byte(subscribeRequest))
	if err != nil {
		return
	}

	h.subscriptions[subscriptionID] = sub
}

// readBlocking is a dedicated loop running in a separate goroutine
// because the library "nhooyr.io/websocket" doesn't allow reading with a context with Timeout
// we'll block forever on reading until the context of the gqlTWSConnectionHandler stops
// TODO: think about extracting the readBlocking loop into a separate helper
func (h *gqlTWSConnectionHandler) readBlocking(ctx context.Context, dataCh chan []byte) {
	for {
		msgType, data, err := h.conn.Read(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			continue
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

func (h *gqlTWSConnectionHandler) checkActiveSubscriptions() (hasActiveSubscriptions bool) {
	for id, sub := range h.subscriptions {
		if sub.ctx.Err() != nil {
			h.unsubscribe(id)
		}
	}
	return len(h.subscriptions) != 0
}
