package datasource

import (
	"context"
	"io"
)

type Client interface {
	Do(ctx context.Context, url, method, headers, body []byte, out io.Writer) (err error)
}
