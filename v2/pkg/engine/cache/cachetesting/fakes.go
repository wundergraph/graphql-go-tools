// Package cachetesting provides the shared, cosmo-free test doubles for the
// caching implementation: a recording controller/request-cache pair, an
// in-memory L2 store with an ordered op log, a gate-channel datasource for
// deterministic ordering, and a registry that swaps real plan datasources for
// in-process fakes. Time and TTLs are never faked here — tests wrap
// time-dependent bodies in testing/synctest, which fakes the real time calls.
package cachetesting

import (
	"cmp"
	"context"
	"maps"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// Call is one normalized cache-hook invocation recorded by FakeRequestCache,
// with every loader-supplied signal flattened for full-value assertions.
type Call struct {
	Op           string
	FetchPath    string
	Items        int
	InputBytes   string
	HeaderHash   uint64
	ResponseData string
	MergePath    []string
	HasErrors    bool
	FetchFailed  bool
	EmptyEntity  bool
	StatusCode   int
	Decision     resolve.Decision
}

// ScriptedDecision is what FakeRequestCache returns for a fetch path.
type ScriptedDecision struct {
	Decision resolve.Decision
	Handle   *resolve.FetchCacheHandle
}

// StoreOp is one entry of FakeStore's ordered operation log: one op per store
// CALL, so a batched read logs a single entry carrying every key it asked for.
type StoreOp struct {
	Kind string // "GetMany" or "SetMany"
	// Keys are the keys a GetMany asked for, in call order.
	Keys []string
	// Hits is the per-key outcome of a GetMany, positionally aligned with Keys.
	Hits []bool
	// Items are the entries a SetMany wrote, in call order.
	Items []StoreOpItem
	// Failed marks a call the failure injection rejected: it touched no data.
	Failed bool
}

// StoreOpItem is one written entry inside a SetMany op.
type StoreOpItem struct {
	Key   string
	Value string
	TTL   time.Duration
	// Tags are the entry's invalidation tags, in write order.
	Tags []string
}

// FakeCacheController counts BeginRequest calls and hands out a fixed
// RequestCache, so lifecycle laziness (exactly one BeginRequest per request)
// is observable.
type FakeCacheController struct {
	begins atomic.Int64
	rc     resolve.RequestCache
}

func NewFakeCacheController(rc resolve.RequestCache) *FakeCacheController {
	return &FakeCacheController{rc: rc}
}

func (f *FakeCacheController) BeginRequest(*resolve.Context) resolve.RequestCache {
	f.begins.Add(1)
	return f.rc
}

// Begins returns how often BeginRequest ran.
func (f *FakeCacheController) Begins() int64 {
	return f.begins.Load()
}

// FakeRequestCache records every hook invocation as a normalized Call and
// returns scripted decisions keyed by the fetch's response path. It is safe
// for concurrent use (parallel fetches within one request).
type FakeRequestCache struct {
	mu            sync.Mutex
	calls         []Call
	resultHandles []*resolve.FetchCacheHandle
	script        map[string]ScriptedDecision
	errs          map[string]error
}

func NewFakeRequestCache(script map[string]ScriptedDecision) *FakeRequestCache {
	return &FakeRequestCache{
		script: script,
		errs:   make(map[string]error),
	}
}

// SetError makes the given hook ("Skipped" or "Result") fail for a fetch path.
func (f *FakeRequestCache) SetError(fetchPath, op string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errs[fetchPath+op] = err
}

func (f *FakeRequestCache) PrepareFetch(in resolve.PrepareFetchInput) (resolve.Decision, *resolve.FetchCacheHandle) {
	path := pathOf(in.Item)
	scripted := f.script[path]
	f.record(Call{
		Op:         "Prepare",
		FetchPath:  path,
		Items:      len(in.Items),
		InputBytes: string(in.Input),
		HeaderHash: in.HeaderHash,
		Decision:   scripted.Decision,
	})
	return scripted.Decision, scripted.Handle
}

// OnUncachedFetch records the loader's notification that a fetch ran with no
// cache configuration. It carries no fetch identity — the position in the call
// log is what an assertion reads.
func (f *FakeRequestCache) OnUncachedFetch() {
	f.record(Call{Op: "Uncached"})
}

func (f *FakeRequestCache) OnFetchSkipped(h *resolve.FetchCacheHandle, in resolve.MergeInput) error {
	path := pathOf(in.Item)
	f.record(mergeCall("Skipped", path, in))
	return f.err(path, "Skipped")
}

func (f *FakeRequestCache) OnFetchResult(h *resolve.FetchCacheHandle, in resolve.MergeInput) error {
	path := pathOf(in.Item)
	f.recordResultHandle(h)
	f.record(mergeCall("Result", path, in))
	return f.err(path, "Result")
}

func (f *FakeRequestCache) EndRequest() {
	f.record(Call{Op: "End"})
}

// Calls returns a copy of the recorded calls in invocation order.
func (f *FakeRequestCache) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// ResultHandles returns the handles OnFetchResult received, in order, so tests
// can assert pointer identity with the handle PrepareFetch returned.
func (f *FakeRequestCache) ResultHandles() []*resolve.FetchCacheHandle {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.resultHandles)
}

