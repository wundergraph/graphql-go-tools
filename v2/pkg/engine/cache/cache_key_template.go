package cache

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"slices"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

// cacheFormatVersion leads every L2 key and its preimage: it segments the
// keyspace by STORAGE FORMAT, so changing the key layout or the value envelope
// makes older entries unreachable instead of misreadable.
const cacheFormatVersion = "v1"

// cacheKeyTemplate is the SOLE source of read, write, and invalidate keys for
// ONE fetch: the same template renders the same byte-identical keys wherever it
// is used, so the read and write key spaces cannot silently diverge.
type cacheKeyTemplate struct {
	// subgraph is the fetch's subgraph name: every subgraph owns its keyspace,
	// in both layers.
	subgraph string
	// prefix is the visible L2 key prefix — format version, subgraph, and the
	// subgraph header hash when the config varies by headers. It is EMPTY for a
	// fetch that does not use L2, which is what keeps an L1-only fetch away
	// from prefix material and hashing entirely.
	prefix string
	// representation is the fetch's merged representation node — the same node
	// its input template renders on the wire.
	representation *resolve.Object
	// args is the digest of the request's argument values for the fetch's
	// parameterized fields, EMPTY when the fetch selects none. It is derived
	// once per fetch per request (the values come from the variables, the field
	// set from the plan) and is the same for every item of a batch.
	args string
	// partition is the hashed requester identity of a statically-private fetch,
	// EMPTY for a public one. It enters the L2 preimage ONLY: the L1 map is
	// per-request and therefore per-requester, so partitioning it would only
	// fragment it.
	partition string
}

// l2Access is one fetch's per-request store access: whether it may touch the
// store at all, and the partition segment its keys carry when it is private.
// A statically-private fetch whose request carries no identity resolves to a
// disabled access — it must not read from or write to a shared keyspace it
// cannot partition.
type l2Access struct {
	enabled   bool
	partition string
}

// itemKeys are the two keys one cached item is addressed by, derived from a
// single rendering:
//
//	body = {"__typename":T,"representation":{...}[,"args":D]
//	L1   = {subgraph}:<body>}                                      (raw preimage)
//	L2   = v1:{subgraph}[:h<headerHash>]:<16-hex xxhash64 of "<prefix>:<body>[,"partition":"P"]}">
//
// The L1 key is the preimage itself: the map hashes internally, nothing
// persists (no format version needed), and the map is per-requester (no header
// hash and no partition segment needed). L2 is empty when the fetch does not
// use the store.
type itemKeys struct {
	L1 string
	L2 string
}

// newCacheKeyTemplate derives the fetch's key template from the config, the
// request's variables, its subgraph header hash, and its resolved store access.
// It runs ONCE per fetch — the argument digest walk is per fetch, never per
// item — and returns ok=false when the config carries no representation node
// (nothing to key on).
func newCacheKeyTemplate(ctx *resolve.Context, cfg *resolve.FetchCacheConfig, headerHash uint64, l2 l2Access) (cacheKeyTemplate, bool) {
	if cfg == nil || cfg.KeySpec.Representation == nil {
		return cacheKeyTemplate{}, false
	}
	template := cacheKeyTemplate{
		subgraph:       cfg.SubgraphName,
		representation: cfg.KeySpec.Representation,
		args:           argsDigest(ctx, cfg.ProvidesData),
	}
	if l2.enabled {
		template.prefix = cacheKeyPrefix(cfg, headerHash)
		template.partition = l2.partition
	}
	return template, true
}

