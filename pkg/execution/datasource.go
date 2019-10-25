package execution

import "github.com/jensneuse/graphql-go-tools/pkg/astvisitor"

type DataSource interface {
	Resolve(ctx Context, args ResolvedArgs) []byte
}

type DataSourcePlanner interface {
	DirectiveName() []byte
	Plan() (DataSource, []Argument)
	astvisitor.EnterInlineFragmentVisitor
	astvisitor.LeaveInlineFragmentVisitor
	astvisitor.EnterSelectionSetVisitor
	astvisitor.LeaveSelectionSetVisitor
	astvisitor.EnterFieldVisitor
	astvisitor.LeaveFieldVisitor
}
