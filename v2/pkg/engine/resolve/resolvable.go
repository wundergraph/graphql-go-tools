package resolve

import (
	"fmt"
	"io"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astjson"
)

type Resolvable struct {
	storage    *astjson.JSON
	dataRoot   int
	errorsRoot int
	print      bool
	out        io.Writer
	printErr   error
	path       []astjson.PathElement
}

func NewResolvable() *Resolvable {
	return &Resolvable{
		storage: &astjson.JSON{},
	}
}

func (r *Resolvable) Reset() {
	r.storage.Reset()
	r.dataRoot = -1
	r.errorsRoot = -1
	r.print = false
	r.out = nil
	r.printErr = nil
	r.path = r.path[:0]
}

func (r *Resolvable) Init(initialData []byte) (err error) {
	r.dataRoot, r.errorsRoot, err = r.storage.InitResolvable(initialData)
	return
}

func (r *Resolvable) Resolve(root *Object, out io.Writer) error {
	r.out = out
	r.print = false
	r.printErr = nil
	err := r.walkObject(root, r.storage.RootNode)
	r.printBytes(lBrace)
	if r.hasErrors() {
		r.printErrors()
	}
	if err != nil {
		r.printBytes(quote)
		r.printBytes(literalData)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printBytes(null)
	} else {
		r.printData(root)
	}
	r.printBytes(rBrace)
	return r.printErr
}

func (r *Resolvable) printErrors() {
	r.printBytes(quote)
	r.printBytes(literalErrors)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printNode(r.errorsRoot)
	r.printBytes(comma)
}

func (r *Resolvable) printData(root *Object) {
	r.print = true
	_ = r.walkObject(root, r.storage.RootNode)
	r.print = false
}

func (r *Resolvable) hasErrors() bool {
	if r.errorsRoot == -1 {
		return false
	}
	return len(r.storage.Nodes[r.errorsRoot].ArrayValues) > 0
}

func (r *Resolvable) printBytes(b []byte) {
	if r.printErr != nil {
		return
	}
	_, r.printErr = r.out.Write(b)
}

func (r *Resolvable) printNode(ref int) {
	if r.printErr != nil {
		return
	}
	r.printErr = r.storage.PrintNode(r.storage.Nodes[ref], r.out)
}

func (r *Resolvable) pushArrayPathElement(index int) {
	r.path = append(r.path, astjson.PathElement{
		ArrayIndex: index,
	})
}

func (r *Resolvable) popArrayPathElement() {
	r.path = r.path[:len(r.path)-1]
}

func (r *Resolvable) pushNodePathElement(path []string) {
	for i := range path {
		r.path = append(r.path, astjson.PathElement{
			Name: path[i],
		})
	}
}

func (r *Resolvable) popNodePathElement(path []string) {
	r.path = r.path[:len(r.path)-len(path)]
}

func (r *Resolvable) walkNode(node Node, ref int) error {
	switch n := node.(type) {
	case *Object:
		return r.walkObject(n, ref)
	case *Array:
		return r.walkArray(n, ref)
	case *Null:
		return r.walkNull()
	case *String:
		return r.walkString(n, ref)
	case *Boolean:
		return r.walkBoolean(n, ref)
	case *Integer:
		return r.walkInteger(n, ref)
	case *Float:
		return r.walkFloat(n, ref)
	case *BigInt:
		return r.walkBigInt(n, ref)
	case *Scalar:
		return r.walkScalar(n, ref)
	case *EmptyObject:
		return r.walkEmptyObject(n)
	case *EmptyArray:
		return r.walkEmptyArray(n)
	case *CustomNode:
		return r.walkCustom(n, ref)
	default:
		return nil
	}
}

func (r *Resolvable) walkObject(obj *Object, ref int) (err error) {
	r.pushNodePathElement(obj.Path)
	isRoot := len(r.path) == 0
	defer r.popNodePathElement(obj.Path)
	ref = r.storage.Get(ref, obj.Path)
	if ref == -1 {
		if obj.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(obj.Path)
		return errNonNullableFieldValueIsNull
	}
	if r.storage.Nodes[ref].Kind == astjson.NodeKindNull {
		return r.walkNull()
	}
	if r.storage.Nodes[ref].Kind != astjson.NodeKindObject {
		r.addTypeMismatchError("Object cannot represent non-object value.", obj.Path)
		return errInvalidFieldValue
	}
	if r.print && !isRoot {
		r.printBytes(lBrace)
	}
	for i := range obj.Fields {
		if r.print {
			if i != 0 {
				r.printBytes(comma)
			}
			r.printBytes(quote)
			r.printBytes(obj.Fields[i].Name)
			r.printBytes(quote)
			r.printBytes(colon)
		}
		err = r.walkNode(obj.Fields[i].Value, ref)
		if err != nil {
			if obj.Nullable {
				r.storage.Nodes[ref].Kind = astjson.NodeKindNull
				return nil
			}
			return
		}
	}
	if r.print && !isRoot {
		r.printBytes(rBrace)
	}
	return nil
}

func (r *Resolvable) walkArray(arr *Array, ref int) (err error) {
	r.pushNodePathElement(arr.Path)
	defer r.popNodePathElement(arr.Path)
	ref = r.storage.Get(ref, arr.Path)
	if ref == -1 {
		if arr.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(arr.Path)
		return errNonNullableFieldValueIsNull
	}
	if r.storage.Nodes[ref].Kind != astjson.NodeKindArray {
		r.addTypeMismatchError("Array cannot represent non-array value.", arr.Path)
		return errInvalidFieldValue
	}
	if r.print {
		r.printBytes(lBrack)
	}
	for i, value := range r.storage.Nodes[ref].ArrayValues {
		if r.print && i != 0 {
			r.printBytes(comma)
		}
		r.pushArrayPathElement(i)
		err = r.walkNode(arr.Item, value)
		r.popArrayPathElement()
		if err != nil {
			return
		}
	}
	if r.print {
		r.printBytes(rBrack)
	}
	return nil
}

