package asttransform

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
)

func parseMergeOperation(t *testing.T, source string) *ast.Document {
	t.Helper()
	doc, report := astparser.ParseGraphqlDocumentString(source)
	require.False(t, report.HasErrors(), report.Error())
	return &doc
}

// mergeMember parses source into a member whose variables are renamed by
// appending suffix to every variable declared in the operation.
func mergeMember(t *testing.T, source, suffix, alias, includeVar string) OperationMergeMember {
	t.Helper()
	doc := parseMergeOperation(t, source)
	rename := map[string]string{}
	for _, ref := range doc.OperationDefinitions[0].VariableDefinitions.Refs {
		name := doc.VariableDefinitionNameString(ref)
		rename[name] = name + suffix
	}
	return OperationMergeMember{Document: doc, Alias: alias, IncludeVariable: includeVar, VariableRename: rename}
}

func printMergeCompact(t *testing.T, doc *ast.Document) string {
	t.Helper()
	out, err := astprinter.PrintString(doc)
	require.NoError(t, err)
	return out
}

func TestMergeOperationDocuments(t *testing.T) {
	const m1 = `query($representations: [_Any!]!, $first: Int){_entities(representations: $representations){... on Employee {__typename products(first: $first){upc}}}}`
	const m2 = `query($representations: [_Any!]!, $first: Int){_entities(representations: $representations){... on Employee {__typename notes(first: $first)}}}`

	t.Run("two members with overlapping variable names, anonymous", func(t *testing.T) {
		merged, err := MergeOperationDocuments("", []OperationMergeMember{
			mergeMember(t, m1, "_f1", "f1", "includeF1"),
			mergeMember(t, m2, "_f2", "f2", "includeF2"),
		})
		require.NoError(t, err)
		require.Equal(t, `query($representations_f1: [_Any!]!, $first_f1: Int, $includeF1: Boolean!, $representations_f2: [_Any!]!, $first_f2: Int, $includeF2: Boolean!){f1: _entities(representations: $representations_f1)@include(if: $includeF1) {... on Employee {__typename products(first: $first_f1){upc}}} f2: _entities(representations: $representations_f2)@include(if: $includeF2) {... on Employee {__typename notes(first: $first_f2)}}}`, printMergeCompact(t, merged))
	})

	t.Run("named operation", func(t *testing.T) {
		merged, err := MergeOperationDocuments("Q__multi_3_5", []OperationMergeMember{
			mergeMember(t, m1, "_f1", "f1", "includeF1"),
			mergeMember(t, m2, "_f2", "f2", "includeF2"),
		})
		require.NoError(t, err)
		compact := printMergeCompact(t, merged)
		require.Equal(t, `query Q__multi_3_5($representations_f1: [_Any!]!, $first_f1: Int, $includeF1: Boolean!, $representations_f2: [_Any!]!, $first_f2: Int, $includeF2: Boolean!){f1: _entities(representations: $representations_f1)@include(if: $includeF1) {... on Employee {__typename products(first: $first_f1){upc}}} f2: _entities(representations: $representations_f2)@include(if: $includeF2) {... on Employee {__typename notes(first: $first_f2)}}}`, compact)
	})

	t.Run("pretty printed output", func(t *testing.T) {
		merged, err := MergeOperationDocuments("", []OperationMergeMember{
			mergeMember(t, m1, "_f1", "f1", "includeF1"),
			mergeMember(t, m2, "_f2", "f2", "includeF2"),
		})
		require.NoError(t, err)
		pretty, err := astprinter.PrintStringIndent(merged, "  ")
		require.NoError(t, err)
		require.Equal(t, `query($representations_f1: [_Any!]!, $first_f1: Int, $includeF1: Boolean!, $representations_f2: [_Any!]!, $first_f2: Int, $includeF2: Boolean!){
  f1: _entities(representations: $representations_f1)@include(if: $includeF1) {
    ... on Employee {
      __typename
      products(first: $first_f1){
        upc
      }
    }
  }
  f2: _entities(representations: $representations_f2)@include(if: $includeF2) {
    ... on Employee {
      __typename
      notes(first: $first_f2)
    }
  }
}`, pretty)
	})

	t.Run("default values are preserved on renamed definitions", func(t *testing.T) {
		const src = `query($representations: [_Any!]!, $first: Int = 10){_entities(representations: $representations){... on Employee {__typename products(first: $first){upc}}}}`
		merged, err := MergeOperationDocuments("", []OperationMergeMember{
			mergeMember(t, src, "_f1", "f1", "includeF1"),
		})
		require.NoError(t, err)
		require.Equal(t, `query($representations_f1: [_Any!]!, $first_f1: Int = 10, $includeF1: Boolean!){f1: _entities(representations: $representations_f1)@include(if: $includeF1) {... on Employee {__typename products(first: $first_f1){upc}}}}`, printMergeCompact(t, merged))
	})

	t.Run("directives on fields and inline fragments are renamed", func(t *testing.T) {
		const src = `query($representations: [_Any!]!, $skipF: Boolean!){_entities(representations: $representations){... on Employee @skip(if: $skipF) {__typename name @include(if: $skipF)}}}`
		merged, err := MergeOperationDocuments("", []OperationMergeMember{
			mergeMember(t, src, "_f1", "f1", "includeF1"),
		})
		require.NoError(t, err)
		require.Equal(t, `query($representations_f1: [_Any!]!, $skipF_f1: Boolean!, $includeF1: Boolean!){f1: _entities(representations: $representations_f1)@include(if: $includeF1) {... on Employee @skip(if: $skipF_f1){__typename name @include(if: $skipF_f1)}}}`, printMergeCompact(t, merged))
	})

	t.Run("fragment spreads are rejected", func(t *testing.T) {
		const src = `query($representations: [_Any!]!){_entities(representations: $representations){...EmployeeFields}}`
		_, err := MergeOperationDocuments("", []OperationMergeMember{
			mergeMember(t, src, "_f1", "f1", "includeF1"),
		})
		require.Error(t, err)
	})

	t.Run("single-root-field violation is rejected", func(t *testing.T) {
		const src = `query($representations: [_Any!]!){_entities(representations: $representations){__typename} __typename}`
		_, err := MergeOperationDocuments("", []OperationMergeMember{
			mergeMember(t, src, "_f1", "f1", "includeF1"),
		})
		require.Error(t, err)
	})

	t.Run("nil document is rejected", func(t *testing.T) {
		_, err := MergeOperationDocuments("", []OperationMergeMember{{Alias: "f1", IncludeVariable: "includeF1"}})
		require.Error(t, err)
	})
}
