package parser

import (
	"bytes"
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
	. "github.com/onsi/gomega"
	"testing"
)

func TestParserReadAll(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.readAllUntil.foreach should iterate all tokens", func() {
		g.It("should read all idents until the next }", func() {

			reader := bytes.NewReader(token.Literal(`foo bar baz }`))

			parser := NewParser()
			parser.l.SetInput(reader)

			var idents []token.Token

			err := parser.readAllUntil(token.CURLYBRACKETCLOSE,
				WithWhitelist(token.IDENT),
				WithIgnore(token.SPACE)).
				foreach(func(tok token.Token) bool {

					idents = append(idents, tok)
					identStr := string(tok.Literal)
					_ = identStr

					return true
				})

			Expect(err).To(BeNil())
			Expect(len(idents)).To(Equal(3))
			Expect(string(idents[0].Literal)).To(Equal("foo"))
			Expect(string(idents[1].Literal)).To(Equal("bar"))
			Expect(string(idents[2].Literal)).To(Equal("baz"))
		})

	})

	g.Describe("parser.readAllUntil().foreachMatchedPattern", func() {
		g.It("should match once", func() {

			parser := NewParser()
			parser.l.SetInput(bytes.NewReader(token.Literal(`foo bar baz }`)))

			matched, err := parser.readAllUntil(token.CURLYBRACKETCLOSE,
				WithIgnore(token.SPACE)).
				foreachMatchedPattern(Pattern(token.IDENT, token.IDENT, token.IDENT), func(tokens []token.Token) error {
					Expect(tokens[0].Literal).To(Equal(token.Literal("foo")))
					Expect(tokens[1].Literal).To(Equal(token.Literal("bar")))
					Expect(tokens[2].Literal).To(Equal(token.Literal("baz")))
					return nil
				})

			Expect(err).To(BeNil())
			Expect(matched).To(Equal(1))
		})

		g.It("should return matched = 0 if pattern didn't match ('}' instead of '|')", func() {

			parser := NewParser()
			parser.l.SetInput(bytes.NewReader(token.Literal(`foo bar baz }`)))

			matched, err := parser.readAllUntil(token.CURLYBRACKETCLOSE).
				foreachMatchedPattern(Pattern(token.IDENT, token.IDENT, token.PIPE), func(tokens []token.Token) error {
					return nil
				})

			Expect(err).To(BeNil())
			Expect(matched).To(Equal(0))
		})

		g.It("should return matched = 3", func() {

			parser := NewParser()
			parser.l.SetInput(bytes.NewReader(token.Literal(`foo bar baz }`)))

			matched, err := parser.readAllUntil(token.CURLYBRACKETCLOSE).
				foreachMatchedPattern(Pattern(token.IDENT), func(tokens []token.Token) error {
					return nil
				})

			Expect(err).To(BeNil())
			Expect(matched).To(Equal(3))
		})
	})
}
