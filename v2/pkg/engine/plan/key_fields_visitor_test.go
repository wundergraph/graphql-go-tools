package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
)

func TestKeyFieldPaths(t *testing.T) {
	definitionSDL := `
		type User @key(fields: "id surname") @key(fields: "name info { age }") {
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
		fieldSet      string
		parentPath    string
		expectedPaths []string
	}{
		{
			fieldSet:   "name surname",
			parentPath: "query.me",
			expectedPaths: []string{
				"query.me.name",
				"query.me.surname",
			},
		},
		{
			fieldSet:   "name info { age }",
			parentPath: "query.me.admin",
			expectedPaths: []string{
				"query.me.admin.name",
				"query.me.admin.info",
				"query.me.admin.info.age",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.fieldSet, func(t *testing.T) {
			fieldSet, report := RequiredFieldsFragment("User", c.fieldSet, false)
			require.False(t, report.HasErrors())

			input := &keyVisitorInput{
				typeName:   "User",
				key:        fieldSet,
				definition: &definition,
				report:     report,
				parentPath: c.parentPath,
			}

			keyPaths := keyFieldPaths(input)
			assert.False(t, report.HasErrors())
			assert.Equal(t, c.expectedPaths, keyPaths)
		})
	}
}

func TestKeyInfo(t *testing.T) {

	cases := []struct {
		name        string
		definition  string
		parentPath  string
		typeName    string
		keyFieldSet string

		dataSource      DataSource
		providesEntries []*NodeSuggestion

		expectPaths          []string
		expectExternalFields bool
	}{
		{
			name: "regular key",
			definition: `
				type User @key(fields: "id name") {
					id: ID!
					name: String!
				}`,
			parentPath:  "query.me",
			typeName:    "User",
			keyFieldSet: "id name",
			dataSource: dsb().Hash(22).
				RootNode("User", "id", "name").DS(),
			expectPaths: []string{
				"query.me.id",
				"query.me.name",
			},
			expectExternalFields: false,
		},
		{
			name: "regular key with all fields external",
			definition: `
				type User @key(fields: "id name") {
					id: ID! @external
					name: String! @external
				}`,
			parentPath:  "query.me",
			typeName:    "User",
			keyFieldSet: "id name",
			dataSource: dsb().Hash(22).
				RootNode("User").
				AddRootNodeExternalFieldNames("User", "id", "name").DS(),
			expectPaths:          nil,
			expectExternalFields: true,
		},
		{
			name: "regular key with all fields external, but provided",
			definition: `
				type Query {
					me: User @provides(fields: "id name")
				}
				type User @key(fields: "id name") {
					id: ID! @external
					name: String! @external
				}`,
			parentPath:  "query.me",
			typeName:    "User",
			keyFieldSet: "id name",
			dataSource: dsb().Hash(22).
				RootNode("User").
				AddRootNodeExternalFieldNames("User", "id", "name").DS(),
			providesEntries: []*NodeSuggestion{
				{
					TypeName:  "User",
					FieldName: "id",
					Path:      "query.me.id",
				},
				{
					TypeName:  "User",
					FieldName: "name",
					Path:      "query.me.name",
				},
			},
			expectPaths: []string{
				"query.me.id",
				"query.me.name",
			},
			expectExternalFields: false,
		},
	}

	for _, c := range cases {
		t.Run(c.keyFieldSet, func(t *testing.T) {
			fieldSet, report := RequiredFieldsFragment(c.typeName, c.keyFieldSet, false)
			require.False(t, report.HasErrors())

			definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(c.definition)

			input := &keyVisitorInput{
				typeName:   c.typeName,
				key:        fieldSet,
				definition: &definition,
				report:     report,
				parentPath: c.parentPath,

				dataSource:      c.dataSource,
				providesEntries: c.providesEntries,
			}

			keyPaths, hasExternalFields := keyInfo(input)
			assert.False(t, report.HasErrors())
			assert.Equal(t, c.expectPaths, keyPaths)
			assert.Equal(t, c.expectExternalFields, hasExternalFields)
		})
	}
}
