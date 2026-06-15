package resolve

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type TraceOptions struct {
	// Enable switches tracing on or off
	Enable bool
	// ExcludeParseStats excludes parse timing information from the trace output
	ExcludeParseStats bool
	// ExcludeNormalizeStats excludes normalize timing information from the trace output
	ExcludeNormalizeStats bool
	// ExcludeValidateStats excludes validation timing information from the trace output
	ExcludeValidateStats bool
	// ExcludePlannerStats excludes planner timing information from the trace output
	ExcludePlannerStats bool
	// ExcludeRawInputData excludes the raw input for a load operation from the trace output
	ExcludeRawInputData bool
	// ExcludeInput excludes the rendered input for a load operation from the trace output
	ExcludeInput bool
	// ExcludeOutput excludes the result of a load operation from the trace output
	ExcludeOutput bool
	// ExcludeLoadStats excludes the load timing information from the trace output
	ExcludeLoadStats bool
	// ExcludeCacheStats excludes cache information from the trace output
	ExcludeCacheStats bool
	// EnablePredictableDebugTimings makes the timings in the trace output predictable for debugging purposes
	EnablePredictableDebugTimings bool
	// IncludeTraceOutputInResponseExtensions includes the trace output in the response extensions
	IncludeTraceOutputInResponseExtensions bool
	// Debug makes trace IDs of fetches predictable for debugging purposes
	Debug bool
}

func (r *TraceOptions) EnableAll() {
	r.Enable = true
	r.ExcludeParseStats = false
	r.ExcludeNormalizeStats = false
	r.ExcludeValidateStats = false
	r.ExcludePlannerStats = false
	r.ExcludeRawInputData = false
	r.ExcludeInput = false
	r.ExcludeOutput = false
	r.ExcludeLoadStats = false
	r.ExcludeCacheStats = false
	r.EnablePredictableDebugTimings = false
	r.IncludeTraceOutputInResponseExtensions = true
}

func (r *TraceOptions) DisableAll() {
	r.Enable = false
	r.ExcludeParseStats = true
	r.ExcludeNormalizeStats = true
	r.ExcludeValidateStats = true
	r.ExcludePlannerStats = true
	r.ExcludeRawInputData = true
	r.ExcludeInput = true
	r.ExcludeOutput = true
	r.ExcludeLoadStats = true
	r.ExcludeCacheStats = true
	r.EnablePredictableDebugTimings = false
	r.IncludeTraceOutputInResponseExtensions = false
}

type BodyData struct {
	Query         string          `json:"query,omitempty"`
	OperationName string          `json:"operationName,omitempty"`
	Variables     json.RawMessage `json:"variables,omitempty"`
}

type RequestData struct {
	Method  string      `json:"method"`
	URL     string      `json:"url"`
	Headers http.Header `json:"headers"`
	Body    BodyData    `json:"body"`
}

type TraceData struct {
	Version string              `json:"version"`
	Info    *TraceInfo          `json:"info"`
	Fetches *FetchTreeTraceNode `json:"fetches"`
	Request *RequestData        `json:"request,omitempty"`
}

type CacheTrace struct {
	DurationSinceStartNano   int64  `json:"duration_since_start_nanoseconds,omitempty"`
	DurationSinceStartPretty string `json:"duration_since_start_pretty,omitempty"`
	DurationNano             int64  `json:"duration_nanoseconds,omitempty"`
	DurationPretty           string `json:"duration_pretty,omitempty"`

	L1Enabled  bool   `json:"l1_enabled"`
	L2Enabled  bool   `json:"l2_enabled"`
	CacheName  string `json:"cache_name,omitempty"`
	TTLSeconds int64  `json:"ttl_seconds,omitempty"`

	EntityCount       int `json:"entity_count"`
	L1Hit             int `json:"l1_hit"`
	L1Miss            int `json:"l1_miss"`
	L2Hit             int `json:"l2_hit"`
	L2Miss            int `json:"l2_miss"`
	NegativeCacheHits int `json:"negative_cache_hits,omitempty"`

	L2GetDurationNano   int64  `json:"l2_get_duration_nanoseconds,omitempty"`
	L2GetDurationPretty string `json:"l2_get_duration_pretty,omitempty"`
	L2SetDurationNano   int64  `json:"l2_set_duration_nanoseconds,omitempty"`
	L2SetDurationPretty string `json:"l2_set_duration_pretty,omitempty"`

	PartialCacheLoad            bool `json:"partial_cache_load,omitempty"`
	ShadowMode                  bool `json:"shadow_mode,omitempty"`
	IncludeSubgraphHeaderPrefix bool `json:"include_subgraph_header_prefix,omitempty"`

	Entities []CacheTraceEntity `json:"entities,omitempty"`
	Keys     []string           `json:"keys,omitempty"`

	L2GetError string `json:"l2_get_error,omitempty"`
	L2SetError string `json:"l2_set_error,omitempty"`
}

