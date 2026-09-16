package operation_complexity

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestOperationComplexityFieldCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		operation       string
		rootFieldCounts []int
	}{
		{
			name:            "scalar roots, aliases, and typename with introspection skipped",
			operation:       `{ currentPeriod first: currentPeriod kind: __typename user(id: "1") { __typename id } }`,
			rootFieldCounts: []int{1, 1, 1, 3},
		},
		{
			name: "fragment reused under distinct fields",
			operation: `{
				first: user(id: "1") { ...UserFields }
				second: user(id: "2") { ...UserFields }
			}
			fragment UserFields on User { id name }
			fragment Unused on User { balance email }`,
			rootFieldCounts: []int{3, 3},
		},
		{
			name: "merged fields are counted once",
			operation: `{
				user(id: "1") { id ... on User { id } ...UserFields }
			}
			fragment UserFields on User { id }`,
			rootFieldCounts: []int{2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			definition := unsafeparser.ParseGraphqlDocumentString(testDefinition)
			operation := unsafeparser.ParseGraphqlDocumentString(tt.operation)
			report := operationreport.Report{}
			astnormalization.NormalizeOperation(&operation, &definition, &report)
			require.False(t, report.HasErrors(), report.Error())

			stats, roots := NewOperationComplexityEstimator(true).Do(&operation, &definition, &report)
			require.False(t, report.HasErrors(), report.Error())
			require.Len(t, roots, len(tt.rootFieldCounts))
			total := 0
			for i, count := range tt.rootFieldCounts {
				assert.Equal(t, count, roots[i].Stats.FieldCount)
				total += count
			}
			assert.Equal(t, total, stats.FieldCount)
		})
	}
}

func TestOperationComplexityNestedRootType(t *testing.T) {
	t.Parallel()

	const definition = `schema { query: Query } type Query { self: Query id: String } scalar String`
	for _, field := range []string{"__typename", "id"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			run(t, definition, `{ self { `+field+` } }`,
				OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 2},
				[]RootFieldStats{{
					TypeName:  "Query",
					FieldName: "self",
					Stats:     OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 1},
				}},
			)
		})
	}
}

func TestOperationComplexityFieldNamesSurviveOperationReuse(t *testing.T) {
	t.Parallel()

	definition := unsafeparser.ParseGraphqlDocumentString(testDefinition)
	operation := unsafeparser.ParseGraphqlDocumentString(`{ currentPeriod __typename }`)
	report := operationreport.Report{}
	astnormalization.NormalizeOperation(&operation, &definition, &report)
	require.False(t, report.HasErrors(), report.Error())

	_, roots := NewOperationComplexityEstimator(false).Do(&operation, &definition, &report)
	require.False(t, report.HasErrors(), report.Error())
	require.Len(t, roots, 2)

	// Overwrite the operation's reused input buffer while retaining the results.
	operation.Input.ResetInputString(strings.Repeat("x", len(operation.Input.RawBytes)))
	assert.Equal(t, "currentPeriod", roots[0].FieldName)
	assert.Equal(t, "__typename", roots[1].FieldName)
}
