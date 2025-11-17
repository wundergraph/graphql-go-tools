package pubsub_datasource

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type KafkaEventConfiguration struct {
	Topics []string `json:"topics"`
}

type KafkaConnector interface {
	New(ctx context.Context) KafkaPubSub
}

// KafkaPubSub describe the interface that implements the primitive operations for pubsub
type KafkaPubSub interface {
	// Subscribe starts listening on the given subjects and sends the received messages to the given next channel
	Subscribe(ctx context.Context, config KafkaSubscriptionEventConfiguration, updater resolve.SubscriptionUpdater) error
	// Publish sends the given data to the given subject
	Publish(ctx context.Context, config KafkaPublishEventConfiguration) error
}

type KafkaSubscriptionSource struct {
	pubSub KafkaPubSub
}

func (s *KafkaSubscriptionSource) Start(ctx *resolve.Context, headers http.Header, input []byte, updater resolve.SubscriptionUpdater) error {
	var subscriptionConfiguration KafkaSubscriptionEventConfiguration
	err := json.Unmarshal(input, &subscriptionConfiguration)
	if err != nil {
		return err
	}

	return s.pubSub.Subscribe(ctx.Context(), subscriptionConfiguration, updater)
}

type KafkaPublishDataSource struct {
	pubSub KafkaPubSub
}

func (s *KafkaPublishDataSource) Load(ctx context.Context, headers http.Header, input []byte) (data []byte, err error) {
	var publishConfiguration KafkaPublishEventConfiguration
	err = json.Unmarshal(input, &publishConfiguration)
	if err != nil {
		return nil, err
	}

	if err := s.pubSub.Publish(ctx, publishConfiguration); err != nil {
		return []byte(`{"success": false}`), err
	}
	return []byte(`{"success": true}`), nil
}

func (s *KafkaPublishDataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) (data []byte, err error) {
	panic("not implemented")
}
