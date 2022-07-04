package kafka_datasource

import (
	"context"
	"encoding/json"

	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/pkg/engine/plan"
)

type Planner struct {
	ctx    context.Context
	config Configuration
}

func (p *Planner) Register(_ *plan.Visitor, configuration plan.DataSourceConfiguration, _ bool) error {
	return json.Unmarshal(configuration.Custom, &p.config)
}

func (p *Planner) ConfigureFetch() plan.FetchConfiguration {
	return plan.FetchConfiguration{}
}

func (p *Planner) ConfigureSubscription() plan.SubscriptionConfiguration {
	input, _ := json.Marshal(p.config.Subscription)
	return plan.SubscriptionConfiguration{
		Input: string(input),
		DataSource: &SubscriptionSource{
			client: NewKafkaConsumerGroupBridge(p.ctx, abstractlogger.NoopLogger),
		},
	}
}

func (p *Planner) DataSourcePlanningBehavior() plan.DataSourcePlanningBehavior {
	return plan.DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      false,
		OverrideFieldPathFromAlias: false,
	}
}

func (p *Planner) DownstreamResponseFieldAlias(_ int) (alias string, exists bool) { return }

type Factory struct{}

func (f *Factory) Planner(ctx context.Context) plan.DataSourcePlanner {
	return &Planner{
		ctx: ctx,
	}
}

func ConfigJSON(config Configuration) json.RawMessage {
	out, _ := json.Marshal(config)
	return out
}

type GraphQLSubscriptionClient interface {
	Subscribe(ctx context.Context, options GraphQLSubscriptionOptions, next chan<- []byte) error
}

type SubscriptionSource struct {
	client GraphQLSubscriptionClient
}

func (s *SubscriptionSource) Start(ctx context.Context, input []byte, next chan<- []byte) error {
	var options GraphQLSubscriptionOptions
	err := json.Unmarshal(input, &options)
	if err != nil {
		return err
	}
	return s.client.Subscribe(ctx, options, next)
}

var _ plan.PlannerFactory = (*Factory)(nil)
var _ plan.DataSourcePlanner = (*Planner)(nil)
