package resolve

import (
	"fmt"
	"net/http"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/entitycaching"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

// entityCacheEnabled reports whether this request was handed a cache. It is the
// only enablement check in the engine; there is no configuration to consult.
func (l *Loader) entityCacheEnabled() bool {
	return l.ctx != nil && l.ctx.entityCache != nil
}

func (l *Loader) reportEntityCacheError(err error) {
	if l.ctx == nil || l.ctx.entityCache == nil || l.ctx.entityCache.onError == nil {
		return
	}
	l.ctx.entityCache.onError(err)
}

func entityCacheSelectionHash(header, footer []byte) uint64 {
	d := pool.Hash64.Get()
	defer pool.Hash64.Put(d)
	_, _ = d.Write(header)
	// Written in between so that a byte moving from the end of the header to the
	// start of the footer cannot go unnoticed.
	_, _ = d.Write([]byte{0})
	_, _ = d.Write(footer)
	return d.Sum64()
}

func (l *Loader) entityCacheLookup(prepared *preparedFetch) bool {
	keys := prepared.entityCacheKeys
	if len(keys) == 0 {
		return false
	}

	found, err := l.ctx.entityCache.store.GetMany(l.ctx.ctx, keys)
	if err != nil {
		l.reportEntityCacheError(fmt.Errorf("entity cache lookup of %d keys: %w", len(keys), err))
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

func (l *Loader) entityCacheStore(prepared *preparedFetch) {
	keys := prepared.entityCacheKeys
	if len(keys) == 0 {
		return
	}

	res := prepared.res
	if res.err != nil || len(res.out) == 0 || res.statusCode >= 400 {
		return
	}

	response, err := res.parsedResponse(l)
	if err != nil {
		return
	}

	errorsPath := res.postProcessing.SelectResponseErrorsPath
	if errorsPath == nil {
		errorsPath = defaultEntityCacheErrorsPath
	}

	if errs := response.Get(errorsPath...); astjson.ValueIsNonNull(errs) && len(errs.GetArray()) > 0 {
		return
	}

	ttl, ok := entitycaching.TTL(entityCacheResponseHeaders(res), l.ctx.entityCache.defaultTTL)
	if !ok {
		return
	}

	entities := response.Get("data", "_entities")
	if entities == nil || entities.Type() != astjson.TypeArray {
		return
	}
	values := entities.GetArray()

	if len(values) != len(keys) {
		return
	}

	items := make([]entitycaching.Item, len(keys))
	for i, value := range values {
		if value.Type() != astjson.TypeObject {
			return
		}
		items[i] = entitycaching.Item{
			Key:   keys[i],
			Value: value.MarshalTo(nil),
			TTL:   ttl,
		}
	}

	if err := l.ctx.entityCache.store.SetMany(l.ctx.ctx, items); err != nil {
		l.reportEntityCacheError(fmt.Errorf("entity cache write of %d entities: %w", len(items), err))
	}
}

func entityCacheResponseHeaders(res *result) http.Header {
	if res.httpResponseContext == nil || res.httpResponseContext.Response == nil {
		return nil
	}
	return res.httpResponseContext.Response.Header
}

var (
	entitiesResponsePrefix       = []byte(`{"data":{"_entities":[`)
	entitiesResponseSuffix       = []byte(`]}}`)
	defaultEntityCacheErrorsPath = []string{"errors"}
)
