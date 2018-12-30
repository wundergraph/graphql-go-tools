package lexer

import (
	"bytes"
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
	"io"
	"io/ioutil"
	"testing"
)

func TestLexer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Lexer")
}

func TestLexerRegressions(t *testing.T) {

	lexer := NewLexer()
	reader := bytes.NewReader(introspectionQuery)
	lexer.SetInput(reader)

	var total []token.Token
	for {
		tok, err := lexer.Read()
		if err != nil {
			t.Fatal(err)
		}
		if tok.Keyword == keyword.EOF {
			break
		}

		tok.Description = string(tok.Literal)

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
	It("should not panic if reader is nil", func() {
		lexer := NewLexer()
		f := func() {
			_, err := lexer.Read()
			Expect(err).To(HaveOccurred())
		}

		Expect(f).ShouldNot(Panic())
	})
	It("should read correctly from reader when re-setting input", func() {
		lexer := NewLexer()
		lexer.SetInput(bytes.NewReader([]byte("x")))
		x, err := lexer.Read()

		lexer.SetInput(bytes.NewReader([]byte("x")))
		x, err = lexer.Read()
		Expect(err).NotTo(HaveOccurred())
		Expect(x).To(Equal(token.Token{
			Keyword: keyword.IDENT,
			Literal: []byte("x"),
			Position: position.Position{
				Line: 1,
				Char: 1,
			},
		}))
	})
	It("should read eof multiple times correctly", func() {
		lexer := NewLexer()
		lexer.SetInput(bytes.NewReader([]byte("x")))

		x, err := lexer.Read()
		Expect(err).NotTo(HaveOccurred())
		Expect(x).To(Equal(token.Token{
			Keyword: keyword.IDENT,
			Literal: []byte("x"),
			Position: position.Position{
				Line: 1,
				Char: 1,
			},
		}))

		eof1, err := lexer.Read()
		Expect(err).NotTo(HaveOccurred())
		Expect(eof1).To(Equal(token.Token{
			Keyword: keyword.EOF,
			Literal: []byte("eof"),
			Position: position.Position{
				Line: 1,
				Char: 2,
			},
		}))

		eof2, err := lexer.Read()
		Expect(err).NotTo(HaveOccurred())
		Expect(eof2).To(Equal(token.Token{
			Keyword: keyword.EOF,
			Literal: []byte("eof"),
			Position: position.Position{
				Line: 1,
				Char: 2,
			},
		}))
	})
})

