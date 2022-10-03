package federationtesting

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"

	accounts "github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/accounts/graph"
	"github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/gateway"
	products "github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/products/graph"
	reviews "github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/reviews/graph"
)

func newFederationSetup() *federationSetup {
	accountUpstreamServer := httptest.NewServer(accounts.GraphQLEndpointHandler(accounts.TestOptions))
	productsUpstreamServer := httptest.NewServer(products.GraphQLEndpointHandler(products.TestOptions))
	reviewsUpstreamServer := httptest.NewServer(reviews.GraphQLEndpointHandler(reviews.TestOptions))

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

	gqlClient := NewGraphqlClient(http.DefaultClient)

	t.Run("single upstream query operation", func(t *testing.T) {
		resp := gqlClient.Query(ctx, setup.gatewayServer.URL, path.Join("testdata", "queries/single_upstream.query"), nil, t)
		assert.Equal(t, `{"data":{"me":{"id":"1234","username":"Me"}}}`, string(resp))
	})

	t.Run("query spans multiple federated servers", func(t *testing.T) {
		resp := gqlClient.Query(ctx, setup.gatewayServer.URL, path.Join("testdata", "queries/multiple_upstream.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","author":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","author":{"username":"Me"}}]},{"name":"Boater","reviews":[{"body":"This is the last straw. Hat you will wear. 11/10","author":{"username":"User 7777"}}]}]}}`, string(resp))
	})

	t.Run("mutation operation with variables", func(t *testing.T) {
		resp := gqlClient.Query(ctx, setup.gatewayServer.URL, path.Join("testdata", "mutations/mutation_with_variables.query"), queryVariables{
			"authorID": "3210",
			"upc":      "top-1",
			"review":   "This is the last straw. Hat you will wear. 11/10",
		}, t)
		assert.Equal(t, `{"data":{"addReview":{"body":"This is the last straw. Hat you will wear. 11/10","author":{"username":"User 3210"}}}}`, string(resp))
	})

	t.Run("union query", func(t *testing.T) {
		resp := gqlClient.Query(ctx, setup.gatewayServer.URL, path.Join("testdata", "queries/union.query"), nil, t)
		assert.Equal(t, `{"data":{"me":{"username":"Me","history":[{"__typename":"Purchase","wallet":{"amount":123}},{"__typename":"Sale","rating":5},{"__typename":"Purchase","wallet":{"amount":123}}]}}}`, string(resp))
	})

	t.Run("interface query", func(t *testing.T) {
		resp := gqlClient.Query(ctx, setup.gatewayServer.URL, path.Join("testdata", "queries/interface.query"), nil, t)
		assert.Equal(t, `{"data":{"me":{"username":"Me","history":[{"wallet":{"amount":123,"specialField1":"some special value 1"}},{"rating":5},{"wallet":{"amount":123,"specialField2":"some special value 2"}}]}}}`, string(resp))
	})

	t.Run("subscription query through WebSocket transport", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		wsAddr := strings.ReplaceAll(setup.gatewayServer.URL, "http://", "ws://")
		fmt.Println("setup.gatewayServer.URL", wsAddr)
		messages := gqlClient.Subscription(ctx, wsAddr, path.Join("testdata", "subscriptions/subscription.query"), queryVariables{
			"upc": "top-1",
		}, t)

		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-1","name":"Trilby","price":1}}}}`, string(<-messages))
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-1","name":"Trilby","price":2}}}}`, string(<-messages))
	})
}
