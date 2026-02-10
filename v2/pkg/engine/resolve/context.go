package resolve

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"time"

	"go.uber.org/atomic"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

// Context should not ever be initialized directly, and should be initialized via the NewContext function
type Context struct {
	ctx              context.Context
	Variables        *astjson.Value
	VariablesHash    uint64
	Files            []*httpclient.FileUpload
	Request          Request
	RenameTypeNames  []RenameTypeName
	RemapVariables   map[string]string
	TracingOptions   TraceOptions
	RateLimitOptions RateLimitOptions
	ExecutionOptions ExecutionOptions
	InitialPayload   []byte
	Extensions       []byte
	LoaderHooks      LoaderHooks

	authorizer    Authorizer
	rateLimiter   RateLimiter
	fieldRenderer FieldValueRenderer

	subgraphErrors map[string]error

	SubgraphHeadersBuilder SubgraphHeadersBuilder

	// Debug enables enrichment of context with debug metadata (e.g., cache fetch info).
	// Zero overhead when disabled (production default). Tests opt in via engine.WithDebugMode().
	Debug bool

	// cacheStats tracks L1/L2 cache hit/miss statistics for the current request.
	// Use GetCacheStats() to retrieve the statistics after execution.
	cacheStats CacheStats
}

// SubgraphHeadersBuilder allows the user of the engine to "define" the headers for a subgraph request
// Instead of going back and forth between engine & transport,
// you can simply define a function that returns headers for a Subgraph request
// In addition to just the header, the implementer can return a hash for the header which will be used by request deduplication
type SubgraphHeadersBuilder interface {
	// HeadersForSubgraph must return the headers and a hash for a Subgraph Request
	// The hash will be used for request deduplication
	HeadersForSubgraph(subgraphName string) (http.Header, uint64)
	// HashAll must return the hash for all subgraph requests combined
	HashAll() uint64
}

// HeadersForSubgraphRequest returns headers and a hash for a request that the engine will make to a subgraph
func (c *Context) HeadersForSubgraphRequest(subgraphName string) (http.Header, uint64) {
	if c.SubgraphHeadersBuilder == nil {
		return nil, 0
	}
	return c.SubgraphHeadersBuilder.HeadersForSubgraph(subgraphName)
}

type ExecutionOptions struct {
	// SkipLoader will, as the name indicates, skip loading data
	// However, it does indeed resolve a response
	// This can be useful, e.g. in combination with IncludeQueryPlanInResponse
	// The purpose is to get a QueryPlan (even for Subscriptions)
	SkipLoader bool
	// IncludeQueryPlanInResponse generates a QueryPlan as part of the response in Resolvable
	IncludeQueryPlanInResponse bool
	// SendHeartbeat sends regular HeartBeats for Subscriptions
	SendHeartbeat bool
	// DisableSubgraphRequestDeduplication disables deduplication of requests to the same subgraph with the same input within a single operation execution.
	DisableSubgraphRequestDeduplication bool
	// DisableInboundRequestDeduplication disables deduplication of inbound client requests
	// The engine is hashing the normalized operation, variables, and forwarded headers to achieve robust deduplication
	// By default, overhead is negligible and as such this should be false (not disabled) most of the time
	// However, if you're benchmarking internals of the engine, it can be helpful to switch it off
	// When disabled (set to true) the code becomes a no-op
	DisableInboundRequestDeduplication bool
	// Caching configures L1 (per-request) and L2 (external) entity caching.
	Caching CachingOptions
	// ErrorBehavior controls error handling during resolution.
	// Only effective when OnErrorEnabled is true in ResolverOptions.
	// Default is ErrorBehaviorPropagate for backward compatibility.
	ErrorBehavior ErrorBehavior
}

