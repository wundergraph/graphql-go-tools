package astnormalization

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

func NormalizeOperation(operation, definition *ast.Document, report *operationreport.Report) {
	normalizer := NewNormalizer(false)
	normalizer.NormalizeOperation(operation, definition, report)
}

type registerNormalizeFunc func(walker *astvisitor.Walker)

type OperationNormalizer struct {
	walkers                   []*astvisitor.Walker
	removeFragmentDefinitions bool
}

func NewNormalizer(removeFragmentDefinitions bool) *OperationNormalizer {
	normalizer := &OperationNormalizer{
		removeFragmentDefinitions: removeFragmentDefinitions,
	}
	normalizer.setupWalkers()
	return normalizer
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
	if o.removeFragmentDefinitions {
		removeFragmentDefinitions(&other)
	}

	o.walkers = append(o.walkers, &fragmentInline, &other)
}

func (o *OperationNormalizer) NormalizeOperation(operation, definition *ast.Document, report *operationreport.Report) {
	for i := range o.walkers {
		o.walkers[i].Walk(operation, definition, report)
	}
}
