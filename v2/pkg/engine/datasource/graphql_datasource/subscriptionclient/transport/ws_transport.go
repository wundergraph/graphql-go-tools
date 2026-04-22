package transport

import (
	"context"
	"encoding/json"
	"errors"
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

// ErrDialFailed indicates that the WebSocket dial (TCP + HTTP upgrade) failed.
// The underlying cause is available via errors.Unwrap.
var ErrDialFailed = errors.New("websocket dial failed")

// ErrInitFailed indicates that the GraphQL protocol init (connection_init /
// connection_ack handshake) failed after a successful WebSocket dial. The
// underlying cause (e.g. protocol.ErrAckTimeout) is available via errors.Unwrap.
var ErrInitFailed = errors.New("protocol init failed")

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

// WSTransportOptions configures a WSTransport.
type WSTransportOptions struct {
	// UpgradeClient is the HTTP client used for the WebSocket upgrade request.
	UpgradeClient *http.Client

	// Logger is the logger used for transport and connection-level events.
	Logger abstractlogger.Logger

	// ReadLimit is the maximum message size in bytes the WebSocket connection
	// will accept.
	ReadLimit int64

	// PingInterval is how often the transport sends a ping to each connection.
	// Zero disables pinging.
	PingInterval time.Duration

	// PingTimeout is how long a connection may go without a pong before it is
	// considered dead. Only meaningful when PingInterval is set.
	PingTimeout time.Duration

	// AckTimeout is the maximum time to wait for a connection_ack during the
	// protocol init handshake. Passed to the protocol at construction.
	AckTimeout time.Duration

	// WriteTimeout is the deadline applied to each WebSocket write (subscribe,
	// unsubscribe, ping, pong). Passed to each connection.
	WriteTimeout time.Duration

	// IdleTimeout is the duration a connection stays open after its last
	// subscription is removed, allowing new subscriptions to reuse it without
	// re-dialing. Zero means close immediately.
	IdleTimeout time.Duration
}

type WSTransport struct {
	ctx  context.Context
	opts WSTransportOptions

	mu      sync.Mutex
	dialing map[uint64]*dialResult
	conns   map[uint64]*wsConnection
}

type dialResult struct {
	done chan struct{}
	conn *wsConnection
	err  error
}

// NewWSTransport creates a new WSTransport. Connections are not closed when ctx
// is cancelled; instead they close themselves when their last subscriber is
// removed via the resolver's drain chain. The ping loop exits on ctx cancellation.
//
// If PingInterval is set, a single goroutine sends protocol-level pings to all
// connections at that cadence. If PingTimeout is also set, connections that fail
// to respond with a pong within that window are shut down.
func NewWSTransport(ctx context.Context, opts WSTransportOptions) *WSTransport {
	if opts.UpgradeClient == nil {
		opts.UpgradeClient = http.DefaultClient
	}

	if opts.Logger == nil {
		opts.Logger = abstractlogger.NoopLogger
	}

	t := &WSTransport{
		ctx:     ctx,
		opts:    opts,
		conns:   make(map[uint64]*wsConnection),
		dialing: make(map[uint64]*dialResult),
	}

	if opts.PingInterval > 0 {
		go t.pingLoop()
	}

	return t
}

// Subscribe initiates a GraphQL subscription over WebSocket. It reuses an
// existing connection when one is available for the same endpoint, subprotocol,
// headers, and init payload, dialing a new one otherwise.
func (t *WSTransport) Subscribe(ctx context.Context, req *common.Request, opts common.Options, handler common.Handler) (func(), error) {
	conn, err := t.getOrDial(ctx, opts)
	if err != nil {
		return nil, err
	}

	id := xid.New().String()
	return conn.subscribe(ctx, id, req, handler)
}

// pingLoop sends periodic pings to all active connections and shuts down
// any that have not responded with a pong in time.
func (t *WSTransport) pingLoop() {
	tick := time.Tick(t.opts.PingInterval)
	for {
		select {
		case <-t.ctx.Done():
			return
		case <-tick:
			t.mu.Lock()
			conns := make([]*wsConnection, 0, len(t.conns))
			for _, conn := range t.conns {
				conns = append(conns, conn)
			}
			t.mu.Unlock()

			for _, conn := range conns {
				if conn.isClosed() {
					continue
				}

				if t.opts.PingTimeout > 0 && conn.pongOverdue(t.opts.PingTimeout) {
					t.opts.Logger.Debug("wsTransport.pingLoop",
						abstractlogger.String("action", "pong_timeout"),
					)
					conn.closeConn()
					continue
				}

				if err := conn.sendPing(); err != nil {
					t.opts.Logger.Debug("wsTransport.pingLoop",
						abstractlogger.String("action", "ping_failed"),
						abstractlogger.Error(err),
					)
				}
			}
		}
	}
}

func (t *WSTransport) ConnCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	return len(t.conns)
}

