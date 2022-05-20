//go:generate go run -mod=mod github.com/99designs/gqlgen
package accounts

import (
	"net/http"

	"github.com/wundergraph/graphql-go-tools/pkg/graphql/federationtesting/accounts/graph"
)

func Handler() http.Handler {
	return graph.GraphQLEndpointHandler(graph.EndpointOptions{EnableDebug: true})
}