// CachingOptions configures the L1/L2 entity caching behavior.
//
// L1 Cache (Per-Request, In-Memory):
//   - Stored in Loader as sync.Map
//   - Lifecycle: Single GraphQL request
//   - Key format: Entity cache key WITHOUT subgraph header prefix
//   - Thread-safe via sync.Map for parallel fetch support
//   - Purpose: Prevents redundant fetches for same entity at different paths
//   - IMPORTANT: Only used for entity fetches, NOT root fetches.
//     Root fields have no prior entity data to look up.
//
// L2 Cache (External, Cross-Request):
//   - Uses LoaderCache interface implementations (e.g., Redis)
//   - Lifecycle: Configured TTL, shared across requests
//   - Key format: Entity cache key WITH optional subgraph header prefix
//   - Purpose: Reduces subgraph load by caching across requests
//   - Applies to both root fetches and entity fetches
//
// Lookup Order (entity fetches): L1 -> L2 -> Subgraph Fetch
// Lookup Order (root fetches): L2 -> Subgraph Fetch (no L1)
type CachingOptions struct {
	// EnableL1Cache enables per-request in-memory entity caching.
	// L1 prevents redundant fetches for the same entity within a single request.
	// Only applies to entity fetches (not root queries) since root queries
	// have no prior entity data to use as a cache key.
	// Default: false (must be explicitly enabled)
	EnableL1Cache bool
	// EnableL2Cache enables external cache lookups (e.g., Redis).
	// L2 allows sharing entity data across requests.
	// Default: false (must be explicitly enabled)
	// Note: When false, existing FetchCacheConfiguration.Enabled still controls
	// per-fetch L2 behavior for backward compatibility.
	EnableL2Cache bool
}

// CacheStats tracks cache hit/miss statistics for L1 and L2 caches.
// These statistics are collected during query execution and can be used
// for monitoring, debugging, and testing cache effectiveness.
//
// Thread Safety:
//   - L1 stats use plain int64 (main thread only)
//   - L2 stats use *atomic.Int64 (accessed from parallel goroutines)
type CacheStats struct {
	// L1 cache statistics (per-request, in-memory)
	// Safe: Only accessed from main thread
	L1Hits   int64 // Number of L1 cache hits
	L1Misses int64 // Number of L1 cache misses

	// L2 cache statistics (external cache)
	// Thread-safe: Accessed from parallel goroutines via atomic operations
	L2Hits   *atomic.Int64 // Number of L2 cache hits
	L2Misses *atomic.Int64 // Number of L2 cache misses
}

type FieldValue struct {
	// Name is the name of the field, e.g. "id", "name", etc.
	Name string
	// Type is the type of the field, e.g. "String", "Int", etc.
	Type string
	// ParentType is the type of the parent object, e.g. "User", "Post", etc.
	ParentType string
	// IsListItem indicates whether the field is a list (array) item.
	IsListItem bool
	// IsNullable indicates whether the field is nullable.
	IsNullable bool
	// IsEnum is a value of Enum
	IsEnum bool

	// Path holds the path to the field in the response.
	Path string

	// Data holds the actual field value data.
	Data []byte

	// ParsedData is the astjson.Value representation of the field value data.
	ParsedData *astjson.Value
}

type FieldValueRenderer interface {
	// RenderFieldValue renders a field value to the provided writer.
	RenderFieldValue(ctx *Context, value FieldValue, out io.Writer) error
}

func (c *Context) SetFieldValueRenderer(renderer FieldValueRenderer) {
	c.fieldRenderer = renderer
}

type AuthorizationDeny struct {
	Reason string
}

type Authorizer interface {
	// AuthorizePreFetch is called prior to making a fetch in the loader
	// This allows to implement policies to prevent fetches to an origin
	// E.g. for Mutations, it might be undesired to just filter out the response
	// You'd want to prevent sending the Operation to the Origin completely
	//
	// The input argument is the final render of the datasource input
	AuthorizePreFetch(ctx *Context, dataSourceID string, input json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error)
	// AuthorizeObjectField operates on the response and can solely be used to implement policies to filter out response fields
	// In contrast to AuthorizePreFetch, this cannot be used to prevent origin requests
	// This function only allows you to filter the response before rendering it to the client
	//
	// The object argument is the flat render of the field-enclosing response object
	// Flat render means, we're only rendering scalars, not arrays or objects
	AuthorizeObjectField(ctx *Context, dataSourceID string, object json.RawMessage, coordinate GraphCoordinate) (result *AuthorizationDeny, err error)
	HasResponseExtensionData(ctx *Context) bool
	RenderResponseExtension(ctx *Context, out io.Writer) error
}

func (c *Context) SetAuthorizer(authorizer Authorizer) {
	c.authorizer = authorizer
}

func (c *Context) SetEngineLoaderHooks(hooks LoaderHooks) {
	c.LoaderHooks = hooks
}

