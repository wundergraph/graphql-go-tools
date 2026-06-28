package plan

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

// fieldMergingAliasVisitor walks the operation before data source selection and assigns
// planner-generated aliases to fields that share a response name across concrete union/interface
// members but whose subgraph types differ only in nullability. Running before node selection keeps
// the response-name based paths consistent across every subsequent planning phase.
type fieldMergingAliasVisitor struct {
	walker      *astvisitor.Walker
	dataSources []DataSource
	rewriters   []*fieldSelectionRewriter
}

func (v *fieldMergingAliasVisitor) EnterDocument(operation, definition *ast.Document) {
	v.rewriters = v.rewriters[:0]
	for _, ds := range v.dataSources {
		// A data source without an upstream schema cannot be the one that would reject the merge.
		rewriter, err := newFieldSelectionRewriter(operation, definition, ds)
		if err != nil {
			continue
		}
		v.rewriters = append(v.rewriters, rewriter)
	}
}

func (v *fieldMergingAliasVisitor) EnterField(ref int) {
	// Every data source is consulted: a field already aliased by an earlier one is skipped (its
	// alias is now defined), so the pass is idempotent and still catches a conflict that only some
	// subgraph's schema exhibits.
	for _, rewriter := range v.rewriters {
		if _, err := rewriter.aliasNullabilityConflictingMemberFields(ref); err != nil {
			v.walker.StopWithInternalErr(fmt.Errorf("failed to alias conflicting member fields: %w", err))
			return
		}
	}
}

// upstreamFieldMergingAliasPrefix marks an alias that the planner generated (not the client)
// to disambiguate fields which share a response name across non-overlapping concrete members
// of a union/interface but whose types differ in nullability in the subgraph schema
// (e.g. User.id: ID! vs Admin.id: ID).
//
// Both the strict GraphQL "OverlappingFieldsCanBeMerged" validation in this engine and a real
// subgraph (graphql-js) reject such a selection set
//
//	accounts { ... on User { id } ... on Admin { id } }
//
// even though the two branches can never co-resolve. The spec-suggested remedy is to use a
// different alias on each field. We do exactly that, sending
//
//	accounts { ... on User { __sg_merge_User_id: id } ... on Admin { __sg_merge_Admin_id: id } }
//
// and recover the original response name when building the resolve tree (see Visitor.EnterField).
const upstreamFieldMergingAliasPrefix = "__sg_merge_"

// aliasNullabilityConflictingMemberFields detects fields that share a response name across
// concrete object-type members of the abstract field's selection set and whose subgraph types
// differ only in nullability. Each such field is given a deterministic, planner-generated alias so
// that the upstream operation is valid against the subgraph schema. It is a no-op unless a genuine
// nullability-only conflict exists, keeping the blast radius limited to that case.
func (r *fieldSelectionRewriter) aliasNullabilityConflictingMemberFields(fieldRef int) (changed bool, err error) {
	// Scalar fields (and any field without a selection set) carry no member fragments to compare.
	if !r.operation.FieldHasSelections(fieldRef) {
		return false, nil
	}

	info, err := r.collectFieldInformation(fieldRef)
	if err != nil {
		return false, err
	}

	// Only concrete object-type members are mutually exclusive at runtime. We conservatively skip
	// interface fragments, since a single concrete type could implement two interfaces and overlap.
	if len(info.inlineFragmentsOnObjects) < 2 {
		return false, nil
	}

	type occurrence struct {
		typeName string
		fieldRef int
		typeRef  int
	}

	occurrencesByName := make(map[string][]occurrence)
	for _, fragment := range info.inlineFragmentsOnObjects {
		node, hasNode := r.upstreamDefinition.NodeByNameStr(fragment.typeName)
		if !hasNode {
			continue
		}
		for _, field := range fragment.selectionSetInfo.fields {
			if field.fieldName == typeNameField {
				continue
			}
			memberFieldRef := r.operation.Selections[field.fieldSelectionRef].Ref
			// A client-provided alias owns the response name; never overwrite it.
			if r.operation.FieldAliasIsDefined(memberFieldRef) {
				continue
			}
			fieldDefinitionRef, exists := r.upstreamDefinition.NodeFieldDefinitionByName(node, ast.ByteSlice(field.fieldName))
			if !exists {
				continue
			}
			occurrencesByName[field.fieldName] = append(occurrencesByName[field.fieldName], occurrence{
				typeName: fragment.typeName,
				fieldRef: memberFieldRef,
				typeRef:  r.upstreamDefinition.FieldDefinitionType(fieldDefinitionRef),
			})
		}
	}

	for fieldName, occurrences := range occurrencesByName {
		if len(occurrences) < 2 {
			continue
		}
		typeRefs := make([]int, len(occurrences))
		for i := range occurrences {
			typeRefs[i] = occurrences[i].typeRef
		}
		if !r.memberFieldTypesNeedAlias(typeRefs) {
			continue
		}
		for _, occurrence := range occurrences {
			r.setGeneratedFieldAlias(occurrence.fieldRef, occurrence.typeName, fieldName)
		}
		changed = true
	}

	return changed, nil
}

// memberFieldTypesNeedAlias reports whether the member field types differ only in nullability.
// Aliasing is required (and safe) when every pair is compatible ignoring nullability but at least
// one pair is not byte-for-byte equal. A genuinely incompatible pair (e.g. Int vs String) is left
// untouched so it surfaces through the normal validation path rather than being silently masked.
func (r *fieldSelectionRewriter) memberFieldTypesNeedAlias(typeRefs []int) bool {
	nullabilityDiffers := false
	for i := range typeRefs {
		for j := i + 1; j < len(typeRefs); j++ {
			if r.upstreamDefinition.TypesAreEqualDeep(typeRefs[i], typeRefs[j]) {
				continue
			}
			// Not equal: only safe to alias when the sole difference is nullability.
			if !r.upstreamDefinition.TypesAreCompatibleIgnoringNullability(typeRefs[i], typeRefs[j]) {
				return false
			}
			nullabilityDiffers = true
		}
	}
	return nullabilityDiffers
}

// setGeneratedFieldAlias assigns a deterministic planner-generated alias to a member field.
func (r *fieldSelectionRewriter) setGeneratedFieldAlias(fieldRef int, typeName, fieldName string) {
	alias := upstreamFieldMergingAliasPrefix + typeName + "_" + fieldName
	r.operation.Fields[fieldRef].Alias = ast.Alias{
		IsDefined: true,
		Name:      r.operation.Input.AppendInputBytes([]byte(alias)),
	}
}
