package staticdatasource

import (
	"context"
	"net/http"

	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type Factory[T Configuration] struct{}

func (f *Factory[T]) Planner(_ abstractlogger.Logger) plan.DataSourcePlanner[T] {
	return &Planner[T]{}
}

func (f *Factory[T]) Context() context.Context {
	return context.TODO()
}

func (f *Factory[T]) UpstreamSchema(_ plan.DataSourceConfiguration[T]) (*ast.Document, bool) {
	return nil, false
}

func (f *Factory[T]) PlanningBehavior() plan.DataSourcePlanningBehavior {
	return plan.DataSourcePlanningBehavior{}
}

type Configuration struct {
	Data string `json:"data"`
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

func (Source) Load(ctx context.Context, headers http.Header, input []byte) (data []byte, err error) {
	return input, nil
}

func (Source) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) (data []byte, err error) {
	panic("not implemented")
}
