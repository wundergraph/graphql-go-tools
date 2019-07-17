package token

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/input"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
)

type Token struct {
	Keyword      keyword.Keyword
	Literal      input.ByteSliceReference
	TextPosition position.Position
}

func (t Token) String() string {
	return fmt.Sprintf("token:: Keyword: %s, Pos: %s", t.Keyword, t.TextPosition)
}

func (t *Token) SetStart(inputPosition int, textPosition position.Position) {
	t.Literal.Start = uint32(inputPosition)
	t.TextPosition.LineStart = textPosition.LineStart
	t.TextPosition.CharStart = textPosition.CharStart
}

func (t *Token) SetEnd(inputPosition int, textPosition position.Position) {
	t.Literal.End = uint32(inputPosition)
	t.TextPosition.LineEnd = textPosition.LineStart
	t.TextPosition.CharEnd = textPosition.CharStart
}
