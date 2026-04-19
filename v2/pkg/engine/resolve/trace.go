package resolve

import (
	"context"
	"encoding/json"
	"net/http"
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
	Body    BodyData    `json:"body,omitempty"`
}

type TraceData struct {
	Version string              `json:"version"`
	Info    *TraceInfo          `json:"info"`
	Fetches *FetchTreeTraceNode `json:"fetches"`
	Request *RequestData        `json:"request,omitempty"`
}

// CacheTrace captures per-fetch caching behavior for trace output.
// Built AFTER mergeResult + populateCachesAfterFetch, when final cache state is known.
type CacheTrace struct {
	// Overall cache timing (aligned with DataSourceLoadTrace)
	DurationSinceStartNano   int64  `json:"duration_since_start_nanoseconds,omitempty"`
	DurationSinceStartPretty string `json:"duration_since_start_pretty,omitempty"`
	DurationNano             int64  `json:"duration_nanoseconds,omitempty"`
	DurationPretty           string `json:"duration_pretty,omitempty"`

	// Runtime state (global switches AND per-fetch config combined)
	L1Enabled  bool   `json:"l1_enabled"`
	L2Enabled  bool   `json:"l2_enabled"`
	CacheName  string `json:"cache_name,omitempty"`
	TTLSeconds int64  `json:"ttl_seconds,omitempty"`

	// Entity count — total number of entities involved in this fetch
	EntityCount int `json:"entity_count"`

	// L1 cache results
	L1Hit  int `json:"l1_hit"`
	L1Miss int `json:"l1_miss"`

	// L2 cache results
	L2Hit  int `json:"l2_hit"`
	L2Miss int `json:"l2_miss"`

	// Negative caching
	NegativeCacheHits int `json:"negative_cache_hits,omitempty"`

	// L2 operation timing (Get)
	L2GetDurationNano   int64  `json:"l2_get_duration_nanoseconds,omitempty"`
	L2GetDurationPretty string `json:"l2_get_duration_pretty,omitempty"`

	// L2 operation timing (Set — regular entries)
	L2SetDurationNano   int64  `json:"l2_set_duration_nanoseconds,omitempty"`
	L2SetDurationPretty string `json:"l2_set_duration_pretty,omitempty"`

	// L2 operation timing (Set — negative entries, separate TTL)
	L2SetNegativeDurationNano   int64  `json:"l2_set_negative_duration_nanoseconds,omitempty"`
	L2SetNegativeDurationPretty string `json:"l2_set_negative_duration_pretty,omitempty"`

	// Configuration flags that affected behavior
	PartialCacheLoad            bool `json:"partial_cache_load,omitempty"`
	ShadowMode                  bool `json:"shadow_mode,omitempty"`
	ShadowHit                   bool `json:"shadow_hit,omitempty"` // L2 had data but shadow mode forced fetch
	IncludeSubgraphHeaderPrefix bool `json:"include_subgraph_header_prefix,omitempty"`

	// Entity-level detail (only for entity/batch fetches with multiple items)
	Entities []CacheTraceEntity `json:"entities,omitempty"`

	// Cache keys (when not excluded)
	Keys []string `json:"keys,omitempty"`

	// Errors
	L2GetError         string `json:"l2_get_error,omitempty"`
	L2SetError         string `json:"l2_set_error,omitempty"`
	L2SetNegativeError string `json:"l2_set_negative_error,omitempty"`
}

// CacheTraceEntity records cache outcome for a single entity in batch fetches.
type CacheTraceEntity struct {
	Key                string  `json:"key"`                            // Cache key (or hash)
	Source             string  `json:"source"`                         // "l1", "l2", "subgraph", "negative_cache"
	ByteSize           int     `json:"byte_size,omitempty"`            // Size of cached/fetched data
	RemainingTTLSeconds float64 `json:"remaining_ttl_seconds,omitempty"` // Remaining TTL in seconds (L2 hits only, 0 = unknown)
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
