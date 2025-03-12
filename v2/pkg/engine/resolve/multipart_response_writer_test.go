package resolve_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestMultipartJSONWriter(t *testing.T) {
	tests := []struct {
		name          string
		boundaryToken string
		path          []any
		parts         [][]string
		expected      string
	}{
		{
			name:          "simple case",
			boundaryToken: "boundary",
			path:          []any{"path", "to", "part"},
			parts:         [][]string{[]string{`{"data":"part1"}`}, []string{`{"data":"part2"}`}, []string{`{"data":"part3"}`}},
			expected: strings.ReplaceAll(`--boundary
Content-Type: application/json; charset=utf-8

{"hasNext":true,"data":"part1"}
--boundary
Content-Type: application/json; charset=utf-8

{"hasNext":true,"incremental":[{"data":"part2","path":["path","to","part"]}]}
--boundary
Content-Type: application/json; charset=utf-8

{"hasNext":true,"incremental":[{"data":"part3","path":["path","to","part"]}]}
--boundary
Content-Type: application/json; charset=utf-8

{"hasNext":false,"incremental":[]}
--boundary--
`, "\n", "\r\n"),
		},
		{
			name:          "multiple writes",
			boundaryToken: "boundary",
			path:          []any{"path", 4, "part"},
			parts:         [][]string{[]string{`{"data":`, `"part1a`, `part1b"}`}, []string{`{"data":"part2"}`}, []string{`{"data":"part3a`, `part3b"}`}},
			expected: strings.ReplaceAll(`--boundary
Content-Type: application/json; charset=utf-8

{"hasNext":true,"data":"part1apart1b"}
--boundary
Content-Type: application/json; charset=utf-8

{"hasNext":true,"incremental":[{"data":"part2","path":["path",4,"part"]}]}
--boundary
Content-Type: application/json; charset=utf-8

{"hasNext":true,"incremental":[{"data":"part3apart3b","path":["path",4,"part"]}]}
--boundary
Content-Type: application/json; charset=utf-8

{"hasNext":false,"incremental":[]}
--boundary--
`, "\n", "\r\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writer := &resolve.MultipartJSONWriter{
				Writer:        &buf,
				BoundaryToken: tt.boundaryToken,
			}

			for _, part := range tt.parts {
				for _, p := range part {
					_, err := writer.Write([]byte(p))
					require.NoError(t, err)
				}
				err := writer.Flush(tt.path)
				require.NoError(t, err)
			}

			err := writer.Complete()
			require.NoError(t, err)

			assert.Equal(t, tt.expected, buf.String())
		})
	}
}

func TestMultipartJSONWriter_Write(t *testing.T) {
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
			writer := &resolve.MultipartJSONWriter{
				Writer: &buf,
			}

			n, err := writer.Write(tt.input)
			assert.Equal(t, tt.expectedWrite, n)
			assert.Equal(t, tt.expectedErr, err)
		})
	}
}

func TestMultipartJSONWriter_Flush(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		expectedErr error
	}{
		{
			name:        "successful flush",
			input:       []byte(`{"data":"test data"}`),
			expectedErr: nil,
		},
		{
			name:        "flush with empty buffer",
			input:       []byte(""),
			expectedErr: nil,
		},
		{
			name:        "flush with error",
			input:       []byte(`{"error": "data"}`),
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
			writer := &resolve.MultipartJSONWriter{
				Writer: w,
			}

			_, err := writer.Write(tt.input)
			require.NoError(t, err)

			err = writer.Flush(nil)
			assert.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

func TestMultipartJSONWriter_Complete(t *testing.T) {
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
			writer := &resolve.MultipartJSONWriter{
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
