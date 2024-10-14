package graphql_datasource

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/textproto"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash/v2"
	ws "github.com/gorilla/websocket"
	"github.com/jensneuse/abstractlogger"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/epoller"
)

const ackWaitTimeout = 30 * time.Second

// subscriptionClient allows running multiple subscriptions via the same WebSocket either SSE connection
// It takes care of de-duplicating connections to the same origin under certain circumstances
// If Hash(URL,Body,Headers) result in the same result, an existing connection is re-used
type subscriptionClient struct {
	streamingClient            *http.Client
	httpClient                 *http.Client
	engineCtx                  context.Context
	log                        abstractlogger.Logger
	hashPool                   sync.Pool
	onWsConnectionInitCallback *OnWsConnectionInitCallback

	readTimeout time.Duration

	epoll epoller.Poller

	connections   map[int]ConnectionHandler
	connectionsMu sync.Mutex

	activeConnections   map[int]int
	activeConnectionsMu sync.Mutex

	triggers map[uint64]int
}

func (c *subscriptionClient) SubscribeAsync(reqCtx *resolve.Context, id uint64, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error {
	if options.UseSSE {
		return c.subscribeSSE(reqCtx, options, updater)
	}

	if strings.HasPrefix(options.URL, "https") {
		options.URL = "wss" + options.URL[5:]
	} else if strings.HasPrefix(options.URL, "http") {
		options.URL = "ws" + options.URL[4:]
	}

	return c.asyncSubscribeWS(reqCtx, id, options, updater)
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

type opts struct {
	readTimeout                time.Duration
	log                        abstractlogger.Logger
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
	epoll, err := epoller.NewPoller(1024, time.Second)
	if err != nil {
		fmt.Printf("failed to create epoll: %v\n", err)
	}
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
		epoll:                      epoll,
		connections:                make(map[int]ConnectionHandler),
		triggers:                   make(map[uint64]int),
	}
	return client
}

// Subscribe initiates a new GraphQL Subscription with the origin
// If an existing WS connection with the same ID (Hash) exists, it is being re-used
// If connection protocol is SSE, a new connection is always created
// If no connection exists, the client initiates a new one
func (c *subscriptionClient) Subscribe(reqCtx *resolve.Context, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error {
	if options.UseSSE {
		return c.subscribeSSE(reqCtx, options, updater)
	}

	if strings.HasPrefix(options.URL, "https") {
		options.URL = "wss" + options.URL[5:]
	} else if strings.HasPrefix(options.URL, "http") {
		options.URL = "ws" + options.URL[4:]
	}

	return c.subscribeWS(reqCtx, options, updater)
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

func (c *subscriptionClient) subscribeSSE(reqCtx *resolve.Context, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error {
	if c.streamingClient == nil {
		return fmt.Errorf("streaming http client is nil")
	}

	sub := Subscription{
		ctx:     reqCtx.Context(),
		options: options,
		updater: updater,
	}

	handler := newSSEConnectionHandler(reqCtx, c.streamingClient, options, c.log)

	go handler.StartBlocking(sub)

	return nil
}

func (c *subscriptionClient) subscribeWS(reqCtx *resolve.Context, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error {
	if c.httpClient == nil {
		return fmt.Errorf("http client is nil")
	}

	sub := Subscription{
		ctx:     reqCtx.Context(),
		options: options,
		updater: updater,
	}

	handler, err := c.newWSConnectionHandler(reqCtx.Context(), options)
	if err != nil {
		return err
	}

	go handler.StartBlocking(sub)

	return nil
}

func (c *subscriptionClient) asyncSubscribeWS(reqCtx *resolve.Context, id uint64, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error {
	if c.httpClient == nil {
		return fmt.Errorf("http client is nil")
	}

	sub := Subscription{
		ctx:     reqCtx.Context(),
		options: options,
		updater: updater,
	}

	handler, err := c.newWSConnectionHandler(reqCtx.Context(), options)
	if err != nil {
		return err
	}

	netConn := handler.NetConn()
	if err := c.epoll.Add(netConn); err != nil {
		return err
	}

	c.connectionsMu.Lock()
	fd := socketFd(netConn)
	c.connections[fd] = handler
	c.triggers[id] = fd
	count := len(c.connections)
	c.connectionsMu.Unlock()

	if count == 1 {
		go c.runEpoll(c.engineCtx)
	}

	fmt.Printf("added connection to epoll\n")

	handler.Subscribe(sub)

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

func (c *subscriptionClient) newWSConnectionHandler(reqCtx context.Context, options GraphQLSubscriptionOptions) (ConnectionHandler, error) {
	subProtocols := []string{ProtocolGraphQLWS, ProtocolGraphQLTWS}
	if options.WsSubProtocol != "" && options.WsSubProtocol != "auto" {
		subProtocols = []string{options.WsSubProtocol}
	}

	dialer := ws.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: time.Second * 10,
		Subprotocols:     subProtocols,
	}

	conn, upgradeResponse, err := dialer.DialContext(reqCtx, options.URL, options.Header)
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(math.MaxInt32)
	if upgradeResponse.StatusCode != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("upgrade unsuccessful")
	}

	connectionInitMessage, err := c.getConnectionInitMessage(reqCtx, options.URL, options.Header)
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
	err = conn.WriteMessage(ws.TextMessage, connectionInitMessage)
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

	if err := waitForAck(conn); err != nil {
		return nil, err
	}

	switch wsSubProtocol {
	case ProtocolGraphQLWS:
		return newGQLWSConnectionHandler(c.engineCtx, conn, c.readTimeout, c.log), nil
	case ProtocolGraphQLTWS:
		return newGQLTWSConnectionHandler(c.engineCtx, conn, c.readTimeout, c.log), nil
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
	StartBlocking(sub Subscription)
	NetConn() net.Conn
	ReadMessage() (done bool)
	ServerClose()
	ClientClose()
	Subscribe(sub Subscription)
}

type Subscription struct {
	ctx     context.Context
	options GraphQLSubscriptionOptions
	updater resolve.SubscriptionUpdater
}

func waitForAck(conn *ws.Conn) error {
	timer := time.NewTimer(ackWaitTimeout)
	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout while waiting for connection_ack")
		default:
		}

		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if msgType != ws.TextMessage {
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
			err := conn.WriteMessage(ws.TextMessage, []byte(pongMessage))
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
	done := ctx.Done()
	tick := time.NewTicker(time.Millisecond * 10)
	for {
		connections, err := c.epoll.Wait(50)
		if err != nil {
			c.log.Error("epoll.Wait", abstractlogger.Error(err))
			return
		}
		fmt.Printf("ePoll - time: %v, connections: %d\n", time.Now(), len(connections))
		c.connectionsMu.Lock()
		for _, conn := range connections {
			id := socketFd(conn)
			handler, ok := c.connections[id]
			if !ok {
				continue
			}
			c.activeConnectionsMu.Lock()
			if i, ok := c.activeConnections[id]; ok {
				fmt.Printf("connection %d is active, queueing event\n", id)
				c.activeConnections[id] = i + 1
			}
			c.activeConnectionsMu.Unlock()
			go c.handleConnection(id, handler, conn)
		}
		c.connectionsMu.Unlock()
		select {
		case <-done:
			return
		case <-tick.C:
			continue
		}
	}
}

func (c *subscriptionClient) handleConnection(id int, handler ConnectionHandler, conn net.Conn) {
	fmt.Printf("handling connection %d\n", id)
	for {
		done := handler.ReadMessage()
		if done {
			fmt.Printf("connection %d done\n", id)
			c.connectionsMu.Lock()
			delete(c.connections, id)
			c.connectionsMu.Unlock()
			handler.ServerClose()
			_ = c.epoll.Remove(conn)
			return
		}
		c.activeConnectionsMu.Lock()
		if i, ok := c.activeConnections[id]; ok {
			if i == 0 {
				delete(c.activeConnections, id)
				c.activeConnectionsMu.Unlock()
				fmt.Printf("handleConnection: event queue empty, returning to ePoll for connection %d\n", id)
				return
			}
			c.activeConnections[id] = i - 1
			fmt.Printf("handleConnection: event queue not empty, processing next event for connection %d\n", id)
		}
		c.activeConnectionsMu.Unlock()
	}
}

func socketFd(conn net.Conn) int {
	if con, ok := conn.(syscall.Conn); ok {
		raw, err := con.SyscallConn()
		if err != nil {
			return 0
		}
		sfd := 0
		_ = raw.Control(func(fd uintptr) {
			sfd = int(fd)
		})
		return sfd
	}
	if con, ok := conn.(epoller.ConnImpl); ok {
		return con.GetFD()
	}
	return 0
}
