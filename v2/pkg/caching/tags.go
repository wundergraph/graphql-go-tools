package caching

import "strings"

// Tag namespaces. Every tag an entry is indexed under carries exactly one, so a
// tag a subgraph declared can never name the same index entry as one the router
// derived. Building them lives here rather than beside the resolver because the
// invalidation side reads the index from another module, and a format defined
// twice is a format that drifts.
const (
	declaredTagPrefix = "declared:"
	subgraphTagPrefix = "subgraph:"
	typeTagPrefix     = "type:"
)

const tagScopeSeparator = ":"

func SubgraphTag(subgraph string) string {
	return subgraphTagPrefix + subgraph
}

func TypeTag(subgraph, typeName string) string {
	return typeTagPrefix + subgraph + tagScopeSeparator + typeName
}

// A subgraph name carrying the separator would otherwise spell another
// subgraph's tag, so separators within the scope are doubled and the single one
// after it is what ends the scope.
func DeclaredTag(subgraph, tag string) string {
	return declaredTagPrefix + escapeTagScope(subgraph) + tagScopeSeparator + tag
}

func escapeTagScope(scope string) string {
	return strings.ReplaceAll(scope, tagScopeSeparator, tagScopeSeparator+tagScopeSeparator)
}
