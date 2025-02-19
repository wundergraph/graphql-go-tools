package resolve_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestMultipartIncrementalResponseWriter_Write(t *testing.T) {
	tests := []struct {
		name          string
		input         []byte
		expectedWrite int
		expectedErr   error
	}{
		{
			name:          "successful write",
			input:         []byte("test data"),
			expectedWrite: 9,
			expectedErr:   nil,
		},
		{
			name:          "empty input",
			input:         []byte(""),
			expectedWrite: 0,
			expectedErr:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writer := &resolve.MultipartIncrementalResponseWriter{
				Writer: &buf,
			}

			n, err := writer.Write(tt.input)
			assert.Equal(t, tt.expectedWrite, n)
			assert.Equal(t, tt.expectedErr, err)
		})
	}
}

func TestMultipartIncrementalResponseWriter_Flush(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		expectedErr error
	}{
		{
			name:        "successful flush",
			input:       []byte("test data"),
			expectedErr: nil,
		},
		{
			name:        "flush with empty buffer",
			input:       []byte(""),
			expectedErr: nil,
		},
		{
			name:        "flush with error",
			input:       []byte("error data"),
			expectedErr: errors.New("flush error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w io.Writer
			if tt.expectedErr != nil {
				w = &errorWriter{
					err: tt.expectedErr,
				}
			} else {
				w = &bytes.Buffer{}
			}
			writer := &resolve.MultipartIncrementalResponseWriter{
				Writer: w,
			}

			_, err := writer.Write(tt.input)
			require.NoError(t, err)

			err = writer.Flush()
			assert.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

func TestMultipartIncrementalResponseWriter_Complete(t *testing.T) {
	tests := []struct {
		name        string
		expectedErr error
	}{
		{
			name:        "success",
			expectedErr: nil,
		},
		{
			name:        "error",
			expectedErr: errors.New("write error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w io.Writer
			if tt.expectedErr != nil {
				w = &errorWriter{
					err: tt.expectedErr,
				}
			} else {
				w = &bytes.Buffer{}
			}
			writer := &resolve.MultipartIncrementalResponseWriter{
				Writer: w,
			}

			err := writer.Complete()
			assert.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

type errorWriter struct {
	err error
}

func (e *errorWriter) Write(p []byte) (n int, err error) {
	return 0, e.err
}
