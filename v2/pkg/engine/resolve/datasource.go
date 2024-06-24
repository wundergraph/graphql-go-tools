package resolve

import (
	"context"
	"io"

	"github.com/cespare/xxhash/v2"
)

type DataSource interface {
	Load(ctx context.Context, input []byte, w io.Writer) (err error)
}

type SubscriptionDataSource interface {
	// Start is called when a new subscription is created. It establishes the connection to the data source.
	// Start should be run in a separate goroutine to not block the main execution. The updater is used
	// to send updates to the client. Deduplication of the request must be done before calling this method.
	Start(ctx *Context, input []byte, updater SubscriptionUpdater) error
	UniqueRequestID(ctx *Context, input []byte, xxh *xxhash.Digest) (err error)
}
