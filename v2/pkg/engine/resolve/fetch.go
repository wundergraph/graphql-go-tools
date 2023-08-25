package resolve

type FetchKind int

const (
	FetchKindSingle FetchKind = iota + 1
	FetchKindParallel
	FetchKindBatch
	FetchKindSerial
	FetchKindEntity
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
	ProcessResponseConfig  ProcessResponseConfig
	// SetTemplateOutputToNullOnVariableNull will safely return "null" if one of the template variables renders to null
	// This is the case, e.g. when using batching and one sibling is null, resulting in a null value for one batch item
	// Returning null in this case tells the batch implementation to skip this item
	SetTemplateOutputToNullOnVariableNull bool
}

type ProcessResponseConfig struct {
	ExtractGraphqlResponse    bool
	ExtractFederationEntities bool
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

type EntityFetch struct {
	Fetch *SingleFetch
}

func (_ *EntityFetch) FetchKind() FetchKind {
	return FetchKindEntity
}
