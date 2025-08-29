package graphql_datasource

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash/v2"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/jensneuse/abstractlogger"
	"go.uber.org/atomic"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/netpoll"
)

const (
	// The time to write a message to the server connection before timing out
	writeTimeout = 10 * time.Second
	// The time to wait for a connection ack message from the server before timing out
	ackWaitTimeout = 30 * time.Second
)

type netPollState struct {
	// connections is a map of fd -> connection to keep track of all active connections
	connections    map[int]*connection
	hasConnections atomic.Bool
	// triggers is a map of subscription id -> fd to easily look up the connection for a subscription id
	triggers map[uint64]int

	// clientUnsubscribe is a channel to signal to the netPoll run loop that a client needs to be unsubscribed
	clientUnsubscribe chan uint64
	// addConn is a channel to signal to the netPoll run loop that a new connection needs to be added
	addConn chan *connection
	// waitForEventsTicker is the ticker for the netPoll run loop
	// it is used to prevent busy waiting and to limit the CPU usage
	// instead of polling the netPoll instance all the time, we wait until the next tick to throttle the netPoll loop
	waitForEventsTicker *time.Ticker

	// waitForEventsTick is the channel to receive the tick from the waitForEventsTicker
	waitForEventsTick <-chan time.Time
}

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

	readTimeout  time.Duration
	pingInterval time.Duration
	frameTimeout time.Duration
	pingTimeout  time.Duration

	netPoll       netpoll.Poller
	netPollConfig NetPollConfiguration
	netPollState  *netPollState
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
	// if we don't have netPoll, we don't have a channel consumer of the clientUnsubscribe channel
	// we have to return to prevent a deadlock
	if c.netPoll == nil {
		return
	}
	c.netPollState.clientUnsubscribe <- id
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

func WithPingInterval(interval time.Duration) Options {
	return func(options *opts) {
		options.pingInterval = interval
	}
}

func WithFrameTimeout(timeout time.Duration) Options {
	return func(options *opts) {
		options.frameTimeout = timeout
	}
}

func WithPingTimeout(timeout time.Duration) Options {
	return func(options *opts) {
		options.pingTimeout = timeout
	}
}

type NetPollConfiguration struct {
	// Enable can be set to true to enable netPoll
	Enable bool
	// BufferSize defines the size of the buffer for the netPoll loop
	BufferSize int
	// WaitForNumEvents defines how many events are waited for in the netPoll loop before TickInterval cancels the wait
	WaitForNumEvents int
	// MaxEventWorkers defines the parallelism of how many connections can be handled at the same time
	// The higher the number, the more CPU is used.
	MaxEventWorkers int
	// TickInterval defines the time between each netPoll loop when WaitForNumEvents is not reached
	TickInterval time.Duration
}

func (e *NetPollConfiguration) ApplyDefaults() {
	e.Enable = true

	if e.BufferSize == 0 {
		e.BufferSize = 1024
	}
	if e.MaxEventWorkers == 0 {
		e.MaxEventWorkers = 6
	}
	if e.WaitForNumEvents == 0 {
		e.WaitForNumEvents = 1024
	}
	if e.TickInterval == 0 {
		e.TickInterval = time.Millisecond * 100
	}
}

func WithNetPollConfiguration(config NetPollConfiguration) Options {
	return func(options *opts) {
		options.netPollConfiguration = config
	}
}

