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
)

// responseCacheEnabled reports whether this request was handed a cache. Whether
// a particular fetch is cached is responseCacheSubgraph's call.
func (l *Loader) responseCacheEnabled() bool {
	return l.ctx != nil && l.ctx.responseCache != nil
}

// responseCacheSubgraph is the cache's settings for the subgraph a fetch goes
// to, or false when nothing of that subgraph is cached.
func (l *Loader) responseCacheSubgraph(subgraph string) (responseCacheSubgraph, bool) {
	if !l.responseCacheEnabled() {
		return responseCacheSubgraph{}, false
	}
	return l.ctx.responseCacheFor(subgraph)
}

// responseCacheEnabledFor is responseCacheSubgraph for a caller that only gates.
func (l *Loader) responseCacheEnabledFor(subgraph string) bool {
	_, ok := l.responseCacheSubgraph(subgraph)
	return ok
}

func (l *Loader) reportResponseCacheError(err error) {
	if l.responseCacheEnabled() && l.ctx.responseCache.onError != nil {
		l.ctx.responseCache.onError(err)
	}
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

// responseCacheSetKeys builds the keys of a fetch, one per entity, and
// their per-user twins when the request carries a user id for this subgraph.
// The response is not known yet, so both are looked up and the response
// decides which one is written. A subgraph the cache is off for gets no keys,
// and so no lookup and no write.
func (l *Loader) responseCacheSetKeys(prepared *preparedFetch, selection caching.Digest, entities []caching.Digest) {
	sub, ok := l.responseCacheSubgraph(prepared.res.ds.Name)
	if !ok {
		return
	}
	prepared.responseCacheKeys, prepared.responseCachePrivateKeys = l.responseCacheKeys(sub, selection, entities)
}

// responseCacheKeys is responseCacheSetKeys for a caller that keeps the keys
// itself. privateKeys is nil when the subgraph has no user id.
func (l *Loader) responseCacheKeys(sub responseCacheSubgraph, selection caching.Digest, entities []caching.Digest) (keys, privateKeys []string) {
	keys = make([]string, len(entities))
	for i, entity := range entities {
		keys[i] = caching.Key(entity, selection)
	}

	if !sub.hasPrivateID {
		return keys, nil
	}

	privateKeys = make([]string, len(entities))
	for i, entity := range entities {
		privateKeys[i] = caching.PrivateKey(entity, selection, sub.privateID)
	}
	return keys, privateKeys
}

// responseCacheLookupKeys is what one GetMany asks for: the shared keys and,
// when the request carries a user id, their per-user twins.
func responseCacheLookupKeys(keys, privateKeys []string) []string {
	if privateKeys == nil {
		return keys
	}
	lookup := make([]string, 0, len(keys)+len(privateKeys))
	lookup = append(lookup, keys...)
	return append(lookup, privateKeys...)
}

// responseCacheFoundItem picks one position's entry, the user's own over the
// shared: a body, or a record pointing at one. An invalid body is a miss.
func (l *Loader) responseCacheFoundItem(found map[string]caching.Item, key, privateKey string) (item caching.Item, private, ok bool) {
	if privateKey != "" {
		item, ok = found[privateKey]
		ok = ok && (len(item.Value) > 0 || len(item.Vary) > 0)
		private = ok
	}
	if !ok {
		item, ok = found[key]
		if !ok || (len(item.Value) == 0 && len(item.Vary) == 0) {
			return caching.Item{}, false, false
		}
	}
	if len(item.Vary) > 0 {
		// A record has no body; the variant it points at is read next.
		return item, private, true
	}
	if !l.responseCacheValidBody(key, item) {
		return caching.Item{}, false, false
	}
	return item, private, true
}

// responseCacheValidBody reports whether a body read back is JSON. One that is
// not is a miss, reported, rather than something to splice into a response.
func (l *Loader) responseCacheValidBody(key string, item caching.Item) bool {
	if len(item.Value) == 0 {
		return false
	}
	if validationErr := astjson.ValidateBytes(item.Value); validationErr != nil {
		l.reportResponseCacheError(fmt.Errorf("wrong response cache value for key %v: %w", key, validationErr))
		return false
	}
	return true
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
	// Since we don't know in advance we need to query both key types
	lookup := responseCacheLookupKeys(keys, privateKeys)

	found, err := l.ctx.responseCache.store.GetMany(l.ctx.ctx, lookup)
	if err != nil {
		l.reportResponseCacheError(fmt.Errorf("response cache lookup of %d keys: %w", len(lookup), err))
		return false
	}
	prepared.responseCacheFound = found
	if len(found) < len(keys) {
		return false
	}

	items := make([]caching.Item, len(keys))
	private := false
	for i, key := range keys {
		privateKey := ""
		if privateKeys != nil {
			privateKey = privateKeys[i]
		}
		item, itemPrivate, ok := l.responseCacheFoundItem(found, key, privateKey)
		if !ok {
			return false
		}
		private = private || itemPrivate
		items[i] = item
	}

	// A record keeps its position in items until the second round replaces
	// it with the body it points at.
	candidates, variants := l.responseCacheVariantKeys(prepared, items)
	if len(variants) > 0 {
		found, err = l.ctx.responseCache.store.GetMany(l.ctx.ctx, variants)
		if err != nil {
			l.reportResponseCacheError(fmt.Errorf("response cache lookup of %d variants: %w", len(variants), err))
			return false
		}
		if !l.responseCacheFillVariants(items, candidates, found) {
			return false
		}
	}

	size := 0
	for _, item := range items {
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
	res.responseCacheSurrogateKeys = foundSurrogateKeys(items)

	return true
}

// responseCacheVariantKeys names, per record in items, the variant each of its
// sets points this request at; candidates[i] is nil for a body. flat is every
// candidate for one GetMany. Records stay in place for responseCacheFillVariants.
func (l *Loader) responseCacheVariantKeys(prepared *preparedFetch, items []caching.Item) (candidates [][]string, flat []string) {
	var sent http.Header
	for i, item := range items {
		if len(item.Vary) == 0 {
			continue
		}
		if candidates == nil {
			candidates = make([][]string, len(items))
			// Once: the headers builder is not free. Collect digests what was sent.
			sent, _ = l.ctx.HeadersForSubgraphRequest(prepared.res.ds.Name)
		}
		for _, set := range item.Vary {
			key := caching.VariantKey(item.Key, caching.VaryDigest(set, sent))
			candidates[i] = append(candidates[i], key)
			flat = append(flat, key)
		}
	}
	return candidates, flat
}

// responseCacheFillVariants replaces each record with the first valid body
// among its candidates, the newest set's. False when a record has none.
func (l *Loader) responseCacheFillVariants(items []caching.Item, candidates [][]string, found map[string]caching.Item) bool {
	for i := range candidates {
		if len(candidates[i]) == 0 {
			continue
		}
		filled := false
		for _, key := range candidates[i] {
			body, ok := found[key]
			if ok && l.responseCacheValidBody(key, body) {
				items[i] = body
				filled = true
				break
			}
		}
		if !filled {
			return false
		}
	}
	return true
}

// responseVary is what an answer varied on and the digest of what the request
// sent for it.
type responseVary struct {
	names  []string
	digest caching.Digest
}

// responseCacheVary is not ok for "Vary: *", which matches no request, or more
// names than a record keeps.
func responseCacheVary(headers, sent http.Header) (responseVary, bool) {
	names, star := caching.Vary(headers)
	if star || len(names) > caching.MaxVaryHeaders {
		return responseVary{}, false
	}
	return responseVary{names: names, digest: caching.VaryDigest(names, sent)}, true
}

// append adds body under base or, when the answer varied, a record at base and
// body under its variant. The record keeps seen so earlier variants stay
// reachable; same tags so invalidation drops both.
func (v responseVary) append(items []caching.Item, base string, body caching.Item, seen [][]string) []caching.Item {
	if len(v.names) == 0 {
		body.Key = base
		return append(items, body)
	}
	body.Key = caching.VariantKey(base, v.digest)
	items = append(items, caching.Item{Key: base, Vary: caching.MergeVarySets(v.names, seen), TTL: body.TTL, Tags: body.Tags})
	return append(items, body)
}

// foundSurrogateKeys unions what the hit entries were stored with.
func foundSurrogateKeys(items []caching.Item) []string {
	lists := make([][]string, 0, len(items))
	for _, item := range items {
		lists = append(lists, item.SurrogateKeys)
	}
	return caching.MergeSurrogateKeys(nil, lists...)
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

	sub, ok := l.responseCacheSubgraph(res.ds.Name)
	if !ok {
		return nil
	}

	headers := responseCacheHeaders(res)
	ttl, private, ok := caching.TTL(headers, sub.ttl)
	if !ok {
		return nil
	}

	vary, ok := responseCacheVary(headers, res.sentHeaders)
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

	items := make([]caching.Item, 0, 2*len(prepared.responseCacheKeys))
	surrogateKeyLists := make([][]string, 0, len(prepared.responseCacheKeys))
	for i, value := range values {
		if value.Type() != astjson.TypeObject {
			continue
		}
		item := caching.Item{
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
		item.Tags, item.SurrogateKeys = responseCacheIdentities(input)
		surrogateKeyLists = append(surrogateKeyLists, item.SurrogateKeys)
		items = vary.append(items, writeKeys[i], item, prepared.responseCacheFound[writeKeys[i]].Vary)
	}

	prepared.responseCacheItems = items
	prepared.res.responseCacheSurrogateKeys = caching.MergeSurrogateKeys(nil, surrogateKeyLists...)
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
		// Over long is a key name the subgraph sized; the rest cannot go in a header.
		if len(tag) > maxResponseCacheTagLength || !caching.ValidDeclaredSurrogateKey(tag) {
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

// responseCacheIdentities: tags follow the index options, surrogate keys carry
// every tier.
func responseCacheIdentities(input responseCacheTagInput) (tags, surrogateKeys []string) {
	if input.subgraph == "" {
		return nil, nil
	}

	surrogateKeys = make([]string, 0, len(input.declared)+2)
	surrogateKeys = append(surrogateKeys, caching.SubgraphSurrogateKey(input.subgraph))

	var typeName string
	if !input.isRootFetch {
		typeName = string(input.value.GetStringBytes("__typename"))
	}

	if typeName != "" {
		surrogateKeys = append(surrogateKeys, caching.TypeSurrogateKey(input.subgraph, typeName))
	}
	surrogateKeys = append(surrogateKeys, input.declared...)

	if !input.opts.any() {
		return nil, surrogateKeys
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

	return tags, surrogateKeys
}

// responseCacheMergeSurrogateKeys adds what one fetch contributed, hit or miss, to the
// request's set. Called under the data lock, which is what serializes it.
func (l *Loader) responseCacheMergeSurrogateKeys(res *result) {
	if !l.responseCacheEnabled() || len(res.responseCacheSurrogateKeys) == 0 {
		return
	}
	l.ctx.responseCache.surrogateKeys = caching.MergeSurrogateKeys(l.ctx.responseCache.surrogateKeys, res.responseCacheSurrogateKeys)
}

// responseCacheCollectMultiEntity gathers what a merged response contributes to
// the cache, one alias at a time. Unlike the single-fetch path this cannot be
// all-or-nothing: an entry that was never sent (excluded at prepare, or already
// served from the cache) has no alias in the response at all, and an entry whose
// alias carries errors is skipped on its own so the others are still stored —
// the unmerged fetches this replaces are independent that way.
func (l *Loader) responseCacheCollectMultiEntity(prepared *preparedFetch, response *astjson.Value, entryErrors []*astjson.Value) {
	if !l.responseCacheEnabled() {
		return
	}

	res := prepared.res
	if res.err != nil || len(res.out) == 0 || res.statusCode >= 400 {
		return
	}

	sub, ok := l.responseCacheSubgraph(res.ds.Name)
	if !ok {
		return
	}

	// One HTTP response, one Cache-Control: the lifetime is genuinely shared,
	// and so is being private.
	headers := responseCacheHeaders(res)
	ttl, private, ok := caching.TTL(headers, sub.ttl)
	if !ok {
		return
	}

	// One Vary as well: what the merged answer varied on covers every entry in
	// it, even one that alone would have varied on less.
	vary, ok := responseCacheVary(headers, res.sentHeaders)
	if !ok {
		return
	}

	var items []caching.Item
	var surrogateKeyLists [][]string
	for i := range prepared.multiEntries {
		entry := &prepared.multiEntries[i]
		if len(entry.responseCacheKeys) == 0 || entry.cacheHit() || entry.res.fetchSkipped {
			continue
		}
		// A private body only ever lands under a per-user key.
		writeKeys := entry.responseCacheKeys
		if private {
			if entry.responseCachePrivateKeys == nil {
				continue
			}
			writeKeys = entry.responseCachePrivateKeys
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
			item := caching.Item{
				Value: value.MarshalTo(nil),
				TTL:   ttl,
			}

			// Declared tags are left out: apolloEntityCacheTags is one flat
			// list with no alias to attribute it to, so entries would take
			// each other's tags. Subgraph and type identities still apply, to
			// the index and the header alike.
			item.Tags, item.SurrogateKeys = responseCacheIdentities(responseCacheTagInput{
				value:       value,
				subgraph:    prepared.res.ds.Name,
				isRootFetch: prepared.isRootFetchCache,
				opts:        l.ctx.responseCache.invalidation,
			})
			surrogateKeyLists = append(surrogateKeyLists, item.SurrogateKeys)

			items = vary.append(items, writeKeys[j], item, prepared.responseCacheFound[writeKeys[j]].Vary)
		}
	}

	prepared.responseCacheItems = items
	prepared.res.responseCacheSurrogateKeys = caching.MergeSurrogateKeys(nil, surrogateKeyLists...)
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
			entry := &prepared.multiEntries[i]
			keys = append(keys, responseCacheLookupKeys(entry.responseCacheKeys, entry.responseCachePrivateKeys)...)
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
	prepared.responseCacheFound = found
	if len(found) == 0 {
		return false
	}

	// Round one per entry; items stays nil for a miss.
	type entryLookup struct {
		items      []caching.Item
		candidates [][]string
		private    bool
	}
	lookups := make([]entryLookup, len(prepared.multiEntries))
	var variants []string
	for i := range prepared.multiEntries {
		entry := &prepared.multiEntries[i]
		if !included[i] || len(entry.responseCacheKeys) == 0 {
			continue
		}

		items := make([]caching.Item, 0, len(entry.responseCacheKeys))
		private := false
		for j, key := range entry.responseCacheKeys {
			privateKey := ""
			if entry.responseCachePrivateKeys != nil {
				privateKey = entry.responseCachePrivateKeys[j]
			}
			item, itemPrivate, ok := l.responseCacheFoundItem(found, key, privateKey)
			if !ok {
				items = nil
				break
			}
			private = private || itemPrivate
			items = append(items, item)
		}
		if items == nil {
			// Partially warm: this entry is fetched whole, like a batch fetch
			// missing one of its representations.
			continue
		}

		candidates, flat := l.responseCacheVariantKeys(prepared, items)
		variants = append(variants, flat...)
		lookups[i] = entryLookup{items: items, candidates: candidates, private: private}
	}

	// One second round for every entry's records together.
	if len(variants) > 0 {
		found, err = l.ctx.responseCache.store.GetMany(l.ctx.ctx, variants)
		if err != nil {
			l.reportResponseCacheError(fmt.Errorf("response cache lookup of %d variants: %w", len(variants), err))
			return false
		}
	}

	anyHit := false
	for i := range lookups {
		lookup := &lookups[i]
		if lookup.items == nil {
			continue
		}
		// A record whose variant is gone leaves the entry to the origin, whole.
		if !l.responseCacheFillVariants(lookup.items, lookup.candidates, found) {
			continue
		}

		values := make([][]byte, len(lookup.items))
		for j, item := range lookup.items {
			values[j] = item.Value
		}

		entry := &prepared.multiEntries[i]
		entry.cachedValues = values
		entry.responseCachePrivate = lookup.private
		entry.responseCacheTTL = remainingTTL(lookup.items)
		entry.responseCacheSurrogateKeys = foundSurrogateKeys(lookup.items)
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
