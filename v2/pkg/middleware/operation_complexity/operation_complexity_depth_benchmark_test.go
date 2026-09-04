package operation_complexity

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func BenchmarkEstimateComplexityByDepth(b *testing.B) {
	definition := unsafeparser.ParseGraphqlDocumentString(`
		scalar String

		schema { query: Query }

		type Query { node: Node }
		type Node { child: Node leaf: String }
	`)

	for _, depth := range []int{19, 50} {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			operation := unsafeparser.ParseGraphqlDocumentString(operationWithDepth(depth))
			report := operationreport.Report{}
			stats, _ := NewOperationComplexityEstimator(false).Do(&operation, &definition, &report)
			if report.HasErrors() {
				b.Fatal(report.Error())
			}
			if stats.Depth != depth {
				b.Fatalf("want depth %d, got %d", depth, stats.Depth)
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				estimator := NewOperationComplexityEstimator(false)
				report := operationreport.Report{}
				stats, _ := estimator.Do(&operation, &definition, &report)
				if report.HasErrors() {
					b.Fatal(report.Error())
				}
				if stats.Depth != depth {
					b.Fatalf("want depth %d, got %d", depth, stats.Depth)
				}
			}
		})
	}
}

func operationWithDepth(depth int) string {
	if depth < 2 {
		panic("depth must be at least 2")
	}

	var operation strings.Builder
	operation.WriteString("{ node {")
	for i := 0; i < depth-2; i++ {
		operation.WriteString(" child {")
	}
	operation.WriteString(" leaf")
	for i := 0; i < depth-1; i++ {
		operation.WriteString(" }")
	}
	operation.WriteString(" }")

	return operation.String()
}
