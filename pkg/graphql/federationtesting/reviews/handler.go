//go:generate go run -mod=mod github.com/99designs/gqlgen
package reviews

import (
	"net/http"

	"github.com/jensneuse/graphql-go-tools/pkg/graphql/federationtesting/reviews/graph"
)

func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/query", graph.GraphQLEndpointHandler(graph.EndpointOptions{EnableDebug: true}))

	return mux
}
