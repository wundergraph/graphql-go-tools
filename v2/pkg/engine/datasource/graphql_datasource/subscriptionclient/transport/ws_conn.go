package transport

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/protocol"
)

var (
	errSubscriptionExists = errors.New("subscription ID already exists")

	defaultWriteTimeout = 5 * time.Second
	defaultReadLimit    = int64(1024 * 1024) // 1MB
)

type wsConnectionOptions struct {
	logger       abstractlogger.Logger
	writeTimeout time.Duration
	onEmpty      func()
}

// wsConnectionOption configures a wsConnection.
type wsConnectionOption func(*wsConnectionOptions)

// withConnLogger sets the logger for connection-level debug output.
func withConnLogger(l abstractlogger.Logger) wsConnectionOption {
	return func(o *wsConnectionOptions) {
		if l != nil {
			o.logger = l
		}
	}
}

// withConnWriteTimeout sets the timeout for write operations (subscribe, unsubscribe, pong).
func withConnWriteTimeout(d time.Duration) wsConnectionOption {
	return func(o *wsConnectionOptions) {
		if d > 0 {
			o.writeTimeout = d
		}
	}
}

// withOnEmpty sets a callback invoked when the last subscription is removed or the connection shuts down.
func withOnEmpty(f func()) wsConnectionOption {
	return func(o *wsConnectionOptions) {
		o.onEmpty = f
	}
}

type wsConnection struct {
	conn     *websocket.Conn
	protocol protocol.Protocol
	log      abstractlogger.Logger

	// cancel cancels the connection-scoped context, unblocking readLoop and
	// any in-flight writes. It is called exactly once inside shutdown().
	cancel context.CancelFunc
	ctx    context.Context

	subsMu sync.RWMutex
	subs   map[string]chan<- *common.Message

	closed atomic.Bool

	onEmpty func()

	writeTimeout time.Duration

	// Ping/pong tracking for client-initiated heartbeats.
	// Values stored as UnixNano timestamps.
	lastPingSentAt atomic.Int64
	lastPongAt     atomic.Int64
}

