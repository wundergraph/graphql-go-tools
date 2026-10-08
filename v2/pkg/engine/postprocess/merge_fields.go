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
			// 2. propagate onTypeNames to all descendants as parent type conditions
			// The descendants merge in a later pass, and the conditions keep the fragment path they came from.
			m.propagateParentTypeNames(n.Fields[i])
		}
		// 3. merge fields without onTypeNames "over" fields with onTypeNames
		// A field without onTypeNames always resolves, so the merged field keeps no type condition.
		// For objects, the children of the conditional field move into the unconditional one.
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
					m.mergeTypeConditions(n.Fields[i], n.Fields[j])
					m.mergeValues(n.Fields[i], n.Fields[j])
					n.Fields = append(n.Fields[:j], n.Fields[j+1:]...)
					if i > j {
						i--
					}
					j--
				}
			}
		}
		// 4. merge sibling fields with the same name and the same onTypeNames
		// Fields with different onTypeNames stay separate to preserve the order of the fields.
		for i := 0; i < len(n.Fields); i++ {
			for j := i + 1; j < len(n.Fields); j++ {
				if m.fieldsCanMerge(n.Fields[i], n.Fields[j]) {
					m.mergeTypeConditions(n.Fields[i], n.Fields[j])
					m.mergeValues(n.Fields[i], n.Fields[j])
					n.Fields = append(n.Fields[:j], n.Fields[j+1:]...)
					j--
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

// mergeTypeConditions ORs the type conditions of right into left.
// A field without any condition always renders, so it clears the merged conditions.
// Groups that differ at one depth only fold into one group, which keeps the common case at one group.
func (m *mergeFields) mergeTypeConditions(left, right *resolve.Field) {
	if m.unconditional(left) {
		return
	}
	if m.unconditional(right) {
		left.OnTypeNames = nil
		left.ParentOnTypeNames = nil
		return
	}
	// Different onTypeNames cannot stay on the merged field, so they move into the groups as depth 0.
	foldOwnTypes := !m.sameOnTypeNames(left.OnTypeNames, right.OnTypeNames)
	groups := append(m.conditionGroups(left, foldOwnTypes), m.conditionGroups(right, foldOwnTypes)...)
	if foldOwnTypes {
		left.OnTypeNames = nil
	}
	left.ParentOnTypeNames = nil
	for _, group := range groups {
		if len(group) == 0 {
			// One path has no parent condition, so the merged field has none.
			left.ParentOnTypeNames = nil
			return
		}
		left.ParentOnTypeNames = m.addConditionGroup(left.ParentOnTypeNames, group)
	}
}

func (m *mergeFields) unconditional(field *resolve.Field) bool {
	return field.OnTypeNames == nil && len(field.ParentOnTypeNames) == 0
}

// conditionGroups returns the parent type condition groups of a field.
// A field without groups has one empty group.
// With foldOwnTypes, the field's own onTypeNames join every group at depth 0.
func (m *mergeFields) conditionGroups(field *resolve.Field, foldOwnTypes bool) [][]resolve.ParentOnTypeNames {
	groups := field.ParentOnTypeNames
	if len(groups) == 0 {
		groups = [][]resolve.ParentOnTypeNames{nil}
	}
	if !foldOwnTypes || field.OnTypeNames == nil {
		return groups
	}
	folded := make([][]resolve.ParentOnTypeNames, len(groups))
	for i, group := range groups {
		folded[i] = append(slices.Clone(group), resolve.ParentOnTypeNames{Depth: 0, Names: field.OnTypeNames})
	}
	return folded
}

// addConditionGroup adds group to groups, folded into an existing group when possible.
func (m *mergeFields) addConditionGroup(groups [][]resolve.ParentOnTypeNames, group []resolve.ParentOnTypeNames) [][]resolve.ParentOnTypeNames {
	for i := range groups {
		if combined, ok := m.combineConditionGroups(groups[i], group); ok {
			groups[i] = combined
			return groups
		}
	}
	return append(groups, group)
}

// combineConditionGroups folds two groups into one when they differ at one depth at most.
// (A AND B) OR (A AND C) = A AND (B OR C).
// Propagation emits one entry per ancestor depth, so a depth occurs once in a group.
func (m *mergeFields) combineConditionGroups(left, right []resolve.ParentOnTypeNames) ([]resolve.ParentOnTypeNames, bool) {
	if len(left) != len(right) {
		return nil, false
	}
	differing := -1
	var names [][]byte
	for i, condition := range left {
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
	return m.sameOnTypeNames(left.OnTypeNames, right.OnTypeNames)
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

// mergeValues moves the children of right into left.
// The children keep their own conditions, which filter the nested selections.
func (m *mergeFields) mergeValues(left, right *resolve.Field) {
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

func (m *mergeFields) propagateParentTypeNames(field *resolve.Field) {
	if field.OnTypeNames == nil {
		return
	}
	m.setParentTypeNames(field, field.OnTypeNames, 1)
}

// setParentTypeNames adds a condition for typeNames to every descendant of field.
// The depth grows by one per object level, and an array adds no depth.
func (m *mergeFields) setParentTypeNames(field *resolve.Field, typeNames [][]byte, depth int) {
	var object *resolve.Object
	switch value := field.Value.(type) {
	case *resolve.Object:
		object = value
	case *resolve.Array:
		object, _ = value.Item.(*resolve.Object)
	}
	if object == nil {
		return
	}
	condition := resolve.ParentOnTypeNames{Depth: depth, Names: typeNames}
	for _, child := range object.Fields {
		m.appendConditionToGroups(child, condition)
		m.setParentTypeNames(child, typeNames, depth+1)
	}
}

// appendConditionToGroups ANDs condition into every group of the field.
func (m *mergeFields) appendConditionToGroups(field *resolve.Field, condition resolve.ParentOnTypeNames) {
	if len(field.ParentOnTypeNames) == 0 {
		field.ParentOnTypeNames = [][]resolve.ParentOnTypeNames{{condition}}
		return
	}
	for i := range field.ParentOnTypeNames {
		field.ParentOnTypeNames[i] = append(field.ParentOnTypeNames[i], condition)
	}
}
