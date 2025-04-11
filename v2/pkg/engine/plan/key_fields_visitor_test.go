package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
)

func TestKeyFieldPaths(t *testing.T) {
	definitionSDL := `
		type User @key(fields: "name surname") @key(fields: "name info { age }") {
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

	dataSource := dsb().Hash(22).
		RootNode("User", "name", "surname", "info", "address").
		ChildNode("Info", "age", "weight").
		ChildNode("Address", "city", "street", "zip").
		DS()

	definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(definitionSDL)

	cases := []struct {
		fieldSet      string
		parentPath    string
		expectedPaths []KeyInfoFieldPath
	}{
		{
			fieldSet:   "name surname",
			parentPath: "query.me",
			expectedPaths: []KeyInfoFieldPath{
				{Path: "query.me.name"},
				{Path: "query.me.surname"},
			},
		},
		{
			fieldSet:   "name info { age }",
			parentPath: "query.me.admin",
			expectedPaths: []KeyInfoFieldPath{
				{Path: "query.me.admin.name"},
				{Path: "query.me.admin.info"},
				{Path: "query.me.admin.info.age"},
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
				dataSource: dataSource,
			}

			keyPaths, _ := getKeyPaths(input)
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

		expectPaths          []KeyInfoFieldPath
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
			expectPaths: []KeyInfoFieldPath{
				{Path: "query.me.id"},
				{Path: "query.me.name"},
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
			expectPaths: []KeyInfoFieldPath{
				{Path: "query.me.id", IsExternal: true},
				{Path: "query.me.name", IsExternal: true},
			},
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
			expectPaths: []KeyInfoFieldPath{
				{Path: "query.me.id"},
				{Path: "query.me.name"},
			},
			expectExternalFields: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
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

			keyPaths, hasExternalFields := getKeyPaths(input)
			assert.False(t, report.HasErrors())
			assert.Equal(t, c.expectPaths, keyPaths)
			assert.Equal(t, c.expectExternalFields, hasExternalFields)
		})
	}
}

func TestCollectKeysForPath(t *testing.T) {

	cases := []struct {
		name       string
		definition string
		parentPath string
		typeName   string

		dataSource      DataSource
		providesEntries []*NodeSuggestion

		expectKeys []DSKeyInfo
	}{
		{
			name: "regular key",
			definition: `
				type User @key(fields: "id name") {
					id: ID!
					name: String!
				}`,
			parentPath: "query.me",
			typeName:   "User",
			dataSource: dsb().Hash(22).
				RootNode("User", "id", "name").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id name",
					},
				}).
				DS(),
			expectKeys: []DSKeyInfo{
				{
					DSHash:   22,
					TypeName: "User",
					Path:     "query.me",
					Keys: []KeyInfo{
						{
							DSHash:       22,
							Source:       true,
							Target:       true,
							TypeName:     "User",
							SelectionSet: "id name",
							FieldPaths: []KeyInfoFieldPath{
								{Path: "query.me.id"},
								{Path: "query.me.name"},
							},
						},
					},
				},
			},
		},
		{
			name: "regular key with all fields external",
			definition: `
				type User @key(fields: "id name") {
					id: ID! @external
					name: String! @external
				}`,
			parentPath: "query.me",
			typeName:   "User",
			dataSource: dsb().Hash(22).
				RootNode("User").
				AddRootNodeExternalFieldNames("User", "id", "name").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id name",
					},
				}).
				DS(),
			expectKeys: []DSKeyInfo{
				{
					DSHash:   22,
					TypeName: "User",
					Path:     "query.me",
					Keys: []KeyInfo{
						{
							DSHash:       22,
							Source:       false,
							Target:       true,
							TypeName:     "User",
							SelectionSet: "id name",
							FieldPaths: []KeyInfoFieldPath{
								{Path: "query.me.id", IsExternal: true},
								{Path: "query.me.name", IsExternal: true},
							},
						},
					},
				},
			},
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
			parentPath: "query.me",
			typeName:   "User",
			dataSource: dsb().Hash(22).
				RootNode("User").
				AddRootNodeExternalFieldNames("User", "id", "name").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id name",
					},
				}).
				DS(),
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
			expectKeys: []DSKeyInfo{
				{
					DSHash:   22,
					TypeName: "User",
					Path:     "query.me",
					Keys: []KeyInfo{
						{
							DSHash:       22,
							Source:       true,
							Target:       true,
							TypeName:     "User",
							SelectionSet: "id name",
							FieldPaths: []KeyInfoFieldPath{
								{Path: "query.me.id"},
								{Path: "query.me.name"},
							},
						},
					},
				},
			},
		},
		{
			name: "regular key with all fields external - target only",
			definition: `
				type Query {
					me: User
				}
				type User @key(fields: "id name") {
					id: ID! @external
					name: String! @external
				}`,
			parentPath: "query.me",
			typeName:   "User",
			dataSource: dsb().Hash(22).
				RootNode("User").
				AddRootNodeExternalFieldNames("User", "id", "name").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:     "User",
						SelectionSet: "id name",
					},
				}).
				DS(),
			expectKeys: []DSKeyInfo{
				{
					DSHash:   22,
					TypeName: "User",
					Path:     "query.me",
					Keys: []KeyInfo{
						{
							DSHash:       22,
							Source:       false,
							Target:       true,
							TypeName:     "User",
							SelectionSet: "id name",
							FieldPaths: []KeyInfoFieldPath{
								{Path: "query.me.id", IsExternal: true},
								{Path: "query.me.name", IsExternal: true},
							},
						},
					},
				},
			},
		},
		{
			name: "resolvable false key",
			definition: `
				type Query {
					me: User
				}
				type User @key(fields: "id name", resolvable: false) {
					id: ID!
					name: String!
				}`,
			parentPath: "query.me",
			typeName:   "User",
			dataSource: dsb().Hash(22).
				RootNode("User", "id", "name").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:              "User",
						SelectionSet:          "id name",
						DisableEntityResolver: true,
					},
				}).
				DS(),
			expectKeys: []DSKeyInfo{
				{
					DSHash:   22,
					TypeName: "User",
					Path:     "query.me",
					Keys: []KeyInfo{
						{
							DSHash:       22,
							Source:       true,
							Target:       false,
							TypeName:     "User",
							SelectionSet: "id name",
							FieldPaths: []KeyInfoFieldPath{
								{Path: "query.me.id"},
								{Path: "query.me.name"},
							},
						},
					},
				},
			},
		},
		{
			name: "resolvable false all fields external - not usable key",
			definition: `
				type Query {
					me: User
				}
				type User @key(fields: "id name", resolvable: false) {
					id: ID! @external
					name: String! @external
				}`,
			parentPath: "query.me",
			typeName:   "User",
			dataSource: dsb().Hash(22).
				RootNode("User").
				AddRootNodeExternalFieldNames("User", "id", "name").
				KeysMetadata(FederationFieldConfigurations{
					{
						TypeName:              "User",
						SelectionSet:          "id name",
						DisableEntityResolver: true,
					},
				}).
				DS(),
			expectKeys: []DSKeyInfo{
				{
					DSHash:   22,
					TypeName: "User",
					Path:     "query.me",
					Keys:     []KeyInfo{},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(c.definition)

			collectNodesVisitor := &collectNodesVisitor{
				definition:          &definition,
				dataSource:          c.dataSource,
				providesEntries:     c.providesEntries,
				keys:                make([]DSKeyInfo, 0, 2),
				localSeenKeys:       make(map[SeenKeyPath]struct{}),
				notExternalKeyPaths: make(map[string]struct{}),
			}

			collectNodesVisitor.collectKeysForPath(c.typeName, c.parentPath)
			// call it again to test the deduplication
			collectNodesVisitor.collectKeysForPath(c.typeName, c.parentPath)

			assert.Equal(t, len(c.expectKeys), len(collectNodesVisitor.keys))
			assert.Equal(t, c.expectKeys, collectNodesVisitor.keys)
		})
	}
}
