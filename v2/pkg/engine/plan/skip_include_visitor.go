package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func hasConditionalSkipInclude(operation *ast.Document, definition *ast.Document, report *operationreport.Report) bool {
	walker := astvisitor.NewWalker(32)
	visitor := &skipIncludeVisitor{
		operation:  operation,
		definition: definition,
		walker:     &walker,
	}
	walker.RegisterEnterInlineFragmentVisitor(visitor)
	walker.RegisterEnterFieldVisitor(visitor)
	walker.Walk(operation, definition, report)

	return visitor.HasConditionalSkipInclude
}

type skipIncludeVisitor struct {
	operation                 *ast.Document
	definition                *ast.Document
	walker                    *astvisitor.Walker
	HasConditionalSkipInclude bool
}

func (v *skipIncludeVisitor) EnterInlineFragment(inlineFragmentRef int) {
	v.checkDirectives(v.operation.InlineFragments[inlineFragmentRef].Directives.Refs)
}

func (v *skipIncludeVisitor) EnterField(fieldRef int) {
	v.checkDirectives(v.operation.Fields[fieldRef].Directives.Refs)
}

func (v *skipIncludeVisitor) checkDirectives(directiveRefs []int) {
	_, skip := v.operation.ResolveSkipDirectiveVariable(directiveRefs)
	_, include := v.operation.ResolveIncludeDirectiveVariable(directiveRefs)

	if skip || include {
		v.HasConditionalSkipInclude = true
		v.walker.Stop()
	}
}
