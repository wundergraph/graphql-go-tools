package astnormalization

import "github.com/jensneuse/graphql-go-tools/pkg/ast"

type NormalizeFunc func(operation, definition *ast.Document) error

func NormalizeOperation(operation, definition *ast.Document) error {
	normalizer := &OperationNormalizer{}
	return normalizer.Do(operation, definition)
}

type OperationNormalizer struct {
	err                     error
	fieldDeduplicate        FieldDeduplicate
	fieldSelectionMerge     FieldSelectionMerger
	fragmentInline          FragmentInline
	inlineFragmentResolving InlineFragmentResolver
	selfAliasRemove         SelfAliasRemove
}

func (o *OperationNormalizer) Do(operation, definition *ast.Document) error {
	o.err = nil
	o.must(o.fragmentInline.Do(operation, definition))
	o.must(o.inlineFragmentResolving.Do(operation, definition))
	o.must(o.fieldSelectionMerge.Do(operation, definition))
	o.must(o.selfAliasRemove.Do(operation, definition))
	o.must(o.fieldDeduplicate.Do(operation, definition))
	return o.err
}

func (o *OperationNormalizer) must(err error) {
	if o.err != nil {
		return
	}
	o.err = err
}
