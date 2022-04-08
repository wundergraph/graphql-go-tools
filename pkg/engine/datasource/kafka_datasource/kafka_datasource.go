package kafka_datasource

import "github.com/Shopify/sarama"

type GraphQLSubscriptionOptions struct {
	BrokerAddr   string `json:"BrokerAddr"`
	Topic        string `json:"Topic"`
	GroupID      string `json:"GroupID"`
	ClientID     string `json:"ClientID"`
	saramaConfig *sarama.Config
}
