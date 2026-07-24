package postprocess

import (
	"math"
	"math/rand"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestScheduleFetches_Scenarios(t *testing.T) {
	t.Parallel()
	type scenario struct {
		name      string
		input     []*resolve.FetchTreeNode
		wantError string
		want      *resolve.FetchTreeNode // the winner
		inlined   *resolve.FetchTreeNode // specify when it's not equal to winner
		waves     *resolve.FetchTreeNode // specify when it's not equal to winner
	}

	scenarios := []scenario{
		{
			name:  "independent components baseline",
			input: nodes(sf(0), sf(1), sf(2, deps(0))),
			want:  par(seq(sf(0), sf(2)), sf(1)),
		},
		{
			name:  "single chain",
			input: nodes(sf(0), sf(1, deps(0)), sf(2, deps(1)), sf(3, deps(2))),
			want:  seq(sf(0), sf(1), sf(2), sf(3)),
		},
		{
			name:  "diamond join",
			input: nodes(sf(0), sf(1, deps(0)), sf(2, deps(0)), sf(3, deps(1, 2))),
			want:  seq(sf(0), par(sf(1), sf(2)), sf(3)),
		},
		{
			name:  "two chains joining",
			input: nodes(sf(0), sf(1), sf(2, deps(0)), sf(3, deps(1)), sf(4, deps(2, 3))),
			want: seq(
				par(
					seq(sf(0), sf(2)),
					seq(sf(1), sf(3)),
				),
				sf(4),
			),
			waves: seq(
				par(sf(0), sf(1)),
				par(sf(2), sf(3)),
				sf(4),
			),
		},
		{
			name:  "wide fan out",
			input: nodes(sf(0), sf(1, deps(0)), sf(2, deps(0)), sf(3, deps(0)), sf(4, deps(0))),
			want:  seq(sf(0), par(sf(1), sf(2), sf(3), sf(4))),
		},
		{
			name:  "wide fan in",
			input: nodes(sf(0), sf(1), sf(2), sf(3), sf(4, deps(0, 1, 2, 3))),
			want:  seq(par(sf(0), sf(1), sf(2), sf(3)), sf(4)),
		},
		{
			name: "independent diamonds",
			input: nodes(
				sf(0), sf(1, deps(0)), sf(2, deps(0)), sf(3, deps(1, 2)),
				sf(4), sf(5, deps(4)), sf(6, deps(4)), sf(7, deps(5, 6)),
			),
			want: par(
				seq(sf(0), par(sf(1), sf(2)), sf(3)),
				seq(sf(4), par(sf(5), sf(6)), sf(7)),
			),
		},
		{
			//     / 1 > 2 \       / 6 > 7 \
			// 0 ->	        -> 5 ->         -> 10
			//     \ 3 > 4 /       \ 8 > 9 /
			name: "sequenced diamonds with chain arms",
			input: nodes(
				sf(0), sf(1, deps(0)), sf(2, deps(1)), sf(3, deps(0)), sf(4, deps(3)), sf(5, deps(2, 4)),
				sf(6, deps(5)), sf(7, deps(6)), sf(8, deps(5)), sf(9, deps(8)), sf(10, deps(7, 9)),
			),
			want: seq(
				sf(0),
				par(
					seq(sf(1), sf(2)),
					seq(sf(3), sf(4)),
				),
				sf(5),
				par(
					seq(sf(6), sf(7)),
					seq(sf(8), sf(9)),
				),
				sf(10),
			),
			waves: seq(
				sf(0),
				par(sf(1), sf(3)),
				par(sf(2), sf(4)),
				sf(5),
				par(sf(6), sf(8)),
				par(sf(7), sf(9)),
				sf(10),
			),
		},
		{
			name:  "requires chain",
			input: nodes(sf(0), sf(1, deps(0))),
			want:  seq(sf(0), sf(1)),
		},
		{
			name:  "batch entity component with independent root",
			input: nodes(sf(0), bf(1, 0), sf(2)),
			want:  par(seq(sf(0), bf(1)), sf(2)),
		},
		{
			name:  "nested entity chain",
			input: nodes(sf(0), ef(1, 0), ef(2, 1)),
			want:  seq(sf(0), ef(1), ef(2)),
		},
		{
			name:  "interface expansion",
			input: nodes(sf(0), sf(1), sf(2)),
			want:  par(sf(0), sf(1), sf(2)),
		},
		{
			name:  "provides skips fetch",
			input: nodes(sf(0)),
			want:  sf(0),
		},
		{
			name:  "sequential mutation",
			input: nodes(sf(0), sf(1, deps(0)), sf(2, deps(0, 1))),
			want:  seq(sf(0), sf(1), sf(2)),
		},
		{
			name:  "single fetch",
			input: nodes(sf(0)),
			want:  sf(0),
		},
		{
			name:      "cycle",
			input:     nodes(sf(0, deps(1)), sf(1, deps(0))),
			wantError: "cycle detected in fetch dependency graph",
		},
		{
			name:  "composite key fan in",
			input: nodes(sf(0), sf(1, deps(0)), sf(2, deps(0, 1))),
			want:  seq(sf(0), sf(1), sf(2)),
		},
		{
			name:  "asymmetric chain merge with leaf",
			input: nodes(sf(0), sf(1, deps(0)), sf(2, deps(0)), sf(3, deps(1, 2)), sf(4, deps(2))),
			want: seq(
				sf(0),
				par(sf(1), sf(2)),
				par(sf(3), sf(4)),
			),
			inlined: seq(
				sf(0),
				par(
					sf(1),
					seq(sf(2), sf(4)),
				),
				sf(3),
			),
		},
		{
			name:  "deep multi parent fan in",
			input: nodes(sf(0), sf(1), sf(2), sf(3, deps(0, 1, 2)), sf(4, deps(3)), sf(5, deps(4))),
			want:  seq(par(sf(0), sf(1), sf(2)), sf(3), sf(4), sf(5)),
		},
		{
			name:  "non inlined n shape",
			input: nodes(sf(0), sf(1), sf(2, deps(0)), sf(3, deps(0, 1)), sf(4, deps(1))),
			want: seq(
				par(sf(0), sf(1)),
				par(sf(2), sf(3), sf(4)),
			),
			inlined: seq(
				par(
					seq(sf(0), sf(2)),
					seq(sf(1), sf(4)),
				),
				sf(3),
			),
		},
		{
			name:  "independent root with shared join",
			input: nodes(sf(0), sf(1), sf(2, deps(0, 1)), sf(3)),
			want: par(
				seq(par(sf(0), sf(1)), sf(2)),
				sf(3),
			),
		},
		{
			name:  "independent leaf alongside chain",
			input: nodes(sf(0), sf(1, deps(0)), sf(2, deps(0, 1)), sf(3, deps(0))),
			want: seq(
				sf(0),
				par(
					seq(sf(1), sf(2)),
					sf(3),
				),
			),
		},
		{
			name:  "incomparable dominance fallback",
			input: nodes(sf(0), sf(1), sf(2, deps(1)), sf(3, deps(0, 2)), sf(4, deps(0, 1))),
			want: seq(
				par(sf(0), sf(1)),
				par(
					seq(sf(2), sf(3)),
					sf(4),
				),
			),
			inlined: seq(
				par(sf(0), seq(sf(1), sf(2))),
				par(sf(3), sf(4)),
			),
		},
		{
			name:  "chain off a shared join with a generation-skipping edge",
			input: nodes(sf(0), sf(1), sf(2, deps(0, 1)), sf(3, deps(2)), sf(4, deps(3, 0))),
			want: seq(
				par(sf(0), sf(1)),
				sf(2),
				sf(3),
				sf(4),
			),
		},
		{
			name: "weak components with different shapes have the mixed winner strategies",
			input: nodes(
				sf(0), sf(1), sf(2, deps(0)), sf(3, deps(0, 1)), sf(4, deps(1)),
				sf(5), sf(6), sf(7, deps(5)), sf(8, deps(6)), sf(9, deps(7, 8)),
			),
			// 2 weak components: the N component (0..4) keeps its waves tree,
			// the chains component (5..9) wins with its inlined tree.
			// The mixed winner dominates the all-waves tree.
			//   2  ->3  ->4       5->7\
			//   ^ /  ^ /               ->9
			//   |/   |/           6->8/
			//   0    1
			want: par(
				seq(
					par(sf(0), sf(1)),
					par(sf(2), sf(3), sf(4)),
				),
				seq(
					par(
						seq(sf(5), sf(7)),
						seq(sf(6), sf(8)),
					),
					sf(9),
				),
			),
			waves: par(
				seq(
					par(sf(0), sf(1)),
					par(sf(2), sf(3), sf(4)),
				),
				seq(
					par(sf(5), sf(6)),
					par(sf(7), sf(8)),
					sf(9),
				),
			),
			inlined: par(
				seq(
					par(
						seq(sf(0), sf(2)),
						seq(sf(1), sf(4)),
					),
					sf(3),
				),
				seq(
					par(
						seq(sf(5), sf(7)),
						seq(sf(6), sf(8)),
					),
					sf(9),
				),
			),
		},
		{
			name: "wide fan-out with deeply nested user entity chains",
			input: nodes(
				sf(0), sf(1, deps(0)), sf(3, deps(0)), sf(5, deps(0)), sf(7, deps(0)), sf(9, deps(0)),
				sf(10, deps(0)), sf(14, deps(0)), sf(15, deps(14)), sf(16, deps(14)), sf(17, deps(0)),
				sf(18, deps(17)), sf(19, deps(17)), sf(32, deps(0)), sf(33, deps(32)), sf(34, deps(32)),
				sf(35, deps(0)), sf(36, deps(35)), sf(37, deps(35)), sf(44, deps(0)), sf(45, deps(44)),
				sf(46, deps(44)), sf(47, deps(0)), sf(48, deps(47)), sf(49, deps(47)), sf(56, deps(0)),
				sf(57, deps(56)), sf(58, deps(56)), sf(59, deps(0)), sf(60, deps(59)), sf(61, deps(59)),
				sf(62, deps(0)), sf(63, deps(62)), sf(64, deps(62)), sf(68, deps(0)), sf(69, deps(68)),
				sf(82, deps(68)), sf(83, deps(82)), sf(84, deps(82)), sf(85, deps(68)), sf(86, deps(85)),
				sf(87, deps(85)),
			),
			want: seq(
				sf(0),
				par(
					sf(1),
					sf(3),
					sf(5),
					sf(7),
					sf(9),
					sf(10),
					seq(sf(14), par(sf(15), sf(16))),
					seq(sf(17), par(sf(18), sf(19))),
					seq(sf(32), par(sf(33), sf(34))),
					seq(sf(35), par(sf(36), sf(37))),
					seq(sf(44), par(sf(45), sf(46))),
					seq(sf(47), par(sf(48), sf(49))),
					seq(sf(56), par(sf(57), sf(58))),
					seq(sf(59), par(sf(60), sf(61))),
					seq(sf(62), par(sf(63), sf(64))),
					seq(
						sf(68),
						par(
							sf(69),
							seq(
								sf(82),
								par(sf(83), sf(84))),
							seq(
								sf(85),
								par(sf(86), sf(87))),
						),
					),
				),
			),
			// The legacy wave pipeline:
			//   seq(
			//     0,
			//     par(1, 3, 5, 7, 9, 10, 14, 17, 32, 35, 44, 47, 56, 59, 62, 68),
			//     par(15, 16, 18, 19, 33, 34, 36, 37, 45, 46, 48, 49, 57, 58, 60, 61, 63, 64, 69, 82, 85),
			//     par(83, 84, 86, 87),
			//   )
			// so fetch 83 waits on all 21 second-wave fetches instead of just 0, 68, 69, 82.
		},
		{
			name: "mixed depth entity chains",
			input: nodes(
				sf(0), sf(1, deps(0)), sf(3, deps(0)), sf(5, deps(0)), sf(7, deps(0)), sf(9, deps(0)),
				sf(10, deps(0)), sf(14, deps(0)), sf(15, deps(14)), sf(16, deps(14)), sf(17, deps(0)),
				sf(18, deps(17)), sf(29, deps(0)), sf(30, deps(29)), sf(31, deps(29)), sf(32, deps(0)),
				sf(33, deps(32)), sf(39, deps(0)), sf(40, deps(39)), sf(41, deps(39)), sf(42, deps(0)),
				sf(43, deps(42)), sf(49, deps(0)), sf(50, deps(49)), sf(51, deps(49)), sf(52, deps(0)),
				sf(53, deps(52)), sf(54, deps(0)), sf(55, deps(54)), sf(59, deps(0)), sf(71, deps(59)),
				sf(72, deps(71)), sf(73, deps(71)), sf(74, deps(59)), sf(75, deps(74)),
			),
			want: seq(
				sf(0),
				par(
					sf(1),
					sf(3),
					sf(5),
					sf(7),
					sf(9),
					sf(10),
					seq(sf(14),
						par(sf(15),
							sf(16))),
					seq(sf(17), sf(18)),
					seq(sf(29), par(sf(30), sf(31))),
					seq(sf(32), sf(33)),
					seq(sf(39), par(sf(40), sf(41))),
					seq(sf(42), sf(43)),
					seq(sf(49), par(sf(50), sf(51))),
					seq(sf(52), sf(53)),
					seq(sf(54), sf(55)),
					seq(
						sf(59),
						par(
							seq(
								sf(71),
								par(sf(72), sf(73)),
							),
							seq(sf(74), sf(75)),
						),
					),
				),
			),
			// The legacy wave pipeline:
			//   seq(
			//     0,
			//     par(1, 3, 5, 7, 9, 10, 14, 17, 29, 32, 39, 42, 49, 52, 54, 59),
			//     par(15, 16, 18, 30, 31, 33, 40, 41, 43, 50, 51, 53, 55, 71, 74),
			//     par(72, 73, 75),
			//   )
			// so fetch 72 waits on all 15 second-wave fetches instead of just 0, 59, 71.
		},
		{
			name: "inlining wins on exclusive chain beside a late join",
			input: nodes(
				sf(0),
				sf(1, deps(0)),
				sf(2, deps(0)),
				sf(3, deps(2)),
				sf(4, deps(2)),
				sf(5, deps(0, 1, 2, 3, 4)),
				sf(6, deps(0, 5)),
			),
			want: seq(
				sf(0),
				par(
					sf(1),
					seq(
						sf(2),
						par(sf(3), sf(4)),
					),
				),
				sf(5),
				sf(6),
			),
			waves: seq(
				sf(0),
				par(sf(1), sf(2)),
				par(sf(3), sf(4)),
				sf(5),
				sf(6),
			),
			// The legacy wave pipeline:
			//   seq(
			//     0,
			//     par(1, 2),
			//     par(3, 4),
			//     5,
			//     6,
			//   )
		},
		{
			name: "inlining wins on independent chains gathered by ending joins",
			input: nodes(
				sf(0),
				sf(1, deps(0)), sf(2, deps(0)), sf(3, deps(0)), sf(4, deps(0)),
				sf(5, deps(4)), sf(6, deps(4)), sf(7, deps(4)),
				sf(12, deps(0)),
				sf(15, deps(2)),
				sf(16, deps(15)), sf(17, deps(15)),
				sf(18, deps(0)),
				sf(19, deps(18)), sf(20, deps(18)),
				sf(21, deps(4)),
				sf(24, deps(15)),
				sf(25, deps(18)),
				sf(26, deps(0, 2, 3, 5, 6, 12, 15, 16, 17, 18, 19, 20, 21, 24, 25)),
				sf(27, deps(0, 4, 4, 5, 5, 6, 6, 7, 7, 21, 21)),
				sf(28, deps(0, 1, 18, 26, 27)),
			),
			want: seq(
				sf(0),
				par(
					sf(1),
					seq(
						sf(2),
						sf(15),
						par(sf(16), sf(17), sf(24)),
					),
					sf(3),
					seq(
						sf(4),
						par(sf(5), sf(6), sf(7), sf(21)),
						sf(27),
					),
					sf(12),
					seq(
						sf(18),
						par(sf(19), sf(20), sf(25)),
					),
				),
				sf(26),
				sf(28),
			),
			waves: seq(
				sf(0),
				par(
					sf(1), sf(2), sf(3), sf(4), sf(12), sf(18),
				),
				par(
					sf(5), sf(6), sf(7), sf(15), sf(19), sf(20), sf(21), sf(25),
				),
				par(
					sf(16), sf(17), sf(24), sf(27),
				),
				sf(26),
				sf(28),
			),
			// The legacy wave pipeline:
			//   seq(
			//     0,
			//     par(1, 2, 3, 4, 12, 18),
			//     par(5, 6, 7, 15, 19, 20, 21, 25),
			//     par(16, 17, 24, 27),
			//     26,
			//     28,
			//   )
		},
	}

	for i, tc := range scenarios {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dag, err := newFetchDAG(tc.input)
			require.NoError(t, err)
			ids := dag.sortedIDs()

			actualInlined, inlinedErr := schedule(ids, dag, true)
			actualWaves, wavesErr := schedule(ids, dag, false)
			actualWinner, winnerErr := buildScheduleTree(tc.input, dag)
			if tc.wantError != "" {
				require.EqualError(t, inlinedErr, tc.wantError)
				require.EqualError(t, wavesErr, tc.wantError)
				require.EqualError(t, winnerErr, tc.wantError)
				return
			}

			require.NoError(t, inlinedErr)
			require.NoError(t, wavesErr)
			require.NoError(t, winnerErr)
			require.NoError(t, validateSchedule(actualInlined, dag))
			require.NoError(t, validateSchedule(actualWaves, dag))
			require.NoError(t, validateSchedule(actualWinner, dag))

			byID := fetchesByID(tc.input)
			want := materialize(t, tc.want, byID)
			expectedInlined := materialize(t, tc.inlined, byID)
			expectedWaves := materialize(t, tc.waves, byID)
			if expectedInlined == nil {
				expectedInlined = want
			}
			if expectedWaves == nil {
				expectedWaves = want
			}
			requireEqualTrees(t, expectedInlined, actualInlined)
			requireEqualTrees(t, expectedWaves, actualWaves)
			requireEqualTrees(t, want, actualWinner)

			// Beyond the pinned shapes, the trees must satisfy the scheduler
			// invariants under randomized duration profiles.
			assertScheduleProperties(t, actualInlined, actualWaves, actualWinner, durationProfiles(tc.input, 50, int64(9000+i)))
		})
	}
}

