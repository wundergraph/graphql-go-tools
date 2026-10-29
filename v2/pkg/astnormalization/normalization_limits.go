package astnormalization

// Limits bounds fragment expansion and sibling field counts during normalization.
// A zero limit disables that check. The zero value imposes no limits.
type Limits struct {
	// MaxFragmentExpansionSelections limits new selection nodes created per
	// operation while inlining named fragments. It counts every copied field,
	// inline fragment, and spread, plus each generated wrapper. Original selections
	// are not charged.
	MaxFragmentExpansionSelections int
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
