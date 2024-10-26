package graphql_datasource

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash/v2"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
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

	connections   map[int]*connection
	connectionsMu sync.RWMutex

	triggers          map[uint64]int
	clientUnsubscribe chan uint64
	serverUnsubscribe chan int
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
	c.clientUnsubscribe <- id
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
	// Disable can be set to true to disable epoll
	Disable bool
	// BufferSize defines the size of the buffer for the epoll loop
	BufferSize int
	// WaitForNumEvents defines how many events are waited for in the epoll loop before TickInterval cancels the wait
	WaitForNumEvents int
	// MaxEventWorkers defines the parallelism of how many connections can be handled at the same time
	// The higher the number, the more CPU is used.
	MaxEventWorkers int
	// TickInterval defines the time between each epoll loop when WaitForNumEvents is not reached
	TickInterval time.Duration
}

func (e *EpollConfiguration) ApplyDefaults() {
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
		readTimeout: time.Millisecond * 100,
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
		connections:                make(map[int]*connection),
		triggers:                   make(map[uint64]int),
		clientUnsubscribe:          make(chan uint64, op.epollConfiguration.BufferSize),
		serverUnsubscribe:          make(chan int, op.epollConfiguration.BufferSize),
		epollConfig:                op.epollConfiguration,
	}
	if !op.epollConfiguration.Disable {
		// ignore error is ok, it means that epoll is not supported, which is handled gracefully by the client
		epoll, _ := epoller.NewPoller(op.epollConfiguration.BufferSize, op.epollConfiguration.TickInterval)
		if epoll != nil {
			client.epoll = epoll
			go client.runEpoll(engineCtx)
		}
	}
	return client
}

type connection struct {
	id      uint64
	fd      int
	conn    net.Conn
	handler ConnectionHandler
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
	if c.httpClient == nil {
		return fmt.Errorf("http client is nil")
	}

	conn, err := c.newWSConnectionHandler(requestContext, engineContext, options, updater)
	if err != nil {
		return err
	}

	if c.epoll == nil {
		go func() {
			err := conn.handler.StartBlocking()
			if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
				c.log.Error("subscriptionClient.asyncSubscribeWS", abstractlogger.Error(err))
			}
		}()
		return nil
	}

	err = conn.handler.Subscribe()
	if err != nil {
		return err
	}

	if err := c.epoll.Add(conn.conn); err != nil {
		return err
	}

	c.connectionsMu.Lock()
	fd := epoller.SocketFD(conn.conn)
	conn.id = id
	conn.fd = fd
	c.connections[fd] = conn
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

