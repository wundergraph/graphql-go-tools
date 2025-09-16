package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
)

func TestAddRequiredFields(t *testing.T) {
	tests := []struct {
		name string
		// input
		definition                   string
		operation                    string
		typeName                     string
		fieldSet                     string
		isKey                        bool
		allowTypename                bool
		isTypeNameForEntityInterface bool
		selectionSetRef              int
		enforceTypenameForRequired   bool

		// output
		expectedOperation           string
		expectedSkipFieldsCount     int
		expectedRequiredFieldsCount int
		expectedModifiedFieldsCount int
		expectedRemappedPaths       map[string]string
	}{
		{
			name: "simple key",
			definition: `
				type Query {
					user(id: ID!): User
				}
				type User {
					id: ID!
					name: String!
					email: String!
				}`,
			operation: `
				query {
					user(id: "1") {
						name
					}
				}`,
			typeName:        "User",
			fieldSet:        "id",
			isKey:           true,
			selectionSetRef: 0,
			expectedOperation: `
				query {
					user(id: "1") {
						name
						id
					}
				}`,
			expectedSkipFieldsCount:     1,
			expectedRequiredFieldsCount: 1,
		},
		{
			name: "composite key",
			definition: `
				type Query {
					user: User
				}
				type User {
					id: ID!
					email: String!
					name: String!
					info: UserInfo!
				}
				type UserInfo {
					age: Int!
					country: String!
				}`,
			operation: `
				query {
					user {
						name
					}
				}`,
			typeName: "User",
			fieldSet: "id email info { age }",
			isKey:    true,
			expectedOperation: `
				query {
					user {
						name
						id
						email
						info {
							age
						}
					}
				}`,
			expectedSkipFieldsCount:     4, // id, email, info, age
			expectedRequiredFieldsCount: 4,
		},
		{
			name: "requires with 2 fields",
			definition: `
				type Query {
					user: User
				}
				type User {
					id: ID!
					firstName: String!
					lastName: String!
					fullName: String!
				}`,
			operation: `
				query {
					user {
						fullName
					}
				}`,
			typeName: "User",
			fieldSet: "firstName lastName",
			isKey:    false,
			expectedOperation: `
				query {
					user {
						fullName
						firstName
						lastName
					}
				}`,
			expectedSkipFieldsCount:     2,
			expectedRequiredFieldsCount: 2,
		},
		{
			name: "requires with existing field in selection set",
			definition: `
				type Query {
					user: User
				}
				type User {
					id: ID!
					name: String!
					email: String!
				}`,
			operation: `
				query {
					user {
						id
						name
					}
				}`,
			typeName: "User",
			fieldSet: "id",
			isKey:    true,
			expectedOperation: `
				query {
					user {
						id
						name
					}
				}`,
			expectedSkipFieldsCount:     0, // no new fields added
			expectedRequiredFieldsCount: 1, // existing field is marked as required
		},
		{
			name: "requires with conflicting arguments, needs alias",
			definition: `
				type Query {
					user: User
				}
				type User {
					id: ID!
					name: String!
					profile(lang: String!): String!
				}`,
			operation: `
				query {
					user {
						name
						profile(lang: "en")
					}
				}`,
			typeName: "User",
			fieldSet: "profile(lang: \"es\")",
			isKey:    false,
			expectedOperation: `
				query {
					user {
						name
						profile(lang: "en")
						__internal_profile: profile(lang: "es")
					}
				}`,
			expectedSkipFieldsCount:     1,
			expectedRequiredFieldsCount: 1,
			expectedRemappedPaths:       map[string]string{"User.profile": "__internal_profile"},
		},
		{
			name: "key typename addition with allowTypename",
			definition: `
				type Query {
					user: User
				}
				type User {
					id: ID!
					name: String!
				}`,
			operation: `
				query {
					user {
						name
					}
				}`,
			typeName:      "User",
			fieldSet:      "id",
			isKey:         true,
			allowTypename: true,
			expectedOperation: `
				query {
					user {
						name
						__typename
						id
					}
				}`,
			expectedSkipFieldsCount:     2,
			expectedRequiredFieldsCount: 1, // only id is required, __typename is skipped for keys
		},
		{
			name: "requires with nested field",
			definition: `
				type Query {
					user: User
				}
				type User {
					id: ID!
					address: Address!
				}
				type Address {
					street: String!
					city: String!
					zip: String!
				}`,
			operation: `
				query {
					user {
						address {
							city
						}
					}
				}
			`,
			typeName:        "User",
			fieldSet:        "address { street zip }",
			isKey:           false,
			selectionSetRef: 1,
			expectedOperation: `
				query {
					user {
						address {
							city
							street
							zip
						}
					}
				}`,
			expectedSkipFieldsCount:     2, // street, zip
			expectedRequiredFieldsCount: 3,
			expectedModifiedFieldsCount: 1, // address
		},
		{
			name: "requires with inline fragment",
			definition: `
				type Query {
					account: Account
				}
				type Account {
					id: ID!
					node: Node!
				}
				interface Node {
					id: ID!
				}
				type User implements Node {
					id: ID!
					name: String!
				}
				type Admin implements Node {
					id: ID!
					role: String!
				}`,
			operation: `
				query {
					account {
						id
					}
				}`,
			typeName: "Account",
			fieldSet: "node { ... on User { name } ... on Admin { role } }",
			isKey:    false,
			expectedOperation: `
				query {
					account {
						id
						node {
							__typename
							... on User {
								name
							}
							... on Admin {
								role
							}
						}
					}
				}`,
			expectedSkipFieldsCount:     4, // node, __typename, name, role
			expectedRequiredFieldsCount: 3, // node, name, role
		},
		{
			name: "key with complex nested requirements",
			definition: `
				type Query {
					user: User
				}
				type User {
					id: ID!
					account: Account!
					profile: Profile!
				}
				type Account {
					id: ID!
					type: String!
					settings: Settings!
				}
				type Settings {
					theme: String!
					notifications: Boolean!
				}
				type Profile {
					bio: String!
				}`,
			operation: `
				query {
					user {
						profile {
							bio
						}
					}
				}`,
			typeName:        "User",
			fieldSet:        "id account { id type settings { theme } }",
			isKey:           true,
			selectionSetRef: 1,
			expectedOperation: `
				query {
					user {
						profile {
							bio
						}
						id
						account {
							id
							type
							settings {
								theme
							}
						}
					}
				}`,
			expectedSkipFieldsCount:     6, // id, account, id, type, settings, theme
			expectedRequiredFieldsCount: 6,
		},
		{
			name: "key with complex nested requirements and enforced typename",
			definition: `
				type Query {
					user: User
				}
				type User {
					id: ID!
					account: Account!
					profile: Profile!
				}
				type Account {
					id: ID!
					type: String!
					settings: Settings!
				}
				type Settings {
					theme: String!
					notifications: Boolean!
				}
				type Profile {
					bio: String!
				}`,
			operation: `
				query {
					user {
						profile {
							bio
						}
					}
				}`,
			typeName:                   "User",
			fieldSet:                   "id account { id type settings { theme } }",
			isKey:                      true,
			selectionSetRef:            1,
			enforceTypenameForRequired: true, // should not add __typename for keys
			expectedOperation: `
				query {
					user {
						profile {
							bio
						}
						id
						account {
							id
							type
							settings {
								theme
							}
						}
					}
				}`,
			expectedSkipFieldsCount:     6, // id, account, id, type, settings, theme
			expectedRequiredFieldsCount: 6,
		},
		{
			name: "requires with complex nested requirements and enforced typename",
			definition: `
				type Query {
					user: User
				}
				type User {
					id: ID!
					account: Account!
					profile: Profile!
				}
				type Account {
					id: ID!
					type: String!
					settings: Settings!
				}
				type Settings {
					theme: String!
					notifications: Boolean!
				}
				type Profile {
					bio: String!
				}`,
			operation: `
				query {
					user {
						profile {
							bio
						}
					}
				}`,
			typeName:                   "User",
			fieldSet:                   "id account { id type settings { theme } }",
			isKey:                      false,
			selectionSetRef:            1,
			enforceTypenameForRequired: true,
			expectedOperation: `
				query {
					user {
						profile {
							bio
						}
						id
						account {
							__typename
							id
							type
							settings {
								__typename
								theme
							}
						}
					}
				}`,
			expectedSkipFieldsCount:     8, // id, account, __typename, id, type, settings, __typename, theme
			expectedRequiredFieldsCount: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(tt.definition)
			operation := unsafeparser.ParseGraphqlDocumentString(tt.operation)

			config := &addRequiredFieldsConfiguration{
				operation:                    &operation,
				definition:                   &definition,
				operationSelectionSetRef:     tt.selectionSetRef,
				isTypeNameForEntityInterface: tt.isTypeNameForEntityInterface,
				isKey:                        tt.isKey,
				allowTypename:                tt.allowTypename,
				typeName:                     tt.typeName,
				fieldSet:                     tt.fieldSet,
				enforceTypenameForRequired:   tt.enforceTypenameForRequired,
			}

			result, report := addRequiredFields(config)

			require.False(t, report.HasErrors(), "addRequiredFields should not produce errors")

			assert.Equal(t, tt.expectedSkipFieldsCount, len(result.skipFieldRefs),
				"skipFieldRefs count mismatch")
			assert.Equal(t, tt.expectedRequiredFieldsCount, len(result.requiredFieldRefs),
				"requiredFieldRefs count mismatch")
			assert.Equal(t, tt.expectedModifiedFieldsCount, len(result.modifiedFieldRefs),
				"modifiedFieldRefs count mismatch")

			if tt.expectedRemappedPaths != nil {
				assert.Equal(t, tt.expectedRemappedPaths, result.remappedPaths,
					"remappedPaths mismatch")
			}

			actualOperation, err := astprinter.PrintStringIndent(&operation, "  ")
			require.NoError(t, err, "failed to print actual operation")

			// prettified printed operation
			expectedOp := unsafeparser.ParseGraphqlDocumentString(tt.expectedOperation)
			expectedOperation, err := astprinter.PrintStringIndent(&expectedOp, "  ")
			require.NoError(t, err, "failed to print expected operation")

			assert.Equal(t, expectedOperation, actualOperation,
				"operation structure mismatch")
		})
	}
}

