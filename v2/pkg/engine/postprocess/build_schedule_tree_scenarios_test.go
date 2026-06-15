package postprocess

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestBuildScheduleTreeScenarios(t *testing.T) {
	type scenario struct {
		name        string
		input       []*resolve.FetchTreeNode
		sp          *resolve.FetchTreeNode
		level       *resolve.FetchTreeNode
		hybrid      *resolve.FetchTreeNode
		expectError string
	}

	scenarios := []scenario{
		{
			name:   "1 independent components baseline",
			input:  nodes(sf(0), sf(1), sf(2, 0)),
			sp:     par(seq(sf(0), sf(2, 0)), sf(1)),
			level:  par(seq(sf(0), sf(2, 0)), sf(1)),
			hybrid: par(seq(sf(0), sf(2, 0)), sf(1)),
		},
		{
			name:   "2 single chain",
			input:  nodes(sf(0), sf(1, 0), sf(2, 1), sf(3, 2)),
			sp:     seq(sf(0), sf(1, 0), sf(2, 1), sf(3, 2)),
			level:  seq(sf(0), sf(1, 0), sf(2, 1), sf(3, 2)),
			hybrid: seq(sf(0), sf(1, 0), sf(2, 1), sf(3, 2)),
		},
		{
			name:   "3 diamond join",
			input:  nodes(sf(0), sf(1, 0), sf(2, 0), sf(3, 1, 2)),
			sp:     seq(sf(0), par(sf(1, 0), sf(2, 0)), sf(3, 1, 2)),
			level:  seq(sf(0), par(sf(1, 0), sf(2, 0)), sf(3, 1, 2)),
			hybrid: seq(sf(0), par(sf(1, 0), sf(2, 0)), sf(3, 1, 2)),
		},
		{
			name:   "4 two chains joining",
			input:  nodes(sf(0), sf(1), sf(2, 0), sf(3, 1), sf(4, 2, 3)),
			sp:     seq(par(seq(sf(0), sf(2, 0)), seq(sf(1), sf(3, 1))), sf(4, 2, 3)),
			level:  seq(par(sf(0), sf(1)), par(sf(2, 0), sf(3, 1)), sf(4, 2, 3)),
			hybrid: seq(par(seq(sf(0), sf(2, 0)), seq(sf(1), sf(3, 1))), sf(4, 2, 3)),
		},
		{
			name:   "5 wide fan out",
			input:  nodes(sf(0), sf(1, 0), sf(2, 0), sf(3, 0), sf(4, 0)),
			sp:     seq(sf(0), par(sf(1, 0), sf(2, 0), sf(3, 0), sf(4, 0))),
			level:  seq(sf(0), par(sf(1, 0), sf(2, 0), sf(3, 0), sf(4, 0))),
			hybrid: seq(sf(0), par(sf(1, 0), sf(2, 0), sf(3, 0), sf(4, 0))),
		},
		{
			name:   "6 wide fan in",
			input:  nodes(sf(0), sf(1), sf(2), sf(3), sf(4, 0, 1, 2, 3)),
			sp:     seq(par(sf(0), sf(1), sf(2), sf(3)), sf(4, 0, 1, 2, 3)),
			level:  seq(par(sf(0), sf(1), sf(2), sf(3)), sf(4, 0, 1, 2, 3)),
			hybrid: seq(par(sf(0), sf(1), sf(2), sf(3)), sf(4, 0, 1, 2, 3)),
		},
		{
			name: "7 independent diamonds",
			input: nodes(
				sf(0), sf(1, 0), sf(2, 0), sf(3, 1, 2),
				sf(4), sf(5, 4), sf(6, 4), sf(7, 5, 6),
			),
			sp: par(
				seq(sf(0), par(sf(1, 0), sf(2, 0)), sf(3, 1, 2)),
				seq(sf(4), par(sf(5, 4), sf(6, 4)), sf(7, 5, 6)),
			),
			level: par(
				seq(sf(0), par(sf(1, 0), sf(2, 0)), sf(3, 1, 2)),
				seq(sf(4), par(sf(5, 4), sf(6, 4)), sf(7, 5, 6)),
			),
			hybrid: par(
				seq(sf(0), par(sf(1, 0), sf(2, 0)), sf(3, 1, 2)),
				seq(sf(4), par(sf(5, 4), sf(6, 4)), sf(7, 5, 6)),
			),
		},
		{
			name:   "8 requires chain",
			input:  nodes(sf(0), sf(1, 0)),
			sp:     seq(sf(0), sf(1, 0)),
			level:  seq(sf(0), sf(1, 0)),
			hybrid: seq(sf(0), sf(1, 0)),
		},
		{
			name:   "9 batch entity component with independent root",
			input:  nodes(sf(0), bf(1, 0), sf(2)),
			sp:     par(seq(sf(0), bf(1, 0)), sf(2)),
			level:  par(seq(sf(0), bf(1, 0)), sf(2)),
			hybrid: par(seq(sf(0), bf(1, 0)), sf(2)),
		},
		{
			name:   "10 nested entity chain",
			input:  nodes(sf(0), ef(1, 0), ef(2, 1)),
			sp:     seq(sf(0), ef(1, 0), ef(2, 1)),
			level:  seq(sf(0), ef(1, 0), ef(2, 1)),
			hybrid: seq(sf(0), ef(1, 0), ef(2, 1)),
		},
		{
			name:   "11 interface expansion",
			input:  nodes(sf(0), sf(1), sf(2)),
			sp:     par(sf(0), sf(1), sf(2)),
			level:  par(sf(0), sf(1), sf(2)),
			hybrid: par(sf(0), sf(1), sf(2)),
		},
		{
			name:   "12 provides skips fetch",
			input:  nodes(sf(0)),
			sp:     sf(0),
			level:  sf(0),
			hybrid: sf(0),
		},
		{
			name:   "13 sequential mutation",
			input:  nodes(sf(0), sf(1, 0), sf(2, 0, 1)),
			sp:     seq(sf(0), sf(1, 0), sf(2, 0, 1)),
			level:  seq(sf(0), sf(1, 0), sf(2, 0, 1)),
			hybrid: seq(sf(0), sf(1, 0), sf(2, 0, 1)),
		},
		{
			name:   "14 single fetch",
			input:  nodes(sf(0)),
			sp:     sf(0),
			level:  sf(0),
			hybrid: sf(0),
		},
		{
			name:        "15 cycle",
			input:       nodes(sf(0, 1), sf(1, 0)),
			expectError: "cycle detected in fetch dependency graph",
		},
		{
			name:   "16 composite key fan in",
			input:  nodes(sf(0), sf(1, 0), sf(2, 0, 1)),
			sp:     seq(sf(0), sf(1, 0), sf(2, 0, 1)),
			level:  seq(sf(0), sf(1, 0), sf(2, 0, 1)),
			hybrid: seq(sf(0), sf(1, 0), sf(2, 0, 1)),
		},
		{
			name:   "18 asymmetric chain merge with leaf",
			input:  nodes(sf(0), sf(1, 0), sf(2, 0), sf(3, 1, 2), sf(4, 2)),
			sp:     seq(sf(0), par(sf(1, 0), seq(sf(2, 0), sf(4, 2))), sf(3, 1, 2)),
			level:  seq(sf(0), par(sf(1, 0), sf(2, 0)), par(sf(3, 1, 2), sf(4, 2))),
			hybrid: seq(sf(0), par(sf(1, 0), sf(2, 0)), par(sf(3, 1, 2), sf(4, 2))),
		},
		{
			name:   "19 deep multi parent fan in",
			input:  nodes(sf(0), sf(1), sf(2), sf(3, 0, 1, 2), sf(4, 3), sf(5, 4)),
			sp:     seq(par(sf(0), sf(1), sf(2)), sf(3, 0, 1, 2), sf(4, 3), sf(5, 4)),
			level:  seq(par(sf(0), sf(1), sf(2)), sf(3, 0, 1, 2), sf(4, 3), sf(5, 4)),
			hybrid: seq(par(sf(0), sf(1), sf(2)), sf(3, 0, 1, 2), sf(4, 3), sf(5, 4)),
		},
		{
			name:   "20 non sp n shape",
			input:  nodes(sf(0), sf(1), sf(2, 0), sf(3, 0, 1), sf(4, 1)),
			sp:     seq(par(seq(sf(0), sf(2, 0)), seq(sf(1), sf(4, 1))), sf(3, 0, 1)),
			level:  seq(par(sf(0), sf(1)), par(sf(2, 0), sf(3, 0, 1), sf(4, 1))),
			hybrid: seq(par(sf(0), sf(1)), par(sf(2, 0), sf(3, 0, 1), sf(4, 1))),
		},
		{
			name:   "21 independent root with shared join",
			input:  nodes(sf(0), sf(1), sf(2, 0, 1), sf(3)),
			sp:     par(seq(par(sf(0), sf(1)), sf(2, 0, 1)), sf(3)),
			level:  par(seq(par(sf(0), sf(1)), sf(2, 0, 1)), sf(3)),
			hybrid: par(seq(par(sf(0), sf(1)), sf(2, 0, 1)), sf(3)),
		},
		{
			name:   "22 independent leaf alongside chain",
			input:  nodes(sf(0), sf(1, 0), sf(2, 0, 1), sf(3, 0)),
			sp:     seq(sf(0), par(seq(sf(1, 0), sf(2, 0, 1)), sf(3, 0))),
			level:  seq(sf(0), par(seq(sf(1, 0), sf(2, 0, 1)), sf(3, 0))),
			hybrid: seq(sf(0), par(seq(sf(1, 0), sf(2, 0, 1)), sf(3, 0))),
		},
		{
			name:   "23 incomparable dominance fallback",
			input:  nodes(sf(0), sf(1), sf(2, 1), sf(3, 0, 2), sf(4, 0, 1)),
			sp:     seq(par(sf(0), seq(sf(1), sf(2, 1))), par(sf(3, 0, 2), sf(4, 0, 1))),
			level:  seq(par(sf(0), sf(1)), par(seq(sf(2, 1), sf(3, 0, 2)), sf(4, 0, 1))),
			hybrid: seq(par(sf(0), sf(1)), par(seq(sf(2, 1), sf(3, 0, 2)), sf(4, 0, 1))),
		},
	}

	for _, tc := range scenarios {
		t.Run(tc.name, func(t *testing.T) {
			dag, err := newFetchDAG(tc.input)
			require.NoError(t, err)
			ids := dag.sortedIDs()

			actualSP, spErr := scheduleSP(ids, dag)
			actualLevel, levelErr := scheduleLevel(ids, dag)
			actualHybrid, hybridErr := buildScheduleTree(tc.input, dag)
			if tc.expectError != "" {
				require.EqualError(t, spErr, tc.expectError)
				require.EqualError(t, levelErr, tc.expectError)
				require.EqualError(t, hybridErr, tc.expectError)
				return
			}

			require.NoError(t, spErr)
			require.NoError(t, levelErr)
			require.NoError(t, hybridErr)
			require.NoError(t, validateSchedule(actualSP, dag))
			require.NoError(t, validateSchedule(actualLevel, dag))
			require.NoError(t, validateSchedule(actualHybrid, dag))
			require.Equal(t, tc.sp, actualSP)
			require.Equal(t, tc.level, actualLevel)
			require.Equal(t, tc.hybrid, actualHybrid)
		})
	}
}

