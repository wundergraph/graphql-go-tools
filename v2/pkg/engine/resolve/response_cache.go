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
			if declared != nil {
				declaredForValue = declared[i]
			}
			item.Tags = responseCacheTagIdentities(declaredForValue, value, subgraph, invalidation)
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

func responseCacheTagIdentities(declared []string, value *astjson.Value, subgraph string, opts ResponseCacheInvalidationOptions) []string {
	if subgraph == "" {
		return nil
	}

	identities := make([]string, 0, len(declared)+2)

	for _, tag := range declared {
		identities = append(identities, caching.DeclaredTag(subgraph, tag))
	}

	if opts.Subgraph {
		identities = append(identities, caching.SubgraphTag(subgraph))
	}

	if opts.Type {
		if name := value.GetStringBytes("__typename"); len(name) > 0 {
			identities = append(identities, caching.TypeTag(subgraph, string(name)))
		}
	}

	if len(identities) == 0 {
		return nil
	}
	return identities
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
