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
		for i := 0; i < len(n.Fields); i++ {
			for j := i + 1; j < len(n.Fields); j++ {
				if m.fieldsCanMerge(n.Fields[i], n.Fields[j]) {
					m.mergeValues(n.Fields[i].Value, n.Fields[j].Value)
					n.Fields = append(n.Fields[:j], n.Fields[j+1:]...)
					j--
				}
			}
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
