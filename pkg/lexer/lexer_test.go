package lexer

import (
	"bytes"
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
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
			lexer.SetInput(bytes.NewReader(token.Literal("1337")))

			tok, err := lexer.Read()
			Expect(err).To(BeNil())

			Expect(tok.Keyword).To(Equal(token.INTEGER))
			Expect(tok.Literal).To(Equal(token.Literal("1337")))
		})

		g.It("should parse float32(13.37)", func() {
			lexer := NewLexer(runestringer.NewBuffered())
			lexer.SetInput(bytes.NewReader(token.Literal("13.37")))

			tok, err := lexer.Read()
			Expect(err).To(BeNil())

			Expect(tok.Keyword).To(Equal(token.FLOAT))
			Expect(tok.Literal).To(Equal(token.Literal("13.37")))
		})

		g.It("should not parse 13.37.1337", func() {
			lexer := NewLexer(runestringer.NewBuffered())
			lexer.SetInput(bytes.NewReader(token.Literal("13.37.1337")))

			tok, err := lexer.Read()
			Expect(err).NotTo(BeNil())
			Expect(tok.Keyword).To(Equal(token.UNDEFINED))
		})

		g.It("should allow un-reading four values (and more)", func() {
			lexer := NewLexer(runestringer.NewBuffered())
			lexer.SetInput(bytes.NewReader(token.Literal(`""""
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
			lexer.SetInput(bytes.NewReader(token.Literal(`foo`)))

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
			lexer.SetInput(bytes.NewReader(token.Literal(`foo
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
			Expect(tok.Keyword).To(Equal(token.EOF))

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
			Expect(tok.Keyword).To(Equal(token.EOF))

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
			Expect(tok.Keyword).To(Equal(token.EOF))
		})
	})

	g.Describe("lexer.PeekMatchRunes", func() {

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
				lexer.SetInput(bytes.NewReader(token.Literal(test.input)))

				matches, err := lexer.PeekMatchRunes(test.match, test.amount)
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
				expectKeyword: Equal(token.STRING),
				expectLiteral: Equal(token.Literal("foo bar")),
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
				expectKeyword: Equal(token.STRING),
				expectLiteral: Equal(token.Literal(`foo "" bar`)),
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
				expectKeyword: Equal(token.NULL),
			},
			{
				it:            "should lex comment",
				input:         "# foo bar",
				expectErr:     BeNil(),
				expectKeyword: Equal(token.COMMENT),
				expectLiteral: Equal(token.Literal(`foo bar`)),
			},
			{
				it: "should dismiss all whitespace (LINETERMINATOR,TAB,SPACE,COMMA) before a token",
				input: `
	 , foo`,
				expectErr:     BeNil(),
				expectKeyword: Equal(token.IDENT),
				expectLiteral: Equal(token.Literal(`foo`)),
			},
			{
				it:            "should scan valid variable",
				input:         `$foo `,
				expectErr:     BeNil(),
				expectKeyword: Equal(token.VARIABLE),
				expectLiteral: Equal(token.Literal(`foo`)),
			},
			{
				it:            "should scan valid variable with digits",
				input:         `$123Foo `,
				expectErr:     BeNil(),
				expectKeyword: Equal(token.VARIABLE),
				expectLiteral: Equal(token.Literal(`123Foo`)),
			},
			{
				it:            "should scan valid variable with underscore and digits",
				input:         `$_123Foo `,
				expectErr:     BeNil(),
				expectKeyword: Equal(token.VARIABLE),
				expectLiteral: Equal(token.Literal(`_123Foo`)),
			},
			{
				it:            "should fail scanning variable with space after $",
				input:         `$ foo `,
				expectErr:     Not(BeNil()),
				expectKeyword: Equal(token.VARIABLE),
			},
			{
				it: "should fail scaning variable with tab after $",
				input: `$	foo `,
				expectErr:     Not(BeNil()),
				expectKeyword: Equal(token.VARIABLE),
			},
			{
				it: "should fail scaning variable with lineTerminator after $",
				input: `$
foo `,
				expectErr:     Not(BeNil()),
				expectKeyword: Equal(token.VARIABLE),
			},
			{
				it:            "should scan SPREAD",
				input:         `... `,
				expectErr:     BeNil(),
				expectKeyword: Equal(token.SPREAD),
				expectLiteral: Equal(token.Literal(`...`)),
			},
			{
				it:            "should throw error on '..'",
				input:         `..`,
				expectErr:     Not(BeNil()),
				expectKeyword: Equal(token.DOT),
				expectLiteral: Equal(token.Literal(`.`)),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {
				lexer := NewLexer(runestringer.NewBuffered())
				lexer.SetInput(bytes.NewReader(token.Literal(test.input)))

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
			lexer.SetInput(bytes.NewReader(token.Literal("$foo:")))

			tok, err := lexer.Read()
			Expect(err).To(BeNil())

			Expect(tok.Keyword).To(Equal(token.VARIABLE))
			Expect(tok.Literal).To(Equal(token.Literal("foo")))

			tok, err = lexer.Read()
			Expect(err).To(BeNil())

			Expect(tok.Keyword).To(Equal(token.COLON))
			Expect(tok.Literal).To(Equal(token.Literal(":")))
		})
	})
}

