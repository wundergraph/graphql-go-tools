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
