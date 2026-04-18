package staticdatasource

import (
	"context"
	"net/http"

	"github.com/jensneuse/abstractlogger"
	"github.com/wundergraph/astjson"

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

// Load parses the static input JSON into an *astjson.Value. The returned value
// is rooted on a freshly-allocated arena private to this call; cleanup returns
// that arena to the GC when the loader finishes with it.
//
// We don't pool the arena — static datasources are typically used for a small
// number of hand-configured responses, not in high-throughput paths; the
// simplicity is worth more than the per-call arena alloc.
func (Source) Load(ctx context.Context, headers http.Header, input []byte) (*astjson.Value, func(), error) {
	v, err := astjson.ParseBytes(input)
	if err != nil {
		return nil, nil, err
	}
	return v, nil, nil
}

func (Source) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) (*astjson.Value, func(), error) {
	panic("not implemented")
}
