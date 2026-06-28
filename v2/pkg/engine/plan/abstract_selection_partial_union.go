package plan

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// partialUnionFieldInfo captures, for a single field occurrence in the operation,
// the enclosing type and field name plus the datasources that hold a node for it.
// It is used to compute which datasources can actually resolve a union field.
type partialUnionFieldInfo struct {
	typeName  string
	fieldName string
	// nodeDataSources are the datasources that have a node for this field. Note this
	// is a superset of the datasources that can actually reach it: a datasource may
	// define a field on a type it can never produce on this path (e.g. it defines
	// Wrapper.actions but cannot resolve the Wrapper because the producing field is
	// owned by another subgraph). Reachability is resolved in reachableCandidates.
	nodeDataSources map[DSHash]struct{}
}

// prunePartialUnionMembers implements the "partial union" intersection rule used by
// spec-compliant federation routers (e.g. Hive Router's narrow_partial_union_paths).
//
// The same union can have different members in different subgraphs, for example:
//
//	subgraph A: union Action = Common | OnlyA
//	subgraph B: union Action = Common | OnlyB
//
// When a union field can still be resolved by multiple candidate subgraphs (a
// @shareable path on a shared entity), the response shape must not depend on which
// candidate the planner ultimately picks. Only members defined by EVERY candidate
// subgraph are guaranteed safe. For the remaining members:
//
//   - a member unique to a subset but defined by the RESOLVING subgraph (the one the
//     union field is selected on) is kept in the response shape but excluded from the
//     upstream fetch, so it resolves to null (matching the reference behaviour);
//   - a member not defined by the resolving subgraph (foreign) is dropped entirely -
//     no element of that type can be returned, and fetching it is invalid.
//
// Without this, the planner marks a foreign member (e.g. OnlyB.b) as resolvable in
// the subgraph that defines it and creates an entity hop to fetch it. That is invalid
// for a non-keyed union value type reached via a shareable list - the entity hop
// re-resolves the whole list - and the dependent planner collapses to an empty
// selection set, producing an HTTP 500 at runtime.
//
// It returns the field refs that must be kept in the response but excluded from the
// upstream fetch, and whether the operation was modified. When modified, the caller
// must rebuild the datasource suggestions from the pruned operation.
func (p *NodeSelectionBuilder) prunePartialUnionMembers(operation, definition *ast.Document, suggestions *NodeSuggestions) (map[int]struct{}, bool) {
	if suggestions == nil {
		return nil, false
	}

	// Index, per field ref, the enclosing type/field name and the datasources that
	// have a node for it. Field refs are stable across datasources.
	fields := make(map[int]*partialUnionFieldInfo)
	for i := range suggestions.items {
		item := suggestions.items[i]
		if item.IsOrphan {
			continue
		}
		info, ok := fields[item.FieldRef]
		if !ok {
			info = &partialUnionFieldInfo{
				typeName:        item.TypeName,
				fieldName:       item.FieldName,
				nodeDataSources: make(map[DSHash]struct{}),
			}
			fields[item.FieldRef] = info
		}
		info.nodeDataSources[item.DataSourceHash] = struct{}{}
	}

	dataSourceByHash := make(map[DSHash]DataSource, len(p.config.DataSources))
	for _, ds := range p.config.DataSources {
		dataSourceByHash[ds.Hash()] = ds
	}

	pruner := &partialUnionPruner{
		operation:        operation,
		definition:       definition,
		fields:           fields,
		parentByFieldRef: buildParentFieldMap(operation),
		dataSourceByHash: dataSourceByHash,
		candidatesMemo:   make(map[int]map[DSHash]struct{}),
		hopFreeMemo:      make(map[int]map[DSHash]struct{}),
		responseOnly:     make(map[int]struct{}),
	}

	modified := false
	for fieldRef := range fields {
		candidates := pruner.reachableCandidates(fieldRef)
		// A single reachable candidate has no cross-subgraph ambiguity: the
		// per-datasource selection rewriter already prunes members the datasource
		// does not define, so we leave it untouched to avoid changing plans that
		// already work (e.g. a union reached through a hop forced into one subgraph).
		if len(candidates) < 2 {
			continue
		}
		if pruner.pruneUnionFieldToIntersection(fieldRef, candidates) {
			modified = true
		}
	}

	if !modified {
		return nil, false
	}
	return pruner.responseOnly, true
}

