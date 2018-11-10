package transform

import (
	. "github.com/franela/goblin"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestTemplate(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("TrimWhitespace", func() {

		tests := []struct {
			it     string
			input  []byte
			expect types.GomegaMatcher
		}{
			{
				it:     "should trim space",
				input:  []byte(` lorem ipsum `),
				expect: Equal([]byte(`lorem ipsum`)),
			},
			{
				it: "should trim tabs",
				input: []byte(`	lorem ipsum	`),
				expect: Equal([]byte(`lorem ipsum`)),
			},
			{
				it: "should trim lineterminators",
				input: []byte(`
lorem ipsum
`),
				expect: Equal([]byte(`lorem ipsum`)),
			},
			{
				it: "should trim all kinds of whitespace",
				input: []byte(`
	 lorem ipsum
	 `),
				expect: Equal([]byte(`lorem ipsum`)),
			},
		}

		for _, test := range tests {

			test := test

			g.It(test.it, func() {
				out := TrimWhitespace(test.input)
				Expect(out).To(test.expect)
			})
		}
	})
}
