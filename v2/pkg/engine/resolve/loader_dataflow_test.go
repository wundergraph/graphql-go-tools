package resolve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

// The tests in this file verify the dataflow executor against the wave executor
// by running the IDENTICAL plan through both and asserting full-output equality
// (rule: assert.Equal on complete bytes, never Contains). They use the direct
// Loader harness (no Resolver background goroutines) so goleak can verify the
// dataflow coordinator leaks nothing.
//
// Batch entity fetches are exercised through the dataflow path end-to-end
// by the router-level byte-identity gate (perf-analysis/q_byteid.sh runs the
// full federated query corpus with ENGINE_ENABLE_DATAFLOW=true); the unit tests
// here pin the dataflow-specific machinery: scheduling, coordinator-owned
// prepare, swap-capture error staging, leaf-order flush, fallbacks,
// cancellation, and goroutine hygiene.

type delayDataSource struct {
	response []byte
	delay    time.Duration
	loadErr  error

	mu     sync.Mutex
	inputs []string

	loadCalls int64
}

func newDelayDataSource(response string, delay time.Duration) *delayDataSource {
	return &delayDataSource{response: []byte(response), delay: delay}
}

func (d *delayDataSource) Load(ctx context.Context, _ http.Header, input []byte) ([]byte, error) {
	d.mu.Lock()
	d.loadCalls++
	d.inputs = append(d.inputs, string(input))
	d.mu.Unlock()
	if d.delay > 0 {
		select {
		case <-time.After(d.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if d.loadErr != nil {
		return nil, d.loadErr
	}
	return d.response, nil
}

func (d *delayDataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, _ []*httpclient.FileUpload) ([]byte, error) {
	return d.Load(ctx, headers, input)
}

func (d *delayDataSource) requireInputs(t *testing.T, expected ...string) {
	t.Helper()
	d.mu.Lock()
	defer d.mu.Unlock()
	require.Equal(t, expected, d.inputs)
}

func (d *delayDataSource) calls() int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.loadCalls
}

// dataflowFetch builds a SingleFetch with a static input, FetchInfo (data source
// name feeds error messages) and explicit FetchID/dependency edges.
func dataflowFetch(ds DataSource, id int, deps []int, name, staticInput string) *SingleFetch {
	f := nestedParallelSingleFetchWithInfo(ds, staticInput, name, name)
	f.FetchDependencies.FetchID = id
	f.FetchDependencies.DependsOnFetchIDs = deps
	return f
}

// dataflowFetchReading builds a SingleFetch whose input template renders the
// given fields from the parent-merged response data (arena reads at prepare).
func dataflowFetchReading(ds DataSource, id int, deps []int, name string, fields ...string) *SingleFetch {
	f := nestedParallelSingleFetchWithInfoAndTemplate(ds, nestedParallelInputForFields(fields...), name, name)
	f.FetchDependencies.FetchID = id
	f.FetchDependencies.DependsOnFetchIDs = deps
	return f
}

// runDataflowScenario executes the response plan through the direct Loader
// harness and returns the SERIALIZED response (the real query-order renderer,
// resolvable.Resolve — raw arena insertion order is merge-order-dependent by
// design and is not part of the byte-identity contract) plus the load error.
func runDataflowScenario(t *testing.T, reqCtx context.Context, enableDataflow bool, opType ast.OperationType, response *GraphQLResponse, configure func(*Loader, *Context)) (string, error) {
	t.Helper()
	ctx := NewContext(reqCtx)
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	resolvable := NewResolvable(nil, ResolvableOptions{})
	loader := &Loader{
		enableDataflow:               enableDataflow,
		propagateSubgraphErrors:      true,
		propagateSubgraphStatusCodes: true,
		subgraphErrorPropagationMode: SubgraphErrorPropagationModeWrapped,
		// pass-through mode filters subgraph error fields by allowlist; mirror the
		// production default of allowing "message"
		allowedSubgraphErrorFields: map[string]struct{}{"message": {}},
	}
	if configure != nil {
		configure(loader, ctx)
	}
	require.NoError(t, resolvable.Init(ctx, nil, opType))
	err := loader.LoadGraphQLResponseData(ctx, response, resolvable)
	buf := &bytes.Buffer{}
	require.NoError(t, resolvable.Resolve(ctx.ctx, response.Data, response.Fetches, buf))
	return buf.String(), err
}

// runBothExecutors builds a fresh plan per executor (fresh datasources, fresh
// channels) and returns both outputs; both runs must succeed.
func runBothExecutors(t *testing.T, build func() *GraphQLResponse, configure func(*Loader, *Context)) (wave string, dataflow string) {
	t.Helper()
	wave, waveErr := runDataflowScenario(t, context.Background(), false, ast.OperationTypeQuery, build(), configure)
	require.NoError(t, waveErr)
	dataflow, dataflowErr := runDataflowScenario(t, context.Background(), true, ast.OperationTypeQuery, build(), configure)
	require.NoError(t, dataflowErr)
	return wave, dataflow
}

func TestDataflowByteIdenticalToWave(t *testing.T) {
	t.Run("two waves", func(t *testing.T) {
		build := func() *GraphQLResponse {
			r := newDelayDataSource(`{"data":{"r":"R"}}`, 0)
			b := newDelayDataSource(`{"data":{"b":"B"}}`, 4*time.Millisecond)
			c := newDelayDataSource(`{"data":{"c":"C"}}`, time.Millisecond)
			return &GraphQLResponse{
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				Fetches: Sequence(
					Single(dataflowFetch(r, 0, nil, "R", `{"fetch":"r"}`)),
					Parallel(
						Single(dataflowFetch(b, 1, []int{0}, "B", `{"fetch":"b"}`)),
						Single(dataflowFetch(c, 2, []int{0}, "C", `{"fetch":"c"}`)),
					),
				),
				Data: nestedParallelData("r", "b", "c"),
			}
		}
		wave, dataflow := runBothExecutors(t, build, nil)
		require.Equal(t, `{"data":{"r":"R","b":"B","c":"C"}}`, wave)
		require.Equal(t, wave, dataflow)
	})

	t.Run("diamond reads merged data", func(t *testing.T) {
		var dWave, dDataflow *delayDataSource
		build := func() *delayDataSource {
			return newDelayDataSource(`{"data":{"d":"D"}}`, 0)
		}
		buildResponse := func(d *delayDataSource) *GraphQLResponse {
			r := newDelayDataSource(`{"data":{"r":"R"}}`, 0)
			b := newDelayDataSource(`{"data":{"b":"B"}}`, 3*time.Millisecond)
			c := newDelayDataSource(`{"data":{"c":"C"}}`, time.Millisecond)
			return &GraphQLResponse{
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				Fetches: Sequence(
					Single(dataflowFetch(r, 0, nil, "R", `{"fetch":"r"}`)),
					Parallel(
						Single(dataflowFetchReading(b, 1, []int{0}, "B", "r")),
						Single(dataflowFetchReading(c, 2, []int{0}, "C", "r")),
					),
					Single(dataflowFetchReading(d, 3, []int{1, 2}, "D", "b", "c")),
				),
				Data: nestedParallelData("r", "b", "c", "d"),
			}
		}
		dWave = build()
		wave, waveErr := runDataflowScenario(t, context.Background(), false, ast.OperationTypeQuery, buildResponse(dWave), nil)
		require.NoError(t, waveErr)
		dDataflow = build()
		dataflow, dataflowErr := runDataflowScenario(t, context.Background(), true, ast.OperationTypeQuery, buildResponse(dDataflow), nil)
		require.NoError(t, dataflowErr)
		require.Equal(t, `{"data":{"r":"R","b":"B","c":"C","d":"D"}}`, wave)
		require.Equal(t, wave, dataflow)
		// d's input renders from BOTH parents' merged data — identical in both modes.
		dWave.requireInputs(t, `{"b":"B","c":"C"}`)
		dDataflow.requireInputs(t, `{"b":"B","c":"C"}`)
	})

	t.Run("fan-out 8", func(t *testing.T) {
		delays := []time.Duration{5, 0, 3, 1, 4, 2, 0, 1}
		build := func() *GraphQLResponse {
			r := newDelayDataSource(`{"data":{"r":"R"}}`, 0)
			children := make([]*FetchTreeNode, 8)
			fields := []string{"r"}
			for i := range 8 {
				name := fmt.Sprintf("f%d", i)
				ds := newDelayDataSource(fmt.Sprintf(`{"data":{"%s":"V%d"}}`, name, i), delays[i]*time.Millisecond)
				children[i] = Single(dataflowFetch(ds, i+1, []int{0}, name, fmt.Sprintf(`{"fetch":"%s"}`, name)))
				fields = append(fields, name)
			}
			return &GraphQLResponse{
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				Fetches: Sequence(
					append([]*FetchTreeNode{Single(dataflowFetch(r, 0, nil, "R", `{"fetch":"r"}`))},
						Parallel(children...))...,
				),
				Data: nestedParallelData(fields...),
			}
		}
		wave, dataflow := runBothExecutors(t, build, nil)
		require.Equal(t, `{"data":{"r":"R","f0":"V0","f1":"V1","f2":"V2","f3":"V3","f4":"V4","f5":"V5","f6":"V6","f7":"V7"}}`, wave)
		require.Equal(t, wave, dataflow)
	})
}

func TestDataflowRaceStress(t *testing.T) {
	// 12-fetch DAG with seeded random per-fetch delays. Run repeatedly so the
	// dispatch/merge interleavings vary; -race is the gate, full-byte equality
	// the assertion.
	rng := rand.New(rand.NewSource(0x5eed))
	for iter := range 30 {
		d := func() time.Duration { return time.Duration(rng.Intn(4)) * time.Millisecond }
		build := func() *GraphQLResponse {
			r := newDelayDataSource(`{"data":{"r":"R"}}`, d())
			mk := func(name string, id int, deps []int, readFields ...string) *FetchTreeNode {
				ds := newDelayDataSource(fmt.Sprintf(`{"data":{"%s":"%s"}}`, name, name), d())
				if len(readFields) > 0 {
					return Single(dataflowFetchReading(ds, id, deps, name, readFields...))
				}
				return Single(dataflowFetch(ds, id, deps, name, fmt.Sprintf(`{"fetch":"%s"}`, name)))
			}
			return &GraphQLResponse{
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				Fetches: Sequence(
					Single(dataflowFetch(r, 0, nil, "R", `{"fetch":"r"}`)),
					Parallel(
						mk("a1", 1, []int{0}, "r"),
						mk("a2", 2, []int{0}, "r"),
						mk("a3", 3, []int{0}, "r"),
					),
					Parallel(
						mk("b1", 4, []int{1}, "a1"),
						mk("b2", 5, []int{1}),
						mk("b3", 6, []int{2}, "a2"),
						mk("b4", 7, []int{2}),
						mk("b5", 8, []int{3}, "a3"),
						mk("b6", 9, []int{3}),
					),
					Parallel(
						mk("c1", 10, []int{4, 5}, "b1", "b2"),
						mk("c2", 11, []int{8, 9}, "b5", "b6"),
					),
				),
				Data: nestedParallelData("r", "a1", "a2", "a3", "b1", "b2", "b3", "b4", "b5", "b6", "c1", "c2"),
			}
		}
		wave, dataflow := runBothExecutors(t, build, nil)
		require.Equal(t, `{"data":{"r":"R","a1":"a1","a2":"a2","a3":"a3","b1":"b1","b2":"b2","b3":"b3","b4":"b4","b5":"b5","b6":"b6","c1":"c1","c2":"c2"}}`, wave, "iteration %d", iter)
		require.Equal(t, wave, dataflow, "iteration %d", iter)
	}
}

// TestDataflowPrepareMergeOverlap reconstructs the ORIGINAL arena race the
// hardening eliminated: sibling A merges into the root object while sibling B's
// input would render from the same object. With coordinator-owned prepare, B's
// input is rendered at dispatch time on the coordinator — before either sibling
// completes — so no arena access can overlap a merge. The gate is -race
// cleanliness plus full-byte equality.
func TestDataflowPrepareMergeOverlap(t *testing.T) {
	for range 20 {
		var bWave, bDataflow *delayDataSource
		buildWith := func(b *delayDataSource) func() *GraphQLResponse {
			return func() *GraphQLResponse {
				p := newDelayDataSource(`{"data":{"p":"P"}}`, 0)
				a := newDelayDataSource(`{"data":{"a":"A"}}`, 0)
				return &GraphQLResponse{
					Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
					Fetches: Sequence(
						Single(dataflowFetch(p, 0, nil, "P", `{"fetch":"p"}`)),
						Parallel(
							Single(dataflowFetchReading(a, 1, []int{0}, "A", "p")),
							Single(dataflowFetchReading(b, 2, []int{0}, "B", "p")),
						),
					),
					Data: nestedParallelData("p", "a", "b"),
				}
			}
		}
		bWave = newDelayDataSource(`{"data":{"b":"B"}}`, 4*time.Millisecond)
		wave, waveErr := runDataflowScenario(t, context.Background(), false, ast.OperationTypeQuery, buildWith(bWave)(), nil)
		require.NoError(t, waveErr)
		bDataflow = newDelayDataSource(`{"data":{"b":"B"}}`, 4*time.Millisecond)
		dataflow, dataflowErr := runDataflowScenario(t, context.Background(), true, ast.OperationTypeQuery, buildWith(bDataflow)(), nil)
		require.NoError(t, dataflowErr)
		require.Equal(t, `{"data":{"p":"P","a":"A","b":"B"}}`, wave)
		require.Equal(t, wave, dataflow)
		bWave.requireInputs(t, `{"p":"P"}`)
		bDataflow.requireInputs(t, `{"p":"P"}`)
	}
}

// TestDataflowErrorOrderDeterminism inverts completion order against plan order
// (first leaf slowest) and asserts the errors array is byte-identical to the
// wave executor — the property the swap-capture staging plus leaf-order flush
// exists to guarantee.
func TestDataflowErrorOrderDeterminism(t *testing.T) {
	build := func() *GraphQLResponse {
		p := newDelayDataSource(`{"data":{"p":"P"}}`, 0)
		mkErr := func(name string, id int, delay time.Duration) *FetchTreeNode {
			ds := newDelayDataSource(fmt.Sprintf(`{"errors":[{"message":"%s exploded"}],"data":{"%s":null}}`, name, name), delay)
			f := dataflowFetch(ds, id, []int{0}, name, fmt.Sprintf(`{"fetch":"%s"}`, name))
			f.PostProcessing.SelectResponseErrorsPath = []string{"errors"}
			return SingleWithPath(f, "query."+name)
		}
		return &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Sequence(
				Single(dataflowFetch(p, 0, nil, "P", `{"fetch":"p"}`)),
				Parallel(
					mkErr("e1", 1, 6*time.Millisecond),
					mkErr("e2", 2, 3*time.Millisecond),
					mkErr("e3", 3, 0),
				),
			),
			Data: nestedParallelNullableData("p", "e1", "e2", "e3"),
		}
	}

	for _, mode := range []struct {
		name string
		mode SubgraphErrorPropagationMode
	}{
		{name: "wrap", mode: SubgraphErrorPropagationModeWrapped},
		{name: "pass-through", mode: SubgraphErrorPropagationModePassThrough},
	} {
		t.Run(mode.name, func(t *testing.T) {
			configure := func(l *Loader, _ *Context) {
				l.subgraphErrorPropagationMode = mode.mode
			}
			wave, dataflow := runBothExecutors(t, build, configure)
			// Completion order is e3, e2, e1 — the errors array must still be in
			// PLAN order e1, e2, e3, exactly as the wave executor emits it.
			require.Equal(t, wave, dataflow)
			switch mode.mode {
			case SubgraphErrorPropagationModeWrapped:
				require.Equal(t, `{"errors":[{"message":"Failed to fetch from Subgraph 'e1' at Path 'query.e1'.","extensions":{"errors":[{"message":"e1 exploded"}]}},{"message":"Failed to fetch from Subgraph 'e2' at Path 'query.e2'.","extensions":{"errors":[{"message":"e2 exploded"}]}},{"message":"Failed to fetch from Subgraph 'e3' at Path 'query.e3'.","extensions":{"errors":[{"message":"e3 exploded"}]}}],"data":{"p":"P","e1":null,"e2":null,"e3":null}}`, dataflow)
			case SubgraphErrorPropagationModePassThrough:
				require.Equal(t, `{"errors":[{"message":"e1 exploded"},{"message":"e2 exploded"},{"message":"e3 exploded"}],"data":{"p":"P","e1":null,"e2":null,"e3":null}}`, dataflow)
			}
		})
	}
}

type stubRateLimiter struct {
	denyName string
	errName  string
}

func (s *stubRateLimiter) RateLimitPreFetch(_ *Context, info *FetchInfo, _ json.RawMessage) (*RateLimitDeny, error) {
	if info != nil && info.DataSourceName == s.errName {
		return nil, errors.New("rate limiter exploded")
	}
	if info != nil && info.DataSourceName == s.denyName {
		return &RateLimitDeny{Reason: "test denied"}, nil
	}
	return nil, nil
}

func (s *stubRateLimiter) RenderResponseExtension(_ *Context, _ io.Writer) error {
	return nil
}

// TestDataflowRateLimitDenyAbortPath: a mid-DAG fetch is denied at prepare
// (skipLoad), its rendered denial error goes through the staged merge, and its
// dependent still dispatches — byte-identical to the wave executor.
func TestDataflowRateLimitDenyAbortPath(t *testing.T) {
	var dWave, dDataflow *delayDataSource
	buildWith := func(d *delayDataSource) *GraphQLResponse {
		p := newDelayDataSource(`{"data":{"p":"P"}}`, 0)
		m := newDelayDataSource(`{"data":{"m":"M"}}`, 0)
		mFetch := dataflowFetch(m, 1, []int{0}, "M", `{"fetch":"m"}`)
		return &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Sequence(
				Single(dataflowFetch(p, 0, nil, "P", `{"fetch":"p"}`)),
				SingleWithPath(mFetch, "query.m"),
				// static input: the dependent must still dispatch and LOAD after its
				// parent was denied (denial nulls m, but d does not read it)
				Single(dataflowFetch(d, 2, []int{1}, "D", `{"fetch":"d"}`)),
			),
			Data: nestedParallelNullableData("p", "m", "d"),
		}
	}
	configure := func(_ *Loader, ctx *Context) {
		ctx.RateLimitOptions = RateLimitOptions{Enable: true}
		ctx.SetRateLimiter(&stubRateLimiter{denyName: "M"})
	}

	dWave = newDelayDataSource(`{"data":{"d":"D"}}`, 0)
	wave, waveErr := runDataflowScenario(t, context.Background(), false, ast.OperationTypeQuery, buildWith(dWave), configure)
	require.NoError(t, waveErr)
	dDataflow = newDelayDataSource(`{"data":{"d":"D"}}`, 0)
	dataflow, dataflowErr := runDataflowScenario(t, context.Background(), true, ast.OperationTypeQuery, buildWith(dDataflow), configure)
	require.NoError(t, dataflowErr)

	require.Equal(t, wave, dataflow)
	require.Equal(t, `{"errors":[{"message":"Rate limit exceeded for Subgraph 'M' at Path 'query.m', Reason: test denied."}],"data":{"p":"P","m":null,"d":"D"}}`, wave)
	// The denied fetch loads nothing; the dependent still runs in both modes.
	require.Equal(t, int64(1), dWave.calls())
	require.Equal(t, int64(1), dDataflow.calls())
	dWave.requireInputs(t, `{"fetch":"d"}`)
	dDataflow.requireInputs(t, `{"fetch":"d"}`)
}