func TestDisableScheduleFetches_OptionWiring(t *testing.T) {
	t.Parallel()
	input := func() *resolve.FetchTreeNode {
		return seq(
			sf(0),
			sf(1),
			sf(2, deps(0)),
			sf(3, deps(1)),
			sf(4, deps(2, 3)))
	}
	wantWaves := seq(
		par(sf(0), sf(1)),
		par(sf(2, deps(0)),
			sf(3, deps(1))),
		sf(4, deps(2, 3)))
	wantScheduled := seq(
		par(seq(sf(0), sf(2, deps(0))),
			seq(sf(1), sf(3, deps(1)))),
		sf(4, deps(2, 3)))

	scheduled := input()
	NewProcessor().fetchTreeProcessors.organizeFetchTree(scheduled)
	requireEqualTrees(t, wantScheduled, scheduled)

	waves := input()
	NewProcessor(DisableScheduleFetches()).fetchTreeProcessors.organizeFetchTree(waves)
	requireEqualTrees(t, wantWaves, waves)
}

func TestScheduleFetches_SubscriptionRootStaysSequence(t *testing.T) {
	t.Parallel()
	// The plan printer renders the Subscription Primary/Rest wrapper only for Sequence roots:
	// the scheduled tree must not collapse into the root when it carries a Trigger.
	trigger := &resolve.FetchTreeNode{
		Kind: resolve.FetchTreeNodeKindTrigger,
		Item: &resolve.FetchItem{Fetch: &resolve.SingleFetch{}},
	}

	single := seq(sf(0))
	single.Trigger = trigger
	(&scheduleFetches{}).ProcessFetchTree(single)
	require.Equal(t, resolve.FetchTreeNodeKindSequence, single.Kind)
	require.Equal(t, trigger, single.Trigger)
	require.Equal(t, nodes(sf(0)), single.ChildNodes)

	parallel := seq(sf(0), sf(1))
	parallel.Trigger = trigger
	(&scheduleFetches{}).ProcessFetchTree(parallel)
	require.Equal(t, resolve.FetchTreeNodeKindSequence, parallel.Kind)
	require.Equal(t, trigger, parallel.Trigger)
	require.Equal(t, nodes(par(sf(0), sf(1))), parallel.ChildNodes)

	// Without a Trigger the root may collapse into the scheduled tree.
	sync := seq(sf(0))
	(&scheduleFetches{}).ProcessFetchTree(sync)
	require.Equal(t, sf(0), sync)
}

