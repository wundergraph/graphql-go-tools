package parser

import (
	"bytes"
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestOperationDefinitionParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseOperationDefinition", func() {

		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it: "should parse a simple OperationDefinition",
				input: `
				allGophers($color: String)@rename(index: 3) {
					name
				}
				`,
				expectErr: BeNil(),
				expectValues: Equal(document.OperationDefinition{
					OperationType: document.OperationTypeQuery,
					Name:          []byte("allGophers"),
					VariableDefinitions: document.VariableDefinitions{
						{
							Variable: []byte("color"),
							Type: document.NamedType{
								Name: []byte("String"),
							},
						},
					},
					Directives: document.Directives{
						document.Directive{
							Name: []byte("rename"),
							Arguments: document.Arguments{
								document.Argument{
									Name: []byte("index"),
									Value: document.IntValue{
										Val: 3,
									},
								},
							},
						},
					},
					SelectionSet: document.SelectionSet{
						document.Field{
							Name: []byte("name"),
						},
					},
				}),
			},
			{
				it: "should parse a OperationDefinition with optional Directives",
				input: `
				allGophers($color: String) {
					name
				}
				`,
				expectErr: BeNil(),
				expectValues: Equal(document.OperationDefinition{
					OperationType: document.OperationTypeQuery,
					Name:          []byte("allGophers"),
					VariableDefinitions: document.VariableDefinitions{
						{
							Variable: []byte("color"),
							Type: document.NamedType{
								Name: []byte("String"),
							},
						},
					},
					SelectionSet: document.SelectionSet{
						document.Field{
							Name: []byte("name"),
						},
					},
				}),
			},
			{
				it: "should parse a OperationDefinition with optional VariableDefinitions",
				input: `
				allGophers@rename(index: 3) {
					name
				}
				`,
				expectErr: BeNil(),
				expectValues: Equal(document.OperationDefinition{
					OperationType: document.OperationTypeQuery,
					Name:          []byte("allGophers"),
					Directives: document.Directives{
						document.Directive{
							Name: []byte("rename"),
							Arguments: document.Arguments{
								document.Argument{
									Name: []byte("index"),
									Value: document.IntValue{
										Val: 3,
									},
								},
							},
						},
					},
					SelectionSet: document.SelectionSet{
						document.Field{
							Name: []byte("name"),
						},
					},
				}),
			},
			{
				it: "should parse an OperationDefinition with optional Name",
				input: `
				($color: String)@rename(index: 3) {
					name
				}
				`,
				expectErr: BeNil(),
				expectValues: Equal(document.OperationDefinition{
					OperationType: document.OperationTypeQuery,
					VariableDefinitions: document.VariableDefinitions{
						{
							Variable: []byte("color"),
							Type: document.NamedType{
								Name: []byte("String"),
							},
						},
					},
					Directives: document.Directives{
						document.Directive{
							Name: []byte("rename"),
							Arguments: document.Arguments{
								document.Argument{
									Name: []byte("index"),
									Value: document.IntValue{
										Val: 3,
									},
								},
							},
						},
					},
					SelectionSet: document.SelectionSet{
						document.Field{
							Name: []byte("name"),
						},
					},
				}),
			},
			{
				it: "should parse a OperationDefinition omitting all optional types",
				input: `
				{
					name
				}
				`,
				expectErr: BeNil(),
				expectValues: Equal(document.OperationDefinition{
					OperationType: document.OperationTypeQuery,
					SelectionSet: document.SelectionSet{
						document.Field{
							Name: []byte("name"),
						},
					},
				}),
			},
			{
				it: "should not parse a OperationDefinition without SelectionSet",
				input: `
				allGophers($color: String)@rename(index: 3) `,
				expectErr: Not(BeNil()),
				expectValues: Equal(document.OperationDefinition{
					OperationType: document.OperationTypeQuery,
					Name:          []byte("allGophers"),
					VariableDefinitions: document.VariableDefinitions{
						{
							Variable: []byte("color"),
							Type: document.NamedType{
								Name: []byte("String"),
							},
						},
					},
					Directives: document.Directives{
						document.Directive{
							Name: []byte("rename"),
							Arguments: document.Arguments{
								document.Argument{
									Name: []byte("index"),
									Value: document.IntValue{
										Val: 3,
									},
								},
							},
						},
					},
				}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				reader := bytes.NewReader([]byte(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)

				val, err := parser.parseOperationDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
