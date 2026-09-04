package resolve

import (
	"bytes"
	"context"
	goerrors "errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/pkg/errors"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/caching"
)

// preparedMultiEntry is the per-entry view carried from the prepare phase into
// the merge phase of a MultiEntityFetch.
type preparedMultiEntry struct {
	entry *MultiEntityFetchEntry
	items []*astjson.Value // merge targets from selectItemsForPath (jsonArena-backed)
	res   *result          // per-entry view; init(entry.PostProcessing, entry.Info)

	// representationItemHashes holds one hash per unique representation this
	// entry rendered. Only live during prepare, where responseCacheKeys are
	// derived from it.
	representationItemHashes []uint64
	// responseCacheKeys holds one key per unique representation this entry
	// rendered, in render order: the same order as res.batchStats and as the
	// entry's slice of the response.
	responseCacheKeys []string
	// cachedValues is set only when every one of responseCacheKeys hit: the
	// stored entity objects, written back into the response at merge time.
	cachedValues [][]byte
	// responseCacheTTL is the life left on the least fresh of cachedValues.
	responseCacheTTL time.Duration
}

// cacheHit reports whether this entry was answered entirely from the response
// cache, in which case it was switched off in the merged request.
func (e *preparedMultiEntry) cacheHit() bool { return len(e.cachedValues) > 0 }

// multiAssembly is what the load phase needs to rebuild the merged request
// after the response-cache lookup switches warm entries off.
type multiAssembly struct {
	fetch               *MultiEntityFetch
	included            []bool
	representationBytes [][]byte
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
	// included entries send representations and render includeFN:true
	included := make([]bool, len(fetch.Input.Entries))
	// rendered representation bytes.
	repsBytes := make([][]byte, len(fetch.Input.Entries))
	// still false at the end means no request goes out
	anyIncluded := false

	// One item-render buffer serves every entry: it is reset per item and the
	// arena behind it (res.tools.a) survives until assembly.
	itemInput := arena.NewArenaBuffer(res.tools.a)

	for k := range fetch.Input.Entries {
		entry := &fetch.Input.Entries[k]
		entryRes := &result{}
		entryRes.init(entry.PostProcessing, entry.Info)
		items := l.selectEntryTargets(entry)
		entries[k] = preparedMultiEntry{entry: entry, items: items, res: entryRes}

		allowed, err := l.authorizeEntry(entry, entryRes)
		if err != nil {
			return err
		}
		if !allowed {
			continue
		}

		result, err := l.renderEntryRepresentations(entry, entryRes, items, itemInput, res.tools)
		if err != nil {
			return err
		}

		if !result.entryIncluded {
			continue
		}
		included[k] = true
		repsBytes[k] = result.representationBuffer
		entries[k].representationItemHashes = result.representationItemHashes
		anyIncluded = true
	}

	prepared.multiEntries = entries

	assembled, err := l.assembleMultiEntity(&assembleMultiEntityOptions{
		fetch:               fetch,
		result:              res,
		included:            included,
		representationBytes: repsBytes,
	})
	if err != nil {
		return err
	}

	if !anyIncluded {
		res.fetchSkipped = true
		prepared.skipLoad = true
		if l.ctx.TracingOptions.Enable {
			l.setTracingInput(fetchItem, assembled.inputBuffer, fetch.Trace)
		}
		return nil
	}

	// Derived here, under the data lock, because the keys are what the lookup in
	// the load phase asks for. The lookup itself stays out of the lock.
	l.setResponseCacheKeys(entries, assembled)

	allowed, err := l.rateLimitFetch(assembled.inputBuffer, fetch.Info, res)
	if err != nil {
		return err
	}
	if !allowed {
		prepared.skipLoad = true
	}

	prepared.source = fetch.DataSource
	prepared.input = assembled.inputBuffer
	prepared.multiAssembly = &multiAssembly{
		fetch:               fetch,
		included:            included,
		representationBytes: repsBytes,
	}
	prepared.trace = fetch.Trace
	if l.ctx.TracingOptions.Enable && !l.ctx.TracingOptions.ExcludeRawInputData {
		l.setMultiFetchRawInputTrace(entries, fetch.Trace)
	}
	return nil
}

// selectEntryTargets picks this entry's merge targets via its own FetchPath
// (type-condition- and taint-aware); the targets are jsonArena-backed.
func (l *Loader) selectEntryTargets(entry *MultiEntityFetchEntry) []*astjson.Value {
	return l.selectItemsForPath(entry.Item.FetchPath)
}

