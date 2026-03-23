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
	UpgradeClient *http.Client
	Logger        abstractlogger.Logger
	PingInterval  time.Duration
	PingTimeout   time.Duration
	AckTimeout    time.Duration
	WriteTimeout  time.Duration
	ReadLimit     int64
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
	if opts.ReadLimit <= 0 {
		opts.ReadLimit = defaultReadLimit
	}
	if opts.WriteTimeout <= 0 {
		opts.WriteTimeout = defaultWriteTimeout
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

				if err := conn.sendPing(t.opts.WriteTimeout); err != nil {
					t.opts.Logger.Debug("wsTransport.pingLoop",
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
	return t.opts.ReadLimit
}

// WriteTimeout returns the configured write timeout for new connections.
func (t *WSTransport) WriteTimeout() time.Duration {
	return t.opts.WriteTimeout
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
		wsConn.Close(websocket.StatusProtocolError, err.Error())
		return nil, err
	}

	if err := proto.Init(ctx, wsConn, opts.InitPayload); err != nil {
		t.opts.Logger.Error("wsTransport.dial",
			abstractlogger.String("endpoint", opts.Endpoint),
			abstractlogger.String("error", "protocol init failed"),
			abstractlogger.Error(err),
		)
		wsConn.Close(websocket.StatusProtocolError, "init failed")
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
		p := protocol.NewGraphQLTransportWS()
		if t.opts.AckTimeout > 0 {
			p.AckTimeout = t.opts.AckTimeout
		}
		return p, nil
	case common.SubprotocolGraphQLWS:
		p := protocol.NewGraphQLWS()
		if t.opts.AckTimeout > 0 {
			p.AckTimeout = t.opts.AckTimeout
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