func TestScheduleFetches_Validator(t *testing.T) {
	t.Run("response-path nesting without a FetchID edge is valid", func(t *testing.T) {
		y := sf(0, at("user"))
		x := sf(1, at("user.details"), provides("user.details"))
		tree := par(x, y)
		dag, err := newFetchDAG(nodes(x, y))
		require.NoError(t, err)
		require.NoError(t, validateSchedule(tree, dag))
	})

	t.Run("explicit FetchID edge between parallel siblings is invalid", func(t *testing.T) {
		y := sf(0)
		x := sf(1, deps(0))
		tree := par(x, y)
		dag, err := newFetchDAG(nodes(x, y))
		require.NoError(t, err)
		require.EqualError(t, validateSchedule(tree, dag), "fetch 1 is scheduled before its dependency 0 completes")
	})

	t.Run("self-dependency is invalid", func(t *testing.T) {
		y := sf(0)
		x := sf(1, deps(1))
		_, err := newFetchDAG(nodes(x, y))
		require.EqualError(t, err, "self-dependent id 1")
	})

	// Conservation: a schedule must never lose or duplicate a fetch. The
	// property tests lean on these rejections for their completeness checks.
	t.Run("schedule missing a fetch is invalid", func(t *testing.T) {
		input := nodes(sf(0), sf(1, deps(0)), sf(2, deps(0)))
		dag, err := newFetchDAG(input)
		require.NoError(t, err)
		missing := seq(input[0], input[1])
		require.EqualError(t, validateSchedule(missing, dag), "fetch 2 missing from schedule")
	})

	t.Run("schedule duplicating a fetch is invalid", func(t *testing.T) {
		input := nodes(sf(0), sf(1, deps(0)), sf(2, deps(0)))
		dag, err := newFetchDAG(input)
		require.NoError(t, err)
		duplicated := seq(input[0], par(input[1], input[2]), input[1])
		require.EqualError(t, validateSchedule(duplicated, dag), "fetch 1 scheduled 2 times")
	})
}