func newWSConnection(conn *websocket.Conn, proto protocol.Protocol, opts ...wsConnectionOption) *wsConnection {
	o := wsConnectionOptions{
		logger:       abstractlogger.NoopLogger,
		writeTimeout: defaultWriteTimeout,
	}
	for _, apply := range opts {
		apply(&o)
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &wsConnection{
		conn:     conn,
		protocol: proto,
		log:      o.logger,
		cancel:   cancel,
		ctx:      ctx,
		subs:     make(map[string]chan<- *common.Message),
		onEmpty:  o.onEmpty,

		writeTimeout: o.writeTimeout,
	}

	c.lastPongAt.Store(time.Now().UnixNano())

	return c
}

func (c *wsConnection) subscribe(ctx context.Context, id string, req *common.Request) (<-chan *common.Message, func(), error) {
	if c.closed.Load() {
		return nil, nil, common.ErrConnectionClosed
	}

	// Small buffer to absorb bursts
	ch := make(chan *common.Message, 8)

	c.subsMu.Lock()

	if _, exists := c.subs[id]; exists {
		c.subsMu.Unlock()
		return nil, nil, errSubscriptionExists
	}

	c.subs[id] = ch
	c.subsMu.Unlock()

	if err := c.protocol.Subscribe(ctx, c.conn, id, req); err != nil {
		c.log.Error("wsConnection.Subscribe",
			abstractlogger.String("id", id),
			abstractlogger.Error(err),
		)
		c.removeSub(id)
		return nil, nil, err
	}

	c.log.Debug("wsConnection.Subscribe",
		abstractlogger.String("id", id),
		abstractlogger.String("status", "subscribed"),
	)

	cancel := func() { c.unsubscribe(id) }

	return ch, cancel, nil
}

func (c *wsConnection) removeSub(id string) {
	c.subsMu.Lock()
	ch, exists := c.subs[id]
	delete(c.subs, id)
	isEmpty := len(c.subs) == 0
	c.subsMu.Unlock()

	if exists {
		close(ch)
	}

	if isEmpty {
		c.closeConn()
	}
}

func (c *wsConnection) unsubscribe(id string) {
	c.subsMu.Lock()
	_, exists := c.subs[id]
	c.subsMu.Unlock()

	if !exists {
		return
	}

	c.log.Debug("wsConnection.unsubscribe", abstractlogger.String("id", id))

	unsubscribeCtx, cancel := context.WithTimeout(context.Background(), c.writeTimeout)
	defer cancel()

	_ = c.protocol.Unsubscribe(unsubscribeCtx, c.conn, id)

	c.removeSub(id)
}

func (c *wsConnection) readLoop() {
	defer c.shutdown(errors.New("read loop exited"))

	for {
		if c.closed.Load() {
			return
		}

		msg, err := c.protocol.Read(c.ctx, c.conn)
		if err != nil {
			c.log.Debug("wsConnection.ReadLoop",
				abstractlogger.String("status", "error"),
				abstractlogger.Error(err),
			)
			c.shutdown(fmt.Errorf("%w: read: %w", common.ErrConnectionClosed, err))
			return
		}

		switch msg.Type {
		case protocol.MessagePing:
			c.log.Debug("wsConnection.ReadLoop", abstractlogger.String("message", "ping"))
			pongCtx, cancel := context.WithTimeout(c.ctx, c.writeTimeout)
			_ = c.protocol.Pong(pongCtx, c.conn)
			cancel()
		case protocol.MessagePong:
			c.lastPongAt.Store(time.Now().UnixNano())
			c.log.Debug("wsConnection.ReadLoop", abstractlogger.String("message", "pong"))
		case protocol.MessageData, protocol.MessageError, protocol.MessageComplete:
			c.dispatch(msg)
		}
	}
}

func (c *wsConnection) dispatch(msg *protocol.Message) {
	c.subsMu.RLock()
	ch, exists := c.subs[msg.ID]
	c.subsMu.RUnlock()

	if !exists {
		return
	}

	ch <- msg.IntoClientMessage()

	if msg.Type == protocol.MessageComplete || msg.Type == protocol.MessageError {
		c.unsubscribe(msg.ID)
	}
}

func (c *wsConnection) shutdown(err error) {
	if !c.closed.CompareAndSwap(false, true) {
		return
	}

	c.log.Debug("wsConnection.shutdown",
		abstractlogger.Error(err),
	)

	// Cancel the connection-scoped context so readLoop's Read unblocks.
	c.cancel()

	c.conn.Close(websocket.StatusNormalClosure, "shutdown")

	c.subsMu.Lock()
	subs := c.subs
	c.subs = make(map[string]chan<- *common.Message)
	c.subsMu.Unlock()

	errMsg := &common.Message{Err: err, Done: true}
	for _, ch := range subs {
		select {
		case ch <- errMsg:
		case <-time.After(100 * time.Millisecond):
			// dead consumer
		}
		close(ch)
	}

	if c.onEmpty != nil {
		c.onEmpty()
	}
}

func (c *wsConnection) closeConn() {
	c.shutdown(common.ErrConnectionClosed)
}

// writeTimeoutDuration returns the configured write timeout.
func (c *wsConnection) writeTimeoutDuration() time.Duration {
	return c.writeTimeout
}

func (c *wsConnection) subCount() int {
	c.subsMu.RLock()
	defer c.subsMu.RUnlock()
	return len(c.subs)
}

// sendPing sends a protocol-level ping message and records the timestamp.
func (c *wsConnection) sendPing(timeout time.Duration) error {
	pingCtx, cancel := context.WithTimeout(c.ctx, timeout)
	defer cancel()

	err := c.protocol.Ping(pingCtx, c.conn)
	if err != nil {
		return err
	}

	c.lastPingSentAt.Store(time.Now().UnixNano())
	return nil
}

// pongOverdue returns true if a pong has not been received since the last ping
// and the ping timeout has elapsed.
func (c *wsConnection) pongOverdue(timeout time.Duration) bool {
	pingSent := c.lastPingSentAt.Load()
	if pingSent == 0 {
		return false
	}
	return c.lastPongAt.Load() < pingSent && time.Since(time.Unix(0, pingSent)) > timeout
}

func (c *wsConnection) isClosed() bool {
	return c.closed.Load()
}
