package resolve

type FetchKind int

const (
	FetchKindSingle FetchKind = iota + 1
	FetchKindParallel
	FetchKindSerial
	FetchKindParallelListItem
	FetchKindEntity
	FetchKindEntityBatch
)

type Fetch interface {
	FetchKind() FetchKind
}

type Fetches []Fetch

type SingleFetch struct {
	FetchConfiguration
	SerialID             int
	InputTemplate        InputTemplate
	DataSourceIdentifier []byte
}

type PostProcessingConfiguration struct {
	// SelectResponseDataPath used to make a jsonparser.Get call on the response data
	SelectResponseDataPath []string
	// SelectResponseErrorsPath is similar to SelectResponseDataPath, but for errors
	// If this is set, the response will be considered an error if the jsonparser.Get call returns a non-empty value
	// The value will be expected to be a GraphQL error object
	SelectResponseErrorsPath []string
	// ResponseTemplate is processed after the SelectResponseDataPath is applied
	// It can be used to "render" the response data into a different format
	// E.g. when you're making a representations Request with two entities, you will get back an array of two objects
	// However, you might want to render this into a single object with two properties
	// This can be done with a ResponseTemplate
	ResponseTemplate *InputTemplate
	// MergePath can be defined to merge the result of the post-processing into the parent object at the given path
	// e.g. if the parent is {"a":1}, result is {"foo":"bar"} and the MergePath is ["b"],
	// the result will be {"a":1,"b":{"foo":"bar"}}
	// If the MergePath is empty, the result will be merged into the parent object
	// In this case, the result would be {"a":1,"foo":"bar"}
	// This is useful if you make multiple fetches, e.g. parallel fetches, that would otherwise overwrite each other
	MergePath []string
}

func (_ *SingleFetch) FetchKind() FetchKind {
	return FetchKindSingle
}

// ParallelFetch - TODO: document better
// should be used only for object fields which could be fetched parallel
type ParallelFetch struct {
	Fetches []Fetch
}

func (_ *ParallelFetch) FetchKind() FetchKind {
	return FetchKindParallel
}

// SerialFetch - TODO: document better
// should be used only for object fields which should be fetched serial
type SerialFetch struct {
	Fetches []Fetch
}

func (_ *SerialFetch) FetchKind() FetchKind {
	return FetchKindSerial
}

// BatchEntityFetch - TODO: document better
// allows to join nested fetches to the same subgraph into a single fetch
type BatchEntityFetch struct {
	Input                BatchInput
	DataSource           DataSource
	PostProcessing       PostProcessingConfiguration
	DataSourceIdentifier []byte
	DisallowSingleFlight bool
}

type BatchInput struct {
	Header InputTemplate
	Items  []InputTemplate
	// If SkipNullItems is set to true, items that render to null will not be included in the batch but skipped
	SkipNullItems bool
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

type EntityFetch struct {
	Input                EntityInput
	DataSource           DataSource
	PostProcessing       PostProcessingConfiguration
	DataSourceIdentifier []byte
	DisallowSingleFlight bool
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
	Fetch *SingleFetch
}

func (_ *ParallelListItemFetch) FetchKind() FetchKind {
	return FetchKindParallelListItem
}

type FetchConfiguration struct {
	Input      string
	Variables  Variables
	DataSource DataSource
	// DisallowSingleFlight is used for write operations like mutations, POST, DELETE etc. to disable singleFlight
	// By default SingleFlight for fetches is disabled and needs to be enabled on the Resolver first
	// If the resolver allows SingleFlight it's up to each individual DataSource Planner to decide whether an Operation
	// should be allowed to use SingleFlight
	DisallowSingleFlight bool
	// RequiresSerialFetch is used to indicate that the single fetches should be executed serially
	// When we have multiple fetches attached to the object - after post-processing of a plan we will get SerialFetch instead of ParallelFetch
	RequiresSerialFetch bool
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
}
