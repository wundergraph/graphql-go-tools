package resolve

import (
	"bytes"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/caching"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

// responseCacheEnabled reports whether this request was handed a cache. It is the
// only enablement check in the engine; there is no configuration to consult.
func (l *Loader) responseCacheEnabled() bool {
	return l.ctx != nil && l.ctx.responseCache != nil
}

func (l *Loader) reportResponseCacheError(err error) {
	if l.responseCacheEnabled() && l.ctx.responseCache.onError != nil {
		l.ctx.responseCache.onError(err)
	}
}

func responseCacheSelectionHash(header, footer []byte) uint64 {
	d := pool.Hash64.Get()
	defer pool.Hash64.Put(d)
	_, _ = d.Write(header)
	// Written in between so that a byte moving from the end of the header to the
	// start of the footer cannot go unnoticed.
	_, _ = d.Write([]byte{0})
	_, _ = d.Write(footer)
	return d.Sum64()
}

// responseCacheEntrySelectionHash keys one entry of a merged fetch. The header
// and footer span every alias, so two more parts are hashed in: the alias, or
// entries over the same entity collide, and the entry's own variables, which
// sit in between and would otherwise be left out of the key.
func responseCacheEntrySelectionHash(header []byte, alias string, entryVariables, footer []byte) uint64 {
	d := pool.Hash64.Get()
	defer pool.Hash64.Put(d)
	// A zero byte after every part, so material moving from the end of one to
	// the start of the next cannot go unnoticed.
	_, _ = d.Write(header)
	_, _ = d.Write(zeroByte)
	_, _ = d.WriteString(alias)
	_, _ = d.Write(zeroByte)
	_, _ = d.Write(entryVariables)
	_, _ = d.Write(zeroByte)
	_, _ = d.Write(footer)
	return d.Sum64()
}

// rootFetchCacheable reports whether this fetch is the one shape the cache can
// key: a root query fetch answered by a GraphQL subgraph.
func rootFetchCacheable(fetchItem *FetchItem, fetch *SingleFetch) bool {
	// Root position only: a nested fetch's request was rendered from data an
	// earlier fetch returned.
	if fetchItem == nil || len(fetchItem.FetchPath) != 0 {
		return false
	}

	// Queries only.
	if fetch.Info == nil || fetch.Info.OperationType != ast.OperationTypeQuery {
		return false
	}

	// What is stored is the data object on its own, and a hit rebuilds
	// {"data":...} around it.
	if !slices.Equal(fetch.PostProcessing.SelectResponseDataPath, dataResponsePath) {
		return false
	}

	// The identifier of the graphql datasource
	return bytes.Equal(fetch.DataSourceIdentifier, graphqlDataSourceIdentifier)
}

func (l *Loader) responseCacheLookup(prepared *preparedFetch) bool {
	if !l.responseCacheEnabled() {
		return false
	}

	keys := prepared.responseCacheKeys
	if len(keys) == 0 {
		return false
	}

	found, err := l.ctx.responseCache.store.GetMany(l.ctx.ctx, keys)
	if err != nil {
		l.reportResponseCacheError(fmt.Errorf("response cache lookup of %d keys: %w", len(keys), err))
		return false
	}
	if len(found) != len(keys) {
		return false
	}

	prefix, suffix := entitiesResponsePrefix, entitiesResponseSuffix
	if prepared.isRootFetchCache {
		prefix, suffix = dataResponsePrefix, dataResponseSuffix
	}

	size := len(prefix) + len(suffix) + len(keys) - 1
	for _, key := range keys {
		item, ok := found[key]
		if !ok || len(item.Value) == 0 {
			return false
		}
		size += len(item.Value)
	}

	out := make([]byte, 0, size)
	out = append(out, prefix...)
	for i, key := range keys {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, found[key].Value...)
	}
	out = append(out, suffix...)

	res := prepared.res
	res.out = out
	res.statusCode = http.StatusOK
	res.responseCacheHit = true
	res.responseCacheTTL = remainingTTL(found, keys)

	return true
}

