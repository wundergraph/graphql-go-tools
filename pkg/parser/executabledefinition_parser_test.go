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
			it                         string
			input                      string
			expectErr                  types.GomegaMatcher
			expectExecutableDefinition types.GomegaMatcher
			expectParsedDefinitions    types.GomegaMatcher
		}{
			{
				it: "should parse a simple ExecutableDefinition with OperationDefinition",
				input: `
				query allGophers($color: String)@rename(index: 3) {
					name
				}`,
				expectErr: BeNil(),
				expectExecutableDefinition: Equal(document.ExecutableDefinition{
					OperationDefinitions: []int{0},
					FragmentDefinitions:  []int{},
				}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Directives: document.Directives{
						{
							Name:      "rename",
							Arguments: []int{0},
						},
					},
					Arguments: document.Arguments{
						{
							Name: "index",
							Value: document.Value{
								ValueType: document.ValueTypeInt,
								IntValue:  3,
							},
						},
					},
					VariableDefinitions: document.VariableDefinitions{
						{
							Variable: "color",
							Type: document.NamedType{
								Name: "String",
							},
						},
					},
					OperationDefinitions: document.OperationDefinitions{
						{
							OperationType:       document.OperationTypeQuery,
							Name:                "allGophers",
							VariableDefinitions: []int{0},
							Directives:          []int{0},
							SelectionSet: document.SelectionSet{
								Fields:          []int{0},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
					Fields: document.Fields{
						{
							Name:       "name",
							Directives: []int{},
							Arguments:  []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
				}.initEmptySlices()),
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
				expectExecutableDefinition: Equal(document.ExecutableDefinition{
					OperationDefinitions: []int{0},
					FragmentDefinitions:  []int{0},
				}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Directives: document.Directives{
						{
							Name:      "rename",
							Arguments: []int{0},
						},
					},
					Arguments: document.Arguments{
						{
							Name: "index",
							Value: document.Value{
								ValueType: document.ValueTypeInt,
								IntValue:  3,
							},
						},
					},
					OperationDefinitions: document.OperationDefinitions{
						{
							Name:                "Q1",
							OperationType:       document.OperationTypeQuery,
							VariableDefinitions: []int{},
							Directives:          []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{1},
								InlineFragments: []int{},
								FragmentSpreads: []int{},
							},
						},
					},
					FragmentDefinitions: document.FragmentDefinitions{
						{
							FragmentName: "MyFragment",
							TypeCondition: document.NamedType{
								Name: "SomeType",
							},
							Directives: []int{0},
							SelectionSet: document.SelectionSet{
								Fields:          []int{0},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
					Fields: document.Fields{
						{
							Name:       "name",
							Directives: []int{},
							Arguments:  []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
						{
							Name:       "foo",
							Directives: []int{},
							Arguments:  []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
				}.initEmptySlices()),
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
				expectExecutableDefinition: Equal(document.ExecutableDefinition{
					OperationDefinitions: []int{0, 1},
					FragmentDefinitions:  []int{},
				}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					OperationDefinitions: document.OperationDefinitions{
						{
							OperationType:       document.OperationTypeQuery,
							Name:                "allGophers",
							VariableDefinitions: []int{0},
							Directives:          []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{0},
								InlineFragments: []int{},
								FragmentSpreads: []int{},
							},
						},
						{
							OperationType:       document.OperationTypeQuery,
							Name:                "allGophinas",
							VariableDefinitions: []int{1},
							Directives:          []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{1},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
					VariableDefinitions: document.VariableDefinitions{
						{
							Variable: "color",
							Type: document.NamedType{
								Name: "String",
							},
						},
						{
							Variable: "color",
							Type: document.NamedType{
								Name: "String",
							},
						},
					},
					Fields: document.Fields{
						{
							Name:       "name",
							Directives: []int{},
							Arguments:  []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
						{
							Name:       "name",
							Directives: []int{},
							Arguments:  []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
				}.initEmptySlices()),
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
				expectExecutableDefinition: Equal(document.ExecutableDefinition{
					OperationDefinitions: []int{0, 1},
					FragmentDefinitions:  []int{0},
				}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					FragmentSpreads: document.FragmentSpreads{},
					InlineFragments: document.InlineFragments{},
					Arguments: document.Arguments{
						{
							Name: "index",
							Value: document.Value{
								ValueType: document.ValueTypeInt,
								IntValue:  3,
							},
						},
					},
					Directives: document.Directives{
						{
							Name:      "rename",
							Arguments: []int{0},
						},
					},
					OperationDefinitions: document.OperationDefinitions{
						{
							OperationType:       document.OperationTypeQuery,
							Name:                "allGophers",
							VariableDefinitions: []int{0},
							Directives:          []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{0},
								InlineFragments: []int{},
								FragmentSpreads: []int{},
							},
						},
						{
							OperationType:       document.OperationTypeQuery,
							Name:                "allGophinas",
							VariableDefinitions: []int{1},
							Directives:          []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{2},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
					VariableDefinitions: document.VariableDefinitions{
						{
							Variable: "color",
							Type: document.NamedType{
								Name: "String",
							},
						},
						{
							Variable: "color",
							Type: document.NamedType{
								Name: "String",
							},
						},
					},
					FragmentDefinitions: document.FragmentDefinitions{
						{
							FragmentName: "MyFragment",
							TypeCondition: document.NamedType{
								Name: "SomeType",
							},
							Directives: []int{0},
							SelectionSet: document.SelectionSet{
								Fields:          []int{1},
								InlineFragments: []int{},
								FragmentSpreads: []int{},
							},
						},
					},
					Fields: document.Fields{
						{
							Name:       "name",
							Directives: []int{},
							Arguments:  []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
						{
							Name:       "name",
							Directives: []int{},
							Arguments:  []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
						{
							Name:       "name",
							Directives: []int{},
							Arguments:  []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it: "should not parse a ExecutableDefinition without a known identifier",
				input: `
				Barry allGophers($color: String)@rename(index: 3) {
					name
				}`,
				expectErr: HaveOccurred(),
				expectExecutableDefinition: Equal(document.ExecutableDefinition{
					FragmentDefinitions:  []int{},
					OperationDefinitions: []int{},
				}),
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
				expectExecutableDefinition: Equal(document.ExecutableDefinition{
					OperationDefinitions: []int{0},
					FragmentDefinitions:  []int{0, 1},
				}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					VariableDefinitions: document.VariableDefinitions{},
					OperationDefinitions: document.OperationDefinitions{
						{
							Name:                "QueryWithFragments",
							OperationType:       document.OperationTypeQuery,
							Directives:          []int{},
							VariableDefinitions: []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{0},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
					FragmentDefinitions: document.FragmentDefinitions{
						{
							FragmentName: "heroFields",
							TypeCondition: document.NamedType{
								Name: "SuperHero",
							},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{1, 2},
								InlineFragments: []int{0},
								FragmentSpreads: []int{},
							},
						},
						{
							FragmentName: "vehicleFields",
							TypeCondition: document.NamedType{
								Name: "Vehicle",
							},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{4, 5},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
					Fields: document.Fields{
						{
							Name:       "hero",
							Arguments:  []int{},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								FragmentSpreads: []int{0},
								InlineFragments: []int{},
								Fields:          []int{},
							},
						},
						{
							Name:       "name",
							Arguments:  []int{},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
						{
							Name:       "skill",
							Arguments:  []int{},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
						{
							Name:       "vehicles",
							Arguments:  []int{},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								FragmentSpreads: []int{1},
								Fields:          []int{},
								InlineFragments: []int{},
							},
						},
						{
							Name:       "name",
							Arguments:  []int{},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
						{
							Name:       "weapon",
							Arguments:  []int{},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
					FragmentSpreads: document.FragmentSpreads{
						{
							FragmentName: "heroFields",
							Directives:   []int{},
						},
						{
							FragmentName: "vehicleFields",
							Directives:   []int{},
						},
					},
					InlineFragments: document.InlineFragments{
						{
							TypeCondition: document.NamedType{
								Name: "DrivingSuperHero",
							},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{3},
								InlineFragments: []int{},
								FragmentSpreads: []int{},
							},
						},
					},
				}.initEmptySlices()),
			},
			{
				it:        "should parse query with escaped line terminators",
				input:     "{\n  hero {\n    id\n    name\n  }\n}\n",
				expectErr: BeNil(),
				expectExecutableDefinition: Equal(document.ExecutableDefinition{
					OperationDefinitions: []int{0},
					FragmentDefinitions:  []int{},
				}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					OperationDefinitions: document.OperationDefinitions{
						{
							OperationType:       document.OperationTypeQuery,
							Directives:          []int{},
							VariableDefinitions: []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{2},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
					Fields: document.Fields{
						{
							Name:       "id",
							Arguments:  []int{},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
						{
							Name:       "name",
							Arguments:  []int{},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
						{
							Name:       "hero",
							Arguments:  []int{},
							Directives: []int{},
							SelectionSet: document.SelectionSet{
								Fields:          []int{0, 1},
								InlineFragments: []int{},
								FragmentSpreads: []int{},
							},
						},
					},
				}.initEmptySlices()),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				val, err := parser.parseExecutableDefinition()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectExecutableDefinition)
				if test.expectParsedDefinitions != nil {
					Expect(parser.ParsedDefinitions).To(test.expectParsedDefinitions)
				}
			})
		}
	})
}