// render derives the item's keys from ONE canonical representation rendering:
// the L1 key is that preimage body closed as it stands, the L2 key the
// versioned, hashed derivation over the same body plus the partition segment.
// ok=false when a referenced field is absent or null in the item — the same
// condition under which the fetch input itself could not render, so an
// unrenderable key means a miss with no write, never an error.
func (t cacheKeyTemplate) render(item *astjson.Value) (itemKeys, bool) {
	body, ok := t.renderRepresentation(item)
	if !ok {
		return itemKeys{}, false
	}
	keys := itemKeys{L1: preimageCore(t.subgraph, body)}
	if t.prefix != "" {
		// Extending the body in place is safe because the L1 key above has
		// already copied it into its own string.
		l2Preimage := append(appendPartition(body, t.partition), '}')
		keys.L2 = renderCacheKey(t.prefix, l2Preimage)
	}
	return keys, true
}

// preimageCore joins the subgraph segment and the rendered body into the L1
// key, closing the preimage object.
func preimageCore(subgraph string, body []byte) string {
	core := make([]byte, 0, len(subgraph)+len(body)+2)
	core = append(core, subgraph...)
	core = append(core, ':')
	core = append(core, body...)
	core = append(core, '}')
	return string(core)
}

// appendPartition adds the requester's partition segment to an L2 preimage; a
// public fetch (empty partition) appends NOTHING, so its keys stay byte-for-byte
// what they were before privacy existed.
func appendPartition(preimage []byte, partition string) []byte {
	if partition == "" {
		return preimage
	}
	preimage = append(preimage, `,"partition":"`...)
	preimage = append(preimage, partition...)
	return append(preimage, '"')
}

// sha256Hex hashes a partition value into its key segment. Partition
// collisions are cross-requester data leaks, so the 64-bit key hash is not
// good enough here: an identity must not be forgeable into another's partition.
func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// renderRepresentation writes the canonical rendering of the fetch's
// representation against the item, so the key material is EXACTLY what the
// fetch sends. The canonical JSON is written DIRECTLY to a byte buffer — the
// hot lookup path builds no intermediate astjson values (profiled: value
// building dominated the cache-side allocations). The returned body is left
// UNCLOSED (no trailing `}`): each layer closes it, and only L2 inserts a
// partition segment first.
func (t cacheKeyTemplate) renderRepresentation(item *astjson.Value) ([]byte, bool) {
	typeName := t.itemTypeName(item)
	if typeName == "" {
		return nil, false
	}
	preimage := make([]byte, 0, 64)
	preimage = append(preimage, `{"__typename":"`...)
	preimage = append(preimage, typeName...)
	preimage = append(preimage, `","representation":`...)
	representationStart := len(preimage)
	preimage, ok := appendRepresentationObject(preimage, t.representation, item, wholeRepresentation)
	if !ok {
		return nil, false
	}
	if len(preimage) == representationStart+2 {
		// A representation without any field ("{}") would collide across all
		// entities of the type; treat it as unrenderable.
		return nil, false
	}
	if t.args != "" {
		// A fetch selecting no parameterized field appends nothing — not an empty
		// segment — so its preimage stays the plain representation core.
		preimage = append(preimage, `,"args":"`...)
		preimage = append(preimage, t.args...)
		preimage = append(preimage, '"')
	}
	return preimage, true
}

// itemTypeName is the type an item keys and tags under. The node's __typename
// wins when the planner baked it in as a StaticString (the @interfaceObject /
// entity-interface remap): that is the name the fetch sends on the wire, so
// keying under it keeps ONE entry per entity instead of one per implementer.
// Otherwise the item's own __typename decides. "" when neither is available, or
// when there is no representation node or no item at all.
func (t cacheKeyTemplate) itemTypeName(item *astjson.Value) string {
	if t.representation == nil || item == nil {
		return ""
	}
	if typeName := staticTypeName(t.representation); typeName != "" {
		return typeName
	}
	return valueTypeName(item)
}