type opts struct {
	readTimeout                time.Duration
	pingInterval               time.Duration
	pingTimeout                time.Duration
	frameTimeout               time.Duration
	log                        abstractlogger.Logger
	onWsConnectionInitCallback *OnWsConnectionInitCallback
	netPollConfiguration       NetPollConfiguration
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

	// Defaults
	op := &opts{
		readTimeout:  5 * time.Second,
		pingInterval: 15 * time.Second,
		pingTimeout:  30 * time.Second,
		frameTimeout: 100 * time.Millisecond,
		log:          abstractlogger.NoopLogger,
	}

	op.netPollConfiguration.ApplyDefaults()

	for _, option := range options {
		option(op)
	}

	client := &subscriptionClient{
		httpClient:      httpClient,
		streamingClient: streamingClient,
		engineCtx:       engineCtx,
		log:             op.log,
		readTimeout:     op.readTimeout,
		pingInterval:    op.pingInterval,
		pingTimeout:     op.pingTimeout,
		frameTimeout:    op.frameTimeout,
		hashPool: sync.Pool{
			New: func() interface{} {
				return xxhash.New()
			},
		},
		onWsConnectionInitCallback: op.onWsConnectionInitCallback,
		netPollConfig:              op.netPollConfiguration,
	}
	if op.netPollConfiguration.Enable {
		client.netPollState = &netPollState{
			connections:       make(map[int]*connection),
			triggers:          make(map[uint64]int),
			clientUnsubscribe: make(chan uint64, op.netPollConfiguration.BufferSize),
			addConn:           make(chan *connection, op.netPollConfiguration.BufferSize),
			// this is not needed, but we want to make it explicit that we're starting with nil as the tick channel
			// reading from nil channels blocks forever, which allows us to prevent the netPoll loop from starting
			// once we add the first connection, we start the ticker and set the tick channel
			// after the last connection is removed, we set the tick channel to nil again
			// this way we can start and stop the epoll loop dynamically
			waitForEventsTick: nil,
		}

		// ignore error is ok, it means that netPoll is not supported, which is handled gracefully by the client
		poller, _ := netpoll.NewPoller(op.netPollConfiguration.BufferSize, op.netPollConfiguration.TickInterval)
		if poller != nil {
			client.netPoll = poller
			go client.runNetPoll(engineCtx)
		}
	}
	return client
}

type connection struct {
	id          uint64
	fd          int
	netConn     net.Conn
	handler     ConnectionHandler
	shouldClose bool
}

