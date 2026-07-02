package cache

import (
	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// Partial fetching (task 19): a batch entity fetch with SOME buckets covered
// and some not serves the covered ones from cache and sends the subgraph a
// REDUCED representations list; the response then realigns to the original
// bucket positions via ItemCacheState.BatchIndex. Everything lives here — the
// loader only swaps in the reduced input and leaves the data merge to the
// partial arm of OnFetchResult (one hook, one lock acquisition).

// filterBatchInput builds the reduced fetch input: the original rendered
// input with the representations of COVERED buckets removed. keep[i] mirrors
// the BatchStats bucket order (which is the representations order). Returns
// ok=false when the input does not have the expected
// body.variables.representations shape — the caller then falls back to a
// plain full fetch (never an error, never a wrong request).
func filterBatchInput(input []byte, keep []bool) ([]byte, bool) {
	parsed, err := astjson.ParseBytes(input)
	if err != nil {
		return nil, false
	}
	representations := parsed.Get("body", "variables", "representations")
	if representations == nil || representations.Type() != astjson.TypeArray {
		return nil, false
	}
	all := representations.GetArray()
	if len(all) != len(keep) {
		return nil, false
	}
	reduced := astjson.ArrayValue(nil)
	kept := 0
	for i, representation := range all {
		if !keep[i] {
			continue
		}
		reduced.SetArrayItem(nil, kept, representation)
		kept++
	}
	if kept == 0 || kept == len(all) {
		// Nothing to filter (degenerate cases are handled by Fetch /
		// SkipFullHit before this point; guard anyway).
		return nil, false
	}
	parsed.Get("body", "variables").Set(nil, "representations", reduced)
	return parsed.MarshalTo(nil), true
}

// onPartialBatchResult is the FetchPartial merge arm: within ONE transaction
// it splices every covered bucket from cache (denormalized per merge target)
// and realigns the REDUCED response — fetched buckets consume the _entities
// array in order — merging and cache-writing the fresh values. On failure
// signals the covered splice still happens (the cached data is valid; the
// loader has already rendered the fetch errors) and only the fetched subset's
// merge and writes are skipped.
func (r *requestCache) onPartialBatchResult(h *resolve.FetchCacheHandle, in resolve.MergeInput, cfg *resolve.FetchCacheConfig) error {
	tx := in.Arena.Begin()
	defer tx.Commit()

	fetchFailed := in.FetchFailed || in.HasErrors || in.ResponseData == nil
	var batch []*astjson.Value
	if !fetchFailed {
		batch = in.ResponseData.GetArray()
		if batch == nil {
			fetchFailed = true
		}
	}

	prefix := r.prefixes[h]
	missedByItem := r.missedKeys[h]
	fetchedIndex := 0
	for i := range h.Items {
		item := &h.Items[i]
		var targets []*astjson.Value
		if item.BatchIndex >= 0 && item.BatchIndex < len(in.BatchStats) {
			targets = in.BatchStats[item.BatchIndex]
		}
		if item.FromCache != nil {
			// Covered bucket: identical duties to OnFetchSkipped — splice
			// (nothing for a negative sentinel) plus the best-effort
			// refresh/backfill write-backs.
			var missed []string
			if i < len(missedByItem) {
				missed = missedByItem[i]
			}
			if err := r.spliceCachedItem(tx, cfg, prefix, cfg.ProvidesData, item, missed, targets, in.MergePath); err != nil {
				return err
			}
			continue
		}
		// Fetched bucket: consume the reduced response in order.
		if fetchFailed {
			continue
		}
		if fetchedIndex >= len(batch) {
			continue
		}
		src := batch[fetchedIndex]
		fetchedIndex++
		if src == nil || src.Type() == astjson.TypeNull {
			// A legitimately missing entity: nothing to merge, nothing to
			// write (negative caching needs the loader's EmptyEntity signal,
			// which is whole-response scoped — out of the partial path).
			continue
		}
		for _, target := range targets {
			if target == nil {
				continue
			}
			if len(in.MergePath) > 0 {
				if _, err := tx.MergeValuesWithPath(target, tx.StructuralCopy(src), in.MergePath...); err != nil {
					return err
				}
			} else if _, err := tx.MergeValues(target, tx.StructuralCopy(src)); err != nil {
				return err
			}
		}
		r.writeFetchedValue(tx, cfg, h, item, src, cfg.ProvidesData)
	}
	return nil
}
