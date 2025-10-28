package resolve

import (
	"context"
	"net/http"

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

type AsyncSubscriptionDataSource interface {
	AsyncStart(ctx *Context, id uint64, headers http.Header, input []byte, updater SubscriptionUpdater) error
	AsyncStop(id uint64)
}
