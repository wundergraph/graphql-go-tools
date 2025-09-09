package resolve

import (
	"encoding/json"
	"slices"
	"strings"

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
	Dependencies() *FetchDependencies

	// FetchInfo returns additional fetch-related information.
	// Callers must treat FetchInfo as read-only after planning; it may be nil when disabled by planner options.
	FetchInfo() *FetchInfo
}

type FetchItem struct {
	Fetch                Fetch
	FetchPath            []FetchItemPathElement
	ResponsePath         string
	ResponsePathElements []string
}

func FetchItemWithPath(fetch Fetch, responsePath string, path ...FetchItemPathElement) *FetchItem {
	item := &FetchItem{
		Fetch:        fetch,
		FetchPath:    path,
		ResponsePath: responsePath,
	}
	if responsePath != "" {
		item.ResponsePathElements = strings.Split(responsePath, ".")
	}
	return item
}

// EqualSingleFetch compares two FetchItem for equality, both items should be of kind FetchKindSingle
func (f *FetchItem) EqualSingleFetch(other *FetchItem) bool {
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
	Kind      FetchItemPathElementKind
	Path      []string
	TypeNames []string
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

func (s *SingleFetch) Dependencies() *FetchDependencies {
	return &s.FetchDependencies
}

func (s *SingleFetch) FetchInfo() *FetchInfo {
	return s.Info
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
	// This is useful if we make multiple fetches, e.g. parallel fetches, that would otherwise overwrite each other
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

func (*SingleFetch) FetchKind() FetchKind {
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

func (b *BatchEntityFetch) Dependencies() *FetchDependencies {
	return &b.FetchDependencies
}

func (b *BatchEntityFetch) FetchInfo() *FetchInfo {
	return b.Info
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

func (*BatchEntityFetch) FetchKind() FetchKind {
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

func (e *EntityFetch) Dependencies() *FetchDependencies {
	return &e.FetchDependencies
}

func (e *EntityFetch) FetchInfo() *FetchInfo {
	return e.Info
}

type EntityInput struct {
	Header      InputTemplate
	Item        InputTemplate
	SkipErrItem bool
	Footer      InputTemplate
}

func (*EntityFetch) FetchKind() FetchKind {
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

func (p *ParallelListItemFetch) Dependencies() *FetchDependencies {
	return &p.Fetch.FetchDependencies
}

func (p *ParallelListItemFetch) FetchInfo() *FetchInfo {
	return p.Fetch.Info
}

func (*ParallelListItemFetch) FetchKind() FetchKind {
	return FetchKindParallelListItem
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

	// RequiresParallelListItemFetch indicates that the single fetches should be executed without batching.
	// If we have multiple fetches attached to the object, then after post-processing of a plan
	// we will get ParallelListItemFetch instead of ParallelFetch.
	// Happens only for objects under the array path and used only for the introspection.
	RequiresParallelListItemFetch bool

	// RequiresEntityFetch will be set to true if the fetch is an entity fetch on an object.
	// After post-processing, we will get EntityFetch.
	RequiresEntityFetch bool

	// RequiresEntityBatchFetch indicates that entity fetches on array items should be batched.
	// After post-processing, we will get EntityBatchFetch.
	RequiresEntityBatchFetch bool

	// PostProcessing specifies the data and error extraction path in the response along with
	// the merge path where will insert the response.
	PostProcessing PostProcessingConfiguration

	// SetTemplateOutputToNullOnVariableNull will safely return "null" if one of the template variables renders to null
	// This is the case, e.g. when using batching and one sibling is null, resulting in a null value for one batch item
	// Returning null in this case tells the batch implementation to skip this item
	SetTemplateOutputToNullOnVariableNull bool

	QueryPlan *QueryPlan

	// OperationName is non-empty when the operation name is propagated to the upstream subgraph fetch.
	OperationName string
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

	// Note: we do not compare datasources, as they will always be a different instance.

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

// FetchDependency explains how a GraphCoordinate depends on other GraphCoordinates from other fetches
type FetchDependency struct {
	// Coordinate is the type+field which depends on one or more FetchDependencyOrigin
	Coordinate GraphCoordinate `json:"coordinate"`
	// IsUserRequested is true if the field was requested by the user/client
	// If false, this indicates that the Coordinate is a dependency for another fetch
	IsUserRequested bool `json:"isUserRequested"`
	// DependsOn are the FetchDependencyOrigins the Coordinate depends on
	DependsOn []FetchDependencyOrigin `json:"dependsOn"`
}

// FetchDependencyOrigin defines a GraphCoordinate on a FetchID that another Coordinate depends on
// In addition, it contains information on the Subgraph providing the field,
// and if the Coordinate is a @key or a @requires field dependency
type FetchDependencyOrigin struct {
	// FetchID is the fetch id providing the Coordinate
	FetchID int `json:"fetchId"`
	// Subgraph is the subgraph providing the Coordinate
	Subgraph string `json:"subgraph"`
	// Coordinate is the GraphCoordinate that another Coordinate depends on
	Coordinate GraphCoordinate `json:"coordinate"`
	// IsKey is true if the Coordinate is a @key dependency
	IsKey bool `json:"isKey"`
	// IsRequires is true if the Coordinate is a @requires dependency
	IsRequires bool `json:"isRequires"`
}

// FetchReason explains who requested a specific (typeName, fieldName) combination.
// A field can be requested by the user and/or by one or more subgraphs, with optional reasons.
type FetchReason struct {
	TypeName    string   `json:"typename"`
	FieldName   string   `json:"field"`
	BySubgraphs []string `json:"by_subgraphs,omitempty"`
	ByUser      bool     `json:"by_user,omitempty"`
	IsKey       bool     `json:"is_key,omitempty"`
	IsRequires  bool     `json:"is_requires,omitempty"`
}

// FetchInfo contains additional (derived) information about the fetch.
// Some fields may not be generated depending on planner flags.
type FetchInfo struct {
	DataSourceID   string
	DataSourceName string
	RootFields     []GraphCoordinate
	OperationType  ast.OperationType
	QueryPlan      *QueryPlan

	// CoordinateDependencies contain a list of GraphCoordinates (typeName+fieldName)
	// and which fields from other fetches they depend on.
	// This information is useful to understand why a fetch depends on other fetches,
	// and how multiple dependencies lead to a chain of fetches
	CoordinateDependencies []FetchDependency

	// FetchReasons contains provenance for reasons why particular fields were fetched.
	// If this structure is built, then all the fields are processed.
	FetchReasons []FetchReason

	// PropagatedFetchReasons holds those FetchReasons that will be propagated
	// with the request to the subgraph as part of the "fetch_reason" extension.
	PropagatedFetchReasons []FetchReason
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

// Compile-time interface assertions to catch regressions.
var (
	_ Fetch = (*SingleFetch)(nil)
	_ Fetch = (*BatchEntityFetch)(nil)
	_ Fetch = (*EntityFetch)(nil)
	_ Fetch = (*ParallelListItemFetch)(nil)
)