type partialUnionPruner struct {
	operation        *ast.Document
	definition       *ast.Document
	fields           map[int]*partialUnionFieldInfo
	parentByFieldRef map[int]int
	dataSourceByHash map[DSHash]DataSource
	candidatesMemo   map[int]map[DSHash]struct{}
	hopFreeMemo      map[int]map[DSHash]struct{}
	responseOnly     map[int]struct{}
}

// hopFreeCandidates returns the datasources that can resolve the field at fieldRef
// WITHOUT an entity jump - i.e. following the same-source parent chain all the way to
// a root field. Unlike reachableCandidates, it does not allow reaching a field via an
// entity jump into the enclosing type. When exactly one datasource is hop-free for a
// union field, that subgraph resolves the list inline, so its own non-shared members
// can be kept as response-only nulls; members it does not define are foreign and must
// be dropped (fetching them would require re-resolving the list via a hop).
func (u *partialUnionPruner) hopFreeCandidates(fieldRef int) map[DSHash]struct{} {
	if cached, ok := u.hopFreeMemo[fieldRef]; ok {
		return cached
	}
	u.hopFreeMemo[fieldRef] = map[DSHash]struct{}{}

	info, ok := u.fields[fieldRef]
	if !ok {
		return u.hopFreeMemo[fieldRef]
	}

	parentRef, hasParent := u.parentByFieldRef[fieldRef]
	var parentHopFree map[DSHash]struct{}
	if hasParent && parentRef != ast.InvalidRef {
		parentHopFree = u.hopFreeCandidates(parentRef)
	}

	result := make(map[DSHash]struct{}, len(info.nodeDataSources))
	for dsHash := range info.nodeDataSources {
		if !hasParent || parentRef == ast.InvalidRef {
			// Root field: hop-free wherever the field node exists.
			result[dsHash] = struct{}{}
			continue
		}
		if _, ok := parentHopFree[dsHash]; ok {
			result[dsHash] = struct{}{}
		}
	}

	u.hopFreeMemo[fieldRef] = result
	return result
}

// reachableCandidates returns the datasources that can actually resolve the field at
// fieldRef, accounting for federation reachability: a datasource is a candidate if it
// has a node for the field and either (a) the enclosing type is an entity it can be
// jumped into, or (b) the field is a root field, or (c) it is itself a reachable
// candidate for the parent field. This mirrors how a federation planner only keeps a
// subgraph in play for a path when there is a real route to it.
func (u *partialUnionPruner) reachableCandidates(fieldRef int) map[DSHash]struct{} {
	if cached, ok := u.candidatesMemo[fieldRef]; ok {
		return cached
	}
	// Guard against cycles in the memo while recursing.
	u.candidatesMemo[fieldRef] = map[DSHash]struct{}{}

	info, ok := u.fields[fieldRef]
	if !ok {
		return u.candidatesMemo[fieldRef]
	}

	parentRef, hasParent := u.parentByFieldRef[fieldRef]
	var parentCandidates map[DSHash]struct{}
	if hasParent && parentRef != ast.InvalidRef {
		parentCandidates = u.reachableCandidates(parentRef)
	}

	result := make(map[DSHash]struct{}, len(info.nodeDataSources))
	for dsHash := range info.nodeDataSources {
		ds, ok := u.dataSourceByHash[dsHash]
		if !ok {
			continue
		}
		switch {
		case ds.HasEntity(info.typeName) && ds.HasRootNodeWithTypename(info.typeName):
			// The enclosing type is an entity in this datasource, so it can be
			// reached via an entity jump regardless of the parent path.
			result[dsHash] = struct{}{}
		case !hasParent || parentRef == ast.InvalidRef:
			// Root field: reachable wherever the field node exists.
			result[dsHash] = struct{}{}
		default:
			if _, ok := parentCandidates[dsHash]; ok {
				result[dsHash] = struct{}{}
			}
		}
	}

	u.candidatesMemo[fieldRef] = result
	return result
}

