package kafka_datasource

import (
	"context"
	"encoding/json"

	"github.com/cespare/xxhash/v2"
	"github.com/jensneuse/abstractlogger"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
)

var (
	dataSourceName = []byte("kafka")
)

type Planner struct {
	ctx    context.Context
	config Configuration
}

func (p *Planner) UpstreamSchema(_ plan.DataSourceConfiguration) *ast.Document {
	return nil
}

func (p *Planner) Register(visitor *plan.Visitor, configuration plan.DataSourceConfiguration, _ plan.DataSourcePlannerConfiguration) error {
	return json.Unmarshal(configuration.Custom, &p.config)
}

func (p *Planner) ConfigureFetch() resolve.FetchConfiguration {
	return resolve.FetchConfiguration{}
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
	Subscribe(ctx *resolve.Context, options GraphQLSubscriptionOptions, updater resolve.CloseableSubscriptionUpdater) error
	UniqueRequestID(ctx *resolve.Context, options GraphQLSubscriptionOptions, hash *xxhash.Digest) (err error)
}

type SubscriptionSource struct {
	client GraphQLSubscriptionClient
}

func (s *SubscriptionSource) UniqueRequestID(ctx *resolve.Context, input []byte, xxh *xxhash.Digest) (err error) {
	_, err = xxh.Write(dataSourceName)
	if err != nil {
		return err
	}
	var options GraphQLSubscriptionOptions
	err = json.Unmarshal(input, &options)
	if err != nil {
		return err
	}
	return s.client.UniqueRequestID(ctx, options, xxh)
}

func (s *SubscriptionSource) Start(ctx *resolve.Context, input []byte, updater resolve.SubscriptionUpdater) error {
	var options GraphQLSubscriptionOptions
	err := json.Unmarshal(input, &options)
	if err != nil {
		return err
	}
	closeableUpdater, ok := resolve.AsCloseableSubscriptionUpdater(updater)
	if !ok {
		return resolve.ErrSubscriptionUpdaterNotCloseable
	}
	return s.client.Subscribe(ctx, options, closeableUpdater)
}

var _ plan.PlannerFactory = (*Factory)(nil)
var _ plan.DataSourcePlanner = (*Planner)(nil)
