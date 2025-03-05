package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
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
	fieldSet, report := providesFragment("User", keySDL, &definition)
	assert.False(t, report.HasErrors())

	cases := []struct {
		selectionSetRef int
		expected        []*NodeSuggestion
	}{
		{
			selectionSetRef: 2,
			expected: []*NodeSuggestion{
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "User",
					FieldName:      "name",
					DataSourceHash: 2023,
					Path:           "query.me.name",
					ParentPath:     "query.me",
				},
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "User",
					FieldName:      "info",
					DataSourceHash: 2023,
					Path:           "query.me.info",
					ParentPath:     "query.me",
				},
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "Info",
					FieldName:      "age",
					DataSourceHash: 2023,
					Path:           "query.me.info.age",
					ParentPath:     "query.me.info",
				},
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "Info",
					FieldName:      "__typename",
					DataSourceHash: 2023,
					Path:           "query.me.info.__typename",
					ParentPath:     "query.me.info",
				},
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "User",
					FieldName:      "address",
					DataSourceHash: 2023,
					Path:           "query.me.address",
					ParentPath:     "query.me",
				},
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "Address",
					FieldName:      "street",
					DataSourceHash: 2023,
					Path:           "query.me.address.street",
					ParentPath:     "query.me.address",
				},
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "Address",
					FieldName:      "zip",
					DataSourceHash: 2023,
					Path:           "query.me.address.zip",
					ParentPath:     "query.me.address",
				},
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "Address",
					FieldName:      "__typename",
					DataSourceHash: 2023,
					Path:           "query.me.address.__typename",
					ParentPath:     "query.me.address",
				},
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "User",
					FieldName:      "__typename",
					DataSourceHash: 2023,
					Path:           "query.me.__typename",
					ParentPath:     "query.me",
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(keySDL, func(t *testing.T) {
			report := &operationreport.Report{}

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

			ds := &dataSourceConfiguration[string]{
				hash:               2023,
				DataSourceMetadata: meta,
			}

			input := &providesInput{
				operationSelectionSet: c.selectionSetRef,
				providesFieldSet:      fieldSet,
				definition:            &definition,
				report:                report,
				parentPath:            "query.me",
				dataSource:            ds,
			}

			suggestions := providesSuggestions(input)
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
		expected             []*NodeSuggestion
	}{
		{
			selectionSetRef:      1,
			parentPath:           "query.ab",
			fieldTypeName:        "AB",
			providesSelectionSet: `... on A {a} ... on B {b}`,
			expected: []*NodeSuggestion{
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "A",
					FieldName:      "a",
					DataSourceHash: 2023,
					Path:           "query.ab.a",
					ParentPath:     "query.ab",
				},
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "A",
					FieldName:      "__typename",
					DataSourceHash: 2023,
					Path:           "query.ab.__typename",
					ParentPath:     "query.ab",
				},
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "B",
					FieldName:      "b",
					DataSourceHash: 2023,
					Path:           "query.ab.b",
					ParentPath:     "query.ab",
				},
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "B",
					FieldName:      "__typename",
					DataSourceHash: 2023,
					Path:           "query.ab.__typename",
					ParentPath:     "query.ab",
				},
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "AB",
					FieldName:      "__typename",
					DataSourceHash: 2023,
					Path:           "query.ab.__typename",
					ParentPath:     "query.ab",
				},
			},
		},
		{
			selectionSetRef:      1,
			parentPath:           "query.nestedAB",
			fieldTypeName:        "NestedAB",
			providesSelectionSet: `ab { ... on A {a} ... on B {b} }`,
			expected: []*NodeSuggestion{
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "NestedAB",
					FieldName:      "ab",
					DataSourceHash: 2023,
					Path:           "query.nestedAB.ab",
					ParentPath:     "query.nestedAB",
				},
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "A",
					FieldName:      "a",
					DataSourceHash: 2023,
					Path:           "query.nestedAB.ab.a",
					ParentPath:     "query.nestedAB.ab",
				},
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "A",
					FieldName:      "__typename",
					DataSourceHash: 2023,
					Path:           "query.nestedAB.ab.__typename",
					ParentPath:     "query.nestedAB.ab",
				},
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "B",
					FieldName:      "b",
					DataSourceHash: 2023,
					Path:           "query.nestedAB.ab.b",
					ParentPath:     "query.nestedAB.ab",
				},
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "B",
					FieldName:      "__typename",
					DataSourceHash: 2023,
					Path:           "query.nestedAB.ab.__typename",
					ParentPath:     "query.nestedAB.ab",
				},
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "AB",
					FieldName:      "__typename",
					DataSourceHash: 2023,
					Path:           "query.nestedAB.ab.__typename",
					ParentPath:     "query.nestedAB.ab",
				},
				{
					FieldRef:       ast.InvalidRef,
					TypeName:       "NestedAB",
					FieldName:      "__typename",
					DataSourceHash: 2023,
					Path:           "query.nestedAB.__typename",
					ParentPath:     "query.nestedAB",
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.providesSelectionSet, func(t *testing.T) {
			report := &operationreport.Report{}

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

			ds := &dataSourceConfiguration[string]{
				hash:               2023,
				DataSourceMetadata: meta,
			}

			fieldSet, report := providesFragment(c.fieldTypeName, c.providesSelectionSet, &definition)
			assert.False(t, report.HasErrors())

			input := &providesInput{
				operationSelectionSet: c.selectionSetRef,
				providesFieldSet:      fieldSet,
				definition:            &definition,
				report:                report,
				parentPath:            c.parentPath,
				dataSource:            ds,
			}

			suggestions := providesSuggestions(input)
			assert.False(t, report.HasErrors())
			assert.Equal(t, c.expected, suggestions)
		})
	}
}
