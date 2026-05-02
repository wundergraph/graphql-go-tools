package federationtesting

import (
	_ "embed"
	"net/http/httptest"

	accounts "github.com/wundergraph/graphql-go-tools/execution/federationtesting/accounts/graph"
	products "github.com/wundergraph/graphql-go-tools/execution/federationtesting/products/graph"
	reviews "github.com/wundergraph/graphql-go-tools/execution/federationtesting/reviews/graph"
)

const (
	QueryReviewsOfMe = `query ReviewsOfMe {
  me {
    reviews {
      body
      product {
        upc
        name
        price
      }
    }
  }
}`
)

var (
	//go:embed config.json
	RouterConfigJson []byte
	//go:embed accounts/graph/schema.graphqls
	AccountSDL []byte
	//go:embed products/graph/schema.graphqls
	ProductsSDL []byte
	//go:embed reviews/graph/schema.graphqls
	ReviewsSDL []byte
)

func NewFederationSetup(addGateway ...func(s *FederationSetup) *httptest.Server) *FederationSetup {
	accountUpstreamServer := httptest.NewServer(accounts.GraphQLEndpointHandler(accounts.TestOptions))
	productsUpstreamServer := httptest.NewServer(products.GraphQLEndpointHandler(products.TestOptions))
	reviewsUpstreamServer := httptest.NewServer(reviews.GraphQLEndpointHandler(reviews.TestOptions))

	setup := &FederationSetup{
		AccountsUpstreamServer: accountUpstreamServer,
		ProductsUpstreamServer: productsUpstreamServer,
		ReviewsUpstreamServer:  reviewsUpstreamServer,
	}

	if len(addGateway) > 0 {
		setup.GatewayServer = addGateway[0](setup)
	}

	return setup
}

type FederationSetup struct {
	AccountsUpstreamServer *httptest.Server
	ProductsUpstreamServer *httptest.Server
	ReviewsUpstreamServer  *httptest.Server
	GatewayServer          *httptest.Server
}

func (f *FederationSetup) Close() {
	f.AccountsUpstreamServer.Close()
	f.ProductsUpstreamServer.Close()
	f.ReviewsUpstreamServer.Close()
	if f.GatewayServer != nil {
		f.GatewayServer.Close()
	}
}
