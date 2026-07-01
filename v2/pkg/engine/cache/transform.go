package cache

import (
	"cmp"
	"slices"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

// The transform module: values are cached in NORMALIZED form — schema field
// names (aliases resolved via Field.OriginalName) with a deterministic
// argument suffix folded into the object key (from Field.CacheArgs and the
// request's variable values) — and DENORMALIZED back to the requesting
// operation's aliases at serve time. Normalization is what makes a value
// cached by one operation reusable by another with different aliases, and the
// argument suffix is what keeps `friends(first:5)` from ever serving
// `friends(first:20)`. Partial fetching (task 19) reuses these walks per field.

// normalizedFieldName is the SINGLE derivation of a field's cache-side key:
// the schema name (OriginalName when aliased) plus the argument suffix. The
// write path, the coverage walk, and the read path all go through it, so the
// key spaces cannot diverge.
func normalizedFieldName(ctx *resolve.Context, field *resolve.Field) string {
	name := string(field.Name)
	if len(field.OriginalName) > 0 {
		name = string(field.OriginalName)
	}
	if len(field.CacheArgs) == 0 {
		return name
	}
	return name + computeArgSuffix(ctx, field.CacheArgs)
}

// computeArgSuffix hashes the field's variable-bound argument values into a
// deterministic "_<16-hex xxhash64>" suffix: args sorted by name, each value
// taken from the request variables (through the remap that normalization may
// have applied), absent values hashed as null.
func computeArgSuffix(ctx *resolve.Context, args []resolve.CacheFieldArg) string {
	sorted := args
	if !slices.IsSortedFunc(sorted, func(a, b resolve.CacheFieldArg) int {
		return cmp.Compare(a.Name, b.Name)
	}) {
		sorted = slices.Clone(args)
		slices.SortFunc(sorted, func(a, b resolve.CacheFieldArg) int {
			return cmp.Compare(a.Name, b.Name)
		})
	}
	h := pool.Hash64.Get()
	for i, arg := range sorted {
		if i > 0 {
			_, _ = h.WriteString(",")
		}
		_, _ = h.WriteString(arg.Name)
		_, _ = h.WriteString(":")
		var value *astjson.Value
		if ctx != nil && ctx.Variables != nil {
			variableName := arg.VariableName
			if mapped, ok := ctx.RemapVariables[variableName]; ok {
				variableName = mapped
			}
			value = ctx.Variables.Get(variableName)
		}
		if value == nil {
			_, _ = h.WriteString("null")
		} else {
			_, _ = h.Write(value.MarshalTo(nil))
		}
	}
	sum := h.Sum64()
	pool.Hash64.Put(h)
	return "_" + hex64(sum)
}

// normalizeToSchema rewrites a fetched (alias-shaped) value into the stored
// form: object keys become normalized field names, recursively; fields the
// tree does not select (e.g. __typename, key fields) are preserved as-is. The
// caller gates on HasAliases — a tree without aliases or args stores the raw
// value unchanged.
func normalizeToSchema(tx *resolve.CacheTransaction, ctx *resolve.Context, value *astjson.Value, node resolve.Node) *astjson.Value {
	if value == nil || node == nil {
		return value
	}
	switch typed := node.(type) {
	case *resolve.Object:
		if value.Type() != astjson.TypeObject {
			return value
		}
		out := tx.NewObject()
		seen := make(map[string]struct{}, len(typed.Fields))
		for _, field := range typed.Fields {
			responseKey := string(field.Name)
			fieldValue := value.Get(responseKey)
			if fieldValue == nil {
				continue
			}
			out.Set(nil, normalizedFieldName(ctx, field), normalizeToSchema(tx, ctx, fieldValue, field.Value))
			seen[responseKey] = struct{}{}
		}
		obj, err := value.Object()
		if err != nil {
			return value
		}
		obj.Visit(func(key []byte, fieldValue *astjson.Value) {
			responseKey := string(key)
			if _, ok := seen[responseKey]; ok {
				return
			}
			out.Set(nil, responseKey, fieldValue)
		})
		return out
	case *resolve.Array:
		if value.Type() != astjson.TypeArray {
			return value
		}
		items, err := value.Array()
		if err != nil {
			return value
		}
		out := tx.NewArray()
		for i, item := range items {
			out.SetArrayItem(nil, i, normalizeToSchema(tx, ctx, item, typed.Item))
		}
		return out
	default:
		return value
	}
}

// denormalizeToSelection rewrites a cached (normalized) value into the
// REQUESTING operation's shape: fields come out under their aliases, in
// selection order, with cached-only extras appended after (so a write-back or
// resolver never loses data the selection did not name). It always builds a
// fresh transaction-owned value, which also serves as the aliasing-safe copy
// for the splice.
func denormalizeToSelection(tx *resolve.CacheTransaction, ctx *resolve.Context, value *astjson.Value, node resolve.Node) *astjson.Value {
	if value == nil || node == nil {
		return value
	}
	switch typed := node.(type) {
	case *resolve.Object:
		if value.Type() != astjson.TypeObject {
			return value
		}
		out := tx.NewObject()
		seen := make(map[string]struct{}, len(typed.Fields))
		for _, field := range typed.Fields {
			storedKey := normalizedFieldName(ctx, field)
			fieldValue := value.Get(storedKey)
			if fieldValue == nil {
				continue
			}
			out.Set(nil, string(field.Name), denormalizeToSelection(tx, ctx, fieldValue, field.Value))
			seen[storedKey] = struct{}{}
		}
		obj, err := value.Object()
		if err != nil {
			return value
		}
		obj.Visit(func(key []byte, fieldValue *astjson.Value) {
			storedKey := string(key)
			if _, ok := seen[storedKey]; ok {
				return
			}
			out.Set(nil, storedKey, fieldValue)
		})
		return out
	case *resolve.Array:
		if value.Type() != astjson.TypeArray {
			return value
		}
		items, err := value.Array()
		if err != nil {
			return value
		}
		out := tx.NewArray()
		for i, item := range items {
			out.SetArrayItem(nil, i, denormalizeToSelection(tx, ctx, item, typed.Item))
		}
		return out
	default:
		return value
	}
}