func TestScheduleFetches_ProcessorFallsBackOnError(t *testing.T) {
	t.Parallel()
	// on any scheduler/validator error the processor degrades to the LEGACY wave pipeline.
	build := func() *resolve.FetchTreeNode {
		return seq(sf(7), sf(7, deps(7)))
	}

	legacy := build()
	(&orderSequenceByDependencies{}).ProcessFetchTree(legacy)
	(&createParallelNodes{}).ProcessFetchTree(legacy)

	scheduled := build()
	require.NotPanics(t, func() {
		(&scheduleFetches{}).ProcessFetchTree(scheduled)
	})
	require.Equal(t, legacy, scheduled)
}

// TestScheduleFetches_BigPlan pins the schedule for a realistic 13-fetch plan
func TestScheduleFetches_BigPlan(t *testing.T) {
	// Fetch IDs and order preserve the plan-generator's emission order.
	input := func() []*resolve.FetchTreeNode {
		return nodes(
			sf(0),
			sf(5),
			sf(1, at("users"), deps(0)),
			sf(6, at("topProducts"), deps(5)),
			sf(11, at("topProducts"), deps(5)),
			sf(2, at("users.@.reviews.@.product"), deps(1)),
			sf(3, at("users.@.reviews.@.product.reviews.@.author"), deps(1)),
			sf(4, at("users.@.reviews.@.product.reviews.@.author.reviews.@.product"), deps(1)),
			sf(7, at("topProducts.@.reviews.@.author"), deps(6)),
			sf(8, at("topProducts.@.reviews.@.author.reviews.@.product"), deps(6)),
			sf(9, at("users.@.reviews.@.product"), deps(1, 2)),
			sf(10, at("users.@.reviews.@.product.reviews.@.author.reviews.@.product"), deps(1, 4)),
			sf(12, at("topProducts.@.reviews.@.author.reviews.@.product"), deps(6, 8)),
		)
	}

	expected := materialize(t, par(
		seq(
			sf(0),
			sf(1),
			par(
				seq(sf(2), sf(9)),
				sf(3),
				seq(sf(4), sf(10)),
			),
		),
		seq(
			sf(5),
			par(
				seq(
					sf(6),
					par(
						sf(7),
						seq(sf(8), sf(12)),
					),
				),
				sf(11),
			),
		),
	), fetchesByID(input()))

	t.Run("scheduler produces the tree without errors", func(t *testing.T) {
		fetches := input()
		dag, err := newFetchDAG(fetches)
		require.NoError(t, err)
		tree, err := buildScheduleTree(fetches, dag)
		require.NoError(t, err)
		require.NoError(t, validateSchedule(tree, dag))
		requireEqualTrees(t, expected, tree)
	})

	t.Run("scheduler does not fall back to legacy waves", func(t *testing.T) {
		root := seq(input()...)
		(&scheduleFetches{}).ProcessFetchTree(root)
		requireEqualTrees(t, expected, root)
	})
}

