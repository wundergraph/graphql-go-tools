package kafka_datasource

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestConfig_GraphQLSubscriptionOptions(t *testing.T) {
	t.Run("Set default isolation_level", func(t *testing.T) {
		g := &GraphQLSubscriptionOptions{}
		g.Sanitize()
		require.Equal(t, DefaultIsolationLevel, g.IsolationLevel)
	})

	t.Run("Set default balance_strategy", func(t *testing.T) {
		g := &GraphQLSubscriptionOptions{}
		g.Sanitize()
		require.Equal(t, DefaultBalanceStrategy, g.BalanceStrategy)
	})

	t.Run("Set default Kafka version", func(t *testing.T) {
		g := &GraphQLSubscriptionOptions{}
		g.Sanitize()
		require.Equal(t, DefaultKafkaVersion, g.KafkaVersion)
	})

	t.Run("Empty broker_addr not allowed", func(t *testing.T) {
		g := &GraphQLSubscriptionOptions{
			Topic:    "foobar",
			GroupID:  "groupid",
			ClientID: "clientid",
		}
		g.Sanitize()
		err := g.Validate()
		require.Equal(t, err.Error(), "broker_addr cannot be empty")
	})

	t.Run("Empty topic not allowed", func(t *testing.T) {
		g := &GraphQLSubscriptionOptions{
			BrokerAddr: "localhost:9092",
			GroupID:    "groupid",
			ClientID:   "clientid",
		}
		g.Sanitize()
		err := g.Validate()
		require.Equal(t, err.Error(), "topic cannot be empty")
	})

	t.Run("Empty client_id not allowed", func(t *testing.T) {
		g := &GraphQLSubscriptionOptions{
			BrokerAddr: "localhost:9092",
			Topic:      "foobar",
			GroupID:    "groupid",
		}
		g.Sanitize()
		err := g.Validate()
		require.Equal(t, err.Error(), "client_id cannot be empty")
	})

	t.Run("Invalid Kafka version", func(t *testing.T) {
		g := &GraphQLSubscriptionOptions{
			BrokerAddr:   "localhost:9092",
			Topic:        "foobar",
			GroupID:      "groupid",
			ClientID:     "clientid",
			KafkaVersion: "1.3.5",
		}
		g.Sanitize()
		err := g.Validate()
		require.Equal(t, err.Error(), "kafka_version is invalid: 1.3.5")
	})
}