type recordingRateLimiter struct {
	mu   sync.Mutex
	seen []string
}

func (r *recordingRateLimiter) RateLimitPreFetch(_ *Context, info *FetchInfo, _ json.RawMessage) (*RateLimitDeny, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if info != nil {
		r.seen = append(r.seen, info.DataSourceName)
	}
	return nil, nil
}

func (r *recordingRateLimiter) RenderResponseExtension(_ *Context, _ io.Writer) error {
	return nil
}

func (r *recordingRateLimiter) order() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seen
}

// denyThenRecordRateLimiter records every pre-fetch call (via inner) and denies
// the named data source.
type denyThenRecordRateLimiter struct {
	deny  string
	inner *recordingRateLimiter
}

func (d *denyThenRecordRateLimiter) RateLimitPreFetch(ctx *Context, info *FetchInfo, input json.RawMessage) (*RateLimitDeny, error) {
	_, _ = d.inner.RateLimitPreFetch(ctx, info, input)
	if info != nil && info.DataSourceName == d.deny {
		return &RateLimitDeny{Reason: "test denied"}, nil
	}
	return nil, nil
}

func (d *denyThenRecordRateLimiter) RenderResponseExtension(_ *Context, _ io.Writer) error {
	return nil
}

// TestDataflowPreFetchHookOrder is the regression test for the codex P1 on the
// hardening: pre-fetch hooks (RateLimitPreFetch/AuthorizePreFetch) fire at
// PREPARE time and can render order-sensitive accumulated state into response
// extensions, so dataflow must dispatch in LEAF order (= the wave executor's
// spawn order), not FetchID order. FetchIDs here are deliberately INVERTED
// against tree order — FetchID-ordered seeding would call the hook for B first.
func TestDataflowPreFetchHookOrder(t *testing.T) {
	t.Run("serial sequence, inverted fetch IDs", func(t *testing.T) {
		run := func(enableDataflow bool) []string {
			a := newDelayDataSource(`{"data":{"a":"A"}}`, 0)
			b := newDelayDataSource(`{"data":{"b":"B"}}`, 0)
			response := &GraphQLResponse{
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				Fetches: Sequence(
					Single(dataflowFetch(a, 2, nil, "A", `{"fetch":"a"}`)),
					Single(dataflowFetch(b, 1, nil, "B", `{"fetch":"b"}`)),
				),
				Data: nestedParallelData("a", "b"),
			}
			limiter := &recordingRateLimiter{}
			configure := func(_ *Loader, ctx *Context) {
				ctx.RateLimitOptions = RateLimitOptions{Enable: true}
				ctx.SetRateLimiter(limiter)
			}
			out, err := runDataflowScenario(t, context.Background(), enableDataflow, ast.OperationTypeQuery, response, configure)
			require.NoError(t, err)
			require.Equal(t, `{"data":{"a":"A","b":"B"}}`, out)
			return limiter.order()
		}
		require.Equal(t, []string{"A", "B"}, run(false))
		require.Equal(t, []string{"A", "B"}, run(true))
	})

	t.Run("inline skip unblocks ahead of queued siblings", func(t *testing.T) {
		// codex P1 round 2: A is rate-limit denied (inline skipLoad completion),
		// which makes C (leaf 1, depends on A) ready while B (leaf 2) is already
		// queued. FIFO dispatch would call hooks as A,B,C; the wave executor's
		// serial order is A,C,B — global leaf-minimum popping must reproduce it.
		run := func(enableDataflow bool) []string {
			a := newDelayDataSource(`{"data":{"a":"never"}}`, 0)
			c := newDelayDataSource(`{"data":{"c":"C"}}`, 0)
			b := newDelayDataSource(`{"data":{"b":"B"}}`, 0)
			aFetch := dataflowFetch(a, 0, nil, "A", `{"fetch":"a"}`)
			response := &GraphQLResponse{
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				Fetches: Sequence(
					SingleWithPath(aFetch, "query.a"),
					Single(dataflowFetch(c, 2, []int{0}, "C", `{"fetch":"c"}`)),
					Single(dataflowFetch(b, 1, nil, "B", `{"fetch":"b"}`)),
				),
				Data: nestedParallelNullableData("a", "c", "b"),
			}
			limiter := &recordingRateLimiter{}
			deny := &denyThenRecordRateLimiter{deny: "A", inner: limiter}
			configure := func(_ *Loader, ctx *Context) {
				ctx.RateLimitOptions = RateLimitOptions{Enable: true}
				ctx.SetRateLimiter(deny)
			}
			out, err := runDataflowScenario(t, context.Background(), enableDataflow, ast.OperationTypeQuery, response, configure)
			require.NoError(t, err)
			require.Equal(t, `{"errors":[{"message":"Rate limit exceeded for Subgraph 'A' at Path 'query.a', Reason: test denied."}],"data":{"a":null,"c":"C","b":"B"}}`, out)
			return limiter.order()
		}
		require.Equal(t, []string{"A", "C", "B"}, run(false))
		require.Equal(t, []string{"A", "C", "B"}, run(true))
	})

	t.Run("parallel wave, inverted fetch IDs", func(t *testing.T) {
		// The wave executor calls hooks CONCURRENTLY within a Parallel wave
		// (unordered), so only the dataflow order is asserted: deterministic
		// leaf order, root first.
		r := newDelayDataSource(`{"data":{"r":"R"}}`, 0)
		x := newDelayDataSource(`{"data":{"x":"X"}}`, 0)
		y := newDelayDataSource(`{"data":{"y":"Y"}}`, 0)
		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Sequence(
				Single(dataflowFetch(r, 0, nil, "R", `{"fetch":"r"}`)),
				Parallel(
					Single(dataflowFetch(x, 5, []int{0}, "X", `{"fetch":"x"}`)),
					Single(dataflowFetch(y, 3, []int{0}, "Y", `{"fetch":"y"}`)),
				),
			),
			Data: nestedParallelData("r", "x", "y"),
		}
		limiter := &recordingRateLimiter{}
		configure := func(_ *Loader, ctx *Context) {
			ctx.RateLimitOptions = RateLimitOptions{Enable: true}
			ctx.SetRateLimiter(limiter)
		}
		out, err := runDataflowScenario(t, context.Background(), true, ast.OperationTypeQuery, response, configure)
		require.NoError(t, err)
		require.Equal(t, `{"data":{"r":"R","x":"X","y":"Y"}}`, out)
		require.Equal(t, []string{"R", "X", "Y"}, limiter.order())
	})
}

