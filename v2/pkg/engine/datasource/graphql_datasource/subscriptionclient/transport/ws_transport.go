package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/rs/xid"
	shared "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/protocol"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

type WSTransport struct {
	httpClient *http.Client

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
// for WebSocket upgrade requests.
func NewWSTransport(httpClient *http.Client) (*WSTransport, error) {
	if httpClient == nil {
		return nil, fmt.Errorf("WSTransport: http.Client must not be nil")
	}
	return &WSTransport{
		httpClient: httpClient,
		conns:      make(map[uint64]*WSConnection),
		dialing:    make(map[uint64]*dialResult),
	}, nil
}

func (t *WSTransport) Subscribe(ctx context.Context, req *shared.Request, opts shared.Options) (<-chan *shared.Message, func(), error) {
	conn, err := t.getOrDial(ctx, opts)
	if err != nil {
		return nil, nil, err
	}

	id := xid.New().String()
	return conn.Subscribe(ctx, id, req)
}

func (t *WSTransport) Close() error {
	t.mu.Lock()

	// Copy because conn.Close -> shutdown -> onEmpty -> t.removeConn -> t.mu.Lock
	// would cause a deadlock
	conns := make([]*WSConnection, 0, len(t.conns))
	for _, conn := range t.conns {
		conns = append(conns, conn)
	}

	t.conns = make(map[uint64]*WSConnection)

	t.mu.Unlock()

	for _, conn := range conns {
		conn.Close()
	}

	return nil
}

func (t *WSTransport) ConnCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	return len(t.conns)
}

func (t *WSTransport) getOrDial(ctx context.Context, opts shared.Options) (*WSConnection, error) {
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

func (t *WSTransport) dial(ctx context.Context, key uint64, opts shared.Options) (*WSConnection, error) {
	var subprotocols []string
	switch opts.WSSubprotocol {
	case shared.SubprotocolGraphQLTWS:
		subprotocols = []string{"graphql-transport-ws"}
	case shared.SubprotocolGraphQLWS:
		subprotocols = []string{"graphql-ws"}
	default:
		// Auto: prefer modern, fall back to legacy
		subprotocols = []string{"graphql-transport-ws", "graphql-ws"}
	}

	wsConn, _, err := websocket.Dial(ctx, opts.Endpoint, &websocket.DialOptions{
		HTTPClient:   t.httpClient,
		Subprotocols: subprotocols,
		HTTPHeader:   opts.Headers,
	})
	if err != nil {
		return nil, err
	}

	proto, err := t.negotiateSubprotocol(opts.WSSubprotocol, wsConn.Subprotocol())
	if err != nil {
		wsConn.Close(websocket.StatusProtocolError, err.Error())
		return nil, err
	}

	if err := proto.Init(ctx, wsConn, opts.InitPayload); err != nil {
		wsConn.Close(websocket.StatusProtocolError, "init failed")
		return nil, err
	}

	conn := NewWSConnection(wsConn, proto, func() {
		t.removeConn(key)
	})

	go conn.ReadLoop()

	return conn, nil
}

func (t *WSTransport) negotiateSubprotocol(requested shared.WSSubprotocol, accepted string) (protocol.Protocol, error) {
	if requested != shared.SubprotocolAuto {
		if accepted != string(requested) {
			return nil, fmt.Errorf("server accepted %q but requested %q", accepted, requested)
		}
	}
	switch shared.WSSubprotocol(accepted) {
	case shared.SubprotocolGraphQLTWS:
		return protocol.NewGraphQLWS(), nil
	case shared.SubprotocolGraphQLWS:
		return protocol.NewGraphQLWSLegacy(), nil
	default:
		return nil, fmt.Errorf("unsupported subprotocol: %q", accepted)
	}
}

func (t *WSTransport) removeConn(key uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.conns, key)
}

// connKey computes a hash key for connection pooling.
func connKey(opts shared.Options) uint64 {
	h := pool.Hash64.Get()
	defer pool.Hash64.Put(h)

	h.WriteString(opts.Endpoint)
	h.WriteString("\x00")

	h.WriteString(string(opts.WSSubprotocol))
	h.WriteString("\x00")

	if len(opts.Headers) > 0 {
		opts.Headers.Write(h)
	}
	h.WriteString("\x00")

	if len(opts.InitPayload) > 0 {
		if data, err := json.Marshal(opts.InitPayload); err == nil {
			h.Write(data)
		}
	}

	return h.Sum64()
}
