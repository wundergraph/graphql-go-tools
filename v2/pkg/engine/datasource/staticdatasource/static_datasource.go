package staticdatasource

import (
	"bytes"
	"context"

	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type Configuration struct {
	Data string `json:"data"`
}

type Factory[T Configuration] struct{}

func (f *Factory[T]) Planner(logger abstractlogger.Logger) plan.DataSourcePlanner[T] {
	return &Planner[T]{}
}

func (f *Factory[T]) Context() context.Context {
	return context.TODO()
}

func (f *Factory[T]) UpstreamSchema(dataSourceConfig plan.DataSourceConfiguration[T]) (*ast.Document, bool) {
	return nil, false
}

type Planner[T Configuration] struct {
	id     int
	config Configuration
}

func (p *Planner[T]) SetID(id int) {
	p.id = id
}

func (p *Planner[T]) ID() (id int) {
	return p.id
}

func (p *Planner[T]) DownstreamResponseFieldAlias(downstreamFieldRef int) (alias string, exists bool) {
	// skip, not required
	return
}

func (p *Planner[T]) DataSourcePlanningBehavior() plan.DataSourcePlanningBehavior {
	return plan.DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      false,
		OverrideFieldPathFromAlias: false,
	}
}

func (p *Planner[T]) Register(_ *plan.Visitor, configuration plan.DataSourceConfiguration[T], _ plan.DataSourcePlannerConfiguration) error {
	p.config = Configuration(configuration.CustomConfiguration())
	return nil
}

func (p *Planner[T]) ConfigureFetch() resolve.FetchConfiguration {
	return resolve.FetchConfiguration{
		Input:      p.config.Data,
		DataSource: Source{},
	}
}

func (p *Planner[T]) ConfigureSubscription() plan.SubscriptionConfiguration {
	return plan.SubscriptionConfiguration{
		Input: p.config.Data,
	}
}

type Source struct{}

func (Source) Load(ctx context.Context, input []byte, out *bytes.Buffer) (err error) {
	_, err = out.Write(input)
	return
}

func (Source) LoadWithFiles(ctx context.Context, input []byte, files []httpclient.File, out *bytes.Buffer) (err error) {
	panic("not implemented")
}