func TestDataflowFallbacks(t *testing.T) {
	t.Run("duplicate fetch IDs", func(t *testing.T) {
		build := func() *GraphQLResponse {
			a := newDelayDataSource(`{"data":{"a":"A"}}`, 0)
			b := newDelayDataSource(`{"data":{"b":"B"}}`, 0)
			return &GraphQLResponse{
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				Fetches: Sequence(
					Single(dataflowFetch(a, 7, nil, "A", `{"fetch":"a"}`)),
					Single(dataflowFetch(b, 7, nil, "B", `{"fetch":"b"}`)),
				),
				Data: nestedParallelData("a", "b"),
			}
		}
		wave, dataflow := runBothExecutors(t, build, nil)
		require.Equal(t, `{"data":{"a":"A","b":"B"}}`, wave)
		require.Equal(t, wave, dataflow)
	})

	t.Run("dependency cycle", func(t *testing.T) {
		build := func() *GraphQLResponse {
			a := newDelayDataSource(`{"data":{"a":"A"}}`, 0)
			b := newDelayDataSource(`{"data":{"b":"B"}}`, 0)
			return &GraphQLResponse{
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				Fetches: Sequence(
					Single(dataflowFetch(a, 1, []int{2}, "A", `{"fetch":"a"}`)),
					Single(dataflowFetch(b, 2, []int{1}, "B", `{"fetch":"b"}`)),
				),
				Data: nestedParallelData("a", "b"),
			}
		}
		wave, dataflow := runBothExecutors(t, build, nil)
		require.Equal(t, `{"data":{"a":"A","b":"B"}}`, wave)
		require.Equal(t, wave, dataflow)
	})

	t.Run("mutation", func(t *testing.T) {
		build := func() *GraphQLResponse {
			a := newDelayDataSource(`{"data":{"a":"A"}}`, 0)
			b := newDelayDataSource(`{"data":{"b":"B"}}`, 0)
			return &GraphQLResponse{
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeMutation},
				Fetches: Sequence(
					Single(dataflowFetch(a, 0, nil, "A", `{"fetch":"a"}`)),
					Single(dataflowFetch(b, 1, []int{0}, "B", `{"fetch":"b"}`)),
				),
				Data: nestedParallelData("a", "b"),
			}
		}
		wave, waveErr := runDataflowScenario(t, context.Background(), false, ast.OperationTypeMutation, build(), nil)
		require.NoError(t, waveErr)
		dataflow, dataflowErr := runDataflowScenario(t, context.Background(), true, ast.OperationTypeMutation, build(), nil)
		require.NoError(t, dataflowErr)
		require.Equal(t, `{"data":{"a":"A","b":"B"}}`, wave)
		require.Equal(t, wave, dataflow)
	})
}

