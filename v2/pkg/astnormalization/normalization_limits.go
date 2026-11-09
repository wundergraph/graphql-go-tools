package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// Limits bounds fragment expansion and sibling field counts during normalization.
// A zero limit disables that check. The zero value imposes no limits.
type Limits struct {
	// MaxSelections bounds the document's stored selection nodes, including source
	// selections, copies, and generated wrappers. Removed nodes remain charged.
	// A completed replacement may exceed the limit; normalization then stops.
	MaxSelections int
	// MaxExpandedSelectionSetFields counts sibling fields before field merging,
	// with inline-fragment wrappers treated as transparent. Child field selection
	// sets are counted separately. Named-fragment contents count only when
	// WithInlineFragmentSpreads is enabled.
	// Counts include duplicates and branches remaining after skip/include pruning.
	MaxExpandedSelectionSetFields int
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
