package client

import (
	"context"
	"errors"
	"net/http"

	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/transport"
)

var (
	ErrClientClosed       = errors.New("client closed")
	ErrSubscriptionClosed = errors.New("subscription closed")
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

		ws:  transport.NewWSTransport(ctx, cfg.UpgradeClient, cfg.Logger),
		sse: transport.NewSSETransport(ctx, cfg.StreamingClient, cfg.Logger),
	}

	c.log.Debug("subscriptionClient.New", abstractlogger.String("status", "initialized"))

	return c
}

// Subscribe creates a new upstream via the appropriate transport.
func (c *Client) Subscribe(ctx context.Context, req *common.Request, opts common.Options) (<-chan *common.Message, func(), error) {
	if c.ctx.Err() != nil {
		return nil, nil, ErrClientClosed
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

	return source, cancel, err
}

// Stats returns client statistics.
func (c *Client) Stats() Stats {
	stats := Stats{
		WSConns:  c.ws.ConnCount(),
		SSEConns: c.sse.ConnCount(),
	}
	return stats
}
