package astjson

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"unsafe"

	"github.com/buger/jsonparser"
)

var (
	Pool = &pool{
		p: sync.Pool{
			New: func() interface{} {
				return &JSON{}
			},
		},
	}
)

type pool struct {
	p sync.Pool
}

func (p *pool) Get() *JSON {
	return p.p.Get().(*JSON)
}

func (p *pool) Put(j *JSON) {
	j.Reset()
	p.p.Put(j)
}

type JSON struct {
	storage      []byte
	Nodes        []Node
	RootNode     int
	_intSlices   [][]int
	_intSlicePos int
}

func (j *JSON) Get(nodeRef int, path []string) int {
	if len(path) == 0 {
		return nodeRef
	}
	elem := path[0]
	if j.isArrayElem(elem) {
		if j.Nodes[nodeRef].Kind != NodeKindArray {
			return -1
		}
		index := j.arrayElemIndex(elem)
		if index == -1 {
			return -1
		}
		if len(j.Nodes[nodeRef].ArrayValues) <= index {
			return -1
		}
		return j.Get(j.Nodes[nodeRef].ArrayValues[index], path[1:])
	}
	if j.Nodes[nodeRef].Kind != NodeKindObject {
		return -1
	}
	for _, i := range j.Nodes[nodeRef].ObjectFields {
		if j.objectFieldKeyEquals(i, path[0]) {
			return j.Get(j.Nodes[i].ObjectFieldValue, path[1:])
		}
	}
	return -1
}

func (j *JSON) isArrayElem(elem string) bool {
	if len(elem) < 2 {
		return false
	}
	return elem[0] == '[' && elem[len(elem)-1] == ']'
}

func (j *JSON) arrayElemIndex(elem string) int {
	if len(elem) < 3 {
		return -1
	}
	subStr := elem[1 : len(elem)-1]
	out, err := jsonparser.GetInt(unsafe.Slice(unsafe.StringData(subStr), len(subStr)))
	if err != nil {
		return -1
	}
	return int(out)
}

func (j *JSON) DebugPrintNode(ref int) string {
	out := &bytes.Buffer{}
	err := j.PrintNode(j.Nodes[ref], out)
	if err != nil {
		panic(err)
	}
	return out.String()
}

func (j *JSON) SetObjectField(nodeRef, setFieldNodeRef int, path []string) bool {
	before := j.DebugPrintNode(nodeRef)
	if len(path) >= 2 {
		subPath := path[:len(path)-1]
		nodeRef = j.Get(nodeRef, subPath)
	}
	after := j.DebugPrintNode(nodeRef)
	_, _ = before, after
	for i, fieldRef := range j.Nodes[nodeRef].ObjectFields {
		if j.objectFieldKeyEquals(fieldRef, path[len(path)-1]) {
			objectFieldNodeRef := j.Nodes[nodeRef].ObjectFields[i]
			j.Nodes[objectFieldNodeRef].ObjectFieldValue = setFieldNodeRef
			return true
		}
	}
	key := path[len(path)-1]
	j.storage = append(j.storage, key...)
	j.Nodes = append(j.Nodes, Node{
		Kind:             NodeKindObjectField,
		ObjectFieldValue: setFieldNodeRef,
		keyStart:         len(j.storage) - len(key),
		keyEnd:           len(j.storage),
	})
	objectFieldNodeRef := len(j.Nodes) - 1
	j.Nodes[nodeRef].ObjectFields = append(j.Nodes[nodeRef].ObjectFields, objectFieldNodeRef)
	return false
}

func (j *JSON) objectFieldKeyEquals(objectFieldRef int, another string) bool {
	key := j.objectFieldKey(objectFieldRef)
	if len(key) != len(another) {
		return false
	}
	for i := range key {
		if key[i] != another[i] {
			return false
		}
	}
	return true
}

func (j *JSON) objectFieldKey(objectFieldRef int) []byte {
	return j.storage[j.Nodes[objectFieldRef].keyStart:j.Nodes[objectFieldRef].keyEnd]
}

type Node struct {
	Kind             NodeKind
	ObjectFieldValue int
	keyStart         int
	keyEnd           int
	valueStart       int
	valueEnd         int
	ObjectFields     []int
	ArrayValues      []int
}

func (n *Node) ValueBytes(j *JSON) []byte {
	return j.storage[n.valueStart:n.valueEnd]
}

type NodeKind int

