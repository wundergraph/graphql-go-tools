package postprocess

import (
	"slices"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/representationvariable"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type cacheKeySpecFreezer struct {
	federation map[string]plan.FederationMetaData
	definition *ast.Document
}

func (f *cacheKeySpecFreezer) freeze(scope resolve.CacheScope, info *resolve.FetchInfo) (resolve.CacheKeySpec, bool) {
	if info == nil || len(info.RootFields) == 0 {
		return resolve.CacheKeySpec{}, false
	}
	typeName := info.RootFields[0].TypeName
	spec := resolve.CacheKeySpec{
		Scope:     scope,
		TypeName:  typeName,
		FieldName: info.RootFields[0].FieldName,
	}
	if scope == resolve.CacheScopeRootField {
		return spec, true
	}
	fed, ok := f.federation[info.DataSourceID]
	if !ok {
		return resolve.CacheKeySpec{}, false
	}
	if scope != resolve.CacheScopeEntity {
		return resolve.CacheKeySpec{}, false
	}
	if !fed.HasEntity(typeName) {
		return resolve.CacheKeySpec{}, false
	}
	keySets := fed.RequiredFieldsByKey(typeName)
	slices.SortFunc(keySets, func(a, b plan.FederationFieldConfiguration) int {
		return strings.Compare(a.SelectionSet, b.SelectionSet)
	})
	for _, keySet := range keySets {
		node, err := representationvariable.BuildRepresentationVariableNode(f.definition, keySet, fed)
		if err != nil {
			continue
		}
		spec.Candidates = append(spec.Candidates, resolve.CacheKeyCandidate{Representation: node})
	}
	if len(spec.Candidates) == 0 {
		return resolve.CacheKeySpec{}, false
	}
	spec.EntityKeyMappings = freezeEntityKeyMappings(fed, typeName)
	return spec, true
}

func freezeEntityKeyMappings(fed plan.FederationMetaData, typeName string) []resolve.EntityKeyMapping {
	// TODO(C1): freeze EntityKeyMappings.
	return nil
}
