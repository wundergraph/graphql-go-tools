package cachingtesting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// enableP1 is the minimal caching configuration that makes the P1 walk run;
// the provider content is irrelevant to ProvidesData itself.
func enableP1() map[string]cacheconfig.CachingConfiguration {
	return map[string]cacheconfig.CachingConfiguration{"products": {}}
}

// TestProvidesDataFidelity is the fidelity gate over REAL federation plans
// (real fieldPlanners from the main walk, not synthetic attribution): the full
// per-fetch ProvidesData side-table is pinned for a root + batch-entity plan.
// This is a planner-level test that pins plan-internal state (the ProvidesData
// resolve.Object trees) not visible in a client response.
func TestProvidesDataFidelity(t *testing.T) {
	result := Plan(t, `{ products(first: 2) { upc name reviews { body } } }`, enableP1(), nil)
	pd := result.Response.CacheProvidesData()
	require.Len(t, pd, 2)

	byDS := make(map[string]*resolve.Object, len(pd))
	for info, obj := range pd {
		require.NotContains(t, byDS, info.DataSourceID)
		byDS[info.DataSourceID] = obj
	}

	assert.Equal(t, map[string]*resolve.Object{
		// products root fetch: the full selection it returns, list-shaped.
		"0": {
			Fields: []*resolve.Field{
				{
					Name: []byte("products"),
					Value: &resolve.Array{
						Nullable: false,
						Path:     []string{"products"},
						Item: &resolve.Object{
							Nullable: false,
							Fields: []*resolve.Field{
								{
									Name: []byte("upc"),
									Value: &resolve.Scalar{
										Nullable: false,
										Path:     []string{"upc"},
									},
								},
								{
									Name: []byte("name"),
									Value: &resolve.Scalar{
										Nullable: false,
										Path:     []string{"name"},
									},
								},
								{
									Name: []byte("__typename"),
									Value: &resolve.Scalar{
										Nullable: false,
										Path:     []string{"__typename"},
									},
								},
							},
						},
					},
				},
			},
		},
		// reviews batch entity fetch: the entity-boundary reset makes its tree
		// start at the Product entity, with the entity type condition.
		"2": {
			Fields: []*resolve.Field{
				{
					Name: []byte("reviews"),
					Value: &resolve.Array{
						Nullable: false,
						Path:     []string{"reviews"},
						Item: &resolve.Object{
							Nullable: false,
							Fields: []*resolve.Field{
								{
									Name: []byte("body"),
									Value: &resolve.Scalar{
										Nullable: false,
										Path:     []string{"body"},
									},
								},
							},
						},
					},
					OnTypeNames: [][]byte{[]byte("Product")},
				},
			},
		},
	}, byDS)
}

// TestProvidesDataDeferredFetchOwnTree pins that a DEFERRED fetch gets its own
// side-table entry (keyed per *FetchInfo, shared across the initial response
// and all defer groups): the deferred inventory fetch's tree is exactly
// {stock}, independent of the initial inventory fetch's larger tree. This is a
// planner-level test that pins plan-internal state (the deferred fetch's
// ProvidesData tree keyed by its *FetchInfo) not visible in a client response.
func TestProvidesDataDeferredFetchOwnTree(t *testing.T) {
	query := `
		query {
			me { favoriteProduct { upc stock warehouse { id location } } }
			products(first: 1) {
				upc
				... @defer { stock }
			}
		}`
	result := Plan(t, query, enableP1(), nil)
	require.NotNil(t, result.DeferResponse)
	pd := result.Response.CacheProvidesData()
	require.NotEmpty(t, pd)

	groups := DeferGroups(result.DeferResponse)
	require.Len(t, groups, 1)
	deferredInfo := firstFetchInfo(t, groups[0].Fetches)
	require.NotNil(t, deferredInfo)

	assert.Equal(t, &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name: []byte("stock"),
				Value: &resolve.Scalar{
					Nullable: false,
					Path:     []string{"stock"},
				},
				OnTypeNames: [][]byte{[]byte("Product")},
			},
		},
	}, pd[deferredInfo])
}

// firstFetchInfo returns the FetchInfo of the first fetch item in the tree.
func firstFetchInfo(t *testing.T, node *resolve.FetchTreeNode) *resolve.FetchInfo {
	t.Helper()
	if node == nil {
		return nil
	}
	if node.Item != nil && node.Item.Fetch != nil {
		return node.Item.Fetch.FetchInfo()
	}
	for _, child := range node.ChildNodes {
		if info := firstFetchInfo(t, child); info != nil {
			return info
		}
	}
	return nil
}