// TestDataflowCancellation cancels the request context while a load is blocked
// on it; both executors must return promptly with identical output and fully
// drain their goroutines.
func TestDataflowCancellation(t *testing.T) {
	defer goleak.VerifyNone(t)
	run := func(enableDataflow bool) string {
		blocked := newControlledLoaderDataSource(`{"data":{"s":"never"}}`)
		blocked.waitForCancel = true
		fast := newDelayDataSource(`{"data":{"f":"F"}}`, 0)
		sFetch := nestedParallelSingleFetchWithInfo(blocked, `{"fetch":"s"}`, "S", "S")
		sFetch.FetchDependencies.FetchID = 0
		fFetch := dataflowFetch(fast, 1, nil, "F", `{"fetch":"f"}`)
		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Sequence(
				Parallel(
					SingleWithPath(sFetch, "query.s"),
					Single(fFetch),
				),
			),
			Data: nestedParallelNullableData("s", "f"),
		}
		reqCtx, cancel := context.WithCancel(context.Background())
		timer := time.AfterFunc(30*time.Millisecond, cancel)
		defer timer.Stop()
		defer cancel()
		start := time.Now()
		out, err := runDataflowScenario(t, reqCtx, enableDataflow, ast.OperationTypeQuery, response, nil)
		require.NoError(t, err)
		require.Less(t, time.Since(start), 2*time.Second)
		require.True(t, blocked.cancelled.Load())
		return out
	}
	wave := run(false)
	dataflow := run(true)
	require.Equal(t, `{"errors":[{"message":"Failed to fetch from Subgraph 'S' at Path 'query.s'."}],"data":{"s":null,"f":"F"}}`, wave)
	require.Equal(t, wave, dataflow)
}

