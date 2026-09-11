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

// responseCacheSetKeys builds the keys of a fetch, one per entity hash, and
// their per-user twins when the request carries a user id. The response is
// not known yet, so both are looked up and the response decides which one is
// written.
func (l *Loader) responseCacheSetKeys(prepared *preparedFetch, selectionHash uint64, entityHashes []uint64) {
	prepared.responseCacheKeys = make([]string, len(entityHashes))
	for i, entityHash := range entityHashes {
		prepared.responseCacheKeys[i] = caching.Key(entityHash, selectionHash)
	}

	privateIDHash, ok := l.ctx.responseCachePrivateIDHash()
	if !ok {
		return
	}

	prepared.responseCachePrivateKeys = make([]string, len(entityHashes))
	for i, entityHash := range entityHashes {
		prepared.responseCachePrivateKeys[i] = caching.PrivateKey(entityHash, selectionHash, privateIDHash)
	}
}

func (l *Loader) responseCacheLookup(prepared *preparedFetch) bool {
	if !l.responseCacheEnabled() {
		return false
	}

	keys := prepared.responseCacheKeys
	if len(keys) == 0 {
		return false
	}

	privateKeys := prepared.responseCachePrivateKeys
	lookup := keys
	// Since we don't know in advance we need to query both key types
	if privateKeys != nil {
		lookup = make([]string, 0, len(keys)+len(privateKeys))
		lookup = append(lookup, keys...)
		lookup = append(lookup, privateKeys...)
	}

	found, err := l.ctx.responseCache.store.GetMany(l.ctx.ctx, lookup)
	if err != nil {
		l.reportResponseCacheError(fmt.Errorf("response cache lookup of %d keys: %w", len(lookup), err))
		return false
	}
	if len(found) < len(keys) {
		return false
	}

	// Per position the user's own entry wins over the shared one.
	items := make([]caching.Item, len(keys))
	private := false
	size := 0
	for i, key := range keys {
		item, ok := caching.Item{}, false
		if privateKeys != nil {
			item, ok = found[privateKeys[i]]
			ok = ok && len(item.Value) > 0
			private = private || ok
		}
		if !ok {
			item, ok = found[key]
			if !ok || len(item.Value) == 0 {
				return false
			}
		}
		items[i] = item
		size += len(item.Value)
	}

	prefix, suffix := entitiesResponsePrefix, entitiesResponseSuffix
	if prepared.isRootFetchCache {
		prefix, suffix = dataResponsePrefix, dataResponseSuffix
	}

	out := make([]byte, 0, size+len(prefix)+len(suffix)+len(items)-1)
	out = append(out, prefix...)
	for i, item := range items {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, item.Value...)
	}
	out = append(out, suffix...)

	res := prepared.res
	res.out = out
	res.statusCode = http.StatusOK
	res.responseCacheHit = true
	res.responseCachePrivate = private
	res.responseCacheTTL = remainingTTL(items)
	res.responseCacheHeaderTags = foundHeaderTags(items)

	return true
}

// foundHeaderTags unions what the hit entries were stored with.
func foundHeaderTags(items []caching.Item) []string {
	lists := make([][]string, 0, len(items))
	for _, item := range items {
		lists = append(lists, item.HeaderTags)
	}
	return caching.MergeHeaderTags(nil, lists...)
}

