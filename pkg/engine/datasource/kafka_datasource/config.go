package kafka_datasource

import (
	"fmt"

	"github.com/Shopify/sarama"
)

const (
	BalanceStrategyRange      = "BalanceStrategyRange"
	BalanceStrategySticky     = "BalanceStrategySticky"
	BalanceStrategyRoundRobin = "BalanceStrategyRoundRobin"
)

const DefaultBalanceStrategy = BalanceStrategyRange

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
	BalanceStrategy      string `json:"balance_strategy"`
	startedCallback      func()
}

func (g *GraphQLSubscriptionOptions) Sanitize() {
	if g.KafkaVersion == "" {
		g.KafkaVersion = DefaultKafkaVersion
	}

	// Strategy for allocating topic partitions to members (default BalanceStrategyRange)
	if g.BalanceStrategy == "" {
		g.BalanceStrategy = DefaultBalanceStrategy
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

	switch g.BalanceStrategy {
	case BalanceStrategyRange, BalanceStrategySticky, BalanceStrategyRoundRobin:
	default:
		return fmt.Errorf("balance_strategy is invalid: %s", g.BalanceStrategy)
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
	BalanceStrategy      string `json:"balance_strategy"`
}

type Configuration struct {
	Subscription SubscriptionConfiguration
}
