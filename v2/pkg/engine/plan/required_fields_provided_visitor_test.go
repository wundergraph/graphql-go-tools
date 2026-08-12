package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
)

func TestAreRequiredFieldsProvided(t *testing.T) {
	definitionSDL := `
		type User {
			id: ID!
			name: String!
			username: String!
			address: Address
			thing: Thing
		}

		type Address {
			street: String!
			zip: String!
		}

		type A {
			a: String!
		}

		type B {
			b: String!
		}

		union Thing = A | B

		type Query {
			me: User
		}
	`
	definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(definitionSDL)

	// each providedSelection mimics what providesSuggestions builds from the
	// @provides fields string annotated on the case; __typename entries that
	// providesSuggestions auto-adds are omitted for brevity
	cases := []struct {
		name              string
		typeName          string
		requiredFields    string
		providedSelection providesSelection
		expected          bool
		datasource        DataSource
	}{
		{
			name:           "all fields provided",
			typeName:       "User",
			requiredFields: "id name",
			// @provides(fields: "id name")
			providedSelection: providesSelection{
				"id":   {{allowedTypes: pTypes("User")}},
				"name": {{allowedTypes: pTypes("User")}},
			},
			expected: true,
		},
		{
			name:           "one field missing",
			typeName:       "User",
			requiredFields: "id name",
			// @provides(fields: "id")
			providedSelection: providesSelection{
				"id": {{allowedTypes: pTypes("User")}},
			},
			expected: false,
		},
		{
			name:           "nested fields provided",
			typeName:       "User",
			requiredFields: "address { street }",
			// @provides(fields: "address { street }")
			providedSelection: providesSelection{
				"address": {{allowedTypes: pTypes("User"), selection: providesSelection{
					"street": {{allowedTypes: pTypes("Address")}},
				}}},
			},
			expected: true,
		},
		{
			name:           "one nested field missing - missing field is external",
			typeName:       "User",
			requiredFields: "address { street zip }",
			// @provides(fields: "address { street }")
			providedSelection: providesSelection{
				"address": {{allowedTypes: pTypes("User"), selection: providesSelection{
					"street": {{allowedTypes: pTypes("Address")}},
				}}},
			},
			expected: false,
			datasource: dsb().
				ChildNode("User", "address").
				ChildNode("Address", "street").
				AddChildNodeExternalFieldNames("Address", "zip").
				DS(),
		},
		{
			// case of implicitly provided field, due to provided parent
			name:           "one nested field missing - missing field is not external",
			typeName:       "User",
			requiredFields: "address { street zip }",
			// @provides(fields: "address { street }")
			providedSelection: providesSelection{
				"address": {{allowedTypes: pTypes("User"), selection: providesSelection{
					"street": {{allowedTypes: pTypes("Address")}},
				}}},
			},
			expected: true,
			datasource: dsb().
				ChildNode("User", "address").
				ChildNode("Address", "street", "zip").
				DS(),
		},
		{
			name:           "deeply nested fields provided",
			typeName:       "User",
			requiredFields: "address { street zip }",
			// @provides(fields: "address { street zip }")
			providedSelection: providesSelection{
				"address": {{allowedTypes: pTypes("User"), selection: providesSelection{
					"street": {{allowedTypes: pTypes("Address")}},
					"zip":    {{allowedTypes: pTypes("Address")}},
				}}},
			},
			expected: true,
		},
		{
			name:           "requires with field name",
			typeName:       "User",
			requiredFields: "name",
			// @provides(fields: "name")
			providedSelection: providesSelection{
				"name": {{allowedTypes: pTypes("User")}},
			},
			expected: true,
		},
		{
			name:           "no provided fields",
			typeName:       "User",
			requiredFields: "id",
			// no @provides directive - nothing is provided
			providedSelection: providesSelection{},
			expected:          false,
		},
		{
			name:           "nested fragments (union)",
			typeName:       "User",
			requiredFields: "thing { ... on A { a } ... on B { b } }",
			// @provides(fields: "thing { ... on A { a } ... on B { b } }")
			providedSelection: providesSelection{
				"thing": {{allowedTypes: pTypes("User"), selection: providesSelection{
					"a": {{allowedTypes: pTypes("A")}},
					"b": {{allowedTypes: pTypes("B")}},
				}}},
			},
			expected: true,
		},
		{
			name:           "nested fragments (union) - missing B",
			typeName:       "User",
			requiredFields: "thing { ... on A { a } ... on B { b } }",
			// @provides(fields: "thing { ... on A { a } }")
			providedSelection: providesSelection{
				"thing": {{allowedTypes: pTypes("User"), selection: providesSelection{
					"a": {{allowedTypes: pTypes("A")}},
				}}},
			},
			expected: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			input := areRequiredFieldsProvidedInput{
				typeName:          c.typeName,
				requiredFields:    c.requiredFields,
				definition:        &definition,
				providedSelection: c.providedSelection,
				dataSource:        dsb().DS(),
			}
			if c.datasource != nil {
				input.dataSource = c.datasource
			}

			result, report := areRequiredFieldsProvided(input)
			require.False(t, report.HasErrors())
			assert.Equal(t, c.expected, result)
		})
	}
}
