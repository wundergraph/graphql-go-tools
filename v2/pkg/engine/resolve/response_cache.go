package resolve

import (
	"fmt"
	"net/http"

	"github.com/wundergraph/astjson"

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

	size := len(entitiesResponsePrefix) + len(entitiesResponseSuffix) + len(keys) - 1
	for _, key := range keys {
		item, ok := found[key]
		if !ok || len(item.Value) == 0 {
			return false
		}
		size += len(item.Value)
	}

	out := make([]byte, 0, size)
	out = append(out, entitiesResponsePrefix...)
	for i, key := range keys {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, found[key].Value...)
	}
	out = append(out, entitiesResponseSuffix...)

	res := prepared.res
	res.out = out
	res.statusCode = http.StatusOK

	return true
}

func (l *Loader) responseCacheCollect(prepared *preparedFetch) error {
	if !l.responseCacheEnabled() {
		return nil
	}

	if len(prepared.responseCacheKeys) == 0 {
		return nil
	}

	if prepared.skipLoad || prepared.responseCacheHit {
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

	entities := response.Get("data", "_entities")
	if entities == nil || entities.Type() != astjson.TypeArray {
		return fmt.Errorf("_entities not found or invalid type")
	}
	values := entities.GetArray()

	// In case the entity does not exist on the foreign key we should get null in place
	if len(values) != len(prepared.responseCacheKeys) {
		return fmt.Errorf("unexpected number of _entities values found %d", len(values))
	}

	items := make([]caching.Item, 0, len(prepared.responseCacheKeys))
	for i, value := range values {
		if value.Type() != astjson.TypeObject {
			continue
		}
		items = append(items, caching.Item{
			Key:   prepared.responseCacheKeys[i],
			Value: value.MarshalTo(nil),
			TTL:   ttl,
		})
	}

	prepared.responseCacheItems = items
	return nil
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

var (
	entitiesResponsePrefix         = []byte(`{"data":{"_entities":[`)
	entitiesResponseSuffix         = []byte(`]}}`)
	defaultResponseCacheErrorsPath = []string{"errors"}
)
