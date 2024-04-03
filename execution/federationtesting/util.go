package federationtesting

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"

	accounts "github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/accounts/graph"
	products "github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/products/graph"
	reviews "github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/reviews/graph"
)

const (
	federationTestingDirectoryRelativePath = "../federationtesting"

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
	SubscriptionUpdatedPrice = `subscription UpdatedPrice {
  updatedPrice {
    name
    price
  }
}`
)

type Upstream string

const (
	UpstreamAccounts Upstream = "accounts"
	UpstreamProducts Upstream = "products"
	UpstreamReviews  Upstream = "reviews"
)

func LoadTestingSubgraphSDL(upstream Upstream) ([]byte, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	absolutePath := filepath.Join(strings.Split(wd, "pkg")[0], federationTestingDirectoryRelativePath, string(upstream), "graph", "schema.graphqls")
	return os.ReadFile(absolutePath)
}

var lock sync.Mutex

func NewFederationSetup(addGateway ...func(s *FederationSetup) *httptest.Server) *FederationSetup {
	lock.Lock()

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
	lock.Unlock()
}
