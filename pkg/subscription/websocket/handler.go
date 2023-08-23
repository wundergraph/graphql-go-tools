package websocket

import (
	"context"
	"net"
	"time"

	"github.com/jensneuse/abstractlogger"

	"github.com/TykTechnologies/graphql-go-tools/pkg/subscription"
)

// Protocol defines the protocol names as type.
type Protocol string

const (
	ProtocolGraphQLWS Protocol = "graphql-ws"
)

var DefaultProtocol = ProtocolGraphQLWS

// HandleOptions can be used to pass options to the websocket handler.
type HandleOptions struct {
	Logger                           abstractlogger.Logger
	WebSocketInitFunc                InitFunc
	CustomClient                     subscription.TransportClient
	CustomKeepAliveInterval          time.Duration
	CustomSubscriptionUpdateInterval time.Duration
	CustomReadErrorTimeOut           time.Duration
	CustomSubscriptionEngine         subscription.Engine
}

// HandleOptionFunc can be used to define option functions.
type HandleOptionFunc func(opts *HandleOptions)

// WithLogger is a function that sets a logger for the websocket handler.
func WithLogger(logger abstractlogger.Logger) HandleOptionFunc {
	return func(opts *HandleOptions) {
		opts.Logger = logger
	}
}

// WithInitFunc is a function that sets the init function for the websocket handler.
func WithInitFunc(initFunc InitFunc) HandleOptionFunc {
	return func(opts *HandleOptions) {
		opts.WebSocketInitFunc = initFunc
	}
}

// WithCustomClient is a function that set a custom transport client for the websocket handler.
func WithCustomClient(client subscription.TransportClient) HandleOptionFunc {
	return func(opts *HandleOptions) {
		opts.CustomClient = client
	}
}

// WithCustomKeepAliveInterval is a function that sets a custom keep-alive interval for the websocket handler.
func WithCustomKeepAliveInterval(keepAliveInterval time.Duration) HandleOptionFunc {
	return func(opts *HandleOptions) {
		opts.CustomKeepAliveInterval = keepAliveInterval
	}
}

// WithCustomSubscriptionUpdateInterval is a function that sets a custom subscription update interval for the
// websocket handler.
func WithCustomSubscriptionUpdateInterval(subscriptionUpdateInterval time.Duration) HandleOptionFunc {
	return func(opts *HandleOptions) {
		opts.CustomSubscriptionUpdateInterval = subscriptionUpdateInterval
	}
}

// WithCustomReadErrorTimeOut is a function that sets a custom read error time out for the
// websocket handler.
func WithCustomReadErrorTimeOut(readErrorTimeOut time.Duration) HandleOptionFunc {
	return func(opts *HandleOptions) {
		opts.CustomReadErrorTimeOut = readErrorTimeOut
	}
}

// WithCustomSubscriptionEngine is a function that sets a custom subscription engine for the websocket handler.
func WithCustomSubscriptionEngine(subscriptionEngine subscription.Engine) HandleOptionFunc {
	return func(opts *HandleOptions) {
		opts.CustomSubscriptionEngine = subscriptionEngine
	}
}

// Handle will handle the websocket subscription. It can take optional option functions to customize the handler
// behavior.
func Handle(done chan bool, errChan chan error, conn net.Conn, executorPool subscription.ExecutorPool, options ...HandleOptionFunc) {
	definedOptions := HandleOptions{
		Logger: abstractlogger.Noop{},
	}

	for _, optionFunc := range options {
		optionFunc(&definedOptions)
	}

	HandleWithOptions(done, errChan, conn, executorPool, definedOptions)
}

// HandleWithOptions will handle the websocket connection. It requires an option struct to define the behavior.
func HandleWithOptions(done chan bool, errChan chan error, conn net.Conn, executorPool subscription.ExecutorPool, options HandleOptions) {
	// Use noop logger to prevent nil pointers if none was provided
	if options.Logger == nil {
		options.Logger = abstractlogger.Noop{}
	}

	defer func() {
		if err := conn.Close(); err != nil {
			options.Logger.Error("websocket.HandleWithOptions: on deferred closing connection",
				abstractlogger.String("message", "could not close connection to client"),
				abstractlogger.Error(err),
			)
		}
	}()

	var client subscription.TransportClient
	if options.CustomClient != nil {
		client = options.CustomClient
	} else {
		client = NewClient(options.Logger, conn)
	}

	protocolHandler, err := NewProtocolGraphQLWSHandlerWithOptions(client, ProtocolGraphQLWSHandlerOptions{
		Logger:                  options.Logger,
		WebSocketInitFunc:       options.WebSocketInitFunc,
		CustomKeepAliveInterval: options.CustomKeepAliveInterval,
	})
	if err != nil {
		options.Logger.Error("websocket.HandleWithOptions: on protocol handler creation",
			abstractlogger.String("message", "could not create protocol handler"),
			abstractlogger.String("protocol", string(DefaultProtocol)),
			abstractlogger.Error(err),
		)

		errChan <- err
		return
	}

	subscriptionHandler, err := subscription.NewUniversalProtocolHandlerWithOptions(client, protocolHandler, executorPool, subscription.UniversalProtocolHandlerOptions{
		Logger:                           options.Logger,
		CustomSubscriptionUpdateInterval: options.CustomSubscriptionUpdateInterval,
		CustomReadErrorTimeOut:           options.CustomReadErrorTimeOut,
		CustomEngine:                     options.CustomSubscriptionEngine,
	})
	if err != nil {
		options.Logger.Error("websocket.HandleWithOptions: on subscription handler creation",
			abstractlogger.String("message", "could not create subscription handler"),
			abstractlogger.String("protocol", string(DefaultProtocol)),
			abstractlogger.Error(err),
		)

		errChan <- err
		return
	}

	close(done)
	subscriptionHandler.Handle(context.Background()) // Blocking
}
