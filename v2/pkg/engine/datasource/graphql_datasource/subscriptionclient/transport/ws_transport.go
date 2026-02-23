package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/jensneuse/abstractlogger"
	"github.com/rs/xid"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/protocol"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

type ErrFailedUpgrade struct {
	URL        string
	StatusCode int
}

func (e ErrFailedUpgrade) Error() string {
	return fmt.Sprintf("failed to upgrade connection to %s, status code: %d", e.URL, e.StatusCode)
}

type ErrInvalidSubprotocol string

func (e ErrInvalidSubprotocol) Error() string {
	return fmt.Sprintf("provided websocket subprotocol '%s' is not supported. The supported subprotocols are graphql-ws and graphql-transport-ws. Please configure your subscriptions with the mentioned subprotocols", string(e))
}

type wsTransportOptions struct {
	upgradeClient *http.Client
	logger        abstractlogger.Logger
	pingInterval  time.Duration
	pingTimeout   time.Duration
	ackTimeout    time.Duration
	writeTimeout  time.Duration
	readLimit     int64
}

// WSTransportOption configures a WSTransport.
type WSTransportOption func(*wsTransportOptions)

// WithUpgradeClient sets the HTTP client used for WebSocket upgrade requests.
func WithUpgradeClient(c *http.Client) WSTransportOption {
	return func(o *wsTransportOptions) {
		if c != nil {
			o.upgradeClient = c
		}
	}
}

// WithLogger sets the logger for transport-level debug output.
func WithLogger(l abstractlogger.Logger) WSTransportOption {
	return func(o *wsTransportOptions) {
		if l != nil {
			o.logger = l
		}
	}
}

// WithPingInterval sets how often protocol-level pings are sent to all connections.
// Zero disables pinging.
func WithPingInterval(d time.Duration) WSTransportOption {
	return func(o *wsTransportOptions) {
		if d > 0 {
			o.pingInterval = d
		}
	}
}

// WithPingTimeout sets how long a connection may go without a pong before being closed.
// Zero disables the timeout (pings are sent but unresponsive connections are not killed).
func WithPingTimeout(d time.Duration) WSTransportOption {
	return func(o *wsTransportOptions) {
		if d > 0 {
			o.pingTimeout = d
		}
	}
}

// WithAckTimeout sets the maximum time to wait for a connection_ack after sending
// connection_init. Zero uses the protocol default (30s).
func WithAckTimeout(d time.Duration) WSTransportOption {
	return func(o *wsTransportOptions) {
		if d > 0 {
			o.ackTimeout = d
		}
	}
}

// WithWriteTimeout sets the timeout for WebSocket write operations on new connections.
// Zero uses DefaultWriteTimeout (5s).
func WithWriteTimeout(d time.Duration) WSTransportOption {
	return func(o *wsTransportOptions) {
		if d > 0 {
			o.writeTimeout = d
		}
	}
}

// WithReadLimit sets the maximum size in bytes for incoming WebSocket messages.
// Zero uses DefaultReadLimit (1MB).
func WithReadLimit(n int64) WSTransportOption {
	return func(o *wsTransportOptions) {
		if n > 0 {
			o.readLimit = n
		}
	}
}

type WSTransport struct {
	ctx  context.Context
	opts wsTransportOptions

	mu      sync.Mutex
	dialing map[uint64]*dialResult
	conns   map[uint64]*WSConnection
}

type dialResult struct {
	done chan struct{}
	conn *WSConnection
	err  error
}

