package resolve

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// MergeableOperation is the planner hand-off for MultiFetch merging.
type MergeableOperation struct {
	// Document is the normalized and validated upstream operation. Ownership
	// transfers to the plan; the planner nils its own reference after storing.
	Document *ast.Document
	// Variables lists the top-level body.variables entries in write order
	// (value replaced in place on duplicate name). Values are raw fragments
	// that may contain $$N$$ placeholders referring to
	// FetchConfiguration.Variables.
	Variables []NamedVariableFragment
}

type NamedVariableFragment struct {
	Name  string
	Value []byte
}

type EntityFetchOriginKind int

const (
	EntityFetchOriginSingle EntityFetchOriginKind = iota + 1
	EntityFetchOriginBatch
)

// MultiEntityFetch merges several same-subgraph entity fetches into one
// request with aliased _entities fields guarded by @include variables.
type MultiEntityFetch struct {
	FetchDependencies

	Input                MultiEntityInput
	DataSource           DataSource
	DataSourceIdentifier []byte
	Trace                *DataSourceLoadTrace
	// MergedFetchIDs are the original fetch IDs merged into this fetch, in
	// wave order; surfaced in query-plan output.
	MergedFetchIDs []int
	Info           *FetchInfo
}

func (m *MultiEntityFetch) Dependencies() *FetchDependencies { return &m.FetchDependencies }
func (m *MultiEntityFetch) FetchInfo() *FetchInfo            { return m.Info }
func (*MultiEntityFetch) FetchKind() FetchKind               { return FetchKindMultiEntity }

type MultiEntityInput struct {
	Header  InputTemplate
	Entries []MultiEntityFetchEntry
	Footer  InputTemplate
}

// MultiEntityFetchEntry is one original entity fetch inside the merged
// request: raw-fetch identity plus the template material for its slice of
// body.variables. It has no rendered input of its own.
type MultiEntityFetchEntry struct {
	Alias          string
	Item           *FetchItem // original FetchPath/ResponsePath; Fetch is the parent MultiEntityFetch
	Info           *FetchInfo
	PostProcessing PostProcessingConfiguration
	OriginKind     EntityFetchOriginKind

	RepresentationsPrefix []byte // `"representations_f1":[` with a leading ',' for entries after the first
	Representations       InputTemplate
	IncludePrefix         []byte // `],"includeF1":`
	Variables             []MultiEntityFetchVariable

	SkipNullItems        bool
	SkipEmptyObjectItems bool
	SkipErrItems         bool
}

type MultiEntityFetchVariable struct {
	KeyPrefix []byte // `,"first_f1":`
	Value     InputTemplate
}
