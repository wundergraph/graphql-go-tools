package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"

type ByteSlice []byte

func (b ByteSlice) MarshalJSON() ([]byte, error) {
	return append(append(literal.QUOTE, b...), literal.QUOTE...), nil
}

type ByteSliceReference struct {
	Start uint16
	End   uint16
}
