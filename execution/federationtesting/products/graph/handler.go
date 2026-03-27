package graph

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/debug"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/gorilla/websocket"
	"go.uber.org/atomic"

	"github.com/wundergraph/graphql-go-tools/execution/federationtesting/products/graph/generated"
)

type EndpointOptions struct {
	EnableDebug            bool
	EnableRandomness       bool
	OverrideUpdateInterval time.Duration
}

var TestOptions = EndpointOptions{
	EnableDebug:            false,
	EnableRandomness:       false,
	OverrideUpdateInterval: 50 * time.Millisecond,
}

// Endpoint holds the GraphQL handler and its per-instance websocket connection counter.
type Endpoint struct {
	handler              http.Handler
	websocketConnections atomic.Uint32
}

// ServeHTTP delegates to the underlying gqlgen handler.
func (e *Endpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.handler.ServeHTTP(w, r)
}

// WebsocketConnectionsHandler returns an HTTP handler that reports the current
// websocket connection count for this endpoint instance.
func (e *Endpoint) WebsocketConnectionsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := map[string]uint32{
			"websocket_connections": e.websocketConnections.Load(),
		}

		responseBytes, err := json.Marshal(response)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("error"))
			return
		}

		_, _ = w.Write(responseBytes)
	}
}

func GraphQLEndpointHandler(opts EndpointOptions) *Endpoint {
	updateInterval := time.Second
	if opts.OverrideUpdateInterval > 0 {
		updateInterval = opts.OverrideUpdateInterval
	}

	resolver := &Resolver{
		products:          newProducts(),
		extraProducts:     newExtraProducts(),
		digitalProducts:   newDigitalProducts(),
		randomnessEnabled: opts.EnableRandomness,
		minPrice:          10,
		maxPrice:          1499,
		currentPrice:      10,
		updateInterval:    updateInterval,
	}

	endpoint := &Endpoint{}

	srv := handler.New(generated.NewExecutableSchema(generated.Config{Resolvers: resolver}))

	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		InitFunc: func(ctx context.Context, ip transport.InitPayload) (context.Context, *transport.InitPayload, error) {
			endpoint.websocketConnections.Inc()
			go func(ctx context.Context) {
				<-ctx.Done()
				endpoint.websocketConnections.Dec()
			}(ctx)
			return ctx, nil, nil
		},
	})
	srv.Use(extension.Introspection{})

	if opts.EnableDebug {
		srv.Use(&debug.Tracer{})
	}

	endpoint.handler = srv
	return endpoint
}
