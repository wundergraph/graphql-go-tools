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
	ErrConnectionClosed   = errors.New("connection closed")
	ErrSubscriptionExists = errors.New("subscription ID already exists")

	DefaultWriteTimeout = 5 * time.Second
)

type WSConnection struct {
	ctx      context.Context
	conn     *websocket.Conn
	protocol protocol.Protocol
	log      abstractlogger.Logger

	writeMu sync.Mutex

	subsMu sync.RWMutex
	subs   map[string]chan<- *common.Message

	closed   atomic.Bool
	closeErr error
	done     chan struct{}

	onEmpty func()

	WriteTimeout time.Duration
}

func NewWSConnection(ctx context.Context, conn *websocket.Conn, protocol protocol.Protocol, log abstractlogger.Logger, onEmpty func()) *WSConnection {
	if log == nil {
		log = abstractlogger.NoopLogger
	}

	return &WSConnection{
		ctx:      ctx,
		conn:     conn,
		protocol: protocol,
		log:      log,
		subs:     make(map[string]chan<- *common.Message),
		done:     make(chan struct{}),
		onEmpty:  onEmpty,

		WriteTimeout: DefaultWriteTimeout,
	}
}

func (c *WSConnection) Subscribe(ctx context.Context, id string, req *common.Request) (<-chan *common.Message, func(), error) {
	if c.closed.Load() {
		return nil, nil, ErrConnectionClosed
	}

	// Small buffer to absorb bursts
	ch := make(chan *common.Message, 8)

	c.subsMu.Lock()

	if _, exists := c.subs[id]; exists {
		c.subsMu.Unlock()
		return nil, nil, ErrSubscriptionExists
	}

	c.subs[id] = ch
	c.subsMu.Unlock()

	if err := c.withWriteLock(func() error {
		return c.protocol.Subscribe(ctx, c.conn, id, req)
	}); err != nil {
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

	cancel := func() { c.removeSub(id) }

	return ch, cancel, nil
}

func (c *WSConnection) removeSub(id string) {
	c.subsMu.Lock()
	ch, exists := c.subs[id]
	delete(c.subs, id)
	isEmpty := len(c.subs) == 0
	c.subsMu.Unlock()

	if exists {
		close(ch)
	}

	if isEmpty {
		if c.onEmpty != nil {
			c.onEmpty()
		}
		c.Close()
	}
}

func (c *WSConnection) unsubscribe(id string) {
	c.subsMu.Lock()
	_, exists := c.subs[id]
	c.subsMu.Unlock()

	if !exists {
		return
	}

	c.log.Debug("wsConnection.unsubscribe", abstractlogger.String("id", id))

	unsubscribeCtx, cancel := context.WithTimeout(context.Background(), c.WriteTimeout)
	defer cancel()

	_ = c.withWriteLock(func() error {
		return c.protocol.Unsubscribe(unsubscribeCtx, c.conn, id)
	})

	c.removeSub(id)
}

func (c *WSConnection) withWriteLock(f func() error) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.closed.Load() {
		return ErrConnectionClosed
	}

	return f()
}

func (c *WSConnection) ReadLoop() {
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
			c.shutdown(fmt.Errorf("read: %w", err))
			return
		}

		switch msg.Type {
		case protocol.MessagePing:
			c.log.Debug("wsConnection.ReadLoop", abstractlogger.String("message", "ping"))
			pongCtx, cancel := context.WithTimeout(c.ctx, c.WriteTimeout)
			_ = c.withWriteLock(func() error {
				return c.protocol.Pong(pongCtx, c.conn)
			})
			cancel()
		case protocol.MessagePong:
			// Do nothing, pongs can sometimes be used as unidirectional heartbeats
			c.log.Debug("wsConnection.ReadLoop", abstractlogger.String("message", "pong"))
		case protocol.MessageData, protocol.MessageError, protocol.MessageComplete:
			c.dispatch(msg)
		}
	}
}

func (c *WSConnection) dispatch(msg *protocol.Message) {
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

func (c *WSConnection) shutdown(err error) {
	if !c.closed.CompareAndSwap(false, true) {
		return
	}

	c.closeErr = err

	c.log.Debug("wsConnection.shutdown",
		abstractlogger.Error(err),
	)

	close(c.done)

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
}

func (c *WSConnection) Close() error {
	c.shutdown(ErrConnectionClosed)
	return nil
}

func (c *WSConnection) Done() <-chan struct{} {
	return c.done
}

func (c *WSConnection) Err() error {
	return c.closeErr
}

func (c *WSConnection) SubCount() int {
	c.subsMu.RLock()
	defer c.subsMu.RUnlock()
	return len(c.subs)
}

func (c *WSConnection) IsClosed() bool {
	return c.closed.Load()
}
