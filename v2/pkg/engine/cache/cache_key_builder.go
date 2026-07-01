package cache

import (
	"slices"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/representationvariable"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// cacheKeyBuilder turns every resolvable @key set of a cached type into
// multi-key resolve.CacheKeySpec candidates BY VALUE. It is the SOLE
// federation reader on the plan side: the representation nodes it emits are
// built fresh per candidate via the shared representationvariable package, so
// no federation type or pointer ever reaches the runtime config.
type cacheKeyBuilder struct {
	// federation is the per-datasource federation metadata, keyed by
	// datasource ID; read-only input.
	federation map[string]plan.FederationMetaData
	// definition is the composed schema the @key selection sets resolve against.
	definition *ast.Document
}

// buildEntitySpec builds the multi-key spec for an entity fetch: one candidate
// per resolvable @key set of the fetched entity type, deterministically
// ordered by selection-set string. None is required — candidates are
// best-effort renderable at lookup and backfilled at write. Returns
// (zero, false) when the fetch resolves no entity or no usable key exists.
func (b *cacheKeyBuilder) buildEntitySpec(info *resolve.FetchInfo) (resolve.CacheKeySpec, bool) {
	if info == nil || len(info.RootFields) == 0 {
		return resolve.CacheKeySpec{}, false
	}
	typeName := info.RootFields[0].TypeName
	fed, ok := b.federation[info.DataSourceID]
	if !ok {
		return resolve.CacheKeySpec{}, false
	}
	if !fed.HasEntity(typeName) {
		return resolve.CacheKeySpec{}, false
	}

	spec := resolve.CacheKeySpec{
		Scope:    resolve.CacheScopeEntity,
		TypeName: typeName,
	}
	keySets := fed.RequiredFieldsByKey(typeName)
	slices.SortFunc(keySets, func(a, b plan.FederationFieldConfiguration) int {
		return strings.Compare(a.SelectionSet, b.SelectionSet)
	})
	for _, keySet := range keySets {
		node, err := representationvariable.BuildRepresentationVariableNode(b.definition, keySet, fed)
		// Best-effort multi-key: a malformed @key fragment must not drop caching
		// for the whole entity, so skip only that candidate and keep the others.
		// Two malformed shapes exist: a selection set that fails to build (err),
		// and one whose fields do not exist in the schema — the representation
		// walker silently drops unknown fields, leaving only the __typename
		// field, and a __typename-only key would collide across ALL entities of
		// the type. If EVERY @key is malformed, the zero-candidate check below
		// returns (zero, false) and the entity is simply not cached — the
		// conservative, correct fallback.
		if err != nil || node == nil || len(node.Fields) < 2 {
			continue
		}
		spec.Candidates = append(spec.Candidates, resolve.CacheKeyCandidate{Representation: node})
	}
	if len(spec.Candidates) == 0 {
		return resolve.CacheKeySpec{}, false
	}
	return spec, true
}

// buildRootFieldSpec builds the key spec for a root-field fetch: scope +
// the fetch's first root-field coordinate. Root-field keys carry no @key
// candidates (the value is whole-response scoped); entity-key mappings for
// by-key root fields land with task 15.
func (b *cacheKeyBuilder) buildRootFieldSpec(info *resolve.FetchInfo) (resolve.CacheKeySpec, bool) {
	if info == nil || len(info.RootFields) == 0 {
		return resolve.CacheKeySpec{}, false
	}
	return resolve.CacheKeySpec{
		Scope:     resolve.CacheScopeRootField,
		TypeName:  info.RootFields[0].TypeName,
		FieldName: info.RootFields[0].FieldName,
	}, true
}
