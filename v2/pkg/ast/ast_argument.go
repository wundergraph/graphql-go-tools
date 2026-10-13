package ast

import (
	"bytes"
	"io"
	"strconv"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/position"
)

type ArgumentList struct {
	LPAREN position.Position
	Refs   []int // Argument
	RPAREN position.Position
}

type Argument struct {
	Name             ByteSliceReference // e.g. foo
	Colon            position.Position  // :
	Value            Value              // e.g. 100 or "Bar"
	Position         position.Position
	PrintBeforeValue []byte
	PrintAfterValue  []byte
}

func (d *Document) CopyArgument(ref int) int {
	return d.AddArgument(Argument{
		Name: d.copyByteSliceReference(d.Arguments[ref].Name),
		Value: Value{
			Kind: d.Arguments[ref].Value.Kind,
			Ref:  d.copyValueRef(d.Arguments[ref].Value.Kind, d.Arguments[ref].Value.Ref),
		},
	})
}

func (d *Document) CopyArgumentList(list ArgumentList) ArgumentList {
	refs := d.NewEmptyRefs()
	for _, r := range list.Refs {
		refs = append(refs, d.CopyArgument(r))
	}
	return ArgumentList{Refs: refs}
}

// AddArgumentRef appends an existing argument ref to the list. The caller is
// responsible for updating the owner's HasArguments flag.
func (l *ArgumentList) AddArgumentRef(argRef int) {
	l.Refs = append(l.Refs, argRef)
}

// RemoveArgumentByName removes the first argument with the given name from the
// list. The caller is responsible for updating the owner's HasArguments flag,
func (l *ArgumentList) RemoveArgumentByName(document *Document, name string) {
	for i := range l.Refs {
		if document.ArgumentNameString(l.Refs[i]) == name {
			l.Refs = append(l.Refs[:i], l.Refs[i+1:]...)
			return
		}
	}
}

func (d *Document) PrintArgument(ref int, w io.Writer) error {
	_, err := w.Write(d.Input.ByteSlice(d.Arguments[ref].Name))
	if err != nil {
		return err
	}
	_, err = w.Write(literal.COLON)
	if err != nil {
		return err
	}
	_, err = w.Write(literal.SPACE)
	if err != nil {
		return err
	}
	if d.Arguments[ref].PrintBeforeValue != nil {
		_, err = w.Write(d.Arguments[ref].PrintBeforeValue)
		if err != nil {
			return err
		}
	}
	err = d.PrintValue(d.Arguments[ref].Value, w)
	if err != nil {
		return err
	}
	if d.Arguments[ref].PrintAfterValue != nil {
		_, err = w.Write(d.Arguments[ref].PrintAfterValue)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Document) PrintArguments(refs []int, w io.Writer) (err error) {
	_, err = w.Write(literal.LPAREN)
	if err != nil {
		return
	}
	for i, j := range refs {
		err = d.PrintArgument(j, w)
		if err != nil {
			return
		}
		if i != len(refs)-1 {
			_, err = w.Write(literal.COMMA)
			if err != nil {
				return
			}
			_, err = w.Write(literal.SPACE)
			if err != nil {
				return
			}
		}
	}
	_, err = w.Write(literal.RPAREN)
	return
}

func (d *Document) ArgumentNameBytes(ref int) ByteSlice {
	return d.Input.ByteSlice(d.Arguments[ref].Name)
}

func (d *Document) ArgumentNameString(ref int) string {
	return unsafebytes.BytesToString(d.ArgumentNameBytes(ref))
}

func (d *Document) ArgumentValue(ref int) Value {
	return d.Arguments[ref].Value
}

func (d *Document) ArgumentsAreEqual(left, right int) bool {
	return bytes.Equal(d.ArgumentNameBytes(left), d.ArgumentNameBytes(right)) &&
		d.ValuesAreEqual(d.ArgumentValue(left), d.ArgumentValue(right))
}

func (d *Document) ArgumentSetsAreEquals(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		leftArgument, rightArgument := left[i], right[i]
		if !d.ArgumentsAreEqual(leftArgument, rightArgument) {
			return false
		}
	}
	return true
}

// ArgumentSetsAreEqualUnordered compares arguments and nested input objects
// independently of their field order. Matching names at the same position take
// the linear fast path; only mismatched positions require a name lookup.
// List order remains significant. This does not mutate or cache the document.
// Argument and input-object field names are expected to be unique; their
// respective validation rules reject duplicates.
func (d *Document) ArgumentSetsAreEqualUnordered(left, right []int) bool {
	// Keep the common empty case in a small wrapper that can inline at callers.
	if len(left) == 0 {
		return len(right) == 0
	}
	return d.argumentSetsAreEqualUnordered(left, right)
}

