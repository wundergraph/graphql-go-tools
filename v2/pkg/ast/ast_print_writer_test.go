package ast

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errWriteFailed = errors.New("write failed")

// failingWriter makes an error at write number failAt, and at all writes
// after it. The count starts at 0.
type failingWriter struct {
	failAt  int
	written int
}

func (f *failingWriter) Write(p []byte) (int, error) {
	if f.written >= f.failAt {
		return 0, errWriteFailed
	}
	f.written++
	return len(p), nil
}

// failOnceWriter makes an error only at write number failAt. All other writes
// are correct. This shows if a subsequent write removes the error.
type failOnceWriter struct {
	failAt  int
	written int
}

func (f *failOnceWriter) Write(p []byte) (int, error) {
	n := f.written
	f.written++
	if n == f.failAt {
		return 0, errWriteFailed
	}
	return len(p), nil
}

func TestPrintDescriptionWriteError(t *testing.T) {
	doc := NewDocument()
	desc := doc.ImportDescription("line one\nline two")

	// Make an error at the first write, and at a write in the content loop.
	for _, failAt := range []int{0, 5} {
		w := &failingWriter{failAt: failAt}
		require.ErrorIs(t, doc.PrintDescription(desc, []byte("  "), 1, w), errWriteFailed)
	}

	// A subsequent write must not remove the error.
	for failAt := range 4 {
		w := &failOnceWriter{failAt: failAt}
		require.ErrorIs(t, doc.PrintDescription(desc, []byte("  "), 1, w), errWriteFailed, "failAt=%d", failAt)
	}
}

func TestPrintValueWriteError(t *testing.T) {
	doc := NewDocument()
	doc.StringValues = append(doc.StringValues, StringValue{
		Content: doc.Input.AppendInputString("foo"),
	})
	doc.Values = append(doc.Values, Value{Kind: ValueKindString, Ref: 0})
	doc.ListValues = append(doc.ListValues, ListValue{Refs: []int{0}})
	list := Value{Kind: ValueKindList, Ref: 0}

	// failAt 0 makes an error at the bracket. failAt 2 makes an error in the
	// nested PrintValue call.
	for _, failAt := range []int{0, 2} {
		w := &failingWriter{failAt: failAt}
		require.ErrorIs(t, doc.PrintValue(list, w), errWriteFailed)
	}

	// A subsequent write must not remove the error.
	for failAt := range 4 {
		w := &failOnceWriter{failAt: failAt}
		require.ErrorIs(t, doc.PrintValue(list, w), errWriteFailed, "failAt=%d", failAt)
	}

	t.Run("succeeds when the writer does not fail", func(t *testing.T) {
		w := &failingWriter{failAt: 1000}
		assert.NoError(t, doc.PrintValue(list, w))
	})
}
