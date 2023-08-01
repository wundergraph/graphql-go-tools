package astvisitor

import (
	"testing"

	"github.com/wundergraph/graphql-go-tools/internal/pkg/unsafeparser"
)

func BenchmarkSimpleVisitor(b *testing.B) {

	definition := unsafeparser.ParseGraphqlDocumentString(testDefinition)
	operation := unsafeparser.ParseGraphqlDocumentString(testOperation)

	visitor := &dummyVisitor{}

	walker := NewSimpleWalker(48)
	walker.SetVisitor(visitor)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		must(walker.Walk(&operation, &definition))
	}
}
