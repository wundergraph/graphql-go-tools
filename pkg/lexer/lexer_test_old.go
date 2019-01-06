package lexer

// nolint
import (
	"encoding/json"
	"fmt"
	"github.com/jensneuse/diffview"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/sebdah/goldie"
	"io/ioutil"
	"testing"
)

// nolint
func TestLexer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Lexer")
}

// nolint
func _TestLexerRegressions(t *testing.T) {

	lexer := NewLexer()
	lexer.SetInput(introspectionQuery)

	var total []token.Token
	for {
		tok, err := lexer.Read()
		if err != nil {
			t.Fatal(err)
		}
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

var _ = Describe("Lexer.Read", func() {
	It("should read correctly from reader when re-setting input", func() {
		lexer := NewLexer()
		lexer.SetInput("x")
		_, err := lexer.Read()
		Expect(err).NotTo(HaveOccurred())

		lexer.SetInput("x")
		x, err := lexer.Read()
		Expect(err).NotTo(HaveOccurred())
		Expect(x).To(Equal(token.Token{
			Keyword: keyword.IDENT,
			Literal: "x",
			Position: position.Position{
				Line: 1,
				Char: 1,
			},
		}))
	})
	It("should read eof multiple times correctly", func() {
		lexer := NewLexer()
		lexer.SetInput("x")

		x, err := lexer.Read()
		Expect(err).NotTo(HaveOccurred())
		Expect(x).To(Equal(token.Token{
			Keyword: keyword.IDENT,
			Literal: "x",
			Position: position.Position{
				Line: 1,
				Char: 1,
			},
		}))

		eof1, err := lexer.Read()
		Expect(err).NotTo(HaveOccurred())
		Expect(eof1).To(Equal(token.Token{
			Keyword: keyword.EOF,
			Literal: "eof",
			Position: position.Position{
				Line: 1,
				Char: 2,
			},
		}))
	})
})

var _ = Describe("Lexer.Read", func() {

	type Case struct {
		in        string
		out       token.Token
		expectErr types.GomegaMatcher
	}

	var lexer *Lexer

	BeforeEach(func() {
		lexer = NewLexer()
	})

	DescribeTable("Read Single Token", func(c Case) {

		lexer.SetInput(c.in)
		tok, err := lexer.Read()
		if c.expectErr != nil {
			Expect(err).To(c.expectErr)
		} else {
			Expect(err).To(BeNil())
		}
		Expect(tok).To(Equal(c.out))

	},
		Entry("should read integer", Case{
			in: "1337",
			out: token.Token{
				Keyword: keyword.INTEGER,
				Literal: "1337",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read integer with comma at the end", Case{
			in: "1337,",
			out: token.Token{
				Keyword: keyword.INTEGER,
				Literal: "1337",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read float", Case{
			in: "13.37",
			out: token.Token{
				Keyword: keyword.FLOAT,
				Literal: "13.37",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should fail reading incomplete float", Case{
			in:        "13.",
			expectErr: HaveOccurred(),
			out: token.Token{
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read single line string", Case{
			in: `"foo bar"`,
			out: token.Token{
				Keyword: keyword.STRING,
				Literal: `foo bar`,
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read single line string with escaped quote", Case{
			in: "\"foo bar \\\" baz\"",
			out: token.Token{
				Keyword: keyword.STRING,
				Literal: "foo bar \\\" baz",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read multi line string with escaped quote", Case{
			in: "\"\"\"foo bar \\\"\\\"\\\" baz\"\"\"",
			out: token.Token{
				Keyword: keyword.STRING,
				Literal: "foo bar \\\"\\\"\\\" baz",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read multi single line string", Case{
			in: `"""
foo
bar"""`,
			out: token.Token{
				Keyword: keyword.STRING,
				Literal: `foo
bar`,
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read multi single line string with correct whitespace trimming", Case{
			in: `"""
foo
"""`,
			out: token.Token{
				Keyword: keyword.STRING,
				Literal: `foo`,
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read pipe", Case{
			in: "|",
			out: token.Token{
				Keyword: keyword.PIPE,
				Literal: literal.PIPE,
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should not read dot", Case{
			in: ".",
			out: token.Token{
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
			expectErr: HaveOccurred(),
		}),
		Entry("should read spread (...)", Case{
			in: "...",
			out: token.Token{
				Keyword: keyword.SPREAD,
				Literal: literal.SPREAD,
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read $123", Case{
			in: "$123",
			out: token.Token{
				Keyword: keyword.VARIABLE,
				Literal: "123",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read $foo", Case{
			in: "$foo",
			out: token.Token{
				Keyword: keyword.VARIABLE,
				Literal: "foo",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read $_foo", Case{
			in: "$_foo",
			out: token.Token{
				Keyword: keyword.VARIABLE,
				Literal: "_foo",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read $123 ", Case{
			in: "$123 ",
			out: token.Token{
				Keyword: keyword.VARIABLE,
				Literal: "123",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read $123\n", Case{
			in: "$123\n",
			out: token.Token{
				Keyword: keyword.VARIABLE,
				Literal: "123",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read @", Case{
			in: "@",
			out: token.Token{
				Keyword: keyword.AT,
				Literal: "@",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read =", Case{
			in: "=",
			out: token.Token{
				Keyword: keyword.EQUALS,
				Literal: "=",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read :", Case{
			in: ":",
			out: token.Token{
				Keyword: keyword.COLON,
				Literal: ":",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read !", Case{
			in: "!",
			out: token.Token{
				Keyword: keyword.BANG,
				Literal: "!",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read (", Case{
			in: "(",
			out: token.Token{
				Keyword: keyword.BRACKETOPEN,
				Literal: "(",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read )", Case{
			in: ")",
			out: token.Token{
				Keyword: keyword.BRACKETCLOSE,
				Literal: ")",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read {", Case{
			in: "{",
			out: token.Token{
				Keyword: keyword.CURLYBRACKETOPEN,
				Literal: "{",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read }", Case{
			in: "}",
			out: token.Token{
				Keyword: keyword.CURLYBRACKETCLOSE,
				Literal: "}",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read [", Case{
			in: "[",
			out: token.Token{
				Keyword: keyword.SQUAREBRACKETOPEN,
				Literal: "[",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read ]", Case{
			in: "]",
			out: token.Token{
				Keyword: keyword.SQUAREBRACKETCLOSE,
				Literal: "]",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read &", Case{
			in: "&",
			out: token.Token{
				Keyword: keyword.AND,
				Literal: "&",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read EOF", Case{
			in: "",
			out: token.Token{
				Keyword: keyword.EOF,
				Literal: literal.EOF,
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read ident 'foo'", Case{
			in: "foo",
			out: token.Token{
				Keyword: keyword.IDENT,
				Literal: "foo",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read ident 'foo' from 'foo:'", Case{
			in: "foo:",
			out: token.Token{
				Keyword: keyword.IDENT,
				Literal: "foo",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read ident true", Case{
			in: "true",
			out: token.Token{
				Keyword: keyword.TRUE,
				Literal: "true",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read ident false", Case{
			in: "false",
			out: token.Token{
				Keyword: keyword.FALSE,
				Literal: "false",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
	)
})

var _ = Describe("Lexer.Peek()", func() {
	type Case struct {
		input              string
		expectErr          types.GomegaMatcher
		expectKey          types.GomegaMatcher
		expectNextToken    types.GomegaMatcher
		expectNextTokenErr types.GomegaMatcher
	}

	var lexer *Lexer

	BeforeEach(func() {
		lexer = NewLexer()
	})

	DescribeTable("Peek Tests", func(c Case) {
		lexer.SetInput(c.input)
		key, err := lexer.Peek(true)
		if c.expectErr != nil {
			Expect(err).To(c.expectErr)
		}
		if c.expectKey != nil {
			Expect(key).To(c.expectKey)
		}
		if c.expectNextToken != nil {
			tok, err := lexer.Read()
			if c.expectNextTokenErr != nil {
				Expect(err).To(c.expectNextTokenErr)
			}

			Expect(tok).To(c.expectNextToken)
		}
	},
		Entry("should peek EOF", Case{
			input:              "",
			expectKey:          Equal(keyword.EOF),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.EOF,
				Literal: "eof",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek query", Case{
			input:              "query ",
			expectKey:          Equal(keyword.QUERY),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.QUERY,
				Literal: "query",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek mutation", Case{
			input:              "mutation",
			expectKey:          Equal(keyword.MUTATION),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.MUTATION,
				Literal: "mutation",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek subscription", Case{
			input:              "subscription",
			expectKey:          Equal(keyword.SUBSCRIPTION),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.SUBSCRIPTION,
				Literal: "subscription",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek fragment", Case{
			input:              "fragment",
			expectKey:          Equal(keyword.FRAGMENT),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.FRAGMENT,
				Literal: "fragment",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek spread (...)", Case{
			input:              "...",
			expectKey:          Equal(keyword.SPREAD),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.SPREAD,
				Literal: "...",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'implements'", Case{
			input:              "implements",
			expectKey:          Equal(keyword.IMPLEMENTS),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.IMPLEMENTS,
				Literal: "implements",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'schema'", Case{
			input:              "schema",
			expectKey:          Equal(keyword.SCHEMA),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.SCHEMA,
				Literal: "schema",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'scalar'", Case{
			input:              "scalar",
			expectKey:          Equal(keyword.SCALAR),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.SCALAR,
				Literal: "scalar",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'type'", Case{
			input:              "type",
			expectKey:          Equal(keyword.TYPE),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.TYPE,
				Literal: "type",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'interface'", Case{
			input:              "interface",
			expectKey:          Equal(keyword.INTERFACE),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.INTERFACE,
				Literal: "interface",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'union'", Case{
			input:              "union",
			expectKey:          Equal(keyword.UNION),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.UNION,
				Literal: "union",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'enum'", Case{
			input:              "enum",
			expectKey:          Equal(keyword.ENUM),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.ENUM,
				Literal: "enum",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'input'", Case{
			input:              "input",
			expectKey:          Equal(keyword.INPUT),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.INPUT,
				Literal: "input",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'directive'", Case{
			input:              "directive",
			expectKey:          Equal(keyword.DIRECTIVE),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.DIRECTIVE,
				Literal: "directive",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'inputValue' as ident", Case{
			input:              "inputValue",
			expectKey:          Equal(keyword.IDENT),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.IDENT,
				Literal: "inputValue",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek ON", Case{
			input:              "on",
			expectKey:          Equal(keyword.ON),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.ON,
				Literal: "on",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek ON with whitespace behind", Case{
			input:              "on ",
			expectKey:          Equal(keyword.ON),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.ON,
				Literal: "on",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek ignore comma", Case{
			input:     ",",
			expectKey: Equal(keyword.EOF),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.EOF,
				Literal: "eof",
				Position: position.Position{
					Line: 1,
					Char: 2,
				},
			}),
		}),
		Entry("should peek '$color:' as variable color", Case{
			input:     "$color:",
			expectKey: Equal(keyword.VARIABLE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.VARIABLE,
				Literal: "color",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek '$ color:' as invalid", Case{
			input:              "$ color:",
			expectErr:          BeNil(),
			expectNextTokenErr: HaveOccurred(),
		}),
		Entry("should peek ignore space", Case{
			input:     " ",
			expectKey: Equal(keyword.EOF),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.EOF,
				Literal: "eof",
				Position: position.Position{
					Line: 1,
					Char: 2,
				},
			}),
		}),
		Entry("should peek ignore tab", Case{
			input: "	",
			expectKey: Equal(keyword.EOF),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.EOF,
				Literal: "eof",
				Position: position.Position{
					Line: 1,
					Char: 2,
				},
			}),
		}),
		Entry("should peek ignore line terminator", Case{
			input:     "\n",
			expectKey: Equal(keyword.EOF),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.EOF,
				Literal: "eof",
				Position: position.Position{
					Line: 2,
					Char: 1,
				},
			}),
		}),
		Entry("should peek single line string", Case{
			input:     `"foo"`,
			expectKey: Equal(keyword.STRING),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.STRING,
				Literal: "foo",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek multi line string", Case{
			input:     `"""foo"""`,
			expectKey: Equal(keyword.STRING),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.STRING,
				Literal: "foo",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek variable", Case{
			input:     "$foo",
			expectKey: Equal(keyword.VARIABLE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.VARIABLE,
				Literal: "foo",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should throw error when reading invalid variable", Case{
			input:              "$ foo",
			expectNextTokenErr: HaveOccurred(),
		}),
		Entry("should peek pipe", Case{
			input:     "|",
			expectKey: Equal(keyword.PIPE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.PIPE,
				Literal: "|",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek equals", Case{
			input:     "=",
			expectKey: Equal(keyword.EQUALS),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.EQUALS,
				Literal: "=",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek at", Case{
			input:     "@",
			expectKey: Equal(keyword.AT),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.AT,
				Literal: "@",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek null", Case{
			input:     "null",
			expectKey: Equal(keyword.NULL),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.NULL,
				Literal: "null",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek colon", Case{
			input:     ":",
			expectKey: Equal(keyword.COLON),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.COLON,
				Literal: ":",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek bang", Case{
			input:     "!",
			expectKey: Equal(keyword.BANG),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.BANG,
				Literal: "!",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek bracket open", Case{
			input:     "(",
			expectKey: Equal(keyword.BRACKETOPEN),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.BRACKETOPEN,
				Literal: "(",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek bracket close", Case{
			input:     ")",
			expectKey: Equal(keyword.BRACKETCLOSE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.BRACKETCLOSE,
				Literal: ")",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek squared bracket open", Case{
			input:     "[",
			expectKey: Equal(keyword.SQUAREBRACKETOPEN),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.SQUAREBRACKETOPEN,
				Literal: "[",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek squared bracket close", Case{
			input:     "]",
			expectKey: Equal(keyword.SQUAREBRACKETCLOSE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.SQUAREBRACKETCLOSE,
				Literal: "]",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek curly bracket open", Case{
			input:     "{",
			expectKey: Equal(keyword.CURLYBRACKETOPEN),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.CURLYBRACKETOPEN,
				Literal: "{",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek curly bracket close", Case{
			input:     "}",
			expectKey: Equal(keyword.CURLYBRACKETCLOSE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.CURLYBRACKETCLOSE,
				Literal: "}",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek and", Case{
			input:     "&",
			expectKey: Equal(keyword.AND),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.AND,
				Literal: "&",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek ident", Case{
			input:     "foo",
			expectKey: Equal(keyword.IDENT),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.IDENT,
				Literal: "foo",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek integer", Case{
			input:     "1337",
			expectKey: Equal(keyword.INTEGER),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.INTEGER,
				Literal: "1337",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek float", Case{
			input:     "13.37",
			expectKey: Equal(keyword.FLOAT),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.FLOAT,
				Literal: "13.37",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek true", Case{
			input:     "true ",
			expectKey: Equal(keyword.TRUE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.TRUE,
				Literal: "true",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek true with space in front", Case{
			input:     " true ",
			expectKey: Equal(keyword.TRUE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.TRUE,
				Literal: "true",
				Position: position.Position{
					Line: 1,
					Char: 2,
				},
			}),
		}),
		Entry("should peek true with multiple spaces in front", Case{
			input:     "   true",
			expectKey: Equal(keyword.TRUE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.TRUE,
				Literal: "true",
				Position: position.Position{
					Line: 1,
					Char: 4,
				},
			}),
		}),
		Entry("should peek false", Case{
			input:     "false ",
			expectKey: Equal(keyword.FALSE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.FALSE,
				Literal: "false",
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
	)
})

var _ = Describe("Lexer.peekIsFloat", func() {
	type Case struct {
		in      string
		isFloat bool
	}

	var lexer *Lexer

	BeforeEach(func() {
		lexer = NewLexer()
	})

	DescribeTable("peekIsFloat cases", func(c Case) {

		lexer.SetInput(c.in)
		actualIsFloat := lexer.peekIsFloat()
		Expect(actualIsFloat).To(Equal(c.isFloat))

	}, Entry("should identify 13.37 as float", Case{
		in:      "13.37",
		isFloat: true,
	}), Entry("should identify 13.37 as float (with space suffix)", Case{
		in:      "13.37 ",
		isFloat: true,
	}), Entry("should identify 13.37 as float (with tab suffix)", Case{
		in: "13.37	",
		isFloat: true,
	}), Entry("should identify 13.37 as float (with line terminator suffix)", Case{
		in:      "13.37\n",
		isFloat: true,
	}), Entry("should identify 13.37 as float (with comma suffix)", Case{
		in:      "13.37,",
		isFloat: true,
	}), Entry("should identify 1337 as non float", Case{
		in:      "1337",
		isFloat: false,
	}),
	)
})

// nolint
func BenchmarkPeekIsFloat(b *testing.B) {
	input := "13373737.37"
	lexer := NewLexer()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		lexer.SetInput(input)
		lexer.peekIsFloat()
	}
}

var _ = Describe("Lexer.readMultiLineString", func() {
	lexer := NewLexer()
	lexer.SetInput("\"\"\"foo\"\"\" x")

	It("should read foo", func() {
		foo, err := lexer.Read()
		Expect(err).To(BeNil())
		Expect(foo).To(Equal(token.Token{
			Literal: "foo",
			Keyword: keyword.STRING,
			Position: position.Position{
				Line: 1,
				Char: 1,
			},
		}))
	})

	It("should read x", func() {
		foo, err := lexer.Read()
		Expect(err).To(BeNil())
		Expect(foo).To(Equal(token.Token{
			Literal: "x",
			Keyword: keyword.IDENT,
			Position: position.Position{
				Line: 1,
				Char: 11,
			},
		}))
	})

	It("should read eof", func() {
		foo, err := lexer.Read()
		Expect(err).To(BeNil())
		Expect(foo).To(Equal(token.Token{
			Literal: "eof",
			Keyword: keyword.EOF,
			Position: position.Position{
				Line: 1,
				Char: 12,
			},
		}))
	})
})

var _ = Describe("Lexer.Read", func() {

	type Case struct {
		in  string
		out []token.Token
	}

	var lexer *Lexer

	BeforeEach(func() {
		lexer = NewLexer()
	})

	DescribeTable("Read Multiple Tokens", func(c Case) {

		lexer.SetInput(c.in)
		for i := 0; i < len(c.out); i++ {
			peeked, _ := lexer.Peek(true)
			Expect(peeked).To(Equal(c.out[i].Keyword), fmt.Sprintf("Token: %d", i+1))
			tok, err := lexer.Read()
			Expect(err).To(BeNil())
			Expect(tok).To(Equal(c.out[i]))
		}

	},
		Entry("should read ident followed by colon", Case{
			in: "foo:",
			out: []token.Token{
				{
					Keyword: keyword.IDENT,
					Literal: "foo",
					Position: position.Position{
						Line: 1,
						Char: 1,
					},
				},
				{
					Keyword: keyword.COLON,
					Literal: ":",
					Position: position.Position{
						Line: 1,
						Char: 4,
					},
				},
			},
		}),
		Entry("should read complex nested structure", Case{
			in: `Goland {
					... on GoWater {
						... on GoAir {
							go
						}
					}
				}
				`,
			out: []token.Token{
				{
					Keyword:  keyword.IDENT,
					Literal:  "Goland",
					Position: position.Position{1, 1},
				},
				{
					Keyword:  keyword.CURLYBRACKETOPEN,
					Literal:  "{",
					Position: position.Position{1, 8},
				},
				{
					Keyword:  keyword.SPREAD,
					Literal:  "...",
					Position: position.Position{2, 6},
				},
				{
					Keyword:  keyword.ON,
					Literal:  "on",
					Position: position.Position{2, 10},
				},
				{
					Keyword:  keyword.IDENT,
					Literal:  "GoWater",
					Position: position.Position{2, 13},
				},
				{
					Keyword:  keyword.CURLYBRACKETOPEN,
					Literal:  "{",
					Position: position.Position{2, 21},
				},
				{
					Keyword:  keyword.SPREAD,
					Literal:  "...",
					Position: position.Position{3, 7},
				},
				{
					Keyword:  keyword.ON,
					Literal:  "on",
					Position: position.Position{3, 11},
				},
				{
					Keyword:  keyword.IDENT,
					Literal:  "GoAir",
					Position: position.Position{3, 14},
				},
				{
					Keyword:  keyword.CURLYBRACKETOPEN,
					Literal:  "{",
					Position: position.Position{3, 20},
				},
				{
					Keyword:  keyword.IDENT,
					Literal:  "go",
					Position: position.Position{4, 8},
				},
				{
					Keyword:  keyword.CURLYBRACKETCLOSE,
					Literal:  "}",
					Position: position.Position{5, 7},
				},
				{
					Keyword:  keyword.CURLYBRACKETCLOSE,
					Literal:  "}",
					Position: position.Position{6, 6},
				},
				{
					Keyword:  keyword.CURLYBRACKETCLOSE,
					Literal:  "}",
					Position: position.Position{7, 5},
				},
				{
					Keyword:  keyword.EOF,
					Literal:  "eof",
					Position: position.Position{8, 5},
				},
			},
		}),
		Entry("should read multiple keywords", Case{
			in: `1337 1338 1339 "foo" "bar" """foo bar""" """foo
bar""" """foo
bar 
baz
"""
13.37`,
			out: []token.Token{
				{
					Keyword: keyword.INTEGER,
					Literal: "1337",
					Position: position.Position{
						Line: 1,
						Char: 1,
					},
				},
				{
					Keyword: keyword.INTEGER,
					Literal: "1338",
					Position: position.Position{
						Line: 1,
						Char: 6,
					},
				},
				{
					Keyword: keyword.INTEGER,
					Literal: "1339",
					Position: position.Position{
						Line: 1,
						Char: 11,
					},
				},
				{
					Keyword: keyword.STRING,
					Literal: `foo`,
					Position: position.Position{
						Line: 1,
						Char: 16,
					},
				},
				{
					Keyword: keyword.STRING,
					Literal: `bar`,
					Position: position.Position{
						Line: 1,
						Char: 22,
					},
				},
				{
					Keyword: keyword.STRING,
					Literal: `foo bar`,
					Position: position.Position{
						Line: 1,
						Char: 28,
					},
				},
				{
					Keyword: keyword.STRING,
					Literal: `foo
bar`,
					Position: position.Position{
						Line: 1,
						Char: 42,
					},
				},
				{
					Keyword: keyword.STRING,
					Literal: `foo
bar 
baz`,
					Position: position.Position{
						Line: 2,
						Char: 8,
					},
				},
				{
					Keyword: keyword.FLOAT,
					Literal: "13.37",
					Position: position.Position{
						Line: 6,
						Char: 1,
					},
				},
			},
		}),
		Entry("should read the introspection query", Case{
			in: `query IntrospectionQuery {
  __schema {`,
			out: []token.Token{
				{
					Keyword: keyword.QUERY,
					Literal: "query",
					Position: position.Position{
						Line: 1,
						Char: 1,
					},
				},
				{
					Keyword: keyword.IDENT,
					Literal: "IntrospectionQuery",
					Position: position.Position{
						Line: 1,
						Char: 7,
					},
				},
				{
					Keyword: keyword.CURLYBRACKETOPEN,
					Literal: literal.CURLYBRACKETOPEN,
					Position: position.Position{
						Line: 1,
						Char: 26,
					},
				},
				{
					Keyword: keyword.IDENT,
					Literal: "__schema",
					Position: position.Position{
						Line: 2,
						Char: 3,
					},
				},
				{
					Keyword: keyword.CURLYBRACKETOPEN,
					Literal: literal.CURLYBRACKETOPEN,
					Position: position.Position{
						Line: 2,
						Char: 12,
					},
				},
			},
		}),
		Entry("should read '1,2,3' as three integers", Case{
			in: "1,2,3",
			out: []token.Token{
				{
					Keyword: keyword.INTEGER,
					Literal: "1",
					Position: position.Position{
						Line: 1,
						Char: 1,
					},
				},
				{
					Keyword: keyword.INTEGER,
					Literal: "2",
					Position: position.Position{
						Line: 1,
						Char: 3,
					},
				},
				{
					Keyword: keyword.INTEGER,
					Literal: "3",
					Position: position.Position{
						Line: 1,
						Char: 5,
					},
				},
			},
		}),
	)
})

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
