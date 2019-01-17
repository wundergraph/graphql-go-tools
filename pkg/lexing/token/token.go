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
	Description  string
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

/*var (
	EOF = Token{
		Keyword: keyword.EOF,
		Literal: literal.EOF,
	}
	Pipe = Token{
		Keyword: keyword.PIPE,
		Literal: literal.PIPE,
	}
	Dot = Token{
		Keyword: keyword.DOT,
		Literal: literal.DOT,
	}
	Spread = Token{
		Keyword: keyword.SPREAD,
		Literal: literal.SPREAD,
	}
	Equals = Token{
		Keyword: keyword.EQUALS,
		Literal: literal.EQUALS,
	}
	At = Token{
		Keyword: keyword.AT,
		Literal: literal.AT,
	}
	Colon = Token{
		Keyword: keyword.COLON,
		Literal: literal.COLON,
	}
	Bang = Token{
		Keyword: keyword.BANG,
		Literal: literal.BANG,
	}
	BracketOpen = Token{
		Keyword: keyword.BRACKETOPEN,
		Literal: literal.BRACKETOPEN,
	}
	BracketClose = Token{
		Keyword: keyword.BRACKETCLOSE,
		Literal: literal.BRACKETCLOSE,
	}
	CurlyBracketOpen = Token{
		Keyword: keyword.CURLYBRACKETOPEN,
		Literal: literal.CURLYBRACKETOPEN,
	}
	CurlyBracketClose = Token{
		Keyword: keyword.CURLYBRACKETCLOSE,
		Literal: literal.CURLYBRACKETCLOSE,
	}
	SquaredBracketOpen = Token{
		Keyword: keyword.SQUAREBRACKETOPEN,
		Literal: literal.SQUAREBRACKETOPEN,
	}
	SquaredBracketClose = Token{
		Keyword: keyword.SQUAREBRACKETCLOSE,
		Literal: literal.SQUAREBRACKETCLOSE,
	}
	And = Token{
		Keyword: keyword.AND,
		Literal: literal.AND,
	}
)
*/
