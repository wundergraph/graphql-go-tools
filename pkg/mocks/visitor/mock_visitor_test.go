package mock_astvisitor

import (
	"github.com/golang/mock/gomock"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/input"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer"
	"testing"
)

func TestVisit(t *testing.T) {
	raw := []byte(`
		query {
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

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	visitor := NewMockVisitor(ctrl)

	// query ->
	visitor.EXPECT().Enter(ast.NodeKindOperation, 0)
	visitor.EXPECT().Enter(ast.NodeKindSelectionSet, -1)

	// posts ->
	visitor.EXPECT().Enter(ast.NodeKindField, 2)
	visitor.EXPECT().EnterField(gomock.Any())
	visitor.EXPECT().Enter(ast.NodeKindSelectionSet, -1)

	// id
	visitor.EXPECT().Enter(ast.NodeKindField, 0)
	visitor.EXPECT().EnterField(gomock.Any())
	visitor.EXPECT().LeaveField(gomock.Any())
	visitor.EXPECT().Leave(ast.NodeKindField, 0)

	// description
	visitor.EXPECT().Enter(ast.NodeKindField, 1)
	visitor.EXPECT().EnterField(gomock.Any())
	visitor.EXPECT().LeaveField(gomock.Any())
	visitor.EXPECT().Leave(ast.NodeKindField, 1)

	// posts <-
	visitor.EXPECT().LeaveField(gomock.Any())
	visitor.EXPECT().Leave(ast.NodeKindSelectionSet, -1)
	visitor.EXPECT().Leave(ast.NodeKindField, 2)

	// query <-
	visitor.EXPECT().Leave(ast.NodeKindSelectionSet, -1)
	visitor.EXPECT().Leave(ast.NodeKindOperation, 0)

	astvisitor.Visit(doc, visitor)
}