func (d *fetchDAG) sortedIDs() []int {
	ids := make([]int, 0, len(d.nodes))
	for id := range d.nodes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// hasCycle runs Kahn's algorithm over the full DAG.
func (d *fetchDAG) hasCycle() bool {
	indegree := make(map[int]int, len(d.nodes))
	queue := make([]int, 0, len(d.nodes))
	for id, parents := range d.parents {
		indegree[id] = len(parents)
		if len(parents) == 0 {
			queue = append(queue, id)
		}
	}
	scheduled := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		scheduled++
		for child := range d.children[id] {
			if indegree[child]--; indegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}
	return scheduled != len(d.nodes)
}

func TestScheduleFetchesPropertiesRandom(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(42))
	for n := 3; n <= 15; n++ {
		for i := range 50 {
			input := randomDAG(n, 1.5, rng)
			checkScheduleProperties(t, input, durationProfiles(input, 50, int64(n*1000+i)))
		}
	}
}

func TestScheduleFetchesPropertiesSmoke(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(99))
	for _, n := range []int{50, 500} {
		input := randomDAG(n, 1.5, rng)
		checkScheduleProperties(t, input, durationProfiles(input, 50, int64(n)))
	}
}

func checkScheduleProperties(t *testing.T, input []*resolve.FetchTreeNode, profiles []map[int]int) {
	t.Helper()
	dag, err := newFetchDAG(input)
	require.NoError(t, err)
	ids := dag.sortedIDs()
	inlined, inlinedErr := schedule(ids, dag, true)
	waves, wavesErr := schedule(ids, dag, false)
	winner, winnerErr := buildScheduleTree(input, dag)
	if len(ids) > 0 && dag.hasCycle() {
		require.EqualError(t, inlinedErr, "cycle detected in fetch dependency graph")
		require.EqualError(t, wavesErr, "cycle detected in fetch dependency graph")
		require.EqualError(t, winnerErr, "cycle detected in fetch dependency graph")
		return
	}
	require.NoErrorf(t, inlinedErr, "input=%v", dependencyList(input))
	require.NoErrorf(t, wavesErr, "input=%v", dependencyList(input))
	require.NoErrorf(t, winnerErr, "input=%v", dependencyList(input))
	// validateSchedule also enforces conservation: every fetch must be scheduled exactly once:
	require.NoError(t, validateSchedule(inlined, dag))
	require.NoError(t, validateSchedule(waves, dag))
	require.NoError(t, validateSchedule(winner, dag))

	assertScheduleProperties(t, inlined, waves, winner, profiles)
}

