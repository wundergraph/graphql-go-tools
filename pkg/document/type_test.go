package document

import (
	. "github.com/franela/goblin"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestAsGoType(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("TestAsGoType", func() {
		tests := []struct {
			it           string
			input        Type
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it: "should convert gql 'Int' to go 'int32'",
				input: NamedType{
					Name: "Int",
				},

				expectErr:    BeNil(),
				expectValues: Equal("int32"),
			},
			{
				it: "should convert gql 'Float' to go 'float32'",
				input: NamedType{
					Name: "Float",
				},

				expectErr:    BeNil(),
				expectValues: Equal("float32"),
			},
			{
				it: "should convert gql 'String' to go 'string'",
				input: NamedType{
					Name: "String",
				},
				expectErr:    BeNil(),
				expectValues: Equal("string"),
			},
			{
				it: "should convert gql 'Boolean' to go 'bool'",
				input: NamedType{
					Name:    "Boolean",
					NonNull: false,
				},
				expectErr:    BeNil(),
				expectValues: Equal("bool"),
			},
			{
				it: "should convert gql '[Int]' to go '[]int32'",
				input: ListType{
					Type: NamedType{
						Name:    "Int",
						NonNull: false,
					}},

				expectErr:    BeNil(),
				expectValues: Equal("[]int32"),
			},
			{
				it: "should convert gql '[[Int]]' to go '[][]int32'",
				input: ListType{
					Type: NamedType{
						Name:    "Int",
						NonNull: false,
					}},

				expectErr:    BeNil(),
				expectValues: Equal("[]int32"),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {
				fieldType := test.input.AsGoType()
				Expect(fieldType).To(test.expectValues)
			})
		}
	})
}