// pruneUnionFieldToIntersection rewrites the union field's selection so that members
// not common to every candidate datasource are either kept as response-only (when the
// resolving subgraph defines them) or dropped (when foreign). Returns true if the
// operation was modified.
func (u *partialUnionPruner) pruneUnionFieldToIntersection(fieldRef int, candidates map[DSHash]struct{}) bool {
	unionTypeName, allMemberSet, ok := u.unionFieldMembers(fieldRef)
	if !ok {
		return false
	}

	intersection := u.memberIntersection(unionTypeName, candidates, allMemberSet)
	// No conflict: every union member is shared by all candidates, nothing to prune.
	if len(intersection) == len(allMemberSet) {
		return false
	}

	selectionSetRef, ok := u.operation.FieldSelectionSet(fieldRef)
	if !ok {
		return false
	}

	resolvingMembers := u.resolvingSubgraphMembers(fieldRef, unionTypeName)

	kept, changed := u.rewriteUnionMembers(selectionSetRef, allMemberSet, intersection, resolvingMembers)
	if !changed {
		return false
	}

	u.replaceSelectionSet(selectionSetRef, kept)
	return true
}

// unionFieldMembers resolves the field's union return type and the set of its members
// in the federated graph schema. It reports ok=false when the field does not return a
// union, or when any member is an entity - entity-member unions are resolved via the
// existing entity-hop mechanism (e.g. "union-intersection" / "union query on array")
// and must not be touched here; only non-entity value-type members, which cannot be
// resolved independently, need the partial-union treatment.
func (u *partialUnionPruner) unionFieldMembers(fieldRef int) (unionTypeName string, allMemberSet map[string]struct{}, ok bool) {
	info, ok := u.fields[fieldRef]
	if !ok {
		return "", nil, false
	}
	enclosingNode, ok := u.definition.NodeByNameStr(info.typeName)
	if !ok {
		return "", nil, false
	}
	fieldTypeNode, ok := u.definition.FieldTypeNode([]byte(info.fieldName), enclosingNode)
	if !ok || fieldTypeNode.Kind != ast.NodeKindUnionTypeDefinition {
		return "", nil, false
	}
	allMembers, ok := u.definition.UnionTypeDefinitionMemberTypeNames(fieldTypeNode.Ref)
	if !ok || slices.ContainsFunc(allMembers, u.isEntityType) {
		return "", nil, false
	}

	allMemberSet = make(map[string]struct{}, len(allMembers))
	for _, member := range allMembers {
		allMemberSet[member] = struct{}{}
	}
	return u.definition.UnionTypeDefinitionNameString(fieldTypeNode.Ref), allMemberSet, true
}

// memberIntersection returns the union members defined by EVERY candidate datasource,
// restricted to members of the federated union (allMemberSet). Each datasource carries
// its own union members in its upstream schema.
func (u *partialUnionPruner) memberIntersection(unionTypeName string, candidates map[DSHash]struct{}, allMemberSet map[string]struct{}) map[string]struct{} {
	intersection := make(map[string]struct{}, len(allMemberSet))
	first := true
	for dsHash := range candidates {
		members := u.datasourceUnionMembers(dsHash, unionTypeName)
		if first {
			for _, member := range members {
				if _, isMember := allMemberSet[member]; isMember {
					intersection[member] = struct{}{}
				}
			}
			first = false
			continue
		}
		current := make(map[string]struct{}, len(members))
		for _, member := range members {
			current[member] = struct{}{}
		}
		for member := range intersection {
			if _, retained := current[member]; !retained {
				delete(intersection, member)
			}
		}
	}
	return intersection
}

// resolvingSubgraphMembers returns the union members defined by the subgraph that
// resolves the list inline - the unique hop-free candidate. Non-shared members defined
// there can be kept as response-only nulls; the rest are dropped. When the resolving
// subgraph is ambiguous (zero or multiple hop-free candidates), this returns an empty
// set so all non-shared members are dropped, which is the safe intersection behaviour.
func (u *partialUnionPruner) resolvingSubgraphMembers(fieldRef int, unionTypeName string) map[string]struct{} {
	members := make(map[string]struct{})
	hopFree := u.hopFreeCandidates(fieldRef)
	if len(hopFree) != 1 {
		return members
	}
	for dsHash := range hopFree {
		for _, member := range u.datasourceUnionMembers(dsHash, unionTypeName) {
			members[member] = struct{}{}
		}
	}
	return members
}

