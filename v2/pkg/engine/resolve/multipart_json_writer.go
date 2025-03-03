package resolve

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// TODO: should this go somewhere else?

// MultipartJSONWriter is a writer that writes multipart incremental responses.
// Is assumes that all parts share the name contentType.
// Note that it is not smart enough to stream parts to the writer. Parts are written in bulk.
// Useful for testing and smaller applications.
type MultipartJSONWriter struct {
	Writer        io.Writer
	BoundaryToken string

	buf          bytes.Buffer
	wroteInitial bool
}

var _ IncrementalResponseWriter = (*MultipartJSONWriter)(nil)

const (
	// DefaultBoundaryToken is the default boundary token used in multipart responses.
	DefaultBoundaryToken = "graphql-go-tools"

	jsonContentType = "application/json; charset=utf-8"
)

func (w *MultipartJSONWriter) Write(p []byte) (n int, err error) {
	n, err = w.buf.Write(p)
	if err != nil {
		return n, fmt.Errorf("writing: %w", err)
	}
	return n, nil
}

func (w *MultipartJSONWriter) Flush(path []any) error {
	if w.buf.Len() == 0 {
		return nil
	}

	if _, err := w.Writer.Write(w.partHeader()); err != nil {
		return fmt.Errorf("writing part header: %w", err)
	}
	defer w.buf.Reset()

	if w.wroteInitial {
		if _, err := w.Writer.Write([]byte(`{"hasNext":true,"incremental":[`)); err != nil {
			return fmt.Errorf("writing increment preamble: %w", err)
		}
	}

	if _, err := w.buf.WriteTo(w.Writer); err != nil {
		return fmt.Errorf("writing incremental data: %w", err)
	}

	if w.wroteInitial {
		if len(path) > 0 {
			if _, err := w.Writer.Write([]byte(`,"path":`)); err != nil {
				return fmt.Errorf("writing incremental trailer: %w", err)
			}

			var buf bytes.Buffer
			if err := json.NewEncoder(&buf).Encode(path); err != nil {
				return fmt.Errorf("encoding path: %w", err)
			}
			if buf.Len() > 0 {
				buf.Truncate(buf.Len() - 1) // remove trailing newline
			}
			if _, err := buf.WriteTo(w.Writer); err != nil {
				return fmt.Errorf("writing path: %w", err)
			}
		}

		if _, err := w.Writer.Write([]byte(`]}`)); err != nil {
			return fmt.Errorf("writing incremental trailer: %w", err)
		}
	}
	if _, err := w.Writer.Write([]byte("\r\n")); err != nil {
		return fmt.Errorf("writing part terminator: %w", err)
	}
	w.wroteInitial = true

	return nil
}

func (w *MultipartJSONWriter) Complete() error {
	if w.wroteInitial {
		// Kind of a hack, but should work.
		if _, err := w.Writer.Write([]byte(string(w.partHeader()) + `{"hasNext":false,"incremental":[]}` + "\r\n")); err != nil {
			return fmt.Errorf("writing final part: %w", err)
		}
	}
	if _, err := w.Writer.Write([]byte(fmt.Sprintf("--%s--\r\n", w.boundaryToken()))); err != nil {
		return fmt.Errorf("writing final boundary: %w", err)
	}
	return nil
}

func (w *MultipartJSONWriter) partHeader() []byte {
	return []byte(fmt.Sprintf("--%s\r\nContent-Type: %s\r\n\r\n", w.boundaryToken(), jsonContentType))
}

func (w *MultipartJSONWriter) boundaryToken() string {
	if len(w.BoundaryToken) == 0 {
		return DefaultBoundaryToken
	}
	return w.BoundaryToken
}
