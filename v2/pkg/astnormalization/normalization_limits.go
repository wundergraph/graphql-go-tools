package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// Limits bounds stored selection nodes and physical selection-list sizes during normalization.
// A zero limit disables that check. The zero value imposes no limits.
type Limits struct {
	// MaxSelections bounds the document's stored selection nodes, including source
	// selections, copies, and generated wrappers. Removed nodes remain charged.
	// A completed replacement may exceed the limit; normalization then stops.
	MaxSelections int
	// MaxSelectionSetSize bounds the direct selection references in each set.
	// Fields, named spreads, and inline-fragment wrappers each occupy one entry.
	// The limit is checked on source lists and before normalization grows a list.
	MaxSelectionSetSize int
}

// WithLimits sets normalization budgets. Limits are disabled by
// default. Pass an empty Limits to disable both checks.
func WithLimits(limits Limits) Option {
	return func(options *options) {
		options.limits = limits
	}
}

func (l Limits) checkSelections(operation *ast.Document, report *operationreport.Report) bool {
	if l.MaxSelections > 0 && len(operation.Selections) > l.MaxSelections {
		report.AddExternalError(operationreport.ErrNormalizationLimitExceeded("max_selections", l.MaxSelections))
		return false
	}
	return true
}

func (l Limits) checkSelectionSetSize(size int, report *operationreport.Report) bool {
	if l.MaxSelectionSetSize > 0 && size > l.MaxSelectionSetSize {
		report.AddExternalError(operationreport.ErrNormalizationLimitExceeded("max_selection_set_size", l.MaxSelectionSetSize))
		return false
	}
	return true
}

type selectionSetSizeVisitor struct {
	*astvisitor.Walker

	limits    Limits
	operation *ast.Document
}

func registerSelectionSetSizeLimit(walker *astvisitor.Walker, limits Limits) {
	if limits.MaxSelectionSetSize <= 0 {
		return
	}
	v := &selectionSetSizeVisitor{Walker: walker, limits: limits}
	walker.RegisterEnterDocumentVisitor(v)
	walker.RegisterEnterSelectionSetVisitor(v)
}

func (v *selectionSetSizeVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation = operation
}

func (v *selectionSetSizeVisitor) EnterSelectionSet(ref int) {
	if !v.limits.checkSelectionSetSize(len(v.operation.SelectionSets[ref].SelectionRefs), v.Report) {
		v.Stop()
	}
}
