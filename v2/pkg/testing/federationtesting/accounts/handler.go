//go:generate go run -mod=mod github.com/99designs/gqlgen
package accounts

import (
	"net/http"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/federationtesting/accounts/graph"
)

func Handler() http.Handler {
	return graph.GraphQLEndpointHandler(graph.EndpointOptions{EnableDebug: true})
}
