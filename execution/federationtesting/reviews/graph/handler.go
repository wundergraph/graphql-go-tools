package graph

import (
	"net/http"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/debug"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"

	"github.com/wundergraph/graphql-go-tools/execution/federationtesting/reviews/graph/generated"
)

type EndpointOptions struct {
	EnableDebug bool
}

var TestOptions = EndpointOptions{
	EnableDebug: false,
}

func GraphQLEndpointHandler(opts EndpointOptions) http.Handler {
	resolver := &Resolver{
		reviews:     newReviews(),
		attachments: newAttachments(),
	}
	srv := handler.New(generated.NewExecutableSchema(generated.Config{Resolvers: resolver}))
	srv.AddTransport(transport.POST{})
	srv.Use(extension.Introspection{})
	if opts.EnableDebug {
		srv.Use(&debug.Tracer{})
	}

	return srv
}
