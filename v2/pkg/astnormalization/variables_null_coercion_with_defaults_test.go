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

	t.Run("variable type already non-null - split uses nullable type", func(t *testing.T) {
		// Simulates what happens after extractVariablesDefaultValue makes
		// the variable type non-null (because it had a default value).
		// The split variable must still be nullable to allow "not provided".
		runNullCoercionTest(t, nullCoercionTestSchema,
			`query ($limit: Int!) { items_page(limit: $limit) { items { id } } }`,
			`{"limit":null}`,
			`query ($limit_ndf_0: Int) { items_page(limit: $limit_ndf_0) { items { id } } }`,
			`{}`,
		)
	})

	t.Run("full VariablesNormalizer flow - variable with default and null value", func(t *testing.T) {
		// This reproduces the exact user scenario:
		// query ($itemsLimit: Int = 5) { items_page(limit: $itemsLimit) { ... } }
		// with variables {"itemsLimit": null}
		//
		// extractVariablesDefaultValue runs first and:
		// 1. Removes the default from AST
		// 2. Skips injecting default into JSON (because null IS provided)
		// 3. Makes variable type Int! (because it had a default and is used in non-null pos)
		//
		// Then our visitor runs and splits the variable with a nullable type.
		definitionDocument := unsafeparser.ParseGraphqlDocumentString(nullCoercionTestSchema)
		err := asttransform.MergeDefinitionWithBaseSchema(&definitionDocument)
		if err != nil {
			t.Fatal(err)
		}

		operationDocument := unsafeparser.ParseGraphqlDocumentString(
			`query GetItems($itemsLimit: Int = 5) { items_page(limit: $itemsLimit) { items { id } } }`)
		operationDocument.Input.Variables = []byte(`{"itemsLimit":null}`)

		report := &operationreport.Report{}
		normalizer := NewVariablesNormalizer()
		normalizer.NormalizeOperation(&operationDocument, &definitionDocument, report)
		if report.HasErrors() {
			t.Fatal(report.Error())
		}

		actualOperation := mustString(astprinter.PrintString(&operationDocument))
		// After normalization: the original variable gets type Int! (from extractVariablesDefaultValue)
		// and is unreferenced (cleaned up), and the split variable is Int (nullable, not provided)
		assert.Contains(t, actualOperation, "$itemsLimit_ndf_0: Int")
		assert.Contains(t, actualOperation, "limit: $itemsLimit_ndf_0")
		assert.NotContains(t, actualOperation, "$itemsLimit_ndf_0: Int!")

		// The null value should be removed from variables
		assert.Equal(t, "{}", string(operationDocument.Input.Variables))
	})
}