const (
	NodeKindSkip NodeKind = iota
	NodeKindObject
	NodeKindObjectField
	NodeKindArray
	NodeKindString
	NodeKindNumber
	NodeKindBoolean
	NodeKindNull
)

func (j *JSON) ParseObject(input []byte) (err error) {
	j.Reset()
	j.storage = append(j.storage, input...)
	j.RootNode, err = j.parseObject(input, 0)
	return err
}

func (j *JSON) ParseArray(input []byte) (err error) {
	j.Reset()
	j.storage = append(j.storage, input...)
	j.RootNode, err = j.parseArray(input, 0)
	return err
}

func (j *JSON) AppendObject(input []byte) (ref int, err error) {
	start := len(j.storage)
	if j.storage == nil {
		j.storage = make([]byte, 0, 4*1024)
	}
	j.storage = append(j.storage, input...)
	return j.parseObject(input, start)
}

func (j *JSON) AppendArray(input []byte) (ref int, err error) {
	start := len(j.storage) - 1
	j.storage = append(j.storage, input...)
	return j.parseArray(input, start)
}

func (j *JSON) Reset() {
	j.storage = j.storage[:0]
	j._intSlices = j._intSlices[:0]
	j._intSlicePos = 0
	for i := range j.Nodes {
		if j.Nodes[i].ObjectFields != nil {
			j._intSlices = append(j._intSlices, j.Nodes[i].ObjectFields[:0])
		}
		if j.Nodes[i].ArrayValues != nil {
			j._intSlices = append(j._intSlices, j.Nodes[i].ArrayValues[:0])
		}
	}
	j.Nodes = j.Nodes[:0]
}

func (j *JSON) getIntSlice() []int {
	if j._intSlicePos >= len(j._intSlices) {
		return make([]int, 0, 8)
	}
	slice := j._intSlices[j._intSlicePos]
	j._intSlicePos++
	return slice
}

func (j *JSON) parseObject(object []byte, start int) (int, error) {
	node := Node{
		Kind:         NodeKindObject,
		ObjectFields: j.getIntSlice(),
	}
	err := jsonparser.ObjectEach(object, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		storageEnd := start + offset
		if dataType == jsonparser.String {
			storageEnd -= 1
		}
		storageStart := storageEnd - len(value)
		valueNodeRef, err := j.parseKnownValue(value, dataType, storageStart)
		if err != nil {
			return err
		}
		keyEnd := j.findKeyEnd(storageStart)
		keyStart := keyEnd - len(key)
		j.Nodes = append(j.Nodes, Node{
			Kind:             NodeKindObjectField,
			ObjectFieldValue: valueNodeRef,
			keyStart:         keyStart,
			keyEnd:           keyEnd,
		})
		objectFieldRef := len(j.Nodes) - 1
		node.ObjectFields = append(node.ObjectFields, objectFieldRef)
		return nil
	})
	if err != nil {
		return -1, fmt.Errorf("failed to parse JSON")
	}
	j.Nodes = append(j.Nodes, node)
	return len(j.Nodes) - 1, nil
}

func (j *JSON) findKeyEnd(pos int) int {
	for {
		pos--
		if j.storage[pos] == ':' {
			break
		}
	}
	for {
		pos--
		if j.storage[pos] == '"' {
			return pos
		}
	}
}

func (j *JSON) parseArray(array []byte, start int) (ref int, parseArrayErr error) {
	node := Node{
		Kind:        NodeKindArray,
		ArrayValues: j.getIntSlice(),
	}
	_, err := jsonparser.ArrayEach(array, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		storageStart := start + offset
		if dataType == jsonparser.String {
			storageStart -= 1
		}
		valueNodeRef, err := j.parseKnownValue(value, dataType, storageStart)
		if err != nil {
			parseArrayErr = err
			return
		}
		node.ArrayValues = append(node.ArrayValues, valueNodeRef)
	})
	if err != nil {
		return -1, err
	}
	j.Nodes = append(j.Nodes, node)
	ref = len(j.Nodes) - 1
	return ref, parseArrayErr
}

func (j *JSON) parseKnownValue(value []byte, dataType jsonparser.ValueType, start int) (int, error) {
	switch dataType {
	case jsonparser.Object:
		return j.parseObject(value, start)
	case jsonparser.Array:
		return j.parseArray(value, start)
	case jsonparser.String:
		return j.parseString(value, start)
	case jsonparser.Number:
		return j.parseNumber(value, start)
	case jsonparser.Boolean:
		return j.parseBoolean(value, start)
	case jsonparser.Null:
		return j.parseNull(value, start)
	}
	return -1, fmt.Errorf("unknown json type: %v", dataType)
}