// remainingTTL is the shortest life left across a fetch's entries: a fetch is only
// as fresh as its least fresh entry. Zero counts, it is an entry that is stale as
// of now. Only a negative TTL is dropped, as no cache should report one.
func remainingTTL(items []caching.Item) time.Duration {
	ttl := time.Duration(-1)
	for _, item := range items {
		cachedTTL := item.TTL
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

	ttl, private, ok := caching.TTL(responseCacheHeaders(res), l.ctx.responseCache.defaultTTL)
	if !ok {
		return nil
	}

	// A private body only ever lands under a per-user key.
	writeKeys := prepared.responseCacheKeys
	if private {
		if prepared.responseCachePrivateKeys == nil {
			return nil
		}
		writeKeys = prepared.responseCachePrivateKeys
	}

	values, err := responseCacheValues(prepared, response)
	if err != nil {
		return err
	}

	subgraph := prepared.res.ds.Name
	invalidation := l.ctx.responseCache.invalidation

	// Parsed whether or not the cache_tag index is on: the header always carries them.
	declared := responseCacheTags(response, len(values), prepared.isRootFetchCache)

	items := make([]caching.Item, 0, len(prepared.responseCacheKeys))
	headerTagLists := make([][]string, 0, len(prepared.responseCacheKeys))
	for i, value := range values {
		if value.Type() != astjson.TypeObject {
			continue
		}
		item := caching.Item{
			Key:   writeKeys[i],
			Value: value.MarshalTo(nil),
			TTL:   ttl,
		}
		// Indexed by i like the key: a null entity is skipped above without
		// consuming a tag list, so the two stay aligned.
		var declaredForValue []string
		if len(declared) > 0 {
			declaredForValue = declared[i]
		}
		input := responseCacheTagInput{
			declared:    declaredForValue,
			value:       value,
			subgraph:    subgraph,
			isRootFetch: prepared.isRootFetchCache,
			opts:        invalidation,
		}
		item.Tags, item.HeaderTags = responseCacheIdentities(input)
		headerTagLists = append(headerTagLists, item.HeaderTags)
		items = append(items, item)
	}

	prepared.responseCacheItems = items
	prepared.res.responseCacheHeaderTags = caching.MergeHeaderTags(nil, headerTagLists...)
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
	total := 0
	for _, value := range values {
		if value.Type() != astjson.TypeString {
			continue
		}
		tag := string(value.GetStringBytes())
		// Empty is meaningless; over long is a key name the subgraph sized.
		if tag == "" || len(tag) > maxResponseCacheTagLength {
			continue
		}
		// Stored with every entry and merged per request, so bounded as a whole
		// as well as per tag. Rejected outright, like the count.
		if total += len(tag); total > maxResponseCacheTagBytesPerValue {
			return nil
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

// responseCacheIdentities: tags follow the index options, header tags carry
// every tier.
func responseCacheIdentities(input responseCacheTagInput) (tags, headerTags []string) {
	headerTags = make([]string, 0, len(input.declared)+2)
	headerTags = append(headerTags, caching.SubgraphHeaderTag(input.subgraph))

	var typeName string
	if !input.isRootFetch {
		typeName = string(input.value.GetStringBytes("__typename"))
	}

	if typeName != "" {
		headerTags = append(headerTags, caching.TypeHeaderTag(input.subgraph, typeName))
	}
	headerTags = append(headerTags, input.declared...)

	if !input.opts.any() {
		return nil, headerTags
	}

	tags = make([]string, 0, len(input.declared)+2)
	if input.opts.CacheTag {
		for _, tag := range input.declared {
			tags = append(tags, caching.DeclaredTag(input.subgraph, tag))
		}
	}
	if input.opts.Subgraph {
		tags = append(tags, caching.SubgraphTag(input.subgraph))
	}
	if input.opts.Type && typeName != "" {
		tags = append(tags, caching.TypeTag(input.subgraph, typeName))
	}
	if len(tags) == 0 {
		tags = nil
	}

	return tags, headerTags
}

// responseCacheMergeHeaderTags adds what one fetch contributed, hit or miss, to the
// request's set. Called under the data lock, which is what serializes it.
func (l *Loader) responseCacheMergeHeaderTags(res *result) {
	if !l.responseCacheEnabled() || len(res.responseCacheHeaderTags) == 0 {
		return
	}
	l.ctx.responseCache.headerTags = caching.MergeHeaderTags(l.ctx.responseCache.headerTags, res.responseCacheHeaderTags)
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
	maxResponseCacheTagsPerValue     = 10_000
	maxResponseCacheTagLength        = 10_000
	maxResponseCacheTagBytesPerValue = 256 * 1024
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
