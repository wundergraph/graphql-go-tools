package ast_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
)

func TestDocument_OperationNameExists(t *testing.T) {
	run := func(schema string, operationName string, expectedExists bool) func(t *testing.T) {
		return func(t *testing.T) {
			doc := unsafeparser.ParseGraphqlDocumentString(schema)
			exists := doc.OperationNameExists(operationName)
			assert.Equal(t, expectedExists, exists)
		}
	}

	t.Run("not found on empty document", run(
		"",
		"MyOperation",
		false,
	))

	t.Run("not found on document with multiple operations", run(
		"query OtherOperation {other} query AnotherOperation {another}",
		"MyOperation",
		false,
	))

	t.Run("found on document with a single operations", run(
		"query MyOperation {}",
		"MyOperation",
		true,
	))

	t.Run("found on document with multiple operations", run(
		"query OtherOperation {other} query AnotherOperation {another} query MyOperation {}",
		"MyOperation",
		true,
	))

	t.Run("found on a document with preceeding root nodes of not operation type", run(
		"fragment F on T {field} query MyOperation {}",
		"MyOperation",
		true,
	))
}
