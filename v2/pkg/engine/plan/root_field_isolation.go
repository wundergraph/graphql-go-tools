package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

// shouldIsolateRootField is the per-root-field cache isolation gate (RFC-3, in
// core): a QUERY root field whose exact coordinate carries a RootFieldPolicy
// gets its OWN planner during path building, so sibling root fields with
// different (or no) cache policies never merge into one fetch — each cached
// field keeps its own L2 key and TTL, and the task-13 all-or-nothing decline
// stays a rare residual safety net.
//
// The gate holds when ALL of:
//   - caching is configured (non-empty providers — with caching off the
//     isolation branches are provably dead and plans stay byte-identical);
//   - the field is a DIRECT child of the QUERY operation root (mutation roots
//     are already one-planner-per-root; subscriptions are out of core);
//   - the field's datasource has a provider AND that provider has a
//     RootFieldPolicy for the exact (typeName, fieldName) coordinate AND that
//     policy actually enables caching (an inert policy — no TTL, no shadow —
//     yields no FetchCacheConfig via the configurator's all-flags-false safety
//     net, so isolating for it would change the plan without caching anything).
//
// The decision reads the CacheConfigProvider ONLY — never FederationMetaData.
func shouldIsolateRootField(providers map[string]cacheconfig.CacheConfigProvider, field *currentFieldInfo, parentPath string) bool {
	if len(providers) == 0 {
		return false
	}
	if parentPath != "query" {
		return false
	}
	provider := providers[field.ds.Id()]
	if provider == nil {
		return false
	}
	policy, ok := provider.RootFieldPolicy(field.typeName, field.fieldName)
	return ok && policy.EnablesCaching()
}
