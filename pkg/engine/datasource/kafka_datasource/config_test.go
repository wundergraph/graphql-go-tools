package kafka_datasource

import (
	"testing"

	"github.com/stretchr/testify/require"
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

	t.Run("Empty broker_addresses not allowed", func(t *testing.T) {
		g := &GraphQLSubscriptionOptions{
			Topics:   []string{"foobar"},
			GroupID:  "groupid",
			ClientID: "clientid",
		}
		g.Sanitize()
		err := g.Validate()
		require.Equal(t, err.Error(), "broker_addresses cannot be empty")
	})

	t.Run("Empty topic not allowed", func(t *testing.T) {
		g := &GraphQLSubscriptionOptions{
			BrokerAddresses: []string{"localhost:9092"},
			GroupID:         "groupid",
			ClientID:        "clientid",
		}
		g.Sanitize()
		err := g.Validate()
		require.Equal(t, err.Error(), "topics cannot be empty")
	})

	t.Run("Empty client_id not allowed", func(t *testing.T) {
		g := &GraphQLSubscriptionOptions{
			BrokerAddresses: []string{"localhost:9092"},
			Topics:          []string{"foobar"},
			GroupID:         "groupid",
		}
		g.Sanitize()
		err := g.Validate()
		require.Equal(t, err.Error(), "client_id cannot be empty")
	})

	t.Run("Invalid Kafka version", func(t *testing.T) {
		g := &GraphQLSubscriptionOptions{
			BrokerAddresses: []string{"localhost:9092"},
			Topics:          []string{"foobar"},
			GroupID:         "groupid",
			ClientID:        "clientid",
			KafkaVersion:    "1.3.5",
		}
		g.Sanitize()
		err := g.Validate()
		require.Equal(t, err.Error(), "kafka_version is invalid: 1.3.5")
	})

	t.Run("Invalid SASL configuration - SASL nil", func(t *testing.T) {
		g := &GraphQLSubscriptionOptions{
			BrokerAddresses: []string{"localhost:9092"},
			Topics:          []string{"foobar"},
			GroupID:         "groupid",
			ClientID:        "clientid",
			SASL:            SASL{},
		}
		g.Sanitize()
		err := g.Validate()
		require.NoError(t, err)
	})

	t.Run("Invalid SASL configuration - auth disabled", func(t *testing.T) {
		g := &GraphQLSubscriptionOptions{
			BrokerAddresses: []string{"localhost:9092"},
			Topics:          []string{"foobar"},
			GroupID:         "groupid",
			ClientID:        "clientid",
			SASL: SASL{
				Enable: false,
			},
		}
		g.Sanitize()
		err := g.Validate()
		require.NoError(t, err)
	})

	t.Run("Invalid SASL configuration - user cannot be empty", func(t *testing.T) {
		g := &GraphQLSubscriptionOptions{
			BrokerAddresses: []string{"localhost:9092"},
			Topics:          []string{"foobar"},
			GroupID:         "groupid",
			ClientID:        "clientid",
			SASL: SASL{
				Enable: true,
			},
		}
		g.Sanitize()
		err := g.Validate()
		require.Equal(t, err.Error(), "sasl.user cannot be empty")
	})

	t.Run("Invalid SASL configuration - password cannot be empty", func(t *testing.T) {
		g := &GraphQLSubscriptionOptions{
			BrokerAddresses: []string{"localhost:9092"},
			Topics:          []string{"foobar"},
			GroupID:         "groupid",
			ClientID:        "clientid",
			SASL: SASL{
				Enable: true,
				User:   "foobar",
			},
		}
		g.Sanitize()
		err := g.Validate()
		require.Equal(t, err.Error(), "sasl.password cannot be empty")
	})

	t.Run("Valid SASL configuration", func(t *testing.T) {
		g := &GraphQLSubscriptionOptions{
			BrokerAddresses: []string{"localhost:9092"},
			Topics:          []string{"foobar"},
			GroupID:         "groupid",
			ClientID:        "clientid",
			SASL: SASL{
				Enable:   true,
				User:     "foobar",
				Password: "password",
			},
		}
		g.Sanitize()
		err := g.Validate()
		require.NoError(t, err)
	})
}
