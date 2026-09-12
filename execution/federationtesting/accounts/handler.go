//go:generate go tool gqlgen generate
package accounts

import (
	"net/http"

	"github.com/wundergraph/graphql-go-tools/execution/federationtesting/accounts/graph"
)

func Handler() http.Handler {
	return graph.GraphQLEndpointHandler(graph.EndpointOptions{EnableDebug: true})
}