func TestRequiredFieldsFragment(t *testing.T) {
	tests := []struct {
		name             string
		typeName         string
		requiredFields   string
		includeTypename  bool
		expectedFragment string
		expectError      bool
	}{
		{
			name:             "with typename",
			typeName:         "User",
			requiredFields:   "id name",
			includeTypename:  true,
			expectedFragment: `fragment Key on User { __typename id name}`,
		},
		{
			name:             "nested fields",
			typeName:         "User",
			requiredFields:   "id info { age country }",
			expectedFragment: `fragment Key on User {id info { age country }}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fragment, report := RequiredFieldsFragment(tt.typeName, tt.requiredFields, tt.includeTypename)

			if tt.expectError {
				assert.True(t, report.HasErrors(), "expected error but got none")
				return
			}

			require.False(t, report.HasErrors(), "unexpected error")
			require.NotNil(t, fragment)

			actualFragment, err := astprinter.PrintString(fragment)
			require.NoError(t, err)

			expectedDoc := unsafeparser.ParseGraphqlDocumentString(tt.expectedFragment)
			expectedFragment, err := astprinter.PrintString(&expectedDoc)
			require.NoError(t, err)

			assert.Equal(t, expectedFragment, actualFragment)
		})
	}
}

func TestQueryPlanRequiredFieldsFragment(t *testing.T) {
	tests := []struct {
		name             string
		fieldName        string
		typeName         string
		requiredFields   string
		expectedFragment string
	}{
		{
			name:             "without field name",
			fieldName:        "",
			typeName:         "User",
			requiredFields:   "id name",
			expectedFragment: `fragment Key on User { __typename id name }`,
		},
		{
			name:             "with field name",
			fieldName:        "fullName",
			typeName:         "User",
			requiredFields:   "firstName lastName",
			expectedFragment: `fragment Requires_for_fullName on User { firstName lastName }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fragment, report := QueryPlanRequiredFieldsFragment(tt.fieldName, tt.typeName, tt.requiredFields)

			require.False(t, report.HasErrors(), "unexpected error")
			require.NotNil(t, fragment)

			actualFragment, err := astprinter.PrintString(fragment)
			require.NoError(t, err)

			expectedDoc := unsafeparser.ParseGraphqlDocumentString(tt.expectedFragment)
			expectedFragment, err := astprinter.PrintString(&expectedDoc)
			require.NoError(t, err)

			assert.Equal(t, expectedFragment, actualFragment)
		})
	}
}
