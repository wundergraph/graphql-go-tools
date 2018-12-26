package keyword

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
	case ON:
		return "ON"
	case IMPLEMENTS:
		return "IMPLEMENTS"
	case SCHEMA:
		return "SCHEMA"
	case SCALAR:
		return "SCALAR"
	case TYPE:
		return "TYPE"
	case INTERFACE:
		return "INTERFACE"
	case UNION:
		return "UNION"
	case ENUM:
		return "ENUM"
	case INPUT:
		return "INPUT"
	case DIRECTIVE:
		return "DIRECTIVE"
	case QUERY:
		return "QUERY"
	case MUTATION:
		return "MUTATION"
	case SUBSCRIPTION:
		return "SUBSCRIPTION"
	case FRAGMENT:
		return "FRAGMENT"
	default:
		return fmt.Sprintf("#undefined String case for %d# (see keyword.go)", k)
	}
}

type Literal []byte

func (l Literal) Equals(another Literal) bool {
	return bytes.Equal(l, another)
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
	ON

	IMPLEMENTS
	SCHEMA
	SCALAR
	TYPE
	INTERFACE
	UNION
	ENUM
	INPUT
	DIRECTIVE

	VARIABLE
	STRING
	INTEGER
	FLOAT
	TRUE
	FALSE
	NULL
	QUERY
	MUTATION
	SUBSCRIPTION
	FRAGMENT

	BRACKETOPEN
	BRACKETCLOSE
	SQUAREBRACKETOPEN
	SQUAREBRACKETCLOSE
	CURLYBRACKETOPEN
	CURLYBRACKETCLOSE
)
