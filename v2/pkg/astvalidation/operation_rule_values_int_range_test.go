package astvalidation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestValuesIntRange(t *testing.T) {
	const maxSafeInteger = 1<<53 - 1
	safeIntegerRange := WithIntRange(ast.IntRange{Min: -maxSafeInteger, Max: maxSafeInteger})
	const schema = `
		type Query {
			count(arg: Int!): Int
			optional(arg: Int): Int
		}`

	validate := func(t *testing.T, operation string, options ...Option) []string {
		t.Helper()
		definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(schema)
		op := unsafeparser.ParseGraphqlDocumentString(operation)
		report := operationreport.Report{}
		DefaultOperationValidator(options...).Validate(&op, &definition, &report)
		messages := make([]string, 0, len(report.ExternalErrors))
		for _, err := range report.ExternalErrors {
			messages = append(messages, err.Message)
		}
		return messages
	}

	t.Run("without WithIntRange", func(t *testing.T) {
		t.Run("rejects a literal above int32", func(t *testing.T) {
			assert.Equal(t,
				[]string{"Int cannot represent non 32-bit signed integer value: 3000000000"},
				validate(t, `{ count(arg: 3000000000) }`))
		})

		t.Run("rejects a variable default above int32", func(t *testing.T) {
			assert.Equal(t,
				[]string{"Int cannot represent non 32-bit signed integer value: 3000000000"},
				validate(t, `query Q($c: Int = 3000000000) { optional(arg: $c) }`))
		})
	})

	t.Run("with WithIntRange", func(t *testing.T) {
		t.Run("accepts a literal within the range", func(t *testing.T) {
			assert.Empty(t, validate(t, `{ count(arg: 3000000000) }`, safeIntegerRange))
		})

		t.Run("rejects a literal above the range", func(t *testing.T) {
			assert.Equal(t,
				[]string{"Int cannot represent integer value outside [-9007199254740991, 9007199254740991]: 9007199254740992"},
				validate(t, `{ count(arg: 9007199254740992) }`, safeIntegerRange))
		})

		t.Run("rejects a variable default outside the range", func(t *testing.T) {
			assert.Equal(t,
				[]string{"Int cannot represent integer value outside [-9007199254740991, 9007199254740991]: 9007199254740992"},
				validate(t, `query Q($c: Int = 9007199254740992) { optional(arg: $c) }`, safeIntegerRange))
		})
	})
}
