package ast

import (
	"bytes"
	"fmt"
	"io"

	"github.com/wundergraph/graphql-go-tools/pkg/internal/quotes"
	"github.com/wundergraph/graphql-go-tools/pkg/internal/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/position"
)

type ValueKind int

const (
	ValueKindUnknown ValueKind = 4 + iota
	ValueKindString
	ValueKindBoolean
	ValueKindInteger
	ValueKindFloat
	ValueKindVariable
	ValueKindNull
	ValueKindList
	ValueKindObject
	ValueKindEnum
)

type Value struct {
	Kind     ValueKind // e.g. 100 or "Bar"
	Ref      int
	Position position.Position
}

func (d *Document) CopyValue(ref int) int {
	return d.AddValue(Value{
		Kind: d.Values[ref].Kind,
		Ref:  d.copyValueRef(d.Values[ref].Kind, d.Values[ref].Ref),
	})
}

func (d *Document) copyValueRef(kind ValueKind, valueRef int) int {
	switch kind {
	case ValueKindString:
		return d.CopyStringValue(valueRef)
	case ValueKindBoolean:
		// Nothing to copy!
		return valueRef
	case ValueKindInteger:
		return d.CopyIntValue(valueRef)
	case ValueKindFloat:
		return d.CopyFloatValue(valueRef)
	case ValueKindVariable:
		return d.CopyVariableValue(valueRef)
	case ValueKindNull:
		// Nothing to copy!
		return InvalidRef
	case ValueKindList:
		return d.CopyListValue(valueRef)
	case ValueKindObject:
		return d.CopyObjectValue(valueRef)
	case ValueKindEnum:
		return d.CopyEnumValue(valueRef)
	default:
		return InvalidRef
	}
}

func (d *Document) ValueContentBytes(value Value) ByteSlice {
	switch value.Kind {
	case ValueKindEnum:
		return d.EnumValueNameBytes(value.Ref)
	case ValueKindString:
		return d.StringValueContentBytes(value.Ref)
	case ValueKindInteger:
		return d.IntValueRaw(value.Ref)
	case ValueKindFloat:
		return d.FloatValueRaw(value.Ref)
	}
	panic(fmt.Errorf("ValueContentBytes not implemented for ValueKind: %s", value.Kind))
}

func (d *Document) ValueContentString(value Value) string {
	return unsafebytes.BytesToString(d.ValueContentBytes(value))
}

func (d *Document) ValueContainsVariable(value Value) bool {
	switch value.Kind {
	case ValueKindEnum:
		return false
	case ValueKindBoolean:
		return false
	case ValueKindFloat:
		return false
	case ValueKindList:
		for _, ref := range d.ListValues[value.Ref].Refs {
			if d.ValueContainsVariable(d.Value(ref)) {
				return true
			}
		}
		return false
	case ValueKindObject:
		for _, ref := range d.ObjectValues[value.Ref].Refs {
			if d.ValueContainsVariable(d.ObjectFields[ref].Value) {
				return true
			}
		}
		return false
	case ValueKindInteger:
		return false
	case ValueKindNull:
		return false
	case ValueKindString:
		return false
	case ValueKindVariable:
		return true
	default:
		return false
	}
}

func (d *Document) writeJSONValue(buf *bytes.Buffer, value Value) error {
	switch value.Kind {
	case ValueKindNull:
		buf.Write(literal.NULL)
	case ValueKindEnum:
		buf.Write(quotes.WrapBytes(d.EnumValueNameBytes(value.Ref)))
	case ValueKindInteger:
		intValueBytes := d.IntValueRaw(value.Ref)
		if d.IntValueIsNegative(value.Ref) {
			buf.WriteByte(literal.SUB_BYTE)
		}
		buf.Write(intValueBytes)
	case ValueKindFloat:
		floatValueBytes := d.FloatValueRaw(value.Ref)
		if d.FloatValueIsNegative(value.Ref) {
			buf.WriteByte(literal.SUB_BYTE)
		}
		buf.Write(floatValueBytes)
	case ValueKindBoolean:
		if value.Ref == 0 {
			buf.Write(literal.FALSE)
		} else {
			buf.Write(literal.TRUE)
		}
	case ValueKindString:
		buf.Write(quotes.WrapBytes(d.StringValueContentBytes(value.Ref)))
	case ValueKindList:
		buf.WriteByte(literal.LBRACK_BYTE)
		for ii, ref := range d.ListValues[value.Ref].Refs {
			if ii > 0 {
				buf.WriteByte(literal.COMMA_BYTE)
			}
			if err := d.writeJSONValue(buf, d.Values[ref]); err != nil {
				return err
			}
		}
		buf.WriteByte(literal.RBRACK_BYTE)
	case ValueKindObject:
		buf.WriteByte(literal.LBRACE_BYTE)
		for ii, ref := range d.ObjectValues[value.Ref].Refs {
			if ii > 0 {
				buf.WriteByte(literal.COMMA_BYTE)
			}
			fieldNameBytes := d.ObjectFieldNameBytes(ref)
			buf.Write(quotes.WrapBytes(fieldNameBytes))
			buf.WriteByte(literal.COLON_BYTE)
			if err := d.writeJSONValue(buf, d.ObjectFieldValue(ref)); err != nil {
				return err
			}
		}
		buf.WriteByte(literal.RBRACE_BYTE)
	default:
		return fmt.Errorf("ValueToJSON: not implemented for kind: %s", value.Kind.String())
	}
	return nil
}

