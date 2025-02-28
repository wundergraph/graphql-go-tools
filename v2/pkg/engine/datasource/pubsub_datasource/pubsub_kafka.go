package pubsub_datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash/v2"

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

func (s *KafkaSubscriptionSource) UniqueRequestID(ctx *resolve.Context, input []byte, xxh *xxhash.Digest) error {

	val, _, _, err := jsonparser.Get(input, "topics")
	if err != nil {
		return err
	}

	_, err = xxh.Write(val)
	if err != nil {
		return err
	}

	val, _, _, err = jsonparser.Get(input, "providerId")
	if err != nil {
		return err
	}

	_, err = xxh.Write(val)
	return err
}

func (s *KafkaSubscriptionSource) Start(ctx *resolve.Context, input []byte, updater resolve.SubscriptionUpdater) error {
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

func (s *KafkaPublishDataSource) Load(ctx context.Context, input []byte, out *bytes.Buffer) error {
	var publishConfiguration KafkaPublishEventConfiguration
	err := json.Unmarshal(input, &publishConfiguration)
	if err != nil {
		return err
	}

	if err := s.pubSub.Publish(ctx, publishConfiguration); err != nil {
		_, err = io.WriteString(out, `{"success": false}`)
		return err
	}
	_, err = io.WriteString(out, `{"success": true}`)
	return err
}

func (s *KafkaPublishDataSource) LoadWithFiles(ctx context.Context, input []byte, files []*httpclient.FileUpload, out *bytes.Buffer) (err error) {
	panic("not implemented")
}
