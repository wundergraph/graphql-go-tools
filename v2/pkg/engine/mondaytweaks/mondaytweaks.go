// Package mondaytweaks defines compile-time feature flags for monday.com-specific
// behavioural overrides in the cost calculator and resolver.  Both the plan and resolve
// packages import this package so all monday-specific toggles live in one place.
package mondaytweaks

const (
	// UseInterfaceDefaultCostForAbstractTypes makes the cost calculator use a field's
	// return-type default weight (scalar=0, object=1) for abstract-type selections instead
	// of the maximum @cost weight across all implementing types — matches Apollo Router.
	UseInterfaceDefaultCostForAbstractTypes = true
)
