package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type cloneCacheController struct {
	requests []*cloneRequestCache
}

func (c *cloneCacheController) BeginRequest(ctx *Context) RequestCache {
	rc := &cloneRequestCache{id: len(c.requests) + 1}
	c.requests = append(c.requests, rc)
	return rc
}

type cloneRequestCache struct {
	id int
}

func (c *cloneRequestCache) PrepareFetch(in PrepareFetchInput) (Decision, *FetchCacheHandle) {
	return DecisionFetch, nil
}

func (c *cloneRequestCache) OnFetchSkipped(h *FetchCacheHandle, in MergeInput) error {
	return nil
}

func (c *cloneRequestCache) OnFetchResult(h *FetchCacheHandle, in MergeInput) error {
	return nil
}

func (c *cloneRequestCache) EndRequest() {}

func TestContextCloneResetsRequestCacheAndKeepsController(t *testing.T) {
	controller := &cloneCacheController{}
	ctx := NewContext(t.Context())
	ctx.SetCacheController(controller)
	ctx.requestCache = ctx.cacheController.BeginRequest(ctx)

	cloned := ctx.clone(t.Context())

	assert.Equal(t, RequestCache(nil), cloned.requestCache)
	assert.Equal(t, ctx.cacheController, cloned.cacheController)

	cloned.requestCache = cloned.cacheController.BeginRequest(cloned)
	assert.Equal(t, []*cloneRequestCache{{id: 1}, {id: 2}}, controller.requests)
	assert.Equal(t, false, ctx.requestCache == cloned.requestCache)
}