// TestDataflowNoGoroutineLeaks: a prepare-time fatal (rate limiter error) fires
// while a sibling load is in flight; the coordinator must cancel, drain the
// in-flight worker, flush, and return the fatal — leaking nothing.
func TestDataflowNoGoroutineLeaks(t *testing.T) {
	defer goleak.VerifyNone(t)
	build := func() *GraphQLResponse {
		p := newDelayDataSource(`{"data":{"p":"P"}}`, 0)
		slow := newDelayDataSource(`{"data":{"s":"S"}}`, 50*time.Millisecond)
		boom := newDelayDataSource(`{"data":{"x":"X"}}`, 0)
		return &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Sequence(
				Single(dataflowFetch(p, 0, nil, "P", `{"fetch":"p"}`)),
				Parallel(
					Single(dataflowFetch(slow, 1, []int{0}, "S", `{"fetch":"s"}`)),
					Single(dataflowFetch(boom, 2, []int{0}, "X", `{"fetch":"x"}`)),
				),
			),
			Data: nestedParallelNullableData("p", "s", "x"),
		}
	}
	configure := func(_ *Loader, ctx *Context) {
		ctx.RateLimitOptions = RateLimitOptions{Enable: true}
		ctx.SetRateLimiter(&stubRateLimiter{errName: "X"})
	}
	_, waveErr := runDataflowScenario(t, context.Background(), false, ast.OperationTypeQuery, build(), configure)
	require.EqualError(t, waveErr, "rate limiter exploded")
	_, dataflowErr := runDataflowScenario(t, context.Background(), true, ast.OperationTypeQuery, build(), configure)
	require.EqualError(t, dataflowErr, "rate limiter exploded")
}