var _ = Describe("Lexer.Read", func() {

	type Case struct {
		in        []byte
		out       token.Token
		expectErr types.GomegaMatcher
	}

	var lexer *Lexer

	BeforeEach(func() {
		lexer = NewLexer()
	})

	DescribeTable("Read Single Token", func(c Case) {

		lexer.SetInput(bytes.NewReader(c.in))
		tok, err := lexer.Read()
		if c.expectErr != nil {
			Expect(err).To(c.expectErr)
		} else {
			Expect(err).To(BeNil())
		}
		Expect(tok).To(Equal(c.out))

	},
		Entry("should read integer", Case{
			in: []byte("1337"),
			out: token.Token{
				Keyword: keyword.INTEGER,
				Literal: []byte("1337"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read integer with comma at the end", Case{
			in: []byte("1337,"),
			out: token.Token{
				Keyword: keyword.INTEGER,
				Literal: []byte("1337"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read float", Case{
			in: []byte("13.37"),
			out: token.Token{
				Keyword: keyword.FLOAT,
				Literal: []byte("13.37"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should fail reading incomplete float", Case{
			in:        []byte("13."),
			expectErr: Not(BeNil()),
			out: token.Token{
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read single line string", Case{
			in: []byte(`"foo bar"`),
			out: token.Token{
				Keyword: keyword.STRING,
				Literal: []byte(`foo bar`),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read single line string with escaped quote", Case{
			in: []byte(`"foo bar \" baz"`),
			out: token.Token{
				Keyword: keyword.STRING,
				Literal: []byte(`foo bar " baz`),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read multi line string with escaped quote", Case{
			in: []byte(`"""foo bar \""" baz"""`),
			out: token.Token{
				Keyword: keyword.STRING,
				Literal: []byte(`foo bar """ baz`),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read multi single line string", Case{
			in: []byte(`"""
foo
bar"""`),
			out: token.Token{
				Keyword: keyword.STRING,
				Literal: []byte(`foo
bar`),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read multi single line string with correct whitespace trimming", Case{
			in: []byte(`"""
foo
"""`),
			out: token.Token{
				Keyword: keyword.STRING,
				Literal: []byte(`foo`),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read pipe", Case{
			in: []byte("|"),
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
			in: []byte("."),
			out: token.Token{
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
			expectErr: Not(BeNil()),
		}),
		Entry("should read spread (...)", Case{
			in: []byte("..."),
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
			in: []byte("$123"),
			out: token.Token{
				Keyword: keyword.VARIABLE,
				Literal: []byte("123"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read $foo", Case{
			in: []byte("$foo"),
			out: token.Token{
				Keyword: keyword.VARIABLE,
				Literal: []byte("foo"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read $_foo", Case{
			in: []byte("$_foo"),
			out: token.Token{
				Keyword: keyword.VARIABLE,
				Literal: []byte("_foo"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read $123 ", Case{
			in: []byte("$123 "),
			out: token.Token{
				Keyword: keyword.VARIABLE,
				Literal: []byte("123"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read $123\n", Case{
			in: []byte("$123\n"),
			out: token.Token{
				Keyword: keyword.VARIABLE,
				Literal: []byte("123"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read @", Case{
			in: []byte("@"),
			out: token.Token{
				Keyword: keyword.AT,
				Literal: []byte("@"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read =", Case{
			in: []byte("="),
			out: token.Token{
				Keyword: keyword.EQUALS,
				Literal: []byte("="),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read :", Case{
			in: []byte(":"),
			out: token.Token{
				Keyword: keyword.COLON,
				Literal: []byte(":"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read !", Case{
			in: []byte("!"),
			out: token.Token{
				Keyword: keyword.BANG,
				Literal: []byte("!"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read (", Case{
			in: []byte("("),
			out: token.Token{
				Keyword: keyword.BRACKETOPEN,
				Literal: []byte("("),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read )", Case{
			in: []byte(")"),
			out: token.Token{
				Keyword: keyword.BRACKETCLOSE,
				Literal: []byte(")"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read {", Case{
			in: []byte("{"),
			out: token.Token{
				Keyword: keyword.CURLYBRACKETOPEN,
				Literal: []byte("{"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read }", Case{
			in: []byte("}"),
			out: token.Token{
				Keyword: keyword.CURLYBRACKETCLOSE,
				Literal: []byte("}"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read [", Case{
			in: []byte("["),
			out: token.Token{
				Keyword: keyword.SQUAREBRACKETOPEN,
				Literal: []byte("["),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read ]", Case{
			in: []byte("]"),
			out: token.Token{
				Keyword: keyword.SQUAREBRACKETCLOSE,
				Literal: []byte("]"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read &", Case{
			in: []byte("&"),
			out: token.Token{
				Keyword: keyword.AND,
				Literal: []byte("&"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read EOF", Case{
			in: []byte(""),
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
			in: []byte("foo"),
			out: token.Token{
				Keyword: keyword.IDENT,
				Literal: []byte("foo"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read ident 'foo' from 'foo:'", Case{
			in: []byte("foo:"),
			out: token.Token{
				Keyword: keyword.IDENT,
				Literal: []byte("foo"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read ident true", Case{
			in: []byte("true"),
			out: token.Token{
				Keyword: keyword.TRUE,
				Literal: []byte("true"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
		}),
		Entry("should read ident false", Case{
			in: []byte("false"),
			out: token.Token{
				Keyword: keyword.FALSE,
				Literal: []byte("false"),
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
		input              []byte
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
		lexer.SetInput(bytes.NewReader(c.input))
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
			input:              []byte(""),
			expectKey:          Equal(keyword.EOF),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.EOF,
				Literal: []byte("eof"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek query", Case{
			input:              []byte("query"),
			expectKey:          Equal(keyword.QUERY),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.QUERY,
				Literal: []byte("query"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek mutation", Case{
			input:              []byte("mutation"),
			expectKey:          Equal(keyword.MUTATION),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.MUTATION,
				Literal: []byte("mutation"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek subscription", Case{
			input:              []byte("subscription"),
			expectKey:          Equal(keyword.SUBSCRIPTION),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.SUBSCRIPTION,
				Literal: []byte("subscription"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek fragment", Case{
			input:              []byte("fragment"),
			expectKey:          Equal(keyword.FRAGMENT),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.FRAGMENT,
				Literal: []byte("fragment"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek spread (...)", Case{
			input:              []byte("..."),
			expectKey:          Equal(keyword.SPREAD),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.SPREAD,
				Literal: []byte("..."),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'implements'", Case{
			input:              []byte("implements"),
			expectKey:          Equal(keyword.IMPLEMENTS),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.IMPLEMENTS,
				Literal: []byte("implements"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'schema'", Case{
			input:              []byte("schema"),
			expectKey:          Equal(keyword.SCHEMA),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.SCHEMA,
				Literal: []byte("schema"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'scalar'", Case{
			input:              []byte("scalar"),
			expectKey:          Equal(keyword.SCALAR),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.SCALAR,
				Literal: []byte("scalar"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'type'", Case{
			input:              []byte("type"),
			expectKey:          Equal(keyword.TYPE),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.TYPE,
				Literal: []byte("type"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'interface'", Case{
			input:              []byte("interface"),
			expectKey:          Equal(keyword.INTERFACE),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.INTERFACE,
				Literal: []byte("interface"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'union'", Case{
			input:              []byte("union"),
			expectKey:          Equal(keyword.UNION),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.UNION,
				Literal: []byte("union"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'enum'", Case{
			input:              []byte("enum"),
			expectKey:          Equal(keyword.ENUM),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.ENUM,
				Literal: []byte("enum"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'input'", Case{
			input:              []byte("input"),
			expectKey:          Equal(keyword.INPUT),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.INPUT,
				Literal: []byte("input"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'directive'", Case{
			input:              []byte("directive"),
			expectKey:          Equal(keyword.DIRECTIVE),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.DIRECTIVE,
				Literal: []byte("directive"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek 'inputValue' as ident", Case{
			input:              []byte("inputValue"),
			expectKey:          Equal(keyword.IDENT),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.IDENT,
				Literal: []byte("inputValue"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek ON", Case{
			input:              []byte("on"),
			expectKey:          Equal(keyword.ON),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.ON,
				Literal: []byte("on"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek ON with whitespace behind", Case{
			input:              []byte("on "),
			expectKey:          Equal(keyword.ON),
			expectErr:          BeNil(),
			expectNextTokenErr: BeNil(),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.ON,
				Literal: []byte("on"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek ignore comma", Case{
			input:     []byte(","),
			expectKey: Equal(keyword.EOF),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.EOF,
				Literal: []byte("eof"),
				Position: position.Position{
					Line: 1,
					Char: 2,
				},
			}),
		}),
		Entry("should peek '$color:' as variable color", Case{
			input:     []byte("$color:"),
			expectKey: Equal(keyword.VARIABLE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.VARIABLE,
				Literal: []byte("color"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek '$ color:' as invalid", Case{
			input:              []byte("$ color:"),
			expectErr:          BeNil(),
			expectNextTokenErr: HaveOccurred(),
		}),
		Entry("should peek ignore space", Case{
			input:     []byte(" "),
			expectKey: Equal(keyword.EOF),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.EOF,
				Literal: []byte("eof"),
				Position: position.Position{
					Line: 1,
					Char: 2,
				},
			}),
		}),
		Entry("should peek ignore tab", Case{
			input: []byte("	"),
			expectKey: Equal(keyword.EOF),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.EOF,
				Literal: []byte("eof"),
				Position: position.Position{
					Line: 1,
					Char: 2,
				},
			}),
		}),
		Entry("should peek ignore line terminator", Case{
			input:     []byte("\n"),
			expectKey: Equal(keyword.EOF),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.EOF,
				Literal: []byte("eof"),
				Position: position.Position{
					Line: 2,
					Char: 1,
				},
			}),
		}),
		Entry("should peek single line string", Case{
			input:     []byte(`"foo"`),
			expectKey: Equal(keyword.STRING),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.STRING,
				Literal: []byte("foo"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek multi line string", Case{
			input:     []byte(`"""foo"""`),
			expectKey: Equal(keyword.STRING),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.STRING,
				Literal: []byte("foo"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek variable", Case{
			input:     []byte("$foo"),
			expectKey: Equal(keyword.VARIABLE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.VARIABLE,
				Literal: []byte("foo"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should throw error when reading invalid variable", Case{
			input:              []byte("$ foo"),
			expectNextTokenErr: HaveOccurred(),
		}),
		Entry("should peek pipe", Case{
			input:     []byte("|"),
			expectKey: Equal(keyword.PIPE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.PIPE,
				Literal: []byte("|"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek equals", Case{
			input:     []byte("="),
			expectKey: Equal(keyword.EQUALS),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.EQUALS,
				Literal: []byte("="),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek at", Case{
			input:     []byte("@"),
			expectKey: Equal(keyword.AT),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.AT,
				Literal: []byte("@"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek null", Case{
			input:     []byte("null"),
			expectKey: Equal(keyword.NULL),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.NULL,
				Literal: []byte("null"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek colon", Case{
			input:     []byte(":"),
			expectKey: Equal(keyword.COLON),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.COLON,
				Literal: []byte(":"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek bang", Case{
			input:     []byte("!"),
			expectKey: Equal(keyword.BANG),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.BANG,
				Literal: []byte("!"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek bracket open", Case{
			input:     []byte("("),
			expectKey: Equal(keyword.BRACKETOPEN),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.BRACKETOPEN,
				Literal: []byte("("),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek bracket close", Case{
			input:     []byte(")"),
			expectKey: Equal(keyword.BRACKETCLOSE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.BRACKETCLOSE,
				Literal: []byte(")"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek squared bracket open", Case{
			input:     []byte("["),
			expectKey: Equal(keyword.SQUAREBRACKETOPEN),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.SQUAREBRACKETOPEN,
				Literal: []byte("["),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek squared bracket close", Case{
			input:     []byte("]"),
			expectKey: Equal(keyword.SQUAREBRACKETCLOSE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.SQUAREBRACKETCLOSE,
				Literal: []byte("]"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek curly bracket open", Case{
			input:     []byte("{"),
			expectKey: Equal(keyword.CURLYBRACKETOPEN),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.CURLYBRACKETOPEN,
				Literal: []byte("{"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek curly bracket close", Case{
			input:     []byte("}"),
			expectKey: Equal(keyword.CURLYBRACKETCLOSE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.CURLYBRACKETCLOSE,
				Literal: []byte("}"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek and", Case{
			input:     []byte("&"),
			expectKey: Equal(keyword.AND),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.AND,
				Literal: []byte("&"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek ident", Case{
			input:     []byte("foo"),
			expectKey: Equal(keyword.IDENT),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.IDENT,
				Literal: []byte("foo"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek integer", Case{
			input:     []byte("1337"),
			expectKey: Equal(keyword.INTEGER),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.INTEGER,
				Literal: []byte("1337"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek float", Case{
			input:     []byte("13.37"),
			expectKey: Equal(keyword.FLOAT),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.FLOAT,
				Literal: []byte("13.37"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek true", Case{
			input:     []byte("true "),
			expectKey: Equal(keyword.TRUE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.TRUE,
				Literal: []byte("true"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			}),
		}),
		Entry("should peek true with space in front", Case{
			input:     []byte(" true "),
			expectKey: Equal(keyword.TRUE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.TRUE,
				Literal: []byte("true"),
				Position: position.Position{
					Line: 1,
					Char: 2,
				},
			}),
		}),
		Entry("should peek true with multiple spaces in front", Case{
			input:     []byte("   true"),
			expectKey: Equal(keyword.TRUE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.TRUE,
				Literal: []byte("true"),
				Position: position.Position{
					Line: 1,
					Char: 4,
				},
			}),
		}),
		Entry("should peek false", Case{
			input:     []byte("false "),
			expectKey: Equal(keyword.FALSE),
			expectNextToken: Equal(token.Token{
				Keyword: keyword.FALSE,
				Literal: []byte("false"),
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
		in        []byte
		isFloat   bool
		expectErr types.GomegaMatcher
	}

	var lexer *Lexer

	BeforeEach(func() {
		lexer = NewLexer()
	})

	DescribeTable("peekIsFloat cases", func(c Case) {

		lexer.SetInput(bytes.NewReader(c.in))
		actualIsFloat, err := lexer.peekIsFloat()
		Expect(actualIsFloat).To(Equal(c.isFloat))
		if c.expectErr != nil {
			Expect(err).To(c.expectErr)
		}

	}, Entry("should identify 13.37 as float", Case{
		in:        []byte("13.37"),
		expectErr: BeNil(),
		isFloat:   true,
	}), Entry("should identify 13.37 as float (with space suffix)", Case{
		in:        []byte("13.37 "),
		expectErr: BeNil(),
		isFloat:   true,
	}), Entry("should identify 13.37 as float (with tab suffix)", Case{
		in: []byte("13.37	"),
		expectErr: BeNil(),
		isFloat:   true,
	}), Entry("should identify 13.37 as float (with line terminator suffix)", Case{
		in:        []byte("13.37\n"),
		expectErr: BeNil(),
		isFloat:   true,
	}), Entry("should identify 13.37 as float (with comma suffix)", Case{
		in:        []byte("13.37,"),
		expectErr: BeNil(),
		isFloat:   true,
	}), Entry("should identify 1337 as non float", Case{
		in:        []byte("1337"),
		expectErr: BeNil(),
		isFloat:   false,
	}),
	)
})

func BenchmarkPeekIsFloat(b *testing.B) {
	input := bytes.NewReader([]byte("13373737.37"))
	lexer := NewLexer()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		input.Seek(0, io.SeekStart)
		lexer.SetInput(input)
		lexer.peekIsFloat()
	}
}

var _ = Describe("Lexer.Read", func() {

	type Case struct {
		in  []byte
		out []token.Token
	}

	var lexer *Lexer

	BeforeEach(func() {
		lexer = NewLexer()
	})

	DescribeTable("Read Multiple Tokens", func(c Case) {

		lexer.SetInput(bytes.NewReader(c.in))
		for i := 0; i < len(c.out); i++ {
			peeked, _ := lexer.Peek(true)
			Expect(peeked).To(Equal(c.out[i].Keyword), fmt.Sprintf("Token: %d", i+1))
			tok, err := lexer.Read()
			Expect(err).To(BeNil())
			Expect(tok).To(Equal(c.out[i]))
		}

	}, Entry("should read ident followed by colon", Case{
		in: []byte("foo:"),
		out: []token.Token{
			{
				Keyword: keyword.IDENT,
				Literal: []byte("foo"),
				Position: position.Position{
					Line: 1,
					Char: 1,
				},
			},
			{
				Keyword: keyword.COLON,
				Literal: []byte(":"),
				Position: position.Position{
					Line: 1,
					Char: 4,
				},
			},
		},
	}),
		Entry("should read complex nested structure", Case{
			in: []byte(`Goland {
					... on GoWater {
						... on GoAir {
							go
						}
					}
				}
				`),
			out: []token.Token{
				{
					Keyword:  keyword.IDENT,
					Literal:  []byte("Goland"),
					Position: position.Position{1, 1},
				},
				{
					Keyword:  keyword.CURLYBRACKETOPEN,
					Literal:  []byte("{"),
					Position: position.Position{1, 8},
				},
				{
					Keyword:  keyword.SPREAD,
					Literal:  []byte("..."),
					Position: position.Position{2, 6},
				},
				{
					Keyword:  keyword.ON,
					Literal:  []byte("on"),
					Position: position.Position{2, 10},
				},
				{
					Keyword:  keyword.IDENT,
					Literal:  []byte("GoWater"),
					Position: position.Position{2, 13},
				},
				{
					Keyword:  keyword.CURLYBRACKETOPEN,
					Literal:  []byte("{"),
					Position: position.Position{2, 21},
				},
				{
					Keyword:  keyword.SPREAD,
					Literal:  []byte("..."),
					Position: position.Position{3, 7},
				},
				{
					Keyword:  keyword.ON,
					Literal:  []byte("on"),
					Position: position.Position{3, 11},
				},
				{
					Keyword:  keyword.IDENT,
					Literal:  []byte("GoAir"),
					Position: position.Position{3, 14},
				},
				{
					Keyword:  keyword.CURLYBRACKETOPEN,
					Literal:  []byte("{"),
					Position: position.Position{3, 20},
				},
				{
					Keyword:  keyword.IDENT,
					Literal:  []byte("go"),
					Position: position.Position{4, 8},
				},
				{
					Keyword:  keyword.CURLYBRACKETCLOSE,
					Literal:  []byte("}"),
					Position: position.Position{5, 7},
				},
				{
					Keyword:  keyword.CURLYBRACKETCLOSE,
					Literal:  []byte("}"),
					Position: position.Position{6, 6},
				},
				{
					Keyword:  keyword.CURLYBRACKETCLOSE,
					Literal:  []byte("}"),
					Position: position.Position{7, 5},
				},
				{
					Keyword:  keyword.EOF,
					Literal:  []byte("eof"),
					Position: position.Position{8, 5},
				},
			},
		}),
		Entry("should read multiple keywords", Case{
			in: []byte(`1337 1338 1339 "foo" "bar" """foo bar""" """foo
bar""" """foo
bar 
baz
"""
13.37`),
			out: []token.Token{
				{
					Keyword: keyword.INTEGER,
					Literal: []byte("1337"),
					Position: position.Position{
						Line: 1,
						Char: 1,
					},
				},
				{
					Keyword: keyword.INTEGER,
					Literal: []byte("1338"),
					Position: position.Position{
						Line: 1,
						Char: 6,
					},
				},
				{
					Keyword: keyword.INTEGER,
					Literal: []byte("1339"),
					Position: position.Position{
						Line: 1,
						Char: 11,
					},
				},
				{
					Keyword: keyword.STRING,
					Literal: []byte(`foo`),
					Position: position.Position{
						Line: 1,
						Char: 16,
					},
				},
				{
					Keyword: keyword.STRING,
					Literal: []byte(`bar`),
					Position: position.Position{
						Line: 1,
						Char: 22,
					},
				},
				{
					Keyword: keyword.STRING,
					Literal: []byte(`foo bar`),
					Position: position.Position{
						Line: 1,
						Char: 28,
					},
				},
				{
					Keyword: keyword.STRING,
					Literal: []byte(`foo
bar`),
					Position: position.Position{
						Line: 1,
						Char: 42,
					},
				},
				{
					Keyword: keyword.STRING,
					Literal: []byte(`foo
bar 
baz`),
					Position: position.Position{
						Line: 2,
						Char: 8,
					},
				},
				{
					Keyword: keyword.FLOAT,
					Literal: []byte("13.37"),
					Position: position.Position{
						Line: 6,
						Char: 1,
					},
				},
			},
		}),
		Entry("should read the introspection query", Case{
			in: []byte(`query IntrospectionQuery {
  __schema {`),
			out: []token.Token{
				{
					Keyword: keyword.QUERY,
					Literal: []byte("query"),
					Position: position.Position{
						Line: 1,
						Char: 1,
					},
				},
				{
					Keyword: keyword.IDENT,
					Literal: []byte("IntrospectionQuery"),
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
					Literal: []byte("__schema"),
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
			in: []byte("1,2,3"),
			out: []token.Token{
				{
					Keyword: keyword.INTEGER,
					Literal: []byte("1"),
					Position: position.Position{
						Line: 1,
						Char: 1,
					},
				},
				{
					Keyword: keyword.INTEGER,
					Literal: []byte("2"),
					Position: position.Position{
						Line: 1,
						Char: 3,
					},
				},
				{
					Keyword: keyword.INTEGER,
					Literal: []byte("3"),
					Position: position.Position{
						Line: 1,
						Char: 5,
					},
				},
			},
		}),
	)
})

func BenchmarkLexer(b *testing.B) {

	lexer := NewLexer()
	reader := bytes.NewReader(introspectionQuery)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {

		var tok token.Token
		var key keyword.Keyword

		_, err := reader.Seek(0, io.SeekStart)
		if err != nil {
			b.Fatal(err)
		}

		lexer.SetInput(reader)

		for err == nil && tok.Keyword != keyword.EOF && key != keyword.EOF {
			key, err = lexer.Peek(true)
			if err != nil {
				b.Fatal(err)
			}

			tok, err = lexer.Read()
			if err != nil {
				b.Fatal(err)
			}
		}
	}
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
