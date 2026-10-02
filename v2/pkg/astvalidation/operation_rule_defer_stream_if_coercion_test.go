package astvalidation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestDeferStreamOnValidOperationsIfCoercion(t *testing.T) {
	t.Run("[S1] nested subscription defer with an absent variable is rejected", func(t *testing.T) {
		report := normalizeWithDeferPrevalidation(t, testDefinition, `subscription S($d: Boolean) {
  subscribeDog {
    ... @defer(if: $d) { name }
  }
}`, `{}`, "")
		assert.Equal(t, operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
			Message: `directive "@defer" is not allowed on subscription operations`,
			Path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("subscription")},
				{Kind: ast.FieldName, FieldName: []byte("subscribeDog")},
			},
			Locations: []operationreport.Location{{Line: 3, Column: 9}},
		}}}, report)
	})

	t.Run("[S2] nested subscription stream with an absent variable is rejected", func(t *testing.T) {
		report := normalizeWithDeferPrevalidation(t, testDefinition, `subscription S($s: Boolean) {
  subscribeDog {
    extras @stream(if: $s) { string }
  }
}`, `{}`, "")
		assert.Equal(t, operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
			Message: `directive "@stream" is not allowed on subscription operations`,
			Path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("subscription")},
				{Kind: ast.FieldName, FieldName: []byte("subscribeDog")},
				{Kind: ast.FieldName, FieldName: []byte("extras")},
			},
			Locations: []operationreport.Location{{Line: 3, Column: 12}},
		}}}, report)
	})

	t.Run("[S3] nested subscription defer with a false variable is allowed", func(t *testing.T) {
		report := normalizeWithDeferPrevalidation(t, testDefinition, `subscription S($d: Boolean) {
  subscribeDog {
    ... @defer(if: $d) { name }
  }
}`, `{"d":false}`, "")
		assert.Equal(t, operationreport.Report{}, report)
	})

	t.Run("[S4] a reused normalizer forgets the operation of the previous document", func(t *testing.T) {
		definition := unsafeparser.ParseGraphqlDocumentString(testDefinition)
		normalizer := newDeferPrevalidationNormalizer("")

		first := unsafeparser.ParseGraphqlDocumentString(`query Q { dog { name } } subscription S { subscribeDog { name } }`)
		report := operationreport.Report{}
		normalizer.NormalizeOperation(&first, &definition, &report)
		require.Equal(t, operationreport.Report{}, report)

		second := unsafeparser.ParseGraphqlDocumentString(`fragment F on Dog { ... @defer(if: $d) { name } } query Q($d: Boolean) { dog { ...F } }`)
		second.Input.Variables = []byte(`{}`)
		normalizer.NormalizeOperation(&second, &definition, &report)
		assert.Equal(t, operationreport.Report{}, report)
	})
}