// NewWSTransport creates a new WSTransport. The transport will automatically close
// all connections when ctx is cancelled.
//
// If WithPingInterval is set, a single goroutine sends protocol-level pings to all
// connections at that cadence. If WithPingTimeout is also set, connections that fail
// to respond with a pong within that window are shut down.
func NewWSTransport(ctx context.Context, opts ...WSTransportOption) *WSTransport {
	o := wsTransportOptions{
		upgradeClient: http.DefaultClient,
		logger:        abstractlogger.NoopLogger,
		readLimit:     DefaultReadLimit,
	}
	for _, apply := range opts {
		apply(&o)
	}

	t := &WSTransport{
		ctx:     ctx,
		opts:    o,
		conns:   make(map[uint64]*WSConnection),
		dialing: make(map[uint64]*dialResult),
	}

	context.AfterFunc(ctx, t.closeAll)

	if o.pingInterval > 0 {
		go t.pingLoop()
	}

	return t
}

func (t *WSTransport) Subscribe(ctx context.Context, req *common.Request, opts common.Options) (<-chan *common.Message, func(), error) {
	conn, err := t.getOrDial(ctx, opts)
	if err != nil {
		return nil, nil, err
	}

	id := xid.New().String()
	return conn.Subscribe(ctx, id, req)
}

// closeAll closes all connections. Called automatically when context is cancelled.
func (t *WSTransport) closeAll() {
	t.mu.Lock()
	conns := make([]*WSConnection, 0, len(t.conns))
	for _, conn := range t.conns {
		conns = append(conns, conn)
	}

	t.conns = make(map[uint64]*WSConnection)

	t.mu.Unlock()

	t.opts.logger.Debug("wsTransport.closeAll",
		abstractlogger.Int("connections", len(conns)),
	)

	for _, conn := range conns {
		conn.Close()
	}
}

// pingLoop sends periodic pings to all active connections and shuts down
// any that have not responded with a pong in time.
func (t *WSTransport) pingLoop() {
	tick := time.Tick(t.opts.pingInterval)
	for {
		select {
		case <-t.ctx.Done():
			return
		case <-tick:
			t.mu.Lock()
			conns := make([]*WSConnection, 0, len(t.conns))
			for _, conn := range t.conns {
				conns = append(conns, conn)
			}
			t.mu.Unlock()

			for _, conn := range conns {
				if conn.IsClosed() {
					continue
				}

				if t.opts.pingTimeout > 0 && conn.PongOverdue(t.opts.pingTimeout) {
					t.opts.logger.Debug("wsTransport.pingLoop",
						abstractlogger.String("action", "pong_timeout"),
					)
					conn.Close()
					continue
				}

				if err := conn.SendPing(DefaultWriteTimeout); err != nil {
					t.opts.logger.Debug("wsTransport.pingLoop",
						abstractlogger.String("action", "ping_failed"),
						abstractlogger.Error(err),
					)
				}
			}
		}
	}
}

// ReadLimit returns the configured read limit.
func (t *WSTransport) ReadLimit() int64 {
	return t.opts.readLimit
}

// WriteTimeout returns the configured write timeout for new connections.
func (t *WSTransport) WriteTimeout() time.Duration {
	return t.opts.writeTimeout
}

func (t *WSTransport) ConnCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	return len(t.conns)
}

func (t *WSTransport) getOrDial(ctx context.Context, opts common.Options) (*WSConnection, error) {
	key := connKey(opts)

	t.mu.Lock()

	if conn, ok := t.conns[key]; ok && !conn.IsClosed() {
		t.mu.Unlock()
		return conn, nil
	}

	if result, ok := t.dialing[key]; ok {
		t.mu.Unlock()
		<-result.done

		if result.err != nil {
			return nil, result.err
		}

		return result.conn, nil
	}

	result := &dialResult{done: make(chan struct{})}
	t.dialing[key] = result
	t.mu.Unlock()

	conn, err := t.dial(ctx, key, opts)

	result.conn = conn
	result.err = err
	close(result.done)

	t.mu.Lock()
	delete(t.dialing, key)

	if err == nil {
		t.conns[key] = conn
	}
	t.mu.Unlock()

	return conn, err
}

