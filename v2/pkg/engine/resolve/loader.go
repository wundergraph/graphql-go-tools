package resolve

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash/v2"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/sync/errgroup"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/errorcodes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
)

const (
	IntrospectionSchemaTypeDataSourceID     = "introspection__schema&__type"
	IntrospectionTypeFieldsDataSourceID     = "introspection__type__fields"
	IntrospectionTypeEnumValuesDataSourceID = "introspection__type__enumValues"
)

type LoaderHooks interface {
	// OnLoad is called before the fetch is executed
	OnLoad(ctx context.Context, ds DataSourceInfo) context.Context
	// OnFinished is called after the fetch has been executed and the response has been processed and merged
	OnFinished(ctx context.Context, ds DataSourceInfo, info *ResponseInfo)
}

type DataSourceInfo struct {
	ID   string
	Name string
}

type ResponseInfo struct {
	StatusCode int
	Err        error
	// Request is the original request that was sent to the subgraph. This should only be used for reading purposes,
	// in order to ensure there aren't memory conflicts, and the body will be nil, as it was read already.
	Request *http.Request
	// ResponseHeaders contains a clone of the headers of the response from the subgraph.
	ResponseHeaders http.Header
	// This should be private as we do not want user's to access the raw responseBody directly
	responseBody []byte
}

func (r *ResponseInfo) GetResponseBody() string {
	return string(r.responseBody)
}

func newResponseInfo(res *result, subgraphErrors map[string]error) *ResponseInfo {
	responseInfo := &ResponseInfo{
		StatusCode:   res.statusCode,
		Err:          subgraphErrors[res.ds.Name],
		responseBody: res.out,
	}
	if res.httpResponseContext != nil {
		// We're using the response.Request here, because the body will be nil (since the response was read) and won't
		// cause a memory leak.
		if res.httpResponseContext.Response != nil {
			if res.httpResponseContext.Response.Request != nil {
				responseInfo.Request = res.httpResponseContext.Response.Request
			}

			if res.httpResponseContext.Response.Header != nil {
				responseInfo.ResponseHeaders = res.httpResponseContext.Response.Header.Clone()
			}
		}

		if responseInfo.Request == nil {
			// In cases where the request errors, the response will be nil, and so we need to get the original request
			responseInfo.Request = res.httpResponseContext.Request
		}
	}

	return responseInfo
}

type result struct {
	postProcessing PostProcessingConfiguration
	// batchStats represents per-unique-batch-item merge targets.
	// Outer slice index corresponds to the unique representation index in the request batch,
	// and the inner slice contains all target values that should be merged with the response at that index.
	//
	// Example:
	// For 4 original items that deduplicate to 2 unique representations, we might have:
	// [
	//
	//	[item0, item2], // merge response[0] into item0 and item2
	//	[item1, item3], // merge response[1] into item1 and item3
	//
	// ]
	batchStats       [][]*astjson.Value
	fetchSkipped     bool
	nestedMergeItems []*result

	statusCode int
	err        error
	ds         DataSourceInfo

	authorizationRejected        bool
	authorizationRejectedReasons []string

	rateLimitRejected       bool
	rateLimitRejectedReason string

	// loaderHookContext used to share data between the OnLoad and OnFinished hooks
	// It should be valid even when OnLoad isn't called
	loaderHookContext context.Context

	httpResponseContext *httpclient.ResponseContext
	// out is the subgraph response body
	out               []byte
	singleFlightStats *singleFlightStats
	tools             *batchEntityTools

	cache              LoaderCache
	cacheMustBeUpdated bool
	l1CacheKeys        []*CacheKey // L1 cache keys (no prefix, used for merging)
	l2CacheKeys        []*CacheKey // L2 cache keys (with subgraph header prefix)
	cacheSkipFetch     bool
	cacheConfig        FetchCacheConfiguration

	// Partial cache loading fields
	partialCacheEnabled bool  // Whether partial loading is enabled for this fetch
	cachedItemIndices   []int // Indices of items fully served from cache
	fetchItemIndices    []int // Indices of items that need to be fetched
}

func (l *Loader) createOrInitResult(res *result, postProcessing PostProcessingConfiguration, info *FetchInfo) *result {
	if res == nil {
		res = &result{}
	}
	res.postProcessing = postProcessing
	if info != nil {
		res.ds = DataSourceInfo{
			ID:   info.DataSourceID,
			Name: info.DataSourceName,
		}
	}
	return res
}

func IsIntrospectionDataSource(dataSourceID string) bool {
	return dataSourceID == IntrospectionSchemaTypeDataSourceID || dataSourceID == IntrospectionTypeFieldsDataSourceID || dataSourceID == IntrospectionTypeEnumValuesDataSourceID
}

type Loader struct {
	resolvable *Resolvable
	ctx        *Context
	info       *GraphQLResponseInfo

	caches map[string]LoaderCache

	propagateSubgraphErrors           bool
	propagateSubgraphStatusCodes      bool
	subgraphErrorPropagationMode      SubgraphErrorPropagationMode
	rewriteSubgraphErrorPaths         bool
	omitSubgraphErrorLocations        bool
	omitSubgraphErrorExtensions       bool
	attachServiceNameToErrorExtension bool

	allowAllErrorExtensionFields bool
	allowedErrorExtensionFields  map[string]struct{}
	defaultErrorExtensionCode    string

	allowedSubgraphErrorFields map[string]struct{}

	apolloRouterCompatibilitySubrequestHTTPError bool

	propagateFetchReasons bool

	validateRequiredExternalFields bool

	taintedObjs taintedObjects

	// jsonArena is the arena to allocation json, supplied by the Resolver
	// Disclaimer: this arena is NOT thread safe!
	// Only use from main goroutine
	// Don't Reset or Release, the Resolver handles this
	// Disclaimer: When parsing json into the arena, the underlying bytes must also be allocated on the arena!
	// This is very important to "tie" their lifecycles together
	// If you're not doing this, you will see segfaults
	// Example of correct usage in func "mergeResult"
	jsonArena arena.Arena

	// singleFlight is the SubgraphRequestSingleFlight object shared across all client requests.
	// It's thread safe and can be used to de-duplicate subgraph requests.
	singleFlight *SubgraphRequestSingleFlight

	// l1Cache is the per-request entity cache (L1).
	// Key: cache key string (WITHOUT subgraph header prefix)
	// Value: *astjson.Value pointer to entity in jsonArena
	// Thread-safe via sync.Map for parallel fetch support.
	// Only used for entity fetches, NOT root fetches (root fields have no prior entity data).
	l1Cache sync.Map
}

func (l *Loader) Free() {
	l.info = nil
	l.ctx = nil
	l.resolvable = nil
	l.taintedObjs = nil
}

func (l *Loader) LoadGraphQLResponseData(ctx *Context, response *GraphQLResponse, resolvable *Resolvable) (err error) {
	l.resolvable = resolvable
	l.ctx = ctx
	l.info = response.Info
	l.taintedObjs = make(taintedObjects)
	return l.resolveFetchNode(response.Fetches)
}

func (l *Loader) resolveFetchNode(node *FetchTreeNode) error {
	if node == nil {
		return nil
	}
	switch node.Kind {
	case FetchTreeNodeKindSingle:
		return l.resolveSingle(node.Item)
	case FetchTreeNodeKindSequence:
		return l.resolveSerial(node.ChildNodes)
	case FetchTreeNodeKindParallel:
		return l.resolveParallel(node.ChildNodes)
	default:
		return nil
	}
}

func (l *Loader) resolveParallel(nodes []*FetchTreeNode) error {
	if len(nodes) == 0 {
		return nil
	}
	results := make([]*result, len(nodes))
	defer func() {
		for i := range results {
			// no-op if tools == nil
			batchEntityToolPool.Put(results[i].tools)
		}
	}()
	itemsItems := make([][]*astjson.Value, len(nodes))

	// Phase 1: Prepare cache keys + L1 check on MAIN thread for ALL nodes
	// L1 stats use non-atomic operations, so they MUST be on the main thread
	for i := range nodes {
		results[i] = &result{}
		itemsItems[i] = l.selectItemsForPath(nodes[i].Item.FetchPath)
		f := nodes[i].Item.Fetch
		info := getFetchInfo(f)
		cfg := getFetchCaching(f)

		// Set partial loading flag BEFORE cache lookup so tracking arrays are populated
		results[i].partialCacheEnabled = cfg.EnablePartialCacheLoad

		// Prepare cache keys for L1 and L2
		isEntityFetch, err := l.prepareCacheKeys(info, cfg, itemsItems[i], results[i])
		if err != nil {
			return errors.WithStack(err)
		}

		// L1 Check (main thread only - not thread-safe)
		if isEntityFetch && l.ctx.ExecutionOptions.Caching.EnableL1Cache && len(results[i].l1CacheKeys) > 0 {
			allComplete := l.tryL1CacheLoad(info, results[i].l1CacheKeys, results[i])
			if allComplete {
				// All entities found in L1 - mark to skip goroutine
				results[i].cacheSkipFetch = true
			} else if results[i].partialCacheEnabled && len(results[i].cachedItemIndices) > 0 {
				// Partial hit with partial loading enabled - keep FromCache values
				// Continue to L2/fetch for remaining items
			} else {
				// All-or-nothing mode OR no hits - clear FromCache for L2 to try
				for _, ck := range results[i].l1CacheKeys {
					ck.FromCache = nil
				}
				results[i].cachedItemIndices = nil
				results[i].fetchItemIndices = nil
			}
		}
	}

	// Phase 2: Parallel L2 + fetch for nodes that didn't fully hit L1
	// L2 stats use atomic operations - thread-safe
	g, ctx := errgroup.WithContext(l.ctx.ctx)
	for i := range nodes {
		i := i
		f := nodes[i].Item.Fetch
		item := nodes[i].Item
		items := itemsItems[i]
		res := results[i]

		// Skip goroutine if L1 was a complete hit
		if res.cacheSkipFetch {
			continue
		}

		g.Go(func() error {
			return l.loadFetchL2Only(ctx, f, item, items, res)
		})
	}
	err := g.Wait()
	if err != nil {
		return errors.WithStack(err)
	}

	// Phase 3: Merge results (main thread)
	for i := range results {
		if results[i].nestedMergeItems != nil {
			for j := range results[i].nestedMergeItems {
				err = l.mergeResult(nodes[i].Item, results[i].nestedMergeItems[j], itemsItems[i][j:j+1])
				if l.ctx.LoaderHooks != nil && results[i].nestedMergeItems[j].loaderHookContext != nil {
					l.ctx.LoaderHooks.OnFinished(results[i].nestedMergeItems[j].loaderHookContext,
						results[i].nestedMergeItems[j].ds,
						newResponseInfo(results[i].nestedMergeItems[j], l.ctx.subgraphErrors))
				}
				if err != nil {
					return errors.WithStack(err)
				}
			}
		} else {
			err = l.mergeResult(nodes[i].Item, results[i], itemsItems[i])
			if l.ctx.LoaderHooks != nil {
				l.ctx.LoaderHooks.OnFinished(results[i].loaderHookContext, results[i].ds, newResponseInfo(results[i], l.ctx.subgraphErrors))
			}
			if err != nil {
				return errors.WithStack(err)
			}
		}
	}
	return nil
}

