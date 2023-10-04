package websocket

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/subscription"
)

const (
	DefaultConnectionInitTimeOut = "15s"

	HeaderSecWebSocketProtocol = "Sec-WebSocket-Protocol"
)

// Protocol defines the protocol names as type.
type Protocol string

const (
	ProtocolUndefined          Protocol = ""
	ProtocolGraphQLWS          Protocol = "graphql-ws"
	ProtocolGraphQLTransportWS Protocol = "graphql-transport-ws"
)

var DefaultProtocol = ProtocolGraphQLTransportWS

// HandleOptions can be used to pass options to the websocket handler.
type HandleOptions struct {
	Logger                           abstractlogger.Logger
	Protocol                         Protocol
	WebSocketInitFunc                InitFunc
	CustomClient                     subscription.TransportClient
	CustomKeepAliveInterval          time.Duration
	CustomSubscriptionUpdateInterval time.Duration
	CustomConnectionInitTimeOut      time.Duration
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

// WithCustomConnectionInitTimeOut is a function that sets a custom connection init time out.
func WithCustomConnectionInitTimeOut(connectionInitTimeOut time.Duration) HandleOptionFunc {
	return func(opts *HandleOptions) {
		opts.CustomConnectionInitTimeOut = connectionInitTimeOut
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

// WithProtocol is a function that sets the protocol.
func WithProtocol(protocol Protocol) HandleOptionFunc {
	return func(opts *HandleOptions) {
		opts.Protocol = protocol
	}
}

// WithProtocolFromRequestHeaders is a function that sets the protocol based on the request headers.
// It fallbacks to the DefaultProtocol if the header can't be found, the value is invalid or no request
// was provided.
func WithProtocolFromRequestHeaders(req *http.Request) HandleOptionFunc {
	return func(opts *HandleOptions) {
		if req == nil {
			opts.Protocol = DefaultProtocol
			return
		}

		protocolHeaderValue := req.Header.Get(HeaderSecWebSocketProtocol)
		switch Protocol(protocolHeaderValue) {
		case ProtocolGraphQLWS:
			opts.Protocol = ProtocolGraphQLWS
		case ProtocolGraphQLTransportWS:
			opts.Protocol = ProtocolGraphQLTransportWS
		default:
			opts.Protocol = DefaultProtocol
		}
	}
}

// Handle will handle the websocket subscription. It can take optional option functions to customize the handler.
// behavior. By default, it uses the 'graphql-transport-ws' protocol.
func Handle(done chan bool, errChan chan error, conn net.Conn, executorPool subscription.ExecutorPool, options ...HandleOptionFunc) {
	definedOptions := HandleOptions{
		Logger:   abstractlogger.Noop{},
		Protocol: DefaultProtocol,
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

	protocolHandler, err := createProtocolHandler(options, client)
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

func createProtocolHandler(handleOptions HandleOptions, client subscription.TransportClient) (protocolHandler subscription.Protocol, err error) {
	protocol := handleOptions.Protocol
	if protocol == ProtocolUndefined {
		protocol = DefaultProtocol
	}

	switch protocol {
	case ProtocolGraphQLWS:
		protocolHandler, err = NewProtocolGraphQLWSHandlerWithOptions(client, ProtocolGraphQLWSHandlerOptions{
			Logger:                  handleOptions.Logger,
			WebSocketInitFunc:       handleOptions.WebSocketInitFunc,
			CustomKeepAliveInterval: handleOptions.CustomKeepAliveInterval,
		})
	default:
		protocolHandler, err = NewProtocolGraphQLTransportWSHandlerWithOptions(client, ProtocolGraphQLTransportWSHandlerOptions{
			Logger:                    handleOptions.Logger,
			WebSocketInitFunc:         handleOptions.WebSocketInitFunc,
			CustomKeepAliveInterval:   handleOptions.CustomKeepAliveInterval,
			CustomInitTimeOutDuration: handleOptions.CustomConnectionInitTimeOut,
		})
	}

	return protocolHandler, err
}