type RateLimitOptions struct {
	// Enable switches rate limiting on or off
	Enable bool
	// IncludeStatsInResponseExtension includes the rate limit stats in the response extensions
	IncludeStatsInResponseExtension bool

	Rate                    int
	Burst                   int
	Period                  time.Duration
	RateLimitKey            string
	RejectExceedingRequests bool

	ErrorExtensionCode RateLimitErrorExtensionCode
}

type RateLimitErrorExtensionCode struct {
	Enabled bool
	Code    string
}

type RateLimitDeny struct {
	Reason string
}

type RateLimiter interface {
	RateLimitPreFetch(ctx *Context, info *FetchInfo, input json.RawMessage) (result *RateLimitDeny, err error)
	RenderResponseExtension(ctx *Context, out io.Writer) error
}

func (c *Context) SetRateLimiter(limiter RateLimiter) {
	c.rateLimiter = limiter
}

func (c *Context) SubgraphErrors() error {
	if len(c.subgraphErrors) == 0 {
		return nil
	}

	// Ensure the errors are appended in an idempotent order
	keys := make([]string, 0, len(c.subgraphErrors))
	for k := range c.subgraphErrors {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var joined error
	for _, k := range keys {
		joined = errors.Join(joined, c.subgraphErrors[k])
	}
	return joined
}

func (c *Context) appendSubgraphErrors(ds DataSourceInfo, errs ...error) {
	if c.subgraphErrors == nil {
		c.subgraphErrors = make(map[string]error)
	}
	c.subgraphErrors[ds.Name] = errors.Join(c.subgraphErrors[ds.Name], errors.Join(errs...))
}

// CacheStatsSnapshot is a read-only snapshot of cache statistics.
// Uses plain int64 values for easy consumption.
type CacheStatsSnapshot struct {
	L1Hits   int64
	L1Misses int64
	L2Hits   int64
	L2Misses int64
}

// GetCacheStats returns a snapshot of the cache statistics for the current request.
// This includes L1 (per-request) and L2 (external) cache hit/miss counts.
// Returns plain int64 values for easy consumption.
func (c *Context) GetCacheStats() CacheStatsSnapshot {
	return CacheStatsSnapshot{
		L1Hits:   c.cacheStats.L1Hits,
		L1Misses: c.cacheStats.L1Misses,
		L2Hits:   c.cacheStats.L2Hits.Load(),
		L2Misses: c.cacheStats.L2Misses.Load(),
	}
}

// trackL1Hit increments the L1 cache hit counter.
// Called by the loader when an entity is found in L1 cache.
func (c *Context) trackL1Hit() {
	c.cacheStats.L1Hits++
}

// trackL1Miss increments the L1 cache miss counter.
// Called by the loader when an entity is not found in L1 cache.
func (c *Context) trackL1Miss() {
	c.cacheStats.L1Misses++
}

// trackL2Hit increments the L2 cache hit counter.
// Called by the loader when an entity is found in L2 (external) cache.
// Thread-safe: uses atomic operations for parallel goroutine access.
func (c *Context) trackL2Hit() {
	c.cacheStats.L2Hits.Inc()
}

// trackL2Miss increments the L2 cache miss counter.
// Called by the loader when an entity is not found in L2 (external) cache.
// Thread-safe: uses atomic operations for parallel goroutine access.
func (c *Context) trackL2Miss() {
	c.cacheStats.L2Misses.Inc()
}

type Request struct {
	ID     uint64
	Header http.Header
}

func NewContext(ctx context.Context) *Context {
	if ctx == nil {
		panic("nil context.Context")
	}
	return &Context{
		ctx: ctx,
		cacheStats: CacheStats{
			L2Hits:   atomic.NewInt64(0),
			L2Misses: atomic.NewInt64(0),
		},
	}
}

func (c *Context) Context() context.Context {
	return c.ctx
}

func (c *Context) WithContext(ctx context.Context) *Context {
	if ctx == nil {
		panic("nil context.Context")
	}
	cpy := *c
	cpy.ctx = ctx
	return &cpy
}

func (c *Context) clone(ctx context.Context) *Context {
	cpy := *c
	cpy.ctx = ctx
	if c.Variables != nil {
		variablesData := c.Variables.MarshalTo(nil)
		cpy.Variables = astjson.MustParseBytes(variablesData)
	}
	cpy.Files = append([]*httpclient.FileUpload(nil), c.Files...)
	cpy.Request.Header = c.Request.Header.Clone()
	cpy.RenameTypeNames = append([]RenameTypeName(nil), c.RenameTypeNames...)

	if c.RemapVariables != nil {
		cpy.RemapVariables = make(map[string]string, len(c.RemapVariables))
		for k, v := range c.RemapVariables {
			cpy.RemapVariables[k] = v
		}
	}

	if c.subgraphErrors != nil {
		cpy.subgraphErrors = make(map[string]error, len(c.subgraphErrors))
		for k, v := range c.subgraphErrors {
			cpy.subgraphErrors[k] = v
		}
	}

	return &cpy
}

func (c *Context) Free() {
	c.ctx = nil
	c.Variables = nil
	c.Files = nil
	c.Request.Header = nil
	c.RenameTypeNames = nil
	c.RemapVariables = nil
	c.TracingOptions.DisableAll()
	c.Extensions = nil
	c.subgraphErrors = nil
	c.authorizer = nil
	c.LoaderHooks = nil
}

type traceStartKey struct{}

type TraceInfo struct {
	TraceStart     time.Time  `json:"-"`
	TraceStartTime string     `json:"trace_start_time"`
	TraceStartUnix int64      `json:"trace_start_unix"`
	ParseStats     PhaseStats `json:"parse_stats"`
	NormalizeStats PhaseStats `json:"normalize_stats"`
	ValidateStats  PhaseStats `json:"validate_stats"`
	PlannerStats   PhaseStats `json:"planner_stats"`
	debug          bool
}

type PhaseStats struct {
	DurationNano             int64  `json:"duration_nanoseconds"`
	DurationPretty           string `json:"duration_pretty"`
	DurationSinceStartNano   int64  `json:"duration_since_start_nanoseconds"`
	DurationSinceStartPretty string `json:"duration_since_start_pretty"`
}

type requestContextKey struct{}

func SetTraceStart(ctx context.Context, predictableDebugTimings bool) context.Context {
	info := &TraceInfo{}
	if predictableDebugTimings {
		info.debug = true
		info.TraceStart = time.UnixMilli(0)
		info.TraceStartUnix = 0
		info.TraceStartTime = ""
	} else {
		info.TraceStart = time.Now()
		info.TraceStartUnix = info.TraceStart.Unix()
		info.TraceStartTime = info.TraceStart.Format(time.RFC3339)
	}
	return context.WithValue(ctx, traceStartKey{}, info)
}

func GetDurationNanoSinceTraceStart(ctx context.Context) int64 {
	info, ok := ctx.Value(traceStartKey{}).(*TraceInfo)
	if !ok {
		return 0
	}
	if info.debug {
		return 1
	}
	return time.Since(info.TraceStart).Nanoseconds()
}

func SetDebugStats(info *TraceInfo, stats PhaseStats, phaseNo int64) PhaseStats {
	if info.debug {
		stats.DurationSinceStartNano = phaseNo * 5
		stats.DurationSinceStartPretty = time.Duration(phaseNo * 5).String()
		stats.DurationNano = 5
		stats.DurationPretty = time.Duration(5).String()
	}

	return stats
}

func GetTraceInfo(ctx context.Context) *TraceInfo {
	// The context might not have trace info, in that case we return nil
	info, _ := ctx.Value(traceStartKey{}).(*TraceInfo)
	return info
}

func SetParseStats(ctx context.Context, stats PhaseStats) {
	info := GetTraceInfo(ctx)
	if info == nil {
		return
	}
	info.ParseStats = SetDebugStats(info, stats, 1)
}

func SetNormalizeStats(ctx context.Context, stats PhaseStats) {
	info := GetTraceInfo(ctx)
	if info == nil {
		return
	}
	info.NormalizeStats = SetDebugStats(info, stats, 2)
}

func SetValidateStats(ctx context.Context, stats PhaseStats) {
	info := GetTraceInfo(ctx)
	if info == nil {
		return
	}
	info.ValidateStats = SetDebugStats(info, stats, 3)
}

func SetPlannerStats(ctx context.Context, stats PhaseStats) {
	info := GetTraceInfo(ctx)
	if info == nil {
		return
	}
	info.PlannerStats = SetDebugStats(info, stats, 4)
}

func GetRequest(ctx context.Context) *RequestData {
	// The context might not have trace info, in that case we return nil
	req, ok := ctx.Value(requestContextKey{}).(*RequestData)
	if !ok {
		return nil
	}
	return req
}

func SetRequest(ctx context.Context, r *RequestData) context.Context {
	return context.WithValue(ctx, requestContextKey{}, r)
}
