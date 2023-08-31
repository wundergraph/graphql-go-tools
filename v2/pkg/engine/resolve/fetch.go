package resolve

type FetchKind int

const (
	FetchKindSingle FetchKind = iota + 1
	FetchKindParallel
	FetchKindBatch
	FetchKindSerial
	FetchKindParallelListItem
)

type Fetch interface {
	FetchKind() FetchKind
}

type Fetches []Fetch

type SingleFetch struct {
	BufferId   int
	Input      string
	DataSource DataSource
	Variables  Variables
	// DisallowSingleFlight is used for write operations like mutations, POST, DELETE etc. to disable singleFlight
	// By default SingleFlight for fetches is disabled and needs to be enabled on the Resolver first
	// If the resolver allows SingleFlight it's up to each individual DataSource Planner to decide whether an Operation
	// should be allowed to use SingleFlight
	DisallowSingleFlight   bool
	DisableDataLoader      bool
	DissallowParallelFetch bool
	InputTemplate          InputTemplate
	DataSourceIdentifier   []byte
	// SetTemplateOutputToNullOnVariableNull will safely return "null" if one of the template variables renders to null
	// This is the case, e.g. when using batching and one sibling is null, resulting in a null value for one batch item
	// Returning null in this case tells the batch implementation to skip this item
	SetTemplateOutputToNullOnVariableNull bool
	PostProcessing                        PostProcessingConfiguration
}

type PostProcessingConfiguration struct {
	// SelectResponseDataPath used to make a jsonparser.Get call on the response data
	SelectResponseDataPath []string
	// SelectResponseErrorsPath is similar to SelectResponseDataPath, but for errors
	// If this is set, the response will be considered an error if the jsonparser.Get call returns a non-empty value
	// The value will be expected to be a GraphQL error object
	SelectResponseErrorsPath []string
	ResponseTemplate         *InputTemplate
	// ResponseTemplate is processed after the SelectResponseDataPath is applied
	// It can be used to "render" the response data into a different format
	// E.g. when you're making a representations Request with two entities, you will get back an array of two objects
	// However, you might want to render this into a single object with two properties
	// This can be done with a ResponseTemplate
}

func (_ *SingleFetch) FetchKind() FetchKind {
	return FetchKindSingle
}

type ParallelFetch struct {
	Fetches []Fetch
}

func (_ *ParallelFetch) FetchKind() FetchKind {
	return FetchKindParallel
}

type SerialFetch struct {
	Fetches []Fetch
}

func (_ *SerialFetch) FetchKind() FetchKind {
	return FetchKindSerial
}

type BatchFetch struct {
	Fetch        *SingleFetch
	BatchFactory DataSourceBatchFactory
}

func (_ *BatchFetch) FetchKind() FetchKind {
	return FetchKindBatch
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
