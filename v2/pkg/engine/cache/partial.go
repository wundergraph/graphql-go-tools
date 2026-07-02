package cache

import (
	"errors"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// errPartialBatchEntityCountMismatch surfaces a subgraph contract violation:
// the reduced _entities response must contain exactly one element per sent
// representation (nulls included), like the loader's own full-batch
// invalidBatchItemCount check.
var errPartialBatchEntityCountMismatch = errors.New("partial batch _entities count does not match the reduced representations")

// Partial fetching (task 19): a batch entity fetch with SOME buckets covered
// and some not serves the covered ones from cache and sends the subgraph a
// REDUCED representations list; the response then realigns to the original
// bucket positions via ItemCacheState.BatchIndex. The controller marks the
// missing buckets on handle.BatchFetchKeep and the LOADER assembles the
// reduced input from the already-rendered representation segments (before any
// bytes are parsed back); the data merge lives in the partial arm of
// OnFetchResult (one hook, one lock acquisition).

// onPartialBatchResult is the FetchPartial merge arm: within ONE transaction
// it splices every covered bucket from cache (denormalized per merge target)
// and realigns the REDUCED response — fetched buckets consume the _entities
// array in order — merging and cache-writing the fresh values. On failure
// signals the covered splice still happens (the cached data is valid; the
// loader has already rendered the fetch errors) and only the fetched subset's
// merge and writes are skipped.
func (r *requestCache) onPartialBatchResult(h *resolve.FetchCacheHandle, in resolve.MergeInput, state *handleState) error {
	cfg := state.cfg
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

	prefix := state.prefix
	missedByItem := state.missedKeys
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
			return errPartialBatchEntityCountMismatch
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
	if !fetchFailed && fetchedIndex != len(batch) {
		return errPartialBatchEntityCountMismatch
	}
	return nil
}
