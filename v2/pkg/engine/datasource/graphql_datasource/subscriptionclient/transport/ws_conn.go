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
	ErrSubscriptionExists = errors.New("subscription ID already exists")

	defaultWriteTimeout = 5 * time.Second
)

type wsConnectionOptions struct {
	logger       abstractlogger.Logger
	writeTimeout time.Duration
	idleTimeout  time.Duration
	onEmpty      func()
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
	subs   map[string]common.Handler

	closed atomic.Bool

	onEmpty     func()
	idleTimeout time.Duration

	writeTimeout time.Duration

	// Ping/pong tracking for client-initiated heartbeats.
	// Values stored as UnixNano timestamps.
	lastPingSentAt atomic.Int64
	lastPongAt     atomic.Int64
}

func newWSConnection(conn *websocket.Conn, proto protocol.Protocol, opts wsConnectionOptions) *wsConnection {
	if opts.logger == nil {
		opts.logger = abstractlogger.NoopLogger
	}
	if opts.writeTimeout <= 0 {
		opts.writeTimeout = defaultWriteTimeout
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &wsConnection{
		conn:     conn,
		protocol: proto,
		log:      opts.logger,
		cancel:   cancel,
		ctx:      ctx,
		subs:     make(map[string]common.Handler),
		onEmpty:  opts.onEmpty,

		writeTimeout: opts.writeTimeout,
		idleTimeout:  opts.idleTimeout,
	}

	c.lastPongAt.Store(time.Now().UnixNano())

	return c
}

func (c *wsConnection) subscribe(ctx context.Context, id string, req *common.Request, handler common.Handler) (func(), error) {
	if c.closed.Load() {
		return nil, common.ErrConnectionClosed
	}

	c.subsMu.Lock()

	if _, exists := c.subs[id]; exists {
		c.subsMu.Unlock()
		return nil, ErrSubscriptionExists
	}

	c.subs[id] = handler
	c.subsMu.Unlock()

	subscribeCtx, subscribeCancel := context.WithTimeout(ctx, c.writeTimeout)
	defer subscribeCancel()

	if err := c.protocol.Subscribe(subscribeCtx, c.conn, id, req); err != nil {
		c.log.Error("wsConnection.Subscribe",
			abstractlogger.String("id", id),
			abstractlogger.Error(err),
		)
		c.removeSub(id)
		return nil, err
	}

	c.log.Debug("wsConnection.Subscribe",
		abstractlogger.String("id", id),
		abstractlogger.String("status", "subscribed"),
	)

	cancel := func() { c.unsubscribe(id) }

	return cancel, nil
}

func (c *wsConnection) removeSub(id string) {
	c.subsMu.Lock()
	delete(c.subs, id)
	isEmpty := len(c.subs) == 0
	c.subsMu.Unlock()

	if isEmpty {
		if c.idleTimeout > 0 {
			time.AfterFunc(c.idleTimeout, func() {
				c.subsMu.RLock()
				stillEmpty := len(c.subs) == 0
				c.subsMu.RUnlock()
				if stillEmpty {
					c.closeConn()
				}
			})
		} else {
			c.closeConn()
		}
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
	handler, exists := c.subs[msg.ID]
	c.subsMu.RUnlock()

	if !exists {
		return
	}

	handler(msg.IntoClientMessage())

	if msg.Type == protocol.MessageComplete || msg.Type == protocol.MessageError {
		c.removeSub(msg.ID)
	}
}

func (c *wsConnection) shutdown(err error) {
	if !c.closed.CompareAndSwap(false, true) {
		return
	}

	c.log.Debug("wsConnection.shutdown",
		abstractlogger.Error(err),
	)

	c.conn.Close(websocket.StatusNormalClosure, "shutdown")

	c.subsMu.Lock()
	subs := c.subs
	c.subs = make(map[string]common.Handler)
	c.subsMu.Unlock()

	errMsg := &common.Message{Type: common.MessageTypeConnectionError, Err: err}
	for _, handler := range subs {
		handler(errMsg)
	}

	// Cancel after dispatching errors so readLoop consumers still have a live
	// context when they receive the error message.
	c.cancel()

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
func (c *wsConnection) sendPing() error {
	pingCtx, cancel := context.WithTimeout(c.ctx, c.writeTimeout)
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
