package staticdatasource

import (
	"context"
	"encoding/json"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

const (
	UniqueIdentifier = "static"
)

type Configuration struct {
	Data string `json:"data"`
}

func ConfigJSON(config Configuration) json.RawMessage {
	out, _ := json.Marshal(config)
	return out
}

type Factory struct{}

func (f *Factory) Planner() plan.DataSourcePlanner {
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

func (p *Planner) Register(visitor *plan.Visitor, customConfiguration json.RawMessage, isNested bool) error {
	return json.Unmarshal(customConfiguration, &p.config)
}

func (p *Planner) ConfigureFetch() plan.FetchConfiguration {
	return plan.FetchConfiguration{
		Input:      p.config.Data,
		DataSource: Source{},
	}
}

func (p *Planner) ConfigureSubscription() plan.SubscriptionConfiguration {
	return plan.SubscriptionConfiguration{
		Input:                 p.config.Data,
		SubscriptionManagerID: "static",
	}
}

type Source struct{}

var (
	uniqueIdentifier = []byte(UniqueIdentifier)
)

func (_ Source) UniqueIdentifier() []byte {
	return uniqueIdentifier
}

func (_ Source) Load(ctx context.Context, input []byte, bufPair *resolve.BufPair) (err error) {
	bufPair.Data.WriteBytes(input)
	return
}