func (d *Document) ValueToJSON(value Value) ([]byte, error) {
	var buf bytes.Buffer
	if err := d.writeJSONValue(&buf, value); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// nolint
func (d *Document) PrintValue(value Value, w io.Writer) (err error) {
	switch value.Kind {
	case ValueKindBoolean:
		if d.BooleanValues[value.Ref] {
			_, err = w.Write(literal.TRUE)
		} else {
			_, err = w.Write(literal.FALSE)
		}
	case ValueKindString:
		// This code assumes string content is valid for the associated string
		// type (block/non-block) according to the GraphQL spec. Content IS NOT
		// processed to quote characters that are invalid for the associated
		// type.
		//
		// GraphQL spec: https://spec.graphql.org/June2018/#StringValue
		isBlockString := d.StringValues[value.Ref].BlockString
		_, err = w.Write(literal.QUOTE)
		if isBlockString {
			_, err = w.Write(literal.QUOTE)
			_, err = w.Write(literal.QUOTE)
		}
		_, err = w.Write(d.Input.ByteSlice(d.StringValues[value.Ref].Content))
		_, err = w.Write(literal.QUOTE)
		if isBlockString {
			_, err = w.Write(literal.QUOTE)
			_, err = w.Write(literal.QUOTE)
		}
	case ValueKindInteger:
		if d.IntValues[value.Ref].Negative {
			_, err = w.Write(literal.SUB)
		}
		_, err = w.Write(d.Input.ByteSlice(d.IntValues[value.Ref].Raw))
	case ValueKindFloat:
		if d.FloatValues[value.Ref].Negative {
			_, err = w.Write(literal.SUB)
		}
		_, err = w.Write(d.Input.ByteSlice(d.FloatValues[value.Ref].Raw))
	case ValueKindVariable:
		_, err = w.Write(literal.DOLLAR)
		_, err = w.Write(d.Input.ByteSlice(d.VariableValues[value.Ref].Name))
	case ValueKindNull:
		_, err = w.Write(literal.NULL)
	case ValueKindList:
		_, err = w.Write(literal.LBRACK)
		for i, j := range d.ListValues[value.Ref].Refs {
			err = d.PrintValue(d.Value(j), w)
			if err != nil {
				return
			}
			if i != len(d.ListValues[value.Ref].Refs)-1 {
				_, err = w.Write(literal.COMMA)
			}
		}
		_, err = w.Write(literal.RBRACK)
	case ValueKindObject:
		_, err = w.Write(literal.LBRACE)
		for i, j := range d.ObjectValues[value.Ref].Refs {
			_, err = w.Write(d.ObjectFieldNameBytes(j))
			if err != nil {
				return
			}
			_, err = w.Write(literal.COLON)
			if err != nil {
				return
			}
			_, err = w.Write(literal.SPACE)
			if err != nil {
				return
			}
			err = d.PrintValue(d.ObjectFieldValue(j), w)
			if err != nil {
				return
			}
			if i != len(d.ObjectValues[value.Ref].Refs)-1 {
				_, err = w.Write(literal.COMMA)
				if err != nil {
					return
				}
			}
		}
		_, err = w.Write(literal.RBRACE)
	case ValueKindEnum:
		_, err = w.Write(d.Input.ByteSlice(d.EnumValues[value.Ref].Name))
	}
	return
}

func (d *Document) PrintValueBytes(value Value, buf []byte) ([]byte, error) {
	if buf == nil {
		buf = make([]byte, 0, 24)
	}
	b := bytes.NewBuffer(buf)
	err := d.PrintValue(value, b)
	return b.Bytes(), err
}

func (d *Document) Value(ref int) Value {
	return d.Values[ref]
}

func (d *Document) ValuesAreEqual(left, right Value) bool {
	if left.Kind != right.Kind {
		return false
	}
	switch left.Kind {
	case ValueKindString:
		return d.StringValuesAreEquals(left.Ref, right.Ref)
	case ValueKindBoolean:
		return d.BooleanValuesAreEqual(left.Ref, right.Ref)
	case ValueKindInteger:
		return d.IntValuesAreEquals(left.Ref, right.Ref)
	case ValueKindFloat:
		return d.FloatValuesAreEqual(left.Ref, right.Ref)
	case ValueKindVariable:
		return d.VariableValuesAreEqual(left.Ref, right.Ref)
	case ValueKindNull:
		return true
	case ValueKindList:
		return d.ListValuesAreEqual(left.Ref, right.Ref)
	case ValueKindObject:
		return d.ObjectValuesAreEqual(left.Ref, right.Ref)
	case ValueKindEnum:
		return d.EnumValuesAreEqual(left.Ref, right.Ref)
	default:
		return false
	}
}

func (d *Document) AddValue(value Value) (ref int) {
	d.Values = append(d.Values, value)
	return len(d.Values) - 1
}
