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

type WSTransport struct {
	ctx           context.Context
	upgradeClient *http.Client
	log           abstractlogger.Logger

	pingInterval time.Duration
	pingTimeout  time.Duration

	mu      sync.Mutex
	dialing map[uint64]*dialResult
	conns   map[uint64]*WSConnection
}

type dialResult struct {
	done chan struct{}
	conn *WSConnection
	err  error
}

// NewWSTransport creates a new WSTransport with the provided http.Client
// for WebSocket upgrade requests. The transport will automatically close
// all connections when ctx is cancelled.
//
// If pingInterval > 0, a single goroutine sends protocol-level pings to all
// connections at that cadence. If pingTimeout > 0, connections that fail to
// respond with a pong within that window after a ping are shut down.
func NewWSTransport(ctx context.Context, upgradeClient *http.Client, log abstractlogger.Logger, pingInterval, pingTimeout time.Duration) *WSTransport {
	if log == nil {
		log = abstractlogger.NoopLogger
	}

	t := &WSTransport{
		ctx:           ctx,
		upgradeClient: upgradeClient,
		log:           log,
		pingInterval:  pingInterval,
		pingTimeout:   pingTimeout,
		conns:         make(map[uint64]*WSConnection),
		dialing:       make(map[uint64]*dialResult),
	}

	context.AfterFunc(ctx, t.closeAll)

	if pingInterval > 0 {
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

	t.log.Debug("wsTransport.closeAll",
		abstractlogger.Int("connections", len(conns)),
	)

	for _, conn := range conns {
		conn.Close()
	}
}

// pingLoop sends periodic pings to all active connections and shuts down
// any that have not responded with a pong in time.
func (t *WSTransport) pingLoop() {
	tick := time.Tick(t.pingInterval)
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

				if t.pingTimeout > 0 && conn.PongOverdue(t.pingTimeout) {
					t.log.Debug("wsTransport.pingLoop",
						abstractlogger.String("action", "pong_timeout"),
					)
					conn.Close()
					continue
				}

				if err := conn.SendPing(DefaultWriteTimeout); err != nil {
					t.log.Debug("wsTransport.pingLoop",
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
	t.log.Debug("wsTransport.dial",
		abstractlogger.String("endpoint", opts.Endpoint),
		abstractlogger.String("subprotocol", string(opts.WSSubprotocol)),
	)

	wsConn, resp, err := websocket.Dial(ctx, opts.Endpoint, &websocket.DialOptions{ //nolint:bodyclose
		HTTPClient:   t.upgradeClient,
		Subprotocols: opts.WSSubprotocol.Subprotocols(),
		HTTPHeader:   opts.Headers,
	})
	if err != nil {
		t.log.Error("wsTransport.dial",
			abstractlogger.String("endpoint", opts.Endpoint),
			abstractlogger.Error(err),
		)

		// backwards compatibility with error handling in the router
		if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
			return nil, ErrFailedUpgrade{URL: opts.Endpoint, StatusCode: resp.StatusCode}
		}

		return nil, err
	}

	proto, err := t.negotiateSubprotocol(opts.WSSubprotocol, wsConn.Subprotocol())
	if err != nil {
		t.log.Error("wsTransport.dial",
			abstractlogger.String("endpoint", opts.Endpoint),
			abstractlogger.String("error", "subprotocol negotiation failed"),
			abstractlogger.Error(err),
		)
		wsConn.Close(websocket.StatusProtocolError, err.Error())
		return nil, err
	}

	if err := proto.Init(ctx, wsConn, opts.InitPayload); err != nil {
		t.log.Error("wsTransport.dial",
			abstractlogger.String("endpoint", opts.Endpoint),
			abstractlogger.String("error", "protocol init failed"),
			abstractlogger.Error(err),
		)
		wsConn.Close(websocket.StatusProtocolError, "init failed")
		return nil, err
	}

	t.log.Debug("wsTransport.dial",
		abstractlogger.String("endpoint", opts.Endpoint),
		abstractlogger.String("status", "connected"),
		abstractlogger.String("negotiated_subprotocol", wsConn.Subprotocol()),
	)

	conn := NewWSConnection(t.ctx, wsConn, proto, t.log, func() {
		t.removeConn(key)
	})

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