func TestWithBuildScheduleTreeOptionWiring(t *testing.T) {
	defaultProcessor := NewProcessor()
	require.Equal(t, []string{
		"*postprocess.addMissingNestedDependencies",
		"*postprocess.createConcreteSingleFetchTypes",
		"*postprocess.orderSequenceByDependencies",
		"*postprocess.createParallelNodes",
	}, fetchTreeProcessorTypes(defaultProcessor))

	scheduledProcessor := NewProcessor(
		WithBuildScheduleTree(),
		DisableOrderSequenceByDependencies(),
		DisableCreateParallelNodes(),
	)
	require.Equal(t, []string{
		"*postprocess.addMissingNestedDependencies",
		"*postprocess.createConcreteSingleFetchTypes",
		"*postprocess.buildScheduleTreeProcessor",
	}, fetchTreeProcessorTypes(scheduledProcessor))
}

func TestBuildScheduleTreeScenario17ValidatorStress(t *testing.T) {
	y := sfPath(0, "user")
	x := sfPath(1, "user.details", "user.details")
	tree := par(x, y)
	dag, err := newFetchDAG(nodes(x, y))
	require.NoError(t, err)

	err = validateSchedule(tree, dag)
	require.EqualError(t, err, "parallel fetch 1 at response path user.details depends on fetch 0 providing user")
}

