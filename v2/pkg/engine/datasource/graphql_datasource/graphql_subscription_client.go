package graphql_datasource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jensneuse/abstractlogger"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/client"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// SubscriptionClientConfig holds the subscription client configuration.
type SubscriptionClientConfig struct {
	UpgradeClient   *http.Client
	StreamingClient *http.Client
	Logger          abstractlogger.Logger

	// Timeouts
	PingInterval time.Duration
	PingTimeout  time.Duration
	AckTimeout   time.Duration
}

func defaultSubscriptionClientConfig() *SubscriptionClientConfig {
	return &SubscriptionClientConfig{
		UpgradeClient:   http.DefaultClient,
		StreamingClient: http.DefaultClient,
		Logger:          abstractlogger.NoopLogger,

		PingInterval: 30 * time.Second,
		PingTimeout:  10 * time.Second,
		AckTimeout:   30 * time.Second,
	}
}

// SubscriptionClientOption configures the subscription client.
type SubscriptionClientOption func(*SubscriptionClientConfig)

// WithUpgradeClient sets the HTTP client used for WebSocket upgrade requests.
func WithUpgradeClient(c *http.Client) SubscriptionClientOption {
	return func(cfg *SubscriptionClientConfig) {
		if c != nil {
			cfg.UpgradeClient = c
		}
	}
}

// WithStreamingClient sets the HTTP client used for SSE requests.
// This client should have appropriate timeouts for long-lived connections.
func WithStreamingClient(c *http.Client) SubscriptionClientOption {
	return func(cfg *SubscriptionClientConfig) {
		if c != nil {
			cfg.StreamingClient = c
		}
	}
}

// WithLogger sets the logger for the client and its transports.
// If not set, logging is disabled (silent operation).
func WithLogger(log abstractlogger.Logger) SubscriptionClientOption {
	return func(cfg *SubscriptionClientConfig) {
		cfg.Logger = log
	}
}

// WithPingInterval sets the interval between ping messages for connection health checks.
// Only applies to graphql-transport-ws protocol (legacy graphql-ws uses server-initiated keepalive).
// Default: 30s. Set to 0 to disable client-initiated pings.
func WithPingInterval(d time.Duration) SubscriptionClientOption {
	return func(cfg *SubscriptionClientConfig) {
		cfg.PingInterval = d
	}
}

// WithPingTimeout sets the maximum time to wait for a pong response.
// If no pong is received within this duration, the connection is considered dead.
// Default: 10s.
func WithPingTimeout(d time.Duration) SubscriptionClientOption {
	return func(cfg *SubscriptionClientConfig) {
		cfg.PingTimeout = d
	}
}

// WithAckTimeout sets the maximum time to wait for connection_ack after connection_init.
// Default: 30s.
func WithAckTimeout(d time.Duration) SubscriptionClientOption {
	return func(cfg *SubscriptionClientConfig) {
		cfg.AckTimeout = d
	}
}

// subscriptionClientV2 implements GraphQLSubscriptionClient using the new
// channel-based subscription client.
type subscriptionClientV2 struct {
	client *client.Client
}

// NewGraphQLSubscriptionClient creates a new subscription client.
func NewGraphQLSubscriptionClient(ctx context.Context, opts ...SubscriptionClientOption) GraphQLSubscriptionClient {
	cfg := defaultSubscriptionClientConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	return &subscriptionClientV2{
		client: client.New(ctx, client.Config{
			UpgradeClient:   cfg.UpgradeClient,
			StreamingClient: cfg.StreamingClient,
			Logger:          cfg.Logger,
		}),
	}
}

