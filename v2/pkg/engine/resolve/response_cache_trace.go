package resolve

import (
	"encoding/json"
	"strings"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/caching"
)

// ResponseCacheRequestTrace is the response cache's part in a request, for ART.
type ResponseCacheRequestTrace struct {
	// SurrogateKeys is the union over every fetch, before any header limit.
	SurrogateKeys []string `json:"surrogate_keys,omitempty"`
	// Details is what ResponseCacheOptions.TraceDetails returned.
	Details any `json:"details,omitempty"`
}

func (c *Context) responseCacheRequestTrace() *ResponseCacheRequestTrace {
	if c.responseCache == nil {
		return nil
	}
	trace := &ResponseCacheRequestTrace{SurrogateKeys: c.responseCache.surrogateKeys}
	if c.responseCache.traceDetails != nil {
		trace.Details = c.responseCache.traceDetails(trace.SurrogateKeys)
	}
	return trace
}

// ResponseCacheTrace is what the response cache did for a fetch, for ART.
type ResponseCacheTrace struct {
	Status        string `json:"status"`
	StoreDecision string `json:"store_decision,omitempty"`
	KeysRequested int    `json:"keys_requested,omitempty"`
	KeysFound     int    `json:"keys_found,omitempty"`
	// PrivateID reports the request carried a user id for this subgraph.
	PrivateID            bool   `json:"private_id,omitempty"`
	LookupDurationNano   int64  `json:"lookup_duration_nanoseconds,omitempty"`
	LookupDurationPretty string `json:"lookup_duration_pretty,omitempty"`
	// CacheControl is what the subgraph answered with, absent on a hit.
	CacheControl string                    `json:"cache_control,omitempty"`
	Vary         string                    `json:"vary,omitempty"`
	WriteError   string                    `json:"write_error,omitempty"`
	Entries      []ResponseCacheTraceEntry `json:"entries,omitempty"`
}

// ResponseCacheTraceEntry is one entity of a fetch, by key position.
type ResponseCacheTraceEntry struct {
	// Alias names the entry of a merged entity fetch, Path its response path.
	Alias      string `json:"alias,omitempty"`
	Path       string `json:"path,omitempty"`
	Index      int    `json:"index"`
	Key        string `json:"key"`
	PrivateKey string `json:"private_key,omitempty"`
	// Found is the lookup found an entry, Served the entity came from it.
	Found  bool `json:"found"`
	Served bool `json:"served"`
	// Private is the entry found or written under the per-user key.
	// Written is false on a failed write; its TTL and tags are what was attempted.
	Private            bool            `json:"private,omitempty"`
	RemainingTTLNano   int64           `json:"remaining_ttl_nanoseconds,omitempty"`
	RemainingTTLPretty string          `json:"remaining_ttl_pretty,omitempty"`
	Vary               [][]string      `json:"vary,omitempty"`
	Written            bool            `json:"written"`
	WrittenKey         string          `json:"written_key,omitempty"`
	WriteTTLNano       int64           `json:"write_ttl_nanoseconds,omitempty"`
	WriteTTLPretty     string          `json:"write_ttl_pretty,omitempty"`
	TypeName           string          `json:"type_name,omitempty"`
	DeclaredTags       []string        `json:"declared_tags,omitempty"`
	Tags               []string        `json:"tags,omitempty"`
	SurrogateKeys      []string        `json:"surrogate_keys,omitempty"`
	Value              json.RawMessage `json:"value,omitempty"`
}

// responseCacheTraceKeys is one group of keys: a fetch, or a merged fetch's entry.
type responseCacheTraceKeys struct {
	alias       string
	path        string
	keys        []string
	privateKeys []string
	served      bool
}

