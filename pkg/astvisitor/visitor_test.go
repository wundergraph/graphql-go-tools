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
	visitor.EXPECT().EnterOperationDefinition(0)
	visitor.EXPECT().EnterSelectionSet(0, gomock.Any())

	// posts ->
	visitor.EXPECT().EnterField(1, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
	visitor.EXPECT().EnterSelectionSet(gomock.Any(), gomock.Any())

	// id ->
	visitor.EXPECT().EnterField(2, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
	visitor.EXPECT().LeaveField(2, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())

	// description ->
	visitor.EXPECT().EnterField(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
	visitor.EXPECT().LeaveField(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())

	// <- posts
	visitor.EXPECT().LeaveSelectionSet(gomock.Any())
	visitor.EXPECT().LeaveField(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())

	// <- query
	visitor.EXPECT().LeaveSelectionSet(gomock.Any())
	visitor.EXPECT().LeaveOperationDefinition(gomock.Any())

	walker := Walker{}
	walker.Visit(doc, in, visitor)
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

	walker := Walker{
		ancestors: make([]ast.Node, 0, 48),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		walker.Visit(doc, in, visitor)
	}
}

type dummyVisitor struct {
}

func (d *dummyVisitor) EnterOperationDefinition(ref int) {

}

func (d *dummyVisitor) LeaveOperationDefinition(ref int) {

}

func (d *dummyVisitor) EnterSelectionSet(ref int, ancestors []ast.Node) {

}

func (d *dummyVisitor) LeaveSelectionSet(ref int) {

}

func (d *dummyVisitor) EnterField(ref int, ancestors []ast.Node, selectionSet int, selectionsBefore []int, selectionsAfter []int, hasSelections bool) {

}

func (d *dummyVisitor) LeaveField(ref int, ancestors []ast.Node, selectionSet int, selectionsBefore []int, selectionsAfter []int, hasSelections bool) {

}

func (d *dummyVisitor) EnterFragmentSpread(ref int, ancestors []ast.Node, selectionSet int, selectionsBefore []int, selectionsAfter []int) {

}

func (d *dummyVisitor) LeaveFragmentSpread(ref int) {

}

func (d *dummyVisitor) EnterInlineFragment(ref int, ancestors []ast.Node, selectionSet int, selectionsBefore []int, selectionsAfter []int, hasSelections bool) {

}

func (d *dummyVisitor) LeaveInlineFragment(ref int) {

}

func (d *dummyVisitor) EnterFragmentDefinition(ref int) {

}

func (d *dummyVisitor) LeaveFragmentDefinition(ref int) {

}
