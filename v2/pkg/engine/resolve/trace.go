package resolve

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
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

type TraceFetchType string

const (
	TraceFetchTypeSingle           TraceFetchType = "single"
	TraceFetchTypeParallel         TraceFetchType = "parallel"
	TraceFetchTypeSerial           TraceFetchType = "serial"
	TraceFetchTypeParallelListItem TraceFetchType = "parallelListItem"
	TraceFetchTypeEntity           TraceFetchType = "entity"
	TraceFetchTypeBatchEntity      TraceFetchType = "batchEntity"
)

type TraceNodeType string

const (
	TraceNodeTypeObject      TraceNodeType = "object"
	TraceNodeTypeEmptyObject TraceNodeType = "emptyObject"
	TraceNodeTypeArray       TraceNodeType = "array"
	TraceNodeTypeEmptyArray  TraceNodeType = "emptyArray"
	TraceNodeTypeNull        TraceNodeType = "null"
	TraceNodeTypeString      TraceNodeType = "string"
	TraceNodeTypeBoolean     TraceNodeType = "boolean"
	TraceNodeTypeInteger     TraceNodeType = "integer"
	TraceNodeTypeFloat       TraceNodeType = "float"
	TraceNodeTypeBigInt      TraceNodeType = "bigint"
	TraceNodeTypeCustom      TraceNodeType = "custom"
	TraceNodeTypeScalar      TraceNodeType = "scalar"
	TraceNodeTypeUnknown     TraceNodeType = "unknown"
)

type TraceFetch struct {
	Id                   string                 `json:"id,omitempty"`
	Type                 TraceFetchType         `json:"type,omitempty"`
	Path                 string                 `json:"path,omitempty"`
	DataSourceID         string                 `json:"data_source_id,omitempty"`
	Fetches              []*TraceFetch          `json:"fetches,omitempty"`
	DataSourceLoadTrace  *DataSourceLoadTrace   `json:"datasource_load_trace,omitempty"`
	DataSourceLoadTraces []*DataSourceLoadTrace `json:"data_source_load_traces,omitempty"`
}

type TraceFetchEvents struct {
	InputBeforeSourceLoad json.RawMessage `json:"input_before_source_load,omitempty"`
}

type TraceField struct {
	Name            string     `json:"name,omitempty"`
	Value           *TraceNode `json:"value,omitempty"`
	ParentTypeNames []string   `json:"parent_type_names,omitempty"`
	NamedType       string     `json:"named_type,omitempty"`
	DataSourceIDs   []string   `json:"data_source_ids,omitempty"`
}

type TraceNode struct {
	Info                 *TraceInfo    `json:"info,omitempty"`
	Fetch                *TraceFetch   `json:"fetch,omitempty"`
	NodeType             TraceNodeType `json:"node_type,omitempty"`
	Path                 []string      `json:"path,omitempty"`
	Fields               []*TraceField `json:"fields,omitempty"`
	Items                []*TraceNode  `json:"items,omitempty"`
	UnescapeResponseJson bool          `json:"unescape_response_json,omitempty"`
	IsTypeName           bool          `json:"is_type_name,omitempty"`
}

func getNodeType(kind NodeKind) TraceNodeType {
	switch kind {
	case NodeKindObject:
		return TraceNodeTypeObject
	case NodeKindEmptyObject:
		return TraceNodeTypeEmptyObject
	case NodeKindArray:
		return TraceNodeTypeArray
	case NodeKindEmptyArray:
		return TraceNodeTypeEmptyArray
	case NodeKindNull:
		return TraceNodeTypeNull
	case NodeKindString:
		return TraceNodeTypeString
	case NodeKindBoolean:
		return TraceNodeTypeBoolean
	case NodeKindInteger:
		return TraceNodeTypeInteger
	case NodeKindFloat:
		return TraceNodeTypeFloat
	case NodeKindBigInt:
		return TraceNodeTypeBigInt
	case NodeKindCustom:
		return TraceNodeTypeCustom
	case NodeKindScalar:
		return TraceNodeTypeScalar
	default:
		return TraceNodeTypeUnknown
	}
}