func (f *FakeRequestCache) record(call Call) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *FakeRequestCache) recordResultHandle(h *resolve.FetchCacheHandle) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resultHandles = append(f.resultHandles, h)
}

func (f *FakeRequestCache) err(path, op string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.errs[path+op]
}

// RecordingController bundles a FakeCacheController with its FakeRequestCache
// for the common "one recording cache per test" case.
type RecordingController struct {
	controller *FakeCacheController
	request    *FakeRequestCache
}

func NewRecordingController(script map[string]ScriptedDecision) *RecordingController {
	request := NewFakeRequestCache(script)
	return &RecordingController{
		controller: NewFakeCacheController(request),
		request:    request,
	}
}

func (r *RecordingController) BeginRequest(ctx *resolve.Context) resolve.RequestCache {
	return r.controller.BeginRequest(ctx)
}

func (r *RecordingController) Calls() []Call {
	return r.request.Calls()
}

func (r *RecordingController) Begins() int64 {
	return r.controller.Begins()
}

func (r *RecordingController) ResultHandles() []*resolve.FetchCacheHandle {
	return r.request.ResultHandles()
}

// StoredEntry is one FakeStore value with its absolute expiry.
type StoredEntry struct {
	Value     []byte
	ExpiresAt time.Time
}

// FakeStore is the in-memory L2 store double implementing cache.Store: values
// with absolute ExpiresAt (real time.Now, faked by synctest in tests), an
// ordered StoreOp log with ONE entry per call, and per-op failure injection.
// A read past expiry is a miss; expired entries are not purged, so the log
// stays complete.
type FakeStore struct {
	mu   sync.Mutex
	data map[string]StoredEntry
	ops  []StoreOp

	failGetMany    int
	failGetManyErr error
	failSetMany    int
	failSetManyErr error
}

func NewFakeStore() *FakeStore {
	return &FakeStore{data: make(map[string]StoredEntry)}
}

// Seed inserts a value WITHOUT logging an op, for arranging preconditions.
func (s *FakeStore) Seed(key string, v []byte, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = StoredEntry{
		Value:     slices.Clone(v),
		ExpiresAt: time.Now().Add(ttl),
	}
}

// FailGetMany makes the next n reads fail with err WITHOUT touching the data;
// the calls still log, marked Failed.
func (s *FakeStore) FailGetMany(n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failGetMany, s.failGetManyErr = n, err
}

// FailSetMany makes the next n writes fail with err WITHOUT touching the data;
// the calls still log, marked Failed.
func (s *FakeStore) FailSetMany(n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failSetMany, s.failSetManyErr = n, err
}

// GetMany answers one Entry per key, in key order.
func (s *FakeStore) GetMany(_ context.Context, keys []string) ([]cache.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failGetMany > 0 {
		s.failGetMany--
		s.ops = append(s.ops, StoreOp{Kind: "GetMany", Keys: slices.Clone(keys), Failed: true})
		return nil, s.failGetManyErr
	}
	entries := make([]cache.Entry, len(keys))
	hits := make([]bool, len(keys))
	for i, key := range keys {
		entry, ok := s.data[key]
		if !ok || !time.Now().Before(entry.ExpiresAt) {
			continue
		}
		entries[i] = cache.Entry{
			Value:        slices.Clone(entry.Value),
			RemainingTTL: time.Until(entry.ExpiresAt),
			OK:           true,
		}
		hits[i] = true
	}
	s.ops = append(s.ops, StoreOp{Kind: "GetMany", Keys: slices.Clone(keys), Hits: hits})
	return entries, nil
}

