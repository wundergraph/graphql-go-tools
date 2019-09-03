package fastastvisitor

import (
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"testing"
)

func BenchmarkSimpleVisitor(b *testing.B) {

	definition := mustDoc(astparser.ParseGraphqlDocumentString(testDefinition))
	operation := mustDoc(astparser.ParseGraphqlDocumentString(testOperation))

	visitor := &dummyVisitor{}

	walker := NewSimpleWalker(48)
	walker.SetVisitor(visitor)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		must(walker.Walk(operation, definition))
	}
}