func (j *JSON) parseString(value []byte, start int) (int, error) {
	node := Node{
		Kind:       NodeKindString,
		valueStart: start,
		valueEnd:   start + len(value),
	}
	j.Nodes = append(j.Nodes, node)
	return len(j.Nodes) - 1, nil
}

func (j *JSON) parseNumber(value []byte, offset int) (int, error) {
	node := Node{
		Kind:       NodeKindNumber,
		valueStart: offset,
		valueEnd:   offset + len(value),
	}
	j.Nodes = append(j.Nodes, node)
	return len(j.Nodes) - 1, nil
}

func (j *JSON) parseBoolean(value []byte, offset int) (int, error) {
	node := Node{
		Kind:       NodeKindBoolean,
		valueStart: offset,
		valueEnd:   offset + len(value),
	}
	j.Nodes = append(j.Nodes, node)
	return len(j.Nodes) - 1, nil
}

func (j *JSON) parseNull(value []byte, offset int) (int, error) {
	node := Node{
		Kind:       NodeKindNull,
		valueStart: offset,
		valueEnd:   offset + len(value),
	}
	j.Nodes = append(j.Nodes, node)
	return len(j.Nodes) - 1, nil
}

func (j *JSON) PrintRoot(out io.Writer) error {
	if j.RootNode == -1 {
		_, err := out.Write(null)
		return err
	}
	return j.PrintNode(j.Nodes[j.RootNode], out)
}

func (j *JSON) PrintNode(node Node, out io.Writer) error {
	switch node.Kind {
	case NodeKindSkip:
		return nil
	case NodeKindObject:
		return j.printObject(node, out)
	case NodeKindObjectField:
		return j.printObjectField(node, out)
	case NodeKindArray:
		return j.printArray(node, out)
	case NodeKindString:
		return j.printString(node, out)
	case NodeKindNumber, NodeKindBoolean, NodeKindNull:
		return j.printNonStringScalar(node, out)
	}
	return fmt.Errorf("unknown node kind: %v", node.Kind)
}

var (
	lBrace = []byte{'{'}
	rBrace = []byte{'}'}
	lBrack = []byte{'['}
	rBrack = []byte{']'}
	comma  = []byte{','}
	quote  = []byte{'"'}
	colon  = []byte{':'}
	null   = []byte("null")
)

func (j *JSON) printObject(node Node, out io.Writer) error {
	_, err := out.Write(lBrace)
	if err != nil {
		return err
	}
	for i, fieldRef := range node.ObjectFields {
		if i > 0 {
			_, err := out.Write(comma)
			if err != nil {
				return err
			}
		}
		err := j.PrintNode(j.Nodes[fieldRef], out)
		if err != nil {
			return err
		}
	}
	_, err = out.Write(rBrace)
	return err
}

func (j *JSON) printObjectField(node Node, out io.Writer) error {
	_, err := out.Write(quote)
	if err != nil {
		return err
	}
	_, err = out.Write(j.storage[node.keyStart:node.keyEnd])
	if err != nil {
		return err
	}
	_, err = out.Write(quote)
	if err != nil {
		return err
	}
	_, err = out.Write(colon)
	if err != nil {
		return err
	}
	if !j.NodeIsDefined(node.ObjectFieldValue) {
		_, err = out.Write(null)
		return err
	}
	err = j.PrintNode(j.Nodes[node.ObjectFieldValue], out)
	if err != nil {
		return err
	}
	return nil
}

func (j *JSON) printArray(node Node, out io.Writer) error {
	_, err := out.Write(lBrack)
	if err != nil {
		return err
	}
	for i, valueRef := range node.ArrayValues {
		if i > 0 {
			_, err := out.Write(comma)
			if err != nil {
				return err
			}
		}
		err := j.PrintNode(j.Nodes[valueRef], out)
		if err != nil {
			return err
		}
	}
	_, err = out.Write(rBrack)
	return err
}

func (j *JSON) printString(node Node, out io.Writer) error {
	_, err := out.Write(quote)
	if err != nil {
		return err
	}
	_, err = out.Write(j.storage[node.valueStart:node.valueEnd])
	if err != nil {
		return err
	}
	_, err = out.Write(quote)
	return err
}

func (j *JSON) printNonStringScalar(node Node, out io.Writer) error {
	_, err := out.Write(j.storage[node.valueStart:node.valueEnd])
	return err
}