// TestCollectDataflowLeavesAcceptsOnlyFlatTrees pins the structural guard that
// makes ENGINE_ENABLE_DATAFLOW safe to combine with schedule-tree plans: only the
// flat createParallelNodes shape (Sequence of Single / Parallel-of-Single) is
// eligible; any nested tree must report ok=false so resolveDataflow falls back to
// the (mergeMu-protected) wave executor.
func TestCollectDataflowLeavesAcceptsOnlyFlatTrees(t *testing.T) {
	single := func() *FetchTreeNode { return Single(&SingleFetch{}) }
	tests := []struct {
		name      string
		node      *FetchTreeNode
		wantOK    bool
		wantCount int
	}{
		{name: "nil", node: nil, wantOK: true, wantCount: 0},
		{name: "bare single", node: single(), wantOK: true, wantCount: 1},
		{name: "flat sequence", node: Sequence(single(), single()), wantOK: true, wantCount: 2},
		{name: "sequence with parallel of singles", node: Sequence(single(), Parallel(single(), single())), wantOK: true, wantCount: 3},
		// root Parallel is not createParallelNodes output (root is always a Sequence)
		{name: "root parallel", node: Parallel(single(), single()), wantOK: false, wantCount: 0},
		{name: "parallel containing sequence", node: Sequence(single(), Parallel(Sequence(single(), single()))), wantOK: false, wantCount: 0},
		{name: "sequence containing sequence", node: Sequence(Sequence(single())), wantOK: false, wantCount: 0},
		{name: "single without fetch", node: &FetchTreeNode{Kind: FetchTreeNodeKindSingle, Item: &FetchItem{}}, wantOK: false, wantCount: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			leaves, ok := collectDataflowLeaves(tt.node)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.wantCount, len(leaves))
		})
	}
}