type CacheTraceEntity struct {
	Key                 string  `json:"key,omitempty"`
	Source              string  `json:"source"`
	ByteSize            int     `json:"byte_size,omitempty"`
	RemainingTTLSeconds float64 `json:"remaining_ttl_seconds,omitempty"`
}

func (l *Loader) buildCacheTrace(res *result, cfg *FetchCacheConfiguration) *CacheTrace {
	if l == nil || l.ctx == nil || !l.ctx.TracingOptions.Enable || l.ctx.TracingOptions.ExcludeCacheStats {
		return nil
	}
	if res == nil || cfg == nil || cfg.KeyTemplate == nil {
		return nil
	}
	if !l.cacheReadOrWriteEnabled(cfg) && !(l.ctx.ExecutionOptions.Caching.EnableL2Cache && l.mutationL2PopulationEnabled(cfg)) {
		return nil
	}

	l1Hits := res.cacheTraceL1Hits + res.cacheTraceRequestScopedHits
	l1Misses := res.cacheTraceL1Misses - res.cacheTraceRequestScopedHits
	if l1Misses < 0 {
		l1Misses = 0
	}

	trace := &CacheTrace{
		DurationSinceStartNano:      res.cacheTraceDurationSinceStartNano,
		DurationNano:                res.cacheTraceDurationNano,
		L1Enabled:                   cfg.UseL1Cache && l.ctx.ExecutionOptions.Caching.EnableL1Cache,
		L2Enabled:                   cfg.EnableL2Cache && l.ctx.ExecutionOptions.Caching.EnableL2Cache,
		CacheName:                   cfg.CacheName,
		TTLSeconds:                  int64(cfg.TTL.Seconds()),
		EntityCount:                 res.cacheTraceEntityCount,
		L1Hit:                       l1Hits,
		L1Miss:                      l1Misses,
		L2Hit:                       res.cacheTraceL2Hits,
		L2Miss:                      res.cacheTraceL2Misses,
		NegativeCacheHits:           res.cacheTraceNegativeHits,
		L2GetError:                  res.cacheTraceL2GetError,
		L2SetError:                  res.cacheTraceL2SetError,
		PartialCacheLoad:            cfg.EnablePartialCacheLoad,
		ShadowMode:                  cfg.ShadowMode,
		IncludeSubgraphHeaderPrefix: cfg.IncludeSubgraphHeaderPrefix,
		Entities:                    append([]CacheTraceEntity(nil), res.cacheTraceEntityDetails...),
	}
	if res.cacheTraceDurationSinceStartNano > 0 {
		trace.DurationSinceStartPretty = time.Duration(res.cacheTraceDurationSinceStartNano).String()
	}
	if res.cacheTraceDurationNano > 0 {
		trace.DurationPretty = time.Duration(res.cacheTraceDurationNano).String()
	}

	if res.cacheTraceL2GetDuration > 0 {
		trace.L2GetDurationNano = res.cacheTraceL2GetDuration.Nanoseconds()
		trace.L2GetDurationPretty = res.cacheTraceL2GetDuration.String()
	}
	if res.cacheTraceL2SetDuration > 0 {
		trace.L2SetDurationNano = res.cacheTraceL2SetDuration.Nanoseconds()
		trace.L2SetDurationPretty = res.cacheTraceL2SetDuration.String()
	}
	if !l.ctx.TracingOptions.ExcludeRawInputData {
		for _, cacheKey := range res.cacheKeys {
			trace.Keys = append(trace.Keys, cacheKey.Keys...)
		}
	}
	if l.ctx.TracingOptions.EnablePredictableDebugTimings {
		if trace.DurationSinceStartNano != 0 {
			trace.DurationSinceStartNano = 1
			trace.DurationSinceStartPretty = "1ns"
		}
		if trace.DurationNano != 0 {
			trace.DurationNano = 1
			trace.DurationPretty = "1ns"
		}
		if res.cacheTraceL2GetAttempted {
			trace.L2GetDurationNano = 1
			trace.L2GetDurationPretty = "1ns"
		}
		if res.cacheTraceL2SetAttempted {
			trace.L2SetDurationNano = 1
			trace.L2SetDurationPretty = "1ns"
		}
	}
	return trace
}

func (l *Loader) attachCacheTrace(fetch Fetch, res *result, cfg *FetchCacheConfiguration) {
	trace := l.buildCacheTrace(res, cfg)
	if trace == nil {
		return
	}
	ensureFetchTrace(fetch).CacheTrace = trace
}

func GetTrace(ctx context.Context, fetchTree *FetchTreeNode) TraceData {
	trace := TraceData{
		Version: "1",
		Info:    GetTraceInfo(ctx),
		Fetches: fetchTree.Trace(),
	}

	if req := GetRequest(ctx); req != nil {
		trace.Request = req
	}

	return trace
}