// remainingTTL is the shortest life left across a fetch's entries: a fetch is only
// as fresh as its least fresh entry. Zero counts, it is an entry that is stale as
// of now. Only a negative TTL is dropped, as no cache should report one.
func remainingTTL(found map[string]caching.Item, keys []string) time.Duration {
	ttl := time.Duration(-1)
	for _, key := range keys {
		cachedTTL := found[key].TTL
		if cachedTTL < 0 {
			continue
		}

		if ttl < 0 || cachedTTL < ttl {
			ttl = cachedTTL
		}
	}

	if ttl < 0 {
		return 0
	}

	return ttl
}

func (l *Loader) responseCacheCollect(prepared *preparedFetch) error {
	if !l.responseCacheEnabled() {
		return nil
	}

	if prepared.skipLoad || prepared.responseCacheHit {
		return nil
	}

	if len(prepared.responseCacheKeys) == 0 {
		return nil
	}

	res := prepared.res
	if res.err != nil || len(res.out) == 0 || res.statusCode >= 400 {
		// A failed fetch is not cacheable, which is not a collection failure and is
		// handled at other locations.
		return nil //nolint:nilerr
	}

	response, err := res.parsedResponse(l)
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	errorsPath := res.postProcessing.SelectResponseErrorsPath
	if errorsPath == nil {
		errorsPath = defaultResponseCacheErrorsPath
	}

	if errs := response.Get(errorsPath...); astjson.ValueIsNonNull(errs) && len(errs.GetArray()) > 0 {
		return nil
	}

	ttl, ok := caching.TTL(responseCacheHeaders(res), l.ctx.responseCache.defaultTTL)
	if !ok {
		return nil
	}

	values, err := responseCacheValues(prepared, response)
	if err != nil {
		return err
	}

	subgraph := prepared.res.ds.Name
	invalidation := l.ctx.responseCache.invalidation

	var declared [][]string
	if invalidation.CacheTag {
		declared = responseCacheTags(response, len(values), prepared.isRootFetchCache)
	}

	items := make([]caching.Item, 0, len(prepared.responseCacheKeys))
	for i, value := range values {
		if value.Type() != astjson.TypeObject {
			continue
		}
		item := caching.Item{
			Key:   prepared.responseCacheKeys[i],
			Value: value.MarshalTo(nil),
			TTL:   ttl,
		}
		if invalidation.any() {
			// Indexed by i like the key: a null entity is skipped above without
			// consuming a tag list, so the two stay aligned.
			var declaredForValue []string
			if len(declared) > 0 {
				declaredForValue = declared[i]
			}
			item.Tags = responseCacheTagIdentities(responseCacheTagInput{
				declared:    declaredForValue,
				value:       value,
				subgraph:    subgraph,
				isRootFetch: prepared.isRootFetchCache,
				opts:        invalidation,
			})
		}
		items = append(items, item)
	}

	prepared.responseCacheItems = items
	return nil
}

func responseCacheValues(prepared *preparedFetch, response *astjson.Value) ([]*astjson.Value, error) {
	if prepared.isRootFetchCache {
		data := response.Get(dataResponsePath...)
		if data == nil {
			return nil, nil
		}
		return []*astjson.Value{data}, nil
	}

	entities := response.Get("data", "_entities")
	if entities == nil || entities.Type() != astjson.TypeArray {
		return nil, fmt.Errorf("_entities not found or invalid type")
	}
	values := entities.GetArray()

	// In case the entity does not exist on the foreign key we should get null in place
	if len(values) != len(prepared.responseCacheKeys) {
		return nil, fmt.Errorf("unexpected number of _entities values found %d", len(values))
	}

	return values, nil
}

// responseCacheTags reads the declared cache tags, one list per value in the
// order responseCacheValues returned them. A root fetch declares them flat
// under its own extension key, because its one entry has one list; an entity
// fetch declares a list per entity and so nests them.
func responseCacheTags(response *astjson.Value, expectedItems int, isRootFetch bool) [][]string {
	if expectedItems == 0 {
		return nil
	}

	if isRootFetch {
		flat := responseCacheTagList(response.Get(responseCacheRootTagsPath...))
		if flat == nil {
			return nil
		}
		// One value, so one list. responseCacheValues guarantees the count.
		return [][]string{flat}
	}

	extension := response.Get(responseCacheEntityTagsPath...)
	if !astjson.ValueIsNonNull(extension) || extension.Type() != astjson.TypeArray {
		return nil
	}

	lists := extension.GetArray()
	if len(lists) != expectedItems {
		return nil
	}

	tags := make([][]string, expectedItems)
	for i, list := range lists {
		// Costs this value its tags only; the list stays positional.
		tags[i] = responseCacheTagList(list)
	}

	return tags
}

