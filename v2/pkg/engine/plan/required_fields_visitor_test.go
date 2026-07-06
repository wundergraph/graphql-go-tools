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
		deferInfo                    *DeferInfo
		parentFieldDeferID           int

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
		{
			name: "key with defer id - new field added as plain (no alias needed)",
			definition: `
				type Query { user: User }
				type User { id: ID! name: String! }`,
			operation: `query { user { name } }`,
			typeName:  "User",
			fieldSet:  "id",
			isKey:     true,
			deferInfo: &DeferInfo{ID: 1},
			expectedOperation: `
				query {
					user {
						name
						id
					}
				}`,
			expectedSkipFieldsCount:     1,
			expectedRequiredFieldsCount: 1,
			expectedRemappedPaths:       map[string]string{},
		},
		{
			name: "key with defer id - existing plain field is reused (no alias)",
			definition: `
				type Query { user: User }
				type User { id: ID! name: String! }`,
			operation: `query { user { id name } }`,
			typeName:  "User",
			fieldSet:  "id",
			isKey:     true,
			deferInfo: &DeferInfo{ID: 1},
			expectedOperation: `
				query {
					user {
						id
						name
					}
				}`,
			expectedSkipFieldsCount:     0,
			expectedRequiredFieldsCount: 1,
			expectedRemappedPaths:       map[string]string{},
		},
		{
			name: "requires with defer id - new field gets aliased",
			definition: `
				type Query { user: User }
				type User { id: ID! firstName: String! lastName: String! fullName: String! }`,
			operation: `query { user { fullName } }`,
			typeName:  "User",
			fieldSet:  "firstName lastName",
			isKey:     false,
			deferInfo: &DeferInfo{ID: 1},
			expectedOperation: `
				query {
					user {
						fullName
						__internal_firstName: firstName @__defer_internal(id: 1)
						__internal_lastName: lastName @__defer_internal(id: 1)
					}
				}`,
			expectedSkipFieldsCount:     2,
			expectedRequiredFieldsCount: 2,
			expectedRemappedPaths: map[string]string{
				"User.firstName": "__internal_firstName",
				"User.lastName":  "__internal_lastName",
			},
		},
		{
			name: "requires with defer id - existing field still gets aliased",
			definition: `
				type Query { user: User }
				type User { id: ID! firstName: String! fullName: String! }`,
			operation: `query { user { firstName fullName } }`,
			typeName:  "User",
			fieldSet:  "firstName",
			isKey:     false,
			deferInfo: &DeferInfo{ID: 1},
			expectedOperation: `
				query {
					user {
						firstName
						fullName
						__internal_firstName: firstName @__defer_internal(id: 1)
					}
				}`,
			expectedSkipFieldsCount:     1,
			expectedRequiredFieldsCount: 1,
			expectedRemappedPaths:       map[string]string{"User.firstName": "__internal_firstName"},
		},
		{
			name: "key with defer id - existing plain nested field is reused, leaf added inside",
			definition: `
				type Query { user: User }
				type User { id: ID! address: Address! }
				type Address { street: String! city: String! }`,
			operation:       `query { user { address { city } } }`,
			typeName:        "User",
			fieldSet:        "address { street }",
			isKey:           true,
			deferInfo:       &DeferInfo{ID: 1},
			selectionSetRef: 1,
			// existing plain address is reused; street is added into it
			expectedOperation: `
				query {
					user {
						address {
							city
							street
						}
					}
				}`,
			expectedSkipFieldsCount:     1, // street
			expectedRequiredFieldsCount: 2, // address (reused) + street
			expectedModifiedFieldsCount: 1, // address selection set was modified
			expectedRemappedPaths:       map[string]string{},
		},
		{
			name: "key with defer id and parentId - plain field added with directive",
			definition: `
				type Query { user: User }
				type User { id: ID! name: String! }`,
			operation:          `query { user { name } }`,
			typeName:           "User",
			fieldSet:           "id",
			isKey:              true,
			deferInfo:          &DeferInfo{ID: 2, ParentID: 2},
			parentFieldDeferID: 1,
			expectedOperation: `
				query {
					user {
						name
						id @__defer_internal(id: 1)
					}
				}`,
			expectedSkipFieldsCount:     1,
			expectedRequiredFieldsCount: 1,
			expectedRemappedPaths:       map[string]string{},
		},
		{
			name: "requires with defer id and parentId - directive added with all fields",
			definition: `
				type Query { user: User }
				type User { id: ID! firstName: String! fullName: String! }`,
			operation: `query { user { fullName } }`,
			typeName:  "User",
			fieldSet:  "firstName",
			isKey:     false,
			deferInfo: &DeferInfo{ID: 2, Label: "myLabel", ParentID: 1},
			expectedOperation: `
				query {
					user {
						fullName
						__internal_firstName: firstName @__defer_internal(id: 2, label: "myLabel", parentDeferId: 1)
					}
				}`,
			expectedSkipFieldsCount:     1,
			expectedRequiredFieldsCount: 1,
			expectedRemappedPaths:       map[string]string{"User.firstName": "__internal_firstName"},
		},
		{
			name: "key with defer id and parentId - existing plain nested reused, leaf gets directive",
			definition: `
				type Query { user: User }
				type User { id: ID! address: Address! }
				type Address { street: String! city: String! }`,
			operation:          `query { user { address { city } } }`,
			typeName:           "User",
			fieldSet:           "address { street }",
			isKey:              true,
			deferInfo:          &DeferInfo{ID: 2, ParentID: 1},
			parentFieldDeferID: 1,
			selectionSetRef:    1,
			// existing plain address reused; street added with @deferInternal
			expectedOperation: `
				query {
					user {
						address {
							city
							street @__defer_internal(id: 1)
						}
					}
				}`,
			expectedSkipFieldsCount:     1, // street
			expectedRequiredFieldsCount: 2, // address (reused) + street
			expectedModifiedFieldsCount: 1, // address modified
			expectedRemappedPaths:       map[string]string{},
		},
		{
			name: "requires with defer id and parentId - directive added to nested fields too",
			definition: `
				type Query { user: User }
				type User { id: ID! address: Address! fullAddress: String! }
				type Address { street: String! city: String! }`,
			operation: `query { user { fullAddress } }`,
			typeName:  "User",
			fieldSet:  "address { street }",
			isKey:     false,
			deferInfo: &DeferInfo{ID: 2, ParentID: 1},
			expectedOperation: `
				query {
					user {
						fullAddress
						__internal_address: address @__defer_internal(id: 2, parentDeferId: 1) {
							street @__defer_internal(id: 2, parentDeferId: 1)
						}
					}
				}`,
			expectedSkipFieldsCount:     2,
			expectedRequiredFieldsCount: 2,
			expectedModifiedFieldsCount: 0,
			expectedRemappedPaths:       map[string]string{"User.address": "__internal_address"},
		},
		{
			name: "key - existing field has defer_internal, non-deferred requirement gets aliased",
			definition: `
				type Query { user: User }
				type User { id: ID! name: String! }`,
			operation: `query { user { id @__defer_internal(id: 1) name } }`,
			typeName:  "User",
			fieldSet:  "id",
			isKey:     true,
			deferInfo: nil,
			expectedOperation: `
				query {
					user {
						id @__defer_internal(id: 1)
						name
						__internal_id: id
					}
				}`,
			expectedSkipFieldsCount:     1,
			expectedRequiredFieldsCount: 1,
			expectedRemappedPaths:       map[string]string{"User.id": "__internal_id"},
		},
		{
			name: "requires - existing field has defer_internal, non-deferred requirement gets aliased",
			definition: `
				type Query { user: User }
				type User { id: ID! firstName: String! fullName: String! }`,
			operation: `query { user { firstName @__defer_internal(id: 1) fullName } }`,
			typeName:  "User",
			fieldSet:  "firstName",
			isKey:     false,
			deferInfo: nil,
			expectedOperation: `
				query {
					user {
						firstName @__defer_internal(id: 1)
						fullName
						__internal_firstName: firstName
					}
				}`,
			expectedSkipFieldsCount:     1,
			expectedRequiredFieldsCount: 1,
			expectedRemappedPaths:       map[string]string{"User.firstName": "__internal_firstName"},
		},
		{
			name: "key - nested field has defer_internal, non-deferred requirement gets aliased",
			definition: `
				type Query { user: User }
				type User { id: ID! address: Address! }
				type Address { street: String! city: String! }`,
			operation:       `query { user { address { street @__defer_internal(id: 1) city } } }`,
			typeName:        "User",
			fieldSet:        "address { street }",
			isKey:           true,
			deferInfo:       nil,
			selectionSetRef: 1,
			expectedOperation: `
				query {
					user {
						address {
							street @__defer_internal(id: 1)
							city
							__internal_street: street
						}
					}
				}`,
			expectedSkipFieldsCount:     1,
			expectedRequiredFieldsCount: 2, // address (reused) + __internal_street
			expectedModifiedFieldsCount: 1,
			expectedRemappedPaths:       map[string]string{"User.address.street": "__internal_street"},
		},
		{
			name: "requires - nested field has defer_internal, non-deferred requirement gets aliased",
			definition: `
				type Query { user: User }
				type User { id: ID! address: Address! fullAddress: String! }
				type Address { street: String! city: String! }`,
			operation:       `query { user { address { street @__defer_internal(id: 1) city } fullAddress } }`,
			typeName:        "User",
			fieldSet:        "address { street }",
			isKey:           false,
			deferInfo:       nil,
			selectionSetRef: 1,
			expectedOperation: `
				query {
					user {
						address {
							street @__defer_internal(id: 1)
							city
							__internal_street: street
						}
						fullAddress
					}
				}`,
			expectedSkipFieldsCount:     1,
			expectedRequiredFieldsCount: 2, // address (reused) + __internal_street
			expectedModifiedFieldsCount: 1,
			expectedRemappedPaths:       map[string]string{"User.address.street": "__internal_street"},
		},
		{
			name: "requires with defer id - second call with same defer id reuses existing alias",
			definition: `
				type Query { user: User }
				type User { id: ID! settings: Settings! fullName: String! account: Account! }
				type Settings { region: String! }
				type Account { type: String! }`,
			// operation already has __internal_settings from a prior addRequiredFields call;
			// nested region also carries the defer directive
			operation:       `query { user { fullName __internal_settings: settings @__defer_internal(id: 1) { region @__defer_internal(id: 1) } account } }`,
			typeName:        "User",
			fieldSet:        "settings { region }",
			isKey:           false,
			selectionSetRef: 1,
			deferInfo:       &DeferInfo{ID: 1},
			// __internal_settings already exists with same defer scope — reuse it; no new field added
			expectedOperation: `
				query {
					user {
						fullName
						__internal_settings: settings @__defer_internal(id: 1) { region @__defer_internal(id: 1) }
						account
					}
				}`,
			expectedSkipFieldsCount:     0,
			expectedRequiredFieldsCount: 2, // reused settings ref + reused region ref (nested non-deferred path)
			expectedRemappedPaths:       map[string]string{"User.settings": "__internal_settings"},
		},
		{
			name: "requires with defer id - existing alias from different defer scope gets defer-id alias",
			definition: `
				type Query { user: User }
				type User { id: ID! settings: Settings! fullName: String! account: Account! }
				type Settings { region: String! }
				type Account { type: String! }`,
			// operation has __internal_settings belonging to defer scope "1" with directive on nested field too
			operation:       `query { user { fullName __internal_settings: settings @__defer_internal(id: 1) { region @__defer_internal(id: 1) } account } }`,
			typeName:        "User",
			fieldSet:        "settings { region }",
			isKey:           false,
			selectionSetRef: 1, // user's inner selection set; ref 0 is the pre-seeded settings' inner selection set
			deferInfo:       &DeferInfo{ID: 2},
			// __internal_settings exists but belongs to defer "1"; no __internal_2_settings yet — create it
			expectedOperation: `
				query {
					user {
						fullName
						__internal_settings: settings @__defer_internal(id: 1) { region @__defer_internal(id: 1) }
						account
						__internal_2_settings: settings @__defer_internal(id: 2) { region @__defer_internal(id: 2) }
					}
				}`,
			expectedSkipFieldsCount:     2, // __internal_2_settings + nested region
			expectedRequiredFieldsCount: 2,
			expectedRemappedPaths:       map[string]string{"User.settings": "__internal_2_settings"},
		},
		{
			name: "requires with inline fragments in deferred context - enforce typename and assign defer directive",
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
			typeName:  "Account",
			fieldSet:  "node { ... on User { name } ... on Admin { role } }",
			isKey:     false,
			deferInfo: &DeferInfo{ID: 1},
			// addTypenameSelection now calls applyDeferInternalDirective so the
			// auto-added __typename (triggered by inline fragments) carries @__defer_internal
			expectedOperation: `
				query {
					account {
						id
						__internal_node: node @__defer_internal(id: 1) {
							__typename @__defer_internal(id: 1)
							... on User {
								name @__defer_internal(id: 1)
							}
							... on Admin {
								role @__defer_internal(id: 1)
							}
						}
					}
				}`,
			expectedSkipFieldsCount:     4, // __internal_node alias, __typename, name, role
			expectedRequiredFieldsCount: 3, // node, name, role (__typename not stored)
			expectedRemappedPaths:       map[string]string{"Account.node": "__internal_node"},
		},
		{
			name: "requires with addTypenameInNestedSelections no fragments, but typenames are enforced",
			definition: `
				type Query {
					user: User
				}
				type User {
					id: ID!
					account: Account!
					fullAccount: String!
				}
				type Account {
					id: ID!
					type: String!
				}`,
			operation: `
				query {
					user {
						fullAccount
					}
				}`,
			typeName:                   "User",
			fieldSet:                   "account { id }",
			isKey:                      false,
			deferInfo:                  &DeferInfo{ID: 1},
			enforceTypenameForRequired: true,
			expectedOperation: `
				query {
					user {
						fullAccount
						__internal_account: account @__defer_internal(id: 1) {
							__typename @__defer_internal(id: 1)
							id @__defer_internal(id: 1)
						}
					}
				}`,
			expectedSkipFieldsCount:     3, // __internal_account alias, __typename, id
			expectedRequiredFieldsCount: 2, // account, id (__typename not stored)
			expectedRemappedPaths:       map[string]string{"User.account": "__internal_account"},
		},
		{
			name: "requires with addTypenameInNestedSelections no fragments, typenames are not enforced",
			definition: `
				type Query {
					user: User
				}
				type User {
					id: ID!
					account: Account!
					fullAccount: String!
				}
				type Account {
					id: ID!
					type: String!
				}`,
			operation: `
				query {
					user {
						fullAccount
					}
				}`,
			typeName:  "User",
			fieldSet:  "account { id }",
			isKey:     false,
			deferInfo: &DeferInfo{ID: 1},
			expectedOperation: `
				query {
					user {
						fullAccount
						__internal_account: account @__defer_internal(id: 1) {
							id @__defer_internal(id: 1)
						}
					}
				}`,
			expectedSkipFieldsCount:     2, // __internal_account alias, id
			expectedRequiredFieldsCount: 2, // account, id (__typename not stored)
			expectedRemappedPaths:       map[string]string{"User.account": "__internal_account"},
		},
		{
			name: "requires with inline fragments and explicit typename in fieldSet in deferred context - do not add duplicated typename",
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
			typeName:  "Account",
			fieldSet:  "node { __typename ... on User { name } ... on Admin { role } }",
			isKey:     false,
			deferInfo: &DeferInfo{ID: 1},
			expectedOperation: `
				query {
					account {
						id
						__internal_node: node @__defer_internal(id: 1) {
							__typename @__defer_internal(id: 1)
							... on User {
								name @__defer_internal(id: 1)
							}
							... on Admin {
								role @__defer_internal(id: 1)
							}
						}
					}
				}`,
			expectedSkipFieldsCount:     4, // __internal_node alias, __typename, name, role
			expectedRequiredFieldsCount: 3, // node, name, role (__typename not stored)
			expectedRemappedPaths:       map[string]string{"Account.node": "__internal_node"},
		},
		{
			name: "key with defer id - second planner with different defer id but same parent defer id reuses existing alias",
			definition: `
				type Query { user: User }
				type User { id: ID! name: String! }`,
			// operation pre-seeded: plain id is deferred (from prior entity planner),
			// plus __internal_id already created by a prior key planner
			// (deferInfo.ID="1", parentFieldDeferID="1")
			operation:          `query { user { id @__defer_internal(id: 1) name __internal_id: id @__defer_internal(id: 1) } }`,
			typeName:           "User",
			fieldSet:           "id",
			isKey:              true,
			deferInfo:          &DeferInfo{ID: 3},
			parentFieldDeferID: 1,
			// effectiveDeferID = parentFieldDeferID = "1" matches __internal_id's directive → reuse it
			expectedOperation:           `query { user { id @__defer_internal(id: 1) name __internal_id: id @__defer_internal(id: 1) } }`,
			expectedSkipFieldsCount:     0,
			expectedRequiredFieldsCount: 1,
			expectedRemappedPaths:       map[string]string{"User.id": "__internal_id"},
		},
		{
			name: "key with defer id - third planner with yet another defer id but same parent defer id reuses existing alias",
			definition: `
				type Query { user: User }
				type User { id: ID! name: String! }`,
			operation:                   `query { user { id @__defer_internal(id: 1) name __internal_id: id @__defer_internal(id: 1) } }`,
			typeName:                    "User",
			fieldSet:                    "id",
			isKey:                       true,
			deferInfo:                   &DeferInfo{ID: 5},
			parentFieldDeferID:          1,
			expectedOperation:           `query { user { id @__defer_internal(id: 1) name __internal_id: id @__defer_internal(id: 1) } }`,
			expectedSkipFieldsCount:     0,
			expectedRequiredFieldsCount: 1,
			expectedRemappedPaths:       map[string]string{"User.id": "__internal_id"},
		},
		{
			name: "key with defer id - different parent defer id still creates separate alias",
			definition: `
				type Query { user: User }
				type User { id: ID! name: String! }`,
			// __internal_id belongs to parent scope "1"; new planner has parentFieldDeferID="2"
			operation:          `query { user { id @__defer_internal(id: 1) name __internal_id: id @__defer_internal(id: 1) } }`,
			typeName:           "User",
			fieldSet:           "id",
			isKey:              true,
			deferInfo:          &DeferInfo{ID: 3},
			parentFieldDeferID: 2,
			// effectiveDeferID = "2" != "1" → Level 2 → creates __internal_2_id
			expectedOperation: `
				query {
					user {
						id @__defer_internal(id: 1)
						name
						__internal_id: id @__defer_internal(id: 1)
						__internal_2_id: id @__defer_internal(id: 2)
					}
				}`,
			expectedSkipFieldsCount:     1,
			expectedRequiredFieldsCount: 1,
			expectedRemappedPaths:       map[string]string{"User.id": "__internal_2_id"},
		},
		{
			name: "requires with defer id - third call with same conflict defer id reuses conflict alias",
			definition: `
				type Query { user: User }
				type User { id: ID! settings: Settings! fullName: String! account: Account! }
				type Settings { region: String! }
				type Account { type: String! }`,
			operation: `query { user {
				fullName
				__internal_settings: settings @__defer_internal(id: 1) { region @__defer_internal(id: 1) }
				__internal_2_settings: settings @__defer_internal(id: 2) { region @__defer_internal(id: 2) }
				account
			} }`,
			typeName:        "User",
			fieldSet:        "settings { region }",
			isKey:           false,
			selectionSetRef: 2, // user's inner selection set; refs 0 and 1 are the two pre-seeded settings' inner selection sets
			deferInfo:       &DeferInfo{ID: 2},
			// __internal_settings exists but defer "1" != "2"; __internal_2_settings exists with defer "2" — reuse it
			expectedOperation: `query { user {
				fullName
				__internal_settings: settings @__defer_internal(id: 1) { region @__defer_internal(id: 1) }
				__internal_2_settings: settings @__defer_internal(id: 2) { region @__defer_internal(id: 2) }
				account
			} }`,
			expectedSkipFieldsCount:     0,
			expectedRequiredFieldsCount: 2, // reused __internal_2_settings ref + reused nested region ref
			expectedRemappedPaths:       map[string]string{"User.settings": "__internal_2_settings"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(tt.definition)
			operation := unsafeparser.ParseGraphqlDocumentString(tt.operation)

			config := &addRequiredFieldsConfiguration{
				operation:                     &operation,
				definition:                    &definition,
				operationSelectionSetRef:      tt.selectionSetRef,
				isTypeNameForEntityInterface:  tt.isTypeNameForEntityInterface,
				isKey:                         tt.isKey,
				allowTypename:                 tt.allowTypename,
				typeName:                      tt.typeName,
				fieldSet:                      tt.fieldSet,
				deferInfo:                     tt.deferInfo,
				parentFieldDeferID:            tt.parentFieldDeferID,
				addTypenameInNestedSelections: tt.enforceTypenameForRequired,
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
			fragment, report := QueryPlanRequiredFieldsFragment(tt.typeName, tt.fieldName, tt.requiredFields)

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
