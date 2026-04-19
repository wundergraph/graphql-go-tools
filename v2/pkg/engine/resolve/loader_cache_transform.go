// StructuralCopy helpers for entity caching.
//
// This file hosts the four Loader StructuralCopy variants that isolate cache
// storage from the response tree:
//
//   - structuralCopyNormalized            — L2 write path: project to
//     ProvidesData fields only (rename aliases → schema names, drop unlisted).
//   - structuralCopyDenormalized          — L2 read path: rename schema names
//     back to the current query's aliases, projected to ProvidesData.
//   - structuralCopyNormalizedPassthrough — L1 write path: rename aliases but
//     KEEP source fields not listed in ProvidesData (@key fields, fields
//     contributed by sibling fetches). Driven by Transform.Passthrough = true.
//   - structuralCopyDenormalizedPassthrough — L1 read path: restore aliases
//     while preserving all accumulated fields from prior fetches.
//
// All four allocate onto l.jsonArena and return an *astjson.Value owned by
// the current request. StructuralCopy clones container nodes (objects,
// arrays) on the arena and ALIASES leaf nodes (strings, numbers, bools,
// nulls) from the source — safe because every live *astjson.Value within a
// request shares the same arena lifetime.
//
// Why the copies are load-bearing: astjson.MergeValues aliases nested
// container nodes from src into dst, so without a StructuralCopy isolating
// cached values, subsequent mutations of the response tree (a later fetch
// merging into the same item, or the L1 merge-into-existing writeback path)
// would reach back into and corrupt the cached entry. The L1
// merge-into-existing path pushes this further: it must also use
// working-copy-and-swap (StructuralCopy the live entry, MergeValues into
// the copy, Store the copy) because MergeValues is non-atomic on failure
// and a partial mutation of the live entry would corrupt every sibling L1
// key pointing at the same *Value.
//
// Ephemeral Transforms: the *astjson.Transform trees built here are
// constructed inline on the reusable transformEntries/transforms/
// transformMetas slabs and consumed by StructuralCopyWithTransform in the
// same call. They depend on per-request state (Context.Variables,
// RemapVariables flow into CacheArgs OutputKey suffixes), so they must NEVER
// be cached on *Object, the plan tree, the Resolver, or anywhere else that
// outlives a single request.
//
// The per-flow minimum-copy budget is tabulated in
// v2/pkg/engine/resolve/CLAUDE.md §"Copy Budget"; see also §"Entity L1
// Representation" for the full invariant set. Adversarial mutation tests in
// loader_cache_copy_invariant_test.go fail if any of these copies is
// dropped. A few cache-adjacent paths legitimately skip StructuralCopy —
// e.g. extension-based invalidation that consumes the extensions blob once
// and discards it — and document that at the call site.

package resolve

import (
	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
)

// structuralCopyNormalized applies a normalize transform (alias→schema name + arg hash)
// to v guided by obj, returning a structural copy on l.jsonArena.
// When obj is nil or has no aliases, falls back to plain StructuralCopy.
func (l *Loader) structuralCopyNormalized(v *astjson.Value, obj *Object) *astjson.Value {
	if obj == nil || !obj.HasAliases {
		return l.parser.StructuralCopy(l.jsonArena, v)
	}
	l.resetTransformSlabs(obj)
	t := l.buildNormalizeTransform(obj)
	return l.parser.StructuralCopyWithTransform(l.jsonArena, v, t)
}

// structuralCopyNormalizedPassthrough applies a normalize transform (alias→schema name + arg hash)
// with Passthrough=true, so unlisted fields are kept intact. Used for L1 writes
// where we need schema-shape field names but must preserve all entity fields
// (including @key fields not in ProvidesData).
func (l *Loader) structuralCopyNormalizedPassthrough(v *astjson.Value, obj *Object) *astjson.Value {
	if obj == nil || !obj.HasAliases {
		return l.parser.StructuralCopy(l.jsonArena, v)
	}
	l.resetTransformSlabs(obj)
	t := l.buildNormalizeTransform(obj)
	t.Passthrough = true
	return l.parser.StructuralCopyWithTransform(l.jsonArena, v, t)
}

