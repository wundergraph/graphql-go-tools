package operation_complexity

import (
	"fmt"
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
		name              string
		operation         string
		skipIntrospection bool
		rootFieldCounts   []int
	}{
		{
			name:            "scalar root aliases",
			operation:       `{ currentPeriod first: currentPeriod second: currentPeriod }`,
			rootFieldCounts: []int{1, 1, 1},
		},
		{
			name:            "root typename without schema field definition",
			operation:       `{ currentPeriod kind: __typename }`,
			rootFieldCounts: []int{1, 1},
		},
		{
			name:              "typename is counted when introspection is skipped",
			operation:         `{ __typename user(id: "1") { __typename id } }`,
			skipIntrospection: true,
			rootFieldCounts:   []int{1, 3},
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
		{
			name:            "node count skip excludes the subtree",
			operation:       `{ activeUsers { id name } user(id: "1") { id } }`,
			rootFieldCounts: []int{2},
		},
		{
			name:              "introspection excluded from mixed operation",
			operation:         `{ __schema { queryType { name } } currentPeriod }`,
			skipIntrospection: true,
			rootFieldCounts:   []int{1},
		},
		{
			name:            "introspection included",
			operation:       `{ __schema { queryType { name } } currentPeriod }`,
			rootFieldCounts: []int{3, 1},
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

			estimator := NewOperationComplexityEstimator(tt.skipIntrospection)
			// Reusing the estimator must reset both global and per-root counts.
			for range 2 {
				stats, roots := estimator.Do(&operation, &definition, &report)
				require.False(t, report.HasErrors(), report.Error())
				require.Len(t, roots, len(tt.rootFieldCounts))
				total := 0
				for i, count := range tt.rootFieldCounts {
					assert.Equal(t, count, roots[i].Stats.FieldCount)
					total += count
				}
				assert.Equal(t, total, stats.FieldCount)
			}
		})
	}
}

func TestOperationComplexityFieldCountWideQuery(t *testing.T) {
	t.Parallel()

	for _, leaves := range []int{1, 4, 8, 20, 50, 200} {
		t.Run(fmt.Sprintf("%d leaves", leaves), func(t *testing.T) {
			t.Parallel()

			var query strings.Builder
			query.WriteString(`{ user(id: "1") { address {`)
			for i := range leaves {
				fmt.Fprintf(&query, "field%d: city ", i)
			}
			query.WriteString(`} } }`)

			run(t, testDefinition, query.String(),
				OperationStats{FieldCount: leaves + 2, NodeCount: 2, Complexity: 2, Depth: 3},
				[]RootFieldStats{{
					TypeName:  "Query",
					FieldName: "user",
					Stats:     OperationStats{FieldCount: leaves + 2, NodeCount: 2, Complexity: 2, Depth: 2},
				}},
			)
		})
	}
}
