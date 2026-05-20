package ast_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func TestPrintDescription(t *testing.T) {
	t.Run("preserves inner indentation beyond common indent", func(t *testing.T) {
		var doc ast.Document
		desc := doc.ImportDescription(`Outer.
    indented
        deeper
    back`)

		var buf bytes.Buffer
		require.NoError(t, doc.PrintDescription(desc, []byte("  "), 1, &buf))

		out := buf.String()
		// Common indent of non-first lines is 4 (per the spec's
		// BlockStringValue canonicalization), so the 4-leading-space lines
		// are flush against the depth prefix and the 8-leading-space line
		// keeps its 4 extra spaces.
		assert.Contains(t, out, "\n  indented\n")
		assert.Contains(t, out, "\n      deeper\n")
		assert.Contains(t, out, "\n  back\n")
	})

	t.Run("strips common indent contributed by source-level nesting", func(t *testing.T) {
		var doc ast.Document
		desc := doc.ImportDescription(`Outer.
  same
  same
`)

		var buf bytes.Buffer
		require.NoError(t, doc.PrintDescription(desc, []byte("  "), 1, &buf))

		// Lines 1+ all share a 2-space indent, so it is treated as the
		// source's outer indent and stripped, leaving only the depth prefix.
		assert.Contains(t, buf.String(), "\n  same\n  same\n")
	})

	t.Run("preserves verbatim content at depth 0", func(t *testing.T) {
		var doc ast.Document
		desc := doc.ImportDescription(`line one
  line two
line three`)

		var buf bytes.Buffer
		require.NoError(t, doc.PrintDescription(desc, []byte("  "), 0, &buf))

		// Common indent across non-first lines is 0 (line three has 0), so
		// nothing is stripped, and depth=0 means no prefix is added.
		out := buf.String()
		assert.Contains(t, out, "line one\n  line two\nline three")
	})
}
