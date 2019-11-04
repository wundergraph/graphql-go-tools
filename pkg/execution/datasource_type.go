package execution

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"io"
)

type TypeDataSourcePlanner struct {
	walker                *astvisitor.Walker
	operation, definition *ast.Document
	args                  []Argument
}

func (t *TypeDataSourcePlanner) DirectiveName() []byte {
	return []byte("resolveType")
}

func (t *TypeDataSourcePlanner) Initialize(walker *astvisitor.Walker, operation, definition *ast.Document, args []Argument, resolverParameters []ResolverParameter) {
	t.walker, t.operation, t.definition, t.args = walker, operation, definition, args
}

func (t *TypeDataSourcePlanner) EnterInlineFragment(ref int) {

}

func (t *TypeDataSourcePlanner) LeaveInlineFragment(ref int) {

}

func (t *TypeDataSourcePlanner) EnterSelectionSet(ref int) {

}

func (t *TypeDataSourcePlanner) LeaveSelectionSet(ref int) {

}

func (t *TypeDataSourcePlanner) EnterField(ref int) {

}

func (t *TypeDataSourcePlanner) LeaveField(ref int) {

}

func (t *TypeDataSourcePlanner) Plan() (DataSource, []Argument) {
	return &TypeDataSource{}, t.args
}

type TypeDataSource struct {
}

func (t *TypeDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) {

}
