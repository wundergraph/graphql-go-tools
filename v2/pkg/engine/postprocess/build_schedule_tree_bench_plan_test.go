package postprocess

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func sfDeps(id int, responsePath string, deps ...int) *resolve.FetchTreeNode {
	return resolve.SingleWithPath(&resolve.SingleFetch{
		FetchDependencies: resolve.FetchDependencies{FetchID: id, DependsOnFetchIDs: deps},
	}, responsePath)
}

// TestBuildScheduleTreeRealGatewaysBenchmarkPlan pins the schedule for the
// REAL hive gateways-benchmark plan (13 fetches, 4 legacy waves), extracted
// verbatim from cosmo's plan-generator on the bench supergraph. Upstream's
// path-containment validator heuristics rejected this plan (fetch 3 nests
// under fetch 2's provided path without depending on it; fetches 6 and 11
// provide the same path) and the upstream processor PANICKED at plan time —
// a 500 for every benchmark request. With the fork's FetchID-edge-only
// validator the scheduler produces the eager-inline SP schedule below: the
// users chain and the topProducts chain are fully decoupled (no cross-branch
// barriers), which is exactly the skew win the scheduler exists to capture.
func TestBuildScheduleTreeRealGatewaysBenchmarkPlan(t *testing.T) {
	build := func() *resolve.FetchTreeNode {
		return resolve.Sequence(
			sfDeps(0, ""),
			sfDeps(5, ""),
			sfDeps(1, "users", 0),
			sfDeps(6, "topProducts", 5),
			sfDeps(11, "topProducts", 5),
			sfDeps(2, "users.@.reviews.@.product", 1),
			sfDeps(3, "users.@.reviews.@.product.reviews.@.author", 1),
			sfDeps(4, "users.@.reviews.@.product.reviews.@.author.reviews.@.product", 1),
			sfDeps(7, "topProducts.@.reviews.@.author", 6),
			sfDeps(8, "topProducts.@.reviews.@.author.reviews.@.product", 6),
			sfDeps(9, "users.@.reviews.@.product", 1, 2),
			sfDeps(10, "users.@.reviews.@.product.reviews.@.author.reviews.@.product", 1, 4),
			sfDeps(12, "topProducts.@.reviews.@.author.reviews.@.product", 6, 8),
		)
	}

	root := build()
	dag, err := newFetchDAG(root.ChildNodes)
	require.NoError(t, err)
	tree, err := buildScheduleTree(root.ChildNodes, dag)
	require.NoError(t, err)
	require.NoError(t, validateSchedule(tree, dag))

	expected := par(
		seq(
			sfDeps(0, ""),
			sfDeps(1, "users", 0),
			par(
				seq(
					sfDeps(2, "users.@.reviews.@.product", 1),
					sfDeps(9, "users.@.reviews.@.product", 1, 2),
				),
				sfDeps(3, "users.@.reviews.@.product.reviews.@.author", 1),
				seq(
					sfDeps(4, "users.@.reviews.@.product.reviews.@.author.reviews.@.product", 1),
					sfDeps(10, "users.@.reviews.@.product.reviews.@.author.reviews.@.product", 1, 4),
				),
			),
		),
		seq(
			sfDeps(5, ""),
			par(
				seq(
					sfDeps(6, "topProducts", 5),
					par(
						sfDeps(7, "topProducts.@.reviews.@.author", 6),
						seq(
							sfDeps(8, "topProducts.@.reviews.@.author.reviews.@.product", 6),
							sfDeps(12, "topProducts.@.reviews.@.author.reviews.@.product", 6, 8),
						),
					),
				),
				sfDeps(11, "topProducts", 5),
			),
		),
	)
	require.Equal(t, expected, tree)

	// End to end through the processor: must take the schedule path (the
	// schedule differs from the legacy wave tree), not the error fallback.
	processed := build()
	(&buildScheduleTreeProcessor{}).ProcessFetchTree(processed)
	require.Equal(t, expected, processed)
}

// TestBuildScheduleTreeProcessorFallsBackOnError pins the fork's no-panic
// contract: on any scheduler/validator error (duplicate FetchIDs here) the
// processor degrades to the LEGACY wave pipeline — byte-identical planning to
// the flag-off path — instead of panicking like upstream.
func TestBuildScheduleTreeProcessorFallsBackOnError(t *testing.T) {
	build := func() *resolve.FetchTreeNode {
		return seq(sf(7), sf(7, 7))
	}

	legacy := build()
	(&orderSequenceByDependencies{}).ProcessFetchTree(legacy)
	(&createParallelNodes{}).ProcessFetchTree(legacy)

	scheduled := build()
	require.NotPanics(t, func() {
		(&buildScheduleTreeProcessor{}).ProcessFetchTree(scheduled)
	})
	require.Equal(t, legacy, scheduled)
}
