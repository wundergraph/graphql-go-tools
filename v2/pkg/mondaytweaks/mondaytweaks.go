// Package mondaytweaks defines compile-time feature flags for monday.com-specific
// behavioural overrides. Both the astnormalization and engine packages import this
// package so all monday-specific toggles live in one place.
package mondaytweaks

const (
	// CoerceNullVariablesWithDefaults enables the null variable coercion normalizer.
	// When a nullable variable is explicitly null and used in a non-null argument position
	// that has a schema default, the variable reference is split so the subgraph treats it
	// as "not provided" and applies the schema default — matching Apollo Router behavior.
	CoerceNullVariablesWithDefaults = true
)
