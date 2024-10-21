package graphql_datasource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/ws/wsutil"

	"github.com/coder/websocket"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash/v2"
	"github.com/jensneuse/abstractlogger"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/epoller"
)

const ackWaitTimeout = 30 * time.Second

// subscriptionClient allows running multiple subscriptions via the same WebSocket either SSE connection
// It takes care of de-duplicating connections to the same origin under certain circumstances
// If Hash(URL,Body,Headers) result in the same result, an existing connection is re-used
type subscriptionClient struct {
	streamingClient *http.Client
	httpClient      *http.Client

	useHttpClientWithSkipRoundTrip bool

	engineCtx                  context.Context
	log                        abstractlogger.Logger
	hashPool                   sync.Pool
	onWsConnectionInitCallback *OnWsConnectionInitCallback

	readTimeout time.Duration

	epoll       epoller.Poller
	epollConfig EpollConfiguration

	connections   map[int]ConnectionHandler
	connectionsMu sync.Mutex

	activeConnections   map[int]struct{}
	activeConnectionsMu sync.Mutex

	triggers map[uint64]int
}

func (c *subscriptionClient) SubscribeAsync(ctx *resolve.Context, id uint64, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error {
	if options.UseSSE {
		return c.subscribeSSE(ctx.Context(), c.engineCtx, options, updater)
	}

	if strings.HasPrefix(options.URL, "https") {
		options.URL = "wss" + options.URL[5:]
	} else if strings.HasPrefix(options.URL, "http") {
		options.URL = "ws" + options.URL[4:]
	}

	return c.asyncSubscribeWS(ctx.Context(), c.engineCtx, id, options, updater)
}

func (c *subscriptionClient) Unsubscribe(id uint64) {
	c.connectionsMu.Lock()
	defer c.connectionsMu.Unlock()
	fd, ok := c.triggers[id]
	if !ok {
		return
	}
	delete(c.triggers, id)
	handler, ok := c.connections[fd]
	if !ok {
		return
	}
	handler.ClientClose()
	delete(c.connections, fd)
	_ = c.epoll.Remove(handler.NetConn())
}

type InvalidWsSubprotocolError struct {
	InvalidProtocol string
}

func (e InvalidWsSubprotocolError) Error() string {
	return fmt.Sprintf("provided websocket subprotocol '%s' is not supported. The supported subprotocols are graphql-ws and graphql-transport-ws. Please configure your subsciptions with the mentioned subprotocols", e.InvalidProtocol)
}

func NewInvalidWsSubprotocolError(invalidProtocol string) InvalidWsSubprotocolError {
	return InvalidWsSubprotocolError{
		InvalidProtocol: invalidProtocol,
	}
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

type EpollConfiguration struct {
	Disable    bool
	BufferSize int
	Interval   time.Duration
}

func (e *EpollConfiguration) ApplyDefaults() {
	if e.BufferSize == 0 {
		e.BufferSize = 1024
	}
	if e.Interval == 0 {
		e.Interval = time.Millisecond * 100
	}
}

func WithEpollConfiguration(config EpollConfiguration) Options {
	return func(options *opts) {
		options.epollConfiguration = config
	}
}

type opts struct {
	readTimeout                time.Duration
	log                        abstractlogger.Logger
	onWsConnectionInitCallback *OnWsConnectionInitCallback
	epollConfiguration         EpollConfiguration
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

func IsDefaultGraphQLSubscriptionClient(client GraphQLSubscriptionClient) bool {
	_, ok := client.(*subscriptionClient)
	return ok
}

func NewGraphQLSubscriptionClient(httpClient, streamingClient *http.Client, engineCtx context.Context, options ...Options) GraphQLSubscriptionClient {
	op := &opts{
		readTimeout: time.Second,
		log:         abstractlogger.NoopLogger,
	}
	for _, option := range options {
		option(op)
	}
	op.epollConfiguration.ApplyDefaults()
	client := &subscriptionClient{
		httpClient:      httpClient,
		streamingClient: streamingClient,
		engineCtx:       engineCtx,
		log:             op.log,
		readTimeout:     op.readTimeout,
		hashPool: sync.Pool{
			New: func() interface{} {
				return xxhash.New()
			},
		},
		onWsConnectionInitCallback: op.onWsConnectionInitCallback,
		connections:                make(map[int]ConnectionHandler),
		activeConnections:          make(map[int]struct{}),
		triggers:                   make(map[uint64]int),
		epollConfig:                op.epollConfiguration,
	}
	if !op.epollConfiguration.Disable {
		// ignore error is ok, it means that epoll is not supported, which is handled gracefully by the client
		epoll, _ := epoller.NewPoller(op.epollConfiguration.BufferSize, op.epollConfiguration.Interval)
		if epoll != nil {
			client.epoll = epoll
			go client.runEpoll(engineCtx)
		}
	}
	return client
}

// Subscribe initiates a new GraphQL Subscription with the origin
// If an existing WS connection with the same ID (Hash) exists, it is being re-used
// If connection protocol is SSE, a new connection is always created
// If no connection exists, the client initiates a new one
func (c *subscriptionClient) Subscribe(ctx *resolve.Context, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error {
	if options.UseSSE {
		return c.subscribeSSE(ctx.Context(), c.engineCtx, options, updater)
	}

	return c.subscribeWS(ctx.Context(), c.engineCtx, options, updater)
}

var (
	withSSE           = []byte(`sse:true`)
	withSSEMethodPost = []byte(`sse_method_post:true`)
)

func (c *subscriptionClient) UniqueRequestID(ctx *resolve.Context, options GraphQLSubscriptionOptions, hash *xxhash.Digest) (err error) {
	if options.UseSSE {
		_, err = hash.Write(withSSE)
		if err != nil {
			return err
		}
	}
	if options.SSEMethodPost {
		_, err = hash.Write(withSSEMethodPost)
		if err != nil {
			return err
		}
	}
	return c.requestHash(ctx, options, hash)
}

func (c *subscriptionClient) subscribeSSE(requestContext, engineContext context.Context, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error {
	if c.streamingClient == nil {
		return fmt.Errorf("streaming http client is nil")
	}

	handler := newSSEConnectionHandler(requestContext, engineContext, c.streamingClient, updater, options, c.log)

	go handler.StartBlocking()

	return nil
}

func (c *subscriptionClient) subscribeWS(requestContext, engineContext context.Context, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error {
	if c.httpClient == nil {
		return fmt.Errorf("http client is nil")
	}

	handler, err := c.newWSConnectionHandler(requestContext, engineContext, options, updater)
	if err != nil {
		return err
	}

	go func() {
		err := handler.StartBlocking()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return
			}
			c.log.Error("subscriptionClient.subscribeWS", abstractlogger.Error(err))
		}
	}()

	return nil
}

func (c *subscriptionClient) asyncSubscribeWS(requestContext, engineContext context.Context, id uint64, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error {
	if c.httpClient == nil {
		return fmt.Errorf("http client is nil")
	}

	handler, err := c.newWSConnectionHandler(requestContext, engineContext, options, updater)
	if err != nil {
		return err
	}

	if c.epoll == nil {
		go func() {
			err := handler.StartBlocking()
			if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
				c.log.Error("subscriptionClient.asyncSubscribeWS", abstractlogger.Error(err))
			}
		}()
		return nil
	}

	err = handler.Subscribe()
	if err != nil {
		return err
	}

	netConn := handler.NetConn()
	if err := c.epoll.Add(netConn); err != nil {
		return err
	}

	c.connectionsMu.Lock()
	fd := epoller.SocketFD(netConn)
	c.connections[fd] = handler
	c.triggers[id] = fd
	c.connectionsMu.Unlock()

	return nil
}

// generateHandlerIDHash generates a Hash based on: URL and Headers to uniquely identify Upgrade Requests
func (c *subscriptionClient) requestHash(ctx *resolve.Context, options GraphQLSubscriptionOptions, xxh *xxhash.Digest) (err error) {
	if _, err = xxh.WriteString(options.URL); err != nil {
		return err
	}
	if err := options.Header.Write(xxh); err != nil {
		return err
	}
	// Make sure any header that will be forwarded to the subgraph
	// is hashed to create the handlerID, this way requests with
	// different headers will use separate connections.
	for _, headerName := range options.ForwardedClientHeaderNames {
		if _, err = xxh.WriteString(headerName); err != nil {
			return err
		}
		for _, val := range ctx.Request.Header[textproto.CanonicalMIMEHeaderKey(headerName)] {
			if _, err = xxh.WriteString(val); err != nil {
				return err
			}
		}
	}
	for _, headerRegexp := range options.ForwardedClientHeaderRegularExpressions {
		if _, err = xxh.WriteString(headerRegexp.String()); err != nil {
			return err
		}
		for headerName, values := range ctx.Request.Header {
			if headerRegexp.MatchString(headerName) {
				for _, val := range values {
					if _, err = xxh.WriteString(val); err != nil {
						return err
					}
				}
			}
		}
	}
	if len(ctx.InitialPayload) > 0 {
		if _, err = xxh.Write(ctx.InitialPayload); err != nil {
			return err
		}
	}
	if options.Body.Extensions != nil {
		if _, err = xxh.Write(options.Body.Extensions); err != nil {
			return err
		}
	}
	if options.Body.Query != "" {
		_, err = xxh.WriteString(options.Body.Query)
		if err != nil {
			return err
		}
	}
	if options.Body.Variables != nil {
		_, err = xxh.Write(options.Body.Variables)
		if err != nil {
			return err
		}
	}
	if options.Body.OperationName != "" {
		_, err = xxh.WriteString(options.Body.OperationName)
		if err != nil {
			return err
		}
	}
	return nil
}

type UpgradeRequestError struct {
	URL        string
	StatusCode int
}

func (u *UpgradeRequestError) Error() string {
	return fmt.Sprintf("failed to upgrade connection to %s, status code: %d", u.URL, u.StatusCode)
}

func (c *subscriptionClient) newWSConnectionHandler(requestContext, engineContext context.Context, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) (ConnectionHandler, error) {
	subProtocols := []string{ProtocolGraphQLWS, ProtocolGraphQLTWS}
	if options.WsSubProtocol != "" && options.WsSubProtocol != "auto" {
		subProtocols = []string{options.WsSubProtocol}
	}

	var netConn net.Conn

	clientTrace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			netConn = info.Conn
		},
	}
	clientTraceCtx := httptrace.WithClientTrace(requestContext, clientTrace)
	conn, upgradeResponse, err := websocket.Dial(clientTraceCtx, options.URL, &websocket.DialOptions{
		HTTPClient:      c.httpClient,
		HTTPHeader:      options.Header,
		CompressionMode: websocket.CompressionDisabled,
		Subprotocols:    subProtocols,
	})
	if err != nil {
		if upgradeResponse != nil && upgradeResponse.StatusCode != 101 {
			return nil, &UpgradeRequestError{
				URL:        options.URL,
				StatusCode: upgradeResponse.StatusCode,
			}
		}
		return nil, err
	}
	// Disable the maximum message size limit. Don't use MaxInt64 since
	// the github.com/coder/websocket doesn't handle it correctly on 32-bit systems.
	conn.SetReadLimit(math.MaxInt32)
	if upgradeResponse.StatusCode != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("upgrade unsuccessful")
	}

	connectionInitMessage, err := c.getConnectionInitMessage(requestContext, options.URL, options.Header)
	if err != nil {
		return nil, err
	}

	if len(options.InitialPayload) > 0 {
		connectionInitMessage, err = jsonparser.Set(connectionInitMessage, options.InitialPayload, "payload")
		if err != nil {
			return nil, err
		}
	}

	if options.Body.Extensions != nil {
		connectionInitMessage, err = jsonparser.Set(connectionInitMessage, options.Body.Extensions, "payload", "extensions")
		if err != nil {
			return nil, err
		}
	}

	// init + ack
	err = wsutil.WriteClientText(netConn, connectionInitMessage)
	if err != nil {
		return nil, err
	}

	wsSubProtocol := subProtocols[0]
	if options.WsSubProtocol == "" || options.WsSubProtocol == "auto" {
		wsSubProtocol = conn.Subprotocol()
		if wsSubProtocol == "" {
			wsSubProtocol = ProtocolGraphQLWS
		}
	}

	if err := waitForAck(requestContext, conn); err != nil {
		return nil, err
	}

	switch wsSubProtocol {
	case ProtocolGraphQLWS:
		return newGQLWSConnectionHandler(requestContext, engineContext, conn, netConn, options, updater, c.log), nil
	case ProtocolGraphQLTWS:
		return newGQLTWSConnectionHandler(requestContext, engineContext, conn, netConn, options, updater, c.log), nil
	default:
		return nil, NewInvalidWsSubprotocolError(wsSubProtocol)
	}
}

func (c *subscriptionClient) getConnectionInitMessage(ctx context.Context, url string, header http.Header) ([]byte, error) {
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
	StartBlocking() error
	NetConn() net.Conn
	ReadMessage() (done, timeout bool)
	ServerClose()
	ClientClose()
	Subscribe() error
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

func (c *subscriptionClient) runEpoll(ctx context.Context) {
	var (
		done = ctx.Done()
	)
	for {
		connections, err := c.epoll.Wait(50)
		if err != nil {
			c.log.Error("epoll.Wait", abstractlogger.Error(err))
			return
		}
		c.connectionsMu.Lock()
		for _, conn := range connections {
			id := epoller.SocketFD(conn)
			handler, ok := c.connections[id]
			if !ok {
				continue
			}
			c.activeConnectionsMu.Lock()
			_, active := c.activeConnections[id]
			if !active {
				c.activeConnections[id] = struct{}{}
			}
			c.activeConnectionsMu.Unlock()
			if active {
				continue
			}
			go c.handleConnection(id, handler, conn)
		}
		c.connectionsMu.Unlock()

		select {
		case <-done:
			return
		default:
		}
	}
}

func (c *subscriptionClient) handleConnection(id int, handler ConnectionHandler, conn net.Conn) {
	done, timeout := handler.ReadMessage()
	if timeout {
		c.activeConnectionsMu.Lock()
		delete(c.activeConnections, id)
		c.activeConnectionsMu.Unlock()
		return
	}

	if done {
		c.activeConnectionsMu.Lock()
		delete(c.activeConnections, id)
		c.activeConnectionsMu.Unlock()

		c.connectionsMu.Lock()
		delete(c.connections, id)
		c.connectionsMu.Unlock()

		handler.ServerClose()
		_ = c.epoll.Remove(conn)
		return
	}
}

func handleConnectionError(err error) (closed, timeout bool) {
	netOpErr := &net.OpError{}
	if errors.As(err, &netOpErr) {
		if netOpErr.Timeout() {
			return false, true
		}
		return true, false
	}

	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true, false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return false, true
	}
	if errors.As(err, &wsutil.ClosedError{}) {
		return true, false
	}
	if strings.HasSuffix(err.Error(), "use of closed network connection") {
		return true, false
	}
	return false, false
}
