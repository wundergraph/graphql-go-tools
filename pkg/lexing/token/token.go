package token

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
)

type Token struct {
	Keyword      keyword.Keyword
	Literal      document.ByteSliceReference
	TextPosition position.Position
}

func (t Token) String() string {
	return fmt.Sprintf("Token:: Keyword: %s, Pos: %s", t.Keyword, t.TextPosition)
}

func (t *Token) SetStart(inputPosition int, textPosition position.Position) {
	t.Literal.Start = uint16(inputPosition)
	t.TextPosition.LineStart = textPosition.LineStart
	t.TextPosition.CharStart = textPosition.CharStart
}

func (t *Token) SetEnd(inputPosition int, textPosition position.Position) {
	t.Literal.End = uint16(inputPosition)
	t.TextPosition.LineEnd = textPosition.LineStart
	t.TextPosition.CharEnd = textPosition.CharStart
}
