//go:generate go tool gqlgen generate
package reviews

import (
	"net/http"

	"github.com/wundergraph/graphql-go-tools/execution/federationtesting/reviews/graph"
)

func Handler() http.Handler {
	return graph.GraphQLEndpointHandler(graph.EndpointOptions{EnableDebug: true})
}
