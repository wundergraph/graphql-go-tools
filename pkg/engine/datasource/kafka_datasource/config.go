package kafka_datasource

import (
	"fmt"

	"github.com/Shopify/sarama"
)

const (
	IsolationLevelReadUncommitted = "ReadUncommitted"
	IsolationLevelReadCommitted   = "ReadCommitted"
)

const DefaultIsolationLevel = IsolationLevelReadUncommitted

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

type SASL struct {
	// Whether or not to use SASL authentication when connecting to the broker
	// (defaults to false).
	Enable bool `json:"enable"`
	// User is the authentication identity (authcid) to present for
	// SASL/PLAIN or SASL/SCRAM authentication
	User string `json:"user"`
	// Password for SASL/PLAIN authentication
	Password string `json:"password"`
}

type GraphQLSubscriptionOptions struct {
	BrokerAddresses      []string `json:"broker_addresses"`
	Topics               []string `json:"topics"`
	GroupID              string   `json:"group_id"`
	ClientID             string   `json:"client_id"`
	KafkaVersion         string   `json:"kafka_version"`
	StartConsumingLatest bool     `json:"start_consuming_latest"`
	BalanceStrategy      string   `json:"balance_strategy"`
	IsolationLevel       string   `json:"isolation_level"`
	SASL                 SASL     `json:"sasl"`
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

	if g.IsolationLevel == "" {
		g.IsolationLevel = DefaultIsolationLevel
	}
}

func (g *GraphQLSubscriptionOptions) Validate() error {
	switch {
	case len(g.BrokerAddresses) == 0:
		return fmt.Errorf("broker_addresses cannot be empty")
	case len(g.Topics) == 0:
		return fmt.Errorf("topics cannot be empty")
	case g.GroupID == "":
		return fmt.Errorf("group_id cannot be empty")
	case g.ClientID == "":
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

	switch g.IsolationLevel {
	case IsolationLevelReadUncommitted, IsolationLevelReadCommitted:
	default:
		return fmt.Errorf("isolation_level is invalid: %s", g.IsolationLevel)
	}

	if g.SASL.Enable {
		switch {
		case g.SASL.User == "":
			return fmt.Errorf("sasl.user cannot be empty")
		case g.SASL.Password == "":
			return fmt.Errorf("sasl.password cannot be empty")
		}
	}

	return nil
}

type SubscriptionConfiguration struct {
	BrokerAddresses      []string `json:"broker_addresses"`
	Topics               []string `json:"topics"`
	GroupID              string   `json:"group_id"`
	ClientID             string   `json:"client_id"`
	KafkaVersion         string   `json:"kafka_version"`
	StartConsumingLatest bool     `json:"start_consuming_latest"`
	BalanceStrategy      string   `json:"balance_strategy"`
	IsolationLevel       string   `json:"isolation_level"`
	SASL                 SASL     `json:"sasl"`
}

type Configuration struct {
	Subscription SubscriptionConfiguration
}
