package graphql

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"io"
	"net/http"
	"strconv"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type EngineResultWriter struct {
	buf           *bytes.Buffer
	flushCallback func(data []byte)
}

var _ resolve.SubscriptionResponseWriter = (*EngineResultWriter)(nil)

func NewEngineResultWriter() EngineResultWriter {
	return EngineResultWriter{
		buf: &bytes.Buffer{},
	}
}

func NewEngineResultWriterFromBuffer(buf *bytes.Buffer) EngineResultWriter {
	return EngineResultWriter{
		buf: buf,
	}
}

func (e *EngineResultWriter) Complete() {

}

func (e *EngineResultWriter) Close(_ resolve.SubscriptionCloseKind) {

}

func (e *EngineResultWriter) SetFlushCallback(flushCb func(data []byte)) {
	e.flushCallback = flushCb
}

func (e *EngineResultWriter) Write(p []byte) (n int, err error) {
	return e.buf.Write(p)
}

func (e *EngineResultWriter) Read(p []byte) (n int, err error) {
	return e.buf.Read(p)
}

func (e *EngineResultWriter) Flush() error {
	if e.flushCallback != nil {
		e.flushCallback(e.Bytes())
	}

	e.Reset()
	return nil
}

func (e *EngineResultWriter) Len() int {
	return e.buf.Len()
}

func (e *EngineResultWriter) Bytes() []byte {
	return e.buf.Bytes()
}

func (e *EngineResultWriter) String() string {
	return e.buf.String()
}

func (e *EngineResultWriter) Reset() {
	e.buf.Reset()
}

func (e *EngineResultWriter) AsHTTPResponse(status int, headers http.Header) *http.Response {
	b := &bytes.Buffer{}

	switch headers.Get(httpclient.ContentEncodingHeader) {
	case "gzip":
		gzw := gzip.NewWriter(b)
		_, _ = gzw.Write(e.Bytes())
		_ = gzw.Close()
	case "deflate":
		fw, _ := flate.NewWriter(b, 1)
		_, _ = fw.Write(e.Bytes())
		_ = fw.Close()
	default:
		headers.Del(httpclient.ContentEncodingHeader) // delete unsupported compression header
		b = e.buf
	}

	res := &http.Response{}
	res.Body = io.NopCloser(b)
	res.Header = headers
	res.StatusCode = status
	res.ContentLength = int64(b.Len())
	res.Header.Set("Content-Length", strconv.Itoa(b.Len()))
	return res
}
