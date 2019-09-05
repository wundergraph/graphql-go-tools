package astnormalization

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/fastastvisitor"
)

func NormalizeOperation(operation, definition *ast.Document) error {
	normalizer := &OperationNormalizer{}
	return normalizer.Do(operation, definition)
}

type registerNormalizeFunc func(walker *fastastvisitor.Walker)

type OperationNormalizer struct {
	walkers []*fastastvisitor.Walker
	setup   bool
}

func (o *OperationNormalizer) setupWalkers() {
	fragmentInline := fastastvisitor.NewWalker(48)
	fragmentSpreadInline(&fragmentInline)
	directiveIncludeSkip(&fragmentInline)

	other := fastastvisitor.NewWalker(48)
	removeSelfAliasing(&other)
	mergeInlineFragments(&other)
	mergeFieldSelections(&other)
	deduplicateFields(&other)

	o.walkers = append(o.walkers, &fragmentInline, &other)
}

func (o *OperationNormalizer) Do(operation, definition *ast.Document) error {
	if !o.setup {
		o.setupWalkers()
		o.setup = true
	}

	for i := range o.walkers {
		err := o.walkers[i].Walk(operation, definition)
		if err != nil {
			return err
		}
	}

	return nil
}
