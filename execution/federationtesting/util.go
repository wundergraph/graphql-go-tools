package federationtesting

import (
	"context"
	_ "embed"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	accounts "github.com/wundergraph/graphql-go-tools/execution/federationtesting/accounts/graph"
	products "github.com/wundergraph/graphql-go-tools/execution/federationtesting/products/graph"
	reviews "github.com/wundergraph/graphql-go-tools/execution/federationtesting/reviews/graph"
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

func NewFederationSetup(addGateway ...func(s *FederationSetup) *httptest.Server) *FederationSetup {
	return newFederationSetup(false, addGateway...)
}

func NewManualFederationSetup(addGateway ...func(s *FederationSetup) *httptest.Server) *FederationSetup {
	return newFederationSetup(true, addGateway...)
}

func newFederationSetup(enableManualSubscriptionEvents bool, addGateway ...func(s *FederationSetup) *httptest.Server) *FederationSetup {
	accountUpstreamServer := httptest.NewServer(accounts.GraphQLEndpointHandler(accounts.TestOptions))
	productOptions := products.TestOptions
	productOptions.EnableManualSubscriptionEvents = enableManualSubscriptionEvents
	productsEndpoint := products.GraphQLEndpointHandler(productOptions)
	productsUpstreamServer := httptest.NewServer(productsEndpoint)
	reviewsUpstreamServer := httptest.NewServer(reviews.GraphQLEndpointHandler(reviews.TestOptions))

	setup := &FederationSetup{
		AccountsUpstreamServer:     accountUpstreamServer,
		ProductsUpstreamServer:     productsUpstreamServer,
		ReviewsUpstreamServer:      reviewsUpstreamServer,
		productsSubscriptionEvents: productsEndpoint.SubscriptionEvents(),
	}

	if len(addGateway) > 0 {
		setup.GatewayServer = addGateway[0](setup)
	}

	return setup
}

type FederationSetup struct {
	AccountsUpstreamServer     *httptest.Server
	ProductsUpstreamServer     *httptest.Server
	ReviewsUpstreamServer      *httptest.Server
	GatewayServer              *httptest.Server
	productsSubscriptionEvents *products.ManualSubscriptionEventSource
}

func (f *FederationSetup) Close() {
	f.AccountsUpstreamServer.Close()
	f.ProductsUpstreamServer.Close()
	f.ReviewsUpstreamServer.Close()
	if f.GatewayServer != nil {
		f.GatewayServer.Close()
	}
}

func (f *FederationSetup) NextProductSubscription(ctx context.Context) (*products.ManualSubscriptionHandle, error) {
	if f.productsSubscriptionEvents == nil {
		return nil, fmt.Errorf("manual product subscriptions are not enabled for this setup")
	}

	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return f.productsSubscriptionEvents.NextSubscription(waitCtx)
}
