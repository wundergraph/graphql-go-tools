package search_datasource

import (
	"context"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"

	"github.com/jensneuse/abstractlogger"
)

// Factory creates Planner instances for the search datasource.
type Factory struct {
	executionContext context.Context
	indexRegistry    *searchindex.IndexFactoryRegistry
	embedderRegistry *searchindex.EmbedderRegistry
	indices          map[string]searchindex.Index // index name → Index instance
}

// NewFactory creates a new search datasource factory.
func NewFactory(
	ctx context.Context,
	indexRegistry *searchindex.IndexFactoryRegistry,
	embedderRegistry *searchindex.EmbedderRegistry,
) *Factory {
	return &Factory{
		executionContext:  ctx,
		indexRegistry:     indexRegistry,
		embedderRegistry: embedderRegistry,
		indices:          make(map[string]searchindex.Index),
	}
}

// RegisterIndex registers a pre-created index for use by planners.
func (f *Factory) RegisterIndex(name string, index searchindex.Index) {
	f.indices[name] = index
}

// Planner creates a new DataSourcePlanner for the search datasource.
func (f *Factory) Planner(_ abstractlogger.Logger) plan.DataSourcePlanner[Configuration] {
	return &Planner{
		factory: f,
	}
}

// Context returns the execution context.
func (f *Factory) Context() context.Context {
	return f.executionContext
}

// UpstreamSchema returns the upstream schema for the search datasource.
func (f *Factory) UpstreamSchema(_ plan.DataSourceConfiguration[Configuration]) (*ast.Document, bool) {
	return nil, false
}

// PlanningBehavior returns the planning behavior for the search datasource.
func (f *Factory) PlanningBehavior() plan.DataSourcePlanningBehavior {
	return plan.DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      true,
		OverrideFieldPathFromAlias: true,
	}
}

// CreateSourceForConfig creates a Source for the given configuration.
func (f *Factory) CreateSourceForConfig(config Configuration) (*Source, error) {
	idx, ok := f.indices[config.IndexName]
	if !ok {
		return nil, nil
	}

	source := &Source{
		index:  idx,
		config: config,
	}

	// If the entity has embedding fields, find the appropriate embedder
	if len(config.EmbeddingFields) > 0 && f.embedderRegistry != nil {
		model := config.EmbeddingFields[0].Model
		embedder, err := f.embedderRegistry.Get(model)
		if err == nil {
			source.embedder = embedder
		}
	}

	return source, nil
}
