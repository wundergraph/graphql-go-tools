package lexer

import (
	"bytes"
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/runestringer"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestLexer(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("lexer", func() {

		g.It("should parse int32(1337)", func() {
			lexer := NewLexer(runestringer.NewBuffered())
			lexer.SetInput(bytes.NewReader(keyword.Literal("1337")))

			tok, err := lexer.Read()
			Expect(err).To(BeNil())

			Expect(tok.Keyword).To(Equal(keyword.INTEGER))
			Expect(tok.Literal).To(Equal(keyword.Literal("1337")))
		})

		g.It("should parse float32(13.37)", func() {
			lexer := NewLexer(runestringer.NewBuffered())
			lexer.SetInput(bytes.NewReader(keyword.Literal("13.37")))

			tok, err := lexer.Read()
			Expect(err).To(BeNil())

			Expect(tok.Keyword).To(Equal(keyword.FLOAT))
			Expect(tok.Literal).To(Equal(keyword.Literal("13.37")))
		})

		g.It("should not parse 13.37.1337", func() {
			lexer := NewLexer(runestringer.NewBuffered())
			lexer.SetInput(bytes.NewReader(keyword.Literal("13.37.1337")))

			tok, err := lexer.Read()
			Expect(err).NotTo(BeNil())
			Expect(tok.Keyword).To(Equal(keyword.UNDEFINED))
		})

		g.It("should allow un-reading four values (and more)", func() {
			lexer := NewLexer(runestringer.NewBuffered())
			lexer.SetInput(bytes.NewReader(keyword.Literal(`""""
foo`)))

			// read all values
			for i := 0; i < 4; i++ {
				run := lexer.readRune()
				Expect(run.rune).To(Equal('"'))
			}

			// unread all values
			for i := 0; i < 4; i++ {
				Expect(lexer.unread()).To(BeNil())
			}

			// read all values again
			for i := 0; i < 4; i++ {
				run := lexer.readRune()
				Expect(run.rune).To(Equal('"'))
			}

			run := lexer.readRune()
			Expect(run.rune).To(Equal('\n'))

			run = lexer.readRune()
			Expect(run.rune).To(Equal('f'))

			run = lexer.readRune()
			Expect(run.rune).To(Equal('o'))

			run = lexer.readRune()
			Expect(run.rune).To(Equal('o'))

			for i := 0; i < 8; i++ {
				Expect(lexer.unread()).To(BeNil())
			}

			// read all values again
			for i := 0; i < 4; i++ {
				run := lexer.readRune()
				Expect(run.rune).To(Equal('"'))
			}

			run = lexer.readRune()
			Expect(run.rune).To(Equal('\n'))

			run = lexer.readRune()
			Expect(run.rune).To(Equal('f'))

			run = lexer.readRune()
			Expect(run.rune).To(Equal('o'))

			run = lexer.readRune()
			Expect(run.rune).To(Equal('o'))

			// expect EOF
			run = lexer.readRune()
			Expect(run.rune).To(Equal(int32(0)))
		})

		g.It("should fail when un-reading too many runes", func() {

			lexer := NewLexer(runestringer.NewBuffered())
			lexer.SetInput(bytes.NewReader(keyword.Literal(`foo`)))

			for i := 0; i < 3; i++ {
				lexer.readRune()
			}

			for i := 0; i < 3; i++ {
				Expect(lexer.unread()).To(BeNil())
			}

			Expect(lexer.unread()).NotTo(BeNil())
		})

		g.It("should handle position tracking independent from the direction", func() {

			lexer := NewLexer(runestringer.NewBuffered())
			lexer.SetInput(bytes.NewReader(keyword.Literal(`foo
bar

baz`)))

			tok, err := lexer.Read()
			Expect(err).To(BeNil())
			Expect(string(tok.Literal)).To(Equal("foo"))
			Expect(tok.Position.Line).To(Equal(1))
			Expect(tok.Position.Char).To(Equal(1))

			tok, err = lexer.Read()
			Expect(err).To(BeNil())
			Expect(string(tok.Literal)).To(Equal("bar"))
			Expect(tok.Position.Line).To(Equal(2))
			Expect(tok.Position.Char).To(Equal(1))

			tok, err = lexer.Read()
			Expect(err).To(BeNil())
			Expect(string(tok.Literal)).To(Equal("baz"))
			Expect(tok.Position.Line).To(Equal(4))
			Expect(tok.Position.Char).To(Equal(1))

			tok, err = lexer.Read()
			Expect(err).To(BeNil())
			Expect(tok.Keyword).To(Equal(keyword.EOF))

			for i := 0; i < 13; i++ {
				lexer.unread()
			}

			tok, err = lexer.Read()
			Expect(err).To(BeNil())
			Expect(string(tok.Literal)).To(Equal("foo"))
			Expect(tok.Position.Line).To(Equal(1))
			Expect(tok.Position.Char).To(Equal(1))

			tok, err = lexer.Read()
			Expect(err).To(BeNil())
			Expect(string(tok.Literal)).To(Equal("bar"))
			Expect(tok.Position.Line).To(Equal(2))
			Expect(tok.Position.Char).To(Equal(1))

			for i := 0; i < 3; i++ {
				lexer.unread()
			}

			tok, err = lexer.Read()
			Expect(err).To(BeNil())
			Expect(string(tok.Literal)).To(Equal("bar"))
			Expect(tok.Position.Line).To(Equal(2))
			Expect(tok.Position.Char).To(Equal(1))

			tok, err = lexer.Read()
			Expect(err).To(BeNil())
			Expect(string(tok.Literal)).To(Equal("baz"))
			Expect(tok.Position.Line).To(Equal(4))
			Expect(tok.Position.Char).To(Equal(1))

			tok, err = lexer.Read()
			Expect(err).To(BeNil())
			Expect(tok.Keyword).To(Equal(keyword.EOF))

			for i := 0; i < 4; i++ {
				lexer.unread()
			}

			run := lexer.readRune()
			Expect(run.rune).To(Equal('b'))
			Expect(run.position.Line).To(Equal(4))
			Expect(run.position.Char).To(Equal(1))

			run = lexer.readRune()
			Expect(run.rune).To(Equal('a'))
			Expect(run.position.Line).To(Equal(4))
			Expect(run.position.Char).To(Equal(2))

			run = lexer.readRune()
			Expect(run.rune).To(Equal('z'))
			Expect(run.position.Line).To(Equal(4))
			Expect(run.position.Char).To(Equal(3))

			tok, err = lexer.Read()
			Expect(err).To(BeNil())
			Expect(tok.Keyword).To(Equal(keyword.EOF))
		})
	})

	g.Describe("lexer.peekMatchRunes", func() {

		tests := []struct {
			it            string
			input         string
			match         rune
			amount        int
			expectErr     types.GomegaMatcher
			expectMatches types.GomegaMatcher
		}{
			{
				it:            "should match triple Quotes",
				input:         `"""`,
				match:         '"',
				amount:        3,
				expectErr:     BeNil(),
				expectMatches: BeTrue(),
			},
			{
				it:            "should not match if last rune is unexpected",
				input:         `""x`,
				match:         '"',
				amount:        3,
				expectErr:     BeNil(),
				expectMatches: BeFalse(),
			},
			{
				it:            "should not match if second rune is unexpected",
				input:         `"x"`,
				match:         '"',
				amount:        3,
				expectErr:     BeNil(),
				expectMatches: BeFalse(),
			},
			{
				it:            "should not match if first rune is unexpected",
				input:         `x""`,
				match:         '"',
				amount:        3,
				expectErr:     BeNil(),
				expectMatches: BeFalse(),
			},
			{
				it:            "should not match if reading more runes than available",
				input:         `"""`,
				match:         '"',
				amount:        4,
				expectErr:     BeNil(),
				expectMatches: BeFalse(),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {
				lexer := NewLexer(runestringer.NewBuffered())
				lexer.SetInput(bytes.NewReader(keyword.Literal(test.input)))

				matches, err := lexer.peekMatchRunes(test.match, test.amount)
				Expect(err).To(test.expectErr)
				Expect(matches).To(test.expectMatches)
			})
		}
	})

	g.Describe("lexer.Read", func() {

		tests := []struct {
			it            string
			input         string
			expectErr     types.GomegaMatcher
			expectKeyword types.GomegaMatcher
			expectLiteral types.GomegaMatcher
		}{
			{
				it:            "should lex single line string",
				input:         `" foo bar "`,
				expectErr:     BeNil(),
				expectKeyword: Equal(keyword.STRING),
				expectLiteral: Equal(keyword.Literal("foo bar")),
			},
			{
				it: "should fail when single line string contains LINETERMINATOR",
				input: `"foo bar
"`,
				expectErr: Not(BeNil()),
			},
			{
				it:        "should fail when single line string contains EOF",
				input:     `"foo bar`,
				expectErr: Not(BeNil()),
			},
			{
				it: "should lex block string",
				input: `""" foo "" bar
"""`,
				expectErr:     BeNil(),
				expectKeyword: Equal(keyword.STRING),
				expectLiteral: Equal(keyword.Literal(`foo "" bar`)),
			},
			{
				it:        "should fail when block string contains EOF",
				input:     `"""foo "" bar`,
				expectErr: Not(BeNil()),
			},
			{
				it:            "should lex null",
				input:         "null",
				expectErr:     BeNil(),
				expectKeyword: Equal(keyword.NULL),
			},
			{
				it:            "should lex comment",
				input:         "# foo bar",
				expectErr:     BeNil(),
				expectKeyword: Equal(keyword.COMMENT),
				expectLiteral: Equal(keyword.Literal(`foo bar`)),
			},
			{
				it: "should dismiss all whitespace (LINETERMINATOR,TAB,SPACE,COMMA) before a keyword",
				input: `
	 , foo`,
				expectErr:     BeNil(),
				expectKeyword: Equal(keyword.IDENT),
				expectLiteral: Equal(keyword.Literal(`foo`)),
			},
			{
				it:            "should scan valid variable",
				input:         `$foo `,
				expectErr:     BeNil(),
				expectKeyword: Equal(keyword.VARIABLE),
				expectLiteral: Equal(keyword.Literal(`foo`)),
			},
			{
				it:            "should scan valid variable with digits",
				input:         `$123Foo `,
				expectErr:     BeNil(),
				expectKeyword: Equal(keyword.VARIABLE),
				expectLiteral: Equal(keyword.Literal(`123Foo`)),
			},
			{
				it:            "should scan valid variable with underscore and digits",
				input:         `$_123Foo `,
				expectErr:     BeNil(),
				expectKeyword: Equal(keyword.VARIABLE),
				expectLiteral: Equal(keyword.Literal(`_123Foo`)),
			},
			{
				it:            "should fail scanning variable with space after $",
				input:         `$ foo `,
				expectErr:     Not(BeNil()),
				expectKeyword: Equal(keyword.VARIABLE),
			},
			{
				it: "should fail scaning variable with tab after $",
				input: `$	foo `,
				expectErr:     Not(BeNil()),
				expectKeyword: Equal(keyword.VARIABLE),
			},
			{
				it: "should fail scaning variable with lineTerminator after $",
				input: `$
foo `,
				expectErr:     Not(BeNil()),
				expectKeyword: Equal(keyword.VARIABLE),
			},
			{
				it:            "should scan SPREAD",
				input:         `... `,
				expectErr:     BeNil(),
				expectKeyword: Equal(keyword.SPREAD),
				expectLiteral: Equal(keyword.Literal(`...`)),
			},
			{
				it:            "should throw error on '..'",
				input:         `..`,
				expectErr:     Not(BeNil()),
				expectKeyword: Equal(keyword.DOT),
				expectLiteral: Equal(keyword.Literal(`.`)),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {
				lexer := NewLexer(runestringer.NewBuffered())
				lexer.SetInput(bytes.NewReader(keyword.Literal(test.input)))

				tok, err := lexer.Read()
				Expect(err).To(test.expectErr)
				if test.expectKeyword != nil {
					Expect(tok.Keyword).To(test.expectKeyword)
				}
				if test.expectLiteral != nil {
					Expect(tok.Literal).To(test.expectLiteral)
				}
			})
		}
	})

	g.Describe("l.scanVariable", func() {
		g.It("should parse int32(1337)", func() {
			lexer := NewLexer(runestringer.NewBuffered())
			lexer.SetInput(bytes.NewReader(keyword.Literal("$foo:")))

			tok, err := lexer.Read()
			Expect(err).To(BeNil())

			Expect(tok.Keyword).To(Equal(keyword.VARIABLE))
			Expect(tok.Literal).To(Equal(keyword.Literal("foo")))

			tok, err = lexer.Read()
			Expect(err).To(BeNil())

			Expect(tok.Keyword).To(Equal(keyword.COLON))
			Expect(tok.Literal).To(Equal(keyword.Literal(":")))
		})
	})
}

var introspectionQuery = []byte(`query IntrospectionQuery {
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
}`)

func BenchmarkLexer_Buffered(b *testing.B) {

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {

		b.StopTimer()
		reader := bytes.NewReader(introspectionQuery)
		stringer := runestringer.NewBuffered()
		lex := NewLexer(stringer)
		lex.SetInput(reader)
		b.StartTimer()

		for {
			tok, _ := lex.Read()
			if tok.Keyword == keyword.EOF {
				break
			}
		}
	}
}