// SetMany writes every item under its own TTL.
func (s *FakeStore) SetMany(_ context.Context, items []cache.Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	op := StoreOp{Kind: "SetMany", Items: make([]StoreOpItem, 0, len(items))}
	for _, item := range items {
		op.Items = append(op.Items, StoreOpItem{
			Key:   item.Key,
			Value: string(item.Value),
			TTL:   item.TTL,
			Tags:  slices.Clone(item.Tags),
		})
	}
	if s.failSetMany > 0 {
		s.failSetMany--
		op.Failed = true
		s.ops = append(s.ops, op)
		return s.failSetManyErr
	}
	for _, item := range items {
		s.data[item.Key] = StoredEntry{
			Value:     slices.Clone(item.Value),
			ExpiresAt: time.Now().Add(item.TTL),
		}
	}
	s.ops = append(s.ops, op)
	return nil
}

// Value returns the stored value under key and whether it is present and
// unexpired, WITHOUT logging an op, so assertions can read the store directly.
func (s *FakeStore) Value(key string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.data[key]
	if !ok || !time.Now().Before(entry.ExpiresAt) {
		return nil, false
	}
	return slices.Clone(entry.Value), true
}

// Ops returns a copy of the ordered operation log.
func (s *FakeStore) Ops() []StoreOp {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.ops)
}

// ResetOps clears the operation log (the DATA stays), so a multi-request test
// can assert each request's ops in isolation instead of re-listing the
// accumulated history.
func (s *FakeStore) ResetOps() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ops = nil
}

// GatedDataSource is an in-process DataSource whose Load can be ordered
// deterministically with gate channels: it announces arrival on Arrived, then
// blocks until Release yields, then returns Resp/Err. Nil channels skip the
// respective step. Tests coordinate the channels with synctest.Wait — never
// with latency sleeps.
type GatedDataSource struct {
	Name        string
	Resp        []byte
	Err         error
	Arrived     chan<- string
	Release     <-chan struct{}
	LoadCounter *atomic.Int64
	// RecordInput (optional) receives every Load's input bytes, so tests can
	// assert the EXACT request the subgraph saw (e.g. partial-batch filtering).
	RecordInput func(input []byte)
}

// DataSourceGate is the per-fetch gate configuration FakeRegistry attaches to
// a swapped datasource.
type DataSourceGate struct {
	Arrived chan<- string
	Release <-chan struct{}
}

func (g *GatedDataSource) Load(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
	if g.LoadCounter != nil {
		g.LoadCounter.Add(1)
	}
	if g.RecordInput != nil {
		g.RecordInput(input) // the recorder copies (string conversion) only while under its cap
	}
	if g.Arrived != nil {
		g.Arrived <- g.Name
	}
	if g.Release != nil {
		<-g.Release
	}
	return g.Resp, g.Err
}

func (g *GatedDataSource) LoadWithFiles(context.Context, http.Header, []byte, []*httpclient.FileUpload) ([]byte, error) {
	panic("cache tests never upload files")
}

// ShadowCompare is one recorded shadow probe: the stashed entry's key, its
// age (CacheTTL - RemainingTTL), and whether the cached bytes equal the fresh
// value the fetch produced for that item.
type ShadowCompare struct {
	CacheKey string
	IsFresh  bool
	CacheAge time.Duration
}

// RecordingObserver is the CacheObserver double: it counts lifecycle calls,
// records the handles it sees, and materializes shadow compares (byte
// equality of stashed vs fresh, per item).
type RecordingObserver struct {
	mu              sync.Mutex
	beginRequests   int
	endRequests     int
	observedHandles []*resolve.FetchCacheHandle
	compares        []ShadowCompare
	storeErrors     []cache.StoreError
	// uncacheablePrivate records the private results that stayed out of the
	// store, one entry per occurrence.
	uncacheablePrivate []cache.UncacheablePrivate
	// scopeMismatches counts entries discarded for carrying the other privacy
	// scope, per subgraph and stored scope.
	scopeMismatches map[cache.ScopeMismatch]int
}

func (o *RecordingObserver) BeginRequest(ctx *resolve.Context) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.beginRequests++
}

func (o *RecordingObserver) EndRequest(ctx *resolve.Context) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.endRequests++
}

func (o *RecordingObserver) OnFetchObserved(h *resolve.FetchCacheHandle) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.observedHandles = append(o.observedHandles, h)
}

