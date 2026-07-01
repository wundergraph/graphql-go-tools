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
	fieldName := info.RootFields[0].FieldName
	spec := resolve.CacheKeySpec{
		Scope:     scope,
		TypeName:  typeName,
		FieldName: fieldName,
	}
	if scope == resolve.CacheScopeRootField {
		fed, ok := f.federation[info.DataSourceID]
		if ok {
			spec.EntityKeyMappings = freezeEntityKeyMappings(f.definition, fed, typeName, fieldName)
		}
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
			// Best-effort multi-key: a single malformed @key fragment must not
			// drop caching for the whole entity, so skip only that candidate and
			// keep the others (RFC-2 §6.1). If EVERY @key fails to build, the
			// zero-candidate check below returns (zero, false) and the entity is
			// simply not cached — the conservative, correct fallback.
			continue
		}
		spec.Candidates = append(spec.Candidates, resolve.CacheKeyCandidate{Representation: node})
	}
	if len(spec.Candidates) == 0 {
		return resolve.CacheKeySpec{}, false
	}
	// C1 keeps entity-scope mappings nil: entity fetches already key by representation.
	return spec, true
}

func freezeEntityKeyMappings(definition *ast.Document, fed plan.FederationMetaData, rootTypeName, rootFieldName string) []resolve.EntityKeyMapping {
	if definition == nil {
		return nil
	}
	rootNode, ok := definition.Index.FirstNodeByNameStr(rootTypeName)
	if !ok {
		return nil
	}
	fieldDef, ok := definition.NodeFieldDefinitionByName(rootNode, ast.ByteSlice(rootFieldName))
	if !ok {
		return nil
	}
	entityTypeName := definition.FieldDefinitionTypeNameString(fieldDef)
	if !fed.HasEntity(entityTypeName) {
		return nil
	}

	argNames := map[string]struct{}{}
	for _, argRef := range definition.NodeInputValueDefinitions(ast.Node{Kind: ast.NodeKindFieldDefinition, Ref: fieldDef}) {
		argNames[definition.InputValueDefinitionNameString(argRef)] = struct{}{}
	}
	if len(argNames) == 0 {
		return nil
	}

	keySets := fed.RequiredFieldsByKey(entityTypeName)
	slices.SortFunc(keySets, func(a, b plan.FederationFieldConfiguration) int {
		return strings.Compare(a.SelectionSet, b.SelectionSet)
	})

	seen := map[string]struct{}{}
	mappings := make([]resolve.EntityKeyMapping, 0, len(keySets))
	for _, keySet := range keySets {
		keyFields, ok := keySelectionSetFieldNames(entityTypeName, keySet.SelectionSet)
		if !ok {
			continue
		}
		fieldMappings := make([]resolve.EntityFieldMapping, 0, len(keyFields))
		for _, keyField := range keyFields {
			if _, ok := argNames[keyField]; !ok {
				fieldMappings = nil
				break
			}
			fieldMappings = append(fieldMappings, resolve.EntityFieldMapping{
				EntityKeyField:      keyField,
				ArgumentPath:        []string{keyField},
				ArgumentIsEntityKey: true,
			})
		}
		if len(fieldMappings) == 0 {
			continue
		}
		key := entityTypeName + "\x00" + strings.Join(keyFields, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		mappings = append(mappings, resolve.EntityKeyMapping{
			EntityTypeName: entityTypeName,
			FieldMappings:  fieldMappings,
		})
	}
	return mappings
}

func keySelectionSetFieldNames(typeName, selectionSet string) ([]string, bool) {
	fragment, report := plan.RequiredFieldsFragment(typeName, selectionSet, false)
	if report == nil || report.HasErrors() || fragment == nil || len(fragment.FragmentDefinitions) == 0 {
		return nil, false
	}
	fieldNames := fragment.SelectionSetFieldNames(fragment.FragmentDefinitions[0].SelectionSet)
	return fieldNames, len(fieldNames) != 0
}
