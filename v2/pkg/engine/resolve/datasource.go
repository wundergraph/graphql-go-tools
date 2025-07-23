package resolve

import (
	"bytes"
	"context"

	"github.com/cespare/xxhash/v2"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

type DataSource interface {
	Load(ctx context.Context, input []byte, out *bytes.Buffer) (err error)
	LoadWithFiles(ctx context.Context, input []byte, files []*httpclient.FileUpload, out *bytes.Buffer) (err error)
}

type SubscriptionDataSource interface {
	// Start is called when a new subscription is created. It establishes the connection to the data source.
	// The updater is used to send updates to the client. Deduplication of the request must be done before calling this method.
	Start(ctx *Context, input []byte, updater SubscriptionUpdater) error
	UniqueRequestID(ctx *Context, input []byte, xxh *xxhash.Digest) (err error)
}

type AsyncSubscriptionDataSource interface {
	AsyncStart(ctx *Context, id uint64, input []byte, updater SubscriptionUpdater) error
	AsyncStop(id uint64)
	UniqueRequestID(ctx *Context, input []byte, xxh *xxhash.Digest) (err error)
}

// HookableSubscriptionDataSource is a hookable interface for subscription data sources.
// It is used to call a function when a subscription is started.
// This is useful for data sources that need to do some work when a subscription is started,
// e.g. to establish a connection to the data source or to emit updates to the client.
// The function is called with the context and the input of the subscription.
// The function is called before the subscription is started and can be used to emit updates to the client.
type HookableSubscriptionDataSource interface {
	// SubscriptionOnStart is called when a new subscription is created
	// If close is true, the subscription is closed.
	// If an error is returned, the error is propagated to the client.
	SubscriptionOnStart(ctx *Context, input []byte) (close bool, err error)
}