func (o *RecordingObserver) CompareShadow(h *resolve.FetchCacheHandle, fresh *astjson.Value, tx *resolve.CacheTransaction) {
	if h == nil {
		return
	}
	var batch []*astjson.Value
	if h.BatchEntityKey && fresh != nil {
		batch = fresh.GetArray()
	}
	compares := make([]ShadowCompare, 0, len(h.ShadowStash))
	for itemIndex, entry := range h.ShadowStash {
		freshValue := fresh
		if h.BatchEntityKey {
			freshValue = nil
			if itemIndex >= 0 && itemIndex < len(h.Items) {
				if batchIndex := h.Items[itemIndex].BatchIndex; batchIndex >= 0 && batchIndex < len(batch) {
					freshValue = batch[batchIndex]
				}
			}
		}
		compares = append(compares, ShadowCompare{
			CacheKey: entry.CacheKey,
			IsFresh:  string(marshalValue(entry.CachedValue)) == string(marshalValue(freshValue)),
			CacheAge: entry.CacheTTL - entry.RemainingTTL,
		})
	}
	slices.SortFunc(compares, func(a, b ShadowCompare) int {
		return cmp.Compare(a.CacheKey, b.CacheKey)
	})
	o.mu.Lock()
	defer o.mu.Unlock()
	o.compares = append(o.compares, compares...)
}

func marshalValue(v *astjson.Value) []byte {
	if v == nil {
		return nil
	}
	return v.MarshalTo(nil)
}

// OnStoreError records one reported store failure.
func (o *RecordingObserver) OnStoreError(op string, subgraph string, keyCount int, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.storeErrors = append(o.storeErrors, cache.StoreError{
		Op:       op,
		Subgraph: subgraph,
		KeyCount: keyCount,
		Err:      err,
	})
}

// OnUncacheablePrivate records one private result the store never received.
func (o *RecordingObserver) OnUncacheablePrivate(subgraph string, reason string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.uncacheablePrivate = append(o.uncacheablePrivate, cache.UncacheablePrivate{
		Subgraph: subgraph,
		Reason:   reason,
	})
}

// UncacheablePrivate returns the recorded private results the store never
// received, in occurrence order.
func (o *RecordingObserver) UncacheablePrivate() []cache.UncacheablePrivate {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.uncacheablePrivate)
}

// OnScopeMismatch counts one entry discarded for carrying the other privacy
// scope.
func (o *RecordingObserver) OnScopeMismatch(subgraph string, storedScope string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.scopeMismatches == nil {
		o.scopeMismatches = make(map[cache.ScopeMismatch]int)
	}
	o.scopeMismatches[cache.ScopeMismatch{Subgraph: subgraph, StoredScope: storedScope}]++
}

// ScopeMismatches returns the discarded-entry counts per subgraph and stored
// scope.
func (o *RecordingObserver) ScopeMismatches() map[cache.ScopeMismatch]int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return maps.Clone(o.scopeMismatches)
}

// StoreErrors returns the recorded store failures in occurrence order.
func (o *RecordingObserver) StoreErrors() []cache.StoreError {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.storeErrors)
}

func (o *RecordingObserver) OnEntity(h *resolve.FetchCacheHandle, entity *astjson.Value) {}

func (o *RecordingObserver) OnFieldValue(coordinate resolve.GraphCoordinate, value resolve.FieldValue) {
}

// Counts returns (beginRequests, endRequests).
func (o *RecordingObserver) Counts() (int, int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.beginRequests, o.endRequests
}

// ObservedHandles returns the handles passed to OnFetchObserved, in order.
func (o *RecordingObserver) ObservedHandles() []*resolve.FetchCacheHandle {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.observedHandles)
}

// Compares returns the recorded shadow probes, sorted by cache key per call.
func (o *RecordingObserver) Compares() []ShadowCompare {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.compares)
}

// FakeRegistry hands out GatedDataSources with canned responses and tracks
// per-fetch load counts, so tests can assert "no network on a hit" without a
// socket.
type FakeRegistry struct {
	mu        sync.Mutex
	responses map[string]string
	release   chan struct{}
	loads     map[string]*atomic.Int64
	gates     map[string]DataSourceGate
	inputs    map[string][]string
}

// NewFakeRegistry builds a registry over canned responses. Response keys are
// tried in order: "DataSourceName:ResponsePath", "DataSourceName",
// "ResponsePath", "*".
func NewFakeRegistry(responses map[string]string) *FakeRegistry {
	release := make(chan struct{})
	close(release) // ungated by default: Load returns immediately
	return &FakeRegistry{
		responses: responses,
		release:   release,
		loads:     make(map[string]*atomic.Int64),
	}
}

// SetGate attaches gate channels to the datasource identified by name + path;
// call it before SwapDataSources.
func (r *FakeRegistry) SetGate(name, path string, gate DataSourceGate) {
	key := name + ":" + path
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gates == nil {
		r.gates = make(map[string]DataSourceGate)
	}
	r.gates[key] = gate
}

