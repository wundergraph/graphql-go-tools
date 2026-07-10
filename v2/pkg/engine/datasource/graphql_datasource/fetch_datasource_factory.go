package graphql_datasource

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// FetchMode identifies the shape of a planned fetch.
type FetchMode uint8

const (
	FetchModeSingle FetchMode = iota
	FetchModeEntity
	FetchModeEntityBatch
)

// FetchDataSourceFactory creates the runtime data source for a planned fetch.
//
// PlannedFetch.Operation, PlannedFetch.Definition, PlannedFetch.Variables,
// PlannedFetch.RequiredFields, and PlannedFetch.QueryPlan are borrowed and
// read-only, and are valid only for the duration of NewDataSource.
// PlannedFetch.PostProcessing and the top-level slice headers of Variables and
// RequiredFields are value snapshots, but nested renderers, paths, and backing
// data are borrowed and read-only. Implementations must not mutate or retain
// borrowed values; they must synchronously derive or deep-copy anything retained.
type FetchDataSourceFactory interface {
	NewDataSource(fetch PlannedFetch) (resolve.DataSource, error)
}

// PlannedFetch describes a fetch after planning and before its runtime data
// source is created. Its fields follow the ownership rules documented on
// FetchDataSourceFactory.
type PlannedFetch struct {
	Operation      *ast.Document
	Definition     *ast.Document
	Variables      resolve.Variables
	FetchMode      FetchMode
	PostProcessing resolve.PostProcessingConfiguration
	RequiredFields plan.FederationFieldConfigurations
	QueryPlan      *resolve.QueryPlan
}
