package lexer

import (
	"encoding/json"
	"fmt"
	"github.com/jensneuse/diffview"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
	"github.com/sebdah/goldie"
	"io/ioutil"
	"testing"
)

func TestLexer_Peek_Read(t *testing.T) {

	type checkFunc func(lex *Lexer, i int)

	run := func(input string, checks ...checkFunc) {
		lex := NewLexer()
		if err := lex.SetInput([]byte(input)); err != nil {
			panic(err)
		}
		for i := range checks {
			checks[i](lex, i+1)
		}
	}

	mustPeek := func(k keyword.Keyword, ignoreWhitespace bool) checkFunc {
		return func(lex *Lexer, i int) {
			peeked := lex.Peek(ignoreWhitespace)
			if peeked != k {
				panic(fmt.Errorf("mustPeek: want: %s, got: %s [check: %d]", k.String(), peeked.String(), i))
			}
		}
	}

	mustRead := func(k keyword.Keyword, wantLiteral string) checkFunc {
		return func(lex *Lexer, i int) {
			tok := lex.Read()
			if k != tok.Keyword {
				panic(fmt.Errorf("mustRead: want(keyword): %s, got: %s [check: %d]", k.String(), tok.String(), i))
			}
			gotLiteral := string(lex.ByteSlice(tok.Literal))
			if wantLiteral != gotLiteral {
				panic(fmt.Errorf("mustRead: want(literal): %s, got: %s [check: %d]", wantLiteral, gotLiteral, i))
			}
		}
	}

	mustPeekAndRead := func(k keyword.Keyword, literal string) checkFunc {
		return func(lex *Lexer, i int) {
			mustPeek(k, true)(lex, i)
			mustRead(k, literal)(lex, i)
		}
	}

	resetInput := func(input string) checkFunc {
		return func(lex *Lexer, i int) {
			if err := lex.SetInput([]byte(input)); err != nil {
				panic(err)
			}
		}
	}

	mustReadPosition := func(lineStart, charStart, lineEnd, charEnd uint16) checkFunc {
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

	t.Run("set too large input", func(t *testing.T) {
		lex := NewLexer()
		if err := lex.SetInput(make([]byte, 65536)); err == nil {
			panic(fmt.Errorf("must err on too large input"))
		}
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
	t.Run("peek space", func(t *testing.T) {
		run(" ", mustPeek(keyword.SPACE, false))
	})
	t.Run("peek tab", func(t *testing.T) {
		run("\t", mustPeek(keyword.TAB, false))
	})
	t.Run("peek tab 2", func(t *testing.T) {
		run("	", mustPeek(keyword.TAB, false))
	})
	t.Run("peek comma", func(t *testing.T) {
		run(",", mustPeek(keyword.COMMA, false))
	})
	t.Run("peek line terminator", func(t *testing.T) {
		run("\n", mustPeek(keyword.LINETERMINATOR, false))
	})
	t.Run("peek line terminator 2", func(t *testing.T) {
		run(`
`, mustPeek(keyword.LINETERMINATOR, false))
	})
	t.Run("peek dot", func(t *testing.T) {
		run(".. ", mustPeek(keyword.DOT, false))
	})
	t.Run("read integer", func(t *testing.T) {
		run("1337", mustPeekAndRead(keyword.INTEGER, "1337"))
	})
	t.Run("read integer with comma", func(t *testing.T) {
		run("1337,", mustPeekAndRead(keyword.INTEGER, "1337"))
	})
	t.Run("read float", func(t *testing.T) {
		run("13.37", mustPeekAndRead(keyword.FLOAT, "13.37"))
	})
	t.Run("read float with space", func(t *testing.T) {
		run("13.37 ", mustPeekAndRead(keyword.FLOAT, "13.37"))
	})
	t.Run("read float with tab", func(t *testing.T) {
		run("13.37	", mustPeekAndRead(keyword.FLOAT, "13.37"))
	})
	t.Run("read with with lineTerminator", func(t *testing.T) {
		run("13.37\n", mustPeekAndRead(keyword.FLOAT, "13.37"))
	})
	t.Run("read float with comma", func(t *testing.T) {
		run("13.37,", mustPeekAndRead(keyword.FLOAT, "13.37"))
	})
	t.Run("peek invalid float as integer", func(t *testing.T) {
		run("1.3.3", mustPeek(keyword.INTEGER, true))
	})
	t.Run("peek invalid float as integer 2", func(t *testing.T) {
		run("1.3x", mustPeek(keyword.INTEGER, true))
	})
	t.Run("fail reading incomplete float", func(t *testing.T) {
		run("13.", mustPeekAndRead(keyword.FLOAT, "13."))
	})
	t.Run("read single line string", func(t *testing.T) {
		run("\"foo\"", mustPeekAndRead(keyword.STRING, "foo"))
	})
	t.Run("read single line string with escaped quote", func(t *testing.T) {
		run("\"foo \\\" bar\"", mustPeekAndRead(keyword.STRING, "foo \\\" bar"))
	})
	t.Run("read single line string with escaped backslash", func(t *testing.T) {
		run("\"foo \\\\ bar\"", mustPeekAndRead(keyword.STRING, "foo \\\\ bar"))
	})
	t.Run("read multi line string with escaped quote", func(t *testing.T) {
		run("\"\"\"foo \\\" bar\"\"\"", mustPeekAndRead(keyword.STRING, "foo \\\" bar"))
	})
	t.Run("read multi line string with two escaped quotes", func(t *testing.T) {
		run("\"\"\"foo \"\" bar\"\"\"", mustPeekAndRead(keyword.STRING, "foo \"\" bar"))
	})
	t.Run("read multi line string", func(t *testing.T) {
		run("\"\"\"\nfoo\nbar\"\"\"", mustPeekAndRead(keyword.STRING, "\nfoo\nbar"))
	})
	t.Run("read multi line string with escaped backslash", func(t *testing.T) {
		run("\"\"\"foo \\\\ bar\"\"\"", mustPeekAndRead(keyword.STRING, "foo \\\\ bar"))
	})
	t.Run("read pipe", func(t *testing.T) {
		run("|", mustPeekAndRead(keyword.PIPE, "|"))
	})
	t.Run("err reading dot", func(t *testing.T) {
		run(".", mustPeekAndRead(keyword.DOT, "."))
	})
	t.Run("read fragment spread", func(t *testing.T) {
		run("...", mustPeekAndRead(keyword.SPREAD, "..."))
	})
	t.Run("must not read invalid fragment spread", func(t *testing.T) {
		run("..",
			mustPeekAndRead(keyword.DOT, "."),
			mustPeekAndRead(keyword.DOT, "."))
	})
	t.Run("read variable", func(t *testing.T) {
		run("$123", mustPeekAndRead(keyword.VARIABLE, "123"))
	})
	t.Run("read variable 2", func(t *testing.T) {
		run("$foo", mustPeekAndRead(keyword.VARIABLE, "foo"))
	})
	t.Run("read variable 3", func(t *testing.T) {
		run("$_foo", mustPeekAndRead(keyword.VARIABLE, "_foo"))
	})
	t.Run("read variable 3", func(t *testing.T) {
		run("$123", mustPeekAndRead(keyword.VARIABLE, "123"))
	})
	t.Run("read variable 4", func(t *testing.T) {
		run("$foo\n", mustPeekAndRead(keyword.VARIABLE, "foo"))
	})
	t.Run("read err invalid variable", func(t *testing.T) {
		run("$ foo",
			mustPeekAndRead(keyword.VARIABLE, ""),
			mustPeekAndRead(keyword.IDENT, "foo"),
		)
	})
	t.Run("read @", func(t *testing.T) {
		run("@", mustPeekAndRead(keyword.AT, "@"))
	})
	t.Run("read equals", func(t *testing.T) {
		run("=", mustPeekAndRead(keyword.EQUALS, "="))
	})
	t.Run("read variable colon", func(t *testing.T) {
		run(":", mustPeekAndRead(keyword.COLON, ":"))
	})
	t.Run("read bang", func(t *testing.T) {
		run("!", mustPeekAndRead(keyword.BANG, "!"))
	})
	t.Run("read bracket open", func(t *testing.T) {
		run("(", mustPeekAndRead(keyword.BRACKETOPEN, "("))
	})
	t.Run("read bracket close", func(t *testing.T) {
		run(")", mustPeekAndRead(keyword.BRACKETCLOSE, ")"))
	})
	t.Run("read squared bracket open", func(t *testing.T) {
		run("[", mustPeekAndRead(keyword.SQUAREBRACKETOPEN, "["))
	})
	t.Run("read squared bracket close", func(t *testing.T) {
		run("]", mustPeekAndRead(keyword.SQUAREBRACKETCLOSE, "]"))
	})
	t.Run("read curly bracket open", func(t *testing.T) {
		run("{", mustPeekAndRead(keyword.CURLYBRACKETOPEN, "{"))
	})
	t.Run("read curly bracket close", func(t *testing.T) {
		run("}", mustPeekAndRead(keyword.CURLYBRACKETCLOSE, "}"))
	})
	t.Run("read and", func(t *testing.T) {
		run("&", mustPeekAndRead(keyword.AND, "&"))
	})
	t.Run("read EOF", func(t *testing.T) {
		run("", mustPeekAndRead(keyword.EOF, ""))
	})
	t.Run("read ident", func(t *testing.T) {
		run("foo", mustPeekAndRead(keyword.IDENT, "foo"))
	})
	t.Run("read ident with colon", func(t *testing.T) {
		run("foo:", mustPeekAndRead(keyword.IDENT, "foo"))
	})
	t.Run("read ident with negative sign", func(t *testing.T) {
		run("foo-bar", mustPeekAndRead(keyword.IDENT, "foo-bar"))
	})
	t.Run("read true", func(t *testing.T) {
		run("true", mustPeekAndRead(keyword.TRUE, "true"))
	})
	t.Run("read true with space", func(t *testing.T) {
		run(" true ", mustPeekAndRead(keyword.TRUE, "true"))
	})
	t.Run("read false", func(t *testing.T) {
		run("false", mustPeekAndRead(keyword.FALSE, "false"))
	})
	t.Run("read query", func(t *testing.T) {
		run("query", mustPeekAndRead(keyword.QUERY, "query"))
	})
	t.Run("read mutation", func(t *testing.T) {
		run("mutation", mustPeekAndRead(keyword.MUTATION, "mutation"))
	})
	t.Run("read subscription", func(t *testing.T) {
		run("subscription", mustPeekAndRead(keyword.SUBSCRIPTION, "subscription"))
	})
	t.Run("read fragment", func(t *testing.T) {
		run("fragment", mustPeekAndRead(keyword.FRAGMENT, "fragment"))
	})
	t.Run("read implements", func(t *testing.T) {
		run("implements", mustPeekAndRead(keyword.IMPLEMENTS, "implements"))
	})
	t.Run("read schema", func(t *testing.T) {
		run("schema", mustPeekAndRead(keyword.SCHEMA, "schema"))
	})
	t.Run("read scalar", func(t *testing.T) {
		run("scalar", mustPeekAndRead(keyword.SCALAR, "scalar"))
	})
	t.Run("read type", func(t *testing.T) {
		run("type", mustPeekAndRead(keyword.TYPE, "type"))
	})
	t.Run("read interface", func(t *testing.T) {
		run("interface", mustPeekAndRead(keyword.INTERFACE, "interface"))
	})
	t.Run("read union", func(t *testing.T) {
		run("union", mustPeekAndRead(keyword.UNION, "union"))
	})
	t.Run("read enum", func(t *testing.T) {
		run("enum", mustPeekAndRead(keyword.ENUM, "enum"))
	})
	t.Run("read input", func(t *testing.T) {
		run("input", mustPeekAndRead(keyword.INPUT, "input"))
	})
	t.Run("read directive", func(t *testing.T) {
		run("directive", mustPeekAndRead(keyword.DIRECTIVE, "directive"))
	})
	t.Run("read inputValue", func(t *testing.T) {
		run("inputValue", mustPeekAndRead(keyword.IDENT, "inputValue"))
	})
	t.Run("read on", func(t *testing.T) {
		run("on", mustPeekAndRead(keyword.ON, "on"))
	})
	t.Run("read on with whitespace", func(t *testing.T) {
		run("on ", mustPeekAndRead(keyword.ON, "on"))
	})
	t.Run("read ignore comma", func(t *testing.T) {
		run(",", mustPeekAndRead(keyword.EOF, ""))
	})
	t.Run("read ignore space", func(t *testing.T) {
		run(" ", mustPeekAndRead(keyword.EOF, ""))
	})
	t.Run("read ignore tab", func(t *testing.T) {
		run("	", mustPeekAndRead(keyword.EOF, ""))
	})
	t.Run("read ignore lineTerminator", func(t *testing.T) {
		run("\n", mustPeekAndRead(keyword.EOF, ""))
	})
	t.Run("read null", func(t *testing.T) {
		run("null", mustPeekAndRead(keyword.NULL, "null"))
	})
	t.Run("multi read 'foo:'", func(t *testing.T) {
		run("foo:",
			mustPeekAndRead(keyword.IDENT, "foo"),
			mustPeekAndRead(keyword.COLON, ":"),
		)
	})
	t.Run("multi read '1,2,3'", func(t *testing.T) {
		run("1,2,3",
			mustPeekAndRead(keyword.INTEGER, "1"),
			mustPeekAndRead(keyword.INTEGER, "2"),
			mustPeekAndRead(keyword.INTEGER, "3"),
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
			mustReadPosition(7, 1, 7, 5),
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
			mustPeekAndRead(keyword.IDENT, "Goland"), mustPeekAndRead(keyword.CURLYBRACKETOPEN, "{"),
			mustPeekAndRead(keyword.SPREAD, "..."), mustPeekAndRead(keyword.ON, "on"), mustPeekAndRead(keyword.IDENT, "GoWater"), mustPeekAndRead(keyword.CURLYBRACKETOPEN, "{"),
			mustPeekAndRead(keyword.SPREAD, "..."), mustPeekAndRead(keyword.ON, "on"), mustPeekAndRead(keyword.IDENT, "GoAir"), mustPeekAndRead(keyword.CURLYBRACKETOPEN, "{"),
			mustPeekAndRead(keyword.IDENT, "go"),
			mustPeekAndRead(keyword.CURLYBRACKETCLOSE, "}"),
			mustPeekAndRead(keyword.CURLYBRACKETCLOSE, "}"),
			mustPeekAndRead(keyword.CURLYBRACKETCLOSE, "}"),
		)
	})
	t.Run("multi read many idents and strings", func(t *testing.T) {
		run(`1337 1338 1339 "foo" "bar" """foo bar""" """foo
bar""" """foo
bar
baz
"""
13.37`,
			mustPeekAndRead(keyword.INTEGER, "1337"), mustPeekAndRead(keyword.INTEGER, "1338"), mustPeekAndRead(keyword.INTEGER, "1339"),
			mustPeekAndRead(keyword.STRING, "foo"), mustPeekAndRead(keyword.STRING, "bar"), mustPeekAndRead(keyword.STRING, "foo bar"),
			mustPeekAndRead(keyword.STRING, "foo\nbar"),
			mustPeekAndRead(keyword.STRING, "foo\nbar\nbaz\n"),
			mustPeekAndRead(keyword.FLOAT, "13.37"),
		)
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

	lexer := NewLexer()
	if err := lexer.SetInput([]byte(introspectionQuery)); err != nil {
		t.Fatal(err)
	}

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

		fixture, err := ioutil.ReadFile("./fixtures/introspection_lexed.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("introspection_lexed", fixture, data)
	}
}

func BenchmarkLexer(b *testing.B) {

	lexer := NewLexer()
	inputBytes := []byte(introspectionQuery)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {

		if err := lexer.SetInput(inputBytes); err != nil {
			b.Fatal(err)
		}

		var key keyword.Keyword

		for key != keyword.EOF {
			key = lexer.Peek(true)

			tok := lexer.Read()
			_ = tok
		}
	}
}
