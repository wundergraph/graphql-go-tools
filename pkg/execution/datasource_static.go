package execution

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"io"
)

type StaticDataSourcePlanner struct {
	walker                *astvisitor.Walker
	definition, operation *ast.Document
	args                  []Argument
}

func (s *StaticDataSourcePlanner) DirectiveName() []byte {
	return []byte("StaticDataSource")
}

func (s *StaticDataSourcePlanner) Initialize(walker *astvisitor.Walker, operation, definition *ast.Document, args []Argument, resolverParameters []ResolverParameter) {
	s.walker, s.operation, s.definition, s.args = walker, operation, definition, args
}

func (s *StaticDataSourcePlanner) EnterInlineFragment(ref int) {

}

func (s *StaticDataSourcePlanner) LeaveInlineFragment(ref int) {

}

func (s *StaticDataSourcePlanner) EnterSelectionSet(ref int) {

}

func (s *StaticDataSourcePlanner) LeaveSelectionSet(ref int) {

}

func (s *StaticDataSourcePlanner) EnterField(ref int) {
	fieldDefinition, ok := s.walker.FieldDefinition(ref)
	if !ok {
		return
	}
	directive, ok := s.definition.FieldDefinitionDirectiveByName(fieldDefinition, s.DirectiveName())
	if !ok {
		return
	}
	var staticValue []byte
	value, ok := s.definition.DirectiveArgumentValueByName(directive, []byte("data"))
	if !ok || value.Kind != ast.ValueKindString {
		staticValue = literal.NULL
	} else {
		staticValue = s.definition.StringValueContentBytes(value.Ref)
	}
	staticValue = bytes.ReplaceAll(staticValue, literal.BACKSLASH, nil)
	s.args = append(s.args, &StaticVariableArgument{
		Value: staticValue,
	})
}

func (s *StaticDataSourcePlanner) LeaveField(ref int) {

}

func (s *StaticDataSourcePlanner) Plan() (DataSource, []Argument) {
	return &StaticDataSource{}, s.args
}

type StaticDataSource struct {
}

func (s StaticDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) {
	out.Write(args[0].Value)
}
