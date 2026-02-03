package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/jensneuse/abstractlogger"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/transport"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

var (
	ErrClientClosed       = errors.New("client closed")
	ErrSubscriptionClosed = errors.New("subscription closed")
)

// Client manages GraphQL subscriptions with deduplication and fan-out.
// It is designed as a shared singleton for proxy use - each Subscribe call
// provides its own endpoint, headers, and auth from the downstream request.
//
// The client's lifecycle is tied to the provided context. When the context
// is cancelled, all subscriptions are terminated and resources are released.
type Client struct {
	ctx context.Context
	log abstractlogger.Logger

	mu   sync.Mutex
	subs map[uint64]*subscription
	ws   *transport.WSTransport
	sse  *transport.SSETransport
}

// subscription represents a deduplicated upstream subscription with fan-out.
type subscription struct {
	source     <-chan *common.Message
	cancelFn   func()
	cancelOnce sync.Once

	mu        sync.RWMutex
	listeners map[uint64]chan *common.Message
	nextID    uint64

	done chan struct{}
}

// Stats contains client statistics.
type Stats struct {
	WSConns       int // active WebSocket connections
	SSEConns      int // active SSE connections
	Subscriptions int // unique upstream subscriptions
	Listeners     int // total listener channels
}

// Config holds the client configuration.
type Config struct {
	UpgradeClient   *http.Client
	StreamingClient *http.Client
	Logger          abstractlogger.Logger
}

// New creates a new subscription client with the provided config.
//
// The client's lifecycle is tied to ctx. When ctx is cancelled, all subscriptions
// are automatically terminated and resources are released.
func New(ctx context.Context, cfg Config) *Client {
	if cfg.UpgradeClient == nil {
		cfg.UpgradeClient = http.DefaultClient
	}
	if cfg.StreamingClient == nil {
		cfg.StreamingClient = http.DefaultClient
	}
	if cfg.Logger == nil {
		cfg.Logger = abstractlogger.NoopLogger
	}

	c := &Client{
		ctx:  ctx,
		log:  cfg.Logger,
		subs: make(map[uint64]*subscription),
		ws:   transport.NewWSTransport(ctx, cfg.UpgradeClient, cfg.Logger),
		sse:  transport.NewSSETransport(ctx, cfg.StreamingClient, cfg.Logger),
	}

	context.AfterFunc(ctx, c.shutdown)

	c.log.Debug("subscriptionClient.New", abstractlogger.String("status", "initialized"))

	return c
}

// Subscribe creates or joins a subscription.
// If an identical subscription exists (same opts + req), the caller joins it.
// Otherwise, a new upstream subscription is created via the appropriate transport.
func (c *Client) Subscribe(ctx context.Context, req *common.Request, opts common.Options) (<-chan *common.Message, func(), error) {
	key := subscriptionKey(opts, req)

	c.mu.Lock()

	if c.ctx.Err() != nil {
		c.mu.Unlock()
		c.log.Debug("subscriptionClient.Subscribe",
			abstractlogger.String("status", "rejected"),
			abstractlogger.String("reason", "client closed"),
		)
		return nil, nil, ErrClientClosed
	}

	// Dedup check
	if sub, ok := c.subs[key]; ok {
		c.mu.Unlock()
		c.log.Debug("subscriptionClient.Subscribe",
			abstractlogger.String("status", "dedup_hit"),
			abstractlogger.String("endpoint", opts.Endpoint),
		)
		return sub.addListener(ctx)
	}

	// Route to transport
	var source <-chan *common.Message
	var cancel func()
	var err error

	if opts.Transport == common.TransportSSE {
		source, cancel, err = c.sse.Subscribe(ctx, req, opts)
	} else {
		source, cancel, err = c.ws.Subscribe(ctx, req, opts)
	}
	if err != nil {
		c.mu.Unlock()
		c.log.Error("subscriptionClient.Subscribe",
			abstractlogger.String("status", "failed"),
			abstractlogger.String("endpoint", opts.Endpoint),
			abstractlogger.Error(err),
		)
		return nil, nil, err
	}

	c.log.Debug("subscriptionClient.Subscribe",
		abstractlogger.String("status", "created"),
		abstractlogger.String("endpoint", opts.Endpoint),
		abstractlogger.String("transport", string(opts.Transport)),
	)

	sub := &subscription{
		source:    source,
		cancelFn:  cancel,
		listeners: make(map[uint64]chan *common.Message),
		done:      make(chan struct{}),
	}
	c.subs[key] = sub
	c.mu.Unlock()

	// Start fan-out
	go sub.fanout(func() {
		c.mu.Lock()
		delete(c.subs, key)
		c.mu.Unlock()
	})

	return sub.addListener(ctx)
}

