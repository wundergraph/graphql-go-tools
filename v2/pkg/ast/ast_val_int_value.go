package ast

import (
	"bytes"
	"math"
	"strconv"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/position"
)

// IntValue
// example:
// 123 / -123
type IntValue struct {
	Negative     bool               // indicates if the value is negative
	NegativeSign position.Position  // optional -
	Raw          ByteSliceReference // e.g. 123
}

// IntRange is an inclusive range of accepted Int values.
type IntRange struct {
	Min, Max int64
}

// Int32Range is the range the GraphQL spec defines for Int.
var Int32Range = IntRange{Min: math.MinInt32, Max: math.MaxInt32}

// OrDefault returns Int32Range for the zero value, so an unset option keeps spec behavior.
func (r IntRange) OrDefault() IntRange {
	if r == (IntRange{}) {
		return Int32Range
	}
	return r
}

func (r IntRange) Contains(value int64) bool {
	return value >= r.Min && value <= r.Max
}

func (d *Document) CopyIntValue(ref int) int {
	return d.AddIntValue(IntValue{
		Negative: d.IntValues[ref].Negative,
		Raw:      d.copyByteSliceReference(d.IntValues[ref].Raw),
	})
}

func (d *Document) IntValueAsInt(ref int) (out int64) {
	in := d.Input.ByteSlice(d.IntValues[ref].Raw)
	out = unsafebytes.BytesToInt64(in)
	if d.IntValues[ref].Negative {
		out = -out
	}
	return
}

func (d *Document) IntValueAsInt32(ref int) (out int32) {
	in := d.Input.ByteSlice(d.IntValues[ref].Raw)
	out = unsafebytes.BytesToInt32(in)
	if d.IntValues[ref].Negative {
		out = -out
	}
	return
}

func (d *Document) IntValueValidInt32(ref int) bool {
	return d.IntValueInRange(ref, Int32Range)
}

func (d *Document) IntValueInRange(ref int, r IntRange) bool {
	in := d.Input.ByteSlice(d.IntValues[ref].Raw)
	magnitude, err := strconv.ParseUint(unsafebytes.BytesToString(in), 10, 64)
	if err != nil {
		return false
	}
	var value int64
	switch {
	case magnitude <= math.MaxInt64:
		value = int64(magnitude)
		if d.IntValues[ref].Negative {
			value = -value
		}
	case magnitude == 1<<63 && d.IntValues[ref].Negative:
		value = math.MinInt64
	default:
		return false
	}
	return r.Contains(value)
}

func (d *Document) IntValue(ref int) IntValue {
	return d.IntValues[ref]
}

func (d *Document) IntValueIsNegative(ref int) bool {
	return d.IntValues[ref].Negative
}

func (d *Document) IntValueRaw(ref int) ByteSlice {
	return d.Input.ByteSlice(d.IntValues[ref].Raw)
}

func (d *Document) IntValuesAreEquals(left, right int) bool {
	return d.IntValueIsNegative(left) == d.IntValueIsNegative(right) &&
		bytes.Equal(d.IntValueRaw(left), d.IntValueRaw(right))
}

func (d *Document) AddIntValue(value IntValue) (ref int) {
	d.IntValues = append(d.IntValues, value)
	return len(d.IntValues) - 1
}

func (d *Document) ImportIntValue(raw ByteSlice, isNegative bool) (ref int) {
	return d.AddIntValue(IntValue{
		Negative: isNegative,
		Raw:      d.Input.AppendInputBytes(raw),
	})
}
