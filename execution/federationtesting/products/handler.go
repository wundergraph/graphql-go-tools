//go:generate go run github.com/99designs/gqlgen generate
package products

import (
	"net/http"

	"github.com/wundergraph/graphql-go-tools/execution/federationtesting/products/graph"
)

func Handler() http.Handler {
	mux := http.NewServeMux()

	endpoint := graph.GraphQLEndpointHandler(graph.EndpointOptions{EnableDebug: true})
	mux.Handle("/", endpoint)
	mux.HandleFunc("/websocket_connections", endpoint.WebsocketConnectionsHandler())

	return mux
}
