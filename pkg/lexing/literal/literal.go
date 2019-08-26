package literal

import "bytes"

var (
	COLON          = []byte(":")
	BANG           = []byte("!")
	LINETERMINATOR = []byte("\n")
	TAB            = []byte("	")
	SPACE          = []byte(" ")
	QUOTE          = []byte("\"")
	COMMA          = []byte(",")
	AT             = []byte("@")
	DOLLAR         = []byte("$")
	DOT            = []byte(".")
	SPREAD         = []byte("...")
	PIPE           = []byte("|")
	SLASH          = []byte("/")
	BACKSLASH      = []byte("\\")
	EQUALS         = []byte("=")
	SUB            = []byte("-")
	AND            = []byte("&")

	LPAREN = []byte("(")
	RPAREN = []byte(")")
	LBRACK = []byte("[")
	RBRACK = []byte("]")
	LBRACE = []byte("{")
	RBRACE = []byte("}")

	GOBOOL    = []byte("bool")
	GOINT32   = []byte("int32")
	GOFLOAT32 = []byte("float32")
	GOSTRING  = []byte("string")
	GONIL     = []byte("nil")

	EOF          = []byte("eof")
	ID           = []byte("ID")
	BOOLEAN      = []byte("Boolean")
	STRING       = []byte("String")
	INT          = []byte("Int")
	FLOAT        = []byte("Float")
	TYPE         = []byte("type")
	TYPENAME     = []byte("__typename")
	GRAPHQLTYPE  = []byte("graphqlType")
	INTERFACE    = []byte("interface")
	INPUT        = []byte("input")
	INCLUDE      = []byte("include")
	IF           = []byte("if")
	SKIP         = []byte("skip")
	SCHEMA       = []byte("schema")
	SCALAR       = []byte("scalar")
	UNION        = []byte("union")
	ENUM         = []byte("enum")
	DIRECTIVE    = []byte("directive")
	QUERY        = []byte("query")
	MUTATION     = []byte("mutation")
	SUBSCRIPTION = []byte("subscription")
	IMPLEMENTS   = []byte("implements")
	ON           = []byte("on")
	FRAGMENT     = []byte("fragment")
	NULL         = []byte("null")

	TRUE  = []byte("true")
	FALSE = []byte("false")
)

type Literal []byte

func (l Literal) Equals(another Literal) bool {
	return bytes.Equal(l, another)
}
