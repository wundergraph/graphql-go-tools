package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/transport"
)

var ErrClientClosed = errors.New("client closed")

const (
	defaultReadLimit    = 1 << 20 // 1MiB
	defaultAckTimeout   = 30 * time.Second
	defaultWriteTimeout = 5 * time.Second
)

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
	WSIdleTimeout   time.Duration
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
	if cfg.ReadLimit <= 0 {
		cfg.ReadLimit = defaultReadLimit
	}
	if cfg.AckTimeout <= 0 {
		cfg.AckTimeout = defaultAckTimeout
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = defaultWriteTimeout
	}

	c := &Client{
		ctx: ctx,
		log: cfg.Logger,

		ws: transport.NewWSTransport(ctx, transport.WSTransportOptions{
			UpgradeClient: cfg.UpgradeClient,
			Logger:        cfg.Logger,
			PingInterval:  cfg.PingInterval,
			PingTimeout:   cfg.PingTimeout,
			AckTimeout:    cfg.AckTimeout,
			WriteTimeout:  cfg.WriteTimeout,
			ReadLimit:     cfg.ReadLimit,
			IdleTimeout:   cfg.WSIdleTimeout,
		}),
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

	switch opts.Transport {
	case common.TransportSSE:
		return c.sse.Subscribe(ctx, req, opts, handler)
	case common.TransportWS:
		return c.ws.Subscribe(ctx, req, opts, handler)
	default:
		return nil, fmt.Errorf("unsupported transport: %q", opts.Transport)
	}
}

// Stats returns client statistics.
func (c *Client) Stats() Stats {
	stats := Stats{
		WSConns:  c.ws.ConnCount(),
		SSEConns: c.sse.ConnCount(),
	}
	return stats
}
