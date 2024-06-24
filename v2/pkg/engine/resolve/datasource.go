package resolve

import (
	"context"
	"io"

	"github.com/cespare/xxhash/v2"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

type DataSource interface {
	Load(ctx context.Context, input []byte, w io.Writer) (err error)
	LoadWithFiles(ctx context.Context, input []byte, files []httpclient.File, w io.Writer) (err error)
}

type SubscriptionDataSource interface {
	Start(ctx *Context, input []byte, updater SubscriptionUpdater) error
	UniqueRequestID(ctx *Context, input []byte, xxh *xxhash.Digest) (err error)
}
