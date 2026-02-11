package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
)

func TestProvidesSuggestions(t *testing.T) {
	keySDL := `name info {age} address {street zip}`
	definitionSDL := `
		type Query {
			me: User! @provides(fields: "name info {age} address {street zip}")
		}

		type User {
			name: String!
			surname: String!
			info: Info!
			address: Address!
		}

		type Info {
			age: Int!
			weight: Int!
		}

		type Address {
			city: String!
			street: String!
			zip: String!
		}`

	definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(definitionSDL)

	cases := []struct {
		selectionSetRef int
		expected        map[string]struct{}
	}{
		{
			selectionSetRef: 2,
			expected: map[string]struct{}{
				"User|name|query.me.name":                        {},
				"User|info|query.me.info":                        {},
				"Info|age|query.me.info.age":                     {},
				"Info|__typename|query.me.info.__typename":       {},
				"User|address|query.me.address":                  {},
				"Address|street|query.me.address.street":         {},
				"Address|zip|query.me.address.zip":               {},
				"Address|__typename|query.me.address.__typename": {},
				"User|__typename|query.me.__typename":            {},
			},
		},
	}

	for _, c := range cases {
		t.Run(keySDL, func(t *testing.T) {
			meta := &DataSourceMetadata{
				RootNodes: []TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"me"},
					},
					{
						TypeName:           "User",
						FieldNames:         []string{"address"},
						ExternalFieldNames: []string{"name", "info"},
					},

					{
						TypeName:           "Address",
						ExternalFieldNames: []string{"street", "zip"},
					},
				},
				ChildNodes: []TypeField{
					{
						TypeName:           "Info",
						ExternalFieldNames: []string{"age"},
					},
				},
			}
			meta.InitNodesIndex()

			input := &providesInput{
				parentTypeName:       "User",
				providesSelectionSet: keySDL,
				definition:           &definition,
				parentPath:           "query.me",
			}

			suggestions, report := providesSuggestions(input)
			assert.False(t, report.HasErrors())
			assert.Equal(t, c.expected, suggestions)
		})
	}
}

func TestProvidesSuggestionsWithFragments(t *testing.T) {
	definitionSDL := `
		type Query {
			ab: AB! @provides(fields: "... on A {a} ... on B {b}")
			nestedAB: NestedAB! @provides(fields: "ab { ... on A {a} ... on B {b} }")
		}

		type NestedAB {
			ab: AB!
		}

		type A {
			a: String!
			b: String!
		}

		type B {
			a: String!
			b: String!
		}

		union AB = A | B
	`

	definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(definitionSDL)

	cases := []struct {
		selectionSetRef      int
		parentPath           string
		fieldTypeName        string
		providesSelectionSet string
		expected             map[string]struct{}
	}{
		{
			selectionSetRef:      1,
			parentPath:           "query.ab",
			fieldTypeName:        "AB",
			providesSelectionSet: `... on A {a} ... on B {b}`,
			expected: map[string]struct{}{
				"A|a|query.ab.a":                    {},
				"A|__typename|query.ab.__typename":  {},
				"B|b|query.ab.b":                    {},
				"B|__typename|query.ab.__typename":  {},
				"AB|__typename|query.ab.__typename": {},
			},
		},
		{
			selectionSetRef:      2,
			parentPath:           "query.nestedAB.ab",
			fieldTypeName:        "AB",
			providesSelectionSet: `... on A {a} ... on B {b}`,
			expected: map[string]struct{}{
				"A|a|query.nestedAB.ab.a":                    {},
				"A|__typename|query.nestedAB.ab.__typename":  {},
				"B|b|query.nestedAB.ab.b":                    {},
				"B|__typename|query.nestedAB.ab.__typename":  {},
				"AB|__typename|query.nestedAB.ab.__typename": {},
			},
		},
		{
			selectionSetRef:      1,
			parentPath:           "query.nestedAB",
			fieldTypeName:        "NestedAB",
			providesSelectionSet: `ab { ... on A {a} ... on B {b} }`,
			expected: map[string]struct{}{
				"NestedAB|ab|query.nestedAB.ab":                 {},
				"A|a|query.nestedAB.ab.a":                       {},
				"A|__typename|query.nestedAB.ab.__typename":     {},
				"B|b|query.nestedAB.ab.b":                       {},
				"B|__typename|query.nestedAB.ab.__typename":     {},
				"AB|__typename|query.nestedAB.ab.__typename":    {},
				"NestedAB|__typename|query.nestedAB.__typename": {},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.providesSelectionSet, func(t *testing.T) {
			meta := &DataSourceMetadata{
				RootNodes: []TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"ab", "nestedAB"},
					},
					{
						TypeName:           "A",
						FieldNames:         []string{"a"},
						ExternalFieldNames: []string{"b"},
					},
					{
						TypeName:           "B",
						FieldNames:         []string{"b"},
						ExternalFieldNames: []string{"a"},
					},
					{
						TypeName:   "NestedAB",
						FieldNames: []string{"ab"},
					},
				},
			}
			meta.InitNodesIndex()

			input := &providesInput{
				parentTypeName:       c.fieldTypeName,
				providesSelectionSet: c.providesSelectionSet,
				definition:           &definition,
				parentPath:           c.parentPath,
			}

			suggestions, report := providesSuggestions(input)
			assert.False(t, report.HasErrors())
			assert.Equal(t, c.expected, suggestions)
		})
	}
}
