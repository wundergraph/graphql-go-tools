package parser

import (
	"bytes"
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestNamedTypeParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseNamedType", func() {

		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse a simple Named Type",
				input:     "String",
				expectErr: BeNil(),
				expectValues: Equal(document.NamedType{
					Name: "String",
				}),
			},
			{
				it:        "should parse a NonNull Named Type",
				input:     "String!",
				expectErr: BeNil(),
				expectValues: Equal(document.NamedType{
					Name:    "String",
					NonNull: true,
				}),
			},
			{
				it:           "should not parse a Named Type on non-IDENT keyword",
				input:        ":String",
				expectErr:    Not(BeNil()),
				expectValues: Equal(document.NamedType{}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				reader := bytes.NewReader([]byte(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)

				val, err := parser.parseNamedType()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
