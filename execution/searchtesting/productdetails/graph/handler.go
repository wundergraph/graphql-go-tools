package graph

import (
	"net/http"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"

	"github.com/wundergraph/graphql-go-tools/execution/searchtesting/productdetails/graph/generated"
)

func GraphQLEndpointHandler() http.Handler {
	srv := handler.New(generated.NewExecutableSchema(generated.Config{Resolvers: &Resolver{}}))
	srv.AddTransport(transport.POST{})
	srv.Use(extension.Introspection{})
	return srv
}
