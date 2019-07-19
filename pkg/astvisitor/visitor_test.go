package astvisitor

import (
	"github.com/golang/mock/gomock"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/input"
	"github.com/jensneuse/graphql-go-tools/pkg/mocks/visitor"
	"testing"
)

func TestVisit(t *testing.T) {
	raw := []byte(`
		query postsQuery {
  			posts(first: 100) {
    			id
				description
  			}
		}`)

	in := &input.Input{}
	in.ResetInputBytes(raw)
	doc := ast.NewDocument()

	parser := astparser.NewParser()
	err := parser.Parse(in, doc)
	if err != nil {
		t.Fatal(err)
	}

	controller := gomock.NewController(t)
	defer controller.Finish()

	visitor := mock_astvisitor.NewMockVisitor(controller)

	// query ->
	visitor.EXPECT().Enter(ast.NodeKindOperationDefinition, gomock.Any())
	visitor.EXPECT().Enter(ast.NodeKindSelectionSet, gomock.Any())

	// posts ->
	visitor.EXPECT().Enter(ast.NodeKindField, gomock.Any())
	visitor.EXPECT().Enter(ast.NodeKindSelectionSet, gomock.Any())

	// id ->
	visitor.EXPECT().Enter(ast.NodeKindField, gomock.Any())
	visitor.EXPECT().Leave(ast.NodeKindField, gomock.Any())

	// description ->
	visitor.EXPECT().Enter(ast.NodeKindField, gomock.Any())
	visitor.EXPECT().Leave(ast.NodeKindField, gomock.Any())

	// <- posts
	visitor.EXPECT().Leave(ast.NodeKindSelectionSet, gomock.Any())
	visitor.EXPECT().Leave(ast.NodeKindField, gomock.Any())

	// <- query
	visitor.EXPECT().Leave(ast.NodeKindSelectionSet, gomock.Any())
	visitor.EXPECT().Leave(ast.NodeKindOperationDefinition, gomock.Any())

	Visit(doc, visitor)
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
	doc := ast.NewDocument()

	parser := astparser.NewParser()
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
