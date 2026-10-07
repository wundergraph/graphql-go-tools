package postprocess

import (
	"bytes"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type mergeFields struct {
	disable bool
}

func (m *mergeFields) Process(node resolve.Node) {
	if m.disable {
		return
	}
	m.traverseNode(node)
}

func (m *mergeFields) ProcessSubscription(node resolve.Node) {
	if m.disable {
		return
	}
	m.traverseNode(node)
}

func (m *mergeFields) traverseNode(node resolve.Node) {
	switch n := node.(type) {
	case *resolve.Object:
		if len(n.Fields) == 1 {
			m.traverseNode(n.Fields[0].Value)
			return
		}
		for i := 0; i < len(n.Fields); i++ {
			// 1. duplicate fields with multiple onTypeNames so they can be merged
			if len(n.Fields[i].OnTypeNames) >= 1 {
				additionalTypeNames := make([][]byte, len(n.Fields[i].OnTypeNames)-1)
				copy(additionalTypeNames, n.Fields[i].OnTypeNames[1:])
				n.Fields[i].OnTypeNames = [][]byte{n.Fields[i].OnTypeNames[0]}
				for j := range additionalTypeNames {
					additionalField := n.Fields[i].Copy()
					additionalField.OnTypeNames = [][]byte{additionalTypeNames[j]}
					n.Fields = append(n.Fields[:i+1], append([]*resolve.Field{additionalField}, n.Fields[i+1:]...)...)
				}
			}
			// 2.
			// Propagate onTypeNames to all children
			// In a later stage, we merge all nested scalar fields with the same name
			// However, scalar fields can originate from different parent types, which is why we need to propagate them here
			m.propagateParentTypeNames(n.Fields[i])
		}
		// 3. merge fields without onTypeNames "over" fields with onTypeNames
		// This is possible because if a field exists without onTypeNames, it will always be resolved
		// There are 2 variants of this:
		// 3.1. if the source and target fields are scalars, the target (with onTypeNames) will be removed
		// This means that scalars with no onTypeNames will overwrite scalars with onTypeNames
		// 3.2. if the source and target fields are objects, all fields from the source will be merged into the target
		// This means that objects with no onTypeNames will be merged into objects with onTypeNames
		for i := 0; i < len(n.Fields); i++ {
			if n.Fields[i].OnTypeNames != nil {
				continue
			}
			for j := 0; j < len(n.Fields); j++ {
				if i == j {
					continue
				}
				if n.Fields[j].OnTypeNames == nil {
					continue
				}
				if bytes.Equal(n.Fields[i].Name, n.Fields[j].Name) {
					m.mergeValues(n.Fields[i], n.Fields[j])
					n.Fields = append(n.Fields[:j], n.Fields[j+1:]...)
					if i > j {
						i--
					}
					j--
				}
			}
		}
		// 4. merge sibling object fields
		for i := 0; i < len(n.Fields); i++ {
			for j := i + 1; j < len(n.Fields); j++ {
				if m.fieldsCanMerge(n.Fields[i], n.Fields[j]) {
					m.mergeValues(n.Fields[i], n.Fields[j])
					n.Fields = append(n.Fields[:j], n.Fields[j+1:]...)
					j--
				}
			}
		}
		// 5. merge sibling scalar fields
		// Once all objects have been merged, we need to merge (deduplicate) all scalar fields that are left
		for i := 0; i < len(n.Fields); i++ {
			// skip objects
			if m.nodeIsScalar(n.Fields[i].Value) {
				for j := 0; j < len(n.Fields); j++ {
					if i == j {
						continue
					}
					if bytes.Equal(n.Fields[i].Name, n.Fields[j].Name) {
						// we don't merge scalars with different onTypeNames to preserve the order of the fields
						if !m.canMergeScalars(n.Fields[i], n.Fields[j]) {
							continue
						}
						m.mergeTypeConditions(n.Fields[i], n.Fields[j])
						n.Fields = append(n.Fields[:j], n.Fields[j+1:]...)
						if i > j {
							i--
						}
						j--
					}
				}
			}
		}
		for i := 0; i < len(n.Fields); i++ {
			m.traverseNode(n.Fields[i].Value)
		}
	case *resolve.Array:
		m.traverseNode(n.Item)
	}
}

func (m *mergeFields) canMergeScalars(left, right *resolve.Field) bool {
	if left.OnTypeNames != nil && right.OnTypeNames != nil {
		if !m.sameOnTypeNames(left.OnTypeNames, right.OnTypeNames) {
			return false
		}
	}
	return true
}

// mergeTypeConditions ORs the two fields' conditions. Different depths within
// one selection are ANDed, so unrelated selections must remain alternatives.
func (m *mergeFields) mergeTypeConditions(left, right *resolve.Field) {
	// Most fields are unconditional; avoid allocating condition groups for them.
	if left.OnTypeNames == nil && left.ParentOnTypeNames == nil && len(left.ParentOnTypeNamesAlternatives) == 0 {
		return
	}
	if right.OnTypeNames == nil && right.ParentOnTypeNames == nil && len(right.ParentOnTypeNamesAlternatives) == 0 {
		left.OnTypeNames, left.ParentOnTypeNames, left.ParentOnTypeNamesAlternatives = nil, nil, nil
		return
	}
	sameOwnTypes := m.sameOnTypeNames(left.OnTypeNames, right.OnTypeNames)
	groups := append(m.parentTypeConditions(left, !sameOwnTypes), m.parentTypeConditions(right, !sameOwnTypes)...)
	if !sameOwnTypes {
		left.OnTypeNames = nil
	}
	left.ParentOnTypeNames = nil
	left.ParentOnTypeNamesAlternatives = nil
	var alternatives [][]resolve.ParentOnTypeNames
	for _, group := range groups {
		// An unrestricted selection makes all other alternatives redundant.
		if len(group) == 0 {
			return
		}
		merged := false
		for i := range alternatives {
			if combined, ok := m.combineParentTypeConditions(alternatives[i], group); ok {
				alternatives[i] = combined
				merged = true
				break
			}
		}
		if !merged {
			alternatives = append(alternatives, group)
		}
	}
	if len(alternatives) == 1 {
		left.ParentOnTypeNames = alternatives[0]
	} else {
		left.ParentOnTypeNamesAlternatives = alternatives
	}
}

func (m *mergeFields) parentTypeConditions(field *resolve.Field, includeOwnTypes bool) [][]resolve.ParentOnTypeNames {
	base := slices.Clone(field.ParentOnTypeNames)
	if includeOwnTypes && field.OnTypeNames != nil {
		base = append(base, resolve.ParentOnTypeNames{Depth: 0, Names: field.OnTypeNames})
	}
	if len(field.ParentOnTypeNamesAlternatives) == 0 {
		return [][]resolve.ParentOnTypeNames{base}
	}
	groups := make([][]resolve.ParentOnTypeNames, len(field.ParentOnTypeNamesAlternatives))
	for i, alternative := range field.ParentOnTypeNamesAlternatives {
		groups[i] = append(slices.Clone(base), alternative...)
	}
	return groups
}

// Factoring out identical conditions is safe only when at most one depth differs:
// (A AND B) OR (A AND C) = A AND (B OR C).
func (m *mergeFields) combineParentTypeConditions(left, right []resolve.ParentOnTypeNames) ([]resolve.ParentOnTypeNames, bool) {
	if len(left) != len(right) {
		return nil, false
	}
	differing := -1
	var names [][]byte
	for i, condition := range left {
		// Repeated depths are separate AND clauses and cannot be factored this way.
		if slices.ContainsFunc(left[:i], func(previous resolve.ParentOnTypeNames) bool { return previous.Depth == condition.Depth }) {
			return nil, false
		}
		j := slices.IndexFunc(right, func(other resolve.ParentOnTypeNames) bool { return condition.Depth == other.Depth })
		if j == -1 {
			return nil, false
		}
		if m.sameOnTypeNames(condition.Names, right[j].Names) {
			continue
		}
		if differing != -1 {
			return nil, false
		}
		differing, names = i, right[j].Names
	}
	combined := slices.Clone(left)
	if differing != -1 {
		combined[differing].Names = m.deduplicateOnTypeNames(append(slices.Clone(combined[differing].Names), names...))
	}
	return combined, true
}

func (m *mergeFields) fieldsCanMerge(left *resolve.Field, right *resolve.Field) bool {
	if !bytes.Equal(left.Name, right.Name) {
		return false
	}
	if left.Value.NodeKind() != right.Value.NodeKind() {
		return false
	}
	if !m.sameOnTypeNames(left.OnTypeNames, right.OnTypeNames) {
		return false
	}
	// scalars with different parent type conditions can't be merged at this point
	// we're handling this case later when we merge scalar fields
	if m.nodeIsScalar(left.Value) && !m.sameParentOnTypeNames(left, right) {
		return false
	}
	return true
}

func (m *mergeFields) deduplicateOnTypeNames(onTypeNames [][]byte) [][]byte {
	uniqueTypeNames := make(map[string]struct{}, len(onTypeNames))
	for _, typeName := range onTypeNames {
		uniqueTypeNames[string(typeName)] = struct{}{}
	}
	if len(uniqueTypeNames) == len(onTypeNames) {
		return onTypeNames
	}
	result := make([][]byte, 0, len(uniqueTypeNames))
	for typeName := range uniqueTypeNames {
		result = append(result, []byte(typeName))
	}
	return result
}

func (m *mergeFields) sameOnTypeNames(left, right [][]byte) bool {
	if len(left) != len(right) {
		return false
	}
WithNext:
	for i := range left {
		for j := range right {
			if bytes.Equal(left[i], right[j]) {
				continue WithNext
			}
		}
		return false
	}
	return true
}

func (m *mergeFields) sameParentOnTypeNames(left, right *resolve.Field) bool {
	if len(left.ParentOnTypeNames) != len(right.ParentOnTypeNames) {
		return false
	}
	for i := range left.ParentOnTypeNames {
		for j := range right.ParentOnTypeNames {
			if left.ParentOnTypeNames[i].Depth != right.ParentOnTypeNames[j].Depth {
				continue
			}
			if !m.sameOnTypeNames(left.ParentOnTypeNames[i].Names, right.ParentOnTypeNames[j].Names) {
				continue
			}
			break
		}
		return false
	}
	return true
}

func (m *mergeFields) mergeValues(left, right *resolve.Field) {
	// A merged object or array must remain reachable for either field's parent
	// types. Its children retain their own conditions to filter nested selections.
	m.mergeTypeConditions(left, right)
	switch l := left.Value.(type) {
	case *resolve.Object:
		r := right.Value.(*resolve.Object)
		l.Fields = append(l.Fields, r.Fields...)
	case *resolve.Array:
		r := right.Value.(*resolve.Array)
		if l.Item.NodeKind() == resolve.NodeKindObject {
			lo := l.Item.(*resolve.Object)
			ro := r.Item.(*resolve.Object)
			lo.Fields = append(lo.Fields, ro.Fields...)
		}
	}
}

func (m *mergeFields) nodeIsScalar(node resolve.Node) bool {
	switch node.(type) {
	case *resolve.Object, *resolve.Array:
		return false
	}
	return true
}

func (m *mergeFields) propagateParentTypeNames(field *resolve.Field) {
	if field.OnTypeNames == nil {
		return
	}
	m.setParentTypeNames(field, field.OnTypeNames, 1)
}

// setParentTypeNames recursively sets the parent type names for all children of a field
// increasing the depth by 1 for each level
func (m *mergeFields) setParentTypeNames(field *resolve.Field, typeNames [][]byte, depth int) {
	switch field.Value.NodeKind() {
	case resolve.NodeKindObject:
		object := field.Value.(*resolve.Object)
		for i := range object.Fields {
			object.Fields[i].ParentOnTypeNames = append(object.Fields[i].ParentOnTypeNames, resolve.ParentOnTypeNames{
				Depth: depth,
				Names: typeNames,
			})
			m.setParentTypeNames(object.Fields[i], typeNames, depth+1)
		}
	case resolve.NodeKindArray:
		array := field.Value.(*resolve.Array)
		if array.Item.NodeKind() == resolve.NodeKindObject {
			object := array.Item.(*resolve.Object)
			for i := range object.Fields {
				object.Fields[i].ParentOnTypeNames = append(object.Fields[i].ParentOnTypeNames, resolve.ParentOnTypeNames{
					Depth: depth,
					Names: typeNames,
				})
				m.setParentTypeNames(object.Fields[i], typeNames, depth+1)
			}
		}
	}
}