func (t *WSTransport) dial(ctx context.Context, key uint64, opts common.Options) (*WSConnection, error) {
	t.opts.logger.Debug("wsTransport.dial",
		abstractlogger.String("endpoint", opts.Endpoint),
		abstractlogger.String("subprotocol", string(opts.WSSubprotocol)),
	)

	wsConn, resp, err := websocket.Dial(ctx, opts.Endpoint, &websocket.DialOptions{ //nolint:bodyclose
		HTTPClient:   t.opts.upgradeClient,
		Subprotocols: opts.WSSubprotocol.Subprotocols(),
		HTTPHeader:   opts.Headers,
	})
	if err != nil {
		t.opts.logger.Error("wsTransport.dial",
			abstractlogger.String("endpoint", opts.Endpoint),
			abstractlogger.Error(err),
		)

		// backwards compatibility with error handling in the router
		if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
			return nil, ErrFailedUpgrade{URL: opts.Endpoint, StatusCode: resp.StatusCode}
		}

		return nil, err
	}

	wsConn.SetReadLimit(t.opts.readLimit)

	proto, err := t.negotiateSubprotocol(opts.WSSubprotocol, wsConn.Subprotocol())
	if err != nil {
		t.opts.logger.Error("wsTransport.dial",
			abstractlogger.String("endpoint", opts.Endpoint),
			abstractlogger.String("error", "subprotocol negotiation failed"),
			abstractlogger.Error(err),
		)
		wsConn.Close(websocket.StatusProtocolError, err.Error())
		return nil, err
	}

	if err := proto.Init(ctx, wsConn, opts.InitPayload); err != nil {
		t.opts.logger.Error("wsTransport.dial",
			abstractlogger.String("endpoint", opts.Endpoint),
			abstractlogger.String("error", "protocol init failed"),
			abstractlogger.Error(err),
		)
		wsConn.Close(websocket.StatusProtocolError, "init failed")
		return nil, err
	}

	t.opts.logger.Debug("wsTransport.dial",
		abstractlogger.String("endpoint", opts.Endpoint),
		abstractlogger.String("status", "connected"),
		abstractlogger.String("negotiated_subprotocol", wsConn.Subprotocol()),
	)

	conn := NewWSConnection(t.ctx, wsConn, proto,
		WithConnLogger(t.opts.logger),
		WithConnWriteTimeout(t.opts.writeTimeout),
		WithOnEmpty(func() { t.removeConn(key) }),
	)

	go conn.ReadLoop()

	return conn, nil
}

func (t *WSTransport) negotiateSubprotocol(requested common.WSSubprotocol, accepted string) (protocol.Protocol, error) {
	if requested != common.SubprotocolAuto {
		if accepted != string(requested) {
			return nil, ErrInvalidSubprotocol(accepted)
		}
	}

	switch common.WSSubprotocol(accepted) {
	case common.SubprotocolGraphQLTransportWS:
		p := protocol.NewGraphQLTransportWS()
		if t.opts.ackTimeout > 0 {
			p.AckTimeout = t.opts.ackTimeout
		}
		return p, nil
	case common.SubprotocolGraphQLWS:
		p := protocol.NewGraphQLWS()
		if t.opts.ackTimeout > 0 {
			p.AckTimeout = t.opts.ackTimeout
		}
		return p, nil
	default:
		return nil, ErrInvalidSubprotocol(accepted)
	}
}

func (t *WSTransport) removeConn(key uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.conns, key)
}

// connKey computes a hash key for connection pooling.
func connKey(opts common.Options) uint64 {
	h := pool.Hash64.Get()
	defer pool.Hash64.Put(h)

	_, _ = h.WriteString(opts.Endpoint)
	_, _ = h.WriteString("\x00")

	_, _ = h.WriteString(string(opts.WSSubprotocol))
	_, _ = h.WriteString("\x00")

	if len(opts.Headers) > 0 {
		_ = opts.Headers.Write(h)
	}
	_, _ = h.WriteString("\x00")

	if len(opts.InitPayload) > 0 {
		if data, err := json.Marshal(opts.InitPayload); err == nil {
			_, _ = h.Write(data)
		}
	}

	return h.Sum64()
}
