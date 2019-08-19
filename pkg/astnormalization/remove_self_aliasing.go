package astnormalization

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func RemoveSelfAliasing(operation, definition *ast.Document) error {
	remover := &SelfAliasRemove{}
	return remover.Do(operation, definition)
}

type SelfAliasRemove struct {
	walker  astvisitor.Walker
	visitor selfAliasRemoveVisitor
}

func (s *SelfAliasRemove) Do(operation, definition *ast.Document) error {
	s.visitor.operation = operation
	s.visitor.definition = definition
	return s.walker.Visit(operation, definition, &s.visitor)
}

type selfAliasRemoveVisitor struct {
	operation, definition *ast.Document
}

func (s *selfAliasRemoveVisitor) EnterArgument(ref int, definition int, info astvisitor.Info) {

}

func (s *selfAliasRemoveVisitor) LeaveArgument(ref int, definition int, info astvisitor.Info) {

}

func (s *selfAliasRemoveVisitor) EnterOperationDefinition(ref int, info astvisitor.Info) {

}

func (s *selfAliasRemoveVisitor) LeaveOperationDefinition(ref int, info astvisitor.Info) {

}

func (s *selfAliasRemoveVisitor) EnterSelectionSet(ref int, info astvisitor.Info) (instruction astvisitor.Instruction) {
	return
}

func (s *selfAliasRemoveVisitor) LeaveSelectionSet(ref int, info astvisitor.Info) {

}

func (s *selfAliasRemoveVisitor) EnterField(ref int, info astvisitor.Info) {
	if !s.operation.Fields[ref].Alias.IsDefined {
		return
	}
	if !bytes.Equal(s.operation.FieldName(ref), s.operation.FieldAlias(ref)) {
		return
	}
	s.operation.RemoveFieldAlias(ref)
}

func (s *selfAliasRemoveVisitor) LeaveField(ref int, info astvisitor.Info) {

}

func (s *selfAliasRemoveVisitor) EnterFragmentSpread(ref int, info astvisitor.Info) {

}

func (s *selfAliasRemoveVisitor) LeaveFragmentSpread(ref int, info astvisitor.Info) {

}

func (s *selfAliasRemoveVisitor) EnterInlineFragment(ref int, info astvisitor.Info) {

}

func (s *selfAliasRemoveVisitor) LeaveInlineFragment(ref int, info astvisitor.Info) {

}

func (s *selfAliasRemoveVisitor) EnterFragmentDefinition(ref int, info astvisitor.Info) {

}

func (s *selfAliasRemoveVisitor) LeaveFragmentDefinition(ref int, info astvisitor.Info) {

}