// TestDataflowFallsBackOnNestedScheduleTreePlan asserts the flag-interaction
// contract: with EnableDataflowExecution on, a nested (schedule-tree style) plan
// is structurally rejected by collectDataflowLeaves and runs through the wave
// executor, producing bytes identical to the dataflow-off run.
func TestDataflowFallsBackOnNestedScheduleTreePlan(t *testing.T) {
	run := func(t *testing.T, enableDataflow bool) string {
		t.Helper()
		a := newControlledLoaderDataSource(`{"data":{"a":"A"}}`)
		b := newControlledLoaderDataSource(`{"data":{"b":"B"}}`)
		c := newControlledLoaderDataSource(`{"data":{"c":"C"}}`)
		aFetch := nestedParallelSingleFetch(a, `{"fetch":"A"}`)
		aFetch.FetchDependencies.FetchID = 0
		bFetch := nestedParallelSingleFetch(b, `{"fetch":"B"}`)
		bFetch.FetchDependencies.FetchID = 1
		bFetch.FetchDependencies.DependsOnFetchIDs = []int{0}
		cFetch := nestedParallelSingleFetch(c, `{"fetch":"C"}`)
		cFetch.FetchDependencies.FetchID = 2
		cFetch.FetchDependencies.DependsOnFetchIDs = []int{1}

		response := &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Sequence(
				Single(aFetch),
				Parallel(
					Sequence(Single(bFetch), Single(cFetch)),
				),
			),
			Data: nestedParallelData("a", "b", "c"),
		}

		out, err := runDataflowScenario(t, context.Background(), enableDataflow, ast.OperationTypeQuery, response, nil)
		require.NoError(t, err)
		return out
	}

	withDataflow := run(t, true)
	withoutDataflow := run(t, false)
	require.Equal(t, `{"data":{"a":"A","b":"B","c":"C"}}`, withDataflow)
	require.Equal(t, withoutDataflow, withDataflow)
}
