package lexer

import (
	"encoding/json"
	"fmt"
	"github.com/jensneuse/diffview"
	. "github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
	"github.com/sebdah/goldie"
	"io/ioutil"
	"testing"
)

func TestLexer_Peek_Read(t *testing.T) {

	type checkFunc func(lex *Lexer, i int)

	run := func(input string, checks ...checkFunc) {
		lex := NewLexer()
		lex.SetInput(input)
		for i := range checks {
			checks[i](lex, i+1)
		}
	}

	mustPeek := func(k Keyword) checkFunc {
		return func(lex *Lexer, i int) {
			peeked, err := lex.Peek(true)
			if err != nil {
				panic(err)
			}
			if peeked != k {
				panic(fmt.Errorf("mustPeek: want: %s, got: %s [check: %d]", k.String(), peeked.String(), i))
			}
		}
	}

	mustRead := func(k Keyword, literal string) checkFunc {
		return func(lex *Lexer, i int) {
			tok, err := lex.Read()
			if err != nil {
				panic(err)
			}
			if k != tok.Keyword {
				panic(fmt.Errorf("mustRead: want(keyword): %s, got: %s [check: %d]", k.String(), tok.String(), i))
			}
			if literal != tok.Literal {
				panic(fmt.Errorf("mustRead: want(literal): %s, got: %s [check: %d]", literal, tok.Literal, i))
			}
		}
	}

	mustPeekAndRead := func(k Keyword, literal string) checkFunc {
		return func(lex *Lexer, i int) {
			mustPeek(k)(lex, i)
			mustRead(k, literal)(lex, i)
		}
	}

	mustErrRead := func() checkFunc {
		return func(lex *Lexer, i int) {
			_, err := lex.Read()
			if err == nil {
				panic(fmt.Errorf("mustErrRead: want error, got nil [check: %d]", i))
			}
		}
	}

	resetInput := func(input string) checkFunc {
		return func(lex *Lexer, i int) {
			lex.SetInput(input)
		}
	}

	mustReadPosition := func(line, char int) checkFunc {
		return func(lex *Lexer, i int) {
			tok, err := lex.Read()
			if err != nil {
				panic(err)
			}

			if line != tok.Position.Line {
				panic(fmt.Errorf("mustReadPosition: want(line): %d, got: %d [check: %d]", line, tok.Position.Line, i))
			}
			if char != tok.Position.Char {
				panic(fmt.Errorf("mustReadPosition: want(char): %d, got: %d [check: %d]", char, tok.Position.Char, i))
			}
		}
	}

	t.Run("read correct when resetting input", func(t *testing.T) {
		run("x",
			mustRead(IDENT, "x"),
			resetInput("y"),
			mustRead(IDENT, "y"),
		)
	})
	t.Run("read eof multiple times", func(t *testing.T) {
		run("x",
			mustRead(IDENT, "x"),
			mustRead(EOF, "eof"),
			mustRead(EOF, "eof"),
		)
	})
	t.Run("read integer", func(t *testing.T) {
		run("1337", mustPeekAndRead(INTEGER, "1337"))
	})
	t.Run("read integer with comma", func(t *testing.T) {
		run("1337,", mustPeekAndRead(INTEGER, "1337"))
	})
	t.Run("read float", func(t *testing.T) {
		run("13.37", mustPeekAndRead(FLOAT, "13.37"))
	})
	t.Run("read float with space", func(t *testing.T) {
		run("13.37 ", mustPeekAndRead(FLOAT, "13.37"))
	})
	t.Run("read float with tab", func(t *testing.T) {
		run("13.37	", mustPeekAndRead(FLOAT, "13.37"))
	})
	t.Run("read with with lineTerminator", func(t *testing.T) {
		run("13.37\n", mustPeekAndRead(FLOAT, "13.37"))
	})
	t.Run("read float with comma", func(t *testing.T) {
		run("13.37,", mustPeekAndRead(FLOAT, "13.37"))
	})
	t.Run("fail reading incomplete float", func(t *testing.T) {
		run("13.", mustErrRead())
	})
	t.Run("read single line string", func(t *testing.T) {
		run("\"foo\"", mustPeekAndRead(STRING, "foo"))
	})
	t.Run("read single line string with escaped quote", func(t *testing.T) {
		run("\"foo \\\" bar\"", mustPeekAndRead(STRING, "foo \\\" bar"))
	})
	t.Run("read multi line string with escaped quote", func(t *testing.T) {
		run("\"\"\"foo \\\" bar\"\"\"", mustPeekAndRead(STRING, "foo \\\" bar"))
	})
	t.Run("read multi line string", func(t *testing.T) {
		run("\"\"\"\nfoo\nbar\"\"\"", mustPeekAndRead(STRING, "foo\nbar"))
	})
	t.Run("read pipe", func(t *testing.T) {
		run("|", mustPeekAndRead(PIPE, "|"))
	})
	t.Run("err reading dot", func(t *testing.T) {
		run(".", mustErrRead())
	})
	t.Run("read fragment spread", func(t *testing.T) {
		run("...", mustPeekAndRead(SPREAD, "..."))
	})
	t.Run("read variable", func(t *testing.T) {
		run("$123", mustPeekAndRead(VARIABLE, "123"))
	})
	t.Run("read variable 2", func(t *testing.T) {
		run("$foo", mustPeekAndRead(VARIABLE, "foo"))
	})
	t.Run("read variable 3", func(t *testing.T) {
		run("$_foo", mustPeekAndRead(VARIABLE, "_foo"))
	})
	t.Run("read variable 3", func(t *testing.T) {
		run("$123", mustPeekAndRead(VARIABLE, "123"))
	})
	t.Run("read variable 4", func(t *testing.T) {
		run("$foo\n", mustPeekAndRead(VARIABLE, "foo"))
	})
	t.Run("read err invalid variable", func(t *testing.T) {
		run("$ foo", mustErrRead())
	})
	t.Run("read @", func(t *testing.T) {
		run("@", mustPeekAndRead(AT, "@"))
	})
	t.Run("read equals", func(t *testing.T) {
		run("=", mustPeekAndRead(EQUALS, "="))
	})
	t.Run("read variable colon", func(t *testing.T) {
		run(":", mustPeekAndRead(COLON, ":"))
	})
	t.Run("read bang", func(t *testing.T) {
		run("!", mustPeekAndRead(BANG, "!"))
	})
	t.Run("read bracket open", func(t *testing.T) {
		run("(", mustPeekAndRead(BRACKETOPEN, "("))
	})
	t.Run("read bracket close", func(t *testing.T) {
		run(")", mustPeekAndRead(BRACKETCLOSE, ")"))
	})
	t.Run("read squared bracket open", func(t *testing.T) {
		run("[", mustPeekAndRead(SQUAREBRACKETOPEN, "["))
	})
	t.Run("read squared bracket close", func(t *testing.T) {
		run("]", mustPeekAndRead(SQUAREBRACKETCLOSE, "]"))
	})
	t.Run("read curly bracket open", func(t *testing.T) {
		run("{", mustPeekAndRead(CURLYBRACKETOPEN, "{"))
	})
	t.Run("read curly bracket close", func(t *testing.T) {
		run("}", mustPeekAndRead(CURLYBRACKETCLOSE, "}"))
	})
	t.Run("read and", func(t *testing.T) {
		run("&", mustPeekAndRead(AND, "&"))
	})
	t.Run("read EOF", func(t *testing.T) {
		run("", mustPeekAndRead(EOF, "eof"))
	})
	t.Run("read ident", func(t *testing.T) {
		run("foo", mustPeekAndRead(IDENT, "foo"))
	})
	t.Run("read ident with colon", func(t *testing.T) {
		run("foo:", mustPeekAndRead(IDENT, "foo"))
	})
	t.Run("read true", func(t *testing.T) {
		run("true", mustPeekAndRead(TRUE, "true"))
	})
	t.Run("read true with space", func(t *testing.T) {
		run(" true ", mustPeekAndRead(TRUE, "true"))
	})
	t.Run("read false", func(t *testing.T) {
		run("false", mustPeekAndRead(FALSE, "false"))
	})
	t.Run("read query", func(t *testing.T) {
		run("query", mustPeekAndRead(QUERY, "query"))
	})
	t.Run("read mutation", func(t *testing.T) {
		run("mutation", mustPeekAndRead(MUTATION, "mutation"))
	})
	t.Run("read subscription", func(t *testing.T) {
		run("subscription", mustPeekAndRead(SUBSCRIPTION, "subscription"))
	})
	t.Run("read fragment", func(t *testing.T) {
		run("fragment", mustPeekAndRead(FRAGMENT, "fragment"))
	})
	t.Run("read implements", func(t *testing.T) {
		run("implements", mustPeekAndRead(IMPLEMENTS, "implements"))
	})
	t.Run("read schema", func(t *testing.T) {
		run("schema", mustPeekAndRead(SCHEMA, "schema"))
	})
	t.Run("read scalar", func(t *testing.T) {
		run("scalar", mustPeekAndRead(SCALAR, "scalar"))
	})
	t.Run("read type", func(t *testing.T) {
		run("type", mustPeekAndRead(TYPE, "type"))
	})
	t.Run("read interface", func(t *testing.T) {
		run("interface", mustPeekAndRead(INTERFACE, "interface"))
	})
	t.Run("read union", func(t *testing.T) {
		run("union", mustPeekAndRead(UNION, "union"))
	})
	t.Run("read enum", func(t *testing.T) {
		run("enum", mustPeekAndRead(ENUM, "enum"))
	})
	t.Run("read input", func(t *testing.T) {
		run("input", mustPeekAndRead(INPUT, "input"))
	})
	t.Run("read directive", func(t *testing.T) {
		run("directive", mustPeekAndRead(DIRECTIVE, "directive"))
	})
	t.Run("read inputValue", func(t *testing.T) {
		run("inputValue", mustPeekAndRead(IDENT, "inputValue"))
	})
	t.Run("read on", func(t *testing.T) {
		run("on", mustPeekAndRead(ON, "on"))
	})
	t.Run("read on with whitespace", func(t *testing.T) {
		run("on ", mustPeekAndRead(ON, "on"))
	})
	t.Run("read ignore comma", func(t *testing.T) {
		run(",", mustPeekAndRead(EOF, "eof"))
	})
	t.Run("read ignore space", func(t *testing.T) {
		run(" ", mustPeekAndRead(EOF, "eof"))
	})
	t.Run("read ignore tab", func(t *testing.T) {
		run("	", mustPeekAndRead(EOF, "eof"))
	})
	t.Run("read ignore lineTerminator", func(t *testing.T) {
		run("\n", mustPeekAndRead(EOF, "eof"))
	})
	t.Run("read null", func(t *testing.T) {
		run("null", mustPeekAndRead(NULL, "null"))
	})
	t.Run("multi read 'foo:'", func(t *testing.T) {
		run("foo:",
			mustPeekAndRead(IDENT, "foo"),
			mustPeekAndRead(COLON, ":"),
		)
	})
	t.Run("multi read '1,2,3'", func(t *testing.T) {
		run("1,2,3",
			mustPeekAndRead(INTEGER, "1"),
			mustPeekAndRead(INTEGER, "2"),
			mustPeekAndRead(INTEGER, "3"),
		)
	})
	t.Run("multi read positions", func(t *testing.T) {
		run(`foo bar baz
bal
 bas """x"""`,
			mustReadPosition(1, 1),
			mustReadPosition(1, 5),
			mustReadPosition(1, 9),
			mustReadPosition(2, 1),
			mustReadPosition(3, 2),
			mustReadPosition(3, 6),
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
			mustPeekAndRead(IDENT, "Goland"), mustPeekAndRead(CURLYBRACKETOPEN, "{"),
			mustPeekAndRead(SPREAD, "..."), mustPeekAndRead(ON, "on"), mustPeekAndRead(IDENT, "GoWater"), mustPeekAndRead(CURLYBRACKETOPEN, "{"),
			mustPeekAndRead(SPREAD, "..."), mustPeekAndRead(ON, "on"), mustPeekAndRead(IDENT, "GoAir"), mustPeekAndRead(CURLYBRACKETOPEN, "{"),
			mustPeekAndRead(IDENT, "go"),
			mustPeekAndRead(CURLYBRACKETCLOSE, "}"),
			mustPeekAndRead(CURLYBRACKETCLOSE, "}"),
			mustPeekAndRead(CURLYBRACKETCLOSE, "}"),
		)
	})
	t.Run("multi read many idents and strings", func(t *testing.T) {
		run(`1337 1338 1339 "foo" "bar" """foo bar""" """foo
bar""" """foo
bar
baz
"""
13.37`,
			mustPeekAndRead(INTEGER, "1337"), mustPeekAndRead(INTEGER, "1338"), mustPeekAndRead(INTEGER, "1339"),
			mustPeekAndRead(STRING, "foo"), mustPeekAndRead(STRING, "bar"), mustPeekAndRead(STRING, "foo bar"),
			mustPeekAndRead(STRING, "foo\nbar"),
			mustPeekAndRead(STRING, "foo\nbar\nbaz"),
			mustPeekAndRead(FLOAT, "13.37"),
		)
	})
}

func TestLexerRegressions(t *testing.T) {

	lexer := NewLexer()
	lexer.SetInput(introspectionQuery)

	var total []token.Token
	for {
		tok, err := lexer.Read()
		if err != nil {
			t.Fatal(err)
		}
		if tok.Keyword == EOF {
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
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {

		b.StopTimer()
		lexer.SetInput(introspectionQuery)
		b.StartTimer()

		var key Keyword
		var err error

		for key != EOF {
			key, err = lexer.Peek(true)
			if err != nil {
				b.Fatal(err)
			}

			tok, err := lexer.Read()
			if err != nil {
				b.Fatal(err)
			}

			_ = tok
		}
	}
}
