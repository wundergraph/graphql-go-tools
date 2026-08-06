package resolve

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
)

// SubgraphOperation is the planner hand-off for MultiFetch merging.
type SubgraphOperation struct {
	// Document is the normalized and validated upstream operation. Ownership
	// transfers to the plan; the planner nils its own reference after storing.
	Document *ast.Document
	// Variables lists the top-level body.variables entries in write order
	// (value replaced in place on duplicate name). Values are raw fragments
	// that may contain $$N$$ placeholders referring to
	// FetchConfiguration.Variables.
	Variables []SubgraphVariable
	// Envelope carries the static request fields (method, url, header) that
	// wrap the operation when the input is rendered.
	Envelope SubgraphRequestEnvelope

	// printedQuery caches the minified operation print; produced at most once,
	// on first need (dedupe comparison or input rendering). See PrintedQuery.
	printedQuery       []byte
	printedQueryCached bool
}

type SubgraphVariable struct {
	Name  string
	Value []byte
}

// SubgraphRequestEnvelope holds the static request fields that wrap the printed
// operation when the entity-fetch input is assembled — exactly the bytes the
// planner would set via httpclient.AssembleGraphQLRequestInput.
type SubgraphRequestEnvelope struct {
	Method string
	URL    string
	Header []byte // raw JSON header bytes, or nil when none would be printed
}

// SetPrintedQuery seeds the print cache with the exact compact operation bytes
// the planner already produced. Seeding preserves the planner's minification,
// which the deferred input rendering must reproduce byte-for-byte.
func (o *SubgraphOperation) SetPrintedQuery(query []byte) {
	o.printedQuery = query
	o.printedQueryCached = true
}

// PrintedQuery returns the minified upstream operation print, producing it at
// most once and caching the result, so dedupe comparison and input rendering
// share the same bytes. When the cache was not seeded via SetPrintedQuery (e.g.
// hand-built fetches in tests), Document is printed in compact form.
func (o *SubgraphOperation) PrintedQuery() ([]byte, error) {
	if o.printedQueryCached {
		return o.printedQuery, nil
	}
	printed, err := astprinter.PrintString(o.Document)
	if err != nil {
		return nil, err
	}
	o.printedQuery = []byte(printed)
	o.printedQueryCached = true
	return o.printedQuery, nil
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

	Input      MultiEntityInput
	DataSource DataSource
	Trace      *DataSourceLoadTrace
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
