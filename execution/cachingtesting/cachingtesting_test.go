package cachingtesting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
)

// TestNoOpBaseline is the Phase 0 exit proof (appendix row A1 shape): with
// caching unconfigured, the resolved response is byte-identical whether or not
// a runtime controller is set, and the controller is never consulted.
func TestNoOpBaseline(t *testing.T) {
	query := `{ products(first: 2) { upc name } }`
	responses := map[string]string{
		"products": `{"data":{"products":[{"__typename":"Product","upc":"1","name":"Table"},{"__typename":"Product","upc":"2","name":"Chair"}]}}`,
	}
	expected := `{"data":{"products":[{"upc":"1","name":"Table"},{"upc":"2","name":"Chair"}]}}`

	baseline := Plan(t, query, nil, responses)
	baselineBody := ResolveResponse(t, baseline.Response, nil)
	assert.Equal(t, expected, baselineBody)

	controller := cachetesting.NewRecordingController(nil)
	withController := Plan(t, query, nil, responses)
	withControllerBody := ResolveResponse(t, withController.Response, controller)
	assert.Equal(t, expected, withControllerBody)
	assert.Equal(t, baselineBody, withControllerBody)
	assert.Equal(t, int64(0), controller.Begins())
	assert.Empty(t, controller.Calls())
}

// TestFixtureSmoke plans and resolves one representative operation per fixture
// shape through the harness with canned responses, asserting the COMPLETE
// response body.
func TestFixtureSmoke(t *testing.T) {
	t.Run("multi-key entity via second key set", func(t *testing.T) {
		result := Plan(t, `{ productBySku(sku: "S1") { upc sku name price } }`, nil, map[string]string{
			"products": `{"data":{"productBySku":{"__typename":"Product","upc":"1","sku":"S1","name":"Table","price":100}}}`,
		})
		body := ResolveResponse(t, result.Response, nil)
		assert.Equal(t, `{"data":{"productBySku":{"upc":"1","sku":"S1","name":"Table","price":100}}}`, body)
	})

	t.Run("by-key root field", func(t *testing.T) {
		result := Plan(t, `{ product(upc: "1") { name } }`, nil, map[string]string{
			"products": `{"data":{"product":{"__typename":"Product","name":"Table"}}}`,
		})
		body := ResolveResponse(t, result.Response, nil)
		assert.Equal(t, `{"data":{"product":{"name":"Table"}}}`, body)
	})

	t.Run("nested cross-subgraph entities", func(t *testing.T) {
		result := Plan(t, `{ me { username favoriteProduct { name stock warehouse { location } } } }`, nil, map[string]string{
			"users":                        `{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`,
			"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1","name":"Table"}}]}}`,
			"inventory:me.favoriteProduct": `{"data":{"_entities":[{"__typename":"Product","stock":5,"warehouse":{"location":"Berlin"}}]}}`,
		})
		body := ResolveResponse(t, result.Response, nil)
		assert.Equal(t, `{"data":{"me":{"username":"jens","favoriteProduct":{"name":"Table","stock":5,"warehouse":{"location":"Berlin"}}}}}`, body)
	})

	t.Run("batch entity reviews", func(t *testing.T) {
		result := Plan(t, `{ products(first: 2) { upc reviews { body } } }`, nil, map[string]string{
			"products": `{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"}]}}`,
			"reviews":  `{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]},{"__typename":"Product","reviews":[{"body":"Wobbly"}]}]}}`,
		})
		body := ResolveResponse(t, result.Response, nil)
		assert.Equal(t, `{"data":{"products":[{"upc":"1","reviews":[{"body":"Solid"}]},{"upc":"2","reviews":[{"body":"Wobbly"}]}]}}`, body)
	})

	t.Run("sibling root fields on one datasource", func(t *testing.T) {
		result := Plan(t, `{ products(first: 1) { upc } promotions { upc } }`, nil, map[string]string{
			"products": `{"data":{"products":[{"__typename":"Product","upc":"1"}],"promotions":[{"__typename":"Product","upc":"9"}]}}`,
		})
		body := ResolveResponse(t, result.Response, nil)
		assert.Equal(t, `{"data":{"products":[{"upc":"1"}],"promotions":[{"upc":"9"}]}}`, body)
	})

	t.Run("mixed ttl siblings across subgraphs", func(t *testing.T) {
		result := Plan(t, `{ product(upc: "1") { name stock } }`, nil, map[string]string{
			"products":          `{"data":{"product":{"__typename":"Product","name":"Table","upc":"1"}}}`,
			"inventory:product": `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`,
		})
		body := ResolveResponse(t, result.Response, nil)
		assert.Equal(t, `{"data":{"product":{"name":"Table","stock":5}}}`, body)
	})
}

// TestDeferSupersetShape closes the first-pass fixture gap: the initial
// request's inventory entity fetch provides a STRICT SUPERSET (stock +
// warehouse) of the deferred inventory entity fetch in a later group (stock
// only), so cross-defer-group L1 serving is provable (task 18 rows N1/N2/M3).
func TestDeferSupersetShape(t *testing.T) {
	query := `
		query {
			me { favoriteProduct { upc stock warehouse { id location } } }
			products(first: 1) {
				upc
				... @defer { stock }
			}
		}`
	result := Plan(t, query, nil, nil)
	require.NotNil(t, result.DeferResponse, "expected a defer plan")

	groups := DeferGroups(result.DeferResponse)
	require.NotEmpty(t, groups, "expected at least one defer group")

	rendered := make([]string, 0, len(groups))
	for _, group := range groups {
		require.NotNil(t, group.Fetches)
		rendered = append(rendered, group.Fetches.QueryPlan().PrettyPrint())
	}
	// The deferred group holds ONLY the inventory (service "1") entity fetch
	// selecting stock — a strict subset of the initial inventory fetch's
	// {stock warehouse{id location}} for the same entity type.
	assert.Equal(t, []string{`QueryPlan {
  Fetch(service: "1") {
    {
      fragment Key on Product {
          __typename
          upc
      }
    } =>
    {
        _entities(representations: $representations){
            ... on Product {
                __typename
                stock
            }
        }
    }
  }
}
`}, rendered)
}
