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

	// SkipEntityResolutionPlannerCostForParentField prevents entity-resolution planners from
	// inflating the cost of the parent list field they traverse through. When a field (e.g.
	// Team.name) is owned by a different subgraph and requires an _entities call, the entity
	// resolution planner registers itself as a visitor of the parent list field (e.g.
	// Query.teams) so it can walk into the selection set. Without this fix, the cost visitor
	// counts that planner as a second data source for Query.teams and charges
	// ObjectTypeWeight("Team")=1 per item on top of the primary subgraph's configured weight —
	// violating the IBM Cost Specification, which bases costs on the user's operation, not the
	// router's internal fetch strategy.
	//
	// With this fix, getFieldDataSourceHashes skips any planner that does not own the field
	// via a PathTypeField entry (HasPathWithFieldRef), i.e. planners that merely traverse
	// through the field to reach a child.
	SkipEntityResolutionPlannerCostForParentField = true

	// CloseWSConnectionsOnContextCancel makes the WSTransport forcibly close all active
	// WebSocket connections when its parent context is cancelled. Without this, the pingLoop
	// exits on context cancellation but individual connections — whose readLoop blocks on
	// protocol.Read(context.Background()) — stay alive indefinitely, pinning the entire
	// object chain (WSTransport → SubscriptionClient → Factory → DataSources → PlanConfig →
	// Executor → RouterSchema *ast.Document ~200MB) until the remote end closes the socket.
	CloseWSConnectionsOnContextCancel = true
)
