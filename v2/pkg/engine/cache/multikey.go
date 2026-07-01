package cache

import (
	"cmp"
	"time"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// selectMultiCandidateCacheValue is the multi-candidate selection ladder over
// freshest-first candidates (parsed[i] pairs state.FromCacheCandidates[i]):
//  1. the freshest value covers on its own → serve it;
//  2. no single value covers, but the union does → serve the merge, freshest
//     winning on conflicts, and mark NeedsWriteback so the canonical entry is
//     rewritten with the synthesized value;
//  3. an older single value covers → serve it and mark NeedsWriteback (the
//     freshest entry is stale in shape and owes a refresh);
//  4. nothing covers → miss.
//
// It fills FromCache and SelectedRemainingTTL and reports whether a value was
// selected.
func selectMultiCandidateCacheValue(tx *resolve.CacheTransaction, state *resolve.ItemCacheState, parsed []*astjson.Value, providesData *resolve.Object) bool {
	if len(parsed) == 0 || providesData == nil {
		return false
	}
	if parsed[0] != nil && covers(parsed[0], providesData) {
		state.FromCache = parsed[0]
		state.SelectedRemainingTTL = state.FromCacheCandidates[0].RemainingTTL
		return true
	}
	if len(parsed) <= 1 {
		return false
	}

	// Merge synthesis: build the union OLDEST first so the freshest value wins
	// every conflicting field.
	var merged *astjson.Value
	for i := len(parsed) - 1; i >= 0; i-- {
		if parsed[i] == nil {
			continue
		}
		current := tx.StructuralCopy(parsed[i])
		if merged == nil {
			merged = current
			continue
		}
		if _, err := tx.MergeValues(merged, current); err != nil {
			// A merge failure (e.g. type conflicts) voids the synthesis; fall
			// through to the older-single ladder rung instead of serving a
			// half-merged value.
			merged = nil
			break
		}
	}
	if merged != nil && covers(merged, providesData) {
		state.FromCache = merged
		state.SelectedRemainingTTL = state.FromCacheCandidates[0].RemainingTTL
		state.NeedsWriteback = true
		return true
	}

	for i := 1; i < len(parsed); i++ {
		if parsed[i] == nil {
			continue
		}
		if covers(parsed[i], providesData) {
			state.FromCache = parsed[i]
			state.SelectedRemainingTTL = state.FromCacheCandidates[i].RemainingTTL
			state.NeedsWriteback = true
			return true
		}
	}
	return false
}

// compareCacheCandidateFreshness orders candidates freshest first: a known
// remaining TTL always beats an unknown (non-positive) one, larger beats
// smaller.
func compareCacheCandidateFreshness(a, b time.Duration) int {
	aKnown := a > 0
	bKnown := b > 0
	switch {
	case aKnown && bKnown:
		return cmp.Compare(b, a)
	case aKnown:
		return -1
	case bKnown:
		return 1
	default:
		return 0
	}
}

// reorderToSelectionOrder rebuilds the value with its fields in the
// ProvidesData selection order (recursively), appending cached-only extras
// after the selected fields, so a spliced value renders byte-identically to a
// fetched one.
func reorderToSelectionOrder(tx *resolve.CacheTransaction, value *astjson.Value, node resolve.Node) *astjson.Value {
	if value == nil || node == nil {
		return value
	}
	switch typed := node.(type) {
	case *resolve.Object:
		if value.Type() != astjson.TypeObject {
			return value
		}
		reordered := tx.NewObject()
		seen := make(map[string]struct{}, len(typed.Fields))
		for _, field := range typed.Fields {
			fieldName := string(field.Name)
			fieldValue := value.Get(fieldName)
			if fieldValue == nil {
				continue
			}
			reordered.Set(nil, fieldName, reorderToSelectionOrder(tx, fieldValue, field.Value))
			seen[fieldName] = struct{}{}
		}
		obj, err := value.Object()
		if err != nil {
			return value
		}
		// Keep fields the cache carries beyond the selection (e.g. key fields
		// another fetch needed) so a write-back never loses data.
		obj.Visit(func(key []byte, fieldValue *astjson.Value) {
			fieldName := string(key)
			if _, ok := seen[fieldName]; ok {
				return
			}
			reordered.Set(nil, fieldName, fieldValue)
		})
		return reordered
	case *resolve.Array:
		if value.Type() != astjson.TypeArray {
			return value
		}
		items, err := value.Array()
		if err != nil {
			return value
		}
		reordered := tx.NewArray()
		for i, item := range items {
			reordered.SetArrayItem(nil, i, reorderToSelectionOrder(tx, item, typed.Item))
		}
		return reordered
	default:
		return value
	}
}
