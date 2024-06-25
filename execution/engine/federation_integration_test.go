//go:build !race

package engine_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jensneuse/abstractlogger"
	"github.com/sebdah/goldie/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/federationtesting"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting/gateway"
	products "github.com/wundergraph/graphql-go-tools/execution/federationtesting/products/graph"
)

func addGateway(enableART bool) func(setup *federationtesting.FederationSetup) *httptest.Server {
	return func(setup *federationtesting.FederationSetup) *httptest.Server {
		httpClient := http.DefaultClient

		poller := gateway.NewDatasource([]gateway.ServiceConfig{
			{Name: "accounts", URL: setup.AccountsUpstreamServer.URL},
			{Name: "products", URL: setup.ProductsUpstreamServer.URL, WS: strings.ReplaceAll(setup.ProductsUpstreamServer.URL, "http:", "ws:")},
			{Name: "reviews", URL: setup.ReviewsUpstreamServer.URL},
		}, httpClient)

		gtw := gateway.Handler(abstractlogger.NoopLogger, poller, httpClient, enableART)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		poller.Run(ctx)
		return httptest.NewServer(gtw)
	}
}

func testQueryPath(name string) string {
	return path.Join("..", "federationtesting", "testdata", name)
}

func TestFederationIntegrationTestWithArt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	setup := federationtesting.NewFederationSetup(addGateway(true))
	defer setup.Close()

	gqlClient := NewGraphqlClient(http.DefaultClient)

	normalizeResponse := func(resp string) string {
		rex, err := regexp.Compile(`http://127.0.0.1:\d+`)
		require.NoError(t, err)
		resp = rex.ReplaceAllString(resp, "http://localhost/graphql")
		return resp
	}

	t.Run("single upstream query operation with ART", func(t *testing.T) {
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/complex_nesting.graphql"), nil, t)
		respString := normalizeResponse(string(resp))

		assert.Contains(t, respString, `{"data":{"me":{"id":"1234","username":"Me"`)
		assert.Contains(t, respString, `"extensions":{"trace":{"info":{"trace_start_time"`)

		buf := &bytes.Buffer{}
		_ = json.Indent(buf, []byte(respString), "", "  ")
		goldie.New(t, goldie.WithNameSuffix(".json")).Assert(t, "complex_nesting_query_with_art", buf.Bytes())
	})
}

