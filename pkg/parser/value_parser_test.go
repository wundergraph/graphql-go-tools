package parser

import (
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
				expectValues: Equal(document.Value{
					ValueType:     document.ValueTypeVariable,
					VariableValue: "foo",
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
				expectValues: Equal(document.Value{
					ValueType: document.ValueTypeInt,
					IntValue:  1337,
				}),
			},
			{
				it:        "should parse Float32 values",
				input:     "13.37",
				expectErr: BeNil(),
				expectValues: Equal(document.Value{
					ValueType:  document.ValueTypeFloat,
					FloatValue: 13.37,
				}),
			},
			{
				it:        "should parse true as BooleanValue",
				input:     "true",
				expectErr: BeNil(),
				expectValues: Equal(document.Value{
					ValueType:    document.ValueTypeBoolean,
					BooleanValue: true,
				}),
			},
			{
				it:        "should parse false as BooleanValue",
				input:     "false",
				expectErr: BeNil(),
				expectValues: Equal(document.Value{
					ValueType:    document.ValueTypeBoolean,
					BooleanValue: false,
				}),
			},
			{
				it:        "should parse StringValue on single quote",
				input:     `"this is a string value"`,
				expectErr: BeNil(),
				expectValues: Equal(document.Value{
					ValueType:   document.ValueTypeString,
					StringValue: "this is a string value",
				}),
			},
			{
				it:        "should parse StringValue on triple quote",
				input:     `"""this is a string value"""`,
				expectErr: BeNil(),
				expectValues: Equal(document.Value{
					ValueType:   document.ValueTypeString,
					StringValue: "this is a string value",
				}),
			},
			{
				it:        "should parse null value",
				input:     "null",
				expectErr: BeNil(),
				expectValues: Equal(document.Value{
					ValueType: document.ValueTypeNull,
				}),
			},
			{
				it:        "should parse list value",
				input:     "[true]",
				expectErr: BeNil(),
				expectValues: Equal(document.Value{
					ValueType: document.ValueTypeList,
					ListValue: []document.Value{
						{
							ValueType:    document.ValueTypeBoolean,
							BooleanValue: true,
						},
					},
				}),
			},
			{
				it:        "should parse object value",
				input:     "{isTrue: true}",
				expectErr: BeNil(),
				expectValues: Equal(document.Value{
					ValueType: document.ValueTypeObject,
					ObjectValue: []document.ObjectField{
						{
							Name: "isTrue",
							Value: document.Value{
								ValueType:    document.ValueTypeBoolean,
								BooleanValue: true,
							},
						},
					},
				}),
			},
			{
				it:           "should fail at not listed keyword and return an error",
				input:        "}",
				expectErr:    HaveOccurred(),
				expectValues: Equal(document.Value{}),
			},
			{
				it: "should parse value despite whitespace in front",
				input: "		 true",
				expectErr: BeNil(),
				expectValues: Equal(document.Value{
					ValueType:    document.ValueTypeBoolean,
					BooleanValue: true,
				}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				val, err := parser.parseValue()
				Expect(err).To(test.expectErr)
				if test.expectValues != nil {
					Expect(val).To(test.expectValues)
				}
			})
		}

	})
}
