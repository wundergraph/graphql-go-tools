package ast

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
)

// StringValue
// example:
// "foo"
type StringValue struct {
	BlockString bool               // """foo""" = blockString, "foo" string
	Content     ByteSliceReference // e.g. foo
}

func (d *Document) CopyStringValue(ref int) int {
	return d.AddStringValue(StringValue{
		BlockString: d.StringValues[ref].BlockString,
		Content:     d.copyByteSliceReference(d.StringValues[ref].Content),
	})
}

func (d *Document) StringValue(ref int) StringValue {
	return d.StringValues[ref]
}

func (d *Document) StringValueContentBytes(ref int) ByteSlice {
	return d.Input.ByteSlice(d.StringValues[ref].Content)
}

func (d *Document) StringValueContentString(ref int) string {
	return unsafebytes.BytesToString(d.StringValueContentBytes(ref))
}

func (d *Document) StringValueIsBlockString(ref int) bool {
	return d.StringValues[ref].BlockString
}

func (d *Document) BlockStringValueContentRawBytes(ref int) []byte {

	// Gets the full block string content, just inside the """ quotes.
	// This is needed because the lexer ignores whitespace and we need to preserve it
	// to account for the indentation of the block string.

	blockStart := 0
	for i := int(d.StringValues[ref].Content.Start) - 1; i >= 0; i-- {
		if d.Input.RawBytes[i] == '"' {
			blockStart = i + 1
			break
		}
	}

	blockEnd := d.Input.Length
	for i := int(d.StringValues[ref].Content.End); i < d.Input.Length; i++ {
		if d.Input.RawBytes[i] == '"' {
			blockEnd = i
			break
		}
	}

	return d.Input.RawBytes[blockStart:blockEnd]
}

func (d *Document) BlockStringValueContentRawString(ref int) string {
	return unsafebytes.BytesToString(d.BlockStringValueContentRawBytes(ref))
}

func (d *Document) BlockStringValueContentBytes(ref int) []byte {

	// Implements https://spec.graphql.org/October2021/#BlockStringValue()

	// NOTE: This implementation exactly follows the spec.
	// It likely could be optimized for performance.

	// split the raw value into lines
	rawValue := d.BlockStringValueContentRawBytes(ref)
	lines := splitBytesIntoLines(rawValue)

	// find the common indent size (-1 means no common indent)
	commonIndent := -1
	for i, line := range lines {
		if i == 0 {
			continue
		}
		length := len(line)
		indent := leadingWhitespaceCount(line)
		if indent < length {
			if commonIndent == -1 || indent < commonIndent {
				commonIndent = indent
			}
		}
	}

	// remove the common indent from each line
	if commonIndent != -1 {
		for i := 1; i < len(lines); i++ {
			var indent int
			if len(lines[i]) > commonIndent {
				indent = commonIndent
			} else {
				indent = len(lines[i])
			}

			lines[i] = lines[i][indent:]
		}
	}

	// remove leading whitespace-only lines
	for len(lines) > 0 {
		if leadingWhitespaceCount(lines[0]) == len(lines[0]) {
			lines = lines[1:]
		} else {
			break
		}
	}

	// remove trailing whitespace-only lines
	for len(lines) > 0 {
		if leadingWhitespaceCount(lines[len(lines)-1]) == len(lines[len(lines)-1]) {
			lines = lines[:len(lines)-1]
		} else {
			break
		}
	}

	// join the lines and return the result
	return bytes.Join(lines, []byte{'\n'})
}

func (d *Document) BlockStringValueContentString(ref int) string {
	return unsafebytes.BytesToString(d.BlockStringValueContentBytes(ref))
}

func (d *Document) StringValuesAreEquals(left, right int) bool {
	return d.StringValueIsBlockString(left) == d.StringValueIsBlockString(right) &&
		bytes.Equal(d.StringValueContentBytes(left), d.StringValueContentBytes(right))
}

func (d *Document) AddStringValue(value StringValue) (ref int) {
	d.StringValues = append(d.StringValues, value)
	return len(d.StringValues) - 1
}

func (d *Document) ImportStringValue(raw ByteSlice, isBlockString bool) (ref int) {
	return d.AddStringValue(StringValue{
		BlockString: isBlockString,
		Content:     d.Input.AppendInputBytes(raw),
	})
}
