package resolve

import (
	"fmt"
	"time"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

// CacheController is the long-lived, integrator-supplied lifecycle port. The
// router sets one via Context.SetCacheController, exactly as it sets an Authorizer.
// A nil controller is the global NO-OP and is zero-cost. NO-OP, L1-only, L2-only,
// and L1+L2 are all distinguished only by the RequestCache the controller hands back.
type CacheController interface {
	// BeginRequest is called once per request, lazily, under DataBuffer.Lock, the
	// first time a cache-eligible fetch is prepared.
	BeginRequest(ctx *Context) RequestCache
}

// RequestCache is the ONE mode-blind working surface the loader talks to.
type RequestCache interface {
	PrepareFetch(in PrepareFetchInput) (Decision, *FetchCacheHandle)
	OnFetchSkipped(h *FetchCacheHandle, in MergeInput) error
	OnFetchResult(h *FetchCacheHandle, in MergeInput) error
	EndRequest()
}

// Decision is what PrepareFetch tells the loader to do. It is the ONLY cache
// concept the loader branches on.
type Decision uint8

const (
	// DecisionFetch: miss, or caching disabled for this fetch. loadPhase fetches normally.
	DecisionFetch Decision = iota

	// DecisionSkipFullHit: every item is covered by a covering cache value.
	DecisionSkipFullHit

	// DecisionFetchPartial: some items covered, some not. STAGED to v2.
	DecisionFetchPartial

	// DecisionFetchShadow: shadow read hit; the loader treats it exactly like a miss.
	DecisionFetchShadow
)

func (d Decision) String() string {
	switch d {
	case DecisionFetch:
		return "Fetch"
	case DecisionSkipFullHit:
		return "SkipFullHit"
	case DecisionFetchPartial:
		return "FetchPartial"
	case DecisionFetchShadow:
		return "FetchShadow"
	default:
		return fmt.Sprintf("Decision(%d)", d)
	}
}

type PrepareFetchInput struct {
	Ctx        *Context
	Item       *FetchItem
	Items      []*astjson.Value
	Config     *FetchCacheConfig
	BatchStats [][]*astjson.Value
	Input      []byte
	HeaderHash uint64
	Arena      MergeArena
}

type MergeInput struct {
	Item         *FetchItem
	Items        []*astjson.Value
	ResponseData *astjson.Value
	BatchStats   [][]*astjson.Value
	HasErrors    bool
	FetchFailed  bool
	EmptyEntity  bool
	StatusCode   int
	Arena        MergeArena
}

// MergeArena is the narrow, lock-guarded facade over the Loader's jsonArena.
type MergeArena interface {
	Begin() MergeSession
}

// MergeSession is the scoped, lock-held handle for one multi-op arena sequence.
type MergeSession interface {
	ParseBytes(b []byte) (*astjson.Value, error)
	StructuralCopy(v *astjson.Value) *astjson.Value
	MergeValues(dst, src *astjson.Value) (*astjson.Value, error)
	MergeValuesWithPath(dst, src *astjson.Value, path ...string) (*astjson.Value, error)
	NewObject() *astjson.Value
	NewArray() *astjson.Value
	String(s string) *astjson.Value
	Null() *astjson.Value
	Close()
}

// CacheObserver is the optional analytics/trace/shadow-compare port.
type CacheObserver interface {
	BeginRequest(ctx *Context)
	EndRequest(ctx *Context)
	OnFetchObserved(h *FetchCacheHandle)
	CompareShadow(h *FetchCacheHandle, fresh *astjson.Value, s MergeSession)
	OnEntity(h *FetchCacheHandle, entity *astjson.Value)
	OnFieldValue(coordinate GraphCoordinate, value FieldValue)
}

// FetchCacheHandle is the per-fetch opaque cache state, carried on preparedFetch.
type FetchCacheHandle struct {
	Decision       Decision
	WasHit         bool
	MustWriteBack  bool
	BatchEntityKey bool
	Shadow         bool
	ShadowStash    map[int]ShadowCacheEntry
	Items          []ItemCacheState
	Analytics      any
}

// String renders a compact, nil-safe summary for logs and panics.
func (h *FetchCacheHandle) String() string {
	if h == nil {
		return "<nil>"
	}
	hits := 0
	writeback := 0
	for _, item := range h.Items {
		if h.WasHit || item.FromCache != nil || item.NegativeHit {
			hits++
		}
		if item.NeedsWriteback {
			writeback++
		}
	}
	if h.MustWriteBack && writeback == 0 {
		writeback = 1
	}
	return fmt.Sprintf("{decision:%s items:%d hits:%d writeback:%d shadow:%t}", h.Decision, len(h.Items), hits, writeback, h.Shadow)
}

// ItemCacheState is the per-item cache payload, one per merge target.
type ItemCacheState struct {
	Item                 *astjson.Value
	FromCache            *astjson.Value
	RenderedKeys         []string
	PendingCandidates    []CacheKeyCandidate
	FromCacheCandidates  []CacheCandidate
	SelectedRemainingTTL time.Duration
	NeedsWriteback       bool
	EntityMergePath      []string
	BatchIndex           int
	BatchEntityKey       bool
	NegativeHit          bool
	WriteReason          CacheWriteReason
}

type CacheCandidate struct {
	Value        []byte
	RemainingTTL time.Duration
}

type ShadowCacheEntry struct {
	CachedValue  *astjson.Value
	CacheKey     string
	RemainingTTL time.Duration
}

type mergeArena struct {
	a  arena.Arena
	db *DataBuffer
}

func (m mergeArena) Begin() MergeSession {
	m.db.Lock()
	return &mergeSession{a: m.a, db: m.db}
}

type mergeSession struct {
	a  arena.Arena
	db *DataBuffer
}

func (m *mergeSession) ParseBytes(b []byte) (*astjson.Value, error) {
	return astjson.ParseBytesWithArena(m.a, b)
}

func (m *mergeSession) StructuralCopy(v *astjson.Value) *astjson.Value {
	return astjson.DeepCopy(m.a, v)
}

func (m *mergeSession) MergeValues(dst, src *astjson.Value) (*astjson.Value, error) {
	merged, _, err := astjson.MergeValues(m.a, dst, src)
	return merged, err
}

func (m *mergeSession) MergeValuesWithPath(dst, src *astjson.Value, path ...string) (*astjson.Value, error) {
	merged, _, err := astjson.MergeValuesWithPath(m.a, dst, src, path...)
	return merged, err
}

func (m *mergeSession) NewObject() *astjson.Value {
	return astjson.ObjectValue(m.a)
}

func (m *mergeSession) NewArray() *astjson.Value {
	return astjson.ArrayValue(m.a)
}

func (m *mergeSession) String(s string) *astjson.Value {
	return astjson.StringValue(m.a, s)
}

func (m *mergeSession) Null() *astjson.Value {
	return astjson.NullValue
}

func (m *mergeSession) Close() {
	m.db.Unlock()
}
