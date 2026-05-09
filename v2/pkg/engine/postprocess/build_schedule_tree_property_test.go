package postprocess

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestBuildScheduleTreePropertiesExhaustive(t *testing.T) {
	for n := 0; n <= 6; n++ {
		edgeCount := n * (n - 1) / 2
		for mask := 0; mask < 1<<edgeCount; mask++ {
			input := dagFromMask(n, mask)
			checkScheduleProperties(t, input, durationProfiles(input, 50, int64(1000+n*100000+mask)))
		}
	}
}

func TestBuildScheduleTreePropertiesRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for n := 3; n <= 15; n++ {
		for i := 0; i < 50; i++ {
			input := randomDAG(n, 1.5, rng)
			checkScheduleProperties(t, input, durationProfiles(input, 50, int64(n*1000+i)))
		}
	}
}

func TestBuildScheduleTreePropertiesSmoke(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	for _, n := range []int{50, 200} {
		input := randomDAG(n, 1.5, rng)
		checkScheduleProperties(t, input, durationProfiles(input, 50, int64(n)))
	}
}

func TestBuildScheduleTreeFixtureSkewStress(t *testing.T) {
	fixtures := [][]*resolve.FetchTreeNode{
		nodes(sf(0), sf(1), sf(2, 0)),
		nodes(sf(0), sf(1, 0), sf(2, 1), sf(3, 2)),
		nodes(sf(0), sf(1, 0), sf(2, 0), sf(3, 1, 2)),
		nodes(sf(0), sf(1), sf(2, 0), sf(3, 1), sf(4, 2, 3)),
		nodes(sf(0), sf(1, 0), sf(2, 0), sf(3, 0), sf(4, 0)),
		nodes(sf(0), sf(1), sf(2), sf(3), sf(4, 0, 1, 2, 3)),
		nodes(sf(0), sf(1, 0), sf(2, 0), sf(3, 1, 2), sf(4), sf(5, 4), sf(6, 4), sf(7, 5, 6)),
		nodes(sf(0), sf(1, 0)),
		nodes(sf(0), bf(1, 0), sf(2)),
		nodes(sf(0), ef(1, 0), ef(2, 1)),
		nodes(sf(0), sf(1), sf(2)),
		nodes(sf(0)),
		nodes(sf(0), sf(1, 0), sf(2, 0, 1)),
		nodes(sf(0)),
		nodes(sf(0), sf(1, 0), sf(2, 0, 1)),
		nodes(sf(0), sf(1, 0), sf(2, 0), sf(3, 1, 2), sf(4, 2)),
		nodes(sf(0), sf(1), sf(2), sf(3, 0, 1, 2), sf(4, 3), sf(5, 4)),
		nodes(sf(0), sf(1), sf(2, 0), sf(3, 0, 1), sf(4, 1)),
		nodes(sf(0), sf(1), sf(2, 0, 1), sf(3)),
		nodes(sf(0), sf(1, 0), sf(2, 0, 1), sf(3, 0)),
		nodes(sf(0), sf(1), sf(2, 1), sf(3, 0, 2), sf(4, 0, 1)),
	}
	for i, fixture := range fixtures {
		checkScheduleProperties(t, fixture, durationProfiles(fixture, 50, int64(9000+i)))
	}
}

func checkScheduleProperties(t *testing.T, input []*resolve.FetchTreeNode, profiles []map[int]int) {
	t.Helper()
	dag, err := newFetchDAG(input)
	require.NoError(t, err)
	ids := dag.sortedIDs()
	sp, spErr := scheduleSP(ids, dag)
	level, levelErr := scheduleLevel(ids, dag)
	hybrid, hybridErr := buildScheduleTree(input, dag)
	if len(ids) > 0 && dag.hasCycle() {
		require.EqualError(t, spErr, "cycle detected in fetch dependency graph")
		require.EqualError(t, levelErr, "cycle detected in fetch dependency graph")
		require.EqualError(t, hybridErr, "cycle detected in fetch dependency graph")
		return
	}
	require.NoErrorf(t, spErr, "input=%v", dependencyList(input))
	require.NoErrorf(t, levelErr, "input=%v", dependencyList(input))
	require.NoErrorf(t, hybridErr, "input=%v", dependencyList(input))
	require.NoError(t, validateSchedule(sp, dag))
	require.NoError(t, validateSchedule(level, dag))
	require.NoError(t, validateSchedule(hybrid, dag))

	if dominates(sp, level) {
		require.Equal(t, sp, hybrid)
	} else {
		require.Equal(t, level, hybrid)
	}
	require.LessOrEqual(t, uniformMakespan(hybrid), uniformMakespan(level))
	for _, durations := range profiles {
		hybridMakespan := weightedMakespan(hybrid, durations)
		levelMakespan := weightedMakespan(level, durations)
		require.LessOrEqual(t, hybridMakespan, levelMakespan)
		if dominates(sp, level) {
			require.LessOrEqual(t, weightedMakespan(sp, durations), levelMakespan)
		}
		_ = !dominates(sp, level) && weightedMakespan(sp, durations) < levelMakespan
	}
}

func dependencyList(input []*resolve.FetchTreeNode) []resolve.FetchDependencies {
	out := make([]resolve.FetchDependencies, 0, len(input))
	for _, node := range input {
		out = append(out, *node.Item.Fetch.Dependencies())
	}
	return out
}

func dagFromMask(n, mask int) []*resolve.FetchTreeNode {
	deps := make([][]int, n)
	bit := 0
	for from := 0; from < n; from++ {
		for to := from + 1; to < n; to++ {
			if mask&(1<<bit) != 0 {
				deps[to] = append(deps[to], from)
			}
			bit++
		}
	}
	nodes := make([]*resolve.FetchTreeNode, n)
	for i := range nodes {
		nodes[i] = sf(i, deps[i]...)
	}
	return nodes
}

func randomDAG(n int, averageDegree float64, rng *rand.Rand) []*resolve.FetchTreeNode {
	deps := make([][]int, n)
	p := averageDegree / math.Max(1, float64(n-1))
	for from := 0; from < n; from++ {
		for to := from + 1; to < n; to++ {
			if rng.Float64() < p {
				deps[to] = append(deps[to], from)
			}
		}
	}
	nodes := make([]*resolve.FetchTreeNode, n)
	for i := range nodes {
		nodes[i] = sf(i, deps[i]...)
	}
	return nodes
}

func durationProfiles(input []*resolve.FetchTreeNode, count int, seed int64) []map[int]int {
	rng := rand.New(rand.NewSource(seed))
	profiles := make([]map[int]int, count)
	for i := range profiles {
		profile := make(map[int]int, len(input))
		for _, node := range input {
			id := node.Item.Fetch.Dependencies().FetchID
			profile[id] = int(math.Round(math.Exp(rng.Float64() * math.Log(1000))))
			if profile[id] < 1 {
				profile[id] = 1
			}
		}
		profiles[i] = profile
	}
	return profiles
}
