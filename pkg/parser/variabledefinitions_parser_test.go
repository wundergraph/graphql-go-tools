package parser

import (
	"bytes"
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestVariableDefinitionsParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseVariableDefinitions", func() {

		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse a simple, single VariableDefinition",
				input:     "($foo : bar!)",
				expectErr: BeNil(),
				expectValues: Equal(document.VariableDefinitions{
					document.VariableDefinition{
						Variable: []byte("foo"),
						Type: document.NamedType{
							Name:    []byte("bar"),
							NonNull: true,
						},
					},
				}),
			},
			{
				it:        "should parse a simple, single nullable VariableDefinition",
				input:     "($color: String)",
				expectErr: BeNil(),
				expectValues: Equal(document.VariableDefinitions{
					document.VariableDefinition{
						Variable: []byte("color"),
						Type: document.NamedType{
							Name:    []byte("String"),
							NonNull: false,
						},
					},
				}),
			},
			{
				it:        "should parse simple VariableDefinitions",
				input:     "($foo : bar $baz : bax)",
				expectErr: BeNil(),
				expectValues: Equal(document.VariableDefinitions{
					document.VariableDefinition{
						Variable: []byte("foo"),
						Type: document.NamedType{
							Name: []byte("bar"),
						},
					},
					document.VariableDefinition{
						Variable: []byte("baz"),
						Type: document.NamedType{
							Name: []byte("bax"),
						},
					},
				}),
			},
			{
				it:        "should parse simple VariableDefinitions with ListType between",
				input:     "($foo : [bar] $baz : bax)",
				expectErr: BeNil(),
				expectValues: Equal(document.VariableDefinitions{
					document.VariableDefinition{
						Variable: []byte("foo"),
						Type: document.ListType{Type: document.NamedType{
							Name: []byte("bar"),
						}},
					},
					document.VariableDefinition{
						Variable: []byte("baz"),
						Type: document.NamedType{
							Name: []byte("bax"),
						},
					},
				}),
			},
			{
				it:        "should parse simple VariableDefinitions with NonNullType between",
				input:     "($foo : bar! $baz : bax)",
				expectErr: BeNil(),
				expectValues: Equal(document.VariableDefinitions{
					document.VariableDefinition{
						Variable: []byte("foo"),
						Type: document.NamedType{
							Name:    []byte("bar"),
							NonNull: true,
						},
					},
					document.VariableDefinition{
						Variable: []byte("baz"),
						Type: document.NamedType{
							Name: []byte("bax"),
						},
					},
				}),
			},
			{
				it:        "should parse simple VariableDefinitions with DefaultValue between",
				input:     `($foo : bar! = "me" $baz : bax)`,
				expectErr: BeNil(),
				expectValues: Equal(document.VariableDefinitions{
					document.VariableDefinition{
						Variable: []byte("foo"),
						Type: document.NamedType{
							Name:    []byte("bar"),
							NonNull: true,
						},
						DefaultValue: document.StringValue{
							Val: []byte("me"),
						},
					},
					document.VariableDefinition{
						Variable: []byte("baz"),
						Type: document.NamedType{
							Name: []byte("bax"),
						},
					},
				}),
			},
			{
				it:        "should not parse VariableDefinitions when no closing bracket",
				input:     "($foo : bar!",
				expectErr: Not(BeNil()),
				expectValues: Equal(document.VariableDefinitions{
					document.VariableDefinition{
						Variable: []byte("foo"),
						Type: document.NamedType{
							Name:    []byte("bar"),
							NonNull: true,
						},
					},
				}),
			},
			{
				it:           "should not parse optional VariableDefinitions",
				input:        " ",
				expectErr:    BeNil(),
				expectValues: Equal(document.VariableDefinitions(nil)),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				reader := bytes.NewReader([]byte(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)

				val, err := parser.parseVariableDefinitions()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