// Subscribe initiates a new GraphQL Subscription with the origin
// If an existing WS connection with the same ID (Hash) exists, it is being re-used
// If connection protocol is SSE, a new connection is always created
// If no connection exists, the client initiates a new one
func (c *subscriptionClient) Subscribe(ctx *resolve.Context, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error {
	options.readTimeout = c.readTimeout
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
	options.readTimeout = c.readTimeout
	if c.streamingClient == nil {
		return fmt.Errorf("streaming http client is nil")
	}

	handler := newSSEConnectionHandler(requestContext, engineContext, c.streamingClient, updater, options, c.log)

	go handler.StartBlocking()

	return nil
}

func (c *subscriptionClient) subscribeWS(requestContext, engineContext context.Context, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error {
	options.readTimeout = c.readTimeout
	options.pingInterval = c.pingInterval
	options.pingTimeout = c.pingTimeout

	if c.httpClient == nil {
		return fmt.Errorf("http client is nil")
	}

	conn, err := c.newWSConnectionHandler(requestContext, engineContext, options, updater)
	if err != nil {
		return err
	}

	go func() {
		err := conn.handler.StartBlocking()
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
	options.readTimeout = c.readTimeout
	options.pingInterval = c.pingInterval
	options.pingTimeout = c.pingTimeout

	if c.httpClient == nil {
		return fmt.Errorf("http client is nil")
	}

	conn, err := c.newWSConnectionHandler(requestContext, engineContext, options, updater)
	if err != nil {
		return err
	}

	if c.netPoll == nil {
		go func() {
			err := conn.handler.StartBlocking()
			if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
				c.log.Error("subscriptionClient.asyncSubscribeWS", abstractlogger.Error(err))
			}
		}()
		return nil
	}

	// if we have netPoll, we need to add the connection to the netPoll

	// init the subscription
	err = conn.handler.Subscribe()
	if err != nil {
		return err
	}

	var fd int

	// we have to check if the connection is a tls connection to get the underlying net.Conn
	if tlsConn, ok := conn.netConn.(*tls.Conn); ok {
		netConn := tlsConn.NetConn()
		fd = netpoll.SocketFD(netConn)
	} else {
		fd = netpoll.SocketFD(conn.netConn)
	}

	if fd == 0 {
		c.log.Error("failed to get file descriptor from connection. This indicates a problem with the netPoll implementation")
		return fmt.Errorf("failed to get file descriptor from connection")
	}

	conn.id, conn.fd = id, fd
	// submit the connection to the netPoll run loop
	c.netPollState.addConn <- conn
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
		if _, err = xxh.WriteString(headerRegexp.Pattern.String()); err != nil {
			return err
		}

		// Sort header names for deterministic hashing
		headerKeys := make([]string, 0, len(ctx.Request.Header))
		for key := range ctx.Request.Header {
			headerKeys = append(headerKeys, key)
		}
		sort.Strings(headerKeys)

		for _, headerName := range headerKeys {
			values := ctx.Request.Header[headerName]
			result := headerRegexp.Pattern.MatchString(headerName)
			if headerRegexp.NegateMatch {
				result = !result
			}
			if result {
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

func (c *subscriptionClient) newWSConnectionHandler(requestContext, engineContext context.Context, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) (*connection, error) {
	// Any failure will be stored here, needed for deferred body closer.
	var err error

	conn, subProtocol, err := c.dial(requestContext, options)
	if err != nil {
		return nil, err
	}

	if conn == nil {
		return nil, fmt.Errorf("failed to dial connection")
	}

	// conn is not nil. Any errored return below could lead to a leaking connection.
	// To avoid this, close connection if failure happened.
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()

	initMsg, err := c.getConnectionInitMessage(requestContext, options.URL, options.Header)
	if err != nil {
		return nil, err
	}

	if len(options.InitialPayload) > 0 {
		initMsg, err = jsonparser.Set(initMsg, options.InitialPayload, "payload")
		if err != nil {
			return nil, err
		}
	}

	if options.Body.Extensions != nil {
		initMsg, err = jsonparser.Set(initMsg, options.Body.Extensions, "payload", "extensions")
		if err != nil {
			return nil, err
		}
	}

	// init + ack
	if err = conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return nil, err
	}
	err = wsutil.WriteClientText(conn, initMsg)
	if err != nil {
		return nil, err
	}

	if err = waitForAck(conn, c.readTimeout, writeTimeout); err != nil {
		return nil, err
	}

	switch subProtocol {
	case ProtocolGraphQLWS:
		return newGQLWSConnectionHandler(requestContext, engineContext, conn, options, updater, c.log), nil
	case ProtocolGraphQLTWS:
		return newGQLTWSConnectionHandler(requestContext, engineContext, conn, options, updater, c.log), nil
	default:
		return nil, NewInvalidWsSubprotocolError(subProtocol)
	}
}

func (c *subscriptionClient) dial(ctx context.Context, options GraphQLSubscriptionOptions) (conn net.Conn, subProtocol string, err error) {
	subProtocols := []string{ProtocolGraphQLTWS, ProtocolGraphQLWS}
	if options.WsSubProtocol != "" && options.WsSubProtocol != "auto" {
		subProtocols = []string{options.WsSubProtocol}
	}

	clientTrace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			conn = info.Conn
		},
	}
	clientTraceCtx := httptrace.WithClientTrace(ctx, clientTrace)
	u := options.URL
	if strings.HasPrefix(options.URL, "wss") {
		u = "https" + options.URL[3:]
	} else if strings.HasPrefix(options.URL, "ws") {
		u = "http" + options.URL[2:]
	}
	req, err := http.NewRequestWithContext(clientTraceCtx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	req.Proto = "HTTP/1.1"
	req.ProtoMajor = 1
	req.ProtoMinor = 1
	if options.Header != nil {
		req.Header = options.Header
	}
	req.Header.Set("Sec-WebSocket-Protocol", strings.Join(subProtocols, ","))
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")

	challengeKey, err := generateChallengeKey()
	if err != nil {
		return nil, "", err
	}

	req.Header.Set("Sec-WebSocket-Key", challengeKey)

	upgradeResponse, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}

	// On failed upgrades, we close the body without transferring ownership to the caller.

	if upgradeResponse.StatusCode != http.StatusSwitchingProtocols {
		// Drain to EOF to allow connection reuse by net/http.
		_, _ = io.Copy(io.Discard, upgradeResponse.Body)
		upgradeResponse.Body.Close()
		return nil, "", &UpgradeRequestError{
			URL:        u,
			StatusCode: upgradeResponse.StatusCode,
		}
	}

	accept := computeAcceptKey(challengeKey)
	if upgradeResponse.Header.Get("Sec-WebSocket-Accept") != accept {
		_, _ = io.Copy(io.Discard, upgradeResponse.Body)
		upgradeResponse.Body.Close()
		return nil, "", fmt.Errorf("invalid Sec-WebSocket-Accept")
	}

	subProtocol = subProtocols[0]
	if options.WsSubProtocol == "" || options.WsSubProtocol == "auto" {
		subProtocol = upgradeResponse.Header.Get("Sec-WebSocket-Protocol")
		if subProtocol == "" {
			subProtocol = ProtocolGraphQLWS
		}
	}

	return conn, subProtocol, nil
}

func generateChallengeKey() (string, error) {
	p := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, p); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(p), nil
}

var keyGUID = []byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11")

func computeAcceptKey(challengeKey string) string {
	h := sha1.New() // #nosec G401 -- (CWE-326) https://datatracker.ietf.org/doc/html/rfc6455#page-54
	h.Write([]byte(challengeKey))
	h.Write(keyGUID)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
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
	// StartBlocking starts the connection handler and blocks until the connection is closed
	// Only used as fallback when epoll is not available
	StartBlocking() error
	// HandleMessage handles the incoming message from the connection
	HandleMessage(data []byte) (done bool)
	// Ping sends a ping message to the upstream server to keep the connection alive.
	// Implementers must keep track of the last ping time to initiate a connection shutdown
	// if the upstream is not sending a pong.
	Ping()
	// ServerClose closes the connection from the server side
	ServerClose()
	// ClientClose closes the connection from the client side
	ClientClose()
	// Subscribe subscribes to the connection
	Subscribe() error
}

func waitForAck(conn net.Conn, readTimeout, writeTimeout time.Duration) error {
	timer := time.NewTimer(ackWaitTimeout)
	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout while waiting for connection_ack")
		default:
		}
		if err := conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
			return fmt.Errorf("failed to set read deadline: %w", err)
		}
		msg, err := wsutil.ReadServerText(conn)
		if err != nil {
			return err
		}
		respType, err := jsonparser.GetString(msg, "type")
		if err != nil {
			return err
		}

		switch respType {
		// TODO this method mixes message types from different protocols. We should
		//  move the specific protocol handling to the concrete implementation
		case messageTypeConnectionKeepAlive:
			continue
		case messageTypePing:
			if err := conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
				return fmt.Errorf("failed to set write deadline: %w", err)
			}
			err = wsutil.WriteClientText(conn, []byte(pongMessage))
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

