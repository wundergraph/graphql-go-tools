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
	r.EnablePredictableDebugTimings = false
	r.IncludeTraceOutputInResponseExtensions = false
}

type RequestData struct {
	Method  string          `json:"method"`
	URL     string          `json:"url"`
	Headers http.Header     `json:"headers"`
	Body    json.RawMessage `json:"body,omitempty"`
}

type TraceData struct {
	Version string              `json:"version"`
	Info    *TraceInfo          `json:"info"`
	Fetches *FetchTreeTraceNode `json:"fetches"`
	Request *RequestData        `json:"request,omitempty"`
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
