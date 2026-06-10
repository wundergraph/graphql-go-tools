package resolve

import (
	"context"
	"net/http"

	"github.com/cespare/xxhash/v2"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

type DataSource interface {
	Load(ctx context.Context, headers http.Header, input []byte) (data []byte, err error)
	LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) (data []byte, err error)
}

type SubscriptionDataSource interface {
	// Start is called when a new subscription is created. It establishes the connection to the data source.
	// The updater is used to send updates to the client. Deduplication of the request must be done before calling this method.
	Start(ctx *Context, headers http.Header, input []byte, updater SubscriptionUpdater) error
}

// HookableSubscriptionDataSource is a hookable interface for subscription data sources.
// It is used to call a function when a subscription is started.
// This is useful for data sources that need to do some work when a subscription is started,
// e.g. to establish a connection to the data source or to emit updates to the client.
// The function is called with the context and the input of the subscription.
// The function is called before the subscription is started and can be used to emit updates to the client.
type HookableSubscriptionDataSource interface {
	// SubscriptionOnStart is called when a new subscription is created
	// If an error is returned, the error is propagated to the client.
	SubscriptionOnStart(ctx StartupHookContext, input []byte) (err error)
}

// SubscriptionTriggerHasher is an optional interface for subscription datasources
// that need to control which fields contribute to the trigger ID hash.
// When implemented, it replaces the default behaviour of hashing the full raw input.
// The datasource writes only its stable, identity-relevant fields into xxh.
// The resolver still appends the subgraph headers hash afterward.
type SubscriptionTriggerHasher interface {
	UniqueRequestID(ctx *Context, input []byte, xxh *xxhash.Digest) error
}
