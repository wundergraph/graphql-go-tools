package lexer

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/jensneuse/diffview"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/keyword"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/token"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/goldie"
)

func TestLexer_Peek_Read(t *testing.T) {

	type checkFunc func(lex *Lexer, i int)

	run := func(inStr string, checks ...checkFunc) {

		in := &ast.Input{}
		in.ResetInputBytes([]byte(inStr))
		lexer := &Lexer{}
		lexer.SetInput(in)

		for i := range checks {
			checks[i](lexer, i+1)
		}
	}

	mustRead := func(k keyword.Keyword, wantLiteral string) checkFunc {
		return func(lex *Lexer, i int) {
			tok := lex.Read()
			if k != tok.Keyword {
				panic(fmt.Errorf("mustRead: want(keyword): %s, got: %s [check: %d]", k.String(), tok.String(), i))
			}
			gotLiteral := string(lex.input.ByteSlice(tok.Literal))
			if wantLiteral != gotLiteral {
				panic(fmt.Errorf("mustRead: want(literal): %s, got: %s [check: %d]", wantLiteral, gotLiteral, i))
			}
		}
	}

	resetInput := func(input string) checkFunc {
		return func(lex *Lexer, i int) {
			lex.input.ResetInputBytes([]byte(input))
		}
	}

	mustReadPosition := func(lineStart, charStart, lineEnd, charEnd uint32) checkFunc {
		return func(lex *Lexer, i int) {
			tok := lex.Read()

			if lineStart != tok.TextPosition.LineStart {
				panic(fmt.Errorf("mustReadPosition: want(lineStart): %d, got: %d [check: %d]", lineStart, tok.TextPosition.LineStart, i))
			}
			if charStart != tok.TextPosition.CharStart {
				panic(fmt.Errorf("mustReadPosition: want(charStart): %d, got: %d [check: %d]", charStart, tok.TextPosition.CharStart, i))
			}
			if lineEnd != tok.TextPosition.LineEnd {
				panic(fmt.Errorf("mustReadPosition: want(lineEnd): %d, got: %d [check: %d]", lineEnd, tok.TextPosition.LineEnd, i))
			}
			if charEnd != tok.TextPosition.CharEnd {
				panic(fmt.Errorf("mustReadPosition: want(charEnd): %d, got: %d [check: %d]", charEnd, tok.TextPosition.CharEnd, i))
			}
		}
	}

	mustPeekWhitespaceLength := func(want int) checkFunc {
		return func(lex *Lexer, i int) {
			got := lex.peekWhitespaceLength()
			if want != got {
				panic(fmt.Errorf("mustPeekWhitespaceLength: want: %d, got: %d [check: %d]", want, got, i))
			}
		}
	}

	t.Run("peek whitespace length", func(t *testing.T) {
		run("   foo", mustPeekWhitespaceLength(3))
	})
	t.Run("peek whitespace length with tab", func(t *testing.T) {
		run("   	foo", mustPeekWhitespaceLength(4))
	})
	t.Run("peek whitespace length with linebreak", func(t *testing.T) {
		run("   \nfoo", mustPeekWhitespaceLength(4))
	})
	t.Run("peek whitespace length with carriage return", func(t *testing.T) {
		run("   \rfoo", mustPeekWhitespaceLength(4))
	})
	t.Run("peek whitespace length with comma", func(t *testing.T) {
		run("   ,foo", mustPeekWhitespaceLength(4))
	})
	t.Run("read correct when resetting input", func(t *testing.T) {
		run("x",
			mustRead(keyword.IDENT, "x"),
			resetInput("y"),
			mustRead(keyword.IDENT, "y"),
		)
	})
	t.Run("read eof multiple times", func(t *testing.T) {
		run("x",
			mustRead(keyword.IDENT, "x"),
			mustRead(keyword.EOF, ""),
			mustRead(keyword.EOF, ""),
		)
	})
	t.Run("read integer", func(t *testing.T) {
		run("1337", mustRead(keyword.INTEGER, "1337"))
	})
	t.Run("read negative integer", func(t *testing.T) {
		run("-1337", mustRead(keyword.SUB, "-"),
			mustRead(keyword.INTEGER, "1337"))
	})
	t.Run("read integer with comma", func(t *testing.T) {
		run("1337,", mustRead(keyword.INTEGER, "1337"))
	})
	t.Run("read float", func(t *testing.T) {
		run("13.37", mustRead(keyword.FLOAT, "13.37"))
	})
	t.Run("read negative float", func(t *testing.T) {
		run("-13.37", mustRead(keyword.SUB, "-"),
			mustRead(keyword.FLOAT, "13.37"))
	})
	t.Run("read float", func(t *testing.T) {
		run("1.1)", mustRead(keyword.FLOAT, "1.1"))
	})
	t.Run("read float with space", func(t *testing.T) {
		run("13.37 ", mustRead(keyword.FLOAT, "13.37"))
	})
	t.Run("read float with tab", func(t *testing.T) {
		run("13.37	", mustRead(keyword.FLOAT, "13.37"))
	})
	t.Run("read with lineTerminator", func(t *testing.T) {
		run("13.37\n", mustRead(keyword.FLOAT, "13.37"))
	})
	t.Run("read with carriage return and line feed", func(t *testing.T) {
		run("13.37\r\n", mustRead(keyword.FLOAT, "13.37"))
	})
	t.Run("read float with comma", func(t *testing.T) {
		run("13.37,", mustRead(keyword.FLOAT, "13.37"))
	})
	t.Run("read float + . + int", func(t *testing.T) {
		run("1.3.3", mustRead(keyword.FLOAT, "1.3"),
			mustRead(keyword.DOT, "."),
			mustRead(keyword.INTEGER, "3"),
		)
	})
	t.Run("read float + ident", func(t *testing.T) {
		run("1.3x", mustRead(keyword.FLOAT, "1.3"),
			mustRead(keyword.IDENT, "x"),
		)
	})
	t.Run("fail reading incomplete float", func(t *testing.T) {
		run("13.", mustRead(keyword.FLOAT, "13."))
	})
	t.Run("read plancks constant", func(t *testing.T) {
		run("6.63E-34", mustRead(keyword.FLOAT, "6.63E-34"))
	})
	t.Run("read electron mass kg", func(t *testing.T) {
		run("9.10938356e-3", mustRead(keyword.FLOAT, "9.10938356e-3"))
	})
	t.Run("read earth mass kg", func(t *testing.T) {
		run("5.9724e24", mustRead(keyword.FLOAT, "5.9724e24"))
	})
	t.Run("read earth circumference m", func(t *testing.T) {
		run("4E7", mustRead(keyword.FLOAT, "4E7"))
	})
	t.Run("read an inch in mm", func(t *testing.T) {
		run("2.54E+1", mustRead(keyword.FLOAT, "2.54E+1"))
	})
	t.Run("read electron charge/mass ratio", func(t *testing.T) {
		run("-1.758E11", mustRead(keyword.SUB, "-"),
			mustRead(keyword.FLOAT, "1.758E11"))
	})
	t.Run("read single line string", func(t *testing.T) {
		run("\"foo\"", mustRead(keyword.STRING, "foo"))
	})
	t.Run("read single line string with leading/trailing whitespace", func(t *testing.T) {
		run("\" 	foo	 \"", mustRead(keyword.STRING, "foo"))
	})
	t.Run("peek incomplete string as quote", func(t *testing.T) {
		run("\"foo", mustRead(keyword.STRING, "foo"))
	})
	t.Run("read single line string with escaped quote", func(t *testing.T) {
		run("\"foo \\\" bar\"", mustRead(keyword.STRING, "foo \\\" bar"))
	})
	t.Run("read single line string with escaped backslash", func(t *testing.T) {
		run("\"foo \\\\ bar\"", mustRead(keyword.STRING, "foo \\\\ bar"))
	})
	t.Run("read multi line string with escaped quote", func(t *testing.T) {
		run("\"\"\"foo \\\" bar\"\"\"", mustRead(keyword.BLOCKSTRING, "foo \\\" bar"))
	})
	t.Run("read multi line string with two escaped quotes", func(t *testing.T) {
		run("\"\"\"foo \"\" bar\"\"\"", mustRead(keyword.BLOCKSTRING, "foo \"\" bar"))
	})
	t.Run("read multi line string", func(t *testing.T) {
		run("\"\"\"\nfoo\nbar\"\"\"", mustRead(keyword.BLOCKSTRING, "foo\nbar"))
	})
	t.Run("read multi line string with carriage return", func(t *testing.T) {
		run("\"\"\"\r\nfoo\r\nbar\"\"\"", mustRead(keyword.BLOCKSTRING, "foo\r\nbar"))
	})
	t.Run("read multi line string with escaped backslash", func(t *testing.T) {
		run("\"\"\"foo \\\\ bar\"\"\"", mustRead(keyword.BLOCKSTRING, "foo \\\\ bar"))
	})
	t.Run("read multi line string with leading/trailing space", func(t *testing.T) {
		run(`""" foo """`, mustRead(keyword.BLOCKSTRING, "foo"))
	})
	t.Run("read multi line string with trailing leading/trailing tab", func(t *testing.T) {
		run(`"""	foo	"""`, mustRead(keyword.BLOCKSTRING, "foo"))
	})
	t.Run("read multi line string with trailing leading/trailing LT", func(t *testing.T) {
		run(`"""
	  	foo 
"""`, mustRead(keyword.BLOCKSTRING, "foo"))
	})
	t.Run("complex multi line string", func(t *testing.T) {
		run("\"\"\"block string uses \\\"\"\"\n\"\"\"", mustRead(keyword.BLOCKSTRING, "block string uses \\\"\"\""))
	})
	t.Run("complex multi line string with carriage return", func(t *testing.T) {
		run("\"\"\"block string uses \\\"\"\"\r\n\"\"\"", mustRead(keyword.BLOCKSTRING, "block string uses \\\"\"\""))
	})
	t.Run("read multi line string with trailing leading/trailing whitespace combination", func(t *testing.T) {
		run(`	"""	 	 
						foo
				  	"""`, mustRead(keyword.BLOCKSTRING, "foo"))
	})
	t.Run("read pipe", func(t *testing.T) {
		run("|", mustRead(keyword.PIPE, "|"))
	})
	t.Run("err reading dot", func(t *testing.T) {
		run(".", mustRead(keyword.DOT, "."))
	})
	t.Run("read fragment spread", func(t *testing.T) {
		run("...", mustRead(keyword.SPREAD, "..."))
	})
	t.Run("must not read invalid fragment spread", func(t *testing.T) {
		run("..",
			mustRead(keyword.DOT, "."),
			mustRead(keyword.DOT, "."))
	})
	t.Run("read variable", func(t *testing.T) {
		run("$123", mustRead(keyword.DOLLAR, "$"),
			mustRead(keyword.INTEGER, "123"))
	})
	t.Run("read variable 2", func(t *testing.T) {
		run("$foo", mustRead(keyword.DOLLAR, "$"),
			mustRead(keyword.IDENT, "foo"))
	})
	t.Run("read variable 3", func(t *testing.T) {
		run("$_foo", mustRead(keyword.DOLLAR, "$"),
			mustRead(keyword.IDENT, "_foo"))
	})
	t.Run("read variable 3", func(t *testing.T) {
		run("$123", mustRead(keyword.DOLLAR, "$"),
			mustRead(keyword.INTEGER, "123"))
	})
	t.Run("read variable 4", func(t *testing.T) {
		run("$foo\n", mustRead(keyword.DOLLAR, "$"),
			mustRead(keyword.IDENT, "foo"))
	})
	t.Run("read variable 4 with carriage return", func(t *testing.T) {
		run("$foo\r\n", mustRead(keyword.DOLLAR, "$"),
			mustRead(keyword.IDENT, "foo"))
	})
	t.Run("read err invalid variable", func(t *testing.T) {
		run("$ foo",
			mustRead(keyword.DOLLAR, "$"),
			mustRead(keyword.IDENT, "foo"),
		)
	})
	t.Run("read @", func(t *testing.T) {
		run("@", mustRead(keyword.AT, "@"))
	})
	t.Run("read equals", func(t *testing.T) {
		run("=", mustRead(keyword.EQUALS, "="))
	})
	t.Run("read variable colon", func(t *testing.T) {
		run(":", mustRead(keyword.COLON, ":"))
	})
	t.Run("read bang", func(t *testing.T) {
		run("!", mustRead(keyword.BANG, "!"))
	})
	t.Run("read bracket open", func(t *testing.T) {
		run("(", mustRead(keyword.LPAREN, "("))
	})
	t.Run("read bracket close", func(t *testing.T) {
		run(")", mustRead(keyword.RPAREN, ")"))
	})
	t.Run("read squared bracket open", func(t *testing.T) {
		run("[", mustRead(keyword.LBRACK, "["))
	})
	t.Run("read squared bracket close", func(t *testing.T) {
		run("]", mustRead(keyword.RBRACK, "]"))
	})
	t.Run("read curly bracket open", func(t *testing.T) {
		run("{", mustRead(keyword.LBRACE, "{"))
	})
	t.Run("read curly bracket close", func(t *testing.T) {
		run("}", mustRead(keyword.RBRACE, "}"))
	})
	t.Run("read and", func(t *testing.T) {
		run("&", mustRead(keyword.AND, "&"))
	})
	t.Run("read EOF", func(t *testing.T) {
		run("", mustRead(keyword.EOF, ""))
	})
	t.Run("read ident", func(t *testing.T) {
		run("foo", mustRead(keyword.IDENT, "foo"))
	})
	t.Run("read ident with colon", func(t *testing.T) {
		run("foo:", mustRead(keyword.IDENT, "foo"))
	})
	t.Run("read ident with negative sign", func(t *testing.T) {
		run("foo-bar", mustRead(keyword.IDENT, "foo-bar"))
	})
	t.Run("read true", func(t *testing.T) {
		run("true", mustRead(keyword.IDENT, "true"))
	})
	t.Run("read true with space", func(t *testing.T) {
		run(" true ", mustRead(keyword.IDENT, "true"))
	})
	t.Run("read false", func(t *testing.T) {
		run("false", mustRead(keyword.IDENT, "false"))
	})
	t.Run("read query", func(t *testing.T) {
		run("query", mustRead(keyword.IDENT, "query"))
	})
	t.Run("read mutation", func(t *testing.T) {
		run("mutation", mustRead(keyword.IDENT, "mutation"))
	})
	t.Run("read subscription", func(t *testing.T) {
		run("subscription", mustRead(keyword.IDENT, "subscription"))
	})
	t.Run("read fragment", func(t *testing.T) {
		run("fragment", mustRead(keyword.IDENT, "fragment"))
	})
	t.Run("read fragment", func(t *testing.T) {
		run("\n\n fragment", mustRead(keyword.IDENT, "fragment"))
	})
	t.Run("read fragment with carriage return", func(t *testing.T) {
		run("\r\n\r\n fragment", mustRead(keyword.IDENT, "fragment"))
	})
	t.Run("read implements", func(t *testing.T) {
		run("implements", mustRead(keyword.IDENT, "implements"))
	})
	t.Run("read schema", func(t *testing.T) {
		run("schema", mustRead(keyword.IDENT, "schema"))
	})
	t.Run("read scalar", func(t *testing.T) {
		run("scalar", mustRead(keyword.IDENT, "scalar"))
	})
	t.Run("read type", func(t *testing.T) {
		run("type", mustRead(keyword.IDENT, "type"))
	})
	t.Run("read interface", func(t *testing.T) {
		run("interface", mustRead(keyword.IDENT, "interface"))
	})
	t.Run("read union", func(t *testing.T) {
		run("union", mustRead(keyword.IDENT, "union"))
	})
	t.Run("read enum", func(t *testing.T) {
		run("enum", mustRead(keyword.IDENT, "enum"))
	})
	t.Run("read input", func(t *testing.T) {
		run("input", mustRead(keyword.IDENT, "input"))
	})
	t.Run("read directive", func(t *testing.T) {
		run("directive", mustRead(keyword.IDENT, "directive"))
	})
	t.Run("read inputValue", func(t *testing.T) {
		run("inputValue", mustRead(keyword.IDENT, "inputValue"))
	})
	t.Run("read extend", func(t *testing.T) {
		run("extend", mustRead(keyword.IDENT, "extend"))
	})
	t.Run("read on", func(t *testing.T) {
		run("on", mustRead(keyword.IDENT, "on"))
	})
	t.Run("read on with whitespace", func(t *testing.T) {
		run("on ", mustRead(keyword.IDENT, "on"))
	})
	t.Run("read ignore comma", func(t *testing.T) {
		run(",", mustRead(keyword.EOF, ""))
	})
	t.Run("read ignore space", func(t *testing.T) {
		run(" ", mustRead(keyword.EOF, ""))
	})
	t.Run("read ignore tab", func(t *testing.T) {
		run("	", mustRead(keyword.EOF, ""))
	})
	t.Run("read ignore lineTerminator", func(t *testing.T) {
		run("\n", mustRead(keyword.EOF, ""))
	})
	t.Run("read ignore lineTerminator with carriage return", func(t *testing.T) {
		run("\r\n", mustRead(keyword.EOF, ""))
	})
	t.Run("read null", func(t *testing.T) {
		run("null", mustRead(keyword.IDENT, "null"))
	})
	t.Run("read single line comment", func(t *testing.T) {
		run("# A connection to a list of items.",
			mustRead(keyword.COMMENT, "# A connection to a list of items."))
	})
	t.Run("read single line comment", func(t *testing.T) {
		run("#	A connection to a list of items.",
			mustRead(keyword.COMMENT, "#	A connection to a list of items."))
	})
	t.Run("read single line comment", func(t *testing.T) {
		run("# A connection to a list of items.\nident",
			mustRead(keyword.COMMENT, "# A connection to a list of items."),
			mustRead(keyword.IDENT, "ident"),
		)
	})
	t.Run("read single line comment with carriage return", func(t *testing.T) {
		run("# A connection to a list of items.\r\nident",
			mustRead(keyword.COMMENT, "# A connection to a list of items."),
			mustRead(keyword.IDENT, "ident"),
		)
	})
	t.Run("read multi line comment", func(t *testing.T) {
		run(`#1
#2
#three`,
			mustRead(keyword.COMMENT, "#1\n#2\n#three"),
		)
	})
	t.Run("multi read 'foo:'", func(t *testing.T) {
		run("foo:",
			mustRead(keyword.IDENT, "foo"),
			mustRead(keyword.COLON, ":"),
		)
	})
	t.Run("multi read '1,2,3'", func(t *testing.T) {
		run("1,2,3",
			mustRead(keyword.INTEGER, "1"),
			mustRead(keyword.INTEGER, "2"),
			mustRead(keyword.INTEGER, "3"),
		)
	})
	t.Run("multi read positions", func(t *testing.T) {
		run(`foo bar baz
bal
 bas """
x"""
"foo bar baz "
 ...
$foo 
 1337 `,
			mustReadPosition(1, 1, 1, 4),
			mustReadPosition(1, 5, 1, 8),
			mustReadPosition(1, 9, 1, 12),
			mustReadPosition(2, 1, 2, 4),
			mustReadPosition(3, 2, 3, 5),
			mustReadPosition(3, 6, 4, 5),
			mustReadPosition(5, 1, 5, 15),
			mustReadPosition(6, 2, 6, 5),
			mustReadPosition(7, 1, 7, 2),
			mustReadPosition(7, 2, 7, 5),
			mustReadPosition(8, 2, 8, 6),
		)
	})
	t.Run("multi read nested structure", func(t *testing.T) {
		run(`Goland {
						... on GoWater {
							... on GoAir {
								go
							}
						}
					}`,
			mustRead(keyword.IDENT, "Goland"), mustRead(keyword.LBRACE, "{"),
			mustRead(keyword.SPREAD, "..."), mustRead(keyword.IDENT, "on"), mustRead(keyword.IDENT, "GoWater"), mustRead(keyword.LBRACE, "{"),
			mustRead(keyword.SPREAD, "..."), mustRead(keyword.IDENT, "on"), mustRead(keyword.IDENT, "GoAir"), mustRead(keyword.LBRACE, "{"),
			mustRead(keyword.IDENT, "go"),
			mustRead(keyword.RBRACE, "}"),
			mustRead(keyword.RBRACE, "}"),
			mustRead(keyword.RBRACE, "}"),
		)
	})
	t.Run("multi read many idents and strings", func(t *testing.T) {
		run(`1337 1338 1339 "foo" "bar" """foo bar""" """foo
bar""" """foo
bar
baz
"""
13.37`,
			mustRead(keyword.INTEGER, "1337"), mustRead(keyword.INTEGER, "1338"), mustRead(keyword.INTEGER, "1339"),
			mustRead(keyword.STRING, "foo"), mustRead(keyword.STRING, "bar"), mustRead(keyword.BLOCKSTRING, "foo bar"),
			mustRead(keyword.BLOCKSTRING, "foo\nbar"),
			mustRead(keyword.BLOCKSTRING, "foo\nbar\nbaz"),
			mustRead(keyword.FLOAT, "13.37"),
		)
	})
	t.Run("append input", func(t *testing.T) {

		in := &ast.Input{}
		lexer := &Lexer{}
		lexer.SetInput(in)

		in.ResetInputBytes([]byte("foo"))

		foo := lexer.Read()
		if string(in.ByteSlice(foo.Literal)) != "foo" {
			t.Fatal("want foo")
		}

		in.AppendInputBytes([]byte(" bar"))

		bar := lexer.Read()
		if string(in.ByteSlice(bar.Literal)) != "bar" {
			t.Fatal("want bar")
		}

		in.AppendInputBytes([]byte(" baz"))

		baz := lexer.Read()
		if string(in.ByteSlice(baz.Literal)) != "baz" {
			t.Fatal("want baz")
		}
	})
}