var simpleGraphqlDoc = token.Literal(`# root Resolver
	schema {
		query: Query
		mutation: Mutation
	}
	
	# root Query
	type Query {
		# get story
		storyById(
			# you must provide a valid story id
			id: String!
		): Story
		# return all Stories
		allStories: [Story]
		# get images by id
		imageById(
			# image identifier
			id: String
		): SourceImage
	}
	
	# root Mutation
	type Mutation {
		# creates a story
		createStory(
			input: CreateStoryInput
		): Story
		# update a story via a patch
		updateStoryById(
			# patch
			input: UpdateStoryInput
		): Story
		# deletes a story
		deleteStoryById(
			# story identifier
			id: String
		): MutationResult!
		# create a new image - you must attach the image data via multipart
		createImage: SourceImage
		# update image
		updateImageById(
			# update image patch
			input: UpdateImageInput
		): SourceImage
		# delete an image via its id
		deleteImageById(
			# image identifier
			id: String!
		): MutationResult!
	}
	
	# a generic mutation result for operations without a result
	type MutationResult {
		# indicate if the operation succeeded
		success: Boolean
	}
`)

/*func TestLexer(t *testing.T) {
	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("Lexer", func() {
		g.It("should lex simple doc", func() {

			stringer := runestringer.NewBuffered()
			lex := NewLexer(stringer)
			lex.SetInput(bytes.NewReader(simpleGraphqlDoc))

			for {
				tok, err := lex.readRune()
				if err != nil {
					t.Fatal(err)
				}

				if tok.Keyword == token.EOF {
					break
				}

				fmt.Println(tok)
			}
		})
	})
}*/

/*func TestCached(t *testing.T){

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("Cached Stringer", func() {
		g.It("should work", func() {

			Expect("").To(Equal(""),"")

			bufferedStringer := runestringer.NewBuffered()
			cachedStringer := runestringer.NewCached()

			lex := NewLexer(bufferedStringer)
			lex.SetInput(bytes.NewReader(simpleGraphqlDoc))

			for {
				tok, err := lex.readRune()
				if err != nil {
					t.Fatal(err)
				}

				if tok.Keyword == token.EOF {
					break
				}

				if tok.Keyword == token.IDENT {
					cachedStringer.Train(tok.Literal)
				}
			}

			lex = NewLexer(cachedStringer)
			lex.SetInput(bytes.NewReader(simpleGraphqlDoc))

			i := 0

			for {
				tok, err := lex.readRune()
				if err != nil {
					t.Fatal(err)
				}

				if tok.Keyword == token.EOF {
					fmt.Println(i)
					break
				}

				if tok.Keyword == token.IDENT {
					i++
				}

				fmt.Println(tok)
			}

		})
	})
}*/

func BenchmarkLexer_Buffered(b *testing.B) {

	for i := 0; i < b.N; i++ {

		b.StopTimer()
		reader := bytes.NewReader(simpleGraphqlDoc)
		stringer := runestringer.NewBuffered()
		lex := NewLexer(stringer)
		lex.SetInput(reader)
		b.StartTimer()

		for {
			tok, err := lex.Read()
			if err != nil {
				b.Fatal(err)
			}

			if tok.Keyword == token.EOF {
				break
			}
		}
	}
}

/*
func BenchmarkLexer_Unsafe(b *testing.B) {

	for i := 0; i < b.N; i++ {

		b.StopTimer()
		reader := bytes.NewReader(simpleGraphqlDoc)
		stringer := runestringer.NewUnsafe(125)
		lex := NewLexer(stringer)
		lex.SetInput(reader)
		b.StartTimer()

		for {
			tok, err := lex.readRune()
			if err != nil {
				b.Fatal(err)
			}

			if tok.Keyword == token.EOF {
				break
			}
		}

		stringer.Reset()
	}
}

func BenchmarkLexer_Cached(b *testing.B) {

	b.StopTimer()
	bufferedStringer := runestringer.NewBuffered()
	cachedStringer := runestringer.NewCached()

	lex := NewLexer(bufferedStringer)
	lex.SetInput(bytes.NewReader(simpleGraphqlDoc))
	b.StartTimer()

	for {
		tok, err := lex.readRune()
		if err != nil {
			b.Fatal(err)
		}

		if tok.Keyword == token.EOF {
			break
		}

		if tok.Keyword == token.IDENT {
			cachedStringer.Train(tok.Literal)
		}
	}

	b.StartTimer()

	for i := 0; i < b.N; i++ {

		b.StopTimer()
		lex := NewLexer(cachedStringer)
		lex.SetInput(bytes.NewReader(simpleGraphqlDoc))
		b.StartTimer()

		for {
			tok, err := lex.readRune()
			if err != nil {
				b.Fatal(err)
			}

			if tok.Keyword == token.EOF {
				break
			}
		}
	}
}

func BenchmarkLexer_LazyCached(b *testing.B) {

	b.StopTimer()
	stringer := runestringer.NewLazyCached()
	lex := NewLexer(stringer)
	lex.SetInput(bytes.NewReader(simpleGraphqlDoc))

	for {
		tok, err := lex.readRune()
		if err != nil {
			b.Fatal(err)
		}

		if tok.Keyword == token.EOF {
			break
		}
	}
	b.StartTimer()

	for i := 0; i < b.N; i++ {

		b.StopTimer()
		lex := NewLexer(stringer)
		lex.SetInput(bytes.NewReader(simpleGraphqlDoc))
		b.StartTimer()

		for {
			tok, err := lex.readRune()
			if err != nil {
				b.Fatal(err)
			}

			if tok.Keyword == token.EOF {
				break
			}
		}
	}
}*/
