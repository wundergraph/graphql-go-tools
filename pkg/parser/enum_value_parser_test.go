package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestEnumValueParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parsePeekedEnumValue", func() {

		tests := []struct {
			it         string
			input      string
			expectErr  types.GomegaMatcher
			expectName types.GomegaMatcher
		}{
			{
				it:         "should parse MY_ENUM",
				input:      "MY_ENUM",
				expectErr:  BeNil(),
				expectName: Equal("MY_ENUM"),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				val, err := parser.parsePeekedEnumValue()
				Expect(err).To(test.expectErr)
				Expect(val.ValueType).To(Equal(document.ValueTypeEnum))
				Expect(val.EnumValue).To(test.expectName)
			})
		}
	})
}
