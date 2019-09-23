package astnormalization

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/graphqlerror"
)

func NormalizeOperation(operation, definition *ast.Document, report *graphqlerror.Report) {
	normalizer := &OperationNormalizer{}
	normalizer.Do(operation, definition, report)
}

type registerNormalizeFunc func(walker *astvisitor.Walker)

type OperationNormalizer struct {
	walkers []*astvisitor.Walker
	setup   bool
}

func (o *OperationNormalizer) setupWalkers() {
	fragmentInline := astvisitor.NewWalker(48)
	fragmentSpreadInline(&fragmentInline)
	directiveIncludeSkip(&fragmentInline)

	other := astvisitor.NewWalker(48)
	removeSelfAliasing(&other)
	mergeInlineFragments(&other)
	mergeFieldSelections(&other)
	deduplicateFields(&other)

	o.walkers = append(o.walkers, &fragmentInline, &other)
}

func (o *OperationNormalizer) Do(operation, definition *ast.Document, report *graphqlerror.Report) {
	if !o.setup {
		o.setupWalkers()
		o.setup = true
	}

	for i := range o.walkers {
		o.walkers[i].Walk(operation, definition, report)
	}
}
