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

func (w *MultipartJSONWriter) Flush(path []any) (err error) {
	if w.buf.Len() == 0 {
		return nil
	}

	var part incrementalPart

	if err := json.Unmarshal(w.buf.Bytes(), &part); err != nil {
		return fmt.Errorf("unmarshaling data: %w", err)
	}
	part.HasNext = true

	if _, err := w.Writer.Write(w.partHeader()); err != nil {
		return fmt.Errorf("writing part header: %w", err)
	}
	defer w.buf.Reset()

	if w.wroteInitial {
		part.Incremental = []incrementalDataPart{
			{
				Data: part.Data,
				Path: path,
			},
		}
		part.Data = nil

		if len(path) > 0 {
			part.Incremental[0].Path = path
		}
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(part); err != nil {
		return fmt.Errorf("encoding part body: %w", err)
	}
	if buf.Len() > 0 {
		buf.Truncate(buf.Len() - 1) // remove trailing newline
	}
	if _, err := buf.WriteTo(w.Writer); err != nil {
		return fmt.Errorf("writing part body: %w", err)
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

// incrementalPart is a part of a multipart response.
// It can contain a full response (for the first or only part) in `data`, or an incremental part in `incremental`.
type incrementalPart struct {
	HasNext bool `json:"hasNext"`

	Data        json.RawMessage       `json:"data,omitempty"`
	Incremental []incrementalDataPart `json:"incremental,omitempty"`

	Errors     json.RawMessage `json:"errors,omitempty"`
	Extensions json.RawMessage `json:"extensions,omitempty"`
}

type incrementalDataPart struct {
	Data json.RawMessage `json:"data"`
	Path []any           `json:"path,omitempty"`
}
