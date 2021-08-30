package graph

import (
	"context"
	"net/http"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/debug"

	"github.com/jensneuse/graphql-go-tools/examples/federation/reviews/graph/generated"
	"github.com/jensneuse/graphql-go-tools/examples/federation/reviews/graph/model"
)

type EndpointOptions struct {
	EnableDebug bool
}

var TestOptions = EndpointOptions{
	EnableDebug: false,
}

func GraphQLEndpointHandler(opts EndpointOptions) http.Handler {
	cfg := generated.Config{Resolvers: &Resolver{}}
	cfg.Directives.HasRole = func(ctx context.Context, obj interface{}, next graphql.Resolver, role model.Role) (res interface{}, err error) {
		return next(ctx)
	}
	srv := handler.NewDefaultServer(generated.NewExecutableSchema(cfg))
	if opts.EnableDebug {
		srv.Use(&debug.Tracer{})
	}

	return srv
}