// structuralCopyDenormalizedPassthrough applies a denormalize transform (schema→alias)
// with Passthrough=true, so unlisted fields are kept intact. Used for L1 reads
// where we need response-shape field names but must preserve all entity fields
// (including fields from other fetches not in this fetch's ProvidesData).
func (l *Loader) structuralCopyDenormalizedPassthrough(v *astjson.Value, obj *Object) *astjson.Value {
	if obj == nil || !obj.HasAliases {
		return l.parser.StructuralCopy(l.jsonArena, v)
	}
	l.resetTransformSlabs(obj)
	t := l.buildDenormalizeTransform(obj)
	t.Passthrough = true
	return l.parser.StructuralCopyWithTransform(l.jsonArena, v, t)
}

// structuralCopyDenormalized applies a denormalize transform (schema name→alias)
// to v guided by obj, returning a structural copy on l.jsonArena.
// When obj is nil or has no aliases, falls back to plain StructuralCopy.
func (l *Loader) structuralCopyDenormalized(v *astjson.Value, obj *Object) *astjson.Value {
	if obj == nil || !obj.HasAliases {
		return l.parser.StructuralCopy(l.jsonArena, v)
	}
	l.resetTransformSlabs(obj)
	t := l.buildDenormalizeTransform(obj)
	return l.parser.StructuralCopyWithTransform(l.jsonArena, v, t)
}

// fieldMeta stages per-field Transform data while children are being built.
// Kept at package level so it can live on the Loader's transformMetas slab
// (avoids a per-call `make([]fieldMeta, ...)` heap allocation).
type fieldMeta struct {
	inputKey  string
	outputKey string
	child     *astjson.Transform
}

// resetTransformSlabs resets and pre-grows the transform slabs to avoid
// reallocation during recursive tree building. Without sufficient capacity,
// slice appends during recursion can relocate the backing array, invalidating
// pointers (Transform*) and slice headers (Entries) set earlier.
func (l *Loader) resetTransformSlabs(obj *Object) {
	entries, transforms := countTransformAllocations(obj)

	l.transformEntries = l.transformEntries[:0]
	if cap(l.transformEntries) < entries {
		l.transformEntries = make([]astjson.TransformEntry, 0, entries)
	}

	l.transforms = l.transforms[:0]
	if cap(l.transforms) < transforms {
		l.transforms = make([]astjson.Transform, 0, transforms)
	}

	// transformMetas needs at most one slot per field across the tree.
	// entries is an upper bound (entries = fields + forced-__typename per object),
	// so it's safe and keeps the grow logic simple.
	l.transformMetas = l.transformMetas[:0]
	if cap(l.transformMetas) < entries {
		l.transformMetas = make([]fieldMeta, 0, entries)
	}
}

// countTransformAllocations counts the total TransformEntry and Transform
// allocations needed for an Object tree, so slabs can be pre-grown.
func countTransformAllocations(obj *Object) (entries, transforms int) {
	if obj == nil {
		return 0, 0
	}
	transforms = 1
	// One entry per field + one potential identity entry for __typename
	// when the selection set does not include it.
	entries = len(obj.Fields) + 1
	for _, field := range obj.Fields {
		ce, ct := countChildAllocations(field.Value)
		entries += ce
		transforms += ct
	}
	return entries, transforms
}

func countChildAllocations(node Node) (entries, transforms int) {
	switch n := node.(type) {
	case *Object:
		if n == nil || !n.HasAliases {
			return 0, 0
		}
		return countTransformAllocations(n)
	case *Array:
		if n == nil || n.Item == nil {
			return 0, 0
		}
		ce, ct := countChildAllocations(n.Item)
		if ct > 0 {
			ct++
		}
		return ce, ct
	}
	return 0, 0
}

// allocTransformIndex appends a zero Transform to the slab and returns its index.
func (l *Loader) allocTransformIndex() int {
	idx := len(l.transforms)
	l.transforms = append(l.transforms, astjson.Transform{})
	return idx
}

