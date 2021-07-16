package graph

import (
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/debug"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/gorilla/websocket"

	"github.com/jensneuse/graphql-go-tools/examples/federation/products/graph/generated"
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

func GraphQLEndpointHandler(opts EndpointOptions) http.Handler {
	srv := handler.New(generated.NewExecutableSchema(generated.Config{Resolvers: &Resolver{}}))

	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
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