// shutdown terminates all subscriptions. Called automatically when context is cancelled.
func (c *Client) shutdown() {
	c.mu.Lock()

	// Copy to avoid holding lock during cancel
	subs := make([]*subscription, 0, len(c.subs))
	for _, sub := range c.subs {
		subs = append(subs, sub)
	}
	c.subs = make(map[uint64]*subscription)
	c.mu.Unlock()

	c.log.Debug("subscriptionClient.shutdown",
		abstractlogger.Int("subscriptions", len(subs)),
	)

	// Cancel all subscriptions - transports handle their own shutdown via context
	for _, sub := range subs {
		sub.cancel()
	}
}

// Stats returns client statistics.
func (c *Client) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()

	stats := Stats{
		WSConns:       c.ws.ConnCount(),
		SSEConns:      c.sse.ConnCount(),
		Subscriptions: len(c.subs),
	}
	for _, sub := range c.subs {
		sub.mu.RLock()
		stats.Listeners += len(sub.listeners)
		sub.mu.RUnlock()
	}
	return stats
}

// cancel cancels the upstream subscription (idempotent).
func (s *subscription) cancel() {
	s.cancelOnce.Do(s.cancelFn)
}

// fanout reads from source and broadcasts to all listeners.
func (s *subscription) fanout(onDone func()) {
	defer onDone()
	defer close(s.done)

	for msg := range s.source {
		// Copy listeners under lock, then send without lock
		s.mu.RLock()
		listeners := make([]chan *common.Message, 0, len(s.listeners))
		for _, ch := range s.listeners {
			listeners = append(listeners, ch)
		}
		s.mu.RUnlock()

		for _, ch := range listeners {
			select {
			case ch <- msg:
			default:
				// Listener not reading - skip
			}
		}
	}

	// Source closed, close all listeners
	s.mu.Lock()
	for _, ch := range s.listeners {
		close(ch)
	}
	s.listeners = nil
	s.mu.Unlock()
}

// addListener adds a new listener to the subscription.
func (s *subscription) addListener(ctx context.Context) (<-chan *common.Message, func(), error) {
	ch := make(chan *common.Message, 8)

	s.mu.Lock()
	if s.listeners == nil {
		s.mu.Unlock()
		return nil, nil, ErrSubscriptionClosed
	}
	id := s.nextID
	s.nextID++
	s.listeners[id] = ch
	s.mu.Unlock()

	cancel := sync.OnceFunc(func() { s.removeListener(id) })
	stop := context.AfterFunc(ctx, cancel)
	wrappedCancel := func() {
		stop()
		cancel()
	}

	return ch, wrappedCancel, nil
}

// removeListener removes a listener and cancels upstream if last.
func (s *subscription) removeListener(id uint64) {
	s.mu.Lock()
	delete(s.listeners, id)
	isEmpty := len(s.listeners) == 0
	s.mu.Unlock()

	if isEmpty {
		s.cancel()
	}
}

// subscriptionKey generates a deduplication key for a subscription.
func subscriptionKey(opts common.Options, req *common.Request) uint64 {
	h := pool.Hash64.Get()
	defer pool.Hash64.Put(h)

	h.WriteString(opts.Endpoint)
	h.WriteString("\x00")

	h.WriteString(string(opts.Transport))
	h.WriteString("\x00")

	h.WriteString(string(opts.WSSubprotocol))
	h.WriteString("\x00")

	h.WriteString(string(opts.SSEMethod))
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
	h.WriteString("\x00")

	h.WriteString(req.Query)
	h.WriteString("\x00")

	h.WriteString(req.OperationName)
	h.WriteString("\x00")

	if len(req.Variables) > 0 {
		h.Write(req.Variables)
	}
	h.WriteString("\x00")

	if len(req.Extensions) > 0 {
		h.Write(req.Extensions)
	}

	return h.Sum64()
}
