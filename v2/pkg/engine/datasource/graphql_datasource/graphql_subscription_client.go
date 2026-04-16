package graphql_datasource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/jensneuse/abstractlogger"

	client "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// SubscriptionClientConfig holds the subscription client configuration.
type SubscriptionClientConfig struct {
	UpgradeClient   *http.Client
	StreamingClient *http.Client
	Logger          abstractlogger.Logger

	// Timeouts and limits
	PingInterval time.Duration
	PingTimeout  time.Duration
	AckTimeout   time.Duration
	WriteTimeout time.Duration
	ReadLimit    int64

	// DefaultErrorExtensionCode is the extension code attached to GraphQL
	// errors produced by upstream connection failures. Should match the
	// resolve package's setting for consistent error formatting.
	DefaultErrorExtensionCode string
}

func defaultSubscriptionClientConfig() *SubscriptionClientConfig {
	return &SubscriptionClientConfig{
		UpgradeClient:   http.DefaultClient,
		StreamingClient: http.DefaultClient,
		Logger:          abstractlogger.NoopLogger,

		PingInterval: 30 * time.Second,
		PingTimeout:  10 * time.Second,
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
		if log != nil {
			cfg.Logger = log
		}
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

// WithWriteTimeout sets the timeout for WebSocket write operations (subscribe, unsubscribe, ping, pong).
// Default: 5s.
func WithWriteTimeout(d time.Duration) SubscriptionClientOption {
	return func(cfg *SubscriptionClientConfig) {
		cfg.WriteTimeout = d
	}
}

// WithDefaultErrorExtensionCode sets the extension code attached to GraphQL
// errors produced by upstream connection failures.
func WithDefaultErrorExtensionCode(code string) SubscriptionClientOption {
	return func(cfg *SubscriptionClientConfig) {
		cfg.DefaultErrorExtensionCode = code
	}
}

// WithReadLimit sets the maximum size in bytes for incoming WebSocket messages.
// Default: 1MB.
func WithReadLimit(n int64) SubscriptionClientOption {
	return func(cfg *SubscriptionClientConfig) {
		cfg.ReadLimit = n
	}
}

// subscriptionClientV2 implements GraphQLSubscriptionClient using the new
// channel-based subscription client.
type subscriptionClientV2 struct {
	client                    *client.Client
	defaultErrorExtensionCode string
}

// NewGraphQLSubscriptionClient creates a new subscription client.
func NewGraphQLSubscriptionClient(ctx context.Context, opts ...SubscriptionClientOption) GraphQLSubscriptionClient {
	cfg := defaultSubscriptionClientConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	return &subscriptionClientV2{
		defaultErrorExtensionCode: cfg.DefaultErrorExtensionCode,
		client: client.New(ctx, client.Config{
			UpgradeClient:   cfg.UpgradeClient,
			StreamingClient: cfg.StreamingClient,
			Logger:          cfg.Logger,
			PingInterval:    cfg.PingInterval,
			PingTimeout:     cfg.PingTimeout,
			AckTimeout:      cfg.AckTimeout,
			WriteTimeout:    cfg.WriteTimeout,
			ReadLimit:       cfg.ReadLimit,
		}),
	}
}

// Subscribe implements GraphQLSubscriptionClient.
func (c *subscriptionClientV2) Subscribe(ctx *resolve.Context, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error {
	opts, req, err := convertToClientOptions(options)
	if err != nil {
		return err
	}

	handler := buildMessageHandler(updater, c.defaultErrorExtensionCode)

	cancel, err := c.client.Subscribe(ctx.Context(), req, opts, handler)
	if err != nil {
		if isUpstreamError(err) {
			updater.Error(formatUpstreamServiceError(err, c.defaultErrorExtensionCode))
			updater.Done()
			return nil
		}
		return err
	}

	context.AfterFunc(ctx.Context(), func() {
		cancel()
		updater.Done()
	})

	return nil
}

// buildMessageHandler creates the handler that bridges client.Message → resolve.SubscriptionUpdater.
func buildMessageHandler(updater resolve.SubscriptionUpdater, errorCode string) client.Handler {
	return func(msg *client.Message) {
		switch msg.Type {
		case client.MessageTypeConnectionError:
			updater.Error(formatUpstreamServiceError(msg.Err, errorCode))
			updater.Done()
		case client.MessageTypeError:
			data, err := json.Marshal(msg.Payload)
			if err != nil {
				updater.Error(formatSubscriptionError(err))
				updater.Done()
				return
			}
			updater.Error(data)
			updater.Done()
		case client.MessageTypeData:
			data, err := json.Marshal(msg.Payload)
			if err != nil {
				updater.Error(formatSubscriptionError(err))
				updater.Done()
				return
			}
			updater.Update(data)
		case client.MessageTypeComplete:
			updater.Complete()
			updater.Done()
		}
	}
}

// isUpstreamError reports whether err is a connection-level upstream error
// that should be surfaced as a GraphQL error to the client.
// ErrFailedUpgrade and ErrInvalidSubprotocol are intentionally excluded so
// they propagate to the router, which formats detailed error messages
// (e.g. including the subgraph name and HTTP status code).
func isUpstreamError(err error) bool {
	return errors.Is(err, client.ErrConnectionClosed) ||
		errors.Is(err, client.ErrConnectionError) ||
		errors.Is(err, client.ErrInitFailed) ||
		errors.Is(err, client.ErrDialFailed) ||
		errors.Is(err, client.ErrAckTimeout) ||
		errors.Is(err, client.ErrAckNotReceived) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

// convertToClientOptions converts GraphQLSubscriptionOptions to the new client's types.
func convertToClientOptions(options GraphQLSubscriptionOptions) (client.Options, *client.Request, error) {
	opts := client.Options{
		Endpoint: options.URL,
		Headers:  options.Header,
	}

	// Transport selection
	if options.UseSSE {
		opts.Transport = client.TransportSSE
		if options.SSEMethodPost {
			opts.SSEMethod = client.SSEMethodPOST
		} else {
			opts.SSEMethod = client.SSEMethodGET
		}
	} else {
		opts.Transport = client.TransportWS
		opts.WSSubprotocol = mapWSSubprotocol(options.WsSubProtocol)
	}

	// Convert InitialPayload from json.RawMessage to map[string]any
	if len(options.InitialPayload) > 0 {
		var initPayload map[string]any
		if err := json.Unmarshal(options.InitialPayload, &initPayload); err != nil {
			return client.Options{}, nil, fmt.Errorf("failed to unmarshal initial payload: %w", err)
		}
		opts.InitPayload = initPayload
	}

	req := &client.Request{
		Query:         options.Body.Query,
		OperationName: options.Body.OperationName,
		Variables:     options.Body.Variables,
		Extensions:    options.Body.Extensions,
	}

	return opts, req, nil
}

// mapWSSubprotocol maps the string subprotocol to the client.WSSubprotocol type.
func mapWSSubprotocol(proto string) client.WSSubprotocol {
	switch proto {
	case string(client.SubprotocolGraphQLWS):
		return client.SubprotocolGraphQLWS
	case string(client.SubprotocolGraphQLTransportWS):
		return client.SubprotocolGraphQLTransportWS
	default:
		return client.SubprotocolAuto
	}
}

// formatUpstreamServiceError formats a connection-level error as a GraphQL error
// response with the configured error extension code. If the error chain contains
// a WebSocket close error, the close code and reason are included in extensions.
func formatUpstreamServiceError(err error, code string) []byte {
	type errorExtensions struct {
		Code      string `json:"code"`
		CloseCode int    `json:"closeCode,omitempty"`
		Reason    string `json:"closeReason,omitempty"`
	}

	type graphqlError struct {
		Message    string          `json:"message"`
		Extensions errorExtensions `json:"extensions"`
	}

	gqlErr := graphqlError{
		Message:    "upstream service error",
		Extensions: errorExtensions{Code: code},
	}

	var closeErr websocket.CloseError
	if errors.As(err, &closeErr) {
		gqlErr.Extensions.CloseCode = int(closeErr.Code)
		gqlErr.Extensions.Reason = closeErr.Reason
	}

	resp := struct {
		Errors []graphqlError `json:"errors"`
	}{
		Errors: []graphqlError{gqlErr},
	}
	data, _ := json.Marshal(resp)
	return data
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
