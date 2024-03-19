package introspection_datasource

import (
	"context"

	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/introspection"
)

type Factory[T Configuration] struct {
	introspectionData *introspection.Data
}

func NewFactory[T Configuration](introspectionData *introspection.Data) *Factory[T] {
	return &Factory[T]{introspectionData: introspectionData}
}

func (f *Factory[T]) Planner(logger abstractlogger.Logger) plan.DataSourcePlanner[T] {
	return &Planner[T]{introspectionData: f.introspectionData}
}

func (f *Factory[T]) Context() context.Context {
	return context.TODO()
}
