package token

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
)

type Token struct {
	Keyword     keyword.Keyword
	Literal     literal.Literal
	Position    position.Position
	Description string
}

func (t Token) String() string {
	return fmt.Sprintf("Token:: Keyword: %s, Literal: %s Pos: %s", t.Keyword, t.Literal, t.Position)
}

var (
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
