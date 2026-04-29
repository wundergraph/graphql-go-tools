package grpcdatasource

import (
	"fmt"
	"strconv"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type v2ResponseFrameKind uint8

const (
	v2ResponseFrameKindNull v2ResponseFrameKind = iota
	v2ResponseFrameKindObject
	v2ResponseFrameKindArray
	v2ResponseFrameKindString
	v2ResponseFrameKindNumber
	v2ResponseFrameKindBool
)

type v2ResponseFrameBuilder struct {
	nodes  []v2ResponseFrameNode
	buffer []byte
}

type v2ResponseFrameNode struct {
	kind         v2ResponseFrameKind
	boolValue    bool
	stringValue  string
	objectFields []v2ResponseFrameField
	arrayValues  []int
}

type v2ResponseFrameField struct {
	name  string
	value int
}

type v2NativeMergeResult struct {
	frame *v2ResponseFrameBuilder
	root  int
}

func newV2ResponseFrameBuilder() *v2ResponseFrameBuilder {
	return &v2ResponseFrameBuilder{
		nodes: make([]v2ResponseFrameNode, 0, 32),
	}
}

func (b *v2ResponseFrameBuilder) reset() {
	for i := range b.nodes {
		b.nodes[i].boolValue = false
		b.nodes[i].stringValue = ""
		b.nodes[i].objectFields = b.nodes[i].objectFields[:0]
		b.nodes[i].arrayValues = b.nodes[i].arrayValues[:0]
	}
	b.nodes = b.nodes[:0]
	b.buffer = b.buffer[:0]
}

func (b *v2ResponseFrameBuilder) newObject() int {
	return b.newNode(v2ResponseFrameNode{kind: v2ResponseFrameKindObject})
}

func (b *v2ResponseFrameBuilder) newArray() int {
	return b.newNode(v2ResponseFrameNode{kind: v2ResponseFrameKindArray})
}

func (b *v2ResponseFrameBuilder) newString(value string) int {
	return b.newNode(v2ResponseFrameNode{kind: v2ResponseFrameKindString, stringValue: value})
}

func (b *v2ResponseFrameBuilder) newNumber(value string) int {
	return b.newNode(v2ResponseFrameNode{kind: v2ResponseFrameKindNumber, stringValue: value})
}

func (b *v2ResponseFrameBuilder) newBool(value bool) int {
	return b.newNode(v2ResponseFrameNode{kind: v2ResponseFrameKindBool, boolValue: value})
}

func (b *v2ResponseFrameBuilder) newNull() int {
	return b.newNode(v2ResponseFrameNode{kind: v2ResponseFrameKindNull})
}

func (b *v2ResponseFrameBuilder) newNode(node v2ResponseFrameNode) int {
	index := len(b.nodes)
	if index < cap(b.nodes) {
		b.nodes = b.nodes[:index+1]
		existing := &b.nodes[index]
		objectFields := existing.objectFields[:0]
		arrayValues := existing.arrayValues[:0]
		*existing = node
		if node.kind == v2ResponseFrameKindObject {
			existing.objectFields = objectFields
		}
		if node.kind == v2ResponseFrameKindArray {
			existing.arrayValues = arrayValues
		}
		return index
	}

	b.nodes = append(b.nodes, node)
	return index
}

func (b *v2ResponseFrameBuilder) setObjectField(objectIndex int, name string, valueIndex int) {
	object := &b.nodes[objectIndex]
	for i := range object.objectFields {
		if object.objectFields[i].name == name {
			object.objectFields[i].value = valueIndex
			return
		}
	}
	object.objectFields = append(object.objectFields, v2ResponseFrameField{
		name:  name,
		value: valueIndex,
	})
}

func (b *v2ResponseFrameBuilder) getObjectField(objectIndex int, name string) (int, bool) {
	object := &b.nodes[objectIndex]
	for i := range object.objectFields {
		if object.objectFields[i].name == name {
			return object.objectFields[i].value, true
		}
	}
	return 0, false
}

func (b *v2ResponseFrameBuilder) appendArrayItem(arrayIndex int, valueIndex int) {
	array := &b.nodes[arrayIndex]
	array.arrayValues = append(array.arrayValues, valueIndex)
}

func (b *v2ResponseFrameBuilder) marshalDataEnvelope(root int) []byte {
	b.buffer = b.buffer[:0]
	b.buffer = append(b.buffer, `{"data":`...)
	b.buffer = b.appendNodeJSON(b.buffer, root)
	b.buffer = append(b.buffer, '}')
	out := b.buffer
	b.buffer = nil
	return out
}

func (b *v2ResponseFrameBuilder) dataEnvelopeValue(a arena.Arena, root int) *astjson.Value {
	data := astjson.ObjectValue(a)
	data.Set(a, "data", b.nodeValue(a, root))
	return data
}

func (b *v2ResponseFrameBuilder) nodeValue(a arena.Arena, nodeIndex int) *astjson.Value {
	node := &b.nodes[nodeIndex]
	switch node.kind {
	case v2ResponseFrameKindNull:
		return astjson.NullValue
	case v2ResponseFrameKindObject:
		obj := astjson.ObjectValue(a)
		for i := range node.objectFields {
			obj.Set(a, node.objectFields[i].name, b.nodeValue(a, node.objectFields[i].value))
		}
		return obj
	case v2ResponseFrameKindArray:
		arr := astjson.ArrayValue(a)
		for i := range node.arrayValues {
			arr.SetArrayItem(a, i, b.nodeValue(a, node.arrayValues[i]))
		}
		return arr
	case v2ResponseFrameKindString:
		return astjson.StringValue(a, node.stringValue)
	case v2ResponseFrameKindNumber:
		return astjson.NumberValue(a, node.stringValue)
	case v2ResponseFrameKindBool:
		if node.boolValue {
			return astjson.TrueValue(a)
		}
		return astjson.FalseValue(a)
	default:
		panic(fmt.Sprintf("unsupported response frame kind %d", node.kind))
	}
}

func (b *v2ResponseFrameBuilder) appendNodeJSON(dst []byte, nodeIndex int) []byte {
	node := &b.nodes[nodeIndex]

	switch node.kind {
	case v2ResponseFrameKindNull:
		return append(dst, "null"...)
	case v2ResponseFrameKindObject:
		dst = append(dst, '{')
		for i := range node.objectFields {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = strconv.AppendQuote(dst, node.objectFields[i].name)
			dst = append(dst, ':')
			dst = b.appendNodeJSON(dst, node.objectFields[i].value)
		}
		return append(dst, '}')
	case v2ResponseFrameKindArray:
		dst = append(dst, '[')
		for i := range node.arrayValues {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = b.appendNodeJSON(dst, node.arrayValues[i])
		}
		return append(dst, ']')
	case v2ResponseFrameKindString:
		return strconv.AppendQuote(dst, node.stringValue)
	case v2ResponseFrameKindNumber:
		return append(dst, node.stringValue...)
	case v2ResponseFrameKindBool:
		if node.boolValue {
			return append(dst, "true"...)
		}
		return append(dst, "false"...)
	default:
		panic(fmt.Sprintf("unsupported response frame kind %d", node.kind))
	}
}

func (b *v2ResponseFrameBuilder) flatten(nodeIndex int, path ast.Path) ([]int, error) {
	if len(path) == 0 {
		if b.nodes[nodeIndex].kind == v2ResponseFrameKindArray {
			return append([]int(nil), b.nodes[nodeIndex].arrayValues...), nil
		}
		return []int{nodeIndex}, nil
	}

	node := &b.nodes[nodeIndex]
	switch node.kind {
	case v2ResponseFrameKindObject:
		next, ok := b.getObjectField(nodeIndex, path[0].FieldName.String())
		if !ok {
			return nil, fmt.Errorf("response path %s not found", path.String())
		}
		return b.flatten(next, path[1:])
	case v2ResponseFrameKindArray:
		result := make([]int, 0, len(node.arrayValues))
		for i := range node.arrayValues {
			values, err := b.flatten(node.arrayValues[i], path)
			if err != nil {
				return nil, err
			}
			result = append(result, values...)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("cannot traverse response path %s through node kind %d", path.String(), node.kind)
	}
}

func (r *v2NativeMergeResult) MarshalTo(dst []byte) []byte {
	return append(dst[:0], r.frame.marshalDataEnvelope(r.root)...)
}

func (r *v2NativeMergeResult) MergeInto(a arena.Arena, items []*astjson.Value, post resolve.PostProcessingConfiguration, batchStats [][]*astjson.Value) (*astjson.Value, error) {
	nodeIndex, ok := r.selectDataNode(post.SelectResponseDataPath)
	if !ok {
		return astjson.NullValue, nil
	}

	if len(items) == 0 {
		return r.frame.nodeValue(a, nodeIndex), nil
	}

	node := &r.frame.nodes[nodeIndex]
	if len(items) == 1 && batchStats == nil {
		return nil, r.mergeNodeIntoItem(a, items[0], nodeIndex, post.MergePath)
	}

	if node.kind != v2ResponseFrameKindArray {
		return nil, fmt.Errorf("expected array response frame node, got %d", node.kind)
	}

	if batchStats != nil {
		if len(batchStats) != len(node.arrayValues) {
			return nil, fmt.Errorf("invalid batch item count: expected %d, got %d", len(batchStats), len(node.arrayValues))
		}
		for batchIndex, targets := range batchStats {
			for _, target := range targets {
				if err := r.mergeNodeIntoItem(a, target, node.arrayValues[batchIndex], post.MergePath); err != nil {
					return nil, err
				}
			}
		}
		return nil, nil
	}

	if len(items) != len(node.arrayValues) {
		return nil, fmt.Errorf("invalid batch item count: expected %d, got %d", len(items), len(node.arrayValues))
	}
	for i := range items {
		if err := r.mergeNodeIntoItem(a, items[i], node.arrayValues[i], post.MergePath); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func (r *v2NativeMergeResult) selectDataNode(path []string) (int, bool) {
	nodeIndex := r.root
	if len(path) > 0 && path[0] == "data" {
		path = path[1:]
	}
	for _, segment := range path {
		node := &r.frame.nodes[nodeIndex]
		switch node.kind {
		case v2ResponseFrameKindObject:
			next, ok := r.frame.getObjectField(nodeIndex, segment)
			if !ok {
				return 0, false
			}
			nodeIndex = next
		case v2ResponseFrameKindArray:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(node.arrayValues) {
				return 0, false
			}
			nodeIndex = node.arrayValues[index]
		default:
			return 0, false
		}
	}
	return nodeIndex, true
}

func (r *v2NativeMergeResult) mergeNodeIntoItem(a arena.Arena, target *astjson.Value, nodeIndex int, path []string) error {
	if len(path) == 0 {
		if r.frame.nodes[nodeIndex].kind == v2ResponseFrameKindObject && target.Type() == astjson.TypeObject {
			r.mergeObjectNodeIntoObject(a, target, nodeIndex)
			return nil
		}
		value := r.frame.nodeValue(a, nodeIndex)
		_, _, err := astjson.MergeValuesWithPath(a, target, value)
		return err
	}

	if target.Type() != astjson.TypeObject {
		value := r.frame.nodeValue(a, nodeIndex)
		_, _, err := astjson.MergeValuesWithPath(a, target, value, path...)
		return err
	}

	parent := r.ensureObjectPath(a, target, path[:len(path)-1])
	leaf := path[len(path)-1]
	existing := parent.Get(leaf)
	if existing != nil && existing.Type() == astjson.TypeObject && r.frame.nodes[nodeIndex].kind == v2ResponseFrameKindObject {
		r.mergeObjectNodeIntoObject(a, existing, nodeIndex)
		return nil
	}
	parent.Set(a, leaf, r.frame.nodeValue(a, nodeIndex))
	return nil
}

func (r *v2NativeMergeResult) ensureObjectPath(a arena.Arena, target *astjson.Value, path []string) *astjson.Value {
	current := target
	for _, segment := range path {
		next := current.Get(segment)
		if next == nil || next.Type() != astjson.TypeObject {
			next = astjson.ObjectValue(a)
			current.Set(a, segment, next)
		}
		current = next
	}
	return current
}

func (r *v2NativeMergeResult) mergeObjectNodeIntoObject(a arena.Arena, target *astjson.Value, nodeIndex int) {
	node := &r.frame.nodes[nodeIndex]
	for i := range node.objectFields {
		field := node.objectFields[i]
		childNodeIndex := field.value
		childNode := &r.frame.nodes[childNodeIndex]
		existing := target.Get(field.name)
		if existing != nil && existing.Type() == astjson.TypeObject && childNode.kind == v2ResponseFrameKindObject {
			r.mergeObjectNodeIntoObject(a, existing, childNodeIndex)
			continue
		}
		target.Set(a, field.name, r.frame.nodeValue(a, childNodeIndex))
	}
}
