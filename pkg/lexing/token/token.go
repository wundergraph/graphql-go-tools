package token

import (
	"bytes"
	"fmt"
)

type Keyword int

func (k Keyword) String() string {
	switch k {
	case IDENT:
		return "IDENT"
	case EOF:
		return "EOF"
	case COLON:
		return "COLON"
	case BANG:
		return "BANG"
	case LINETERMINATOR:
		return "LINETERMINATOR"
	case TAB:
		return "TAB"
	case SPACE:
		return "SPACE"
	case COMMA:
		return "COMMA"
	case COMMENT:
		return "COMMENT"
	case AT:
		return "AT"
	case DOT:
		return "DOT"
	case SPREAD:
		return "SPREAD"
	case PIPE:
		return "PIPE"
	case EQUALS:
		return "EQUALS"
	case BRACKETOPEN:
		return "BRACKETOPEN"
	case BRACKETCLOSE:
		return "BRACKETCLOSE"
	case SQUAREBRACKETOPEN:
		return "SQUAREBRACKETOPEN"
	case SQUAREBRACKETCLOSE:
		return "SQUAREBRACKETCLOSE"
	case CURLYBRACKETOPEN:
		return "CURLYBRACKETOPEN"
	case CURLYBRACKETCLOSE:
		return "CURLYBRACKETCLOSE"
	case VARIABLE:
		return "VARIABLE"
	case NEGATIVESIGN:
		return "NEGATIVESIGN"
	case AND:
		return "AND"
	case INTEGER:
		return "INTEGER"
	case FLOAT:
		return "FLOAT"
	case STRING:
		return "STRING"
	case SLASH:
		return "SLASH"
	case NULL:
		return "NULL"
	default:
		return fmt.Sprintf("#undefined String case for %d# (see token.go)", k)
	}
}

type Position struct {
	Line int
	Char int
}

func (p Position) String() string {
	return fmt.Sprintf("%d:%d", p.Line, p.Char)
}

type Literal []byte

func (l Literal) Equals(another Literal) bool {
	return bytes.Equal(l, another)
}

type Token struct {
	Keyword     Keyword
	Literal     Literal
	Position    Position
	Description string
}

func (t Token) String() string {
	return fmt.Sprintf("Token:: Type: %d, Literal: %s Pos: %s", t.Keyword, t.Literal, t.Position)
}

const (
	UNDEFINED Keyword = iota
	IDENT
	COMMENT
	EOF

	COLON
	BANG
	LINETERMINATOR
	TAB
	SPACE
	COMMA
	AT
	DOT
	SPREAD
	PIPE
	SLASH
	EQUALS
	NEGATIVESIGN
	AND

	VARIABLE
	STRING
	INTEGER
	FLOAT
	TRUE
	FALSE
	NULL

	BRACKETOPEN
	BRACKETCLOSE
	SQUAREBRACKETOPEN
	SQUAREBRACKETCLOSE
	CURLYBRACKETOPEN
	CURLYBRACKETCLOSE
)
