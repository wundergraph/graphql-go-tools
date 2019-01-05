package parser

/*func TestSelectionParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseSelection", func() {

		tests := []struct {
			it                      string
			input                   string
			expectErr               types.GomegaMatcher
			expectSelectionSet      types.GomegaMatcher
			expectParsedDefinitions types.GomegaMatcher
		}{
			{
				it:        "should parse an InlineFragment",
				input:     "...on Land",
				expectErr: BeNil(),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Fields:               document.Fields{},
					FragmentSpreads:      document.FragmentSpreads{},
					FragmentDefinitions:  document.FragmentDefinitions{},
					VariableDefinitions:  document.VariableDefinitions{},
					OperationDefinitions: document.OperationDefinitions{},
					InlineFragments: document.InlineFragments{
						{
							TypeCondition: document.NamedType{
								Name: "Land",
							},
						},
					},
				}),
				expectSelectionSet: Equal(document.SelectionSet{
					InlineFragments: []int{0},
					FragmentSpreads: []int{},
					Fields:          []int{},
				}),
			},
			{
				it:        "should parse a simple Field",
				input:     "originalName",
				expectErr: BeNil(),
				expectSelectionSet: Equal(document.SelectionSet{
					Fields:          []int{0},
					FragmentSpreads: []int{},
					InlineFragments: []int{},
				}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					OperationDefinitions: document.OperationDefinitions{},
					VariableDefinitions:  document.VariableDefinitions{},
					FragmentDefinitions:  document.FragmentDefinitions{},
					FragmentSpreads:      document.FragmentSpreads{},
					InlineFragments:      document.InlineFragments{},
					Fields: document.Fields{
						{
							Name: "originalName",
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
				}),
			},
			{
				it:        "should parse a nested selection",
				input:     `t { kind name ofType { kind name ofType { kind name } } }`,
				expectErr: BeNil(),
				expectSelectionSet: Equal(document.SelectionSet{
					Fields:          []int{8},
					InlineFragments: []int{},
					FragmentSpreads: []int{},
				}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InlineFragments:      document.InlineFragments{},
					FragmentSpreads:      document.FragmentSpreads{},
					FragmentDefinitions:  document.FragmentDefinitions{},
					VariableDefinitions:  document.VariableDefinitions{},
					OperationDefinitions: document.OperationDefinitions{},
					Fields: document.Fields{
						{
							Name: "kind",
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
						{
							Name: "name",
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
						{
							Name: "kind",
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
						{
							Name: "name",
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
						{
							Name: "kind",
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
						{
							Name: "name",
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
						{
							Name: "ofType",
							SelectionSet: document.SelectionSet{
								Fields:          []int{4, 5},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
						{
							Name: "ofType",
							SelectionSet: document.SelectionSet{
								Fields:          []int{2, 3, 6},
								InlineFragments: []int{},
								FragmentSpreads: []int{},
							},
						},
						{
							Name: "t",
							SelectionSet: document.SelectionSet{
								Fields:          []int{0, 1, 7},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
				}),
			},
			{
				it:        "should parse a simple Field with an argument",
				input:     "originalName(isSet: true)",
				expectErr: BeNil(),
				expectSelectionSet: Equal(document.SelectionSet{
					Fields:          []int{0},
					InlineFragments: []int{},
					FragmentSpreads: []int{},
				}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					OperationDefinitions: document.OperationDefinitions{},
					VariableDefinitions:  document.VariableDefinitions{},
					FragmentDefinitions:  document.FragmentDefinitions{},
					FragmentSpreads:      document.FragmentSpreads{},
					InlineFragments:      document.InlineFragments{},
					Fields: document.Fields{
						{
							Name: "originalName",
							Arguments: document.Arguments{
								document.Argument{
									Name: "isSet",
									Value: document.Value{
										ValueType:    document.ValueTypeBoolean,
										BooleanValue: true,
									},
								},
							},
							SelectionSet: document.SelectionSet{
								Fields:          []int{},
								FragmentSpreads: []int{},
								InlineFragments: []int{},
							},
						},
					},
				}),
			},
			{
				it:        "should parse a FragmentSpread",
				input:     "...Land",
				expectErr: BeNil(),
				expectSelectionSet: Equal(document.SelectionSet{
					FragmentSpreads: []int{0},
					InlineFragments: []int{},
					Fields:          []int{},
				}),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					InlineFragments:      document.InlineFragments{},
					Fields:               document.Fields{},
					OperationDefinitions: document.OperationDefinitions{},
					VariableDefinitions:  document.VariableDefinitions{},
					FragmentDefinitions:  document.FragmentDefinitions{},
					FragmentSpreads: document.FragmentSpreads{
						{
							FragmentName: "Land",
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

				var set document.SelectionSet
				err := parser.parseSelection(&set)
				Expect(err).To(test.expectErr)
				if test.expectSelectionSet != nil {
					Expect(set).To(test.expectSelectionSet)
				}
				if test.expectParsedDefinitions != nil {
					Expect(parser.ParsedDefinitions).To(test.expectParsedDefinitions)
				}
			})
		}
	})
}

var parseSelectionBenchmarkInput = `t { kind name ofType { kind name ofType { kind name } } }`

func BenchmarkParseSelection(b *testing.B) {

	parser := NewParser()

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {

		parser.l.SetInput(parseSelectionBenchmarkInput)
		var set document.SelectionSet
		err := parser.parseSelection(&set)
		if err != nil {
			b.Fatal(err)
		}
	}
}
*/