type connResult struct {
	fd          int
	shouldClose bool
}

func (c *subscriptionClient) runNetPoll(ctx context.Context) {
	defer c.close()
	done := ctx.Done()
	// both handleConnCh and connResults are buffered channels with a size of WaitForNumEvents
	// this is important because we submit all events before we start processing them
	// and we start evaluating the results only after all events have been submitted
	// this would not be possible with unbuffered channels
	handleConnCh := make(chan *connection, c.netPollConfig.WaitForNumEvents)
	connResults := make(chan connResult, c.netPollConfig.WaitForNumEvents)
	pingCh := make(chan *connection, c.netPollConfig.WaitForNumEvents)

	// Start workers to handle connection events
	// MaxEventWorkers defines the parallelism of how many connections can be handled at the same time
	// This is the critical number on how much CPU is used
	for i := 0; i < c.netPollConfig.MaxEventWorkers; i++ {
		go func() {
			for {
				select {
				case conn := <-pingCh:
					conn.handler.Ping()
				case conn := <-handleConnCh:
					shouldClose := c.handleConnectionEvent(conn)
					connResults <- connResult{fd: conn.fd, shouldClose: shouldClose}
				case <-done:
					return
				}
			}
		}()
	}

	pingTicker := time.NewTicker(c.pingInterval)
	defer pingTicker.Stop()

	// This is the main netPoll run loop
	// It's a single threaded event loop that reacts to several events, such as added connections, clients unsubscribing, etc.
	for {
		select {
		// if the engine context is done, we close the netPoll loop
		case <-done:
			return
		case <-pingTicker.C:
			// Send a ping to all connections
			// We distribute the ping to all workers to prevent single threaded bottlenecks
			// However, this required state synchronization with the last ping time on the handler
			// because PING and PONG can be handled on different go routines
			for _, conn := range c.netPollState.connections {
				pingCh <- conn
			}
		case conn := <-c.netPollState.addConn:
			c.handleAddConn(conn)
		case id := <-c.netPollState.clientUnsubscribe:
			c.handleClientUnsubscribe(id)
			// while len(c.connections) == 0, this channel is nil, so we will never try to wait for netPoll events
			// this is important to prevent busy waiting
			// once we add the first connection, we start the ticker and set the tick channel
			// the ticker ensures that we don't poll the netPoll instance all the time,
			// but at most every TickInterval
		case <-c.netPollState.waitForEventsTick:
			events, err := c.netPoll.Wait(c.netPollConfig.WaitForNumEvents)
			if err != nil {
				c.log.Error("netPoll.Wait", abstractlogger.Error(err))
				continue
			}

			waitForEvents := len(events)

			for i := range events {
				fd := netpoll.SocketFD(events[i])
				conn, ok := c.netPollState.connections[fd]
				if !ok {
					// This can happen if the client was unsubscribed
					// and the ticker is still running because we haven't removed the last connection yet
					continue
				}
				// submit the connection to the worker pool
				handleConnCh <- conn
			}

			// we submit all events to the worker pool to handle all events in parallel
			// instead of just waiting until all handlers are done, we can handle newly added connections or clients unsubscribing simultaneously
			// we keep doing this until we have results for all events or the engine context is done
			// this allows us to keep handling events in parallel while being able to manage connections without locks
			// as a result, we can handle a large number of connections with a single threaded event loop

			for {
				if waitForEvents == 0 {
					// once we have results for all events, we can return to the top level loop and wait for the next tick
					break
				}
				select {
				case result := <-connResults:
					// if the connection indicates that it should be closed, we close and remove it
					if result.shouldClose {
						c.handleServerUnsubscribe(result.fd)
					}
					// we decrease the number of events we're waiting for to eventually break the loop
					waitForEvents--
				case conn := <-c.netPollState.addConn:
					c.handleAddConn(conn)
				case id := <-c.netPollState.clientUnsubscribe:
					c.handleClientUnsubscribe(id)
				case <-done:
					return
				}
			}
		}
	}
}

