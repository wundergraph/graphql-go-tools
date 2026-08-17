package resolve

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/buger/jsonparser"

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
	if res.err != nil || len(res.out) == 0 {
		return
	}
	if res.statusCode >= 400 {
		return
	}

	if errs, dataType, _, err := jsonparser.Get(res.out, "errors"); err == nil &&
		dataType != jsonparser.NotExist && dataType != jsonparser.Null && !isEmptyJSONArray(errs) {
		return
	}

	ttl, ok := entitycaching.TTL(entityCacheResponseHeaders(res), l.ctx.entityCache.defaultTTL)
	if !ok {
		return
	}

	// Collected first and matched to keys afterwards. ArrayEach cannot be
	// stopped part way, so counting the whole array is the only way to tell a
	// response of the expected shape from one that merely starts out that way.
	entities := make([][]byte, 0, len(keys))
	usable := true
	_, err := jsonparser.ArrayEach(res.out, func(value []byte, dataType jsonparser.ValueType, _ int, _ error) {
		// A null entry is legal GraphQL, normally alongside an error saying why.
		// There is no key it could be stored under that would not later be read
		// back as a real entity, so one of them spoils the whole batch.
		if dataType != jsonparser.Object {
			usable = false
			return
		}
		entities = append(entities, value)
	}, "data", "_entities")
	if err != nil || !usable {
		return
	}

	// The same count the loader itself insists on when merging. If it does not
	// hold, this response is not the shape the keys were built for and no
	// element can be trusted to belong to any particular key.
	if len(entities) != len(keys) {
		return
	}

	items := make([]entitycaching.Item, len(keys))
	for i, value := range entities {
		items[i] = entitycaching.Item{
			Key: keys[i],
			// Copied, not referenced. value points into the subgraph response
			// buffer, and an in-memory cache holding onto that slice would pin
			// the whole response body for as long as one entity lives.
			Value: bytes.Clone(value),
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

// isEmptyJSONArray reports whether value is "[]". jsonparser hands arrays back
// with their brackets, and a subgraph that always writes an errors key sends an
// empty one on success, which must not read as a response carrying errors.
func isEmptyJSONArray(value []byte) bool {
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) < 2 || trimmed[0] != '[' || trimmed[len(trimmed)-1] != ']' {
		return false
	}
	return len(bytes.TrimSpace(trimmed[1:len(trimmed)-1])) == 0
}

var (
	entitiesResponsePrefix = []byte(`{"data":{"_entities":[`)
	entitiesResponseSuffix = []byte(`]}}`)
)