func (d *Document) argumentSetsAreEqualUnordered(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i, leftRef := range left {
		rightRef := right[i]
		if !bytes.Equal(d.ArgumentNameBytes(leftRef), d.ArgumentNameBytes(rightRef)) {
			rightRef = InvalidRef
			for _, candidate := range right {
				if bytes.Equal(d.ArgumentNameBytes(leftRef), d.ArgumentNameBytes(candidate)) {
					rightRef = candidate
					break
				}
			}
			if rightRef == InvalidRef {
				return false
			}
		}
		if !d.argumentValuesAreEqual(d.ArgumentValue(leftRef), d.ArgumentValue(rightRef)) {
			return false
		}
	}
	return true
}

func (d *Document) argumentValuesAreEqual(left, right Value) bool {
	if left.Kind != right.Kind {
		return false
	}
	switch left.Kind {
	case ValueKindObject:
		leftFields, rightFields := d.ObjectValues[left.Ref].Refs, d.ObjectValues[right.Ref].Refs
		if len(leftFields) != len(rightFields) {
			return false
		}
		for i, leftRef := range leftFields {
			rightRef := rightFields[i]
			if !bytes.Equal(d.ObjectFieldNameBytes(leftRef), d.ObjectFieldNameBytes(rightRef)) {
				rightRef = InvalidRef
				for _, candidate := range rightFields {
					if bytes.Equal(d.ObjectFieldNameBytes(leftRef), d.ObjectFieldNameBytes(candidate)) {
						rightRef = candidate
						break
					}
				}
				if rightRef == InvalidRef {
					return false
				}
			}
			if !d.argumentValuesAreEqual(d.ObjectFieldValue(leftRef), d.ObjectFieldValue(rightRef)) {
				return false
			}
		}
		return true
	case ValueKindList:
		leftValues, rightValues := d.ListValues[left.Ref].Refs, d.ListValues[right.Ref].Refs
		if len(leftValues) != len(rightValues) {
			return false
		}
		for i := range leftValues {
			if !d.argumentValuesAreEqual(d.Value(leftValues[i]), d.Value(rightValues[i])) {
				return false
			}
		}
		return true
	default:
		return d.ValuesAreEqual(left, right)
	}
}

func (d *Document) ArgumentsBefore(ancestor Node, argument int) []int {
	switch ancestor.Kind {
	case NodeKindField:
		for i, j := range d.Fields[ancestor.Ref].Arguments.Refs {
			if argument == j {
				return d.Fields[ancestor.Ref].Arguments.Refs[:i]
			}
		}
	case NodeKindDirective:
		for i, j := range d.Directives[ancestor.Ref].Arguments.Refs {
			if argument == j {
				return d.Directives[ancestor.Ref].Arguments.Refs[:i]
			}
		}
	}
	return nil
}

func (d *Document) ArgumentsAfter(ancestor Node, argument int) []int {
	switch ancestor.Kind {
	case NodeKindField:
		for i, j := range d.Fields[ancestor.Ref].Arguments.Refs {
			if argument == j {
				return d.Fields[ancestor.Ref].Arguments.Refs[i+1:]
			}
		}
	case NodeKindDirective:
		for i, j := range d.Directives[ancestor.Ref].Arguments.Refs {
			if argument == j {
				return d.Directives[ancestor.Ref].Arguments.Refs[i+1:]
			}
		}
	}
	return nil
}

func (d *Document) AddArgument(argument Argument) (ref int) {
	d.Arguments = append(d.Arguments, argument)
	return len(d.Arguments) - 1
}

func (d *Document) ImportArgument(name string, value Value) (ref int) {
	arg := Argument{
		Name:  d.Input.AppendInputString(name),
		Value: value,
	}

	return d.AddArgument(arg)
}

func (d *Document) ImportVariableValueArgument(argName, variableName ByteSlice) (variableValueRef, argRef int) {
	variableValueRef = d.ImportVariableValue(variableName)

	arg := Argument{
		Name: d.Input.AppendInputBytes(argName),
		Value: Value{
			Kind: ValueKindVariable,
			Ref:  variableValueRef,
		},
	}

	argRef = d.AddArgument(arg)

	return
}

func (d *Document) AddStringArgument(name, value string) int {
	strValueRef := d.AddStringValue(StringValue{
		Content: d.Input.AppendInputString(value),
	})

	arg := Argument{
		Name:  d.Input.AppendInputString(name),
		Value: Value{Kind: ValueKindString, Ref: strValueRef},
	}

	return d.AddArgument(arg)
}

func (d *Document) AddIntArgument(name string, value int) int {
	intValueRef := d.AddIntValue(IntValue{
		Raw: d.Input.AppendInputString(strconv.Itoa(value)),
	})

	arg := Argument{
		Name:  d.Input.AppendInputString(name),
		Value: Value{Kind: ValueKindInteger, Ref: intValueRef},
	}

	return d.AddArgument(arg)
}
