package resolve

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"go.uber.org/atomic"
)

type Context struct {
	ctx              context.Context
	Variables        []byte
	Files            []httpclient.File
	Request          Request
	RenameTypeNames  []RenameTypeName
	TracingOptions   TraceOptions
	RateLimitOptions RateLimitOptions
	ExecutionOptions ExecutionOptions
	InitialPayload   []byte
	Extensions       []byte
	Stats            Stats
	LoaderHooks      LoaderHooks

	authorizer  Authorizer
	rateLimiter RateLimiter

	subgraphErrors error
}

type ExecutionOptions struct {
	SkipLoader                 bool
	IncludeQueryPlanInResponse bool
	SendHeartbeat              bool
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
	return c.subgraphErrors
}

func (c *Context) appendSubgraphError(err error) {
	c.subgraphErrors = errors.Join(c.subgraphErrors, err)
}

type Stats struct {
	NumberOfFetches      atomic.Int32
	CombinedResponseSize atomic.Int64
	ResolvedNodes        int
	ResolvedObjects      int
	ResolvedLeafs        int
}

func (s *Stats) Reset() {
	s.NumberOfFetches.Store(0)
	s.CombinedResponseSize.Store(0)
	s.ResolvedNodes = 0
	s.ResolvedObjects = 0
	s.ResolvedLeafs = 0
}

type Request struct {
	ID     string
	Header http.Header
}

func NewContext(ctx context.Context) *Context {
	if ctx == nil {
		panic("nil context.Context")
	}
	return &Context{
		ctx: ctx,
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
	cpy.Variables = append([]byte(nil), c.Variables...)
	cpy.Files = append([]httpclient.File(nil), c.Files...)
	cpy.Request.Header = c.Request.Header.Clone()
	cpy.RenameTypeNames = append([]RenameTypeName(nil), c.RenameTypeNames...)
	return &cpy
}

func (c *Context) Free() {
	c.ctx = nil
	c.Variables = nil
	c.Files = nil
	c.Request.Header = nil
	c.RenameTypeNames = nil
	c.TracingOptions.DisableAll()
	c.Extensions = nil
	c.Stats.Reset()
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
