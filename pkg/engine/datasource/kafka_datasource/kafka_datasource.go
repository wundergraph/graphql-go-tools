package kafka_datasource

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Shopify/sarama"
	"github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
)

var (
	DefaultKafkaVersion          = "V1_0_0_0"
	SaramaSupportedKafkaVersions = map[string]sarama.KafkaVersion{
		"V0_10_2_0": sarama.V0_10_2_0,
		"V0_10_2_1": sarama.V0_10_2_1,
		"V0_11_0_0": sarama.V0_11_0_0,
		"V0_11_0_1": sarama.V0_11_0_1,
		"V0_11_0_2": sarama.V0_11_0_2,
		"V1_0_0_0":  sarama.V1_0_0_0,
		"V1_1_0_0":  sarama.V1_1_0_0,
		"V1_1_1_0":  sarama.V1_1_1_0,
		"V2_0_0_0":  sarama.V2_0_0_0,
		"V2_0_1_0":  sarama.V2_0_1_0,
		"V2_1_0_0":  sarama.V2_1_0_0,
		"V2_2_0_0":  sarama.V2_2_0_0,
		"V2_3_0_0":  sarama.V2_3_0_0,
		"V2_4_0_0":  sarama.V2_4_0_0,
		"V2_5_0_0":  sarama.V2_5_0_0,
		"V2_6_0_0":  sarama.V2_6_0_0,
		"V2_7_0_0":  sarama.V2_7_0_0,
		"V2_8_0_0":  sarama.V2_8_0_0,
	}
)

type GraphQLSubscriptionOptions struct {
	BrokerAddr           string `json:"broker_addr"`
	Topic                string `json:"topic"`
	GroupID              string `json:"group_id"`
	ClientID             string `json:"client_id"`
	KafkaVersion         string `json:"kafka_version"`
	StartConsumingLatest bool   `json:"start_consuming_latest"`
	StartedCallback      func() `json:",omitempty"`
}

func (g *GraphQLSubscriptionOptions) Sanitize() {
	if g.KafkaVersion == "" {
		g.KafkaVersion = DefaultKafkaVersion
	}
}

func (g *GraphQLSubscriptionOptions) Validate() error {
	if g.BrokerAddr == "" {
		return fmt.Errorf("broker_addr cannot be empty")
	}

	if g.Topic == "" {
		return fmt.Errorf("topic cannot be empty")
	}

	if g.GroupID == "" {
		return fmt.Errorf("group_id cannot be empty")
	}

	if g.ClientID == "" {
		return fmt.Errorf("client_id cannot be empty")
	}

	if _, ok := SaramaSupportedKafkaVersions[g.KafkaVersion]; !ok {
		return fmt.Errorf("kafka_version is invalid: %s", g.KafkaVersion)
	}

	return nil
}

type SubscriptionConfiguration struct {
	BrokerAddr           string `json:"broker_addr"`
	Topic                string `json:"topic"`
	GroupID              string `json:"group_id"`
	ClientID             string `json:"client_id"`
	KafkaVersion         string `json:"kafka_version"`
	StartConsumingLatest bool   `json:"start_consuming_latest"`
}

type Configuration struct {
	Subscription SubscriptionConfiguration
}

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
