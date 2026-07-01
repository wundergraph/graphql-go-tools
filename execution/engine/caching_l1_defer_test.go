package engine_test

import (
	"bytes"
	"cmp"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	engine "github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve/cache/cachetesting"
)

func TestCaching_L1Concurrency_M1_LazyBeginRequestOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		query := `{ topProducts { upc name reviews { body } } product(upc:"1") { upc name reviews { body } } }`
		responses := map[string]string{
			"1":             `{"data":{"topProducts":[{"__typename":"Product","upc":"1","name":"Table"}],"product":{"__typename":"Product","upc":"1","name":"Table"}}}`,
			"2:topProducts": `{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]}]}}`,
			"2:product":     `{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]}]}}`,
		}
		pr := engine.Plan(t, cachetesting.StageL1, query, responses)
		deferResp := deferResponseFromPlan(pr)
		store := cachetesting.NewFakeStore()
		begins := &atomic.Int64{}
		controller := &beginCountingCacheController{
			inner:  cachetesting.NewRealishCache(t, cachetesting.ModeL1, store, nil),
			begins: begins,
		}

		writer := resolveGraphQLDeferResponse(t, deferResp, controller)

		assert.Equal(t, int64(1), begins.Load())
		assert.Equal(t, int64(1), pr.Fakes.LoadCount("1", ""))
		assert.Equal(t, int64(1), pr.Fakes.LoadCount("2", "topProducts"))
		assert.Equal(t, int64(1), pr.Fakes.LoadCount("2", "product"))
		assert.Equal(t, []cachetesting.StoreOp(nil), store.Ops())
		assert.Equal(t, []string{
			`{"data":{"topProducts":[{"upc":"1","name":"Table","reviews":[{"body":"Solid"}]}],"product":{"upc":"1","name":"Table","reviews":[{"body":"Solid"}]}},"hasNext":false}`,
		}, writer.Payloads())
		assert.Equal(t, true, writer.CompleteCalled())
	})
}

func TestCaching_L1Concurrency_M2_ParallelWritesSharedL1(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		query := `{ topProducts { upc name reviews { body } } product(upc:"2") { upc name reviews { body } } }`
		responses := map[string]string{
			"1":             `{"data":{"topProducts":[{"__typename":"Product","upc":"1","name":"Table"}],"product":{"__typename":"Product","upc":"2","name":"Chair"}}}`,
			"2:topProducts": `{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]}]}}`,
			"2:product":     `{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Stable"}]}]}}`,
		}
		pr := engine.Plan(t, cachetesting.StageL1, query, responses)
		forceEntityL1(pr.Response.Fetches)
		deferResp := deferResponseFromPlan(pr)

		arrived := make(chan string, 2)
		release := make(chan struct{})
		pr.Fakes.SetGate("2", "topProducts", cachetesting.DataSourceGate{Arrived: arrived, Release: release})
		pr.Fakes.SetGate("2", "product", cachetesting.DataSourceGate{Arrived: arrived, Release: release})
		cachetesting.SwapDataSources(pr.Response.Fetches, pr.Fakes)

		store := cachetesting.NewFakeStore()
		ctx := resolve.NewContext(t.Context())
		ctx.SetCacheController(cachetesting.NewRealishCache(t, cachetesting.ModeL1L2, store, nil))
		writer := &recordingDeferWriter{}
		done := make(chan error, 1)

		go func() {
			r := resolve.New(t.Context(), resolve.ResolverOptions{})
			_, err := r.ResolveGraphQLDeferResponse(ctx, deferResp, writer)
			done <- err
		}()

		arrivals := []string{<-arrived, <-arrived}
		slices.Sort(arrivals)
		assert.Equal(t, []string{"2", "2"}, arrivals)
		close(release)
		synctest.Wait()
		require.NoError(t, <-done)

		assert.Equal(t, int64(1), pr.Fakes.LoadCount("1", ""))
		assert.Equal(t, int64(1), pr.Fakes.LoadCount("2", "topProducts"))
		assert.Equal(t, int64(1), pr.Fakes.LoadCount("2", "product"))
		ops := sortedStoreOps(store.Ops())
		require.Equal(t, 4, len(ops))
		assert.Equal(t, []cachetesting.StoreOp{
			{Kind: "Get", Key: ops[0].Key},
			{Kind: "Get", Key: ops[1].Key},
			{Kind: "Set", Key: ops[2].Key, Value: `{"__typename":"Product","reviews":[{"body":"Solid"}]}`, TTL: time.Minute},
			{Kind: "Set", Key: ops[3].Key, Value: `{"__typename":"Product","reviews":[{"body":"Stable"}]}`, TTL: time.Minute},
		}, ops)
		assert.Equal(t, []string{
			`{"data":{"topProducts":[{"upc":"1","name":"Table","reviews":[{"body":"Solid"}]}],"product":{"upc":"2","name":"Chair","reviews":[{"body":"Stable"}]}},"hasNext":false}`,
		}, writer.Payloads())
		assert.Equal(t, true, writer.CompleteCalled())
	})
}

type beginCountingCacheController struct {
	inner  resolve.CacheController
	begins *atomic.Int64
}

func (c *beginCountingCacheController) BeginRequest(ctx *resolve.Context) resolve.RequestCache {
	c.begins.Add(1)
	return c.inner.BeginRequest(ctx)
}

func deferResponseFromPlan(pr engine.PlanResult) *resolve.GraphQLDeferResponse {
	if pr.DeferResponse != nil {
		return pr.DeferResponse
	}
	return &resolve.GraphQLDeferResponse{Response: pr.Response}
}

func forceEntityL1(node *resolve.FetchTreeNode) {
	if node == nil {
		return
	}
	if node.Item != nil {
		switch fetch := node.Item.Fetch.(type) {
		case *resolve.EntityFetch:
			if fetch.Cache != nil {
				fetch.Cache.L1 = true
			}
		case *resolve.BatchEntityFetch:
			if fetch.Cache != nil {
				fetch.Cache.L1 = true
			}
		}
	}
	for _, child := range node.ChildNodes {
		forceEntityL1(child)
	}
}

func sortedStoreOps(ops []cachetesting.StoreOp) []cachetesting.StoreOp {
	ops = slices.Clone(ops)
	slices.SortFunc(ops, func(a, b cachetesting.StoreOp) int {
		return cmp.Or(
			cmp.Compare(a.Kind, b.Kind),
			cmp.Compare(a.Value, b.Value),
			cmp.Compare(a.Key, b.Key),
		)
	})
	return ops
}

func resolveGraphQLDeferResponse(t *testing.T, resp *resolve.GraphQLDeferResponse, controller resolve.CacheController) *recordingDeferWriter {
	t.Helper()

	ctx := resolve.NewContext(t.Context())
	ctx.SetCacheController(controller)
	writer := &recordingDeferWriter{}
	r := resolve.New(t.Context(), resolve.ResolverOptions{})
	_, err := r.ResolveGraphQLDeferResponse(ctx, resp, writer)
	require.NoError(t, err)
	return writer
}

type recordingDeferWriter struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	payloads []string
	complete atomic.Bool
}

func (w *recordingDeferWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *recordingDeferWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.payloads = append(w.payloads, w.buf.String())
	w.buf.Reset()
	return nil
}

func (w *recordingDeferWriter) Complete() {
	w.complete.Store(true)
}

func (w *recordingDeferWriter) Payloads() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.payloads)
}

func (w *recordingDeferWriter) CompleteCalled() bool {
	return w.complete.Load()
}