// argsDigest folds the request's argument VALUES for every parameterized field
// of the fetch's selection into one 16-hex digest, "" when the selection has
// none — without it two requests differing only in an argument value would
// share one entry and replace each other's value on every write. The digest
// input is the SORTED set of normalized field PATHS from the ProvidesData
// root, the last segment carrying the field's argument suffix: the path keeps
// same-named fields at different positions apart, the schema names and the
// value-derived suffix make aliases, selection order, and variable names
// irrelevant. The digest belongs to the fetch, never to an item.
func argsDigest(ctx *resolve.Context, provides *resolve.Object) string {
	if provides == nil || !provides.HasAliases {
		// HasAliases marks every object holding an aliased or parameterized field
		// below it, so a false here proves there are no arguments to fold in —
		// the same gate the normalize and denormalize walks run on.
		return ""
	}
	entries := appendParameterizedFields(ctx, provides, "", nil)
	if len(entries) == 0 {
		return ""
	}
	slices.Sort(entries)
	h := pool.Hash64.Get()
	for i, entry := range entries {
		if i > 0 {
			_, _ = h.WriteString(",")
		}
		_, _ = h.WriteString(entry)
	}
	sum := h.Sum64()
	pool.Hash64.Put(h)
	return hex64(sum)
}

// appendParameterizedFields collects the normalized paths of every field
// carrying arguments below obj, at ANY depth. A leaf field without arguments
// costs nothing: its name is only built when it is an entry or a path prefix.
func appendParameterizedFields(ctx *resolve.Context, obj *resolve.Object, path string, entries []string) []string {
	for _, field := range obj.Fields {
		child := parameterizedSubtree(field.Value)
		if len(field.CacheArgs) == 0 && child == nil {
			continue
		}
		name := path + normalizedFieldName(ctx, field)
		if len(field.CacheArgs) > 0 {
			entries = append(entries, name)
		}
		if child != nil {
			entries = appendParameterizedFields(ctx, child, name+".", entries)
		}
	}
	return entries
}

// parameterizedSubtree unwraps a field value to the object it selects, through
// any list nesting — but only when that object's subtree can carry arguments at
// all, so the walk descends into the annotated parts of the tree and nothing
// else. nil for a leaf or an argument-free subtree.
func parameterizedSubtree(node resolve.Node) *resolve.Object {
	for {
		switch typed := node.(type) {
		case *resolve.Object:
			if !typed.HasAliases {
				return nil
			}
			return typed
		case *resolve.Array:
			if typed.Item == nil {
				return nil
			}
			node = typed.Item
		default:
			return nil
		}
	}
}

// staticTypeName returns the node's baked-in __typename when the planner made it
// a StaticString (the @interfaceObject / entity-interface remap), "" otherwise.
func staticTypeName(node *resolve.Object) string {
	for _, field := range node.Fields {
		if string(field.Name) != "__typename" {
			continue
		}
		if static, ok := field.Value.(*resolve.StaticString); ok {
			return static.Value
		}
		return ""
	}
	return ""
}

// representationScope selects which fields of a representation node a render
// covers: the whole node — the fetch's identity, `@key` and `@requires` alike —
// or its `@key` fields alone, which is the entity a tag addresses.
type representationScope uint8

const (
	wholeRepresentation representationScope = iota
	keyFieldsOnly
)

// appendRepresentationObject writes one canonical representation object from
// the item: fields in node order (GraphQL names never need JSON escaping),
// objects recurse, and every field that APPLIES must render. A merged
// representation for an abstract-type fetch carries per-concrete-type
// conditioned fields, so a field whose OnTypeNames exclude the item's type is
// skipped — exactly the coverage walk's rule (an absent __typename keeps
// conditioned fields required, the conservative choice). Under keyFieldsOnly a
// field outside the entity's `@key` set is skipped the same way.
func appendRepresentationObject(buf []byte, node *resolve.Object, value *astjson.Value, scope representationScope) ([]byte, bool) {
	typeName := valueTypeName(value)
	buf = append(buf, '{')
	rendered := 0
	for _, field := range node.Fields {
		name := string(field.Name)
		if name == "__typename" {
			continue
		}
		if scope == keyFieldsOnly && !field.CacheEntityKeyField {
			continue
		}
		if skipFieldForTypeName(field, typeName) {
			continue
		}
		if rendered > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, '"')
		buf = append(buf, name...)
		buf = append(buf, '"', ':')
		var ok bool
		buf, ok = appendRepresentationValue(buf, field.Value, value.Get(name), scope)
		if !ok {
			return buf, false
		}
		rendered++
	}
	if rendered == 0 && len(node.Fields) > 0 {
		// Only __typename and non-applicable conditioned fields: nothing
		// key-worthy below this node.
		return buf, false
	}
	buf = append(buf, '}')
	return buf, true
}