func (c *subscriptionClient) newWSConnectionHandler(requestContext, engineContext context.Context, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) (*connection, error) {

	conn, subProtocol, err := c.dial(requestContext, options)
	if err != nil {
		return nil, err
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
	err = wsutil.WriteClientText(conn, connectionInitMessage)
	if err != nil {
		return nil, err
	}

	if err := waitForAck(conn); err != nil {
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
	subProtocols := []string{ProtocolGraphQLWS, ProtocolGraphQLTWS}
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
	if upgradeResponse.StatusCode != http.StatusSwitchingProtocols {
		return nil, "", &UpgradeRequestError{
			URL:        u,
			StatusCode: upgradeResponse.StatusCode,
		}
	}

	accept := computeAcceptKey(challengeKey)
	if upgradeResponse.Header.Get("Sec-WebSocket-Accept") != accept {
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
	h := sha1.New() //#nosec G401 -- (CWE-326) https://datatracker.ietf.org/doc/html/rfc6455#page-54
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
	StartBlocking() error
	HandleMessage(data []byte) (done bool)
	ServerClose()
	ClientClose()
	Subscribe() error
}

func waitForAck(conn net.Conn) error {
	timer := time.NewTimer(ackWaitTimeout)
	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout while waiting for connection_ack")
		default:
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
		case messageTypeConnectionKeepAlive:
			continue
		case messageTypePing:
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

func (c *subscriptionClient) runEpoll(ctx context.Context) {
	defer c.close()

	done := ctx.Done()

	handleConnCh := make(chan *connection)

	// Start workers to handle incoming connections
	// MaxEventWorkers defines the parallelism of how many connections can be handled at the same time
	// This is the critical number on how much CPU is used
	for i := 0; i < c.epollConfig.MaxEventWorkers; i++ {
		go func() {
			for {
				select {
				case id, ok := <-c.clientUnsubscribe:
					if !ok {
						return
					}
					c.handlePendingClientUnsubscribe(id)
				case id, ok := <-c.serverUnsubscribe:
					if !ok {
						return
					}
					c.handlePendingServerUnsubscribe(id)
				case conn, ok := <-handleConnCh:
					if !ok {
						return
					}
					c.handleConnection(conn)
				case <-done:
					return
				}
			}
		}()
	}

	delay := time.Millisecond * 100

	for {
		select {
		case <-done:
			return
		default:
			now := time.Now()
			connections, err := c.epoll.Wait(c.epollConfig.WaitForNumEvents)

			if err != nil {
				c.log.Error("epoll.Wait", abstractlogger.Error(err))
				continue
			}

			for i := range connections {

				id := epoller.SocketFD(connections[i])

				c.connectionsMu.RLock()
				conn, ok := c.connections[id]
				c.connectionsMu.RUnlock()

				if !ok {
					// Should never happen
					continue
				}

				handleConnCh <- conn
			}

			// sleep for the remaining time of the delay
			// to not spinlock the CPU

			sleepTime := delay - time.Since(now)

			if sleepTime > 0 {
				time.Sleep(sleepTime)
			}
		}
	}
}

func (c *subscriptionClient) close() {
	defer c.log.Debug("subscriptionClient.close", abstractlogger.String("reason", "epoll closed by context"))
	c.connectionsMu.Lock()
	defer c.connectionsMu.Unlock()
	for _, conn := range c.connections {
		_ = c.epoll.Remove(conn.conn)
		conn.handler.ServerClose()
	}
	if c.epoll != nil {
		err := c.epoll.Close(false)
		if err != nil {
			c.log.Error("subscriptionClient.close", abstractlogger.Error(err))
		}
	}
}

func (c *subscriptionClient) handlePendingClientUnsubscribe(id uint64) {
	c.connectionsMu.Lock()
	defer c.connectionsMu.Unlock()

	fd, ok := c.triggers[id]
	if !ok {
		return
	}
	delete(c.triggers, id)
	conn, ok := c.connections[fd]
	if !ok {
		return
	}
	delete(c.connections, fd)

	_ = c.epoll.Remove(conn.conn)
	conn.handler.ClientClose()
}

func (c *subscriptionClient) handlePendingServerUnsubscribe(id int) {
	c.connectionsMu.Lock()
	defer c.connectionsMu.Unlock()

	conn, ok := c.connections[id]
	if !ok {
		return
	}
	delete(c.connections, id)
	delete(c.triggers, conn.id)

	_ = c.epoll.Remove(conn.conn)
	conn.handler.ServerClose()
}

func (c *subscriptionClient) handleConnection(conn *connection) {
	data, err := readMessage(conn.conn, c.readTimeout)
	if err != nil {
		if handleConnectionError(err) {
			c.serverUnsubscribe <- conn.fd
			return
		}
		return
	}
	if conn.handler.HandleMessage(data) {
		c.serverUnsubscribe <- conn.fd
		return
	}
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

func readMessage(conn net.Conn, timeout time.Duration) ([]byte, error) {
	controlHandler := wsutil.ControlFrameHandler(conn, ws.StateClientSide)
	rd := &wsutil.Reader{
		Source:          conn,
		State:           ws.StateClientSide,
		CheckUTF8:       true,
		SkipHeaderCheck: false,
		OnIntermediate:  controlHandler,
	}
	for {
		err := conn.SetReadDeadline(time.Now().Add(timeout))
		if err != nil {
			return nil, err
		}
		hdr, err := rd.NextFrame()
		if err != nil {
			return nil, err
		}
		if hdr.OpCode.IsControl() {
			if err := controlHandler(hdr, rd); err != nil {
				return nil, err
			}
			continue
		}
		if hdr.OpCode&ws.OpText == 0 {
			if err := rd.Discard(); err != nil {
				return nil, err
			}
			continue
		}
		err = conn.SetReadDeadline(time.Now().Add(time.Second))
		if err != nil {
			return nil, err
		}
		return io.ReadAll(rd)
	}
}
