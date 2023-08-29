package astvisitor_test

import (
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

func BenchmarkSimpleVisitor(b *testing.B) {

	definition := unsafeparser.ParseGraphqlDocumentString(testDefinition)
	operation := unsafeparser.ParseGraphqlDocumentString(testOperation)

	visitor := &dummyVisitor{}

	walker := astvisitor.NewSimpleWalker(48)
	walker.SetVisitor(visitor)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		must(walker.Walk(&operation, &definition))
	}
}
