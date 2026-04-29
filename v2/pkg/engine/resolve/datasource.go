package resolve

import (
	"context"
	"net/http"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

type DataSource interface {
	Load(ctx context.Context, headers http.Header, input []byte) (data []byte, err error)
	LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) (data []byte, err error)
}

// NativeDataSource is an optional extension for datasources that can return an
// arena-rooted JSON value directly instead of serialized bytes.
//
// The returned value remains valid until cleanup is called. The loader must call
// cleanup exactly once after it has finished reading the value.
type NativeDataSource interface {
	LoadValue(ctx context.Context, headers http.Header, input []byte) (value *astjson.Value, cleanup func(), err error)
	LoadWithFilesValue(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) (value *astjson.Value, cleanup func(), err error)
}

// NativeMergeResult is an optional result shape for datasources that can merge
// native results into the resolver's arena directly, without first materializing
// an intermediate astjson response tree per subgraph fetch.
type NativeMergeResult interface {
	MergeInto(a arena.Arena, items []*astjson.Value, postProcessing PostProcessingConfiguration, batchStats [][]*astjson.Value) (root *astjson.Value, err error)
	MarshalTo(dst []byte) []byte
}

// NativeMergeDataSource is an optional extension for datasources that can
// return a mergeable native result object directly.
//
// Returning a nil result tells the loader to continue with the next available
// datasource contract, e.g. NativeDataSource or the byte contract.
type NativeMergeDataSource interface {
	LoadResult(ctx context.Context, headers http.Header, input []byte) (result NativeMergeResult, cleanup func(), err error)
	LoadWithFilesResult(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) (result NativeMergeResult, cleanup func(), err error)
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