// buildNormalizeTransform builds a normalize transform tree. Children are built
// first (bottom-up) so their appends to transformEntries complete before the
// parent records its Entries slice range. When the selection set does not
// include __typename, an identity entry is appended so polymorphic type
// identity survives projection to the cache shape.
func (l *Loader) buildNormalizeTransform(obj *Object) *astjson.Transform {
	tIdx := l.allocTransformIndex()

	// Phase 1: reserve a per-call region on the transformMetas slab and fill it.
	// Pre-grown in resetTransformSlabs; recursive children append further down
	// the slab, but our `metas` slice stays valid because capacity never shrinks.
	metasStart := len(l.transformMetas)
	metasEnd := metasStart + len(obj.Fields)
	l.transformMetas = l.transformMetas[:metasEnd]
	metas := l.transformMetas[metasStart:metasEnd]
	hasTypenameField := false
	for i, field := range obj.Fields {
		metas[i].inputKey = unsafebytes.BytesToString(field.Name)
		metas[i].outputKey = l.cacheFieldName(field)
		if metas[i].outputKey == "__typename" {
			hasTypenameField = true
		}
		metas[i].child = l.buildNormalizeChild(field.Value)
	}

	// Phase 2: append entries contiguously (no interleaved child appends).
	entriesStart := len(l.transformEntries)
	for _, m := range metas {
		l.transformEntries = append(l.transformEntries, astjson.TransformEntry{
			InputKey:  m.inputKey,
			OutputKey: m.outputKey,
			Child:     m.child,
		})
	}
	if !hasTypenameField {
		l.transformEntries = append(l.transformEntries, astjson.TransformEntry{
			InputKey: "__typename", OutputKey: "__typename",
		})
	}

	t := &l.transforms[tIdx]
	t.Entries = l.transformEntries[entriesStart:]
	return t
}

func (l *Loader) buildDenormalizeTransform(obj *Object) *astjson.Transform {
	tIdx := l.allocTransformIndex()

	metasStart := len(l.transformMetas)
	metasEnd := metasStart + len(obj.Fields)
	l.transformMetas = l.transformMetas[:metasEnd]
	metas := l.transformMetas[metasStart:metasEnd]
	hasTypenameField := false
	for i, field := range obj.Fields {
		aliasName := unsafebytes.BytesToString(field.Name)
		cacheName := l.cacheFieldName(field)
		if cacheName == "__typename" {
			hasTypenameField = true
		}
		metas[i].inputKey = cacheName
		metas[i].outputKey = aliasName
		metas[i].child = l.buildDenormalizeChild(field.Value)
	}

	entriesStart := len(l.transformEntries)
	for _, m := range metas {
		l.transformEntries = append(l.transformEntries, astjson.TransformEntry{
			InputKey:  m.inputKey,
			OutputKey: m.outputKey,
			Child:     m.child,
		})
	}
	if !hasTypenameField {
		l.transformEntries = append(l.transformEntries, astjson.TransformEntry{
			InputKey: "__typename", OutputKey: "__typename",
		})
	}

	t := &l.transforms[tIdx]
	t.Entries = l.transformEntries[entriesStart:]
	return t
}

func (l *Loader) buildNormalizeChild(node Node) *astjson.Transform {
	switch n := node.(type) {
	case *Object:
		if n == nil || !n.HasAliases {
			return nil
		}
		return l.buildNormalizeTransform(n)
	case *Array:
		if n == nil || n.Item == nil {
			return nil
		}
		inner := l.buildNormalizeChild(n.Item)
		if inner == nil {
			return nil
		}
		tIdx := l.allocTransformIndex()
		t := &l.transforms[tIdx]
		t.ArrayItem = inner
		return t
	}
	return nil
}

func (l *Loader) buildDenormalizeChild(node Node) *astjson.Transform {
	switch n := node.(type) {
	case *Object:
		if n == nil || !n.HasAliases {
			return nil
		}
		return l.buildDenormalizeTransform(n)
	case *Array:
		if n == nil || n.Item == nil {
			return nil
		}
		inner := l.buildDenormalizeChild(n.Item)
		if inner == nil {
			return nil
		}
		tIdx := l.allocTransformIndex()
		t := &l.transforms[tIdx]
		t.ArrayItem = inner
		return t
	}
	return nil
}

