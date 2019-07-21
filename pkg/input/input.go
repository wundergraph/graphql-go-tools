package input

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
)

// RawBytes is a raw graphql document containing the raw input + meta data
type Input struct {
	// RawBytes is the raw byte input
	RawBytes []byte
	// Length of RawBytes
	Length int
	// InputPosition is the current position in the RawBytes
	InputPosition int
	// TextPosition is the current position within the text (line and character information about the current Tokens)
	TextPosition position.Position
}

func (i *Input) Reset() {
	i.RawBytes = i.RawBytes[:0]
	i.InputPosition = 0
	i.TextPosition.Reset()
}

func (i *Input) ResetInputBytes(bytes []byte) {
	i.Reset()
	i.AppendInputBytes(bytes)
	i.Length = len(i.RawBytes)
}

func (i *Input) AppendInputBytes(bytes []byte) {
	i.RawBytes = append(i.RawBytes, bytes...)
	i.Length = len(i.RawBytes)
}

func (i *Input) ByteSlice(reference ByteSliceReference) ByteSlice {
	return i.RawBytes[reference.Start:reference.End]
}

func (i *Input) ByteSliceString(reference ByteSliceReference) string {
	return string(i.ByteSlice(reference))
}

type ByteSlice []byte

func (b ByteSlice) MarshalJSON() ([]byte, error) {
	return append(append(literal.QUOTE, b...), literal.QUOTE...), nil
}

type ByteSliceReference struct {
	Start uint32
	End   uint32
}

func (b ByteSliceReference) Length() uint32 {
	return b.End - b.Start
}

func ByteSliceEquals(left ByteSliceReference, leftInput *Input, right ByteSliceReference, rightInput *Input) bool {
	if left.Length() != right.Length() {
		return false
	}
	length := int(left.Length())
	for i := 0; i < length; i++ {
		if leftInput.RawBytes[int(left.Start)+i] != rightInput.RawBytes[int(right.Start)+i] {
			return false
		}
	}
	return true
}