// authorizeEntry runs authorization first, before any input is rendered: for
// query-typed entries the authorizer path is unreachable, so this only
// exercises the pre-fetch cache. A denied entry is excluded like a skipped one,
// and its representations are never sent.
func (l *Loader) authorizeEntry(entry *MultiEntityFetchEntry, entryRes *result) (bool, error) {
	return l.isFetchAuthorized(nil, entry.Info, entryRes)
}

// renderEntryRepresentationsResult is the result of rendering a single
// multi fetch entry's representations.
type renderEntryRepresentationsResult struct {
	// representationBuffer is the buffer containing the rendered representations.
	representationBuffer []byte
	// representationItemHashes is the list of hashes of the unique representations.
	representationItemHashes []uint64
	// entryIncluded is true if the entry is included in the merged request.
	// Each entry in the request carries an `@include(if:)` directive which is
	// controlled by the corresponding `includeFN` variable.
	entryIncluded bool
}

// renderEntryRepresentations renders this entry's representations exactly like a
// batch entity fetch: per item apply skip flags, xxhash-dedup, and comma
// separators. The per-entry batchStats are copied to the heap and the dedup
// state is cleared before the next entry reuses the shared tools; the arena
// buffers themselves survive until assembly. An entry with zero unique items is
// excluded (fetchSkipped) and reports entryIncluded=false.
func (l *Loader) renderEntryRepresentations(
	entry *MultiEntityFetchEntry,
	entryRes *result,
	items []*astjson.Value,
	itemInput *arena.Buffer,
	tools *batchEntityTools,
) (*renderEntryRepresentationsResult, error) {
	repsBuf := arena.NewArenaBuffer(tools.a)
	batchStats := arena.AllocateSlice[[]*astjson.Value](tools.a, 0, len(items))
	batchItemIndex := 0
	addSeparator := false

	var responseCacheItemHashes []uint64

	for i, item := range items {
		itemInput.Reset()
		err := entry.Representations.Render(l.ctx, item, itemInput)
		if err != nil {
			if entry.SkipErrItems {
				continue
			}
			return nil, errors.WithStack(err)
		}
		if entry.SkipNullItems && itemInput.Len() == 4 && bytes.Equal(itemInput.Bytes(), null) {
			continue
		}
		if entry.SkipEmptyObjectItems && itemInput.Len() == 2 && bytes.Equal(itemInput.Bytes(), emptyObject) {
			continue
		}
		tools.keyGen.Reset()
		_, _ = tools.keyGen.Write(itemInput.Bytes())
		itemHash := tools.keyGen.Sum64()
		if existingIndex, ok := tools.batchHashToIndex[itemHash]; ok {
			batchStats[existingIndex] = arena.SliceAppend(tools.a, batchStats[existingIndex], items[i])
			continue
		}
		if addSeparator {
			_ = repsBuf.WriteByte(',')
		}
		_, _ = itemInput.WriteTo(repsBuf)
		tools.batchHashToIndex[itemHash] = batchItemIndex
		if l.responseCacheEnabled() {
			responseCacheItemHashes = append(responseCacheItemHashes, itemHash)
		}

		// The targets bucket must live on the arena: a heap bucket referenced
		// only from arena memory could be collected while still in use.
		bucket := arena.AllocateSlice[*astjson.Value](tools.a, 1, 1)
		bucket[0] = items[i]
		batchStats = arena.SliceAppend(tools.a, batchStats, bucket)
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
	tools.clearDedupState()

	if len(entryRes.batchStats) == 0 {
		entryRes.fetchSkipped = true
		return &renderEntryRepresentationsResult{
			entryIncluded: false,
		}, nil
	}
	return &renderEntryRepresentationsResult{
		representationBuffer:     repsBuf.Bytes(),
		representationItemHashes: responseCacheItemHashes,
		entryIncluded:            true,
	}, nil
}

type assembleMultiEntityOptions struct {
	fetch               *MultiEntityFetch
	result              *result
	included            []bool
	representationBytes [][]byte
}

// assembleMultiEntityResult carries the assembled request body.
//
// In addition it carries the header, footer and entryVariables slices of out.
// This is so that the response cache keys can be derived from the assembled request body.
type assembleMultiEntityResult struct {
	inputBuffer []byte

	header         []byte
	footer         []byte
	entryVariables [][]byte
}

// assembleMultiEntity writes the merged request body into one buffer:
// Header, then per entry "representations_fN":[<items or empty>],"includeFN":<bool>
// plus that entry's other variables, then Footer. The result carries the buffer
// along with the parts of it a per-entry cache key is hashed from.
func (l *Loader) assembleMultiEntity(opts *assembleMultiEntityOptions) (*assembleMultiEntityResult, error) {
	if opts == nil {
		return nil, errors.New("options are nil")
	}

	fetch := opts.fetch

	buf := &bytes.Buffer{}
	var undefined []string
	if err := fetch.Input.Header.RenderAndCollectUndefinedVariables(l.ctx, nil, buf, &undefined); err != nil {
		return nil, errors.WithStack(err)
	}

	headerEnd := buf.Len()

	// Additional variables besides the representations and include flags, the
	// trailing pairs below. These are included in the response cache keys.
	//
	//   "representations_f1":[{...}],"includeF1":true,"foo_f1":"bar"
	//                                                ^^^^^^^^^^^^^^^
	variableBounds := make([][2]int, len(fetch.Input.Entries))

	scratch := arena.NewArenaBuffer(opts.result.tools.a)
	for k := range fetch.Input.Entries {
		entry := &fetch.Input.Entries[k]
		buf.Write(entry.RepresentationsPrefix)
		if opts.included[k] {
			buf.Write(opts.representationBytes[k])
		}
		buf.Write(entry.IncludePrefix)
		if opts.included[k] {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
		variableStart := buf.Len()
		if err := l.renderEntryVariables(entry, buf, scratch); err != nil {
			return nil, err
		}
		variableBounds[k] = [2]int{variableStart, buf.Len()}
	}

	footerStart := buf.Len()

	if err := fetch.Input.Footer.RenderAndCollectUndefinedVariables(l.ctx, nil, buf, &undefined); err != nil {
		return nil, errors.WithStack(err)
	}

	rendered := buf.Bytes()
	entryVariables := make([][]byte, len(variableBounds))
	for i, bounds := range variableBounds {
		entryVariables[i] = rendered[bounds[0]:bounds[1]]
	}

	return &assembleMultiEntityResult{
		inputBuffer:    rendered,
		header:         rendered[:headerEnd],
		footer:         rendered[footerStart:],
		entryVariables: entryVariables,
	}, nil
}

// setResponseCacheKeys derives one key per unique representation of every entry
// that renders representations. Each entry hashes its own slice of the request,
// so warm entries can be recognised and switched off one at a time.
func (l *Loader) setResponseCacheKeys(mulitEntries []preparedMultiEntry, assembled *assembleMultiEntityResult) {
	if !l.responseCacheEnabled() {
		return
	}

	for i := range mulitEntries {
		multiEntry := &mulitEntries[i]
		if len(multiEntry.representationItemHashes) == 0 {
			continue
		}
		selectionHash := responseCacheEntrySelectionHash(
			assembled.header,
			multiEntry.entry.Alias,
			assembled.entryVariables[i],
			assembled.footer,
		)
		keys := make([]string, len(multiEntry.representationItemHashes))
		for j, itemHash := range multiEntry.representationItemHashes {
			keys[j] = caching.Key(itemHash, selectionHash)
		}
		multiEntry.responseCacheKeys = keys
	}
}

// applyMultiEntityResponseCache resolves the response cache for a merged fetch:
// it looks every included entry up, switches the warm ones off and rewrites the
// request body around what is left. Reports whether the fetch is served whole.
//
// It runs in the load phase, not in prepare, because a cache round trip under
// the data lock would hold up every fetch queued behind this one.
func (l *Loader) applyMultiEntityResponseCache(ctx context.Context, prepared *preparedFetch) (bool, error) {
	assembly := prepared.multiAssembly
	if assembly == nil || !l.responseCacheEnabled() {
		return false, nil
	}

	if !l.multiEntityCacheLookup(prepared, assembly.included) {
		return false, nil
	}

	// An entry the cache answered is not asked for again: dropping it from
	// included is what renders its includeFN:false and empty representations.
	for i := range prepared.multiEntries {
		if prepared.multiEntries[i].cacheHit() {
			assembly.included[i] = false
		}
	}

	if !slices.Contains(assembly.included, true) {
		// Every entry hit, so no request goes out and the body prepare assembled
		// is never sent. The hooks still run, as they do for a cached single or
		// batch fetch, and res.out stays empty: the merge treats that as "no
		// response" and each entry takes its cached entities.
		if l.ctx.LoaderHooks != nil {
			prepared.res.loaderHookContext = l.ctx.LoaderHooks.OnLoad(ctx, prepared.res.ds)
		}
		prepared.res.statusCode = http.StatusOK
		prepared.res.responseCacheHit = true
		prepared.res.responseCacheTTL = shortestCachedEntryTTL(prepared.multiEntries)
		prepared.responseCacheHit = true
		if prepared.trace != nil {
			prepared.trace.LoadSkipped = true
		}
		return true, nil
	}

	// Some entries are still cold, so a request goes out after all: rebuild the
	// body with the warm ones switched off, so the origin is asked for no more
	// than what is missing.
	assembled, err := l.assembleMultiEntity(&assembleMultiEntityOptions{
		fetch:               assembly.fetch,
		result:              prepared.res,
		included:            assembly.included,
		representationBytes: assembly.representationBytes,
	})
	if err != nil {
		return false, err
	}
	prepared.input = assembled.inputBuffer

	return false, nil
}

// renderEntryVariables appends the entry's non-representations variable pairs to
// buf, rendering each value through the shared scratch buffer.
func (l *Loader) renderEntryVariables(entry *MultiEntityFetchEntry, buf *bytes.Buffer, scratch *arena.Buffer) error {
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
	return nil
}

// setMultiFetchRawInputTrace records the per-alias parent data that fed the
// merged request into the fetch trace.
func (l *Loader) setMultiFetchRawInputTrace(entries []preparedMultiEntry, trace *DataSourceLoadTrace) {
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
	trace.RawInputData, _ = l.compactJSON(rawData.Bytes())
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
// It runs under the data lock, like mergeResult. A request that failed fans its
// state out per sent entry so each renders today's unmerged guards; on the
// parsed path each entry merges its aliased slice. Extensions are collected and
// OnFinished fires exactly once for the single request.
func (l *Loader) mergeMultiEntityResult(prepared *preparedFetch) error {
	res := prepared.res

	// All entries excluded at prepare: no request was sent, nothing to merge.
	if res.fetchSkipped && !res.rateLimitRejected {
		return nil
	}

	response, ok := l.parseMultiEntityResponse(res)
	if !ok {
		// Nothing to demux. Entries that were sent take the request's failure;
		// entries the cache answered are unaffected and merge off a document
		// built from what it held, which is also the path an entirely warm
		// fetch takes, no request having been sent for it.
		l.applyTransportStateToEntries(prepared)

		if err := l.serveCachedEntriesWithoutResponse(prepared); err != nil {
			return err
		}
	} else {
		if err := l.applyParsedResponseToEntries(prepared, response); err != nil {
			return err
		}
	}

	return l.mergeEntryResults(prepared)
}

// parseMultiEntityResponse returns the merged response, parsed once for all
// entries. It reports false when there is nothing to demux: the request failed
// (error, auth/rate-limit rejection, unparseable body), or none was sent at all
// because every entry was served from the response cache.
func (l *Loader) parseMultiEntityResponse(res *result) (*astjson.Value, bool) {
	if res.err != nil || res.authorizationRejected || res.rateLimitRejected || len(res.out) == 0 {
		return nil, false
	}
	response, parseErr := astjson.ParseBytesWithArena(l.jsonArena, res.out)
	if parseErr != nil {
		// Invalid body: fan out so each entry re-parses and renders today's
		// guards. loadPhase recorded no errored fetch ID, so dependents still run.
		return nil, false
	}
	return response, true
}

// applyTransportStateToEntries copies the merged request's transport state onto
// every entry except those excluded at prepare time (their unmerged
// counterparts were never sent), so each entry's ordinary mergeResult guards
// render the failed-to-fetch / rate-limit errors at that entry's response path.
func (l *Loader) applyTransportStateToEntries(prepared *preparedFetch) {
	res := prepared.res
	for i := range prepared.multiEntries {
		entryRes := prepared.multiEntries[i].res
		if entryRes.fetchSkipped {
			// Excluded at prepare: never sent, so it gets no transport error.
			continue
		}
		if prepared.multiEntries[i].cacheHit() {
			// Answered from the cache and switched off in the request, so the
			// request failing says nothing about it.
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
}

// applyParsedResponseToEntries handles the success path: it collects the shared
// response extensions once, partitions the errors by entry, and attaches a
// per-entry multiEntryMergeConfig so each entry's mergeResult reads its aliased
// slice, pre-partitioned errors, and taint info.
func (l *Loader) applyParsedResponseToEntries(prepared *preparedFetch, response *astjson.Value) error {
	res := prepared.res
	if l.allowCustomExtensionProperties {
		extensions := response.Get("extensions")
		if astjson.ValueIsNonNull(extensions) && extensions.Type() == astjson.TypeObject {
			l.subgraphExtensions = append(l.subgraphExtensions, extensions.GetObject())
		}
	}
	entryErrors, unmatchedErrors, err := l.partitionResponseErrors(prepared, response)
	if err != nil {
		return err
	}

	// If we have any errors that couldn't be matched to an alias, we can't cache the response.
	if !unmatchedErrors {
		// Collected off the response as the subgraph sent it, before the cached
		// entities are written in.
		l.responseCacheCollectMultiEntity(prepared, response, entryErrors)
	}

	if err := l.applyCachedEntriesToResponse(prepared, response); err != nil {
		return err
	}

	for i := range prepared.multiEntries {
		l.setEntryMergeConfig(prepared, i, response, entryErrors[i])
		prepared.multiEntries[i].res.httpResponseContext = res.httpResponseContext
	}
	return nil
}

// setEntryMergeConfig points this entry's mergeResult at its aliased slice of
// response, and copies over the transport state of the one merged request.
func (l *Loader) setEntryMergeConfig(prepared *preparedFetch, i int, response, entryErrors *astjson.Value) {
	res := prepared.res
	entry := prepared.multiEntries[i].entry
	entryRes := prepared.multiEntries[i].res
	entryRes.multi = &multiEntryMergeConfig{
		alias:        entry.Alias,
		originSingle: entry.OriginKind == EntityFetchOriginSingle,
		info:         entry.Info,
		response:     response,
		errors:       entryErrors,
	}
	entryRes.statusCode = res.statusCode
	entryRes.ds = res.ds
	entryRes.out = res.out
}

// applyCachedEntriesToResponse writes the entities a cache hit already answered
// into response under that entry's alias — they have none of their own, having
// been switched off in the request — so that nothing downstream of here can tell
// a hit from a fetch.
func (l *Loader) applyCachedEntriesToResponse(prepared *preparedFetch, response *astjson.Value) error {
	for i := range prepared.multiEntries {
		entry := &prepared.multiEntries[i]
		if !entry.cacheHit() {
			continue
		}

		entities := astjson.ArrayValue(l.jsonArena)
		for _, value := range entry.cachedValues {
			parsed, parseErr := astjson.ParseBytesWithArena(l.jsonArena, value)
			if parseErr != nil {
				return errors.WithStack(fmt.Errorf("cached entity for alias %s: %w", entry.entry.Alias, parseErr))
			}
			astjson.AppendToArray(l.jsonArena, entities, parsed)
		}

		data := response.Get("data")
		if data == nil || data.Type() != astjson.TypeObject {
			data = astjson.ObjectValue(l.jsonArena)
			response.Set(l.jsonArena, "data", data)
		}
		data.Set(l.jsonArena, entry.entry.Alias, entities)
	}
	return nil
}

// serveCachedEntriesWithoutResponse merges the warm entries of a merged fetch
// that has no response to read: the request failed, or none was sent because
// every entry was warm. They are merged off a document built here, so a
// subgraph being down does not throw away entities already in hand.
func (l *Loader) serveCachedEntriesWithoutResponse(prepared *preparedFetch) error {
	if !slices.ContainsFunc(prepared.multiEntries, func(e preparedMultiEntry) bool { return e.cacheHit() }) {
		return nil
	}

	response := astjson.ObjectValue(l.jsonArena)
	if err := l.applyCachedEntriesToResponse(prepared, response); err != nil {
		return err
	}

	for i := range prepared.multiEntries {
		if !prepared.multiEntries[i].cacheHit() {
			continue
		}
		l.setEntryMergeConfig(prepared, i, response, nil)
		// Neither a failed request's status code nor its body says anything
		// about an entry that was answered before it was sent.
		prepared.multiEntries[i].res.statusCode = http.StatusOK
		prepared.multiEntries[i].res.out = nil
	}
	return nil
}

// mergeEntryResults runs the standard mergeResult for each entry, joins every
// entry's subgraph error into the shared result, and fires OnFinished exactly
// once for the single merged request. Returns the first entry merge error.
func (l *Loader) mergeEntryResults(prepared *preparedFetch) error {
	res := prepared.res
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

// partitionResponseErrors splits the shared response's top-level errors by
// their leading path element: errors keyed by an entry alias are returned
// aligned with prepared.multiEntries; the rest are merged once against the
// parent multi fetch (empty response path) and reported as unmatched, since an
// error belonging to no alias is a problem with the merged request as a whole.
func (l *Loader) partitionResponseErrors(prepared *preparedFetch, response *astjson.Value) ([]*astjson.Value, bool, error) {
	entryErrors := make([]*astjson.Value, len(prepared.multiEntries))
	responseErrors := response.Get("errors")
	if !astjson.ValueIsNonNull(responseErrors) || responseErrors.Type() != astjson.TypeArray {
		return entryErrors, false, nil
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
			return entryErrors, true, err
		}
		return entryErrors, true, nil
	}
	return entryErrors, false, nil
}
