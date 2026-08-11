package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
)

// shouldIsolateRootField is the per-root-field cache isolation gate: a QUERY
// root field whose exact (typeName, fieldName) coordinate resolves a cached
// root-field entry gets its OWN planner during path building, so sibling root
// fields with different (or no) cache settings never merge into one fetch and
// each cached field keeps its own L2 key and TTL. With caching off the
// isolation branches are dead and plans stay byte-identical; mutation roots
// are already one-planner-per-root. The decision reads the caching
// configuration ONLY — never FederationMetaData.
func shouldIsolateRootField(caching *cacheconfig.CachingConfiguration, field *currentFieldInfo, parentPath string) bool {
	if caching == nil {
		return false
	}
	if parentPath != "query" {
		return false
	}
	_, ok := caching.Resolve(field.ds.Id()).RootField(field.typeName, field.fieldName)
	return ok
}
