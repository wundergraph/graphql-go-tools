package graphql_datasource

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash/v2"
	"github.com/jensneuse/abstractlogger"
	"nhooyr.io/websocket"
)

const ackWaitTimeout = 30 * time.Second

// SubscriptionClient is a WebSocket client that allows running multiple subscriptions via the same WebSocket Connection
// It takes care of de-duplicating WebSocket connections to the same origin under certain circumstances
// If Hash(URL,Body,Headers) result in the same result, an existing WS connection is re-used
type SubscriptionClient struct {
	httpClient *http.Client
	ctx        context.Context
	log        abstractlogger.Logger
	hashPool   sync.Pool
	handlers   map[uint64]ConnectionHandler
	handlersMu sync.Mutex

	readTimeout time.Duration
}

type Options func(options *opts)

func WithLogger(log abstractlogger.Logger) Options {
	return func(options *opts) {
		options.log = log
	}
}

func WithReadTimeout(timeout time.Duration) Options {
	return func(options *opts) {
		options.readTimeout = timeout
	}
}

type opts struct {
	readTimeout time.Duration
	log         abstractlogger.Logger
}

func NewGraphQLSubscriptionClient(httpClient *http.Client, ctx context.Context, options ...Options) *SubscriptionClient {
	op := &opts{
		readTimeout: time.Second,
		log:         abstractlogger.NoopLogger,
	}
	for _, option := range options {
		option(op)
	}
	return &SubscriptionClient{
		httpClient:  httpClient,
		ctx:         ctx,
		handlers:    make(map[uint64]ConnectionHandler),
		log:         op.log,
		readTimeout: op.readTimeout,
		hashPool: sync.Pool{
			New: func() interface{} {
				return xxhash.New()
			},
		},
	}
}

// Subscribe initiates a new GraphQL Subscription with the origin
// Each WebSocket (WS) to an origin is uniquely identified by the Hash(URL,Headers,Body)
// If an existing WS with the same ID (Hash) exists, it is being re-used
// If no connection exists, the client initiates a new one and sends the "init" and "connection ack" messages
func (c *SubscriptionClient) Subscribe(ctx context.Context, options GraphQLSubscriptionOptions, next chan<- []byte) error {
	// TODO: starting point for the Subscription
	handlerID, err := c.generateHandlerIDHash(options)
	if err != nil {
		return err
	}

	sub := Subscription{
		ctx:     ctx,
		options: options,
		next:    next,
	}

	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()
	handler, exists := c.handlers[handlerID]
	if exists {
		select {
		case handler.SubscribeCH() <- sub:
		case <-ctx.Done():
		}
		return nil
	}

	handler, err = c.newWSConnectionHandler(options)
	if err != nil {
		return err
	}

	c.handlers[handlerID] = handler

	go func(handlerID uint64) {
		handler.StartBlocking(sub)
		c.handlersMu.Lock()
		delete(c.handlers, handlerID)
		c.handlersMu.Unlock()
	}(handlerID)

	return nil
}

func waitForAck(ctx context.Context, conn *websocket.Conn) error {
	timer := time.NewTimer(ackWaitTimeout)
	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout while waiting for connection_ack")
		default:
		}

		msgType, msg, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		if msgType != websocket.MessageText {
			return fmt.Errorf("unexpected message type")
		}

		respType, err := jsonparser.GetString(msg, "type")
		if err != nil {
			return err
		}

		switch respType {
		case MessageTypeConnectionKeepAlive:
			continue
		case MessageTypeConnectionAck:
			return nil
		default:
			return fmt.Errorf("expected connection_ack or ka, got %s", respType)
		}
	}
}

// generateHandlerIDHash generates a Hash based on: URL and Headers to uniquely identify Upgrade Requests
func (c *SubscriptionClient) generateHandlerIDHash(options GraphQLSubscriptionOptions) (uint64, error) {
	var (
		err error
	)
	xxh := c.hashPool.Get().(*xxhash.Digest)
	defer c.hashPool.Put(xxh)
	xxh.Reset()

	_, err = xxh.WriteString(options.URL)
	if err != nil {
		return 0, err
	}
	err = options.Header.Write(xxh)
	if err != nil {
		return 0, err
	}

	return xxh.Sum64(), nil
}

func (c *SubscriptionClient) newWSConnectionHandler(options GraphQLSubscriptionOptions) (ConnectionHandler, error) {
	conn, upgradeResponse, err := websocket.Dial(c.ctx, options.URL, &websocket.DialOptions{
		HTTPClient:      c.httpClient,
		HTTPHeader:      options.Header,
		CompressionMode: websocket.CompressionDisabled,
		Subprotocols:    []string{protocolGraphQLWS, protocolGraphQL},
	})
	if err != nil {
		return nil, err
	}
	if upgradeResponse.StatusCode != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("upgrade unsuccessful")
	}

	// wsProtocol := conn.Subprotocol()

	// init + ack
	err = conn.Write(c.ctx, websocket.MessageText, connectionInitMessage)
	if err != nil {
		return nil, err
	}

	if err := waitForAck(c.ctx, conn); err != nil {
		return nil, err
	}

	return newConnectionHandler(c.ctx, conn, c.readTimeout, c.log), nil
}

type ConnectionHandler interface {
	StartBlocking(sub Subscription)
	SubscribeCH() chan<- Subscription
}

type Subscription struct {
	ctx     context.Context
	options GraphQLSubscriptionOptions
	next    chan<- []byte
}
