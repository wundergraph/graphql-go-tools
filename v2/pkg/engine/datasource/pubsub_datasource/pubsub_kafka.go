package pubsub_datasource

import (
	"context"
	"fmt"
	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash/v2"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"io"
)

type KafkaEventConfiguration struct {
	Topics []string `json:"topics"`
}

type KafkaConnector interface {
	New(ctx context.Context) KafkaPubSub
}

// KafkaPubSub describe the interface that implements the primitive operations for pubsub
type KafkaPubSub interface {
	// ID is the unique identifier of the pubsub implementation (e.g. Kafka)
	// This is used to uniquely identify a subscription
	ID() string
	// Subscribe starts listening on the given subjects and sends the received messages to the given next channel
	Subscribe(ctx context.Context, topics []string, updater resolve.SubscriptionUpdater) error
	// Publish sends the given data to the given subject
	Publish(ctx context.Context, subject string, data []byte) error
}

type KafkaSubscriptionSource struct {
	pubSub KafkaPubSub
}

func (s *KafkaSubscriptionSource) UniqueRequestID(ctx *resolve.Context, input []byte, xxh *xxhash.Digest) error {
	// input must be unique across datasources
	_, err := xxh.Write(input)
	return err
}

func (s *KafkaSubscriptionSource) Start(ctx *resolve.Context, input []byte, updater resolve.SubscriptionUpdater) error {
	// TODO: implement
	return s.pubSub.Subscribe(ctx.Context(), nil, updater)
}

type KafkaPublishDataSource struct {
	pubSub KafkaPubSub
}

func (s *KafkaPublishDataSource) Load(ctx context.Context, input []byte, w io.Writer) error {
	subject, err := jsonparser.GetString(input, "subject")
	if err != nil {
		return fmt.Errorf("error getting subject from input: %w", err)
	}

	data, _, _, err := jsonparser.Get(input, "data")
	if err != nil {
		return fmt.Errorf("error getting data from input: %w", err)
	}

	if err := s.pubSub.Publish(ctx, subject, data); err != nil {
		return err
	}
	_, err = io.WriteString(w, `{"success": true}`)
	return err
}
