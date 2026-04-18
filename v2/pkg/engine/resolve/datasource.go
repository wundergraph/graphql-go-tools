package resolve

import (
	"context"
	"net/http"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

// DataSource produces a GraphQL response fragment for a single fetch.
//
// # Why not just take the loader's arena?
//
// The loader runs fetches concurrently (errgroup fan-out). astjson arenas are
// intentionally single-writer for performance — passing the loader's arena to
// concurrent Loads would race on every allocation. Each DataSource.Load must
// therefore own its own arena (typically pooled inside the datasource) for the
// duration of the call. The loader reads from the returned Value, copies data
// onto its own arena as part of merging, then calls cleanup() to release the
// datasource-side resources.
//
// # Contract
//
//   - The returned *astjson.Value is rooted on the datasource's arena. It is
//     valid only until cleanup() is called.
//   - The loader MUST call cleanup() exactly once after it has finished reading
//     the Value. Failure to call cleanup() leaks arena memory from the pool.
//   - cleanup may be nil. A nil cleanup signals the datasource has no per-call
//     resources to release (e.g. a value rooted on a long-lived arena, or a
//     cheaply garbage-collected value).
//   - The returned Value may be nil (typically paired with a non-nil error).
//     cleanup is still returned in that case and still must be called if non-nil.
//   - It is safe for the loader to hold the Value across subsequent operations
//     that do NOT invalidate the datasource's arena (the cleanup boundary
//     defines the lifetime).
type DataSource interface {
	Load(ctx context.Context, headers http.Header, input []byte) (value *astjson.Value, cleanup func(), err error)
	LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) (value *astjson.Value, cleanup func(), err error)
}

type SubscriptionDataSource interface {
	// Start is called when a new subscription is created. It establishes the connection to the data source.
	// The updater is used to send updates to the client. Deduplication of the request must be done before calling this method.
	Start(ctx *Context, headers http.Header, input []byte, updater SubscriptionUpdater) error
}

type AsyncSubscriptionDataSource interface {
	AsyncStart(ctx *Context, id uint64, headers http.Header, input []byte, updater SubscriptionUpdater) error
	AsyncStop(id uint64)
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
