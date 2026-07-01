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

// buildRootFieldSpec builds the key spec for a root-field fetch: scope + the
// fetch's first root-field coordinate. A BY-KEY root field — one returning an
// entity type whose @key fields are all covered by the field's argument names
// — additionally gets the structurally derived EntityKeyMappings (D10) and the
// entity's FULL candidate set, so it participates in the ENTITY key space (an
// arg-derived candidate renders at lookup; data-derived ones backfill at
// write). Reuse then works exactly when the root-field policy shares its
// CacheName with the entity policy (read key == write key).
func (b *cacheKeyBuilder) buildRootFieldSpec(info *resolve.FetchInfo) (resolve.CacheKeySpec, bool) {
	if info == nil || len(info.RootFields) == 0 {
		return resolve.CacheKeySpec{}, false
	}
	spec := resolve.CacheKeySpec{
		Scope:     resolve.CacheScopeRootField,
		TypeName:  info.RootFields[0].TypeName,
		FieldName: info.RootFields[0].FieldName,
	}
	// Only a single-root-field fetch can be a by-key entity lookup.
	if len(info.RootFields) == 1 {
		if entityTypeName, mappings := b.deriveEntityKeyMappings(info, spec.TypeName, spec.FieldName); len(mappings) > 0 {
			entitySpec, ok := b.buildEntitySpec(&resolve.FetchInfo{
				DataSourceID: info.DataSourceID,
				RootFields:   []resolve.GraphCoordinate{{TypeName: entityTypeName}},
			})
			if ok {
				// The lookup item derives from ARGUMENTS and carries no
				// __typename, so each candidate's representation gets the
				// entity type name for the template's fallback. The nodes are
				// freshly built per spec — setting TypeName mutates nothing
				// shared.
				for _, candidate := range entitySpec.Candidates {
					candidate.Representation.TypeName = entityTypeName
				}
				spec.EntityKeyMappings = mappings
				spec.Candidates = entitySpec.Candidates
			}
		}
	}
	return spec, true
}

// deriveEntityKeyMappings resolves the root field's return type from the
// definition and, when it is an entity of the fetch's datasource, emits one
// mapping per resolvable @key set whose EVERY key field name matches an
// argument name of the root field (product(upc:) maps @key(upc), never
// @key(sku)). Structural derivation only (D10): definition + federation, no
// external mapping config. v1 CONSTRAINT: the runtime reads the argument value
// from a request variable NAMED LIKE the key field, so reuse requires
// arguments passed as same-named variables.
func (b *cacheKeyBuilder) deriveEntityKeyMappings(info *resolve.FetchInfo, rootTypeName, rootFieldName string) (string, []resolve.EntityKeyMapping) {
	if b.definition == nil {
		return "", nil
	}
	fed, ok := b.federation[info.DataSourceID]
	if !ok {
		return "", nil
	}
	rootNode, ok := b.definition.Index.FirstNodeByNameStr(rootTypeName)
	if !ok {
		return "", nil
	}
	fieldDef, ok := b.definition.NodeFieldDefinitionByName(rootNode, ast.ByteSlice(rootFieldName))
	if !ok {
		return "", nil
	}
	entityTypeName := b.definition.FieldDefinitionTypeNameString(fieldDef)
	if !fed.HasEntity(entityTypeName) {
		return "", nil
	}

	argNames := map[string]struct{}{}
	for _, argRef := range b.definition.NodeInputValueDefinitions(ast.Node{Kind: ast.NodeKindFieldDefinition, Ref: fieldDef}) {
		argNames[b.definition.InputValueDefinitionNameString(argRef)] = struct{}{}
	}
	if len(argNames) == 0 {
		return "", nil
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
				// A key set with ANY field outside the argument names cannot
				// be rendered from the arguments — skip the whole set.
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
		dedupeKey := entityTypeName + "\x00" + strings.Join(keyFields, "\x00")
		if _, ok := seen[dedupeKey]; ok {
			continue
		}
		seen[dedupeKey] = struct{}{}
		mappings = append(mappings, resolve.EntityKeyMapping{
			EntityTypeName: entityTypeName,
			FieldMappings:  fieldMappings,
		})
	}
	return entityTypeName, mappings
}

// keySelectionSetFieldNames parses one @key selection set and returns its
// top-level field names; composite (nested) key sets return their top-level
// names, which then fail the argument-name match unless an argument carries
// the object — the conservative outcome.
func keySelectionSetFieldNames(typeName, selectionSet string) ([]string, bool) {
	fragment, report := plan.RequiredFieldsFragment(typeName, selectionSet, false)
	if report == nil || report.HasErrors() || fragment == nil || len(fragment.FragmentDefinitions) == 0 {
		return nil, false
	}
	fieldNames := fragment.SelectionSetFieldNames(fragment.FragmentDefinitions[0].SelectionSet)
	return fieldNames, len(fieldNames) != 0
}
