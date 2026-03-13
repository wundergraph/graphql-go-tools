package service_datasource

import (
	"context"

	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

// Factory creates planners for the __service field.
type Factory[T Configuration] struct {
	service *Service
}

// NewFactory creates a new Factory with the given service configuration.
func NewFactory[T Configuration](service *Service) *Factory[T] {
	return &Factory[T]{service: service}
}

// Planner implements the PlannerFactory interface.
func (f *Factory[T]) Planner(logger abstractlogger.Logger) plan.DataSourcePlanner[T] {
	return &Planner[T]{service: f.service}
}

// Context implements the PlannerFactory interface.
func (f *Factory[T]) Context() context.Context {
	return context.TODO()
}

// UpstreamSchema implements the PlannerFactory interface.
func (f *Factory[T]) UpstreamSchema(_ plan.DataSourceConfiguration[T]) (*ast.Document, bool) {
	return nil, false
}

// PlanningBehavior implements the PlannerFactory interface.
func (f *Factory[T]) PlanningBehavior() plan.DataSourcePlanningBehavior {
	return plan.DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      false,
		OverrideFieldPathFromAlias: true,
		AllowPlanningTypeName:      true,
	}
}
