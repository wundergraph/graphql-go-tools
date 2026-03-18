package client

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/transport"
)

var ErrClientClosed = errors.New("client closed")

type Client struct {
	ctx context.Context
	log abstractlogger.Logger

	ws  *transport.WSTransport
	sse *transport.SSETransport
}

// Stats contains client statistics.
type Stats struct {
	WSConns  int // active WebSocket connections
	SSEConns int // active SSE connections
}

// Config holds the client configuration.
type Config struct {
	UpgradeClient   *http.Client
	StreamingClient *http.Client
	Logger          abstractlogger.Logger
	PingInterval    time.Duration
	PingTimeout     time.Duration
	AckTimeout      time.Duration
	WriteTimeout    time.Duration
	ReadLimit       int64
}

// New creates a new subscription client with the provided config.
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
		ctx: ctx,
		log: cfg.Logger,

		ws: transport.NewWSTransport(ctx,
			transport.WithUpgradeClient(cfg.UpgradeClient),
			transport.WithLogger(cfg.Logger),
			transport.WithPingInterval(cfg.PingInterval),
			transport.WithPingTimeout(cfg.PingTimeout),
			transport.WithAckTimeout(cfg.AckTimeout),
			transport.WithWriteTimeout(cfg.WriteTimeout),
			transport.WithReadLimit(cfg.ReadLimit),
		),
		sse: transport.NewSSETransport(ctx, cfg.StreamingClient, cfg.Logger),
	}

	c.log.Debug("subscriptionClient.New", abstractlogger.String("status", "initialized"))

	return c
}

// Subscribe creates a new upstream via the appropriate transport.
func (c *Client) Subscribe(ctx context.Context, req *common.Request, opts common.Options, handler common.Handler) (func(), error) {
	if c.ctx.Err() != nil {
		return nil, ErrClientClosed
	}

	if opts.Transport == common.TransportSSE {
		return c.sse.Subscribe(ctx, req, opts, handler)
	}
	return c.ws.Subscribe(ctx, req, opts, handler)
}

// Stats returns client statistics.
func (c *Client) Stats() Stats {
	stats := Stats{
		WSConns:  c.ws.ConnCount(),
		SSEConns: c.sse.ConnCount(),
	}
	return stats
}
