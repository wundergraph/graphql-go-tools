package astnormalization

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

const nullCoercionTestSchema = `
	schema { query: Query }
	type Query {
		items_page(limit: Int! = 25): ItemsPage
		search(query: String!, limit: Int! = 10, offset: Int = 0): SearchResult
		mixed(a: Int! = 5, b: Int): MixedResult
		multi_default(x: Int! = 1, y: Int! = 2): MultiResult
	}
	type ItemsPage { items: [Item] }
	type Item { id: ID }
	type SearchResult { results: [Item] }
	type MixedResult { value: Int }
	type MultiResult { value: Int }
`

func runNullCoercionTest(t *testing.T, definition, operation, variablesInput, expectedOperation, expectedVariables string) {
	t.Helper()

	definitionDocument := unsafeparser.ParseGraphqlDocumentString(definition)
	err := asttransform.MergeDefinitionWithBaseSchema(&definitionDocument)
	if err != nil {
		t.Fatal(err)
	}

	operationDocument := unsafeparser.ParseGraphqlDocumentString(operation)
	if variablesInput != "" {
		operationDocument.Input.Variables = []byte(variablesInput)
	}

	walker := astvisitor.NewWalker(48)
	coerceNullVariablesWithDefaults(&walker)

	report := &operationreport.Report{}
	walker.Walk(&operationDocument, &definitionDocument, report)
	if report.HasErrors() {
		t.Fatal(report.Error())
	}

	actualOperation := mustString(astprinter.PrintString(&operationDocument))
	expectedOpDoc := unsafeparser.ParseGraphqlDocumentString(expectedOperation)
	expectedOp := mustString(astprinter.PrintString(&expectedOpDoc))
	assert.Equal(t, expectedOp, actualOperation)
	assert.Equal(t, expectedVariables, string(operationDocument.Input.Variables))
}

func TestNullVariableCoercionWithDefaults(t *testing.T) {
	t.Run("null variable for non-null argument with default - splits variable", func(t *testing.T) {
		runNullCoercionTest(t, nullCoercionTestSchema,
			`query ($limit: Int) { items_page(limit: $limit) { items { id } } }`,
			`{"limit":null}`,
			`query ($limit_ndf_0: Int) { items_page(limit: $limit_ndf_0) { items { id } } }`,
			`{}`,
		)
	})

	t.Run("non-null variable value - no change", func(t *testing.T) {
		runNullCoercionTest(t, nullCoercionTestSchema,
			`query ($limit: Int) { items_page(limit: $limit) { items { id } } }`,
			`{"limit":10}`,
			`query ($limit: Int) { items_page(limit: $limit) { items { id } } }`,
			`{"limit":10}`,
		)
	})

	t.Run("variable not provided - no change", func(t *testing.T) {
		runNullCoercionTest(t, nullCoercionTestSchema,
			`query ($limit: Int) { items_page(limit: $limit) { items { id } } }`,
			`{}`,
			`query ($limit: Int) { items_page(limit: $limit) { items { id } } }`,
			`{}`,
		)
	})

	t.Run("mixed usage - splits only non-null-with-default argument", func(t *testing.T) {
		// $val is used in both:
		// - mixed.a: Int! = 5 (non-null with default) → should be split
		// - mixed.b: Int (nullable) → should keep null
		runNullCoercionTest(t, nullCoercionTestSchema,
			`query ($val: Int) { mixed(a: $val, b: $val) { value } }`,
			`{"val":null}`,
			`query ($val: Int, $val_ndf_0: Int) { mixed(a: $val_ndf_0, b: $val) { value } }`,
			`{"val":null}`,
		)
	})

	t.Run("multiple non-null-with-default arguments same variable", func(t *testing.T) {
		runNullCoercionTest(t, nullCoercionTestSchema,
			`query ($n: Int) { multi_default(x: $n, y: $n) { value } }`,
			`{"n":null}`,
			`query ($n_ndf_0: Int, $n_ndf_1: Int) { multi_default(x: $n_ndf_0, y: $n_ndf_1) { value } }`,
			`{}`,
		)
	})

	t.Run("nullable argument with null variable - no change", func(t *testing.T) {
		// offset: Int = 0 is nullable (no !), so null is valid
		runNullCoercionTest(t, nullCoercionTestSchema,
			`query ($off: Int) { search(query: "test", offset: $off) { results { id } } }`,
			`{"off":null}`,
			`query ($off: Int) { search(query: "test", offset: $off) { results { id } } }`,
			`{"off":null}`,
		)
	})

	t.Run("non-null argument without default - no change", func(t *testing.T) {
		// query: String! has no default, so we don't touch it
		runNullCoercionTest(t, nullCoercionTestSchema,
			`query ($q: String) { search(query: $q) { results { id } } }`,
			`{"q":null}`,
			`query ($q: String) { search(query: $q) { results { id } } }`,
			`{"q":null}`,
		)
	})

	t.Run("literal argument value - no change", func(t *testing.T) {
		runNullCoercionTest(t, nullCoercionTestSchema,
			`query { items_page(limit: 5) { items { id } } }`,
			`{}`,
			`query { items_page(limit: 5) { items { id } } }`,
			`{}`,
		)
	})
}