func responseCacheTagList(list *astjson.Value) []string {
	if !astjson.ValueIsNonNull(list) || list.Type() != astjson.TypeArray {
		return nil
	}

	values := list.GetArray()
	// Rejected outright rather than truncated
	if len(values) > maxResponseCacheTagsPerValue {
		return nil
	}

	parsed := make([]string, 0, len(values))
	for _, value := range values {
		if value.Type() != astjson.TypeString {
			continue
		}
		tag := string(value.GetStringBytes())
		// Empty is meaningless; over long is a key name the subgraph sized.
		if tag == "" || len(tag) > maxResponseCacheTagLength {
			continue
		}
		parsed = append(parsed, tag)
	}

	if len(parsed) == 0 {
		return nil
	}

	return parsed
}

// responseCacheTagInput is what one value is indexed under: the subgraph that
// answered, the value, what it declared, and which kind of fetch it came from.
type responseCacheTagInput struct {
	declared    []string
	value       *astjson.Value
	subgraph    string
	isRootFetch bool
	opts        ResponseCacheTagIndexOptions
}

func responseCacheTagIdentities(input responseCacheTagInput) []string {
	if input.subgraph == "" {
		return nil
	}

	identities := make([]string, 0, len(input.declared)+2)

	for _, tag := range input.declared {
		identities = append(identities, caching.DeclaredTag(input.subgraph, tag))
	}

	if input.opts.Subgraph {
		identities = append(identities, caching.SubgraphTag(input.subgraph))
	}

	if input.opts.Type && !input.isRootFetch {
		if name := input.value.GetStringBytes("__typename"); len(name) > 0 {
			identities = append(identities, caching.TypeTag(input.subgraph, string(name)))
		}
	}

	if len(identities) == 0 {
		return nil
	}
	return identities
}

// responseCacheCollectMultiEntity gathers what a merged response contributes to
// the cache, one alias at a time. Unlike the single-fetch path this cannot be
// all-or-nothing: an entry that was never sent (excluded at prepare, or already
// served from the cache) has no alias in the response at all, and an entry whose
// alias carries errors is skipped on its own so the others are still stored —
// the unmerged fetches this replaces are independent that way.
func (l *Loader) responseCacheCollectMultiEntity(prepared *preparedFetch, response *astjson.Value, entryErrors []*astjson.Value, unmatchedErrors bool) {
	if !l.responseCacheEnabled() {
		return
	}

	// An error that belongs to no alias is a problem with the merged request as
	// a whole, so nothing from it is cacheable.
	if unmatchedErrors {
		return
	}

	res := prepared.res
	if res.err != nil || len(res.out) == 0 || res.statusCode >= 400 {
		return
	}

	// One HTTP response, one Cache-Control: the lifetime is genuinely shared.
	ttl, ok := caching.TTL(responseCacheHeaders(res), l.ctx.responseCache.defaultTTL)
	if !ok {
		return
	}

	var items []caching.Item
	for i := range prepared.multiEntries {
		entry := &prepared.multiEntries[i]
		if len(entry.responseCacheKeys) == 0 || entry.cacheHit() || entry.res.fetchSkipped {
			continue
		}
		if errs := entryErrors[i]; astjson.ValueIsNonNull(errs) && len(errs.GetArray()) > 0 {
			continue
		}

		entities := response.Get("data", entry.entry.Alias)
		if entities == nil || entities.Type() != astjson.TypeArray {
			continue
		}
		values := entities.GetArray()
		// One key per unique representation, in the same order the subgraph
		// answers them. A different count means the response does not line up
		// with what was asked, which is not something to cache.
		if len(values) != len(entry.responseCacheKeys) {
			continue
		}

		for j, value := range values {
			if value.Type() != astjson.TypeObject {
				continue
			}
			items = append(items, caching.Item{
				Key:   entry.responseCacheKeys[j],
				Value: value.MarshalTo(nil),
				TTL:   ttl,
			})
		}
	}

	prepared.responseCacheItems = items
}