var introspectionQuery = `query IntrospectionQuery {
  __schema {
    queryType {
      name
    }
    mutationType {
      name
    }
    subscriptionType {
      name
    }
    types {
      ...FullType
    }
    directives {
      name
      description
      locations
      args {
        ...InputValue
      }
    }
  }
}

fragment FullType on __Type {
  kind
  name
  description
  fields(includeDeprecated: true) {
    name
    description
    args {
      ...InputValue
    }
    type {
      ...TypeRef
    }
    isDeprecated
    deprecationReason
  }
  inputFields {
    ...InputValue
  }
  interfaces {
    ...TypeRef
  }
  enumValues(includeDeprecated: true) {
    name
    description
    isDeprecated
    deprecationReason
  }
  possibleTypes {
    ...TypeRef
  }
}

fragment InputValue on __InputValue {
  name
  description
  type {
    ...TypeRef
  }
  defaultValue
}

fragment TypeRef on __Type {
  kind
  name
  ofType {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
              }
            }
          }
        }
      }
    }
  }
}`

func TestLexerRegressions(t *testing.T) {

	in := &ast.Input{}
	in.ResetInputBytes([]byte(introspectionQuery))
	lexer := &Lexer{}
	lexer.SetInput(in)

	var total []token.Token
	for {
		tok := lexer.Read()
		if tok.Keyword == keyword.EOF {
			break
		}

		total = append(total, tok)
	}

	data, err := json.MarshalIndent(total, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	goldie.Assert(t, "introspection_lexed", data)
	if t.Failed() {

		fixture, err := os.ReadFile("./fixtures/introspection_lexed.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("introspection_lexed", fixture, data)
	}
}

func BenchmarkLexer(b *testing.B) {

	in := &ast.Input{}
	lexer := &Lexer{}
	lexer.SetInput(in)

	inputBytes := []byte(introspectionQuery)

	b.ReportAllocs()
	b.ResetTimer()
	b.SetBytes(int64(len(inputBytes)))

	for i := 0; i < b.N; i++ {

		in.ResetInputBytes(inputBytes)

		var key keyword.Keyword

		for key != keyword.EOF {
			key = lexer.Read().Keyword
		}
	}
}
