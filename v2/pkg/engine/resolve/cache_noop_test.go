package resolve

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"
)

// countingCacheController records BeginRequest calls and hands out ONE counting
// request surface, so the no-op gates can be asserted observably: without a
// controller the loader must never reach cache code at all, and with one it
// must reach nothing beyond the uncached-fetch notification.
type countingCacheController struct {
	beginRequestCalls int
	request           *countingRequestCache
}

func (c *countingCacheController) BeginRequest(ctx *Context) RequestCache {
	c.beginRequestCalls++
	if c.request == nil {
		c.request = &countingRequestCache{}
	}
	return c.request
}

// TestCacheNoOpGates proves the runtime no-op invariant of the loader seam: the
// loader output is byte-identical with no controller and with a controller over
// fetches none of which is cache-configured.
func TestCacheNoOpGates(t *testing.T) {
	newResponse := func(ctrl *gomock.Controller) *GraphQLResponse {
		ds := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name}}"}}`,
			`{"data":{"topProducts":[{"name":"Table"},{"name":"Couch"}]}}`)
		return &GraphQLResponse{
			Fetches: Sequence(
				Single(&SingleFetch{
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name}}"}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					FetchConfiguration: FetchConfiguration{
						DataSource: ds,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
				}),
			),
		}
	}

	load := func(t *testing.T, ctx *Context, response *GraphQLResponse) string {
		t.Helper()
		loader := &Loader{dataBuffer: &DataBuffer{data: astjson.ObjectValue(nil)}}
		err := loader.LoadGraphQLResponseData(ctx, response)
		assert.NoError(t, err)
		return fastjsonext.PrintGraphQLResponse(loader.dataBuffer.Get(), loader.errors)
	}

	expected := `{"data":{"topProducts":[{"name":"Table"},{"name":"Couch"}]}}`

	t.Run("no controller", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		ctx := NewContext(t.Context())
		out := load(t, ctx, newResponse(ctrl))
		assert.Equal(t, expected, out)
		assert.Nil(t, ctx.requestCache)
	})

	t.Run("controller set but no fetch is cache-configured", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		controller := &countingCacheController{}
		ctx := NewContext(t.Context())
		ctx.SetCacheController(controller)
		out := load(t, ctx, newResponse(ctrl))
		assert.Equal(t, expected, out)
		// The response is what the controller-free run produced. The one thing
		// the controller hears is that the fetch ran outside the cache — the
		// signal that keeps a response with uncached parts from being advertised
		// as keepable — and no lookup, merge or write hook runs.
		assert.Equal(t, 1, controller.beginRequestCalls)
		assert.Equal(t, countingRequestCache{uncachedFetchCalls: 1}, *controller.request)
	})
}

// TestEndCacheRequestIdempotent pins the request-end lifecycle: EndRequest runs
// once, the surface is reset, and calling endCacheRequest again is a no-op.
func TestEndCacheRequestIdempotent(t *testing.T) {
	ctx := NewContext(t.Context())

	// A Context that never used caching must no-op.
	ctx.endCacheRequest()
	assert.Nil(t, ctx.requestCache)

	rc := &countingRequestCache{}
	ctx.requestCache = rc
	ctx.endCacheRequest()
	ctx.endCacheRequest()
	assert.Equal(t, 1, rc.endRequestCalls)
	assert.Nil(t, ctx.requestCache)
}

// TestContextCloneResetsRequestCache pins subscription-event isolation: a
// cloned resolution keeps the controller port but builds its own per-request
// cache surface.
func TestContextCloneResetsRequestCache(t *testing.T) {
	controller := &countingCacheController{}
	ctx := NewContext(t.Context())
	ctx.SetCacheController(controller)
	ctx.requestCache = &countingRequestCache{}

	cloned := ctx.clone(t.Context())
	assert.Nil(t, cloned.requestCache)
	assert.Same(t, controller, cloned.cacheController)
}

// countingRequestCache is a minimal RequestCache fake recording the hook calls
// the lifecycle and no-op rows assert on.
type countingRequestCache struct {
	endRequestCalls    int
	uncachedFetchCalls int
}

func (c *countingRequestCache) PrepareFetch(in PrepareFetchInput) (Decision, *FetchCacheHandle) {
	return DecisionFetch, nil
}

func (c *countingRequestCache) OnUncachedFetch() {
	c.uncachedFetchCalls++
}

func (c *countingRequestCache) OnFetchSkipped(h *FetchCacheHandle, in MergeInput) error {
	return nil
}

func (c *countingRequestCache) OnFetchResult(h *FetchCacheHandle, in MergeInput) error {
	return nil
}

func (c *countingRequestCache) EndRequest() {
	c.endRequestCalls++
}
