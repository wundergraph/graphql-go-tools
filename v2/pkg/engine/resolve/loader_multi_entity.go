package resolve

import (
	"bytes"
	goerrors "errors"

	"github.com/pkg/errors"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

// preparedMultiEntry is the per-entry view carried from the prepare phase into
// the merge phase of a MultiEntityFetch.
type preparedMultiEntry struct {
	entry *MultiEntityFetchEntry
	items []*astjson.Value // merge targets from selectItemsForPath (jsonArena-backed)
	res   *result          // per-entry view; init(entry.PostProcessing, entry.Info)
}

// prepareMultiEntityFetch renders one merged upstream request out of several
// entity fetches. Each entry authorizes and renders its own representations;
// excluded entries emit an empty representations array and includeFN:false so
// variable coercion still passes on the subgraph. Rate limiting runs once for
// the merged request.
func (l *Loader) prepareMultiEntityFetch(fetchItem *FetchItem, fetch *MultiEntityFetch, res *result, prepared *preparedFetch) error {
	res.init(PostProcessingConfiguration{SelectResponseErrorsPath: []string{"errors"}}, fetch.Info)
	if l.ctx.TracingOptions.Enable {
		fetch.Trace = &DataSourceLoadTrace{}
	}
	res.tools = batchEntityToolPool.Get(len(fetch.Input.Entries))

	entries := make([]preparedMultiEntry, len(fetch.Input.Entries))
	included := make([]bool, len(fetch.Input.Entries))
	repsBytes := make([][]byte, len(fetch.Input.Entries))
	anyIncluded := false

	itemInput := arena.NewArenaBuffer(res.tools.a)

	for k := range fetch.Input.Entries {
		entry := &fetch.Input.Entries[k]
		entryRes := &result{}
		entryRes.init(entry.PostProcessing, entry.Info)
		items := l.selectItemsForPath(entry.Item.FetchPath)
		entries[k] = preparedMultiEntry{entry: entry, items: items, res: entryRes}

		// Authorization first: for query-typed entries the authorizer path is
		// unreachable, so this only exercises the pre-fetch cache. A denied entry
		// is excluded like a skipped one, and its representations are never sent.
		allowed, err := l.isFetchAuthorized(nil, entry.Info, entryRes)
		if err != nil {
			return err
		}
		if !allowed {
			continue
		}

		repsBuf := arena.NewArenaBuffer(res.tools.a)
		batchStats := arena.AllocateSlice[[]*astjson.Value](res.tools.a, 0, len(items))
		batchItemIndex := 0
		addSeparator := false
		for i, item := range items {
			itemInput.Reset()
			err = entry.Representations.Render(l.ctx, item, itemInput)
			if err != nil {
				if entry.SkipErrItems {
					err = nil // nolint:ineffassign
					continue
				}
				return errors.WithStack(err)
			}
			if entry.SkipNullItems && itemInput.Len() == 4 && bytes.Equal(itemInput.Bytes(), null) {
				continue
			}
			if entry.SkipEmptyObjectItems && itemInput.Len() == 2 && bytes.Equal(itemInput.Bytes(), emptyObject) {
				continue
			}
			res.tools.keyGen.Reset()
			_, _ = res.tools.keyGen.Write(itemInput.Bytes())
			itemHash := res.tools.keyGen.Sum64()
			if existingIndex, ok := res.tools.batchHashToIndex[itemHash]; ok {
				batchStats[existingIndex] = arena.SliceAppend(res.tools.a, batchStats[existingIndex], items[i])
				continue
			}
			if addSeparator {
				_ = repsBuf.WriteByte(',')
			}
			_, _ = itemInput.WriteTo(repsBuf)
			res.tools.batchHashToIndex[itemHash] = batchItemIndex
			// The targets bucket must live on the arena: a heap bucket referenced
			// only from arena memory could be collected while still in use.
			bucket := arena.AllocateSlice[*astjson.Value](res.tools.a, 1, 1)
			bucket[0] = items[i]
			batchStats = arena.SliceAppend(res.tools.a, batchStats, bucket)
			batchItemIndex++
			addSeparator = true
		}

		// Copy the entry's batchStats to the heap before the next entry reuses
		// the dedup scope; the arena buffers themselves survive until assembly.
		entryRes.batchStats = make([][]*astjson.Value, len(batchStats))
		for i := range batchStats {
			entryRes.batchStats[i] = make([]*astjson.Value, len(batchStats[i]))
			copy(entryRes.batchStats[i], batchStats[i])
			batchStats[i] = nil
		}
		res.tools.clearDedupState()

		if len(entryRes.batchStats) == 0 {
			entryRes.fetchSkipped = true
			continue
		}
		included[k] = true
		repsBytes[k] = repsBuf.Bytes()
		anyIncluded = true
	}

	prepared.multiEntries = entries

	buf := &bytes.Buffer{}
	var undefined []string
	if err := fetch.Input.Header.RenderAndCollectUndefinedVariables(l.ctx, nil, buf, &undefined); err != nil {
		return errors.WithStack(err)
	}
	scratch := arena.NewArenaBuffer(res.tools.a)
	for k := range fetch.Input.Entries {
		entry := &fetch.Input.Entries[k]
		buf.Write(entry.RepresentationsPrefix)
		if included[k] {
			buf.Write(repsBytes[k])
		}
		buf.Write(entry.IncludePrefix)
		if included[k] {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
		for v := range entry.Variables {
			variable := &entry.Variables[v]
			scratch.Reset()
			var varUndefined []string
			if err := variable.Value.RenderAndCollectUndefinedVariables(l.ctx, nil, scratch, &varUndefined); err != nil {
				return errors.WithStack(err)
			}
			// Omit a pair whose value is null only because an undefined context
			// variable was collected; an explicit client null (empty slice) stays.
			if len(varUndefined) > 0 && bytes.Equal(scratch.Bytes(), null) {
				continue
			}
			buf.Write(variable.KeyPrefix)
			buf.Write(scratch.Bytes())
		}
	}
	if err := fetch.Input.Footer.RenderAndCollectUndefinedVariables(l.ctx, nil, buf, &undefined); err != nil {
		return errors.WithStack(err)
	}

	if !anyIncluded {
		res.fetchSkipped = true
		prepared.skipLoad = true
		if l.ctx.TracingOptions.Enable {
			l.setTracingInput(fetchItem, buf.Bytes(), fetch.Trace)
		}
		return nil
	}

	allowed, err := l.rateLimitFetch(buf.Bytes(), fetch.Info, res)
	if err != nil {
		return err
	}
	if !allowed {
		prepared.skipLoad = true
	}

	prepared.source = fetch.DataSource
	prepared.input = buf.Bytes()
	prepared.trace = fetch.Trace
	if l.ctx.TracingOptions.Enable && !l.ctx.TracingOptions.ExcludeRawInputData {
		var rawData bytes.Buffer
		rawData.WriteByte('{')
		for k := range entries {
			if k > 0 {
				rawData.WriteByte(',')
			}
			rawData.WriteByte('"')
			rawData.WriteString(entries[k].entry.Alias)
			rawData.WriteString(`":`)
			data := l.itemsData(entries[k].items)
			if data == nil {
				rawData.Write(null)
			} else {
				rawData.Write(data.MarshalTo(nil))
			}
		}
		rawData.WriteByte('}')
		fetch.Trace.RawInputData, _ = l.compactJSON(rawData.Bytes())
	}
	return nil
}

// multiEntryMergeConfig is set on a per-entry result view so the shared
// mergeResult machinery demuxes one merged response into each entry: it selects
// the entry's aliased data, uses pre-partitioned errors, and rewrites alias
// prefixes in error paths.
type multiEntryMergeConfig struct {
	alias        string
	originSingle bool
	info         *FetchInfo     // taint-info source
	response     *astjson.Value // pre-parsed shared response
	errors       *astjson.Value // pre-partitioned errors array for this entry (nil = none)
}

// mergeMultiEntityResult demuxes one merged subgraph response into its entries.
// It runs under the data lock, like mergeResult. Transport-level failures fan
// out per non-excluded entry so each renders today's unmerged guards; on the
// parsed path each entry merges its aliased slice. Extensions are collected and
// OnFinished fires exactly once for the single request.
func (l *Loader) mergeMultiEntityResult(prepared *preparedFetch) error {
	res := prepared.res

	// All entries excluded at prepare: no request was sent, nothing to merge.
	if res.fetchSkipped && !res.rateLimitRejected {
		return nil
	}

	transportFailure := res.err != nil || res.authorizationRejected || res.rateLimitRejected || len(res.out) == 0
	var response *astjson.Value
	if !transportFailure {
		var parseErr error
		response, parseErr = astjson.ParseBytesWithArena(l.jsonArena, res.out)
		if parseErr != nil {
			// Invalid body: fan out so each entry re-parses and renders today's
			// guards. loadPhase recorded no errored fetch ID, so dependents still run.
			transportFailure = true
			response = nil
		}
	}

	if transportFailure {
		for i := range prepared.multiEntries {
			entryRes := prepared.multiEntries[i].res
			if entryRes.fetchSkipped {
				// Excluded at prepare: never sent, so it gets no transport error.
				continue
			}
			entryRes.err = res.err
			entryRes.statusCode = res.statusCode
			entryRes.ds = res.ds
			entryRes.out = res.out
			entryRes.rateLimitRejected = res.rateLimitRejected
			entryRes.rateLimitRejectedReason = res.rateLimitRejectedReason
			entryRes.authorizationRejected = res.authorizationRejected
			entryRes.authorizationRejectedReasons = res.authorizationRejectedReasons
		}
	} else {
		if l.allowCustomExtensionProperties {
			extensions := response.Get("extensions")
			if astjson.ValueIsNonNull(extensions) && extensions.Type() == astjson.TypeObject {
				l.subgraphExtensions = append(l.subgraphExtensions, extensions.GetObject())
			}
		}
		entryErrors, err := l.partitionMultiEntityErrors(prepared, response)
		if err != nil {
			return err
		}
		for i := range prepared.multiEntries {
			entry := prepared.multiEntries[i].entry
			entryRes := prepared.multiEntries[i].res
			entryRes.multi = &multiEntryMergeConfig{
				alias:        entry.Alias,
				originSingle: entry.OriginKind == EntityFetchOriginSingle,
				info:         entry.Info,
				response:     response,
				errors:       entryErrors[i],
			}
			entryRes.statusCode = res.statusCode
			entryRes.ds = res.ds
			entryRes.out = res.out
			entryRes.httpResponseContext = res.httpResponseContext
		}
	}

	var firstErr error
	for i := range prepared.multiEntries {
		entry := prepared.multiEntries[i]
		if err := l.mergeResult(entry.entry.Item, entry.res, entry.items); err != nil && firstErr == nil {
			firstErr = err
		}
		res.subgraphError = goerrors.Join(res.subgraphError, entry.res.subgraphError)
	}
	l.callOnFinished(res)
	return firstErr
}

// partitionMultiEntityErrors splits the shared response's top-level errors by
// their leading path element: errors keyed by an entry alias are returned
// aligned with prepared.multiEntries; the rest are merged once against the
// parent multi fetch (empty response path).
func (l *Loader) partitionMultiEntityErrors(prepared *preparedFetch, response *astjson.Value) ([]*astjson.Value, error) {
	entryErrors := make([]*astjson.Value, len(prepared.multiEntries))
	responseErrors := response.Get("errors")
	if !astjson.ValueIsNonNull(responseErrors) || responseErrors.Type() != astjson.TypeArray {
		return entryErrors, nil
	}
	aliasIndex := make(map[string]int, len(prepared.multiEntries))
	for i := range prepared.multiEntries {
		aliasIndex[prepared.multiEntries[i].entry.Alias] = i
	}
	var unmatched *astjson.Value
	for _, errValue := range responseErrors.GetArray() {
		idx := -1
		if path := errValue.Get("path"); astjson.ValueIsNonNull(path) && path.Type() == astjson.TypeArray {
			if items := path.GetArray(); len(items) > 0 && items[0].Type() == astjson.TypeString {
				if i, ok := aliasIndex[string(items[0].GetStringBytes())]; ok {
					idx = i
				}
			}
		}
		if idx == -1 {
			if unmatched == nil {
				unmatched = astjson.ArrayValue(l.jsonArena)
			}
			astjson.AppendToArray(l.jsonArena, unmatched, errValue)
			continue
		}
		if entryErrors[idx] == nil {
			entryErrors[idx] = astjson.ArrayValue(l.jsonArena)
		}
		astjson.AppendToArray(l.jsonArena, entryErrors[idx], errValue)
	}
	if unmatched != nil && len(unmatched.GetArray()) > 0 {
		if err := l.mergeErrors(prepared.res, prepared.item, unmatched); err != nil {
			return entryErrors, err
		}
	}
	return entryErrors, nil
}
