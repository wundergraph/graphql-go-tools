package resolve

import (
	"context"
	"io"
)

type DataSource interface {
	Load(ctx context.Context, input []byte, w io.Writer) (err error)
}

type SubscriptionDataSource interface {
	Start(ctx context.Context, input []byte, next chan<- []byte) error
}
