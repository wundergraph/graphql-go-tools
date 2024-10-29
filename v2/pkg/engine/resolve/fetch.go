package resolve

import (
	"encoding/json"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type FetchKind int

const (
	FetchKindSingle FetchKind = iota + 1
	FetchKindParallelListItem
	FetchKindEntity
	FetchKindEntityBatch
)

type Fetch interface {
	FetchKind() FetchKind
	Dependencies() FetchDependencies
	DataSourceInfo() DataSourceInfo
}

type FetchItem struct {
	Fetch                Fetch
	FetchPath            []FetchItemPathElement
	ResponsePath         string
	ResponsePathElements []string
}

func (f *FetchItem) Equals(other *FetchItem) bool {
	if len(f.FetchPath) != len(other.FetchPath) {
		return false
	}

	for i := range f.FetchPath {
		if f.FetchPath[i].Kind != other.FetchPath[i].Kind {
			return false
		}

		if !slices.Equal(f.FetchPath[i].Path, other.FetchPath[i].Path) {
			return false
		}
	}

	if f.Fetch.FetchKind() != FetchKindSingle || other.Fetch.FetchKind() != FetchKindSingle {
		return false
	}
	l, ok := f.Fetch.(*SingleFetch)
	if !ok {
		return false
	}
	r, ok := other.Fetch.(*SingleFetch)
	if !ok {
		return false
	}
	return l.FetchConfiguration.Equals(&r.FetchConfiguration)
}

type FetchItemPathElement struct {
	Kind FetchItemPathElementKind
	Path []string
}

type FetchItemPathElementKind string

const (
	FetchItemPathElementKindObject FetchItemPathElementKind = "object"
	FetchItemPathElementKindArray  FetchItemPathElementKind = "array"
)

type SingleFetch struct {
	FetchConfiguration
	FetchDependencies
	InputTemplate        InputTemplate
	DataSourceIdentifier []byte
	Trace                *DataSourceLoadTrace
	Info                 *FetchInfo
}

func (s *SingleFetch) Dependencies() FetchDependencies {
	return s.FetchDependencies
}

func (s *SingleFetch) DataSourceInfo() DataSourceInfo {
	return DataSourceInfo{
		ID:   s.Info.DataSourceID,
		Name: s.Info.DataSourceName,
	}
}

// FetchDependencies holding current fetch id and ids of fetches that current fetch depends on
// e.g. should be fetched only after all dependent fetches are fetched
type FetchDependencies struct {
	FetchID           int
	DependsOnFetchIDs []int
}

type PostProcessingConfiguration struct {
	// SelectResponseDataPath used to make a jsonparser.Get call on the response data
	SelectResponseDataPath []string
	// SelectResponseErrorsPath is similar to SelectResponseDataPath, but for errors
	// If this is set, the response will be considered an error if the jsonparser.Get call returns a non-empty value
	// The value will be expected to be a GraphQL error object
	SelectResponseErrorsPath []string
	// MergePath can be defined to merge the result of the post-processing into the parent object at the given path
	// e.g. if the parent is {"a":1}, result is {"foo":"bar"} and the MergePath is ["b"],
	// the result will be {"a":1,"b":{"foo":"bar"}}
	// If the MergePath is empty, the result will be merged into the parent object
	// In this case, the result would be {"a":1,"foo":"bar"}
	// This is useful if you make multiple fetches, e.g. parallel fetches, that would otherwise overwrite each other
	MergePath []string
}

// Equals compares two PostProcessingConfiguration objects
func (ppc *PostProcessingConfiguration) Equals(other *PostProcessingConfiguration) bool {
	if !slices.Equal(ppc.SelectResponseDataPath, other.SelectResponseDataPath) {
		return false
	}

	if !slices.Equal(ppc.SelectResponseErrorsPath, other.SelectResponseErrorsPath) {
		return false
	}

	// Response template is unused in the current codebase - so we can ignore it

	if !slices.Equal(ppc.MergePath, other.MergePath) {
		return false
	}

	return true
}

func (_ *SingleFetch) FetchKind() FetchKind {
	return FetchKindSingle
}

// BatchEntityFetch - represents nested entity fetch on array field
// allows to join nested fetches to the same subgraph into a single fetch
// representations variable will contain multiple items according to amount of entities matching this query
type BatchEntityFetch struct {
	FetchDependencies
	Input                BatchInput
	DataSource           DataSource
	PostProcessing       PostProcessingConfiguration
	DataSourceIdentifier []byte
	Trace                *DataSourceLoadTrace
	Info                 *FetchInfo
}

func (b *BatchEntityFetch) Dependencies() FetchDependencies {
	return b.FetchDependencies
}

func (b *BatchEntityFetch) DataSourceInfo() DataSourceInfo {
	return DataSourceInfo{
		ID:   b.Info.DataSourceID,
		Name: b.Info.DataSourceName,
	}
}

type BatchInput struct {
	Header InputTemplate
	Items  []InputTemplate
	// If SkipNullItems is set to true, items that render to null will not be included in the batch but skipped
	SkipNullItems bool
	// Same as SkipNullItems but for empty objects
	SkipEmptyObjectItems bool
	// If SkipErrItems is set to true, items that return an error during rendering will not be included in the batch but skipped
	// In this case, the error will be swallowed
	// E.g. if a field is not nullable and the value is null, the item will be skipped
	SkipErrItems bool
	Separator    InputTemplate
	Footer       InputTemplate
}

func (_ *BatchEntityFetch) FetchKind() FetchKind {
	return FetchKindEntityBatch
}

// EntityFetch - represents nested entity fetch on object field
// representations variable will contain single item
type EntityFetch struct {
	FetchDependencies
	Input                EntityInput
	DataSource           DataSource
	PostProcessing       PostProcessingConfiguration
	DataSourceIdentifier []byte
	Trace                *DataSourceLoadTrace
	Info                 *FetchInfo
}

func (e *EntityFetch) Dependencies() FetchDependencies {
	return e.FetchDependencies
}

func (e *EntityFetch) DataSourceInfo() DataSourceInfo {
	return DataSourceInfo{
		ID:   e.Info.DataSourceID,
		Name: e.Info.DataSourceName,
	}
}

type EntityInput struct {
	Header      InputTemplate
	Item        InputTemplate
	SkipErrItem bool
	Footer      InputTemplate
}

func (_ *EntityFetch) FetchKind() FetchKind {
	return FetchKindEntity
}

// The ParallelListItemFetch can be used to make nested parallel fetches within a list
// Usually, you want to batch fetches within a list, which is the default behavior of SingleFetch
// However, if the data source does not support batching, you can use this fetch to make parallel fetches within a list
type ParallelListItemFetch struct {
	Fetch  *SingleFetch
	Traces []*SingleFetch
	Trace  *DataSourceLoadTrace
}

func (p *ParallelListItemFetch) Dependencies() FetchDependencies {
	return p.Fetch.FetchDependencies
}

func (_ *ParallelListItemFetch) FetchKind() FetchKind {
	return FetchKindParallelListItem
}

func (p *ParallelListItemFetch) DataSourceInfo() DataSourceInfo {
	return p.Fetch.DataSourceInfo()
}

type QueryPlan struct {
	DependsOnFields []Representation
	Query           string
}

type Representation struct {
	Kind      RepresentationKind `json:"kind"`
	TypeName  string             `json:"typeName"`
	FieldName string             `json:"fieldName,omitempty"`
	Fragment  string             `json:"fragment"`
}

type RepresentationKind string

const (
	RepresentationKindKey      RepresentationKind = "@key"
	RepresentationKindRequires RepresentationKind = "@requires"
)

type FetchConfiguration struct {
	Input      string
	Variables  Variables
	DataSource DataSource
	// RequiresParallelListItemFetch is used to indicate that the single fetches should be executed without batching
	// When we have multiple fetches attached to the object - after post-processing of a plan we will get ParallelListItemFetch instead of ParallelFetch
	RequiresParallelListItemFetch bool
	// RequiresEntityFetch will be set to true if the fetch is an entity fetch on an object. After post-processing, we will get EntityFetch
	RequiresEntityFetch bool
	// RequiresEntityBatchFetch indicates that entity fetches on array items could be batched. After post-processing, we will get EntityBatchFetch
	RequiresEntityBatchFetch bool
	PostProcessing           PostProcessingConfiguration
	// SetTemplateOutputToNullOnVariableNull will safely return "null" if one of the template variables renders to null
	// This is the case, e.g. when using batching and one sibling is null, resulting in a null value for one batch item
	// Returning null in this case tells the batch implementation to skip this item
	SetTemplateOutputToNullOnVariableNull bool
	QueryPlan                             *QueryPlan
}

func (fc *FetchConfiguration) Equals(other *FetchConfiguration) bool {
	if fc.Input != other.Input {
		return false
	}
	if !slices.EqualFunc(fc.Variables, other.Variables, func(a, b Variable) bool {
		return a.Equals(b)
	}) {
		return false
	}

	// Note: we do not compare datasources, as they will always be a different instance

	if fc.RequiresParallelListItemFetch != other.RequiresParallelListItemFetch {
		return false
	}
	if fc.RequiresEntityFetch != other.RequiresEntityFetch {
		return false
	}
	if fc.RequiresEntityBatchFetch != other.RequiresEntityBatchFetch {
		return false
	}
	if !fc.PostProcessing.Equals(&other.PostProcessing) {
		return false
	}
	if fc.SetTemplateOutputToNullOnVariableNull != other.SetTemplateOutputToNullOnVariableNull {
		return false
	}

	return true
}

type FetchInfo struct {
	DataSourceID   string
	DataSourceName string
	RootFields     []GraphCoordinate
	OperationType  ast.OperationType
	QueryPlan      *QueryPlan
}

type GraphCoordinate struct {
	TypeName             string `json:"typeName"`
	FieldName            string `json:"fieldName"`
	HasAuthorizationRule bool   `json:"-"`
}

type DataSourceLoadTrace struct {
	RawInputData               json.RawMessage `json:"raw_input_data,omitempty"`
	Input                      json.RawMessage `json:"input,omitempty"`
	Output                     json.RawMessage `json:"output,omitempty"`
	LoadError                  string          `json:"error,omitempty"`
	DurationSinceStartNano     int64           `json:"duration_since_start_nanoseconds,omitempty"`
	DurationSinceStartPretty   string          `json:"duration_since_start_pretty,omitempty"`
	DurationLoadNano           int64           `json:"duration_load_nanoseconds,omitempty"`
	DurationLoadPretty         string          `json:"duration_load_pretty,omitempty"`
	SingleFlightUsed           bool            `json:"single_flight_used"`
	SingleFlightSharedResponse bool            `json:"single_flight_shared_response"`
	LoadSkipped                bool            `json:"load_skipped"`
	LoadStats                  *LoadStats      `json:"load_stats,omitempty"`
	Path                       string          `json:"-"`
}

type LoadStats struct {
	GetConn              GetConnStats              `json:"get_conn"`
	GotConn              GotConnStats              `json:"got_conn"`
	GotFirstResponseByte GotFirstResponseByteStats `json:"got_first_response_byte"`
	DNSStart             DNSStartStats             `json:"dns_start"`
	DNSDone              DNSDoneStats              `json:"dns_done"`
	ConnectStart         ConnectStartStats         `json:"connect_start"`
	ConnectDone          ConnectDoneStats          `json:"connect_done"`
	TLSHandshakeStart    TLSHandshakeStartStats    `json:"tls_handshake_start"`
	TLSHandshakeDone     TLSHandshakeDoneStats     `json:"tls_handshake_done"`
	WroteHeaders         WroteHeadersStats         `json:"wrote_headers"`
	WroteRequest         WroteRequestStats         `json:"wrote_request"`
}

type GetConnStats struct {
	DurationSinceStartNano   int64  `json:"duration_since_start_nanoseconds"`
	DurationSinceStartPretty string `json:"duration_since_start_pretty"`
	HostPort                 string `json:"host_port"`
}

type GotConnStats struct {
	DurationSinceStartNano   int64  `json:"duration_since_start_nanoseconds"`
	DurationSinceStartPretty string `json:"duration_since_start_pretty"`
	Reused                   bool   `json:"reused"`
	WasIdle                  bool   `json:"was_idle"`
	IdleTimeNano             int64  `json:"idle_time_nanoseconds"`
	IdleTimePretty           string `json:"idle_time_pretty"`
}

type GotFirstResponseByteStats struct {
	DurationSinceStartNano   int64  `json:"duration_since_start_nanoseconds"`
	DurationSinceStartPretty string `json:"duration_since_start_pretty"`
}

type DNSStartStats struct {
	DurationSinceStartNano   int64  `json:"duration_since_start_nanoseconds"`
	DurationSinceStartPretty string `json:"duration_since_start_pretty"`
	Host                     string `json:"host"`
}

type DNSDoneStats struct {
	DurationSinceStartNano   int64  `json:"duration_since_start_nanoseconds"`
	DurationSinceStartPretty string `json:"duration_since_start_pretty"`
}

type ConnectStartStats struct {
	DurationSinceStartNano   int64  `json:"duration_since_start_nanoseconds"`
	DurationSinceStartPretty string `json:"duration_since_start_pretty"`
	Network                  string `json:"network"`
	Addr                     string `json:"addr"`
}

type ConnectDoneStats struct {
	DurationSinceStartNano   int64  `json:"duration_since_start_nanoseconds"`
	DurationSinceStartPretty string `json:"duration_since_start_pretty"`
	Network                  string `json:"network"`
	Addr                     string `json:"addr"`
	Err                      string `json:"err,omitempty"`
}

type TLSHandshakeStartStats struct {
	DurationSinceStartNano   int64  `json:"duration_since_start_nanoseconds"`
	DurationSinceStartPretty string `json:"duration_since_start_pretty"`
}

type TLSHandshakeDoneStats struct {
	DurationSinceStartNano   int64  `json:"duration_since_start_nanoseconds"`
	DurationSinceStartPretty string `json:"duration_since_start_pretty"`
	Err                      string `json:"err,omitempty"`
}

type WroteHeadersStats struct {
	DurationSinceStartNano   int64  `json:"duration_since_start_nanoseconds"`
	DurationSinceStartPretty string `json:"duration_since_start_pretty"`
}

type WroteRequestStats struct {
	DurationSinceStartNano   int64  `json:"duration_since_start_nanoseconds"`
	DurationSinceStartPretty string `json:"duration_since_start_pretty"`
	Err                      string `json:"err,omitempty"`
}