// rewriteUnionMembers walks the union field's selections and decides each member
// fragment's fate: shared members and non-union fragments are kept untouched; a
// non-shared member is kept (as a response-only null) or dropped via
// classifyNonSharedMember. It returns the selection refs to keep and whether the set
// of fetched fields changed.
func (u *partialUnionPruner) rewriteUnionMembers(selectionSetRef int, allMemberSet, intersection, resolvingMembers map[string]struct{}) (kept []int, changed bool) {
	selectionRefs := u.operation.SelectionSets[selectionSetRef].SelectionRefs
	kept = make([]int, 0, len(selectionRefs))
	for _, selectionRef := range selectionRefs {
		selection := u.operation.Selections[selectionRef]
		if selection.Kind != ast.SelectionKindInlineFragment {
			kept = append(kept, selectionRef)
			continue
		}

		member := string(u.operation.InlineFragmentTypeConditionName(selection.Ref))
		if _, isMember := allMemberSet[member]; !isMember {
			// fragment on the union type itself or an interface - leave untouched.
			kept = append(kept, selectionRef)
			continue
		}
		if _, shared := intersection[member]; shared {
			kept = append(kept, selectionRef)
			continue
		}

		keep, memberChanged := u.classifyNonSharedMember(selection.Ref, member, resolvingMembers)
		if keep {
			kept = append(kept, selectionRef)
		}
		if memberChanged {
			changed = true
		}
	}
	return kept, changed
}

// classifyNonSharedMember decides the fate of an inline fragment on a non-shared union
// member. keep reports whether the fragment stays in the selection; changed reports
// whether this alters what is fetched. A member the resolving subgraph defines is kept
// as a response-only null (or, when it has only __typename, kept and fetched as-is); a
// foreign member, or one with nested selections that cannot be safely nulled, is dropped.
func (u *partialUnionPruner) classifyNonSharedMember(inlineFragmentRef int, member string, resolvingMembers map[string]struct{}) (keep, changed bool) {
	if _, defined := resolvingMembers[member]; !defined {
		return false, true // foreign - drop
	}
	marked, onlyTypename := u.tryMarkResponseOnly(inlineFragmentRef)
	switch {
	case marked:
		return true, true // kept in the response, excluded from the upstream fetch
	case onlyTypename:
		return true, false // nothing but __typename - keep and fetch normally
	default:
		return false, true // nested selections we cannot safely null out - drop
	}
}

// replaceSelectionSet replaces the selections of selectionSetRef with kept, adding a
// __typename when pruning removed every selection so the set is never empty.
func (u *partialUnionPruner) replaceSelectionSet(selectionSetRef int, kept []int) {
	u.operation.EmptySelectionSet(selectionSetRef)
	for _, selectionRef := range kept {
		u.operation.AddSelectionRefToSelectionSet(selectionSetRef, selectionRef)
	}
	if len(kept) == 0 {
		u.operation.AddSelectionRefToSelectionSet(selectionSetRef, u.newTypenameSelection())
	}
}

// datasourceUnionMembers returns the named union's members as defined by the
// datasource's upstream schema, or nil when the datasource is unknown.
func (u *partialUnionPruner) datasourceUnionMembers(dsHash DSHash, unionTypeName string) []string {
	ds, ok := u.dataSourceByHash[dsHash]
	if !ok {
		return nil
	}
	return upstreamUnionMemberNames(ds, unionTypeName)
}

