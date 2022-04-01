package kafka_datasource

type GraphQLSubscriptionOptions struct {
	BrokerAddr string `json:"BrokerAddr"`
	Topic      string `json:"Topic"`
	GroupID    string `json:"GroupID"`
	ClientID   string `json:"ClientID"`
}
