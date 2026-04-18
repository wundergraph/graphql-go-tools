package grpcdatasource

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// v2ResponseFrame is a flat, index-based response tree representation
// replacing per-node astjson.Value allocation for V2's hot path.
//
// Design rationale (from codex hollow-playroom Stage 21):
//   - All nodes live in one slice; cross-refs are int indices.
//   - Reset-in-place: slice length zeroed, capacity retained. One backing
//     allocation serves many requests when pooled via sync.Pool.
//   - No pointer chasing during traversal; the node array fits cache lines
//     better than astjson's pointer-linked Value tree.
//
// At the interface boundary the V2 datasource must return *astjson.Value
// (current resolve.DataSource contract). The frame materializes the final
// Value via a single pass in toAstjson(arena, root) — one astjson allocation
// per node, but only at the very end and only for the subset reached from
// root.
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
	buffer []byte // scratch for marshalDataEnvelope
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

func newV2ResponseFrameBuilder() *v2ResponseFrameBuilder {
	return &v2ResponseFrameBuilder{
		nodes: make([]v2ResponseFrameNode, 0, 32),
	}
}

// reset clears state but keeps the backing arrays for the node slice and
// every inner object/array slice. Returning to the pool after reset means the
// next request reuses the same allocation.
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

// newNode returns a fresh index, reusing pre-allocated slots from prior requests
// when available. Caller inspects the returned index; existing inner slices are
// preserved (reset to zero length) so they can accept new children without
// reallocating.
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

// setObjectField attaches a name -> child-index entry on an object node.
// If the name already exists the value index is overwritten (last-write-wins).
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

// marshalDataEnvelope serializes the root index with a `{"data":...}` wrapper
// into bytes. Used when the V2 Load needs to hand raw JSON bytes somewhere;
// our main output path prefers toAstjson.
func (b *v2ResponseFrameBuilder) marshalDataEnvelope(root int) []byte {
	b.buffer = b.buffer[:0]
	b.buffer = append(b.buffer, `{"data":`...)
	b.buffer = b.appendNodeJSON(b.buffer, root)
	b.buffer = append(b.buffer, '}')
	out := b.buffer
	b.buffer = nil
	return out
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

// toAstjson materializes the frame rooted at `root` as an *astjson.Value on the
// given arena. This is the bridge between V2's internal flat representation
// and the outer resolve.DataSource interface which demands an astjson.Value.
// The conversion is a single-pass recursive walk — cheaper than marshalling
// to bytes then parsing.
func (b *v2ResponseFrameBuilder) toAstjson(a arena.Arena, root int) *astjson.Value {
	return b.nodeToAstjson(a, root)
}

func (b *v2ResponseFrameBuilder) nodeToAstjson(a arena.Arena, nodeIndex int) *astjson.Value {
	node := &b.nodes[nodeIndex]
	switch node.kind {
	case v2ResponseFrameKindNull:
		return astjson.NullValue
	case v2ResponseFrameKindObject:
		obj := astjson.ObjectValue(a)
		for i := range node.objectFields {
			child := b.nodeToAstjson(a, node.objectFields[i].value)
			obj.Set(a, node.objectFields[i].name, child)
		}
		return obj
	case v2ResponseFrameKindArray:
		arr := astjson.ArrayValue(a)
		for i, child := range node.arrayValues {
			arr.SetArrayItem(a, i, b.nodeToAstjson(a, child))
		}
		return arr
	case v2ResponseFrameKindString:
		return astjson.StringValue(a, node.stringValue)
	case v2ResponseFrameKindNumber:
		// Parse the pre-formatted number string into an astjson number.
		// We use ParseBytesWithArena for simplicity — the number bytes are small.
		v, err := astjson.ParseBytesWithArena(a, []byte(node.stringValue))
		if err != nil {
			return astjson.NullValue
		}
		return v
	case v2ResponseFrameKindBool:
		if node.boolValue {
			return astjson.TrueValue(a)
		}
		return astjson.FalseValue(a)
	default:
		return astjson.NullValue
	}
}

// flatten walks a path across the frame and returns indices of leaves reached.
// Used by resolve-kind attach to find target slots for merging per-row values.
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

// v2FramePool recycles frame builders across requests.
var v2FramePool = sync.Pool{
	New: func() any { return newV2ResponseFrameBuilder() },
}