// SwapDataSources walks a fetch tree and replaces every fetch's transport with
// a registry-backed GatedDataSource (via Fetch.SetDataSource — no switch over
// concrete fetch types).
func SwapDataSources(node *resolve.FetchTreeNode, reg *FakeRegistry) {
	if node == nil || reg == nil {
		return
	}
	if node.Item != nil && node.Item.Fetch != nil {
		node.Item.Fetch.SetDataSource(reg.dataSourceFor(node.Item))
	}
	for _, child := range node.ChildNodes {
		SwapDataSources(child, reg)
	}
}

// LoadCount returns how often the datasource identified by name + path
// loaded, or -1 when no such datasource was ever swapped in (counters are
// created eagerly at swap time) — a typo'd name/path pair can then never
// satisfy a zero-loads assertion vacuously.
func (r *FakeRegistry) LoadCount(name, path string) int64 {
	key := name + ":" + path
	r.mu.Lock()
	defer r.mu.Unlock()
	counter := r.loads[key]
	if counter == nil {
		return -1
	}
	return counter.Load()
}

// Inputs returns the exact input bytes every Load of the datasource
// identified by name + path received, in order. An unknown pair returns nil —
// assert LoadCount alongside when "no inputs" is the expectation.
func (r *FakeRegistry) Inputs(name, path string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.inputs[name+":"+path])
}

// maxRecordedInputs bounds per-datasource input recording so long-running
// callers (benchmarks) do not accumulate unbounded copies; assertions read
// the first loads, which is what the e2e rows need.
const maxRecordedInputs = 16

func (r *FakeRegistry) recordInput(key string) func([]byte) {
	return func(input []byte) {
		r.mu.Lock()
		defer r.mu.Unlock()
		if len(r.inputs[key]) >= maxRecordedInputs {
			return
		}
		if r.inputs == nil {
			r.inputs = make(map[string][]string)
		}
		r.inputs[key] = append(r.inputs[key], string(input))
	}
}

func (r *FakeRegistry) dataSourceFor(item *resolve.FetchItem) resolve.DataSource {
	name := dataSourceName(item)
	path := pathOf(item)
	resp := r.responseFor(name, path)
	gate := r.gateFor(name, path)
	var release <-chan struct{} = r.release
	if gate.Release != nil {
		release = gate.Release
	}
	return &GatedDataSource{
		Name:        name,
		Resp:        []byte(resp),
		Arrived:     gate.Arrived,
		Release:     release,
		LoadCounter: r.loadCounter(name, path),
		RecordInput: r.recordInput(name + ":" + path),
	}
}

func (r *FakeRegistry) gateFor(name, path string) DataSourceGate {
	key := name + ":" + path
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.gates[key]
}

func (r *FakeRegistry) loadCounter(name, path string) *atomic.Int64 {
	key := name + ":" + path
	r.mu.Lock()
	defer r.mu.Unlock()
	counter := r.loads[key]
	if counter == nil {
		counter = &atomic.Int64{}
		r.loads[key] = counter
	}
	return counter
}

func (r *FakeRegistry) responseFor(name, path string) string {
	keys := []string{name + ":" + path, name, path, "*"}
	for _, key := range keys {
		if value, ok := r.responses[key]; ok {
			return value
		}
	}
	return ""
}

// Compact normalizes a JSON string for full-value response assertions.
func Compact(tb testing.TB, s string) string {
	tb.Helper()
	v, err := astjson.ParseBytes([]byte(s))
	if err != nil {
		tb.Fatalf("compact json: %v", err)
	}
	return string(v.MarshalTo(nil))
}

func pathOf(item *resolve.FetchItem) string {
	if item == nil {
		return ""
	}
	return item.ResponsePath
}

func dataSourceName(item *resolve.FetchItem) string {
	if item == nil || item.Fetch == nil || item.Fetch.FetchInfo() == nil {
		return ""
	}
	return item.Fetch.FetchInfo().DataSourceName
}

func mergeCall(op, path string, in resolve.MergeInput) Call {
	return Call{
		Op:           op,
		FetchPath:    path,
		Items:        len(in.Items),
		ResponseData: string(valueBytes(in.ResponseData)),
		MergePath:    slices.Clone(in.MergePath),
		HasErrors:    in.HasErrors,
		FetchFailed:  in.FetchFailed,
		EmptyEntity:  in.EmptyEntity,
		StatusCode:   in.StatusCode,
	}
}

func valueBytes(v *astjson.Value) []byte {
	if v == nil {
		return nil
	}
	return v.MarshalTo(nil)
}
