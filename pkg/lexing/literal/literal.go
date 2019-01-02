package literal

import "bytes"

var (
	COLON          = ":"
	BANG           = "!"
	LINETERMINATOR = "\n"
	TAB            = "	"
	SPACE          = " "
	QUOTE          = `"`
	COMMA          = ","
	AT             = "@"
	DOLLAR         = "$"
	DOT            = "."
	SPREAD         = "..."
	PIPE           = "|"
	SLASH          = "/"
	BACKSLASH      = "\\"
	EQUALS         = "="
	NEGATIVESIGN   = "-"
	AND            = "&"

	BRACKETOPEN        = "("
	BRACKETCLOSE       = ")"
	SQUAREBRACKETOPEN  = "["
	SQUAREBRACKETCLOSE = "]"
	CURLYBRACKETOPEN   = "{"
	CURLYBRACKETCLOSE  = "}"

	GOBOOL    = "bool"
	GOINT32   = "int32"
	GOFLOAT32 = "float32"
	GOSTRING  = "string"
	GONIL     = "nil"

	EOF          = "eof"
	ID           = "ID"
	BOOLEAN      = "Boolean"
	STRING       = "String"
	INT          = "Int"
	FLOAT        = "Float"
	TYPE         = "type"
	GRAPHQLTYPE  = "graphqlType"
	INTERFACE    = "interface"
	INPUT        = "input"
	SCHEMA       = "schema"
	SCALAR       = "scalar"
	UNION        = "union"
	ENUM         = "enum"
	DIRECTIVE    = "directive"
	QUERY        = "query"
	MUTATION     = "mutation"
	SUBSCRIPTION = "subscription"
	IMPLEMENTS   = "implements"
	ON           = "on"
	FRAGMENT     = "fragment"
	NULL         = "null"

	TRUE  = "true"
	FALSE = "false"
)

type Literal []byte

func (l Literal) Equals(another Literal) bool {
	return bytes.Equal(l, another)
}