func (j *JSON) MergeNodes(left, right int) int {
	if j.NodeIsDefined(left) && !j.NodeIsDefined(right) {
		return left
	}
	if !j.NodeIsDefined(left) && j.NodeIsDefined(right) {
		return right
	}
	if !j.NodeIsDefined(left) && !j.NodeIsDefined(right) {
		return -1
	}
	if j.Nodes[left].Kind != j.Nodes[right].Kind {
		return right
	}
	if j.Nodes[right].Kind != NodeKindObject {
		return right
	}
WithNextLeftField:
	for _, leftField := range j.Nodes[left].ObjectFields {
		leftKey := j.objectFieldKey(leftField)
		for _, rightField := range j.Nodes[right].ObjectFields {
			rightKey := j.objectFieldKey(rightField)
			if bytes.Equal(leftKey, rightKey) {
				j.Nodes[leftField].ObjectFieldValue = j.MergeNodes(j.Nodes[leftField].ObjectFieldValue, j.Nodes[rightField].ObjectFieldValue)
				continue WithNextLeftField
			}
		}
	}
WithNextRightField:
	for _, rightField := range j.Nodes[right].ObjectFields {
		rightKey := j.objectFieldKey(rightField)
		for _, leftField := range j.Nodes[left].ObjectFields {
			leftKey := j.objectFieldKey(leftField)
			if bytes.Equal(leftKey, rightKey) {
				continue WithNextRightField
			}
		}
		j.Nodes[left].ObjectFields = append(j.Nodes[left].ObjectFields, rightField)
	}
	return left
}

func (j *JSON) MergeNodesWithPath(left, right int, path []string) int {
	if len(path) == 0 {
		return j.MergeNodes(left, right)
	}
	root, child := j.buildObjectPath(path)
	j.Nodes[child].ObjectFieldValue = right
	return j.MergeNodes(left, root)
}

func (j *JSON) buildObjectPath(path []string) (root, child int) {
	root, child = -1, -1
	for _, elem := range path {
		keyStart, keyEnd := j.appendString(elem)
		field := Node{
			Kind:     NodeKindObjectField,
			keyStart: keyStart,
			keyEnd:   keyEnd,
		}
		fieldRef := j.appendNode(field)
		object := Node{
			Kind:         NodeKindObject,
			ObjectFields: j.getIntSlice(),
		}
		object.ObjectFields = append(object.ObjectFields, fieldRef)
		objectRef := j.appendNode(object)
		if root == -1 {
			root = objectRef
		} else {
			j.Nodes[child].ObjectFieldValue = objectRef
		}
		child = fieldRef
	}
	return root, child
}

func (j *JSON) appendNode(node Node) int {
	j.Nodes = append(j.Nodes, node)
	return len(j.Nodes) - 1
}

func (j *JSON) appendString(str string) (start, end int) {
	start = len(j.storage)
	j.storage = append(j.storage, str...)
	end = len(j.storage)
	return start, end
}

func (j *JSON) NodeIsDefined(ref int) bool {
	if ref == -1 {
		return false
	}
	if len(j.Nodes) <= ref {
		return false
	}
	if j.Nodes[ref].Kind == NodeKindSkip {
		return false
	}
	if j.Nodes[ref].Kind == NodeKindNull {
		return false
	}
	return true
}

func (j *JSON) AppendJSON(another *JSON) (nodeRef, storageOffset, nodeOffset int) {
	storageOffset = len(j.storage)
	nodeOffset = len(j.Nodes)
	nodeRef = another.RootNode + nodeOffset
	j.storage = append(j.storage, another.storage...)
	for _, node := range another.Nodes {
		node.applyOffset(storageOffset, nodeOffset)
		j.Nodes = append(j.Nodes, node)
	}
	return
}

func (n *Node) applyOffset(storage, node int) {
	n.keyStart += storage
	n.keyEnd += storage
	n.valueStart += storage
	n.valueEnd += storage
	n.ObjectFieldValue += node
	for i := range n.ObjectFields {
		n.ObjectFields[i] += node
	}
	for i := range n.ArrayValues {
		n.ArrayValues[i] += node
	}
}

func (j *JSON) MergeObjects(nodeRefs []int) int {
	out := j.appendNode(Node{
		Kind:         NodeKindObject,
		ObjectFields: j.getIntSlice(),
	})
	for _, nodeRef := range nodeRefs {
		j.MergeNodes(out, nodeRef)
	}
	return out
}