// multiEntityCacheLookup asks the cache, in one round trip, for the entities of
// every entry still bound for the origin, and records what came back whole on
// the entry itself as cachedValues. An entry is served all-or-nothing, mirroring
// what a single unmerged fetch does. Reports whether anything was found; the
// caller decides what that means for the request.
func (l *Loader) multiEntityCacheLookup(prepared *preparedFetch, included []bool) bool {
	var keys []string
	for i := range prepared.multiEntries {
		if included[i] {
			keys = append(keys, prepared.multiEntries[i].responseCacheKeys...)
		}
	}
	if len(keys) == 0 {
		return false
	}

	found, err := l.ctx.responseCache.store.GetMany(l.ctx.ctx, keys)
	if err != nil {
		// A cache failure is not a fetch failure: ask the origin for everything.
		l.reportResponseCacheError(fmt.Errorf("response cache lookup of %d keys: %w", len(keys), err))
		return false
	}
	if len(found) == 0 {
		return false
	}

	anyHit := false
	for i := range prepared.multiEntries {
		entry := &prepared.multiEntries[i]
		if !included[i] || len(entry.responseCacheKeys) == 0 {
			continue
		}

		values := make([][]byte, 0, len(entry.responseCacheKeys))
		for _, key := range entry.responseCacheKeys {
			item, ok := found[key]
			if !ok || len(item.Value) == 0 {
				values = nil
				break
			}
			values = append(values, item.Value)
		}
		if values == nil {
			// Partially warm: this entry is fetched whole, like a batch fetch
			// missing one of its representations.
			continue
		}

		entry.cachedValues = values
		entry.responseCacheTTL = remainingTTL(found, entry.responseCacheKeys)
		anyHit = true
	}

	return anyHit
}

// shortestCachedEntryTTL is the life left on a merged fetch answered entirely
// from the cache: the least fresh of its entries, as remainingTTL is for one.
func shortestCachedEntryTTL(entries []preparedMultiEntry) time.Duration {
	ttl := time.Duration(-1)
	for i := range entries {
		if !entries[i].cacheHit() {
			continue
		}
		if ttl < 0 || entries[i].responseCacheTTL < ttl {
			ttl = entries[i].responseCacheTTL
		}
	}
	if ttl < 0 {
		return 0
	}
	return ttl
}

// responseCacheFlush writes what responseCacheCollect gathered. It is called with
// the data lock released so a slow cache never blocks the fetches queued behind it.
func (l *Loader) responseCacheFlush(prepared *preparedFetch) {
	if !l.responseCacheEnabled() {
		return
	}

	if len(prepared.responseCacheItems) == 0 {
		return
	}

	items := prepared.responseCacheItems
	prepared.responseCacheItems = nil

	if err := l.ctx.responseCache.store.SetMany(l.ctx.ctx, items); err != nil {
		l.reportResponseCacheError(fmt.Errorf("response cache write of %d entities: %w", len(items), err))
	}
}

func responseCacheHeaders(res *result) http.Header {
	if res.httpResponseContext == nil || res.httpResponseContext.Response == nil {
		return nil
	}
	return res.httpResponseContext.Response.Header
}

const (
	maxResponseCacheTagsPerValue = 10_000
	maxResponseCacheTagLength    = 10_000
)

// Where a subgraph attaches its cache tags.
const (
	responseCacheEntityTagsExtensionKey = "apolloEntityCacheTags"
	responseCacheRootTagsExtensionKey   = "apolloCacheTags"
)

var (
	responseCacheEntityTagsPath = []string{"extensions", responseCacheEntityTagsExtensionKey}
	responseCacheRootTagsPath   = []string{"extensions", responseCacheRootTagsExtensionKey}

	// The data object of a root fetch response, taken apart on the way into the
	// cache and put back together on the way out.
	// Separator written between the parts of a selection hash.
	zeroByte                       = []byte{0}
	dataResponsePath               = []string{"data"}
	dataResponsePrefix             = []byte(`{"data":`)
	dataResponseSuffix             = []byte(`}`)
	entitiesResponsePrefix         = []byte(`{"data":{"_entities":[`)
	entitiesResponseSuffix         = []byte(`]}}`)
	defaultResponseCacheErrorsPath = []string{"errors"}
	// What the planner records for an HTTP GraphQL subgraph: reflect.TypeOf of
	// the datasource, see plan.Visitor.configureFetch.
	graphqlDataSourceIdentifier = []byte("graphql_datasource.Source")
)