// appendRepresentationValue writes the canonical key value for one template
// node from the item: objects recurse over the template's fields, scalars pass
// through. Numbers are unified with STRINGS of the same literal (the number 1
// and the string "1" render the same key material) — astjson preserves the
// original literal, so 1 and 1.0 remain DISTINCT keys: a conservative split
// (extra miss, never wrong data). Full numeric canonicalization is deliberately
// avoided: parsing to float64 would corrupt integers beyond 2^53. A null or
// absent value makes the key unrenderable.
func appendRepresentationValue(buf []byte, node resolve.Node, value *astjson.Value, scope representationScope) ([]byte, bool) {
	if value == nil || value.Type() == astjson.TypeNull {
		return buf, false
	}
	switch typed := node.(type) {
	case *resolve.Object:
		if value.Type() != astjson.TypeObject {
			return buf, false
		}
		return appendRepresentationObject(buf, typed, value, scope)
	default:
		if value.Type() == astjson.TypeNumber {
			buf = append(buf, '"')
			buf = value.MarshalTo(buf)
			buf = append(buf, '"')
			return buf, true
		}
		return value.MarshalTo(buf), true
	}
}

// cacheKeyPrefix returns the visible L2 key prefix: the format version, the
// fetch's subgraph name — each subgraph owns its keyspace — and
// "h<headerHash>" when a PUBLIC fetch varies by its forwarded subgraph headers.
// On a private fetch that same hash is the requester IDENTITY instead and lives
// in the hashed partition segment, so the prefix stays plain: one fetch is
// partitioned by exactly one mechanism.
func cacheKeyPrefix(cfg *resolve.FetchCacheConfig, headerHash uint64) string {
	prefix := cacheFormatVersion + ":" + cfg.SubgraphName
	if cfg.IncludeSubgraphHeaders && !cfg.Private {
		return prefix + ":h" + hex64(headerHash)
	}
	return prefix
}

// renderCacheKey hashes the preimage "<prefix>:<payload>" with the pooled
// xxhash64 and returns "<prefix>:<16-hex sum>": the visible prefix is part of
// the hashed material, so keys of different prefixes can never collide on their
// digests alone.
func renderCacheKey(prefix string, payload []byte) string {
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

// rootFieldCacheKey renders the whole-response root-field L2 key in the same
// layout as entity keys — "v1:{subgraph}[:h<headerHash>]:<digest>" — over a
// preimage of the fetch's root-field coordinate and the request variables in
// canonical (name-sorted) form. The QUERY TEXT is deliberately
// excluded so alias-variant operations share the entry (coverage and
// normalization guard servability and shape). PRECONDITION: operations are
// normalized with variable extraction (the engine always does this), so inline
// argument literals are variables and cannot collide under one key.
func rootFieldCacheKey(cfg *resolve.FetchCacheConfig, headerHash uint64, ctx *resolve.Context, partition string) string {
	prefix := cacheKeyPrefix(cfg, headerHash)
	preimage := make([]byte, 0, 64)
	preimage = append(preimage, cfg.KeySpec.TypeName...)
	preimage = append(preimage, '.')
	preimage = append(preimage, cfg.KeySpec.FieldName...)
	preimage = append(preimage, ':')
	preimage = append(preimage, canonicalVariables(ctx)...)
	// Root fields carry the same partition segment as entities (their preimage
	// is not a JSON object, so it simply trails the variables).
	return renderCacheKey(prefix, appendPartition(preimage, partition))
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
