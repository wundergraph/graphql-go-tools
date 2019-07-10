package astvisitor

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/input"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer"
	"testing"
)

func TestVisit(t *testing.T) {
	raw := []byte(`
		query posts {
  			posts(first: 100) {
    			id
				description
  			}
		}`)

	in := &input.Input{}
	in.ResetInputBytes(raw)
	doc := &ast.Document{}

	parser := astparser.NewParser(&lexer.Lexer{})
	err := parser.Parse(in, doc)
	if err != nil {
		t.Fatal(err)
	}

	Visit(doc, &dummyVisitor{})
}

func BenchmarkVisitor(b *testing.B) {
	raw := []byte(`
		query posts {
  			posts(first: 100) {
    			id
				description
  			}
		}`)

	in := &input.Input{}
	in.ResetInputBytes(raw)
	doc := &ast.Document{}

	parser := astparser.NewParser(&lexer.Lexer{})
	err := parser.Parse(in, doc)
	if err != nil {
		b.Fatal(err)
	}

	visitor := &dummyVisitor{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		Visit(doc, visitor)
	}
}

type dummyVisitor struct {
}

func (_ dummyVisitor) Enter(node ast.NodeKind, ref int) {

}

func (_ dummyVisitor) Leave(node ast.NodeKind, ref int) {

}

func (_ dummyVisitor) EnterField(field ast.Field) {

}

func (_ dummyVisitor) LeaveField(field ast.Field) {

}