// assertScheduleProperties checks the invariants relating the three trees:
// the winner always dominates the waves tree,
// when the inlined tree dominates the waves tree, the winner is exactly the inlined tree.
func assertScheduleProperties(t *testing.T, inlined, waves, winner *resolve.FetchTreeNode, profiles []map[int]int) {
	t.Helper()
	require.True(t, dominates(winner, waves))
	if dominates(inlined, waves) {
		require.Equal(t, inlined, winner)
	}
	require.LessOrEqual(t, uniformMakespan(winner), uniformMakespan(waves))
	for _, durations := range profiles {
		require.LessOrEqual(t, weightedMakespan(winner, durations), weightedMakespan(waves, durations))
	}
}

func dependencyList(input []*resolve.FetchTreeNode) []resolve.FetchDependencies {
	out := make([]resolve.FetchDependencies, 0, len(input))
	for _, node := range input {
		out = append(out, *node.Item.Fetch.Dependencies())
	}
	return out
}

func randomDAG(n int, averageDegree float64, rng *rand.Rand) []*resolve.FetchTreeNode {
	depLists := make([][]int, n)
	p := averageDegree / math.Max(1, float64(n-1))
	for from := range n {
		for to := from + 1; to < n; to++ {
			if rng.Float64() < p {
				depLists[to] = append(depLists[to], from)
			}
		}
	}
	out := make([]*resolve.FetchTreeNode, n)
	for i := range out {
		out[i] = sf(i, deps(depLists[i]...))
	}
	return out
}

