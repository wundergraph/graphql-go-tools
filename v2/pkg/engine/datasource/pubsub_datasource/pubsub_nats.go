package pubsub_datasource

import (
	"context"
	"fmt"
	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash/v2"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"io"
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
	// ID is the unique identifier of the pubsub implementation (e.g. NATS)
	// This is used to uniquely identify a subscription
	ID() string
	// Subscribe starts listening on the given subjects and sends the received messages to the given next channel
	Subscribe(ctx context.Context, subjects []string, updater resolve.SubscriptionUpdater, streamConfiguration *NatsStreamConfiguration) error
	// Publish sends the given data to the given subject
	Publish(ctx context.Context, subject string, data []byte) error
	// Request sends a request on the given subject and writes the response to the given writer
	Request(ctx context.Context, subject string, data []byte, w io.Writer) error
}

type NatsSubscriptionSource struct {
	pubSub NatsPubSub
}

func (s *NatsSubscriptionSource) UniqueRequestID(ctx *resolve.Context, input []byte, xxh *xxhash.Digest) error {
	// input must be unique across datasources
	_, err := xxh.Write(input)
	return err
}

func (s *NatsSubscriptionSource) Start(ctx *resolve.Context, input []byte, updater resolve.SubscriptionUpdater) error {
	// TODO: implement
	return s.pubSub.Subscribe(ctx.Context(), nil, updater, nil)
}

type NatsPublishDataSource struct {
	pubSub NatsPubSub
}

func (s *NatsPublishDataSource) Load(ctx context.Context, input []byte, w io.Writer) error {
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

type NatsRequestDataSource struct {
	pubSub NatsPubSub
}

func (s *NatsRequestDataSource) Load(ctx context.Context, input []byte, w io.Writer) error {
	subject, err := jsonparser.GetString(input, "subject")
	if err != nil {
		return err
	}

	return s.pubSub.Request(ctx, subject, nil, w)
}