func (t *WSTransport) getOrDial(ctx context.Context, opts common.Options) (*wsConnection, error) {
	key := connKey(opts)

	t.mu.Lock()

	if conn, ok := t.conns[key]; ok && !conn.isClosed() {
		t.mu.Unlock()
		return conn, nil
	}

	if result, ok := t.dialing[key]; ok {
		t.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-result.done:
		}

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

func (t *WSTransport) dial(ctx context.Context, key uint64, opts common.Options) (*wsConnection, error) {
	t.opts.Logger.Debug("wsTransport.dial",
		abstractlogger.String("endpoint", opts.Endpoint),
		abstractlogger.String("subprotocol", string(opts.WSSubprotocol)),
	)

	wsConn, resp, err := websocket.Dial(ctx, opts.Endpoint, &websocket.DialOptions{ //nolint:bodyclose
		HTTPClient:   t.opts.UpgradeClient,
		Subprotocols: opts.WSSubprotocol.Subprotocols(),
		HTTPHeader:   opts.Headers,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}

		t.opts.Logger.Error("wsTransport.dial",
			abstractlogger.String("endpoint", opts.Endpoint),
			abstractlogger.Error(err),
		)

		// backwards compatibility with error handling in the router
		if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
			return nil, ErrFailedUpgrade{URL: opts.Endpoint, StatusCode: resp.StatusCode}
		}

		return nil, fmt.Errorf("%w: %w", ErrDialFailed, err)
	}

	wsConn.SetReadLimit(t.opts.ReadLimit)

	proto, err := t.negotiateSubprotocol(opts.WSSubprotocol, wsConn.Subprotocol())
	if err != nil {
		t.opts.Logger.Error("wsTransport.dial",
			abstractlogger.String("endpoint", opts.Endpoint),
			abstractlogger.String("error", "subprotocol negotiation failed"),
			abstractlogger.Error(err),
		)
		_ = wsConn.Close(websocket.StatusProtocolError, err.Error())
		return nil, err
	}

	initCtx, initCancel := context.WithTimeout(ctx, t.opts.AckTimeout)
	defer initCancel()

	if err := proto.Init(initCtx, wsConn, opts.InitPayload); err != nil {
		t.opts.Logger.Error("wsTransport.dial",
			abstractlogger.String("endpoint", opts.Endpoint),
			abstractlogger.String("error", "protocol init failed"),
			abstractlogger.Error(err),
		)
		_ = wsConn.Close(websocket.StatusProtocolError, "init failed")
		return nil, fmt.Errorf("%w: %w", ErrInitFailed, err)
	}

	t.opts.Logger.Debug("wsTransport.dial",
		abstractlogger.String("endpoint", opts.Endpoint),
		abstractlogger.String("status", "connected"),
		abstractlogger.String("negotiated_subprotocol", wsConn.Subprotocol()),
	)

	conn := newWSConnection(wsConn, proto, wsConnectionOptions{
		logger:       t.opts.Logger,
		writeTimeout: t.opts.WriteTimeout,
		idleTimeout:  t.opts.IdleTimeout,
		onEmpty:      func() { t.removeConn(key) },
	})

	go conn.readLoop()

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
		return protocol.NewGraphQLTransportWS(), nil
	case common.SubprotocolGraphQLWS:
		return protocol.NewGraphQLWS(), nil
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
