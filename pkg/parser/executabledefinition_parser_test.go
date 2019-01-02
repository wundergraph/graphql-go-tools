package parser

import (
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
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
							Name:          "allGophers",
							VariableDefinitions: document.VariableDefinitions{
								{
									Variable: "color",
									Type: document.NamedType{
										Name: "String",
									},
								},
							},
							Directives: document.Directives{
								document.Directive{
									Name: "rename",
									Arguments: document.Arguments{
										document.Argument{
											Name: "index",
											Value: document.IntValue{
												Val: 3,
											},
										},
									},
								},
							},
							SelectionSet: document.SelectionSet{
								document.Field{
									Name: "name",
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
							Name:          "Q1",
							OperationType: document.OperationTypeQuery,
							SelectionSet: []document.Selection{
								document.Field{
									Name: "foo",
								},
							},
						},
					},
					FragmentDefinitions: document.FragmentDefinitions{
						{
							FragmentName: "MyFragment",
							TypeCondition: document.NamedType{
								Name: "SomeType",
							},
							Directives: document.Directives{
								document.Directive{
									Name: "rename",
									Arguments: document.Arguments{
										document.Argument{
											Name: "index",
											Value: document.IntValue{
												Val: 3,
											},
										},
									},
								},
							},
							SelectionSet: document.SelectionSet{
								document.Field{
									Name: "name",
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
							Name:          "allGophers",
							VariableDefinitions: document.VariableDefinitions{
								{
									Variable: "color",
									Type: document.NamedType{
										Name: "String",
									},
								},
							},
							SelectionSet: document.SelectionSet{
								document.Field{
									Name: "name",
								},
							},
						},
						{
							OperationType: document.OperationTypeQuery,
							Name:          "allGophinas",
							VariableDefinitions: document.VariableDefinitions{
								{
									Variable: "color",
									Type: document.NamedType{
										Name: "String",
									},
								},
							},
							SelectionSet: document.SelectionSet{
								document.Field{
									Name: "name",
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
							Name:          "allGophers",
							VariableDefinitions: document.VariableDefinitions{
								{
									Variable: "color",
									Type: document.NamedType{
										Name: "String",
									},
								},
							},
							SelectionSet: document.SelectionSet{
								document.Field{
									Name: "name",
								},
							},
						},
						{
							OperationType: document.OperationTypeQuery,
							Name:          "allGophinas",
							VariableDefinitions: document.VariableDefinitions{
								{
									Variable: "color",
									Type: document.NamedType{
										Name: "String",
									},
								},
							},
							SelectionSet: document.SelectionSet{
								document.Field{
									Name: "name",
								},
							},
						},
					},
					FragmentDefinitions: document.FragmentDefinitions{
						{
							FragmentName: "MyFragment",
							TypeCondition: document.NamedType{
								Name: "SomeType",
							},
							Directives: document.Directives{
								document.Directive{
									Name: "rename",
									Arguments: document.Arguments{
										document.Argument{
											Name: "index",
											Value: document.IntValue{
												Val: 3,
											},
										},
									},
								},
							},
							SelectionSet: document.SelectionSet{
								document.Field{
									Name: "name",
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
							Name:          "QueryWithFragments",
							OperationType: document.OperationTypeQuery,
							SelectionSet: document.SelectionSet{
								document.Field{
									Name: "hero",
									SelectionSet: document.SelectionSet{
										document.FragmentSpread{
											FragmentName: "heroFields",
										},
									},
								},
							},
						},
					},
					FragmentDefinitions: document.FragmentDefinitions{
						{
							FragmentName: "heroFields",
							TypeCondition: document.NamedType{
								Name: "SuperHero",
							},
							SelectionSet: document.SelectionSet{
								document.Field{
									Name: "name",
								},
								document.Field{
									Name: "skill",
								},
								document.InlineFragment{
									TypeCondition: document.NamedType{
										Name: "DrivingSuperHero",
									},
									SelectionSet: document.SelectionSet{
										document.Field{
											Name: "vehicles",
											SelectionSet: document.SelectionSet{
												document.FragmentSpread{
													FragmentName: "vehicleFields",
												},
											},
										},
									},
								},
							},
						},
						{
							FragmentName: "vehicleFields",
							TypeCondition: document.NamedType{
								Name: "Vehicle",
							},
							SelectionSet: document.SelectionSet{
								document.Field{
									Name: "name",
								},
								document.Field{
									Name: "weapon",
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
									Name: "hero",
									SelectionSet: []document.Selection{
										document.Field{
											Name: "id",
										},
										document.Field{
											Name: "name",
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
				parser.l.SetInput(test.input)

				val, err := parser.parseExecutableDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
