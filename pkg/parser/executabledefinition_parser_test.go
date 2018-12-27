package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"strings"
	"testing"
)

func TestExecutableDefinitionParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseExecutableDefinition", func() {

		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it: "should parse a simple ExecutableDefinition with OperationDefinition",
				input: `
				query allGophers($color: String)@rename(index: 3) {
					name
				}`,
				expectErr: BeNil(),
				expectValues: Equal(document.ExecutableDefinition{
					OperationDefinitions: document.OperationDefinitions{
						{
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
						},
					},
				}),
			},
			{
				it: "should parse a simple ExecutableDefinition with FragmentDefinition",
				input: `
				fragment MyFragment on SomeType @rename(index: 3){
					name
				}
				query Q1 {
					foo
				}
				`,
				expectErr: BeNil(),
				expectValues: Equal(document.ExecutableDefinition{
					OperationDefinitions: []document.OperationDefinition{
						{
							Name:          []byte("Q1"),
							OperationType: document.OperationTypeQuery,
							SelectionSet: []document.Selection{
								document.Field{
									Name: []byte("foo"),
								},
							},
						},
					},
					FragmentDefinitions: document.FragmentDefinitions{
						{
							FragmentName: []byte("MyFragment"),
							TypeCondition: document.NamedType{
								Name: []byte("SomeType"),
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
						},
					},
				}),
			},
			{
				it: "should parse a ExecutableDefinition with multiple elements",
				input: `
				query allGophers($color: String) {
					name
				}

				query allGophinas($color: String) {
					name
				}

				`,
				expectErr: BeNil(),
				expectValues: Equal(document.ExecutableDefinition{
					OperationDefinitions: document.OperationDefinitions{
						{
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
						},
						{
							OperationType: document.OperationTypeQuery,
							Name:          []byte("allGophinas"),
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
						},
					},
				}),
			},
			{
				it: "should parse a ExecutableDefinition with multiple elements of different types",
				input: `
				query allGophers($color: String) {
					name
				}

				fragment MyFragment on SomeType @rename(index: 3){
					name
				}

				query allGophinas($color: String) {
					name
				}

				`,
				expectErr: BeNil(),
				expectValues: Equal(document.ExecutableDefinition{
					OperationDefinitions: document.OperationDefinitions{
						{
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
						},
						{
							OperationType: document.OperationTypeQuery,
							Name:          []byte("allGophinas"),
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
						},
					},
					FragmentDefinitions: document.FragmentDefinitions{
						{
							FragmentName: []byte("MyFragment"),
							TypeCondition: document.NamedType{
								Name: []byte("SomeType"),
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
						},
					},
				}),
			},
			{
				it: "should not parse a ExecutableDefinition without a known identifier",
				input: `
				Barry allGophers($color: String)@rename(index: 3) {
					name
				}`,
				expectErr:    Not(BeNil()),
				expectValues: Equal(document.ExecutableDefinition{}),
			},
			{
				it: "should parse an ExecutableDefinition with inline and spread Fragments",
				input: `
				query QueryWithFragments {
					hero {
						...heroFields
					}
				}

				fragment heroFields on SuperHero {
					name
					skill
					...on DrivingSuperHero {
						vehicles {
							...vehicleFields
						}
					}
				}

				fragment vehicleFields on Vehicle {
					name
					weapon
				}
				`,
				expectErr: BeNil(),
				expectValues: Equal(document.ExecutableDefinition{
					OperationDefinitions: document.OperationDefinitions{
						{
							Name:          []byte("QueryWithFragments"),
							OperationType: document.OperationTypeQuery,
							SelectionSet: document.SelectionSet{
								document.Field{
									Name: []byte("hero"),
									SelectionSet: document.SelectionSet{
										document.FragmentSpread{
											FragmentName: []byte("heroFields"),
										},
									},
								},
							},
						},
					},
					FragmentDefinitions: document.FragmentDefinitions{
						{
							FragmentName: []byte("heroFields"),
							TypeCondition: document.NamedType{
								Name: []byte("SuperHero"),
							},
							SelectionSet: document.SelectionSet{
								document.Field{
									Name: []byte("name"),
								},
								document.Field{
									Name: []byte("skill"),
								},
								document.InlineFragment{
									TypeCondition: document.NamedType{
										Name: []byte("DrivingSuperHero"),
									},
									SelectionSet: document.SelectionSet{
										document.Field{
											Name: []byte("vehicles"),
											SelectionSet: document.SelectionSet{
												document.FragmentSpread{
													FragmentName: []byte("vehicleFields"),
												},
											},
										},
									},
								},
							},
						},
						{
							FragmentName: []byte("vehicleFields"),
							TypeCondition: document.NamedType{
								Name: []byte("Vehicle"),
							},
							SelectionSet: document.SelectionSet{
								document.Field{
									Name: []byte("name"),
								},
								document.Field{
									Name: []byte("weapon"),
								},
							},
						},
					},
				}),
			},
			{
				it:        "should parse query with escaped line terminators",
				input:     "{\n  hero {\n    id\n    name\n  }\n}\n",
				expectErr: BeNil(),
				expectValues: Equal(document.ExecutableDefinition{
					OperationDefinitions: []document.OperationDefinition{
						{
							OperationType: document.OperationTypeQuery,
							SelectionSet: []document.Selection{
								document.Field{
									Name: []byte("hero"),
									SelectionSet: []document.Selection{
										document.Field{
											Name: []byte("id"),
										},
										document.Field{
											Name: []byte("name"),
										},
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

				parser := NewParser()
				parser.l.SetInput(strings.NewReader(test.input))

				val, err := parser.parseExecutableDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
