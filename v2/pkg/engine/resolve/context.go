package resolve

import (
	"context"
	"net/http"
	"time"
)

type Context struct {
	ctx                   context.Context
	Variables             []byte
	Request               Request
	RenameTypeNames       []RenameTypeName
	RequestTracingOptions RequestTraceOptions
	Extensions            []byte
}

type Request struct {
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

func (c *Context) clone() Context {
	cpy := *c
	cpy.ctx = context.Background()
	cpy.Variables = append([]byte(nil), c.Variables...)
	cpy.Request.Header = c.Request.Header.Clone()
	cpy.RenameTypeNames = append([]RenameTypeName(nil), c.RenameTypeNames...)
	return cpy
}

func (c *Context) Free() {
	c.ctx = nil
	c.Variables = nil
	c.Request.Header = nil
	c.RenameTypeNames = nil
	c.RequestTracingOptions.DisableAll()
}

type traceStartKey struct{}

type TraceInfo struct {
	TraceStart     time.Time    `json:"-"`
	TraceStartTime string       `json:"trace_start_time"`
	TraceStartUnix int64        `json:"trace_start_unix"`
	PlannerStats   PlannerStats `json:"planner_stats"`
	debug          bool
}

type PlannerStats struct {
	PlanningTimeNano         int64  `json:"planning_time_nanoseconds"`
	PlanningTimePretty       string `json:"planning_time_pretty"`
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

func SetPlannerStats(ctx context.Context, stats PlannerStats) {
	info, ok := ctx.Value(traceStartKey{}).(*TraceInfo)
	if !ok {
		return
	}
	if info.debug {
		stats.DurationSinceStartNano = 5
		stats.DurationSinceStartPretty = time.Duration(5).String()
		stats.PlanningTimeNano = 5
		stats.PlanningTimePretty = time.Duration(5).String()
	}
	info.PlannerStats = stats
}

func GetTraceInfo(ctx context.Context) *TraceInfo {
	return ctx.Value(traceStartKey{}).(*TraceInfo)
}
