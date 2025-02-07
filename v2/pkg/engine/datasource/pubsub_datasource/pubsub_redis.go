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

type RedisEventConfiguration struct {
	Channels []string `json:"channels"`
}

type RedisConnector interface {
	New(ctx context.Context) Redis
}

// Redis describe the interface that implements the primitive operations for pubsub
type Redis interface {
	// Subscribe starts listening on the given channels and sends the received messages to the given next channel
	Subscribe(ctx context.Context, event RedisSubscriptionEventConfiguration, updater resolve.SubscriptionUpdater) error
	// Publish sends the given data to the given channel
	Publish(ctx context.Context, event RedisPublishEventConfiguration) error
}

type RedisSubscriptionSource struct {
	pubSub Redis
}

func (s *RedisSubscriptionSource) UniqueRequestID(ctx *resolve.Context, input []byte, xxh *xxhash.Digest) error {

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

func (s *RedisSubscriptionSource) Start(ctx *resolve.Context, input []byte, updater resolve.SubscriptionUpdater) error {
	var subscriptionConfiguration RedisSubscriptionEventConfiguration
	err := json.Unmarshal(input, &subscriptionConfiguration)
	if err != nil {
		return err
	}

	return s.pubSub.Subscribe(ctx.Context(), subscriptionConfiguration, updater)
}

type RedisPublishDataSource struct {
	pubSub Redis
}

func (s *RedisPublishDataSource) Load(ctx context.Context, input []byte, out *bytes.Buffer) error {
	var publishConfiguration RedisPublishEventConfiguration
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

func (s *RedisPublishDataSource) LoadWithFiles(ctx context.Context, input []byte, files []httpclient.File, out *bytes.Buffer) error {
	panic("not implemented")
}