// setResponseCacheTrace records the cache's part in a fetch once its write is done.
func (l *Loader) setResponseCacheTrace(prepared *preparedFetch, written []caching.Item, writeErr error) {
	if prepared.trace == nil || l.ctx.TracingOptions.ExcludeResponseCache || !l.responseCacheEnabled() {
		return
	}

	res := prepared.res
	info := res.responseCache
	trace := &ResponseCacheTrace{
		Status:        info.Status.String(),
		KeysRequested: info.KeysRequested,
		KeysFound:     info.KeysFound,
	}
	if info.StoreDecision != caching.StoreDecisionNone {
		trace.StoreDecision = info.StoreDecision.String()
	}
	if info.LookupDuration > 0 && !l.ctx.TracingOptions.EnablePredictableDebugTimings {
		trace.LookupDurationNano = info.LookupDuration.Nanoseconds()
		trace.LookupDurationPretty = info.LookupDuration.String()
	}
	if headers := responseCacheHeaders(res); headers != nil {
		trace.CacheControl = headers.Get("Cache-Control")
		trace.Vary = headers.Get("Vary")
	}
	if writeErr != nil {
		trace.WriteError = writeErr.Error()
	}

	groups := l.responseCacheTraceGroups(prepared)
	if len(groups) == 0 {
		prepared.trace.ResponseCache = trace
		return
	}

	bodies := make(map[string]caching.Item, len(written))
	for _, item := range written {
		if len(item.Vary) == 0 {
			bodies[responseCacheBaseKey(item.Key)] = item
		}
	}

	subgraph := res.ds.Name
	for _, group := range groups {
		trace.PrivateID = trace.PrivateID || group.privateKeys != nil
		for i, key := range group.keys {
			entry := ResponseCacheTraceEntry{Alias: group.alias, Path: group.path, Index: i, Key: key, Served: group.served}
			if group.privateKeys != nil {
				entry.PrivateKey = group.privateKeys[i]
			}
			value := l.traceLookup(prepared, &entry)
			if written := l.traceWrite(bodies, &entry); written != nil {
				value = written
			}
			l.traceValue(&entry, value)
			entry.Written = entry.Written && writeErr == nil
			entry.DeclaredTags = declaredSurrogateKeys(entry.SurrogateKeys, subgraph, entry.TypeName)
			trace.Entries = append(trace.Entries, entry)
		}
	}

	prepared.trace.ResponseCache = trace
}

func (l *Loader) responseCacheTraceGroups(prepared *preparedFetch) []responseCacheTraceKeys {
	if prepared.multiEntries == nil {
		if len(prepared.responseCacheKeys) == 0 {
			return nil
		}
		return []responseCacheTraceKeys{{
			keys:        prepared.responseCacheKeys,
			privateKeys: prepared.responseCachePrivateKeys,
			served:      prepared.responseCacheHit,
		}}
	}

	groups := make([]responseCacheTraceKeys, 0, len(prepared.multiEntries))
	for i := range prepared.multiEntries {
		entry := &prepared.multiEntries[i]
		if len(entry.responseCacheKeys) == 0 {
			continue
		}
		var path string
		if entry.entry.Item != nil {
			path = entry.entry.Item.ResponsePath
		}
		groups = append(groups, responseCacheTraceKeys{
			alias:       entry.entry.Alias,
			path:        path,
			keys:        entry.responseCacheKeys,
			privateKeys: entry.responseCachePrivateKeys,
			served:      entry.cacheHit(),
		})
	}
	return groups
}

// traceLookup fills in what the lookup found for the entry and returns its body.
func (l *Loader) traceLookup(prepared *preparedFetch, entry *ResponseCacheTraceEntry) []byte {
	item, _, private, ok := responseCachePick(prepared.responseCacheFound, entry.Key, entry.PrivateKey)
	if !ok {
		return nil
	}
	entry.Found = true
	entry.Private = private
	entry.RemainingTTLNano = item.TTL.Nanoseconds()
	entry.RemainingTTLPretty = item.TTL.String()

	if len(item.Vary) > 0 {
		entry.Vary = item.Vary
		body, served := prepared.responseCacheVariantServed[item.Key]
		if !served {
			return nil
		}
		item = body
	}
	entry.SurrogateKeys = item.SurrogateKeys
	return item.Value
}

// traceWrite fills in what was written for the entry and returns its body.
func (l *Loader) traceWrite(bodies map[string]caching.Item, entry *ResponseCacheTraceEntry) []byte {
	base := entry.Key
	item, ok := bodies[base]
	if !ok && entry.PrivateKey != "" {
		base = entry.PrivateKey
		item, ok = bodies[base]
	}
	if !ok {
		return nil
	}
	entry.Written = true
	entry.Private = base == entry.PrivateKey
	entry.WrittenKey = item.Key
	entry.WriteTTLNano = item.TTL.Nanoseconds()
	entry.WriteTTLPretty = item.TTL.String()
	entry.Tags = item.Tags
	entry.SurrogateKeys = item.SurrogateKeys
	return item.Value
}

func (l *Loader) traceValue(entry *ResponseCacheTraceEntry, value []byte) {
	if len(value) == 0 {
		return
	}
	if parsed, err := astjson.ParseBytes(value); err == nil {
		entry.TypeName = string(parsed.GetStringBytes("__typename"))
	}
	if !l.ctx.TracingOptions.ExcludeOutput {
		entry.Value = value
	}
}

// responseCacheBaseKey strips the variant from a key.
func responseCacheBaseKey(key string) string {
	base, _, _ := strings.Cut(key, "+")
	return base
}

// declaredSurrogateKeys is what the subgraph declared: the surrogate keys minus the derived tiers.
func declaredSurrogateKeys(surrogateKeys []string, subgraph, typeName string) []string {
	var declared []string
	for _, key := range surrogateKeys {
		if key == caching.SubgraphSurrogateKey(subgraph) || (typeName != "" && key == caching.TypeSurrogateKey(subgraph, typeName)) {
			continue
		}
		declared = append(declared, key)
	}
	return declared
}
