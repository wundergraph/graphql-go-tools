package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"

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