func (c *subscriptionClient) close() {
	defer c.log.Debug("subscriptionClient.close", abstractlogger.String("reason", "netPoll closed by context"))
	if c.netPollState.waitForEventsTicker != nil {
		c.netPollState.waitForEventsTicker.Stop()
	}
	for _, conn := range c.netPollState.connections {
		_ = c.netPoll.Remove(conn.netConn)
		conn.handler.ServerClose()
	}
	if c.netPoll != nil {
		err := c.netPoll.Close(false)
		if err != nil {
			c.log.Error("subscriptionClient.close", abstractlogger.Error(err))
		}
	}
}

func (c *subscriptionClient) handleAddConn(conn *connection) {
	var netConn net.Conn

	if tlsConn, ok := conn.netConn.(*tls.Conn); ok {
		netConn = tlsConn.NetConn()
	} else {
		netConn = conn.netConn
	}

	if err := c.netPoll.Add(netConn); err != nil {
		c.log.Error("subscriptionClient.handleAddConn", abstractlogger.Error(err))
		conn.handler.ServerClose()
		return
	}

	c.netPollState.connections[conn.fd] = conn
	c.netPollState.triggers[conn.id] = conn.fd
	// when we previously had 0 connections, we will have 1 connection now
	// this means we need to start the ticker so that we get netPoll events
	if len(c.netPollState.connections) == 1 {
		c.netPollState.waitForEventsTicker = time.NewTicker(c.netPollConfig.TickInterval)
		c.netPollState.waitForEventsTick = c.netPollState.waitForEventsTicker.C
		c.netPollState.hasConnections.Store(true)
	}
}

