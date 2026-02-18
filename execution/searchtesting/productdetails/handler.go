//go:generate go run github.com/99designs/gqlgen generate
package productdetails

import (
	"net/http"

	"github.com/wundergraph/graphql-go-tools/execution/searchtesting/productdetails/graph"
)

func Handler() http.Handler {
	return graph.GraphQLEndpointHandler()
}
