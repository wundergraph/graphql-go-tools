package resolve

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"
)

// countingCacheController records BeginRequest calls so the no-op gates can be
// asserted observably: with no cache-configured fetch, the loader must never
// reach the controller at all.
type countingCacheController struct {
	beginRequestCalls int
}

func (c *countingCacheController) BeginRequest(ctx *Context) RequestCache {
	c.beginRequestCalls++
	return nil
}

// TestCacheNoOpGates proves the runtime no-op invariant of the loader seam:
// with no controller, and with a controller but no per-fetch cache config, the
// loader output is byte-identical and no cache code runs.
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
		// The per-fetch gate wins before the controller is ever consulted.
		assert.Equal(t, 0, controller.beginRequestCalls)
		assert.Nil(t, ctx.requestCache)
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

// countingRequestCache is a minimal RequestCache fake recording EndRequest
// calls for the lifecycle tests.
type countingRequestCache struct {
	endRequestCalls int
}

func (c *countingRequestCache) PrepareFetch(in PrepareFetchInput) (Decision, *FetchCacheHandle) {
	return DecisionFetch, nil
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
