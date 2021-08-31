package federationtesting

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jensneuse/abstractlogger"

	"github.com/jensneuse/graphql-go-tools/pkg/graphql/federationtesting/accounts"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql/federationtesting/gateway"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql/federationtesting/products"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql/federationtesting/reviews"
)

func newFederationSetup() *federationSetup {
	accountUpstreamServer := httptest.NewServer(accounts.Handler())
	productsUpstreamServer := httptest.NewServer(products.Handler())
	reviewsUpstreamServer := httptest.NewServer(reviews.Handler())

	httpClient := http.DefaultClient

	poller := gateway.NewDatasource([]gateway.ServiceConfig{
		{Name: "accounts", URL: accountUpstreamServer.URL},
		{Name: "products", URL: productsUpstreamServer.URL, WS: strings.ReplaceAll(productsUpstreamServer.URL, "http:", "ws:")},
		{Name: "reviews", URL: reviewsUpstreamServer.URL},
	}, httpClient)

	gtw := gateway.Handler(abstractlogger.NoopLogger, poller, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	poller.Run(ctx)

	gatewayServer := httptest.NewServer(gtw)

	return &federationSetup{
		accountsUpstreamServer: accountUpstreamServer,
		productsUpstreamServer: productsUpstreamServer,
		reviewsUpstreamServer:  reviewsUpstreamServer,
		gatewayServer:          gatewayServer,
	}
}

type federationSetup struct {
	accountsUpstreamServer *httptest.Server
	productsUpstreamServer *httptest.Server
	reviewsUpstreamServer  *httptest.Server
	gatewayServer          *httptest.Server
}

func (f *federationSetup) close() {
	f.accountsUpstreamServer.Close()
	f.productsUpstreamServer.Close()
	f.reviewsUpstreamServer.Close()
	f.gatewayServer.Close()
}

func TestFederationIntegrationTest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	setup := newFederationSetup()
	defer setup.close()

	t.Run("simple operation", func(t *testing.T) {

	})
}