func fetchTreeProcessorTypes(processor *Processor) []string {
	out := make([]string, len(processor.processFetchTree))
	for i, processor := range processor.processFetchTree {
		out[i] = reflect.TypeOf(processor).String()
	}
	return out
}

func nodes(items ...*resolve.FetchTreeNode) []*resolve.FetchTreeNode {
	return items
}

func sf(id int, deps ...int) *resolve.FetchTreeNode {
	return resolve.Single(&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: id, DependsOnFetchIDs: deps}})
}

func ef(id int, deps ...int) *resolve.FetchTreeNode {
	return resolve.Single(&resolve.EntityFetch{FetchDependencies: resolve.FetchDependencies{FetchID: id, DependsOnFetchIDs: deps}})
}

func bf(id int, deps ...int) *resolve.FetchTreeNode {
	return resolve.Single(&resolve.BatchEntityFetch{FetchDependencies: resolve.FetchDependencies{FetchID: id, DependsOnFetchIDs: deps}})
}

func sfPath(id int, responsePath string, mergePath ...string) *resolve.FetchTreeNode {
	return resolve.SingleWithPath(&resolve.SingleFetch{
		FetchDependencies: resolve.FetchDependencies{FetchID: id},
		FetchConfiguration: resolve.FetchConfiguration{
			PostProcessing: resolve.PostProcessingConfiguration{
				MergePath: mergePath,
			},
		},
	}, responsePath)
}

func seq(children ...*resolve.FetchTreeNode) *resolve.FetchTreeNode {
	return resolve.Sequence(children...)
}

func par(children ...*resolve.FetchTreeNode) *resolve.FetchTreeNode {
	return resolve.Parallel(children...)
}
