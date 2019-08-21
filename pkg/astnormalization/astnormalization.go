package astnormalization

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func NormalizeOperation(operation, definition *ast.Document) error {
	normalizer := &OperationNormalizer{}
	return normalizer.Do(operation, definition)
}

type registerNormalizeFunc func(walker *astvisitor.Walker)

type OperationNormalizer struct {
	walkers []*astvisitor.Walker
	setup   bool
}

func (o *OperationNormalizer) setupWalkers() {
	fragmentInline := astvisitor.NewWalker(48)
	fragmentSpreadInline(&fragmentInline)

	other := astvisitor.NewWalker(48)
	removeSelfAliasing(&other)
	mergeInlineFragments(&other)
	mergeFieldSelections(&other)
	deduplicateFields(&other)

	o.walkers = append(o.walkers, &fragmentInline)
	o.walkers = append(o.walkers, &other)
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
