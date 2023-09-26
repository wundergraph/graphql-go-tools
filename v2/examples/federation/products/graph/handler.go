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

	"github.com/wundergraph/graphql-go-tools/v2/examples/federation/products/graph/generated"
)

var websocketConnections atomic.Uint32

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

func GraphQLEndpointHandler(opts EndpointOptions) http.Handler {
	websocketConnections.Store(0)
	srv := handler.New(generated.NewExecutableSchema(generated.Config{Resolvers: &Resolver{}}))

	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		InitFunc: func(ctx context.Context, _ transport.InitPayload) (context.Context, error) {
			websocketConnections.Inc()
			go func(ctx context.Context) {
				<-ctx.Done()
				websocketConnections.Dec()
			}(ctx)
			return ctx, nil
		},
	})
	srv.Use(extension.Introspection{})

	if opts.EnableDebug {
		srv.Use(&debug.Tracer{})
	}

	randomnessEnabled = opts.EnableRandomness

	if opts.OverrideUpdateInterval > 0 {
		updateInterval = opts.OverrideUpdateInterval
	}

	return srv
}

func WebsocketConnectionsHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]uint32{
		"websocket_connections": websocketConnections.Load(),
	}

	responseBytes, err := json.Marshal(response)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("error"))
		return
	}

	_, _ = w.Write(responseBytes)
}
