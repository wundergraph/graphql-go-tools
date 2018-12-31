package parser

import (
	"bytes"
	"testing"

	. "github.com/franela/goblin"

	"github.com/jensneuse/graphql-go-tools/pkg/document"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

func TestValueParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parseValue", func() {
		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse variableValue",
				input:     "$foo",
				expectErr: BeNil(),
				expectValues: Equal(document.VariableValue{
					Name: []byte("foo"),
				}),
			},
			{
				it:        "should throw an error when using '$ foo' instead of '$foo'",
				input:     "$ foo",
				expectErr: HaveOccurred(),
			},
			{
				it:        "should parse Int32 values",
				input:     "1337",
				expectErr: BeNil(),
				expectValues: Equal(document.IntValue{
					Val: int32(1337),
				}),
			},
			{
				it:        "should parse Float32 values",
				input:     "13.37",
				expectErr: BeNil(),
				expectValues: Equal(document.FloatValue{
					Val: float32(13.37),
				}),
			},
			{
				it:        "should parse true as BooleanValue",
				input:     "true",
				expectErr: BeNil(),
				expectValues: Equal(document.BooleanValue{
					Val: true,
				}),
			},
			{
				it:        "should parse false as BooleanValue",
				input:     "false",
				expectErr: BeNil(),
				expectValues: Equal(document.BooleanValue{
					Val: false,
				}),
			},
			{
				it:        "should parse StringValue on single quote",
				input:     `"this is a string value"`,
				expectErr: BeNil(),
				expectValues: Equal(document.StringValue{
					Val: []byte("this is a string value"),
				}),
			},
			{
				it:        "should parse StringValue on triple quote",
				input:     `"""this is a string value"""`,
				expectErr: BeNil(),
				expectValues: Equal(document.StringValue{
					Val: []byte("this is a string value"),
				}),
			},
			{
				it:           "should parse null value",
				input:        "null",
				expectErr:    BeNil(),
				expectValues: Equal(document.NullValue{}),
			},
			{
				it:        "should parse list value",
				input:     "[true]",
				expectErr: BeNil(),
				expectValues: Equal(document.ListValue{
					Values: []document.Value{
						document.BooleanValue{
							Val: true,
						},
					},
				}),
			},
			{
				it:        "should parse object value",
				input:     "{isTrue: true}",
				expectErr: BeNil(),
				expectValues: Equal(document.ObjectValue{
					Val: []document.ObjectField{
						{
							Name: []byte("isTrue"),
							Value: document.BooleanValue{
								Val: true,
							},
						},
					},
				}),
			},
			{
				it:           "should fail at not listed keyword and return an error",
				input:        "}",
				expectErr:    Not(BeNil()),
				expectValues: BeNil(),
			},
			{
				it: "should parse value despite whitespace in front",
				input: "		 true",
				expectErr: BeNil(),
				expectValues: Equal(document.BooleanValue{
					Val: true,
				}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				reader := bytes.NewReader([]byte(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)

				val, err := parser.parseValue()
				Expect(err).To(test.expectErr)
				if test.expectValues != nil {
					Expect(val).To(test.expectValues)
				}
			})
		}

	})
}
