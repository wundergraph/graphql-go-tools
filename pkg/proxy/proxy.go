package proxy

import (
	"bytes"
	"context"
	"io"
)

type Proxy interface {
	AcceptRequest(uri string, body io.ReadCloser, ctx context.Context) (*bytes.Buffer, error)
	DispatchRequest(input []byte) ([]byte, error)
	AcceptResponse()
	DispatchResponse()
}