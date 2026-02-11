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

	cases := []struct {
		name           string
		typeName       string
		requiredFields string
		parentPath     string
		providedFields map[string]struct{}
		expected       bool
		datasource     DataSource
	}{
		{
			name:           "all fields provided",
			typeName:       "User",
			requiredFields: "id name",
			parentPath:     "query.me",
			providedFields: map[string]struct{}{
				"User|id|query.me.id":     {},
				"User|name|query.me.name": {},
			},
			expected: true,
		},
		{
			name:           "one field missing",
			typeName:       "User",
			requiredFields: "id name",
			parentPath:     "query.me",
			providedFields: map[string]struct{}{
				"User|id|query.me.id": {},
			},
			expected: false,
		},
		{
			name:           "nested fields provided",
			typeName:       "User",
			requiredFields: "address { street }",
			parentPath:     "query.me",
			providedFields: map[string]struct{}{
				"User|address|query.me.address":          {},
				"Address|street|query.me.address.street": {},
			},
			expected: true,
		},
		{
			name:           "one nested field missing - missing field is external",
			typeName:       "User",
			requiredFields: "address { street zip }",
			parentPath:     "query.me",
			providedFields: map[string]struct{}{
				"User|address|query.me.address":          {},
				"Address|street|query.me.address.street": {},
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
			parentPath:     "query.me",
			providedFields: map[string]struct{}{
				"User|address|query.me.address":          {},
				"Address|street|query.me.address.street": {},
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
			parentPath:     "query.me",
			providedFields: map[string]struct{}{
				"User|address|query.me.address":          {},
				"Address|street|query.me.address.street": {},
				"Address|zip|query.me.address.zip":       {},
			},
			expected: true,
		},
		{
			name:           "requires with field name",
			typeName:       "User",
			requiredFields: "name",
			parentPath:     "query.me",
			providedFields: map[string]struct{}{
				"User|name|query.me.name": {},
			},
			expected: true,
		},
		{
			name:           "no provided fields",
			typeName:       "User",
			requiredFields: "id",
			parentPath:     "query.me",
			providedFields: map[string]struct{}{},
			expected:       false,
		},
		{
			name:           "nested fragments (union)",
			typeName:       "User",
			requiredFields: "thing { ... on A { a } ... on B { b } }",
			parentPath:     "query.me",
			providedFields: map[string]struct{}{
				"User|thing|query.me.thing": {},
				"A|a|query.me.thing.a":      {},
				"B|b|query.me.thing.b":      {},
			},
			expected: true,
		},
		{
			name:           "nested fragments (union) - missing B",
			typeName:       "User",
			requiredFields: "thing { ... on A { a } ... on B { b } }",
			parentPath:     "query.me",
			providedFields: map[string]struct{}{
				"User|thing|query.me.thing": {},
				"A|a|query.me.thing.a":      {},
			},
			expected: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			input := areRequiredFieldsProvidedInput{
				typeName:       c.typeName,
				requiredFields: c.requiredFields,
				definition:     &definition,
				providedFields: c.providedFields,
				parentPath:     c.parentPath,
				dataSource:     dsb().DS(),
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
