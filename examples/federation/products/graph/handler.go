package graph

import (
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/debug"

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
	srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: &Resolver{}}))
	if opts.EnableDebug {
		srv.Use(&debug.Tracer{})
	}

	randomnessEnabled = opts.EnableRandomness

	if opts.OverrideUpdateInterval > 0 {
		updateInterval = opts.OverrideUpdateInterval
	}

	return srv
}
