package resolve

import (
	"context"
	"io"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastbuffer"
)

type DataSourceBatchFactory interface {
	CreateBatch(inputs [][]byte) (DataSourceBatch, error)
}

type DataSourceBatch interface {
	Demultiplex(responseBufPair *BufPair, outputBuffers []*BufPair) (err error)
	Input() *fastbuffer.FastBuffer
}

type DataSource interface {
	Load(ctx context.Context, input []byte, w io.Writer) (err error)
}

type SubscriptionDataSource interface {
	Start(ctx context.Context, input []byte, next chan<- []byte) error
}
