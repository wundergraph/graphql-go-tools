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
	"net/http"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wundergraph/astjson"

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

// StoreOp is one entry of FakeStore's ordered operation log.
type StoreOp struct {
	Kind  string // "Get" or "Set"
	Key   string
	Value string        // set only for "Set"
	TTL   time.Duration // set only for "Set"
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

// FakeStore is the in-memory L2 store double: values with absolute ExpiresAt
// (real time.Now, faked by synctest in tests) and an ordered StoreOp log.
// A Get past expiry is a miss; expired entries are not purged, so the log
// stays complete.
type FakeStore struct {
	mu   sync.Mutex
	data map[string]StoredEntry
	ops  []StoreOp
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

func (s *FakeStore) Set(key string, v []byte, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = StoredEntry{
		Value:     slices.Clone(v),
		ExpiresAt: time.Now().Add(ttl),
	}
	s.ops = append(s.ops, StoreOp{Kind: "Set", Key: key, Value: string(v), TTL: ttl})
}

func (s *FakeStore) Get(key string) (StoredEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ops = append(s.ops, StoreOp{Kind: "Get", Key: key})
	entry, ok := s.data[key]
	if !ok || !time.Now().Before(entry.ExpiresAt) {
		return StoredEntry{}, false
	}
	entry.Value = slices.Clone(entry.Value)
	return entry, true
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
// equality of stashed vs fresh, per item). Production observer wiring arrives
// with ART (task 20); this double pins the compare inputs.
type RecordingObserver struct {
	mu              sync.Mutex
	beginRequests   int
	endRequests     int
	observedHandles []*resolve.FetchCacheHandle
	compares        []ShadowCompare
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

// LoadCount returns how often the datasource identified by name + path loaded.
func (r *FakeRegistry) LoadCount(name, path string) int64 {
	return r.loadCounter(name, path).Load()
}

// Inputs returns the exact input bytes every Load of the datasource
// identified by name + path received, in order.
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
