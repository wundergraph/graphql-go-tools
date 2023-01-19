package graphql_datasource

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash/v2"
	"github.com/jensneuse/abstractlogger"
	"nhooyr.io/websocket"
)

const ackWaitTimeout = 30 * time.Second

// SubscriptionClient allows running multiple subscriptions via the same WebSocket either SSE connection
// It takes care of de-duplicating connections to the same origin under certain circumstances
// If Hash(URL,Body,Headers) result in the same result, an existing connection is re-used
type SubscriptionClient struct {
	streamingClient            *http.Client
	httpClient                 *http.Client
	engineCtx                  context.Context
	log                        abstractlogger.Logger
	hashPool                   sync.Pool
	handlers                   map[uint64]ConnectionHandler
	handlersMu                 sync.Mutex
	wsSubProtocol              string
	onWsConnectionInitCallback *OnWsConnectionInitCallback

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

func WithWSSubProtocol(protocol string) Options {
	return func(options *opts) {
		options.wsSubProtocol = protocol
	}
}

func WithOnWsConnectionInitCallback(callback *OnWsConnectionInitCallback) Options {
	return func(options *opts) {
		options.onWsConnectionInitCallback = callback
	}
}

type opts struct {
	readTimeout                time.Duration
	log                        abstractlogger.Logger
	wsSubProtocol              string
	onWsConnectionInitCallback *OnWsConnectionInitCallback
}

// GraphQLSubscriptionClientFactory abstracts the way of creating a new GraphQLSubscriptionClient.
// This can be very handy for testing purposes.
type GraphQLSubscriptionClientFactory interface {
	NewSubscriptionClient(httpClient, streamingClient *http.Client, engineCtx context.Context, options ...Options) GraphQLSubscriptionClient
}

type DefaultSubscriptionClientFactory struct{}

func (d *DefaultSubscriptionClientFactory) NewSubscriptionClient(httpClient, streamingClient *http.Client, engineCtx context.Context, options ...Options) GraphQLSubscriptionClient {
	return NewGraphQLSubscriptionClient(httpClient, streamingClient, engineCtx, options...)
}

func NewGraphQLSubscriptionClient(httpClient, streamingClient *http.Client, engineCtx context.Context, options ...Options) *SubscriptionClient {
	op := &opts{
		readTimeout: time.Second,
		log:         abstractlogger.NoopLogger,
	}
	for _, option := range options {
		option(op)
	}
	return &SubscriptionClient{
		httpClient:      httpClient,
		streamingClient: streamingClient,
		engineCtx:       engineCtx,
		handlers:        make(map[uint64]ConnectionHandler),
		log:             op.log,
		readTimeout:     op.readTimeout,
		hashPool: sync.Pool{
			New: func() interface{} {
				return xxhash.New()
			},
		},
		wsSubProtocol:              op.wsSubProtocol,
		onWsConnectionInitCallback: op.onWsConnectionInitCallback,
	}
}

// Subscribe initiates a new GraphQL Subscription with the origin
// If an existing WS connection with the same ID (Hash) exists, it is being re-used
// If connection protocol is SSE, a new connection is always created
// If no connection exists, the client initiates a new one
func (c *SubscriptionClient) Subscribe(reqCtx context.Context, options GraphQLSubscriptionOptions, next chan<- []byte) error {
	if options.UseSSE {
		return c.subscribeSSE(reqCtx, options, next)
	}

	return c.subscribeWS(reqCtx, options, next)
}

func (c *SubscriptionClient) subscribeSSE(reqCtx context.Context, options GraphQLSubscriptionOptions, next chan<- []byte) error {
	if c.streamingClient == nil {
		return fmt.Errorf("streaming http client is nil")
	}

	sub := Subscription{
		ctx:     reqCtx,
		options: options,
		next:    next,
	}

	handler := newSSEConnectionHandler(reqCtx, c.streamingClient, options, c.log)

	go func() {
		handler.StartBlocking(sub)
	}()

	return nil
}

func (c *SubscriptionClient) subscribeWS(reqCtx context.Context, options GraphQLSubscriptionOptions, next chan<- []byte) error {
	if c.httpClient == nil {
		return fmt.Errorf("http client is nil")
	}

	sub := Subscription{
		ctx:     reqCtx,
		options: options,
		next:    next,
	}

	// each WS connection to an origin is uniquely identified by the Hash(URL,Headers,Body)
	handlerID, err := c.generateHandlerIDHash(options)
	if err != nil {
		return err
	}

	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()
	handler, exists := c.handlers[handlerID]
	if exists {
		select {
		case handler.SubscribeCH() <- sub:
		case <-reqCtx.Done():
		}
		return nil
	}

	handler, err = c.newWSConnectionHandler(reqCtx, options)
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

func (c *SubscriptionClient) newWSConnectionHandler(reqCtx context.Context, options GraphQLSubscriptionOptions) (ConnectionHandler, error) {
	subProtocols := []string{ProtocolGraphQLWS, ProtocolGraphQLTWS}
	if c.wsSubProtocol != "" {
		subProtocols = []string{c.wsSubProtocol}
	}

	conn, upgradeResponse, err := websocket.Dial(reqCtx, options.URL, &websocket.DialOptions{
		HTTPClient:      c.httpClient,
		HTTPHeader:      options.Header,
		CompressionMode: websocket.CompressionDisabled,
		Subprotocols:    subProtocols,
	})
	if err != nil {
		return nil, err
	}
	// Disable the maximum message size limit. Don't use MaxInt64 since
	// the nhooyr.io/websocket doesn't handle it correctly on 32 bit systems.
	conn.SetReadLimit(math.MaxInt32)
	if upgradeResponse.StatusCode != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("upgrade unsuccessful")
	}

	connectionInitMessage, err := c.getConnectionInitMessage(reqCtx, options.URL, options.Header)
	if err != nil {
		return nil, err
	}

	// init + ack
	err = conn.Write(reqCtx, websocket.MessageText, connectionInitMessage)
	if err != nil {
		return nil, err
	}

	if c.wsSubProtocol == "" {
		c.wsSubProtocol = conn.Subprotocol()
	}

	if err := waitForAck(reqCtx, conn); err != nil {
		return nil, err
	}

	switch c.wsSubProtocol {
	case ProtocolGraphQLWS:
		return newGQLWSConnectionHandler(c.engineCtx, conn, c.readTimeout, c.log), nil
	case ProtocolGraphQLTWS:
		return newGQLTWSConnectionHandler(c.engineCtx, conn, c.readTimeout, c.log), nil
	default:
		return nil, fmt.Errorf("unknown protocol %s", conn.Subprotocol())
	}
}

func (c *SubscriptionClient) getConnectionInitMessage(ctx context.Context, url string, header http.Header) ([]byte, error) {
	if c.onWsConnectionInitCallback == nil {
		return connectionInitMessage, nil
	}

	callback := *c.onWsConnectionInitCallback

	payload, err := callback(ctx, url, header)
	if err != nil {
		return nil, err
	}

	if len(payload) == 0 {
		return connectionInitMessage, nil
	}

	msg, err := jsonparser.Set(connectionInitMessage, payload, "payload")
	if err != nil {
		return nil, err
	}

	return msg, nil
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
		case messageTypeConnectionKeepAlive:
			continue
		case messageTypePing:
			err := conn.Write(ctx, websocket.MessageText, []byte(pongMessage))
			if err != nil {
				return fmt.Errorf("failed to send pong message: %w", err)
			}

			continue
		case messageTypeConnectionAck:
			return nil
		default:
			return fmt.Errorf("expected connection_ack or ka, got %s", respType)
		}
	}
}