// Subscribe implements GraphQLSubscriptionClient.
// It bridges the channel-based new client API to the callback-based updater interface.
func (c *subscriptionClientV2) Subscribe(ctx *resolve.Context, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error {
	opts, req, err := convertToClientOptions(options)
	if err != nil {
		return err
	}

	msgCh, cancel, err := c.client.Subscribe(ctx.Context(), req, opts)
	if err != nil {
		return err
	}

	go c.readLoop(ctx.Context(), msgCh, cancel, updater)

	return nil
}

// readLoop bridges the channel-based API to the callback-based updater.
func (c *subscriptionClientV2) readLoop(ctx context.Context, msgCh <-chan *common.Message, cancel func(), updater resolve.SubscriptionUpdater) {
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			updater.Complete()
			return

		case msg, ok := <-msgCh:
			if !ok {
				updater.Complete()
				return
			}

			if msg.Err != nil {
				// Only send error message if it's not a connection closure.
				// Connection closures are communicated via the close frame reason.
				if !errors.Is(msg.Err, common.ErrConnectionClosed) {
					updater.Update(formatSubscriptionError(msg.Err))
				}
				updater.Close(resolve.SubscriptionCloseKindDownstreamServiceError)
				return
			}

			if msg.Payload != nil {
				data, err := json.Marshal(msg.Payload)
				if err != nil {
					updater.Update(formatSubscriptionError(err))
					updater.Close(resolve.SubscriptionCloseKindDownstreamServiceError)
					return
				}
				updater.Update(data)
			}

			if msg.Done {
				updater.Complete()
				return
			}
		}
	}
}

// convertToClientOptions converts GraphQLSubscriptionOptions to the new client's types.
func convertToClientOptions(options GraphQLSubscriptionOptions) (common.Options, *common.Request, error) {
	opts := common.Options{
		Endpoint: options.URL,
		Headers:  options.Header,
	}

	// Transport selection
	if options.UseSSE {
		opts.Transport = common.TransportSSE
		if options.SSEMethodPost {
			opts.SSEMethod = common.SSEMethodPOST
		} else {
			opts.SSEMethod = common.SSEMethodGET
		}
	} else {
		opts.Transport = common.TransportWS
		opts.WSSubprotocol = mapWSSubprotocol(options.WsSubProtocol)
	}

	// Convert InitialPayload from json.RawMessage to map[string]any
	if len(options.InitialPayload) > 0 {
		var initPayload map[string]any
		if err := json.Unmarshal(options.InitialPayload, &initPayload); err != nil {
			return common.Options{}, nil, fmt.Errorf("failed to unmarshal initial payload: %w", err)
		}
		opts.InitPayload = initPayload
	}

	req := &common.Request{
		Query:         options.Body.Query,
		OperationName: options.Body.OperationName,
		Variables:     options.Body.Variables,
		Extensions:    options.Body.Extensions,
	}

	return opts, req, nil
}

// mapWSSubprotocol maps the string subprotocol to the common.WSSubprotocol type.
func mapWSSubprotocol(proto string) common.WSSubprotocol {
	switch proto {
	case ProtocolGraphQLWS:
		return common.SubprotocolGraphQLWS
	case ProtocolGraphQLTWS:
		return common.SubprotocolGraphQLTransportWS
	default:
		return common.SubprotocolAuto
	}
}

// formatSubscriptionError formats an error as a GraphQL error response.
func formatSubscriptionError(err error) []byte {
	errResponse := struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}{
		Errors: []struct {
			Message string `json:"message"`
		}{
			{Message: err.Error()},
		},
	}
	data, _ := json.Marshal(errResponse)
	return data
}

// GraphQLSubscriptionClientFactory abstracts the way of creating a new GraphQLSubscriptionClient.
// This can be very handy for testing purposes.
type GraphQLSubscriptionClientFactory interface {
	NewSubscriptionClient(ctx context.Context, options ...SubscriptionClientOption) GraphQLSubscriptionClient
}

type DefaultSubscriptionClientFactory struct{}

func (d *DefaultSubscriptionClientFactory) NewSubscriptionClient(ctx context.Context, options ...SubscriptionClientOption) GraphQLSubscriptionClient {
	return NewGraphQLSubscriptionClient(ctx, options...)
}

func IsDefaultGraphQLSubscriptionClient(client GraphQLSubscriptionClient) bool {
	_, ok := client.(*subscriptionClientV2)
	return ok
}
