//go:generate go run github.com/99designs/gqlgen
package products

import (
	"net/http"

	"github.com/wundergraph/graphql-go-tools/execution/federationtesting/products/graph"
)

func Handler() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("/", graph.GraphQLEndpointHandler(graph.EndpointOptions{EnableDebug: true}))
	mux.HandleFunc("/websocket_connections", graph.WebsocketConnectionsHandler)

	return mux
}
