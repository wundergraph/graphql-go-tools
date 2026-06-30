package cachetesting

import (
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

// CacheStage mirrors the staged caching implementation order from the S4 plan.
type CacheStage uint8

const (
	StageNoop CacheStage = iota
	StageL2Entities
	StageL2RootFields
	StageL2RootReusesEntity
	StageL1
)

type Call struct {
	Op           string
	FetchPath    string
	Items        int
	InputBytes   string
	HeaderHash   uint64
	ResponseData string
	HasErrors    bool
	FetchFailed  bool
	EmptyEntity  bool
	StatusCode   int
	Decision     resolve.Decision
}

type ScriptedDecision struct {
	Decision resolve.Decision
	Handle   *resolve.FetchCacheHandle
}

type StoreOp struct {
	Kind  string
	Key   string
	Value string
	TTL   time.Duration
}

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

func (f *FakeCacheController) Begins() int64 {
	return f.begins.Load()
}

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

func (f *FakeRequestCache) SetError(fetchPath, op string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.errs == nil {
		f.errs = make(map[string]error)
	}
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

func (f *FakeRequestCache) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

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

type RecordingController struct {
	controller *FakeCacheController
	request    *FakeRequestCache
}

func NewRecordingCache(script map[string]ScriptedDecision) *RecordingController {
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

type StoredEntry struct {
	Value     []byte
	ExpiresAt time.Time
}

type FakeStore struct {
	mu   sync.Mutex
	data map[string]StoredEntry
	ops  []StoreOp
}

func NewFakeStore() *FakeStore {
	return &FakeStore{data: make(map[string]StoredEntry)}
}

func (s *FakeStore) Seed(key string, v []byte, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensure()
	s.data[key] = StoredEntry{
		Value:     slices.Clone(v),
		ExpiresAt: time.Now().Add(ttl),
	}
}

func (s *FakeStore) Set(key string, v []byte, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensure()
	s.data[key] = StoredEntry{
		Value:     slices.Clone(v),
		ExpiresAt: time.Now().Add(ttl),
	}
	s.ops = append(s.ops, StoreOp{Kind: "Set", Key: key, Value: string(v), TTL: ttl})
}

func (s *FakeStore) Get(key string) (StoredEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensure()
	s.ops = append(s.ops, StoreOp{Kind: "Get", Key: key})
	entry, ok := s.data[key]
	if !ok || !time.Now().Before(entry.ExpiresAt) {
		return StoredEntry{}, false
	}
	entry.Value = slices.Clone(entry.Value)
	return entry, true
}

func (s *FakeStore) Ops() []StoreOp {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.ops)
}

func (s *FakeStore) ensure() {
	if s.data == nil {
		s.data = make(map[string]StoredEntry)
	}
}

type GatedDataSource struct {
	Name    string
	Resp    []byte
	Err     error
	Arrived chan<- string
	Release <-chan struct{}
}

func (g *GatedDataSource) Load(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
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

type ShadowCompare struct {
	CacheKey   string
	EntityType string
	IsFresh    bool
	CacheAge   time.Duration
}

type RecordingObserver struct {
	mu       sync.Mutex
	compares []ShadowCompare
}

func (o *RecordingObserver) BeginRequest(ctx *resolve.Context) {}

func (o *RecordingObserver) EndRequest(ctx *resolve.Context) {}

func (o *RecordingObserver) OnFetchObserved(h *resolve.FetchCacheHandle) {}

func (o *RecordingObserver) CompareShadow(h *resolve.FetchCacheHandle, fresh *astjson.Value, s resolve.MergeSession) {
	if h == nil {
		return
	}
	freshBytes := valueBytes(fresh)
	compares := make([]ShadowCompare, 0, len(h.ShadowStash))
	for _, cached := range h.ShadowStash {
		compares = append(compares, ShadowCompare{
			CacheKey: cached.CacheKey,
			IsFresh:  string(valueBytes(cached.CachedValue)) == string(freshBytes),
		})
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.compares = append(o.compares, compares...)
}

func (o *RecordingObserver) OnEntity(h *resolve.FetchCacheHandle, entity *astjson.Value) {}

func (o *RecordingObserver) OnFieldValue(coordinate resolve.GraphCoordinate, value resolve.FieldValue) {
}

func (o *RecordingObserver) Compares() []ShadowCompare {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.compares)
}

type FakeRegistry struct {
	responses map[string]string
	release   chan struct{}
}

func NewFakeRegistry(responses map[string]string) *FakeRegistry {
	release := make(chan struct{})
	close(release)
	return &FakeRegistry{
		responses: responses,
		release:   release,
	}
}

// SwapDataSources keys responses by DataSourceName + ":" + ResponsePath, with
// fallbacks to DataSourceName, ResponsePath, and "*" for small S4b fixtures.
func SwapDataSources(node *resolve.FetchTreeNode, reg *FakeRegistry) {
	if node == nil || reg == nil {
		return
	}
	if node.Item != nil {
		ds := reg.dataSourceFor(node.Item)
		switch fetch := node.Item.Fetch.(type) {
		case *resolve.SingleFetch:
			fetch.DataSource = ds
		case *resolve.EntityFetch:
			fetch.DataSource = ds
		case *resolve.BatchEntityFetch:
			fetch.DataSource = ds
		}
	}
	for _, child := range node.ChildNodes {
		SwapDataSources(child, reg)
	}
}

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

func mergeCall(op, path string, in resolve.MergeInput) Call {
	return Call{
		Op:           op,
		FetchPath:    path,
		Items:        len(in.Items),
		ResponseData: string(valueBytes(in.ResponseData)),
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

func (r *FakeRegistry) dataSourceFor(item *resolve.FetchItem) resolve.DataSource {
	name := dataSourceName(item)
	path := pathOf(item)
	resp := r.responseFor(name, path)
	return &GatedDataSource{
		Name:    name,
		Resp:    []byte(resp),
		Release: r.release,
	}
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

func dataSourceName(item *resolve.FetchItem) string {
	if item == nil || item.Fetch == nil || item.Fetch.FetchInfo() == nil {
		return ""
	}
	return item.Fetch.FetchInfo().DataSourceName
}