func (c *subscriptionClient) handleClientUnsubscribe(id uint64) {
	fd, ok := c.netPollState.triggers[id]
	if !ok {
		return
	}
	delete(c.netPollState.triggers, id)
	conn, ok := c.netPollState.connections[fd]
	if !ok {
		return
	}
	delete(c.netPollState.connections, fd)
	_ = c.netPoll.Remove(conn.netConn)
	conn.handler.ClientClose()
	// if we have no connections left, we stop the ticker
	if len(c.netPollState.connections) == 0 {
		c.netPollState.waitForEventsTicker.Stop()
		c.netPollState.waitForEventsTick = nil
		c.netPollState.hasConnections.Store(false)
	}
}

func (c *subscriptionClient) handleServerUnsubscribe(fd int) {
	conn, ok := c.netPollState.connections[fd]
	if !ok {
		return
	}
	delete(c.netPollState.connections, fd)
	delete(c.netPollState.triggers, conn.id)
	_ = c.netPoll.Remove(conn.netConn)
	conn.handler.ServerClose()
	// if we have no connections left, we stop the ticker
	if len(c.netPollState.connections) == 0 {
		c.netPollState.waitForEventsTicker.Stop()
		c.netPollState.waitForEventsTick = nil
		c.netPollState.hasConnections.Store(false)
	}
}

func (c *subscriptionClient) handleConnectionEvent(conn *connection) bool {
	data, err := readMessage(conn.netConn, c.frameTimeout, c.readTimeout)
	if err != nil {
		return handleConnectionError(err)
	}
	return conn.handler.HandleMessage(data)
}

func handleConnectionError(err error) (done bool) {
	netOpErr := &net.OpError{}
	if errors.As(err, &netOpErr) {
		return !netOpErr.Timeout()
	}

	// Check if we have errors during reading from the connection
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	// Check if we have a context error
	if errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Check if the error is a connection reset by peer
	if errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	if errors.Is(err, syscall.EPIPE) {
		return true
	}

	// Check if the error is a closed network connection. Introduced in go 1.16.
	// This replaces the string match of "use of closed network connection"
	if errors.Is(err, net.ErrClosed) {
		return true
	}

	// Check if the error is closed websocket connection
	if errors.As(err, &wsutil.ClosedError{}) {
		return true
	}

	return false
}

// readMessage reads a message from the connection
func readMessage(conn net.Conn, frameTimeout time.Duration, readTimeout time.Duration) ([]byte, error) {
	controlHandler := wsutil.ControlFrameHandler(conn, ws.StateClientSide)
	rd := &wsutil.Reader{
		Source:          conn,
		State:           ws.StateClientSide,
		CheckUTF8:       true,
		SkipHeaderCheck: false,
		OnIntermediate:  controlHandler,
	}

	for {
		// This method is used to check if we have data on the connection. The timeout needs to be much smaller
		// than the readTimeout to ensure we don't block the connection for too long. If we have no data, we move
		// on to the next connection.
		err := conn.SetReadDeadline(time.Now().Add(frameTimeout))
		if err != nil {
			return nil, err
		}

		// If we have data, we can read it. Otherwise, it will timeout and we wait for the next epoll tick
		hdr, err := rd.NextFrame()
		if err != nil {
			// A timeout will not close the connection but return an error
			return nil, err
		}
		if hdr.OpCode.IsControl() {
			// The controlHandler writes the control frames.
			// We need to work with a proper timeout to ensure we don't block forever.
			err := conn.SetWriteDeadline(time.Now().Add(frameTimeout))
			if err != nil {
				return nil, err
			}
			// Handles PING/PONG and CLOSE frames, but only on the ws protocol level
			// We still need to handle the PING/PONG frames on the application protocol level
			if err := controlHandler(hdr, rd); err != nil {
				return nil, err
			}
			continue
		}

		// We are only interested in text frames
		if hdr.OpCode&ws.OpText == 0 {
			// If we see anything else than a text frame, we need to discard the frame
			if err := rd.Discard(); err != nil {
				return nil, err
			}
			continue
		}

		// We limit the amount of time we wait for a message to be read from the connection
		// This is important to ensure we don't block the connection for too long
		err = conn.SetReadDeadline(time.Now().Add(readTimeout))
		if err != nil {
			return nil, err
		}
		return io.ReadAll(rd)
	}
}
