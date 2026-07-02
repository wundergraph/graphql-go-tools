package cache

import (
	"cmp"
	"slices"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

// cacheKeyTemplate is the SOLE source of read, write, and invalidate keys for
// ONE key candidate: the same template renders the same byte-identical key
// wherever it is used, so the read and write key spaces cannot silently
// diverge. The final key is "<prefix>:<16-hex xxhash64 of the preimage>",
// where the preimage is "<prefix>:<canonical rendered key JSON>". L1 and L2
// share these keys (derive once; the L1 store reuses them from task 17 on).
type cacheKeyTemplate struct {
	// prefix is the visible key prefix: the policy's CacheName, plus the
	// subgraph header hash when the policy varies by headers.
	prefix string
	// representation is the frozen @key template node for this candidate.
	representation *resolve.Object
}

// newCacheKeyTemplates derives one template per candidate for a fetch, from
// the config and the request's subgraph header hash.
func newCacheKeyTemplates(cfg *resolve.FetchCacheConfig, headerHash uint64) []cacheKeyTemplate {
	prefix := cacheKeyPrefix(cfg, headerHash)
	templates := make([]cacheKeyTemplate, 0, len(cfg.KeySpec.Candidates))
	for _, candidate := range cfg.KeySpec.Candidates {
		templates = append(templates, cacheKeyTemplate{
			prefix:         prefix,
			representation: candidate.Representation,
		})
	}
	return templates
}

// render produces the candidate's key for one entity item, best-effort: it
// returns ok=false when any referenced key field is absent or null in the
// item (an unrenderable candidate is skipped, never an error).
func (t cacheKeyTemplate) render(item *astjson.Value) (string, bool) {
	if t.representation == nil || item == nil {
		return "", false
	}
	typename := item.Get("__typename")
	if typename == nil {
		// Entity items always carry __typename in federation responses; the
		// template's type name stands in when a caller renders from a value
		// that legitimately lacks it (e.g. argument-derived lookups, task 15).
		if t.representation.TypeName == "" {
			return "", false
		}
		typename = astjson.StringValue(nil, t.representation.TypeName)
	}
	keyObj := astjson.ObjectValue(nil)
	keyObj.Set(nil, "__typename", typename)
	keysObj := astjson.ObjectValue(nil)
	renderedFields := 0
	for _, field := range t.representation.Fields {
		name := string(field.Name)
		if name == "__typename" {
			continue
		}
		value, ok := renderRepresentationValue(field.Value, item.Get(name))
		if !ok {
			return "", false
		}
		keysObj.Set(nil, name, value)
		renderedFields++
	}
	if renderedFields == 0 {
		// A key without any key field would collide across all entities of the
		// type; treat it as unrenderable.
		return "", false
	}
	keyObj.Set(nil, "key", keysObj)
	return renderCacheKey(t.prefix, keyObj.MarshalTo(nil)), true
}

// renderRepresentationValue extracts the canonical key value for one template
// node from the item: objects recurse over the template's fields, scalars pass
// through. Numbers are unified with STRINGS of the same literal (the number 1
// and the string "1" render the same key material) — astjson preserves the
// original literal, so 1 and 1.0 remain DISTINCT keys: a conservative split
// (extra miss, never wrong data). Full numeric canonicalization is deliberately
// avoided: parsing to float64 would corrupt integers beyond 2^53. A null or
// absent value makes the candidate unrenderable.
func renderRepresentationValue(node resolve.Node, value *astjson.Value) (*astjson.Value, bool) {
	if value == nil || value.Type() == astjson.TypeNull {
		return nil, false
	}
	switch typed := node.(type) {
	case *resolve.Object:
		if value.Type() != astjson.TypeObject {
			return nil, false
		}
		out := astjson.ObjectValue(nil)
		rendered := 0
		for _, field := range typed.Fields {
			name := string(field.Name)
			if name == "__typename" {
				continue
			}
			child, ok := renderRepresentationValue(field.Value, value.Get(name))
			if !ok {
				return nil, false
			}
			out.Set(nil, name, child)
			rendered++
		}
		return out, rendered > 0
	default:
		if value.Type() == astjson.TypeNumber {
			return astjson.StringValue(nil, string(value.MarshalTo(nil))), true
		}
		return value, true
	}
}

// cacheKeyPrefix returns the visible key prefix: the policy's CacheName, plus
// "h<headerHash>" when the policy folds the subgraph header hash into the key
// (the sole vary-by knob).
func cacheKeyPrefix(cfg *resolve.FetchCacheConfig, headerHash uint64) string {
	if cfg == nil {
		return ""
	}
	if cfg.IncludeSubgraphHeaderPrefix {
		return cfg.CacheName + ":h" + hex64(headerHash)
	}
	return cfg.CacheName
}

// renderCacheKey hashes the preimage "<prefix>:<payload>" with the pooled
// xxhash64 and returns "<prefix>:<16-hex sum>" (or the bare sum when there is
// no prefix).
func renderCacheKey(prefix string, payload []byte) string {
	if prefix == "" {
		return hashHex(payload)
	}
	preimage := make([]byte, 0, len(prefix)+1+len(payload))
	preimage = append(preimage, prefix...)
	preimage = append(preimage, ':')
	preimage = append(preimage, payload...)
	return prefix + ":" + hashHex(preimage)
}

func hashHex(value []byte) string {
	h := pool.Hash64.Get()
	_, _ = h.Write(value)
	sum := h.Sum64()
	pool.Hash64.Put(h)
	return hex64(sum)
}

func hex64(sum uint64) string {
	var buf [16]byte
	const digits = "0123456789abcdef"
	for i := 15; i >= 0; i-- {
		buf[i] = digits[sum&0xf]
		sum >>= 4
	}
	return string(buf[:])
}

// rootFieldCacheKey renders the whole-response root-field key: the policy
// prefix plus a preimage of the fetch's root-field coordinate and the request
// variables in canonical (name-sorted) form. The QUERY TEXT is deliberately
// excluded so alias-variant operations share the entry (coverage and
// normalization guard servability and shape). PRECONDITION: operations are
// normalized with variable extraction (the engine always does this), so inline
// argument literals are variables and cannot collide under one key.
func rootFieldCacheKey(cfg *resolve.FetchCacheConfig, headerHash uint64, ctx *resolve.Context) string {
	prefix := cacheKeyPrefix(cfg, headerHash)
	preimage := make([]byte, 0, 64)
	preimage = append(preimage, cfg.KeySpec.TypeName...)
	preimage = append(preimage, '.')
	preimage = append(preimage, cfg.KeySpec.FieldName...)
	preimage = append(preimage, ':')
	preimage = append(preimage, canonicalVariables(ctx)...)
	return renderCacheKey(prefix, preimage)
}

// canonicalVariables renders the request variables with name-sorted top-level
// keys, so clients sending the same variables in different order share keys.
func canonicalVariables(ctx *resolve.Context) []byte {
	if ctx == nil || ctx.Variables == nil {
		return []byte("null")
	}
	obj, err := ctx.Variables.Object()
	if err != nil {
		return ctx.Variables.MarshalTo(nil)
	}
	type pair struct {
		name  string
		value *astjson.Value
	}
	pairs := make([]pair, 0, obj.Len())
	obj.Visit(func(key []byte, v *astjson.Value) {
		pairs = append(pairs, pair{name: string(key), value: v})
	})
	slices.SortFunc(pairs, func(a, b pair) int {
		return cmp.Compare(a.name, b.name)
	})
	out := make([]byte, 0, 64)
	out = append(out, '{')
	for i, p := range pairs {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, '"')
		out = append(out, p.name...)
		out = append(out, '"', ':')
		out = p.value.MarshalTo(out)
	}
	out = append(out, '}')
	return out
}