func (l *Loader) resolveSerial(nodes []*FetchTreeNode) error {
	for i := range nodes {
		err := l.resolveFetchNode(nodes[i])
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (l *Loader) resolveSingle(item *FetchItem) error {
	if item == nil {
		return nil
	}
	items := l.selectItemsForPath(item.FetchPath)

	switch f := item.Fetch.(type) {
	case *SingleFetch:
		res := l.createOrInitResult(nil, f.PostProcessing, f.Info)
		skip, err := l.tryCacheLoad(l.ctx.ctx, f.Info, f.Caching, items, res)
		if err != nil {
			return errors.WithStack(err)
		}
		if !skip {
			err = l.loadSingleFetch(l.ctx.ctx, f, item, items, res)
			if err != nil {
				return err
			}
		}
		err = l.mergeResult(item, res, items)
		if l.ctx.LoaderHooks != nil {
			l.ctx.LoaderHooks.OnFinished(res.loaderHookContext, res.ds, newResponseInfo(res, l.ctx.subgraphErrors))
		}
		return err
	case *BatchEntityFetch:
		res := l.createOrInitResult(nil, f.PostProcessing, f.Info)
		defer batchEntityToolPool.Put(res.tools)
		skip, err := l.tryCacheLoad(l.ctx.ctx, f.Info, f.Caching, items, res)
		if err != nil {
			return errors.WithStack(err)
		}
		if !skip {
			err = l.loadBatchEntityFetch(l.ctx.ctx, item, f, items, res)
			if err != nil {
				return errors.WithStack(err)
			}
		}
		err = l.mergeResult(item, res, items)
		if l.ctx.LoaderHooks != nil {
			l.ctx.LoaderHooks.OnFinished(res.loaderHookContext, res.ds, newResponseInfo(res, l.ctx.subgraphErrors))
		}
		return err
	case *EntityFetch:
		res := l.createOrInitResult(nil, f.PostProcessing, f.Info)
		skip, err := l.tryCacheLoad(l.ctx.ctx, f.Info, f.Caching, items, res)
		if err != nil {
			return errors.WithStack(err)
		}
		if !skip {
			err = l.loadEntityFetch(l.ctx.ctx, item, f, items, res)
			if err != nil {
				return errors.WithStack(err)
			}
		}
		err = l.mergeResult(item, res, items)
		if l.ctx.LoaderHooks != nil {
			l.ctx.LoaderHooks.OnFinished(res.loaderHookContext, res.ds, newResponseInfo(res, l.ctx.subgraphErrors))
		}
		return err
	default:
		return nil
	}
}

func (l *Loader) selectItemsForPath(path []FetchItemPathElement) []*astjson.Value {
	// Use arena allocation for the initial items slice
	items := arena.AllocateSlice[*astjson.Value](l.jsonArena, 1, 1)
	items[0] = l.resolvable.data
	if len(path) == 0 {
		return l.taintedObjs.filterOutTainted(items)
	}
	for i := range path {
		if len(items) == 0 {
			break
		}
		items = selectItems(l.jsonArena, items, path[i])
	}
	return l.taintedObjs.filterOutTainted(items)
}

func isItemAllowedByTypename(obj *astjson.Value, typeNames []string) bool {
	if len(typeNames) == 0 {
		return true
	}
	if obj == nil || obj.Type() != astjson.TypeObject {
		return true
	}
	__typeName := obj.GetStringBytes("__typename")
	if __typeName == nil {
		return true
	}

	__typeNameStr := string(__typeName)
	return slices.Contains(typeNames, __typeNameStr)
}

func selectItems(a arena.Arena, items []*astjson.Value, element FetchItemPathElement) []*astjson.Value {
	if len(items) == 0 {
		return nil
	}
	if len(element.Path) == 0 {
		return items
	}

	if len(items) == 1 {
		if !isItemAllowedByTypename(items[0], element.TypeNames) {
			return nil
		}

		field := items[0].Get(element.Path...)
		if field == nil {
			return nil
		}
		if field.Type() == astjson.TypeArray {
			return field.GetArray()
		}
		return []*astjson.Value{field}
	}
	selected := arena.AllocateSlice[*astjson.Value](a, 0, len(items))
	for _, item := range items {
		if !isItemAllowedByTypename(item, element.TypeNames) {
			continue
		}
		field := item.Get(element.Path...)
		if field == nil {
			continue
		}
		if field.Type() == astjson.TypeArray {
			selected = arena.SliceAppend(a, selected, field.GetArray()...)
			continue
		}
		selected = arena.SliceAppend(a, selected, field)
	}
	return selected
}

func (l *Loader) itemsData(items []*astjson.Value) *astjson.Value {
	if len(items) == 0 {
		return astjson.NullValue
	}
	if len(items) == 1 {
		return items[0]
	}
	// previously, we used: l.resolvable.astjsonArena.NewArray()
	// however, itemsData can be called concurrently, so this might result in a race
	arr := astjson.MustParseBytes([]byte(`[]`))
	for i, item := range items {
		arr.SetArrayItem(nil, i, item)
	}
	return arr
}

type CacheEntry struct {
	Key   string
	Value []byte
}

type LoaderCache interface {
	Get(ctx context.Context, keys []string) ([]*CacheEntry, error)
	Set(ctx context.Context, entries []*CacheEntry, ttl time.Duration) error
	Delete(ctx context.Context, keys []string) error
}

// extractCacheKeysStrings extracts all unique cache key strings from CacheKeys
// If includePrefix is true and subgraphName is provided, keys are prefixed with the subgraph header hash.
func (l *Loader) extractCacheKeysStrings(a arena.Arena, cacheKeys []*CacheKey) []string {
	if len(cacheKeys) == 0 {
		return nil
	}
	out := arena.AllocateSlice[string](a, 0, len(cacheKeys))
	for i := range cacheKeys {
		for j := range cacheKeys[i].Keys {
			keyLen := len(cacheKeys[i].Keys[j])
			key := arena.AllocateSlice[byte](a, 0, keyLen)
			key = arena.SliceAppend(a, key, unsafebytes.StringToBytes(cacheKeys[i].Keys[j])...)
			out = arena.SliceAppend(a, out, unsafebytes.BytesToString(key))
		}
	}
	return out
}

// populateFromCache populates CacheKey.FromCache fields from cache entries
// If includePrefix is true and subgraphName is provided, keys are looked up with the subgraph header hash prefix.
func (l *Loader) populateFromCache(a arena.Arena, cacheKeys []*CacheKey, entries []*CacheEntry) (err error) {
	for i := range entries {
		if entries[i] == nil || entries[i].Value == nil {
			continue
		}
		for j := range cacheKeys {
			for k := range cacheKeys[j].Keys {
				if cacheKeys[j].Keys[k] == entries[i].Key {
					cacheKeys[j].FromCache, err = astjson.ParseBytesWithArena(a, entries[i].Value)
					if err != nil {
						return errors.WithStack(err)
					}
				}
			}
		}
	}
	return nil
}

// cacheKeysToEntries converts CacheKeys to CacheEntries for storage
// For each CacheKey, creates entries for all its KeyEntries with the same value
// If includePrefix is true and subgraphName is provided, keys are prefixed with the subgraph header hash.
func (l *Loader) cacheKeysToEntries(a arena.Arena, cacheKeys []*CacheKey) ([]*CacheEntry, error) {
	out := arena.AllocateSlice[*CacheEntry](a, 0, len(cacheKeys))
	buf := arena.AllocateSlice[byte](a, 64, 64)
	for i := range cacheKeys {
		for j := range cacheKeys[i].Keys {
			if cacheKeys[i].Item == nil {
				continue
			}
			buf = cacheKeys[i].Item.MarshalTo(buf[:0])
			entry := &CacheEntry{
				Key:   cacheKeys[i].Keys[j],
				Value: arena.AllocateSlice[byte](a, len(buf), len(buf)),
			}
			copy(entry.Value, buf)
			out = arena.SliceAppend(a, out, entry)
		}
	}
	return out, nil
}

// prepareCacheKeys generates cache keys for L1 and/or L2 based on configuration.
// Called on main thread before any cache lookups.
// Sets res.l1CacheKeys for L1 lookup (no prefix) and res.l2CacheKeys for L2 lookup (with prefix).
// Returns isEntityFetch to indicate if this fetch supports L1 caching.
func (l *Loader) prepareCacheKeys(info *FetchInfo, cfg FetchCacheConfiguration, inputItems []*astjson.Value, res *result) (isEntityFetch bool, err error) {
	if cfg.CacheKeyTemplate == nil {
		return false, nil
	}

	// Skip all cache operations if both L1 and L2 are disabled
	if !l.ctx.ExecutionOptions.Caching.EnableL1Cache && !l.ctx.ExecutionOptions.Caching.EnableL2Cache {
		return false, nil
	}

	res.cacheConfig = cfg

	// Check if this is an entity fetch (L1 only applies to entity fetches)
	entityTemplate, isEntity := cfg.CacheKeyTemplate.(*EntityQueryCacheKeyTemplate)

	// Always generate cache keys (needed for merging cached data into response)
	// For entity fetches: uses L1-style keys (no prefix)
	// For root fetches: uses regular keys (no prefix)
	if isEntity {
		res.l1CacheKeys, err = entityTemplate.RenderL1CacheKeys(l.jsonArena, l.ctx, inputItems)
	} else {
		res.l1CacheKeys, err = cfg.CacheKeyTemplate.RenderCacheKeys(l.jsonArena, l.ctx, inputItems, "")
	}
	if err != nil {
		return false, err
	}

	// Generate L2 keys (with prefix for cache isolation)
	if l.ctx.ExecutionOptions.Caching.EnableL2Cache {
		// Get cache first to ensure it exists
		if l.caches != nil {
			res.cache = l.caches[cfg.CacheName]
		}
		if res.cache != nil {
			// Calculate prefix for L2 (subgraph header isolation)
			var prefix string
			if cfg.IncludeSubgraphHeaderPrefix && l.ctx.SubgraphHeadersBuilder != nil {
				_, headersHash := l.ctx.SubgraphHeadersBuilder.HeadersForSubgraph(info.DataSourceName)
				var buf [20]byte
				b := strconv.AppendUint(buf[:0], headersHash, 10)
				prefix = string(b)
			}

			// Render L2 cache keys with prefix
			if isEntity {
				res.l2CacheKeys, err = entityTemplate.RenderL2CacheKeys(l.jsonArena, l.ctx, inputItems, prefix)
			} else {
				res.l2CacheKeys, err = cfg.CacheKeyTemplate.RenderCacheKeys(l.jsonArena, l.ctx, inputItems, prefix)
			}
			if err != nil {
				return false, err
			}
		}
	}

	return isEntity, nil
}

// tryCacheLoad orchestrates cache lookups for sequential execution paths.
// Uses the 3-function approach: prepareCacheKeys -> tryL1CacheLoad -> tryL2CacheLoad
// Returns skipFetch=true if cache provides complete data.
//
// IMPORTANT: This function is for SEQUENTIAL execution only (main thread).
// For PARALLEL execution, use prepareCacheKeys + tryL1CacheLoad on main thread,
// then tryL2CacheLoad in goroutines.
//
// Lookup Order (entity fetches): L1 -> L2 -> Subgraph Fetch
// Lookup Order (root fetches): L2 -> Subgraph Fetch (no L1)
func (l *Loader) tryCacheLoad(ctx context.Context, info *FetchInfo, cfg FetchCacheConfiguration, inputItems []*astjson.Value, res *result) (skipFetch bool, err error) {
	// Step 1: Prepare cache keys for L1 and L2
	isEntityFetch, err := l.prepareCacheKeys(info, cfg, inputItems, res)
	if err != nil {
		return false, err
	}

	// No cache keys generated - nothing to do
	if len(res.l1CacheKeys) == 0 && len(res.l2CacheKeys) == 0 {
		return false, nil
	}

	// Set partial loading flag BEFORE cache lookup so tracking arrays are populated
	res.partialCacheEnabled = cfg.EnablePartialCacheLoad

	// Step 2: L1 Check (per-request, in-memory) - entity fetches only
	// Safe to call: this is sequential execution on main thread
	if isEntityFetch && l.ctx.ExecutionOptions.Caching.EnableL1Cache && len(res.l1CacheKeys) > 0 {
		allComplete := l.tryL1CacheLoad(info, res.l1CacheKeys, res)
		if allComplete {
			// All entities found in L1 with complete data - skip fetch
			res.cacheSkipFetch = true
			return true, nil
		}

		if res.partialCacheEnabled && len(res.cachedItemIndices) > 0 {
			// Partial hit with partial loading enabled
			// cachedItemIndices and fetchItemIndices already populated by tryL1CacheLoad
			// Keep FromCache values for cached items, proceed to fetch only missing items
			res.cacheMustBeUpdated = true
			return false, nil
		}

		// All-or-nothing mode OR no hits - clear FromCache and try L2
		for _, ck := range res.l1CacheKeys {
			ck.FromCache = nil
		}
		res.cachedItemIndices = nil
		res.fetchItemIndices = nil
	}

	// Step 3: L2 Check (external cache) - if L1 missed
	// Safe to call: this is sequential execution on main thread
	if l.ctx.ExecutionOptions.Caching.EnableL2Cache && len(res.l2CacheKeys) > 0 {
		skipFetch, err = l.tryL2CacheLoad(ctx, info, res)
		if err != nil || skipFetch {
			return skipFetch, err
		}

		if res.partialCacheEnabled && len(res.cachedItemIndices) > 0 {
			// Partial hit from L2 with partial loading enabled
			// Keep FromCache values, return false to proceed with fetch for missing items
			return false, nil
		}
	}

	// Both missed - fetch required
	res.cacheMustBeUpdated = true
	return false, nil
}

// tryL1CacheLoad attempts to load all items from the L1 (per-request) cache.
// MUST be called from main thread only (L1 stats are not atomic).
// Tracks per-entity hits/misses: HIT if entity found with complete data, MISS otherwise.
// Returns true only if ALL items are found in cache with complete data for the fetch.
// L1 uses cache keys WITHOUT subgraph header prefix (same request context).
// NOTE: Only called for entity fetches, not root fetches.
// When res.partialCacheEnabled is true, populates res.cachedItemIndices and res.fetchItemIndices
// to track which items were cached vs need fetching.
func (l *Loader) tryL1CacheLoad(info *FetchInfo, cacheKeys []*CacheKey, res *result) bool {
	if info == nil || info.OperationType != ast.OperationTypeQuery {
		return false
	}

	allComplete := true
	for i, ck := range cacheKeys {
		var foundComplete bool
		for _, keyStr := range ck.Keys {
			if cached, ok := l.l1Cache.Load(keyStr); ok {
				cachedValue := cached.(*astjson.Value)
				// Check if cached entity has all required fields for this fetch
				if info.ProvidesData != nil && l.validateItemHasRequiredData(cachedValue, info.ProvidesData) {
					// Entity found with complete data - L1 HIT
					// Use shallow copy to prevent pointer aliasing with self-referential entities
					ck.FromCache = l.shallowCopyProvidedFields(cachedValue, info.ProvidesData)
					l.ctx.trackL1Hit()
					foundComplete = true
					break
				}
			}
		}

		if foundComplete {
			// Track cached item index when partial loading enabled
			if res.partialCacheEnabled {
				res.cachedItemIndices = append(res.cachedItemIndices, i)
			}
		} else {
			allComplete = false
			l.ctx.trackL1Miss()
			// Track fetch item index when partial loading enabled
			if res.partialCacheEnabled {
				res.fetchItemIndices = append(res.fetchItemIndices, i)
			}
		}
	}
	return allComplete
}

// tryL2CacheLoad checks the external (L2) cache for entity data.
// Thread-safe: can be called from parallel goroutines (uses atomic L2 stats).
// Expects res.l2CacheKeys to be pre-populated by prepareCacheKeys().
// Uses subgraph header prefix for cache key isolation across different configurations.
func (l *Loader) tryL2CacheLoad(ctx context.Context, info *FetchInfo, res *result) (skipFetch bool, err error) {
	// L2 keys should be pre-populated by prepareCacheKeys
	if len(res.l2CacheKeys) == 0 || res.cache == nil {
		res.cacheMustBeUpdated = true
		return false, nil
	}

	cacheKeyStrings := l.extractCacheKeysStrings(l.jsonArena, res.l2CacheKeys)
	if len(cacheKeyStrings) == 0 {
		res.cacheMustBeUpdated = true
		return false, nil
	}

	// Get cache entries from L2
	cacheEntries, err := res.cache.Get(ctx, cacheKeyStrings)
	if err != nil {
		// L2 cache errors are non-fatal, continue to fetch
		res.cacheMustBeUpdated = true
		return false, nil
	}

	// Populate FromCache fields in L2 CacheKeys (which have prefixed keys)
	err = l.populateFromCache(l.jsonArena, res.l2CacheKeys, cacheEntries)
	if err != nil {
		res.cacheMustBeUpdated = true
		return false, nil
	}

	// Copy FromCache values from L2 keys to L1 keys (if L1 keys exist) and track per-entity hits/misses
	// The keys have the same structure, just different key strings
	allComplete := true
	if len(res.l1CacheKeys) > 0 {
		// Entity fetch with L1 keys - copy to L1 keys for merging
		for i := range res.l1CacheKeys {
			if i < len(res.l2CacheKeys) {
				res.l1CacheKeys[i].FromCache = res.l2CacheKeys[i].FromCache
				// Track per-entity L2 hit/miss (atomic operations - thread-safe)
				if res.l1CacheKeys[i].FromCache != nil {
					if info != nil && info.ProvidesData != nil && l.validateItemHasRequiredData(res.l1CacheKeys[i].FromCache, info.ProvidesData) {
						l.ctx.trackL2Hit()
						// Track cached item index when partial loading enabled
						if res.partialCacheEnabled {
							res.cachedItemIndices = append(res.cachedItemIndices, i)
						}
					} else {
						l.ctx.trackL2Miss()
						allComplete = false
						// Track fetch item index when partial loading enabled
						if res.partialCacheEnabled {
							res.fetchItemIndices = append(res.fetchItemIndices, i)
						}
					}
				} else {
					l.ctx.trackL2Miss()
					allComplete = false
					// Track fetch item index when partial loading enabled
					if res.partialCacheEnabled {
						res.fetchItemIndices = append(res.fetchItemIndices, i)
					}
				}
			}
		}
	} else {
		// Root fetch (no L1 keys) - track directly from L2 keys
		for i, ck := range res.l2CacheKeys {
			if ck.FromCache != nil {
				if info != nil && info.ProvidesData != nil && l.validateItemHasRequiredData(ck.FromCache, info.ProvidesData) {
					l.ctx.trackL2Hit()
					// Track cached item index when partial loading enabled
					if res.partialCacheEnabled {
						res.cachedItemIndices = append(res.cachedItemIndices, i)
					}
				} else {
					l.ctx.trackL2Miss()
					allComplete = false
					// Track fetch item index when partial loading enabled
					if res.partialCacheEnabled {
						res.fetchItemIndices = append(res.fetchItemIndices, i)
					}
				}
			} else {
				l.ctx.trackL2Miss()
				allComplete = false
				// Track fetch item index when partial loading enabled
				if res.partialCacheEnabled {
					res.fetchItemIndices = append(res.fetchItemIndices, i)
				}
			}
		}
	}

	if allComplete {
		res.cacheSkipFetch = true
		return true, nil
	}

	res.cacheMustBeUpdated = true
	return false, nil
}

// populateL1Cache stores entity data in the L1 (per-request) cache for later reuse.
// Called after successful fetch and merge for entity fetches only.
// OPTIMIZATION: Only stores if key is missing - existing entries are pointers
// to the same arena data, so no update needed. This minimizes sync.Map calls.
func (l *Loader) populateL1Cache(fetchItem *FetchItem, res *result, items []*astjson.Value) {
	if !l.ctx.ExecutionOptions.Caching.EnableL1Cache {
		return
	}
	for _, ck := range res.l1CacheKeys {
		if ck.Item == nil {
			continue
		}
		for _, keyStr := range ck.Keys {
			// LoadOrStore only writes if key is missing, minimizing map operations
			l.l1Cache.LoadOrStore(keyStr, ck.Item)
		}
	}
	// Also populate L1 cache for root fields that return entities
	l.populateL1CacheForRootFieldEntities(fetchItem)
}

// populateL1CacheForRootFieldEntities populates the L1 cache with entities returned by root fields.
// This allows subsequent entity fetches to benefit from L1 cache hits when the same entities
// were already fetched as part of a root field query.
func (l *Loader) populateL1CacheForRootFieldEntities(fetchItem *FetchItem) {
	// Only applies to SingleFetch (root field fetches)
	singleFetch, ok := fetchItem.Fetch.(*SingleFetch)
	if !ok {
		return
	}

	templates := singleFetch.Caching.RootFieldL1EntityCacheKeyTemplates
	if len(templates) == 0 {
		return
	}

	// Get response data
	data := l.resolvable.data
	if data == nil {
		return
	}

	// Get the path from any template to find where entities are located
	// (all templates for the same root field have the same path)
	var fieldPath []string
	for _, template := range templates {
		entityTemplate, ok := template.(*EntityQueryCacheKeyTemplate)
		if !ok || entityTemplate.L1Keys == nil || entityTemplate.L1Keys.Renderer == nil {
			continue
		}
		obj, ok := entityTemplate.L1Keys.Renderer.Node.(*Object)
		if !ok {
			continue
		}
		fieldPath = obj.Path
		break
	}

	if len(fieldPath) == 0 {
		return
	}

	// Navigate to the entities using the path
	entitiesValue := data.Get(fieldPath...)
	if entitiesValue == nil {
		return
	}

	// Handle both single entity (object) and array of entities
	var entities []*astjson.Value
	switch entitiesValue.Type() {
	case astjson.TypeArray:
		entities = entitiesValue.GetArray()
	case astjson.TypeObject:
		entities = []*astjson.Value{entitiesValue}
	default:
		return
	}

	// For each entity, render cache key and store in L1 cache
	for _, entity := range entities {
		if entity == nil {
			continue
		}

		// Extract __typename to find the right template
		typenameValue := entity.Get("__typename")
		if typenameValue == nil {
			continue
		}
		typename := string(typenameValue.GetStringBytes())

		// Look up template for this typename
		template, ok := templates[typename]
		if !ok {
			continue
		}

		entityTemplate, ok := template.(*EntityQueryCacheKeyTemplate)
		if !ok {
			continue
		}

		// Render cache key(s) for this entity
		cacheKeys, err := entityTemplate.RenderL1CacheKeys(l.jsonArena, l.ctx, []*astjson.Value{entity})
		if err != nil || len(cacheKeys) == 0 {
			continue
		}

		// Store in L1 cache
		for _, ck := range cacheKeys {
			if ck == nil {
				continue
			}
			for _, keyStr := range ck.Keys {
				// Use the entity directly as the cache value
				l.l1Cache.LoadOrStore(keyStr, entity)
			}
		}
	}
}

// getFetchInfo extracts FetchInfo from a Fetch interface
func getFetchInfo(fetch Fetch) *FetchInfo {
	switch f := fetch.(type) {
	case *SingleFetch:
		return f.Info
	case *EntityFetch:
		return f.Info
	case *BatchEntityFetch:
		return f.Info
	}
	return nil
}

// getFetchCaching extracts FetchCacheConfiguration from a Fetch interface
func getFetchCaching(fetch Fetch) FetchCacheConfiguration {
	switch f := fetch.(type) {
	case *SingleFetch:
		return f.Caching
	case *EntityFetch:
		return f.Caching
	case *BatchEntityFetch:
		return f.Caching
	}
	return FetchCacheConfiguration{}
}

// loadFetchL2Only loads data assuming L1 cache has already been checked on main thread.
// Used by resolveParallel to avoid L1 access from goroutines (L1 stats are not thread-safe).
// If res.cacheSkipFetch is true, returns immediately (L1 hit).
// Otherwise checks L2 cache (thread-safe) and performs actual fetch if needed.
func (l *Loader) loadFetchL2Only(ctx context.Context, fetch Fetch, fetchItem *FetchItem, items []*astjson.Value, res *result) error {
	// If L1 was a complete hit, skip everything
	if res.cacheSkipFetch {
		return nil
	}

	info := getFetchInfo(fetch)

	// Check L2 cache (thread-safe - uses atomic stats)
	if l.ctx.ExecutionOptions.Caching.EnableL2Cache && len(res.l2CacheKeys) > 0 {
		skip, err := l.tryL2CacheLoad(ctx, info, res)
		if err != nil {
			return errors.WithStack(err)
		}
		if skip {
			return nil
		}
	}

	// Perform actual fetch
	switch f := fetch.(type) {
	case *SingleFetch:
		res = l.createOrInitResult(res, f.PostProcessing, f.Info)
		return l.loadSingleFetch(ctx, f, fetchItem, items, res)
	case *EntityFetch:
		res = l.createOrInitResult(res, f.PostProcessing, f.Info)
		return l.loadEntityFetch(ctx, fetchItem, f, items, res)
	case *BatchEntityFetch:
		res = l.createOrInitResult(res, f.PostProcessing, f.Info)
		return l.loadBatchEntityFetch(ctx, fetchItem, f, items, res)
	}
	return nil
}

type ErrMergeResult struct {
	Subgraph string
	Reason   error
	Path     string
}

func (e ErrMergeResult) Error() string {
	if errors.Is(e.Reason, astjson.ErrMergeDifferingArrayLengths) {
		if e.Path == "" {
			return fmt.Sprintf("unable to merge results from subgraph %s: differing array lengths", e.Subgraph)
		}
		return fmt.Sprintf("unable to merge results from subgraph '%s' at path '%s': differing array lengths", e.Subgraph, e.Path)
	}
	if errors.Is(e.Reason, astjson.ErrMergeDifferentTypes) {
		if e.Path == "" {
			return fmt.Sprintf("unable to merge results from subgraph %s: differing types", e.Subgraph)
		}
		return fmt.Sprintf("unable to merge results from subgraph '%s' at path '%s': differing types", e.Subgraph, e.Path)
	}
	return fmt.Sprintf("unable to merge results from subgraph %s", e.Subgraph)
}

func (l *Loader) mergeResult(fetchItem *FetchItem, res *result, items []*astjson.Value) error {
	if res.err != nil {
		return l.renderErrorsFailedToFetch(fetchItem, res, failedToFetchNoReason)
	}
	if rejected, err := l.evaluateRejected(fetchItem, res, items); err != nil || rejected {
		return err
	}
	if res.cacheSkipFetch {
		// Merge cached data into items
		for _, key := range res.l1CacheKeys {
			// Merge cached data into item
			_, _, err := astjson.MergeValues(l.jsonArena, key.Item, key.FromCache)
			if err != nil {
				return l.renderErrorsFailedToFetch(fetchItem, res, "invalid cache item")
			}
		}
		return nil
	}

	// Handle partial cache loading: merge cached items first
	if res.partialCacheEnabled && len(res.cachedItemIndices) > 0 {
		for _, idx := range res.cachedItemIndices {
			if idx < len(res.l1CacheKeys) && res.l1CacheKeys[idx] != nil && res.l1CacheKeys[idx].FromCache != nil {
				_, _, err := astjson.MergeValues(l.jsonArena, res.l1CacheKeys[idx].Item, res.l1CacheKeys[idx].FromCache)
				if err != nil {
					return l.renderErrorsFailedToFetch(fetchItem, res, "invalid cache item")
				}
			}
		}
	}
	if res.fetchSkipped {
		return nil
	}
	if len(res.out) == 0 {
		return l.renderErrorsFailedToFetch(fetchItem, res, emptyGraphQLResponse)
	}
	// before parsing bytes with an arena.Arena, it's important to first allocate the bytes ON the same arena.Arena
	// this ties their lifecycles together
	// if you don't do this, you'll get segfaults
	slice := arena.AllocateSlice[byte](l.jsonArena, len(res.out), len(res.out))
	copy(slice, res.out)
	response, err := astjson.ParseBytesWithArena(l.jsonArena, slice)
	if err != nil {
		// Fall back to status code if parsing fails and non-2XX
		if (res.statusCode > 0 && res.statusCode < 200) || res.statusCode >= 300 {
			return l.renderErrorsStatusFallback(fetchItem, res, res.statusCode)
		}
		return l.renderErrorsFailedToFetch(fetchItem, res, invalidGraphQLResponse)
	}

	var responseData *astjson.Value
	if res.postProcessing.SelectResponseDataPath != nil {
		responseData = response.Get(res.postProcessing.SelectResponseDataPath...)
	} else {
		responseData = response
	}

	hasErrors := false

	var taintedIndices []int
	// Check if the subgraph response has errors.
	if res.postProcessing.SelectResponseErrorsPath != nil {
		responseErrors := response.Get(res.postProcessing.SelectResponseErrorsPath...)
		if astjson.ValueIsNonNull(responseErrors) {
			hasErrors = len(responseErrors.GetArray()) > 0
			// If the response has the "errors" key, and its value is empty,
			// we don't consider it as an error. Note: it is not compliant with graphql spec.
			if hasErrors {
				if l.validateRequiredExternalFields && res.postProcessing.SelectResponseDataPath != nil {
					taintedIndices = getTaintedIndices(fetchItem.Fetch, responseData, responseErrors)
				}
				if len(taintedIndices) > 0 {
					// Override errors with generic error about missing deps.
					err = l.renderErrorsFailedDeps(fetchItem, res)
					if err != nil {
						return errors.WithStack(err)
					}
				}
				// Look for errors in the response and merge them into the "errors" array.
				err = l.mergeErrors(res, fetchItem, responseErrors)
				if err != nil {
					return errors.WithStack(err)
				}
			}
		}
	}

	// Check if data needs processing.
	if res.postProcessing.SelectResponseDataPath != nil && astjson.ValueIsNull(responseData) {
		// When:
		// - No errors or data are present
		// - Status code is not within the 2XX range
		// We can fall back to a status code based error
		if !hasErrors && ((res.statusCode > 0 && res.statusCode < 200) || res.statusCode >= 300) {
			return l.renderErrorsStatusFallback(fetchItem, res, res.statusCode)
		}

		// If we didn't get any data nor errors, we return an error because the response is invalid
		// Returning an error here also avoids the need to walk over it later.
		if !hasErrors && !l.resolvable.options.ApolloCompatibilitySuppressFetchErrors {
			return l.renderErrorsFailedToFetch(fetchItem, res, invalidGraphQLResponseShape)
		}

		// we have no data but only errors
		// skip value completion
		if hasErrors && l.resolvable.options.ApolloCompatibilityValueCompletionInExtensions {
			l.resolvable.skipValueCompletion = true
		}

		// no data
		return nil
	}
	if len(items) == 0 {
		// If the data is set, it must be an object according to GraphQL over HTTP spec
		if responseData.Type() != astjson.TypeObject {
			return l.renderErrorsFailedToFetch(fetchItem, res, invalidGraphQLResponseShape)
		}
		l.resolvable.data = responseData
		// Only populate caches on success (no errors)
		if !hasErrors {
			l.populateL1Cache(fetchItem, res, items)
			l.updateL2Cache(res)
		}
		return nil
	}
	if len(items) == 1 && res.batchStats == nil {
		items[0], _, err = astjson.MergeValuesWithPath(l.jsonArena, items[0], responseData, res.postProcessing.MergePath...)
		if err != nil {
			return errors.WithStack(ErrMergeResult{
				Subgraph: res.ds.Name,
				Reason:   err,
				Path:     fetchItem.ResponsePath,
			})
		}
		if slices.Contains(taintedIndices, 0) {
			l.taintedObjs.add(items[0])
		}
		// Update cache key item to point to merged data for L1 cache
		if len(res.l1CacheKeys) > 0 && res.l1CacheKeys[0] != nil {
			res.l1CacheKeys[0].Item = items[0]
		}
		// Only populate caches on success (no errors)
		if !hasErrors {
			defer func() {
				l.populateL1Cache(fetchItem, res, items)
				l.updateL2Cache(res)
			}()
		}
		return nil
	}
	batch := responseData.GetArray()
	if batch == nil {
		return l.renderErrorsFailedToFetch(fetchItem, res, invalidGraphQLResponseShape)
	}

	if res.batchStats != nil {
		if len(res.batchStats) != len(batch) {
			return l.renderErrorsFailedToFetch(fetchItem, res, fmt.Sprintf(invalidBatchItemCount, len(res.batchStats), len(batch)))
		}

		// Build a mapping from original item pointers to merged pointers
		// This is needed because MergeValuesWithPath may return new objects
		originalToMerged := make(map[*astjson.Value]*astjson.Value)

		for batchIndex, targets := range res.batchStats {
			src := batch[batchIndex]
			for targetIdx, target := range targets {
				mergedTarget, _, mErr := astjson.MergeValuesWithPath(l.jsonArena, target, src, res.postProcessing.MergePath...)
				if mErr != nil {
					return errors.WithStack(ErrMergeResult{
						Subgraph: res.ds.Name,
						Reason:   mErr,
						Path:     fetchItem.ResponsePath,
					})
				}
				// Track the original to merged mapping
				originalToMerged[target] = mergedTarget
				// Update the target in batchStats with the merged result
				res.batchStats[batchIndex][targetIdx] = mergedTarget
				if slices.Contains(taintedIndices, batchIndex) {
					l.taintedObjs.add(mergedTarget)
				}
			}
		}
		// Update cache key items to point to merged data for L1 cache
		for _, ck := range res.l1CacheKeys {
			if ck != nil && ck.Item != nil {
				if merged, ok := originalToMerged[ck.Item]; ok {
					ck.Item = merged
				}
			}
		}
		// Only populate caches on success (no errors)
		if !hasErrors {
			l.populateL1Cache(fetchItem, res, items)
			l.updateL2Cache(res)
		}
		return nil
	}

	if batchCount, itemCount := len(batch), len(items); batchCount != itemCount {
		return l.renderErrorsFailedToFetch(fetchItem, res, fmt.Sprintf(invalidBatchItemCount, itemCount, batchCount))
	}

	for i := range items {
		items[i], _, err = astjson.MergeValuesWithPath(l.jsonArena, items[i], batch[i], res.postProcessing.MergePath...)
		if err != nil {
			return errors.WithStack(ErrMergeResult{
				Subgraph: res.ds.Name,
				Reason:   err,
				Path:     fetchItem.ResponsePath,
			})
		}
		if slices.Contains(taintedIndices, i) {
			l.taintedObjs.add(items[i])
		}
		// Update cache key item to point to merged data for L1 cache
		if i < len(res.l1CacheKeys) && res.l1CacheKeys[i] != nil {
			res.l1CacheKeys[i].Item = items[i]
		}
	}

	// Only populate caches on success (no errors)
	if !hasErrors {
		l.populateL1Cache(fetchItem, res, items)
		l.updateL2Cache(res)
	}
	return nil
}

func (l *Loader) evaluateRejected(fetchItem *FetchItem, res *result, items []*astjson.Value) (bool, error) {
	if res.authorizationRejected {
		err := l.renderAuthorizationRejectedErrors(fetchItem, res)
		if err != nil {
			return false, err
		}
		l.setSkipErrors(res, items)
		return true, nil
	}
	if res.rateLimitRejected {
		err := l.renderRateLimitRejectedErrors(fetchItem, res)
		if err != nil {
			return false, err
		}
		l.setSkipErrors(res, items)
		return true, nil
	}
	return false, nil
}

func (l *Loader) setSkipErrors(res *result, items []*astjson.Value) {
	trueValue := astjson.TrueValue(l.jsonArena)
	skipErrorsPath := make([]string, len(res.postProcessing.MergePath)+1)
	copy(skipErrorsPath, res.postProcessing.MergePath)
	skipErrorsPath[len(skipErrorsPath)-1] = "__skipErrors"
	for _, item := range items {
		astjson.SetValue(item, trueValue, skipErrorsPath...)
	}
}

var (
	errorsInvalidInputHeader = []byte(`{"errors":[{"message":"Failed to render Fetch Input","path":[`)
	errorsInvalidInputFooter = []byte(`]}]}`)
)

func (l *Loader) renderErrorsInvalidInput(fetchItem *FetchItem) []byte {
	out := bytes.NewBuffer(nil)
	elements := fetchItem.ResponsePathElements
	if len(elements) > 0 && elements[len(elements)-1] == "@" {
		elements = elements[:len(elements)-1]
	}
	if len(elements) > 0 {
		elements = elements[1:]
	}
	_, _ = out.Write(errorsInvalidInputHeader)
	for i := range elements {
		if i != 0 {
			_, _ = out.Write(comma)
		}
		_, _ = out.Write(quote)
		_, _ = out.WriteString(elements[i])
		_, _ = out.Write(quote)
	}
	_, _ = out.Write(errorsInvalidInputFooter)
	return out.Bytes()
}

// updateL2Cache writes entity data to the L2 (external) cache.
// This enables cross-request caching via external stores like Redis.
func (l *Loader) updateL2Cache(res *result) {
	if !l.ctx.ExecutionOptions.Caching.EnableL2Cache {
		return
	}
	if res.cache == nil || !res.cacheMustBeUpdated {
		return
	}

	// Use l2CacheKeys (with prefix) if available, otherwise fall back to cacheKeys
	keysToStore := res.l2CacheKeys
	if len(keysToStore) == 0 {
		keysToStore = res.l1CacheKeys
	}
	if len(keysToStore) == 0 {
		return
	}

	// Convert CacheKeys to CacheEntries
	cacheEntries, err := l.cacheKeysToEntries(l.jsonArena, keysToStore)
	if err != nil {
		// Cache update errors are non-fatal - silently ignore
		return
	}

	if len(cacheEntries) == 0 {
		return
	}

	// Cache set errors are non-fatal - silently ignore
	_ = res.cache.Set(l.ctx.ctx, cacheEntries, res.cacheConfig.TTL)
}

func (l *Loader) appendSubgraphError(res *result, fetchItem *FetchItem, value *astjson.Value, values []*astjson.Value) error {
	// print them into the buffer to be able to parse them
	errorsJSON := value.MarshalTo(nil)
	graphqlErrors := make([]GraphQLError, 0, len(values))
	err := json.Unmarshal(errorsJSON, &graphqlErrors)
	if err != nil {
		return errors.WithStack(err)
	}

	subgraphError := NewSubgraphError(res.ds, fetchItem.ResponsePath, failedToFetchNoReason, res.statusCode)

	for _, gqlError := range graphqlErrors {
		gErr := gqlError
		subgraphError.AppendDownstreamError(&gErr)
	}

	l.ctx.appendSubgraphErrors(res.ds, res.err, subgraphError)

	return nil
}

func (l *Loader) mergeErrors(res *result, fetchItem *FetchItem, value *astjson.Value) error {
	values := value.GetArray()
	l.optionallyOmitErrorLocations(values)
	if l.rewriteSubgraphErrorPaths {
		rewriteErrorPaths(l.jsonArena, fetchItem, values)
	}
	l.optionallyEnsureExtensionErrorCode(values)

	if !l.allowAllErrorExtensionFields {
		l.optionallyAllowCustomExtensionProperties(values)
	}

	if l.subgraphErrorPropagationMode == SubgraphErrorPropagationModePassThrough {
		// Attach datasource information to all errors when we don't wrap them
		l.optionallyAttachServiceNameToErrorExtension(values, res.ds.Name)
		l.setSubgraphStatusCode(values, res.statusCode)

		// Allow to delete extensions entirely
		l.optionallyOmitErrorExtensions(values)

		l.optionallyOmitErrorFields(values)

		// If enabled, add the extra http status error for Apollo Router compat
		if err := l.addApolloRouterCompatibilityError(res); err != nil {
			return err
		}

		if len(values) > 0 {
			// Append the subgraph errors to the response payload
			if err := l.appendSubgraphError(res, fetchItem, value, values); err != nil {
				return err
			}
		}
		// for efficiency purposes, resolvable.errors is not initialized
		// don't change this, it's measurable
		// downside: we have to verify it's initialized before appending to it
		l.resolvable.ensureErrorsInitialized()
		// If the error propagation mode is pass-through, we append the errors to the root array
		l.resolvable.errors.AppendArrayItems(value)
		return nil
	}

	if len(values) > 0 {
		// Append the subgraph errors to the response payload
		if err := l.appendSubgraphError(res, fetchItem, value, values); err != nil {
			return err
		}
	}

	// Wrap mode (default)
	errorObject, err := astjson.ParseWithArena(l.jsonArena, l.renderSubgraphBaseError(res.ds, fetchItem.ResponsePath, failedToFetchNoReason))
	if err != nil {
		return err
	}

	if l.propagateSubgraphErrors {
		// Attach all errors to the root array in the "errors" extension field
		astjson.SetValue(errorObject, value, "extensions", "errors")
	}

	v := []*astjson.Value{errorObject}

	// Only datasource information are attached to the root error in wrap mode
	l.optionallyAttachServiceNameToErrorExtension(v, res.ds.Name)
	l.setSubgraphStatusCode(v, res.statusCode)

	// Allow to delete extensions entirely
	l.optionallyOmitErrorExtensions(v)

	// If enabled, add the extra http status error for Apollo Router compat
	if err := l.addApolloRouterCompatibilityError(res); err != nil {
		return err
	}
	// for efficiency purposes, resolvable.errors is not initialized
	// don't change this, it's measurable
	// downside: we have to verify it's initialized before appending to it
	l.resolvable.ensureErrorsInitialized()
	astjson.AppendToArray(l.resolvable.errors, errorObject)

	return nil
}

// optionallyAllowCustomExtensionProperties removes all properties from the "extensions" object
// that are not in the allowedProperties map.
// If no properties are left, the "extensions" object is removed.
func (l *Loader) optionallyAllowCustomExtensionProperties(values []*astjson.Value) {
	for _, value := range values {
		if value.Exists("extensions") {
			extensions := value.Get("extensions")
			if extensions.Type() != astjson.TypeObject {
				continue
			}
			extObj := extensions.GetObject()

			extObj.Visit(func(k []byte, v *astjson.Value) {
				kb := unsafebytes.BytesToString(k)
				if _, ok := l.allowedErrorExtensionFields[kb]; !ok {
					extensions.Del(kb)
				}
			})

			// If there are no more properties, we remove the extensions object
			if len(l.allowedErrorExtensionFields) == 0 || extObj.Len() == 0 {
				value.Del("extensions")
				continue
			}
		}
	}
}

// optionallyEnsureExtensionErrorCode ensures that all values have an error code in the "extensions" object.
func (l *Loader) optionallyEnsureExtensionErrorCode(values []*astjson.Value) {
	if l.defaultErrorExtensionCode == "" {
		return
	}

	for _, value := range values {
		if value.Exists("extensions") {
			extensions := value.Get("extensions")
			switch extensions.Type() {
			case astjson.TypeObject:
				if !extensions.Exists("code") {
					extensions.Set(l.jsonArena, "code", astjson.StringValue(l.jsonArena, l.defaultErrorExtensionCode))
				}
			case astjson.TypeNull:
				extensionsObj := astjson.ObjectValue(l.jsonArena)
				extensionsObj.Set(l.jsonArena, "code", astjson.StringValue(l.jsonArena, l.defaultErrorExtensionCode))
				value.Set(l.jsonArena, "extensions", extensionsObj)
			}
		} else {
			extensionsObj := astjson.ObjectValue(l.jsonArena)
			extensionsObj.Set(l.jsonArena, "code", astjson.StringValue(l.jsonArena, l.defaultErrorExtensionCode))
			value.Set(l.jsonArena, "extensions", extensionsObj)
		}
	}
}

// optionallyAttachServiceNameToErrorExtension for all values attaches the service name
// to the "extensions" object.
func (l *Loader) optionallyAttachServiceNameToErrorExtension(values []*astjson.Value, serviceName string) {
	if !l.attachServiceNameToErrorExtension {
		return
	}

	for _, value := range values {
		if value.Exists("extensions") {
			extensions := value.Get("extensions")
			switch extensions.Type() {
			case astjson.TypeObject:
				extensions.Set(l.jsonArena, "serviceName", astjson.StringValue(l.jsonArena, serviceName))
			case astjson.TypeNull:
				extensionsObj := astjson.ObjectValue(l.jsonArena)
				extensionsObj.Set(l.jsonArena, "serviceName", astjson.StringValue(l.jsonArena, serviceName))
				value.Set(l.jsonArena, "extensions", extensionsObj)
			}
		} else {
			extensionsObj := astjson.ObjectValue(l.jsonArena)
			extensionsObj.Set(l.jsonArena, "serviceName", astjson.StringValue(l.jsonArena, serviceName))
			value.Set(l.jsonArena, "extensions", extensionsObj)
		}
	}
}

// optionallyOmitErrorExtensions removes the "extensions" object from all values
func (l *Loader) optionallyOmitErrorExtensions(values []*astjson.Value) {
	if !l.omitSubgraphErrorExtensions {
		return
	}
	for _, value := range values {
		if value.Exists("extensions") {
			value.Del("extensions")
		}
	}
}

// optionallyOmitErrorFields removes all fields from the subgraph error that are not allowlisted.
// It does not remove the message.
func (l *Loader) optionallyOmitErrorFields(values []*astjson.Value) {
	for _, value := range values {
		if value.Type() == astjson.TypeObject {
			obj := value.GetObject()
			var keysToDelete []string
			obj.Visit(func(k []byte, v *astjson.Value) {
				key := unsafebytes.BytesToString(k)
				if _, ok := l.allowedSubgraphErrorFields[key]; !ok {
					keysToDelete = append(keysToDelete, key)
				}
			})
			for _, key := range keysToDelete {
				obj.Del(key)
			}
		}
	}
}

// optionallyOmitErrorLocations removes the "locations" object from all values.
func (l *Loader) optionallyOmitErrorLocations(values []*astjson.Value) {

	for _, value := range values {
		// If the flag is set, delete all locations
		if !value.Exists(locationsField) || l.omitSubgraphErrorLocations {
			value.Del(locationsField)
			continue
		}

		// Create a new array via astjson we can append to the valid types
		validLocations := astjson.ArrayValue(l.jsonArena)
		validIndex := 0

		// GetArray will return nil if not an array which will not be ranged over
		allLocations := value.Get(locationsField)
		for _, loc := range allLocations.GetArray() {
			line := loc.Get("line")
			column := loc.Get("column")

			// Keep location only if both line and column are > 0 (spec says 0 is invalid)
			// In case it is not an int, 0 will be returned which is invalid anyway
			if line.GetInt() > 0 && column.GetInt() > 0 {
				validLocations.SetArrayItem(l.jsonArena, validIndex, loc)
				validIndex++
			}
		}

		// If all locations were invalid, delete the locations field
		if len(validLocations.GetArray()) > 0 {
			value.Set(l.jsonArena, locationsField, validLocations)
		} else {
			value.Del(locationsField)
		}
	}
}

// rewriteErrorPaths rewrites GraphQL error "path" arrays for subgraph errors routed via _entities:
//   - Prefixes with fetchItem.ResponsePathElements (trailing "@" removed).
//   - Drops the numeric index immediately following "_entities".
//   - Converts all subsequent numeric segments to strings (e.g., 1 -> "1").
//   - Skips non-string/non-number segments.
func rewriteErrorPaths(a arena.Arena, fetchItem *FetchItem, values []*astjson.Value) {
	pathPrefix := make([]string, len(fetchItem.ResponsePathElements))
	copy(pathPrefix, fetchItem.ResponsePathElements)
	// remove the trailing @ in case we're in an array as it looks weird in the path
	// errors, like fetches, are attached to objects, not arrays
	if len(fetchItem.ResponsePathElements) != 0 && fetchItem.ResponsePathElements[len(fetchItem.ResponsePathElements)-1] == "@" {
		pathPrefix = pathPrefix[:len(pathPrefix)-1]
	}
	for _, value := range values {
		errorPath := value.Get("path")
		if astjson.ValueIsNull(errorPath) {
			continue
		}
		if errorPath.Type() != astjson.TypeArray {
			continue
		}
		pathItems := errorPath.GetArray()
		if len(pathItems) == 0 {
			continue
		}
		for i, item := range pathItems {
			if item.Type() != astjson.TypeString ||
				unsafebytes.BytesToString(item.GetStringBytes()) != "_entities" {
				continue
			}
			arr := astjson.ArrayValue(a)
			for j := range pathPrefix {
				astjson.AppendToArray(arr, astjson.StringValue(a, pathPrefix[j]))
			}
			for j := i + 1; j < len(pathItems); j++ {
				// If the item after _entities is an index (number), we should ignore it.
				if j == i+1 && pathItems[j].Type() == astjson.TypeNumber {
					continue
				}
				switch pathItems[j].Type() {
				case astjson.TypeString, astjson.TypeNumber:
					astjson.AppendToArray(arr, pathItems[j])
				}
			}
			value.Set(a, "path", arr)
			break
		}
	}
}

func (l *Loader) setSubgraphStatusCode(values []*astjson.Value, statusCode int) {
	if !l.propagateSubgraphStatusCodes {
		return
	}

	if statusCode == 0 {
		return
	}

	for _, value := range values {
		if value.Exists("extensions") {
			extensions := value.Get("extensions")
			if extensions.Type() != astjson.TypeObject {
				continue
			}
			v, err := astjson.ParseWithArena(l.jsonArena, strconv.Itoa(statusCode))
			if err != nil {
				continue
			}
			extensions.Set(l.jsonArena, "statusCode", v)
		} else {
			v, err := astjson.ParseWithArena(l.jsonArena, `{"statusCode":`+strconv.Itoa(statusCode)+`}`)
			if err != nil {
				continue
			}
			value.Set(l.jsonArena, "extensions", v)
		}
	}
}

const (
	failedToFetchNoReason       = ""
	emptyGraphQLResponse        = "empty response"
	invalidGraphQLResponse      = "invalid JSON"
	invalidGraphQLResponseShape = "no data or errors in response"
	invalidBatchItemCount       = "returned entities count does not match the count of representation variables in the entities request. Expected %d, got %d"
)

func (l *Loader) renderAtPathErrorPart(path string) string {
	if path == "" {
		return ""
	}
	return fmt.Sprintf(` at Path '%s'`, path)
}

func (l *Loader) addApolloRouterCompatibilityError(res *result) error {
	if !l.apolloRouterCompatibilitySubrequestHTTPError || (res.statusCode < 400) {
		return nil
	}

	apolloRouterStatusErrorJSON := fmt.Sprintf(`{
			"message": "HTTP fetch failed from '%[1]s': %[3]d: %[2]s",
			"path": [],
			"extensions": {
				"code": "SUBREQUEST_HTTP_ERROR",
				"service": "%[1]s",
				"reason": "%[3]d: %[2]s",
				"http": {
					"status": %[3]d
				}
			}
		}`, res.ds.Name, http.StatusText(res.statusCode), res.statusCode)
	apolloRouterStatusError, err := astjson.ParseWithArena(l.jsonArena, apolloRouterStatusErrorJSON)
	if err != nil {
		return err
	}
	// for efficiency purposes, resolvable.errors is not initialized
	// don't change this, it's measurable
	// downside: we have to verify it's initialized before appending to it
	l.resolvable.ensureErrorsInitialized()
	astjson.AppendToArray(l.resolvable.errors, apolloRouterStatusError)

	return nil
}

func (l *Loader) renderErrorsFailedDeps(fetchItem *FetchItem, res *result) error {
	path := l.renderAtPathErrorPart(fetchItem.ResponsePath)
	msg := fmt.Sprintf(`{"message":"Failed to obtain field dependencies from Subgraph '%s'%s."}`, res.ds.Name, path)
	errorObject, err := astjson.ParseWithArena(l.jsonArena, msg)
	if err != nil {
		return err
	}
	l.setSubgraphStatusCode([]*astjson.Value{errorObject}, res.statusCode)
	// for efficiency purposes, resolvable.errors is not initialized
	// don't change this, it's measurable
	// downside: we have to verify it's initialized before appending to it
	l.resolvable.ensureErrorsInitialized()
	astjson.AppendToArray(l.resolvable.errors, errorObject)
	return nil
}

func (l *Loader) renderErrorsFailedToFetch(fetchItem *FetchItem, res *result, reason string) error {
	l.ctx.appendSubgraphErrors(res.ds, res.err, NewSubgraphError(res.ds, fetchItem.ResponsePath, reason, res.statusCode))
	errorObject, err := astjson.ParseWithArena(l.jsonArena, l.renderSubgraphBaseError(res.ds, fetchItem.ResponsePath, reason))
	if err != nil {
		return err
	}
	l.setSubgraphStatusCode([]*astjson.Value{errorObject}, res.statusCode)
	// for efficiency purposes, resolvable.errors is not initialized
	// don't change this, it's measurable
	// downside: we have to verify it's initialized before appending to it
	l.resolvable.ensureErrorsInitialized()
	astjson.AppendToArray(l.resolvable.errors, errorObject)
	return nil
}

func (l *Loader) renderErrorsStatusFallback(fetchItem *FetchItem, res *result, statusCode int) error {
	reason := fmt.Sprintf("%d", statusCode)
	if statusText := http.StatusText(statusCode); statusText != "" {
		reason += fmt.Sprintf(": %s", statusText)
	}

	l.ctx.appendSubgraphErrors(res.ds, res.err, NewSubgraphError(res.ds, fetchItem.ResponsePath, reason, res.statusCode))

	errorObject, err := astjson.ParseWithArena(l.jsonArena, fmt.Sprintf(`{"message":"%s"}`, reason))
	if err != nil {
		return err
	}

	l.setSubgraphStatusCode([]*astjson.Value{errorObject}, res.statusCode)
	// for efficiency purposes, resolvable.errors is not initialized
	// don't change this, it's measurable
	// downside: we have to verify it's initialized before appending to it
	l.resolvable.ensureErrorsInitialized()
	astjson.AppendToArray(l.resolvable.errors, errorObject)
	return nil
}

func (l *Loader) renderSubgraphBaseError(ds DataSourceInfo, path, reason string) string {
	pathPart := l.renderAtPathErrorPart(path)
	if ds.Name == "" {
		if reason == "" {
			return fmt.Sprintf(`{"message":"Failed to fetch from Subgraph%s."}`, pathPart)
		}
		return fmt.Sprintf(`{"message":"Failed to fetch from Subgraph%s, Reason: %s."}`, pathPart, reason)
	}
	if reason == "" {
		return fmt.Sprintf(`{"message":"Failed to fetch from Subgraph '%s'%s."}`, ds.Name, pathPart)
	}
	return fmt.Sprintf(`{"message":"Failed to fetch from Subgraph '%s'%s, Reason: %s."}`, ds.Name, pathPart, reason)
}

func (l *Loader) renderAuthorizationRejectedErrors(fetchItem *FetchItem, res *result) error {
	for i := range res.authorizationRejectedReasons {
		l.ctx.appendSubgraphErrors(res.ds, res.err, NewSubgraphError(res.ds, fetchItem.ResponsePath, res.authorizationRejectedReasons[i], res.statusCode))
	}
	pathPart := l.renderAtPathErrorPart(fetchItem.ResponsePath)
	extensionErrorCode := fmt.Sprintf(`"extensions":{"code":"%s"}`, errorcodes.UnauthorizedFieldOrType)
	// for efficiency purposes, resolvable.errors is not initialized
	// don't change this, it's measurable
	// downside: we have to verify it's initialized before appending to it
	l.resolvable.ensureErrorsInitialized()
	if res.ds.Name == "" {
		for _, reason := range res.authorizationRejectedReasons {
			if reason == "" {
				errorObject, err := astjson.ParseWithArena(l.jsonArena, fmt.Sprintf(`{"message":"Unauthorized Subgraph request%s.",%s}`, pathPart, extensionErrorCode))
				if err != nil {
					continue
				}
				astjson.AppendToArray(l.resolvable.errors, errorObject)
			} else {
				errorObject, err := astjson.ParseWithArena(l.jsonArena, fmt.Sprintf(`{"message":"Unauthorized Subgraph request%s, Reason: %s.",%s}`, pathPart, reason, extensionErrorCode))
				if err != nil {
					continue
				}
				astjson.AppendToArray(l.resolvable.errors, errorObject)
			}
		}
	} else {
		for _, reason := range res.authorizationRejectedReasons {
			if reason == "" {
				errorObject, err := astjson.ParseWithArena(l.jsonArena, fmt.Sprintf(`{"message":"Unauthorized request to Subgraph '%s'%s.",%s}`, res.ds.Name, pathPart, extensionErrorCode))
				if err != nil {
					continue
				}
				astjson.AppendToArray(l.resolvable.errors, errorObject)
			} else {
				errorObject, err := astjson.ParseWithArena(l.jsonArena, fmt.Sprintf(`{"message":"Unauthorized request to Subgraph '%s'%s, Reason: %s.",%s}`, res.ds.Name, pathPart, reason, extensionErrorCode))
				if err != nil {
					continue
				}
				astjson.AppendToArray(l.resolvable.errors, errorObject)
			}
		}
	}
	return nil
}

func (l *Loader) renderRateLimitRejectedErrors(fetchItem *FetchItem, res *result) error {
	l.ctx.appendSubgraphErrors(res.ds, res.err, NewRateLimitError(res.ds.Name, fetchItem.ResponsePath, res.rateLimitRejectedReason))
	pathPart := l.renderAtPathErrorPart(fetchItem.ResponsePath)
	var (
		err         error
		errorObject *astjson.Value
	)
	if res.ds.Name == "" {
		if res.rateLimitRejectedReason == "" {
			errorObject, err = astjson.ParseWithArena(l.jsonArena, fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph request%s."}`, pathPart))
			if err != nil {
				return err
			}
		} else {
			errorObject, err = astjson.ParseWithArena(l.jsonArena, fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph request%s, Reason: %s."}`, pathPart, res.rateLimitRejectedReason))
			if err != nil {
				return err
			}
		}
	} else {
		if res.rateLimitRejectedReason == "" {
			errorObject, err = astjson.ParseWithArena(l.jsonArena, fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph '%s'%s."}`, res.ds.Name, pathPart))
			if err != nil {
				return err
			}
		} else {
			errorObject, err = astjson.ParseWithArena(l.jsonArena, fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph '%s'%s, Reason: %s."}`, res.ds.Name, pathPart, res.rateLimitRejectedReason))
			if err != nil {
				return err
			}
		}
	}
	if l.ctx.RateLimitOptions.ErrorExtensionCode.Enabled {
		extension, err := astjson.ParseWithArena(l.jsonArena, fmt.Sprintf(`{"code":"%s"}`, l.ctx.RateLimitOptions.ErrorExtensionCode.Code))
		if err != nil {
			return err
		}
		errorObject, _, err = astjson.MergeValuesWithPath(l.jsonArena, errorObject, extension, "extensions")
		if err != nil {
			return err
		}
	}
	// for efficiency purposes, resolvable.errors is not initialized
	// don't change this, it's measurable
	// downside: we have to verify it's initialized before appending to it
	l.resolvable.ensureErrorsInitialized()
	astjson.AppendToArray(l.resolvable.errors, errorObject)
	return nil
}

func (l *Loader) isFetchAuthorized(input []byte, info *FetchInfo, res *result) (authorized bool, err error) {
	if info.OperationType == ast.OperationTypeQuery {
		// we only want to authorize Mutations and Subscriptions at the load level
		// Mutations can have side effects, so we don't want to send them to a subgraph if the user is not authorized
		// Subscriptions only have one single root field, so it's safe to deny the whole request if unauthorized
		// Queries can have multiple root fields, but have no side effects
		// So we don't need to deny the request if one of the root fields is unauthorized
		// Instead, we send the request to the subgraph and filter out the unauthorized fields later
		// This is done in the resolvable during the response resolution phase
		return true, nil
	}
	if l.ctx.authorizer == nil {
		return true, nil
	}
	authorized = true
	for i := range info.RootFields {
		if !info.RootFields[i].HasAuthorizationRule {
			continue
		}
		reject, err := l.ctx.authorizer.AuthorizePreFetch(l.ctx, info.DataSourceID, input, info.RootFields[i])
		if err != nil {
			return false, err
		}
		if reject != nil {
			authorized = false
			res.fetchSkipped = true
			res.authorizationRejected = true
			res.authorizationRejectedReasons = append(res.authorizationRejectedReasons, reject.Reason)
		}
	}
	return authorized, nil
}

func (l *Loader) rateLimitFetch(input []byte, info *FetchInfo, res *result) (allowed bool, err error) {
	if !l.ctx.RateLimitOptions.Enable {
		return true, nil
	}
	if l.ctx.rateLimiter == nil {
		return true, nil
	}
	result, err := l.ctx.rateLimiter.RateLimitPreFetch(l.ctx, info, input)
	if err != nil {
		return false, err
	}
	if result != nil {
		res.rateLimitRejected = true
		res.fetchSkipped = true
		res.rateLimitRejectedReason = result.Reason
		return false, nil
	}
	return true, nil
}

func (l *Loader) validatePreFetch(input []byte, info *FetchInfo, res *result) (allowed bool, err error) {
	if info == nil {
		return true, nil
	}
	allowed, err = l.isFetchAuthorized(input, info, res)
	if err != nil || !allowed {
		return
	}
	return l.rateLimitFetch(input, info, res)
}

func (l *Loader) loadSingleFetch(ctx context.Context, fetch *SingleFetch, fetchItem *FetchItem, items []*astjson.Value, res *result) error {
	buf := bytes.NewBuffer(nil)
	inputData := l.itemsData(items)
	if l.ctx.TracingOptions.Enable {
		fetch.Trace = &DataSourceLoadTrace{}
		if !l.ctx.TracingOptions.ExcludeRawInputData && inputData != nil {
			fetch.Trace.RawInputData, _ = l.compactJSON(inputData.MarshalTo(nil))
		}
	}

	// When we don't have parent data it makes no sense to proceed with next fetches in a sequence
	// Right now, it is the case only for the introspection - because introspection uses
	// only single fetches.
	// Having null means that the previous fetch returned null as data
	if len(items) == 1 && items[0].Type() == astjson.TypeNull {
		res.fetchSkipped = true
		if l.ctx.TracingOptions.Enable {
			fetch.Trace.LoadSkipped = true
		}
		return nil
	}

	err := fetch.InputTemplate.Render(l.ctx, inputData, buf)
	if err != nil {
		res.out = l.renderErrorsInvalidInput(fetchItem)
		return nil
	}
	fetchInput := buf.Bytes()
	allowed, err := l.validatePreFetch(fetchInput, fetch.Info, res)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
	l.executeSourceLoad(ctx, fetchItem, fetch.DataSource, fetchInput, res, fetch.Trace)
	return nil
}

func (l *Loader) loadEntityFetch(ctx context.Context, fetchItem *FetchItem, fetch *EntityFetch, items []*astjson.Value, res *result) error {
	input := l.itemsData(items)
	if l.ctx.TracingOptions.Enable {
		fetch.Trace = &DataSourceLoadTrace{}
		if !l.ctx.TracingOptions.ExcludeRawInputData && input != nil {
			fetch.Trace.RawInputData, _ = l.compactJSON(input.MarshalTo(nil))
		}
	}

	preparedInput := bytes.NewBuffer(nil)
	item := bytes.NewBuffer(nil)

	var undefinedVariables []string

	err := fetch.Input.Header.RenderAndCollectUndefinedVariables(l.ctx, nil, preparedInput, &undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}

	err = fetch.Input.Item.Render(l.ctx, input, item)
	if err != nil {
		if fetch.Input.SkipErrItem {
			// skip fetch on render item error
			if l.ctx.TracingOptions.Enable {
				fetch.Trace.LoadSkipped = true
			}
			res.fetchSkipped = true
			return nil
		}
		return errors.WithStack(err)
	}
	renderedItem := item.Bytes()
	if bytes.Equal(renderedItem, null) {
		// skip fetch if item is null
		res.fetchSkipped = true
		if l.ctx.TracingOptions.Enable {
			fetch.Trace.LoadSkipped = true
		} else {
			return nil
		}
	}
	if bytes.Equal(renderedItem, emptyObject) {
		// skip fetch if item is empty
		res.fetchSkipped = true
		if l.ctx.TracingOptions.Enable {
			fetch.Trace.LoadSkipped = true
		} else {
			return nil
		}
	}
	_, _ = item.WriteTo(preparedInput)
	err = fetch.Input.Footer.RenderAndCollectUndefinedVariables(l.ctx, nil, preparedInput, &undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}

	err = SetInputUndefinedVariables(preparedInput, undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}
	fetchInput := preparedInput.Bytes()

	if l.ctx.TracingOptions.Enable && res.fetchSkipped {
		l.setTracingInput(fetchItem, fetchInput, fetch.Trace)
		return nil
	}

	allowed, err := l.validatePreFetch(fetchInput, fetch.Info, res)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
	l.executeSourceLoad(ctx, fetchItem, fetch.DataSource, fetchInput, res, fetch.Trace)
	return nil
}

type batchEntityTools struct {
	keyGen           *xxhash.Digest
	batchHashToIndex map[uint64]int
	a                arena.Arena
}

func (b *batchEntityTools) reset() {
	b.keyGen.Reset()
	b.a.Reset()
	for i := range b.batchHashToIndex {
		delete(b.batchHashToIndex, i)
	}
}

type _batchEntityToolPool struct {
	pool sync.Pool
}

func (p *_batchEntityToolPool) Get(items int) *batchEntityTools {
	item := p.pool.Get()
	if item == nil {
		return &batchEntityTools{
			keyGen:           xxhash.New(),
			batchHashToIndex: make(map[uint64]int, items),
			a:                arena.NewMonotonicArena(arena.WithMinBufferSize(1024)),
		}
	}
	return item.(*batchEntityTools)
}

func (p *_batchEntityToolPool) Put(item *batchEntityTools) {
	if item == nil {
		return
	}
	item.reset()
	p.pool.Put(item)
}

var (
	batchEntityToolPool = _batchEntityToolPool{}
)

func (l *Loader) loadBatchEntityFetch(ctx context.Context, fetchItem *FetchItem, fetch *BatchEntityFetch, items []*astjson.Value, res *result) error {
	if l.ctx.TracingOptions.Enable {
		fetch.Trace = &DataSourceLoadTrace{}
		if !l.ctx.TracingOptions.ExcludeRawInputData && len(items) != 0 {
			data := l.itemsData(items)
			if data != nil {
				fetch.Trace.RawInputData, _ = l.compactJSON(data.MarshalTo(nil))
			}
		}
	}

	res.tools = batchEntityToolPool.Get(len(items))
	preparedInput := arena.NewArenaBuffer(res.tools.a)
	itemInput := arena.NewArenaBuffer(res.tools.a)
	batchStats := arena.AllocateSlice[[]*astjson.Value](res.tools.a, 0, len(items))
	defer func() {
		// we need to clear the batchStats slice to avoid memory corruption
		// once the outer func returns, we must not keep pointers to items on the arena
		for i := range batchStats {
			// nolint:ineffassign
			batchStats[i] = nil
		}
		// nolint:ineffassign
		batchStats = nil
	}()

	// I tried using arena here, but it only worsened the situation
	var undefinedVariables []string

	err := fetch.Input.Header.RenderAndCollectUndefinedVariables(l.ctx, nil, preparedInput, &undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}
	batchItemIndex := 0
	addSeparator := false

	// Build a set of indices that need fetching for partial cache loading
	// Only allocate the map when partial loading is enabled and there are items to fetch
	var fetchIndexSet map[int]struct{}
	if res.partialCacheEnabled && len(res.fetchItemIndices) > 0 {
		fetchIndexSet = make(map[int]struct{}, len(res.fetchItemIndices))
		for _, idx := range res.fetchItemIndices {
			fetchIndexSet[idx] = struct{}{}
		}
	}

WithNextItem:
	for i, item := range items {
		// Skip items that are already cached when partial loading is enabled
		if fetchIndexSet != nil {
			if _, needsFetch := fetchIndexSet[i]; !needsFetch {
				continue
			}
		}

		for j := range fetch.Input.Items {
			itemInput.Reset()
			err = fetch.Input.Items[j].Render(l.ctx, item, itemInput)
			if err != nil {
				if fetch.Input.SkipErrItems {
					err = nil // nolint:ineffassign
					continue
				}
				if l.ctx.TracingOptions.Enable {
					fetch.Trace.LoadSkipped = true
				}
				return errors.WithStack(err)
			}
			if fetch.Input.SkipNullItems && itemInput.Len() == 4 && bytes.Equal(itemInput.Bytes(), null) {
				continue
			}
			if fetch.Input.SkipEmptyObjectItems && itemInput.Len() == 2 && bytes.Equal(itemInput.Bytes(), emptyObject) {
				continue
			}

			res.tools.keyGen.Reset()
			_, _ = res.tools.keyGen.Write(itemInput.Bytes())
			itemHash := res.tools.keyGen.Sum64()
			if existingIndex, ok := res.tools.batchHashToIndex[itemHash]; ok {
				batchStats[existingIndex] = arena.SliceAppend(res.tools.a, batchStats[existingIndex], items[i])
				continue WithNextItem
			} else {
				if addSeparator {
					err = fetch.Input.Separator.Render(l.ctx, nil, preparedInput)
					if err != nil {
						return errors.WithStack(err)
					}
				}
				_, _ = itemInput.WriteTo(preparedInput)
				// new unique representation
				res.tools.batchHashToIndex[itemHash] = batchItemIndex
				// create a new targets bucket for this unique index
				batchStats = arena.SliceAppend(res.tools.a, batchStats, []*astjson.Value{items[i]})
				batchItemIndex++
				addSeparator = true
			}
		}
	}

	if len(batchStats) == 0 {
		// all items were skipped - discard fetch
		res.fetchSkipped = true
		if l.ctx.TracingOptions.Enable {
			fetch.Trace.LoadSkipped = true
		} else {
			return nil
		}
	}

	err = fetch.Input.Footer.RenderAndCollectUndefinedVariables(l.ctx, nil, preparedInput, &undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}

	err = SetInputUndefinedVariables(preparedInput, undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}

	fetchInput := preparedInput.Bytes()
	// it's important to copy the *astjson.Value's off the arena to avoid memory corruption
	res.batchStats = make([][]*astjson.Value, len(batchStats))
	for i := range batchStats {
		res.batchStats[i] = make([]*astjson.Value, len(batchStats[i]))
		copy(res.batchStats[i], batchStats[i])
	}

	if l.ctx.TracingOptions.Enable && res.fetchSkipped {
		l.setTracingInput(fetchItem, fetchInput, fetch.Trace)
		return nil
	}

	allowed, err := l.validatePreFetch(fetchInput, fetch.Info, res)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}

	l.executeSourceLoad(ctx, fetchItem, fetch.DataSource, fetchInput, res, fetch.Trace)
	return nil
}

func redactHeaders(rawJSON json.RawMessage) (json.RawMessage, error) {
	var obj map[string]interface{}

	sensitiveHeaders := []string{
		"authorization",
		"www-authenticate",
		"proxy-authenticate",
		"proxy-authorization",
		"cookie",
		"set-cookie",
	}

	err := json.Unmarshal(rawJSON, &obj)
	if err != nil {
		return nil, err
	}

	if headers, ok := obj["header"]; ok {
		if headerMap, isMap := headers.(map[string]interface{}); isMap {
			for key, values := range headerMap {
				if slices.Contains(sensitiveHeaders, strings.ToLower(key)) {
					headerMap[key] = []string{"****"}
				} else {
					headerMap[key] = values
				}
			}
		}
	}

	redactedJSON, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	return redactedJSON, nil
}

type singleFlightStats struct {
	used, shared bool
}

func (l *Loader) setTracingInput(fetchItem *FetchItem, input []byte, trace *DataSourceLoadTrace) {
	trace.Path = fetchItem.ResponsePath
	if !l.ctx.TracingOptions.ExcludeInput {
		trace.Input = make([]byte, len(input))
		copy(trace.Input, input) // copy input explicitly, omit __trace__ field
		redactedInput, err := redactHeaders(trace.Input)
		if err != nil {
			return
		}
		trace.Input = redactedInput
	}
}

type loaderContextKey string

const (
	operationTypeContextKey loaderContextKey = "operationType"
)

// GetOperationTypeFromContext can be used, e.g. by the transport, to check if the operation is a Mutation
func GetOperationTypeFromContext(ctx context.Context) ast.OperationType {
	if ctx == nil {
		return ast.OperationTypeQuery
	}
	if v := ctx.Value(operationTypeContextKey); v != nil {
		if opType, ok := v.(ast.OperationType); ok {
			return opType
		}
	}
	return ast.OperationTypeQuery
}

func (l *Loader) headersForSubgraphRequest(fetchItem *FetchItem) (http.Header, uint64) {
	if fetchItem == nil || fetchItem.Fetch == nil {
		return nil, 0
	}
	info := fetchItem.Fetch.FetchInfo()
	if info == nil {
		return nil, 0
	}
	return l.ctx.HeadersForSubgraphRequest(info.DataSourceName)
}

// singleFlightAllowed returns true if the specific GraphQL Operation is a Query
// even if the root operation type is a Mutation or Subscription
// sub-operations can still be of type Query
// even in such cases we allow request de-duplication because such requests are idempotent
func (l *Loader) singleFlightAllowed(fetchItem *FetchItem) bool {
	if l.ctx.ExecutionOptions.DisableSubgraphRequestDeduplication {
		return false
	}
	if fetchItem == nil {
		return false
	}
	if fetchItem.Fetch == nil {
		return false
	}
	info := fetchItem.Fetch.FetchInfo()
	if info == nil {
		return false
	}
	if info.OperationType == ast.OperationTypeQuery {
		return true
	}
	return false
}

func (l *Loader) loadByContext(ctx context.Context, source DataSource, fetchItem *FetchItem, input []byte, res *result) error {

	if l.info != nil {
		ctx = context.WithValue(ctx, operationTypeContextKey, l.info.OperationType)
	}

	headers, extraKey := l.headersForSubgraphRequest(fetchItem)

	if !l.singleFlightAllowed(fetchItem) {
		// Disable single flight for mutations
		return l.loadByContextDirect(ctx, source, headers, input, res)
	}

	item, shared := l.singleFlight.GetOrCreateItem(fetchItem, input, extraKey)
	if res.singleFlightStats != nil {
		res.singleFlightStats.used = true
		res.singleFlightStats.shared = shared
	}

	if shared {
		select {
		case <-item.loaded:
		case <-ctx.Done():
			return ctx.Err()
		}

		if item.err != nil {
			return item.err
		}

		res.out = item.response
		return nil
	}

	// helps the http client to create buffers at the right size
	ctx = httpclient.WithHTTPClientSizeHint(ctx, item.sizeHint)

	defer l.singleFlight.Finish(item)

	// Perform the actual load
	err := l.loadByContextDirect(ctx, source, headers, input, res)
	if err != nil {
		item.err = err
		return err
	}

	item.response = res.out
	return nil
}

func (l *Loader) loadByContextDirect(ctx context.Context, source DataSource, headers http.Header, input []byte, res *result) error {
	if l.ctx.Files != nil {
		res.out, res.err = source.LoadWithFiles(ctx, headers, input, l.ctx.Files)
	} else {
		res.out, res.err = source.Load(ctx, headers, input)
	}
	if res.err != nil {
		return errors.WithStack(res.err)
	}
	return nil
}

func (l *Loader) executeSourceLoad(ctx context.Context, fetchItem *FetchItem, source DataSource, input []byte, res *result, trace *DataSourceLoadTrace) {
	if l.ctx.Extensions != nil {
		input, res.err = jsonparser.Set(input, l.ctx.Extensions, "body", "extensions")
		if res.err != nil {
			res.err = errors.WithStack(res.err)
			return
		}
	}
	if l.propagateFetchReasons && !IsIntrospectionDataSource(res.ds.ID) {
		info := fetchItem.Fetch.FetchInfo()
		if info != nil && len(info.PropagatedFetchReasons) > 0 {
			var encoded []byte
			encoded, res.err = json.Marshal(info.PropagatedFetchReasons)
			if res.err != nil {
				res.err = errors.WithStack(res.err)
				return
			}
			// We expect that body.extensions is an object
			input, res.err = jsonparser.Set(input, encoded, "body", "extensions", "fetch_reasons")
			if res.err != nil {
				res.err = errors.WithStack(res.err)
				return
			}
		}
	}
	if l.ctx.TracingOptions.Enable {
		res.singleFlightStats = &singleFlightStats{}
		trace.Path = fetchItem.ResponsePath
		if !l.ctx.TracingOptions.ExcludeInput {
			trace.Input = make([]byte, len(input))
			copy(trace.Input, input) // copy input explicitly, omit __trace__ field
			redactedInput, err := redactHeaders(trace.Input)
			if err != nil {
				res.err = errors.WithStack(err)
				return
			}
			trace.Input = redactedInput
		}
		if gjson.ValidBytes(input) {
			inputCopy := make([]byte, len(input))
			copy(inputCopy, input)
			input, _ = jsonparser.Set(inputCopy, []byte("true"), "__trace__")
		}
		if !l.ctx.TracingOptions.ExcludeLoadStats {
			trace.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
			trace.DurationSinceStartPretty = time.Duration(trace.DurationSinceStartNano).String()
			trace.LoadStats = &LoadStats{}
			clientTrace := &httptrace.ClientTrace{
				GetConn: func(hostPort string) {
					trace.LoadStats.GetConn.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.GetConn.DurationSinceStartPretty = time.Duration(trace.LoadStats.GetConn.DurationSinceStartNano).String()
					if !l.ctx.TracingOptions.EnablePredictableDebugTimings {
						trace.LoadStats.GetConn.HostPort = hostPort
					}
				},
				GotConn: func(info httptrace.GotConnInfo) {
					trace.LoadStats.GotConn.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.GotConn.DurationSinceStartPretty = time.Duration(trace.LoadStats.GotConn.DurationSinceStartNano).String()
					if !l.ctx.TracingOptions.EnablePredictableDebugTimings {
						trace.LoadStats.GotConn.Reused = info.Reused
						trace.LoadStats.GotConn.WasIdle = info.WasIdle
						trace.LoadStats.GotConn.IdleTimeNano = info.IdleTime.Nanoseconds()
						trace.LoadStats.GotConn.IdleTimePretty = info.IdleTime.String()
					}
				},
				PutIdleConn: nil,
				GotFirstResponseByte: func() {
					trace.LoadStats.GotFirstResponseByte.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.GotFirstResponseByte.DurationSinceStartPretty = time.Duration(trace.LoadStats.GotFirstResponseByte.DurationSinceStartNano).String()
				},
				Got100Continue: nil,
				Got1xxResponse: nil,
				DNSStart: func(info httptrace.DNSStartInfo) {
					trace.LoadStats.DNSStart.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.DNSStart.DurationSinceStartPretty = time.Duration(trace.LoadStats.DNSStart.DurationSinceStartNano).String()
					if !l.ctx.TracingOptions.EnablePredictableDebugTimings {
						trace.LoadStats.DNSStart.Host = info.Host
					}
				},
				DNSDone: func(info httptrace.DNSDoneInfo) {
					trace.LoadStats.DNSDone.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.DNSDone.DurationSinceStartPretty = time.Duration(trace.LoadStats.DNSDone.DurationSinceStartNano).String()
				},
				ConnectStart: func(network, addr string) {
					trace.LoadStats.ConnectStart.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.ConnectStart.DurationSinceStartPretty = time.Duration(trace.LoadStats.ConnectStart.DurationSinceStartNano).String()
					if !l.ctx.TracingOptions.EnablePredictableDebugTimings {
						trace.LoadStats.ConnectStart.Network = network
						trace.LoadStats.ConnectStart.Addr = addr
					}
				},
				ConnectDone: func(network, addr string, err error) {
					trace.LoadStats.ConnectDone.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.ConnectDone.DurationSinceStartPretty = time.Duration(trace.LoadStats.ConnectDone.DurationSinceStartNano).String()
					if !l.ctx.TracingOptions.EnablePredictableDebugTimings {
						trace.LoadStats.ConnectDone.Network = network
						trace.LoadStats.ConnectDone.Addr = addr
					}
					if err != nil {
						trace.LoadStats.ConnectDone.Err = err.Error()
					}
				},
				TLSHandshakeStart: func() {
					trace.LoadStats.TLSHandshakeStart.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.TLSHandshakeStart.DurationSinceStartPretty = time.Duration(trace.LoadStats.TLSHandshakeStart.DurationSinceStartNano).String()
				},
				TLSHandshakeDone: func(state tls.ConnectionState, err error) {
					trace.LoadStats.TLSHandshakeDone.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.TLSHandshakeDone.DurationSinceStartPretty = time.Duration(trace.LoadStats.TLSHandshakeDone.DurationSinceStartNano).String()
					if err != nil {
						trace.LoadStats.TLSHandshakeDone.Err = err.Error()
					}
				},
				WroteHeaderField: nil,
				WroteHeaders: func() {
					trace.LoadStats.WroteHeaders.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.WroteHeaders.DurationSinceStartPretty = time.Duration(trace.LoadStats.WroteHeaders.DurationSinceStartNano).String()
				},
				Wait100Continue: nil,
				WroteRequest: func(info httptrace.WroteRequestInfo) {
					trace.LoadStats.WroteRequest.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.WroteRequest.DurationSinceStartPretty = time.Duration(trace.LoadStats.WroteRequest.DurationSinceStartNano).String()
					if info.Err != nil {
						trace.LoadStats.WroteRequest.Err = info.Err.Error()
					}
				},
			}
			ctx = httptrace.WithClientTrace(ctx, clientTrace)
		}
	}
	var responseContext *httpclient.ResponseContext
	ctx, responseContext = httpclient.InjectResponseContext(ctx)

	if l.ctx.LoaderHooks != nil {
		res.loaderHookContext = l.ctx.LoaderHooks.OnLoad(ctx, res.ds)

		// Prevent that the context is destroyed when the loader hook return an empty context
		if res.loaderHookContext != nil {
			res.err = l.loadByContext(res.loaderHookContext, source, fetchItem, input, res)
		} else {
			res.err = l.loadByContext(ctx, source, fetchItem, input, res)
			res.loaderHookContext = ctx // Set the context to the original context to ensure that OnFinished hook gets valid context
		}

	} else {
		res.err = l.loadByContext(ctx, source, fetchItem, input, res)
	}

	res.statusCode = responseContext.StatusCode
	res.httpResponseContext = responseContext

	if l.ctx.TracingOptions.Enable {
		if res.singleFlightStats != nil {
			trace.SingleFlightUsed = res.singleFlightStats.used
			trace.SingleFlightSharedResponse = res.singleFlightStats.shared
		}
		if !l.ctx.TracingOptions.ExcludeOutput && len(res.out) > 0 {
			trace.Output, _ = l.compactJSON(res.out)
			if l.ctx.TracingOptions.EnablePredictableDebugTimings {
				trace.Output, _ = sjson.DeleteBytes(trace.Output, "extensions.trace.response.headers.Date")
			}
		}
		if !l.ctx.TracingOptions.ExcludeLoadStats {
			if l.ctx.TracingOptions.EnablePredictableDebugTimings {
				trace.DurationLoadNano = 1
			} else {
				trace.DurationLoadNano = GetDurationNanoSinceTraceStart(ctx) - trace.DurationSinceStartNano
			}
			trace.DurationLoadPretty = time.Duration(trace.DurationLoadNano).String()
		}
	}
	if res.err != nil {
		if l.ctx.TracingOptions.Enable {
			trace.LoadError = res.err.Error()
			res.err = errors.WithStack(res.err)
		}
	}
}

func (l *Loader) compactJSON(data []byte) ([]byte, error) {
	dst := bytes.NewBuffer(make([]byte, len(data))[:0])
	err := json.Compact(dst, data)
	if err != nil {
		return nil, err
	}
	out := dst.Bytes()
	// Don't use arena here to avoid segfaults.
	// If we're not keeping the result long-term on the arena,
	// we just parse and re-marshal it to deduplicate object keys.
	// This is not a hot path so it's fine.
	// it's also not a hot path and not important to optimize
	// arena requires the parsed content to be on the arena as well
	v, err := astjson.ParseBytes(out)
	if err != nil {
		return nil, err
	}
	astjson.DeduplicateObjectKeysRecursively(v)
	return v.MarshalTo(nil), nil
}

// canSkipFetch returns true if the cache provided exactly the information required to satisfy the query plan
// the query planner generates info.ProvidesData which tells precisely which fields the fetch must load
// if a single value is missing, we will execute the fetch
func (l *Loader) canSkipFetch(info *FetchInfo, res *result) bool {
	if info == nil || info.OperationType != ast.OperationTypeQuery || info.ProvidesData == nil {
		return false
	}
	for i := range res.l1CacheKeys {
		if !l.validateItemHasRequiredData(res.l1CacheKeys[i].FromCache, info.ProvidesData) {
			return false
		}
	}
	return true
}

// validateItemHasRequiredData checks if the given item contains all required data
// as specified by the provided Object schema
func (l *Loader) validateItemHasRequiredData(item *astjson.Value, obj *Object) bool {
	if item == nil {
		return false
	}
	// Validate each field in the object
	for _, field := range obj.Fields {
		if !l.validateFieldData(item, field) {
			return false
		}
	}

	return true
}

// validateFieldData validates a single field against the item data
func (l *Loader) validateFieldData(item *astjson.Value, field *Field) bool {
	fieldValue := item.Get(unsafebytes.BytesToString(field.Name))

	// Check if field exists
	if fieldValue == nil {
		// Field is missing - this fails validation regardless of nullability
		// Even nullable fields must be present (can be null, but not missing)
		return false
	}

	// Validate the field value against its specification
	return l.validateNodeValue(fieldValue, field.Value)
}

// validateScalarData validates scalar field data
func (l *Loader) validateScalarData(value *astjson.Value, scalar *Scalar) bool {
	if value.Type() == astjson.TypeNull {
		// Null is only allowed if the scalar is nullable
		return scalar.Nullable
	}

	// Any non-null value is acceptable for a scalar
	return true
}

// validateObjectData validates object field data
func (l *Loader) validateObjectData(value *astjson.Value, obj *Object) bool {
	if value.Type() == astjson.TypeNull {
		// Null is only allowed if the object is nullable
		return obj.Nullable
	}

	if value.Type() != astjson.TypeObject {
		// Must be an object (or null if nullable)
		return false
	}

	// Recursively validate the object's fields
	return l.validateItemHasRequiredData(value, obj)
}

// validateArrayData validates array field data
func (l *Loader) validateArrayData(value *astjson.Value, arr *Array) bool {
	if value.Type() == astjson.TypeNull {
		// Null is only allowed if the array is nullable
		return arr.Nullable
	}

	if value.Type() != astjson.TypeArray {
		// Must be an array (or null if nullable)
		return false
	}

	// If there's no item specification, we just validate the array exists
	if arr.Item == nil {
		return true
	}

	// Validate each item in the array
	arrayItems, err := value.Array()
	if err != nil {
		return false
	}

	for _, item := range arrayItems {
		if !l.validateNodeValue(item, arr.Item) {
			return false
		}
	}

	return true
}

// validateNodeValue validates a value against a Node specification
func (l *Loader) validateNodeValue(value *astjson.Value, nodeSpec Node) bool {
	switch v := nodeSpec.(type) {
	case *Scalar:
		return l.validateScalarData(value, v)
	case *Object:
		return l.validateObjectData(value, v)
	case *Array:
		return l.validateArrayData(value, v)
	default:
		// Unknown type - assume invalid
		return false
	}
}
