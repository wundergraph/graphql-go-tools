package staticdatasource

import (
	"context"
	"encoding/json"
	"io"

	"github.com/wundergraph/graphql-go-tools/pkg/engine/plan"
)

type Configuration struct {
	Data string `json:"data"`
}

func ConfigJSON(config Configuration) json.RawMessage {
	out, _ := json.Marshal(config)
	return out
}

type Factory struct{}

func (f *Factory) Planner(ctx context.Context) plan.DataSourcePlanner {
	return &Planner{}
}

type Planner struct {
	config Configuration
}

func (p *Planner) DownstreamResponseFieldAlias(downstreamFieldRef int) (alias string, exists bool) {
	// skip, not required
	return
}

func (p *Planner) DataSourcePlanningBehavior() plan.DataSourcePlanningBehavior {
	return plan.DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      false,
		OverrideFieldPathFromAlias: false,
	}
}

func (p *Planner) Register(_ *plan.Visitor, configuration plan.DataSourceConfiguration, _ bool) error {
	return json.Unmarshal(configuration.Custom, &p.config)
}

func (p *Planner) ConfigureFetch() plan.FetchConfiguration {
	return plan.FetchConfiguration{
		Input:                p.config.Data,
		DataSource:           Source{},
		DisableDataLoader:    true,
		DisallowSingleFlight: true,
	}
}

func (p *Planner) ConfigureSubscription() plan.SubscriptionConfiguration {
	return plan.SubscriptionConfiguration{
		Input: p.config.Data,
	}
}

type Source struct{}

func (Source) Load(ctx context.Context, input []byte, w io.Writer) (err error) {
	_, err = w.Write(input)
	return
}