// This tests produces data races in the generated gql code. Disable it when the race
// detector is enabled.
func TestFederationIntegrationTest(t *testing.T) {

	t.Run("single upstream query operation", func(t *testing.T) {
		setup := federationtesting.NewFederationSetup(addGateway(false))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/single_upstream.query"), nil, t)
		assert.Equal(t, `{"data":{"me":{"id":"1234","username":"Me"}}}`, string(resp))
	})

	t.Run("query spans multiple federated servers", func(t *testing.T) {
		setup := federationtesting.NewFederationSetup(addGateway(false))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/multiple_upstream.query"), nil, t)
		assert.Equal(t, `{"data":{"topProducts":[{"name":"Trilby","reviews":[{"body":"A highly effective form of birth control.","author":{"username":"Me"}}]},{"name":"Fedora","reviews":[{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","author":{"username":"Me"}}]}]}}`, string(resp))
	})

	t.Run("mutation operation with variables", func(t *testing.T) {
		setup := federationtesting.NewFederationSetup(addGateway(false))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("mutations/mutation_with_variables.query"), queryVariables{
			"authorID": "3210",
			"upc":      "top-1",
			"review":   "This is the last straw. Hat you will wear. 11/10",
		}, t)
		assert.Equal(t, `{"data":{"addReview":{"body":"This is the last straw. Hat you will wear. 11/10","author":{"username":"User 3210"}}}}`, string(resp))
	})

	t.Run("union query", func(t *testing.T) {
		setup := federationtesting.NewFederationSetup(addGateway(false))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/union.query"), nil, t)
		assert.Equal(t, `{"data":{"me":{"username":"Me","history":[{"__typename":"Purchase","wallet":{"amount":123}},{"__typename":"Sale","rating":5},{"__typename":"Purchase","wallet":{"amount":123}}]}}}`, string(resp))
	})

	t.Run("interface query", func(t *testing.T) {
		setup := federationtesting.NewFederationSetup(addGateway(false))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/interface.query"), nil, t)
		assert.Equal(t, `{"data":{"me":{"username":"Me","history":[{"wallet":{"amount":123,"specialField1":"some special value 1"}},{"rating":5},{"wallet":{"amount":123,"specialField2":"some special value 2"}}]}}}`, string(resp))
	})

	t.Run("subscription query through WebSocket transport", func(t *testing.T) {
		setup := federationtesting.NewFederationSetup(addGateway(false))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		// Reset the products slice to the original state
		defer products.Reset()

		wsAddr := strings.ReplaceAll(setup.GatewayServer.URL, "http://", "ws://")
		// fmt.Println("setup.GatewayServer.URL", wsAddr)
		messages := gqlClient.Subscription(ctx, wsAddr, testQueryPath("subscriptions/subscription.query"), queryVariables{
			"upc": "top-1",
		}, t)

		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-1","name":"Trilby","price":1}}}}`, string(<-messages))
		assert.Equal(t, `{"id":"1","type":"data","payload":{"data":{"updateProductPrice":{"upc":"top-1","name":"Trilby","price":2}}}}`, string(<-messages))
	})

	t.Run("Multiple queries and nested fragments", func(t *testing.T) {
		setup := federationtesting.NewFederationSetup(addGateway(false))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/multiple_queries_with_nested_fragments.query"), nil, t)
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
		setup := federationtesting.NewFederationSetup(addGateway(false))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/multiple_queries.query"), nil, t)
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
		setup := federationtesting.NewFederationSetup(addGateway(false))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/multiple_queries_with_union_return.query"), nil, t)
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
		setup := federationtesting.NewFederationSetup(addGateway(false))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/interface_fragment_on_object.graphql"), nil, t)
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
		setup := federationtesting.NewFederationSetup(addGateway(false))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/object_fragment_on_interface.graphql"), nil, t)
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
		setup := federationtesting.NewFederationSetup(addGateway(false))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/interface_fragments_on_union.graphql"), nil, t)
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
		setup := federationtesting.NewFederationSetup(addGateway(false))
		defer setup.Close()

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/complex_nesting.graphql"), nil, t)
		expected := `{"data":{"me":{"id":"1234","username":"Me","history":[{"wallet":{"currency":"USD"}},{"location":"Germany","product":{"upc":"top-2","name":"Fedora"}},{"wallet":{"currency":"USD"}}],"reviews":[{"__typename":"Review","attachments":[{"__typename":"Question","upc":"top-1","body":"How do I turn it on?"}]},{"__typename":"Review","attachments":[{"__typename":"Rating","upc":"top-2","body":"The best hat I have ever bought in my life."},{"__typename":"Video","upc":"top-2","size":13.37}]}]}}}`
		assert.Equal(t, compact(expected), string(resp))
	})

	t.Run("More complex nesting", func(t *testing.T) {
		setup := federationtesting.NewFederationSetup(addGateway(false))
		defer setup.Close()

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/more_complex_nesting.graphql"), nil, t)
		expected := `{"data":{"me":{"id":"1234","username":"Me","history":[{"wallet":{"currency":"USD"}},{"location":"Germany","product":{"name":"Fedora","upc":"top-2"}},{"wallet":{"currency":"USD"}}],"reviews":[{"__typename":"Review","attachments":[{"__typename":"Question","upc":"top-1","body":"How do I turn it on?"}],"comment":{"__typename":"Question","subject":"Life"}},{"__typename":"Review","attachments":[{"__typename":"Rating","upc":"top-2","body":"The best hat I have ever bought in my life."},{"__typename":"Video","upc":"top-2","size":13.37}],"comment":{"__typename":"Question","subject":"Life"}}]}}}`
		assert.Equal(t, compact(expected), string(resp))
	})

	t.Run("Multiple nested interfaces", func(t *testing.T) {
		setup := federationtesting.NewFederationSetup(addGateway(false))
		defer setup.Close()

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/titlename.graphql"), nil, t)
		expected := `{"data":{"titleName":{"__typename":"TitleName","title":"Title","a":"A","b":"B","name":"Name","c":"C"}}}`
		assert.Equal(t, compact(expected), string(resp))
	})

	t.Run("Multiple nested unions", func(t *testing.T) {
		setup := federationtesting.NewFederationSetup(addGateway(false))
		defer setup.Close()

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/cd.graphql"), nil, t)
		expected := `{"data":{"cds":[{"name":{"first":"Leonie","middle":"Johanna"}},{"name":{"first":"Jannik","last":"Neuse"}}]}}`
		assert.Equal(t, compact(expected), string(resp))
	})

	t.Run("More complex nesting typename variant", func(t *testing.T) {
		setup := federationtesting.NewFederationSetup(addGateway(false))
		defer setup.Close()

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/more_complex_nesting_typename_variant.graphql"), nil, t)
		expected := `{"data":{"me":{"id":"1234","username":"Me","history":[{"wallet":{"currency":"USD"}},{"location":"Germany","product":{"name":"Fedora","upc":"top-2"}},{"wallet":{"currency":"USD"}}],"reviews":[{"__typename":"Review","attachments":[{"__typename":"Question","upc":"top-1","body":"How do I turn it on?"}]},{"__typename":"Review","attachments":[{"__typename":"Rating","upc":"top-2","body":"The best hat I have ever bought in my life."},{"__typename":"Video","upc":"top-2","size":13.37}]}]}}}`
		assert.Equal(t, compact(expected), string(resp))
	})

	t.Run("Abstract object", func(t *testing.T) {
		setup := federationtesting.NewFederationSetup(addGateway(false))
		defer setup.Close()

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/abstract_object.graphql"), nil, t)
		expected := `{"data":{"abstractList":[{"__typename":"ConcreteListItem1","obj":{"__typename":"SomeType1","name":"1","age":1}},{"__typename":"ConcreteListItem2","obj":{"__typename":"SomeType2","name":"2","height":2}}]}}`
		assert.Equal(t, compact(expected), string(resp))
	})

	t.Run("Abstract object mixed", func(t *testing.T) {
		setup := federationtesting.NewFederationSetup(addGateway(false))
		defer setup.Close()

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/abstract_object_mixed.graphql"), nil, t)
		expected := `{"data":{"abstractList":[{},{"obj":{"names":["3","4"],"__typename":"SomeType2","height":2}}]}}`
		assert.Equal(t, compact(expected), string(resp))
	})

	t.Run("Abstract interface field", func(t *testing.T) {
		setup := federationtesting.NewFederationSetup(addGateway(false))
		defer setup.Close()

		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/ab.graphql"), nil, t)
		expected := `{"data":{"interfaceUnion":{"__typename":"A","name":"A"}}}`
		assert.Equal(t, compact(expected), string(resp))
	})

	t.Run("Merged fields are still resolved", func(t *testing.T) {
		setup := federationtesting.NewFederationSetup(addGateway(false))
		t.Cleanup(setup.Close)
		gqlClient := NewGraphqlClient(http.DefaultClient)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		resp := gqlClient.Query(ctx, setup.GatewayServer.URL, testQueryPath("queries/merged_field.graphql"), nil, t)
		expected := `{"data":{"cat":{"name":"Pepper"},"me":{"id":"1234","username":"Me","realName":"User Usington","reviews":[{"body":"A highly effective form of birth control."},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits."}],"history":[{},{"rating":5},{}]}}}`
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
