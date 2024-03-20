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
	Start(ctx *Context, input []byte, updater SubscriptionUpdater) error
	UniqueRequestID(ctx *Context, input []byte, xxh *xxhash.Digest) (err error)
}
