package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
)

func TestProvidesSuggestions(t *testing.T) {
	keySDL := `name info { age }`
	definitionSDL := `
		type User {
			name: String!
			info: Info!
		}

		type Info {
			age: Int!
		}`

	key, report := RequiredFieldsFragment("User", keySDL, false)
	assert.False(t, report.HasErrors())

	definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(definitionSDL)

	input := &providesInput{
		key:        key,
		definition: &definition,
		report:     report,
		parentPath: "query.me",
		DSHash:     2023,
	}

	suggestions := providesSuggestions(input)

	assert.Equal(t, []NodeSuggestion{
		{
			TypeName:       "User",
			FieldName:      "name",
			DataSourceHash: 2023,
			Path:           "query.me.name",
			ParentPath:     "query.me",
		},
		{
			TypeName:       "User",
			FieldName:      "info",
			DataSourceHash: 2023,
			Path:           "query.me.info",
			ParentPath:     "query.me",
		},
		{
			TypeName:       "Info",
			FieldName:      "age",
			DataSourceHash: 2023,
			Path:           "query.me.info.age",
			ParentPath:     "query.me.info",
		},
	}, suggestions)
}