// structuralCopyProjected applies a denormalize transform (schema name → alias)
// with Passthrough=false and no forced __typename, so only ProvidesData fields
// are included. Unlike structuralCopyDenormalized, this always builds a Transform
// even when !HasAliases, ensuring field projection at every level.
// Used for shadow comparison and mutation analytics where exact field projection matters.
func (l *Loader) structuralCopyProjected(v *astjson.Value, obj *Object) *astjson.Value {
	if obj == nil {
		return l.parser.StructuralCopy(l.jsonArena, v)
	}
	entries, transforms := countProjectAllocations(obj)
	l.transformEntries = l.transformEntries[:0]
	if cap(l.transformEntries) < entries {
		l.transformEntries = make([]astjson.TransformEntry, 0, entries)
	}
	l.transforms = l.transforms[:0]
	if cap(l.transforms) < transforms {
		l.transforms = make([]astjson.Transform, 0, transforms)
	}
	l.transformMetas = l.transformMetas[:0]
	if cap(l.transformMetas) < entries {
		l.transformMetas = make([]fieldMeta, 0, entries)
	}
	t := l.buildProjectTransform(obj)
	return l.parser.StructuralCopyWithTransform(l.jsonArena, v, t)
}

// buildProjectTransform builds a denormalize transform for field projection.
// Unlike buildDenormalizeTransform, it does not force __typename and always
// recurses into children regardless of HasAliases.
func (l *Loader) buildProjectTransform(obj *Object) *astjson.Transform {
	tIdx := l.allocTransformIndex()

	metasStart := len(l.transformMetas)
	metasEnd := metasStart + len(obj.Fields)
	l.transformMetas = l.transformMetas[:metasEnd]
	metas := l.transformMetas[metasStart:metasEnd]
	for i, field := range obj.Fields {
		aliasName := unsafebytes.BytesToString(field.Name)
		cacheName := l.cacheFieldName(field)
		metas[i].inputKey = cacheName
		metas[i].outputKey = aliasName
		metas[i].child = l.buildProjectChild(field.Value)
	}

	entriesStart := len(l.transformEntries)
	for _, m := range metas {
		l.transformEntries = append(l.transformEntries, astjson.TransformEntry{
			InputKey:  m.inputKey,
			OutputKey: m.outputKey,
			Child:     m.child,
		})
	}
	entriesEnd := len(l.transformEntries)

	t := &l.transforms[tIdx]
	t.Entries = l.transformEntries[entriesStart:entriesEnd]
	return t
}

func (l *Loader) buildProjectChild(node Node) *astjson.Transform {
	switch n := node.(type) {
	case *Object:
		if n == nil {
			return nil
		}
		return l.buildProjectTransform(n)
	case *Array:
		if n == nil || n.Item == nil {
			return nil
		}
		inner := l.buildProjectChild(n.Item)
		if inner == nil {
			return nil
		}
		tIdx := l.allocTransformIndex()
		t := &l.transforms[tIdx]
		t.ArrayItem = inner
		return t
	}
	return nil
}

// countProjectAllocations counts TransformEntry and Transform allocations
// for field projection. Unlike countTransformAllocations, it always recurses
// into children (no HasAliases short-circuit) and does not count forced __typename.
func countProjectAllocations(obj *Object) (entries, transforms int) {
	if obj == nil {
		return 0, 0
	}
	transforms = 1
	entries = len(obj.Fields)
	for _, field := range obj.Fields {
		ce, ct := countProjectChildAllocations(field.Value)
		entries += ce
		transforms += ct
	}
	return entries, transforms
}

func countProjectChildAllocations(node Node) (entries, transforms int) {
	switch n := node.(type) {
	case *Object:
		if n == nil {
			return 0, 0
		}
		return countProjectAllocations(n)
	case *Array:
		if n == nil || n.Item == nil {
			return 0, 0
		}
		ce, ct := countProjectChildAllocations(n.Item)
		if ct > 0 {
			ct++
		}
		return ce, ct
	}
	return 0, 0
}
