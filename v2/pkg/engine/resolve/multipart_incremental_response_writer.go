package resolve

import (
	"bytes"
	"fmt"
	"io"
)

// TODO: should this go somewhere else?

// MultipartIncrementalResponseWriter is a writer that writes multipart incremental responses.
// Is assumes that all parts share the name contentType.
// Note that it is not smart enough to stream parts to the writer. Parts are written in bulk.
// Useful for testing and smaller applications.
type MultipartIncrementalResponseWriter struct {
	Writer        io.Writer
	BoundaryToken string
	ContentType   string

	buf bytes.Buffer
}

const (
	// DefaultBoundaryToken is the default boundary token used in multipart responses.
	DefaultBoundaryToken = "graphql-go-tools"
	// DefaultContentType is the default content type used in multipart responses.
	DefaultContentType = "application/json; charset=utf-8"
)

func (w *MultipartIncrementalResponseWriter) Write(p []byte) (n int, err error) {
	n, err = w.buf.Write(p)
	if err != nil {
		return n, fmt.Errorf("writing: %w", err)
	}
	return n, nil
}

func (w *MultipartIncrementalResponseWriter) Flush() error {
	if _, err := w.Writer.Write(w.partHeader()); err != nil {
		return fmt.Errorf("writing part header: %w", err)
	}
	defer w.buf.Reset()

	if _, err := w.buf.WriteTo(w.Writer); err != nil {
		return fmt.Errorf("writing buffer: %w", err)
	}
	return nil
}

func (w *MultipartIncrementalResponseWriter) Complete() error {
	if _, err := w.Writer.Write([]byte(fmt.Sprintf("\r\n--%s--\r\n", w.boundaryToken()))); err != nil {
		return fmt.Errorf("writing final boundary: %w", err)
	}
	return nil
}

func (w *MultipartIncrementalResponseWriter) partHeader() []byte {
	return []byte(fmt.Sprintf("\r\n--%s\r\n%s\r\n\r\n", w.contentType(), w.boundaryToken()))
}

func (w *MultipartIncrementalResponseWriter) boundaryToken() string {
	if len(w.BoundaryToken) == 0 {
		return DefaultBoundaryToken
	}
	return w.BoundaryToken
}

func (w *MultipartIncrementalResponseWriter) contentType() string {
	if len(w.ContentType) == 0 {
		return DefaultBoundaryToken
	}
	return w.ContentType
}