// tryMarkResponseOnly attempts to keep an inline fragment in the response while
// excluding its leaf fields from the upstream fetch (so they resolve to null). It
// adds a __typename to the fragment when needed so the upstream fragment is never
// empty after the leaf fields are excluded, and records the leaf field refs as
// response-only.
//
// Returns marked=true when the fragment was successfully made response-only.
// Returns onlyTypename=true when the fragment contains only __typename (nothing to
// null out - the caller keeps it as a normal fetch). Both false means the fragment
// has nested selections that cannot be safely nulled and should be dropped.
func (u *partialUnionPruner) tryMarkResponseOnly(inlineFragmentRef int) (marked bool, onlyTypename bool) {
	selectionSetRef, ok := u.operation.InlineFragmentSelectionSet(inlineFragmentRef)
	if !ok {
		return false, false
	}

	allSelections := u.operation.SelectionSets[selectionSetRef].SelectionRefs
	fieldSelections := u.operation.SelectionSetFieldSelections(selectionSetRef)
	if len(allSelections) != len(fieldSelections) {
		// nested inline fragments / fragment spreads - unsafe to null out.
		return false, false
	}

	leafFieldRefs := make([]int, 0, len(fieldSelections))
	hasTypename := false
	for _, selectionRef := range fieldSelections {
		fieldRef := u.operation.Selections[selectionRef].Ref
		if _, hasChildren := u.operation.FieldSelectionSet(fieldRef); hasChildren {
			// nested object selection - unsafe to null out.
			return false, false
		}
		if u.operation.FieldNameString(fieldRef) == "__typename" {
			hasTypename = true
			continue
		}
		leafFieldRefs = append(leafFieldRefs, fieldRef)
	}

	if len(leafFieldRefs) == 0 {
		return false, true
	}

	if !hasTypename {
		u.operation.AddSelectionRefToSelectionSet(selectionSetRef, u.newTypenameSelection())
	}

	for _, fieldRef := range leafFieldRefs {
		u.responseOnly[fieldRef] = struct{}{}
	}

	return true, false
}

// newTypenameSelection creates a __typename field selection in the operation document
// and returns its selection ref.
func (u *partialUnionPruner) newTypenameSelection() int {
	field := u.operation.AddField(ast.Field{
		Name: u.operation.Input.AppendInputString("__typename"),
	})
	return u.operation.AddSelectionToDocument(ast.Selection{
		Ref:  field.Ref,
		Kind: ast.SelectionKindField,
	})
}

// buildParentFieldMap maps every field ref in the operation to its parent field ref
// (ast.InvalidRef for root fields). Inline fragments are transparent: a field inside
// `... on Member { ... }` keeps the union field as its parent, which is what matters
// for resolving the reachability of object-field chains.
func buildParentFieldMap(operation *ast.Document) map[int]int {
	parentByFieldRef := make(map[int]int)

	var walkSelectionSet func(selectionSetRef, parentFieldRef int)
	walkSelectionSet = func(selectionSetRef, parentFieldRef int) {
		if selectionSetRef == ast.InvalidRef {
			return
		}
		for _, selectionRef := range operation.SelectionSets[selectionSetRef].SelectionRefs {
			selection := operation.Selections[selectionRef]
			switch selection.Kind {
			case ast.SelectionKindField:
				fieldRef := selection.Ref
				parentByFieldRef[fieldRef] = parentFieldRef
				if childSelectionSetRef, ok := operation.FieldSelectionSet(fieldRef); ok {
					walkSelectionSet(childSelectionSetRef, fieldRef)
				}
			case ast.SelectionKindInlineFragment:
				if childSelectionSetRef, ok := operation.InlineFragmentSelectionSet(selection.Ref); ok {
					walkSelectionSet(childSelectionSetRef, parentFieldRef)
				}
			}
		}
	}

	for i := range operation.OperationDefinitions {
		operationDefinition := operation.OperationDefinitions[i]
		if !operationDefinition.HasSelections {
			continue
		}
		walkSelectionSet(operationDefinition.SelectionSet, ast.InvalidRef)
	}

	return parentByFieldRef
}

// isEntityType reports whether the named type is an entity (or otherwise an
// independently resolvable root node) in any datasource. Such types are reachable
// via entity hops and are handled by the existing planner, so the partial-union pass
// leaves unions with entity members untouched.
func (u *partialUnionPruner) isEntityType(typeName string) bool {
	for _, ds := range u.dataSourceByHash {
		if ds.HasEntity(typeName) || ds.HasRootNodeWithTypename(typeName) {
			return true
		}
	}
	return false
}

// upstreamUnionMemberNames returns the member type names of the named union as
// defined by the datasource's upstream (subgraph) schema. Returns nil if the
// datasource has no upstream schema or does not define the union.
func upstreamUnionMemberNames(ds DataSource, unionTypeName string) []string {
	upstreamDefinition, ok := ds.UpstreamSchema()
	if !ok {
		return nil
	}
	unionNode, ok := upstreamDefinition.NodeByNameStr(unionTypeName)
	if !ok || unionNode.Kind != ast.NodeKindUnionTypeDefinition {
		return nil
	}
	members, _ := upstreamDefinition.UnionTypeDefinitionMemberTypeNames(unionNode.Ref)
	return members
}
