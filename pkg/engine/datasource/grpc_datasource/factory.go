package grpc_datasource

import (
	"context"

	"github.com/wundergraph/graphql-go-tools/pkg/engine/plan"
)

type Factory struct {
}

func NewFactory() *Factory {
	return &Factory{}
}

func (f *Factory) Planner(_ context.Context) plan.DataSourcePlanner {
	return &Planner{}
}
