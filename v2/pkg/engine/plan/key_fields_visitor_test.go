package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestKeyInfo(t *testing.T) {

	cases := []struct {
		name       string
		definition string
		parentPath string
		typeName   string

		dataSource      DataSource
		providesEntries []*NodeSuggestion

		expectPaths          []KeyInfoFieldPath
		expectExternalFields bool
	}{
		{
			name:       "composite key with nested fields",
			parentPath: "query.me.admin",
			typeName:   "User",
			dataSource: dsb().Hash(22).
				RootNode("User", "name", "surname", "info").
				ChildNode("Info", "age", "weight").
				KeysMetadata(FederationFieldConfigurations{
					FederationFieldConfiguration{
						TypeName:     "User",
						SelectionSet: "name info { age }",
					},
				}).DS(),
			definition: `
				type User @key(fields: "name info { age }") {
					name: String!
					surname: String!
					info: Info!
				}
		
				type Info {
					age: Int!
					weight: Int!
				}`,
			expectPaths: []KeyInfoFieldPath{
				{Path: "query.me.admin.name"},
				{Path: "query.me.admin.info"},
				{Path: "query.me.admin.info.age"},
			},
		},
		{
			name:       "composite key with nested childs fields are external",
			parentPath: "query.me.admin",
			typeName:   "User",
			dataSource: dsb().Hash(22).
				RootNode("User", "name", "surname", "info").
				ChildNode("Info").
				AddChildNodeExternalFieldNames("Info", "age", "nested").
				ChildNode("NestedInfo").
				AddChildNodeExternalFieldNames("NestedInfo", "weight").
				KeysMetadata(FederationFieldConfigurations{
					FederationFieldConfiguration{
						TypeName:     "User",
						SelectionSet: "name info { age nested {weight}}",
					},
				}).DS(),
			definition: `
				type User @key(fields: "name info { age nested {weight}}") {
					name: String!
					surname: String!
					info: Info! @external
				}
		
				type Info {
					age: Int! @external
					nested: NestedInfo! @external
				}
				
				type NestedInfo {
					weight: Int! @external
				}
				`,
			/*
				Composition will give us User.info - as not external because it is used in key
				But at the same time we have info.age - which is external in configuration
				For the given key path it should not be external
			*/
			expectPaths: []KeyInfoFieldPath{
				{Path: "query.me.admin.name"},
				{Path: "query.me.admin.info"},
				{Path: "query.me.admin.info.age"},
				{Path: "query.me.admin.info.nested"},
				{Path: "query.me.admin.info.nested.weight"},
			},
		},
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
					FederationFieldConfiguration{
						TypeName:     "User",
						SelectionSet: "id name",
					},
				}).DS(),
			expectPaths: []KeyInfoFieldPath{
				{Path: "query.me.id"},
				{Path: "query.me.name"},
			},
			expectExternalFields: false,
		},
		{
			name: "regular key with all fields - really external",
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
					FederationFieldConfiguration{
						TypeName:     "User",
						SelectionSet: "id name",
					},
				}).DS(),
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
			parentPath: "query.me",
			typeName:   "User",
			dataSource: dsb().Hash(22).
				RootNode("User").
				AddRootNodeExternalFieldNames("User", "id", "name").
				KeysMetadata(FederationFieldConfigurations{
					FederationFieldConfiguration{
						TypeName:     "User",
						SelectionSet: "id name",
					},
				}).DS(),
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

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key := tc.dataSource.FederationConfiguration().Keys[0]
			report := operationreport.NewReport()

			definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(tc.definition)

			input := &keyVisitorInput{
				typeName:   tc.typeName,
				key:        key.parsedSelectionSet,
				definition: &definition,
				report:     report,
				parentPath: tc.parentPath,

				dataSource:      tc.dataSource,
				providesEntries: tc.providesEntries,
			}

			keyPaths, hasExternalFields := getKeyPaths(input)
			assert.False(t, report.HasErrors())
			assert.Equal(t, tc.expectPaths, keyPaths)
			assert.Equal(t, tc.expectExternalFields, hasExternalFields)
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
