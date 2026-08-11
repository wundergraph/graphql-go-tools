package cache

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// The key freezer: it turns a finished fetch into the data-only
// resolve.CacheKeySpec the runtime renders keys from. It reads NO federation
// metadata and no schema — an entity fetch's identity is the merged
// representation node the fetch ALREADY sends, so the freezer only has to find
// that node on the fetch and hand it over.

// buildEntitySpec builds the key spec for an entity fetch: scope, the fetched
// entity type, and the fetch's own merged representation node. Because the key
// renders from the very object the fetch puts on the wire, read key == write
// key == fetch identity, and @requires fields participate in the identity.
// Returns (zero, false) when the fetch resolves no entity or sends no
// representation.
func buildEntitySpec(fetch resolve.Fetch, info *resolve.FetchInfo) (resolve.CacheKeySpec, bool) {
	if info == nil || len(info.RootFields) == 0 {
		return resolve.CacheKeySpec{}, false
	}
	node, ok := representationNode(fetch)
	if !ok {
		return resolve.CacheKeySpec{}, false
	}
	return resolve.CacheKeySpec{
		Scope:          resolve.CacheScopeEntity,
		TypeName:       info.RootFields[0].TypeName,
		Representation: node,
	}, true
}

// buildRootFieldSpec builds the key spec for a root-field fetch: scope plus the
// fetch's first root-field coordinate. Root fields have their own key space
// (coordinate + canonical request variables) and never join the entity one.
func buildRootFieldSpec(info *resolve.FetchInfo) (resolve.CacheKeySpec, bool) {
	if info == nil || len(info.RootFields) == 0 {
		return resolve.CacheKeySpec{}, false
	}
	return resolve.CacheKeySpec{
		Scope:     resolve.CacheScopeRootField,
		TypeName:  info.RootFields[0].TypeName,
		FieldName: info.RootFields[0].FieldName,
	}, true
}

// representationNode returns the merged representation object the fetch sends,
// read from the ResolvableObjectVariable segment of the fetch's representation
// input template. The node is plan-owned and read-only at runtime, so the spec
// REFERENCES it (intentional identity) instead of copying it.
func representationNode(fetch resolve.Fetch) (*resolve.Object, bool) {
	if fetch == nil {
		return nil, false
	}
	template := fetch.RepresentationInputTemplate()
	if template == nil {
		return nil, false
	}
	for _, segment := range template.Segments {
		if segment.SegmentType != resolve.VariableSegmentType || segment.VariableKind != resolve.ResolvableObjectVariableKind {
			continue
		}
		renderer, ok := segment.Renderer.(*resolve.GraphQLVariableResolveRenderer)
		if !ok {
			continue
		}
		if node, ok := renderer.Node.(*resolve.Object); ok {
			return node, true
		}
	}
	return nil, false
}
