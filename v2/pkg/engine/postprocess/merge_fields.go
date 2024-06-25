package postprocess

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type MergeFields struct {
}

func (m *MergeFields) Process(node resolve.Node) {
	m.traverseNode(node)
}

func (m *MergeFields) ProcessSubscription(node resolve.Node, trigger *resolve.GraphQLSubscriptionTrigger) {
	m.traverseNode(node)
}

func (m *MergeFields) traverseNode(node resolve.Node) {
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
				for j := 0; j < len(additionalTypeNames); j++ {
					additionalField := &resolve.Field{
						Name:        n.Fields[i].Name,
						Value:       n.Fields[i].Value.Copy(),
						Position:    n.Fields[i].Position,
						OnTypeNames: [][]byte{additionalTypeNames[j]},
						Info:        n.Fields[i].Info,
					}
					n.Fields = append(n.Fields[:i+1], append([]*resolve.Field{additionalField}, n.Fields[i+1:]...)...)
				}
			}
		}
		// 2. merge fields without onTypeNames "over" fields with onTypeNames
		// This is possible because if a field exists without onTypeNames, it will always be resolved
		// There are 2 variants of this:
		// 2.1. if the source and target fields are scalars, the target (with onTypeNames) will be removed
		// This means that scalars with no onTypeNames will overwrite scalars with onTypeNames
		// 2.2. if the source and target fields are objects, all fields from the source will be merged into the target
		// This means that objects with no onTypeNames will be merged into objects with onTypeNames
		for i := 0; i < len(n.Fields); i++ {
			removeI := false
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
					if m.nodeIsScalar(n.Fields[i].Value) {
						n.Fields = append(n.Fields[:j], n.Fields[j+1:]...)
						j--
					} else {
						removeI = true
						m.mergeValues(n.Fields[j].Value, n.Fields[i].Value)
					}
				}
			}
			if removeI {
				n.Fields = append(n.Fields[:i], n.Fields[i+1:]...)
				i--
			}
		}
		// 3. merge sibling scalar fields that have onTypeNames
		for i := 0; i < len(n.Fields); i++ {
			if n.Fields[i].OnTypeNames == nil {
				continue
			}
			if !m.nodeIsScalar(n.Fields[i].Value) {
				continue
			}
			for j := i + 1; j < len(n.Fields); j++ {
				if n.Fields[j].OnTypeNames == nil {
					continue
				}
				if !m.nodeIsScalar(n.Fields[j].Value) {
					continue
				}
				if bytes.Equal(n.Fields[i].Name, n.Fields[j].Name) {
					n.Fields[i].OnTypeNames = append(n.Fields[i].OnTypeNames, n.Fields[j].OnTypeNames...)
					n.Fields = append(n.Fields[:j], n.Fields[j+1:]...)
					j--
				}
			}
			n.Fields[i].OnTypeNames = m.deduplicateOnTypeNames(n.Fields[i].OnTypeNames)
		}
		// 4. merge sibling object fields
		for i := 0; i < len(n.Fields); i++ {
			for j := i + 1; j < len(n.Fields); j++ {
				if m.fieldsCanMerge(n.Fields[i], n.Fields[j]) {
					m.mergeValues(n.Fields[i].Value, n.Fields[j].Value)
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

func (m *MergeFields) fieldsCanMerge(left *resolve.Field, right *resolve.Field) bool {
	if !bytes.Equal(left.Name, right.Name) {
		return false
	}
	if left.Value.NodeKind() != right.Value.NodeKind() {
		return false
	}
	if !m.sameOnTypeNames(left.OnTypeNames, right.OnTypeNames) {
		return false
	}
	return true
}

func (m *MergeFields) deduplicateOnTypeNames(onTypeNames [][]byte) [][]byte {
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

func (m *MergeFields) sameOnTypeNames(left, right [][]byte) bool {
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

func (m *MergeFields) mergeValues(left, right resolve.Node) {
	switch v := left.(type) {
	case *resolve.Object:
		r := right.(*resolve.Object)
		v.Fields = append(v.Fields, r.Fields...)
		if r.Fetch != nil {
			v.Fetch = r.Fetch
		}
	case *resolve.Array:
		m.mergeValues(v.Item, right.(*resolve.Array).Item)
	}
}

func (m *MergeFields) nodeIsScalar(node resolve.Node) bool {
	switch node.(type) {
	case *resolve.Object, *resolve.Array:
		return false
	}
	return true
}
