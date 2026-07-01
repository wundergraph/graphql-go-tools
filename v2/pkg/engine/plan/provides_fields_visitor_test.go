package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
)

// pTypes builds an allowed types set for provides selection assertions
func pTypes(names ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[name] = struct{}{}
	}
	return out
}

func TestProvidesSuggestions(t *testing.T) {
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

	input := &providesInput{
		parentTypeName:       "User",
		providesSelectionSet: `name info {age} address {street zip}`,
		definition:           &definition,
	}

	suggestions, report := providesSuggestions(input)
	require.False(t, report.HasErrors())

	expected := providesSelection{
		"name": {{allowedTypes: pTypes("User")}},
		"info": {{allowedTypes: pTypes("User"), selection: providesSelection{
			"age":        {{allowedTypes: pTypes("Info")}},
			"__typename": {{allowedTypes: pTypes("Info")}},
		}}},
		"address": {{allowedTypes: pTypes("User"), selection: providesSelection{
			"street":     {{allowedTypes: pTypes("Address")}},
			"zip":        {{allowedTypes: pTypes("Address")}},
			"__typename": {{allowedTypes: pTypes("Address")}},
		}}},
		"__typename": {{allowedTypes: pTypes("User")}},
	}

	assert.Equal(t, expected, suggestions)
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

	t.Run("fragments on union", func(t *testing.T) {
		input := &providesInput{
			parentTypeName:       "AB",
			providesSelectionSet: `... on A {a} ... on B {b}`,
			definition:           &definition,
		}

		suggestions, report := providesSuggestions(input)
		require.False(t, report.HasErrors())

		expected := providesSelection{
			"a": {{allowedTypes: pTypes("A")}},
			"b": {{allowedTypes: pTypes("B")}},
			"__typename": {
				{allowedTypes: pTypes("A")},
				{allowedTypes: pTypes("B")},
				{allowedTypes: pTypes("AB", "A", "B")},
			},
		}

		assert.Equal(t, expected, suggestions)
	})

	t.Run("nested fragments on union", func(t *testing.T) {
		input := &providesInput{
			parentTypeName:       "NestedAB",
			providesSelectionSet: `ab { ... on A {a} ... on B {b} }`,
			definition:           &definition,
		}

		suggestions, report := providesSuggestions(input)
		require.False(t, report.HasErrors())

		expected := providesSelection{
			"ab": {{allowedTypes: pTypes("NestedAB"), selection: providesSelection{
				"a": {{allowedTypes: pTypes("A")}},
				"b": {{allowedTypes: pTypes("B")}},
				"__typename": {
					{allowedTypes: pTypes("A")},
					{allowedTypes: pTypes("B")},
					{allowedTypes: pTypes("AB", "A", "B")},
				},
			}}},
			"__typename": {{allowedTypes: pTypes("NestedAB")}},
		}

		assert.Equal(t, expected, suggestions)
	})
}

func TestProvidesSuggestionsOnInterfaceSelections(t *testing.T) {
	definitionSDL := `
		type Query {
			media: Media
		}

		interface Media {
			id: ID!
			animals: [Animal]
		}

		type Book implements Media {
			id: ID!
			animals: [Animal]
		}

		interface Animal {
			id: ID!
			name: String
		}

		type Cat implements Animal {
			id: ID!
			name: String
		}

		type Dog implements Animal {
			id: ID!
			name: String
		}`

	definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(definitionSDL)

	input := &providesInput{
		parentTypeName:       "Media",
		providesSelectionSet: `animals { id name }`,
		definition:           &definition,
	}

	suggestions, report := providesSuggestions(input)
	require.False(t, report.HasErrors())

	// fields on abstract types are allowed for the abstract type itself and every
	// implementer, so the selection matches the query both before and after
	// abstract to concrete rewriting
	expected := providesSelection{
		"animals": {{allowedTypes: pTypes("Media", "Book"), selection: providesSelection{
			"id":         {{allowedTypes: pTypes("Animal", "Cat", "Dog")}},
			"name":       {{allowedTypes: pTypes("Animal", "Cat", "Dog")}},
			"__typename": {{allowedTypes: pTypes("Animal", "Cat", "Dog")}},
		}}},
		"__typename": {{allowedTypes: pTypes("Media", "Book")}},
	}

	assert.Equal(t, expected, suggestions)
}

func TestProvidesSuggestionsNestedConcreteFragmentsStayPinned(t *testing.T) {
	definitionSDL := `
		type Query {
			f: I1
		}

		interface I1 {
			i2: I2
		}

		type A1 implements I1 {
			i2: I2
		}

		type B1 implements I1 {
			i2: I2
		}

		interface I2 {
			i3: I3
		}

		type A2 implements I2 {
			i3: I3
		}

		type B2 implements I2 {
			i3: I3
		}

		interface I3 {
			x: String
		}

		type A3 implements I3 {
			x: String
		}

		type B3 implements I3 {
			x: String
		}`

	definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(definitionSDL)

	input := &providesInput{
		parentTypeName:       "I1",
		providesSelectionSet: `i2 { ... on A2 { i3 { ... on A3 { x } } } }`,
		definition:           &definition,
	}

	suggestions, report := providesSuggestions(input)
	require.False(t, report.HasErrors())

	// i2 sits on the abstract I1 - allowed for I1 and each implementer
	i2Selection, ok := suggestions.providedTypeSelection("i2", "I1")
	require.True(t, ok)
	_, ok = suggestions.providedTypeSelection("i2", "A1")
	assert.True(t, ok)
	_, ok = suggestions.providedTypeSelection("i2", "B1")
	assert.True(t, ok)

	// i3 is pinned by the inline fragment to A2 - B2 must not match
	i3Selection, ok := i2Selection.providedTypeSelection("i3", "A2")
	require.True(t, ok)
	_, ok = i2Selection.providedTypeSelection("i3", "B2")
	assert.False(t, ok, "i3 under B2 is not promised by the provides selection")
	_, ok = i2Selection.providedTypeSelection("i3", "I2")
	assert.False(t, ok, "i3 on the abstract I2 is not promised, provides pinned it to A2")

	// x is pinned to A3 - B3 must not match
	_, ok = i3Selection.providedTypeSelection("x", "A3")
	assert.True(t, ok)
	_, ok = i3Selection.providedTypeSelection("x", "B3")
	assert.False(t, ok, "x under B3 is not promised by the provides selection")
}
