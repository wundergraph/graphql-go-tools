//go:build !race

package federationtesting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
	"time"

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

// This tests produces data races in the generated gql code. Disable it when the race
// detector is enabled.
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
		// Reset the products slice to the original state
		defer products.Reset()

		wsAddr := strings.ReplaceAll(setup.gatewayServer.URL, "http://", "ws://")
		fmt.Println("setup.gatewayServer.URL", wsAddr)
		messages := gqlClient.Subscription(ctx, wsAddr, path.Join("testdata", "subscriptions/subscription.query"), queryVariables{
			"upc": "top-1",
		}, t)

		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-1","name":"Trilby","price":1}}}}`, string(<-messages))
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-1","name":"Trilby","price":2}}}}`, string(<-messages))
	})

	t.Run("Multiple queries and nested fragments", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		resp := gqlClient.Query(ctx, setup.gatewayServer.URL, path.Join("testdata", "queries/multiple_queries_with_nested_fragments.query"), nil, t)
		expected := `
{
	"data": {
		"topProducts": [
			{
				"__typename": "Product",
				"price": 11,
				"upc": "top-1"
			},
			{
				"__typename": "Product",
				"price": 22,
				"upc": "top-2"
			},
			{
				"__typename": "Product",
				"price": 33,
				"upc": "top-3"
			}
		],
		"me": {
			"__typename": "User",
			"id": "1234",
			"username": "Me",
			"reviews": [
				{
					"__typename": "Review",
					"product": {
						"__typename": "Product",
						"price": 11,
						"upc": "top-1"
					}
				},
				{
					"__typename": "Review",
					"product": {
						"__typename": "Product",
						"price": 22,
						"upc": "top-2"
					}
				}
			]
		}
	}
}`
		assert.Equal(t, compact(expected), string(resp))
	})

	t.Run("Multiple queries with __typename", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		resp := gqlClient.Query(ctx, setup.gatewayServer.URL, path.Join("testdata", "queries/multiple_queries.query"), nil, t)
		expected := `
{
	"data": {
		"topProducts": [
			{
				"__typename": "Product",
				"price": 11,
				"upc": "top-1"
			},
			{
				"__typename": "Product",
				"price": 22,
				"upc": "top-2"
			},
			{
				"__typename": "Product",
				"price": 33,
				"upc": "top-3"
			}
		],
		"me": {
			"__typename": "User",
			"id": "1234",
			"username": "Me"
		}
	}
}`
		assert.Equal(t, compact(expected), string(resp))
	})

	t.Run("Query that returns union", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		resp := gqlClient.Query(ctx, setup.gatewayServer.URL, path.Join("testdata", "queries/multiple_queries_with_union_return.query"), nil, t)
		expected := `
{
	"data": {
		"me": {
			"__typename": "User",
			"id": "1234",
			"username": "Me"
		},
		"histories": [
			{
				"__typename": "Purchase",
				"product": {
					"__typename": "Product",
					"upc": "top-1"
				},
				"wallet": {
					"__typename": "WalletType1",
					"currency": "USD"
				}
			},
			{
				"__typename": "Sale",
				"product": {
					"__typename": "Product",
					"upc": "top-1"
				},
				"rating": 1
			},
			{
				"__typename": "Purchase",
				"product": {
					"__typename": "Product",
					"upc": "top-2"
				},
				"wallet": {
					"__typename": "WalletType2",
					"currency": "USD"
				}
			},
			{
				"__typename": "Sale",
				"product": {
					"__typename": "Product",
					"upc": "top-2"
				},
				"rating": 2
			},
			{
				"__typename": "Purchase",
				"product": {
					"__typename": "Product",
					"upc": "top-3"
				},
				"wallet": {
					"__typename": "WalletType2",
					"currency": "USD"
				}
			},
			{
				"__typename": "Sale",
				"product": {
					"__typename": "Product",
					"upc": "top-3"
				},
				"rating": 3
			}
		]
	}
}`
		assert.Equal(t, compact(expected), string(resp))
	})

	t.Run("Object response type with interface and object fragment", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		resp := gqlClient.Query(ctx, setup.gatewayServer.URL, path.Join("testdata", "queries/interface_fragment_on_object.graphql"), nil, t)
		expected := `
{
	"data": {
		"me": {
			"id": "1234",
			"username": "Me"
		}
	}
}`
		assert.Equal(t, compact(expected), string(resp))
	})

	t.Run("Interface response type with object fragment", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		resp := gqlClient.Query(ctx, setup.gatewayServer.URL, path.Join("testdata", "queries/object_fragment_on_interface.graphql"), nil, t)
		expected := `
{
	"data": {
		"identifiable": {
			"__typename": "User",
			"id": "1234",
			"username": "Me"
		}
	}
}`
		assert.Equal(t, compact(expected), string(resp))
	})

	t.Run("Union response type with interface fragments", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		resp := gqlClient.Query(ctx, setup.gatewayServer.URL, path.Join("testdata", "queries/interface_fragments_on_union.graphql"), nil, t)
		expected := `
{
	"data": {
		"histories": [
			{
				"__typename": "Purchase",
				"quantity": 1
			},
			{
				"__typename": "Sale",
				"location": "Germany"
			},
			{
				"__typename": "Purchase",
				"quantity": 2
			},
			{
				"__typename": "Sale",
				"location": "UK"
			},
			{
				"__typename": "Purchase",
				"quantity": 3
			},
			{
				"__typename": "Sale",
				"location": "Ukraine"
			}
		]
	}
}`
		assert.Equal(t, compact(expected), string(resp))
	})

	// This response data of this test returns location twice: from the interface selection and from the type fragment
	// Duplicated properties (and therefore invalid JSON) are usually removed during normalization processes.
	// It is not yet decided whether this should be addressed before these normalization processes.
	t.Run("Complex nesting", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		resp := gqlClient.Query(ctx, setup.gatewayServer.URL, path.Join("testdata", "queries/complex_nesting.graphql"), nil, t)
		expected := `
{
  "data": {
    "me": {
      "id": "1234",
      "username": "Me",
      "history": [
        {
          "wallet": {
            "currency": "USD"
          }
        },
        {
          "location": "Germany",
          "location": "Germany",
          "product": {
            "upc": "top-2",
            "name": "Fedora"
          }
        },
        {
          "wallet": {
            "currency": "USD"
          }
        }
      ],
      "reviews": [
        {
          "__typename": "Review",
          "attachments": [
            {
              "upc": "top-1",
              "body": "How do I turn it on?"
            }
          ]
        },
        {
          "__typename": "Review",
          "attachments": [
            {
              "upc": "top-2",
              "body": "The best hat I have ever bought in my life."
            },
            {
              "upc": "top-2",
              "size": 13.37
            }
          ]
        }
      ]
    }
  }
}
`
		assert.Equal(t, compact(expected), string(resp))
	})

	t.Run("Merged fields are still resolved", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		resp := gqlClient.Query(ctx, setup.gatewayServer.URL, path.Join("testdata", "queries/merged_field.graphql"), nil, t)
		expected := `
{
	"data": {
		"cat": {
			"name": "Pepper"
		},
		"me": {
			"id": "1234",
			"username": "Me",
			"realName": "User Usington",
			"reviews": [
				{
					"body": "A highly effective form of birth control."
				},
				{
					"body": "Fedoras are one of the most fashionable hats around and can look great with a variety of outfits."
				}
			],
			"history": [
				{
					"rating": 5
				}
			]
		}
	}
}`
		assert.Equal(t, compact(expected), string(resp))
	})
}

func compact(input string) string {
	var out bytes.Buffer
	err := json.Compact(&out, []byte(input))
	if err != nil {
		return ""
	}
	return out.String()
}