func durationProfiles(input []*resolve.FetchTreeNode, count int, seed int64) []map[int]int {
	rng := rand.New(rand.NewSource(seed))
	profiles := make([]map[int]int, count)
	for i := range profiles {
		profile := make(map[int]int, len(input))
		for _, node := range input {
			id := node.Item.Fetch.Dependencies().FetchID
			profile[id] = max(int(math.Round(math.Exp(rng.Float64()*math.Log(1000)))), 1)
		}
		profiles[i] = profile
	}
	return profiles
}

// uniformMakespan is the tree makespan with every fetch cost 1.
func uniformMakespan(node *resolve.FetchTreeNode) int {
	durations := map[int]int{}
	for id := range treePredecessors(node) {
		durations[id] = 1
	}
	return weightedMakespan(node, durations)
}

// weightedMakespan is the critical-path weight of the tree under durations:
// sequences add, parallels take the maximum.
func weightedMakespan(node *resolve.FetchTreeNode, durations map[int]int) int {
	if node == nil {
		return 0
	}
	switch node.Kind {
	case resolve.FetchTreeNodeKindSingle:
		return durations[node.Item.Fetch.Dependencies().FetchID]
	case resolve.FetchTreeNodeKindSequence:
		sum := 0
		for _, child := range node.ChildNodes {
			sum += weightedMakespan(child, durations)
		}
		return sum
	case resolve.FetchTreeNodeKindParallel:
		maxSpan := 0
		for _, child := range node.ChildNodes {
			if m := weightedMakespan(child, durations); m > maxSpan {
				maxSpan = m
			}
		}
		return maxSpan
	default:
		return 0
	}
}
