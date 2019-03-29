package proxy

import (
	"bytes"
	"io"
)

type Proxy interface {
	AcceptRequest(contextValues map[string][]byte, requestURI []byte, body io.Reader, buff *bytes.Buffer) error
	DispatchRequest(buff *bytes.Buffer) (io.ReadCloser, error)
	AcceptResponse()
	DispatchResponse()
}
