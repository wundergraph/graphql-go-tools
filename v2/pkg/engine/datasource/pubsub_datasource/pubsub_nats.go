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

type NatsStreamConfiguration struct {
	Consumer   string `json:"consumer"`
	StreamName string `json:"streamName"`
}

type NatsEventConfiguration struct {
	StreamConfiguration *NatsStreamConfiguration `json:"streamConfiguration,omitempty"`
	Subjects            []string                 `json:"subjects"`
}

type NatsConnector interface {
	New(ctx context.Context) NatsPubSub
}

// NatsPubSub describe the interface that implements the primitive operations for pubsub
type NatsPubSub interface {
	// Subscribe starts listening on the given subjects and sends the received messages to the given next channel
	Subscribe(ctx context.Context, event NatsSubscriptionEventConfiguration, updater resolve.SubscriptionUpdater) error
	// Publish sends the given data to the given subject
	Publish(ctx context.Context, event NatsPublishAndRequestEventConfiguration) error
	// Request sends a request on the given subject and writes the response to the given writer
	Request(ctx context.Context, event NatsPublishAndRequestEventConfiguration, w io.Writer) error
}

type NatsSubscriptionSource struct {
	pubSub NatsPubSub
}

func (s *NatsSubscriptionSource) UniqueRequestID(ctx *resolve.Context, input []byte, xxh *xxhash.Digest) error {

	val, _, _, err := jsonparser.Get(input, "subjects")
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

func (s *NatsSubscriptionSource) Start(ctx *resolve.Context, input []byte, updater resolve.SubscriptionUpdater) error {
	var subscriptionConfiguration NatsSubscriptionEventConfiguration
	err := json.Unmarshal(input, &subscriptionConfiguration)
	if err != nil {
		return err
	}
	subscriptionConfiguration.SendHeartbeat = ctx.ExecutionOptions.SendHeartbeat

	return s.pubSub.Subscribe(ctx.Context(), subscriptionConfiguration, updater)
}

type NatsPublishDataSource struct {
	pubSub NatsPubSub
}

func (s *NatsPublishDataSource) Load(ctx context.Context, input []byte, out *bytes.Buffer) error {
	var publishConfiguration NatsPublishAndRequestEventConfiguration
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

func (s *NatsPublishDataSource) LoadWithFiles(ctx context.Context, input []byte, files []httpclient.File, out *bytes.Buffer) error {
	panic("not implemented")
}

type NatsRequestDataSource struct {
	pubSub NatsPubSub
}

func (s *NatsRequestDataSource) Load(ctx context.Context, input []byte, out *bytes.Buffer) error {
	var subscriptionConfiguration NatsPublishAndRequestEventConfiguration
	err := json.Unmarshal(input, &subscriptionConfiguration)
	if err != nil {
		return err
	}

	return s.pubSub.Request(ctx, subscriptionConfiguration, out)
}

func (s *NatsRequestDataSource) LoadWithFiles(ctx context.Context, input []byte, files []httpclient.File, out *bytes.Buffer) error {
	panic("not implemented")
}