func parseField(f *Field, options *getTraceOptions) *TraceField {
	if f == nil {
		return nil
	}

	field := &TraceField{
		Name:  string(f.Name),
		Value: parseNode(f.Value, options),
	}

	if f.Info == nil {
		return field
	}

	field.ParentTypeNames = f.Info.ParentTypeNames
	field.NamedType = f.Info.NamedType
	field.DataSourceIDs = f.Info.Source.IDs

	return field
}

func parseFetch(fetch Fetch, options *getTraceOptions) *TraceFetch {
	traceFetch := &TraceFetch{}
	if options.debug {
		traceFetch.Id = "00000000-0000-0000-0000-000000000000"
	} else {
		traceFetch.Id = uuid.New().String()
	}

	switch f := fetch.(type) {
	case *SingleFetch:
		traceFetch.Type = TraceFetchTypeSingle
		if f.Trace != nil {
			traceFetch.DataSourceLoadTrace = f.Trace
			traceFetch.Path = f.Trace.Path
		}
		if f.Info != nil {
			traceFetch.DataSourceID = f.Info.DataSourceID
		}

	case *ParallelFetch:
		traceFetch.Type = TraceFetchTypeParallel
		if f.Trace != nil {
			traceFetch.Path = f.Trace.Path
		}
		for _, subFetch := range f.Fetches {
			traceFetch.Fetches = append(traceFetch.Fetches, parseFetch(subFetch, options))
		}

	case *SerialFetch:
		traceFetch.Type = TraceFetchTypeSerial
		if f.Trace != nil {
			traceFetch.Path = f.Trace.Path
		}
		for _, subFetch := range f.Fetches {
			traceFetch.Fetches = append(traceFetch.Fetches, parseFetch(subFetch, options))
		}

	case *ParallelListItemFetch:
		traceFetch.Type = TraceFetchTypeParallelListItem
		if f.Trace != nil {
			traceFetch.Path = f.Trace.Path
		}
		traceFetch.Fetches = append(traceFetch.Fetches, parseFetch(f.Fetch, options))
		if f.Traces != nil {
			for _, trace := range f.Traces {
				if trace.Trace != nil {
					traceFetch.DataSourceLoadTraces = append(traceFetch.DataSourceLoadTraces, trace.Trace)
				}
				if trace.Info != nil {
					traceFetch.DataSourceID = trace.Info.DataSourceID
				}
			}
		}
	case *EntityFetch:
		traceFetch.Type = TraceFetchTypeEntity
		if f.Trace != nil {
			traceFetch.DataSourceLoadTrace = f.Trace
			traceFetch.Path = f.Trace.Path
		}
		if f.Info != nil {
			traceFetch.DataSourceID = f.Info.DataSourceID
		}

	case *BatchEntityFetch:
		traceFetch.Type = TraceFetchTypeBatchEntity
		if f.Trace != nil {
			traceFetch.DataSourceLoadTrace = f.Trace
			traceFetch.Path = f.Trace.Path
		}
		if f.Info != nil {
			traceFetch.DataSourceID = f.Info.DataSourceID
		}

	default:
		return nil
	}

	return traceFetch
}

func parseNode(n Node, options *getTraceOptions) *TraceNode {
	node := &TraceNode{
		NodeType: getNodeType(n.NodeKind()),
		Path:     n.NodePath(),
	}

	switch v := n.(type) {
	case *Object:
		node.Fields = make([]*TraceField, 0, len(v.Fields))
		for _, field := range v.Fields {
			node.Fields = append(node.Fields, parseField(field, options))
		}
		//node.Fetch = parseFetch(v.Fetch, options)

	case *Array:
		node.Items = append(node.Items, parseNode(v.Item, options))
	case *String:
		node.UnescapeResponseJson = v.UnescapeResponseJson
		node.IsTypeName = v.IsTypeName
	}

	return node
}

type getTraceOptions struct {
	debug bool
}

type GetTraceOption func(*getTraceOptions)

func GetTraceDebug() GetTraceOption {
	return func(o *getTraceOptions) {
		o.debug = true
	}
}

func GetTrace(ctx context.Context, root *Object, opts ...GetTraceOption) *TraceNode {
	options := &getTraceOptions{}
	for i := range opts {
		opts[i](options)
	}
	node := parseNode(root, options)
	node.Info = GetTraceInfo(ctx)
	return node
}