func (r *Resolvable) walkNull() error {
	if r.print {
		r.printBytes(null)
	}
	return nil
}

func (r *Resolvable) walkString(s *String, ref int) (err error) {
	ref = r.storage.Get(ref, s.Path)
	if ref == -1 {
		if s.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(s.Path)
		return errNonNullableFieldValueIsNull
	}
	if r.storage.Nodes[ref].Kind != astjson.NodeKindString {
		value := string(r.storage.Nodes[ref].ValueBytes(r.storage))
		r.addTypeMismatchError(fmt.Sprintf("String cannot represent non-string value: \\\"%s\\\"", value), s.Path)
		return errInvalidFieldValue
	}
	if r.print {
		r.printNode(ref)
	}
	return nil
}

func (r *Resolvable) walkBoolean(b *Boolean, ref int) (err error) {
	ref = r.storage.Get(ref, b.Path)
	if ref == -1 {
		if b.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(b.Path)
		return errNonNullableFieldValueIsNull
	}
	if r.storage.Nodes[ref].Kind != astjson.NodeKindBoolean {
		value := string(r.storage.Nodes[ref].ValueBytes(r.storage))
		r.addTypeMismatchError(fmt.Sprintf("Bool cannot represent non-boolean value: \\\"%s\\\"", value), b.Path)
		return errInvalidFieldValue
	}
	if r.print {
		r.printNode(ref)
	}
	return nil
}

func (r *Resolvable) walkInteger(i *Integer, ref int) (err error) {
	ref = r.storage.Get(ref, i.Path)
	if ref == -1 {
		if i.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(i.Path)
		return errNonNullableFieldValueIsNull
	}
	if r.storage.Nodes[ref].Kind != astjson.NodeKindNumber {
		value := string(r.storage.Nodes[ref].ValueBytes(r.storage))
		r.addTypeMismatchError(fmt.Sprintf("Int cannot represent non-integer value: \\\"%s\\\"", value), i.Path)
		return errInvalidFieldValue
	}
	if r.print {
		r.printNode(ref)
	}
	return nil
}

func (r *Resolvable) walkFloat(f *Float, ref int) (err error) {
	ref = r.storage.Get(ref, f.Path)
	if ref == -1 {
		if f.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(f.Path)
		return errNonNullableFieldValueIsNull
	}
	if r.storage.Nodes[ref].Kind != astjson.NodeKindNumber {
		value := string(r.storage.Nodes[ref].ValueBytes(r.storage))
		r.addTypeMismatchError(fmt.Sprintf("Float cannot represent non-float value: \\\"%s\\\"", value), f.Path)
		return errInvalidFieldValue
	}
	if r.print {
		r.printNode(ref)
	}
	return nil
}

func (r *Resolvable) walkBigInt(b *BigInt, ref int) (err error) {
	ref = r.storage.Get(ref, b.Path)
	if ref == -1 {
		if b.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(b.Path)
		return errNonNullableFieldValueIsNull
	}
	if r.print {
		r.printNode(ref)
	}
	return nil
}

func (r *Resolvable) walkScalar(s *Scalar, ref int) (err error) {
	ref = r.storage.Get(ref, s.Path)
	if ref == -1 {
		if s.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(s.Path)
		return errNonNullableFieldValueIsNull
	}
	if r.storage.Nodes[ref].Kind != astjson.NodeKindString {
		value := string(r.storage.Nodes[ref].ValueBytes(r.storage))
		r.addTypeMismatchError(fmt.Sprintf("Custom scalar cannot represent non-string value: \\\"%s\\\"", value), s.Path)
		return errInvalidFieldValue
	}
	if r.print {
		r.printNode(ref)
	}
	return nil
}

func (r *Resolvable) walkEmptyObject(_ *EmptyObject) (err error) {
	if r.print {
		r.printBytes(lBrace)
		r.printBytes(rBrace)
	}
	return nil
}

func (r *Resolvable) walkEmptyArray(_ *EmptyArray) (err error) {
	if r.print {
		r.printBytes(lBrack)
		r.printBytes(rBrack)
	}
	return nil
}

func (r *Resolvable) walkCustom(c *CustomNode, ref int) (err error) {
	ref = r.storage.Get(ref, c.Path)
	if ref == -1 {
		if c.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(c.Path)
		return errNonNullableFieldValueIsNull
	}
	if r.print {
		r.printNode(ref)
	}
	return nil
}

func (r *Resolvable) addNonNullableFieldError(fieldPath []string) {
	r.pushNodePathElement(fieldPath)
	ref := r.storage.AppendNonNullableFieldIsNullErr("", r.path[1:])
	r.storage.Nodes[r.errorsRoot].ArrayValues = append(r.storage.Nodes[r.errorsRoot].ArrayValues, ref)
	r.popNodePathElement(fieldPath)
}

func (r *Resolvable) addTypeMismatchError(message string, fieldPath []string) {
	r.pushNodePathElement(fieldPath)
	ref := r.storage.AppendTypeMismatchError(message, r.path[1:])
	r.storage.Nodes[r.errorsRoot].ArrayValues = append(r.storage.Nodes[r.errorsRoot].ArrayValues, ref)
	r.popNodePathElement(fieldPath)
}
